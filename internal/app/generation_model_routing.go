package app

// 本文件从 generation.go 拆分：图片生成的模型路由、候选选择与 failover 冷却逻辑。

import (
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (a *App) modelConfigForGeneration(settings AppSettings) (*ModelConfig, error) {
	strategy := normalizeModelRoutingStrategy(settings.ModelRoutingStrategy)
	if !settings.ModelRoutingEnabled || strategy == ModelRoutingStrategyDefault {
		return a.defaultImageModelConfig(settings)
	}

	switch strategy {
	case ModelRoutingStrategySpeedFirst:
		model, err := a.speedFirstImageModelConfig()
		if err != nil {
			return nil, err
		}
		if model != nil {
			return model, nil
		}
	case ModelRoutingStrategyRoundRobin:
		model, err := a.roundRobinImageModelConfig()
		if err != nil {
			return nil, err
		}
		if model != nil {
			return model, nil
		}
	}

	return a.defaultImageModelConfig(settings)
}

func (a *App) modelConfigCandidatesForGeneration(settings AppSettings) ([]ModelConfig, error) {
	strategy := normalizeModelRoutingStrategy(settings.ModelRoutingStrategy)
	var candidates []ModelConfig
	var err error
	if !settings.ModelRoutingEnabled || strategy == ModelRoutingStrategyDefault {
		candidates, err = a.defaultImageModelCandidates(settings)
	} else {
		switch strategy {
		case ModelRoutingStrategySpeedFirst:
			candidates, err = a.speedFirstImageModelCandidates(settings)
		case ModelRoutingStrategyRoundRobin:
			candidates, err = a.roundRobinImageModelCandidates(settings)
		default:
			candidates, err = a.defaultImageModelCandidates(settings)
		}
	}
	if err != nil {
		return nil, err
	}
	candidates, err = a.completeImageModelCandidates(settings, candidates)
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		if err := a.hydrateModelConfig(&candidates[i]); err != nil {
			return nil, err
		}
	}
	if len(candidates) == 0 {
		return candidates, nil
	}
	prioritized, err := a.prioritizeModelFailoverCooldown(candidates, time.Now())
	if err != nil {
		return nil, err
	}
	return prioritized, nil
}

func (a *App) completeImageModelCandidates(settings AppSettings, primary []ModelConfig) ([]ModelConfig, error) {
	candidates := append([]ModelConfig(nil), primary...)
	defaults, err := a.defaultImageModelCandidates(settings)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, defaults...)
	return dedupeModelCandidates(candidates), nil
}

func (a *App) defaultImageModelCandidates(settings AppSettings) ([]ModelConfig, error) {
	candidates := make([]ModelConfig, 0, 4)
	if model, err := a.onlineImageModelConfigByID(settings.DefaultImageModelID); err != nil {
		return nil, err
	} else if model != nil {
		candidates = append(candidates, *model)
	}
	if model, err := a.onlineImageModelConfigByID(settings.FallbackModelID); err != nil {
		return nil, err
	} else if model != nil {
		candidates = append(candidates, *model)
	}
	if model, err := a.onlineImageModelConfigForRuntime(settings.ActiveImageModel); err != nil {
		return nil, err
	} else if model != nil {
		candidates = append(candidates, *model)
	}
	online, err := a.onlineImageRoutingCandidates()
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, online...)
	return dedupeModelCandidates(candidates), nil
}

func (a *App) defaultImageModelConfig(settings AppSettings) (*ModelConfig, error) {
	if model, err := a.imageModelConfigByID(settings.DefaultImageModelID); err != nil || model != nil {
		return model, err
	}
	if model, err := a.imageModelConfigByID(settings.FallbackModelID); err != nil || model != nil {
		return model, err
	}
	return a.modelConfigForRuntime(settings.ActiveImageModel)
}

func (a *App) onlineImageModelConfigByID(id *uint) (*ModelConfig, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	var model ModelConfig
	err := a.db.Where("id = ? AND type = ? AND status = ?", *id, ModelConfigTypeImage, ModelConfigStatusOnline).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := a.hydrateModelConfig(&model); err != nil {
		return nil, err
	}
	return &model, nil
}

func (a *App) imageModelConfigByID(id *uint) (*ModelConfig, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	var model ModelConfig
	err := a.db.Where("id = ? AND type = ?", *id, ModelConfigTypeImage).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := a.hydrateModelConfig(&model); err != nil {
		return nil, err
	}
	return &model, nil
}

func (a *App) onlineImageModelConfigForRuntime(runtimeModel string) (*ModelConfig, error) {
	runtimeModel = strings.TrimSpace(runtimeModel)
	if runtimeModel == "" {
		return nil, nil
	}
	var model ModelConfig
	err := a.db.
		Where("type = ? AND status = ? AND (runtime_model = ? OR name = ?)", ModelConfigTypeImage, ModelConfigStatusOnline, runtimeModel, runtimeModel).
		Order("runtime_model desc, sort_order asc, id asc").
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := a.hydrateModelConfig(&model); err != nil {
		return nil, err
	}
	return &model, nil
}

func (a *App) onlineImageRoutingCandidates() ([]ModelConfig, error) {
	var models []ModelConfig
	err := a.db.
		Where("type = ? AND status = ?", ModelConfigTypeImage, ModelConfigStatusOnline).
		Where("runtime_model <> '' OR api_base_url <> '' OR api_endpoint <> ''").
		Order("sort_order asc, id asc").
		Find(&models).Error
	return models, err
}

func (a *App) speedFirstImageModelConfig() (*ModelConfig, error) {
	candidates, err := a.speedFirstImageModelCandidates(AppSettings{})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return &candidates[0], nil
}

func (a *App) speedFirstImageModelCandidates(settings AppSettings) ([]ModelConfig, error) {
	candidates, err := a.onlineImageRoutingCandidates()
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(candidates))
	byID := make(map[uint]ModelConfig, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
		byID[candidate.ID] = candidate
	}

	type modelLatencyRank struct {
		ModelConfigID  uint
		AverageLatency float64
		Samples        int64
	}
	var ranks []modelLatencyRank
	err = a.db.Model(&GenerationRecord{}).
		Select("model_config_id, AVG(latency_ms) AS average_latency, COUNT(*) AS samples").
		Where("model_config_id IN ? AND status = ? AND latency_ms > 0 AND created_at >= ?", ids, GenerationStatusSucceeded, time.Now().Add(-7*24*time.Hour)).
		Group("model_config_id").
		Order("average_latency asc, samples desc, model_config_id asc").
		Scan(&ranks).Error
	if err != nil {
		return nil, err
	}
	ordered := make([]ModelConfig, 0, len(candidates))
	used := map[uint]bool{}
	for _, rank := range ranks {
		if model, ok := byID[rank.ModelConfigID]; ok {
			ordered = append(ordered, model)
			used[model.ID] = true
		}
	}
	if len(ordered) == 0 {
		defaults, err := a.defaultImageModelCandidates(settings)
		if err != nil {
			return nil, err
		}
		for _, model := range defaults {
			if _, ok := byID[model.ID]; ok && !used[model.ID] {
				ordered = append(ordered, model)
				used[model.ID] = true
			}
		}
	}
	for _, candidate := range candidates {
		if !used[candidate.ID] {
			ordered = append(ordered, candidate)
			used[candidate.ID] = true
		}
	}
	return ordered, nil
}

func (a *App) roundRobinImageModelConfig() (*ModelConfig, error) {
	candidates, err := a.roundRobinImageModelCandidates(AppSettings{})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return &candidates[0], nil
}

func (a *App) roundRobinImageModelCandidates(settings AppSettings) ([]ModelConfig, error) {
	_ = settings
	candidates, err := a.onlineImageRoutingCandidates()
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}

	type modelCallCount struct {
		ModelConfigID uint
		Calls         int64
	}
	var rows []modelCallCount
	if err := a.db.Model(&GenerationRecord{}).
		Select("model_config_id, COUNT(*) AS calls").
		Where("model_config_id IN ?", ids).
		Group("model_config_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.ModelConfigID] = row.Calls
	}

	ordered := append([]ModelConfig(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]
		leftCalls := counts[left.ID]
		rightCalls := counts[right.ID]
		if leftCalls != rightCalls {
			return leftCalls < rightCalls
		}
		if left.SortOrder != right.SortOrder {
			return left.SortOrder < right.SortOrder
		}
		return left.ID < right.ID
	})
	return ordered, nil
}

func dedupeModelCandidates(candidates []ModelConfig) []ModelConfig {
	if len(candidates) == 0 {
		return nil
	}
	deduped := make([]ModelConfig, 0, len(candidates))
	seen := make(map[uint]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == 0 || seen[candidate.ID] {
			continue
		}
		seen[candidate.ID] = true
		deduped = append(deduped, candidate)
	}
	return deduped
}

func (a *App) prioritizeModelFailoverCooldown(candidates []ModelConfig, now time.Time) ([]ModelConfig, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	ids := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID > 0 {
			ids = append(ids, candidate.ID)
		}
	}
	if len(ids) == 0 {
		return candidates, nil
	}
	var recent []ModelCallAttempt
	if err := a.db.
		Where("model_config_id IN ? AND status = ? AND started_at >= ?", ids, ModelCallAttemptStatusFailed, now.Add(-modelFailoverCooldownTTL)).
		Find(&recent).Error; err != nil {
		return nil, err
	}
	cooling := map[uint]bool{}
	for _, attempt := range recent {
		if providerErrorTriggersModelFailover(&ProviderError{
			HTTPStatus:        attempt.HTTPStatus,
			Code:              attempt.ErrorCode,
			Message:           attempt.ErrorMessage,
			ProviderRequestID: attempt.ProviderRequestID,
			FailureStage:      attempt.FailureStage,
		}) {
			cooling[attempt.ModelConfigID] = true
		}
	}
	if len(cooling) == 0 {
		return candidates, nil
	}
	prioritized := make([]ModelConfig, 0, len(candidates))
	delayed := make([]ModelConfig, 0, len(candidates))
	for _, candidate := range candidates {
		if cooling[candidate.ID] {
			delayed = append(delayed, candidate)
			continue
		}
		prioritized = append(prioritized, candidate)
	}
	prioritized = append(prioritized, delayed...)
	return prioritized, nil
}

func (a *App) modelConfigForRuntime(runtimeModel string) (*ModelConfig, error) {
	runtimeModel = strings.TrimSpace(runtimeModel)
	if runtimeModel == "" {
		return nil, nil
	}
	var model ModelConfig
	err := a.db.
		Where("type = ? AND (runtime_model = ? OR name = ?)", ModelConfigTypeImage, runtimeModel, runtimeModel).
		Order("runtime_model desc, sort_order asc, id asc").
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := a.hydrateModelConfig(&model); err != nil {
		return nil, err
	}
	return &model, nil
}

func modelConfigProviderBaseURL(model *ModelConfig) string {
	if model == nil {
		return ""
	}
	return strings.TrimSpace(model.APIBaseURL)
}

func modelConfigProviderAPIKey(model *ModelConfig) string {
	if model == nil {
		return ""
	}
	return strings.TrimSpace(model.APIKey)
}

func modelConfigProviderAPIEndpoint(model *ModelConfig) string {
	if model == nil {
		return ""
	}
	return strings.TrimSpace(model.APIEndpoint)
}

func generationRuntimeModel(settings AppSettings, model *ModelConfig) string {
	if model != nil {
		if runtimeModel := strings.TrimSpace(model.RuntimeModel); runtimeModel != "" {
			return runtimeModel
		}
		if name := strings.TrimSpace(model.Name); name != "" {
			return name
		}
	}
	return strings.TrimSpace(settings.ActiveImageModel)
}

func modelConfigIDValue(model *ModelConfig) uint {
	if model == nil {
		return 0
	}
	return model.ID
}

func applyGenerationRecordModel(record *GenerationRecord, settings AppSettings, model *ModelConfig) {
	record.ModelConfigID = modelConfigIDValue(model)
	record.Model = generationRuntimeModel(settings, model)
}

func applyGenerationRecordModelCenter(record *GenerationRecord, candidate *modelCenterCandidate) {
	if candidate == nil {
		return
	}
	runtimeModel := modelCenterRuntimeModel(candidate)
	record.ModelID = candidate.Model.ID
	record.ChannelID = candidate.Channel.ID
	record.ModelName = candidate.Model.Name
	record.ChannelName = candidate.Channel.Name
	record.RuntimeModel = runtimeModel
	record.ModelConfigID = candidate.Channel.LegacyModelConfigID
	record.Model = runtimeModel
}

func selectedModelCenterCandidate(job *generationJob) *modelCenterCandidate {
	if job == nil {
		return nil
	}
	if len(job.ModelCenterCandidates) > 0 {
		return &job.ModelCenterCandidates[0]
	}
	if job.ModelCenterModel == nil || job.ModelCenterChannel == nil {
		return nil
	}
	return &modelCenterCandidate{
		Model:   *job.ModelCenterModel,
		Channel: *job.ModelCenterChannel,
	}
}
