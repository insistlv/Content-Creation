package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

var errCreditsInsufficient = errors.New("credits_insufficient")
var errGenerationRecordAlreadyTerminal = errors.New("generation_record_already_terminal")

const modelFailoverCooldownTTL = 60 * time.Second
const maxExpandCanvasPixels = 16_000_000
const imageGenerationHardTimeout = 10 * time.Minute

const (
	imageGenerationTimeoutErrorCode    = "provider_timeout"
	imageGenerationCancelledErrorCode  = "user_cancelled"
	imageGenerationCancelledMessage    = "已取消生成，未扣点。"
	imageGenerationTimeoutErrorMessage = "图片生成超过10分钟未完成，已自动判定失败，请重新生成。"
)

type generationRequest struct {
	Prompt                   string         `json:"prompt"`
	NegativePrompt           string         `json:"negative_prompt"`
	AspectRatio              string         `json:"aspect_ratio"`
	Quality                  string         `json:"quality"`
	StylePreset              string         `json:"style_preset"`
	ToolMode                 string         `json:"tool_mode"`
	ToolOptions              map[string]any `json:"tool_options"`
	StyleStrength            *int           `json:"style_strength"`
	ReferenceWeight          *int           `json:"reference_weight"`
	Seed                     string         `json:"seed"`
	SourceWorkID             *uint          `json:"source_work_id"`
	MaskAssetID              *uint          `json:"mask_asset_id"`
	EditInstruction          string         `json:"edit_instruction"`
	ReferenceAssetIDs        []uint         `json:"reference_asset_ids"`
	ReferenceWorkIDs         []uint         `json:"reference_work_ids"`
	Num                      int            `json:"num"`
	BatchID                  string         `json:"batch_id"`
	BatchIndex               int            `json:"batch_index"`
	BatchTotal               int            `json:"batch_total"`
	VariationMode            string         `json:"variation_mode"`
	VariationPrompt          string         `json:"variation_prompt"`
	ReferenceIntent          string         `json:"reference_intent"`
	BackgroundReferenceIndex *int           `json:"background_reference_index"`
	ModelID                  uint           `json:"model_id"`
	Size                     string
}

type generationJob struct {
	User                  User
	Settings              AppSettings
	ModelConfig           *ModelConfig
	ModelCandidates       []ModelConfig
	ModelCenterModel      *ModelCatalog
	ModelCenterChannel    *ModelChannel
	ModelCenterCandidates []modelCenterCandidate
	Request               generationRequest
	SourceWork            *Work
	ReferenceAssets       []ReferenceAsset
	ReferenceWorks        []Work
}

type generationTaskResult struct {
	Record           GenerationRecord
	AvailableCredits int
}

func generationReferenceCreditCost(referenceAssetCount, referenceWorkCount int) int {
	return 0
}

func generationTaskCreditCost(req generationRequest, referenceAssetCount, referenceWorkCount int) int {
	return generationTaskCreditCostWithBase(req, referenceAssetCount, referenceWorkCount, 1)
}

func generationTaskCreditCostWithBase(req generationRequest, referenceAssetCount, referenceWorkCount, baseCost int) int {
	return 1
}

func generationRequiredCredits(req generationRequest, referenceAssetCount, referenceWorkCount int) int {
	return generationRequiredCreditsWithBase(req, referenceAssetCount, referenceWorkCount, 1)
}

func generationRequiredCreditsWithBase(req generationRequest, referenceAssetCount, referenceWorkCount, baseCost int) int {
	if req.BatchID != "" && req.BatchTotal > 1 && req.BatchIndex <= 0 {
		return req.BatchTotal
	}
	return generationTaskCreditCostWithBase(req, referenceAssetCount, referenceWorkCount, baseCost)
}

func generationUnitCreditCost(req generationRequest, baseCost int) int {
	return 1
}

func generationJobCreditCost(job *generationJob) int {
	if job == nil {
		return 1
	}
	return generationTaskCreditCostWithBase(job.Request, len(job.ReferenceAssets), len(job.ReferenceWorks), generationJobBaseCreditCost(job))
}

func generationJobBaseCreditCost(job *generationJob) int {
	return 1
}

func generationJobRequiredCredits(job *generationJob) int {
	if job == nil {
		return 1
	}
	return generationRequiredCreditsWithBase(job.Request, len(job.ReferenceAssets), len(job.ReferenceWorks), generationJobBaseCreditCost(job))
}

func generationRecordCreditCost(record GenerationRecord) int {
	if record.CreditsCost > 0 {
		return record.CreditsCost
	}
	return 1
}

func (a *App) prepareGenerationJob(c *gin.Context) (*generationJob, bool) {
	job, ok := a.prepareGenerationJobInputs(c)
	if !ok {
		return nil, false
	}

	estimate, err := a.buildCreditEstimate(job.User.ID, generationJobRequiredCredits(job))
	if err != nil {
		writeError(c, 500, "balance_load_failed", "账户读取失败")
		return nil, false
	}
	if !estimate.Enough {
		writeCreditsInsufficientError(c, estimate)
		return nil, false
	}

	rateKey := clientIP(c.Request) + "|user:" + strconv.FormatUint(uint64(job.User.ID), 10)
	window := time.Duration(job.Settings.RateLimitWindowSeconds) * time.Second
	if !a.rateLimiter.Allow(rateKey, time.Now(), window, job.Settings.RateLimitMaxRequests) {
		writeError(c, 429, "too_many_requests", "请求过于频繁")
		return nil, false
	}

	return job, true
}

func (a *App) prepareGenerationJobInputs(c *gin.Context) (*generationJob, bool) {
	user := currentUser(c)

	var req generationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid_request", "请求格式错误")
		return nil, false
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.NegativePrompt = strings.TrimSpace(req.NegativePrompt)
	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	req.Quality = normalizeGenerationQuality(req.Quality)
	req.StylePreset = strings.TrimSpace(req.StylePreset)
	req.ToolMode = normalizeGenerationToolMode(req.ToolMode)
	req.ToolOptions = normalizeGenerationToolOptions(req.ToolOptions)
	req.ToolOptions = normalizeGenerationToolOptionsForMode(req.ToolMode, req.ToolOptions)
	req.Seed = strings.TrimSpace(req.Seed)
	req.EditInstruction = strings.TrimSpace(req.EditInstruction)
	req.VariationMode = normalizeGenerationVariationMode(req.VariationMode)
	req.VariationPrompt = strings.TrimSpace(req.VariationPrompt)
	if req.VariationPrompt == "" {
		req.VariationMode = ""
	}
	req.ReferenceIntent = normalizeGenerationReferenceIntent(req.ReferenceIntent)
	normalizeGenerationBatchMetadata(&req)
	if req.Prompt == "" {
		writeError(c, 400, "prompt_required", "提示词不能为空")
		return nil, false
	}
	if !isValidGenerationQuality(req.Quality) {
		writeError(c, 400, "invalid_generation_parameter", "不支持的清晰度设置")
		return nil, false
	}
	if !isValidGenerationToolMode(req.ToolMode) {
		writeError(c, 400, "invalid_generation_parameter", "不支持的创作工具")
		return nil, false
	}
	if !normalizeGenerationNum(&req) {
		writeError(c, 400, "invalid_generation_parameter", "生成数量必须在 1 到 4 之间")
		return nil, false
	}
	if ok, message := validateGenerationToolOptions(req); !ok {
		writeError(c, 400, "invalid_generation_parameter", message)
		return nil, false
	}
	if !isValidGenerationReferenceIntent(req.ReferenceIntent) {
		writeError(c, 400, "invalid_generation_parameter", "不支持的参考图意图")
		return nil, false
	}
	if req.StyleStrength == nil {
		defaultValue := 65
		req.StyleStrength = &defaultValue
	}
	if req.ReferenceWeight == nil {
		defaultValue := 75
		req.ReferenceWeight = &defaultValue
	}
	if !isPercentRange(*req.StyleStrength) || !isPercentRange(*req.ReferenceWeight) {
		writeError(c, 400, "invalid_generation_parameter", "强度参数必须在 0 到 100 之间")
		return nil, false
	}
	size, ok := aspectRatioToSize(req.AspectRatio)
	if !ok {
		writeError(c, 400, "invalid_aspect_ratio", "不支持的画幅比例")
		return nil, false
	}
	req.Size = size

	referenceAssets, ok := a.prepareReferenceAssets(c, user.ID, req.ReferenceAssetIDs)
	if !ok {
		return nil, false
	}
	referenceWorks, ok := a.prepareReferenceWorks(c, user.ID, req.ReferenceWorkIDs, len(referenceAssets))
	if !ok {
		return nil, false
	}
	sourceWork, ok := a.prepareSourceWork(c, user.ID, req.SourceWorkID)
	if !ok {
		return nil, false
	}
	if isEditToolMode(req.ToolMode) && sourceWork == nil && len(referenceAssets) == 0 && len(referenceWorks) == 0 {
		writeError(c, 400, "invalid_generation_parameter", "编辑类工具需要上传图片或选择当前作品")
		return nil, false
	}
	if req.ToolMode == GenerationToolModeExpand || req.ToolMode == GenerationToolModeErase || req.ToolMode == GenerationToolModeRemoveBackground || req.ToolMode == GenerationToolModeUpscale || req.ToolMode == GenerationToolModePrecisionEdit {
		sourceInputCount := len(referenceAssets) + len(referenceWorks)
		if sourceWork != nil {
			sourceInputCount++
		}
		if sourceInputCount != 1 {
			message := "智能扩图需要且只能选择一张源图"
			if req.ToolMode == GenerationToolModeErase {
				message = "移除物体需要且只能选择一张源图"
			}
			if req.ToolMode == GenerationToolModeRemoveBackground {
				message = "移除背景需要且只能选择一张源图"
			}
			if req.ToolMode == GenerationToolModeUpscale {
				message = "高清放大需要且只能选择一张源图"
			}
			if req.ToolMode == GenerationToolModePrecisionEdit {
				message = "精细编辑需要且只能选择一张源图"
			}
			writeError(c, 400, "invalid_generation_parameter", message)
			return nil, false
		}
	}
	if !a.validateMaskAsset(c, user.ID, req.MaskAssetID) {
		return nil, false
	}
	referenceInputCount := len(referenceAssets) + len(referenceWorks)
	if sourceWork != nil {
		referenceInputCount++
	}
	if !normalizeGenerationBackgroundReferenceIndex(&req, referenceInputCount) {
		writeError(c, 400, "invalid_generation_parameter", "背景参考图序号无效")
		return nil, false
	}

	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, 500, "settings_load_failed", "配置读取失败")
		return nil, false
	}
	modelCenterCandidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, req.ModelID)
	if err != nil {
		writeError(c, 500, "model_center_load_failed", "模型中心配置读取失败")
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
			writeError(c, 500, "model_config_load_failed", "模型配置读取失败")
			return nil, false
		}
		if len(modelCandidates) > 0 {
			modelConfig = &modelCandidates[0]
		} else {
			modelConfig, err = a.modelConfigForGeneration(settings)
			if err != nil {
				writeError(c, 500, "model_config_load_failed", "模型配置读取失败")
				return nil, false
			}
		}
	}

	return &generationJob{
		User:                  *user,
		Settings:              settings,
		ModelConfig:           modelConfig,
		ModelCandidates:       modelCandidates,
		ModelCenterModel:      modelCenterModel,
		ModelCenterChannel:    modelCenterChannel,
		ModelCenterCandidates: modelCenterCandidates,
		Request:               req,
		SourceWork:            sourceWork,
		ReferenceAssets:       referenceAssets,
		ReferenceWorks:        referenceWorks,
	}, true
}

func (a *App) acquireGenerationLock(c *gin.Context, userID uint) (string, bool) {
	lockKey := strconv.FormatUint(uint64(userID), 10)
	if !a.concurrencyLimiter.TryAcquire(lockKey) {
		writeError(c, 409, "generation_in_progress", "已有图片生成任务进行中")
		return "", false
	}
	return lockKey, true
}

// acquireImageGenerationSlot 为单用户图片生成申请一个并发槽位。与视频用的单任务互斥锁不同，
// 这里是计数信号量（上限 maxConcurrentImageGenerationsPerUser），既允许批量并行，又能防止
// 单用户瞬间发起海量并行任务。返回的 key 在任务结束后必须 Release。
func (a *App) acquireImageGenerationSlot(c *gin.Context, userID uint) (string, bool) {
	key := strconv.FormatUint(uint64(userID), 10)
	if !a.imageGenLimiter.TryAcquire(key) {
		writeError(c, http.StatusTooManyRequests, "generation_concurrency_limit", "并发生成任务过多，请稍后再试")
		return "", false
	}
	return key, true
}

func (a *App) createGenerationRecord(job *generationJob, status, stage string) (GenerationRecord, error) {
	record := GenerationRecord{
		UserID:          job.User.ID,
		Prompt:          job.Request.Prompt,
		NegativePrompt:  job.Request.NegativePrompt,
		AspectRatio:     job.Request.AspectRatio,
		Quality:         job.Request.Quality,
		StylePreset:     job.Request.StylePreset,
		ToolMode:        job.Request.ToolMode,
		ToolOptionsJSON: encodeGenerationToolOptions(job.Request.ToolOptions),
		BatchID:         job.Request.BatchID,
		BatchIndex:      job.Request.BatchIndex,
		BatchTotal:      job.Request.BatchTotal,
		VariationMode:   job.Request.VariationMode,
		VariationPrompt: job.Request.VariationPrompt,
		StyleStrength:   *job.Request.StyleStrength,
		ReferenceWeight: *job.Request.ReferenceWeight,
		Seed:            job.Request.Seed,
		SourceWorkID:    job.Request.SourceWorkID,
		MaskAssetID:     job.Request.MaskAssetID,
		EditInstruction: job.Request.EditInstruction,
		Status:          normalizeGenerationStatus(status),
		Stage:           normalizeGenerationStage(status, stage),
		CreditsCost:     generationJobCreditCost(job),
	}
	if candidate := selectedModelCenterCandidate(job); candidate != nil {
		applyGenerationRecordModelCenter(&record, candidate)
	} else {
		record.ModelConfigID = modelConfigIDValue(job.ModelConfig)
		record.Model = generationRuntimeModel(job.Settings, job.ModelConfig)
		record.RuntimeModel = record.Model
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if len(job.ReferenceAssets) == 0 {
			return nil
		}

		links := make([]GenerationReferenceAsset, 0, len(job.ReferenceAssets))
		for index, asset := range job.ReferenceAssets {
			links = append(links, GenerationReferenceAsset{
				GenerationRecordID: record.ID,
				ReferenceAssetID:   asset.ID,
				SortOrder:          index,
			})
		}
		return tx.Create(&links).Error
	})
	if err == nil && len(job.ReferenceAssets) > 0 {
		record.ReferenceAssetIDs = make([]uint, 0, len(job.ReferenceAssets))
		for _, asset := range job.ReferenceAssets {
			record.ReferenceAssetIDs = append(record.ReferenceAssetIDs, asset.ID)
		}
	}
	if err == nil {
		a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "task_created", "生成任务已创建", map[string]any{
			"user_id":                 record.UserID,
			"model":                   record.Model,
			"status":                  record.Status,
			"tool_mode":               record.ToolMode,
			"quality":                 record.Quality,
			"aspect_ratio":            record.AspectRatio,
			"reference_asset_count":   len(job.ReferenceAssets),
			"reference_work_count":    len(job.ReferenceWorks),
			"credits_cost":            record.CreditsCost,
			"source_work_configured":  job.SourceWork != nil,
			"model_configured":        job.ModelConfig != nil,
			"provider_api_endpoint":   modelConfigProviderAPIEndpoint(job.ModelConfig),
			"provider_base_url_host":  providerHostForEvent(modelConfigProviderBaseURL(job.ModelConfig)),
			"request_timeout_seconds": job.Settings.RequestTimeoutSeconds,
		})
	}
	return record, err
}

func (a *App) registerImageGenerationCancel(recordID uint, cancel context.CancelFunc) {
	if a == nil || recordID == 0 || cancel == nil {
		return
	}
	a.imageGenerationCancels.Store(recordID, cancel)
}

func (a *App) unregisterImageGenerationCancel(recordID uint) {
	if a == nil || recordID == 0 {
		return
	}
	a.imageGenerationCancels.Delete(recordID)
}

func (a *App) cancelImageGenerationContext(recordID uint) {
	if a == nil || recordID == 0 {
		return
	}
	value, ok := a.imageGenerationCancels.Load(recordID)
	if !ok {
		return
	}
	if cancel, ok := value.(context.CancelFunc); ok {
		cancel()
	}
}

func (a *App) executeGenerationRecord(record *GenerationRecord, job *generationJob) (*generationTaskResult, *ProviderError, error) {
	return a.executeGenerationRecordWithOptions(record, job, generationExecutionOptions{Context: context.Background(), BillingMode: generationBillingDirect})
}

type generationBillingMode string

const (
	generationBillingDirect              generationBillingMode = "direct"
	generationBillingExternalReservation generationBillingMode = "external_reservation"
	generationBillingQueueReservation    generationBillingMode = "queue_reservation"
)

type generationExecutionOptions struct {
	Context              context.Context
	BillingMode          generationBillingMode
	ResultStorageScope   string
	ResultWorkCategory   string
	IdempotencyKey       string
	CommerceProjectID    uint
	AfterObjectGuard     func()
	TransformResult      func(base64Image, mimeType string) (string, string, error)
	DeferProviderFailure bool
	ExecutionLeaseToken  string
}

func (a *App) executeGenerationRecordWithOptions(record *GenerationRecord, job *generationJob, options generationExecutionOptions) (*generationTaskResult, *ProviderError, error) {
	if strings.TrimSpace(options.ExecutionLeaseToken) == "" {
		parent := options.Context
		if parent == nil {
			parent = context.Background()
		}
		recordID := record.ID
		leaseToken, err := a.acquireImageExecutionLease(parent, "direct-"+uuid.NewString(), generationExecutionLeaseMeta{
			RecordID: &recordID, UserID: job.User.ID, ProviderID: generationJobProviderID(job), ChannelID: generationJobChannelID(job), EntryPoint: fallbackString(job.Request.ToolMode, "image"),
		})
		if err != nil {
			return nil, nil, err
		}
		stopRenew := make(chan struct{})
		go a.renewImageExecutionLease(leaseToken, stopRenew)
		defer close(stopRenew)
		defer a.releaseImageExecutionLease(leaseToken)
		options.ExecutionLeaseToken = leaseToken
	}
	record.Status = GenerationStatusRunning
	record.Stage = GenerationStageRequestingProvider
	record.ErrorCode = ""
	record.ErrorMessage = ""
	record.ProviderHTTPStatus = 0
	record.ProviderErrorCode = ""
	record.ProviderErrorMessage = ""
	record.ProviderFailureStage = ""
	record.ProviderAttemptCount = 0
	if err := a.db.Save(record).Error; err != nil {
		return nil, nil, err
	}
	a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "task_running", "生成任务开始执行", map[string]any{
		"model": record.Model,
		"stage": record.Stage,
	})

	startedAt := time.Now()
	requestTimeout := generationRequestTimeout(job.Settings)
	parent := options.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	if record.ID != 0 {
		a.registerImageGenerationCancel(record.ID, cancel)
		defer a.unregisterImageGenerationCancel(record.ID)
	}
	defer cancel()

	referenceSpoolFiles := make([]string, 0, len(job.ReferenceAssets)+len(job.ReferenceWorks)+2)
	defer func() {
		for _, name := range referenceSpoolFiles {
			if strings.TrimSpace(name) != "" {
				_ = os.Remove(name)
			}
		}
	}()
	referenceImages, err := a.buildReferenceImages(job.ReferenceAssets)
	if err != nil {
		a.failGenerationRecord(record, "reference_asset_read_failed", "参考素材读取失败")
		return nil, nil, err
	}
	for _, image := range referenceImages {
		referenceSpoolFiles = append(referenceSpoolFiles, image.FilePath)
	}
	workReferenceImages, err := a.buildReferenceImagesFromWorks(job.ReferenceWorks)
	if err != nil {
		a.failGenerationRecord(record, "reference_work_read_failed", "参考作品读取失败")
		return nil, nil, err
	}
	for _, image := range workReferenceImages {
		referenceSpoolFiles = append(referenceSpoolFiles, image.FilePath)
	}
	referenceImages = append(referenceImages, workReferenceImages...)
	sourceImage, err := a.buildSourceImage(job.SourceWork)
	if err != nil {
		a.failGenerationRecord(record, "source_work_read_failed", "源作品读取失败")
		return nil, nil, err
	}
	if sourceImage != nil {
		referenceSpoolFiles = append(referenceSpoolFiles, sourceImage.FilePath)
	}
	if sourceImage == nil && isEditToolMode(job.Request.ToolMode) && len(referenceImages) > 0 {
		promoted := referenceImages[0]
		sourceImage = &promoted
		referenceImages = referenceImages[1:]
	}
	var expandOriginalSource *ReferenceImageInput
	var expandMaskImage *ReferenceImageInput
	if job.Request.ToolMode == GenerationToolModeExpand && sourceImage != nil {
		originalSource := *sourceImage
		expandedSource, generatedMask, providerSize, err := buildExpandedSourceImage(originalSource, job.Request.ToolOptions)
		if err != nil {
			a.failGenerationRecord(record, "expand_source_prepare_failed", "扩图源图处理失败")
			return nil, nil, err
		}
		expandOriginalSource = &originalSource
		expandMaskImage = generatedMask
		sourceImage = expandedSource
		if providerSize != "" {
			job.Request.Size = providerSize
		}
	}
	maskImage, err := a.buildMaskImage(job.User.ID, job.Request.MaskAssetID)
	if err != nil {
		a.failGenerationRecord(record, "mask_asset_read_failed", "蒙版素材读取失败")
		return nil, nil, err
	}
	if maskImage != nil {
		referenceSpoolFiles = append(referenceSpoolFiles, maskImage.FilePath)
	}
	if expandMaskImage != nil {
		maskImage = expandMaskImage
	}
	orderedReferenceInputCount := len(referenceImages)
	if sourceImage != nil {
		orderedReferenceInputCount++
	}
	if maskImage != nil {
		orderedReferenceInputCount++
	}
	a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "reference_inputs_prepared", "生成输入素材已准备", map[string]any{
		"reference_asset_count": len(job.ReferenceAssets),
		"reference_work_count":  len(job.ReferenceWorks),
		"reference_image_count": orderedReferenceInputCount,
		"source_work_id":        optionalUintForEvent(job.Request.SourceWorkID),
		"mask_asset_id":         optionalUintForEvent(job.Request.MaskAssetID),
	})
	compositionPlan := a.planImageComposition(ctx, job, orderedReferenceInputCount)
	if compositionPlan != nil {
		a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "compose_plan_prepared", "多参考图合成计划已准备", map[string]any{
			"source":                     compositionPlan.Source,
			"fallback_reason":            compositionPlan.FallbackReason,
			"reference_image_count":      orderedReferenceInputCount,
			"background_reference_index": optionalIntPointerForEvent(compositionPlan.BackgroundReferenceIndex),
		})
	}

	inputModel := generationRuntimeModel(job.Settings, job.ModelConfig)
	providerBaseURL := modelConfigProviderBaseURL(job.ModelConfig)
	providerAPIKey := modelConfigProviderAPIKey(job.ModelConfig)
	providerAPIEndpoint := modelConfigProviderAPIEndpoint(job.ModelConfig)
	if candidate := selectedModelCenterCandidate(job); candidate != nil {
		inputModel = modelCenterRuntimeModel(candidate)
		providerBaseURL = modelCenterProviderBaseURL(candidate)
		providerAPIKey = modelCenterProviderAPIKey(candidate)
		providerAPIEndpoint = modelCenterProviderEndpoint(candidate)
	}
	input := ImageGenerationInput{
		Model:               inputModel,
		Prompt:              generationPromptForToolMode(job.Request),
		NegativePrompt:      job.Request.NegativePrompt,
		AspectRatio:         job.Request.AspectRatio,
		Size:                job.Request.Size,
		Quality:             job.Request.Quality,
		StylePreset:         job.Request.StylePreset,
		ToolMode:            job.Request.ToolMode,
		StyleStrength:       *job.Request.StyleStrength,
		ReferenceWeight:     *job.Request.ReferenceWeight,
		Seed:                job.Request.Seed,
		VariationMode:       job.Request.VariationMode,
		VariationPrompt:     job.Request.VariationPrompt,
		ReferenceIntent:     job.Request.ReferenceIntent,
		ProviderBaseURL:     providerBaseURL,
		ProviderAPIKey:      providerAPIKey,
		ProviderAPIEndpoint: providerAPIEndpoint,
		SourceImage:         sourceImage,
		MaskImage:           maskImage,
		MaskRegions:         maskRegionsFromToolOptions(job.Request.ToolOptions),
		ReferenceImages:     referenceImages,
		IdempotencyKey:      options.IdempotencyKey,
		ExternalReservation: options.BillingMode != generationBillingDirect,
	}
	if compositionPlan != nil {
		input.CompositionPlan = compositionPlan
		input.BackgroundReferenceIndex = compositionPlan.BackgroundReferenceIndex
	} else if job.Request.BackgroundReferenceIndex != nil && job.Request.ReferenceIntent != GenerationReferenceIntentCharacter {
		input.BackgroundReferenceIndex = job.Request.BackgroundReferenceIndex
	}
	providerRequestMetadata := map[string]any{
		"model":                  input.Model,
		"tool_mode":              input.ToolMode,
		"quality":                input.Quality,
		"size":                   input.Size,
		"aspect_ratio":           input.AspectRatio,
		"reference_intent":       input.ReferenceIntent,
		"reference_image_count":  len(input.ReferenceImages),
		"source_image_present":   input.SourceImage != nil,
		"provider_api_endpoint":  input.ProviderAPIEndpoint,
		"provider_base_url_host": providerHostForEvent(input.ProviderBaseURL),
	}
	for key, value := range imageReferenceTransportMetadata(input) {
		providerRequestMetadata[key] = value
	}
	a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "provider_request_start", "开始请求图片供应商", providerRequestMetadata)
	var result ImageGenerationResult
	var providerErr *ProviderError
	var finalModel *ModelConfig
	var finalCandidate *modelCenterCandidate
	if len(job.ModelCenterCandidates) > 0 {
		result, providerErr, finalCandidate, err = a.generateImageWithModelCenterFailover(ctx, requestTimeout, record, job, input)
	} else {
		result, providerErr, finalModel, err = a.generateImageWithFailover(ctx, requestTimeout, record, job, input)
	}
	record.LatencyMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		return nil, nil, err
	}

	if providerErr != nil {
		if finalCandidate != nil {
			applyGenerationRecordModelCenter(record, finalCandidate)
			job.ModelCenterModel = &finalCandidate.Model
			job.ModelCenterChannel = &finalCandidate.Channel
		} else if finalModel != nil {
			applyGenerationRecordModel(record, job.Settings, finalModel)
			job.ModelConfig = finalModel
		}
		record.ProviderRequestID = providerErr.ProviderRequestID
		record.ProviderHTTPStatus = providerErr.HTTPStatus
		record.ProviderErrorCode = strings.TrimSpace(providerErr.Code)
		record.ProviderErrorMessage = strings.TrimSpace(providerErr.Message)
		record.ProviderFailureStage = strings.TrimSpace(providerErr.FailureStage)
		record.ProviderAttemptCount = providerErr.AttemptCount
		code, message, _ := publicProviderFailure(providerErr)
		a.logGenerationEvent(record.ID, generationEventLevelError, fallbackString(record.ProviderFailureStage, record.Stage), "provider_request_failed", "供应商图片生成请求失败", map[string]any{
			"provider_http_status":     record.ProviderHTTPStatus,
			"provider_error_code":      record.ProviderErrorCode,
			"provider_error_message":   record.ProviderErrorMessage,
			"provider_failure_stage":   record.ProviderFailureStage,
			"provider_attempt_count":   record.ProviderAttemptCount,
			"provider_request_id":      record.ProviderRequestID,
			"public_error_code":        code,
			"latency_ms":               record.LatencyMS,
			"provider_policy_rejected": code == "provider_policy_rejected",
		})
		if !options.DeferProviderFailure {
			a.failGenerationRecord(record, code, message)
		}
		return nil, providerErr, nil
	}
	if finalCandidate != nil {
		applyGenerationRecordModelCenter(record, finalCandidate)
		job.ModelCenterModel = &finalCandidate.Model
		job.ModelCenterChannel = &finalCandidate.Channel
	} else if finalModel != nil {
		applyGenerationRecordModel(record, job.Settings, finalModel)
		job.ModelConfig = finalModel
	}
	a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "provider_request_succeeded", "供应商已返回图片结果", map[string]any{
		"provider_request_id":    result.ProviderRequestID,
		"provider_attempt_count": result.ProviderAttemptCount,
		"mime_type":              result.MIMEType,
		"latency_ms":             record.LatencyMS,
	})
	if strings.TrimSpace(result.FilePath) != "" {
		_ = a.db.Model(&ImageGenerationJob{}).Where("generation_record_id = ?", record.ID).Updates(map[string]any{"spool_path": result.FilePath, "stage": "persisting"}).Error
		_ = a.db.Model(&GenerationRecord{}).Where("id = ?", record.ID).Update("mime_type", result.MIMEType).Error
		record.MIMEType = result.MIMEType
	}

	if err := a.ensureGenerationRecordStillActive(record); err != nil {
		return nil, nil, err
	}

	record.Status = GenerationStatusRunning
	record.Stage = GenerationStagePersistingResult
	record.ProviderRequestID = result.ProviderRequestID
	record.ProviderHTTPStatus = 0
	record.ProviderErrorCode = ""
	record.ProviderErrorMessage = ""
	record.ProviderFailureStage = ""
	record.ProviderAttemptCount = result.ProviderAttemptCount
	if err := a.db.Save(record).Error; err != nil {
		return nil, nil, err
	}
	a.logGenerationEvent(record.ID, generationEventLevelInfo, record.Stage, "result_persist_start", "开始保存生成结果", map[string]any{
		"provider_request_id": record.ProviderRequestID,
	})

	resultBase64Image := result.Base64Image
	resultMIMEType := result.MIMEType
	resultFilePath := strings.TrimSpace(result.FilePath)
	needsInMemoryResult := job.Request.ToolMode == GenerationToolModeExpand && expandOriginalSource != nil || options.TransformResult != nil
	if needsInMemoryResult && resultBase64Image == "" && resultFilePath != "" {
		content, readErr := os.ReadFile(resultFilePath)
		if readErr != nil {
			a.failGenerationRecord(record, "generation_spool_read_failed", "生成结果临时文件读取失败")
			return nil, nil, readErr
		}
		resultBase64Image = base64.StdEncoding.EncodeToString(content)
		result.Base64Image = resultBase64Image
	}
	if job.Request.ToolMode == GenerationToolModeExpand && expandOriginalSource != nil {
		resultBase64Image, resultMIMEType, err = preserveExpandedResultOriginalArea(result, *expandOriginalSource, job.Request.ToolOptions)
		if err != nil {
			a.failGenerationRecord(record, "expand_result_preserve_failed", "扩图结果保护失败")
			return nil, nil, err
		}
	}
	if options.TransformResult != nil {
		resultBase64Image, resultMIMEType, err = options.TransformResult(resultBase64Image, resultMIMEType)
		if err != nil {
			a.failGenerationRecord(record, "result_transform_failed", "生成结果后处理失败")
			return nil, nil, err
		}
	}

	if err := ctx.Err(); err != nil {
		a.failGenerationRecord(record, imageGenerationCancelledErrorCode, imageGenerationCancelledMessage)
		return nil, nil, err
	}
	resultStore, err := a.assetStoreForScope(options.ResultStorageScope)
	if err != nil {
		return nil, nil, err
	}
	var assetKey, mimeType string
	if resultFilePath != "" && !needsInMemoryResult {
		file, openErr := os.Open(resultFilePath)
		if openErr != nil {
			err = openErr
		} else {
			assetKey, mimeType, err = resultStore.SaveStream(file, resultMIMEType)
			_ = file.Close()
		}
	} else {
		assetKey, mimeType, err = resultStore.SaveBase64(resultBase64Image, resultMIMEType)
	}
	if err != nil {
		if options.BillingMode != generationBillingQueueReservation || resultFilePath == "" {
			a.failGenerationRecord(record, "asset_store_failed", "作品保存失败")
		}
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = resultStore.Delete(assetKey)
		if options.ResultStorageScope == StorageScopeCommercePrivate && a.commerceAssets != nil && options.CommerceProjectID != 0 {
			_, _ = a.commerceAssets.ScheduleOrphanCleanup(context.Background(), ecommerce.OrphanCleanupInput{UserID: job.User.ID, ProjectID: options.CommerceProjectID, StorageScope: StorageScopeCommercePrivate, ObjectKey: assetKey, Reason: "generation_canceled", DeleteAfter: time.Now()})
		}
		return nil, nil, err
	}
	if options.ResultStorageScope == StorageScopeCommercePrivate && a.commerceAssets != nil {
		if err := a.commerceAssets.EnsureObjectGuard(ctx, job.User.ID, StorageScopeCommercePrivate, assetKey); err != nil {
			_ = resultStore.Delete(assetKey)
			return nil, nil, err
		}
	}
	if options.AfterObjectGuard != nil {
		options.AfterObjectGuard()
	}
	if err := ctx.Err(); err != nil {
		a.cleanupCanceledGenerationObject(options, job.User.ID, assetKey, resultStore)
		return nil, nil, err
	}

	availableCredits := 0
	var persistedWork Work
	if err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		work := Work{
			UserID:             job.User.ID,
			GenerationRecordID: record.ID,
			Prompt:             job.Request.Prompt,
			AspectRatio:        job.Request.AspectRatio,
			BatchID:            job.Request.BatchID,
			BatchIndex:         job.Request.BatchIndex,
			BatchTotal:         job.Request.BatchTotal,
			VariationMode:      job.Request.VariationMode,
			VariationPrompt:    job.Request.VariationPrompt,
			Category:           fallbackString(options.ResultWorkCategory, WorkCategoryImage),
			Model:              record.Model,
			Status:             GenerationStatusSucceeded,
			Visibility:         WorkVisibilityPrivate,
			AssetKey:           assetKey,
			MIMEType:           mimeType,
			ProviderRequestID:  result.ProviderRequestID,
			StorageScope:       options.ResultStorageScope,
		}
		if err := tx.Create(&work).Error; err != nil {
			return err
		}
		if publicURL := resultStore.PublicURL(assetKey); publicURL != "" {
			work.PreviewURL = publicURL
			work.DownloadURL = publicURL
		} else {
			work.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
			work.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
		}
		if err := tx.Save(&work).Error; err != nil {
			return err
		}

		creditsCost := generationRecordCreditCost(*record)
		remainingCredits := 0
		if options.BillingMode == generationBillingDirect {
			var err error
			remainingCredits, err = deductGenerationCredits(tx, job.User.ID, creditsCost)
			if err != nil {
				return err
			}
			availableCredits = remainingCredits
			creditTransaction := CreditTransaction{
				UserID:       job.User.ID,
				Type:         CreditTransactionTypeGenerationCharge,
				Amount:       -creditsCost,
				BalanceAfter: remainingCredits,
				Reason:       "图片生成扣点",
				RelatedType:  "generation",
				RelatedID:    record.ID,
			}
			if err := tx.Create(&creditTransaction).Error; err != nil {
				return err
			}
		} else if options.BillingMode == generationBillingQueueReservation {
			var err error
			availableCredits, err = settleQueuedGenerationCredits(tx, job.User.ID, record.ID, creditsCost)
			if err != nil {
				return err
			}
		}

		record.WorkID = &work.ID
		record.Status = GenerationStatusSucceeded
		record.Stage = GenerationStageSucceeded
		record.AssetKey = assetKey
		record.PreviewURL = work.PreviewURL
		record.DownloadURL = work.DownloadURL
		record.MIMEType = mimeType
		record.CreditsCost = creditsCost
		record.CreditsDeducted = options.BillingMode == generationBillingDirect || options.BillingMode == generationBillingQueueReservation
		record.StorageScope = options.ResultStorageScope
		if err := tx.Save(record).Error; err != nil {
			return err
		}
		if options.BillingMode == generationBillingExternalReservation {
			traceID := commerceGenerationTraceID(options.IdempotencyKey, record.ID)
			mark := AIContentMark{GenerationRecordID: record.ID, UserID: job.User.ID, AssetKey: assetKey, VisibleLabel: "AI生成", TraceID: traceID, Model: record.Model, ProviderRequestID: result.ProviderRequestID}
			if err := tx.Create(&mark).Error; err != nil {
				return err
			}
		}

		persistedWork = work
		return nil
	}); err != nil {
		if options.BillingMode == generationBillingQueueReservation {
			_ = resultStore.Delete(assetKey)
			record.WorkID = nil
			record.AssetKey, record.PreviewURL, record.DownloadURL, record.MIMEType = "", "", "", ""
			record.CreditsDeducted = false
		}
		if options.BillingMode == generationBillingExternalReservation && options.ResultStorageScope == StorageScopeCommercePrivate {
			a.cleanupCanceledGenerationObject(options, job.User.ID, assetKey, resultStore)
			record.WorkID = nil
			record.AssetKey, record.PreviewURL, record.DownloadURL, record.MIMEType = "", "", "", ""
			record.CreditsDeducted = false
		}
		if errors.Is(err, errCreditsInsufficient) {
			a.failGenerationRecord(record, "credits_insufficient", "点数不足，请先充值")
			return nil, nil, errCreditsInsufficient
		}
		if options.BillingMode != generationBillingQueueReservation || resultFilePath == "" {
			a.failGenerationRecord(record, "generation_persist_failed", "作品保存失败")
		}
		return nil, nil, err
	}
	if resultFilePath != "" {
		_ = os.Remove(resultFilePath)
		_ = a.db.Model(&ImageGenerationJob{}).Where("generation_record_id = ?", record.ID).Update("spool_path", "").Error
	}

	record.AssetKey = assetKey
	record.PreviewURL = persistedWork.PreviewURL
	record.DownloadURL = persistedWork.DownloadURL
	record.MIMEType = mimeType
	a.logGenerationEvent(record.ID, generationEventLevelInfo, GenerationStageSucceeded, "generation_succeeded", "生成任务已完成", map[string]any{
		"work_id":             optionalUintForEvent(record.WorkID),
		"asset_key_present":   record.AssetKey != "",
		"public_url_present":  record.PreviewURL != "" && !strings.HasPrefix(record.PreviewURL, "/api/"),
		"mime_type":           record.MIMEType,
		"credits_deducted":    record.CreditsDeducted,
		"available_credits":   availableCredits,
		"provider_request_id": record.ProviderRequestID,
		"latency_ms":          record.LatencyMS,
	})

	return &generationTaskResult{
		Record:           *record,
		AvailableCredits: availableCredits,
	}, nil, nil
}

func (a *App) cleanupCanceledGenerationObject(options generationExecutionOptions, userID uint, assetKey string, store AssetStore) {
	if store != nil {
		_ = store.Delete(assetKey)
	}
	if options.ResultStorageScope == StorageScopeCommercePrivate && a.commerceAssets != nil && options.CommerceProjectID != 0 {
		_, _ = a.commerceAssets.ScheduleOrphanCleanup(context.Background(), ecommerce.OrphanCleanupInput{UserID: userID, ProjectID: options.CommerceProjectID, StorageScope: StorageScopeCommercePrivate, ObjectKey: assetKey, Reason: "generation_canceled", DeleteAfter: time.Now()})
	}
}

func (a *App) runGenerationTask(record *GenerationRecord, job *generationJob, slotKey string) {
	if slotKey != "" {
		defer a.imageGenLimiter.Release(slotKey)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			if record.LatencyMS <= 0 && !record.CreatedAt.IsZero() {
				record.LatencyMS = time.Since(record.CreatedAt).Milliseconds()
			}
			a.failGenerationRecord(record, "generation_task_panic", "生成任务异常中断，请重新生成")
		}
	}()

	if _, providerErr, err := a.executeGenerationRecord(record, job); err != nil {
		if strings.TrimSpace(record.ErrorCode) == "" {
			a.failGenerationRecord(record, "generation_failed", "生成任务失败")
		}
		return
	} else if providerErr != nil {
		return
	}
}

func deductGenerationCredits(tx *gorm.DB, userID uint, amount int) (int, error) {
	if amount <= 0 {
		var balance CreditBalance
		if err := tx.Where("user_id = ?", userID).First(&balance).Error; err != nil {
			return 0, err
		}
		return balance.AvailableCredits, nil
	}

	result := tx.Model(&CreditBalance{}).
		Where("user_id = ? AND available_credits >= ?", userID, amount).
		UpdateColumn("available_credits", gorm.Expr("available_credits - ?", amount))
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, errCreditsInsufficient
	}

	var balance CreditBalance
	if err := tx.Where("user_id = ?", userID).First(&balance).Error; err != nil {
		return 0, err
	}
	return balance.AvailableCredits, nil
}

func (a *App) generateImageWithFailover(ctx context.Context, totalTimeout time.Duration, record *GenerationRecord, job *generationJob, baseInput ImageGenerationInput) (ImageGenerationResult, *ProviderError, *ModelConfig, error) {
	_ = totalTimeout
	candidates := job.ModelCandidates
	if len(candidates) == 0 && job.ModelConfig != nil {
		candidates = []ModelConfig{*job.ModelConfig}
	}
	if len(candidates) == 0 {
		record.ChannelID = 0
		record.ProviderRequestStarted = true
		record.ProviderIdempotencySupported = baseInput.SupportsIdempotencyKey
		result, providerErr, err := a.generateImageAttempt(ctx, record.ID, nil, 1, baseInput, job.Settings)
		return result, providerErr, nil, err
	}

	var lastErr *ProviderError
	var lastModel *ModelConfig
	for index := range candidates {
		if index >= imageQueueMaxExternalAttempts {
			break
		}
		model := candidates[index]
		lastModel = &model
		input := imageGenerationInputForModel(baseInput, job.Settings, &model)
		record.ChannelID = 0
		record.ProviderRequestStarted = true
		record.ProviderIdempotencySupported = input.SupportsIdempotencyKey
		result, providerErr, err := a.generateImageAttempt(ctx, record.ID, &model, index+1, input, job.Settings)
		if err != nil {
			return ImageGenerationResult{}, nil, &model, err
		}
		if providerErr == nil {
			result.ProviderAttemptCount = index + 1
			return result, nil, &model, nil
		}
		providerErr.AttemptCount = index + 1
		lastErr = providerErr
		if input.ExternalReservation && queuedProviderFailureWaits(providerErr) {
			return ImageGenerationResult{}, providerErr, &model, nil
		}
		if !commerceProviderFailureAllowsRetry(input, providerErr) {
			providerErr.Code = "provider_result_unknown"
			providerErr.Message = "provider result is unknown and cannot be replayed safely"
			return ImageGenerationResult{}, providerErr, &model, nil
		}
		if !providerErrorTriggersModelFailover(providerErr) {
			return ImageGenerationResult{}, providerErr, &model, nil
		}
	}
	if lastErr == nil {
		lastErr = &ProviderError{Code: "provider_error", Message: "no available provider candidate"}
	}
	return ImageGenerationResult{}, lastErr, lastModel, nil
}

func (a *App) generateImageWithModelCenterFailover(ctx context.Context, totalTimeout time.Duration, record *GenerationRecord, job *generationJob, baseInput ImageGenerationInput) (ImageGenerationResult, *ProviderError, *modelCenterCandidate, error) {
	_ = totalTimeout
	candidates := job.ModelCenterCandidates
	if len(candidates) == 0 {
		record.ChannelID = 0
		record.ProviderRequestStarted = true
		record.ProviderIdempotencySupported = baseInput.SupportsIdempotencyKey
		if candidate := selectedModelCenterCandidate(job); candidate != nil {
			candidates = []modelCenterCandidate{*candidate}
		}
	}
	if len(candidates) == 0 {
		result, providerErr, err := a.generateImageAttempt(ctx, record.ID, nil, 1, baseInput, job.Settings)
		return result, providerErr, nil, err
	}

	var lastErr *ProviderError
	var lastCandidate *modelCenterCandidate
	attemptIndex := 1
	for index := range candidates {
		candidate := candidates[index]
		lastCandidate = &candidate
		input := imageGenerationInputForModelCenterCandidate(baseInput, &candidate)
		for channelAttempt := 1; channelAttempt <= maxModelCenterChannelProviderAttempts; channelAttempt++ {
			if attemptIndex > imageQueueMaxExternalAttempts {
				return ImageGenerationResult{}, lastErr, lastCandidate, nil
			}
			record.ChannelID = candidate.Channel.ID
			record.ProviderRequestStarted = true
			record.ProviderIdempotencySupported = input.SupportsIdempotencyKey
			currentAttemptIndex := attemptIndex
			attemptIndex++
			result, providerErr, err := a.generateImageAttemptForModelCenter(ctx, record.ID, &candidate, currentAttemptIndex, input, job.Settings)
			if err != nil {
				return ImageGenerationResult{}, nil, &candidate, err
			}
			if providerErr == nil {
				result.ProviderAttemptCount = currentAttemptIndex
				return result, nil, &candidate, nil
			}
			providerErr.AttemptCount = currentAttemptIndex
			lastErr = providerErr
			if input.ExternalReservation && queuedProviderFailureWaits(providerErr) {
				return ImageGenerationResult{}, providerErr, &candidate, nil
			}
			if !commerceProviderFailureAllowsRetry(input, providerErr) {
				providerErr.Code = "provider_result_unknown"
				providerErr.Message = "provider result is unknown and cannot be replayed safely"
				return ImageGenerationResult{}, providerErr, &candidate, nil
			}
			if channelAttempt < maxModelCenterChannelProviderAttempts && providerErrorTriggersSameChannelRetry(providerErr) && ctx.Err() == nil {
				continue
			}
			a.markModelCenterChannelFailure(&candidate, providerErr)
			if !providerErrorTriggersModelFailover(providerErr) {
				return ImageGenerationResult{}, providerErr, &candidate, nil
			}
			break
		}
	}
	if lastErr == nil {
		lastErr = &ProviderError{Code: "provider_error", Message: "no available provider channel"}
	}
	return ImageGenerationResult{}, lastErr, lastCandidate, nil
}

func imageGenerationInputForModel(input ImageGenerationInput, settings AppSettings, model *ModelConfig) ImageGenerationInput {
	input.Model = generationRuntimeModel(settings, model)
	input.ProviderBaseURL = modelConfigProviderBaseURL(model)
	input.ProviderAPIKey = modelConfigProviderAPIKey(model)
	input.ProviderAPIEndpoint = modelConfigProviderAPIEndpoint(model)
	return input
}

func imageGenerationInputForModelCenterCandidate(input ImageGenerationInput, candidate *modelCenterCandidate) ImageGenerationInput {
	input.Model = modelCenterRuntimeModel(candidate)
	input.ProviderBaseURL = modelCenterProviderBaseURL(candidate)
	input.ProviderAPIKey = modelCenterProviderAPIKey(candidate)
	input.ProviderAPIEndpoint = modelCenterProviderEndpoint(candidate)
	input.SupportsIdempotencyKey = modelCatalogHasCapability(candidate.Model, "idempotency_key")
	if input.SupportsIdempotencyKey && strings.TrimSpace(input.IdempotencyKey) != "" {
		input.IdempotencyKey = fmt.Sprintf("%s:channel-%d", strings.TrimSpace(input.IdempotencyKey), candidate.Channel.ID)
	} else {
		input.IdempotencyKey = ""
	}
	return input
}

func modelCatalogHasCapability(model ModelCatalog, capability string) bool {
	tags := model.CapabilityTags
	if tags == nil {
		tags = decodeStringList(model.CapabilityTagsJSON)
	}
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), capability) {
			return true
		}
	}
	return false
}

func commerceProviderFailureAllowsRetry(input ImageGenerationInput, providerErr *ProviderError) bool {
	if !input.ExternalReservation || input.SupportsIdempotencyKey || providerErr == nil {
		return true
	}
	if providerErr.RequestNotSent {
		return true
	}
	switch strings.TrimSpace(providerErr.Code) {
	case "provider_timeout", "provider_request_failed":
		return false
	default:
		return true
	}
}

const maxModelCenterChannelProviderAttempts = 2

func generationRequestTimeout(settings AppSettings) time.Duration {
	return generationTimeoutFromSeconds(settings.RequestTimeoutSeconds)
}

func modelCenterImageAttemptTimeout(settings AppSettings, candidate *modelCenterCandidate) time.Duration {
	if candidate != nil && candidate.Provider.DefaultTimeoutSeconds > 0 {
		return generationTimeoutFromSeconds(candidate.Provider.DefaultTimeoutSeconds)
	}
	return generationRequestTimeout(settings)
}

func generationTimeoutFromSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultRequestTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func generationTimeoutSecondsForEvent(timeout time.Duration) int {
	seconds := int(timeout / time.Second)
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds <= 0 && timeout > 0 {
		return 1
	}
	return seconds
}

func generationAttemptTimeoutSecondsForEvent(ctx context.Context, startedAt time.Time, configuredTimeout time.Duration) int {
	if deadline, ok := ctx.Deadline(); ok {
		effectiveTimeout := deadline.Sub(startedAt)
		if effectiveTimeout > 0 && effectiveTimeout < configuredTimeout {
			return generationTimeoutSecondsForEvent(effectiveTimeout)
		}
	}
	return generationTimeoutSecondsForEvent(configuredTimeout)
}

func (a *App) generateImageAttemptForModelCenter(ctx context.Context, generationRecordID uint, candidate *modelCenterCandidate, attemptIndex int, input ImageGenerationInput, settings AppSettings) (ImageGenerationResult, *ProviderError, error) {
	startedAt := time.Now()
	attemptTimeout := modelCenterImageAttemptTimeout(settings, candidate)
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	if err := a.persistGenerationProviderAttemptStarted(attemptCtx, generationRecordID, modelCenterCandidateChannelID(candidate), input.SupportsIdempotencyKey); err != nil {
		return ImageGenerationResult{}, nil, err
	}

	a.logGenerationEvent(generationRecordID, generationEventLevelInfo, GenerationStageRequestingProvider, "model_call_attempt_start", "开始尝试图片渠道", map[string]any{
		"model_id":                modelCenterCandidateModelID(candidate),
		"channel_id":              modelCenterCandidateChannelID(candidate),
		"model_config_id":         modelCenterCandidateLegacyModelConfigID(candidate),
		"attempt_index":           attemptIndex,
		"attempt_timeout_seconds": generationAttemptTimeoutSecondsForEvent(attemptCtx, startedAt, attemptTimeout),
		"model":                   input.Model,
		"provider_api_endpoint":   input.ProviderAPIEndpoint,
		"provider_base_url_host":  providerHostForEvent(input.ProviderBaseURL),
	})

	a.logImageIdempotencyAttempt(generationRecordID, input)
	result, providerErr := a.provider.Generate(attemptCtx, input)
	finishedAt := time.Now()
	latencyMS := finishedAt.Sub(startedAt).Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	if providerErr != nil {
		providerErr.AttemptCount = attemptIndex
		attempt := modelCallAttemptFromProviderError(generationRecordID, nil, attemptIndex, latencyMS, startedAt, finishedAt, providerErr)
		attempt.ModelConfigID = modelCenterCandidateLegacyModelConfigID(candidate)
		attempt.ChannelID = modelCenterCandidateChannelID(candidate)
		if err := a.db.Create(&attempt).Error; err != nil {
			return ImageGenerationResult{}, nil, err
		}
		a.logGenerationEvent(generationRecordID, generationEventLevelError, fallbackString(providerErr.FailureStage, GenerationStageRequestingProvider), "model_call_attempt_failed", "图片渠道尝试失败", map[string]any{
			"model_id":               modelCenterCandidateModelID(candidate),
			"channel_id":             modelCenterCandidateChannelID(candidate),
			"model_config_id":        modelCenterCandidateLegacyModelConfigID(candidate),
			"attempt_index":          attemptIndex,
			"provider_http_status":   providerErr.HTTPStatus,
			"provider_error_code":    strings.TrimSpace(providerErr.Code),
			"provider_error_message": strings.TrimSpace(providerErr.Message),
			"provider_failure_stage": strings.TrimSpace(providerErr.FailureStage),
			"provider_request_id":    strings.TrimSpace(providerErr.ProviderRequestID),
			"latency_ms":             latencyMS,
		})
		return ImageGenerationResult{}, providerErr, nil
	}
	result.ProviderAttemptCount = attemptIndex
	if candidate != nil && candidate.Channel.ID != 0 {
		_ = a.db.Model(&ModelChannel{}).Where("id = ?", candidate.Channel.ID).Updates(map[string]any{
			"consecutive_failure_count": 0, "health_status": ModelChannelHealthHealthy,
			"fail_cooldown_until": nil, "last_error_code": "",
		}).Error
	}
	attempt := ModelCallAttempt{
		GenerationRecordID: generationRecordID,
		ChannelID:          modelCenterCandidateChannelID(candidate),
		ModelConfigID:      modelCenterCandidateLegacyModelConfigID(candidate),
		AttemptIndex:       attemptIndex,
		Status:             ModelCallAttemptStatusSucceeded,
		LatencyMS:          latencyMS,
		ProviderRequestID:  strings.TrimSpace(result.ProviderRequestID),
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
	}
	if err := a.db.Create(&attempt).Error; err != nil {
		return ImageGenerationResult{}, nil, err
	}
	a.logGenerationEvent(generationRecordID, generationEventLevelInfo, GenerationStageRequestingProvider, "model_call_attempt_succeeded", "图片渠道尝试成功", map[string]any{
		"model_id":            modelCenterCandidateModelID(candidate),
		"channel_id":          modelCenterCandidateChannelID(candidate),
		"model_config_id":     modelCenterCandidateLegacyModelConfigID(candidate),
		"attempt_index":       attemptIndex,
		"provider_request_id": strings.TrimSpace(result.ProviderRequestID),
		"latency_ms":          latencyMS,
	})
	return result, nil, nil
}

func (a *App) markModelCenterChannelFailure(candidate *modelCenterCandidate, providerErr *ProviderError) {
	if candidate == nil || candidate.Channel.ID == 0 || providerErr == nil || !providerErrorTriggersModelFailover(providerErr) {
		return
	}
	now := time.Now()
	_ = a.db.Model(&ModelChannel{}).Where("id = ?", candidate.Channel.ID).Updates(map[string]any{
		"consecutive_failure_count": gorm.Expr("consecutive_failure_count + 1"),
		"last_failure_at":           now, "last_error_code": strings.TrimSpace(providerErr.Code),
	}).Error
	var channel ModelChannel
	if err := a.db.Select("id, consecutive_failure_count").First(&channel, candidate.Channel.ID).Error; err == nil && channel.ConsecutiveFailureCount >= 3 {
		cooldownUntil := now.Add(modelFailoverCooldownTTL)
		_ = a.db.Model(&ModelChannel{}).Where("id = ?", candidate.Channel.ID).Updates(map[string]any{
			"health_status": ModelChannelHealthDegraded, "fail_cooldown_until": cooldownUntil,
		}).Error
	}
}

func modelCenterCandidateModelID(candidate *modelCenterCandidate) uint {
	if candidate == nil {
		return 0
	}
	return candidate.Model.ID
}

func modelCenterCandidateChannelID(candidate *modelCenterCandidate) uint {
	if candidate == nil {
		return 0
	}
	return candidate.Channel.ID
}

func modelCenterCandidateLegacyModelConfigID(candidate *modelCenterCandidate) uint {
	if candidate == nil {
		return 0
	}
	return candidate.Channel.LegacyModelConfigID
}

func (a *App) generateImageAttempt(ctx context.Context, generationRecordID uint, model *ModelConfig, attemptIndex int, input ImageGenerationInput, settings AppSettings) (ImageGenerationResult, *ProviderError, error) {
	startedAt := time.Now()
	attemptTimeout := generationRequestTimeout(settings)
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	if err := a.persistGenerationProviderAttemptStarted(attemptCtx, generationRecordID, 0, input.SupportsIdempotencyKey); err != nil {
		return ImageGenerationResult{}, nil, err
	}

	a.logGenerationEvent(generationRecordID, generationEventLevelInfo, GenerationStageRequestingProvider, "model_call_attempt_start", "开始尝试图片模型", map[string]any{
		"model_config_id":         modelConfigIDValue(model),
		"attempt_index":           attemptIndex,
		"attempt_timeout_seconds": generationAttemptTimeoutSecondsForEvent(attemptCtx, startedAt, attemptTimeout),
		"model":                   input.Model,
		"provider_api_endpoint":   input.ProviderAPIEndpoint,
		"provider_base_url_host":  providerHostForEvent(input.ProviderBaseURL),
	})

	result, providerErr := a.provider.Generate(attemptCtx, input)
	finishedAt := time.Now()
	latencyMS := finishedAt.Sub(startedAt).Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	if providerErr != nil {
		providerErr.AttemptCount = attemptIndex
		attempt := modelCallAttemptFromProviderError(generationRecordID, model, attemptIndex, latencyMS, startedAt, finishedAt, providerErr)
		if err := a.db.Create(&attempt).Error; err != nil {
			return ImageGenerationResult{}, nil, err
		}
		a.logGenerationEvent(generationRecordID, generationEventLevelError, fallbackString(providerErr.FailureStage, GenerationStageRequestingProvider), "model_call_attempt_failed", "图片模型尝试失败", map[string]any{
			"model_config_id":        modelConfigIDValue(model),
			"attempt_index":          attemptIndex,
			"provider_http_status":   providerErr.HTTPStatus,
			"provider_error_code":    strings.TrimSpace(providerErr.Code),
			"provider_error_message": strings.TrimSpace(providerErr.Message),
			"provider_failure_stage": strings.TrimSpace(providerErr.FailureStage),
			"provider_request_id":    strings.TrimSpace(providerErr.ProviderRequestID),
			"latency_ms":             latencyMS,
		})
		return ImageGenerationResult{}, providerErr, nil
	}
	result.ProviderAttemptCount = attemptIndex
	attempt := ModelCallAttempt{
		GenerationRecordID: generationRecordID,
		ModelConfigID:      modelConfigIDValue(model),
		AttemptIndex:       attemptIndex,
		Status:             ModelCallAttemptStatusSucceeded,
		LatencyMS:          latencyMS,
		ProviderRequestID:  strings.TrimSpace(result.ProviderRequestID),
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
	}
	if err := a.db.Create(&attempt).Error; err != nil {
		return ImageGenerationResult{}, nil, err
	}
	a.logGenerationEvent(generationRecordID, generationEventLevelInfo, GenerationStageRequestingProvider, "model_call_attempt_succeeded", "图片模型尝试成功", map[string]any{
		"model_config_id":     modelConfigIDValue(model),
		"attempt_index":       attemptIndex,
		"provider_request_id": strings.TrimSpace(result.ProviderRequestID),
		"latency_ms":          latencyMS,
	})
	return result, nil, nil
}

func (a *App) persistGenerationProviderAttemptStarted(ctx context.Context, recordID, channelID uint, idempotencySupported bool) error {
	if recordID == 0 {
		return nil
	}
	return a.db.WithContext(ctx).Model(&GenerationRecord{}).Where("id = ?", recordID).Updates(map[string]any{
		"channel_id": channelID, "provider_request_started": true, "provider_idempotency_supported": idempotencySupported,
	}).Error
}

func (a *App) logImageIdempotencyAttempt(recordID uint, input ImageGenerationInput) {
	if !input.SupportsIdempotencyKey || strings.TrimSpace(input.IdempotencyKey) == "" {
		return
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(input.IdempotencyKey)))
	a.logGenerationEvent(recordID, generationEventLevelInfo, GenerationStageRequestingProvider, "provider_idempotency_key_applied", "Provider 幂等键已应用", map[string]any{
		"idempotency_key_hash_prefix": hex.EncodeToString(sum[:])[:12],
	})
}

func modelCallAttemptFromProviderError(generationRecordID uint, model *ModelConfig, attemptIndex int, latencyMS int64, startedAt, finishedAt time.Time, providerErr *ProviderError) ModelCallAttempt {
	attempt := ModelCallAttempt{
		GenerationRecordID: generationRecordID,
		ModelConfigID:      modelConfigIDValue(model),
		AttemptIndex:       attemptIndex,
		Status:             ModelCallAttemptStatusFailed,
		LatencyMS:          latencyMS,
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
	}
	if providerErr != nil {
		attempt.HTTPStatus = providerErr.HTTPStatus
		attempt.ErrorCode = strings.TrimSpace(providerErr.Code)
		attempt.ErrorMessage = strings.TrimSpace(providerErr.Message)
		attempt.FailureStage = strings.TrimSpace(providerErr.FailureStage)
		attempt.ProviderRequestID = strings.TrimSpace(providerErr.ProviderRequestID)
	}
	return attempt
}

func providerErrorTriggersModelFailover(err *ProviderError) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(err.FailureStage) == providerFailureStageProviderAssetFetch {
		return false
	}
	if isProviderPolicyRejection(err) {
		return false
	}
	code := strings.TrimSpace(err.Code)
	if providerModelUnavailableErrorCode(code) {
		return true
	}
	switch code {
	case "provider_timeout", "provider_request_failed", "provider_decode_failed", "provider_empty_image",
		"provider_rate_limited", "provider_unavailable", "provider_auth_failed", "provider_request_build_failed":
		return true
	}
	if status, ok := providerHTTPStatusFromErrorCode(code); ok {
		if status == http.StatusBadRequest {
			return false
		}
		return providerHTTPStatusTriggersModelFailover(status)
	}
	if err.HTTPStatus == http.StatusBadRequest {
		return providerModelUnavailableErrorCode(code)
	}
	return providerHTTPStatusTriggersModelFailover(err.HTTPStatus)
}

func providerErrorTriggersSameChannelRetry(err *ProviderError) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(err.FailureStage) == providerFailureStageProviderAssetFetch {
		return false
	}
	if isProviderPolicyRejection(err) {
		return false
	}
	code := strings.TrimSpace(err.Code)
	if code == "provider_timeout" {
		return true
	}
	if status, ok := providerHTTPStatusFromErrorCode(code); ok {
		return providerHTTPStatusTriggersSameChannelRetry(status)
	}
	return providerHTTPStatusTriggersSameChannelRetry(err.HTTPStatus)
}

func providerHTTPStatusTriggersSameChannelRetry(status int) bool {
	// 429 必须回到持久队列并释放执行租约；明确的网关/服务端瞬时错误保留既有渠道路由语义，
	// 但整个调用链最多受 imageQueueMaxExternalAttempts 限制。
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func providerHTTPStatusTriggersModelFailover(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func providerHTTPStatusFromErrorCode(code string) (int, bool) {
	code = strings.TrimSpace(code)
	if !strings.HasPrefix(code, "provider_http_") {
		return 0, false
	}
	status, err := strconv.Atoi(strings.TrimPrefix(code, "provider_http_"))
	if err != nil {
		return 0, false
	}
	return status, true
}

func providerModelUnavailableErrorCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "invalid_model", "model_not_found", "model_not_available", "model_disabled", "unsupported_model":
		return true
	default:
		return false
	}
}

func isActiveGenerationRecordStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case GenerationStatusQueued, GenerationStatusRunning:
		return true
	default:
		return false
	}
}

func (a *App) ensureGenerationRecordStillActive(record *GenerationRecord) error {
	if a == nil || a.db == nil || record == nil || record.ID == 0 {
		return nil
	}
	var latest GenerationRecord
	if err := a.db.First(&latest, record.ID).Error; err != nil {
		return err
	}
	if isActiveGenerationRecordStatus(latest.Status) {
		return nil
	}
	*record = latest
	return errGenerationRecordAlreadyTerminal
}

func imageGenerationTimeoutLatencyMS(createdAt, now time.Time) int64 {
	if createdAt.IsZero() {
		return int64(imageGenerationHardTimeout / time.Millisecond)
	}
	latencyMS := now.Sub(createdAt).Milliseconds()
	minLatencyMS := int64(imageGenerationHardTimeout / time.Millisecond)
	if latencyMS < minLatencyMS {
		return minLatencyMS
	}
	return latencyMS
}

func imageGenerationCancelLatencyMS(createdAt, now time.Time) int64 {
	if now.IsZero() {
		now = time.Now()
	}
	if createdAt.IsZero() {
		return 0
	}
	latencyMS := now.Sub(createdAt).Milliseconds()
	if latencyMS < 0 {
		return 0
	}
	return latencyMS
}

func (a *App) expireStaleImageGenerations(now time.Time) error {
	if a == nil || a.db == nil {
		return nil
	}
	// 持久化队列启用后，排队、外部调用和持久化分别由队列期限、执行租约与恢复状态机判定。
	// 不能再用 GenerationRecord.created_at 把排队或 worker 中断伪装成 provider_timeout。
	if a.db.Migrator().HasTable(&ImageGenerationJob{}) {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.Add(-imageGenerationHardTimeout)
	var records []GenerationRecord
	err := a.db.
		Select("id, created_at").
		Where("status IN ? AND created_at <= ?", []string{GenerationStatusQueued, GenerationStatusRunning}, cutoff).
		Where("NOT EXISTS (SELECT 1 FROM video_generation_records WHERE video_generation_records.generation_record_id = generation_records.id)").
		Where("NOT EXISTS (SELECT 1 FROM image_generation_jobs WHERE image_generation_jobs.generation_record_id = generation_records.id)").
		Find(&records).Error
	if isMissingDatabaseObjectError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, record := range records {
		latencyMS := imageGenerationTimeoutLatencyMS(record.CreatedAt, now)
		updates := map[string]any{
			"status":           GenerationStatusFailed,
			"stage":            GenerationStageFailed,
			"error_code":       imageGenerationTimeoutErrorCode,
			"error_message":    imageGenerationTimeoutErrorMessage,
			"asset_key":        "",
			"preview_url":      "",
			"download_url":     "",
			"mime_type":        "",
			"credits_deducted": false,
			"latency_ms":       latencyMS,
		}
		result := a.db.Model(&GenerationRecord{}).
			Where("id = ? AND status IN ?", record.ID, []string{GenerationStatusQueued, GenerationStatusRunning}).
			Where("NOT EXISTS (SELECT 1 FROM video_generation_records WHERE video_generation_records.generation_record_id = generation_records.id)").
			Where("NOT EXISTS (SELECT 1 FROM image_generation_jobs WHERE image_generation_jobs.generation_record_id = generation_records.id)").
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		a.logGenerationEvent(record.ID, generationEventLevelError, GenerationStageFailed, "generation_failed", "图片生成任务超时失败", map[string]any{
			"error_code":       imageGenerationTimeoutErrorCode,
			"error_message":    imageGenerationTimeoutErrorMessage,
			"timeout_seconds":  int(imageGenerationHardTimeout / time.Second),
			"latency_ms":       latencyMS,
			"credits_deducted": false,
		})
	}
	return nil
}

func (a *App) startImageGenerationTimeoutCleanupTask() {
	if a == nil || a.cleanupStop == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = a.expireStaleImageGenerations(time.Now())
			case <-a.cleanupStop:
				return
			}
		}
	}()
}

func (a *App) failGenerationRecord(record *GenerationRecord, code, message string) {
	if err := a.ensureGenerationRecordStillActive(record); errors.Is(err, errGenerationRecordAlreadyTerminal) {
		return
	}
	record.Status = GenerationStatusFailed
	record.Stage = GenerationStageFailed
	record.ErrorCode = code
	record.ErrorMessage = message
	record.AssetKey = ""
	record.PreviewURL = ""
	record.DownloadURL = ""
	record.MIMEType = ""
	record.CreditsDeducted = false
	_ = a.db.Save(record).Error
	a.logGenerationEvent(record.ID, generationEventLevelError, record.Stage, "generation_failed", "生成任务失败", map[string]any{
		"error_code":               record.ErrorCode,
		"error_message":            record.ErrorMessage,
		"provider_http_status":     record.ProviderHTTPStatus,
		"provider_error_code":      record.ProviderErrorCode,
		"provider_error_message":   record.ProviderErrorMessage,
		"provider_failure_stage":   record.ProviderFailureStage,
		"provider_attempt_count":   record.ProviderAttemptCount,
		"provider_request_id":      record.ProviderRequestID,
		"credits_deducted":         record.CreditsDeducted,
		"provider_policy_rejected": record.ErrorCode == "provider_policy_rejected",
	})
}

func generationPayload(record GenerationRecord, availableCredits int) gin.H {
	payload := gin.H{
		"generation_id":           record.ID,
		"status":                  normalizeGenerationStatus(record.Status),
		"stage":                   normalizeGenerationStage(record.Status, record.Stage),
		"available_credits":       availableCredits,
		"provider_request_id":     record.ProviderRequestID,
		"batch_id":                record.BatchID,
		"batch_index":             record.BatchIndex,
		"batch_total":             record.BatchTotal,
		"seed":                    record.Seed,
		"variation_mode":          record.VariationMode,
		"variation_prompt":        record.VariationPrompt,
		"credits_cost":            generationRecordCreditCost(record),
		"credits_deducted":        record.CreditsDeducted,
		"queue_position":          record.QueuePosition,
		"queue_wait_ms":           record.QueueWaitMS,
		"execution_attempt_count": record.ExecutionAttemptCount,
		"next_attempt_at":         record.NextAttemptAt,
		"parameters":              generationParametersPayload(record),
	}

	if record.WorkID != nil {
		payload["work_id"] = *record.WorkID
	}

	if payload["status"] == GenerationStatusSucceeded {
		payload["preview_url"] = record.PreviewURL
		payload["download_url"] = record.DownloadURL
		payload["mime_type"] = fallbackString(record.MIMEType, "image/png")
		payload["latency_ms"] = record.LatencyMS
		return payload
	}

	if payload["status"] == GenerationStatusFailed {
		code := fallbackString(record.ErrorCode, "generation_failed")
		payload["error"] = gin.H{
			"code":      code,
			"message":   fallbackString(record.ErrorMessage, "生成失败，请稍后再试"),
			"retryable": isRetryableGenerationErrorCode(code),
		}
	}

	return payload
}

func isRetryableGenerationErrorCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "provider_timeout", "provider_request_failed", "provider_decode_failed", "provider_rate_limited", "provider_unavailable", "provider_asset_fetch_failed", "provider_empty_image", "user_cancelled":
		return true
	default:
		return false
	}
}

func generationParametersPayload(record GenerationRecord) gin.H {
	parameters := gin.H{
		// prompt / aspect_ratio 必须回传：前端"重新生成"会把 parameters
		// 原样作为创建请求体重放，缺失会被 prompt_required 拒绝。
		"prompt":           record.Prompt,
		"aspect_ratio":     record.AspectRatio,
		"negative_prompt":  record.NegativePrompt,
		"quality":          fallbackString(record.Quality, GenerationQualityMedium),
		"style_preset":     record.StylePreset,
		"tool_mode":        fallbackString(record.ToolMode, GenerationToolModeGenerate),
		"style_strength":   record.StyleStrength,
		"reference_weight": record.ReferenceWeight,
		"seed":             record.Seed,
		"variation_mode":   record.VariationMode,
		"variation_prompt": record.VariationPrompt,
		"num":              maxInt(record.BatchTotal, 1),
	}
	if options := decodeGenerationToolOptions(record.ToolOptionsJSON); len(options) > 0 {
		parameters["tool_options"] = options
	}
	if record.SourceWorkID != nil {
		parameters["source_work_id"] = *record.SourceWorkID
	}
	if record.MaskAssetID != nil {
		parameters["mask_asset_id"] = *record.MaskAssetID
	}
	if strings.TrimSpace(record.EditInstruction) != "" {
		parameters["edit_instruction"] = record.EditInstruction
	}
	if len(record.ReferenceAssetIDs) > 0 {
		parameters["reference_asset_ids"] = record.ReferenceAssetIDs
	}
	return parameters
}

func (a *App) handleGenerateImageLegacy(c *gin.Context) {
	job, ok := a.prepareGenerationJob(c)
	if !ok {
		return
	}

	slotKey, ok := a.acquireImageGenerationSlot(c, job.User.ID)
	if !ok {
		return
	}
	defer a.imageGenLimiter.Release(slotKey)

	record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "record_create_failed", "记录创建失败")
		return
	}

	taskResult, providerErr, err := a.executeGenerationRecord(&record, job)
	if err != nil {
		if errors.Is(err, errGenerationRecordAlreadyTerminal) {
			balance, _ := a.lookupBalance(job.User.ID)
			writeJSON(c, http.StatusOK, generationPayload(record, balance.AvailableCredits))
			return
		}
		if errors.Is(err, errCreditsInsufficient) {
			estimate, estimateErr := a.buildCreditEstimate(job.User.ID, generationRecordCreditCost(record))
			if estimateErr != nil {
				writeError(c, http.StatusConflict, "credits_insufficient", "点数不足，请先充值")
				return
			}
			writeCreditsInsufficientError(c, estimate)
			return
		}
		writeError(c, http.StatusInternalServerError, "generation_execute_failed", "生成任务执行失败")
		return
	}
	if providerErr != nil {
		status, code, message := mapProviderError(providerErr)
		writeError(c, status, code, message)
		return
	}

	writeJSON(c, http.StatusOK, generationPayload(taskResult.Record, taskResult.AvailableCredits))
}

func (a *App) handleEstimateImageGeneration(c *gin.Context) {
	job, ok := a.prepareGenerationJobInputs(c)
	if !ok {
		return
	}
	estimate, err := a.buildCreditEstimate(job.User.ID, generationJobRequiredCredits(job))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, estimate)
}

func (a *App) handleCreateAsyncGenerationLegacy(c *gin.Context) {
	job, ok := a.prepareGenerationJob(c)
	if !ok {
		return
	}

	slotKey, ok := a.acquireImageGenerationSlot(c, job.User.ID)
	if !ok {
		return
	}

	record, err := a.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
	if err != nil {
		a.imageGenLimiter.Release(slotKey)
		writeError(c, http.StatusInternalServerError, "record_create_failed", "记录创建失败")
		return
	}

	go a.runGenerationTask(&record, job, slotKey)

	balance, _ := a.lookupBalance(job.User.ID)
	writeJSON(c, http.StatusAccepted, gin.H{
		"generation_id":     record.ID,
		"status":            record.Status,
		"stage":             record.Stage,
		"batch_id":          record.BatchID,
		"batch_index":       record.BatchIndex,
		"batch_total":       record.BatchTotal,
		"seed":              record.Seed,
		"variation_mode":    record.VariationMode,
		"variation_prompt":  record.VariationPrompt,
		"available_credits": balance.AvailableCredits,
		"credits_cost":      generationRecordCreditCost(record),
		"credits_deducted":  record.CreditsDeducted,
	})
}

func (a *App) handleCancelImageGeneration(c *gin.Context) {
	user := currentUser(c)
	rawID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || rawID == 0 {
		writeError(c, http.StatusNotFound, "generation_not_found", "生成任务不存在")
		return
	}
	recordID := uint(rawID)

	if err := a.expireStaleImageGenerations(time.Now()); err != nil {
		writeError(c, http.StatusInternalServerError, "generation_timeout_cleanup_failed", "生成任务状态刷新失败")
		return
	}

	var record GenerationRecord
	if err := a.db.
		Where("id = ? AND user_id = ?", recordID, user.ID).
		Where("NOT EXISTS (SELECT 1 FROM video_generation_records WHERE video_generation_records.generation_record_id = generation_records.id)").
		First(&record).Error; err != nil {
		writeError(c, http.StatusNotFound, "generation_not_found", "生成任务不存在")
		return
	}

	if isActiveGenerationRecordStatus(record.Status) {
		if queueJob, ok := a.generationQueueProjection(record.ID); ok {
			now := time.Now().UTC()
			if err := a.db.Model(&ImageGenerationJob{}).Where("id = ?", queueJob.ID).Update("cancel_requested_at", now).Error; err != nil {
				writeError(c, http.StatusInternalServerError, "generation_cancel_failed", "取消生成失败")
				return
			}
			if err := a.db.First(&queueJob, queueJob.ID).Error; err != nil {
				writeError(c, http.StatusInternalServerError, "generation_cancel_failed", "取消生成失败")
				return
			}
			var cancelErr error
			for attempt := 0; attempt < 3; attempt++ {
				cancelErr = a.failClaimedGenerationJob(queueJob, queueJob.LeaseToken, imageGenerationCancelledErrorCode, imageGenerationCancelledMessage)
				if cancelErr == nil || !strings.Contains(strings.ToLower(cancelErr.Error()), "database is locked") {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if cancelErr != nil && !errors.Is(cancelErr, errGenerationQueueLeaseLost) {
				writeError(c, http.StatusInternalServerError, "generation_cancel_failed", "取消生成失败")
				return
			}
			a.cancelImageGenerationContext(record.ID)
			_ = a.db.First(&record, record.ID).Error
		}
	}

	if isActiveGenerationRecordStatus(record.Status) {
		now := time.Now()
		latencyMS := imageGenerationCancelLatencyMS(record.CreatedAt, now)
		updates := map[string]any{
			"status":           GenerationStatusFailed,
			"stage":            GenerationStageFailed,
			"error_code":       imageGenerationCancelledErrorCode,
			"error_message":    imageGenerationCancelledMessage,
			"asset_key":        "",
			"preview_url":      "",
			"download_url":     "",
			"mime_type":        "",
			"credits_deducted": false,
			"latency_ms":       latencyMS,
		}
		result := a.db.Model(&GenerationRecord{}).
			Where("id = ? AND user_id = ? AND status IN ?", record.ID, user.ID, []string{GenerationStatusQueued, GenerationStatusRunning}).
			Where("NOT EXISTS (SELECT 1 FROM video_generation_records WHERE video_generation_records.generation_record_id = generation_records.id)").
			Updates(updates)
		if result.Error != nil {
			writeError(c, http.StatusInternalServerError, "generation_cancel_failed", "取消生成失败")
			return
		}
		if result.RowsAffected > 0 {
			a.cancelImageGenerationContext(record.ID)
			a.logGenerationEvent(record.ID, generationEventLevelInfo, GenerationStageFailed, "generation_cancelled", "用户取消图片生成任务", map[string]any{
				"error_code":       imageGenerationCancelledErrorCode,
				"error_message":    imageGenerationCancelledMessage,
				"latency_ms":       latencyMS,
				"credits_deducted": false,
			})
		}
		if err := a.db.First(&record, record.ID).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "generation_load_failed", "生成任务读取失败")
			return
		}
	}

	if ids, err := a.generationReferenceAssetIDs(record.ID); err == nil {
		record.ReferenceAssetIDs = ids
	}
	a.applyGenerationRecordPublicURL(&record)
	a.hydrateGenerationQueueProjection(&record)

	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}

	writeJSON(c, http.StatusOK, generationPayload(record, balance.AvailableCredits))
}

func (a *App) handleGetGeneration(c *gin.Context) {
	user := currentUser(c)

	if err := a.expireStaleImageGenerations(time.Now()); err != nil {
		writeError(c, http.StatusInternalServerError, "generation_timeout_cleanup_failed", "生成任务状态刷新失败")
		return
	}

	var record GenerationRecord
	if err := a.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&record).Error; err != nil {
		writeError(c, http.StatusNotFound, "generation_not_found", "生成任务不存在")
		return
	}
	if ids, err := a.generationReferenceAssetIDs(record.ID); err == nil {
		record.ReferenceAssetIDs = ids
	}
	a.applyGenerationRecordPublicURL(&record)
	a.hydrateGenerationQueueProjection(&record)

	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}

	writeJSON(c, http.StatusOK, generationPayload(record, balance.AvailableCredits))
}

func (a *App) generationReferenceAssetIDs(recordID uint) ([]uint, error) {
	if recordID == 0 {
		return nil, nil
	}
	var links []GenerationReferenceAsset
	if err := a.db.Where("generation_record_id = ?", recordID).Order("sort_order asc, id asc").Find(&links).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.ReferenceAssetID)
	}
	return ids, nil
}

func (a *App) applyGenerationRecordPublicURL(record *GenerationRecord) {
	if record == nil {
		return
	}
	if publicURL := a.assetStore.PublicURL(record.AssetKey); publicURL != "" {
		record.PreviewURL = publicURL
		record.DownloadURL = publicURL
	}
}
