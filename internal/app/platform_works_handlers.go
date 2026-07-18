package app

// 本文件从 platform_handlers.go 拆分：作品库（works）查询、详情、复用、预览下载与公开作品。

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

func isKnownWorkCategory(category string) bool {
	switch category {
	case WorkCategoryImage, WorkCategoryVideo, WorkCategoryAudio, WorkCategoryPosterKV, WorkCategoryProductMain, WorkCategoryCover:
		return true
	default:
		return false
	}
}

func isKnownWorkMediaType(mediaType string) bool {
	switch mediaType {
	case WorkCategoryImage, WorkCategoryVideo, WorkCategoryAudio:
		return true
	default:
		return false
	}
}

func normalizeWorkCategory(category string) string {
	category = strings.TrimSpace(category)
	if isKnownWorkCategory(category) {
		return category
	}
	return WorkCategoryImage
}

func isKnownWorkVisibility(visibility string) bool {
	switch visibility {
	case WorkVisibilityPrivate, WorkVisibilityPublic:
		return true
	default:
		return false
	}
}

func isKnownWorkStatus(status string) bool {
	switch status {
	case GenerationStatusQueued, GenerationStatusRunning, GenerationStatusSucceeded, GenerationStatusFailed:
		return true
	default:
		return false
	}
}

func applyWorkCategoryFilter(dbQuery *gorm.DB, category string) *gorm.DB {
	if category == WorkCategoryImage {
		return dbQuery.Where("(category = ? OR category = '' OR category IS NULL)", WorkCategoryImage)
	}
	return dbQuery.Where("category = ?", category)
}

func applyWorkMediaTypeFilter(dbQuery *gorm.DB, mediaType string) *gorm.DB {
	switch mediaType {
	case WorkCategoryImage:
		imageCategories := []string{WorkCategoryImage, WorkCategoryPosterKV, WorkCategoryProductMain, WorkCategoryCover}
		return dbQuery.
			Joins("LEFT JOIN generation_records ON generation_records.id = works.generation_record_id").
			Where("(works.category IN ? OR works.category = '' OR works.category IS NULL)", imageCategories).
			Where("(works.mime_type = '' OR works.mime_type IS NULL OR (LOWER(works.mime_type) NOT LIKE ? AND LOWER(works.mime_type) NOT LIKE ?))", "video/%", "audio/%").
			Where("(generation_records.id IS NULL OR LOWER(TRIM(generation_records.tool_mode)) <> ?)", "video")
	case WorkCategoryVideo:
		return dbQuery.
			Joins("LEFT JOIN generation_records ON generation_records.id = works.generation_record_id").
			Where("(works.category = ? OR LOWER(works.mime_type) LIKE ? OR LOWER(TRIM(generation_records.tool_mode)) = ?)", WorkCategoryVideo, "video/%", "video")
	case WorkCategoryAudio:
		return dbQuery.Where("(works.category = ? OR LOWER(works.mime_type) LIKE ?)", WorkCategoryAudio, "audio/%")
	default:
		return dbQuery
	}
}

func workTimeRangeStart(timeRange string) (time.Time, bool) {
	now := time.Now()
	switch timeRange {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), true
	case "week":
		return now.AddDate(0, 0, -7), true
	case "month":
		return now.AddDate(0, -1, 0), true
	default:
		return time.Time{}, false
	}
}

func (a *App) albumPageWorkIDsForUser(userID uint) *gorm.DB {
	return a.db.Model(&CoupleAlbumPage{}).
		Select("couple_album_pages.work_id").
		Joins("JOIN couple_albums ON couple_albums.id = couple_album_pages.album_id").
		Where("couple_albums.user_id = ? AND couple_albums.deleted_at IS NULL AND couple_album_pages.work_id IS NOT NULL", userID)
}

func (a *App) worksLibraryQuery(userID uint, excludeAlbumPages bool) *gorm.DB {
	dbQuery := a.db.Model(&Work{}).Where("works.user_id = ?", userID)
	if excludeAlbumPages {
		dbQuery = dbQuery.Where("works.id NOT IN (?)", a.albumPageWorkIDsForUser(userID))
	}
	return dbQuery
}

func (a *App) buildWorksSummary(userID uint, excludeAlbumPages bool) (worksSummary, error) {
	summary := worksSummary{
		CategoryCounts: map[string]int64{
			WorkCategoryImage:       0,
			WorkCategoryVideo:       0,
			WorkCategoryAudio:       0,
			WorkCategoryPosterKV:    0,
			WorkCategoryProductMain: 0,
			WorkCategoryCover:       0,
		},
	}

	if err := a.worksLibraryQuery(userID, excludeAlbumPages).Count(&summary.Total).Error; err != nil {
		return summary, err
	}
	if err := a.worksLibraryQuery(userID, excludeAlbumPages).Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).Count(&summary.WeekNew).Error; err != nil {
		return summary, err
	}
	if err := a.worksLibraryQuery(userID, excludeAlbumPages).Where("visibility = ?", WorkVisibilityPrivate).Count(&summary.PrivateCount).Error; err != nil {
		return summary, err
	}
	if summary.Total > 0 {
		summary.StoredPercent = 100
	}

	var categoryRows []struct {
		Category string
		Count    int64
	}
	if err := a.worksLibraryQuery(userID, excludeAlbumPages).
		Select("category, count(*) as count").
		Group("category").
		Scan(&categoryRows).Error; err != nil {
		return summary, err
	}
	for _, row := range categoryRows {
		summary.CategoryCounts[normalizeWorkCategory(row.Category)] += row.Count
	}

	return summary, nil
}

func (a *App) handleListWorks(c *gin.Context) {
	user := currentUser(c)
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	query := strings.TrimSpace(c.Query("q"))
	category := strings.TrimSpace(c.Query("category"))
	mediaType := strings.ToLower(strings.TrimSpace(c.Query("media_type")))
	timeRange := strings.TrimSpace(c.Query("time_range"))
	sortMode := strings.TrimSpace(c.Query("sort"))
	status := strings.TrimSpace(c.Query("status"))
	favorite := strings.TrimSpace(c.Query("favorite"))
	excludeAlbumPages := queryBool(c.Query("exclude_album_pages"))

	dbQuery := a.worksLibraryQuery(user.ID, excludeAlbumPages)
	if query != "" {
		dbQuery = dbQuery.Where("prompt LIKE ?", "%"+query+"%")
	}
	if category != "" && category != "all" {
		if !isKnownWorkCategory(category) {
			writeError(c, http.StatusBadRequest, "invalid_work_category", "不支持的作品类型")
			return
		}
		dbQuery = applyWorkCategoryFilter(dbQuery, category)
	}
	if mediaType != "" && mediaType != "all" {
		if !isKnownWorkMediaType(mediaType) {
			writeError(c, http.StatusBadRequest, "invalid_work_media_type", "不支持的作品媒体类型")
			return
		}
		dbQuery = applyWorkMediaTypeFilter(dbQuery, mediaType)
	}
	if since, ok := workTimeRangeStart(timeRange); ok {
		dbQuery = dbQuery.Where("works.created_at >= ?", since)
	}
	if status != "" && status != "all" {
		if !isKnownWorkStatus(status) {
			writeError(c, http.StatusBadRequest, "invalid_work_status", "不支持的作品状态")
			return
		}
		dbQuery = dbQuery.Where("status = ?", status)
	}
	if favorite == "true" || favorite == "1" {
		dbQuery = dbQuery.Where("is_favorite = ?", true)
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "works_count_failed", "作品读取失败")
		return
	}

	var items []Work
	orderClause := "works.created_at desc, works.id desc"
	if sortMode == "oldest" {
		orderClause = "works.created_at asc, works.id asc"
	}
	if err := dbQuery.Order(orderClause).Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "works_load_failed", "作品读取失败")
		return
	}
	for index := range items {
		items[index].Category = normalizeWorkCategory(items[index].Category)
		a.applyWorkPublicURL(&items[index])
	}
	if err := a.attachWorkReferenceAssetIDs(items); err != nil {
		writeError(c, http.StatusInternalServerError, "work_references_load_failed", "作品参考素材读取失败")
		return
	}

	summary, err := a.buildWorksSummary(user.ID, excludeAlbumPages)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "works_summary_failed", "作品统计读取失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"summary":   summary,
	})
}

func (a *App) handleGetWork(c *gin.Context) {
	work, ok := a.findOwnedWork(c, currentUser(c).ID)
	if !ok {
		return
	}
	work.Category = normalizeWorkCategory(work.Category)
	a.applyWorkPublicURL(&work)
	workItems := []Work{work}
	if err := a.attachWorkReferenceAssetIDs(workItems); err != nil {
		writeError(c, http.StatusInternalServerError, "work_references_load_failed", "作品参考素材读取失败")
		return
	}
	work = workItems[0]
	writeJSON(c, http.StatusOK, work)
}

func (a *App) handleUpdateWork(c *gin.Context) {
	work, ok := a.findOwnedWork(c, currentUser(c).ID)
	if !ok {
		return
	}

	var req struct {
		Visibility *string `json:"visibility"`
		IsFavorite *bool   `json:"is_favorite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	updates := map[string]any{}
	if req.Visibility != nil {
		visibility := strings.TrimSpace(*req.Visibility)
		if !isKnownWorkVisibility(visibility) {
			writeError(c, http.StatusBadRequest, "invalid_work_visibility", "不支持的可见性")
			return
		}
		if visibility == WorkVisibilityPublic {
			blocked, err := a.workHasBlockingContentReview(work)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "work_review_check_failed", "作品审核状态读取失败")
				return
			}
			if blocked {
				writeError(c, http.StatusForbidden, "work_review_required", "作品存在未通过或待处理的内容审核，暂不能公开分享")
				return
			}
		}
		updates["visibility"] = visibility
	}
	if req.IsFavorite != nil {
		updates["is_favorite"] = *req.IsFavorite
	}
	if len(updates) == 0 {
		writeError(c, http.StatusBadRequest, "empty_work_update", "至少需要更新一个字段")
		return
	}

	if err := a.db.Model(&Work{}).Where("id = ? AND user_id = ?", work.ID, work.UserID).Updates(updates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "work_update_failed", "作品更新失败")
		return
	}
	if err := a.db.Where("id = ? AND user_id = ?", work.ID, work.UserID).First(&work).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "work_load_failed", "作品读取失败")
		return
	}
	work.Category = normalizeWorkCategory(work.Category)
	a.applyWorkPublicURL(&work)
	writeJSON(c, http.StatusOK, work)
}

func (a *App) handleDeleteWork(c *gin.Context) {
	work, ok := a.findOwnedWork(c, currentUser(c).ID)
	if !ok {
		return
	}
	if err := a.db.Delete(&work).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "work_delete_failed", "作品删除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleReuseWork(c *gin.Context) {
	user := currentUser(c)
	work, ok := a.findOwnedWork(c, user.ID)
	if !ok {
		return
	}
	a.applyWorkPublicURL(&work)
	payload, err := a.reuseWorkPayload(user.ID, work)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "work_reuse_failed", "作品复用参数读取失败")
		return
	}
	writeJSON(c, http.StatusOK, payload)
}

func (a *App) reuseWorkPayload(userID uint, work Work) (gin.H, error) {
	payload := gin.H{
		"work_id":        work.ID,
		"prompt":         work.Prompt,
		"aspect_ratio":   work.AspectRatio,
		"source_work_id": work.ID,
	}
	if normalizeWorkCategory(work.Category) == WorkCategoryImage && strings.TrimSpace(work.AssetKey) != "" {
		payload["reference_work_ids"] = []uint{work.ID}
		payload["reference_works"] = []workReuseReferenceWorkPayload{
			{
				ID:         work.ID,
				PreviewURL: work.PreviewURL,
				MIMEType:   fallbackString(work.MIMEType, "image/png"),
			},
		}
	} else {
		payload["reference_work_ids"] = []uint{}
		payload["reference_works"] = []workReuseReferenceWorkPayload{}
	}

	var record GenerationRecord
	if err := a.db.Where("id = ? AND user_id = ?", work.GenerationRecordID, userID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return payload, nil
		}
		return nil, err
	}

	payload["negative_prompt"] = record.NegativePrompt
	payload["style_preset"] = record.StylePreset
	payload["tool_mode"] = fallbackString(record.ToolMode, GenerationToolModeGenerate)
	payload["style_strength"] = record.StyleStrength
	payload["reference_weight"] = record.ReferenceWeight
	payload["seed"] = record.Seed
	payload["variation_mode"] = record.VariationMode
	payload["variation_prompt"] = record.VariationPrompt
	if record.SourceWorkID != nil && *record.SourceWorkID > 0 {
		payload["source_work_id"] = *record.SourceWorkID
	}

	referenceAssetIDs, referenceAssets, err := a.reuseWorkReferenceAssets(userID, record.ID)
	if err != nil {
		return nil, err
	}
	payload["reference_asset_ids"] = referenceAssetIDs
	payload["reference_assets"] = referenceAssets
	return payload, nil
}

func (a *App) reuseWorkReferenceAssets(userID, generationRecordID uint) ([]uint, []workReuseReferenceAssetPayload, error) {
	var links []GenerationReferenceAsset
	if err := a.db.Where("generation_record_id = ?", generationRecordID).Order("sort_order asc, id asc").Find(&links).Error; err != nil {
		return nil, nil, err
	}
	if len(links) == 0 {
		return []uint{}, []workReuseReferenceAssetPayload{}, nil
	}

	ids := make([]uint, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.ReferenceAssetID)
	}

	var assets []ReferenceAsset
	if err := a.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&assets).Error; err != nil {
		return nil, nil, err
	}
	byID := make(map[uint]ReferenceAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}

	orderedIDs := make([]uint, 0, len(links))
	orderedAssets := make([]workReuseReferenceAssetPayload, 0, len(links))
	for _, link := range links {
		asset, ok := byID[link.ReferenceAssetID]
		if !ok {
			continue
		}
		a.applyReferenceAssetPublicURL(&asset)
		orderedIDs = append(orderedIDs, asset.ID)
		orderedAssets = append(orderedAssets, workReuseReferenceAssetPayload{
			ID:               asset.ID,
			PreviewURL:       asset.PreviewURL,
			MIMEType:         asset.MIMEType,
			OriginalFilename: asset.OriginalFilename,
		})
	}
	return orderedIDs, orderedAssets, nil
}

func (a *App) handleServeWorkPreview(c *gin.Context) {
	work, ok := a.findOwnedWork(c, currentUser(c).ID)
	if !ok {
		return
	}
	a.serveWorkAsset(c, work, false)
}

func (a *App) handleServeWorkDownload(c *gin.Context) {
	work, ok := a.findOwnedWork(c, currentUser(c).ID)
	if !ok {
		return
	}
	a.serveWorkAsset(c, work, true)
}

func parsePublicWorkIDs(raw string) ([]uint, string) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, "invalid_public_work_ids"
	}
	parts := strings.Split(text, ",")
	if len(parts) > maxPublicWorksShareIDs {
		return nil, "too_many_public_work_ids"
	}
	ids := make([]uint, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, "invalid_public_work_ids"
		}
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			return nil, "invalid_public_work_ids"
		}
		ids = append(ids, uint(parsed))
	}
	if len(ids) == 0 {
		return nil, "invalid_public_work_ids"
	}
	return ids, ""
}

func publicWorkPreviewURL(id uint) string {
	return fmt.Sprintf("/api/public/works/%d/file", id)
}

func publicWorkListPayload(work Work) publicWorkPayload {
	return publicWorkPayload{
		WorkID:      work.ID,
		Prompt:      work.Prompt,
		AspectRatio: work.AspectRatio,
		Category:    normalizeWorkCategory(work.Category),
		Status:      work.Status,
		MIMEType:    normalizeImageMimeType(work.MIMEType),
		PreviewURL:  publicWorkPreviewURL(work.ID),
		CreatedAt:   work.CreatedAt,
	}
}

func (a *App) handleListPublicWorks(c *gin.Context) {
	ids, code := parsePublicWorkIDs(c.Query("ids"))
	if code != "" {
		message := "作品 ID 参数错误"
		if code == "too_many_public_work_ids" {
			message = "一次最多分享 16 个作品"
		}
		writeError(c, http.StatusBadRequest, code, message)
		return
	}

	var works []Work
	if err := a.db.Where("id IN ? AND visibility = ?", ids, WorkVisibilityPublic).Find(&works).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "works_load_failed", "作品读取失败")
		return
	}
	byID := make(map[uint]Work, len(works))
	for _, work := range works {
		byID[work.ID] = work
	}
	items := make([]publicWorkPayload, 0, len(works))
	for _, id := range ids {
		work, ok := byID[id]
		if !ok {
			continue
		}
		items = append(items, publicWorkListPayload(work))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleServePublicWorkPreview(c *gin.Context) {
	var work Work
	result := a.db.Where("id = ?", c.Param("id")).Limit(1).Find(&work)
	if result.Error != nil {
		writeError(c, http.StatusInternalServerError, "work_load_failed", "作品读取失败")
		return
	}
	if result.RowsAffected == 0 {
		writeError(c, http.StatusNotFound, "work_not_found", "作品不存在")
		return
	}
	if work.Visibility != WorkVisibilityPublic {
		visible, err := a.workVisibleThroughSharedCoupleAlbum(work.ID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "work_load_failed", "作品读取失败")
			return
		}
		if !visible {
			writeError(c, http.StatusNotFound, "work_not_found", "作品不存在")
			return
		}
	}
	a.serveWorkAsset(c, work, false)
}

func (a *App) workVisibleThroughSharedCoupleAlbum(workID uint) (bool, error) {
	if workID == 0 {
		return false, nil
	}
	var count int64
	err := a.db.Model(&CoupleAlbumPage{}).
		Joins("JOIN couple_albums ON couple_albums.id = couple_album_pages.album_id").
		Where("couple_album_pages.work_id = ? AND couple_album_pages.status = ? AND couple_albums.share_enabled = ? AND couple_albums.share_token <> ?", workID, GenerationStatusSucceeded, true, "").
		Count(&count).Error
	return count > 0, err
}

func (a *App) serveWorkAsset(c *gin.Context, work Work, download bool) {
	filename := fmt.Sprintf("work-%d%s", work.ID, extensionForMimeType(work.MIMEType))
	if download {
		filename = a.commerceWorkDownloadFilename(work, filename)
	}
	if download {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", filename, url.PathEscape(filename)))
	} else {
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	}
	store, err := a.assetStoreForScope(work.StorageScope)
	if err != nil {
		writeError(c, http.StatusNotFound, "asset_not_found", "作品文件不存在")
		return
	}
	if work.StorageScope != StorageScopeCommercePrivate {
		if publicURL := store.PublicURL(work.AssetKey); publicURL != "" {
			c.Redirect(http.StatusFound, publicURL)
			return
		}
	} else if signedStore, ok := store.(SignedAssetStore); ok {
		if signedURL, signErr := signedStore.SignedReadURL(work.AssetKey, a.commerceSignedURLTTL()); signErr == nil && signedURL != "" {
			c.Redirect(http.StatusFound, signedURL)
			return
		}
	}

	content, err := store.Read(work.AssetKey)
	if err != nil {
		writeError(c, http.StatusNotFound, "asset_not_found", "作品文件不存在")
		return
	}
	c.Data(http.StatusOK, normalizeAssetMimeType(work.MIMEType), content)
}

func (a *App) commerceWorkDownloadFilename(work Work, fallback string) string {
	var item ecommerce.CommerceGenerationItem
	if err := a.db.Where("work_id = ? AND user_id = ?", work.ID, work.UserID).Order("id desc").First(&item).Error; err != nil {
		return fallback
	}
	compiled, err := ecommerce.DecodeGenerationItemSnapshot(item.OutputSpecJSON)
	if err != nil {
		return fallback
	}
	identity := compiled.SKUCode
	if compiled.Scope == "shared" {
		identity = "公共内容"
	}
	identity, section := safeDownloadName(identity), safeDownloadName(compiled.Section)
	if identity == "" || section == "" {
		return fallback
	}
	return identity + "-" + section + extensionForMimeType(work.MIMEType)
}

func safeDownloadName(value string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.TrimSpace(value)), "-")
}

func (a *App) applyWorkPublicURL(work *Work) {
	if work == nil {
		return
	}
	if work.StorageScope == StorageScopeCommercePrivate {
		return
	}
	store, err := a.assetStoreForScope(work.StorageScope)
	if err != nil {
		return
	}
	if publicURL := store.PublicURL(work.AssetKey); publicURL != "" {
		work.PreviewURL = publicURL
		work.DownloadURL = publicURL
	}
}

func (a *App) attachWorkReferenceAssetIDs(works []Work) error {
	if len(works) == 0 {
		return nil
	}

	recordIDs := make([]uint, 0, len(works))
	recordIDSet := make(map[uint]struct{}, len(works))
	for _, work := range works {
		if work.GenerationRecordID == 0 {
			continue
		}
		if _, exists := recordIDSet[work.GenerationRecordID]; exists {
			continue
		}
		recordIDSet[work.GenerationRecordID] = struct{}{}
		recordIDs = append(recordIDs, work.GenerationRecordID)
	}
	if len(recordIDs) == 0 {
		return nil
	}

	var links []GenerationReferenceAsset
	if err := a.db.Where("generation_record_id IN ?", recordIDs).Order("generation_record_id asc, sort_order asc, id asc").Find(&links).Error; err != nil {
		return err
	}
	byRecordID := make(map[uint][]uint, len(recordIDs))
	for _, link := range links {
		byRecordID[link.GenerationRecordID] = append(byRecordID[link.GenerationRecordID], link.ReferenceAssetID)
	}
	for index := range works {
		works[index].ReferenceAssetIDs = byRecordID[works[index].GenerationRecordID]
	}
	return nil
}
