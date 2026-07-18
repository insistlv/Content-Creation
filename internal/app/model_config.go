package app

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errVideoModelPublishKeyMissing = errors.New("video_model_publish_key_missing")

const zzVideoDSDisplayName = "DS 2.0"

var zzVideoDSLegacyNames = []string{"ZZ API Video DS 2.0 Fast", "Video DS 2.0"}

type adminModelSummary struct {
	OnlineModels   int64  `json:"online_models"`
	ImageModels    int64  `json:"image_models"`
	VideoModels    int64  `json:"video_models"`
	DefaultModel   string `json:"default_model"`
	DefaultModelID uint   `json:"default_model_id"`
	TotalCalls     int64  `json:"total_calls"`
}

type adminModelUsageStats struct {
	TotalCalls       int64      `json:"total_calls"`
	SucceededCalls   int64      `json:"succeeded_calls"`
	FailedCalls      int64      `json:"failed_calls"`
	TodayCalls       int64      `json:"today_calls"`
	Last7DaysCalls   int64      `json:"last_7d_calls"`
	AverageLatencyMS int64      `json:"average_latency_ms"`
	LastCalledAt     *time.Time `json:"last_called_at"`
}

type adminModelListItem struct {
	ModelConfig
	APIKeySet bool                 `json:"api_key_set"`
	Usage     adminModelUsageStats `json:"usage"`
}

type adminModelTrendPoint struct {
	Date      string `json:"date"`
	Calls     int64  `json:"calls"`
	Succeeded int64  `json:"succeeded"`
	Failed    int64  `json:"failed"`
}

type adminModelStatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type adminModelRecentGeneration struct {
	ID            uint      `json:"id"`
	UserID        uint      `json:"user_id"`
	Model         string    `json:"model"`
	Status        string    `json:"status"`
	PromptSummary string    `json:"prompt_summary"`
	LatencyMS     int64     `json:"latency_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

type adminModelRecentCallAttempt struct {
	ID                 uint      `json:"id"`
	GenerationRecordID uint      `json:"generation_record_id"`
	ModelConfigID      uint      `json:"model_config_id"`
	AttemptIndex       int       `json:"attempt_index"`
	Status             string    `json:"status"`
	LatencyMS          int64     `json:"latency_ms"`
	HTTPStatus         int       `json:"http_status"`
	ErrorCode          string    `json:"error_code"`
	ErrorMessage       string    `json:"error_message"`
	FailureStage       string    `json:"failure_stage"`
	ProviderRequestID  string    `json:"provider_request_id"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
}

type adminModelConfigWriteRequest struct {
	Name         *string `json:"name"`
	Type         *string `json:"type"`
	Provider     *string `json:"provider"`
	Status       *string `json:"status"`
	Priority     *int    `json:"priority"`
	CostLabel    *string `json:"cost_label"`
	Permission   *string `json:"permission"`
	Weight       *int    `json:"weight"`
	SortOrder    *int    `json:"sort_order"`
	RuntimeModel *string `json:"runtime_model"`
	APIBaseURL   *string `json:"api_base_url"`
	APIEndpoint  *string `json:"api_endpoint"`
	APIKey       *string `json:"api_key"`
	ClearAPIKey  *bool   `json:"clear_api_key"`
}

type adminModelVideoReadinessRequest struct {
	Status *string `json:"status"`
	Reason *string `json:"reason"`
}

type adminModelWeightRequest struct {
	ID     uint `json:"id"`
	Weight int  `json:"weight"`
}

func (a *App) seedModelConfigs() error {
	zzPermission := ModelConfigPermissionInternal
	if strings.TrimSpace(a.cfg.ZZAPIKey) != "" {
		zzPermission = ModelConfigPermissionPublic
	}
	defaults := []ModelConfig{
		{Name: zzVideoDSDisplayName, Type: ModelConfigTypeVideo, Provider: zzVideoProviderName, Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "18-24 点/秒", Permission: zzPermission, Weight: 0, SortOrder: 39, RuntimeModel: zzVideoDSFastRuntimeModel, APIBaseURL: zzVideoProviderBaseURL, APIEndpoint: zzVideoEndpoint},
		{Name: "Grok Imagine", Type: ModelConfigTypeVideo, Provider: "Wuyin", Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "3 点/秒", Permission: ModelConfigPermissionPublic, Weight: 0, SortOrder: 38, RuntimeModel: wuyinGrokImagineRuntimeModel, APIBaseURL: wuyinGrokImagineProviderBaseURL, APIEndpoint: wuyinGrokImagineSubmitEndpoint},
		{Name: "Doubao Seedance 2.0 Mini", Type: ModelConfigTypeVideo, Provider: arkVideoProviderName, Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "10-15 点/秒", Permission: ModelConfigPermissionInternal, Weight: 0, SortOrder: 39, RuntimeModel: arkSeedanceMiniRuntimeModel, APIBaseURL: arkVideoProviderBaseURL, APIEndpoint: arkVideoTasksEndpoint, VideoReadinessStatus: arkVideoReadinessStatusFailed, VideoReadinessReason: arkVideoReadinessUnsupportedReason},
		{Name: "Doubao Seedance 2.0", Type: ModelConfigTypeVideo, Provider: arkVideoProviderName, Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "30-50 点/秒", Permission: ModelConfigPermissionInternal, Weight: 0, SortOrder: 39, RuntimeModel: arkSeedance2RuntimeModel, APIBaseURL: arkVideoProviderBaseURL, APIEndpoint: arkVideoTasksEndpoint, VideoReadinessStatus: arkVideoReadinessStatusFailed, VideoReadinessReason: arkVideoReadinessUnsupportedReason},
		{Name: "Midjourney v6", Type: ModelConfigTypeImage, Provider: "Midjourney", Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "8 点/次", Permission: ModelConfigPermissionPublic, Weight: 35, SortOrder: 10},
		{Name: "SDXL 1.0", Type: ModelConfigTypeImage, Provider: "Stability AI", Status: ModelConfigStatusOnline, Priority: 2, CostLabel: "3 点/次", Permission: ModelConfigPermissionPublic, Weight: 25, SortOrder: 20},
		{Name: "DALL-E 3", Type: ModelConfigTypeImage, Provider: "OpenAI", Status: ModelConfigStatusOnline, Priority: 3, CostLabel: "5 点/次", Permission: ModelConfigPermissionPublic, Weight: 40, SortOrder: 30, RuntimeModel: "gpt-image-2", APIEndpoint: "/v1/images/generations"},
		{Name: "Sora2", Type: ModelConfigTypeVideo, Provider: "GPT-Best", Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "5 点/10s", Permission: ModelConfigPermissionPublic, Weight: 0, SortOrder: 40, RuntimeModel: "sora-2", APIEndpoint: "/v2/videos/generations"},
		{Name: "Sora2 Pro", Type: ModelConfigTypeVideo, Provider: "GPT-Best", Status: ModelConfigStatusOnline, Priority: 2, CostLabel: "8 点/10s", Permission: ModelConfigPermissionPublic, Weight: 0, SortOrder: 45, RuntimeModel: "sora-2-pro", APIEndpoint: "/v2/videos/generations"},
		{Name: "智能配乐", Type: ModelConfigTypeAudio, Provider: "GPT-Best", Status: ModelConfigStatusOnline, Priority: 1, CostLabel: "1 点/次", Permission: ModelConfigPermissionPublic, Weight: 0, SortOrder: 46, RuntimeModel: "music-for-video", APIEndpoint: "/v1/audio/soundtracks"},
		{Name: "Runway Gen-3", Type: ModelConfigTypeVideo, Provider: "Runway", Status: ModelConfigStatusOnline, Priority: 3, CostLabel: "12 点/次", Permission: ModelConfigPermissionInternal, Weight: 0, SortOrder: 50},
		{Name: "Kling", Type: ModelConfigTypeVideo, Provider: "Kling", Status: ModelConfigStatusOnline, Priority: 4, CostLabel: "10 点/次", Permission: ModelConfigPermissionInternal, Weight: 0, SortOrder: 60},
		{Name: "Pika 1.0", Type: ModelConfigTypeVideo, Provider: "Pika", Status: ModelConfigStatusOnline, Priority: 5, CostLabel: "7 点/次", Permission: ModelConfigPermissionPublic, Weight: 0, SortOrder: 70},
		{Name: "Stable Video Diffusion", Type: ModelConfigTypeVideo, Provider: "Stability AI", Status: ModelConfigStatusOffline, Priority: 6, CostLabel: "6 点/次", Permission: ModelConfigPermissionInternal, Weight: 0, SortOrder: 80},
		{Name: "FLUX.1 [dev]", Type: ModelConfigTypeImage, Provider: "Black Forest Labs", Status: ModelConfigStatusOffline, Priority: 4, CostLabel: "4 点/次", Permission: ModelConfigPermissionInternal, Weight: 0, SortOrder: 80},
	}

	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := normalizeZZVideoDS20Data(tx); err != nil {
			return err
		}
		for _, seed := range defaults {
			var existing ModelConfig
			err := tx.Unscoped().Where("name = ?", seed.Name).First(&existing).Error
			if err == nil && existing.DeletedAt.Valid {
				continue
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&seed).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}

			updates := defaultModelConfigBackfill(existing, seed)
			if len(updates) > 0 {
				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var settings AppSettings
		if err := tx.First(&settings, 1).Error; err != nil {
			return err
		}
		if err := ensureAllowedImageModelConfigs(tx, settings); err != nil {
			return err
		}
		hadRoutingConfig := settings.DefaultImageModelID != nil ||
			settings.DefaultVideoModelID != nil ||
			settings.FallbackModelID != nil ||
			settings.ModelConcurrencyLimit > 0

		dalle, err := loadPreferredSeedRoutingModel(tx, "DALL-E 3", ModelConfigTypeImage)
		if err != nil {
			return err
		}
		videoDefault, err := loadPreferredSeedRoutingModel(tx, "Grok Imagine", ModelConfigTypeVideo)
		if err != nil {
			return err
		}
		sdxl, err := loadPreferredSeedRoutingModel(tx, "SDXL 1.0", ModelConfigTypeImage)
		if err != nil {
			return err
		}

		updates := map[string]any{}
		if settings.DefaultImageModelID == nil && dalle != nil {
			updates["default_image_model_id"] = dalle.ID
		}
		if settings.DefaultVideoModelID == nil && videoDefault != nil {
			updates["default_video_model_id"] = videoDefault.ID
		}
		if settings.FallbackModelID == nil && sdxl != nil {
			updates["fallback_model_id"] = sdxl.ID
		}
		if settings.ModelConcurrencyLimit <= 0 {
			updates["model_concurrency_limit"] = 4
		}
		if strings.TrimSpace(settings.ModelRoutingStrategy) == "" {
			updates["model_routing_strategy"] = ModelRoutingStrategyDefault
		}
		if !hadRoutingConfig {
			updates["model_routing_enabled"] = true
		}
		if len(updates) > 0 {
			if err := tx.Model(&settings).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func normalizeZZVideoDS20Data(tx *gorm.DB) error {
	runtimeValues := zzVideoDSRuntimeValues()
	if err := tx.Model(&ModelConfig{}).
		Where("runtime_model IN ? AND name IN ?", runtimeValues, zzVideoDSLegacyNames).
		Update("name", zzVideoDSDisplayName).Error; err != nil {
		return err
	}

	var modelConfigIDs []uint
	if err := tx.Model(&ModelConfig{}).
		Where("runtime_model IN ?", runtimeValues).
		Pluck("id", &modelConfigIDs).Error; err != nil {
		return err
	}

	var modelCatalogIDs []uint
	if tx.Migrator().HasTable(&ModelChannel{}) {
		channelHasRuntime := tx.Migrator().HasColumn(&ModelChannel{}, "RuntimeModel")
		if channelHasRuntime && tx.Migrator().HasColumn(&ModelChannel{}, "ModelID") {
			if err := tx.Model(&ModelChannel{}).
				Where("runtime_model IN ?", runtimeValues).
				Pluck("model_id", &modelCatalogIDs).Error; err != nil {
				return err
			}
		}
		if channelHasRuntime && tx.Migrator().HasColumn(&ModelChannel{}, "Name") {
			if err := tx.Model(&ModelChannel{}).
				Where("runtime_model IN ? AND name IN ?", runtimeValues, zzVideoDSLegacyNames).
				Update("name", zzVideoDSDisplayName).Error; err != nil {
				return err
			}
		}
	}
	if len(modelCatalogIDs) > 0 && tx.Migrator().HasTable(&ModelCatalog{}) && tx.Migrator().HasColumn(&ModelCatalog{}, "Name") {
		if err := tx.Model(&ModelCatalog{}).
			Where("id IN ? AND name IN ?", modelCatalogIDs, zzVideoDSLegacyNames).
			Update("name", zzVideoDSDisplayName).Error; err != nil {
			return err
		}
	}
	if err := normalizeZZVideoDSGenerationRecordNames(tx, runtimeValues, modelConfigIDs); err != nil {
		return err
	}
	if err := normalizeZZVideoDSVideoGenerationRecordNames(tx, runtimeValues, modelConfigIDs); err != nil {
		return err
	}
	return nil
}

func normalizeZZVideoDSGenerationRecordNames(tx *gorm.DB, runtimeValues []string, modelConfigIDs []uint) error {
	if !tx.Migrator().HasTable(&GenerationRecord{}) || !tx.Migrator().HasColumn(&GenerationRecord{}, "ModelName") {
		return nil
	}
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if tx.Migrator().HasColumn(&GenerationRecord{}, "RuntimeModel") {
		conditions = append(conditions, "runtime_model IN ?")
		args = append(args, runtimeValues)
	}
	if tx.Migrator().HasColumn(&GenerationRecord{}, "Model") {
		conditions = append(conditions, "model IN ?")
		args = append(args, runtimeValues)
	}
	if len(modelConfigIDs) > 0 && tx.Migrator().HasColumn(&GenerationRecord{}, "ModelConfigID") {
		conditions = append(conditions, "model_config_id IN ?")
		args = append(args, modelConfigIDs)
	}
	if len(conditions) == 0 {
		return nil
	}
	query := tx.Model(&GenerationRecord{}).Where("model_name IN ?", zzVideoDSLegacyNames)
	query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
	return query.UpdateColumn("model_name", zzVideoDSDisplayName).Error
}

func normalizeZZVideoDSVideoGenerationRecordNames(tx *gorm.DB, runtimeValues []string, modelConfigIDs []uint) error {
	if !tx.Migrator().HasTable(&VideoGenerationRecord{}) || !tx.Migrator().HasColumn(&VideoGenerationRecord{}, "ModelName") {
		return nil
	}
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if tx.Migrator().HasColumn(&VideoGenerationRecord{}, "RuntimeModel") {
		conditions = append(conditions, "runtime_model IN ?")
		args = append(args, runtimeValues)
	}
	if len(modelConfigIDs) > 0 && tx.Migrator().HasColumn(&VideoGenerationRecord{}, "ModelConfigID") {
		conditions = append(conditions, "model_config_id IN ?")
		args = append(args, modelConfigIDs)
	}
	if len(conditions) == 0 {
		return nil
	}
	query := tx.Model(&VideoGenerationRecord{}).Where("model_name IN ?", zzVideoDSLegacyNames)
	query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
	return query.UpdateColumn("model_name", zzVideoDSDisplayName).Error
}

func zzVideoDSRuntimeValues() []string {
	return []string{zzVideoDSFastRuntimeModel, "video-ds-2.0"}
}

func loadPreferredSeedRoutingModel(tx *gorm.DB, preferredName, modelType string) (*ModelConfig, error) {
	var preferred ModelConfig
	err := tx.Where("name = ? AND type = ?", preferredName, modelType).First(&preferred).Error
	if err == nil {
		return &preferred, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var replacement ModelConfig
	err = tx.
		Where("type = ?", modelType).
		Order("CASE WHEN status = '" + ModelConfigStatusOnline + "' THEN 0 ELSE 1 END").
		Order("sort_order asc, id asc").
		First(&replacement).Error
	if err == nil {
		return &replacement, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

func defaultModelConfigBackfill(existing ModelConfig, seed ModelConfig) map[string]any {
	updates := map[string]any{}
	if strings.TrimSpace(existing.Type) == "" {
		updates["type"] = seed.Type
	}
	if strings.TrimSpace(existing.Provider) == "" {
		updates["provider"] = seed.Provider
	}
	if strings.TrimSpace(existing.Status) == "" {
		updates["status"] = seed.Status
	}
	if existing.Priority <= 0 {
		updates["priority"] = seed.Priority
	}
	if shouldBackfillModelCostLabel(existing, seed) {
		updates["cost_label"] = seed.CostLabel
	}
	if strings.TrimSpace(existing.Permission) == "" {
		updates["permission"] = seed.Permission
	}
	if existing.SortOrder == 0 {
		updates["sort_order"] = seed.SortOrder
	}
	if strings.TrimSpace(existing.RuntimeModel) == "" && strings.TrimSpace(seed.RuntimeModel) != "" {
		updates["runtime_model"] = seed.RuntimeModel
	}
	if strings.TrimSpace(existing.APIBaseURL) == "" && strings.TrimSpace(seed.APIBaseURL) != "" {
		updates["api_base_url"] = seed.APIBaseURL
	}
	if shouldBackfillImageAPIEndpoint(existing, seed) {
		updates["api_endpoint"] = seed.APIEndpoint
	}
	if isArkSeedanceVideoModel(seed.RuntimeModel, &seed) &&
		strings.TrimSpace(existing.VideoReadinessStatus) == "" &&
		strings.TrimSpace(seed.VideoReadinessStatus) != "" {
		updates["video_readiness_status"] = seed.VideoReadinessStatus
		updates["video_readiness_reason"] = seed.VideoReadinessReason
		updates["video_readiness_checked_at"] = time.Now()
	}
	return updates
}

func shouldBackfillModelCostLabel(existing ModelConfig, seed ModelConfig) bool {
	label := strings.TrimSpace(existing.CostLabel)
	if label == "" {
		return true
	}
	switch strings.TrimSpace(existing.Name) {
	case "Grok Imagine":
		return label == "5 points/sec"
	case "Doubao Seedance 2.0 Mini":
		return label == "5 points+"
	default:
		return false
	}
}

func ensureAllowedImageModelConfigs(tx *gorm.DB, settings AppSettings) error {
	for index, runtimeModel := range settings.AllowedImageModels() {
		runtimeModel = strings.TrimSpace(runtimeModel)
		if runtimeModel == "" {
			continue
		}
		var existing ModelConfig
		err := tx.
			Where("runtime_model = ? OR name = ?", runtimeModel, runtimeModel).
			Order("sort_order asc, id asc").
			First(&existing).Error
		if err == nil {
			seed := ModelConfig{Type: ModelConfigTypeImage, RuntimeModel: runtimeModel, APIEndpoint: "/v1/images/generations"}
			if shouldBackfillImageAPIEndpoint(existing, seed) {
				if err := tx.Model(&existing).Update("api_endpoint", seed.APIEndpoint).Error; err != nil {
					return err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		model := ModelConfig{
			Name:         runtimeModel,
			Type:         ModelConfigTypeImage,
			Provider:     "OpenAI",
			Status:       ModelConfigStatusOnline,
			Priority:     index + 1,
			CostLabel:    "按配置扣点",
			Permission:   ModelConfigPermissionPublic,
			Weight:       0,
			SortOrder:    90 + index*10,
			RuntimeModel: runtimeModel,
			APIEndpoint:  "/v1/images/generations",
		}
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
	}
	return nil
}

func shouldBackfillImageAPIEndpoint(existing ModelConfig, seed ModelConfig) bool {
	seedEndpoint := strings.TrimSpace(seed.APIEndpoint)
	if seedEndpoint == "" {
		return false
	}
	existingEndpoint := strings.ToLower(strings.TrimSpace(existing.APIEndpoint))
	return existingEndpoint == ""
}

func (a *App) handleListAdminModels(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 10), 1), 100)
	query := a.db.Model(&ModelConfig{})

	if modelType := strings.TrimSpace(c.Query("type")); modelType != "" {
		query = query.Where("type = ?", modelType)
	}
	if provider := strings.TrimSpace(c.Query("provider")); provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if search := strings.ToLower(strings.TrimSpace(c.Query("q"))); search != "" {
		like := "%" + search + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(provider) LIKE ? OR LOWER(runtime_model) LIKE ?)", like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "models_count_failed", "模型统计失败")
		return
	}

	var models []ModelConfig
	if err := query.Order("sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&models).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "models_load_failed", "模型读取失败")
		return
	}
	items, err := a.adminModelListItems(models)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_usage_load_failed", "模型调用统计失败")
		return
	}

	summary, err := a.adminModelSummary()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "models_summary_failed", "模型统计失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"summary":   summary,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) adminModelSummary() (adminModelSummary, error) {
	var summary adminModelSummary
	if err := a.db.Model(&ModelConfig{}).Where("status = ?", ModelConfigStatusOnline).Count(&summary.OnlineModels).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&ModelConfig{}).Where("type = ?", ModelConfigTypeImage).Count(&summary.ImageModels).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&ModelConfig{}).Where("type = ?", ModelConfigTypeVideo).Count(&summary.VideoModels).Error; err != nil {
		return summary, err
	}
	var attemptCalls int64
	if err := a.db.Model(&ModelCallAttempt{}).Count(&attemptCalls).Error; err != nil {
		return summary, err
	}
	if attemptCalls > 0 {
		summary.TotalCalls = attemptCalls
	} else if err := a.db.Model(&GenerationRecord{}).Count(&summary.TotalCalls).Error; err != nil {
		return summary, err
	}

	settings, err := a.loadSettings()
	if err != nil {
		return summary, err
	}
	if settings.DefaultImageModelID != nil {
		var model ModelConfig
		if err := a.db.First(&model, *settings.DefaultImageModelID).Error; err == nil {
			summary.DefaultModel = model.Name
			summary.DefaultModelID = model.ID
		}
	}
	if summary.DefaultModel == "" {
		summary.DefaultModel = settings.ActiveImageModel
	}
	return summary, nil
}

func (a *App) adminModelListItems(models []ModelConfig) ([]adminModelListItem, error) {
	items := make([]adminModelListItem, 0, len(models))
	for _, model := range models {
		usage, err := a.adminModelUsageStats(model)
		if err != nil {
			return nil, err
		}
		items = append(items, adminModelListItem{
			ModelConfig: model,
			APIKeySet:   a.adminModelConfigAPIKeySet(model),
			Usage:       usage,
		})
	}
	return items, nil
}

func (a *App) handleGetAdminModel(c *gin.Context) {
	var model ModelConfig
	if err := a.db.First(&model, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_not_found", "模型不存在")
		return
	}
	usage, err := a.adminModelUsageStats(model)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_usage_load_failed", "模型调用统计失败")
		return
	}
	statusBreakdown, err := a.adminModelStatusBreakdown(model)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_usage_load_failed", "模型调用统计失败")
		return
	}
	dailyTrend, err := a.adminModelDailyTrend(model, 14)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_usage_load_failed", "模型调用统计失败")
		return
	}
	recentGenerations, err := a.adminModelRecentGenerations(model, 10)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_usage_load_failed", "模型调用统计失败")
		return
	}
	recentCallAttempts, err := a.adminModelRecentCallAttempts(model, 10)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_usage_load_failed", "模型调用统计失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"model":                a.adminModelConfigResponse(model),
		"usage":                usage,
		"status_breakdown":     statusBreakdown,
		"daily_trend":          dailyTrend,
		"recent_generations":   recentGenerations,
		"recent_call_attempts": recentCallAttempts,
	})
}

func (a *App) adminModelUsageStats(model ModelConfig) (adminModelUsageStats, error) {
	var usage adminModelUsageStats
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	last7DaysStart := now.AddDate(0, 0, -7)

	if hasAttempts, err := a.adminModelHasCallAttempts(model); err != nil {
		return usage, err
	} else if hasAttempts {
		if err := a.adminModelUsageAttemptQuery(model).Count(&usage.TotalCalls).Error; err != nil {
			return usage, err
		}
		if err := a.adminModelUsageAttemptQuery(model).Where("status = ?", ModelCallAttemptStatusSucceeded).Count(&usage.SucceededCalls).Error; err != nil {
			return usage, err
		}
		if err := a.adminModelUsageAttemptQuery(model).Where("status = ?", ModelCallAttemptStatusFailed).Count(&usage.FailedCalls).Error; err != nil {
			return usage, err
		}
		if err := a.adminModelUsageAttemptQuery(model).Where("started_at >= ?", todayStart).Count(&usage.TodayCalls).Error; err != nil {
			return usage, err
		}
		if err := a.adminModelUsageAttemptQuery(model).Where("started_at >= ?", last7DaysStart).Count(&usage.Last7DaysCalls).Error; err != nil {
			return usage, err
		}
		var average float64
		if err := a.adminModelUsageAttemptQuery(model).
			Where("latency_ms > 0").
			Select("COALESCE(AVG(latency_ms), 0)").
			Scan(&average).Error; err != nil {
			return usage, err
		}
		usage.AverageLatencyMS = int64(math.Round(average))

		var lastCalledAt time.Time
		if err := a.adminModelUsageAttemptQuery(model).
			Select("started_at").
			Order("started_at desc, id desc").
			Limit(1).
			Scan(&lastCalledAt).Error; err != nil {
			return usage, err
		}
		if !lastCalledAt.IsZero() {
			usage.LastCalledAt = &lastCalledAt
		}
		return usage, nil
	}

	if err := a.adminModelUsageRecordQuery(model).Count(&usage.TotalCalls).Error; err != nil {
		return usage, err
	}
	if err := a.adminModelUsageRecordQuery(model).Where("status = ?", GenerationStatusSucceeded).Count(&usage.SucceededCalls).Error; err != nil {
		return usage, err
	}
	if err := a.adminModelUsageRecordQuery(model).Where("status = ?", GenerationStatusFailed).Count(&usage.FailedCalls).Error; err != nil {
		return usage, err
	}
	if err := a.adminModelUsageRecordQuery(model).Where("created_at >= ?", todayStart).Count(&usage.TodayCalls).Error; err != nil {
		return usage, err
	}
	if err := a.adminModelUsageRecordQuery(model).Where("created_at >= ?", last7DaysStart).Count(&usage.Last7DaysCalls).Error; err != nil {
		return usage, err
	}
	var average float64
	if err := a.adminModelUsageRecordQuery(model).
		Where("latency_ms > 0").
		Select("COALESCE(AVG(latency_ms), 0)").
		Scan(&average).Error; err != nil {
		return usage, err
	}
	usage.AverageLatencyMS = int64(math.Round(average))

	var lastCalledAt time.Time
	if err := a.adminModelUsageRecordQuery(model).
		Select("created_at").
		Order("created_at desc").
		Limit(1).
		Scan(&lastCalledAt).Error; err != nil {
		return usage, err
	}
	if !lastCalledAt.IsZero() {
		usage.LastCalledAt = &lastCalledAt
	}
	return usage, nil
}

func (a *App) adminModelStatusBreakdown(model ModelConfig) ([]adminModelStatusCount, error) {
	var rows []adminModelStatusCount
	if hasAttempts, err := a.adminModelHasCallAttempts(model); err != nil {
		return nil, err
	} else if hasAttempts {
		err := a.adminModelUsageAttemptQuery(model).
			Select("status, COUNT(*) AS count").
			Group("status").
			Order("count desc, status asc").
			Scan(&rows).Error
		if rows == nil {
			rows = []adminModelStatusCount{}
		}
		return rows, err
	}
	err := a.adminModelUsageRecordQuery(model).
		Select("status, COUNT(*) AS count").
		Group("status").
		Order("count desc, status asc").
		Scan(&rows).Error
	if rows == nil {
		rows = []adminModelStatusCount{}
	}
	return rows, err
}

func (a *App) adminModelDailyTrend(model ModelConfig, days int) ([]adminModelTrendPoint, error) {
	if days <= 0 {
		days = 14
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := todayStart.AddDate(0, 0, -(days - 1))

	points := make([]adminModelTrendPoint, 0, days)
	indexByDate := map[string]int{}
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		indexByDate[date] = i
		points = append(points, adminModelTrendPoint{Date: date})
	}
	if hasAttempts, err := a.adminModelHasCallAttempts(model); err != nil {
		return nil, err
	} else if hasAttempts {
		var attempts []ModelCallAttempt
		if err := a.adminModelUsageAttemptQuery(model).Where("started_at >= ?", start).Find(&attempts).Error; err != nil {
			return nil, err
		}
		for _, attempt := range attempts {
			date := attempt.StartedAt.Format("2006-01-02")
			index, ok := indexByDate[date]
			if !ok {
				continue
			}
			points[index].Calls++
			if attempt.Status == ModelCallAttemptStatusSucceeded {
				points[index].Succeeded++
			}
			if attempt.Status == ModelCallAttemptStatusFailed {
				points[index].Failed++
			}
		}
		return points, nil
	}
	var records []GenerationRecord
	if err := a.adminModelUsageRecordQuery(model).Where("created_at >= ?", start).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		date := record.CreatedAt.Format("2006-01-02")
		index, ok := indexByDate[date]
		if !ok {
			continue
		}
		points[index].Calls++
		if record.Status == GenerationStatusSucceeded {
			points[index].Succeeded++
		}
		if record.Status == GenerationStatusFailed {
			points[index].Failed++
		}
	}
	return points, nil
}

func (a *App) adminModelRecentGenerations(model ModelConfig, limit int) ([]adminModelRecentGeneration, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var records []GenerationRecord
	if err := a.adminModelUsageRecordQuery(model).Order("created_at desc, id desc").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]adminModelRecentGeneration, 0, len(records))
	for _, record := range records {
		items = append(items, adminModelRecentGeneration{
			ID:            record.ID,
			UserID:        record.UserID,
			Model:         record.Model,
			Status:        record.Status,
			PromptSummary: promptSummary(record.Prompt),
			LatencyMS:     record.LatencyMS,
			CreatedAt:     record.CreatedAt,
		})
	}
	return items, nil
}

func (a *App) adminModelRecentCallAttempts(model ModelConfig, limit int) ([]adminModelRecentCallAttempt, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var attempts []ModelCallAttempt
	if err := a.adminModelUsageAttemptQuery(model).
		Order("started_at desc, id desc").
		Limit(limit).
		Find(&attempts).Error; err != nil {
		return nil, err
	}
	items := make([]adminModelRecentCallAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		items = append(items, adminModelRecentCallAttempt{
			ID:                 attempt.ID,
			GenerationRecordID: attempt.GenerationRecordID,
			ModelConfigID:      attempt.ModelConfigID,
			AttemptIndex:       attempt.AttemptIndex,
			Status:             attempt.Status,
			LatencyMS:          attempt.LatencyMS,
			HTTPStatus:         attempt.HTTPStatus,
			ErrorCode:          attempt.ErrorCode,
			ErrorMessage:       attempt.ErrorMessage,
			FailureStage:       attempt.FailureStage,
			ProviderRequestID:  attempt.ProviderRequestID,
			StartedAt:          attempt.StartedAt,
			FinishedAt:         attempt.FinishedAt,
		})
	}
	return items, nil
}

func (a *App) adminModelHasCallAttempts(model ModelConfig) (bool, error) {
	var count int64
	if err := a.adminModelUsageAttemptQuery(model).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (a *App) adminModelUsageAttemptQuery(model ModelConfig) *gorm.DB {
	return a.db.Model(&ModelCallAttempt{}).Where("model_config_id = ?", model.ID)
}

func (a *App) adminModelUsageRecordQuery(model ModelConfig) *gorm.DB {
	return a.db.Model(&GenerationRecord{}).Where("model_config_id = ?", model.ID)
}

func (a *App) handleCreateAdminModel(c *gin.Context) {
	var req adminModelConfigWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := a.ensureModelConfigAPIColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_schema_migration_failed", "模型配置表升级失败，请稍后重试")
		return
	}
	model := ModelConfig{
		Status:     ModelConfigStatusOnline,
		Permission: ModelConfigPermissionPublic,
		Priority:   1,
	}
	if err := a.applyModelConfigRequest(&model, req); err != nil {
		writeAdminModelConfigValidationError(c, err)
		return
	}
	apiKey := model.APIKey
	if a.secretStore != nil {
		model.APIKey = ""
	}
	if err := a.db.Create(&model).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_create_failed", "模型创建失败")
		return
	}
	if err := a.saveModelConfigAPIKey(model.ID, apiKey, req.ClearAPIKey != nil && *req.ClearAPIKey, "admin-api"); err != nil {
		writeError(c, http.StatusInternalServerError, "model_secret_save_failed", "模型密钥保存失败")
		return
	}
	model.APIKey = apiKey
	a.writeAdminAudit(c, "model.create", "model_config", model.ID, gin.H{"name": model.Name})
	writeJSON(c, http.StatusCreated, a.adminModelConfigResponse(model))
}

func (a *App) handleUpdateAdminModel(c *gin.Context) {
	var model ModelConfig
	if err := a.db.First(&model, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_not_found", "模型不存在")
		return
	}
	if err := a.hydrateModelConfig(&model); err != nil {
		writeError(c, http.StatusInternalServerError, "model_secret_load_failed", "模型密钥读取失败")
		return
	}
	var req adminModelConfigWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := a.applyModelConfigRequest(&model, req); err != nil {
		writeAdminModelConfigValidationError(c, err)
		return
	}
	if err := a.ensureModelConfigAPIColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_schema_migration_failed", "模型配置表升级失败，请稍后重试")
		return
	}
	apiKey := model.APIKey
	toSave := model
	if a.secretStore != nil {
		toSave.APIKey = ""
	}
	if err := a.db.Save(&toSave).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_save_failed", "模型保存失败")
		return
	}
	if err := a.saveModelConfigAPIKey(model.ID, apiKey, req.ClearAPIKey != nil && *req.ClearAPIKey, "admin-api"); err != nil {
		writeError(c, http.StatusInternalServerError, "model_secret_save_failed", "模型密钥保存失败")
		return
	}
	a.writeAdminAudit(c, "model.update", "model_config", model.ID, gin.H{"name": model.Name, "status": model.Status})
	writeJSON(c, http.StatusOK, a.adminModelConfigResponse(model))
}

func (a *App) handlePatchAdminModelVideoReadiness(c *gin.Context) {
	var model ModelConfig
	if err := a.db.First(&model, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_not_found", "妯″瀷涓嶅瓨鍦?")
		return
	}
	if model.Type != ModelConfigTypeVideo || !isArkSeedanceVideoModel(model.RuntimeModel, &model) {
		writeError(c, http.StatusUnprocessableEntity, "video_readiness_not_supported", "video readiness is only supported for Ark video models")
		return
	}
	var req adminModelVideoReadinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "璇锋眰鏍煎紡閿欒")
		return
	}
	if req.Status == nil {
		writeError(c, http.StatusBadRequest, "video_readiness_status_required", "video readiness status is required")
		return
	}
	status := strings.ToLower(strings.TrimSpace(*req.Status))
	switch status {
	case "", "unchecked", "pending":
		status = ""
	case arkVideoReadinessStatusPassed, arkVideoReadinessStatusFailed:
	default:
		writeError(c, http.StatusBadRequest, "invalid_video_readiness_status", "invalid video readiness status")
		return
	}

	reason := ""
	if req.Reason != nil {
		reason = strings.TrimSpace(*req.Reason)
	}
	if status == arkVideoReadinessStatusPassed {
		reason = ""
	} else if status == arkVideoReadinessStatusFailed && reason == "" {
		reason = arkVideoReadinessUnsupportedReason
	}
	now := time.Now()
	model.VideoReadinessStatus = status
	model.VideoReadinessReason = reason
	model.VideoReadinessCheckedAt = &now
	if err := a.ensureModelConfigAPIColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_schema_migration_failed", "妯″瀷閰嶇疆琛ㄥ崌绾уけ璐ワ紝璇风◢鍚庨噸璇?")
		return
	}
	if err := a.db.Save(&model).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_save_failed", "妯″瀷淇濆瓨澶辫触")
		return
	}
	a.writeAdminAudit(c, "model.video_readiness.update", "model_config", model.ID, gin.H{"status": model.VideoReadinessStatus})
	writeJSON(c, http.StatusOK, gin.H{"model": a.adminModelConfigResponse(model)})
}

func writeAdminModelConfigValidationError(c *gin.Context, err error) {
	if errors.Is(err, errVideoModelPublishKeyMissing) {
		writeError(c, http.StatusUnprocessableEntity, "video_model_publish_key_missing", "ARK_API_KEY, ZZ_API_KEY or model API key is required before publishing this video model")
		return
	}
	writeError(c, http.StatusBadRequest, "invalid_model_config", "模型配置无效")
}

func (a *App) handleDeleteAdminModel(c *gin.Context) {
	var model ModelConfig
	if err := a.db.First(&model, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_not_found", "模型不存在")
		return
	}
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	force := strings.EqualFold(strings.TrimSpace(c.Query("force")), "true")
	if modelConfigInUseByRouting(model, settings) && !force {
		writeError(c, http.StatusUnprocessableEntity, "model_in_use", "模型正在作为默认或回退模型使用，不能删除")
		return
	}

	replacementChanges, ok := a.modelDeleteReplacementPlan(c, model, settings, force)
	if !ok {
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if replacementChanges.ImageReplacement != nil {
			imageID := replacementChanges.ImageReplacement.ID
			if replacementChanges.ReplaceDefaultImage {
				settings.DefaultImageModelID = &imageID
			}
			if replacementChanges.ReplaceFallback {
				settings.FallbackModelID = &imageID
			}
			if replacementChanges.ReplaceActiveImage {
				settings.ActiveImageModel = modelConfigRuntimeKey(*replacementChanges.ImageReplacement)
			}
		}
		if replacementChanges.VideoReplacement != nil {
			videoID := replacementChanges.VideoReplacement.ID
			if replacementChanges.ReplaceDefaultVideo {
				settings.DefaultVideoModelID = &videoID
			}
		}
		if model.Type == ModelConfigTypeImage {
			changed, err := removeAllowedImageModelForDelete(tx, &settings, model)
			if err != nil {
				return err
			}
			if changed {
				replacementChanges.SettingsChanged = true
			}
		}
		if replacementChanges.ImageReplacement != nil {
			if err := ensureAllowedImageModel(&settings, modelConfigRuntimeKey(*replacementChanges.ImageReplacement)); err != nil {
				return err
			}
			replacementChanges.SettingsChanged = true
		}
		if replacementChanges.SettingsChanged {
			if err := tx.Save(&settings).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "model_delete_failed", "模型删除失败")
		return
	}

	auditPayload := gin.H{"name": model.Name, "type": model.Type, "forced": force}
	if replacementChanges.ImageReplacement != nil {
		auditPayload["replacement_image_model_id"] = replacementChanges.ImageReplacement.ID
	}
	if replacementChanges.VideoReplacement != nil {
		auditPayload["replacement_video_model_id"] = replacementChanges.VideoReplacement.ID
	}
	a.writeAdminAudit(c, "model.delete", "model_config", model.ID, auditPayload)
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

type modelDeleteReplacementChanges struct {
	ImageReplacement    *ModelConfig
	VideoReplacement    *ModelConfig
	ReplaceDefaultImage bool
	ReplaceDefaultVideo bool
	ReplaceFallback     bool
	ReplaceActiveImage  bool
	SettingsChanged     bool
}

func (a *App) modelDeleteReplacementPlan(c *gin.Context, model ModelConfig, settings AppSettings, force bool) (modelDeleteReplacementChanges, bool) {
	changes := modelDeleteReplacementChanges{
		ReplaceDefaultImage: settings.DefaultImageModelID != nil && *settings.DefaultImageModelID == model.ID,
		ReplaceDefaultVideo: settings.DefaultVideoModelID != nil && *settings.DefaultVideoModelID == model.ID,
		ReplaceFallback:     settings.FallbackModelID != nil && *settings.FallbackModelID == model.ID,
		ReplaceActiveImage:  settings.DefaultImageModelID != nil && *settings.DefaultImageModelID == model.ID,
	}
	if !force {
		return changes, true
	}

	needsImageReplacement := model.Type == ModelConfigTypeImage && (changes.ReplaceDefaultImage || changes.ReplaceFallback || changes.ReplaceActiveImage)
	if needsImageReplacement {
		replacement, ok := a.loadReplacementModelForDelete(c, model, ModelConfigTypeImage)
		if !ok {
			return changes, false
		}
		changes.ImageReplacement = &replacement
		if changes.ReplaceDefaultImage || changes.ReplaceActiveImage {
			changes.ReplaceActiveImage = true
		}
		changes.SettingsChanged = true
	}
	if model.Type == ModelConfigTypeVideo && changes.ReplaceDefaultVideo {
		replacement, ok := a.loadReplacementModelForDelete(c, model, ModelConfigTypeVideo)
		if !ok {
			return changes, false
		}
		changes.VideoReplacement = &replacement
		changes.SettingsChanged = true
	}
	return changes, true
}

func (a *App) loadReplacementModelForDelete(c *gin.Context, model ModelConfig, modelType string) (ModelConfig, bool) {
	var replacement ModelConfig
	err := a.db.
		Where("type = ? AND id <> ?", modelType, model.ID).
		Order("CASE WHEN status = '" + ModelConfigStatusOnline + "' THEN 0 ELSE 1 END").
		Order("sort_order asc, id asc").
		First(&replacement).Error
	if err == nil {
		return replacement, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusUnprocessableEntity, "model_replacement_required", "没有可替换的同类型模型，请先新增模型后再删除")
		return replacement, false
	}
	writeError(c, http.StatusInternalServerError, "model_replacement_load_failed", "替代模型读取失败")
	return replacement, false
}

func modelConfigInUseByRouting(model ModelConfig, settings AppSettings) bool {
	if settings.DefaultImageModelID != nil && *settings.DefaultImageModelID == model.ID {
		return true
	}
	if settings.DefaultVideoModelID != nil && *settings.DefaultVideoModelID == model.ID {
		return true
	}
	if settings.FallbackModelID != nil && *settings.FallbackModelID == model.ID {
		return true
	}
	return false
}

func modelConfigRuntimeKey(model ModelConfig) string {
	if runtimeModel := strings.TrimSpace(model.RuntimeModel); runtimeModel != "" {
		return runtimeModel
	}
	return strings.TrimSpace(model.Name)
}

func removeAllowedImageModel(settings *AppSettings, values ...string) bool {
	remove := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			remove[value] = true
		}
	}
	if len(remove) == 0 {
		return false
	}
	current := settings.AllowedImageModels()
	next := make([]string, 0, len(current))
	changed := false
	for _, model := range current {
		if remove[strings.TrimSpace(model)] {
			changed = true
			continue
		}
		next = append(next, model)
	}
	if !changed {
		return false
	}
	_ = settings.SetAllowedImageModels(next)
	return true
}

func removeAllowedImageModelForDelete(tx *gorm.DB, settings *AppSettings, model ModelConfig) (bool, error) {
	remove := map[string]bool{}
	for _, value := range []string{model.RuntimeModel, model.Name} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		var remaining int64
		if err := tx.Model(&ModelConfig{}).
			Where("id <> ? AND type = ? AND (runtime_model = ? OR name = ?)", model.ID, ModelConfigTypeImage, value, value).
			Count(&remaining).Error; err != nil {
			return false, err
		}
		if remaining == 0 {
			remove[value] = true
		}
	}
	if len(remove) == 0 {
		return false, nil
	}
	current := settings.AllowedImageModels()
	next := make([]string, 0, len(current))
	changed := false
	for _, model := range current {
		if remove[strings.TrimSpace(model)] {
			changed = true
			continue
		}
		next = append(next, model)
	}
	if !changed {
		return false, nil
	}
	return true, settings.SetAllowedImageModels(next)
}

func (a *App) adminModelConfigAPIKeySet(model ModelConfig) bool {
	if model.Type == ModelConfigTypeVideo && isZZVideoModel(model.RuntimeModel, &model) {
		return a.videoModelAPIKeySet(model)
	}
	return a.modelConfigAPIKeyConfigured(model)
}

func (a *App) adminModelConfigResponse(model ModelConfig) gin.H {
	return gin.H{
		"id":                         model.ID,
		"name":                       model.Name,
		"type":                       model.Type,
		"provider":                   model.Provider,
		"status":                     model.Status,
		"priority":                   model.Priority,
		"cost_label":                 model.CostLabel,
		"permission":                 model.Permission,
		"weight":                     model.Weight,
		"sort_order":                 model.SortOrder,
		"runtime_model":              model.RuntimeModel,
		"api_base_url":               model.APIBaseURL,
		"api_endpoint":               model.APIEndpoint,
		"api_key_set":                a.adminModelConfigAPIKeySet(model),
		"video_readiness_status":     model.VideoReadinessStatus,
		"video_readiness_reason":     model.VideoReadinessReason,
		"video_readiness_checked_at": model.VideoReadinessCheckedAt,
		"created_at":                 model.CreatedAt,
		"updated_at":                 model.UpdatedAt,
	}
}

func (a *App) applyModelConfigRequest(model *ModelConfig, req adminModelConfigWriteRequest) error {
	if req.Name != nil {
		model.Name = strings.TrimSpace(*req.Name)
	}
	if req.Type != nil {
		model.Type = strings.TrimSpace(*req.Type)
	}
	if req.Provider != nil {
		model.Provider = strings.TrimSpace(*req.Provider)
	}
	if req.Status != nil {
		model.Status = strings.TrimSpace(*req.Status)
	}
	if req.Priority != nil {
		model.Priority = *req.Priority
	}
	if req.CostLabel != nil {
		model.CostLabel = strings.TrimSpace(*req.CostLabel)
	}
	if req.Permission != nil {
		model.Permission = strings.TrimSpace(*req.Permission)
	}
	if req.Weight != nil {
		model.Weight = *req.Weight
	}
	if req.SortOrder != nil {
		model.SortOrder = *req.SortOrder
	}
	if req.RuntimeModel != nil {
		model.RuntimeModel = strings.TrimSpace(*req.RuntimeModel)
	}
	if req.APIBaseURL != nil {
		model.APIBaseURL = strings.TrimRight(strings.TrimSpace(*req.APIBaseURL), "/")
	}
	if req.APIEndpoint != nil {
		model.APIEndpoint = strings.TrimSpace(*req.APIEndpoint)
	}
	if req.ClearAPIKey != nil && *req.ClearAPIKey {
		model.APIKey = ""
	}
	if req.APIKey != nil {
		nextAPIKey := strings.TrimSpace(*req.APIKey)
		if nextAPIKey != "" {
			model.APIKey = nextAPIKey
		}
	}
	if err := validateModelConfig(*model); err != nil {
		return err
	}
	return a.validateModelConfigPublishReadiness(*model)
}

func (a *App) validateModelConfigPublishReadiness(model ModelConfig) error {
	if model.Type == ModelConfigTypeVideo &&
		model.Permission == ModelConfigPermissionPublic &&
		isArkSeedanceVideoModel(model.RuntimeModel, &model) &&
		!a.videoModelAPIKeySet(model) {
		return errVideoModelPublishKeyMissing
	}
	if model.Type == ModelConfigTypeVideo &&
		model.Permission == ModelConfigPermissionPublic &&
		isZZVideoModel(model.RuntimeModel, &model) &&
		!a.videoModelAPIKeySet(model) {
		return errVideoModelPublishKeyMissing
	}
	return nil
}

func (a *App) ensureModelConfigAPIColumns() error {
	migrator := a.db.Migrator()
	for _, field := range []string{"APIBaseURL", "APIEndpoint", "APIKey", "VideoReadinessStatus", "VideoReadinessReason", "VideoReadinessCheckedAt"} {
		if migrator.HasColumn(&ModelConfig{}, field) {
			continue
		}
		if err := migrator.AddColumn(&ModelConfig{}, field); err != nil {
			return err
		}
	}
	return nil
}

func validateModelConfig(model ModelConfig) error {
	switch {
	case strings.TrimSpace(model.Name) == "":
		return errors.New("name_required")
	case strings.TrimSpace(model.Provider) == "":
		return errors.New("provider_required")
	case !validModelConfigType(model.Type):
		return errors.New("invalid_type")
	case !validModelConfigStatus(model.Status):
		return errors.New("invalid_status")
	case !validModelConfigPermission(model.Permission):
		return errors.New("invalid_permission")
	case model.Priority <= 0:
		return errors.New("invalid_priority")
	case model.Weight < 0 || model.Weight > 100:
		return errors.New("invalid_weight")
	case model.SortOrder < 0:
		return errors.New("invalid_sort_order")
	default:
		return nil
	}
}

func (a *App) handleGetModelRouting(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}

	imageModels, err := a.modelOptionsByType(ModelConfigTypeImage)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "models_load_failed", "模型读取失败")
		return
	}
	videoModels, err := a.modelOptionsByType(ModelConfigTypeVideo)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "models_load_failed", "模型读取失败")
		return
	}
	concurrencyLimit := settings.ModelConcurrencyLimit
	if concurrencyLimit <= 0 {
		concurrencyLimit = 4
	}

	writeJSON(c, http.StatusOK, gin.H{
		"default_image_model_id": uintPointerValue(settings.DefaultImageModelID),
		"default_video_model_id": uintPointerValue(settings.DefaultVideoModelID),
		"fallback_model_id":      uintPointerValue(settings.FallbackModelID),
		"routing_enabled":        settings.ModelRoutingEnabled,
		"routing_strategy":       normalizeModelRoutingStrategy(settings.ModelRoutingStrategy),
		"concurrency_limit":      concurrencyLimit,
		"image_models":           imageModels,
		"video_models":           videoModels,
		"fallback_models":        imageModels,
	})
}

func (a *App) handlePutModelRouting(c *gin.Context) {
	var req struct {
		DefaultImageModelID *uint                     `json:"default_image_model_id"`
		DefaultVideoModelID *uint                     `json:"default_video_model_id"`
		FallbackModelID     *uint                     `json:"fallback_model_id"`
		RoutingEnabled      *bool                     `json:"routing_enabled"`
		RoutingStrategy     *string                   `json:"routing_strategy"`
		ConcurrencyLimit    *int                      `json:"concurrency_limit"`
		ImageWeights        []adminModelWeightRequest `json:"image_weights"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}

	defaultImageID := chooseUint(req.DefaultImageModelID, settings.DefaultImageModelID)
	defaultVideoID := chooseUint(req.DefaultVideoModelID, settings.DefaultVideoModelID)
	fallbackID := chooseUint(req.FallbackModelID, settings.FallbackModelID)
	if defaultImageID == 0 || defaultVideoID == 0 || fallbackID == 0 {
		writeError(c, http.StatusUnprocessableEntity, "model_required", "默认模型不存在")
		return
	}

	defaultImage, ok := a.loadModelForRouting(c, defaultImageID, ModelConfigTypeImage)
	if !ok {
		return
	}
	defaultVideo, ok := a.loadModelForRouting(c, defaultVideoID, ModelConfigTypeVideo)
	if !ok {
		return
	}
	fallbackModel, ok := a.loadModelForRouting(c, fallbackID, ModelConfigTypeImage)
	if !ok {
		return
	}

	concurrencyLimit := settings.ModelConcurrencyLimit
	if req.ConcurrencyLimit != nil {
		concurrencyLimit = *req.ConcurrencyLimit
	}
	if concurrencyLimit <= 0 || concurrencyLimit > 100 {
		writeError(c, http.StatusUnprocessableEntity, "invalid_concurrency_limit", "并发限制无效")
		return
	}

	if req.ImageWeights != nil {
		if ok := a.validateImageWeights(c, req.ImageWeights); !ok {
			return
		}
	}

	routingEnabled := settings.ModelRoutingEnabled
	if req.RoutingEnabled != nil {
		routingEnabled = *req.RoutingEnabled
	}
	routingStrategy := normalizeModelRoutingStrategy(settings.ModelRoutingStrategy)
	if req.RoutingStrategy != nil {
		incomingStrategy := strings.TrimSpace(*req.RoutingStrategy)
		if incomingStrategy == "" {
			incomingStrategy = ModelRoutingStrategyDefault
		}
		if !validModelRoutingStrategy(incomingStrategy) {
			writeError(c, http.StatusUnprocessableEntity, "invalid_routing_strategy", "模型路由策略无效")
			return
		}
		routingStrategy = incomingStrategy
	}

	if err := a.db.Transaction(func(tx *gorm.DB) error {
		settings.DefaultImageModelID = &defaultImage.ID
		settings.DefaultVideoModelID = &defaultVideo.ID
		settings.FallbackModelID = &fallbackModel.ID
		settings.ModelRoutingEnabled = routingEnabled
		settings.ModelRoutingStrategy = routingStrategy
		settings.ModelConcurrencyLimit = concurrencyLimit
		if strings.TrimSpace(defaultImage.RuntimeModel) != "" {
			settings.ActiveImageModel = strings.TrimSpace(defaultImage.RuntimeModel)
			if err := ensureAllowedImageModel(&settings, settings.ActiveImageModel); err != nil {
				return err
			}
		}
		if err := tx.Save(&settings).Error; err != nil {
			return err
		}
		for _, item := range req.ImageWeights {
			if err := tx.Model(&ModelConfig{}).Where("id = ?", item.ID).Update("weight", item.Weight).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "model_routing_save_failed", "模型路由保存失败")
		return
	}

	a.writeAdminAudit(c, "model.routing.update", "settings", settings.ID, gin.H{
		"default_image_model_id": defaultImage.ID,
		"default_video_model_id": defaultVideo.ID,
		"fallback_model_id":      fallbackModel.ID,
		"routing_enabled":        routingEnabled,
		"routing_strategy":       routingStrategy,
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) modelOptionsByType(modelType string) ([]ModelConfig, error) {
	var models []ModelConfig
	err := a.db.Where("type = ?", modelType).Order("sort_order asc, id asc").Find(&models).Error
	return models, err
}

func (a *App) loadModelForRouting(c *gin.Context, id uint, expectedType string) (ModelConfig, bool) {
	var model ModelConfig
	if err := a.db.First(&model, id).Error; err != nil {
		writeError(c, http.StatusUnprocessableEntity, "model_not_found", "默认模型不存在")
		return model, false
	}
	if model.Type != expectedType {
		writeError(c, http.StatusUnprocessableEntity, "model_type_mismatch", "默认模型类型不匹配")
		return model, false
	}
	return model, true
}

func (a *App) validateImageWeights(c *gin.Context, weights []adminModelWeightRequest) bool {
	total := 0
	seen := map[uint]bool{}
	for _, item := range weights {
		if item.ID == 0 || seen[item.ID] || item.Weight < 0 || item.Weight > 100 {
			writeError(c, http.StatusUnprocessableEntity, "invalid_model_weight", "模型权重无效")
			return false
		}
		seen[item.ID] = true
		var model ModelConfig
		if err := a.db.First(&model, item.ID).Error; err != nil || model.Type != ModelConfigTypeImage {
			writeError(c, http.StatusUnprocessableEntity, "invalid_model_weight", "模型权重无效")
			return false
		}
		total += item.Weight
	}
	if total != 100 {
		writeError(c, http.StatusUnprocessableEntity, "invalid_model_weight_total", "图片模型权重总和必须为 100")
		return false
	}
	return true
}

func ensureAllowedImageModel(settings *AppSettings, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	allowed := settings.AllowedImageModels()
	if contains(allowed, model) {
		return nil
	}
	allowed = append(allowed, model)
	return settings.SetAllowedImageModels(allowed)
}

func chooseUint(incoming, current *uint) uint {
	if incoming != nil {
		return *incoming
	}
	return uintPointerValue(current)
}

func uintPointerValue(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func validModelConfigType(value string) bool {
	return value == ModelConfigTypeImage || value == ModelConfigTypeVideo || value == ModelConfigTypeAudio || value == ModelConfigTypeChat
}

func validModelConfigStatus(value string) bool {
	return value == ModelConfigStatusOnline || value == ModelConfigStatusOffline
}

func normalizeModelRoutingStrategy(value string) string {
	switch strings.TrimSpace(value) {
	case ModelRoutingStrategyRoundRobin:
		return ModelRoutingStrategyRoundRobin
	case ModelRoutingStrategySpeedFirst:
		return ModelRoutingStrategySpeedFirst
	case ModelRoutingStrategyWeighted:
		return ModelRoutingStrategyWeighted
	default:
		return ModelRoutingStrategyDefault
	}
}

func validModelRoutingStrategy(value string) bool {
	switch strings.TrimSpace(value) {
	case ModelRoutingStrategyDefault, ModelRoutingStrategyRoundRobin, ModelRoutingStrategySpeedFirst, ModelRoutingStrategyWeighted:
		return true
	default:
		return false
	}
}

func validModelConfigPermission(value string) bool {
	return value == ModelConfigPermissionPublic || value == ModelConfigPermissionInternal
}
