package app

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type modelCenterModelWriteRequest struct {
	Name                 *string  `json:"name"`
	Modality             *string  `json:"modality"`
	Status               *string  `json:"status"`
	Visibility           *string  `json:"visibility"`
	DefaultCreditsCost   *int     `json:"default_credits_cost"`
	CapabilityTags       []string `json:"capability_tags"`
	VideoDurations       []string `json:"video_durations"`
	DefaultVideoDuration *string  `json:"default_video_duration"`
	SortOrder            *int     `json:"sort_order"`
}

type modelCenterProviderWriteRequest struct {
	Name                  *string `json:"name"`
	Provider              *string `json:"provider"`
	BaseURL               *string `json:"base_url"`
	APIKey                *string `json:"api_key"`
	ClearAPIKey           *bool   `json:"clear_api_key"`
	DefaultTimeoutSeconds *int    `json:"default_timeout_seconds"`
	ConcurrencyLimit      *int    `json:"concurrency_limit"`
	Status                *string `json:"status"`
}

type modelCenterChannelWriteRequest struct {
	ModelID        *uint    `json:"model_id"`
	ProviderID     *uint    `json:"provider_id"`
	Name           *string  `json:"name"`
	RuntimeModel   *string  `json:"runtime_model"`
	VideoDurations []string `json:"video_durations"`
	Endpoint       *string  `json:"endpoint"`
	Weight         *int     `json:"weight"`
	Priority       *int     `json:"priority"`
	Status         *string  `json:"status"`
	HealthStatus   *string  `json:"health_status"`
	LastErrorCode  *string  `json:"last_error_code"`
}

type modelCenterRoutingPutRequest struct {
	Routes []modelCenterRoutingRouteRequest `json:"routes"`
}

type modelCenterRoutingRouteRequest struct {
	Modality        string                           `json:"modality"`
	DefaultModelID  uint                             `json:"default_model_id"`
	FallbackModelID uint                             `json:"fallback_model_id"`
	RoutingEnabled  bool                             `json:"routing_enabled"`
	RoutingStrategy string                           `json:"routing_strategy"`
	Entries         []modelCenterRoutingEntryRequest `json:"entries"`
}

type modelCenterRoutingEntryRequest struct {
	ModelID   uint `json:"model_id"`
	ChannelID uint `json:"channel_id"`
	Enabled   bool `json:"enabled"`
	Weight    int  `json:"weight"`
	Priority  int  `json:"priority"`
}

type modelCenterCandidate struct {
	Model    ModelCatalog
	Channel  ModelChannel
	Provider ModelProvider
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		normalized = append(normalized, text)
	}
	return normalized
}

func decodeStringList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	var values []string
	if err := jsonUnmarshalStringList(value, &values); err != nil {
		return []string{}
	}
	return normalizeStringList(values)
}

func jsonUnmarshalStringList(value string, target *[]string) error {
	return json.Unmarshal([]byte(value), target)
}

func (a *App) ensureModelCenter() error {
	a.modelCenterSyncMu.Lock()
	defer a.modelCenterSyncMu.Unlock()
	if err := a.ensureVideoDurationCapabilityColumns(); err != nil {
		return err
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		return a.syncModelCenterFromLegacy(tx)
	}); err != nil {
		return err
	}
	return a.syncLegacyModelSecretsToProviders()
}

func (a *App) syncModelCenterFromLegacy(tx *gorm.DB) error {
	var configs []ModelConfig
	if err := tx.Order("sort_order asc, id asc").Find(&configs).Error; err != nil {
		return err
	}
	for _, config := range configs {
		if _, _, err := ensureModelCenterChannelForConfig(tx, config); err != nil {
			return err
		}
	}
	if err := backfillVideoDurationCapabilities(tx); err != nil {
		return err
	}
	return syncLegacyModelCenterRouting(tx)
}

func ensureModelCenterChannelForConfig(tx *gorm.DB, config ModelConfig) (ModelCatalog, ModelChannel, error) {
	var channel ModelChannel
	err := tx.Where("legacy_model_config_id = ?", config.ID).First(&channel).Error
	if err == nil {
		provider, providerDeleted, err := ensureModelCenterProviderForConfig(tx, config)
		if err != nil {
			return ModelCatalog{}, channel, err
		}
		if providerDeleted {
			if err := deleteModelCenterChannelsAndRoutingEntries(tx, []ModelChannel{channel}); err != nil {
				return ModelCatalog{}, channel, err
			}
			return ModelCatalog{}, ModelChannel{}, nil
		}
		model, err := ensureModelCenterCatalogForConfig(tx, config)
		if err != nil {
			return model, channel, err
		}
		updates := map[string]any{}
		if channel.ModelID != model.ID {
			updates["model_id"] = model.ID
		}
		if channel.ProviderID != provider.ID {
			updates["provider_id"] = provider.ID
		}
		if name := fallbackString(strings.TrimSpace(config.Name), channel.Name); channel.Name != name {
			updates["name"] = name
		}
		if runtime := legacyChannelRuntime(config); channel.RuntimeModel != runtime {
			updates["runtime_model"] = runtime
		}
		if endpoint := strings.TrimSpace(config.APIEndpoint); channel.Endpoint != endpoint {
			updates["endpoint"] = endpoint
		}
		if channel.Weight != config.Weight {
			updates["weight"] = config.Weight
		}
		if priority := legacyChannelPriority(config); channel.Priority != priority {
			updates["priority"] = priority
		}
		if status := normalizeModelCenterStatus(config.Status); channel.Status != status {
			updates["status"] = status
		}
		if strings.TrimSpace(channel.HealthStatus) == "" {
			updates["health_status"] = ModelChannelHealthHealthy
		}
		if len(updates) > 0 {
			if err := tx.Model(&channel).Updates(updates).Error; err != nil {
				return model, channel, err
			}
			if err := tx.First(&channel, channel.ID).Error; err != nil {
				return model, channel, err
			}
		}
		return model, channel, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ModelCatalog{}, ModelChannel{}, err
	}

	deletedChannel, deleted, err := deletedLegacyModelCenterChannel(tx, config.ID)
	if err != nil {
		return ModelCatalog{}, ModelChannel{}, err
	}
	if deleted {
		if err := deleteModelCenterRoutingEntriesForChannelIDs(tx, []uint{deletedChannel.ID}); err != nil {
			return ModelCatalog{}, ModelChannel{}, err
		}
		return ModelCatalog{}, ModelChannel{}, nil
	}

	provider, providerDeleted, err := ensureModelCenterProviderForConfig(tx, config)
	if err != nil {
		return ModelCatalog{}, ModelChannel{}, err
	}
	if providerDeleted {
		return ModelCatalog{}, ModelChannel{}, nil
	}
	model, err := ensureModelCenterCatalogForConfig(tx, config)
	if err != nil {
		return ModelCatalog{}, ModelChannel{}, err
	}
	channel = ModelChannel{
		ModelID:             model.ID,
		ProviderID:          provider.ID,
		LegacyModelConfigID: config.ID,
		Name:                fallbackString(strings.TrimSpace(config.Name), model.Name),
		RuntimeModel:        legacyChannelRuntime(config),
		Endpoint:            strings.TrimSpace(config.APIEndpoint),
		Weight:              config.Weight,
		Priority:            legacyChannelPriority(config),
		Status:              normalizeModelCenterStatus(config.Status),
		HealthStatus:        ModelChannelHealthHealthy,
	}
	if err := tx.Create(&channel).Error; err != nil {
		return model, channel, err
	}
	return model, channel, nil
}

func deletedLegacyModelCenterChannel(tx *gorm.DB, legacyModelConfigID uint) (ModelChannel, bool, error) {
	if legacyModelConfigID == 0 {
		return ModelChannel{}, false, nil
	}
	var channel ModelChannel
	err := tx.Unscoped().
		Where("legacy_model_config_id = ?", legacyModelConfigID).
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc, id desc").
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ModelChannel{}, false, nil
	}
	if err != nil {
		return ModelChannel{}, false, err
	}
	return channel, true, nil
}

func ensureModelCenterCatalogForConfig(tx *gorm.DB, config ModelConfig) (ModelCatalog, error) {
	runtime := strings.TrimSpace(config.RuntimeModel)
	expectedModality := legacyModelConfigModality(config)
	if runtime != "" {
		var existingChannel ModelChannel
		if err := tx.Where("runtime_model = ?", runtime).Order("id asc").First(&existingChannel).Error; err == nil && existingChannel.ModelID != 0 {
			var model ModelCatalog
			if err := tx.First(&model, existingChannel.ModelID).Error; err == nil {
				if model.Modality == expectedModality {
					return model, nil
				}
				var channelCount int64
				if err := tx.Model(&ModelChannel{}).Where("model_id = ?", model.ID).Count(&channelCount).Error; err != nil {
					return ModelCatalog{}, err
				}
				if channelCount == 1 {
					model.Modality = expectedModality
					model.CapabilityTags = []string{expectedModality}
					if err := tx.Save(&model).Error; err != nil {
						return ModelCatalog{}, err
					}
					return model, nil
				}
			}
		}
	}

	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = fallbackString(runtime, "未命名模型")
	}
	model := ModelCatalog{
		Name:               name,
		Modality:           expectedModality,
		Status:             normalizeModelCenterStatus(config.Status),
		Visibility:         normalizeModelCenterVisibility(config.Permission),
		DefaultCreditsCost: 1,
		CapabilityTags:     []string{expectedModality},
		SortOrder:          config.SortOrder,
	}
	var existing ModelCatalog
	if err := tx.Where("modality = ? AND name = ?", model.Modality, model.Name).First(&existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ModelCatalog{}, err
	}
	if err := tx.Create(&model).Error; err != nil {
		return ModelCatalog{}, err
	}
	return model, nil
}

func legacyModelConfigModality(config ModelConfig) string {
	if strings.TrimSpace(config.RuntimeModel) == "music-for-video" {
		return ModelConfigTypeAudio
	}
	return normalizeModelModality(config.Type)
}

func ensureModelCenterProviderForConfig(tx *gorm.DB, config ModelConfig) (ModelProvider, bool, error) {
	name := strings.TrimSpace(config.Provider)
	if name == "" {
		name = "默认供应商"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	providerCode := strings.ToLower(strings.TrimSpace(config.Provider))
	if providerCode == "" {
		providerCode = "default"
	}
	if isZZVideoModel(config.RuntimeModel, &config) {
		providerCode = zzVideoProviderCode
	}
	var provider ModelProvider
	err := tx.Where("name = ? AND provider = ? AND base_url = ? AND api_key = ?", name, providerCode, baseURL, strings.TrimSpace(config.APIKey)).First(&provider).Error
	if err == nil {
		return provider, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return provider, false, err
	}
	err = tx.Unscoped().
		Where("name = ? AND provider = ? AND base_url = ? AND api_key = ?", name, providerCode, baseURL, strings.TrimSpace(config.APIKey)).
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc, id desc").
		First(&provider).Error
	if err == nil {
		return provider, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return provider, false, err
	}
	provider = ModelProvider{
		Name:                  name,
		Provider:              providerCode,
		BaseURL:               baseURL,
		APIKey:                strings.TrimSpace(config.APIKey),
		DefaultTimeoutSeconds: defaultRequestTimeoutSeconds,
		ConcurrencyLimit:      0,
		Status:                ModelCenterStatusOnline,
	}
	if err := tx.Create(&provider).Error; err != nil {
		return provider, false, err
	}
	return provider, false, nil
}

func modelCenterChannelIDs(channels []ModelChannel) []uint {
	ids := make([]uint, 0, len(channels))
	for _, channel := range channels {
		if channel.ID != 0 {
			ids = append(ids, channel.ID)
		}
	}
	return ids
}

func (a *App) normalizeBailinAIConcurrency() error {
	return a.db.Model(&ModelProvider{}).
		Where("LOWER(name) = ? OR LOWER(provider) = ?", "bailinai", "bailinai").
		Where("concurrency_limit <> ?", 0).
		Update("concurrency_limit", 0).Error
}

func deleteModelCenterChannelsAndRoutingEntries(tx *gorm.DB, channels []ModelChannel) error {
	channelIDs := modelCenterChannelIDs(channels)
	if len(channelIDs) == 0 {
		return nil
	}
	if err := deleteModelCenterRoutingEntriesForChannelIDs(tx, channelIDs); err != nil {
		return err
	}
	return tx.Where("id IN ?", channelIDs).Delete(&ModelChannel{}).Error
}

func deleteModelCenterRoutingEntriesForChannelIDs(tx *gorm.DB, channelIDs []uint) error {
	if len(channelIDs) == 0 {
		return nil
	}
	return tx.Where("channel_id IN ?", channelIDs).Delete(&ModelRoutingEntry{}).Error
}

func syncLegacyModelCenterRouting(tx *gorm.DB) error {
	var settings AppSettings
	if err := tx.First(&settings, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if err := syncLegacyModelCenterRoutingForModality(tx, settings, ModelConfigTypeImage); err != nil {
		return err
	}
	if err := syncLegacyModelCenterRoutingForModality(tx, settings, ModelConfigTypeVideo); err != nil {
		return err
	}
	return syncLegacyModelCenterRoutingForModality(tx, settings, ModelConfigTypeAudio)
}

func syncLegacyModelCenterRoutingForModality(tx *gorm.DB, settings AppSettings, modality string) error {
	var policy ModelRoutingPolicy
	err := tx.Where("modality = ?", modality).First(&policy).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && policy.Source == ModelRoutingSourceModelCenter {
		return nil
	}

	defaultModelID, fallbackModelID, err := legacyPolicyModelIDs(tx, settings, modality)
	if err != nil {
		return err
	}
	if defaultModelID == 0 {
		return nil
	}
	if fallbackModelID == 0 {
		fallbackModelID = defaultModelID
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || policy.ID == 0 {
		policy = ModelRoutingPolicy{
			Modality:        modality,
			DefaultModelID:  defaultModelID,
			FallbackModelID: fallbackModelID,
			RoutingEnabled:  settings.ModelRoutingEnabled,
			RoutingStrategy: normalizeModelRoutingStrategy(settings.ModelRoutingStrategy),
			Source:          ModelRoutingSourceLegacy,
		}
		if err := tx.Create(&policy).Error; err != nil {
			return err
		}
	} else {
		strategy := normalizeModelRoutingStrategy(settings.ModelRoutingStrategy)
		if policy.DefaultModelID != defaultModelID ||
			policy.FallbackModelID != fallbackModelID ||
			policy.RoutingEnabled != settings.ModelRoutingEnabled ||
			policy.RoutingStrategy != strategy ||
			policy.Source != ModelRoutingSourceLegacy {
			policy.DefaultModelID = defaultModelID
			policy.FallbackModelID = fallbackModelID
			policy.RoutingEnabled = settings.ModelRoutingEnabled
			policy.RoutingStrategy = strategy
			policy.Source = ModelRoutingSourceLegacy
			if err := tx.Save(&policy).Error; err != nil {
				return err
			}
		}
	}
	return syncLegacyModelCenterRoutingEntries(tx, policy)
}

func legacyPolicyModelIDs(tx *gorm.DB, settings AppSettings, modality string) (uint, uint, error) {
	var defaultLegacyID, fallbackLegacyID uint
	switch modality {
	case ModelConfigTypeVideo:
		defaultLegacyID = uintPointerValue(settings.DefaultVideoModelID)
	case ModelConfigTypeImage:
		defaultLegacyID = uintPointerValue(settings.DefaultImageModelID)
		fallbackLegacyID = uintPointerValue(settings.FallbackModelID)
	}
	defaultModelID, err := modelCenterModelIDForLegacyConfig(tx, defaultLegacyID)
	if err != nil {
		return 0, 0, err
	}
	fallbackModelID, err := modelCenterModelIDForLegacyConfig(tx, fallbackLegacyID)
	if err != nil {
		return 0, 0, err
	}
	if defaultModelID == 0 {
		var model ModelCatalog
		err := tx.Where("modality = ?", modality).Order("sort_order asc, id asc").First(&model).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, nil
		}
		if err != nil {
			return 0, 0, err
		}
		defaultModelID = model.ID
	}
	return defaultModelID, fallbackModelID, nil
}

func modelCenterModelIDForLegacyConfig(tx *gorm.DB, legacyID uint) (uint, error) {
	if legacyID == 0 {
		return 0, nil
	}
	var channel ModelChannel
	err := tx.Where("legacy_model_config_id = ?", legacyID).First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return channel.ModelID, nil
}

func syncLegacyModelCenterRoutingEntries(tx *gorm.DB, policy ModelRoutingPolicy) error {
	var channels []ModelChannel
	if err := tx.Joins("JOIN model_catalogs ON model_catalogs.id = model_channels.model_id").
		Where("model_catalogs.modality = ?", policy.Modality).
		Order("model_channels.priority asc, model_channels.id asc").
		Find(&channels).Error; err != nil {
		return err
	}
	for _, channel := range channels {
		entry := ModelRoutingEntry{}
		err := tx.Where("policy_id = ? AND channel_id = ?", policy.ID, channel.ID).First(&entry).Error
		enabled := channel.Status == ModelCenterStatusOnline
		if err == nil {
			updates := map[string]any{}
			if entry.ModelID != channel.ModelID {
				updates["model_id"] = channel.ModelID
			}
			if entry.Enabled != enabled {
				updates["enabled"] = enabled
			}
			if entry.Weight != channel.Weight {
				updates["weight"] = channel.Weight
			}
			if entry.Priority != channel.Priority {
				updates["priority"] = channel.Priority
			}
			if len(updates) > 0 {
				if err := tx.Model(&entry).Updates(updates).Error; err != nil {
					return err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		entry = ModelRoutingEntry{
			PolicyID:  policy.ID,
			ModelID:   channel.ModelID,
			ChannelID: channel.ID,
			Enabled:   enabled,
			Weight:    channel.Weight,
			Priority:  channel.Priority,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
	}
	return nil
}

func (a *App) modelCenterCandidatesForGeneration(settings AppSettings, modality string, explicitModelID uint) ([]modelCenterCandidate, error) {
	if err := a.ensureModelCenter(); err != nil {
		return nil, err
	}
	var policy ModelRoutingPolicy
	err := a.db.Where("modality = ?", modality).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if policy.Source == ModelRoutingSourceLegacy && explicitModelID == 0 && modality == ModelConfigTypeImage {
		return a.modelCenterCandidatesForLegacyImageSettings(settings)
	}

	modelID := explicitModelID
	if modelID == 0 {
		modelID = policy.DefaultModelID
	}
	if modelID == 0 {
		return nil, nil
	}
	strategy := normalizeModelRoutingStrategy(policy.RoutingStrategy)
	candidates, err := a.modelCenterCandidatesForPolicyModel(policy, modelID, strategy)
	if err != nil {
		return nil, err
	}
	if policy.FallbackModelID != 0 && policy.FallbackModelID != modelID {
		fallback, err := a.modelCenterCandidatesForPolicyModel(policy, policy.FallbackModelID, strategy)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, fallback...)
	}
	if policy.RoutingEnabled && strategy != ModelRoutingStrategyDefault {
		rest, err := a.modelCenterCandidatesForPolicy(policy, strategy)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, rest...)
	}
	return prioritizeModelCenterCooldown(dedupeModelCenterCandidates(candidates), time.Now()), nil
}

func (a *App) modelCenterCandidatesForLegacyImageSettings(settings AppSettings) ([]modelCenterCandidate, error) {
	modelConfigs, err := a.modelConfigCandidatesForGeneration(settings)
	if err != nil {
		return nil, err
	}
	candidates := make([]modelCenterCandidate, 0, len(modelConfigs))
	now := time.Now()
	for _, modelConfig := range modelConfigs {
		candidate, ok, err := a.modelCenterCandidateForLegacyConfig(modelConfig)
		if err != nil {
			return nil, err
		}
		if !ok || !modelCenterChannelAvailable(candidate.Channel, now) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return dedupeModelCenterCandidates(candidates), nil
}

func (a *App) modelCenterCandidateForLegacyConfig(modelConfig ModelConfig) (modelCenterCandidate, bool, error) {
	if modelConfig.ID == 0 {
		return modelCenterCandidate{}, false, nil
	}
	var channel ModelChannel
	err := a.db.Preload("Model").Preload("Provider").
		Where("legacy_model_config_id = ?", modelConfig.ID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return modelCenterCandidate{}, false, nil
	}
	if err != nil {
		return modelCenterCandidate{}, false, err
	}
	if err := a.hydrateModelProvider(&channel.Provider); err != nil {
		return modelCenterCandidate{}, false, err
	}
	if strings.TrimSpace(channel.Provider.APIKey) == "" {
		if err := a.hydrateModelConfig(&modelConfig); err != nil {
			return modelCenterCandidate{}, false, err
		}
		channel.Provider.APIKey = modelConfig.APIKey
	}
	return modelCenterCandidate{Model: channel.Model, Channel: channel, Provider: channel.Provider}, true, nil
}

func (a *App) modelCenterCandidatesForPolicy(policy ModelRoutingPolicy, strategy string) ([]modelCenterCandidate, error) {
	var entries []ModelRoutingEntry
	err := a.db.Where("policy_id = ? AND enabled = ?", policy.ID, true).Order("priority asc, id asc").Find(&entries).Error
	if err != nil {
		return nil, err
	}
	return a.modelCenterCandidatesFromEntries(entries, strategy)
}

func (a *App) modelCenterCandidatesForPolicyModel(policy ModelRoutingPolicy, modelID uint, strategy string) ([]modelCenterCandidate, error) {
	var entries []ModelRoutingEntry
	err := a.db.Where("policy_id = ? AND model_id = ? AND enabled = ?", policy.ID, modelID, true).Order("priority asc, id asc").Find(&entries).Error
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return a.modelCenterCandidatesFromEntries(entries, strategy)
	}
	var channels []ModelChannel
	err = a.db.Preload("Model").Preload("Provider").
		Where("model_id = ? AND status = ?", modelID, ModelCenterStatusOnline).
		Order("priority asc, id asc").
		Find(&channels).Error
	if err != nil {
		return nil, err
	}
	candidates := make([]modelCenterCandidate, 0, len(channels))
	for _, channel := range channels {
		if !modelCenterChannelAvailable(channel, time.Now()) {
			continue
		}
		if err := a.hydrateModelProvider(&channel.Provider); err != nil {
			return nil, err
		}
		candidates = append(candidates, modelCenterCandidate{Model: channel.Model, Channel: channel, Provider: channel.Provider})
	}
	a.orderModelCenterCandidates(candidates, strategy)
	return candidates, nil
}

func (a *App) modelCenterCandidatesFromEntries(entries []ModelRoutingEntry, strategy string) ([]modelCenterCandidate, error) {
	candidates := make([]modelCenterCandidate, 0, len(entries))
	now := time.Now()
	for _, entry := range entries {
		var channel ModelChannel
		err := a.db.Preload("Model").Preload("Provider").First(&channel, entry.ChannelID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !modelCenterChannelAvailable(channel, now) {
			continue
		}
		if err := a.hydrateModelProvider(&channel.Provider); err != nil {
			return nil, err
		}
		channel.Weight = entry.Weight
		channel.Priority = entry.Priority
		candidates = append(candidates, modelCenterCandidate{Model: channel.Model, Channel: channel, Provider: channel.Provider})
	}
	a.orderModelCenterCandidates(candidates, strategy)
	return candidates, nil
}

func (a *App) orderModelCenterCandidates(candidates []modelCenterCandidate, strategy string) {
	switch normalizeModelRoutingStrategy(strategy) {
	case ModelRoutingStrategySpeedFirst:
		a.orderModelCenterCandidatesBySpeed(candidates)
	case ModelRoutingStrategyRoundRobin:
		a.orderModelCenterCandidatesByFewestCalls(candidates)
	default:
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].Channel.Priority != candidates[j].Channel.Priority {
				return candidates[i].Channel.Priority < candidates[j].Channel.Priority
			}
			return candidates[i].Channel.ID < candidates[j].Channel.ID
		})
	}
}

func (a *App) orderModelCenterCandidatesBySpeed(candidates []modelCenterCandidate) {
	ids := modelCenterCandidateChannelIDs(candidates)
	if len(ids) == 0 {
		return
	}
	type channelLatencyRank struct {
		ChannelID      uint
		AverageLatency float64
		Samples        int64
	}
	var ranks []channelLatencyRank
	if err := a.db.Model(&ModelCallAttempt{}).
		Select("channel_id, AVG(latency_ms) AS average_latency, COUNT(*) AS samples").
		Where("channel_id IN ? AND status = ? AND latency_ms > 0 AND started_at >= ?", ids, ModelCallAttemptStatusSucceeded, time.Now().Add(-7*24*time.Hour)).
		Group("channel_id").
		Order("average_latency asc, samples desc, channel_id asc").
		Scan(&ranks).Error; err != nil {
		return
	}
	rankByID := map[uint]int{}
	for index, rank := range ranks {
		rankByID[rank.ChannelID] = index + 1
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRank := rankByID[candidates[i].Channel.ID]
		rightRank := rankByID[candidates[j].Channel.ID]
		if leftRank != rightRank {
			if leftRank == 0 {
				return false
			}
			if rightRank == 0 {
				return true
			}
			return leftRank < rightRank
		}
		if candidates[i].Channel.Priority != candidates[j].Channel.Priority {
			return candidates[i].Channel.Priority < candidates[j].Channel.Priority
		}
		return candidates[i].Channel.ID < candidates[j].Channel.ID
	})
}

func (a *App) orderModelCenterCandidatesByFewestCalls(candidates []modelCenterCandidate) {
	ids := modelCenterCandidateChannelIDs(candidates)
	if len(ids) == 0 {
		return
	}
	type channelCallCount struct {
		ChannelID uint
		Calls     int64
	}
	var rows []channelCallCount
	if err := a.db.Model(&ModelCallAttempt{}).
		Select("channel_id, COUNT(*) AS calls").
		Where("channel_id IN ?", ids).
		Group("channel_id").
		Scan(&rows).Error; err != nil {
		return
	}
	counts := map[uint]int64{}
	for _, row := range rows {
		counts[row.ChannelID] = row.Calls
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if counts[left.Channel.ID] != counts[right.Channel.ID] {
			return counts[left.Channel.ID] < counts[right.Channel.ID]
		}
		if left.Channel.Priority != right.Channel.Priority {
			return left.Channel.Priority < right.Channel.Priority
		}
		return left.Channel.ID < right.Channel.ID
	})
}

func modelCenterCandidateChannelIDs(candidates []modelCenterCandidate) []uint {
	ids := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Channel.ID != 0 {
			ids = append(ids, candidate.Channel.ID)
		}
	}
	return ids
}

func modelCenterChannelAvailable(channel ModelChannel, now time.Time) bool {
	if channel.Status != ModelCenterStatusOnline {
		return false
	}
	if channel.ProviderID != 0 && channel.Provider.Status != "" && channel.Provider.Status != ModelCenterStatusOnline {
		return false
	}
	if channel.HealthStatus == ModelChannelHealthDown {
		return false
	}
	return true
}

func dedupeModelCenterCandidates(candidates []modelCenterCandidate) []modelCenterCandidate {
	deduped := make([]modelCenterCandidate, 0, len(candidates))
	seen := map[uint]bool{}
	for _, candidate := range candidates {
		if candidate.Channel.ID == 0 || seen[candidate.Channel.ID] {
			continue
		}
		seen[candidate.Channel.ID] = true
		deduped = append(deduped, candidate)
	}
	return deduped
}

func prioritizeModelCenterCooldown(candidates []modelCenterCandidate, now time.Time) []modelCenterCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	cooling := map[uint]bool{}
	for _, candidate := range candidates {
		if candidate.Channel.FailCooldownUntil != nil && candidate.Channel.FailCooldownUntil.After(now) {
			cooling[candidate.Channel.ID] = true
		}
	}
	prioritized := make([]modelCenterCandidate, 0, len(candidates))
	delayed := make([]modelCenterCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if cooling[candidate.Channel.ID] {
			delayed = append(delayed, candidate)
			continue
		}
		prioritized = append(prioritized, candidate)
	}
	return append(prioritized, delayed...)
}

func modelCenterRuntimeModel(candidate *modelCenterCandidate) string {
	if candidate == nil {
		return ""
	}
	if runtime := strings.TrimSpace(candidate.Channel.RuntimeModel); runtime != "" {
		return runtime
	}
	return strings.TrimSpace(candidate.Model.Name)
}

func modelCenterProviderBaseURL(candidate *modelCenterCandidate) string {
	if candidate == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(candidate.Provider.BaseURL), "/")
}

func modelCenterProviderAPIKey(candidate *modelCenterCandidate) string {
	if candidate == nil {
		return ""
	}
	return strings.TrimSpace(candidate.Provider.APIKey)
}

func modelCenterProviderEndpoint(candidate *modelCenterCandidate) string {
	if candidate == nil {
		return ""
	}
	return strings.TrimSpace(candidate.Channel.Endpoint)
}

func modelCenterDefaultCreditsCost(model *ModelCatalog) int {
	return 1
}

func legacyChannelRuntime(config ModelConfig) string {
	if runtime := strings.TrimSpace(config.RuntimeModel); runtime != "" {
		return runtime
	}
	return strings.TrimSpace(config.Name)
}

func legacyChannelPriority(config ModelConfig) int {
	if config.Priority > 0 {
		return config.Priority
	}
	if config.SortOrder > 0 {
		return config.SortOrder
	}
	return int(config.ID)
}

func normalizeModelModality(value string) string {
	switch strings.TrimSpace(value) {
	case ModelConfigTypeVideo:
		return ModelConfigTypeVideo
	case ModelConfigTypeAudio:
		return ModelConfigTypeAudio
	case ModelConfigTypeChat:
		return ModelConfigTypeChat
	default:
		return ModelConfigTypeImage
	}
}

func normalizeModelCenterStatus(value string) string {
	if strings.TrimSpace(value) == ModelCenterStatusOffline {
		return ModelCenterStatusOffline
	}
	return ModelCenterStatusOnline
}

func normalizeModelCenterVisibility(value string) string {
	if strings.TrimSpace(value) == ModelCenterVisibilityInternal {
		return ModelCenterVisibilityInternal
	}
	return ModelCenterVisibilityPublic
}

func normalizeModelChannelHealth(value string) string {
	switch strings.TrimSpace(value) {
	case ModelChannelHealthDegraded:
		return ModelChannelHealthDegraded
	case ModelChannelHealthDown:
		return ModelChannelHealthDown
	default:
		return ModelChannelHealthHealthy
	}
}

func parseCreditsCostLabel(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 1
	}
	total := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			break
		}
		total = total*10 + int(char-'0')
	}
	if total <= 0 {
		return 1
	}
	return total
}

func validModelCenterStatus(value string) bool {
	return value == ModelCenterStatusOnline || value == ModelCenterStatusOffline
}

func validModelCenterVisibility(value string) bool {
	return value == ModelCenterVisibilityPublic || value == ModelCenterVisibilityInternal
}

func validModelChannelHealth(value string) bool {
	return value == ModelChannelHealthHealthy || value == ModelChannelHealthDegraded || value == ModelChannelHealthDown
}

func (a *App) handleGetModelCenterOverview(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	models, providers, channels, routes, err := a.modelCenterOverviewData()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_load_failed", "模型中心读取失败")
		return
	}
	monitoring, err := modelCenterMonitoringSummary(a.db)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_monitoring_load_failed", "调用监控读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"summary": gin.H{
			"models":    len(models),
			"providers": len(providers),
			"channels":  len(channels),
		},
		"models":     modelCatalogResponses(models),
		"providers":  modelProviderResponses(providers),
		"channels":   modelChannelResponses(channels),
		"routing":    routes,
		"monitoring": monitoring,
	})
}

func (a *App) modelCenterOverviewData() ([]ModelCatalog, []ModelProvider, []ModelChannel, []gin.H, error) {
	var models []ModelCatalog
	if err := a.db.Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	var providers []ModelProvider
	if err := a.db.Order("id asc").Find(&providers).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	var channels []ModelChannel
	if err := a.db.Preload("Model").Preload("Provider").Order("priority asc, id asc").Find(&channels).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	routes, err := a.modelCenterRoutingResponses()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return models, providers, channels, routes, nil
}

func (a *App) handleListModelCenterModels(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	query := a.db.Model(&ModelCatalog{})
	if modality := strings.TrimSpace(c.Query("modality")); modality != "" {
		query = query.Where("modality = ?", modality)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if search := strings.ToLower(strings.TrimSpace(c.Query("q"))); search != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+search+"%")
	}
	var models []ModelCatalog
	if err := query.Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_models_load_failed", "模型目录读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": modelCatalogResponses(models)})
}

func (a *App) handleCreateModelCenterModel(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	var req modelCenterModelWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	model := ModelCatalog{
		Status:             ModelCenterStatusOnline,
		Visibility:         ModelCenterVisibilityPublic,
		DefaultCreditsCost: 1,
		CapabilityTags:     []string{},
	}
	if err := applyModelCenterModelRequest(&model, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_model_center_model", "业务模型配置无效")
		return
	}
	if err := validateModelVideoDurationCompatibility(a.db, model); err != nil {
		writeModelCenterVideoDurationValidationError(c, err)
		return
	}
	if err := a.db.Create(&model).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_model_create_failed", "业务模型创建失败")
		return
	}
	a.writeAdminAudit(c, "model_center.model.create", "model_catalog", model.ID, gin.H{"after": modelCatalogResponse(model)})
	writeJSON(c, http.StatusCreated, modelCatalogResponse(model))
}

func (a *App) handleUpdateModelCenterModel(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	var model ModelCatalog
	if err := a.db.First(&model, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_center_model_not_found", "业务模型不存在")
		return
	}
	before := modelCatalogResponse(model)
	var req modelCenterModelWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyModelCenterModelRequest(&model, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_model_center_model", "业务模型配置无效")
		return
	}
	if err := validateModelVideoDurationCompatibility(a.db, model); err != nil {
		writeModelCenterVideoDurationValidationError(c, err)
		return
	}
	if err := a.db.Save(&model).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_model_save_failed", "业务模型保存失败")
		return
	}
	after := modelCatalogResponse(model)
	a.writeAdminAudit(c, "model_center.model.update", "model_catalog", model.ID, gin.H{"before": before, "after": after})
	writeJSON(c, http.StatusOK, after)
}

func (a *App) handleDeleteModelCenterModel(c *gin.Context) {
	var model ModelCatalog
	if err := a.db.First(&model, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_center_model_not_found", "业务模型不存在")
		return
	}
	if err := a.db.Delete(&model).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_model_delete_failed", "业务模型删除失败")
		return
	}
	a.writeAdminAudit(c, "model_center.model.delete", "model_catalog", model.ID, gin.H{"before": modelCatalogResponse(model)})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func applyModelCenterModelRequest(model *ModelCatalog, req modelCenterModelWriteRequest) error {
	if req.Name != nil {
		model.Name = strings.TrimSpace(*req.Name)
	}
	if req.Modality != nil {
		model.Modality = normalizeModelModality(*req.Modality)
	}
	if req.Status != nil {
		model.Status = strings.TrimSpace(*req.Status)
	}
	if req.Visibility != nil {
		model.Visibility = strings.TrimSpace(*req.Visibility)
	}
	if req.DefaultCreditsCost != nil {
		model.DefaultCreditsCost = *req.DefaultCreditsCost
	}
	if req.CapabilityTags != nil {
		model.CapabilityTags = normalizeStringList(req.CapabilityTags)
	}
	if req.VideoDurations != nil {
		values, err := normalizeVideoDurations(req.VideoDurations)
		if err != nil {
			return err
		}
		model.VideoDurations = values
	}
	if req.DefaultVideoDuration != nil {
		model.DefaultVideoDuration = strings.TrimSpace(*req.DefaultVideoDuration)
	}
	if req.SortOrder != nil {
		model.SortOrder = *req.SortOrder
	}
	if strings.TrimSpace(model.Name) == "" {
		return errors.New("name_required")
	}
	if !validModelConfigType(model.Modality) {
		return errors.New("invalid_modality")
	}
	if !validModelCenterStatus(model.Status) {
		return errors.New("invalid_status")
	}
	if !validModelCenterVisibility(model.Visibility) {
		return errors.New("invalid_visibility")
	}
	if model.DefaultCreditsCost < 0 || model.DefaultCreditsCost > 1000 {
		return errors.New("invalid_credits_cost")
	}
	if model.DefaultCreditsCost == 0 && (model.Modality != ModelConfigTypeChat || model.Visibility != ModelCenterVisibilityInternal) {
		return errors.New("invalid_credits_cost")
	}
	if model.SortOrder < 0 {
		return errors.New("invalid_sort_order")
	}
	if model.Modality != ModelConfigTypeVideo {
		model.VideoDurations = []string{}
		model.DefaultVideoDuration = ""
	} else {
		if len(model.VideoDurations) == 0 {
			return errors.New("video_duration_required")
		}
		if !videoDurationContains(model.VideoDurations, model.DefaultVideoDuration) {
			return errors.New("invalid_default_video_duration")
		}
	}
	return nil
}

func (a *App) handleListModelCenterProviders(c *gin.Context) {
	var providers []ModelProvider
	if err := a.db.Order("id asc").Find(&providers).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_providers_load_failed", "供应商读取失败")
		return
	}
	for i := range providers {
		if err := a.hydrateModelProvider(&providers[i]); err != nil {
			writeError(c, http.StatusInternalServerError, "model_center_provider_secret_load_failed", "供应商密钥读取失败")
			return
		}
	}
	writeJSON(c, http.StatusOK, gin.H{"items": modelProviderResponses(providers)})
}

func (a *App) handleCreateModelCenterProvider(c *gin.Context) {
	var req modelCenterProviderWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	provider := ModelProvider{Status: ModelCenterStatusOnline, DefaultTimeoutSeconds: defaultRequestTimeoutSeconds}
	if err := applyModelCenterProviderRequest(&provider, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_model_center_provider", "供应商配置无效")
		return
	}
	apiKey := provider.APIKey
	if a.secretStore != nil {
		provider.APIKey = ""
	}
	if err := a.db.Create(&provider).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_provider_create_failed", "供应商创建失败")
		return
	}
	if err := a.saveModelProviderAPIKey(provider.ID, apiKey, req.ClearAPIKey != nil && *req.ClearAPIKey, "admin-api"); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_provider_secret_save_failed", "供应商密钥保存失败")
		return
	}
	provider.APIKey = apiKey
	a.writeAdminAudit(c, "model_center.provider.create", "model_provider", provider.ID, gin.H{"after": modelProviderResponse(provider)})
	writeJSON(c, http.StatusCreated, modelProviderResponse(provider))
}

func (a *App) handleUpdateModelCenterProvider(c *gin.Context) {
	var provider ModelProvider
	if err := a.db.First(&provider, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_center_provider_not_found", "供应商不存在")
		return
	}
	if err := a.hydrateModelProvider(&provider); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_provider_secret_load_failed", "供应商密钥读取失败")
		return
	}
	before := modelProviderResponse(provider)
	var req modelCenterProviderWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyModelCenterProviderRequest(&provider, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_model_center_provider", "供应商配置无效")
		return
	}
	apiKey := provider.APIKey
	toSave := provider
	if a.secretStore != nil {
		toSave.APIKey = ""
	}
	if err := a.db.Save(&toSave).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_provider_save_failed", "供应商保存失败")
		return
	}
	if err := a.saveModelProviderAPIKey(provider.ID, apiKey, req.ClearAPIKey != nil && *req.ClearAPIKey, "admin-api"); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_provider_secret_save_failed", "供应商密钥保存失败")
		return
	}
	after := modelProviderResponse(provider)
	a.writeAdminAudit(c, "model_center.provider.update", "model_provider", provider.ID, gin.H{"before": before, "after": after})
	writeJSON(c, http.StatusOK, after)
}

func (a *App) handleDeleteModelCenterProvider(c *gin.Context) {
	var before gin.H
	var targetID uint
	var affectedChannelIDs []uint
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var provider ModelProvider
		if err := tx.First(&provider, c.Param("id")).Error; err != nil {
			return err
		}
		before = modelProviderResponse(provider)
		targetID = provider.ID

		var channels []ModelChannel
		if err := tx.Where("provider_id = ?", provider.ID).Find(&channels).Error; err != nil {
			return err
		}
		affectedChannelIDs = modelCenterChannelIDs(channels)
		if err := deleteModelCenterChannelsAndRoutingEntries(tx, channels); err != nil {
			return err
		}
		return tx.Delete(&provider).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "model_center_provider_not_found", "供应商不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_provider_delete_failed", "供应商删除失败")
		return
	}
	if a.secretStore != nil {
		_ = a.secretStore.Delete(c.Request.Context(), "model_provider", modelSecretOwner(targetID), "api_key")
	}
	a.writeAdminAudit(c, "model_center.provider.delete", "model_provider", targetID, gin.H{
		"before":                 before,
		"affected_channel_count": len(affectedChannelIDs),
		"affected_channel_ids":   affectedChannelIDs,
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func applyModelCenterProviderRequest(provider *ModelProvider, req modelCenterProviderWriteRequest) error {
	if req.Name != nil {
		provider.Name = strings.TrimSpace(*req.Name)
	}
	if req.Provider != nil {
		provider.Provider = strings.ToLower(strings.TrimSpace(*req.Provider))
	}
	if req.BaseURL != nil {
		provider.BaseURL = strings.TrimRight(strings.TrimSpace(*req.BaseURL), "/")
	}
	if req.DefaultTimeoutSeconds != nil {
		provider.DefaultTimeoutSeconds = *req.DefaultTimeoutSeconds
	}
	if req.ConcurrencyLimit != nil {
		provider.ConcurrencyLimit = *req.ConcurrencyLimit
	}
	if req.Status != nil {
		provider.Status = strings.TrimSpace(*req.Status)
	}
	if req.ClearAPIKey != nil && *req.ClearAPIKey {
		provider.APIKey = ""
	}
	if req.APIKey != nil {
		if apiKey := strings.TrimSpace(*req.APIKey); apiKey != "" {
			provider.APIKey = apiKey
		}
	}
	if provider.Name == "" {
		return errors.New("name_required")
	}
	if provider.Provider == "" {
		provider.Provider = strings.ToLower(provider.Name)
	}
	if !validModelCenterStatus(provider.Status) {
		return errors.New("invalid_status")
	}
	if provider.DefaultTimeoutSeconds < 0 || provider.DefaultTimeoutSeconds > 3600 {
		return errors.New("invalid_timeout")
	}
	if provider.ConcurrencyLimit < 0 || provider.ConcurrencyLimit > 10000 {
		return errors.New("invalid_concurrency")
	}
	return nil
}

func (a *App) handleListModelCenterChannels(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	query := a.db.Preload("Model").Preload("Provider").Model(&ModelChannel{})
	if modelID := getQueryInt(c, "model_id", 0); modelID > 0 {
		query = query.Where("model_id = ?", uint(modelID))
	}
	if providerID := getQueryInt(c, "provider_id", 0); providerID > 0 {
		query = query.Where("provider_id = ?", uint(providerID))
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var channels []ModelChannel
	if err := query.Order("priority asc, id asc").Find(&channels).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channels_load_failed", "渠道读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": modelChannelResponses(channels)})
}

func (a *App) handleListModelCenterChannelCallAttempts(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}

	var channel ModelChannel
	if err := a.db.Preload("Model").Preload("Provider").First(&channel, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_center_channel_not_found", "渠道不存在")
		return
	}

	dateFrom, err := parseModelCenterDateQuery(c.Query("date_from"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}
	dateTo, err := parseModelCenterDateQuery(c.Query("date_to"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_date", "日期格式无效")
		return
	}

	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	modelID := uint(getQueryInt(c, "model_id", 0))
	status := normalizeModelCenterCallAttemptStatus(c.Query("status"))

	var total int64
	if err := a.modelCenterChannelCallAttemptsQuery(channel.ID, modelID, status, dateFrom, dateTo).Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_call_attempts_load_failed", "渠道调用尝试读取失败")
		return
	}

	var rows []modelCenterChannelCallAttemptRow
	if err := a.modelCenterChannelCallAttemptsQuery(channel.ID, modelID, status, dateFrom, dateTo).
		Select("model_call_attempts.*, generation_records.model_id AS model_id").
		Order("model_call_attempts.started_at desc, model_call_attempts.id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_call_attempts_load_failed", "渠道调用尝试读取失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"channel":   modelChannelResponse(channel),
		"model_id":  modelID,
		"items":     modelCenterChannelCallAttemptResponses(rows),
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

type modelCenterChannelCallAttemptRow struct {
	ModelCallAttempt
	ModelID uint `json:"model_id"`
}

func (a *App) modelCenterChannelCallAttemptsQuery(channelID, modelID uint, status string, dateFrom, dateTo *time.Time) *gorm.DB {
	query := a.db.Model(&ModelCallAttempt{}).
		Joins("LEFT JOIN generation_records ON generation_records.id = model_call_attempts.generation_record_id").
		Where("model_call_attempts.channel_id = ?", channelID)
	if modelID > 0 {
		query = query.Where("generation_records.model_id = ?", modelID)
	}
	if status != "" {
		query = query.Where("model_call_attempts.status = ?", status)
	}
	if dateFrom != nil {
		query = query.Where("model_call_attempts.started_at >= ?", *dateFrom)
	}
	if dateTo != nil {
		query = query.Where("model_call_attempts.started_at < ?", dateTo.AddDate(0, 0, 1))
	}
	return query
}

func parseModelCenterDateQuery(value string) (*time.Time, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", text)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func normalizeModelCenterCallAttemptStatus(value string) string {
	status := strings.TrimSpace(value)
	if status == "all" {
		return ""
	}
	return status
}

func (a *App) handleCreateModelCenterChannel(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	var req modelCenterChannelWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	channel := ModelChannel{Status: ModelCenterStatusOnline, HealthStatus: ModelChannelHealthHealthy, Weight: 100, Priority: 1}
	if err := applyModelCenterChannelRequest(a.db, &channel, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_model_center_channel", "渠道配置无效")
		return
	}
	if err := validateChannelVideoDurationCompatibility(a.db, channel); err != nil {
		writeModelCenterVideoDurationValidationError(c, err)
		return
	}
	if err := a.db.Create(&channel).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_create_failed", "渠道创建失败")
		return
	}
	if err := a.db.Preload("Model").Preload("Provider").First(&channel, channel.ID).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_load_failed", "渠道读取失败")
		return
	}
	a.writeAdminAudit(c, "model_center.channel.create", "model_channel", channel.ID, gin.H{"after": modelChannelResponse(channel)})
	writeJSON(c, http.StatusCreated, modelChannelResponse(channel))
}

func (a *App) handleUpdateModelCenterChannel(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	var channel ModelChannel
	if err := a.db.Preload("Model").Preload("Provider").First(&channel, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "model_center_channel_not_found", "渠道不存在")
		return
	}
	before := modelChannelResponse(channel)
	var req modelCenterChannelWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyModelCenterChannelRequest(a.db, &channel, req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_model_center_channel", "渠道配置无效")
		return
	}
	if err := validateChannelVideoDurationCompatibility(a.db, channel); err != nil {
		writeModelCenterVideoDurationValidationError(c, err)
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := syncLegacyModelConfigForChannelUpdate(tx, channel); err != nil {
			return err
		}
		return tx.Save(&channel).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_save_failed", "渠道保存失败")
		return
	}
	if err := a.db.Preload("Model").Preload("Provider").First(&channel, channel.ID).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_load_failed", "渠道读取失败")
		return
	}
	after := modelChannelResponse(channel)
	a.writeAdminAudit(c, "model_center.channel.update", "model_channel", channel.ID, gin.H{"before": before, "after": after})
	writeJSON(c, http.StatusOK, after)
}

func (a *App) handleDeleteModelCenterChannel(c *gin.Context) {
	var before gin.H
	var targetID uint
	var affectedRoutingEntryCount int64
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var channel ModelChannel
		if err := tx.Preload("Model").Preload("Provider").First(&channel, c.Param("id")).Error; err != nil {
			return err
		}
		before = modelChannelResponse(channel)
		targetID = channel.ID
		if err := tx.Model(&ModelRoutingEntry{}).Where("channel_id = ?", channel.ID).Count(&affectedRoutingEntryCount).Error; err != nil {
			return err
		}
		if err := deleteModelCenterChannelsAndRoutingEntries(tx, []ModelChannel{channel}); err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "model_center_channel_not_found", "渠道不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_channel_delete_failed", "渠道删除失败")
		return
	}
	a.writeAdminAudit(c, "model_center.channel.delete", "model_channel", targetID, gin.H{
		"before":                       before,
		"affected_routing_entry_count": affectedRoutingEntryCount,
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func syncLegacyModelConfigForChannelUpdate(tx *gorm.DB, channel ModelChannel) error {
	if channel.LegacyModelConfigID == 0 {
		return nil
	}
	var config ModelConfig
	if err := tx.First(&config, channel.LegacyModelConfigID).Error; err != nil {
		return err
	}
	updates := map[string]any{
		"name":          strings.TrimSpace(channel.Name),
		"runtime_model": strings.TrimSpace(channel.RuntimeModel),
		"api_endpoint":  strings.TrimSpace(channel.Endpoint),
		"weight":        channel.Weight,
		"priority":      channel.Priority,
		"status":        strings.TrimSpace(channel.Status),
	}
	if channel.ProviderID != 0 {
		var provider ModelProvider
		if err := tx.First(&provider, channel.ProviderID).Error; err != nil {
			return err
		}
		updates["provider"] = strings.TrimSpace(provider.Name)
		updates["api_base_url"] = strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
		// API key stays in SecretRecord and is never copied to the legacy plaintext column.
	}
	return tx.Model(&config).Updates(updates).Error
}

func applyModelCenterChannelRequest(db *gorm.DB, channel *ModelChannel, req modelCenterChannelWriteRequest) error {
	if req.ModelID != nil {
		channel.ModelID = *req.ModelID
	}
	if req.ProviderID != nil {
		channel.ProviderID = *req.ProviderID
	}
	if req.Name != nil {
		channel.Name = strings.TrimSpace(*req.Name)
	}
	if req.RuntimeModel != nil {
		channel.RuntimeModel = strings.TrimSpace(*req.RuntimeModel)
	}
	if req.VideoDurations != nil {
		values, err := normalizeVideoDurations(req.VideoDurations)
		if err != nil {
			return err
		}
		channel.VideoDurations = values
	}
	if req.Endpoint != nil {
		channel.Endpoint = strings.TrimSpace(*req.Endpoint)
	}
	if req.Weight != nil {
		channel.Weight = *req.Weight
	}
	if req.Priority != nil {
		channel.Priority = *req.Priority
	}
	if req.Status != nil {
		channel.Status = strings.TrimSpace(*req.Status)
	}
	if req.HealthStatus != nil {
		channel.HealthStatus = strings.TrimSpace(*req.HealthStatus)
	}
	if req.LastErrorCode != nil {
		channel.LastErrorCode = strings.TrimSpace(*req.LastErrorCode)
	}
	if channel.ModelID == 0 || channel.ProviderID == 0 {
		return errors.New("model_provider_required")
	}
	var model ModelCatalog
	if err := db.First(&model, channel.ModelID).Error; err != nil {
		return err
	}
	var providerCount int64
	if err := db.Model(&ModelProvider{}).Where("id = ?", channel.ProviderID).Count(&providerCount).Error; err != nil {
		return err
	}
	if providerCount == 0 {
		return errors.New("provider_not_found")
	}
	if channel.Name == "" {
		return errors.New("name_required")
	}
	if !validModelCenterStatus(channel.Status) {
		return errors.New("invalid_status")
	}
	if channel.HealthStatus == "" {
		channel.HealthStatus = ModelChannelHealthHealthy
	}
	if !validModelChannelHealth(channel.HealthStatus) {
		return errors.New("invalid_health_status")
	}
	if channel.Weight < 0 || channel.Weight > 100 {
		return errors.New("invalid_weight")
	}
	if channel.Priority <= 0 {
		return errors.New("invalid_priority")
	}
	if model.Modality != ModelConfigTypeVideo {
		channel.VideoDurations = []string{}
	} else if len(channel.VideoDurations) == 0 {
		return errors.New("video_duration_required")
	}
	return nil
}

func (a *App) handleGetModelCenterRouting(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	routes, err := a.modelCenterRoutingResponses()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_routing_load_failed", "路由配置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"routes": routes})
}

func (a *App) handlePutModelCenterRouting(c *gin.Context) {
	if err := a.ensureModelCenter(); err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_sync_failed", "模型中心同步失败")
		return
	}
	var req modelCenterRoutingPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if len(req.Routes) == 0 {
		writeError(c, http.StatusBadRequest, "routing_required", "路由配置不能为空")
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		for _, route := range req.Routes {
			if err := saveModelCenterRoutingRoute(tx, route); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Printf("model_center_routing_update_failed error=%v", err)
		if errors.Is(err, errVideoDurationCapabilityConflict) {
			writeModelCenterVideoDurationConflict(c, err)
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_model_center_routing", "路由配置无效")
		return
	}
	a.writeAdminAudit(c, "model_center.routing.update", "model_routing_policy", 0, gin.H{"routes": req.Routes})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func writeModelCenterVideoDurationConflict(c *gin.Context, err error) {
	payload := modelCenterVideoDurationConflictPayload(err)
	c.Set(requestLogErrorCodeKey, "video_duration_capability_conflict")
	c.Set(requestLogErrorMessageKey, payload["message"])
	c.JSON(http.StatusUnprocessableEntity, gin.H{"error": payload})
}

func writeModelCenterVideoDurationValidationError(c *gin.Context, err error) {
	if errors.Is(err, errVideoDurationCapabilityConflict) {
		writeModelCenterVideoDurationConflict(c, err)
		return
	}
	writeErrorWithLogDetail(c, http.StatusInternalServerError, "video_duration_capability_validation_failed", "视频时长能力校验失败", err.Error())
}

func saveModelCenterRoutingRoute(tx *gorm.DB, route modelCenterRoutingRouteRequest) error {
	modality := normalizeModelModality(route.Modality)
	if route.DefaultModelID == 0 {
		return errors.New("default_model_required")
	}
	if route.FallbackModelID == 0 {
		route.FallbackModelID = route.DefaultModelID
	}
	if err := ensureRoutingModelExists(tx, route.DefaultModelID, modality); err != nil {
		return err
	}
	if err := ensureRoutingModelExists(tx, route.FallbackModelID, modality); err != nil {
		return err
	}
	strategy := normalizeModelRoutingStrategy(route.RoutingStrategy)
	policy := ModelRoutingPolicy{}
	err := tx.Where("modality = ?", modality).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		policy = ModelRoutingPolicy{Modality: modality}
	} else if err != nil {
		return err
	}
	policy.DefaultModelID = route.DefaultModelID
	policy.FallbackModelID = route.FallbackModelID
	policy.RoutingEnabled = route.RoutingEnabled
	policy.RoutingStrategy = strategy
	policy.Source = ModelRoutingSourceModelCenter
	if policy.ID == 0 {
		if err := tx.Create(&policy).Error; err != nil {
			return err
		}
	} else if err := tx.Save(&policy).Error; err != nil {
		return err
	}
	if err := tx.Where("policy_id = ?", policy.ID).Delete(&ModelRoutingEntry{}).Error; err != nil {
		return err
	}
	for _, entryReq := range route.Entries {
		if err := ensureRoutingChannelExists(tx, entryReq.ModelID, entryReq.ChannelID, modality); err != nil {
			return err
		}
		if entryReq.Weight < 0 || entryReq.Weight > 100 || entryReq.Priority <= 0 {
			return errors.New("invalid_entry_weight_priority")
		}
		if entryReq.Enabled && modality == ModelConfigTypeVideo {
			var channel ModelChannel
			if err := tx.First(&channel, entryReq.ChannelID).Error; err != nil {
				return err
			}
			if err := validateChannelVideoDurationCompatibility(tx, channel); err != nil {
				return err
			}
		}
		entry := ModelRoutingEntry{
			PolicyID:  policy.ID,
			ModelID:   entryReq.ModelID,
			ChannelID: entryReq.ChannelID,
			Enabled:   entryReq.Enabled,
			Weight:    entryReq.Weight,
			Priority:  entryReq.Priority,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureRoutingModelExists(tx *gorm.DB, modelID uint, modality string) error {
	var count int64
	if err := tx.Model(&ModelCatalog{}).Where("id = ? AND modality = ?", modelID, modality).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("routing_model_not_found")
	}
	return nil
}

func ensureRoutingChannelExists(tx *gorm.DB, modelID, channelID uint, modality string) error {
	var channel ModelChannel
	if err := tx.Preload("Model").First(&channel, channelID).Error; err != nil {
		return errors.New("routing_channel_not_found")
	}
	if channel.ModelID != modelID || channel.Model.Modality != modality {
		return errors.New("routing_channel_model_mismatch")
	}
	return nil
}

func (a *App) modelCenterRoutingResponses() ([]gin.H, error) {
	var policies []ModelRoutingPolicy
	if err := a.db.Order("modality asc").Find(&policies).Error; err != nil {
		return nil, err
	}
	routes := make([]gin.H, 0, len(policies))
	for _, policy := range policies {
		var entries []ModelRoutingEntry
		if err := a.db.Where("policy_id = ?", policy.ID).Order("priority asc, id asc").Find(&entries).Error; err != nil {
			return nil, err
		}
		entryPayloads := make([]gin.H, 0, len(entries))
		for _, entry := range entries {
			entryPayloads = append(entryPayloads, gin.H{
				"id":         entry.ID,
				"model_id":   entry.ModelID,
				"channel_id": entry.ChannelID,
				"enabled":    entry.Enabled,
				"weight":     entry.Weight,
				"priority":   entry.Priority,
			})
		}
		routes = append(routes, gin.H{
			"id":                policy.ID,
			"modality":          policy.Modality,
			"default_model_id":  policy.DefaultModelID,
			"fallback_model_id": policy.FallbackModelID,
			"routing_enabled":   policy.RoutingEnabled,
			"routing_strategy":  normalizeModelRoutingStrategy(policy.RoutingStrategy),
			"source":            policy.Source,
			"entries":           entryPayloads,
		})
	}
	return routes, nil
}

func (a *App) handleListModelCenterAuditLogs(c *gin.Context) {
	var logs []AdminAuditLog
	if err := a.db.Where("action LIKE ?", "model_center.%").Order("created_at desc, id desc").Limit(100).Find(&logs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "model_center_audit_logs_load_failed", "审计日志读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": logs})
}

func modelCatalogResponses(models []ModelCatalog) []gin.H {
	items := make([]gin.H, 0, len(models))
	for _, model := range models {
		items = append(items, modelCatalogResponse(model))
	}
	return items
}

func modelCatalogResponse(model ModelCatalog) gin.H {
	tags := model.CapabilityTags
	if tags == nil {
		tags = decodeStringList(model.CapabilityTagsJSON)
	}
	durations := model.VideoDurations
	if durations == nil {
		durations = decodeVideoDurations(model.VideoDurationsJSON)
	}
	return gin.H{
		"id":                     model.ID,
		"name":                   model.Name,
		"modality":               model.Modality,
		"status":                 model.Status,
		"visibility":             model.Visibility,
		"default_credits_cost":   model.DefaultCreditsCost,
		"capability_tags":        tags,
		"video_durations":        durations,
		"default_video_duration": model.DefaultVideoDuration,
		"sort_order":             model.SortOrder,
		"created_at":             model.CreatedAt,
		"updated_at":             model.UpdatedAt,
	}
}

func modelProviderResponses(providers []ModelProvider) []gin.H {
	items := make([]gin.H, 0, len(providers))
	for _, provider := range providers {
		items = append(items, modelProviderResponse(provider))
	}
	return items
}

func modelProviderResponse(provider ModelProvider) gin.H {
	return gin.H{
		"id":                      provider.ID,
		"name":                    provider.Name,
		"provider":                provider.Provider,
		"base_url":                provider.BaseURL,
		"api_key_set":             strings.TrimSpace(provider.APIKey) != "",
		"default_timeout_seconds": provider.DefaultTimeoutSeconds,
		"concurrency_limit":       provider.ConcurrencyLimit,
		"status":                  provider.Status,
		"created_at":              provider.CreatedAt,
		"updated_at":              provider.UpdatedAt,
	}
}

func modelChannelResponses(channels []ModelChannel) []gin.H {
	items := make([]gin.H, 0, len(channels))
	for _, channel := range channels {
		items = append(items, modelChannelResponse(channel))
	}
	return items
}

func modelChannelResponse(channel ModelChannel) gin.H {
	durations := channel.VideoDurations
	if durations == nil {
		durations = decodeVideoDurations(channel.VideoDurationsJSON)
	}
	missingDurations := []string{}
	if channel.Model.Modality == ModelConfigTypeVideo && channel.Status == ModelCenterStatusOnline {
		missingDurations = missingVideoDurations(channel.Model.VideoDurations, durations)
	}
	return gin.H{
		"id":                      channel.ID,
		"model_id":                channel.ModelID,
		"model_name":              channel.Model.Name,
		"provider_id":             channel.ProviderID,
		"provider_name":           channel.Provider.Name,
		"legacy_model_config_id":  channel.LegacyModelConfigID,
		"name":                    channel.Name,
		"runtime_model":           channel.RuntimeModel,
		"video_durations":         durations,
		"duration_compatible":     len(missingDurations) == 0,
		"missing_video_durations": missingDurations,
		"endpoint":                channel.Endpoint,
		"weight":                  channel.Weight,
		"priority":                channel.Priority,
		"status":                  channel.Status,
		"health_status":           channel.HealthStatus,
		"fail_cooldown_until":     channel.FailCooldownUntil,
		"last_failure_at":         channel.LastFailureAt,
		"last_error_code":         channel.LastErrorCode,
		"created_at":              channel.CreatedAt,
		"updated_at":              channel.UpdatedAt,
	}
}

func modelCenterChannelCallAttemptResponses(rows []modelCenterChannelCallAttemptRow) []gin.H {
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{
			"id":                   row.ID,
			"generation_record_id": row.GenerationRecordID,
			"model_id":             row.ModelID,
			"channel_id":           row.ChannelID,
			"model_config_id":      row.ModelConfigID,
			"attempt_index":        row.AttemptIndex,
			"status":               row.Status,
			"latency_ms":           row.LatencyMS,
			"http_status":          row.HTTPStatus,
			"error_code":           row.ErrorCode,
			"error_message":        row.ErrorMessage,
			"failure_stage":        row.FailureStage,
			"provider_request_id":  row.ProviderRequestID,
			"started_at":           row.StartedAt,
			"finished_at":          row.FinishedAt,
			"created_at":           row.CreatedAt,
		})
	}
	return items
}

func modelCenterMonitoringSummary(db *gorm.DB) ([]gin.H, error) {
	type row struct {
		ModelID        uint
		ChannelID      uint
		TotalCalls     int64
		SucceededCalls int64
		FailedCalls    int64
		AverageLatency float64
	}
	var rows []row
	if err := db.Model(&ModelCallAttempt{}).
		Select("generation_records.model_id AS model_id, model_call_attempts.channel_id AS channel_id, COUNT(*) AS total_calls, SUM(CASE WHEN model_call_attempts.status = 'succeeded' THEN 1 ELSE 0 END) AS succeeded_calls, SUM(CASE WHEN model_call_attempts.status = 'failed' THEN 1 ELSE 0 END) AS failed_calls, COALESCE(AVG(model_call_attempts.latency_ms), 0) AS average_latency").
		Joins("LEFT JOIN generation_records ON generation_records.id = model_call_attempts.generation_record_id").
		Where("model_call_attempts.channel_id > 0").
		Group("generation_records.model_id, model_call_attempts.channel_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		successRate := 0.0
		if row.TotalCalls > 0 {
			successRate = math.Round(float64(row.SucceededCalls) / float64(row.TotalCalls) * 100)
		}
		items = append(items, gin.H{
			"model_id":           row.ModelID,
			"channel_id":         row.ChannelID,
			"total_calls":        row.TotalCalls,
			"succeeded_calls":    row.SucceededCalls,
			"failed_calls":       row.FailedCalls,
			"success_rate":       successRate,
			"average_latency_ms": int64(math.Round(row.AverageLatency)),
		})
	}
	return items, nil
}
