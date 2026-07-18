package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const virtualTryOnToolModeForTest = "virtual_try_on"

func TestEstimateVirtualTryOnRequiresGarmentReference(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "tryon_missing_garment", "test-password")
	setUserCredits(t, testApp, user.ID, 5)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/estimate", map[string]any{
		"body_profile": map[string]any{
			"height_cm": 168,
			"weight_kg": 56,
			"body_type": "standard",
		},
		"scene": map[string]any{
			"category":  "work_business",
			"sub_scene": "office",
		},
		"generation": map[string]any{
			"quality":      GenerationQualityHigh,
			"aspect_ratio": "3:4",
		},
	}, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected garment reference validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "garment_reference_required" {
		t.Fatalf("expected garment_reference_required, got %+v", payload.Error)
	}
}

func TestEstimateVirtualTryOnValidatesBodyMeasurements(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "tryon_invalid_body", "test-password")
	setUserCredits(t, testApp, user.ID, 5)
	garment := seedReferenceAsset(t, testApp, user.ID, "jacket.png", "image/png", []byte("garment"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/estimate", map[string]any{
		"body_profile": map[string]any{
			"height_cm": 30,
			"weight_kg": 56,
		},
		"garment": map[string]any{
			"garment_reference_asset_id": garment.ID,
			"category":                   "jacket",
		},
		"scene": map[string]any{
			"category":  "social_etiquette",
			"sub_scene": "banquet",
		},
		"generation": map[string]any{
			"quality":      GenerationQualityMedium,
			"aspect_ratio": "3:4",
		},
	}, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected body validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "invalid_body_profile" {
		t.Fatalf("expected invalid_body_profile, got %+v", payload.Error)
	}
	errors := payload.Error.ValidationErrors
	if len(errors) != 1 {
		t.Fatalf("expected one validation error, got %+v", errors)
	}
	assertBodyValidationError(t, errors[0], bodyProfileValidationError{
		Field:    "height_cm",
		Label:    "身高",
		Value:    float64PtrForTest(30),
		Min:      80,
		Max:      230,
		Unit:     "cm",
		Required: true,
	})
}

func TestEstimateVirtualTryOnReturnsAllBodyValidationErrors(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "tryon_invalid_body_many", "test-password")
	setUserCredits(t, testApp, user.ID, 5)
	garment := seedReferenceAsset(t, testApp, user.ID, "jacket.png", "image/png", []byte("garment"))
	body := validVirtualTryOnBody(garment.ID, 0)
	body["body_profile"] = map[string]any{
		"height_cm":   30,
		"weight_kg":   300,
		"shoulder_cm": 10,
		"chest_cm":    200,
		"waist_cm":    20,
		"hip_cm":      240,
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/estimate", body, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected body validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "invalid_body_profile" {
		t.Fatalf("expected invalid_body_profile, got %+v", payload.Error)
	}
	errors := payload.Error.ValidationErrors
	if len(errors) != 6 {
		t.Fatalf("expected six validation errors, got %+v", errors)
	}
	assertBodyValidationError(t, errors[0], bodyProfileValidationError{Field: "height_cm", Label: "身高", Value: float64PtrForTest(30), Min: 80, Max: 230, Unit: "cm", Required: true})
	assertBodyValidationError(t, errors[1], bodyProfileValidationError{Field: "weight_kg", Label: "体重", Value: float64PtrForTest(300), Min: 25, Max: 250, Unit: "kg", Required: true})
	assertBodyValidationError(t, errors[2], bodyProfileValidationError{Field: "shoulder_cm", Label: "肩宽", Value: float64PtrForTest(10), Min: 20, Max: 80, Unit: "cm", Required: false})
	assertBodyValidationError(t, errors[3], bodyProfileValidationError{Field: "chest_cm", Label: "胸围", Value: float64PtrForTest(200), Min: 40, Max: 180, Unit: "cm", Required: false})
	assertBodyValidationError(t, errors[4], bodyProfileValidationError{Field: "waist_cm", Label: "腰围", Value: float64PtrForTest(20), Min: 40, Max: 180, Unit: "cm", Required: false})
	assertBodyValidationError(t, errors[5], bodyProfileValidationError{Field: "hip_cm", Label: "臀围", Value: float64PtrForTest(240), Min: 40, Max: 180, Unit: "cm", Required: false})
}

func TestEstimateVirtualTryOnReportsMissingRequiredBodyMeasurements(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "tryon_missing_body_required", "test-password")
	setUserCredits(t, testApp, user.ID, 5)
	garment := seedReferenceAsset(t, testApp, user.ID, "jacket.png", "image/png", []byte("garment"))
	body := validVirtualTryOnBody(garment.ID, 0)
	body["body_profile"] = map[string]any{}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/estimate", body, cookies)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected body validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "invalid_body_profile" {
		t.Fatalf("expected invalid_body_profile, got %+v", payload.Error)
	}
	errors := payload.Error.ValidationErrors
	if len(errors) != 2 {
		t.Fatalf("expected two validation errors, got %+v", errors)
	}
	assertBodyValidationError(t, errors[0], bodyProfileValidationError{Field: "height_cm", Label: "身高", Value: nil, Min: 80, Max: 230, Unit: "cm", Required: true})
	assertBodyValidationError(t, errors[1], bodyProfileValidationError{Field: "weight_kg", Label: "体重", Value: nil, Min: 25, Max: 250, Unit: "kg", Required: true})
}

func TestEstimateVirtualTryOnRejectsForeignReferenceAsset(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "tryon_asset_owner", "test-password")
	other, cookies := createLoggedInUser(t, testApp, "tryon_asset_other", "test-password")
	setUserCredits(t, testApp, other.ID, 5)
	garment := seedReferenceAsset(t, testApp, owner.ID, "owner-dress.png", "image/png", []byte("owner-garment"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/estimate", validVirtualTryOnBody(garment.ID, 0), cookies)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected foreign garment reference 404, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.Error.Code != "reference_asset_not_found" {
		t.Fatalf("expected reference_asset_not_found, got %+v", payload.Error)
	}
	assertNoGenerationRecordsForUser(t, db, other.ID)
}

func TestEstimateVirtualTryOnReturnsCreditsWithoutMutating(t *testing.T) {
	provider := &stubProvider{}
	testApp, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "tryon_estimate", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	garment := seedReferenceAsset(t, testApp, user.ID, "coat.png", "image/png", []byte("garment"))
	body := seedReferenceAsset(t, testApp, user.ID, "person.png", "image/png", []byte("body"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/estimate", validVirtualTryOnBody(garment.ID, body.ID), cookies)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 1 || payload.AvailableCredits != 3 || payload.MissingCredits != 0 || !payload.Enough {
		t.Fatalf("expected one-credit estimate, got %+v", payload)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
	assertUserCreditsForTest(t, testApp, user.ID, 3)
	if provider.calls != 0 {
		t.Fatalf("estimate must not call provider, got %d calls", provider.calls)
	}
}

func TestCreateVirtualTryOnStoresToolModeOptionsAndReferenceOrder(t *testing.T) {
	provider := newBlockingImageProvider()
	testApp, db := newTestApp(t, provider)
	defer func() {
		close(provider.release)
		waitForCondition(t, time.Second, func() bool {
			return provider.finishedCount() >= 1
		})
	}()
	user, cookies := createLoggedInUser(t, testApp, "tryon_create", "test-password")
	setUserCredits(t, testApp, user.ID, 5)
	garment := seedReferenceAsset(t, testApp, user.ID, "shirt.png", "image/png", []byte("garment"))
	body := seedReferenceAsset(t, testApp, user.ID, "model.png", "image/png", []byte("body"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/virtual-try-on/generations/async", validVirtualTryOnBody(garment.ID, body.ID), cookies)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected async create 202, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint   `json:"generation_id"`
		Status       string `json:"status"`
		CreditsCost  int    `json:"credits_cost"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create payload: %v", err)
	}
	if created.GenerationID == 0 || (created.Status != GenerationStatusQueued && created.Status != GenerationStatusRunning) || created.CreditsCost != 1 {
		t.Fatalf("unexpected create payload: %+v", created)
	}

	var record GenerationRecord
	if err := db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ToolMode != virtualTryOnToolModeForTest {
		t.Fatalf("expected tool mode %q, got %q", virtualTryOnToolModeForTest, record.ToolMode)
	}
	if record.AspectRatio != "3:4" || record.Quality != GenerationQualityHigh {
		t.Fatalf("expected generation settings persisted, got aspect=%q quality=%q", record.AspectRatio, record.Quality)
	}
	options := decodeGenerationToolOptions(record.ToolOptionsJSON)
	virtualTryOn, ok := options[virtualTryOnToolModeForTest].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_options.virtual_try_on object, got %+v", options)
	}
	if virtualTryOn["privacy_mode"] != "ephemeral" {
		t.Fatalf("expected privacy_mode=ephemeral, got %+v", virtualTryOn)
	}
	if garmentOptions, ok := virtualTryOn["garment"].(map[string]any); !ok || garmentOptions["category"] != "shirt" {
		t.Fatalf("expected garment options in tool_options, got %+v", virtualTryOn["garment"])
	}
	ids, err := testApp.generationReferenceAssetIDs(record.ID)
	if err != nil {
		t.Fatalf("load reference links: %v", err)
	}
	if len(ids) != 2 || ids[0] != garment.ID || ids[1] != body.ID {
		t.Fatalf("expected garment first and body second, got %+v", ids)
	}
	if record.Prompt == "" || !containsAll(record.Prompt, []string{"建模试衣", "职场商务", "shirt", "regular"}) {
		t.Fatalf("expected server-built virtual try-on prompt, got %q", record.Prompt)
	}
}

func validVirtualTryOnBody(garmentID, bodyID uint) map[string]any {
	bodyProfile := map[string]any{
		"height_cm":        172,
		"weight_kg":        62,
		"shoulder_cm":      40,
		"chest_cm":         86,
		"waist_cm":         68,
		"hip_cm":           92,
		"body_type":        "standard",
		"body_fat_label":   "normal",
		"fit_preference":   "regular",
		"style_preference": "通勤利落",
	}
	if bodyID != 0 {
		bodyProfile["body_reference_asset_id"] = bodyID
	}
	return map[string]any{
		"body_profile": bodyProfile,
		"garment": map[string]any{
			"garment_reference_asset_id": garmentID,
			"category":                   "shirt",
			"size":                       "M",
			"material":                   "cotton",
			"color":                      "white",
			"fit":                        "regular",
			"details":                    "立领、长袖、隐藏门襟",
		},
		"scene": map[string]any{
			"category":              "work_business",
			"sub_scene":             "office",
			"pose":                  "standing",
			"background_preference": "明亮办公空间",
		},
		"generation": map[string]any{
			"quality":      GenerationQualityHigh,
			"aspect_ratio": "3:4",
		},
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func float64PtrForTest(value float64) *float64 {
	return &value
}

func assertBodyValidationError(t *testing.T, actual bodyProfileValidationError, expected bodyProfileValidationError) {
	t.Helper()
	if actual.Field != expected.Field || actual.Label != expected.Label || actual.Min != expected.Min || actual.Max != expected.Max || actual.Unit != expected.Unit || actual.Required != expected.Required {
		t.Fatalf("unexpected validation error metadata: got %+v want %+v", actual, expected)
	}
	if expected.Value == nil {
		if actual.Value != nil {
			t.Fatalf("expected nil value for %s, got %+v", expected.Field, *actual.Value)
		}
		return
	}
	if actual.Value == nil || *actual.Value != *expected.Value {
		t.Fatalf("unexpected validation error value for %s: got %+v want %+v", expected.Field, actual.Value, expected.Value)
	}
}
