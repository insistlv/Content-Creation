package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

const defaultCommerceSignedURLTTLSeconds = 900

type commerceAssetCompleteUploadRequest struct {
	ObjectKey   string `json:"object_key"`
	UploadToken string `json:"upload_token"`
	SKUID       *uint  `json:"sku_id"`
	Role        string `json:"role"`
	Lifecycle   string `json:"lifecycle"`
	SortOrder   int    `json:"sort_order"`
}

type commerceAssetWithReference struct {
	ecommerce.CommerceAsset
	AssetKey         string `gorm:"column:asset_key"`
	PreviewURL       string `gorm:"column:preview_url"`
	MIMEType         string `gorm:"column:mime_type"`
	OriginalFilename string `gorm:"column:original_filename"`
	StorageScope     string `gorm:"column:storage_scope"`
}

func buildCommercePrivateAssetStore(cfg Config) (AssetStore, error) {
	if err := validateCommercePrivateStorageConfig(cfg); err != nil {
		return nil, err
	}
	storageType := strings.ToLower(strings.TrimSpace(cfg.AICommercePrivateStorageType))
	if storageType == "" {
		storageType = "local"
	}
	switch storageType {
	case "local":
		root := strings.TrimSpace(cfg.AICommercePrivateAssetPath)
		if root == "" {
			if strings.TrimSpace(cfg.AssetStoragePath) != "" {
				root = filepath.Join(filepath.Dir(cfg.AssetStoragePath), "commerce-assets")
			} else {
				root = filepath.Join("data", "commerce-assets")
			}
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, fmt.Errorf("create commerce private asset directory: %w", err)
		}
		return NewLocalAssetStore(root), nil
	case "oss":
		store, err := NewOSSAssetStore(
			cfg.AICommerceOSSEndpoint,
			cfg.AICommerceOSSAccessKeyID,
			cfg.AICommerceOSSAccessKeySecret,
			cfg.AICommerceOSSBucket,
			cfg.AICommerceOSSBasePath,
			"",
		)
		if err != nil {
			return nil, fmt.Errorf("create commerce private OSS asset store: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported AI_COMMERCE_PRIVATE_STORAGE_TYPE %q", cfg.AICommercePrivateStorageType)
	}
}

func validateCommercePrivateStorageConfig(cfg Config) error {
	storageType := strings.ToLower(strings.TrimSpace(cfg.AICommercePrivateStorageType))
	if storageType == "" {
		storageType = "local"
	}
	if cfg.AICommerceEnabled && storageType != "oss" {
		return errors.New("AI_COMMERCE_ENABLED requires AI_COMMERCE_PRIVATE_STORAGE_TYPE=oss")
	}
	if storageType != "oss" {
		return nil
	}
	required := []struct {
		name, value string
	}{
		{"AI_COMMERCE_OSS_ENDPOINT", cfg.AICommerceOSSEndpoint},
		{"AI_COMMERCE_OSS_ACCESS_KEY_ID", cfg.AICommerceOSSAccessKeyID},
		{"AI_COMMERCE_OSS_ACCESS_KEY_SECRET", cfg.AICommerceOSSAccessKeySecret},
		{"AI_COMMERCE_OSS_BUCKET", cfg.AICommerceOSSBucket},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("commerce private OSS requires %s", item.name)
		}
	}
	return nil
}

func (a *App) assetStoreForScope(scope string) (AssetStore, error) {
	if strings.TrimSpace(scope) == "" || strings.TrimSpace(scope) == StorageScopeDefault {
		if a.assetStore != nil {
			return a.assetStore, nil
		}
	}
	return a.assetStores.ForScope(scope)
}

func (a *App) referenceAssetAccessURL(asset ReferenceAsset, fallbackMIMEType string, allowSigned, allowStoredPreview bool) (string, error) {
	store, err := a.assetStoreForScope(asset.StorageScope)
	if err != nil {
		return "", err
	}
	private := strings.TrimSpace(asset.StorageScope) == StorageScopeCommercePrivate
	if private && allowSigned {
		if signedStore, ok := store.(SignedAssetStore); ok {
			signedURL, signErr := signedStore.SignedReadURL(asset.AssetKey, a.commerceSignedURLTTL())
			if signErr != nil {
				return "", signErr
			}
			if strings.TrimSpace(signedURL) != "" {
				return strings.TrimSpace(signedURL), nil
			}
		}
	}
	if !private {
		if publicURL := strings.TrimSpace(store.PublicURL(asset.AssetKey)); publicURL != "" {
			return publicURL, nil
		}
		if allowStoredPreview && strings.TrimSpace(asset.PreviewURL) != "" {
			return strings.TrimSpace(asset.PreviewURL), nil
		}
	}
	content, err := store.Read(asset.AssetKey)
	if err != nil {
		return "", err
	}
	mimeType := fallbackString(asset.MIMEType, fallbackMIMEType)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(content)), nil
}

func (a *App) handleCreateCommerceAssetUploadPolicy(c *gin.Context) {
	if !a.commerceAssetDirectUploadEnabled() {
		writeError(c, http.StatusConflict, "commerce_asset_direct_upload_unavailable", "当前私有存储不支持直传")
		return
	}
	projectID, ok := a.requireOwnedCommerceProject(c)
	if !ok {
		return
	}
	var req referenceAssetUploadPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	mimeType, ok := validateReferenceAssetUploadRequest(req, a.referenceAssetUploadMaxBytes())
	if !ok {
		writeReferenceAssetUploadRequestError(c, req, a.referenceAssetUploadMaxBytes())
		return
	}
	user := currentUser(c)
	originalFilename := sanitizeReferenceAssetFilename(req.Filename, mimeType)
	objectKey := a.newCommerceAssetObjectKey(user.ID, projectID, mimeType, time.Now())
	if err := a.commerceAssets.EnsureObjectGuard(c.Request.Context(), user.ID, StorageScopeCommercePrivate, objectKey); err != nil {
		writeError(c, http.StatusConflict, "commerce_asset_object_unavailable", "素材对象不可用")
		return
	}
	expiresAt := time.Now().UTC().Add(time.Duration(a.referenceAssetUploadPolicyTTLSeconds()) * time.Second)
	policy, err := buildReferenceAssetPostPolicy(objectKey, mimeType, a.referenceAssetUploadMaxBytes(), expiresAt)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "commerce_asset_policy_failed", "直传凭证生成失败")
		return
	}
	claims := referenceAssetUploadTokenClaims{
		UserID: user.ID, ProjectID: projectID, StorageScope: StorageScopeCommercePrivate,
		ObjectKey: objectKey, MIMEType: mimeType, OriginalFilename: originalFilename,
		MaxBytes: a.referenceAssetUploadMaxBytes(), ExpiresAt: expiresAt.Unix(), Nonce: newUploadTokenNonce(),
	}
	token, err := a.signReferenceAssetUploadToken(claims)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "commerce_asset_policy_failed", "直传凭证生成失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"upload_url":   ossPostUploadURL(a.cfg.AICommerceOSSEndpoint, a.cfg.AICommerceOSSBucket),
		"object_key":   objectKey,
		"expires_at":   expiresAt,
		"upload_token": token,
		"form_data": gin.H{
			"key": objectKey, "policy": policy,
			"OSSAccessKeyId": a.cfg.AICommerceOSSAccessKeyID,
			"Signature":      signOSSPostPolicy(policy, a.cfg.AICommerceOSSAccessKeySecret),
			"Content-Type":   mimeType, "success_action_status": "201", "x-oss-forbid-overwrite": "true",
		},
	})
}

func (a *App) handleCompleteCommerceAssetUpload(c *gin.Context) {
	if !a.commerceAssetCompletionEnabled() {
		writeError(c, http.StatusConflict, "commerce_asset_direct_upload_unavailable", "当前私有存储不支持直传")
		return
	}
	projectID, ok := a.requireOwnedCommerceProject(c)
	if !ok {
		return
	}
	var req commerceAssetCompleteUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	req.ObjectKey = strings.TrimSpace(req.ObjectKey)
	claims, err := a.parseReferenceAssetUploadToken(req.UploadToken)
	if err != nil {
		writeError(c, http.StatusBadRequest, "commerce_asset_upload_token_invalid", "直传凭证无效")
		return
	}
	user := currentUser(c)
	if claims.UserID != user.ID || claims.ProjectID != projectID || claims.StorageScope != StorageScopeCommercePrivate || claims.ObjectKey != req.ObjectKey || !strings.HasPrefix(claims.ObjectKey, a.commerceAssetObjectPrefix(user.ID, projectID)) {
		writeError(c, http.StatusForbidden, "commerce_asset_upload_token_invalid", "直传凭证无效")
		return
	}
	if err := a.commerceAssets.EnsureObjectGuard(c.Request.Context(), user.ID, StorageScopeCommercePrivate, claims.ObjectKey); err != nil {
		writeError(c, http.StatusConflict, "commerce_asset_object_unavailable", "素材对象不可用")
		return
	}
	if handled := a.handleCommerceUploadReplay(c, user.ID, projectID, claims, req); handled {
		return
	}
	store, err := a.assetStoreForScope(StorageScopeCommercePrivate)
	if err != nil {
		writeError(c, http.StatusConflict, "commerce_private_storage_unavailable", "私有存储不可用")
		return
	}
	if err := validateCompletedReferenceAssetUpload(store, claims, a.referenceAssetUploadMaxBytes()); err != nil {
		a.scheduleCommerceOrphanCleanup(c.Request.Context(), user.ID, projectID, claims.ObjectKey, "upload_validation_failed")
		writeCompletedReferenceAssetUploadError(c, err)
		return
	}

	role, lifecycle, validLifecycle := normalizedCommerceAssetBinding(req)
	if !validLifecycle {
		a.scheduleCommerceOrphanCleanup(c.Request.Context(), user.ID, projectID, claims.ObjectKey, "invalid_lifecycle")
		writeError(c, http.StatusUnprocessableEntity, "invalid_input", "素材生命周期无效")
		return
	}
	var reference ReferenceAsset
	var asset ecommerce.CommerceAsset
	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		reference = ReferenceAsset{
			UserID: user.ID, AssetKey: claims.ObjectKey, MIMEType: claims.MIMEType,
			OriginalFilename: claims.OriginalFilename, StorageScope: StorageScopeCommercePrivate,
		}
		if err := tx.Create(&reference).Error; err != nil {
			return err
		}
		asset = ecommerce.CommerceAsset{
			UserID: user.ID, ProjectID: projectID, ReferenceAssetID: reference.ID,
			SKUID: req.SKUID, Role: role, Lifecycle: lifecycle, SortOrder: req.SortOrder,
		}
		if lifecycle == ecommerce.AssetLifecycleTemporary {
			retentionHours := a.cfg.AICommerceTempRetentionHours
			if retentionHours <= 0 {
				retentionHours = 168
			}
			retainUntil := time.Now().UTC().Add(time.Duration(retentionHours) * time.Hour)
			asset.RetainUntil = &retainUntil
		}
		if err := tx.Create(&asset).Error; err != nil {
			return err
		}
		reference.PreviewURL = fmt.Sprintf("/api/ecommerce/assets/%d/file", asset.ID)
		return tx.Model(&ReferenceAsset{}).Where("id = ?", reference.ID).Update("preview_url", reference.PreviewURL).Error
	})
	if err != nil {
		if handled := a.handleCommerceUploadReplay(c, user.ID, projectID, claims, req); handled {
			return
		}
		a.scheduleCommerceOrphanCleanup(c.Request.Context(), user.ID, projectID, claims.ObjectKey, "complete_upload_transaction_failed")
		writeError(c, http.StatusInternalServerError, "commerce_asset_create_failed", "素材保存失败")
		return
	}
	writeJSON(c, http.StatusCreated, commerceAssetPayload(asset, reference))
}

func (a *App) handleListCommerceAssets(c *gin.Context) {
	projectID, ok := a.requireOwnedCommerceProject(c)
	if !ok {
		return
	}
	user := currentUser(c)
	items, err := a.loadCommerceAssets(c.Request.Context(), user.ID, projectID, 0)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "commerce_assets_load_failed", "素材读取失败")
		return
	}
	payloads := make([]gin.H, 0, len(items))
	for _, item := range items {
		payloads = append(payloads, commerceAssetPayload(item.CommerceAsset, referenceFromCommerceAsset(item)))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": payloads})
}

func (a *App) handleDeleteCommerceAsset(c *gin.Context) {
	projectID, ok := a.requireOwnedCommerceProject(c)
	if !ok {
		return
	}
	assetID, err := strconv.ParseUint(strings.TrimSpace(c.Param("asset_id")), 10, 64)
	if err != nil || assetID == 0 {
		writeError(c, http.StatusNotFound, "commerce_asset_not_found", "素材不存在")
		return
	}
	user := currentUser(c)
	items, err := a.loadCommerceAssets(c.Request.Context(), user.ID, projectID, uint(assetID))
	if err != nil || len(items) != 1 {
		writeError(c, http.StatusNotFound, "commerce_asset_not_found", "素材不存在")
		return
	}
	item := items[0]
	lease, err := a.commerceAssets.BeginObjectDeletion(c.Request.Context(), item.UserID, item.StorageScope, item.AssetKey, &item.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "commerce_asset_delete_failed", "素材删除失败")
		return
	}
	if lease.References.HasReferences() {
		if err := a.db.WithContext(c.Request.Context()).Delete(&ecommerce.CommerceAsset{}, item.ID).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "commerce_asset_delete_failed", "素材删除失败")
			return
		}
		writeJSON(c, http.StatusOK, gin.H{"ok": true, "object_retained": true})
		return
	}
	store, err := a.assetStoreForScope(item.StorageScope)
	if err != nil {
		_ = a.commerceAssets.ReleaseObjectDeletion(c.Request.Context(), item.UserID, item.StorageScope, item.AssetKey, lease.Token)
		writeError(c, http.StatusInternalServerError, "commerce_asset_delete_failed", "素材删除失败")
		return
	}
	if err := store.Delete(item.AssetKey); err != nil {
		_ = a.commerceAssets.ReleaseObjectDeletion(c.Request.Context(), item.UserID, item.StorageScope, item.AssetKey, lease.Token)
		writeError(c, http.StatusInternalServerError, "commerce_asset_delete_failed", "素材删除失败")
		return
	}
	now := time.Now().UTC()
	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := a.commerceAssets.CompleteObjectDeletionTx(tx, item.UserID, item.StorageScope, item.AssetKey, lease.Token); err != nil {
			return err
		}
		if err := tx.Model(&ecommerce.CommerceAsset{}).Where("id = ? AND user_id = ? AND project_id = ?", item.ID, user.ID, projectID).Update("object_deleted_at", now).Error; err != nil {
			return err
		}
		if err := tx.Delete(&ecommerce.CommerceAsset{}, item.ID).Error; err != nil {
			return err
		}
		return tx.Delete(&ReferenceAsset{}, item.ReferenceAssetID).Error
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "commerce_asset_delete_failed", "素材删除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleServeCommerceAssetFile(c *gin.Context) {
	assetID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || assetID == 0 {
		writeError(c, http.StatusNotFound, "commerce_asset_not_found", "素材不存在")
		return
	}
	user := currentUser(c)
	items, err := a.loadCommerceAssets(c.Request.Context(), user.ID, 0, uint(assetID))
	if err != nil || len(items) != 1 {
		writeError(c, http.StatusNotFound, "commerce_asset_not_found", "素材不存在")
		return
	}
	item := items[0]
	store, err := a.assetStoreForScope(item.StorageScope)
	if err != nil {
		writeError(c, http.StatusNotFound, "commerce_asset_not_found", "素材不存在")
		return
	}
	if signedStore, ok := store.(SignedAssetStore); ok {
		ttl := a.commerceSignedURLTTL()
		signedURL, err := signedStore.SignedReadURL(item.AssetKey, ttl)
		if err == nil && strings.TrimSpace(signedURL) != "" {
			c.Redirect(http.StatusFound, signedURL)
			return
		}
	}
	content, err := store.Read(item.AssetKey)
	if err != nil {
		writeError(c, http.StatusNotFound, "commerce_asset_not_found", "素材不存在")
		return
	}
	filename := strings.TrimSpace(item.OriginalFilename)
	if filename == "" {
		filename = fmt.Sprintf("commerce-asset-%d%s", item.ID, extensionForMimeType(item.MIMEType))
	}
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	c.Data(http.StatusOK, normalizeAssetMimeType(item.MIMEType), content)
}

func (a *App) requireOwnedCommerceProject(c *gin.Context) (uint, bool) {
	projectID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || projectID == 0 {
		writeError(c, http.StatusNotFound, "commerce_project_not_found", "项目不存在")
		return 0, false
	}
	if _, err := a.commerceService.GetProject(c.Request.Context(), currentUser(c).ID, uint(projectID)); err != nil {
		writeError(c, http.StatusNotFound, "commerce_project_not_found", "项目不存在")
		return 0, false
	}
	return uint(projectID), true
}

func (a *App) loadCommerceAssets(ctx context.Context, userID, projectID, assetID uint) ([]commerceAssetWithReference, error) {
	query := a.db.WithContext(ctx).Table("commerce_assets AS ca").
		Select("ca.*, ra.asset_key, ra.preview_url, ra.mime_type, ra.original_filename, ra.storage_scope").
		Joins("JOIN reference_assets AS ra ON ra.id = ca.reference_asset_id AND ra.deleted_at IS NULL").
		Where("ca.user_id = ? AND ca.deleted_at IS NULL", userID)
	if projectID != 0 {
		query = query.Where("ca.project_id = ?", projectID)
	}
	if assetID != 0 {
		query = query.Where("ca.id = ?", assetID)
	}
	var items []commerceAssetWithReference
	err := query.Order("ca.sort_order asc, ca.id asc").Scan(&items).Error
	return items, err
}

func (a *App) commerceAssetDirectUploadEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(a.cfg.AICommercePrivateStorageType), "oss") &&
		strings.TrimSpace(a.cfg.AICommerceOSSEndpoint) != "" && strings.TrimSpace(a.cfg.AICommerceOSSAccessKeyID) != "" &&
		strings.TrimSpace(a.cfg.AICommerceOSSAccessKeySecret) != "" && strings.TrimSpace(a.cfg.AICommerceOSSBucket) != ""
}

func (a *App) commerceAssetCompletionEnabled() bool {
	if !strings.EqualFold(strings.TrimSpace(a.cfg.AICommercePrivateStorageType), "oss") {
		return false
	}
	_, err := a.assetStoreForScope(StorageScopeCommercePrivate)
	return err == nil
}

func (a *App) commerceAssetObjectPrefix(userID, projectID uint) string {
	basePath := normalizeOSSBasePath(a.cfg.AICommerceOSSBasePath)
	if basePath == "" {
		basePath = "commerce/"
	}
	return fmt.Sprintf("%s%d/%d/", basePath, userID, projectID)
}

func (a *App) newCommerceAssetObjectKey(userID, projectID uint, mimeType string, now time.Time) string {
	now = now.UTC()
	return fmt.Sprintf("%s%04d/%02d/%s%s", a.commerceAssetObjectPrefix(userID, projectID), now.Year(), int(now.Month()), newUploadTokenNonce(), extensionForMimeType(mimeType))
}

func (a *App) commerceSignedURLTTL() time.Duration {
	seconds := a.cfg.AICommerceSignedURLTTLSeconds
	if seconds <= 0 {
		seconds = defaultCommerceSignedURLTTLSeconds
	}
	if seconds > int(maxSignedAssetURLTTL/time.Second) {
		seconds = int(maxSignedAssetURLTTL / time.Second)
	}
	return time.Duration(seconds) * time.Second
}

func (a *App) scheduleCommerceOrphanCleanup(ctx context.Context, userID, projectID uint, objectKey, reason string) {
	if a.commerceAssets == nil {
		return
	}
	if err := a.commerceAssets.EnsureObjectGuard(ctx, userID, StorageScopeCommercePrivate, objectKey); err != nil {
		return
	}
	references, err := a.commerceAssets.InspectObjectReferences(ctx, StorageScopeCommercePrivate, objectKey, nil)
	if err != nil || references.HasReferences() {
		return
	}
	var boundCount int64
	if err := a.db.WithContext(ctx).Model(&ReferenceAsset{}).
		Where("user_id = ? AND storage_scope = ? AND asset_key = ?", userID, StorageScopeCommercePrivate, strings.TrimSpace(objectKey)).
		Count(&boundCount).Error; err != nil || boundCount > 0 {
		return
	}
	_, _ = a.commerceAssets.ScheduleOrphanCleanup(ctx, ecommerce.OrphanCleanupInput{
		UserID: userID, ProjectID: projectID, StorageScope: StorageScopeCommercePrivate,
		ObjectKey: objectKey, Reason: reason, DeleteAfter: time.Now().UTC().Add(24 * time.Hour),
	})
}

func (a *App) queueExpiredCommerceAssets(ctx context.Context) (int, error) {
	if a.commerceAssets == nil {
		return 0, errors.New("commerce asset service unavailable")
	}
	return a.commerceAssets.QueueExpiredTemporaryAssets(ctx)
}

func (a *App) processDueCommerceObjectCleanups(ctx context.Context, limit int) (int, error) {
	if a.commerceAssets == nil {
		return 0, errors.New("commerce asset service unavailable")
	}
	cleanups, err := a.commerceAssets.DueCleanups(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, cleanup := range cleanups {
		lease, err := a.commerceAssets.BeginObjectDeletion(ctx, cleanup.UserID, cleanup.StorageScope, cleanup.ObjectKey, cleanup.CommerceAssetID)
		if err != nil {
			if errors.Is(err, ecommerce.ErrObjectDeletionBusy) || errors.Is(err, ecommerce.ErrObjectGuardUnavailable) {
				if recordErr := a.commerceAssets.RecordCleanupProtected(ctx, cleanup.ID, err.Error()); recordErr != nil {
					return processed, recordErr
				}
				continue
			}
			return processed, err
		}
		if lease.References.HasReferences() {
			if err := a.commerceAssets.RecordCleanupProtected(ctx, cleanup.ID, "object remains referenced"); err != nil {
				return processed, err
			}
			continue
		}
		store, err := a.assetStoreForScope(cleanup.StorageScope)
		if err == nil {
			err = store.Delete(cleanup.ObjectKey)
		}
		if err != nil {
			if releaseErr := a.commerceAssets.ReleaseObjectDeletion(ctx, cleanup.UserID, cleanup.StorageScope, cleanup.ObjectKey, lease.Token); releaseErr != nil {
				return processed, releaseErr
			}
			if recordErr := a.commerceAssets.RecordCleanupFailure(ctx, cleanup.ID, err); recordErr != nil {
				return processed, recordErr
			}
			continue
		}
		if err := a.commerceAssets.RecordCleanupSuccessWithGuard(ctx, cleanup.ID, lease.Token); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func normalizedCommerceAssetBinding(req commerceAssetCompleteUploadRequest) (string, string, bool) {
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "reference"
	}
	lifecycle := strings.ToLower(strings.TrimSpace(req.Lifecycle))
	if lifecycle == "" {
		lifecycle = ecommerce.AssetLifecycleProject
	}
	valid := lifecycle == ecommerce.AssetLifecycleProject || lifecycle == ecommerce.AssetLifecycleTemporary
	return role, lifecycle, valid
}

func (a *App) handleCommerceUploadReplay(c *gin.Context, userID, projectID uint, claims referenceAssetUploadTokenClaims, req commerceAssetCompleteUploadRequest) bool {
	var reference ReferenceAsset
	err := a.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND storage_scope = ? AND asset_key = ?", userID, claims.StorageScope, claims.ObjectKey).
		First(&reference).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "commerce_asset_create_failed", "素材保存失败")
		return true
	}
	var asset ecommerce.CommerceAsset
	err = a.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND project_id = ? AND reference_asset_id = ?", userID, projectID, reference.ID).
		First(&asset).Error
	if err == nil && commerceUploadReplayMatches(reference, asset, claims, req) {
		writeJSON(c, http.StatusOK, commerceAssetPayload(asset, reference))
		return true
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusInternalServerError, "commerce_asset_create_failed", "素材保存失败")
		return true
	}
	writeError(c, http.StatusConflict, "commerce_asset_upload_replay_conflict", "该对象已绑定，上传参数不一致")
	return true
}

func commerceUploadReplayMatches(reference ReferenceAsset, asset ecommerce.CommerceAsset, claims referenceAssetUploadTokenClaims, req commerceAssetCompleteUploadRequest) bool {
	role, lifecycle, valid := normalizedCommerceAssetBinding(req)
	if !valid || reference.MIMEType != claims.MIMEType || reference.OriginalFilename != claims.OriginalFilename {
		return false
	}
	if asset.Role != role || asset.Lifecycle != lifecycle || asset.SortOrder != req.SortOrder {
		return false
	}
	return equalOptionalUint(asset.SKUID, req.SKUID)
}

func equalOptionalUint(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func commerceAssetPayload(asset ecommerce.CommerceAsset, reference ReferenceAsset) gin.H {
	return gin.H{
		"id": asset.ID, "project_id": asset.ProjectID, "reference_asset_id": asset.ReferenceAssetID,
		"sku_id": asset.SKUID, "role": asset.Role, "lifecycle": asset.Lifecycle, "sort_order": asset.SortOrder,
		"metadata": decodeCommerceJSON(asset.MetadataJSON), "retain_until": asset.RetainUntil,
		"preview_url": fmt.Sprintf("/api/ecommerce/assets/%d/file", asset.ID),
		"mime_type":   reference.MIMEType, "original_filename": reference.OriginalFilename,
		"created_at": asset.CreatedAt, "updated_at": asset.UpdatedAt,
	}
}

func referenceFromCommerceAsset(item commerceAssetWithReference) ReferenceAsset {
	return ReferenceAsset{
		ID: item.ReferenceAssetID, UserID: item.UserID, AssetKey: item.AssetKey,
		PreviewURL: item.PreviewURL, MIMEType: item.MIMEType, OriginalFilename: item.OriginalFilename,
		StorageScope: item.StorageScope,
	}
}
