package app

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminVideoGenerationFilters struct {
	Query         string `json:"q"`
	UserID        uint   `json:"user_id"`
	UserKeyword   string `json:"user_keyword"`
	Status        string `json:"status"`
	Source        string `json:"source"`
	Provider      string `json:"provider"`
	RuntimeModel  string `json:"runtime_model"`
	ModelConfigID uint   `json:"model_config_id"`
	DateFrom      string `json:"date_from"`
	DateTo        string `json:"date_to"`
}

type adminVideoGenerationSummary struct {
	TodayVideos                int64   `json:"today_videos"`
	TodayVideosDeltaPercent    float64 `json:"today_videos_delta_percent"`
	SuccessRate                float64 `json:"success_rate"`
	SuccessRateDeltaPercent    float64 `json:"success_rate_delta_percent"`
	AverageLatencyMS           int64   `json:"average_latency_ms"`
	AverageLatencyDeltaPercent float64 `json:"average_latency_delta_percent"`
	FailedTasks                int64   `json:"failed_tasks"`
	FailedTasksDeltaPercent    float64 `json:"failed_tasks_delta_percent"`
}

type adminVideoGenerationListRow struct {
	ID                 uint
	GenerationRecordID uint
	UserID             uint
	WorkID             *uint
	Source             string
	Prompt             string
	AspectRatio        string
	StylePreset        string
	DurationSeconds    int
	ModelConfigID      uint
	ModelName          string
	RuntimeModel       string
	Provider           string
	ProviderRequestID  string
	Status             string
	Stage              string
	ErrorCode          string
	ErrorMessage       string
	LatencyMS          int64
	CreditsCost        int
	CreditsDeducted    bool
	PreviewURL         string
	DownloadURL        string
	MIMEType           string
	CreatedAt          time.Time
	Username           string
	DisplayName        string
	Email              string
	AvatarURL          string
}

type adminVideoGenerationListItem struct {
	ID                 uint                        `json:"id"`
	GenerationRecordID uint                        `json:"generation_record_id"`
	UserID             uint                        `json:"user_id"`
	WorkID             *uint                       `json:"work_id"`
	User               adminGenerationUserSnapshot `json:"user"`
	Source             string                      `json:"source"`
	PromptSummary      string                      `json:"prompt_summary"`
	AspectRatio        string                      `json:"aspect_ratio"`
	StylePreset        string                      `json:"style_preset"`
	DurationSeconds    int                         `json:"duration_seconds"`
	ModelConfigID      uint                        `json:"model_config_id"`
	ModelName          string                      `json:"model_name"`
	RuntimeModel       string                      `json:"runtime_model"`
	Provider           string                      `json:"provider"`
	ProviderRequestID  string                      `json:"provider_request_id"`
	Status             string                      `json:"status"`
	Stage              string                      `json:"stage"`
	ErrorCode          string                      `json:"error_code"`
	LatencyMS          int64                       `json:"latency_ms"`
	CreditsCost        int                         `json:"credits_cost"`
	PreviewURL         string                      `json:"preview_url"`
	DownloadURL        string                      `json:"download_url"`
	MIMEType           string                      `json:"mime_type"`
	CreatedAt          time.Time                   `json:"created_at"`
}

type adminVideoGenerationDetailPayload struct {
	ID                  uint                                      `json:"id"`
	GenerationRecordID  uint                                      `json:"generation_record_id"`
	TaskID              string                                    `json:"task_id"`
	UserID              uint                                      `json:"user_id"`
	WorkID              *uint                                     `json:"work_id"`
	User                adminGenerationUserSnapshot               `json:"user"`
	Source              string                                    `json:"source"`
	CreatedAt           time.Time                                 `json:"created_at"`
	Status              string                                    `json:"status"`
	Stage               string                                    `json:"stage"`
	ModelConfigID       uint                                      `json:"model_config_id"`
	ModelName           string                                    `json:"model_name"`
	RuntimeModel        string                                    `json:"runtime_model"`
	Provider            string                                    `json:"provider"`
	ProviderRequestID   string                                    `json:"provider_request_id"`
	LatencyMS           int64                                     `json:"latency_ms"`
	CreditsCost         int                                       `json:"credits_cost"`
	Prompt              string                                    `json:"prompt"`
	AspectRatio         string                                    `json:"aspect_ratio"`
	StylePreset         string                                    `json:"style_preset"`
	DurationSeconds     int                                       `json:"duration_seconds"`
	InputImageCount     int                                       `json:"input_image_count"`
	ReferenceAssetCount int                                       `json:"reference_asset_count"`
	ResultVideo         adminGenerationImagePayload               `json:"result_video"`
	ReferenceImages     []adminGenerationReferenceImagePayload    `json:"reference_images"`
	NovelVideo          *adminVideoGenerationNovelPayload         `json:"novel_video,omitempty"`
	Error               *adminGenerationErrorPayload              `json:"error"`
	ProviderDiagnostics adminGenerationProviderDiagnosticsPayload `json:"provider_diagnostics"`
	Events              []adminGenerationEventPayload             `json:"events"`
}

type adminVideoGenerationNovelPayload struct {
	ProjectID uint   `json:"project_id"`
	EpisodeID uint   `json:"episode_id"`
	ShotID    uint   `json:"shot_id"`
	AttemptID uint   `json:"attempt_id"`
	Title     string `json:"title"`
	ShotTitle string `json:"shot_title"`
}

func (a *App) handleListVideoGenerations(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	filters, err := adminVideoGenerationFiltersFromQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}

	var total int64
	if err := a.adminVideoGenerationsQuery(filters).Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_generations_load_failed", "视频记录读取失败")
		return
	}

	var rows []adminVideoGenerationListRow
	if err := a.adminVideoGenerationsQuery(filters).
		Select("video_generation_records.id, video_generation_records.generation_record_id, video_generation_records.user_id, video_generation_records.work_id, video_generation_records.source, video_generation_records.prompt, video_generation_records.aspect_ratio, video_generation_records.style_preset, video_generation_records.duration_seconds, video_generation_records.model_config_id, video_generation_records.model_name, video_generation_records.runtime_model, video_generation_records.provider, video_generation_records.provider_request_id, video_generation_records.status, video_generation_records.stage, video_generation_records.error_code, video_generation_records.error_message, video_generation_records.latency_ms, video_generation_records.credits_cost, video_generation_records.credits_deducted, video_generation_records.preview_url, video_generation_records.download_url, video_generation_records.mime_type, video_generation_records.created_at, users.username, users.display_name, users.email, users.avatar_url").
		Order("video_generation_records.created_at desc, video_generation_records.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_generations_load_failed", "视频记录读取失败")
		return
	}
	summary, err := a.adminVideoGenerationSummary(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "video_generations_load_failed", "视频记录读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     adminVideoGenerationItemsFromRows(rows),
		"summary":   summary,
		"filters":   filters,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

func (a *App) handleGetAdminVideoGeneration(c *gin.Context) {
	var record VideoGenerationRecord
	if err := a.db.First(&record, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "video_generation_not_found", "视频记录不存在")
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
	referenceImages, err := a.adminGenerationReferenceImages(record.GenerationRecordID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "video_generation_reference_images_load_failed", "参考图片读取失败")
		return
	}
	var eventLogs []GenerationEventLog
	if err := a.db.Where("generation_record_id = ?", record.GenerationRecordID).Order("created_at asc, id asc").Find(&eventLogs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_generation_events_load_failed", "生成调用日志读取失败")
		return
	}
	var errorPayload *adminGenerationErrorPayload
	if record.Status == GenerationStatusFailed || strings.TrimSpace(record.ErrorCode) != "" || strings.TrimSpace(record.ErrorMessage) != "" {
		errorPayload = &adminGenerationErrorPayload{Code: record.ErrorCode, Message: record.ErrorMessage}
	}
	writeJSON(c, http.StatusOK, adminVideoGenerationDetailPayload{
		ID:                  record.ID,
		GenerationRecordID:  record.GenerationRecordID,
		TaskID:              fmt.Sprintf("VID-%d", record.ID),
		UserID:              record.UserID,
		WorkID:              record.WorkID,
		User:                user,
		Source:              record.Source,
		CreatedAt:           record.CreatedAt,
		Status:              record.Status,
		Stage:               record.Stage,
		ModelConfigID:       record.ModelConfigID,
		ModelName:           record.ModelName,
		RuntimeModel:        record.RuntimeModel,
		Provider:            record.Provider,
		ProviderRequestID:   record.ProviderRequestID,
		LatencyMS:           record.LatencyMS,
		CreditsCost:         generationCreditsCost(record.CreditsCost, record.CreditsDeducted),
		Prompt:              record.Prompt,
		AspectRatio:         record.AspectRatio,
		StylePreset:         record.StylePreset,
		DurationSeconds:     record.DurationSeconds,
		InputImageCount:     record.InputImageCount,
		ReferenceAssetCount: record.ReferenceAssetCount,
		ResultVideo: adminGenerationImagePayload{
			WorkID:      record.WorkID,
			PreviewURL:  record.PreviewURL,
			DownloadURL: record.DownloadURL,
			MIMEType:    record.MIMEType,
		},
		ReferenceImages: referenceImages,
		NovelVideo:      a.adminVideoGenerationNovelPayload(record),
		Error:           errorPayload,
		ProviderDiagnostics: adminGenerationProviderDiagnosticsPayload{
			HTTPStatus:   record.ProviderHTTPStatus,
			ErrorCode:    record.ProviderErrorCode,
			ErrorMessage: record.ProviderErrorMessage,
			FailureStage: record.ProviderFailureStage,
		},
		Events: adminGenerationEventPayloads(eventLogs),
	})
}

func (a *App) handleExportVideoGenerations(c *gin.Context) {
	filters, err := adminVideoGenerationFiltersFromQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	var rows []adminVideoGenerationListRow
	if err := a.adminVideoGenerationsQuery(filters).
		Select("video_generation_records.id, video_generation_records.generation_record_id, video_generation_records.user_id, video_generation_records.work_id, video_generation_records.source, video_generation_records.prompt, video_generation_records.aspect_ratio, video_generation_records.duration_seconds, video_generation_records.model_config_id, video_generation_records.model_name, video_generation_records.runtime_model, video_generation_records.provider, video_generation_records.provider_request_id, video_generation_records.status, video_generation_records.stage, video_generation_records.error_code, video_generation_records.error_message, video_generation_records.latency_ms, video_generation_records.credits_cost, video_generation_records.credits_deducted, video_generation_records.preview_url, video_generation_records.download_url, video_generation_records.mime_type, video_generation_records.created_at, users.username, users.display_name, users.email, users.avatar_url").
		Order("video_generation_records.created_at desc, video_generation_records.id desc").
		Scan(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_generations_export_failed", "视频记录导出失败")
		return
	}
	csvRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		csvRows = append(csvRows, []string{
			strconv.FormatUint(uint64(row.ID), 10),
			strconv.FormatUint(uint64(row.GenerationRecordID), 10),
			strconv.FormatUint(uint64(row.UserID), 10),
			fallbackString(row.DisplayName, row.Username),
			row.Source,
			row.Prompt,
			fallbackString(row.ModelName, row.RuntimeModel),
			row.Provider,
			row.ProviderRequestID,
			strconv.Itoa(row.DurationSeconds),
			row.AspectRatio,
			row.Status,
			strconv.FormatInt(row.LatencyMS, 10),
			strconv.Itoa(generationCreditsCost(row.CreditsCost, row.CreditsDeducted)),
			row.CreatedAt.Format(time.RFC3339),
			row.PreviewURL,
			row.DownloadURL,
			row.ErrorCode,
			row.ErrorMessage,
		})
	}
	writeCSV(c, "video-generations.csv", []string{"视频记录ID", "生成记录ID", "用户ID", "用户", "来源", "提示词", "模型", "Provider", "Provider Task ID", "时长", "比例", "状态", "耗时(ms)", "点数", "创建时间", "预览链接", "下载链接", "错误码", "错误信息"}, csvRows)
}

func adminVideoGenerationFiltersFromQuery(c *gin.Context) (adminVideoGenerationFilters, error) {
	filters := adminVideoGenerationFilters{
		Query:        strings.TrimSpace(c.Query("q")),
		UserKeyword:  strings.TrimSpace(c.Query("user_keyword")),
		Status:       strings.TrimSpace(c.Query("status")),
		Source:       strings.TrimSpace(c.Query("source")),
		Provider:     strings.TrimSpace(c.Query("provider")),
		RuntimeModel: strings.TrimSpace(c.Query("runtime_model")),
		DateFrom:     strings.TrimSpace(c.Query("date_from")),
		DateTo:       strings.TrimSpace(c.Query("date_to")),
	}
	if filters.Status == "all" {
		filters.Status = ""
	}
	if filters.Source == "all" {
		filters.Source = ""
	}
	if userID := getQueryInt(c, "user_id", 0); userID > 0 {
		filters.UserID = uint(userID)
	}
	if modelConfigID := getQueryInt(c, "model_config_id", 0); modelConfigID > 0 {
		filters.ModelConfigID = uint(modelConfigID)
	}
	if _, err := parseDateFilter(filters.DateFrom); err != nil {
		return filters, err
	}
	if _, err := parseDateFilter(filters.DateTo); err != nil {
		return filters, err
	}
	return filters, nil
}

func (a *App) adminVideoGenerationsQuery(filters adminVideoGenerationFilters) *gorm.DB {
	query := a.db.Model(&VideoGenerationRecord{}).
		Joins("LEFT JOIN users ON users.id = video_generation_records.user_id")
	if filters.Query != "" {
		like := "%" + filters.Query + "%"
		query = query.Where("video_generation_records.prompt LIKE ? OR video_generation_records.provider_request_id LIKE ? OR video_generation_records.runtime_model LIKE ?", like, like, like)
	}
	if filters.UserID > 0 {
		query = query.Where("video_generation_records.user_id = ?", filters.UserID)
	}
	if filters.UserKeyword != "" {
		like := "%" + strings.ToLower(filters.UserKeyword) + "%"
		query = query.Where("(LOWER(users.username) LIKE ? OR users.phone = ?)", like, filters.UserKeyword)
	}
	if filters.Status != "" {
		query = query.Where("video_generation_records.status = ?", filters.Status)
	}
	if filters.Source != "" {
		query = query.Where("video_generation_records.source = ?", filters.Source)
	}
	if filters.Provider != "" {
		query = query.Where("video_generation_records.provider = ?", filters.Provider)
	}
	if filters.RuntimeModel != "" {
		query = query.Where("video_generation_records.runtime_model = ?", filters.RuntimeModel)
	}
	if filters.ModelConfigID > 0 {
		query = query.Where("video_generation_records.model_config_id = ?", filters.ModelConfigID)
	}
	if filters.DateFrom != "" {
		from, _ := parseDateFilter(filters.DateFrom)
		query = query.Where("video_generation_records.created_at >= ?", *from)
	}
	if filters.DateTo != "" {
		to, _ := parseDateFilter(filters.DateTo)
		query = query.Where("video_generation_records.created_at < ?", to.AddDate(0, 0, 1))
	}
	return query
}

func (a *App) adminVideoGenerationSummary(now time.Time) (adminVideoGenerationSummary, error) {
	var summary adminVideoGenerationSummary
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	todayTotal, todaySucceeded, todayFailed, todayAverage, err := a.videoGenerationStatsForRange(todayStart, tomorrowStart)
	if err != nil {
		return summary, err
	}
	yesterdayTotal, yesterdaySucceeded, yesterdayFailed, yesterdayAverage, err := a.videoGenerationStatsForRange(yesterdayStart, todayStart)
	if err != nil {
		return summary, err
	}
	summary.TodayVideos = todayTotal
	summary.TodayVideosDeltaPercent = percentChange(todayTotal, yesterdayTotal)
	summary.SuccessRate = generationSuccessRate(todaySucceeded, todayTotal)
	summary.SuccessRateDeltaPercent = roundPercent(summary.SuccessRate - generationSuccessRate(yesterdaySucceeded, yesterdayTotal))
	summary.AverageLatencyMS = todayAverage
	summary.AverageLatencyDeltaPercent = percentChange(todayAverage, yesterdayAverage)
	summary.FailedTasks = todayFailed
	summary.FailedTasksDeltaPercent = percentChange(todayFailed, yesterdayFailed)
	return summary, nil
}

func (a *App) videoGenerationStatsForRange(start, end time.Time) (total, succeeded, failed, averageLatency int64, err error) {
	if err = a.db.Model(&VideoGenerationRecord{}).Where("created_at >= ? AND created_at < ?", start, end).Count(&total).Error; err != nil {
		return
	}
	if err = a.db.Model(&VideoGenerationRecord{}).Where("created_at >= ? AND created_at < ? AND status = ?", start, end, GenerationStatusSucceeded).Count(&succeeded).Error; err != nil {
		return
	}
	if err = a.db.Model(&VideoGenerationRecord{}).Where("created_at >= ? AND created_at < ? AND status = ?", start, end, GenerationStatusFailed).Count(&failed).Error; err != nil {
		return
	}
	var average float64
	if err = a.db.Model(&VideoGenerationRecord{}).
		Where("created_at >= ? AND created_at < ? AND latency_ms > 0", start, end).
		Select("COALESCE(AVG(latency_ms), 0)").
		Scan(&average).Error; err != nil {
		return
	}
	averageLatency = int64(math.Round(average))
	return
}

func adminVideoGenerationItemsFromRows(rows []adminVideoGenerationListRow) []adminVideoGenerationListItem {
	items := make([]adminVideoGenerationListItem, 0, len(rows))
	now := time.Now()
	for _, row := range rows {
		items = append(items, adminVideoGenerationListItem{
			ID:                 row.ID,
			GenerationRecordID: row.GenerationRecordID,
			UserID:             row.UserID,
			WorkID:             row.WorkID,
			User:               adminVideoGenerationUserFromRow(row),
			Source:             row.Source,
			PromptSummary:      promptSummary(row.Prompt),
			AspectRatio:        row.AspectRatio,
			StylePreset:        row.StylePreset,
			DurationSeconds:    row.DurationSeconds,
			ModelConfigID:      row.ModelConfigID,
			ModelName:          row.ModelName,
			RuntimeModel:       row.RuntimeModel,
			Provider:           row.Provider,
			ProviderRequestID:  row.ProviderRequestID,
			Status:             row.Status,
			Stage:              row.Stage,
			ErrorCode:          row.ErrorCode,
			LatencyMS:          adminVideoGenerationLatencyMS(row, now),
			CreditsCost:        generationCreditsCost(row.CreditsCost, row.CreditsDeducted),
			PreviewURL:         row.PreviewURL,
			DownloadURL:        row.DownloadURL,
			MIMEType:           row.MIMEType,
			CreatedAt:          row.CreatedAt,
		})
	}
	return items
}

func adminVideoGenerationLatencyMS(row adminVideoGenerationListRow, now time.Time) int64 {
	if row.LatencyMS > 0 {
		return row.LatencyMS
	}
	if row.Status == GenerationStatusQueued || row.Status == GenerationStatusRunning {
		return now.Sub(row.CreatedAt).Milliseconds()
	}
	return 0
}

func adminVideoGenerationUserFromRow(row adminVideoGenerationListRow) adminGenerationUserSnapshot {
	return adminGenerationUserSnapshot{
		ID:          row.UserID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Email:       row.Email,
		AvatarURL:   row.AvatarURL,
	}
}

func (a *App) adminVideoGenerationNovelPayload(record VideoGenerationRecord) *adminVideoGenerationNovelPayload {
	if record.NovelVideoProjectID == nil && record.NovelVideoShotID == nil && record.NovelVideoAttemptID == nil {
		return nil
	}
	payload := &adminVideoGenerationNovelPayload{}
	if record.NovelVideoProjectID != nil {
		payload.ProjectID = *record.NovelVideoProjectID
		var project NovelVideoProject
		if err := a.db.First(&project, *record.NovelVideoProjectID).Error; err == nil {
			payload.Title = project.Title
		}
	}
	if record.NovelVideoEpisodeID != nil {
		payload.EpisodeID = *record.NovelVideoEpisodeID
	}
	if record.NovelVideoShotID != nil {
		payload.ShotID = *record.NovelVideoShotID
		var shot NovelVideoShot
		if err := a.db.First(&shot, *record.NovelVideoShotID).Error; err == nil {
			payload.ShotTitle = shot.Title
		}
	}
	if record.NovelVideoAttemptID != nil {
		payload.AttemptID = *record.NovelVideoAttemptID
	}
	return payload
}
