package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

var (
	errVideoDurationCapabilityConflict = errors.New("video_duration_capability_conflict")
	errVideoDurationChannelUnavailable = errors.New("video_duration_channel_unavailable")
)

type videoDurationCapabilityConflict struct {
	ChannelID   uint
	ChannelName string
	Missing     []string
}

func (e *videoDurationCapabilityConflict) Error() string {
	return fmt.Sprintf("%s: channel=%d missing=%s", errVideoDurationCapabilityConflict, e.ChannelID, strings.Join(e.Missing, ","))
}

func (e *videoDurationCapabilityConflict) Unwrap() error { return errVideoDurationCapabilityConflict }

func decodeVideoDurations(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return []string{}
	}
	normalized, err := normalizeVideoDurations(values)
	if err != nil {
		return []string{}
	}
	return normalized
}

func normalizeVideoDurations(values []string) ([]string, error) {
	seen := map[int]bool{}
	numbers := make([]int, 0, len(values))
	hasAuto := false
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if value == "-1" {
			hasAuto = true
			continue
		}
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds < 1 || seconds > 60 {
			return nil, errors.New("invalid_video_duration")
		}
		if !seen[seconds] {
			seen[seconds] = true
			numbers = append(numbers, seconds)
		}
	}
	sort.Ints(numbers)
	result := make([]string, 0, len(numbers)+1)
	for _, seconds := range numbers {
		result = append(result, strconv.Itoa(seconds))
	}
	if hasAuto {
		result = append(result, "-1")
	}
	return result, nil
}

func encodeVideoDurations(values []string) string {
	payload, _ := json.Marshal(values)
	return string(payload)
}

func videoDurationContains(values []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func missingVideoDurations(required, supported []string) []string {
	missing := make([]string, 0)
	for _, value := range required {
		if !videoDurationContains(supported, value) {
			missing = append(missing, value)
		}
	}
	return missing
}

func intersectVideoDurations(left, right []string) []string {
	result := make([]string, 0, len(left))
	for _, value := range left {
		if videoDurationContains(right, value) {
			result = append(result, value)
		}
	}
	return result
}

func recommendedVideoDurations(runtimeModel string, config *ModelConfig) ([]string, bool) {
	runtime := strings.ToLower(strings.TrimSpace(runtimeModel))
	switch {
	case isWuyinGrokImagineModel(runtimeModel, config):
		return []string{"1", "3", "6", "10", "15"}, true
	case isArkSeedanceVideoModel(runtimeModel, config):
		return append([]string(nil), videoModelCapabilities(runtimeModel, config).Durations...), true
	case isZZVideoModel(runtimeModel, config):
		return []string{"15"}, true
	case strings.Contains(runtime, "sora"):
		return []string{"10", "15", "25"}, true
	default:
		return nil, false
	}
}

func (a *App) ensureVideoDurationCapabilityColumns() error {
	migrator := a.db.Migrator()
	checks := []struct {
		model any
		field string
	}{
		{&ModelCatalog{}, "VideoDurationsJSON"},
		{&ModelCatalog{}, "DefaultVideoDuration"},
		{&ModelChannel{}, "VideoDurationsJSON"},
	}
	for _, check := range checks {
		if migrator.HasColumn(check.model, check.field) {
			continue
		}
		if err := migrator.AddColumn(check.model, check.field); err != nil {
			return err
		}
	}
	return nil
}

func backfillVideoDurationCapabilities(tx *gorm.DB) error {
	var channels []ModelChannel
	if err := tx.Preload("Model").Where("status = ?", ModelCenterStatusOnline).Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		channel := &channels[i]
		if channel.Model.Modality != ModelConfigTypeVideo || len(channel.VideoDurations) > 0 {
			continue
		}
		var config *ModelConfig
		if channel.LegacyModelConfigID != 0 {
			var legacy ModelConfig
			if err := tx.First(&legacy, channel.LegacyModelConfigID).Error; err == nil {
				config = &legacy
			}
		}
		if values, ok := recommendedVideoDurations(channel.RuntimeModel, config); ok {
			normalized, _ := normalizeVideoDurations(values)
			if err := tx.Model(channel).Updates(map[string]any{"video_durations_json": encodeVideoDurations(normalized)}).Error; err != nil {
				return err
			}
			channel.VideoDurations = normalized
		}
	}

	var models []ModelCatalog
	if err := tx.Where("modality = ?", ModelConfigTypeVideo).Find(&models).Error; err != nil {
		return err
	}
	for i := range models {
		model := &models[i]
		if len(model.VideoDurations) > 0 {
			continue
		}
		var modelChannels []ModelChannel
		if err := tx.Where("model_id = ? AND status = ?", model.ID, ModelCenterStatusOnline).Order("priority asc, id asc").Find(&modelChannels).Error; err != nil {
			return err
		}
		var intersection []string
		hasUnknownChannel := false
		for _, channel := range modelChannels {
			if len(channel.VideoDurations) == 0 {
				hasUnknownChannel = true
				break
			}
			if intersection == nil {
				intersection = append([]string(nil), channel.VideoDurations...)
			} else {
				intersection = intersectVideoDurations(intersection, channel.VideoDurations)
			}
		}
		if hasUnknownChannel || len(intersection) == 0 {
			continue
		}
		defaultDuration := ""
		for _, channel := range modelChannels {
			var config ModelConfig
			if channel.LegacyModelConfigID != 0 && tx.First(&config, channel.LegacyModelConfigID).Error == nil {
				candidate := defaultVideoDurationForModel(channel.RuntimeModel, &config)
				if videoDurationContains(intersection, candidate) {
					defaultDuration = candidate
					break
				}
			}
		}
		if defaultDuration == "" {
			defaultDuration = intersection[0]
		}
		if err := tx.Model(model).Updates(map[string]any{
			"video_durations_json":   encodeVideoDurations(intersection),
			"default_video_duration": defaultDuration,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateModelVideoDurationCompatibility(tx *gorm.DB, model ModelCatalog) error {
	if model.Modality != ModelConfigTypeVideo || model.Status != ModelCenterStatusOnline || len(model.VideoDurations) == 0 {
		return nil
	}
	var channels []ModelChannel
	if err := tx.Where("model_id = ? AND status = ?", model.ID, ModelCenterStatusOnline).Order("priority asc, id asc").Find(&channels).Error; err != nil {
		return err
	}
	for _, channel := range channels {
		supported := channel.VideoDurations
		if len(supported) == 0 {
			if inferred, ok := recommendedVideoDurations(channel.RuntimeModel, nil); ok {
				supported = inferred
			}
		}
		if missing := missingVideoDurations(model.VideoDurations, supported); len(missing) > 0 {
			return &videoDurationCapabilityConflict{ChannelID: channel.ID, ChannelName: channel.Name, Missing: missing}
		}
	}
	return nil
}

func validateChannelVideoDurationCompatibility(tx *gorm.DB, channel ModelChannel) error {
	if channel.Status != ModelCenterStatusOnline {
		return nil
	}
	var model ModelCatalog
	if err := tx.First(&model, channel.ModelID).Error; err != nil {
		return err
	}
	if model.Modality != ModelConfigTypeVideo || model.Status != ModelCenterStatusOnline || len(model.VideoDurations) == 0 {
		return nil
	}
	if missing := missingVideoDurations(model.VideoDurations, channel.VideoDurations); len(missing) > 0 {
		return &videoDurationCapabilityConflict{ChannelID: channel.ID, ChannelName: channel.Name, Missing: missing}
	}
	return nil
}

func modelCenterVideoDurationConflictPayload(err error) map[string]any {
	payload := map[string]any{"code": "video_duration_capability_conflict", "message": "业务模型公开时长必须被所有启用渠道支持"}
	var conflict *videoDurationCapabilityConflict
	if errors.As(err, &conflict) {
		payload["channel_id"] = conflict.ChannelID
		payload["channel_name"] = conflict.ChannelName
		payload["missing_durations"] = conflict.Missing
	}
	return payload
}

func (a *App) resolvedVideoModelCapability(runtimeModel string, config *ModelConfig) (videoModelCapability, string, error) {
	capability := videoModelCapabilities(runtimeModel, config)
	defaultDuration := defaultVideoDurationForModel(runtimeModel, config)
	if config == nil || config.ID == 0 {
		return capability, defaultDuration, nil
	}
	var channel ModelChannel
	err := a.db.Preload("Model").Where("legacy_model_config_id = ?", config.ID).First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return capability, defaultDuration, nil
	}
	if err != nil {
		return capability, defaultDuration, err
	}
	if len(channel.Model.VideoDurations) > 0 {
		capability.Durations = append([]string(nil), channel.Model.VideoDurations...)
		if videoDurationContains(capability.Durations, channel.Model.DefaultVideoDuration) {
			defaultDuration = channel.Model.DefaultVideoDuration
		} else {
			defaultDuration = capability.Durations[0]
		}
	}
	return capability, defaultDuration, nil
}

func channelSupportsVideoDuration(channel ModelChannel, duration string) bool {
	values := channel.VideoDurations
	if len(values) == 0 {
		if inferred, ok := recommendedVideoDurations(channel.RuntimeModel, nil); ok {
			values = inferred
		} else {
			values = videoModelCapabilities(channel.RuntimeModel, nil).Durations
		}
	}
	return videoDurationContains(values, duration)
}

func (a *App) legacyVideoModelHasCompatibleOnlineChannel(config *ModelConfig, duration string) (bool, bool, error) {
	if config == nil || config.ID == 0 {
		return false, false, nil
	}
	var legacy ModelChannel
	err := a.db.Where("legacy_model_config_id = ?", config.ID).First(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	var channels []ModelChannel
	if err := a.db.Where("model_id = ? AND status = ?", legacy.ModelID, ModelCenterStatusOnline).Find(&channels).Error; err != nil {
		return false, false, err
	}
	if len(channels) == 0 {
		return false, false, nil
	}
	for _, channel := range channels {
		if channelSupportsVideoDuration(channel, duration) {
			return true, true, nil
		}
	}
	return true, false, nil
}
