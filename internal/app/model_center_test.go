package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestAdminModelCenterOverviewMigratesSeedModelsAndMasksProviderKeys(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/model-center/overview", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected model center overview 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Summary struct {
			Models    int64 `json:"models"`
			Providers int64 `json:"providers"`
			Channels  int64 `json:"channels"`
		} `json:"summary"`
		Models []struct {
			ID                 uint   `json:"id"`
			Name               string `json:"name"`
			Modality           string `json:"modality"`
			DefaultCreditsCost int    `json:"default_credits_cost"`
		} `json:"models"`
		Providers []struct {
			ID        uint   `json:"id"`
			Name      string `json:"name"`
			APIKey    string `json:"api_key"`
			APIKeySet bool   `json:"api_key_set"`
		} `json:"providers"`
		Channels []struct {
			ID           uint   `json:"id"`
			ModelID      uint   `json:"model_id"`
			ProviderID   uint   `json:"provider_id"`
			Name         string `json:"name"`
			RuntimeModel string `json:"runtime_model"`
		} `json:"channels"`
		Routing []struct {
			Modality       string `json:"modality"`
			DefaultModelID uint   `json:"default_model_id"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode overview payload: %v", err)
	}
	if payload.Summary.Models == 0 || payload.Summary.Providers == 0 || payload.Summary.Channels == 0 {
		t.Fatalf("expected migrated model center seed data, got %+v", payload.Summary)
	}
	if len(payload.Models) == 0 || len(payload.Channels) == 0 || len(payload.Routing) == 0 {
		t.Fatalf("expected models, channels and routing in overview, got %+v", payload)
	}
	for _, provider := range payload.Providers {
		if provider.APIKey != "" {
			t.Fatalf("provider API key must not be returned in plaintext, got %+v", provider)
		}
	}
}

func TestEnsureModelCenterRepairsLegacySoundtrackModalityAndAudioRouting(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	var config ModelConfig
	if err := db.Where("runtime_model = ?", "music-for-video").First(&config).Error; err != nil {
		t.Fatalf("load soundtrack model config: %v", err)
	}
	if err := db.Model(&ModelConfig{}).Where("id = ?", config.ID).Update("type", ModelConfigTypeImage).Error; err != nil {
		t.Fatalf("corrupt soundtrack legacy type: %v", err)
	}
	config.Type = ModelConfigTypeImage
	var channel ModelChannel
	if err := db.Where("legacy_model_config_id = ?", config.ID).First(&channel).Error; err != nil {
		t.Fatalf("load soundtrack channel: %v", err)
	}
	if err := db.Model(&ModelCatalog{}).Where("id = ?", channel.ModelID).Update("modality", ModelConfigTypeImage).Error; err != nil {
		t.Fatalf("corrupt soundtrack modality: %v", err)
	}
	var policies []ModelRoutingPolicy
	if err := db.Where("modality = ?", ModelConfigTypeAudio).Find(&policies).Error; err != nil {
		t.Fatalf("load audio policies: %v", err)
	}
	for _, policy := range policies {
		if err := db.Where("policy_id = ?", policy.ID).Delete(&ModelRoutingEntry{}).Error; err != nil {
			t.Fatalf("delete audio routing entries: %v", err)
		}
	}
	if err := db.Where("modality = ?", ModelConfigTypeAudio).Delete(&ModelRoutingPolicy{}).Error; err != nil {
		t.Fatalf("delete audio routing policy: %v", err)
	}

	if err := testApp.ensureModelCenter(); err != nil {
		t.Fatalf("repair model center: %v", err)
	}
	if err := db.Preload("Model").Where("legacy_model_config_id = ?", config.ID).First(&channel).Error; err != nil {
		t.Fatalf("reload soundtrack channel: %v", err)
	}
	if channel.Model.Modality != ModelConfigTypeAudio {
		t.Fatalf("expected soundtrack channel to use audio model, got %+v", channel.Model)
	}
	var policy ModelRoutingPolicy
	if err := db.Where("modality = ?", ModelConfigTypeAudio).First(&policy).Error; err != nil {
		t.Fatalf("load repaired audio routing policy: %v", err)
	}
	if policy.DefaultModelID != channel.ModelID {
		t.Fatalf("expected audio policy to default to soundtrack model %d, got %d", channel.ModelID, policy.DefaultModelID)
	}
	var settings AppSettings
	if err := db.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	candidates, err := testApp.modelCenterCandidatesForGeneration(settings, ModelConfigTypeAudio, 0)
	if err != nil || len(candidates) == 0 || candidates[0].Channel.ID != channel.ID {
		t.Fatalf("expected repaired soundtrack candidate, got %+v err=%v", candidates, err)
	}
}

func TestDeleteModelCenterProviderCascadesLegacyChannelsAndPreventsResync(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	config := seedLegacyModelCenterConfig(t, testApp, "Provider Cascade Runtime", "ProviderCascadeVendor", "https://provider-cascade.example", "provider-cascade-key")

	overview := loadModelCenterOverviewForTest(t, testApp, adminCookies)
	providerID := findProviderIDByName(t, overview, "ProviderCascadeVendor")
	channelID := findChannelIDByLegacyConfig(t, overview, config.ID)
	assertModelRoutingEntryCount(t, testApp, channelID, 1)

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/model-center/providers/"+itoa(providerID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected provider delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	overview = loadModelCenterOverviewForTest(t, testApp, adminCookies)
	if providerIDByName(overview, "ProviderCascadeVendor") != 0 {
		t.Fatalf("expected deleted legacy provider to stay hidden after overview sync, got providers %+v", overview.Providers)
	}
	if channelIDByLegacyConfig(overview, config.ID) != 0 {
		t.Fatalf("expected provider delete to remove related legacy channel after overview sync, got channels %+v", overview.Channels)
	}
	assertModelRoutingEntryCount(t, testApp, channelID, 0)

	var activeProviders int64
	if err := testApp.db.Model(&ModelProvider{}).
		Where("name = ? AND provider = ? AND base_url = ? AND api_key = ?", "ProviderCascadeVendor", "providercascadevendor", "https://provider-cascade.example", "provider-cascade-key").
		Count(&activeProviders).Error; err != nil {
		t.Fatalf("count active providers: %v", err)
	}
	if activeProviders != 0 {
		t.Fatalf("expected no active provider recreated from legacy config, got %d", activeProviders)
	}
	var activeChannels int64
	if err := testApp.db.Model(&ModelChannel{}).Where("legacy_model_config_id = ?", config.ID).Count(&activeChannels).Error; err != nil {
		t.Fatalf("count active channels: %v", err)
	}
	if activeChannels != 0 {
		t.Fatalf("expected no active channel recreated from legacy config, got %d", activeChannels)
	}
}

func TestDeleteModelCenterChannelPreventsLegacySyncRecreate(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	config := seedLegacyModelCenterConfig(t, testApp, "Channel Tombstone Runtime", "ChannelTombstoneVendor", "https://channel-tombstone.example", "channel-tombstone-key")

	overview := loadModelCenterOverviewForTest(t, testApp, adminCookies)
	channelID := findChannelIDByLegacyConfig(t, overview, config.ID)
	assertModelRoutingEntryCount(t, testApp, channelID, 1)

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/model-center/channels/"+itoa(channelID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected channel delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	overview = loadModelCenterOverviewForTest(t, testApp, adminCookies)
	if channelIDByLegacyConfig(overview, config.ID) != 0 {
		t.Fatalf("expected deleted legacy channel to stay hidden after overview sync, got channels %+v", overview.Channels)
	}
	assertModelRoutingEntryCount(t, testApp, channelID, 0)
}

func TestUpdateLegacyModelCenterChannelPersistsThroughOverviewSync(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	config := seedLegacyModelCenterConfig(t, testApp, "Legacy Runtime", "LegacyVendor", "https://legacy.example", "legacy-key")

	overview := loadModelCenterOverviewForTest(t, testApp, adminCookies)
	channelID := findChannelIDByLegacyConfig(t, overview, config.ID)
	var initialChannel modelCenterOverviewTestChannel
	for _, channel := range overview.Channels {
		if channel.ID == channelID {
			initialChannel = channel
			break
		}
	}
	if initialChannel.ModelID == 0 {
		t.Fatalf("expected legacy channel model id in overview, got %+v", overview.Channels)
	}
	providerID := createModelCenterProvider(t, testApp, adminCookies, map[string]any{
		"name":     "UpdatedVendor",
		"provider": "updatedvendor",
		"base_url": "https://updated.example",
		"api_key":  "updated-key",
		"status":   "online",
	})

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-center/channels/"+itoa(channelID), map[string]any{
		"model_id":      initialChannel.ModelID,
		"provider_id":   providerID,
		"name":          "Updated Legacy Channel",
		"runtime_model": "updated-runtime",
		"endpoint":      "/v1/updated",
		"weight":        77,
		"priority":      3,
		"status":        "offline",
		"health_status": "degraded",
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected channel update 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	overview = loadModelCenterOverviewForTest(t, testApp, adminCookies)
	var updated modelCenterOverviewTestChannel
	for _, channel := range overview.Channels {
		if channel.ID == channelID {
			updated = channel
			break
		}
	}
	if updated.Name != "Updated Legacy Channel" ||
		updated.RuntimeModel != "updated-runtime" ||
		updated.Endpoint != "/v1/updated" ||
		updated.Weight != 77 ||
		updated.Priority != 3 ||
		updated.Status != "offline" ||
		updated.ProviderID != providerID {
		t.Fatalf("expected updated legacy channel to survive overview sync, got %+v", updated)
	}

	var configAfter ModelConfig
	if err := testApp.db.First(&configAfter, config.ID).Error; err != nil {
		t.Fatalf("load updated legacy model config: %v", err)
	}
	if configAfter.Name != "Updated Legacy Channel" ||
		configAfter.Provider != "UpdatedVendor" ||
		configAfter.RuntimeModel != "updated-runtime" ||
		configAfter.APIBaseURL != "https://updated.example" ||
		configAfter.APIEndpoint != "/v1/updated" ||
		configAfter.APIKey != "updated-key" ||
		configAfter.Weight != 77 ||
		configAfter.Priority != 3 ||
		configAfter.Status != ModelConfigStatusOffline {
		t.Fatalf("expected legacy model config to mirror channel update, got %+v", configAfter)
	}
}

func TestModelCenterGenerationUsesBusinessModelChannelAndSnapshotsCredits(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("model-center-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_model_center",
		},
	}
	testApp, _ := newTestApp(t, provider)
	adminCookies := createAdminSession(t, testApp)
	user, userCookies := createLoggedInUser(t, testApp, "model_center_creator", "test-password")
	setUserCredits(t, testApp, user.ID, 10)

	modelID := createModelCenterModel(t, testApp, adminCookies, map[string]any{
		"name":                 "GPT Image 2",
		"modality":             "image",
		"status":               "online",
		"visibility":           "public",
		"default_credits_cost": 3,
		"capability_tags":      []string{"image", "reference"},
		"sort_order":           1,
	})
	providerID := createModelCenterProvider(t, testApp, adminCookies, map[string]any{
		"name":                    "OpenAI 官方",
		"provider":                "openai",
		"base_url":                "https://official.example",
		"api_key":                 "official-secret",
		"default_timeout_seconds": 45,
		"concurrency_limit":       2,
		"status":                  "online",
	})
	channelID := createModelCenterChannel(t, testApp, adminCookies, map[string]any{
		"model_id":      modelID,
		"provider_id":   providerID,
		"name":          "官方直连",
		"runtime_model": "gpt-image-2",
		"endpoint":      "/v1/images/generations",
		"weight":        100,
		"priority":      1,
		"status":        "online",
		"health_status": "healthy",
	})

	routeResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-center/routing", map[string]any{
		"routes": []map[string]any{
			{
				"modality":          "image",
				"default_model_id":  modelID,
				"fallback_model_id": modelID,
				"routing_enabled":   true,
				"routing_strategy":  "default",
				"entries": []map[string]any{
					{"model_id": modelID, "channel_id": channelID, "enabled": true, "weight": 100, "priority": 1},
				},
			},
		},
	}, adminCookies)
	if routeResp.Code != http.StatusOK {
		t.Fatalf("expected routing save 200, got %d: %s", routeResp.Code, routeResp.Body.String())
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "模型中心路由生成",
		"aspect_ratio": "1:1",
	}, userCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.inputs))
	}
	if provider.inputs[0].Model != "gpt-image-2" || provider.inputs[0].ProviderBaseURL != "https://official.example" || provider.inputs[0].ProviderAPIKey != "official-secret" {
		t.Fatalf("expected provider request to use channel runtime/provider credentials, got %+v", provider.inputs[0])
	}

	var generationPayload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &generationPayload); err != nil {
		t.Fatalf("decode generation response: %v", err)
	}
	var record GenerationRecord
	if err := testApp.db.First(&record, generationPayload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ModelID != modelID || record.ChannelID != channelID {
		t.Fatalf("expected model/channel ids snapshotted, got model_id=%d channel_id=%d", record.ModelID, record.ChannelID)
	}
	if record.ModelName != "GPT Image 2" || record.ChannelName != "官方直连" || record.RuntimeModel != "gpt-image-2" || record.CreditsCost != 1 {
		t.Fatalf("expected model/channel/runtime/credits snapshots, got %+v", record)
	}

	var attempt ModelCallAttempt
	if err := testApp.db.Where("generation_record_id = ?", record.ID).First(&attempt).Error; err != nil {
		t.Fatalf("load model call attempt: %v", err)
	}
	if attempt.ChannelID != channelID {
		t.Fatalf("expected attempt to be attributed by channel_id, got %+v", attempt)
	}
}

func TestModelCenterAllowsZeroCreditsOnlyForInternalChatModels(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	internalChat := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/model-center/models", map[string]any{
		"name":                 "AI 电商视觉分析",
		"modality":             "chat",
		"status":               "online",
		"visibility":           "internal",
		"default_credits_cost": 0,
		"capability_tags":      []string{"vision", "commerce_vision"},
	}, adminCookies)
	if internalChat.Code != http.StatusCreated {
		t.Fatalf("expected internal chat model with zero credits to be created, got %d: %s", internalChat.Code, internalChat.Body.String())
	}

	invalidCreates := []struct {
		name, modality, visibility string
	}{
		{"免费公开图片模型", "image", "public"},
		{"免费内部图片模型", "image", "internal"},
		{"免费视频模型", "video", "internal"},
		{"免费公开文本模型", "chat", "public"},
	}
	for _, tc := range invalidCreates {
		resp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/model-center/models", map[string]any{
			"name":                 tc.name,
			"modality":             tc.modality,
			"status":               "online",
			"visibility":           tc.visibility,
			"default_credits_cost": 0,
			"capability_tags":      []string{tc.modality},
		}, adminCookies)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected %s/%s model with zero credits to be rejected, got %d: %s", tc.modality, tc.visibility, resp.Code, resp.Body.String())
		}
	}

	var created ModelCatalog
	if err := db.Where("name = ?", "AI 电商视觉分析").First(&created).Error; err != nil {
		t.Fatalf("load created internal chat model: %v", err)
	}
	invalidUpdate := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-center/models/"+itoa(created.ID), map[string]any{
		"visibility": "public",
	}, adminCookies)
	if invalidUpdate.Code != http.StatusBadRequest {
		t.Fatalf("expected zero-credit chat model becoming public to be rejected, got %d: %s", invalidUpdate.Code, invalidUpdate.Body.String())
	}
}

func TestModelCenterRejectsVideoDurationCapabilityConflictsWithDetails(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	modelID := createModelCenterModel(t, testApp, adminCookies, map[string]any{
		"name":                   "Duration Routed Video",
		"modality":               ModelConfigTypeVideo,
		"status":                 ModelCenterStatusOnline,
		"visibility":             ModelCenterVisibilityPublic,
		"default_credits_cost":   1,
		"capability_tags":        []string{"video"},
		"video_durations":        []string{"6", "10", "15"},
		"default_video_duration": "10",
	})
	providerID := createModelCenterProvider(t, testApp, adminCookies, map[string]any{
		"name":     "Duration Provider",
		"provider": "duration-provider",
		"api_key":  "duration-test-key",
		"status":   ModelCenterStatusOnline,
	})
	channelID := createModelCenterChannel(t, testApp, adminCookies, map[string]any{
		"model_id":        modelID,
		"provider_id":     providerID,
		"name":            "Duration Channel",
		"runtime_model":   "grok-imagine-video",
		"video_durations": []string{"6", "10", "15"},
		"weight":          100,
		"priority":        1,
		"status":          ModelCenterStatusOnline,
		"health_status":   ModelChannelHealthHealthy,
	})

	conflict := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-center/models/"+itoa(modelID), map[string]any{
		"video_durations":        []string{"1", "6", "10", "15"},
		"default_video_duration": "10",
	}, adminCookies)
	if conflict.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected model duration conflict 422, got %d: %s", conflict.Code, conflict.Body.String())
	}
	var payload struct {
		Error struct {
			Code             string   `json:"code"`
			ChannelID        uint     `json:"channel_id"`
			MissingDurations []string `json:"missing_durations"`
		} `json:"error"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if payload.Error.Code != "video_duration_capability_conflict" || payload.Error.ChannelID != channelID || len(payload.Error.MissingDurations) != 1 || payload.Error.MissingDurations[0] != "1" {
		t.Fatalf("unexpected conflict response: %+v", payload.Error)
	}

	channelConflict := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-center/channels/"+itoa(channelID), map[string]any{
		"video_durations": []string{"10", "15"},
	}, adminCookies)
	if channelConflict.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected channel duration conflict 422, got %d: %s", channelConflict.Code, channelConflict.Body.String())
	}
}

func TestListModelCenterChannelCallAttemptsFiltersAndPaginates(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	modelID := createModelCenterModel(t, testApp, adminCookies, map[string]any{
		"name":                 "生图模型2",
		"modality":             "image",
		"status":               "online",
		"visibility":           "public",
		"default_credits_cost": 3,
		"capability_tags":      []string{"image"},
		"sort_order":           1,
	})
	otherModelID := createModelCenterModel(t, testApp, adminCookies, map[string]any{
		"name":                 "生图模型3",
		"modality":             "image",
		"status":               "online",
		"visibility":           "public",
		"default_credits_cost": 4,
		"capability_tags":      []string{"image"},
		"sort_order":           2,
	})
	providerID := createModelCenterProvider(t, testApp, adminCookies, map[string]any{
		"name":                    "OpenAI 官方",
		"provider":                "openai",
		"base_url":                "https://official.example",
		"api_key":                 "official-secret",
		"default_timeout_seconds": 45,
		"concurrency_limit":       2,
		"status":                  "online",
	})
	channelID := createModelCenterChannel(t, testApp, adminCookies, map[string]any{
		"model_id":      modelID,
		"provider_id":   providerID,
		"name":          "gpt-image-2",
		"runtime_model": "gpt-image-2",
		"endpoint":      "/v1/images/generations",
		"weight":        100,
		"priority":      1,
		"status":        "online",
		"health_status": "healthy",
	})
	otherChannelID := createModelCenterChannel(t, testApp, adminCookies, map[string]any{
		"model_id":      modelID,
		"provider_id":   providerID,
		"name":          "备用线路",
		"runtime_model": "gpt-image-2-backup",
		"endpoint":      "/v1/images/generations",
		"weight":        0,
		"priority":      2,
		"status":        "online",
		"health_status": "healthy",
	})

	base := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	oldRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{ModelID: modelID, ChannelID: channelID, Status: GenerationStatusFailed, CreatedAt: base.Add(-48 * time.Hour)})
	firstRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{ModelID: modelID, ChannelID: channelID, Status: GenerationStatusFailed, CreatedAt: base.Add(-2 * time.Hour)})
	secondRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{ModelID: modelID, ChannelID: channelID, Status: GenerationStatusSucceeded, CreatedAt: base.Add(-time.Hour)})
	thirdRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{ModelID: modelID, ChannelID: channelID, Status: GenerationStatusFailed, CreatedAt: base})
	otherModelRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{ModelID: otherModelID, ChannelID: channelID, Status: GenerationStatusFailed, CreatedAt: base.Add(time.Hour)})
	otherChannelRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{ModelID: modelID, ChannelID: otherChannelID, Status: GenerationStatusFailed, CreatedAt: base.Add(2 * time.Hour)})

	seedModelCallAttempt(t, testApp, ModelCallAttempt{GenerationRecordID: oldRecord.ID, ChannelID: channelID, ModelConfigID: 1, AttemptIndex: 1, Status: ModelCallAttemptStatusFailed, LatencyMS: 1200, HTTPStatus: http.StatusBadGateway, ErrorCode: "old_failed", ErrorMessage: "old upstream failed", FailureStage: "image_generation_request", ProviderRequestID: "req_old", StartedAt: base.AddDate(0, 0, -2), FinishedAt: base.AddDate(0, 0, -2).Add(time.Second)})
	seedModelCallAttempt(t, testApp, ModelCallAttempt{GenerationRecordID: firstRecord.ID, ChannelID: channelID, ModelConfigID: 2, AttemptIndex: 1, Status: ModelCallAttemptStatusFailed, LatencyMS: 980, HTTPStatus: http.StatusBadGateway, ErrorCode: "provider_http_502", ErrorMessage: "upstream failed", FailureStage: "image_generation_request", ProviderRequestID: "req_failed_1", StartedAt: base.Add(-2 * time.Hour), FinishedAt: base.Add(-2*time.Hour + time.Second)})
	secondAttempt := seedModelCallAttempt(t, testApp, ModelCallAttempt{GenerationRecordID: secondRecord.ID, ChannelID: channelID, ModelConfigID: 3, AttemptIndex: 2, Status: ModelCallAttemptStatusSucceeded, LatencyMS: 860, HTTPStatus: http.StatusOK, ProviderRequestID: "req_success", StartedAt: base.Add(-time.Hour), FinishedAt: base.Add(-time.Hour + time.Second)})
	thirdAttempt := seedModelCallAttempt(t, testApp, ModelCallAttempt{GenerationRecordID: thirdRecord.ID, ChannelID: channelID, ModelConfigID: 3, AttemptIndex: 3, Status: ModelCallAttemptStatusFailed, LatencyMS: 660, HTTPStatus: http.StatusGatewayTimeout, ErrorCode: "provider_timeout", ErrorMessage: "provider timeout", FailureStage: "image_generation_request", ProviderRequestID: "req_failed_2", StartedAt: base, FinishedAt: base.Add(time.Second)})
	seedModelCallAttempt(t, testApp, ModelCallAttempt{GenerationRecordID: otherModelRecord.ID, ChannelID: channelID, ModelConfigID: 4, AttemptIndex: 1, Status: ModelCallAttemptStatusFailed, LatencyMS: 500, HTTPStatus: http.StatusBadGateway, ErrorCode: "other_model", ProviderRequestID: "req_other_model", StartedAt: base.Add(time.Hour), FinishedAt: base.Add(time.Hour + time.Second)})
	seedModelCallAttempt(t, testApp, ModelCallAttempt{GenerationRecordID: otherChannelRecord.ID, ChannelID: otherChannelID, ModelConfigID: 5, AttemptIndex: 1, Status: ModelCallAttemptStatusFailed, LatencyMS: 400, HTTPStatus: http.StatusBadGateway, ErrorCode: "other_channel", ProviderRequestID: "req_other_channel", StartedAt: base.Add(2 * time.Hour), FinishedAt: base.Add(2*time.Hour + time.Second)})

	pageResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/model-center/channels/%d/call-attempts?model_id=%d&page=1&page_size=2", channelID, modelID), nil, adminCookies)
	if pageResp.Code != http.StatusOK {
		t.Fatalf("expected call attempts page 200, got %d: %s", pageResp.Code, pageResp.Body.String())
	}
	var pagePayload struct {
		Channel struct {
			ID           uint   `json:"id"`
			ModelID      uint   `json:"model_id"`
			ModelName    string `json:"model_name"`
			ProviderID   uint   `json:"provider_id"`
			ProviderName string `json:"provider_name"`
			Name         string `json:"name"`
			RuntimeModel string `json:"runtime_model"`
			Endpoint     string `json:"endpoint"`
		} `json:"channel"`
		ModelID uint `json:"model_id"`
		Items   []struct {
			ID                 uint   `json:"id"`
			GenerationRecordID uint   `json:"generation_record_id"`
			ModelID            uint   `json:"model_id"`
			ChannelID          uint   `json:"channel_id"`
			ModelConfigID      uint   `json:"model_config_id"`
			AttemptIndex       int    `json:"attempt_index"`
			Status             string `json:"status"`
			LatencyMS          int64  `json:"latency_ms"`
			HTTPStatus         int    `json:"http_status"`
			ErrorCode          string `json:"error_code"`
			ErrorMessage       string `json:"error_message"`
			FailureStage       string `json:"failure_stage"`
			ProviderRequestID  string `json:"provider_request_id"`
			StartedAt          string `json:"started_at"`
			FinishedAt         string `json:"finished_at"`
			CreatedAt          string `json:"created_at"`
		} `json:"items"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Total    int64 `json:"total"`
	}
	if err := json.Unmarshal(pageResp.Body.Bytes(), &pagePayload); err != nil {
		t.Fatalf("decode call attempts page: %v", err)
	}
	if pagePayload.Channel.ID != channelID || pagePayload.Channel.ModelName != "生图模型2" || pagePayload.Channel.ProviderName != "OpenAI 官方" || pagePayload.ModelID != modelID {
		t.Fatalf("unexpected channel metadata: %+v", pagePayload.Channel)
	}
	if pagePayload.Page != 1 || pagePayload.PageSize != 2 || pagePayload.Total != 4 || len(pagePayload.Items) != 2 {
		t.Fatalf("unexpected pagination payload: %+v", pagePayload)
	}
	if pagePayload.Items[0].ID != thirdAttempt.ID || pagePayload.Items[1].ID != secondAttempt.ID {
		t.Fatalf("expected newest attempts first, got %+v", pagePayload.Items)
	}
	if pagePayload.Items[0].GenerationRecordID != thirdRecord.ID || pagePayload.Items[0].ModelID != modelID || pagePayload.Items[0].ChannelID != channelID || pagePayload.Items[0].ErrorCode != "provider_timeout" || pagePayload.Items[0].FailureStage != "image_generation_request" || pagePayload.Items[0].ProviderRequestID != "req_failed_2" || pagePayload.Items[0].StartedAt == "" || pagePayload.Items[0].FinishedAt == "" || pagePayload.Items[0].CreatedAt == "" {
		t.Fatalf("unexpected first attempt diagnostics: %+v", pagePayload.Items[0])
	}

	filterResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/model-center/channels/%d/call-attempts?model_id=%d&status=failed&date_from=2026-05-18&date_to=2026-05-18&page=1&page_size=20", channelID, modelID), nil, adminCookies)
	if filterResp.Code != http.StatusOK {
		t.Fatalf("expected filtered attempts 200, got %d: %s", filterResp.Code, filterResp.Body.String())
	}
	var filterPayload struct {
		Items []struct {
			ID     uint   `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(filterResp.Body.Bytes(), &filterPayload); err != nil {
		t.Fatalf("decode filtered payload: %v", err)
	}
	if filterPayload.Total != 2 || len(filterPayload.Items) != 2 || filterPayload.Items[0].ID != thirdAttempt.ID || filterPayload.Items[0].Status != ModelCallAttemptStatusFailed {
		t.Fatalf("expected failed attempts from the selected date only, got %+v", filterPayload)
	}
}

func TestListModelCenterChannelCallAttemptsErrors(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	missingResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/model-center/channels/999999/call-attempts", nil, adminCookies)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing channel 404, got %d: %s", missingResp.Code, missingResp.Body.String())
	}
	if !jsonErrorCode(missingResp.Body.Bytes(), "model_center_channel_not_found") {
		t.Fatalf("expected model_center_channel_not_found error, got %s", missingResp.Body.String())
	}

	modelID := createModelCenterModel(t, testApp, adminCookies, map[string]any{
		"name":                 "生图模型2",
		"modality":             "image",
		"status":               "online",
		"visibility":           "public",
		"default_credits_cost": 3,
		"capability_tags":      []string{"image"},
		"sort_order":           1,
	})
	providerID := createModelCenterProvider(t, testApp, adminCookies, map[string]any{
		"name":    "OpenAI 官方",
		"api_key": "official-secret",
		"status":  "online",
	})
	channelID := createModelCenterChannel(t, testApp, adminCookies, map[string]any{
		"model_id":      modelID,
		"provider_id":   providerID,
		"name":          "gpt-image-2",
		"runtime_model": "gpt-image-2",
		"weight":        100,
		"priority":      1,
		"status":        "online",
		"health_status": "healthy",
	})

	invalidDateResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/model-center/channels/%d/call-attempts?date_from=2026-99-99", channelID), nil, adminCookies)
	if invalidDateResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid date 400, got %d: %s", invalidDateResp.Code, invalidDateResp.Body.String())
	}
	if !jsonErrorCode(invalidDateResp.Body.Bytes(), "invalid_date") {
		t.Fatalf("expected invalid_date error, got %s", invalidDateResp.Body.String())
	}
}

func createModelCenterModel(t *testing.T, app *App, cookies []*http.Cookie, payload map[string]any) uint {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodPost, "/api/admin/model-center/models", payload, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created model: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected created model id, got %+v", created)
	}
	return created.ID
}

func jsonErrorCode(body []byte, expected string) bool {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Error.Code == expected
}

type modelCenterOverviewTestPayload struct {
	Summary struct {
		Models    int64 `json:"models"`
		Providers int64 `json:"providers"`
		Channels  int64 `json:"channels"`
	} `json:"summary"`
	Providers []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	} `json:"providers"`
	Channels []modelCenterOverviewTestChannel `json:"channels"`
}

type modelCenterOverviewTestChannel struct {
	ID                  uint   `json:"id"`
	ModelID             uint   `json:"model_id"`
	ProviderID          uint   `json:"provider_id"`
	LegacyModelConfigID uint   `json:"legacy_model_config_id"`
	Name                string `json:"name"`
	RuntimeModel        string `json:"runtime_model"`
	Endpoint            string `json:"endpoint"`
	Weight              int    `json:"weight"`
	Priority            int    `json:"priority"`
	Status              string `json:"status"`
}

func seedLegacyModelCenterConfig(t *testing.T, app *App, runtimeModel, provider, baseURL, apiKey string) ModelConfig {
	t.Helper()
	config := ModelConfig{
		Name:         runtimeModel,
		Type:         ModelConfigTypeImage,
		Provider:     provider,
		Status:       ModelConfigStatusOnline,
		Priority:     90,
		CostLabel:    "按配置扣点",
		Permission:   ModelConfigPermissionPublic,
		Weight:       40,
		SortOrder:    90,
		RuntimeModel: runtimeModel,
		APIBaseURL:   baseURL,
		APIEndpoint:  "/v1/images/generations",
		APIKey:       apiKey,
	}
	if err := app.db.Create(&config).Error; err != nil {
		t.Fatalf("create legacy model config: %v", err)
	}
	return config
}

func loadModelCenterOverviewForTest(t *testing.T, app *App, adminCookies []*http.Cookie) modelCenterOverviewTestPayload {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodGet, "/api/admin/model-center/overview", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected model center overview 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload modelCenterOverviewTestPayload
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode overview payload: %v", err)
	}
	return payload
}

func findProviderIDByName(t *testing.T, overview modelCenterOverviewTestPayload, name string) uint {
	t.Helper()
	if id := providerIDByName(overview, name); id != 0 {
		return id
	}
	t.Fatalf("expected provider %q in overview, got %+v", name, overview.Providers)
	return 0
}

func providerIDByName(overview modelCenterOverviewTestPayload, name string) uint {
	for _, provider := range overview.Providers {
		if provider.Name == name {
			return provider.ID
		}
	}
	return 0
}

func findChannelIDByLegacyConfig(t *testing.T, overview modelCenterOverviewTestPayload, legacyConfigID uint) uint {
	t.Helper()
	if id := channelIDByLegacyConfig(overview, legacyConfigID); id != 0 {
		return id
	}
	t.Fatalf("expected legacy channel %d in overview, got %+v", legacyConfigID, overview.Channels)
	return 0
}

func channelIDByLegacyConfig(overview modelCenterOverviewTestPayload, legacyConfigID uint) uint {
	for _, channel := range overview.Channels {
		if channel.LegacyModelConfigID == legacyConfigID {
			return channel.ID
		}
	}
	return 0
}

func assertModelRoutingEntryCount(t *testing.T, app *App, channelID uint, expected int64) {
	t.Helper()
	var count int64
	if err := app.db.Model(&ModelRoutingEntry{}).Where("channel_id = ?", channelID).Count(&count).Error; err != nil {
		t.Fatalf("count model routing entries: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d routing entries for channel %d, got %d", expected, channelID, count)
	}
}

func createModelCenterProvider(t *testing.T, app *App, cookies []*http.Cookie, payload map[string]any) uint {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodPost, "/api/admin/model-center/providers", payload, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		ID        uint   `json:"id"`
		APIKey    string `json:"api_key"`
		APIKeySet bool   `json:"api_key_set"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created provider: %v", err)
	}
	if created.ID == 0 || created.APIKey != "" || !created.APIKeySet {
		t.Fatalf("expected provider id and masked key status, got %+v", created)
	}
	return created.ID
}

func createModelCenterChannel(t *testing.T, app *App, cookies []*http.Cookie, payload map[string]any) uint {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodPost, "/api/admin/model-center/channels", payload, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected channel create 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created channel: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected created channel id, got %+v", created)
	}
	return created.ID
}
