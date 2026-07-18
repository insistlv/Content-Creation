package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"image-agent/internal/app/ecommerce"
)

type appFakeCommerceVisionAnalyzer struct{}

func (appFakeCommerceVisionAnalyzer) AnalyzeProduct(context.Context, ecommerce.ProductAnalysisRequest) (string, error) {
	return "", nil
}

func TestCommerceBootstrapHandlerAtomicAndCSRF(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "commerce-bootstrap-user", "password123")

	missingOrigin := httptest.NewRequest(http.MethodPost, "/api/ecommerce/projects/bootstrap", strings.NewReader(`{"title":"杯子","pipeline":"general"}`))
	missingOrigin.Header.Set("Content-Type", "application/json")
	missingOrigin.Header.Set("Idempotency-Key", "bootstrap-http")
	for _, cookie := range cookies {
		missingOrigin.AddCookie(cookie)
	}
	rejected := httptest.NewRecorder()
	testApp.Router().ServeHTTP(rejected, missingOrigin)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("missing origin = %d: %s", rejected.Code, rejected.Body.String())
	}
	var rejectedProducts int64
	db.Model(&ecommerce.CommerceProduct{}).Count(&rejectedProducts)
	if rejectedProducts != 0 {
		t.Fatalf("CSRF rejection left products: %d", rejectedProducts)
	}

	first := performCommerceKeyedRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/bootstrap", `{"title":"杯子","category":"家居","sku_code":"CUP-1","pipeline":"general"}`, "bootstrap-http", cookies)
	if first.Code != http.StatusCreated || !strings.Contains(first.Body.String(), `"product"`) || !strings.Contains(first.Body.String(), `"sku"`) || !strings.Contains(first.Body.String(), `"project"`) {
		t.Fatalf("bootstrap = %d: %s", first.Code, first.Body.String())
	}
	replay := performCommerceKeyedRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/bootstrap", `{"title":"杯子","category":"家居","sku_code":"CUP-1","pipeline":"general"}`, "bootstrap-http", cookies)
	if replay.Code != http.StatusOK || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay = %d: %s", replay.Code, replay.Body.String())
	}
	conflict := performCommerceKeyedRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/bootstrap", `{"title":"冲突","pipeline":"general"}`, "bootstrap-http", cookies)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "idempotency_conflict") {
		t.Fatalf("conflict = %d: %s", conflict.Code, conflict.Body.String())
	}
}

func TestCommerceProductReportAnalyzeHandler(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-analysis-user", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "灯", Status: "active"}
	db.Create(&product)
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	db.Create(&project)
	asset := ecommerce.CommerceAsset{UserID: user.ID, ProjectID: project.ID, ReferenceAssetID: 900, Role: "product", Lifecycle: ecommerce.AssetLifecycleProject}
	db.Create(&asset)
	path := "/api/ecommerce/projects/" + itoa(project.ID) + "/creative-specs/analyze"
	body := `{"source_asset_ids":[` + itoa(asset.ID) + `],"user_requirements":"简洁"}`

	unconfigured := performCommerceKeyedRequest(t, testApp, http.MethodPost, path, body, "analysis-http", cookies)
	if unconfigured.Code != http.StatusServiceUnavailable || !strings.Contains(unconfigured.Body.String(), "commerce_vision_not_configured") || !strings.Contains(unconfigured.Body.String(), "required_fields") {
		t.Fatalf("unconfigured = %d: %s", unconfigured.Code, unconfigured.Body.String())
	}
	var specs, jobs int64
	db.Model(&ecommerce.CommerceCreativeSpec{}).Count(&specs)
	db.Model(&ecommerce.CommerceJob{}).Count(&jobs)
	if specs != 0 || jobs != 0 {
		t.Fatalf("unconfigured left rows specs=%d jobs=%d", specs, jobs)
	}

	testApp.ConfigureCommerceVisionAnalyzer(appFakeCommerceVisionAnalyzer{})
	created := performCommerceKeyedRequest(t, testApp, http.MethodPost, path, body, "analysis-http", cookies)
	if created.Code != http.StatusAccepted || !strings.Contains(created.Body.String(), `"status":"analyzing"`) || !strings.Contains(created.Body.String(), `"kind":"product_analysis"`) {
		t.Fatalf("analyze = %d: %s", created.Code, created.Body.String())
	}
	replay := performCommerceKeyedRequest(t, testApp, http.MethodPost, path, body, "analysis-http", cookies)
	if replay.Code != http.StatusAccepted || replay.Body.String() != created.Body.String() {
		t.Fatalf("analyze replay = %d: %s", replay.Code, replay.Body.String())
	}
}

func TestCommerceVisionStructuredCreativeFieldsPatchCAS(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-vision-structured-patch", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "结构化报告", Status: "active"}
	db.Create(&product)
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	db.Create(&project)
	spec := ecommerce.CommerceCreativeSpec{
		UserID: user.ID, ProjectID: project.ID, Version: 1, Source: "vision", Status: "draft",
		ProductFactsJSON: "{}", SellingPointsJSON: `["AI卖点"]`, ForbiddenChangesJSON: `["AI禁改"]`, BrandToneJSON: `{"description":"AI调性"}`,
		ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: "[1]",
		ObservedFactsJSON: `[{"field":"name","value":"原始商品","confidence":0.9,"source_asset_ids":[1]}]`, UserOverridesJSON: "{}", MissingFieldsJSON: "[]", SuggestedSectionsJSON: "[]",
	}
	db.Create(&spec)
	path := "/api/ecommerce/creative-specs/" + itoa(spec.ID)
	patched := performJSONRequest(t, testApp, http.MethodPatch, path, map[string]any{
		"expected_version": 1, "selling_points": []string{"用户卖点"}, "forbidden_changes": []string{"不得改变杯盖"}, "brand_tone": map[string]any{"description": "简洁"},
	}, cookies)
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), `"version":2`) || !strings.Contains(patched.Body.String(), `"用户卖点"`) || !strings.Contains(patched.Body.String(), `"原始商品"`) {
		t.Fatalf("structured patch=%d %s", patched.Code, patched.Body.String())
	}

	for _, tc := range []struct {
		name, field string
		invalid     any
	}{
		{name: "selling_points_object", field: "selling_points", invalid: map[string]any{"text": "错误"}},
		{name: "forbidden_changes_empty", field: "forbidden_changes", invalid: []any{""}},
		{name: "brand_tone_array", field: "brand_tone", invalid: []string{"错误"}},
		{name: "brand_tone_null", field: "brand_tone", invalid: nil},
	} {
		response := performJSONRequest(t, testApp, http.MethodPatch, path, map[string]any{"expected_version": 2, tc.field: tc.invalid}, cookies)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid %s=%#v response=%d %s", tc.name, tc.invalid, response.Code, response.Body.String())
		}
	}
	stale := performJSONRequest(t, testApp, http.MethodPatch, path, map[string]any{"expected_version": 1, "selling_points": []string{"陈旧覆盖"}}, cookies)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale patch=%d %s", stale.Code, stale.Body.String())
	}
	var loaded ecommerce.CommerceCreativeSpec
	db.First(&loaded, spec.ID)
	if loaded.Version != 2 || loaded.ObservedFactsJSON != spec.ObservedFactsJSON || loaded.SellingPointsJSON != `["用户卖点"]` || loaded.ForbiddenChangesJSON != `["不得改变杯盖"]` || loaded.BrandToneJSON != `{"description":"简洁"}` {
		t.Fatalf("patch CAS/evidence state=%#v", loaded)
	}
}

func TestCommerceManualFallbackAfterVision503CanPatchAndConfirm(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-manual-fallback", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "手工降级", Status: "active"}
	db.Create(&product)
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	db.Create(&project)
	asset := ecommerce.CommerceAsset{UserID: user.ID, ProjectID: project.ID, ReferenceAssetID: 901, Role: "product_front", Lifecycle: ecommerce.AssetLifecycleProject}
	db.Create(&asset)
	analyzePath := "/api/ecommerce/projects/" + itoa(project.ID) + "/creative-specs/analyze"
	analyze := performCommerceKeyedRequest(t, testApp, http.MethodPost, analyzePath, `{"source_asset_ids":[`+itoa(asset.ID)+`]}`, "manual-fallback-analysis", cookies)
	if analyze.Code != http.StatusServiceUnavailable || !strings.Contains(analyze.Body.String(), "commerce_vision_not_configured") {
		t.Fatalf("analyze=%d %s", analyze.Code, analyze.Body.String())
	}
	invalidObjects := []any{[]any{"not-an-object"}, "not-an-object", 7, nil}
	for _, invalid := range invalidObjects {
		invalidCreate := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/creative-specs", map[string]any{"product_facts": invalid}, cookies)
		if invalidCreate.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCreate.Body.String(), "invalid_input") {
			t.Fatalf("invalid manual create value=%#v response=%d %s", invalid, invalidCreate.Code, invalidCreate.Body.String())
		}
	}

	create := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/creative-specs", map[string]any{}, cookies)
	if create.Code != http.StatusCreated {
		t.Fatalf("manual create=%d %s", create.Code, create.Body.String())
	}
	var created struct {
		ID      uint   `json:"id"`
		Version int    `json:"version"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || created.ID == 0 || created.Source != "manual" {
		t.Fatalf("manual create body=%s err=%v", create.Body.String(), err)
	}
	patch := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/creative-specs/"+itoa(created.ID), map[string]any{
		"expected_version": created.Version,
		"user_overrides":   map[string]any{"name": "手工杯子", "category": "家居", "material": "不锈钢"},
	}, cookies)
	if patch.Code != http.StatusOK {
		t.Fatalf("manual patch=%d %s", patch.Code, patch.Body.String())
	}
	confirm := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/creative-specs/"+itoa(created.ID)+"/confirm", map[string]any{}, cookies)
	if confirm.Code != http.StatusOK || !strings.Contains(confirm.Body.String(), `"status":"confirmed"`) || !strings.Contains(confirm.Body.String(), `"name":"手工杯子"`) {
		t.Fatalf("manual confirm=%d %s", confirm.Code, confirm.Body.String())
	}

	missing := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/creative-specs", map[string]any{}, cookies)
	missingID := commerceResponseID(t, missing)
	for _, invalid := range invalidObjects {
		invalidPatch := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/creative-specs/"+itoa(missingID), map[string]any{"expected_version": 1, "user_overrides": invalid}, cookies)
		if invalidPatch.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidPatch.Body.String(), "invalid_input") {
			t.Fatalf("invalid overrides patch value=%#v response=%d %s", invalid, invalidPatch.Code, invalidPatch.Body.String())
		}
	}
	missingPatch := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/creative-specs/"+itoa(missingID), map[string]any{"expected_version": 1, "user_overrides": map[string]any{"category": "家居"}}, cookies)
	if missingPatch.Code != http.StatusOK {
		t.Fatalf("missing patch=%d %s", missingPatch.Code, missingPatch.Body.String())
	}
	missingConfirm := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/creative-specs/"+itoa(missingID)+"/confirm", map[string]any{}, cookies)
	if missingConfirm.Code != http.StatusUnprocessableEntity || !strings.Contains(missingConfirm.Body.String(), `"field":"name"`) {
		t.Fatalf("missing confirm=%d %s", missingConfirm.Code, missingConfirm.Body.String())
	}
}

func TestConfigureCommerceVisionAnalyzerRejectsRuntimeMutation(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	original := appFakeCommerceVisionAnalyzer{}
	if err := testApp.ConfigureCommerceVisionAnalyzer(original); err != nil {
		t.Fatal(err)
	}
	testApp.commerceWorker = &ecommerce.Worker{}
	if err := testApp.ConfigureCommerceVisionAnalyzer(appFakeCommerceVisionAnalyzer{}); err == nil {
		t.Fatal("runtime analyzer mutation must be rejected")
	}
}

func performCommerceKeyedRequest(t *testing.T, testApp *App, method, path, body, key string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testApp.cfg.AppBaseURL)
	req.Header.Set("Idempotency-Key", key)
	addDefaultCSRFForTest(req)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp := httptest.NewRecorder()
	testApp.Router().ServeHTTP(resp, req)
	return resp
}
