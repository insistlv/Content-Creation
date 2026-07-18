package app

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

func (a *App) createVideoGenerationAuditRecord(tx *gorm.DB, record GenerationRecord, job *videoGenerationJob) error {
	videoRecord := a.videoGenerationRecordFromJob(record, job)
	return tx.Create(&videoRecord).Error
}

func (a *App) syncVideoGenerationAuditRecord(record GenerationRecord, job *videoGenerationJob) error {
	return a.syncVideoGenerationAuditRecordTx(a.db, record, job)
}

func (a *App) syncVideoGenerationAuditRecordTx(tx *gorm.DB, record GenerationRecord, job *videoGenerationJob) error {
	updates := a.videoGenerationRecordUpdates(record, job)
	result := tx.Model(&VideoGenerationRecord{}).
		Where("generation_record_id = ?", record.ID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	videoRecord := a.videoGenerationRecordFromJob(record, job)
	return tx.Create(&videoRecord).Error
}

func (a *App) failVideoGenerationRecord(record *GenerationRecord, job *videoGenerationJob, code, message string) {
	a.failGenerationRecord(record, code, message)
	_ = a.syncVideoGenerationAuditRecord(*record, job)
}

func applyVideoProviderError(record *GenerationRecord, providerErr *ProviderError) {
	if providerErr == nil {
		return
	}
	record.ProviderRequestID = strings.TrimSpace(providerErr.ProviderRequestID)
	record.ProviderHTTPStatus = providerErr.HTTPStatus
	record.ProviderErrorCode = strings.TrimSpace(providerErr.Code)
	record.ProviderErrorMessage = strings.TrimSpace(providerErr.Message)
	record.ProviderFailureStage = strings.TrimSpace(providerErr.FailureStage)
	record.ProviderAttemptCount = providerErr.AttemptCount
}

func (a *App) videoGenerationRecordFromJob(record GenerationRecord, job *videoGenerationJob) VideoGenerationRecord {
	modelConfigID := record.ModelConfigID
	if modelConfigID == 0 {
		modelConfigID = modelConfigIDValue(jobModelConfig(job))
	}
	videoRecord := VideoGenerationRecord{
		GenerationRecordID:   record.ID,
		ConversationID:       job.ConversationID,
		Progress:             record.Progress,
		UserID:               record.UserID,
		WorkID:               record.WorkID,
		Source:               videoGenerationJobSource(job),
		Prompt:               record.Prompt,
		AspectRatio:          record.AspectRatio,
		StylePreset:          record.StylePreset,
		DurationSeconds:      videoGenerationJobDurationSeconds(job, record),
		InputImageCount:      videoGenerationJobInputImageCount(job),
		ReferenceAssetCount:  videoGenerationJobReferenceAssetCount(job),
		ModelConfigID:        modelConfigID,
		ModelName:            fallbackString(record.ModelName, modelConfigName(jobModelConfig(job))),
		RuntimeModel:         fallbackString(record.RuntimeModel, fallbackString(modelConfigRuntime(jobModelConfig(job)), record.Model)),
		Provider:             modelConfigProvider(jobModelConfig(job)),
		ProviderRequestID:    record.ProviderRequestID,
		Status:               record.Status,
		Stage:                record.Stage,
		ErrorCode:            record.ErrorCode,
		ErrorMessage:         record.ErrorMessage,
		ProviderHTTPStatus:   record.ProviderHTTPStatus,
		ProviderErrorCode:    record.ProviderErrorCode,
		ProviderErrorMessage: record.ProviderErrorMessage,
		ProviderFailureStage: record.ProviderFailureStage,
		LatencyMS:            record.LatencyMS,
		CreditsCost:          record.CreditsCost,
		CreditsDeducted:      record.CreditsDeducted,
		AssetKey:             record.AssetKey,
		PreviewURL:           record.PreviewURL,
		DownloadURL:          record.DownloadURL,
		MIMEType:             record.MIMEType,
		MetadataJSON:         videoGenerationMetadataJSON(job),
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
	}
	if job != nil {
		videoRecord.NovelVideoProjectID = job.NovelVideoProjectID
		videoRecord.NovelVideoEpisodeID = job.NovelVideoEpisodeID
		videoRecord.NovelVideoShotID = job.NovelVideoShotID
		videoRecord.NovelVideoAttemptID = job.NovelVideoAttemptID
	}
	return videoRecord
}

func (a *App) videoGenerationRecordUpdates(record GenerationRecord, job *videoGenerationJob) map[string]any {
	videoRecord := a.videoGenerationRecordFromJob(record, job)
	return map[string]any{
		"user_id":                videoRecord.UserID,
		"conversation_id":        videoRecord.ConversationID,
		"progress":               videoRecord.Progress,
		"work_id":                videoRecord.WorkID,
		"source":                 videoRecord.Source,
		"novel_video_project_id": videoRecord.NovelVideoProjectID,
		"novel_video_episode_id": videoRecord.NovelVideoEpisodeID,
		"novel_video_shot_id":    videoRecord.NovelVideoShotID,
		"novel_video_attempt_id": videoRecord.NovelVideoAttemptID,
		"prompt":                 videoRecord.Prompt,
		"aspect_ratio":           videoRecord.AspectRatio,
		"style_preset":           videoRecord.StylePreset,
		"duration_seconds":       videoRecord.DurationSeconds,
		"input_image_count":      videoRecord.InputImageCount,
		"reference_asset_count":  videoRecord.ReferenceAssetCount,
		"model_config_id":        videoRecord.ModelConfigID,
		"model_name":             videoRecord.ModelName,
		"runtime_model":          videoRecord.RuntimeModel,
		"provider":               videoRecord.Provider,
		"provider_request_id":    videoRecord.ProviderRequestID,
		"status":                 videoRecord.Status,
		"stage":                  videoRecord.Stage,
		"error_code":             videoRecord.ErrorCode,
		"error_message":          videoRecord.ErrorMessage,
		"provider_http_status":   videoRecord.ProviderHTTPStatus,
		"provider_error_code":    videoRecord.ProviderErrorCode,
		"provider_error_message": videoRecord.ProviderErrorMessage,
		"provider_failure_stage": videoRecord.ProviderFailureStage,
		"latency_ms":             videoRecord.LatencyMS,
		"credits_cost":           videoRecord.CreditsCost,
		"credits_deducted":       videoRecord.CreditsDeducted,
		"asset_key":              videoRecord.AssetKey,
		"preview_url":            videoRecord.PreviewURL,
		"download_url":           videoRecord.DownloadURL,
		"mime_type":              videoRecord.MIMEType,
		"metadata_json":          videoRecord.MetadataJSON,
	}
}

func videoGenerationMetadataJSON(job *videoGenerationJob) string {
	if job == nil || job.ProviderUsageTokens <= 0 {
		return ""
	}
	metadata := map[string]any{
		"provider":      modelConfigProvider(job.ModelConfig),
		"runtime_model": fallbackString(job.Request.Model, modelConfigRuntime(job.ModelConfig)),
		"usage": map[string]any{
			"total_tokens": job.ProviderUsageTokens,
		},
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (a *App) backfillVideoGenerationRecords() error {
	if !a.db.Migrator().HasTable(&VideoGenerationRecord{}) || !a.db.Migrator().HasTable(&GenerationRecord{}) {
		return nil
	}
	var workRecordIDs []uint
	if a.db.Migrator().HasTable(&Work{}) {
		if err := a.db.Model(&Work{}).
			Where("category = ? AND generation_record_id <> ?", WorkCategoryVideo, 0).
			Distinct().
			Pluck("generation_record_id", &workRecordIDs).Error; err != nil {
			return err
		}
	}

	query := a.db.Model(&GenerationRecord{}).
		Where("tool_mode = ? OR mime_type LIKE ?", "video", "video/%")
	if len(workRecordIDs) > 0 {
		query = query.Or("id IN ?", workRecordIDs)
	}
	var records []GenerationRecord
	if err := query.Order("created_at asc, id asc").Find(&records).Error; err != nil {
		return err
	}
	for _, record := range records {
		if err := a.backfillOneVideoGenerationRecord(record); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) backfillOneVideoGenerationRecord(record GenerationRecord) error {
	var existing VideoGenerationRecord
	err := a.db.Where("generation_record_id = ?", record.ID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	videoRecord, err := a.videoGenerationRecordFromExistingGeneration(record)
	if err != nil {
		return err
	}
	return a.db.Create(&videoRecord).Error
}

func (a *App) videoGenerationRecordFromExistingGeneration(record GenerationRecord) (VideoGenerationRecord, error) {
	videoRecord := VideoGenerationRecord{
		GenerationRecordID:   record.ID,
		UserID:               record.UserID,
		WorkID:               record.WorkID,
		Source:               VideoGenerationSourceWorkspace,
		Prompt:               record.Prompt,
		AspectRatio:          record.AspectRatio,
		DurationSeconds:      durationSecondsFromGenerationRecord(record),
		ModelConfigID:        record.ModelConfigID,
		ModelName:            record.ModelName,
		RuntimeModel:         fallbackString(record.RuntimeModel, record.Model),
		ProviderRequestID:    record.ProviderRequestID,
		Status:               record.Status,
		Stage:                record.Stage,
		ErrorCode:            record.ErrorCode,
		ErrorMessage:         record.ErrorMessage,
		ProviderHTTPStatus:   record.ProviderHTTPStatus,
		ProviderErrorCode:    record.ProviderErrorCode,
		ProviderErrorMessage: record.ProviderErrorMessage,
		ProviderFailureStage: record.ProviderFailureStage,
		LatencyMS:            record.LatencyMS,
		CreditsCost:          record.CreditsCost,
		CreditsDeducted:      record.CreditsDeducted,
		AssetKey:             record.AssetKey,
		PreviewURL:           record.PreviewURL,
		DownloadURL:          record.DownloadURL,
		MIMEType:             record.MIMEType,
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
	}
	model, err := a.videoModelConfigForGenerationRecord(record)
	if err != nil {
		return videoRecord, err
	}
	if model != nil {
		videoRecord.ModelConfigID = model.ID
		videoRecord.ModelName = fallbackString(videoRecord.ModelName, model.Name)
		videoRecord.RuntimeModel = fallbackString(videoRecord.RuntimeModel, model.RuntimeModel)
		videoRecord.Provider = model.Provider
	}
	if videoRecord.WorkID == nil {
		var work Work
		if err := a.db.Where("generation_record_id = ? AND category = ?", record.ID, WorkCategoryVideo).First(&work).Error; err == nil {
			videoRecord.WorkID = &work.ID
			videoRecord.PreviewURL = fallbackString(videoRecord.PreviewURL, work.PreviewURL)
			videoRecord.DownloadURL = fallbackString(videoRecord.DownloadURL, work.DownloadURL)
			videoRecord.MIMEType = fallbackString(videoRecord.MIMEType, work.MIMEType)
		}
	}
	var referenceCount int64
	if a.db.Migrator().HasTable(&GenerationReferenceAsset{}) {
		if err := a.db.Model(&GenerationReferenceAsset{}).Where("generation_record_id = ?", record.ID).Count(&referenceCount).Error; err != nil {
			return videoRecord, err
		}
	}
	videoRecord.ReferenceAssetCount = int(referenceCount)
	videoRecord.InputImageCount = int(referenceCount)
	a.applyNovelVideoLinkage(&videoRecord)
	return videoRecord, nil
}

func (a *App) videoModelConfigForGenerationRecord(record GenerationRecord) (*ModelConfig, error) {
	var model ModelConfig
	if record.ModelConfigID > 0 {
		err := a.db.First(&model, record.ModelConfigID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return &model, err
	}
	runtimeModel := canonicalVideoRuntimeModel(fallbackString(record.RuntimeModel, record.Model))
	if strings.TrimSpace(runtimeModel) == "" {
		return nil, nil
	}
	err := a.db.
		Where("type = ? AND (runtime_model = ? OR name = ?)", ModelConfigTypeVideo, runtimeModel, runtimeModel).
		Order("runtime_model desc, sort_order asc, id asc").
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &model, err
}

func (a *App) applyNovelVideoLinkage(videoRecord *VideoGenerationRecord) {
	var shot NovelVideoShot
	if err := a.db.Where("generation_record_id = ?", videoRecord.GenerationRecordID).First(&shot).Error; err == nil {
		videoRecord.Source = VideoGenerationSourceNovelShot
		videoRecord.NovelVideoProjectID = &shot.ProjectID
		videoRecord.NovelVideoEpisodeID = &shot.EpisodeID
		videoRecord.NovelVideoShotID = &shot.ID
	}
	var attempt NovelVideoShotRenderAttempt
	if err := a.db.Where("generation_record_id = ?", videoRecord.GenerationRecordID).First(&attempt).Error; err == nil {
		videoRecord.Source = VideoGenerationSourceNovelShot
		videoRecord.NovelVideoProjectID = &attempt.ProjectID
		videoRecord.NovelVideoEpisodeID = &attempt.EpisodeID
		videoRecord.NovelVideoShotID = &attempt.ShotID
		videoRecord.NovelVideoAttemptID = &attempt.ID
	}
}

func videoGenerationJobSource(job *videoGenerationJob) string {
	if job == nil || strings.TrimSpace(job.Source) == "" {
		return VideoGenerationSourceWorkspace
	}
	return strings.TrimSpace(job.Source)
}

func videoGenerationJobDurationSeconds(job *videoGenerationJob, record GenerationRecord) int {
	if job != nil {
		if seconds, err := strconv.Atoi(strings.TrimSpace(job.Request.Duration)); err == nil {
			return seconds
		}
	}
	return durationSecondsFromGenerationRecord(record)
}

func durationSecondsFromGenerationRecord(record GenerationRecord) int {
	text := strings.TrimSpace(record.StylePreset)
	text = strings.TrimSuffix(text, "s")
	text = strings.TrimSuffix(text, "S")
	seconds, _ := strconv.Atoi(strings.TrimSpace(text))
	return seconds
}

func videoGenerationJobInputImageCount(job *videoGenerationJob) int {
	if job == nil {
		return 0
	}
	return len(job.Request.Images) + len(job.ReferenceAssets)
}

func videoGenerationJobReferenceAssetCount(job *videoGenerationJob) int {
	if job == nil {
		return 0
	}
	return len(job.ReferenceAssets)
}

func jobModelConfig(job *videoGenerationJob) *ModelConfig {
	if job == nil {
		return nil
	}
	return job.ModelConfig
}

func modelConfigName(model *ModelConfig) string {
	if model == nil {
		return ""
	}
	return strings.TrimSpace(model.Name)
}

func modelConfigProvider(model *ModelConfig) string {
	if model == nil {
		return ""
	}
	return strings.TrimSpace(model.Provider)
}
