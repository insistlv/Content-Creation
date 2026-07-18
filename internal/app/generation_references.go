package app

// 本文件从 generation.go 拆分：参考素材、源图、蒙版与扩图画布的加载与构建。

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	// webp 注册 image.Decode 的解码器，供参考图/源图加载使用。
	_ "golang.org/x/image/webp"
)

func (a *App) prepareReferenceAssets(c *gin.Context, userID uint, ids []uint) ([]ReferenceAsset, bool) {
	return a.prepareReferenceAssetsWithLimitAndKind(c, userID, ids, 4, referenceAssetKindImage)
}

func (a *App) prepareReferenceAssetsWithLimit(c *gin.Context, userID uint, ids []uint, limit int) ([]ReferenceAsset, bool) {
	return a.prepareReferenceAssetsWithLimitAndKind(c, userID, ids, limit, referenceAssetKindImage)
}

func (a *App) prepareReferenceAssetsWithLimitAndKind(c *gin.Context, userID uint, ids []uint, limit int, kind string) ([]ReferenceAsset, bool) {
	if len(ids) == 0 {
		return nil, true
	}
	if limit <= 0 {
		limit = 4
	}
	if len(ids) > limit {
		writeError(c, http.StatusUnprocessableEntity, "reference_asset_limit_exceeded", "最多只能选择 4 张参考图")
		return nil, false
	}

	uniqueIDs := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			writeError(c, http.StatusBadRequest, "invalid_reference_asset", "参考素材无效")
			return nil, false
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}

	var assets []ReferenceAsset
	if err := a.db.Where("user_id = ? AND id IN ?", userID, uniqueIDs).Find(&assets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_assets_load_failed", "参考素材读取失败")
		return nil, false
	}
	if len(assets) != len(uniqueIDs) {
		writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
		return nil, false
	}

	byID := make(map[uint]ReferenceAsset, len(assets))
	for _, asset := range assets {
		a.applyReferenceAssetPublicURL(&asset)
		if kind != referenceAssetKindAll && asset.Kind != kind {
			writeError(c, http.StatusBadRequest, "invalid_reference_asset_type", "参考素材类型不匹配")
			return nil, false
		}
		byID[asset.ID] = asset
	}

	ordered := make([]ReferenceAsset, 0, len(ids))
	for _, id := range ids {
		asset, exists := byID[id]
		if !exists {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
			return nil, false
		}
		ordered = append(ordered, asset)
	}

	return ordered, true
}

func (a *App) prepareReferenceWorks(c *gin.Context, userID uint, ids []uint, existingReferenceCount int) ([]Work, bool) {
	if len(ids) == 0 {
		return nil, true
	}
	if existingReferenceCount+len(ids) > 4 {
		writeError(c, http.StatusUnprocessableEntity, "reference_asset_limit_exceeded", "最多只能选择 4 张参考图")
		return nil, false
	}

	uniqueIDs := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			writeError(c, http.StatusBadRequest, "invalid_reference_work", "参考作品无效")
			return nil, false
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if existingReferenceCount+len(uniqueIDs) > 4 {
		writeError(c, http.StatusUnprocessableEntity, "reference_asset_limit_exceeded", "最多只能选择 4 张参考图")
		return nil, false
	}

	var works []Work
	if err := a.db.Where("user_id = ? AND id IN ?", userID, uniqueIDs).Find(&works).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "reference_works_load_failed", "参考作品读取失败")
		return nil, false
	}
	if len(works) != len(uniqueIDs) {
		writeError(c, http.StatusNotFound, "reference_work_not_found", "参考作品不存在")
		return nil, false
	}

	byID := make(map[uint]Work, len(works))
	for _, work := range works {
		if normalizeWorkCategory(work.Category) != WorkCategoryImage || strings.TrimSpace(work.AssetKey) == "" {
			writeError(c, http.StatusBadRequest, "invalid_reference_work", "参考作品无效")
			return nil, false
		}
		byID[work.ID] = work
	}

	ordered := make([]Work, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		work, exists := byID[id]
		if !exists {
			writeError(c, http.StatusNotFound, "reference_work_not_found", "参考作品不存在")
			return nil, false
		}
		ordered = append(ordered, work)
	}

	return ordered, true
}

func (a *App) prepareSourceWork(c *gin.Context, userID uint, id *uint) (*Work, bool) {
	if id == nil || *id == 0 {
		return nil, true
	}

	var work Work
	if err := a.db.Where("id = ? AND user_id = ?", *id, userID).First(&work).Error; err != nil {
		writeError(c, http.StatusNotFound, "source_work_not_found", "源作品不存在")
		return nil, false
	}
	return &work, true
}

func (a *App) buildReferenceImages(assets []ReferenceAsset) ([]ReferenceImageInput, error) {
	if len(assets) == 0 {
		return nil, nil
	}

	images := make([]ReferenceImageInput, 0, len(assets))
	for _, asset := range assets {
		image, err := a.buildReferenceImageInput(asset.AssetKey, asset.MIMEType, asset.StorageScope)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

func (a *App) buildReferenceImagesFromWorks(works []Work) ([]ReferenceImageInput, error) {
	if len(works) == 0 {
		return nil, nil
	}

	images := make([]ReferenceImageInput, 0, len(works))
	for _, work := range works {
		image, err := a.buildReferenceImageInput(work.AssetKey, fallbackString(work.MIMEType, "image/png"), work.StorageScope)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

func (a *App) buildSourceImage(work *Work) (*ReferenceImageInput, error) {
	if work == nil {
		return nil, nil
	}

	image, err := a.buildReferenceImageInput(work.AssetKey, work.MIMEType, work.StorageScope)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

func (a *App) buildMaskImage(userID uint, maskAssetID *uint) (*ReferenceImageInput, error) {
	if maskAssetID == nil || *maskAssetID == 0 {
		return nil, nil
	}
	var asset ReferenceAsset
	if err := a.db.Where("id = ? AND user_id = ?", *maskAssetID, userID).First(&asset).Error; err != nil {
		return nil, err
	}
	image, err := a.buildReferenceImageInput(asset.AssetKey, asset.MIMEType, asset.StorageScope)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

func (a *App) buildReferenceImageInput(assetKey, mimeType, storageScope string) (ReferenceImageInput, error) {
	store, err := a.assetStoreForScope(storageScope)
	if err != nil {
		return ReferenceImageInput{}, err
	}
	reader, err := store.Open(assetKey)
	if err != nil {
		return ReferenceImageInput{}, err
	}
	defer reader.Close()
	spoolPath := strings.TrimSpace(a.cfg.GenerationSpoolPath)
	if spoolPath == "" {
		spoolPath = os.TempDir()
	}
	if err := os.MkdirAll(spoolPath, 0o750); err != nil {
		return ReferenceImageInput{}, err
	}
	temporary, err := os.CreateTemp(spoolPath, "reference-*.partial")
	if err != nil {
		return ReferenceImageInput{}, err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	written, err := io.Copy(temporary, io.LimitReader(reader, (64<<20)+1))
	if err != nil {
		return ReferenceImageInput{}, err
	}
	if written > 64<<20 {
		return ReferenceImageInput{}, errGenerationPayloadTooLarge
	}
	if err := temporary.Sync(); err != nil {
		return ReferenceImageInput{}, err
	}
	if err := temporary.Close(); err != nil {
		return ReferenceImageInput{}, err
	}
	finalPath := strings.TrimSuffix(temporaryPath, ".partial") + imageFileExtension(mimeType)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return ReferenceImageInput{}, err
	}
	removeTemporary = false
	normalizedMIMEType := normalizeImageMimeType(mimeType)
	inputURL := ""
	if storageScope != StorageScopeCommercePrivate {
		inputURL = strings.TrimSpace(store.PublicURL(assetKey))
	}
	return ReferenceImageInput{
		MIMEType: normalizedMIMEType,
		InputURL: inputURL,
		FilePath: filepath.Clean(finalPath),
	}, nil
}

func buildExpandedSourceImage(source ReferenceImageInput, options map[string]any) (*ReferenceImageInput, *ReferenceImageInput, string, error) {
	expandOptions, ok := normalizedExpandToolOptions(options)
	if !ok {
		return nil, nil, "", errors.New("expand options are required")
	}
	rawImage, err := referenceImageInlineBytes(source)
	if err != nil {
		return nil, nil, "", err
	}
	decoded, _, err := image.Decode(bytes.NewReader(rawImage))
	if err != nil {
		return nil, nil, "", err
	}
	bounds := decoded.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return nil, nil, "", errors.New("source image has invalid dimensions")
	}

	top := percentPixels(sourceHeight, expandOptions.Top)
	bottom := percentPixels(sourceHeight, expandOptions.Bottom)
	left := percentPixels(sourceWidth, expandOptions.Left)
	right := percentPixels(sourceWidth, expandOptions.Right)
	targetWidth := sourceWidth + left + right
	targetHeight := sourceHeight + top + bottom
	if targetWidth <= 0 || targetHeight <= 0 || targetWidth*targetHeight > maxExpandCanvasPixels {
		return nil, nil, "", fmt.Errorf("expanded canvas exceeds limit: %dx%d", targetWidth, targetHeight)
	}

	canvas := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	targetRect := image.Rect(left, top, left+sourceWidth, top+sourceHeight)
	draw.Draw(canvas, targetRect, decoded, bounds.Min, draw.Src)

	mask := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(mask, targetRect, &image.Uniform{C: color.RGBA{A: 255}}, image.Point{}, draw.Src)

	sourceInput, err := pngReferenceImageInput(canvas)
	if err != nil {
		return nil, nil, "", err
	}
	maskInput, err := pngReferenceImageInput(mask)
	if err != nil {
		return nil, nil, "", err
	}
	return sourceInput, maskInput, providerSizeForDimensions(targetWidth, targetHeight), nil
}

func pngReferenceImageInput(img image.Image) (*ReferenceImageInput, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	return &ReferenceImageInput{
		MIMEType:   "image/png",
		Base64Data: base64Data,
		InputURL:   "data:image/png;base64," + base64Data,
	}, nil
}

func preserveExpandedResultOriginalArea(result ImageGenerationResult, original ReferenceImageInput, options map[string]any) (string, string, error) {
	expandOptions, ok := normalizedExpandToolOptions(options)
	if !ok {
		return "", "", errors.New("expand options are required")
	}
	resultBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(result.Base64Image))
	if err != nil {
		return "", "", fmt.Errorf("decode expand result: %w", err)
	}
	resultImage, _, err := image.Decode(bytes.NewReader(resultBytes))
	if err != nil {
		return "", "", fmt.Errorf("decode expand result image: %w", err)
	}
	resultBounds := resultImage.Bounds()
	resultWidth := resultBounds.Dx()
	resultHeight := resultBounds.Dy()
	if resultWidth <= 0 || resultHeight <= 0 {
		return "", "", errors.New("expand result has invalid dimensions")
	}

	originalBytes, err := referenceImageInlineBytes(original)
	if err != nil {
		return "", "", fmt.Errorf("decode expand original: %w", err)
	}
	originalImage, _, err := image.Decode(bytes.NewReader(originalBytes))
	if err != nil {
		return "", "", fmt.Errorf("decode expand original image: %w", err)
	}
	originalBounds := originalImage.Bounds()
	originalWidth := originalBounds.Dx()
	originalHeight := originalBounds.Dy()
	if originalWidth <= 0 || originalHeight <= 0 {
		return "", "", errors.New("expand original has invalid dimensions")
	}

	top := percentPixels(originalHeight, expandOptions.Top)
	bottom := percentPixels(originalHeight, expandOptions.Bottom)
	left := percentPixels(originalWidth, expandOptions.Left)
	right := percentPixels(originalWidth, expandOptions.Right)
	canvasWidth := originalWidth + left + right
	canvasHeight := originalHeight + top + bottom
	if canvasWidth <= 0 || canvasHeight <= 0 {
		return "", "", errors.New("expand canvas has invalid dimensions")
	}

	preserveRect := image.Rect(
		int(math.Round(float64(left)*float64(resultWidth)/float64(canvasWidth))),
		int(math.Round(float64(top)*float64(resultHeight)/float64(canvasHeight))),
		int(math.Round(float64(left+originalWidth)*float64(resultWidth)/float64(canvasWidth))),
		int(math.Round(float64(top+originalHeight)*float64(resultHeight)/float64(canvasHeight))),
	)
	preserveRect = preserveRect.Intersect(image.Rect(0, 0, resultWidth, resultHeight))
	if preserveRect.Empty() {
		return "", "", errors.New("expand preserve rectangle is empty")
	}

	output := image.NewRGBA(image.Rect(0, 0, resultWidth, resultHeight))
	draw.Draw(output, output.Bounds(), resultImage, resultBounds.Min, draw.Src)
	drawScaledNearest(output, preserveRect, originalImage)

	var buf bytes.Buffer
	if err := png.Encode(&buf, output); err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), "image/png", nil
}

func percentPixels(size, percent int) int {
	if percent <= 0 || size <= 0 {
		return 0
	}
	return int(math.Round(float64(size) * float64(percent) / 100))
}

func providerSizeForDimensions(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	ratio := float64(width) / float64(height)
	if ratio >= 1.2 {
		return "1536x1024"
	}
	if ratio <= 0.83 {
		return "1024x1536"
	}
	return "1024x1024"
}
