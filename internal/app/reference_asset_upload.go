package app

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultReferenceAssetUploadMaxBytes         int64 = 50 * 1024 * 1024
	defaultReferenceAssetUploadPolicyTTLSeconds       = 600
	referenceAssetUploadHeadBytes                     = 512
)

type referenceAssetUploadPolicyRequest struct {
	Filename string `json:"filename"`
	MIMEType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

type referenceAssetCompleteUploadRequest struct {
	ObjectKey   string `json:"object_key"`
	UploadToken string `json:"upload_token"`
}

type referenceAssetUploadTokenClaims struct {
	UserID           uint   `json:"user_id"`
	ProjectID        uint   `json:"project_id,omitempty"`
	StorageScope     string `json:"storage_scope,omitempty"`
	ObjectKey        string `json:"object_key"`
	MIMEType         string `json:"mime_type"`
	OriginalFilename string `json:"original_filename"`
	MaxBytes         int64  `json:"max_bytes"`
	ExpiresAt        int64  `json:"expires_at"`
	Nonce            string `json:"nonce"`
}

func (a *App) handleCreateReferenceAssetUploadPolicy(c *gin.Context) {
	if !a.referenceAssetDirectUploadEnabled() {
		writeError(c, http.StatusConflict, "reference_asset_direct_upload_unavailable", "当前存储不支持直传")
		return
	}

	var req referenceAssetUploadPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	maxBytes := a.referenceAssetUploadMaxBytes()
	mimeType, ok := validateReferenceAssetUploadRequest(req, maxBytes)
	if !ok {
		writeReferenceAssetUploadRequestError(c, req, maxBytes)
		return
	}

	user := currentUser(c)
	originalFilename := sanitizeReferenceAssetFilename(req.Filename, mimeType)
	objectKey := a.newReferenceAssetObjectKey(user.ID, mimeType, time.Now())
	expiresAt := time.Now().UTC().Add(time.Duration(a.referenceAssetUploadPolicyTTLSeconds()) * time.Second)
	policy, err := buildReferenceAssetPostPolicy(objectKey, mimeType, maxBytes, expiresAt)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_policy_failed", "直传凭证生成失败")
		return
	}
	signature := signOSSPostPolicy(policy, a.cfg.OSSAccessKeySecret)
	claims := referenceAssetUploadTokenClaims{
		UserID:           user.ID,
		ObjectKey:        objectKey,
		MIMEType:         mimeType,
		OriginalFilename: originalFilename,
		MaxBytes:         maxBytes,
		ExpiresAt:        expiresAt.Unix(),
		Nonce:            uuid.NewString(),
	}
	token, err := a.signReferenceAssetUploadToken(claims)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_policy_failed", "直传凭证生成失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"upload_url":   ossPostUploadURL(a.cfg.OSSEndpoint, a.cfg.OSSBucket),
		"object_key":   objectKey,
		"public_url":   buildOSSPublicURL(a.cfg.OSSPublicBaseURL, objectKey),
		"expires_at":   expiresAt,
		"upload_token": token,
		"form_data": gin.H{
			"key":                    objectKey,
			"policy":                 policy,
			"OSSAccessKeyId":         a.cfg.OSSAccessKeyID,
			"Signature":              signature,
			"Content-Type":           mimeType,
			"success_action_status":  "201",
			"x-oss-forbid-overwrite": "true",
		},
	})
}

func (a *App) handleCompleteReferenceAssetUpload(c *gin.Context) {
	if !a.referenceAssetDirectUploadEnabled() {
		writeError(c, http.StatusConflict, "reference_asset_direct_upload_unavailable", "当前存储不支持直传")
		return
	}

	var req referenceAssetCompleteUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	req.ObjectKey = strings.TrimSpace(req.ObjectKey)
	claims, err := a.parseReferenceAssetUploadToken(req.UploadToken)
	if err != nil {
		writeError(c, http.StatusBadRequest, "reference_asset_upload_token_invalid", "直传凭证无效")
		return
	}
	user := currentUser(c)
	if claims.UserID != user.ID || claims.ObjectKey != req.ObjectKey || !strings.HasPrefix(claims.ObjectKey, a.referenceAssetObjectPrefix(user.ID)) {
		writeError(c, http.StatusForbidden, "reference_asset_upload_token_invalid", "直传凭证无效")
		return
	}

	if err := validateCompletedReferenceAssetUpload(a.assetStore, claims, a.referenceAssetUploadMaxBytes()); err != nil {
		writeCompletedReferenceAssetUploadError(c, err)
		return
	}

	asset := ReferenceAsset{
		UserID:           user.ID,
		AssetKey:         claims.ObjectKey,
		PreviewURL:       buildOSSPublicURL(a.cfg.OSSPublicBaseURL, claims.ObjectKey),
		MIMEType:         claims.MIMEType,
		OriginalFilename: claims.OriginalFilename,
		StorageScope:     StorageScopeDefault,
	}
	if asset.PreviewURL == "" {
		asset.PreviewURL = a.assetStore.PublicURL(claims.ObjectKey)
	}
	if err := a.db.Create(&asset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_asset_create_failed", "参考素材保存失败")
		return
	}
	if asset.PreviewURL == "" {
		asset.PreviewURL = fmt.Sprintf("/api/reference-assets/%d/file", asset.ID)
		_ = a.db.Save(&asset).Error
	}

	a.applyReferenceAssetPublicURL(&asset)
	writeJSON(c, http.StatusCreated, asset)
}

func (a *App) referenceAssetDirectUploadEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(a.cfg.StorageType), "oss")
}

func (a *App) referenceAssetUploadMaxBytes() int64 {
	if a.cfg.ReferenceAssetUploadMaxBytes > 0 {
		return a.cfg.ReferenceAssetUploadMaxBytes
	}
	return defaultReferenceAssetUploadMaxBytes
}

func (a *App) referenceAssetUploadPolicyTTLSeconds() int {
	if a.cfg.ReferenceAssetUploadPolicyTTLSeconds > 0 {
		return a.cfg.ReferenceAssetUploadPolicyTTLSeconds
	}
	return defaultReferenceAssetUploadPolicyTTLSeconds
}

func (a *App) referenceAssetObjectPrefix(userID uint) string {
	return fmt.Sprintf("%sreference-assets/%d/", normalizeOSSBasePath(a.cfg.OSSBasePath), userID)
}

func (a *App) newReferenceAssetObjectKey(userID uint, mimeType string, now time.Time) string {
	now = now.UTC()
	return fmt.Sprintf(
		"%s%04d/%02d/%s%s",
		a.referenceAssetObjectPrefix(userID),
		now.Year(),
		int(now.Month()),
		uuid.NewString(),
		extensionForMimeType(mimeType),
	)
}

func normalizeReferenceAssetUploadMimeType(value string) (string, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		value = parsed
	}
	switch value {
	case "image/png":
		return "image/png", true
	case "image/jpeg", "image/jpg":
		return "image/jpeg", true
	case "image/webp":
		return "image/webp", true
	case "video/mp4":
		return "video/mp4", true
	case "video/webm":
		return "video/webm", true
	case "video/quicktime", "video/mov":
		return "video/quicktime", true
	case "audio/mpeg", "audio/mp3":
		return "audio/mpeg", true
	case "audio/wav", "audio/x-wav":
		return "audio/wav", true
	case "audio/mp4", "audio/m4a":
		return "audio/mp4", true
	case "audio/aac":
		return "audio/aac", true
	case "audio/ogg":
		return "audio/ogg", true
	case "audio/webm":
		return "audio/webm", true
	default:
		return "", false
	}
}

var (
	errReferenceAssetUploadNotFound = errors.New("reference asset upload not found")
	errReferenceAssetUploadSize     = errors.New("reference asset upload size invalid")
	errReferenceAssetUploadTooLarge = errors.New("reference asset upload too large")
	errReferenceAssetUploadMIME     = errors.New("reference asset upload MIME invalid")
)

func validateReferenceAssetUploadRequest(req referenceAssetUploadPolicyRequest, maxBytes int64) (string, bool) {
	mimeType, ok := normalizeReferenceAssetUploadMimeType(req.MIMEType)
	return mimeType, ok && req.Size > 0 && req.Size <= maxBytes
}

func writeReferenceAssetUploadRequestError(c *gin.Context, req referenceAssetUploadPolicyRequest, maxBytes int64) {
	if _, ok := normalizeReferenceAssetUploadMimeType(req.MIMEType); !ok {
		writeError(c, http.StatusBadRequest, "reference_asset_invalid_type", "仅支持 PNG、JPG、WEBP 图片，MP4、WEBM、MOV 视频，以及 MP3、WAV、M4A、AAC、OGG 音频")
		return
	}
	if req.Size <= 0 {
		writeError(c, http.StatusBadRequest, "reference_asset_size_required", "参考素材大小无效")
		return
	}
	if req.Size > maxBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "reference_asset_too_large", "参考素材文件过大")
		return
	}
	writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
}

func validateCompletedReferenceAssetUpload(store AssetStore, claims referenceAssetUploadTokenClaims, maxBytes int64) error {
	meta, err := store.ObjectMeta(claims.ObjectKey)
	if err != nil {
		return fmt.Errorf("%w: %v", errReferenceAssetUploadNotFound, err)
	}
	if meta.ContentLength <= 0 {
		return errReferenceAssetUploadSize
	}
	if meta.ContentLength > claims.MaxBytes || meta.ContentLength > maxBytes {
		return errReferenceAssetUploadTooLarge
	}
	metaMIMEType, ok := normalizeReferenceAssetUploadMimeType(meta.MIMEType)
	if !ok || metaMIMEType != claims.MIMEType {
		return errReferenceAssetUploadMIME
	}
	rangeEnd := minInt64(referenceAssetUploadHeadBytes-1, meta.ContentLength-1)
	head, err := store.ReadRange(claims.ObjectKey, 0, rangeEnd)
	if err != nil {
		return fmt.Errorf("%w: %v", errReferenceAssetUploadNotFound, err)
	}
	if referenceAssetKindForMIMEType(claims.MIMEType) == referenceAssetKindImage {
		detectedMIMEType, ok := detectSupportedImageMimeType(head)
		if !ok || detectedMIMEType != claims.MIMEType {
			return errReferenceAssetUploadMIME
		}
	}
	return nil
}

func writeCompletedReferenceAssetUploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errReferenceAssetUploadTooLarge):
		writeError(c, http.StatusRequestEntityTooLarge, "reference_asset_too_large", "参考素材文件过大")
	case errors.Is(err, errReferenceAssetUploadMIME):
		writeError(c, http.StatusBadRequest, "reference_asset_invalid_type", "仅支持 PNG、JPG、WEBP 图片，MP4、WEBM、MOV 视频，以及 MP3、WAV、M4A、AAC、OGG 音频")
	case errors.Is(err, errReferenceAssetUploadSize):
		writeError(c, http.StatusBadRequest, "reference_asset_size_required", "参考素材大小无效")
	default:
		writeError(c, http.StatusBadRequest, "reference_asset_upload_not_found", "直传文件不存在")
	}
}

func newUploadTokenNonce() string {
	return uuid.NewString()
}

func sanitizeReferenceAssetFilename(filename, mimeType string) string {
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	filename = filepath.Base(filename)
	if filename == "." || filename == "/" || filename == "" {
		return "reference-asset" + extensionForMimeType(mimeType)
	}
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)
}

func buildReferenceAssetPostPolicy(objectKey, mimeType string, maxBytes int64, expiresAt time.Time) (string, error) {
	policy := struct {
		Expiration string `json:"expiration"`
		Conditions []any  `json:"conditions"`
	}{
		Expiration: expiresAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		Conditions: []any{
			[]any{"eq", "$key", objectKey},
			[]any{"content-length-range", 1, maxBytes},
			[]any{"eq", "$Content-Type", mimeType},
			[]any{"eq", "$x-oss-forbid-overwrite", "true"},
		},
	}
	content, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

func signOSSPostPolicy(policy, accessKeySecret string) string {
	mac := hmac.New(sha1.New, []byte(accessKeySecret))
	mac.Write([]byte(policy))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func ossPostUploadURL(endpoint, bucket string) string {
	rawEndpoint := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if rawEndpoint == "" || strings.TrimSpace(bucket) == "" {
		return rawEndpoint
	}
	if !strings.Contains(rawEndpoint, "://") {
		rawEndpoint = "https://" + rawEndpoint
	}
	parsed, err := url.Parse(rawEndpoint)
	if err != nil || parsed.Host == "" {
		return rawEndpoint
	}
	bucket = strings.TrimSpace(bucket)
	if !strings.HasPrefix(strings.ToLower(parsed.Host), strings.ToLower(bucket)+".") {
		parsed.Host = bucket + "." + parsed.Host
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func (a *App) signReferenceAssetUploadToken(claims referenceAssetUploadTokenClaims) (string, error) {
	if strings.TrimSpace(a.cfg.JWTSecret) == "" {
		return "", errors.New("JWTSecret is required")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	signature := signReferenceAssetUploadTokenPart(payloadPart, a.cfg.JWTSecret)
	return payloadPart + "." + signature, nil
}

func (a *App) parseReferenceAssetUploadToken(token string) (referenceAssetUploadTokenClaims, error) {
	var claims referenceAssetUploadTokenClaims
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return claims, errors.New("invalid token format")
	}
	expectedSignature := signReferenceAssetUploadTokenPart(parts[0], a.cfg.JWTSecret)
	if subtle.ConstantTimeCompare([]byte(expectedSignature), []byte(parts[1])) != 1 {
		return claims, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	if claims.ExpiresAt <= time.Now().UTC().Unix() {
		return claims, errors.New("token expired")
	}
	if claims.UserID == 0 || strings.TrimSpace(claims.ObjectKey) == "" || strings.TrimSpace(claims.MIMEType) == "" || claims.MaxBytes <= 0 {
		return claims, errors.New("token incomplete")
	}
	return claims, nil
}

func signReferenceAssetUploadTokenPart(payloadPart, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
