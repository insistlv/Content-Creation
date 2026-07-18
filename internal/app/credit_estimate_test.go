package app

import (
	"encoding/json"
	"net/http"
	"testing"

	"gorm.io/gorm"
)

type creditEstimateTestPayload struct {
	Error struct {
		Code             string                       `json:"code"`
		Message          string                       `json:"message"`
		ValidationErrors []bodyProfileValidationError `json:"validation_errors"`
	} `json:"error"`
	RequiredCredits    int      `json:"required_credits"`
	AvailableCredits   int      `json:"available_credits"`
	MissingCredits     int      `json:"missing_credits"`
	Enough             bool     `json:"enough"`
	RecommendedPackage *Package `json:"recommended_package"`
	BillingPolicy      string   `json:"billing_policy"`
	Message            string   `json:"message"`
}

func TestEstimateImageGenerationUsesBatchTotalOnlyAndDoesNotMutate(t *testing.T) {
	provider := &stubProvider{}
	testApp, db := newTestApp(t, provider)
	adminCookies := createAdminSession(t, testApp)
	user, cookies := createLoggedInUser(t, testApp, "estimate_image_creator", "test-password")
	setUserCredits(t, testApp, user.ID, 4)
	first := seedReferenceAsset(t, testApp, user.ID, "first.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "second.png", "image/png", []byte("second-reference"))
	configureImageModelCenterRouteForEstimateTest(t, testApp, adminCookies, 3)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/estimate", map[string]any{
		"prompt":              "estimate a batched image generation",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
		"batch_id":            "estimate-batch",
		"batch_index":         0,
		"batch_total":         2,
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 2 || payload.AvailableCredits != 4 || payload.MissingCredits != 0 || !payload.Enough {
		t.Fatalf("expected required=2 available=4 missing=0 enough=true, got %+v", payload)
	}
	if payload.RecommendedPackage != nil {
		t.Fatalf("expected no recommended package when credits are enough, got %+v", payload.RecommendedPackage)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
	assertUserCreditsForTest(t, testApp, user.ID, 4)
	if provider.calls != 0 {
		t.Fatalf("estimate must not call provider, got %d calls", provider.calls)
	}
}

func TestEstimateVideoGenerationReturnsMissingCreditsWithoutMutating(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "estimate-video-provider-call"}}
	testApp.videoProvider = videoProvider
	user, cookies := createLoggedInUser(t, testApp, "estimate_video_creator", "test-password")
	setUserCredits(t, testApp, user.ID, 10)
	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "estimate-wuyin-key")
	var txCountBefore int64
	if err := db.Model(&CreditTransaction{}).Where("user_id = ?", user.ID).Count(&txCountBefore).Error; err != nil {
		t.Fatalf("count credit transactions before estimate: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/estimate", map[string]any{
		"prompt":       "estimate a grok video before submit",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 18 || payload.AvailableCredits != 10 || payload.MissingCredits != 8 || payload.Enough {
		t.Fatalf("expected required=18 available=10 missing=8 enough=false, got %+v", payload)
	}
	if payload.BillingPolicy != "success_only" || payload.Message != "提交前预估，生成成功后扣点，失败不扣点" {
		t.Fatalf("expected success-only billing copy, got policy=%q message=%q", payload.BillingPolicy, payload.Message)
	}
	if payload.RecommendedPackage == nil || payload.RecommendedPackage.Credits != 50 || payload.RecommendedPackage.Name != "体验包" {
		t.Fatalf("expected recommended package covering missing credits, got %+v", payload.RecommendedPackage)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
	var txCountAfter int64
	if err := db.Model(&CreditTransaction{}).Where("user_id = ?", user.ID).Count(&txCountAfter).Error; err != nil {
		t.Fatalf("count credit transactions: %v", err)
	}
	if txCountAfter != txCountBefore {
		t.Fatalf("estimate must not write credit transactions, before=%d after=%d", txCountBefore, txCountAfter)
	}
	assertUserCreditsForTest(t, testApp, user.ID, 10)
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("estimate must not call video provider, got %+v", videoProvider.submitInputs)
	}
}

func TestEstimateVideoGenerationKeepsCreateValidationErrors(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.videoProvider = &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "unused"}}
	user, cookies := createLoggedInUser(t, testApp, "estimate_video_reference_missing", "test-password")
	setUserCredits(t, testApp, user.ID, 50)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/estimate", map[string]any{
		"prompt":       "grok without reference image",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected reference-image validation 422, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "reference_image_required" || payload.Error.Message != videoReferenceImageRequiredMessage {
		t.Fatalf("expected reference_image_required payload, got %+v", payload)
	}
}

func TestEstimateImageGenerationIgnoresNumQualityToolOptionsReferencesAndModelCost(t *testing.T) {
	provider := &stubProvider{}
	testApp, db := newTestApp(t, provider)
	adminCookies := createAdminSession(t, testApp)
	user, cookies := createLoggedInUser(t, testApp, "estimate_num_quality_tool_options", "test-password")
	setUserCredits(t, testApp, user.ID, 20)
	reference := seedReferenceAsset(t, testApp, user.ID, "upscale-source.png", "image/png", []byte("upscale-source"))
	modelID := configureImageModelCenterRouteForEstimateTest(t, testApp, adminCookies, 3)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/estimate", map[string]any{
		"prompt":              "estimate upscale variants",
		"aspect_ratio":        "1:1",
		"model_id":            modelID,
		"quality":             GenerationQualityUltra,
		"num":                 4,
		"tool_mode":           GenerationToolModeUpscale,
		"tool_options":        map[string]any{"scale": "4x"},
		"reference_asset_ids": []uint{reference.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 1 || payload.AvailableCredits != 20 || payload.MissingCredits != 0 || !payload.Enough {
		t.Fatalf("expected one credit regardless of num/model/quality/tool/reference options, got %+v", payload)
	}
	if payload.RecommendedPackage != nil {
		t.Fatalf("expected no recommended package when credits are enough, got %+v", payload.RecommendedPackage)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
	assertUserCreditsForTest(t, testApp, user.ID, 20)
	if provider.calls != 0 {
		t.Fatalf("estimate must not call provider, got %d calls", provider.calls)
	}
}

func TestEstimateImageGenerationUsesNormalizedBatchTotalForMissingCredits(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, cookies := createLoggedInUser(t, testApp, "estimate_large_missing_creator", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	configureImageModelCenterRouteForEstimateTest(t, testApp, adminCookies, 30)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/estimate", map[string]any{
		"prompt":       "estimate a very large batch",
		"aspect_ratio": "1:1",
		"batch_id":     "estimate-large-batch",
		"batch_index":  0,
		"batch_total":  400,
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 16 || payload.MissingCredits != 16 || payload.Enough {
		t.Fatalf("expected required=16 missing=16 enough=false, got %+v", payload)
	}
	if payload.RecommendedPackage == nil || payload.RecommendedPackage.Credits != 50 || payload.RecommendedPackage.Name != "体验包" {
		t.Fatalf("expected smallest package covering 16 missing credits, got %+v", payload.RecommendedPackage)
	}
}

func TestEstimateCoupleAlbumUsesEightPagesAndDoesNotCreateAlbum(t *testing.T) {
	provider := &stubProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "estimate_album_creator", "test-password")
	setUserCredits(t, testApp, user.ID, 6)
	male := seedReferenceAsset(t, testApp, user.ID, "male.png", "image/png", []byte("male"))
	female := seedReferenceAsset(t, testApp, user.ID, "female.png", "image/png", []byte("female"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/couple-albums/estimate", coupleAlbumCreateBody(male.ID, female.ID), cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected couple album estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 8 || payload.AvailableCredits != 6 || payload.MissingCredits != 2 || payload.Enough {
		t.Fatalf("expected 8 pages at one credit each, got %+v", payload)
	}
	if payload.RecommendedPackage == nil || payload.RecommendedPackage.Credits != 50 || payload.RecommendedPackage.Name != "体验包" {
		t.Fatalf("expected smallest package covering 2 missing credits, got %+v", payload.RecommendedPackage)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
	var albumCount int64
	if err := db.Model(&CoupleAlbum{}).Where("user_id = ?", user.ID).Count(&albumCount).Error; err != nil {
		t.Fatalf("count albums: %v", err)
	}
	if albumCount != 0 {
		t.Fatalf("estimate must not create album records, got %d", albumCount)
	}
	assertUserCreditsForTest(t, testApp, user.ID, 6)
	if provider.calls != 0 {
		t.Fatalf("estimate must not call provider, got %d calls", provider.calls)
	}
}

func TestAsyncGenerationInsufficientCreditsReturnsEstimateFields(t *testing.T) {
	provider := &stubProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "estimate_async_insufficient", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	first := seedReferenceAsset(t, testApp, user.ID, "first.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "second.png", "image/png", []byte("second-reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "combine uploaded references with too few credits",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
	}, cookies)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "credits_insufficient" || payload.RequiredCredits != 1 || payload.AvailableCredits != 0 ||
		payload.MissingCredits != 1 || payload.Enough {
		t.Fatalf("expected structured insufficient-credit payload, got %+v", payload)
	}
	if payload.RecommendedPackage == nil || payload.RecommendedPackage.Credits != 50 {
		t.Fatalf("expected recommended package in insufficient-credit payload, got %+v", payload.RecommendedPackage)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
	if provider.calls != 0 {
		t.Fatalf("expected provider not called when credits cannot cover cost, got %d", provider.calls)
	}
}

func TestRecommendedPackageUsesExpandedSixTierLadder(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	cases := []struct {
		missing int
		name    string
		credits int
	}{
		{missing: 51, name: "入门包", credits: 188},
		{missing: 689, name: "进阶包", credits: 1488},
		{missing: 1489, name: "专业包", credits: 2588},
		{missing: 2589, name: "旗舰包", credits: 6188},
		{missing: 7000, name: "旗舰包", credits: 6188},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recommended, err := testApp.recommendedPackageForMissingCredits(tc.missing)
			if err != nil {
				t.Fatalf("recommend package: %v", err)
			}
			if recommended == nil || recommended.Name != tc.name || recommended.Credits != tc.credits {
				t.Fatalf("for missing %d expected %s/%d, got %+v", tc.missing, tc.name, tc.credits, recommended)
			}
		})
	}
}

func configureImageModelCenterRouteForEstimateTest(t *testing.T, app *App, adminCookies []*http.Cookie, creditCost int) uint {
	t.Helper()
	modelID := createModelCenterModel(t, app, adminCookies, map[string]any{
		"name":                 "Estimate Image Model",
		"modality":             "image",
		"status":               "online",
		"visibility":           "public",
		"default_credits_cost": creditCost,
		"capability_tags":      []string{"image", "reference"},
		"sort_order":           1,
	})
	providerID := createModelCenterProvider(t, app, adminCookies, map[string]any{
		"name":                    "Estimate Provider",
		"provider":                "openai",
		"base_url":                "https://estimate-provider.example",
		"api_key":                 "estimate-provider-key",
		"default_timeout_seconds": 45,
		"concurrency_limit":       2,
		"status":                  "online",
	})
	channelID := createModelCenterChannel(t, app, adminCookies, map[string]any{
		"model_id":      modelID,
		"provider_id":   providerID,
		"name":          "Estimate Channel",
		"runtime_model": "estimate-image-model",
		"endpoint":      "/v1/images/generations",
		"weight":        100,
		"priority":      1,
		"status":        "online",
		"health_status": "healthy",
	})
	routeResp := performJSONRequest(t, app, http.MethodPut, "/api/admin/model-center/routing", map[string]any{
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
	return modelID
}

func decodeCreditEstimateTestPayload(t *testing.T, body []byte) creditEstimateTestPayload {
	t.Helper()
	var payload creditEstimateTestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode credit estimate payload: %v", err)
	}
	return payload
}

func assertNoGenerationRecordsForUser(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	var count int64
	if err := db.Model(&GenerationRecord{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count generation records: %v", err)
	}
	if count != 0 {
		t.Fatalf("estimate must not create generation records, got %d", count)
	}
}

func assertUserCreditsForTest(t *testing.T, app *App, userID uint, expected int) {
	t.Helper()
	balance, err := app.lookupBalance(userID)
	if err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != expected {
		t.Fatalf("expected user credits %d, got %d", expected, balance.AvailableCredits)
	}
}
