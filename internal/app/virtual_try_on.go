package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const virtualTryOnPrivacyModeEphemeral = "ephemeral"

type virtualTryOnRequest struct {
	BodyProfile virtualTryOnBodyProfile `json:"body_profile"`
	Garment     virtualTryOnGarment     `json:"garment"`
	Scene       virtualTryOnScene       `json:"scene"`
	Generation  virtualTryOnGeneration  `json:"generation"`
}

type virtualTryOnBodyProfile struct {
	HeightCM             *float64 `json:"height_cm"`
	WeightKG             *float64 `json:"weight_kg"`
	ShoulderCM           *float64 `json:"shoulder_cm"`
	ChestCM              *float64 `json:"chest_cm"`
	WaistCM              *float64 `json:"waist_cm"`
	HipCM                *float64 `json:"hip_cm"`
	BodyType             string   `json:"body_type"`
	BodyFatLabel         string   `json:"body_fat_label"`
	FitPreference        string   `json:"fit_preference"`
	StylePreference      string   `json:"style_preference"`
	BodyReferenceAssetID uint     `json:"body_reference_asset_id"`
}

type virtualTryOnGarment struct {
	GarmentReferenceAssetID uint   `json:"garment_reference_asset_id"`
	Category                string `json:"category"`
	Size                    string `json:"size"`
	Material                string `json:"material"`
	Color                   string `json:"color"`
	Fit                     string `json:"fit"`
	Details                 string `json:"details"`
}

type virtualTryOnScene struct {
	Category             string `json:"category"`
	SubScene             string `json:"sub_scene"`
	Pose                 string `json:"pose"`
	BackgroundPreference string `json:"background_preference"`
	CustomDescription    string `json:"custom_description"`
}

type virtualTryOnGeneration struct {
	ModelID     uint   `json:"model_id"`
	Quality     string `json:"quality"`
	AspectRatio string `json:"aspect_ratio"`
}

type bodyProfileValidationError struct {
	Field    string   `json:"field"`
	Label    string   `json:"label"`
	Value    *float64 `json:"value"`
	Min      float64  `json:"min"`
	Max      float64  `json:"max"`
	Unit     string   `json:"unit"`
	Required bool     `json:"required"`
}

type bodyMeasurementRule struct {
	field    string
	label    string
	value    *float64
	min      float64
	max      float64
	unit     string
	required bool
}

func (a *App) handleEstimateVirtualTryOn(c *gin.Context) {
	job, ok := a.prepareVirtualTryOnGenerationJobInputs(c)
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

func (a *App) handleCreateAsyncVirtualTryOn(c *gin.Context) {
	job, ok := a.prepareVirtualTryOnGenerationJob(c)
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
		"available_credits": balance.AvailableCredits,
		"credits_cost":      generationRecordCreditCost(record),
		"credits_deducted":  record.CreditsDeducted,
	})
}

func (a *App) prepareVirtualTryOnGenerationJob(c *gin.Context) (*generationJob, bool) {
	job, ok := a.prepareVirtualTryOnGenerationJobInputs(c)
	if !ok {
		return nil, false
	}

	estimate, err := a.buildCreditEstimate(job.User.ID, generationJobRequiredCredits(job))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return nil, false
	}
	if !estimate.Enough {
		writeCreditsInsufficientError(c, estimate)
		return nil, false
	}

	rateKey := clientIP(c.Request) + "|user:" + strconv.FormatUint(uint64(job.User.ID), 10)
	window := time.Duration(job.Settings.RateLimitWindowSeconds) * time.Second
	if !a.rateLimiter.Allow(rateKey, time.Now(), window, job.Settings.RateLimitMaxRequests) {
		writeError(c, http.StatusTooManyRequests, "too_many_requests", "请求过于频繁")
		return nil, false
	}

	return job, true
}

func (a *App) prepareVirtualTryOnGenerationJobInputs(c *gin.Context) (*generationJob, bool) {
	user := currentUser(c)
	var req virtualTryOnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return nil, false
	}
	req = normalizeVirtualTryOnRequest(req)
	if ok := validateVirtualTryOnRequest(c, req); !ok {
		return nil, false
	}

	referenceIDs := []uint{req.Garment.GarmentReferenceAssetID}
	if req.BodyProfile.BodyReferenceAssetID != 0 {
		referenceIDs = append(referenceIDs, req.BodyProfile.BodyReferenceAssetID)
	}
	referenceAssets, ok := a.prepareReferenceAssets(c, user.ID, referenceIDs)
	if !ok {
		return nil, false
	}

	generationReq := generationRequest{
		Prompt:            buildVirtualTryOnPrompt(req),
		AspectRatio:       req.Generation.AspectRatio,
		Quality:           req.Generation.Quality,
		ToolMode:          GenerationToolModeVirtualTryOn,
		ToolOptions:       buildVirtualTryOnToolOptions(req),
		ReferenceAssetIDs: referenceIDs,
		ModelID:           req.Generation.ModelID,
	}
	styleStrength := 65
	referenceWeight := 85
	generationReq.StyleStrength = &styleStrength
	generationReq.ReferenceWeight = &referenceWeight
	size, ok := aspectRatioToSize(generationReq.AspectRatio)
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_aspect_ratio", "不支持的画幅比例")
		return nil, false
	}
	generationReq.Size = size

	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return nil, false
	}
	modelCenterCandidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, generationReq.ModelID)
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

	return &generationJob{
		User:                  *user,
		Settings:              settings,
		ModelConfig:           modelConfig,
		ModelCandidates:       modelCandidates,
		ModelCenterModel:      modelCenterModel,
		ModelCenterChannel:    modelCenterChannel,
		ModelCenterCandidates: modelCenterCandidates,
		Request:               generationReq,
		ReferenceAssets:       referenceAssets,
	}, true
}

func normalizeVirtualTryOnRequest(req virtualTryOnRequest) virtualTryOnRequest {
	req.BodyProfile.BodyType = strings.TrimSpace(req.BodyProfile.BodyType)
	req.BodyProfile.BodyFatLabel = strings.TrimSpace(req.BodyProfile.BodyFatLabel)
	req.BodyProfile.FitPreference = strings.TrimSpace(req.BodyProfile.FitPreference)
	req.BodyProfile.StylePreference = strings.TrimSpace(req.BodyProfile.StylePreference)
	req.Garment.Category = strings.TrimSpace(req.Garment.Category)
	req.Garment.Size = strings.TrimSpace(req.Garment.Size)
	req.Garment.Material = strings.TrimSpace(req.Garment.Material)
	req.Garment.Color = strings.TrimSpace(req.Garment.Color)
	req.Garment.Fit = strings.TrimSpace(req.Garment.Fit)
	req.Garment.Details = strings.TrimSpace(req.Garment.Details)
	req.Scene.Category = normalizeVirtualTryOnSceneCategory(req.Scene.Category)
	req.Scene.SubScene = strings.TrimSpace(req.Scene.SubScene)
	req.Scene.Pose = strings.TrimSpace(req.Scene.Pose)
	req.Scene.BackgroundPreference = strings.TrimSpace(req.Scene.BackgroundPreference)
	req.Scene.CustomDescription = strings.TrimSpace(req.Scene.CustomDescription)
	req.Generation.Quality = normalizeGenerationQuality(req.Generation.Quality)
	req.Generation.AspectRatio = strings.TrimSpace(req.Generation.AspectRatio)
	if req.Generation.AspectRatio == "" {
		req.Generation.AspectRatio = "3:4"
	}
	return req
}

func validateVirtualTryOnRequest(c *gin.Context, req virtualTryOnRequest) bool {
	if req.Garment.GarmentReferenceAssetID == 0 {
		writeError(c, http.StatusBadRequest, "garment_reference_required", "请先上传服装参考图")
		return false
	}
	if validationErrors := validateVirtualTryOnBodyProfile(req.BodyProfile); len(validationErrors) > 0 {
		writeInvalidBodyProfileError(c, validationErrors)
		return false
	}
	if !isValidVirtualTryOnSceneCategory(req.Scene.Category) {
		writeError(c, http.StatusBadRequest, "invalid_virtual_try_on_scene", "不支持的试衣场景")
		return false
	}
	if !isValidGenerationQuality(req.Generation.Quality) {
		writeError(c, http.StatusBadRequest, "invalid_generation_parameter", "不支持的清晰度设置")
		return false
	}
	if _, ok := aspectRatioToSize(req.Generation.AspectRatio); !ok {
		writeError(c, http.StatusBadRequest, "invalid_aspect_ratio", "不支持的画幅比例")
		return false
	}
	return true
}

func validMeasurement(value *float64, minValue, maxValue float64, required bool) bool {
	if value == nil {
		return !required
	}
	return *value >= minValue && *value <= maxValue
}

func validateVirtualTryOnBodyProfile(profile virtualTryOnBodyProfile) []bodyProfileValidationError {
	rules := []bodyMeasurementRule{
		{field: "height_cm", label: "身高", value: profile.HeightCM, min: 80, max: 230, unit: "cm", required: true},
		{field: "weight_kg", label: "体重", value: profile.WeightKG, min: 25, max: 250, unit: "kg", required: true},
		{field: "shoulder_cm", label: "肩宽", value: profile.ShoulderCM, min: 20, max: 80, unit: "cm"},
		{field: "chest_cm", label: "胸围", value: profile.ChestCM, min: 40, max: 180, unit: "cm"},
		{field: "waist_cm", label: "腰围", value: profile.WaistCM, min: 40, max: 180, unit: "cm"},
		{field: "hip_cm", label: "臀围", value: profile.HipCM, min: 40, max: 180, unit: "cm"},
	}
	errors := make([]bodyProfileValidationError, 0)
	for _, rule := range rules {
		if rule.value == nil {
			if rule.required {
				errors = append(errors, bodyProfileValidationError{
					Field:    rule.field,
					Label:    rule.label,
					Value:    nil,
					Min:      rule.min,
					Max:      rule.max,
					Unit:     rule.unit,
					Required: rule.required,
				})
			}
			continue
		}
		if *rule.value < rule.min || *rule.value > rule.max {
			value := *rule.value
			errors = append(errors, bodyProfileValidationError{
				Field:    rule.field,
				Label:    rule.label,
				Value:    &value,
				Min:      rule.min,
				Max:      rule.max,
				Unit:     rule.unit,
				Required: rule.required,
			})
		}
	}
	return errors
}

func writeInvalidBodyProfileError(c *gin.Context, validationErrors []bodyProfileValidationError) {
	const code = "invalid_body_profile"
	const message = "身型参数填写有误，请按提示修改"
	c.Set(requestLogErrorCodeKey, code)
	c.Set(requestLogErrorMessageKey, message)
	c.JSON(http.StatusBadRequest, gin.H{
		"error": gin.H{
			"code":              code,
			"message":           message,
			"validation_errors": validationErrors,
		},
	})
}

func buildVirtualTryOnToolOptions(req virtualTryOnRequest) map[string]any {
	return map[string]any{
		GenerationToolModeVirtualTryOn: map[string]any{
			"privacy_mode": virtualTryOnPrivacyModeEphemeral,
			"body_profile": map[string]any{
				"height_cm":               req.BodyProfile.HeightCM,
				"weight_kg":               req.BodyProfile.WeightKG,
				"shoulder_cm":             req.BodyProfile.ShoulderCM,
				"chest_cm":                req.BodyProfile.ChestCM,
				"waist_cm":                req.BodyProfile.WaistCM,
				"hip_cm":                  req.BodyProfile.HipCM,
				"body_type":               req.BodyProfile.BodyType,
				"body_fat_label":          req.BodyProfile.BodyFatLabel,
				"fit_preference":          req.BodyProfile.FitPreference,
				"style_preference":        req.BodyProfile.StylePreference,
				"body_reference_asset_id": req.BodyProfile.BodyReferenceAssetID,
			},
			"garment": map[string]any{
				"garment_reference_asset_id": req.Garment.GarmentReferenceAssetID,
				"category":                   req.Garment.Category,
				"size":                       req.Garment.Size,
				"material":                   req.Garment.Material,
				"color":                      req.Garment.Color,
				"fit":                        req.Garment.Fit,
				"details":                    req.Garment.Details,
			},
			"scene": map[string]any{
				"category":              req.Scene.Category,
				"category_label":        virtualTryOnSceneCategoryLabel(req.Scene.Category),
				"sub_scene":             req.Scene.SubScene,
				"pose":                  req.Scene.Pose,
				"background_preference": req.Scene.BackgroundPreference,
				"custom_description":    req.Scene.CustomDescription,
			},
			"generation": map[string]any{
				"model_id":     req.Generation.ModelID,
				"quality":      req.Generation.Quality,
				"aspect_ratio": req.Generation.AspectRatio,
			},
		},
	}
}

func buildVirtualTryOnPrompt(req virtualTryOnRequest) string {
	parts := []string{
		"建模试衣：根据第一张服装参考图生成消费者上身效果图；如提供第二张真人全身参考图，请保持人物体态、比例和气质一致。",
		"参考图顺序：图1为服装商品图，图2为真人全身参考图（如有）。",
		"输出要求：只生成一张真实、自然、可用于穿搭判断的试衣图片；服装颜色、材质、版型和关键细节必须来自服装图，不要生成商品库或尺码表。",
		"隐私要求：身体围度和真人参考图仅用于本次生成，不要表现为可识别身份信息或额外档案。",
	}
	parts = append(parts,
		fmt.Sprintf("身型：身高%s cm，体重%s kg，肩宽%s cm，胸围%s cm，腰围%s cm，臀围%s cm，体型%s，体脂/身形%s，穿衣偏好%s，风格偏好%s。",
			formatOptionalMeasurement(req.BodyProfile.HeightCM),
			formatOptionalMeasurement(req.BodyProfile.WeightKG),
			formatOptionalMeasurement(req.BodyProfile.ShoulderCM),
			formatOptionalMeasurement(req.BodyProfile.ChestCM),
			formatOptionalMeasurement(req.BodyProfile.WaistCM),
			formatOptionalMeasurement(req.BodyProfile.HipCM),
			fallbackString(req.BodyProfile.BodyType, "未填写"),
			fallbackString(req.BodyProfile.BodyFatLabel, "未填写"),
			fallbackString(req.BodyProfile.FitPreference, "regular"),
			fallbackString(req.BodyProfile.StylePreference, "未填写"),
		),
		fmt.Sprintf("服装：品类%s，尺码%s，材质%s，颜色%s，版型%s，细节%s。",
			fallbackString(req.Garment.Category, "未填写"),
			fallbackString(req.Garment.Size, "未填写"),
			fallbackString(req.Garment.Material, "未填写"),
			fallbackString(req.Garment.Color, "未填写"),
			fallbackString(req.Garment.Fit, "未填写"),
			fallbackString(req.Garment.Details, "未填写"),
		),
		fmt.Sprintf("场景：%s，子场景%s，姿态%s，背景%s。%s",
			virtualTryOnSceneCategoryLabel(req.Scene.Category),
			fallbackString(virtualTryOnSubSceneLabel(req.Scene.Category, req.Scene.SubScene), fallbackString(req.Scene.SubScene, "未填写")),
			fallbackString(req.Scene.Pose, "standing"),
			fallbackString(req.Scene.BackgroundPreference, "干净自然背景"),
			req.Scene.CustomDescription,
		),
	)
	return strings.Join(nonEmptyStrings(parts), "\n")
}

func formatOptionalMeasurement(value *float64) string {
	if value == nil {
		return "未填写"
	}
	if *value == float64(int(*value)) {
		return fmt.Sprintf("%d", int(*value))
	}
	return fmt.Sprintf("%.1f", *value)
}

func normalizeVirtualTryOnSceneCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "work_business", "business", "work":
		return "work_business"
	case "social_etiquette", "social":
		return "social_etiquette"
	case "sports_outdoor", "sports", "outdoor":
		return "sports_outdoor"
	case "home_private", "home":
		return "home_private"
	case "special_protection", "protection":
		return "special_protection"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func isValidVirtualTryOnSceneCategory(value string) bool {
	switch normalizeVirtualTryOnSceneCategory(value) {
	case "work_business", "social_etiquette", "sports_outdoor", "home_private", "special_protection":
		return true
	default:
		return false
	}
}

func virtualTryOnSceneCategoryLabel(value string) string {
	switch normalizeVirtualTryOnSceneCategory(value) {
	case "work_business":
		return "职场商务"
	case "social_etiquette":
		return "社交礼仪"
	case "sports_outdoor":
		return "运动户外"
	case "home_private":
		return "居家私密"
	case "special_protection":
		return "特殊防护"
	default:
		return strings.TrimSpace(value)
	}
}

func virtualTryOnSubSceneLabel(category, subScene string) string {
	subScene = strings.ToLower(strings.TrimSpace(subScene))
	switch normalizeVirtualTryOnSceneCategory(category) {
	case "work_business":
		return mapVirtualTryOnSubScene(subScene, map[string]string{
			"office":        "办公室",
			"meeting":       "会议汇报",
			"business_trip": "商务出差",
			"interview":     "面试",
		})
	case "social_etiquette":
		return mapVirtualTryOnSubScene(subScene, map[string]string{
			"banquet":  "宴会",
			"date":     "约会",
			"wedding":  "婚礼宾客",
			"ceremony": "典礼",
		})
	case "sports_outdoor":
		return mapVirtualTryOnSubScene(subScene, map[string]string{
			"running": "跑步",
			"hiking":  "徒步",
			"cycling": "骑行",
			"fitness": "健身",
		})
	case "home_private":
		return mapVirtualTryOnSubScene(subScene, map[string]string{
			"lounge":      "居家休闲",
			"sleepwear":   "睡衣",
			"home_office": "居家办公",
		})
	case "special_protection":
		return mapVirtualTryOnSubScene(subScene, map[string]string{
			"rain":     "雨雪防护",
			"sun":      "防晒",
			"workwear": "工装防护",
		})
	default:
		return ""
	}
}

func mapVirtualTryOnSubScene(value string, labels map[string]string) string {
	if label, ok := labels[value]; ok {
		return label
	}
	return ""
}
