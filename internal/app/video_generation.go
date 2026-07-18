package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type videoGenerationRequest struct {
	Prompt                 string   `json:"prompt"`
	AspectRatio            string   `json:"aspect_ratio"`
	Duration               string   `json:"duration"`
	Model                  string   `json:"model"`
	StylePreset            string   `json:"style_preset"`
	VideoStylePresetID     uint     `json:"video_style_preset_id"`
	CustomVideoStyleID     uint     `json:"custom_video_style_id"`
	Resolution             string   `json:"resolution"`
	HD                     bool     `json:"hd"`
	Watermark              bool     `json:"watermark"`
	Private                *bool    `json:"private"`
	NotifyHook             string   `json:"notify_hook"`
	Images                 []string `json:"images"`
	ReferenceAssetIDs      []uint   `json:"reference_asset_ids"`
	ReferenceVideoAssetIDs []uint   `json:"reference_video_asset_ids"`
	ReferenceAudioAssetIDs []uint   `json:"reference_audio_asset_ids"`
	GenerateAudio          bool     `json:"generate_audio"`
	ConversationID         uint     `json:"conversation_id"`
	ReferenceMode          string   `json:"reference_mode"`
	OutputCount            int      `json:"output_count"`
}

type videoGenerationJob struct {
	User                   User
	Settings               AppSettings
	ModelConfig            *ModelConfig
	ModelCenterCandidates  []modelCenterCandidate
	Request                videoGenerationRequest
	ReferenceAssets        []ReferenceAsset
	ReferenceVideoAssets   []ReferenceAsset
	ReferenceAudioAssets   []ReferenceAsset
	VideoStylePreset       *VideoStylePreset
	CustomStyleTemplate    *UserVideoStyleTemplate
	CustomStyleAsset       *ReferenceAsset
	CreditsCost            int
	Source                 string
	NovelVideoProjectID    *uint
	NovelVideoEpisodeID    *uint
	NovelVideoShotID       *uint
	NovelVideoAttemptID    *uint
	ProviderUsageTokens    int
	ConversationID         *uint
	FallbackProviderAPIKey string
}

type videoGenerationPreparationOptions struct {
	RequireEnoughCredits bool
	EnforceRateLimit     bool
}

type userVideoGenerationFilters struct {
	Query       string
	Status      string
	Model       string
	Enhancement string
}

type userVideoGenerationHistoryItem struct {
	ID                  uint      `json:"id"`
	GenerationID        uint      `json:"generation_id"`
	WorkID              *uint     `json:"work_id"`
	Status              string    `json:"status"`
	Stage               string    `json:"stage"`
	Prompt              string    `json:"prompt"`
	PromptSummary       string    `json:"prompt_summary"`
	PreviewURL          string    `json:"preview_url"`
	DownloadURL         string    `json:"download_url"`
	AspectRatio         string    `json:"aspect_ratio"`
	DurationSeconds     int       `json:"duration_seconds"`
	ModelName           string    `json:"model_name"`
	RuntimeModel        string    `json:"runtime_model"`
	StylePreset         string    `json:"style_preset"`
	CreditsCost         int       `json:"credits_cost"`
	CreatedAt           time.Time `json:"created_at"`
	ErrorCode           string    `json:"error_code,omitempty"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	EnhancementTags     []string  `json:"enhancement_tags"`
	ReferenceAssetIDs   []uint    `json:"reference_asset_ids"`
	ReferenceAssetCount int       `json:"reference_asset_count"`
	HD                  bool      `json:"hd"`
}

type videoModelCapability struct {
	AspectRatios           []string
	Durations              []string
	ResolutionOptions      []string
	DefaultResolution      string
	PriceRules             []videoPriceRule
	SupportsHD             bool
	MaxReferenceImages     int
	RequiresReferenceImage bool
	SupportsReferenceVideo bool
	SupportsReferenceAudio bool
	MaxReferenceVideos     int
	MaxReferenceAudios     int
	SupportsGenerateAudio  bool
}

type videoPriceRule struct {
	Resolution       string `json:"resolution,omitempty"`
	CreditsPerSecond int    `json:"credits_per_second"`
	Label            string `json:"label,omitempty"`
}

type userVideoModelItem struct {
	ID                     uint             `json:"id"`
	Name                   string           `json:"name"`
	RuntimeModel           string           `json:"runtime_model"`
	Provider               string           `json:"provider"`
	Permission             string           `json:"permission"`
	Available              bool             `json:"available"`
	DisabledReason         string           `json:"disabled_reason"`
	APIKeySet              bool             `json:"api_key_set"`
	CostLabel              string           `json:"cost_label"`
	AspectRatios           []string         `json:"aspect_ratios"`
	Durations              []string         `json:"durations"`
	DefaultDuration        string           `json:"default_duration"`
	ResolutionOptions      []string         `json:"resolution_options"`
	DefaultResolution      string           `json:"default_resolution"`
	PriceRules             []videoPriceRule `json:"price_rules"`
	SupportsHD             bool             `json:"supports_hd"`
	MaxReferenceImages     int              `json:"max_reference_images"`
	RequiresReferenceImage bool             `json:"requires_reference_image"`
	SupportsReferenceVideo bool             `json:"supports_reference_video"`
	SupportsReferenceAudio bool             `json:"supports_reference_audio"`
	MaxReferenceVideos     int              `json:"max_reference_videos"`
	MaxReferenceAudios     int              `json:"max_reference_audios"`
	SupportsGenerateAudio  bool             `json:"supports_generate_audio"`
	SortOrder              int              `json:"sort_order"`
}

const (
	arkVideoReadinessStatusPassed = "passed"
	arkVideoReadinessStatusFailed = "failed"

	arkVideoReadinessPendingReason     = "等待 Ark 视频能力校验通过后开放"
	arkVideoReadinessUnsupportedReason = "当前模型不支持视频生成 API，请更换 Ark 视频模型或等待开通"
)

var errVideoProviderKeyMissing = errors.New("provider_key_missing")

const videoReferenceImageRequiredMessage = "当前模型需要至少 1 张参考图，暂不支持纯文字生成视频。请先上传参考图后再提交。"

func (a *App) handleListVideoModels(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	var models []ModelConfig
	if err := a.db.
		Where("type = ? AND status = ?", ModelConfigTypeVideo, ModelConfigStatusOnline).
		Order("sort_order asc, id asc").
		Find(&models).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_models_load_failed", "视频模型读取失败")
		return
	}
	items := make([]userVideoModelItem, 0, len(models))
	for _, model := range models {
		if !shouldExposeVideoModelInWorkspace(model) {
			continue
		}
		items = append(items, a.videoModelItem(model))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) videoModelItem(model ModelConfig) userVideoModelItem {
	capability, defaultDuration, err := a.resolvedVideoModelCapability(model.RuntimeModel, &model)
	if err != nil {
		log.Printf("video duration capability load failed model_config_id=%d error=%v", model.ID, err)
		capability = videoModelCapabilities(model.RuntimeModel, &model)
		defaultDuration = defaultVideoDurationForModel(model.RuntimeModel, &model)
	}
	available, disabledReason := a.videoModelAvailability(model)
	return userVideoModelItem{
		ID:                     model.ID,
		Name:                   model.Name,
		RuntimeModel:           fallbackString(model.RuntimeModel, model.Name),
		Provider:               model.Provider,
		Permission:             model.Permission,
		Available:              available,
		DisabledReason:         disabledReason,
		APIKeySet:              a.videoModelAPIKeySet(model),
		CostLabel:              fallbackString(videoPriceRulesCostLabel(capability.PriceRules), model.CostLabel),
		AspectRatios:           append([]string(nil), capability.AspectRatios...),
		Durations:              append([]string(nil), capability.Durations...),
		DefaultDuration:        defaultDuration,
		ResolutionOptions:      append([]string(nil), capability.ResolutionOptions...),
		DefaultResolution:      capability.DefaultResolution,
		PriceRules:             append([]videoPriceRule(nil), capability.PriceRules...),
		SupportsHD:             capability.SupportsHD,
		MaxReferenceImages:     capability.MaxReferenceImages,
		RequiresReferenceImage: capability.RequiresReferenceImage,
		SupportsReferenceVideo: capability.SupportsReferenceVideo,
		SupportsReferenceAudio: capability.SupportsReferenceAudio,
		MaxReferenceVideos:     capability.MaxReferenceVideos,
		MaxReferenceAudios:     capability.MaxReferenceAudios,
		SupportsGenerateAudio:  capability.SupportsGenerateAudio,
		SortOrder:              model.SortOrder,
	}
}

func (a *App) videoModelAPIKeySet(model ModelConfig) bool {
	_ = a.hydrateModelConfig(&model)
	if modelConfigAPIKeySet(model) {
		return true
	}
	if isArkSeedanceVideoModel(model.RuntimeModel, &model) && strings.TrimSpace(a.cfg.ArkAPIKey) != "" {
		return true
	}
	if isZZVideoModel(model.RuntimeModel, &model) && strings.TrimSpace(a.cfg.ZZAPIKey) != "" {
		return true
	}
	if !videoModelRequiresProviderKey(model.RuntimeModel, &model) {
		return false
	}
	candidates, err := a.videoModelCenterCandidatesForModelConfig(AppSettings{}, &model)
	if err != nil {
		log.Printf("video model center key status load failed model_config_id=%d error=%v", model.ID, err)
		return false
	}
	return selectedKeyedVideoModelCenterCandidate(model.RuntimeModel, &model, candidates) != nil
}

func shouldExposeVideoModelInWorkspace(model ModelConfig) bool {
	if model.Status != ModelConfigStatusOnline {
		return false
	}
	if model.Permission == ModelConfigPermissionPublic {
		return true
	}
	return isArkSeedanceVideoModel(model.RuntimeModel, &model)
}

func (a *App) videoModelAvailability(model ModelConfig) (bool, string) {
	isArkSeedance := isArkSeedanceVideoModel(model.RuntimeModel, &model)
	hasArkKey := !isArkSeedance || a.videoModelAPIKeySet(model)
	isZZVideo := isZZVideoModel(model.RuntimeModel, &model)
	hasZZKey := !isZZVideo || a.videoModelAPIKeySet(model)
	if model.Status != ModelConfigStatusOnline {
		return false, "\u6a21\u578b\u5df2\u4e0b\u7ebf"
	}
	if model.Permission != ModelConfigPermissionPublic {
		if isArkSeedance && !hasArkKey {
			return false, "\u5185\u6d4b\u4e2d\uff0c\u9700\u914d\u7f6e\u706b\u5c71\u65b9\u821f\u5bc6\u94a5\u540e\u516c\u5f00"
		}
		if isZZVideo && !hasZZKey {
			return false, "内测中，需配置 ZZ_API_KEY 或模型 API Key 后公开"
		}
		return false, "\u5185\u6d4b\u4e2d"
	}
	if isArkSeedance && !hasArkKey {
		return false, "\u5f85\u914d\u7f6e\u706b\u5c71\u65b9\u821f\u5bc6\u94a5\uff08ARK_API_KEY \u6216\u6a21\u578b API Key\uff09"
	}
	if isZZVideo && !hasZZKey {
		return false, "待配置 ZZ_API_KEY 或模型 API Key"
	}
	if isArkSeedance {
		switch strings.ToLower(strings.TrimSpace(model.VideoReadinessStatus)) {
		case arkVideoReadinessStatusPassed:
			return true, ""
		case arkVideoReadinessStatusFailed:
			return false, fallbackString(strings.TrimSpace(model.VideoReadinessReason), arkVideoReadinessUnsupportedReason)
		default:
			return false, arkVideoReadinessPendingReason
		}
	}
	return true, ""
}

func modelConfigAPIKeySet(model ModelConfig) bool {
	if isArkSeedanceVideoModel(model.RuntimeModel, &model) {
		return strings.TrimSpace(model.APIKey) != ""
	}
	if isZZVideoModel(model.RuntimeModel, &model) {
		return strings.TrimSpace(model.APIKey) != ""
	}
	return strings.TrimSpace(model.APIKey) != ""
}

func videoModelCapabilities(runtimeModel string, config *ModelConfig) videoModelCapability {
	switch {
	case isArkSeedanceVideoModel(runtimeModel, config):
		capability := videoModelCapability{
			AspectRatios:       []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"},
			Durations:          []string{"4", "5", "6", "8", "10", "12", "15", "-1"},
			SupportsHD:         true,
			MaxReferenceImages: arkVideoReferenceImageMaxCount,
		}
		if isArkSeedance2RuntimeAlias(modelConfigRuntimeValue(runtimeModel, config)) {
			capability.Durations = []string{"4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "-1"}
			capability.SupportsReferenceVideo = true
			capability.SupportsReferenceAudio = true
			capability.MaxReferenceVideos = arkVideoReferenceMediaMaxCount
			capability.MaxReferenceAudios = arkVideoReferenceMediaMaxCount
			capability.SupportsGenerateAudio = true
			applyVideoBillingProfile(&capability, videoBillingProfileSeedance20)
			return capability
		}
		if isSeedance15ProBillingModel(runtimeModel, config) {
			applyVideoBillingProfile(&capability, videoBillingProfileSeedance15Pro)
			return capability
		}
		if isSeedance20FastBillingModel(runtimeModel, config) {
			applyVideoBillingProfile(&capability, videoBillingProfileSeedance20Fast)
			return capability
		}
		applyVideoBillingProfile(&capability, videoBillingProfileSeedanceMini)
		return capability
	case isWuyinGrokImagineModel(runtimeModel, config):
		return videoModelCapability{
			AspectRatios:           []string{"16:9", "9:16"},
			Durations:              []string{"1", "3", "6", "10", "15"},
			PriceRules:             cloneVideoPriceRules(videoBillingProfileGrok.PriceRules),
			SupportsHD:             true,
			MaxReferenceImages:     4,
			RequiresReferenceImage: true,
		}
	case isZZVideoModel(runtimeModel, config):
		capability := videoModelCapability{
			AspectRatios:           []string{"16:9", "9:16", "1:1"},
			Durations:              []string{"15"},
			SupportsHD:             true,
			MaxReferenceImages:     4,
			SupportsReferenceVideo: true,
			SupportsReferenceAudio: true,
			MaxReferenceVideos:     zzVideoReferenceMaxCount,
			MaxReferenceAudios:     zzVideoReferenceMaxCount,
		}
		applyVideoBillingProfile(&capability, videoBillingProfileSeedance20Fast)
		return capability
	default:
		return videoModelCapability{
			AspectRatios:       []string{"16:9", "9:16"},
			Durations:          []string{"10", "15", "25"},
			SupportsHD:         true,
			MaxReferenceImages: 4,
		}
	}
}

type videoBillingProfile struct {
	ResolutionOptions []string
	DefaultResolution string
	PriceRules        []videoPriceRule
}

var (
	videoBillingProfileGrok = videoBillingProfile{
		PriceRules: []videoPriceRule{{CreditsPerSecond: 3, Label: "3 点/秒"}},
	}
	videoBillingProfileSeedance15Pro = videoBillingProfile{
		ResolutionOptions: []string{"480p", "720p"},
		DefaultResolution: "480p",
		PriceRules: []videoPriceRule{
			{Resolution: "480p", CreditsPerSecond: 8, Label: "8 点/秒"},
			{Resolution: "720p", CreditsPerSecond: 12, Label: "12 点/秒"},
		},
	}
	videoBillingProfileSeedanceMini = videoBillingProfile{
		ResolutionOptions: []string{"480p", "720p"},
		DefaultResolution: "480p",
		PriceRules: []videoPriceRule{
			{Resolution: "480p", CreditsPerSecond: 10, Label: "10 点/秒"},
			{Resolution: "720p", CreditsPerSecond: 15, Label: "15 点/秒"},
		},
	}
	videoBillingProfileSeedance20Fast = videoBillingProfile{
		ResolutionOptions: []string{"480p", "720p"},
		DefaultResolution: "480p",
		PriceRules: []videoPriceRule{
			{Resolution: "480p", CreditsPerSecond: 18, Label: "18 点/秒"},
			{Resolution: "720p", CreditsPerSecond: 24, Label: "24 点/秒"},
		},
	}
	videoBillingProfileSeedance20 = videoBillingProfile{
		ResolutionOptions: []string{"720p", "1080p"},
		DefaultResolution: "720p",
		PriceRules: []videoPriceRule{
			{Resolution: "720p", CreditsPerSecond: 30, Label: "30 点/秒"},
			{Resolution: "1080p", CreditsPerSecond: 50, Label: "50 点/秒"},
		},
	}
)

func applyVideoBillingProfile(capability *videoModelCapability, profile videoBillingProfile) {
	capability.ResolutionOptions = append([]string(nil), profile.ResolutionOptions...)
	capability.DefaultResolution = profile.DefaultResolution
	capability.PriceRules = cloneVideoPriceRules(profile.PriceRules)
}

func cloneVideoPriceRules(rules []videoPriceRule) []videoPriceRule {
	if len(rules) == 0 {
		return nil
	}
	return append([]videoPriceRule(nil), rules...)
}

func videoPriceRulesCostLabel(rules []videoPriceRule) string {
	if len(rules) == 0 {
		return ""
	}
	if len(rules) == 1 {
		if label := strings.TrimSpace(rules[0].Label); label != "" {
			return label
		}
		if rules[0].CreditsPerSecond > 0 {
			return strconv.Itoa(rules[0].CreditsPerSecond) + " 点/秒"
		}
		return ""
	}
	minCredits := 0
	maxCredits := 0
	for _, rule := range rules {
		if rule.CreditsPerSecond <= 0 {
			continue
		}
		if minCredits == 0 || rule.CreditsPerSecond < minCredits {
			minCredits = rule.CreditsPerSecond
		}
		if rule.CreditsPerSecond > maxCredits {
			maxCredits = rule.CreditsPerSecond
		}
	}
	if minCredits == 0 {
		return ""
	}
	if minCredits == maxCredits {
		return strconv.Itoa(minCredits) + " 点/秒"
	}
	return strconv.Itoa(minCredits) + "-" + strconv.Itoa(maxCredits) + " 点/秒"
}

func videoBillingProfileForModel(runtimeModel string, config *ModelConfig) (videoBillingProfile, bool) {
	switch {
	case isWuyinGrokImagineModel(runtimeModel, config):
		return videoBillingProfileGrok, true
	case isSeedance15ProBillingModel(runtimeModel, config):
		return videoBillingProfileSeedance15Pro, true
	case isSeedance20FastBillingModel(runtimeModel, config):
		return videoBillingProfileSeedance20Fast, true
	case isArkSeedance2RuntimeAlias(modelConfigRuntimeValue(runtimeModel, config)):
		return videoBillingProfileSeedance20, true
	case isArkSeedanceMiniRuntimeAlias(modelConfigRuntimeValue(runtimeModel, config)) || isArkSeedanceVideoModel(runtimeModel, config):
		return videoBillingProfileSeedanceMini, true
	default:
		return videoBillingProfile{}, false
	}
}

func isSeedance15ProBillingModel(runtimeModel string, config *ModelConfig) bool {
	text := videoBillingModelText(runtimeModel, config)
	return strings.Contains(text, "seedance 1.5 pro") ||
		strings.Contains(text, "seedance-1.5-pro") ||
		strings.Contains(text, "seedance-1-5-pro") ||
		strings.Contains(text, "seed-1-5-pro")
}

func isSeedance20FastBillingModel(runtimeModel string, config *ModelConfig) bool {
	if isZZVideoModel(runtimeModel, config) {
		return true
	}
	text := videoBillingModelText(runtimeModel, config)
	return strings.Contains(text, "seedance 2.0 fast") ||
		strings.Contains(text, "seedance-2.0-fast") ||
		strings.Contains(text, "seedance-2-0-fast") ||
		strings.Contains(text, "seed-2-0-fast")
}

func videoBillingModelText(runtimeModel string, config *ModelConfig) string {
	parts := []string{runtimeModel}
	if config != nil {
		parts = append(parts, config.RuntimeModel, config.Name, config.Provider, config.APIBaseURL, config.APIEndpoint)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func modelConfigRuntimeValue(runtimeModel string, config *ModelConfig) string {
	return canonicalVideoRuntimeModel(fallbackString(runtimeModel, modelConfigRuntime(config)))
}

func (a *App) handleCreateAsyncVideoGeneration(c *gin.Context) {
	job, ok := a.prepareVideoGenerationJob(c)
	if !ok {
		return
	}

	lockKey, ok := a.acquireGenerationLock(c, job.User.ID)
	if !ok {
		return
	}

	record, err := a.createVideoGenerationRecord(job)
	if err != nil {
		a.concurrencyLimiter.Release(lockKey)
		writeError(c, http.StatusInternalServerError, "record_create_failed", "记录创建失败")
		return
	}

	go a.runVideoGenerationTask(&record, job, lockKey)
	if job.ConversationID != nil {
		_ = a.db.Model(&VideoConversation{}).Where("id = ? AND user_id = ?", *job.ConversationID, job.User.ID).Updates(map[string]any{"last_generation_id": record.ID, "last_activity_at": time.Now()}).Error
	}

	balance, _ := a.lookupBalance(job.User.ID)
	writeJSON(c, http.StatusAccepted, gin.H{
		"generation_id":     record.ID,
		"status":            record.Status,
		"stage":             record.Stage,
		"progress":          5,
		"conversation_id":   job.ConversationID,
		"available_credits": balance.AvailableCredits,
	})
}

func (a *App) handleEstimateVideoGeneration(c *gin.Context) {
	job, ok := a.prepareVideoGenerationJobWithOptions(c, videoGenerationPreparationOptions{
		RequireEnoughCredits: false,
		EnforceRateLimit:     false,
	})
	if !ok {
		return
	}
	estimate, err := a.buildCreditEstimate(job.User.ID, job.CreditsCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"required_credits":    estimate.RequiredCredits,
		"available_credits":   estimate.AvailableCredits,
		"missing_credits":     estimate.MissingCredits,
		"enough":              estimate.Enough,
		"recommended_package": estimate.RecommendedPackage,
		"billing_policy":      "success_only",
		"message":             "提交前预估，生成成功后扣点，失败不扣点",
	})
}

func (a *App) handleListUserVideoGenerations(c *gin.Context) {
	user := currentUser(c)
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 8), 1), 50)
	filters := userVideoGenerationFilters{
		Query:       strings.TrimSpace(c.Query("q")),
		Status:      strings.TrimSpace(c.Query("status")),
		Model:       strings.TrimSpace(c.Query("model")),
		Enhancement: strings.TrimSpace(c.Query("enhancement")),
	}
	if filters.Status == "all" {
		filters.Status = ""
	}
	if filters.Model == "all" {
		filters.Model = ""
	}
	if filters.Enhancement == "all" {
		filters.Enhancement = ""
	}

	query := a.userVideoGenerationsQuery(user.ID, filters)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_history_load_failed", "视频历史读取失败")
		return
	}

	var records []VideoGenerationRecord
	if err := a.userVideoGenerationsQuery(user.ID, filters).
		Order("video_generation_records.created_at desc, video_generation_records.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&records).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_history_load_failed", "视频历史读取失败")
		return
	}

	items, err := a.userVideoGenerationHistoryItems(records)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "video_history_load_failed", "视频历史读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

func (a *App) userVideoGenerationsQuery(userID uint, filters userVideoGenerationFilters) *gorm.DB {
	query := a.db.Model(&VideoGenerationRecord{}).Where("video_generation_records.user_id = ?", userID)
	if filters.Query != "" {
		like := "%" + filters.Query + "%"
		query = query.Where("video_generation_records.prompt LIKE ? OR video_generation_records.style_preset LIKE ?", like, like)
	}
	if filters.Status != "" {
		query = query.Where("video_generation_records.status = ?", filters.Status)
	}
	if filters.Model != "" {
		like := "%" + strings.ToLower(filters.Model) + "%"
		query = query.Where("LOWER(video_generation_records.runtime_model) LIKE ? OR LOWER(video_generation_records.model_name) LIKE ?", like, like)
	}
	switch filters.Enhancement {
	case "高清":
		query = query.Where("video_generation_records.runtime_model LIKE ? OR video_generation_records.model_name LIKE ? OR video_generation_records.credits_cost >= ?", "%pro%", "%Pro%", 8)
	case "参考图":
		query = query.Where("video_generation_records.reference_asset_count > 0 OR video_generation_records.input_image_count > 0")
	case "风格模板":
		query = query.Where("TRIM(video_generation_records.style_preset) <> ''")
	case "Pro":
		query = query.Where("LOWER(video_generation_records.runtime_model) LIKE ? OR LOWER(video_generation_records.model_name) LIKE ?", "%pro%", "%pro%")
	case "Seedance":
		query = query.Where("LOWER(video_generation_records.runtime_model) LIKE ? OR LOWER(video_generation_records.model_name) LIKE ?", "%seedance%", "%seedance%")
	case "补帧", "超分", "精修":
		like := "%" + filters.Enhancement + "%"
		query = query.Where("video_generation_records.prompt LIKE ? OR video_generation_records.style_preset LIKE ?", like, like)
	}
	return query
}

func (a *App) userVideoGenerationHistoryItems(records []VideoGenerationRecord) ([]userVideoGenerationHistoryItem, error) {
	referenceIDsByGeneration, err := a.videoGenerationReferenceAssetIDs(records)
	if err != nil {
		return nil, err
	}
	items := make([]userVideoGenerationHistoryItem, 0, len(records))
	for _, record := range records {
		referenceIDs := referenceIDsByGeneration[record.GenerationRecordID]
		item := userVideoGenerationHistoryItem{
			ID:                  record.ID,
			GenerationID:        record.GenerationRecordID,
			WorkID:              record.WorkID,
			Status:              normalizeGenerationStatus(record.Status),
			Stage:               normalizeGenerationStage(record.Status, record.Stage),
			Prompt:              record.Prompt,
			PromptSummary:       promptSummary(record.Prompt),
			PreviewURL:          record.PreviewURL,
			DownloadURL:         record.DownloadURL,
			AspectRatio:         record.AspectRatio,
			DurationSeconds:     record.DurationSeconds,
			ModelName:           record.ModelName,
			RuntimeModel:        record.RuntimeModel,
			StylePreset:         record.StylePreset,
			CreditsCost:         generationCreditsCost(record.CreditsCost, record.CreditsDeducted),
			CreatedAt:           record.CreatedAt,
			ErrorCode:           record.ErrorCode,
			ErrorMessage:        record.ErrorMessage,
			ReferenceAssetIDs:   referenceIDs,
			ReferenceAssetCount: maxInt(record.ReferenceAssetCount, len(referenceIDs)),
			HD:                  videoGenerationHistoryIsHD(record),
		}
		item.EnhancementTags = videoGenerationEnhancementTags(record, item.ReferenceAssetCount, item.HD)
		items = append(items, item)
	}
	return items, nil
}

func (a *App) videoGenerationReferenceAssetIDs(records []VideoGenerationRecord) (map[uint][]uint, error) {
	result := map[uint][]uint{}
	if len(records) == 0 {
		return result, nil
	}
	ids := make([]uint, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.GenerationRecordID)
	}
	var links []GenerationReferenceAsset
	if err := a.db.
		Where("generation_record_id IN ?", ids).
		Order("generation_record_id asc, sort_order asc, id asc").
		Find(&links).Error; err != nil {
		return nil, err
	}
	for _, link := range links {
		result[link.GenerationRecordID] = append(result[link.GenerationRecordID], link.ReferenceAssetID)
	}
	return result, nil
}

func videoGenerationHistoryIsHD(record VideoGenerationRecord) bool {
	text := strings.ToLower(strings.Join([]string{record.RuntimeModel, record.ModelName}, " "))
	return strings.Contains(text, "pro") || generationCreditsCost(record.CreditsCost, record.CreditsDeducted) >= 8
}

func videoGenerationEnhancementTags(record VideoGenerationRecord, referenceCount int, hd bool) []string {
	tags := make([]string, 0, 5)
	add := func(label string) {
		if strings.TrimSpace(label) == "" {
			return
		}
		for _, existing := range tags {
			if existing == label {
				return
			}
		}
		tags = append(tags, label)
	}
	lowerText := strings.ToLower(strings.Join([]string{record.Prompt, record.StylePreset, record.RuntimeModel, record.ModelName}, " "))
	if strings.Contains(lowerText, "seedance") || strings.Contains(lowerText, "doubao-seed") {
		add("Seedance")
	}
	if strings.Contains(record.Prompt, "补帧") || strings.Contains(record.StylePreset, "补帧") || strings.Contains(lowerText, "frame interpolation") {
		add("补帧")
	}
	if strings.Contains(record.Prompt, "超分") || strings.Contains(record.StylePreset, "超分") || strings.Contains(lowerText, "upscale") {
		add("超分")
	}
	if strings.Contains(record.Prompt, "精修") || strings.Contains(record.StylePreset, "精修") || strings.Contains(lowerText, "refine") {
		add("精修")
	}
	if hd {
		add("高清")
	}
	if referenceCount > 0 || record.ReferenceAssetCount > 0 || record.InputImageCount > 0 {
		add("参考图")
	}
	if strings.TrimSpace(record.StylePreset) != "" {
		add("风格模板")
	}
	if strings.Contains(lowerText, "pro") {
		add("Pro")
	}
	return tags
}

func (a *App) handleGetVideoGeneration(c *gin.Context) {
	a.handleGetGeneration(c)
}

func (a *App) prepareVideoGenerationJob(c *gin.Context) (*videoGenerationJob, bool) {
	return a.prepareVideoGenerationJobWithOptions(c, videoGenerationPreparationOptions{
		RequireEnoughCredits: true,
		EnforceRateLimit:     true,
	})
}

func (a *App) prepareVideoGenerationJobWithOptions(c *gin.Context, options videoGenerationPreparationOptions) (*videoGenerationJob, bool) {
	user := currentUser(c)
	var req videoGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return nil, false
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	req.Duration = strings.TrimSpace(req.Duration)
	req.Model = strings.TrimSpace(req.Model)
	req.Resolution = strings.TrimSpace(req.Resolution)
	originalModel := req.Model
	req.Model = canonicalVideoRuntimeModel(req.Model)
	req.StylePreset = strings.TrimSpace(req.StylePreset)
	req.NotifyHook = strings.TrimSpace(req.NotifyHook)
	req.ReferenceMode = strings.TrimSpace(req.ReferenceMode)
	if req.OutputCount == 0 {
		req.OutputCount = 1
	}
	if req.OutputCount != 1 {
		writeError(c, http.StatusUnprocessableEntity, "unsupported_output_count", "当前版本每次只支持生成 1 个视频")
		return nil, false
	}
	if req.ReferenceMode == "" {
		req.ReferenceMode = "omni"
	}
	if req.ReferenceMode != "omni" {
		writeError(c, http.StatusUnprocessableEntity, "unsupported_reference_mode", "当前版本仅支持全能参考模式")
		return nil, false
	}
	if req.ConversationID > 0 {
		var conversation VideoConversation
		if err := a.db.Where("id = ? AND user_id = ?", req.ConversationID, user.ID).First(&conversation).Error; err != nil {
			writeError(c, http.StatusNotFound, "video_conversation_not_found", "视频会话不存在")
			return nil, false
		}
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}
	if req.Prompt == "" {
		writeError(c, http.StatusBadRequest, "prompt_required", "提示词不能为空")
		return nil, false
	}
	if req.AspectRatio == "" {
		writeError(c, http.StatusBadRequest, "invalid_aspect_ratio", "不支持的视频比例")
		return nil, false
	}
	if req.Private == nil {
		defaultPrivate := true
		req.Private = &defaultPrivate
	}

	videoStylePreset, customStyleTemplate, customStyleAsset, ok := a.prepareVideoStyleSelection(c, user.ID, req)
	if !ok {
		return nil, false
	}
	if customStyleTemplate != nil && len(req.ReferenceAssetIDs) > 3 {
		writeError(c, http.StatusUnprocessableEntity, "reference_asset_limit_exceeded", "选择自定义视频风格时最多选择 3 张内容参考图")
		return nil, false
	}

	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return nil, false
	}
	modelConfig, err := a.videoModelConfig(req.Model, settings)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_config_load_failed", "模型配置读取失败")
		return nil, false
	}
	if req.Model != "" && modelConfig == nil {
		writeError(c, http.StatusUnprocessableEntity, "video_model_unavailable", "当前视频模型不可用")
		return nil, false
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = fallbackString(modelConfigRuntime(modelConfig), wuyinGrokImagineRuntimeModel)
	}
	if modelConfig != nil {
		if runtimeModel := canonicalVideoRuntimeModel(modelConfigRuntime(modelConfig)); runtimeModel != "" {
			req.Model = runtimeModel
		}
	}
	if originalModel != "" && req.Model != "" && !strings.EqualFold(originalModel, req.Model) {
		log.Printf("video model runtime canonicalized original_model=%q runtime_model=%q", originalModel, req.Model)
	}
	if modelConfig != nil {
		if available, disabledReason := a.videoModelAvailability(*modelConfig); !available {
			if modelConfig.Permission == ModelConfigPermissionPublic &&
				isArkSeedanceVideoModel(req.Model, modelConfig) &&
				!modelConfigAPIKeySet(*modelConfig) {
				writeError(c, http.StatusUnprocessableEntity, "provider_key_missing", "ARK_API_KEY or model API key is required")
				return nil, false
			}
			writeError(c, http.StatusUnprocessableEntity, "video_model_unavailable", fallbackString(disabledReason, "video model is unavailable"))
			return nil, false
		}
	}
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return nil, false
	}
	capability, defaultDuration, err := a.resolvedVideoModelCapability(req.Model, modelConfig)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_load_failed", "模型中心配置读取失败")
		return nil, false
	}
	req.AspectRatio = normalizeVideoAspectRatioForCapabilities(req.AspectRatio, capability)
	req.Duration = normalizeVideoDurationForCapabilities(req.Duration, capability, defaultDuration)
	if req.AspectRatio == "" {
		writeError(c, http.StatusBadRequest, "invalid_aspect_ratio", "不支持的视频比例")
		return nil, false
	}
	if req.Duration == "" {
		writeError(c, http.StatusUnprocessableEntity, "invalid_video_duration", "当前模型不支持该视频时长")
		return nil, false
	}
	if !capability.SupportsHD {
		req.HD = false
	}
	req.Resolution = normalizeVideoResolutionForCapabilities(req, capability)
	req.HD = videoRequestIsHD(req)
	if isWuyinGrokImagineModel(req.Model, modelConfig) && req.Duration == "25" {
		writeError(c, http.StatusBadRequest, "invalid_generation_parameter", "当前视频模型不支持 25 秒")
		return nil, false
	}

	referenceAssets, ok := a.prepareReferenceAssetsWithLimitAndKind(c, user.ID, req.ReferenceAssetIDs, capability.MaxReferenceImages, referenceAssetKindImage)
	if !ok {
		return nil, false
	}

	var referenceVideoAssets []ReferenceAsset
	if len(req.ReferenceVideoAssetIDs) > 0 {
		if !capability.SupportsReferenceVideo {
			writeError(c, http.StatusUnprocessableEntity, "reference_video_unsupported", "当前视频模型不支持参考视频")
			return nil, false
		}
		referenceVideoAssets, ok = a.prepareReferenceAssetsWithLimitAndKind(c, user.ID, req.ReferenceVideoAssetIDs, capability.MaxReferenceVideos, referenceAssetKindVideo)
		if !ok {
			return nil, false
		}
	}

	var referenceAudioAssets []ReferenceAsset
	if len(req.ReferenceAudioAssetIDs) > 0 {
		if !capability.SupportsReferenceAudio {
			writeError(c, http.StatusUnprocessableEntity, "reference_audio_unsupported", "当前视频模型不支持参考音频")
			return nil, false
		}
		referenceAudioAssets, ok = a.prepareReferenceAssetsWithLimitAndKind(c, user.ID, req.ReferenceAudioAssetIDs, capability.MaxReferenceAudios, referenceAssetKindAudio)
		if !ok {
			return nil, false
		}
		req.GenerateAudio = true
	}
	if req.GenerateAudio && !capability.SupportsGenerateAudio {
		writeError(c, http.StatusUnprocessableEntity, "generate_audio_unsupported", "当前视频模型不支持生成音频")
		return nil, false
	}

	if capability.RequiresReferenceImage && !videoRequestHasReferenceImage(req, referenceAssets) {
		writeError(c, http.StatusUnprocessableEntity, "reference_image_required", videoReferenceImageRequiredMessage)
		return nil, false
	}

	modelCenterCandidates, ok := a.prepareVideoModelCenterCandidates(c, settings, modelConfig, req.Model, req.Duration)
	if !ok {
		return nil, false
	}

	cost := videoCreditCostForModel(req, modelConfig)
	estimate, err := a.buildCreditEstimate(user.ID, cost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return nil, false
	}
	if options.RequireEnoughCredits && !estimate.Enough {
		writeCreditsInsufficientError(c, estimate)
		return nil, false
	}

	if options.EnforceRateLimit {
		rateKey := clientIP(c.Request) + "|user:" + strconv.FormatUint(uint64(user.ID), 10)
		window := time.Duration(settings.RateLimitWindowSeconds) * time.Second
		if !a.rateLimiter.Allow(rateKey, time.Now(), window, settings.RateLimitMaxRequests) {
			writeError(c, http.StatusTooManyRequests, "too_many_requests", "请求过于频繁")
			return nil, false
		}
	}

	job := &videoGenerationJob{
		User:                  *user,
		Settings:              settings,
		ModelConfig:           modelConfig,
		ModelCenterCandidates: modelCenterCandidates,
		Request:               req,
		ReferenceAssets:       referenceAssets,
		ReferenceVideoAssets:  referenceVideoAssets,
		ReferenceAudioAssets:  referenceAudioAssets,
		VideoStylePreset:      videoStylePreset,
		CustomStyleTemplate:   customStyleTemplate,
		CustomStyleAsset:      customStyleAsset,
		CreditsCost:           cost,
		Source:                VideoGenerationSourceWorkspace,
	}
	if isArkSeedanceVideoModel(req.Model, modelConfig) {
		job.FallbackProviderAPIKey = a.cfg.ArkAPIKey
	}
	if isZZVideoModel(req.Model, modelConfig) {
		job.FallbackProviderAPIKey = a.cfg.ZZAPIKey
	}
	if req.ConversationID > 0 {
		job.ConversationID = &req.ConversationID
	}
	return job, true
}

func videoRequestHasReferenceImage(req videoGenerationRequest, referenceAssets []ReferenceAsset) bool {
	for _, image := range req.Images {
		if strings.TrimSpace(image) != "" {
			return true
		}
	}
	return len(referenceAssets) > 0
}

func (a *App) videoModelConfig(runtimeModel string, settings AppSettings) (*ModelConfig, error) {
	runtimeModel = canonicalVideoRuntimeModel(runtimeModel)
	var model ModelConfig
	if runtimeModel != "" {
		lookupRuntimeModels := videoRuntimeModelLookupValues(runtimeModel)
		err := a.db.Where("type = ? AND (runtime_model IN ? OR name IN ?)", ModelConfigTypeVideo, lookupRuntimeModels, lookupRuntimeModels).
			Order("runtime_model desc, sort_order asc, id asc").
			First(&model).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &model, nil
	}
	if settings.DefaultVideoModelID != nil && *settings.DefaultVideoModelID != 0 {
		err := a.db.Where("id = ? AND type = ?", *settings.DefaultVideoModelID, ModelConfigTypeVideo).First(&model).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &model, nil
	}
	return nil, nil
}

func videoRuntimeModelLookupValues(runtimeModel string) []string {
	runtimeModel = canonicalVideoRuntimeModel(runtimeModel)
	if runtimeModel == "" {
		return nil
	}
	if isZZVideoRuntimeAlias(runtimeModel) {
		return zzVideoDSRuntimeValues()
	}
	return []string{runtimeModel}
}

func (a *App) prepareVideoModelCenterCandidates(c *gin.Context, settings AppSettings, modelConfig *ModelConfig, runtimeModel, duration string) ([]modelCenterCandidate, bool) {
	candidates, err := a.resolveVideoModelCenterCandidates(settings, modelConfig, runtimeModel, duration)
	if err != nil {
		if errors.Is(err, errVideoProviderKeyMissing) {
			writeError(c, http.StatusUnprocessableEntity, "provider_key_missing", videoProviderKeyMissingMessage(runtimeModel, modelConfig))
			return nil, false
		}
		if errors.Is(err, errVideoDurationChannelUnavailable) {
			writeError(c, http.StatusUnprocessableEntity, "video_duration_channel_unavailable", "当前时长暂无可用调用渠道")
			return nil, false
		}
		writeError(c, http.StatusInternalServerError, "model_center_load_failed", "模型中心配置读取失败")
		return nil, false
	}
	return candidates, true
}

func (a *App) resolveVideoModelCenterCandidates(settings AppSettings, modelConfig *ModelConfig, runtimeModel, duration string) ([]modelCenterCandidate, error) {
	if !videoModelRequiresProviderKey(runtimeModel, modelConfig) {
		hasChannels, compatible, err := a.legacyVideoModelHasCompatibleOnlineChannel(modelConfig, duration)
		if err != nil {
			return nil, err
		}
		if hasChannels && !compatible {
			return nil, errVideoDurationChannelUnavailable
		}
		return nil, nil
	}
	candidates, err := a.videoModelCenterCandidatesForModelConfig(settings, modelConfig)
	if err != nil {
		return nil, err
	}
	if len(candidates) > 0 {
		compatible := make([]modelCenterCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if channelSupportsVideoDuration(candidate.Channel, duration) {
				compatible = append(compatible, candidate)
			}
		}
		if len(compatible) == 0 {
			return nil, errVideoDurationChannelUnavailable
		}
		candidates = compatible
	}
	if candidate := selectedKeyedVideoModelCenterCandidate(runtimeModel, modelConfig, candidates); candidate != nil {
		return []modelCenterCandidate{*candidate}, nil
	}
	if modelConfigProviderAPIKey(modelConfig) != "" {
		return nil, nil
	}
	if isZZVideoModel(runtimeModel, modelConfig) && strings.TrimSpace(a.cfg.ZZAPIKey) != "" {
		return nil, nil
	}
	return nil, errVideoProviderKeyMissing
}

func (a *App) videoModelCenterCandidatesForModelConfig(_ AppSettings, modelConfig *ModelConfig) ([]modelCenterCandidate, error) {
	if modelConfig == nil || modelConfig.ID == 0 {
		return nil, nil
	}
	legacyCandidate, ok, err := a.modelCenterCandidateForLegacyConfig(*modelConfig)
	if err != nil {
		return nil, err
	}
	if !ok || legacyCandidate.Model.ID == 0 {
		return nil, nil
	}
	direct, err := a.modelCenterChannelCandidatesForModel(legacyCandidate.Model.ID)
	if err != nil {
		return nil, err
	}
	candidates := append([]modelCenterCandidate(nil), direct...)
	candidates = append(candidates, legacyCandidate)
	candidates = prioritizeModelCenterCooldown(dedupeModelCenterCandidates(candidates), time.Now())
	return a.expandWuyinVideoCandidatesWithKeyedProvider(candidates)
}

func (a *App) expandWuyinVideoCandidatesWithKeyedProvider(candidates []modelCenterCandidate) ([]modelCenterCandidate, error) {
	expanded := make([]modelCenterCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if modelCenterProviderAPIKey(&candidate) == "" && isWuyinGrokImagineVideoInput(VideoGenerationInput{
			Model:               modelCenterRuntimeModel(&candidate),
			ProviderBaseURL:     modelCenterProviderBaseURL(&candidate),
			ProviderAPIEndpoint: modelCenterProviderEndpoint(&candidate),
		}) {
			keyedProvider, err := a.keyedWuyinModelProvider()
			if err != nil {
				return nil, err
			}
			if keyedProvider != nil {
				keyedCandidate := candidate
				keyedCandidate.Provider = *keyedProvider
				expanded = append(expanded, keyedCandidate)
			}
		}
		expanded = append(expanded, candidate)
	}
	return expanded, nil
}

func (a *App) keyedWuyinModelProvider() (*ModelProvider, error) {
	var providers []ModelProvider
	err := a.db.Where("status = ?", ModelCenterStatusOnline).
		Where("LOWER(provider) = ? OR LOWER(name) = ?", "wuyin", "wuyin").
		Order("id asc").
		Find(&providers).Error
	if err != nil {
		return nil, err
	}
	for i := range providers {
		if err := a.hydrateModelProvider(&providers[i]); err != nil {
			return nil, err
		}
		if strings.TrimSpace(providers[i].APIKey) != "" {
			return &providers[i], nil
		}
	}
	return nil, nil
}

func (a *App) modelCenterChannelCandidatesForModel(modelID uint) ([]modelCenterCandidate, error) {
	if modelID == 0 {
		return nil, nil
	}
	var channels []ModelChannel
	if err := a.db.Preload("Model").Preload("Provider").
		Where("model_id = ? AND status = ?", modelID, ModelCenterStatusOnline).
		Order("priority asc, id asc").
		Find(&channels).Error; err != nil {
		return nil, err
	}
	candidates := make([]modelCenterCandidate, 0, len(channels))
	now := time.Now()
	for _, channel := range channels {
		if !modelCenterChannelAvailable(channel, now) {
			continue
		}
		if err := a.hydrateModelProvider(&channel.Provider); err != nil {
			return nil, err
		}
		candidates = append(candidates, modelCenterCandidate{Model: channel.Model, Channel: channel, Provider: channel.Provider})
	}
	return candidates, nil
}

func selectedWuyinVideoModelCenterCandidate(candidates []modelCenterCandidate) *modelCenterCandidate {
	return selectedKeyedVideoModelCenterCandidate(wuyinGrokImagineRuntimeModel, nil, candidates)
}

func selectedKeyedVideoModelCenterCandidate(runtimeModel string, config *ModelConfig, candidates []modelCenterCandidate) *modelCenterCandidate {
	for i := range candidates {
		candidate := candidates[i]
		if modelCenterProviderAPIKey(&candidate) == "" {
			continue
		}
		input := VideoGenerationInput{
			Model:               modelCenterRuntimeModel(&candidate),
			ProviderBaseURL:     modelCenterProviderBaseURL(&candidate),
			ProviderAPIEndpoint: modelCenterProviderEndpoint(&candidate),
		}
		switch {
		case isWuyinGrokImagineModel(runtimeModel, config):
			if !isWuyinGrokImagineVideoInput(input) {
				continue
			}
		case isZZVideoModel(runtimeModel, config):
			if !isZZVideoInput(input) {
				continue
			}
		default:
			continue
		}
		return &candidate
	}
	return nil
}

func videoModelRequiresProviderKey(runtimeModel string, config *ModelConfig) bool {
	return isWuyinGrokImagineModel(runtimeModel, config) || isZZVideoModel(runtimeModel, config)
}

func videoProviderKeyMissingMessage(runtimeModel string, config *ModelConfig) string {
	if isZZVideoModel(runtimeModel, config) {
		return "ZZ_API_KEY or model/provider API key is required"
	}
	return "Wuyin provider key is required"
}

func selectedVideoModelCenterCandidate(job *videoGenerationJob) *modelCenterCandidate {
	if job == nil || len(job.ModelCenterCandidates) == 0 {
		return nil
	}
	return &job.ModelCenterCandidates[0]
}

func (a *App) prepareVideoStyleSelection(c *gin.Context, userID uint, req videoGenerationRequest) (*VideoStylePreset, *UserVideoStyleTemplate, *ReferenceAsset, bool) {
	var preset *VideoStylePreset
	if req.VideoStylePresetID != 0 {
		var found VideoStylePreset
		if err := a.db.Where("id = ? AND is_active = ?", req.VideoStylePresetID, true).First(&found).Error; err != nil {
			writeError(c, http.StatusNotFound, "video_style_preset_not_found", "视频风格预设不存在")
			return nil, nil, nil, false
		}
		preset = &found
	}

	var template *UserVideoStyleTemplate
	var asset *ReferenceAsset
	if req.CustomVideoStyleID != 0 {
		var found UserVideoStyleTemplate
		if err := a.db.Where("id = ? AND user_id = ? AND is_active = ?", req.CustomVideoStyleID, userID, true).First(&found).Error; err != nil {
			writeError(c, http.StatusNotFound, "video_style_template_not_found", "视频风格模板不存在")
			return nil, nil, nil, false
		}
		var styleAsset ReferenceAsset
		if err := a.db.Where("id = ? AND user_id = ?", found.ReferenceAssetID, userID).First(&styleAsset).Error; err != nil {
			writeError(c, http.StatusNotFound, "reference_asset_not_found", "风格参考图不存在")
			return nil, nil, nil, false
		}
		template = &found
		asset = &styleAsset
	}

	return preset, template, asset, true
}

func modelConfigRuntime(model *ModelConfig) string {
	if model == nil {
		return ""
	}
	return strings.TrimSpace(model.RuntimeModel)
}

func videoGenerationStyleLabel(job *videoGenerationJob) string {
	parts := make([]string, 0, 3)
	if job.VideoStylePreset != nil {
		parts = append(parts, strings.TrimSpace(job.VideoStylePreset.Title))
	}
	if job.CustomStyleTemplate != nil {
		parts = append(parts, strings.TrimSpace(job.CustomStyleTemplate.Title))
	}
	if label := strings.TrimSpace(job.Request.StylePreset); label != "" {
		parts = append(parts, label)
	}
	normalized := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		normalized = append(normalized, part)
	}
	return strings.Join(normalized, " / ")
}

func videoGenerationProviderPrompt(job *videoGenerationJob) string {
	base := strings.TrimSpace(job.Request.Prompt)
	stylePrompts := make([]string, 0, 3)
	if job.VideoStylePreset != nil {
		if prompt := strings.TrimSpace(job.VideoStylePreset.StylePrompt); prompt != "" {
			stylePrompts = append(stylePrompts, prompt)
		}
	}
	if job.CustomStyleTemplate != nil {
		if prompt := strings.TrimSpace(job.CustomStyleTemplate.StylePrompt); prompt != "" {
			stylePrompts = append(stylePrompts, prompt)
		}
	}
	if prompt := strings.TrimSpace(job.Request.StylePreset); prompt != "" {
		stylePrompts = append(stylePrompts, prompt)
	}
	if len(stylePrompts) == 0 {
		return base
	}
	return strings.TrimSpace(base + "\n\n视觉风格要求：" + strings.Join(stylePrompts, "；"))
}

func (a *App) createVideoGenerationRecord(job *videoGenerationJob) (GenerationRecord, error) {
	runtimeModel := canonicalVideoRuntimeModel(fallbackString(job.Request.Model, modelConfigRuntime(job.ModelConfig)))
	record := GenerationRecord{
		UserID:          job.User.ID,
		Prompt:          job.Request.Prompt,
		AspectRatio:     job.Request.AspectRatio,
		Quality:         mapVideoQuality(job.Request.HD),
		ToolMode:        "video",
		StylePreset:     videoGenerationStyleLabel(job),
		ModelConfigID:   modelConfigIDValue(job.ModelConfig),
		ModelName:       modelConfigName(job.ModelConfig),
		RuntimeModel:    runtimeModel,
		Model:           runtimeModel,
		Status:          GenerationStatusQueued,
		Stage:           GenerationStageQueued,
		Progress:        5,
		CreditsCost:     job.CreditsCost,
		CreditsDeducted: false,
	}
	if candidate := selectedVideoModelCenterCandidate(job); candidate != nil {
		applyGenerationRecordModelCenter(&record, candidate)
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if job.VideoStylePreset != nil {
			if err := tx.Model(&VideoStylePreset{}).Where("id = ?", job.VideoStylePreset.ID).UpdateColumn("use_count", gorm.Expr("use_count + ?", 1)).Error; err != nil {
				return err
			}
		}
		if job.CustomStyleTemplate != nil {
			if err := tx.Model(&UserVideoStyleTemplate{}).Where("id = ?", job.CustomStyleTemplate.ID).UpdateColumn("use_count", gorm.Expr("use_count + ?", 1)).Error; err != nil {
				return err
			}
		}
		if len(job.ReferenceAssets)+len(job.ReferenceVideoAssets)+len(job.ReferenceAudioAssets) == 0 {
			return a.createVideoGenerationAuditRecord(tx, record, job)
		}
		links := make([]GenerationReferenceAsset, 0, len(job.ReferenceAssets)+len(job.ReferenceVideoAssets)+len(job.ReferenceAudioAssets))
		for index, asset := range job.ReferenceAssets {
			links = append(links, GenerationReferenceAsset{
				GenerationRecordID: record.ID,
				ReferenceAssetID:   asset.ID,
				SortOrder:          index,
				Role:               "image",
			})
		}
		for index, asset := range job.ReferenceVideoAssets {
			links = append(links, GenerationReferenceAsset{GenerationRecordID: record.ID, ReferenceAssetID: asset.ID, SortOrder: len(links) + index, Role: "video"})
		}
		for index, asset := range job.ReferenceAudioAssets {
			links = append(links, GenerationReferenceAsset{GenerationRecordID: record.ID, ReferenceAssetID: asset.ID, SortOrder: len(links) + index, Role: "audio"})
		}
		if err := tx.Create(&links).Error; err != nil {
			return err
		}
		return a.createVideoGenerationAuditRecord(tx, record, job)
	})
	return record, err
}

func (a *App) runVideoGenerationTask(record *GenerationRecord, job *videoGenerationJob, lockKey string) {
	defer a.concurrencyLimiter.Release(lockKey)
	defer func() {
		if recovered := recover(); recovered != nil && record != nil && record.Status != GenerationStatusSucceeded {
			a.failVideoGenerationRecord(record, job, "video_generation_failed", "视频生成任务失败")
		}
	}()
	if _, providerErr, err := a.executeVideoGenerationRecord(record, job); err != nil {
		if strings.TrimSpace(record.ErrorCode) != "" {
			return
		}
		a.failVideoGenerationRecord(record, job, "video_generation_failed", "视频生成任务失败")
		return
	} else if providerErr != nil {
		return
	}
}

func (a *App) executeVideoGenerationRecord(record *GenerationRecord, job *videoGenerationJob) (*generationTaskResult, *ProviderError, error) {
	record.Status = GenerationStatusRunning
	record.Stage = GenerationStageRequestingProvider
	record.Progress = 15
	if err := a.db.Save(record).Error; err != nil {
		return nil, nil, err
	}
	if err := a.syncVideoGenerationAuditRecord(*record, job); err != nil {
		return nil, nil, err
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(job.Settings.RequestTimeoutSeconds)*time.Second)
	defer cancel()

	input, err := a.buildVideoProviderInput(ctx, job)
	if err != nil {
		a.failVideoGenerationRecord(record, job, "reference_asset_read_failed", "参考素材读取失败")
		return nil, nil, err
	}

	submitResult, providerErr := a.videoProvider.SubmitVideo(ctx, input)
	if providerErr != nil {
		a.handleArkVideoReadinessProviderError(job, providerErr)
		applyVideoProviderError(record, providerErr)
		a.failVideoGenerationRecord(record, job, fallbackString(providerErr.Code, "provider_error"), publicVideoGenerationErrorMessage(providerErr.Message, "视频生成提交失败"))
		return nil, providerErr, nil
	}
	record.ProviderRequestID = fallbackString(submitResult.TaskID, submitResult.ProviderRequestID)
	if err := a.db.Save(record).Error; err != nil {
		return nil, nil, err
	}
	if err := a.syncVideoGenerationAuditRecord(*record, job); err != nil {
		return nil, nil, err
	}

	taskResult, providerErr := a.pollVideoUntilDone(ctx, submitResult.TaskID, input)
	record.LatencyMS = time.Since(startedAt).Milliseconds()
	if providerErr != nil {
		a.handleArkVideoReadinessProviderError(job, providerErr)
		applyVideoProviderError(record, providerErr)
		a.failVideoGenerationRecord(record, job, fallbackString(providerErr.Code, "provider_error"), publicVideoGenerationErrorMessage(providerErr.Message, "视频生成失败"))
		return nil, providerErr, nil
	}
	if taskResult.Status == VideoTaskFailed {
		a.failVideoGenerationRecord(record, job, "provider_video_failed", publicVideoGenerationErrorMessage(taskResult.FailReason, "视频生成审核未通过或生成失败"))
		return nil, nil, nil
	}
	if strings.TrimSpace(taskResult.OutputBase64) == "" {
		a.failVideoGenerationRecord(record, job, "provider_empty_video", "视频生成结果为空")
		return nil, nil, nil
	}

	job.ProviderUsageTokens = taskResult.UsageTotalTokens

	content, err := base64.StdEncoding.DecodeString(strings.TrimSpace(taskResult.OutputBase64))
	if err != nil {
		a.failVideoGenerationRecord(record, job, "provider_video_decode_failed", "视频结果解析失败")
		return nil, nil, err
	}
	assetKey, mimeType, err := a.assetStore.SaveBytes(content, fallbackString(taskResult.MIMEType, "video/mp4"))
	if err != nil {
		a.failVideoGenerationRecord(record, job, "asset_store_failed", "视频保存失败")
		return nil, nil, err
	}

	availableCredits, work, err := a.persistVideoWork(record, job, assetKey, mimeType, fallbackString(taskResult.ProviderRequestID, submitResult.TaskID))
	if err != nil {
		if errors.Is(err, errCreditsInsufficient) {
			a.failVideoGenerationRecord(record, job, "credits_insufficient", "点数不足，请先充值")
			return nil, nil, errCreditsInsufficient
		}
		a.failVideoGenerationRecord(record, job, "generation_persist_failed", "视频保存失败")
		return nil, nil, err
	}

	record.AssetKey = assetKey
	record.PreviewURL = work.PreviewURL
	record.DownloadURL = work.DownloadURL
	record.MIMEType = mimeType
	return &generationTaskResult{Record: *record, AvailableCredits: availableCredits}, nil, nil
}

func (a *App) handleArkVideoReadinessProviderError(job *videoGenerationJob, providerErr *ProviderError) {
	if job == nil || job.ModelConfig == nil || providerErr == nil {
		return
	}
	if !isArkSeedanceVideoModel(job.Request.Model, job.ModelConfig) || !isArkVideoUnsupportedContentGenerationError(providerErr) {
		return
	}
	now := time.Now()
	updates := map[string]any{
		"video_readiness_status":     arkVideoReadinessStatusFailed,
		"video_readiness_reason":     arkVideoReadinessUnsupportedReason,
		"video_readiness_checked_at": now,
	}
	if err := a.db.Model(&ModelConfig{}).Where("id = ?", job.ModelConfig.ID).Updates(updates).Error; err != nil {
		log.Printf("ark video readiness downgrade failed model_config_id=%d model=%q request_id=%q code=%q error=%v",
			job.ModelConfig.ID,
			canonicalVideoRuntimeModel(fallbackString(job.Request.Model, modelConfigRuntime(job.ModelConfig))),
			providerErr.ProviderRequestID,
			providerErr.Code,
			err,
		)
		return
	}
	job.ModelConfig.VideoReadinessStatus = arkVideoReadinessStatusFailed
	job.ModelConfig.VideoReadinessReason = arkVideoReadinessUnsupportedReason
	job.ModelConfig.VideoReadinessCheckedAt = &now
	log.Printf("ark video readiness downgraded model_config_id=%d model=%q request_id=%q code=%q failure_stage=%q",
		job.ModelConfig.ID,
		canonicalVideoRuntimeModel(fallbackString(job.Request.Model, modelConfigRuntime(job.ModelConfig))),
		providerErr.ProviderRequestID,
		providerErr.Code,
		providerErr.FailureStage,
	)
}

func isArkVideoUnsupportedContentGenerationError(providerErr *ProviderError) bool {
	if providerErr == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(providerErr.Message))
	code := strings.ToLower(strings.TrimSpace(providerErr.Code))
	if strings.Contains(message, "does not support content generation") {
		return true
	}
	return strings.Contains(code, "invalidparameter") &&
		strings.Contains(message, "model") &&
		strings.Contains(message, "content generation")
}

func publicVideoGenerationErrorMessage(message, fallback string) string {
	if localized, ok := localizedKnownVideoGenerationError(message); ok {
		return localized
	}
	return fallbackString(strings.TrimSpace(message), fallback)
}

func localizedKnownVideoGenerationError(message string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return "", false
	}
	referenceImageSignals := []string{
		"requires an input image",
		"input image is required",
		"requires input image",
		"image input is required",
		"text-to-video is not supported",
		"text to video is not supported",
	}
	for _, signal := range referenceImageSignals {
		if strings.Contains(normalized, signal) {
			return videoReferenceImageRequiredMessage, true
		}
	}
	return "", false
}

func (a *App) pollVideoUntilDone(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	for {
		result, providerErr := a.videoProvider.PollVideo(ctx, taskID, input)
		if providerErr != nil {
			return VideoTaskResult{}, providerErr
		}
		switch strings.TrimSpace(result.Status) {
		case VideoTaskSucceeded, VideoTaskFailed:
			return result, nil
		case "", VideoTaskNotStarted, VideoTaskInProgress:
			select {
			case <-ctx.Done():
				return VideoTaskResult{}, &ProviderError{Code: "provider_timeout", Message: ctx.Err().Error()}
			case <-time.After(videoPollInterval()):
			}
		default:
			select {
			case <-ctx.Done():
				return VideoTaskResult{}, &ProviderError{Code: "provider_timeout", Message: ctx.Err().Error()}
			case <-time.After(videoPollInterval()):
			}
		}
	}
}

func (a *App) buildVideoProviderInput(ctx context.Context, job *videoGenerationJob) (VideoGenerationInput, error) {
	images := make([]string, 0, len(job.Request.Images)+len(job.ReferenceAssets))
	for _, image := range job.Request.Images {
		if text := strings.TrimSpace(image); text != "" {
			images = append(images, text)
		}
	}
	referenceImageURLs, err := a.buildReferenceAssetURLs(job.ReferenceAssets, "image/png")
	if err != nil {
		return VideoGenerationInput{}, err
	}
	images = append(images, referenceImageURLs...)
	if job.CustomStyleAsset != nil {
		assetURL, err := a.referenceAssetAccessURL(*job.CustomStyleAsset, "image/png", true, false)
		if err != nil {
			return VideoGenerationInput{}, err
		}
		images = append(images, assetURL)
	}
	referenceVideos, err := a.buildReferenceAssetURLs(job.ReferenceVideoAssets, "video/mp4")
	if err != nil {
		return VideoGenerationInput{}, err
	}
	referenceAudios, err := a.buildReferenceAssetURLs(job.ReferenceAudioAssets, "audio/mpeg")
	if err != nil {
		return VideoGenerationInput{}, err
	}

	return VideoGenerationInput{
		Model:               fallbackString(canonicalVideoRuntimeModel(job.Request.Model), wuyinGrokImagineRuntimeModel),
		Prompt:              videoGenerationProviderPrompt(job),
		AspectRatio:         job.Request.AspectRatio,
		Duration:            job.Request.Duration,
		Resolution:          job.Request.Resolution,
		HD:                  job.Request.HD,
		Watermark:           job.Request.Watermark,
		Private:             job.Request.Private == nil || *job.Request.Private,
		ProviderBaseURL:     videoGenerationProviderBaseURL(job),
		ProviderAPIKey:      videoGenerationProviderAPIKey(job),
		ProviderAPIEndpoint: videoGenerationProviderAPIEndpoint(job),
		NotifyHook:          job.Request.NotifyHook,
		Images:              images,
		ReferenceVideos:     referenceVideos,
		ReferenceAudios:     referenceAudios,
		GenerateAudio:       job.Request.GenerateAudio,
	}, ctx.Err()
}

func (a *App) buildReferenceAssetURLs(assets []ReferenceAsset, fallbackMIMEType string) ([]string, error) {
	if len(assets) == 0 {
		return nil, nil
	}
	urls := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetURL, err := a.referenceAssetAccessURL(asset, fallbackMIMEType, true, false)
		if err != nil {
			return nil, err
		}
		urls = append(urls, assetURL)
	}
	return urls, nil
}

func videoGenerationProviderBaseURL(job *videoGenerationJob) string {
	if candidate := selectedVideoModelCenterCandidate(job); candidate != nil {
		if baseURL := modelCenterProviderBaseURL(candidate); baseURL != "" {
			return baseURL
		}
	}
	return modelConfigProviderBaseURL(jobModelConfig(job))
}

func videoGenerationProviderAPIKey(job *videoGenerationJob) string {
	if candidate := selectedVideoModelCenterCandidate(job); candidate != nil {
		if apiKey := modelCenterProviderAPIKey(candidate); apiKey != "" {
			return apiKey
		}
	}
	if isZZVideoModel("", jobModelConfig(job)) {
		if apiKey := strings.TrimSpace(job.FallbackProviderAPIKey); apiKey != "" {
			return apiKey
		}
	}
	if apiKey := strings.TrimSpace(job.FallbackProviderAPIKey); apiKey != "" {
		return apiKey
	}
	return modelConfigProviderAPIKey(jobModelConfig(job))
}

func videoGenerationProviderAPIEndpoint(job *videoGenerationJob) string {
	if candidate := selectedVideoModelCenterCandidate(job); candidate != nil {
		if endpoint := modelCenterProviderEndpoint(candidate); endpoint != "" {
			return endpoint
		}
	}
	return modelConfigProviderAPIEndpoint(jobModelConfig(job))
}

func (a *App) persistVideoWork(record *GenerationRecord, job *videoGenerationJob, assetKey, mimeType, providerRequestID string) (int, Work, error) {
	availableCredits := 0
	var persistedWork Work
	err := a.db.Transaction(func(tx *gorm.DB) error {
		work := Work{
			UserID:             job.User.ID,
			GenerationRecordID: record.ID,
			Prompt:             job.Request.Prompt,
			AspectRatio:        job.Request.AspectRatio,
			Category:           WorkCategoryVideo,
			Model:              fallbackString(job.Request.Model, modelConfigRuntime(job.ModelConfig)),
			Status:             GenerationStatusSucceeded,
			Visibility:         WorkVisibilityPrivate,
			AssetKey:           assetKey,
			MIMEType:           mimeType,
			ProviderRequestID:  providerRequestID,
		}
		if err := tx.Create(&work).Error; err != nil {
			return err
		}
		if publicURL := a.assetStore.PublicURL(assetKey); publicURL != "" {
			work.PreviewURL = publicURL
			work.DownloadURL = publicURL
		} else {
			work.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
			work.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
		}
		if err := tx.Save(&work).Error; err != nil {
			return err
		}

		remainingCredits, err := deductGenerationCredits(tx, job.User.ID, job.CreditsCost)
		if err != nil {
			return err
		}
		availableCredits = remainingCredits

		creditTransaction := CreditTransaction{
			UserID:       job.User.ID,
			Type:         CreditTransactionTypeGenerationCharge,
			Amount:       -job.CreditsCost,
			BalanceAfter: remainingCredits,
			Reason:       "视频生成扣点",
			RelatedType:  "generation",
			RelatedID:    record.ID,
		}
		if err := tx.Create(&creditTransaction).Error; err != nil {
			return err
		}

		record.WorkID = &work.ID
		record.Status = GenerationStatusSucceeded
		record.Stage = GenerationStageSucceeded
		record.Progress = 100
		record.AssetKey = assetKey
		record.PreviewURL = work.PreviewURL
		record.DownloadURL = work.DownloadURL
		record.MIMEType = mimeType
		record.CreditsDeducted = true
		record.ProviderRequestID = providerRequestID
		if err := tx.Save(record).Error; err != nil {
			return err
		}
		if err := a.syncVideoGenerationAuditRecordTx(tx, *record, job); err != nil {
			return err
		}
		persistedWork = work
		return nil
	})
	return availableCredits, persistedWork, err
}

func normalizeVideoAspectRatio(value string) string {
	switch strings.TrimSpace(value) {
	case "9:16":
		return "9:16"
	default:
		return "16:9"
	}
}

func normalizeVideoDuration(value string) string {
	switch strings.TrimSpace(value) {
	case "6":
		return "6"
	case "15":
		return "15"
	case "25":
		return "25"
	default:
		return "10"
	}
}

func normalizeVideoAspectRatioForCapabilities(value string, capability videoModelCapability) string {
	return normalizeVideoCapabilityValue(value, capability.AspectRatios, "16:9")
}

func normalizeVideoDurationForCapabilities(value string, capability videoModelCapability, fallback string) string {
	return normalizeVideoCapabilityValue(value, capability.Durations, fallback)
}

func normalizeVideoResolutionForCapabilities(req videoGenerationRequest, capability videoModelCapability) string {
	if len(capability.ResolutionOptions) == 0 {
		return ""
	}
	value := strings.ToLower(strings.TrimSpace(req.Resolution))
	if value == "" {
		value = legacyVideoResolution(req, capability)
	}
	if value == "" {
		value = capability.DefaultResolution
	}
	return normalizeVideoCapabilityValue(value, capability.ResolutionOptions, capability.DefaultResolution)
}

func legacyVideoResolution(req videoGenerationRequest, capability videoModelCapability) string {
	if req.HD {
		if contains(capability.ResolutionOptions, "1080p") {
			return "1080p"
		}
		if contains(capability.ResolutionOptions, "720p") {
			return "720p"
		}
	}
	if contains(capability.ResolutionOptions, capability.DefaultResolution) {
		return capability.DefaultResolution
	}
	if len(capability.ResolutionOptions) > 0 {
		return capability.ResolutionOptions[0]
	}
	return ""
}

func videoRequestIsHD(req videoGenerationRequest) bool {
	switch strings.ToLower(strings.TrimSpace(req.Resolution)) {
	case "720p", "1080p":
		return true
	default:
		return req.HD
	}
}

func defaultVideoDurationForModel(runtimeModel string, config *ModelConfig) string {
	if isWuyinGrokImagineModel(runtimeModel, config) {
		return "3"
	}
	if isZZVideoModel(runtimeModel, config) {
		return "15"
	}
	return "10"
}

func normalizeVideoCapabilityValue(value string, allowed []string, fallback string) string {
	value = strings.TrimSpace(value)
	if len(allowed) == 0 {
		return value
	}
	if value == "" {
		if contains(allowed, fallback) {
			return fallback
		}
		return allowed[0]
	}
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), value) {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

func mapVideoQuality(hd bool) string {
	if hd {
		return GenerationQualityHigh
	}
	return GenerationQualityMedium
}

func videoCreditCost(req videoGenerationRequest) int {
	return videoCreditCostForModel(req, nil)
}

func videoCreditCostForModel(req videoGenerationRequest, config *ModelConfig) int {
	if creditsPerSecond, ok := videoCreditsPerSecondForModel(req, config); ok {
		return videoBillingSeconds(req.Duration) * creditsPerSecond
	}
	return legacyVideoCreditCost(req)
}

func videoCreditsPerSecond(req videoGenerationRequest) (int, bool) {
	return videoCreditsPerSecondForModel(req, nil)
}

func videoCreditsPerSecondForModel(req videoGenerationRequest, config *ModelConfig) (int, bool) {
	profile, ok := videoBillingProfileForModel(req.Model, config)
	if !ok {
		return 0, false
	}
	capability := videoModelCapability{
		ResolutionOptions: append([]string(nil), profile.ResolutionOptions...),
		DefaultResolution: profile.DefaultResolution,
	}
	resolution := normalizeVideoResolutionForCapabilities(req, capability)
	for _, rule := range profile.PriceRules {
		if rule.CreditsPerSecond <= 0 {
			continue
		}
		if len(profile.ResolutionOptions) == 0 || strings.EqualFold(rule.Resolution, resolution) {
			return rule.CreditsPerSecond, true
		}
	}
	return 0, false
}

func videoBillingSeconds(value string) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 10
	}
	return seconds
}

func legacyVideoCreditCost(req videoGenerationRequest) int {
	cost := 5
	if req.Duration == "15" {
		cost += 3
	}
	if req.Duration == "25" {
		cost += 7
	}
	if req.HD {
		cost += 4
	}
	if strings.TrimSpace(req.Model) == "sora-2-pro" {
		cost += 3
	}
	return cost
}

func videoPollInterval() time.Duration {
	return 2 * time.Second
}
