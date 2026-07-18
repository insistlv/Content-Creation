package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWorkspaceDiscoveryReturnsActiveInspirationRecommendations(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "recommendation_discovery_user", "test-password")
	setUserCredits(t, testApp, user.ID, 7)

	active := InspirationRecommendation{
		Slug:             "weekly-cyber-city",
		Title:            "Cyberpunk City",
		Category:         "concept",
		Description:      "Neon skyline sample",
		PreviewURL:       "https://oss.example.com/recommendations/cyber-city.png",
		Prompt:           "cyberpunk city at rainy night",
		NegativePrompt:   "low quality",
		AspectRatio:      "16:9",
		StylePreset:      "cinematic",
		Theme:            "cyber",
		ToolMode:         GenerationToolModeGenerate,
		WorkspaceModelID: 42,
		SortOrder:        5,
		IsActive:         true,
	}
	if err := active.SetHeatTags([]string{"weekly-hot", "beginner"}); err != nil {
		t.Fatalf("set heat tags: %v", err)
	}
	if err := active.SetParams(map[string]any{"seed": 918, "guidance": 7}); err != nil {
		t.Fatalf("set params: %v", err)
	}
	if err := testApp.db.Create(&active).Error; err != nil {
		t.Fatalf("create active recommendation: %v", err)
	}
	inactive := InspirationRecommendation{
		Slug:        "hidden-recommendation",
		Title:       "Hidden",
		PreviewURL:  "https://oss.example.com/recommendations/hidden.png",
		Prompt:      "hidden prompt",
		AspectRatio: "1:1",
		ToolMode:    GenerationToolModeGenerate,
		SortOrder:   1,
		IsActive:    false,
	}
	if err := testApp.db.Create(&inactive).Error; err != nil {
		t.Fatalf("create inactive recommendation: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/workspace/discovery", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Recommendations []struct {
			ID             uint           `json:"id"`
			Slug           string         `json:"slug"`
			Title          string         `json:"title"`
			HeatTags       []string       `json:"heat_tags"`
			PreviewURL     string         `json:"preview_url"`
			Prompt         string         `json:"prompt"`
			NegativePrompt string         `json:"negative_prompt"`
			AspectRatio    string         `json:"aspect_ratio"`
			StylePreset    string         `json:"style_preset"`
			ToolMode       string         `json:"tool_mode"`
			ModelID        uint           `json:"model_id"`
			Params         map[string]any `json:"params"`
			SortOrder      int            `json:"sort_order"`
			UseCount       int            `json:"use_count"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode discovery payload: %v", err)
	}
	var found *struct {
		ID             uint           `json:"id"`
		Slug           string         `json:"slug"`
		Title          string         `json:"title"`
		HeatTags       []string       `json:"heat_tags"`
		PreviewURL     string         `json:"preview_url"`
		Prompt         string         `json:"prompt"`
		NegativePrompt string         `json:"negative_prompt"`
		AspectRatio    string         `json:"aspect_ratio"`
		StylePreset    string         `json:"style_preset"`
		ToolMode       string         `json:"tool_mode"`
		ModelID        uint           `json:"model_id"`
		Params         map[string]any `json:"params"`
		SortOrder      int            `json:"sort_order"`
		UseCount       int            `json:"use_count"`
	}
	for index := range payload.Recommendations {
		if payload.Recommendations[index].Slug == active.Slug {
			found = &payload.Recommendations[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected active recommendation in discovery payload, got %+v", payload.Recommendations)
	}
	if found.Title != active.Title || found.Prompt != active.Prompt || found.NegativePrompt != active.NegativePrompt ||
		found.AspectRatio != "16:9" || found.StylePreset != "cinematic" || found.ToolMode != GenerationToolModeGenerate ||
		found.ModelID != 42 || found.SortOrder != 5 || found.UseCount != 0 {
		t.Fatalf("unexpected recommendation payload: %+v", *found)
	}
	if len(found.HeatTags) != 2 || found.HeatTags[0] != "weekly-hot" || found.HeatTags[1] != "beginner" {
		t.Fatalf("expected heat tags in order, got %+v", found.HeatTags)
	}
	if found.Params["seed"] != float64(918) || found.Params["guidance"] != float64(7) {
		t.Fatalf("expected params payload, got %+v", found.Params)
	}
	if resp.Body.String() == "" || jsonContains(resp.Body.Bytes(), "hidden-recommendation") {
		t.Fatalf("discovery must not return inactive recommendation: %s", resp.Body.String())
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 7 {
		t.Fatalf("workspace discovery must not deduct credits, got %d", balance.AvailableCredits)
	}
}

func TestUseInspirationRecommendationAllowsAnonymousAndIncrementsUseCount(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	recommendation := InspirationRecommendation{
		Slug:        "anonymous-use",
		Title:       "Anonymous Use",
		PreviewURL:  "https://oss.example.com/recommendations/anonymous.png",
		Prompt:      "anonymous prompt",
		AspectRatio: "1:1",
		ToolMode:    GenerationToolModeGenerate,
		IsActive:    true,
	}
	if err := testApp.db.Create(&recommendation).Error; err != nil {
		t.Fatalf("create recommendation: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/workspace/inspiration-recommendations/"+itoa(recommendation.ID)+"/use", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected use recommendation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		ID       uint `json:"id"`
		UseCount int  `json:"use_count"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode use payload: %v", err)
	}
	if payload.ID != recommendation.ID || payload.UseCount != 1 {
		t.Fatalf("expected incremented use count payload, got %+v", payload)
	}
	var updated InspirationRecommendation
	if err := testApp.db.First(&updated, recommendation.ID).Error; err != nil {
		t.Fatalf("reload recommendation: %v", err)
	}
	if updated.UseCount != 1 {
		t.Fatalf("expected persisted use count, got %+v", updated)
	}

	missingResp := performJSONRequest(t, testApp, http.MethodPost, "/api/workspace/inspiration-recommendations/999999/use", nil, nil)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing recommendation 404, got %d: %s", missingResp.Code, missingResp.Body.String())
	}
}

func TestAdminInspirationRecommendationsCRUD(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/inspiration-recommendations", map[string]any{
		"slug":            "admin-recommendation",
		"title":           "Admin Recommendation",
		"category":        "poster",
		"description":     "Created from admin",
		"heat_tags":       []string{"weekly-hot", "ecommerce"},
		"preview_url":     "https://oss.example.com/recommendations/admin.png",
		"prompt":          "admin prompt",
		"negative_prompt": "bad anatomy",
		"aspect_ratio":    "4:3",
		"style_preset":    "commercial",
		"theme":           "product",
		"tool_mode":       GenerationToolModeGenerate,
		"model_id":        8,
		"params":          map[string]any{"seed": 101},
		"sort_order":      25,
		"is_active":       true,
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create recommendation 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created InspirationRecommendation
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created recommendation: %v", err)
	}
	if created.ID == 0 || created.Slug != "admin-recommendation" || created.Prompt != "admin prompt" ||
		created.PreviewURL == "" || created.SortOrder != 25 || !created.IsActive {
		t.Fatalf("unexpected created recommendation: %+v", created)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/inspiration-recommendations?q=admin&page=1&page_size=5", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list recommendations 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			ID       uint           `json:"id"`
			HeatTags []string       `json:"heat_tags"`
			Params   map[string]any `json:"params"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if listPayload.Total == 0 || len(listPayload.Items) == 0 || listPayload.Items[0].ID != created.ID ||
		len(listPayload.Items[0].HeatTags) != 2 || listPayload.Items[0].Params["seed"] != float64(101) {
		t.Fatalf("expected created recommendation in admin list, got %+v", listPayload)
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/inspiration-recommendations/"+itoa(created.ID), map[string]any{
		"title":      "Updated Recommendation",
		"heat_tags":  []string{"new-user"},
		"params":     map[string]any{"seed": 202, "style_strength": 0.8},
		"sort_order": 7,
		"is_active":  false,
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update recommendation 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	var updated InspirationRecommendation
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated recommendation: %v", err)
	}
	if updated.Title != "Updated Recommendation" || updated.SortOrder != 7 || updated.IsActive {
		t.Fatalf("unexpected updated recommendation: %+v", updated)
	}
	updatedTags := updated.HeatTags()
	if len(updatedTags) != 1 || updatedTags[0] != "new-user" {
		t.Fatalf("expected updated heat tags, got %+v", updatedTags)
	}
	updatedParams := updated.Params()
	if updatedParams["seed"] != float64(202) || updatedParams["style_strength"] != 0.8 {
		t.Fatalf("expected updated params, got %+v", updatedParams)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/inspiration-recommendations/"+itoa(created.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete recommendation 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	var deleted InspirationRecommendation
	if err := testApp.db.Unscoped().First(&deleted, created.ID).Error; err != nil {
		t.Fatalf("load deleted recommendation: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected recommendation to be soft deleted")
	}
}

func TestGenerateMissingInspirationRecommendationPreviewsPersistsOSSURL(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("generated-recommendation-preview")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_recommendation_preview",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "recommendations/generated.png",
		publicURL: "https://oss.example.com/recommendations/generated.png",
	}

	report, err := testApp.GenerateMissingInspirationRecommendationPreviews(context.Background(), InspirationRecommendationPreviewGenerationOptions{
		Slugs: []string{"weekly-cyberpunk-city"},
		Force: true,
	})
	if err != nil {
		t.Fatalf("generate recommendation previews: %v", err)
	}
	if report.Generated != 1 || report.Failed != 0 || provider.calls != 1 {
		t.Fatalf("expected one generated recommendation preview, report=%+v provider calls=%d", report, provider.calls)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected provider input")
	}
	input := provider.inputs[0]
	if input.AspectRatio != "16:9" || input.Size == "" || input.Quality != GenerationQualityHigh || input.ToolMode != GenerationToolModeGenerate {
		t.Fatalf("expected recommendation generation input with aspect ratio, size, high quality and generate mode, got %+v", input)
	}
	for _, required := range []string{"推荐卡封面", "主体清晰", "无文字", "无水印", "无 logo", "前台卡片裁切"} {
		if !strings.Contains(input.Prompt, required) {
			t.Fatalf("expected prompt to include cover constraint %q, got %q", required, input.Prompt)
		}
	}

	var recommendation InspirationRecommendation
	if err := testApp.db.Where("slug = ?", "weekly-cyberpunk-city").First(&recommendation).Error; err != nil {
		t.Fatalf("load generated recommendation preview: %v", err)
	}
	if recommendation.PreviewAssetKey != "recommendations/generated.png" || recommendation.PreviewURL != "https://oss.example.com/recommendations/generated.png" {
		t.Fatalf("expected stored generated preview fields, got %+v", recommendation)
	}
}

func TestGenerateMissingInspirationRecommendationPreviewsUsesModelCenterImageRoute(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("model-center-recommendation-preview")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_recommendation_model_center",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "recommendations/model-center.png",
		publicURL: "https://oss.example.com/recommendations/model-center.png",
	}
	configureImageModelCenterRouteForRecommendationTest(t, testApp)

	report, err := testApp.GenerateMissingInspirationRecommendationPreviews(context.Background(), InspirationRecommendationPreviewGenerationOptions{
		Slugs: []string{"weekly-cyberpunk-city"},
		Force: true,
	})
	if err != nil {
		t.Fatalf("generate recommendation previews: %v", err)
	}
	if report.Generated != 1 || report.Failed != 0 || len(provider.inputs) != 1 {
		t.Fatalf("expected one generated recommendation preview through model center, report=%+v inputs=%d", report, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.Model != "gpt-image-2" {
		t.Fatalf("expected model center runtime model, got %q", input.Model)
	}
	if input.ProviderBaseURL != "https://api.bailinai.net" || input.ProviderAPIKey != "bailinai-key" || input.ProviderAPIEndpoint != "/v1/images/generations" {
		t.Fatalf("expected model center provider credentials and endpoint, got %+v", input)
	}
}

func TestGenerateMissingInspirationRecommendationPreviewsFiltersSkipsAndForces(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("replacement-recommendation-preview")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_recommendation_force",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "recommendations/replaced.png",
		publicURL: "https://oss.example.com/recommendations/replaced.png",
	}

	if err := testApp.db.Create(&InspirationRecommendation{
		Slug:        "not-hot-extra",
		Title:       "Not Hot Extra",
		PreviewURL:  "",
		Prompt:      "extra prompt",
		AspectRatio: "1:1",
		ToolMode:    GenerationToolModeGenerate,
		IsActive:    true,
	}).Error; err != nil {
		t.Fatalf("create non-default recommendation: %v", err)
	}
	if err := testApp.db.Model(&InspirationRecommendation{}).
		Where("slug = ?", "weekly-cyberpunk-city").
		Updates(map[string]any{
			"preview_asset_key": "recommendations/existing.png",
			"preview_url":       "https://oss.example.com/recommendations/existing.png",
		}).Error; err != nil {
		t.Fatalf("mark recommendation existing: %v", err)
	}

	report, err := testApp.GenerateMissingInspirationRecommendationPreviews(context.Background(), InspirationRecommendationPreviewGenerationOptions{
		Slugs: []string{"weekly-cyberpunk-city"},
	})
	if err != nil {
		t.Fatalf("generate recommendation previews without force: %v", err)
	}
	if report.Generated != 0 || report.Skipped != 1 || provider.calls != 0 {
		t.Fatalf("expected existing recommendation skipped, report=%+v provider calls=%d", report, provider.calls)
	}

	report, err = testApp.GenerateMissingInspirationRecommendationPreviews(context.Background(), InspirationRecommendationPreviewGenerationOptions{
		Force: true,
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("force generate recommendation previews: %v", err)
	}
	if report.Generated != 1 || provider.calls != 1 {
		t.Fatalf("expected forced single generation, report=%+v provider calls=%d", report, provider.calls)
	}

	var extra InspirationRecommendation
	if err := testApp.db.Where("slug = ?", "not-hot-extra").First(&extra).Error; err != nil {
		t.Fatalf("load extra recommendation: %v", err)
	}
	if extra.PreviewAssetKey != "" || extra.PreviewURL != "" {
		t.Fatalf("default slug filter must not touch non-default recommendation, got %+v", extra)
	}
}

func TestGenerateMissingInspirationRecommendationPreviewsProviderFailureDoesNotWriteDB(t *testing.T) {
	provider := &stubProvider{
		err: &ProviderError{
			Code:    "provider_unavailable",
			Message: "upstream unavailable",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "recommendations/failed.png",
		publicURL: "https://oss.example.com/recommendations/failed.png",
	}

	var before InspirationRecommendation
	if err := testApp.db.Where("slug = ?", "weekly-cyberpunk-city").First(&before).Error; err != nil {
		t.Fatalf("load recommendation before generation: %v", err)
	}
	report, err := testApp.GenerateMissingInspirationRecommendationPreviews(context.Background(), InspirationRecommendationPreviewGenerationOptions{
		Slugs: []string{"weekly-cyberpunk-city"},
		Force: true,
	})
	if err != nil {
		t.Fatalf("generate recommendation previews: %v", err)
	}
	if report.Failed != 1 || report.Generated != 0 {
		t.Fatalf("expected provider failure in report, got %+v", report)
	}

	var after InspirationRecommendation
	if err := testApp.db.Where("slug = ?", "weekly-cyberpunk-city").First(&after).Error; err != nil {
		t.Fatalf("load recommendation after failure: %v", err)
	}
	if after.PreviewAssetKey != before.PreviewAssetKey || after.PreviewURL != before.PreviewURL {
		t.Fatalf("provider failure must not update preview fields, before=%+v after=%+v", before, after)
	}
}

func configureImageModelCenterRouteForRecommendationTest(t *testing.T, app *App) {
	t.Helper()
	model := ModelCatalog{
		Name:               "GPT Image 2",
		Modality:           ModelConfigTypeImage,
		Status:             ModelCenterStatusOnline,
		Visibility:         ModelCenterVisibilityPublic,
		DefaultCreditsCost: 1,
		CapabilityTags:     []string{"image"},
		SortOrder:          1,
	}
	if err := app.db.Create(&model).Error; err != nil {
		t.Fatalf("create model center image model: %v", err)
	}
	provider := ModelProvider{
		Name:                  "BailinAI",
		Provider:              "bailinai",
		BaseURL:               "https://api.bailinai.net/",
		APIKey:                "bailinai-key",
		DefaultTimeoutSeconds: 600,
		ConcurrencyLimit:      2,
		Status:                ModelCenterStatusOnline,
	}
	if err := app.db.Create(&provider).Error; err != nil {
		t.Fatalf("create model center provider: %v", err)
	}
	channel := ModelChannel{
		ModelID:      model.ID,
		ProviderID:   provider.ID,
		Name:         "BailinAI GPT Image 2",
		RuntimeModel: "gpt-image-2",
		Endpoint:     "/v1/images/generations",
		Weight:       100,
		Priority:     1,
		Status:       ModelCenterStatusOnline,
		HealthStatus: ModelChannelHealthHealthy,
	}
	if err := app.db.Create(&channel).Error; err != nil {
		t.Fatalf("create model center channel: %v", err)
	}
	var policy ModelRoutingPolicy
	if err := app.db.Where("modality = ?", ModelConfigTypeImage).FirstOrCreate(&policy, ModelRoutingPolicy{Modality: ModelConfigTypeImage}).Error; err != nil {
		t.Fatalf("load image routing policy: %v", err)
	}
	if err := app.db.Model(&policy).Updates(map[string]any{
		"default_model_id":  model.ID,
		"fallback_model_id": model.ID,
		"routing_enabled":   true,
		"routing_strategy":  ModelRoutingStrategyDefault,
		"source":            ModelRoutingSourceModelCenter,
	}).Error; err != nil {
		t.Fatalf("update image routing policy: %v", err)
	}
	if err := app.db.Where("policy_id = ?", policy.ID).Delete(&ModelRoutingEntry{}).Error; err != nil {
		t.Fatalf("clear image routing entries: %v", err)
	}
	if err := app.db.Create(&ModelRoutingEntry{
		PolicyID:  policy.ID,
		ModelID:   model.ID,
		ChannelID: channel.ID,
		Enabled:   true,
		Weight:    100,
		Priority:  1,
	}).Error; err != nil {
		t.Fatalf("create image routing entry: %v", err)
	}
}

func jsonContains(payload []byte, value string) bool {
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return false
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return false
	}
	return strings.Contains(string(encoded), value)
}
