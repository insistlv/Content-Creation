package app

// 本文件从 platform_handlers.go 拆分：管理端生成审计列表、详情与导出。

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (a *App) handleListGenerations(c *gin.Context) {
	if err := a.expireStaleImageGenerations(time.Now()); err != nil {
		writeError(c, http.StatusInternalServerError, "generations_timeout_cleanup_failed", "生成记录状态刷新失败")
		return
	}

	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	filters, err := adminGenerationFiltersFromQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}

	var total int64
	if err := a.adminGenerationsQuery(filters).Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "generations_load_failed", "生成记录读取失败")
		return
	}

	var rows []adminGenerationListRow
	if err := a.adminGenerationsQuery(filters).
		Select("generation_records.id, generation_records.user_id, generation_records.work_id, generation_records.prompt, generation_records.model_id, generation_records.channel_id, generation_records.model_config_id, COALESCE(NULLIF(generation_records.model_name, ''), model_configs.name) AS model_name, generation_records.channel_name AS channel_name, COALESCE(NULLIF(generation_records.runtime_model, ''), model_configs.runtime_model) AS runtime_model, generation_records.model, generation_records.status, generation_records.latency_ms, generation_records.error_code, generation_records.provider_http_status, generation_records.provider_error_code, generation_records.provider_error_message, generation_records.provider_failure_stage, generation_records.provider_attempt_count, generation_records.preview_url, generation_records.download_url, generation_records.mime_type, generation_records.credits_cost, generation_records.credits_deducted, generation_records.created_at, users.username, users.display_name, users.email, users.avatar_url").
		Order("generation_records.created_at desc, generation_records.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "generations_load_failed", "生成记录读取失败")
		return
	}
	summary, err := a.adminGenerationSummary(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "generations_load_failed", "生成记录读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     adminGenerationItemsFromRows(rows),
		"summary":   summary,
		"filters":   filters,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

func (a *App) handleGetAdminGeneration(c *gin.Context) {
	if err := a.expireStaleImageGenerations(time.Now()); err != nil {
		writeError(c, http.StatusInternalServerError, "generation_timeout_cleanup_failed", "生成记录状态刷新失败")
		return
	}

	var record GenerationRecord
	if err := a.db.First(&record, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "generation_not_found", "生成记录不存在")
		return
	}
	user := adminGenerationUserSnapshot{ID: record.UserID}
	var dbUser User
	if err := a.db.First(&dbUser, record.UserID).Error; err == nil {
		user = adminGenerationUserSnapshot{
			ID:          dbUser.ID,
			Username:    dbUser.Username,
			DisplayName: dbUser.DisplayName,
			Email:       dbUser.Email,
			AvatarURL:   dbUser.AvatarURL,
		}
	}

	var errorPayload *adminGenerationErrorPayload
	if record.Status == GenerationStatusFailed || strings.TrimSpace(record.ErrorCode) != "" || strings.TrimSpace(record.ErrorMessage) != "" {
		errorPayload = &adminGenerationErrorPayload{
			Code:    record.ErrorCode,
			Message: record.ErrorMessage,
		}
	}

	var sourceImage *adminGenerationImagePayload
	if record.SourceWorkID != nil {
		var sourceWork Work
		if err := a.db.First(&sourceWork, *record.SourceWorkID).Error; err == nil && sourceWork.PreviewURL != "" {
			sourceImage = &adminGenerationImagePayload{
				WorkID:      &sourceWork.ID,
				PreviewURL:  sourceWork.PreviewURL,
				DownloadURL: sourceWork.DownloadURL,
				MIMEType:    sourceWork.MIMEType,
			}
		}
	}
	referenceImages, err := a.adminGenerationReferenceImages(record.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "generation_reference_images_load_failed", "参考图片读取失败")
		return
	}
	var eventLogs []GenerationEventLog
	if err := a.db.Where("generation_record_id = ?", record.ID).Order("created_at asc, id asc").Find(&eventLogs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "generation_events_load_failed", "生成调用日志读取失败")
		return
	}
	modelName, channelName, runtimeModel := a.adminGenerationModelIdentity(record)

	writeJSON(c, http.StatusOK, adminGenerationDetailPayload{
		ID:            record.ID,
		TaskID:        fmt.Sprintf("GEN-%d", record.ID),
		UserID:        record.UserID,
		WorkID:        record.WorkID,
		User:          user,
		CreatedAt:     record.CreatedAt,
		Status:        record.Status,
		ModelID:       record.ModelID,
		ChannelID:     record.ChannelID,
		ModelConfigID: record.ModelConfigID,
		ModelName:     modelName,
		ChannelName:   channelName,
		RuntimeModel:  runtimeModel,
		Model:         record.Model,
		LatencyMS:     record.LatencyMS,
		CreditsCost:   generationCreditsCost(record.CreditsCost, record.CreditsDeducted),
		Prompt:        record.Prompt,
		Params: adminGenerationParamsPayload{
			NegativePrompt:  record.NegativePrompt,
			AspectRatio:     record.AspectRatio,
			Quality:         record.Quality,
			StylePreset:     record.StylePreset,
			ToolMode:        record.ToolMode,
			StyleStrength:   record.StyleStrength,
			ReferenceWeight: record.ReferenceWeight,
			Seed:            record.Seed,
		},
		ResultImages:    generationResultImages(record.WorkID, record.PreviewURL, record.DownloadURL, record.MIMEType),
		ReferenceImages: referenceImages,
		SourceImage:     sourceImage,
		Error:           errorPayload,
		ProviderDiagnostics: adminGenerationProviderDiagnosticsPayload{
			HTTPStatus:   record.ProviderHTTPStatus,
			ErrorCode:    record.ProviderErrorCode,
			ErrorMessage: record.ProviderErrorMessage,
			FailureStage: record.ProviderFailureStage,
			AttemptCount: record.ProviderAttemptCount,
		},
		Events: adminGenerationEventPayloads(eventLogs),
	})
}

func (a *App) handleExportGenerations(c *gin.Context) {
	filters, err := adminGenerationFiltersFromQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	var rows []adminGenerationListRow
	if err := a.adminGenerationsQuery(filters).
		Select("generation_records.id, generation_records.user_id, generation_records.work_id, generation_records.prompt, generation_records.model_id, generation_records.channel_id, generation_records.model_config_id, COALESCE(NULLIF(generation_records.model_name, ''), model_configs.name) AS model_name, generation_records.channel_name AS channel_name, COALESCE(NULLIF(generation_records.runtime_model, ''), model_configs.runtime_model) AS runtime_model, generation_records.model, generation_records.status, generation_records.latency_ms, generation_records.error_code, generation_records.provider_http_status, generation_records.provider_error_code, generation_records.provider_error_message, generation_records.provider_failure_stage, generation_records.provider_attempt_count, generation_records.preview_url, generation_records.download_url, generation_records.mime_type, generation_records.credits_cost, generation_records.credits_deducted, generation_records.created_at, users.username, users.display_name, users.email, users.avatar_url").
		Order("generation_records.created_at desc, generation_records.id desc").
		Scan(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "generations_export_failed", "生成记录导出失败")
		return
	}
	csvRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		csvRows = append(csvRows, []string{
			strconv.FormatUint(uint64(row.ID), 10),
			strconv.FormatUint(uint64(row.UserID), 10),
			fallbackString(row.DisplayName, row.Username),
			row.Username,
			row.Prompt,
			row.Model,
			row.Status,
			strconv.FormatInt(row.LatencyMS, 10),
			strconv.Itoa(generationCreditsCost(row.CreditsCost, row.CreditsDeducted)),
			row.CreatedAt.Format(time.RFC3339),
			row.PreviewURL,
			row.DownloadURL,
			row.ErrorCode,
			strconv.Itoa(row.ProviderHTTPStatus),
			row.ProviderErrorCode,
			row.ProviderErrorMessage,
			row.ProviderFailureStage,
			strconv.Itoa(row.ProviderAttemptCount),
		})
	}
	writeCSV(c, "generations.csv", []string{"任务ID", "用户ID", "用户", "账号", "提示词", "模型", "状态", "耗时(ms)", "点数消耗", "创建时间", "预览图", "下载链接", "错误码", "供应商HTTP状态", "供应商错误码", "供应商错误消息", "供应商失败阶段", "供应商尝试次数"}, csvRows)
}

func adminGenerationFiltersFromQuery(c *gin.Context) (adminGenerationFilters, error) {
	filters := adminGenerationFilters{
		Query:       strings.TrimSpace(c.Query("q")),
		Model:       strings.TrimSpace(c.Query("model")),
		UserKeyword: strings.TrimSpace(c.Query("user_keyword")),
		Status:      strings.TrimSpace(c.Query("status")),
		DateFrom:    strings.TrimSpace(c.Query("date_from")),
		DateTo:      strings.TrimSpace(c.Query("date_to")),
	}
	if filters.Status == "all" {
		filters.Status = ""
	}
	if userID := getQueryInt(c, "user_id", 0); userID > 0 {
		filters.UserID = uint(userID)
	}
	if modelConfigID := getQueryInt(c, "model_config_id", 0); modelConfigID > 0 {
		filters.ModelConfigID = uint(modelConfigID)
	}
	if modelID := getQueryInt(c, "model_id", 0); modelID > 0 {
		filters.ModelID = uint(modelID)
	}
	if channelID := getQueryInt(c, "channel_id", 0); channelID > 0 {
		filters.ChannelID = uint(channelID)
	}
	if _, err := parseDateFilter(filters.DateFrom); err != nil {
		return filters, err
	}
	if _, err := parseDateFilter(filters.DateTo); err != nil {
		return filters, err
	}
	return filters, nil
}

func (a *App) adminGenerationsQuery(filters adminGenerationFilters) *gorm.DB {
	query := a.db.Model(&GenerationRecord{}).
		Joins("LEFT JOIN users ON users.id = generation_records.user_id").
		Joins("LEFT JOIN model_configs ON model_configs.id = generation_records.model_config_id")
	if filters.Query != "" {
		like := "%" + filters.Query + "%"
		query = query.Where("generation_records.prompt LIKE ? OR generation_records.negative_prompt LIKE ?", like, like)
	}
	if filters.ModelID > 0 {
		query = query.Where("generation_records.model_id = ?", filters.ModelID)
	}
	if filters.ChannelID > 0 {
		query = query.Where("generation_records.channel_id = ?", filters.ChannelID)
	}
	if filters.ModelConfigID > 0 {
		query = query.Where("generation_records.model_config_id = ?", filters.ModelConfigID)
	} else if filters.ModelID == 0 && filters.ChannelID == 0 && filters.Model != "" && filters.Model != "all" {
		query = query.Where("generation_records.model = ?", filters.Model)
	}
	if filters.UserID > 0 {
		query = query.Where("generation_records.user_id = ?", filters.UserID)
	}
	if filters.UserKeyword != "" {
		like := "%" + strings.ToLower(filters.UserKeyword) + "%"
		query = query.Where("(LOWER(users.username) LIKE ? OR users.phone = ?)", like, filters.UserKeyword)
	}
	if filters.Status != "" {
		query = query.Where("generation_records.status = ?", filters.Status)
	}
	if filters.DateFrom != "" {
		from, _ := parseDateFilter(filters.DateFrom)
		query = query.Where("generation_records.created_at >= ?", *from)
	}
	if filters.DateTo != "" {
		to, _ := parseDateFilter(filters.DateTo)
		query = query.Where("generation_records.created_at < ?", to.AddDate(0, 0, 1))
	}
	return query
}

func (a *App) adminGenerationSummary(now time.Time) (adminGenerationSummary, error) {
	var summary adminGenerationSummary
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	todayTotal, todaySucceeded, todayFailed, todayAverage, err := a.generationStatsForRange(todayStart, tomorrowStart)
	if err != nil {
		return summary, err
	}
	yesterdayTotal, yesterdaySucceeded, yesterdayFailed, yesterdayAverage, err := a.generationStatsForRange(yesterdayStart, todayStart)
	if err != nil {
		return summary, err
	}

	summary.TodayGenerations = todayTotal
	summary.TodayGenerationsDeltaPercent = percentChange(todayTotal, yesterdayTotal)
	summary.SuccessRate = generationSuccessRate(todaySucceeded, todayTotal)
	summary.SuccessRateDeltaPercent = roundPercent(summary.SuccessRate - generationSuccessRate(yesterdaySucceeded, yesterdayTotal))
	summary.AverageLatencyMS = todayAverage
	summary.AverageLatencyDeltaPercent = percentChange(todayAverage, yesterdayAverage)
	summary.FailedTasks = todayFailed
	summary.FailedTasksDeltaPercent = percentChange(todayFailed, yesterdayFailed)
	return summary, nil
}

func (a *App) generationStatsForRange(start, end time.Time) (total, succeeded, failed, averageLatency int64, err error) {
	if err = a.db.Model(&GenerationRecord{}).Where("created_at >= ? AND created_at < ?", start, end).Count(&total).Error; err != nil {
		return
	}
	if err = a.db.Model(&GenerationRecord{}).Where("created_at >= ? AND created_at < ? AND status = ?", start, end, GenerationStatusSucceeded).Count(&succeeded).Error; err != nil {
		return
	}
	if err = a.db.Model(&GenerationRecord{}).Where("created_at >= ? AND created_at < ? AND status = ?", start, end, GenerationStatusFailed).Count(&failed).Error; err != nil {
		return
	}
	var average float64
	if err = a.db.Model(&GenerationRecord{}).
		Where("created_at >= ? AND created_at < ? AND latency_ms > 0", start, end).
		Select("COALESCE(AVG(latency_ms), 0)").
		Scan(&average).Error; err != nil {
		return
	}
	averageLatency = int64(math.Round(average))
	return
}

func adminGenerationItemsFromRows(rows []adminGenerationListRow) []adminGenerationListItem {
	items := make([]adminGenerationListItem, 0, len(rows))
	now := time.Now()
	for _, row := range rows {
		items = append(items, adminGenerationListItem{
			ID:            row.ID,
			UserID:        row.UserID,
			WorkID:        row.WorkID,
			User:          adminGenerationUserFromRow(row),
			PromptSummary: promptSummary(row.Prompt),
			PreviewImages: generationResultImages(row.WorkID, row.PreviewURL, row.DownloadURL, row.MIMEType),
			ModelID:       row.ModelID,
			ChannelID:     row.ChannelID,
			ModelConfigID: row.ModelConfigID,
			ModelName:     row.ModelName,
			ChannelName:   row.ChannelName,
			RuntimeModel:  fallbackString(row.RuntimeModel, row.Model),
			Model:         row.Model,
			Status:        row.Status,
			LatencyMS:     adminGenerationLatencyMS(row, now),
			CreditsCost:   generationCreditsCost(row.CreditsCost, row.CreditsDeducted),
			ErrorCode:     row.ErrorCode,
			CreatedAt:     row.CreatedAt,
		})
	}
	return items
}

func (a *App) adminGenerationModelIdentity(record GenerationRecord) (string, string, string) {
	modelName := strings.TrimSpace(record.ModelName)
	channelName := strings.TrimSpace(record.ChannelName)
	runtimeModel := fallbackString(strings.TrimSpace(record.RuntimeModel), strings.TrimSpace(record.Model))
	if record.ModelID != 0 && modelName == "" {
		var model ModelCatalog
		if err := a.db.Unscoped().First(&model, record.ModelID).Error; err == nil {
			modelName = model.Name
		}
	}
	if record.ChannelID != 0 && (channelName == "" || runtimeModel == "") {
		var channel ModelChannel
		if err := a.db.Unscoped().First(&channel, record.ChannelID).Error; err == nil {
			if channelName == "" {
				channelName = channel.Name
			}
			if runtimeModel == "" {
				runtimeModel = channel.RuntimeModel
			}
		}
	}
	if record.ModelConfigID == 0 {
		return modelName, channelName, runtimeModel
	}
	var model ModelConfig
	if err := a.db.Unscoped().First(&model, record.ModelConfigID).Error; err != nil {
		return modelName, channelName, runtimeModel
	}
	if runtimeModel == "" {
		runtimeModel = strings.TrimSpace(model.RuntimeModel)
	}
	if runtimeModel == "" {
		runtimeModel = strings.TrimSpace(model.Name)
	}
	if modelName == "" {
		modelName = model.Name
	}
	return modelName, channelName, runtimeModel
}

func adminGenerationLatencyMS(row adminGenerationListRow, now time.Time) int64 {
	if row.Status != GenerationStatusQueued && row.Status != GenerationStatusRunning {
		return row.LatencyMS
	}
	if row.CreatedAt.IsZero() {
		return row.LatencyMS
	}
	elapsed := now.Sub(row.CreatedAt).Milliseconds()
	if elapsed <= 0 {
		return row.LatencyMS
	}
	return elapsed
}

func adminGenerationUserFromRow(row adminGenerationListRow) adminGenerationUserSnapshot {
	return adminGenerationUserSnapshot{
		ID:          row.UserID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Email:       row.Email,
		AvatarURL:   row.AvatarURL,
	}
}

func generationResultImages(workID *uint, previewURL, downloadURL, mimeType string) []adminGenerationImagePayload {
	if strings.TrimSpace(previewURL) == "" && strings.TrimSpace(downloadURL) == "" {
		return []adminGenerationImagePayload{}
	}
	return []adminGenerationImagePayload{{
		WorkID:      workID,
		PreviewURL:  previewURL,
		DownloadURL: downloadURL,
		MIMEType:    mimeType,
	}}
}

func (a *App) adminGenerationReferenceImages(recordID uint) ([]adminGenerationReferenceImagePayload, error) {
	var links []GenerationReferenceAsset
	if err := a.db.Where("generation_record_id = ?", recordID).Order("sort_order asc, id asc").Find(&links).Error; err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return []adminGenerationReferenceImagePayload{}, nil
	}

	ids := make([]uint, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.ReferenceAssetID)
	}
	var assets []ReferenceAsset
	if err := a.db.Unscoped().Where("id IN ?", ids).Find(&assets).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]ReferenceAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}

	items := make([]adminGenerationReferenceImagePayload, 0, len(links))
	for _, link := range links {
		asset, exists := byID[link.ReferenceAssetID]
		if !exists {
			continue
		}
		previewURL, err := a.referenceAssetAccessURL(asset, "image/png", true, true)
		if err != nil {
			return nil, err
		}
		items = append(items, adminGenerationReferenceImagePayload{
			ReferenceAssetID: asset.ID,
			PreviewURL:       previewURL,
			DownloadURL:      previewURL,
			MIMEType:         asset.MIMEType,
			OriginalFilename: asset.OriginalFilename,
			SortOrder:        link.SortOrder,
		})
	}
	return items, nil
}

func adminGenerationEventPayloads(events []GenerationEventLog) []adminGenerationEventPayload {
	items := make([]adminGenerationEventPayload, 0, len(events))
	for _, event := range events {
		metadata := map[string]any{}
		if strings.TrimSpace(event.MetadataJSON) != "" {
			_ = json.Unmarshal([]byte(event.MetadataJSON), &metadata)
		}
		items = append(items, adminGenerationEventPayload{
			ID:        event.ID,
			TraceID:   event.TraceID,
			Level:     event.Level,
			Stage:     event.Stage,
			Event:     event.Event,
			Message:   event.Message,
			Metadata:  metadata,
			CreatedAt: event.CreatedAt,
		})
	}
	return items
}

func generationCreditsCost(creditsCost int, creditsDeducted bool) int {
	if creditsCost > 0 {
		return creditsCost
	}
	if creditsDeducted {
		return 1
	}
	return 0
}

func promptSummary(prompt string) string {
	text := strings.TrimSpace(prompt)
	runes := []rune(text)
	if len(runes) <= 64 {
		return text
	}
	return string(runes[:64]) + "..."
}

func generationSuccessRate(succeeded, total int64) float64 {
	if total == 0 {
		return 0
	}
	return roundPercent((float64(succeeded) / float64(total)) * 100)
}

func roundPercent(value float64) float64 {
	return math.Round(value*10) / 10
}
