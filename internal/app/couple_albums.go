package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	coupleAlbumPageCount                   = 8
	childhoodCareerDreamLocationValue      = "childhood_dream_stage"
	childhoodCareerDreamStoryTemplateValue = "childhood_career_dream"
	childhoodCareerDreamDefaultStyleValue  = "children_storybook"
	childhoodCareerDreamNegativePrompt     = "文字、水印、logo、成人化、性感姿态、畸形手、多余人物、脸部变形、恐怖、血腥、脏乱"
)

type coupleAlbumCreateRequest struct {
	Title                  string `json:"title"`
	Location               string `json:"location"`
	StoryTemplate          string `json:"story_template"`
	Style                  string `json:"style"`
	MaleReferenceAssetID   uint   `json:"male_reference_asset_id"`
	FemaleReferenceAssetID uint   `json:"female_reference_asset_id"`
}

type coupleAlbumPageSpec struct {
	PageNumber int
	PageTitle  string
	Caption    string
	Prompt     string
}

type coupleAlbumGenerationContext struct {
	User                  User
	Settings              AppSettings
	ModelConfig           *ModelConfig
	ModelCandidates       []ModelConfig
	ModelCenterModel      *ModelCatalog
	ModelCenterChannel    *ModelChannel
	ModelCenterCandidates []modelCenterCandidate
	ReferenceAssets       []ReferenceAsset
}

type coupleAlbumGenerationTask struct {
	PageID uint
	Record GenerationRecord
	Job    *generationJob
}

func coupleAlbumRequiredCredits(pageCount, referenceAssetCount, baseCreditsCost int) int {
	if pageCount <= 0 {
		pageCount = 1
	}
	return pageCount
}

func (a *App) coupleAlbumBaseCreditCost() (int, error) {
	settings, err := a.loadSettings()
	if err != nil {
		return 0, err
	}
	if _, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0); err != nil {
		return 0, err
	}
	return 1, nil
}

func (a *App) handleCreateCoupleAlbum(c *gin.Context) {
	user := currentUser(c)

	req, ok := a.parseCoupleAlbumCreateRequest(c)
	if !ok {
		return
	}
	if _, err := a.loadCoupleAlbumReferenceAssets(user.ID, req.MaleReferenceAssetID, req.FemaleReferenceAssetID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "reference_assets_load_failed", "参考素材读取失败")
		return
	}

	album := CoupleAlbum{
		UserID:                 user.ID,
		Title:                  truncateRunes(req.Title, 160),
		Location:               truncateRunes(req.Location, 96),
		StoryTemplate:          truncateRunes(req.StoryTemplate, 64),
		Style:                  truncateRunes(req.Style, 64),
		Status:                 CoupleAlbumStatusDraft,
		ShareToken:             uuid.NewString(),
		ShareEnabled:           false,
		MaleReferenceAssetID:   req.MaleReferenceAssetID,
		FemaleReferenceAssetID: req.FemaleReferenceAssetID,
	}
	if err := a.db.Create(&album).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "album_create_failed", "相册创建失败")
		return
	}

	writeJSON(c, http.StatusCreated, gin.H{"album": a.coupleAlbumPayload(album, nil, false)})
}

func (a *App) parseCoupleAlbumCreateRequest(c *gin.Context) (coupleAlbumCreateRequest, bool) {
	var req coupleAlbumCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return coupleAlbumCreateRequest{}, false
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Location = strings.TrimSpace(req.Location)
	req.StoryTemplate = strings.TrimSpace(req.StoryTemplate)
	req.Style = strings.TrimSpace(req.Style)
	req.StoryTemplate = fallbackString(req.StoryTemplate, "city_walk")
	if isChildhoodCareerDreamRequest(req) && req.Location == "" {
		req.Location = childhoodCareerDreamLocationValue
	}
	if req.Title == "" {
		writeError(c, http.StatusBadRequest, "album_title_required", "相册标题不能为空")
		return coupleAlbumCreateRequest{}, false
	}
	if req.Location == "" {
		writeError(c, http.StatusBadRequest, "album_location_required", "请选择旅游地点")
		return coupleAlbumCreateRequest{}, false
	}
	if isChildhoodCareerDreamRequest(req) {
		req.Style = fallbackString(req.Style, childhoodCareerDreamDefaultStyleValue)
	} else {
		req.Style = fallbackString(req.Style, "film")
	}
	if _, ok, failure := a.validateCoupleAlbumCreateOptions(req); !ok {
		parts := strings.SplitN(failure, "|", 2)
		message := "相册配置无效"
		if len(parts) == 2 {
			message = parts[1]
		}
		writeError(c, http.StatusUnprocessableEntity, parts[0], message)
		return coupleAlbumCreateRequest{}, false
	}
	if req.MaleReferenceAssetID == 0 {
		writeError(c, http.StatusBadRequest, "album_reference_required", "请上传参考照片")
		return coupleAlbumCreateRequest{}, false
	}
	if !isChildhoodCareerDreamRequest(req) && req.FemaleReferenceAssetID == 0 {
		writeError(c, http.StatusBadRequest, "album_reference_required", "请上传双方照片")
		return coupleAlbumCreateRequest{}, false
	}
	if req.FemaleReferenceAssetID > 0 && req.MaleReferenceAssetID == req.FemaleReferenceAssetID {
		writeError(c, http.StatusUnprocessableEntity, "album_reference_distinct_required", "双方照片不能使用同一张参考图")
		return coupleAlbumCreateRequest{}, false
	}
	return req, true
}

func (a *App) handleEstimateCoupleAlbum(c *gin.Context) {
	user := currentUser(c)

	req, ok := a.parseCoupleAlbumCreateRequest(c)
	if !ok {
		return
	}
	referenceAssets, err := a.loadCoupleAlbumReferenceAssets(user.ID, req.MaleReferenceAssetID, req.FemaleReferenceAssetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "reference_assets_load_failed", "参考素材读取失败")
		return
	}
	baseCreditsCost, err := a.coupleAlbumBaseCreditCost()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_config_load_failed", "模型配置读取失败")
		return
	}
	estimate, err := a.buildCreditEstimate(user.ID, coupleAlbumRequiredCredits(coupleAlbumPageCount, len(referenceAssets), baseCreditsCost))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, estimate)
}

func (a *App) handleListCoupleAlbums(c *gin.Context) {
	user := currentUser(c)

	var albums []CoupleAlbum
	if err := a.db.
		Preload("Pages", func(db *gorm.DB) *gorm.DB { return db.Order("page_number ASC") }).
		Where("user_id = ?", user.ID).
		Order("created_at DESC, id DESC").
		Find(&albums).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "albums_load_failed", "相册读取失败")
		return
	}

	items := make([]gin.H, 0, len(albums))
	for _, album := range albums {
		worksByID := a.coupleAlbumPageWorks(album.Pages)
		items = append(items, a.coupleAlbumPayload(album, worksByID, false))
	}
	writeJSON(c, http.StatusOK, gin.H{"albums": items})
}

func (a *App) handleGetCoupleAlbum(c *gin.Context) {
	user := currentUser(c)

	album, ok := a.loadOwnedCoupleAlbum(c, user.ID)
	if !ok {
		return
	}
	worksByID := a.coupleAlbumPageWorks(album.Pages)
	writeJSON(c, http.StatusOK, gin.H{"album": a.coupleAlbumPayload(album, worksByID, false)})
}

func (a *App) handleGenerateCoupleAlbum(c *gin.Context) {
	user := currentUser(c)

	album, ok := a.loadOwnedCoupleAlbum(c, user.ID)
	if !ok {
		return
	}
	if album.Status == CoupleAlbumStatusGenerating {
		writeError(c, http.StatusConflict, "album_generation_in_progress", "相册正在生成中")
		return
	}
	if len(album.Pages) > 0 && album.Status != CoupleAlbumStatusDraft {
		writeError(c, http.StatusConflict, "album_already_generated", "相册已生成，请查看详情")
		return
	}

	generationContext, ok := a.prepareCoupleAlbumGenerationContext(c, user, album, coupleAlbumPageCount)
	if !ok {
		return
	}
	specs := a.buildCoupleAlbumPageSpecs(album)
	tasks := make([]coupleAlbumGenerationTask, 0, len(specs))

	if len(album.Pages) > 0 {
		if err := a.db.Where("album_id = ?", album.ID).Delete(&CoupleAlbumPage{}).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "album_generate_failed", "相册生成任务创建失败")
			return
		}
	}
	for _, spec := range specs {
		job := coupleAlbumGenerationJob(generationContext, album, spec)
		record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "album_generate_failed", "相册生成任务创建失败")
			return
		}
		page := CoupleAlbumPage{
			AlbumID:            album.ID,
			PageNumber:         spec.PageNumber,
			PageTitle:          spec.PageTitle,
			Caption:            spec.Caption,
			Prompt:             spec.Prompt,
			Status:             GenerationStatusQueued,
			GenerationRecordID: &record.ID,
		}
		if err := a.db.Create(&page).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "album_generate_failed", "相册生成任务创建失败")
			return
		}
		tasks = append(tasks, coupleAlbumGenerationTask{PageID: page.ID, Record: record, Job: job})
	}
	if err := a.db.Model(&album).Updates(map[string]any{
		"status":        CoupleAlbumStatusGenerating,
		"cover_page_id": nil,
	}).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "album_generate_failed", "相册生成任务创建失败")
		return
	}

	album, _ = a.findCoupleAlbumByIDForUser(album.ID, user.ID)
	worksByID := a.coupleAlbumPageWorks(album.Pages)
	payload := gin.H{"album": a.coupleAlbumPayload(album, worksByID, false)}

	go a.runCoupleAlbumGenerationTasks(album.ID, tasks)

	writeJSON(c, http.StatusAccepted, payload)
}

func (a *App) handleRetryCoupleAlbumPage(c *gin.Context) {
	user := currentUser(c)

	album, ok := a.loadOwnedCoupleAlbum(c, user.ID)
	if !ok {
		return
	}
	pageID, ok := uintParam(c, "page_id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_page_id", "相册页无效")
		return
	}
	var page CoupleAlbumPage
	if err := a.db.Where("id = ? AND album_id = ?", pageID, album.ID).First(&page).Error; err != nil {
		writeError(c, http.StatusNotFound, "album_page_not_found", "相册页不存在")
		return
	}
	if page.Status != GenerationStatusFailed {
		writeError(c, http.StatusConflict, "album_page_not_failed", "只能重试失败页")
		return
	}

	generationContext, ok := a.prepareCoupleAlbumGenerationContext(c, user, album, 1)
	if !ok {
		return
	}
	spec := coupleAlbumPageSpec{
		PageNumber: page.PageNumber,
		PageTitle:  page.PageTitle,
		Caption:    page.Caption,
		Prompt:     page.Prompt,
	}
	job := coupleAlbumGenerationJob(generationContext, album, spec)
	record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "record_create_failed", "记录创建失败")
		return
	}

	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&page).Updates(map[string]any{
			"status":               GenerationStatusQueued,
			"generation_record_id": record.ID,
			"work_id":              nil,
			"error_code":           "",
			"error_message":        "",
		}).Error; err != nil {
			return err
		}
		return tx.Model(&album).Update("status", CoupleAlbumStatusGenerating).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "album_page_retry_failed", "重试任务创建失败")
		return
	}

	go a.runCoupleAlbumGenerationTasks(album.ID, []coupleAlbumGenerationTask{{PageID: page.ID, Record: record, Job: job}})

	album, _ = a.findCoupleAlbumByIDForUser(album.ID, user.ID)
	worksByID := a.coupleAlbumPageWorks(album.Pages)
	writeJSON(c, http.StatusAccepted, gin.H{"album": a.coupleAlbumPayload(album, worksByID, false)})
}

func (a *App) handleShareCoupleAlbum(c *gin.Context) {
	user := currentUser(c)

	album, ok := a.loadOwnedCoupleAlbum(c, user.ID)
	if !ok {
		return
	}
	if strings.TrimSpace(album.ShareToken) == "" {
		album.ShareToken = uuid.NewString()
	}
	if err := a.db.Model(&album).Updates(map[string]any{
		"share_token":   album.ShareToken,
		"share_enabled": true,
	}).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "album_share_failed", "分享开启失败")
		return
	}
	album.ShareEnabled = true
	worksByID := a.coupleAlbumPageWorks(album.Pages)
	writeJSON(c, http.StatusOK, gin.H{
		"album":       a.coupleAlbumPayload(album, worksByID, false),
		"share_token": album.ShareToken,
		"share_url":   a.coupleAlbumShareURL(album.ShareToken),
	})
}

func (a *App) handleGetPublicCoupleAlbum(c *gin.Context) {
	token := strings.TrimSpace(c.Param("share_token"))
	if token == "" {
		writeError(c, http.StatusNotFound, "album_not_found", "相册不存在")
		return
	}

	var album CoupleAlbum
	err := a.db.
		Preload("Pages", func(db *gorm.DB) *gorm.DB { return db.Order("page_number ASC") }).
		Where("share_token = ? AND share_enabled = ?", token, true).
		First(&album).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "album_not_found", "相册不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "album_load_failed", "相册读取失败")
		return
	}

	worksByID := a.coupleAlbumPageWorks(album.Pages)
	writeJSON(c, http.StatusOK, gin.H{"album": a.coupleAlbumPayload(album, worksByID, true)})
}

func (a *App) loadOwnedCoupleAlbum(c *gin.Context, userID uint) (CoupleAlbum, bool) {
	albumID, ok := uintParam(c, "id")
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_album_id", "相册无效")
		return CoupleAlbum{}, false
	}
	album, err := a.findCoupleAlbumByIDForUser(albumID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "album_not_found", "相册不存在")
		return CoupleAlbum{}, false
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "album_load_failed", "相册读取失败")
		return CoupleAlbum{}, false
	}
	return album, true
}

func (a *App) findCoupleAlbumByIDForUser(albumID, userID uint) (CoupleAlbum, error) {
	var album CoupleAlbum
	err := a.db.
		Preload("Pages", func(db *gorm.DB) *gorm.DB { return db.Order("page_number ASC") }).
		Where("id = ? AND user_id = ?", albumID, userID).
		First(&album).Error
	return album, err
}

func (a *App) prepareCoupleAlbumGenerationContext(c *gin.Context, user *User, album CoupleAlbum, pageCount int) (*coupleAlbumGenerationContext, bool) {
	referenceAssets, err := a.loadCoupleAlbumReferenceAssets(user.ID, album.MaleReferenceAssetID, album.FemaleReferenceAssetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "参考素材不存在")
			return nil, false
		}
		writeError(c, http.StatusInternalServerError, "reference_assets_load_failed", "参考素材读取失败")
		return nil, false
	}
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return nil, false
	}
	modelCenterCandidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_load_failed", "模型中心配置读取失败")
		return nil, false
	}
	var modelCenterModel *ModelCatalog
	var modelCenterChannel *ModelChannel
	var modelConfig *ModelConfig
	var modelCandidates []ModelConfig
	if len(modelCenterCandidates) > 0 {
		modelCenterModel = &modelCenterCandidates[0].Model
		modelCenterChannel = &modelCenterCandidates[0].Channel
	} else {
		modelCandidates, err = a.modelConfigCandidatesForGeneration(settings)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "model_config_load_failed", "模型配置读取失败")
			return nil, false
		}
		if len(modelCandidates) > 0 {
			modelConfig = &modelCandidates[0]
		} else {
			modelConfig, err = a.modelConfigForGeneration(settings)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "model_config_load_failed", "模型配置读取失败")
				return nil, false
			}
		}
	}

	estimate, err := a.buildCreditEstimate(user.ID, coupleAlbumRequiredCredits(pageCount, len(referenceAssets), 1))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return nil, false
	}
	if !estimate.Enough {
		writeCreditsInsufficientError(c, estimate)
		return nil, false
	}

	rateKey := clientIP(c.Request) + "|user:" + strconv.FormatUint(uint64(user.ID), 10)
	window := time.Duration(settings.RateLimitWindowSeconds) * time.Second
	if !a.rateLimiter.Allow(rateKey, time.Now(), window, settings.RateLimitMaxRequests) {
		writeError(c, http.StatusTooManyRequests, "too_many_requests", "请求过于频繁")
		return nil, false
	}

	return &coupleAlbumGenerationContext{
		User:                  *user,
		Settings:              settings,
		ModelConfig:           modelConfig,
		ModelCandidates:       modelCandidates,
		ModelCenterModel:      modelCenterModel,
		ModelCenterChannel:    modelCenterChannel,
		ModelCenterCandidates: modelCenterCandidates,
		ReferenceAssets:       referenceAssets,
	}, true
}

func (a *App) loadCoupleAlbumReferenceAssets(userID, maleID, femaleID uint) ([]ReferenceAsset, error) {
	if maleID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	assetIDs := []uint{maleID}
	if femaleID > 0 {
		assetIDs = append(assetIDs, femaleID)
	}
	var assets []ReferenceAsset
	if err := a.db.Where("user_id = ? AND id IN ?", userID, assetIDs).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(assetIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	byID := make(map[uint]ReferenceAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	male, ok := byID[maleID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if femaleID == 0 {
		return []ReferenceAsset{male}, nil
	}
	female, ok := byID[femaleID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return []ReferenceAsset{male, female}, nil
}

func coupleAlbumGenerationJob(ctx *coupleAlbumGenerationContext, album CoupleAlbum, spec coupleAlbumPageSpec) *generationJob {
	styleStrength := 72
	referenceWeight := 86
	referenceAssetIDs := []uint{album.MaleReferenceAssetID}
	if album.FemaleReferenceAssetID > 0 {
		referenceAssetIDs = append(referenceAssetIDs, album.FemaleReferenceAssetID)
	}
	req := generationRequest{
		Prompt:            spec.Prompt,
		NegativePrompt:    coupleAlbumNegativePrompt(album),
		AspectRatio:       "3:4",
		Quality:           GenerationQualityMedium,
		StylePreset:       album.Style,
		ToolMode:          GenerationToolModeGenerate,
		StyleStrength:     &styleStrength,
		ReferenceWeight:   &referenceWeight,
		ReferenceIntent:   GenerationReferenceIntentCharacter,
		ReferenceAssetIDs: referenceAssetIDs,
	}
	req.Size, _ = aspectRatioToSize(req.AspectRatio)
	return &generationJob{
		User:                  ctx.User,
		Settings:              ctx.Settings,
		ModelConfig:           ctx.ModelConfig,
		ModelCandidates:       append([]ModelConfig(nil), ctx.ModelCandidates...),
		ModelCenterModel:      ctx.ModelCenterModel,
		ModelCenterChannel:    ctx.ModelCenterChannel,
		ModelCenterCandidates: append([]modelCenterCandidate(nil), ctx.ModelCenterCandidates...),
		Request:               req,
		ReferenceAssets:       append([]ReferenceAsset(nil), ctx.ReferenceAssets...),
	}
}

func (a *App) runCoupleAlbumGenerationTasks(albumID uint, tasks []coupleAlbumGenerationTask) {
	for _, task := range tasks {
		record := task.Record
		_ = a.db.Model(&CoupleAlbumPage{}).
			Where("id = ?", task.PageID).
			Updates(map[string]any{"status": GenerationStatusRunning, "error_code": "", "error_message": ""}).Error
		_, providerErr, err := a.executeGenerationRecord(&record, task.Job)
		if err != nil && strings.TrimSpace(record.ErrorCode) == "" {
			a.failGenerationRecord(&record, "generation_failed", "生成任务失败")
		}
		if providerErr != nil || err != nil || record.Status == GenerationStatusFailed {
			var latest GenerationRecord
			if loadErr := a.db.First(&latest, record.ID).Error; loadErr == nil {
				record = latest
			}
			_ = a.db.Model(&CoupleAlbumPage{}).
				Where("id = ?", task.PageID).
				Updates(map[string]any{
					"status":        GenerationStatusFailed,
					"work_id":       nil,
					"error_code":    fallbackString(record.ErrorCode, "generation_failed"),
					"error_message": fallbackString(record.ErrorMessage, "生成失败，请稍后重试"),
				}).Error
			_ = a.refreshCoupleAlbumStatus(albumID)
			continue
		}
		var workID any
		if record.WorkID != nil {
			workID = *record.WorkID
		}
		_ = a.db.Model(&CoupleAlbumPage{}).
			Where("id = ?", task.PageID).
			Updates(map[string]any{
				"status":        GenerationStatusSucceeded,
				"work_id":       workID,
				"error_code":    "",
				"error_message": "",
			}).Error
		_ = a.refreshCoupleAlbumStatus(albumID)
	}
}

func (a *App) refreshCoupleAlbumStatus(albumID uint) error {
	var pages []CoupleAlbumPage
	if err := a.db.Where("album_id = ?", albumID).Order("page_number ASC").Find(&pages).Error; err != nil {
		return err
	}
	status := CoupleAlbumStatusDraft
	var coverPageID *uint
	if len(pages) > 0 {
		status = CoupleAlbumStatusSucceeded
		successCount := 0
		failedCount := 0
		runningCount := 0
		for _, page := range pages {
			switch page.Status {
			case GenerationStatusSucceeded:
				successCount++
				if coverPageID == nil {
					pageID := page.ID
					coverPageID = &pageID
				}
			case GenerationStatusFailed:
				failedCount++
			default:
				runningCount++
			}
		}
		switch {
		case runningCount > 0:
			status = CoupleAlbumStatusGenerating
		case failedCount == len(pages):
			status = CoupleAlbumStatusFailed
		case failedCount > 0:
			status = CoupleAlbumStatusPartialFailed
		case successCount == len(pages):
			status = CoupleAlbumStatusSucceeded
		default:
			status = CoupleAlbumStatusGenerating
		}
	}
	return a.db.Model(&CoupleAlbum{}).Where("id = ?", albumID).Updates(map[string]any{
		"status":        status,
		"cover_page_id": coverPageID,
	}).Error
}

func (a *App) coupleAlbumPageWorks(pages []CoupleAlbumPage) map[uint]Work {
	workIDs := make([]uint, 0, len(pages))
	for _, page := range pages {
		if page.WorkID != nil && *page.WorkID > 0 {
			workIDs = append(workIDs, *page.WorkID)
		}
	}
	if len(workIDs) == 0 {
		return nil
	}
	var works []Work
	if err := a.db.Where("id IN ?", workIDs).Find(&works).Error; err != nil {
		return nil
	}
	byID := make(map[uint]Work, len(works))
	for _, work := range works {
		a.applyWorkPublicURL(&work)
		byID[work.ID] = work
	}
	return byID
}

func (a *App) coupleAlbumPayload(album CoupleAlbum, worksByID map[uint]Work, public bool) gin.H {
	pages := append([]CoupleAlbumPage(nil), album.Pages...)
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].PageNumber < pages[j].PageNumber })
	pagePayloads := make([]gin.H, 0, len(pages))
	for _, page := range pages {
		if public && page.Status != GenerationStatusSucceeded {
			continue
		}
		item := gin.H{
			"id":                   page.ID,
			"page_number":          page.PageNumber,
			"page_title":           page.PageTitle,
			"caption":              page.Caption,
			"status":               normalizeGenerationStatus(page.Status),
			"generation_record_id": uint(0),
			"work_id":              uint(0),
			"preview_url":          "",
			"download_url":         "",
			"error_code":           page.ErrorCode,
			"error_message":        page.ErrorMessage,
		}
		if page.GenerationRecordID != nil {
			item["generation_record_id"] = *page.GenerationRecordID
		}
		if page.WorkID != nil {
			item["work_id"] = *page.WorkID
			if work, ok := worksByID[*page.WorkID]; ok {
				if public {
					item["preview_url"] = publicWorkPreviewURL(work.ID)
				} else {
					item["preview_url"] = work.PreviewURL
				}
				item["download_url"] = work.DownloadURL
			}
		}
		pagePayloads = append(pagePayloads, item)
	}

	payload := gin.H{
		"id":             album.ID,
		"title":          album.Title,
		"location":       album.Location,
		"story_template": album.StoryTemplate,
		"style":          album.Style,
		"status":         album.Status,
		"cover_page_id":  album.CoverPageID,
		"pages":          pagePayloads,
		"created_at":     album.CreatedAt,
		"updated_at":     album.UpdatedAt,
	}
	if !public {
		payload["share_token"] = album.ShareToken
		payload["share_enabled"] = album.ShareEnabled
		payload["male_reference_asset_id"] = album.MaleReferenceAssetID
		payload["female_reference_asset_id"] = album.FemaleReferenceAssetID
	}
	return payload
}

func (a *App) coupleAlbumShareURL(token string) string {
	baseURL := strings.TrimRight(a.cfg.AppBaseURL, "/")
	if baseURL == "" {
		return "/pages/couple-album/share/index?token=" + url.QueryEscape(token)
	}
	return fmt.Sprintf("%s/pages/couple-album/share/index?token=%s", baseURL, url.QueryEscape(token))
}

func (a *App) buildCoupleAlbumPageSpecs(album CoupleAlbum) []coupleAlbumPageSpec {
	if isChildhoodCareerDreamAlbum(album) {
		return a.buildChildhoodCareerDreamPageSpecs(album)
	}
	location := fallbackString(strings.TrimSpace(album.Location), "旅途中")
	if label := a.coupleAlbumOptionPromptLabel(CoupleAlbumOptionTypeLocation, album.Location); label != "" {
		location = label
	}
	template := fallbackString(a.coupleAlbumOptionPromptLabel(CoupleAlbumOptionTypeStoryTemplate, album.StoryTemplate), coupleAlbumTemplateLabel(album.StoryTemplate))
	style := fallbackString(a.coupleAlbumOptionPromptLabel(CoupleAlbumOptionTypeStyle, album.Style), coupleAlbumStyleLabel(album.Style))
	moments := []struct {
		title   string
		caption string
		scene   string
	}{
		{"封面", "把这次出发写进只属于两个人的相册。", "旅行相册封面，男女主角并肩站在目的地标志性街景前，画面留出标题空间"},
		{"出发", "清晨的行李箱和第一张车票，都是故事的开场。", "清晨出发前的站台或机场，男女主角带着行李相视微笑"},
		{"抵达", "风吹过城市的第一秒，心也慢慢靠近。", "抵达目的地后的街口，两人第一次望向城市风景"},
		{"漫游", "走过热闹街巷，也走进彼此的日常。", "城市漫步，胶片感街拍，两人穿过有生活气息的小巷"},
		{"午后", "阳光落在杯沿，安静也变得很甜。", "午后咖啡馆或露台，两人分享甜点和饮品"},
		{"黄昏", "天色变柔软时，所有眼神都有答案。", "黄昏观景点，夕阳和远处城市轮廓，两人靠近看风景"},
		{"夜色", "灯火亮起，今天被收藏成一页浪漫。", "夜晚灯光街景，两人手牵手走过温暖光影"},
		{"纪念", "下一次旅行开始前，先把这一刻好好保存。", "纪念照，浪漫相册内页，两人面对镜头自然微笑，画面完整温暖"},
	}
	specs := make([]coupleAlbumPageSpec, 0, len(moments))
	for index, moment := range moments {
		prompt := fmt.Sprintf(
			"以两张参考照片中的男女主角为原型，参考图映射：【图1】为男主角参考，【图2】为女主角参考；保持各自人物身份、五官、发型和气质一致，不要换脸、不要互换角色、不要混合成陌生人、不要新增第三人。创作一本 520 情侣旅行故事相册的第 %d 页：%s。地点：%s。故事模板：%s。画面风格：%s。场景：%s。要求：浪漫但真实，像高端旅行写真，相册分页构图，人物自然互动，避免文字、水印、畸形手部和多余人物。",
			index+1,
			moment.title,
			location,
			template,
			style,
			moment.scene,
		)
		specs = append(specs, coupleAlbumPageSpec{
			PageNumber: index + 1,
			PageTitle:  moment.title,
			Caption:    moment.caption,
			Prompt:     prompt,
		})
	}
	return specs
}

func (a *App) buildChildhoodCareerDreamPageSpecs(album CoupleAlbum) []coupleAlbumPageSpec {
	style := fallbackString(a.coupleAlbumOptionPromptLabel(CoupleAlbumOptionTypeStyle, album.Style), coupleAlbumStyleLabel(album.Style))
	themePrompt := childhoodCareerDreamThemePrompt(album.Location)
	referenceInstruction := "以参考照片中的孩子为唯一主角，保持孩子身份、五官、发型和年龄感一致，保持脸型和气质一致。"
	if album.FemaleReferenceAssetID > 0 {
		referenceInstruction += "【图1】为孩子清晰正脸或半身参考，【图2】为全身体态与服装比例补充参考；不要把两张参考图识别为不同人物。"
	}
	moments := []struct {
		title   string
		caption string
		scene   string
	}{
		{"封面", "把六一的想象力收进一本梦想相册。", "生成一张六一儿童节“童年职业梦想相册”封面图。孩子站在梦幻舞台中央，穿干净可爱的节日服装，周围漂浮火箭、画笔、书本、星星、奖杯、彩色气球等梦想元素，背景明亮温暖，有童话绘本感和节日氛围。画面构图完整，高级儿童摄影海报风格，色彩清新，光线柔和，孩子表情自然开心。"},
		{"小小宇航员", "去月球基地看看蓝色地球。", "孩子穿着可爱的儿童宇航服，站在梦幻月球基地前，远处能看到蓝色地球、星空和小型火箭。画面像儿童节梦想相册内页，充满探索感和童话感，柔和电影光，干净明亮，孩子表情自信又开心。"},
		{"小小医生", "温柔守护每一个小小朋友。", "孩子穿儿童医生白大褂，佩戴玩具听诊器，在明亮温暖的卡通诊室里照顾一个玩偶病人，桌上有儿童医疗玩具、贴纸和小花。画面表现善良、守护和温暖，童话绘本风格，柔和自然光，色彩清新，孩子笑容自然。不要真实医疗恐怖元素，不要血腥。"},
		{"小小画家", "把彩虹、云朵和城堡画出来。", "孩子穿小围裙，坐在阳光洒落的儿童画室里，手拿画笔，正在画一幅彩虹、云朵、动物和童话城堡的画。周围有安全的颜料盘、彩色蜡笔、画架和纸张。画面梦幻、明亮、有创造力，适合六一儿童节相册，水彩绘本质感，孩子表情专注又开心。"},
		{"小小科学家", "把好奇心放进安全实验桌。", "孩子穿可爱的儿童实验服和护目镜，站在安全的儿童科学实验桌前，观察彩色液体瓶、星球模型、放大镜和显微镜。场景干净、安全、明亮，没有危险实验，整体像童年梦想相册内页，表现好奇心和探索精神，3D 动画电影质感，柔和光线，色彩丰富但高级。不要烟雾爆炸。"},
		{"小小厨师", "为儿童节做一只甜甜蛋糕。", "孩子戴儿童厨师帽、穿可爱围裙，在温馨明亮的厨房里制作六一儿童节蛋糕。桌上有水果、奶油、彩色糖霜、小饼干和安全厨具，画面温暖、甜美、干净。孩子表情开心自然，像高质量儿童摄影与绘本结合的相册内页。"},
		{"小小运动员", "阳光操场上，自信闪闪发光。", "孩子穿儿童运动服，站在阳光操场上，手拿小奖牌或运动器材，背景有跑道、彩旗、气球和柔和阳光。画面表现自信、活力和快乐，适合六一儿童节梦想相册，清新儿童写真海报风格，构图完整，色彩明亮自然。不要竞技压迫感。"},
		{"梦想纪念照", "所有梦想汇聚成这一张纪念照。", "孩子站在梦幻儿童节舞台中央，微笑看向镜头，背景融合宇航员、医生、画家、科学家、厨师、运动员等梦想元素，像一本童年职业梦想相册的最终纪念页。画面温暖、明亮、充满希望，童话绘本和高级儿童摄影结合，光线柔和，节日氛围浓厚。"},
	}
	specs := make([]coupleAlbumPageSpec, 0, len(moments))
	for index, moment := range moments {
		prompt := fmt.Sprintf(
			"%s创作一本六一儿童节童年职业梦想相册的第 %d 页：%s。梦想主题：%s。画面风格：%s。场景：%s儿童形象必须自然、健康、天真、年龄合适，全身或半身构图均可，手部、眼睛、表情要自然；不要成人化妆，不要不合适服装，不要性感姿态。不要文字，不要水印，不要logo，不要多余人物，不要畸形手，不要脸部变形。",
			referenceInstruction,
			index+1,
			moment.title,
			themePrompt,
			style,
			moment.scene,
		)
		specs = append(specs, coupleAlbumPageSpec{
			PageNumber: index + 1,
			PageTitle:  moment.title,
			Caption:    moment.caption,
			Prompt:     prompt,
		})
	}
	return specs
}

func childhoodCareerDreamThemePrompt(location string) string {
	switch strings.TrimSpace(location) {
	case "childhood_space_adventure":
		return "星际探索之旅，统一加入宇宙飞船、火箭、星球、星云和探索冒险元素，让每一页像孩子在太空任务中实现职业梦想"
	case "childhood_fairy_tale":
		return "童话奇遇记，统一加入童话城堡、魔法森林、星光小路、奇妙伙伴和温暖童话冒险元素，让每一页像孩子走进绘本故事"
	case "childhood_nature_explorer":
		return "自然小达人，统一加入森林、草地、花朵、昆虫观察、自然笔记和户外探索元素，让每一页像孩子在大自然中发现梦想"
	default:
		return "童年梦想舞台，统一加入儿童节舞台、彩色气球、星星、奖杯和职业梦想道具，让每一页像节日梦想秀"
	}
}

func coupleAlbumNegativePrompt(album CoupleAlbum) string {
	if isChildhoodCareerDreamAlbum(album) {
		return childhoodCareerDreamNegativePrompt
	}
	return ""
}

func isChildhoodCareerDreamRequest(req coupleAlbumCreateRequest) bool {
	return strings.TrimSpace(req.StoryTemplate) == childhoodCareerDreamStoryTemplateValue
}

func isChildhoodCareerDreamAlbum(album CoupleAlbum) bool {
	return strings.TrimSpace(album.StoryTemplate) == childhoodCareerDreamStoryTemplateValue
}

func coupleAlbumTemplateLabel(template string) string {
	switch strings.TrimSpace(template) {
	case "first_trip":
		return "第一次旅行"
	case "proposal":
		return "求婚纪念"
	case "anniversary":
		return "周年纪念"
	case "city_walk":
		return "城市漫游"
	case childhoodCareerDreamStoryTemplateValue:
		return "童年职业梦想相册"
	default:
		return fallbackString(strings.TrimSpace(template), "城市漫游")
	}
}

func coupleAlbumStyleLabel(style string) string {
	switch strings.TrimSpace(style) {
	case "film":
		return "旅行胶片"
	case "watercolor":
		return "清透水彩"
	case "cinematic":
		return "电影旅拍"
	case "storybook":
		return "绘本相册"
	case "children_storybook":
		return "童话绘本"
	case "dreamy_watercolor":
		return "梦幻水彩"
	case "animation_3d":
		return "3D 动画电影"
	case "children_photo_poster":
		return "儿童写真海报"
	default:
		return fallbackString(strings.TrimSpace(style), "旅行胶片")
	}
}

func uintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return uint(value), true
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
