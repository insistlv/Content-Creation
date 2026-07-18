package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeVideoDurationsSortsDeduplicatesAndKeepsAutoLast(t *testing.T) {
	got, err := normalizeVideoDurations([]string{"15", "3", "1", "3", "-1", "10", "6"})
	if err != nil {
		t.Fatalf("normalize video durations: %v", err)
	}
	want := []string{"1", "3", "6", "10", "15", "-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected durations: got %v want %v", got, want)
	}
	for _, invalid := range [][]string{{"0"}, {"61"}, {"1.5"}, {"abc"}} {
		if _, err := normalizeVideoDurations(invalid); err == nil {
			t.Fatalf("expected invalid duration %v to fail", invalid)
		}
	}
}

func TestEnsureVideoDurationCapabilityColumnsIsIdempotent(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatalf("disable sqlite foreign keys: %v", err)
	}
	for _, column := range []struct {
		model any
		name  string
	}{
		{&ModelCatalog{}, "VideoDurationsJSON"},
		{&ModelCatalog{}, "DefaultVideoDuration"},
		{&ModelChannel{}, "VideoDurationsJSON"},
	} {
		if err := db.Migrator().DropColumn(column.model, column.name); err != nil {
			t.Fatalf("drop %s: %v", column.name, err)
		}
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("restore sqlite foreign keys: %v", err)
	}
	if err := testApp.ensureVideoDurationCapabilityColumns(); err != nil {
		t.Fatalf("first schema guard: %v", err)
	}
	if err := testApp.ensureVideoDurationCapabilityColumns(); err != nil {
		t.Fatalf("second schema guard: %v", err)
	}
	for _, column := range []struct {
		model any
		name  string
	}{
		{&ModelCatalog{}, "VideoDurationsJSON"},
		{&ModelCatalog{}, "DefaultVideoDuration"},
		{&ModelChannel{}, "VideoDurationsJSON"},
	} {
		if !db.Migrator().HasColumn(column.model, column.name) {
			t.Fatalf("expected %s to be restored", column.name)
		}
	}
}

func TestVideoDurationCompatibilityRejectsMissingChannelDurations(t *testing.T) {
	_, db := newTestApp(t, &stubProvider{})
	model := ModelCatalog{
		Name:                 "Duration Test Model",
		Modality:             ModelConfigTypeVideo,
		Status:               ModelCenterStatusOnline,
		Visibility:           ModelCenterVisibilityPublic,
		DefaultCreditsCost:   1,
		VideoDurations:       []string{"1", "3", "6"},
		DefaultVideoDuration: "3",
	}
	if err := db.Create(&model).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
	provider := ModelProvider{Name: "Duration Provider", Provider: "duration-test", Status: ModelCenterStatusOnline}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	channel := ModelChannel{
		ModelID:        model.ID,
		ProviderID:     provider.ID,
		Name:           "Limited Channel",
		RuntimeModel:   "grok-imagine-video",
		VideoDurations: []string{"3", "6"},
		Weight:         100,
		Priority:       1,
		Status:         ModelCenterStatusOnline,
		HealthStatus:   ModelChannelHealthHealthy,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	err := validateModelVideoDurationCompatibility(db, model)
	if !errors.Is(err, errVideoDurationCapabilityConflict) {
		t.Fatalf("expected duration capability conflict, got %v", err)
	}
	var conflict *videoDurationCapabilityConflict
	if !errors.As(err, &conflict) || !reflect.DeepEqual(conflict.Missing, []string{"1"}) {
		t.Fatalf("unexpected conflict details: %#v", conflict)
	}
}

func TestGrokVideoDurationRecommendationIncludesConfiguredPublicRange(t *testing.T) {
	values, ok := recommendedVideoDurations(wuyinGrokImagineRuntimeModel, nil)
	if !ok {
		t.Fatal("expected Grok recommendation")
	}
	want := []string{"1", "3", "6", "10", "15"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("unexpected Grok durations: got %v want %v", values, want)
	}
}

func TestListVideoModelsReturnsConfiguredGrokDurationsAndDefault(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list video models: %d %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel    string   `json:"runtime_model"`
			Durations       []string `json:"durations"`
			DefaultDuration string   `json:"default_duration"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel != wuyinGrokImagineRuntimeModel {
			continue
		}
		want := []string{"1", "3", "6", "10", "15"}
		if !reflect.DeepEqual(item.Durations, want) || item.DefaultDuration != "3" {
			t.Fatalf("unexpected Grok duration capability: %+v", item)
		}
		return
	}
	t.Fatalf("Grok model not found in response: %+v", payload.Items)
}

func TestVideoGenerationRejectsUnavailableDurationChannelBeforeDeducting(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "must-not-submit"}}
	testApp.videoProvider = videoProvider
	user, cookies := createLoggedInUser(t, testApp, "duration-channel-unavailable", "test-password")
	setUserCredits(t, testApp, user.ID, 20)

	var config ModelConfig
	if err := db.Where("runtime_model = ?", "sora-2").First(&config).Error; err != nil {
		t.Fatalf("load Sora config: %v", err)
	}
	if err := testApp.ensureModelCenter(); err != nil {
		t.Fatalf("ensure model center: %v", err)
	}
	var legacyChannel ModelChannel
	if err := db.Where("legacy_model_config_id = ?", config.ID).First(&legacyChannel).Error; err != nil {
		t.Fatalf("load Sora channel: %v", err)
	}
	if err := db.Model(&ModelChannel{}).Where("model_id = ?", legacyChannel.ModelID).Update("video_durations_json", `["15"]`).Error; err != nil {
		t.Fatalf("restrict channel durations: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "duration channel must reject before provider",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "sora-2",
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "video_duration_channel_unavailable") {
		t.Fatalf("expected unavailable channel 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("provider must not be called: %+v", videoProvider.submitInputs)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 20 {
		t.Fatalf("credits changed before provider call: %+v", balance)
	}
}
