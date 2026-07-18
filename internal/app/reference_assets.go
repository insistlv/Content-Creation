package app

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"image-agent/internal/app/ecommerce"
)

const referenceAssetDisplayNameMaxRunes = 80

const (
	referenceAssetKindImage = "image"
	referenceAssetKindVideo = "video"
	referenceAssetKindAudio = "audio"
	referenceAssetKindAll   = "all"
)

type updateReferenceAssetRequest struct {
	DisplayName string `json:"display_name"`
}

func (a *App) handleUploadReferenceAsset(c *gin.Context) {
	user := currentUser(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "reference_asset_file_required", "请上传参考素材文件")
		return
	}
	if fileHeader.Size > a.referenceAssetUploadMaxBytes() {
		writeError(c, http.StatusRequestEntityTooLarge, "reference_asset_too_large", "参考素材文件过大")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, "reference_asset_open_failed", "参考素材读取失败")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		writeError(c, http.StatusBadRequest, "reference_asset_read_failed", "参考素材读取失败")
		return
	}

	mimeType, ok := detectSupportedReferenceAssetMimeType(content, fileHeader.Header.Get("Content-Type"), fileHeader.Filename)
	if !ok {
		writeError(c, http.StatusBadRequest, "reference_asset_invalid_type", "仅支持 PNG、JPG、WEBP 图片，MP4、WEBM、MOV 视频，以及 MP3、WAV、M4A、AAC、OGG 音频")
		return
	}

	assetKey, normalizedMimeType, err := a.assetStore.SaveBase64(base64.StdEncoding.EncodeToString(content), mimeType)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_save_failed", "参考素材保存失败")
		return
	}

	asset := ReferenceAsset{
		UserID:           user.ID,
		AssetKey:         assetKey,
		MIMEType:         normalizedMimeType,
		OriginalFilename: strings.TrimSpace(fileHeader.Filename),
		StorageScope:     StorageScopeDefault,
	}
	if asset.OriginalFilename == "" {
		asset.OriginalFilename = "reference-asset" + extensionForMimeType(mimeType)
	}
	if err := a.db.Create(&asset).Error; err != nil {
		_ = a.assetStore.Delete(assetKey)
		writeError(c, http.StatusInternalServerError, "reference_asset_create_failed", "参考素材保存失败")
		return
	}

	asset.PreviewURL = a.assetStore.PublicURL(assetKey)
	if asset.PreviewURL == "" {
		asset.PreviewURL = fmt.Sprintf("/api/reference-assets/%d/file", asset.ID)
	}
	if err := a.db.Save(&asset).Error; err != nil {
		_ = a.assetStore.Delete(assetKey)
		_ = a.db.Delete(&asset).Error
		writeError(c, http.StatusInternalServerError, "reference_asset_create_failed", "参考素材保存失败")
		return
	}

	a.applyReferenceAssetPublicURL(&asset)
	writeJSON(c, http.StatusCreated, asset)
}

func (a *App) handleListReferenceAssets(c *gin.Context) {
	user := currentUser(c)

	var items []ReferenceAsset
	if err := a.db.Where("user_id = ?", user.ID).Order("created_at desc, id desc").Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_assets_load_failed", "参考素材读取失败")
		return
	}
	kind := normalizeReferenceAssetKindQuery(c.Query("kind"))
	filtered := make([]ReferenceAsset, 0, len(items))
	for index := range items {
		a.applyReferenceAssetPublicURL(&items[index])
		if kind == referenceAssetKindAll || items[index].Kind == kind {
			filtered = append(filtered, items[index])
		}
	}

	writeJSON(c, http.StatusOK, gin.H{"items": filtered})
}

func (a *App) handleUpdateReferenceAsset(c *gin.Context) {
	asset, ok := a.findOwnedReferenceAsset(c, currentUser(c).ID)
	if !ok {
		return
	}

	var req updateReferenceAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if len([]rune(displayName)) > referenceAssetDisplayNameMaxRunes {
		writeError(c, http.StatusUnprocessableEntity, "reference_asset_display_name_too_long", "素材名称最多 80 个字符")
		return
	}

	asset.DisplayName = displayName
	if err := a.db.Model(&asset).Update("display_name", displayName).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_update_failed", "素材更新失败")
		return
	}
	a.applyReferenceAssetPublicURL(&asset)
	writeJSON(c, http.StatusOK, asset)
}

func (a *App) handleDeleteReferenceAsset(c *gin.Context) {
	asset, ok := a.findOwnedReferenceAsset(c, currentUser(c).ID)
	if !ok {
		return
	}

	var commerceReferenceCount int64
	if err := a.db.Model(&ecommerce.CommerceAsset{}).Where("reference_asset_id = ?", asset.ID).Count(&commerceReferenceCount).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_delete_failed", "参考素材删除失败")
		return
	}
	if commerceReferenceCount > 0 {
		writeError(c, http.StatusConflict, "reference_asset_in_use", "素材已被电商项目引用，不能直接删除")
		return
	}
	var referenceCount int64
	if err := a.db.Model(&GenerationReferenceAsset{}).Where("reference_asset_id = ?", asset.ID).Count(&referenceCount).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_delete_failed", "参考素材删除失败")
		return
	}
	if referenceCount > 0 {
		if err := a.db.Delete(&asset).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "reference_asset_delete_failed", "参考素材删除失败")
			return
		}
		writeJSON(c, http.StatusOK, gin.H{"ok": true})
		return
	}
	store, err := a.assetStoreForScope(asset.StorageScope)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_delete_failed", "参考素材删除失败")
		return
	}
	if err := store.Delete(asset.AssetKey); err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_delete_failed", "参考素材删除失败")
		return
	}
	if err := a.db.Unscoped().Delete(&asset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_delete_failed", "参考素材删除失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleServeReferenceAssetPreview(c *gin.Context) {
	asset, ok := a.findOwnedReferenceAsset(c, currentUser(c).ID)
	if !ok {
		return
	}
	store, err := a.assetStoreForScope(asset.StorageScope)
	if err != nil {
		writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
		return
	}
	if asset.StorageScope != StorageScopeCommercePrivate {
		if publicURL := store.PublicURL(asset.AssetKey); publicURL != "" {
			c.Redirect(http.StatusFound, publicURL)
			return
		}
	} else if signedStore, ok := store.(SignedAssetStore); ok {
		if signedURL, signErr := signedStore.SignedReadURL(asset.AssetKey, a.commerceSignedURLTTL()); signErr == nil && signedURL != "" {
			c.Redirect(http.StatusFound, signedURL)
			return
		}
	}

	content, err := store.Read(asset.AssetKey)
	if err != nil {
		writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
		return
	}

	filename := fmt.Sprintf("reference-asset-%d%s", asset.ID, extensionForMimeType(asset.MIMEType))
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	c.Data(http.StatusOK, normalizeAssetMimeType(asset.MIMEType), content)
}

func (a *App) applyReferenceAssetPublicURL(asset *ReferenceAsset) {
	if asset == nil {
		return
	}
	asset.Kind = referenceAssetKindForMIMEType(asset.MIMEType)
	if asset.StorageScope == StorageScopeCommercePrivate {
		return
	}
	store, err := a.assetStoreForScope(asset.StorageScope)
	if err != nil {
		return
	}
	if publicURL := store.PublicURL(asset.AssetKey); publicURL != "" {
		asset.PreviewURL = publicURL
	}
}

func (a *App) findOwnedReferenceAsset(c *gin.Context, userID uint) (ReferenceAsset, bool) {
	var asset ReferenceAsset
	if err := a.db.Where("id = ? AND user_id = ?", c.Param("id"), userID).First(&asset).Error; err != nil {
		writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
		return ReferenceAsset{}, false
	}
	a.applyReferenceAssetPublicURL(&asset)
	return asset, true
}

func detectSupportedImageMimeType(content []byte) (string, bool) {
	switch http.DetectContentType(content) {
	case "image/png":
		return "image/png", true
	case "image/jpeg":
		return "image/jpeg", true
	case "image/webp":
		return "image/webp", true
	default:
		return "", false
	}
}

func detectSupportedReferenceAssetMimeType(content []byte, declared, filename string) (string, bool) {
	if detectedImage, ok := detectSupportedImageMimeType(content); ok {
		return detectedImage, true
	}
	if detected, ok := normalizeReferenceAssetUploadMimeType(http.DetectContentType(content)); ok {
		if referenceAssetKindForMIMEType(detected) != referenceAssetKindImage {
			return detected, true
		}
	}
	if declared, ok := normalizeReferenceAssetUploadMimeType(declared); ok {
		if kind := referenceAssetKindForMIMEType(declared); kind == referenceAssetKindVideo || kind == referenceAssetKindAudio {
			return declared, true
		}
	}
	if extMime := referenceAssetMIMETypeForFilename(filename); extMime != "" {
		if kind := referenceAssetKindForMIMEType(extMime); kind == referenceAssetKindVideo || kind == referenceAssetKindAudio {
			return extMime, true
		}
	}
	return "", false
}

func normalizeReferenceAssetKindQuery(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case referenceAssetKindVideo:
		return referenceAssetKindVideo
	case referenceAssetKindAudio:
		return referenceAssetKindAudio
	case referenceAssetKindAll:
		return referenceAssetKindAll
	default:
		return referenceAssetKindImage
	}
}

func referenceAssetKindForMIMEType(mimeType string) string {
	value := strings.ToLower(strings.TrimSpace(mimeType))
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		value = parsed
	}
	switch {
	case strings.HasPrefix(value, "video/"):
		return referenceAssetKindVideo
	case strings.HasPrefix(value, "audio/"):
		return referenceAssetKindAudio
	default:
		return referenceAssetKindImage
	}
}

func referenceAssetMIMETypeForFilename(filename string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(filename))) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov", ".qt":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".ogg":
		return "audio/ogg"
	default:
		return ""
	}
}
