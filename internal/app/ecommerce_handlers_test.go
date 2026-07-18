package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

func TestCommerceProjectHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-project-user", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Lamp", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	create := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects", map[string]any{
		"product_id": product.ID,
		"title":      "Launch",
		"pipeline":   "general",
	}, cookies)
	if create.Code != http.StatusCreated {
		t.Fatalf("create project = %d: %s", create.Code, create.Body.String())
	}
	projectID := commerceResponseID(t, create)

	get := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/projects/"+itoa(projectID), nil, cookies)
	if get.Code != http.StatusOK {
		t.Fatalf("get project = %d: %s", get.Code, get.Body.String())
	}
	patch := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/projects/"+itoa(projectID), map[string]any{"title": "Launch 2", "pipeline": "mixed"}, cookies)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch project = %d: %s", patch.Code, patch.Body.String())
	}
	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/ecommerce/projects/"+itoa(projectID), nil, cookies)
	if deleteResp.Code != http.StatusAccepted {
		t.Fatalf("delete project = %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	var stored ecommerce.CommerceProject
	if err := db.First(&stored, projectID).Error; err != nil {
		t.Fatalf("load deleted project: %v", err)
	}
	if stored.Status != "deletion_requested" || stored.DeletionRequestedAt == nil {
		t.Fatalf("project deletion state = %#v", stored)
	}
}

func TestCommerceBatchHandlersExposePersistedETA(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-eta-user", "password123")
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: 1, Title: "ETA", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	batch := ecommerce.CommerceGenerationBatch{UserID: user.ID, ProjectID: project.ID, Status: ecommerce.CommerceBatchRunning, IdempotencyKey: "eta-handler", ETASeconds: 37}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	list := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches", nil, cookies)
	get := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/batches/"+itoa(batch.ID), nil, cookies)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"eta_seconds":37`) {
		t.Fatalf("list ETA response = %d %s", list.Code, list.Body.String())
	}
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"eta_seconds":37`) {
		t.Fatalf("get ETA response = %d %s", get.Code, get.Body.String())
	}
}

func TestCommerceBatchHandlersExposeFrozenSKUAndStatusProgress(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-item-presentation", "password123")
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: 1, Title: "冻结结果", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	batch := ecommerce.CommerceGenerationBatch{UserID: user.ID, ProjectID: project.ID, Status: ecommerce.CommerceBatchRunning, IdempotencyKey: "frozen-presentation", TotalItems: 2}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}
	skuCompiled := ecommerce.CompiledGenerationItem{SKUID: 71, Scope: "sku", Section: "hero", SKUCode: "OLD-CODE", SpecificationPath: "红色/标准", SKUSnapshotJSON: `{"id":71,"code":"OLD-CODE","specification_path":"红色/标准","attributes_json":"{}"}`}
	sharedCompiled := ecommerce.CompiledGenerationItem{Scope: "shared", Section: "closing", SKUCode: "不得泄露", SpecificationPath: "不得泄露"}
	skuJSON, err := ecommerce.EncodeJSON(skuCompiled)
	if err != nil {
		t.Fatal(err)
	}
	sharedJSON, err := ecommerce.EncodeJSON(sharedCompiled)
	if err != nil {
		t.Fatal(err)
	}
	items := []ecommerce.CommerceGenerationItem{
		{UserID: user.ID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: 1, SKUID: 71, Scope: "sku", SlotKey: "sku:hero", IdempotencyKey: "frozen-sku", Status: ecommerce.CommerceItemRunning, ProgressPercent: 37, OutputSpecJSON: skuJSON},
		{UserID: user.ID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: 1, Scope: "shared", SlotKey: "shared:closing", IdempotencyKey: "frozen-shared", Status: ecommerce.CommerceItemSucceeded, ProgressPercent: 100, OutputSpecJSON: sharedJSON},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/batches/"+itoa(batch.ID), nil, cookies)
	if response.Code != http.StatusOK {
		t.Fatalf("get batch=%d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			SKUID             uint           `json:"sku_id"`
			Scope             string         `json:"scope"`
			Section           string         `json:"section"`
			SKUCode           string         `json:"sku_code"`
			SpecificationPath string         `json:"specification_path"`
			ProgressPercent   int            `json:"progress_percent"`
			SKUSnapshot       map[string]any `json:"sku_snapshot"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := payload.Items[0]; got.SKUID != 71 || got.Scope != "sku" || got.Section != "hero" || got.SKUCode != "OLD-CODE" || got.SpecificationPath != "红色/标准" || got.ProgressPercent != 37 || got.SKUSnapshot["code"] != "OLD-CODE" {
		t.Fatalf("SKU payload=%+v body=%s", got, response.Body.String())
	}
	if got := payload.Items[1]; got.SKUID != 0 || got.Scope != "shared" || got.Section != "closing" || got.ProgressPercent != 100 || got.SKUCode != "" || len(got.SKUSnapshot) != 0 {
		t.Fatalf("shared payload=%+v body=%s", got, response.Body.String())
	}
}

func TestCommerceBrandHandlers(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "commerce-brand-user", "password123")
	create := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/brands", map[string]any{"name": "Acme"}, cookies)
	if create.Code != http.StatusCreated {
		t.Fatalf("create brand = %d: %s", create.Code, create.Body.String())
	}
	id := commerceResponseID(t, create)
	if resp := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/brands", nil, cookies); resp.Code != http.StatusOK {
		t.Fatalf("list brands = %d: %s", resp.Code, resp.Body.String())
	}
	if resp := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/brands/"+itoa(id), map[string]any{"name": "Acme 2"}, cookies); resp.Code != http.StatusOK {
		t.Fatalf("patch brand = %d: %s", resp.Code, resp.Body.String())
	}
}

func TestCommerceBrandHandlersInputAndLogoValidation(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-validation-user", "password123")
	other, _ := createLoggedInUser(t, testApp, "commerce-validation-other", "password123")
	foreignLogo := ReferenceAsset{UserID: other.ID, AssetKey: "foreign/logo.png", MIMEType: "image/png"}
	if err := db.Create(&foreignLogo).Error; err != nil {
		t.Fatalf("create foreign logo: %v", err)
	}

	assertInvalid := func(method, path string, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		resp := performJSONRequest(t, testApp, method, path, body, cookies)
		if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "invalid_input") {
			t.Fatalf("invalid request %s %s = %d: %s", method, path, resp.Code, resp.Body.String())
		}
		return resp
	}
	assertInvalid(http.MethodPost, "/api/ecommerce/brands", map[string]any{"name": "   "})
	assertInvalid(http.MethodPost, "/api/ecommerce/products", map[string]any{"name": ""})

	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Validation product", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	assertInvalid(http.MethodPost, "/api/ecommerce/products/"+itoa(product.ID)+"/skus", map[string]any{"code": ""})

	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	createSpec := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/creative-specs", map[string]any{}, cookies)
	if createSpec.Code != http.StatusCreated {
		t.Fatalf("create empty draft spec = %d: %s", createSpec.Code, createSpec.Body.String())
	}
	assertInvalid(http.MethodPost, "/api/ecommerce/creative-specs/"+itoa(commerceResponseID(t, createSpec))+"/confirm", map[string]any{})

	foreignCreate := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/brands", map[string]any{
		"name":                    "Foreign logo",
		"logo_reference_asset_id": foreignLogo.ID,
	}, cookies)
	if foreignCreate.Code != http.StatusUnprocessableEntity || !strings.Contains(foreignCreate.Body.String(), "ownership_mismatch") {
		t.Fatalf("foreign logo create = %d: %s", foreignCreate.Code, foreignCreate.Body.String())
	}

	brandCreate := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/brands", map[string]any{"name": "Owned brand"}, cookies)
	if brandCreate.Code != http.StatusCreated {
		t.Fatalf("create brand = %d: %s", brandCreate.Code, brandCreate.Body.String())
	}
	brandID := commerceResponseID(t, brandCreate)
	foreignPatch := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/brands/"+itoa(brandID), map[string]any{
		"logo_reference_asset_id": foreignLogo.ID,
	}, cookies)
	if foreignPatch.Code != http.StatusUnprocessableEntity || !strings.Contains(foreignPatch.Body.String(), "ownership_mismatch") {
		t.Fatalf("foreign logo patch = %d: %s", foreignPatch.Code, foreignPatch.Body.String())
	}
	var stored ecommerce.CommerceBrand
	if err := db.First(&stored, brandID).Error; err != nil {
		t.Fatalf("load brand: %v", err)
	}
	if stored.LogoReferenceAssetID != nil {
		t.Fatalf("foreign logo persisted: %v", *stored.LogoReferenceAssetID)
	}
}

func TestCommerceProductHandlers(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "commerce-product-user", "password123")
	create := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/products", map[string]any{"name": "Bag", "category": "fashion"}, cookies)
	if create.Code != http.StatusCreated {
		t.Fatalf("create product = %d: %s", create.Code, create.Body.String())
	}
	id := commerceResponseID(t, create)
	if resp := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/products/"+itoa(id), nil, cookies); resp.Code != http.StatusOK {
		t.Fatalf("get product = %d: %s", resp.Code, resp.Body.String())
	}
	if resp := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/products/"+itoa(id), map[string]any{"name": "Bag 2"}, cookies); resp.Code != http.StatusOK {
		t.Fatalf("patch product = %d: %s", resp.Code, resp.Body.String())
	}
}

func TestCommerceSKUHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-sku-user", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Shirt", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	create := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/products/"+itoa(product.ID)+"/skus", map[string]any{"code": "SHIRT-M"}, cookies)
	if create.Code != http.StatusCreated {
		t.Fatalf("create sku = %d: %s", create.Code, create.Body.String())
	}
	id := commerceResponseID(t, create)
	if resp := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/products/"+itoa(product.ID)+"/skus", nil, cookies); resp.Code != http.StatusOK {
		t.Fatalf("list skus = %d: %s", resp.Code, resp.Body.String())
	}
	if resp := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/skus/"+itoa(id), map[string]any{"size": "M"}, cookies); resp.Code != http.StatusOK {
		t.Fatalf("patch sku = %d: %s", resp.Code, resp.Body.String())
	}
}

func TestCommerceSKUMatrixHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-sku-matrix-user", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "矩阵商品", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	legacyDefault := ecommerce.CommerceSKU{UserID: user.ID, ProductID: product.ID, Code: "DEFAULT", Status: "active", AttributesJSON: `{}`}
	if err := db.Create(&legacyDefault).Error; err != nil {
		t.Fatal(err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, DefaultSKUID: &legacyDefault.ID, Title: "矩阵项目", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"expected_version": 0, "dimensions": []any{map[string]any{"name": "颜色", "values": []any{map[string]any{"name": "红"}, map[string]any{"name": "蓝"}}}}}
	preview := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-matrix/preview", body, cookies)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"add"`) {
		t.Fatalf("preview=%d %s", preview.Code, preview.Body.String())
	}
	missing := performJSONRequest(t, testApp, http.MethodPut, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-matrix", body, cookies)
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing key=%d %s", missing.Code, missing.Body.String())
	}
	apply := performJSONRequestWithHeaders(t, testApp, http.MethodPut, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-matrix", body, cookies, map[string]string{"Idempotency-Key": "handler-matrix"})
	if apply.Code != http.StatusOK || !strings.Contains(apply.Body.String(), `"version":1`) || !strings.Contains(apply.Body.String(), `"default_sku_id":`) {
		t.Fatalf("apply=%d %s", apply.Code, apply.Body.String())
	}
	if err := db.First(&project, project.ID).Error; err != nil || project.DefaultSKUID == nil || *project.DefaultSKUID == legacyDefault.ID {
		t.Fatalf("project default not switched: %+v err=%v", project, err)
	}
	config := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-config", nil, cookies)
	if config.Code != http.StatusOK || !strings.Contains(config.Body.String(), `"specification"`) {
		t.Fatalf("config=%d %s", config.Code, config.Body.String())
	}
	var active []ecommerce.CommerceSKU
	if err := db.Where("product_id=? AND status='active'", product.ID).Order("id").Find(&active).Error; err != nil || len(active) != 2 {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	nonFirstDefault := active[1].ID
	if err := db.Model(&project).Update("default_sku_id", nonFirstDefault).Error; err != nil {
		t.Fatal(err)
	}
	config = performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-config", nil, cookies)
	if config.Code != http.StatusOK || !strings.Contains(config.Body.String(), `"default_sku_id":`+itoa(nonFirstDefault)) {
		t.Fatalf("GET must expose real non-first default=%d %s", config.Code, config.Body.String())
	}
	body["expected_version"] = 1
	apply = performJSONRequestWithHeaders(t, testApp, http.MethodPut, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-matrix", body, cookies, map[string]string{"Idempotency-Key": "handler-matrix-preserve"})
	if apply.Code != http.StatusOK || !strings.Contains(apply.Body.String(), `"default_sku_id":`+itoa(nonFirstDefault)) {
		t.Fatalf("PUT must preserve real non-first default=%d %s", apply.Code, apply.Body.String())
	}
}

func TestCommerceSKUMatrixHandlersRejectCrossUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "commerce-sku-owner", "password123")
	_, foreignCookies := createLoggedInUser(t, testApp, "commerce-sku-foreign", "password123")
	product := ecommerce.CommerceProduct{UserID: owner.ID, Name: "他人商品", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"expected_version": 0, "dimensions": []any{map[string]any{"name": "颜色", "values": []any{map[string]any{"name": "红"}}}}}
	preview := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-matrix/preview", body, foreignCookies)
	put := performJSONRequestWithHeaders(t, testApp, http.MethodPut, "/api/ecommerce/products/"+itoa(product.ID)+"/sku-matrix", body, foreignCookies, map[string]string{"Idempotency-Key": "foreign"})
	for name, response := range map[string]*httptest.ResponseRecorder{"preview": preview, "put": put} {
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "ownership_mismatch") {
			t.Fatalf("%s=%d %s", name, response.Code, response.Body.String())
		}
	}
}

func TestCommerceCreativeSpecHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-spec-user", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Cup", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	missingOrigin := httptest.NewRequest(http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/creative-specs", nil)
	for _, cookie := range cookies {
		missingOrigin.AddCookie(cookie)
	}
	missingRecorder := httptest.NewRecorder()
	testApp.Router().ServeHTTP(missingRecorder, missingOrigin)
	if missingRecorder.Code != http.StatusForbidden {
		t.Fatalf("missing-origin create = %d: %s", missingRecorder.Code, missingRecorder.Body.String())
	}
	var count int64
	if err := db.Model(&ecommerce.CommerceCreativeSpec{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("creative specs after rejected create = %d, err=%v", count, err)
	}

	create := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/creative-specs", map[string]any{
		"product_facts": map[string]any{"name": "Cup", "material": "ceramic"},
	}, cookies)
	if create.Code != http.StatusCreated {
		t.Fatalf("create spec = %d: %s", create.Code, create.Body.String())
	}
	specID := commerceResponseID(t, create)

	_, otherCookies := createLoggedInUser(t, testApp, "commerce-spec-other", "password123")
	crossUser := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/creative-specs/"+itoa(specID), nil, otherCookies)
	if crossUser.Code != http.StatusNotFound {
		t.Fatalf("cross-user get spec = %d: %s", crossUser.Code, crossUser.Body.String())
	}

	assertRejectedCreativeSpecMutation := func(method, path string, body []byte) {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		resp := httptest.NewRecorder()
		testApp.Router().ServeHTTP(resp, req)
		if resp.Code != http.StatusForbidden {
			t.Fatalf("missing-origin %s %s = %d: %s", method, path, resp.Code, resp.Body.String())
		}
	}
	assertRejectedCreativeSpecMutation(http.MethodPatch, "/api/ecommerce/creative-specs/"+itoa(specID), []byte(`{"expected_version":1,"copy_blocks":[{"text":"blocked"}]}`))
	assertRejectedCreativeSpecMutation(http.MethodPost, "/api/ecommerce/creative-specs/"+itoa(specID)+"/confirm", []byte(`{}`))
	var rejectedSpec ecommerce.CommerceCreativeSpec
	if err := db.First(&rejectedSpec, specID).Error; err != nil {
		t.Fatalf("load rejected spec: %v", err)
	}
	if rejectedSpec.Version != 1 || rejectedSpec.Status != "draft" || rejectedSpec.CopyBlocksJSON != "[]" {
		t.Fatalf("rejected mutation changed spec: %#v", rejectedSpec)
	}
	var rejectedProject ecommerce.CommerceProject
	if err := db.First(&rejectedProject, project.ID).Error; err != nil {
		t.Fatalf("load project after rejected confirm: %v", err)
	}
	if rejectedProject.ActiveCreativeSpecID != nil {
		t.Fatalf("rejected confirm changed active spec: %v", *rejectedProject.ActiveCreativeSpecID)
	}

	confirm := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/creative-specs/"+itoa(specID)+"/confirm", map[string]any{}, cookies)
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm spec = %d: %s", confirm.Code, confirm.Body.String())
	}
	patch := performJSONRequest(t, testApp, http.MethodPatch, "/api/ecommerce/creative-specs/"+itoa(specID), map[string]any{
		"expected_version": 1,
		"copy_blocks":      []any{map[string]any{"text": "hello"}},
	}, cookies)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch spec = %d: %s", patch.Code, patch.Body.String())
	}
}

func TestCommerceLatestCreativeSpecHandlerRestoresUnconfirmedSpec(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-latest-spec", "password123")
	_, otherCookies := createLoggedInUser(t, testApp, "commerce-latest-spec-other", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Restore", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	otherProject := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	emptyProject := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	for _, candidate := range []*ecommerce.CommerceProject{&project, &otherProject, &emptyProject} {
		if err := db.Create(candidate).Error; err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	specs := []ecommerce.CommerceCreativeSpec{
		{UserID: user.ID, ProjectID: project.ID, Version: 1, Source: "vision", Status: "analyzing", ProductFactsJSON: `{}`, SellingPointsJSON: `[]`, CreatedAt: base, UpdatedAt: base},
		{UserID: user.ID, ProjectID: project.ID, Version: 2, Source: "manual", Status: "draft", ProductFactsJSON: `{"name":"restored"}`, SellingPointsJSON: `["durable"]`, CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)},
		{UserID: user.ID, ProjectID: otherProject.ID, Version: 1, Source: "vision", Status: "analyzing", ProductFactsJSON: `{}`, SellingPointsJSON: `[]`, CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour)},
	}
	for index := range specs {
		if err := db.Create(&specs[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	path := "/api/ecommerce/projects/" + itoa(project.ID) + "/creative-specs/latest"
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	testApp.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("latest spec = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		ID           uint           `json:"id"`
		ProjectID    uint           `json:"project_id"`
		Source       string         `json:"source"`
		Status       string         `json:"status"`
		ProductFacts map[string]any `json:"product_facts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID != specs[1].ID || payload.ProjectID != project.ID || payload.Source != "manual" || payload.Status != "draft" || payload.ProductFacts["name"] != "restored" {
		t.Fatalf("payload=%+v body=%s", payload, response.Body.String())
	}
	if crossUser := performJSONRequest(t, testApp, http.MethodGet, path, nil, otherCookies); crossUser.Code != http.StatusNotFound {
		t.Fatalf("cross-user latest = %d: %s", crossUser.Code, crossUser.Body.String())
	}
	if empty := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/projects/"+itoa(emptyProject.ID)+"/creative-specs/latest", nil, cookies); empty.Code != http.StatusNotFound {
		t.Fatalf("empty/cross-project latest = %d: %s", empty.Code, empty.Body.String())
	}
	deletionRequestedAt := base.Add(2 * time.Hour)
	if err := db.Model(&ecommerce.CommerceProject{}).Where("id = ?", project.ID).Updates(map[string]any{"status": "deletion_requested", "deletion_requested_at": deletionRequestedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if deleting := performJSONRequest(t, testApp, http.MethodGet, path, nil, cookies); deleting.Code != http.StatusConflict || !strings.Contains(deleting.Body.String(), "project_deletion_requested") {
		t.Fatalf("deletion-requested latest = %d: %s", deleting.Code, deleting.Body.String())
	}
	if err := db.Delete(&project).Error; err != nil {
		t.Fatal(err)
	}
	if deleted := performJSONRequest(t, testApp, http.MethodGet, path, nil, cookies); deleted.Code != http.StatusNotFound {
		t.Fatalf("deleted-project latest = %d: %s", deleted.Code, deleted.Body.String())
	}
}

func TestCommerceCapabilitiesHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "commerce-cap-user", "password123")
	type capabilitiesPayload struct {
		Enabled          bool `json:"enabled"`
		WorkerEnabled    bool `json:"worker_enabled"`
		WorkerConfigured bool `json:"worker_configured"`
		WorkerRunning    bool `json:"worker_running"`
	}
	assertCapabilities := func(want capabilitiesPayload) {
		t.Helper()
		resp := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/capabilities", nil, cookies)
		if resp.Code != http.StatusOK {
			t.Fatalf("capabilities = %d: %s", resp.Code, resp.Body.String())
		}
		var got capabilitiesPayload
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode capabilities: %v", err)
		}
		if got != want {
			t.Fatalf("capabilities=%+v want %+v body=%s", got, want, resp.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"provider", "bucket", "api_key", "beta_user_ids"} {
			if _, ok := body[forbidden]; ok {
				t.Fatalf("capabilities leaked %q: %s", forbidden, resp.Body.String())
			}
		}
	}

	testApp.cfg.AICommerceEnabled = false
	testApp.cfg.AICommerceWorkerEnabled = false
	assertCapabilities(capabilitiesPayload{})

	testApp.cfg.AICommerceEnabled = true
	assertCapabilities(capabilitiesPayload{Enabled: true})

	testApp.cfg.AICommerceWorkerEnabled = true
	assertCapabilities(capabilitiesPayload{Enabled: true, WorkerConfigured: true})

	worker := &ecommerce.Worker{Queue: ecommerce.NewQueue(db, testApp.commerceService, "capabilities-worker"), Handlers: map[ecommerce.JobKind]ecommerce.JobHandler{}, Poll: time.Hour}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()
	testApp.commerceWorker = worker
	assertCapabilities(capabilitiesPayload{Enabled: true, WorkerEnabled: true, WorkerConfigured: true, WorkerRunning: true})
}

func TestCommerceRecipePublishesChineseDisplayOptions(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "commerce-recipe-chinese-options", "password123")
	if err := testApp.commerceRecipes.Register(ecommerce.NewProductDetailSetCompiler(ecommerce.NewSnapshotCostResolver())); err != nil {
		t.Fatalf("register product detail recipe: %v", err)
	}
	if err := testApp.commerceRecipes.Register(commerceHandlerTestCompiler{}); err != nil {
		t.Fatalf("register legacy recipe: %v", err)
	}
	response := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/recipes?pipeline=general", nil, cookies)
	if response.Code != http.StatusOK {
		t.Fatalf("recipes = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode recipes: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("items = %d, want 2: %s", len(payload.Items), response.Body.String())
	}
	type wireOption struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	type wireRecipe struct {
		Key                   string       `json:"key"`
		Sections              []string     `json:"sections"`
		QualityTiers          []string     `json:"quality_tiers"`
		LayoutTemplates       []string     `json:"layout_templates"`
		SectionOptions        []wireOption `json:"section_options"`
		QualityOptions        []wireOption `json:"quality_options"`
		LayoutTemplateOptions []wireOption `json:"layout_template_options"`
	}
	var productDetail wireRecipe
	var productDetailRaw, legacyRaw map[string]json.RawMessage
	for _, raw := range payload.Items {
		var item wireRecipe
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("decode wire recipe: %v", err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("decode raw recipe fields: %v", err)
		}
		switch item.Key {
		case ecommerce.ProductDetailSetRecipeKey:
			productDetail, productDetailRaw = item, fields
		case "handler-poster":
			legacyRaw = fields
		}
	}
	for _, key := range []string{"section_options", "quality_options", "layout_template_options"} {
		if _, exists := productDetailRaw[key]; !exists {
			t.Fatalf("product detail response missing %q: %s", key, response.Body.String())
		}
		if _, exists := legacyRaw[key]; exists {
			t.Fatalf("legacy recipe unexpectedly contains %q: %s", key, response.Body.String())
		}
	}
	assertOptionsMatchValues := func(name string, values []string, options []wireOption) {
		t.Helper()
		if len(options) != len(values) {
			t.Fatalf("%s options length = %d, want %d", name, len(options), len(values))
		}
		for index, value := range values {
			if options[index].Value != value || strings.TrimSpace(options[index].Label) == "" {
				t.Fatalf("%s option %d = %#v, want value %q and non-empty label", name, index, options[index], value)
			}
		}
	}
	assertOptionsMatchValues("section", productDetail.Sections, productDetail.SectionOptions)
	assertOptionsMatchValues("quality", productDetail.QualityTiers, productDetail.QualityOptions)
	assertOptionsMatchValues("layout template", productDetail.LayoutTemplates, productDetail.LayoutTemplateOptions)
	wantSectionLabels := []string{"首屏主视觉", "核心卖点", "材质工艺", "细节展示", "使用场景", "规格参数", "收尾转化"}
	for index, want := range wantSectionLabels {
		if got := productDetail.SectionOptions[index].Label; got != want {
			t.Fatalf("section option %d label = %q, want %q", index, got, want)
		}
	}
	if got := []string{productDetail.QualityOptions[0].Label, productDetail.QualityOptions[1].Label}; !reflect.DeepEqual(got, []string{"标准", "高清"}) {
		t.Fatalf("quality labels = %#v", got)
	}
	if got := []string{productDetail.LayoutTemplateOptions[0].Label, productDetail.LayoutTemplateOptions[1].Label, productDetail.LayoutTemplateOptions[2].Label}; !reflect.DeepEqual(got, []string{"简洁留白", "深色渐变", "品牌色带"}) {
		t.Fatalf("layout template labels = %#v", got)
	}
}

func TestCommerceFeatureGuardBlocksEntireBusinessGroupWhenDisabled(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "commerce-feature-guard", "password123")
	testApp.cfg.AICommerceEnabled = false

	capabilities := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/capabilities", nil, cookies)
	if capabilities.Code != http.StatusOK || !strings.Contains(capabilities.Body.String(), `"enabled":false`) {
		t.Fatalf("disabled capabilities=%d %s", capabilities.Code, capabilities.Body.String())
	}

	requests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/ecommerce/recipes"},
		{http.MethodGet, "/api/ecommerce/projects"},
		{http.MethodPost, "/api/ecommerce/projects/bootstrap"},
		{http.MethodPost, "/api/ecommerce/projects/1/creative-specs"},
		{http.MethodPost, "/api/ecommerce/projects/1/creative-specs/analyze"},
		{http.MethodPatch, "/api/ecommerce/creative-specs/1"},
		{http.MethodPost, "/api/ecommerce/creative-specs/1/confirm"},
		{http.MethodPost, "/api/ecommerce/projects/1/assets/upload-policy"},
		{http.MethodGet, "/api/ecommerce/assets/1/file"},
		{http.MethodPost, "/api/ecommerce/projects/1/batches/estimate"},
		{http.MethodPost, "/api/ecommerce/projects/1/batches"},
		{http.MethodPost, "/api/ecommerce/batches/1/cancel"},
		{http.MethodPost, "/api/ecommerce/items/1/retry"},
	}
	for _, request := range requests {
		response := performJSONRequest(t, testApp, request.method, request.path, map[string]any{}, cookies)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"commerce_disabled"`) {
			t.Fatalf("disabled %s %s=%d %s", request.method, request.path, response.Code, response.Body.String())
		}
	}
	for name, model := range map[string]any{
		"products": &ecommerce.CommerceProduct{}, "projects": &ecommerce.CommerceProject{}, "specs": &ecommerce.CommerceCreativeSpec{},
		"assets": &ecommerce.CommerceAsset{}, "batches": &ecommerce.CommerceGenerationBatch{}, "jobs": &ecommerce.CommerceJob{},
	} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("disabled left %s=%d err=%v", name, count, err)
		}
	}

	testApp.cfg.AICommerceEnabled = true
	projects := performJSONRequest(t, testApp, http.MethodGet, "/api/ecommerce/projects", nil, cookies)
	if projects.Code != http.StatusOK {
		t.Fatalf("enabled projects=%d %s", projects.Code, projects.Body.String())
	}
	created := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/products", map[string]any{"name": "Guard enabled"}, cookies)
	if created.Code != http.StatusCreated {
		t.Fatalf("enabled create product=%d %s", created.Code, created.Body.String())
	}
}

func TestCommerceBatchSubmitHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-batch-submit-user", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Batch product", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	missingKey := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches", map[string]any{}, cookies)
	if missingKey.Code != http.StatusBadRequest || !strings.Contains(missingKey.Body.String(), "idempotency_key_required") {
		t.Fatalf("missing idempotency key = %d: %s", missingKey.Code, missingKey.Body.String())
	}

	missingOrigin := httptest.NewRequest(http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches", strings.NewReader(`{}`))
	missingOrigin.Header.Set("Content-Type", "application/json")
	missingOrigin.Header.Set("Idempotency-Key", "batch-key")
	for _, cookie := range cookies {
		missingOrigin.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	testApp.Router().ServeHTTP(recorder, missingOrigin)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("missing-origin submit = %d: %s", recorder.Code, recorder.Body.String())
	}
}

type commerceHandlerTestCompiler struct{}

func (commerceHandlerTestCompiler) Definition() ecommerce.RecipeDefinition {
	return ecommerce.RecipeDefinition{
		Key: "handler-poster", Pipeline: "general", Version: 1,
		AllowedOutputCounts: []int{1}, AspectRatios: []string{"1:1"}, QualityTiers: []string{"standard"},
	}
}

func (commerceHandlerTestCompiler) Compile(_ context.Context, input ecommerce.CompileInput) ([]ecommerce.CompiledGenerationItem, error) {
	credits, version := 2, input.PricingSnapshot.Version
	if len(input.PricingSnapshot.Entries) > 0 {
		credits = input.PricingSnapshot.Entries[0].Credits
	}
	return []ecommerce.CompiledGenerationItem{{
		SKUID: input.PrimarySKUID, Pipeline: input.Pipeline, RecipeKey: input.RecipeKey, RecipeVersion: input.RecipeVersion,
		SlotKey: "hero", AspectRatio: input.AspectRatio, PricingVersion: version, EstimatedCredits: credits,
	}}, nil
}

func TestCommerceBatchEstimateHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-batch-estimate", "password123")
	setUserCredits(t, testApp, user.ID, 20)
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "Estimate product", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	sku := ecommerce.CommerceSKU{UserID: user.ID, ProductID: product.ID, Code: "EST-1", Status: "active"}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatalf("create sku: %v", err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	lockedAt := time.Now().UTC()
	spec := ecommerce.CommerceCreativeSpec{
		UserID: user.ID, ProjectID: project.ID, Version: 1, Status: "confirmed", LockedAt: &lockedAt,
		ProductFactsJSON: "{}", SellingPointsJSON: "[]", ForbiddenChangesJSON: "[]", BrandToneJSON: "{}",
		ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: "[]",
	}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatalf("create creative spec: %v", err)
	}
	if err := testApp.commerceRecipes.Register(commerceHandlerTestCompiler{}); err != nil {
		t.Fatalf("register compiler: %v", err)
	}
	body := map[string]any{
		"recipe_key": "handler-poster", "recipe_version": 1, "output_count": 1,
		"creative_spec_id": spec.ID, "primary_sku_id": sku.ID, "quality_tier": "standard", "aspect_ratio": "1:1",
	}
	estimate := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches/estimate", body, cookies)
	if estimate.Code != http.StatusOK || !strings.Contains(estimate.Body.String(), "pricing_expires_at") {
		t.Fatalf("estimate = %d: %s", estimate.Code, estimate.Body.String())
	}
	var estimateBody struct {
		PricingSnapshotID string `json:"pricing_snapshot_id"`
		ETASeconds        int    `json:"eta_seconds"`
	}
	if err := json.Unmarshal(estimate.Body.Bytes(), &estimateBody); err != nil || estimateBody.PricingSnapshotID == "" || estimateBody.ETASeconds != 60 {
		t.Fatalf("decode estimate: body=%s err=%v", estimate.Body.String(), err)
	}
	body["pricing_snapshot_id"] = estimateBody.PricingSnapshotID
	submit := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches", body, cookies, map[string]string{"Idempotency-Key": "handler-batch-1"})
	if submit.Code != http.StatusCreated {
		t.Fatalf("submit = %d: %s", submit.Code, submit.Body.String())
	}
	var itemCount, jobCount int64
	if err := db.Model(&ecommerce.CommerceGenerationItem{}).Where("user_id = ?", user.ID).Count(&itemCount).Error; err != nil {
		t.Fatalf("count items: %v", err)
	}
	if err := db.Model(&ecommerce.CommerceJob{}).Where("user_id = ?", user.ID).Count(&jobCount).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if itemCount != 1 || jobCount != 1 {
		t.Fatalf("created item/job counts = %d/%d", itemCount, jobCount)
	}
}

func TestCommerceItemRetryHandlers(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "commerce-item-retry", "password123")
	setUserCredits(t, testApp, user.ID, 10)
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: 1, Title: "Retry", Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create retry project: %v", err)
	}
	parentBatch := ecommerce.CommerceGenerationBatch{
		UserID: user.ID, ProjectID: project.ID, Status: ecommerce.CommerceBatchFailed, IdempotencyKey: "retry-parent-batch",
		RequestDigest: "digest", Pipeline: "general", RecipeKey: "handler-poster", RecipeVersion: 1,
	}
	if err := db.Create(&parentBatch).Error; err != nil {
		t.Fatalf("create parent batch: %v", err)
	}
	compiledJSON, err := ecommerce.EncodeJSON(ecommerce.CompiledGenerationItem{
		SKUID: 1, Pipeline: "general", RecipeKey: "handler-poster", RecipeVersion: 1, EstimatedCredits: 2,
	})
	if err != nil {
		t.Fatalf("encode compiled item: %v", err)
	}
	parentItem := ecommerce.CommerceGenerationItem{
		UserID: user.ID, ProjectID: project.ID, BatchID: parentBatch.ID, ReservationID: 99, SKUID: 1,
		Pipeline: "general", RecipeKey: "handler-poster", RecipeVersion: 1, Status: ecommerce.CommerceItemFailed,
		IdempotencyKey: "retry-parent-item", OutputSpecJSON: compiledJSON, EstimatedCredits: 2, ReservedCredits: 2, ReleasedCredits: 2,
	}
	if err := db.Create(&parentItem).Error; err != nil {
		t.Fatalf("create parent item: %v", err)
	}
	parentBatchID, parentItemID := parentBatch.ID, parentItem.ID
	parentJob := ecommerce.CommerceJob{
		UserID: user.ID, ProjectID: parentItem.ProjectID, BatchID: &parentBatchID, GenerationItemID: &parentItemID,
		Kind: ecommerce.CommerceJobKindGenerateItem, Pipeline: parentItem.Pipeline, RecipeKey: parentItem.RecipeKey,
		Status: ecommerce.CommerceJobFailed, IdempotencyKey: "retry-parent-job", MaxAttempts: 3,
	}
	if err := db.Create(&parentJob).Error; err != nil {
		t.Fatalf("create parent job: %v", err)
	}
	missingKey := performJSONRequest(t, testApp, http.MethodPost, "/api/ecommerce/items/"+itoa(parentItem.ID)+"/retry", map[string]any{}, cookies)
	if missingKey.Code != http.StatusBadRequest || !strings.Contains(missingKey.Body.String(), "idempotency_key_required") {
		t.Fatalf("retry missing idempotency key = %d: %s", missingKey.Code, missingKey.Body.String())
	}
	var missingKeyChildren int64
	if err := db.Model(&ecommerce.CommerceGenerationItem{}).Where("parent_item_id = ?", parentItem.ID).Count(&missingKeyChildren).Error; err != nil {
		t.Fatalf("count missing-key retry children: %v", err)
	}
	if missingKeyChildren != 0 {
		t.Fatalf("missing-key retry created %d children", missingKeyChildren)
	}
	retry := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/ecommerce/items/"+itoa(parentItem.ID)+"/retry", map[string]any{}, cookies, map[string]string{"Idempotency-Key": "retry-child-batch"})
	if retry.Code != http.StatusCreated {
		t.Fatalf("retry = %d: %s", retry.Code, retry.Body.String())
	}
	var retryBody struct {
		Batch struct {
			ID         uint `json:"id"`
			ETASeconds int  `json:"eta_seconds"`
		} `json:"batch"`
	}
	if err := json.Unmarshal(retry.Body.Bytes(), &retryBody); err != nil || retryBody.Batch.ID == 0 || retryBody.Batch.ETASeconds <= 0 {
		t.Fatalf("decode retry batch: body=%s err=%v", retry.Body.String(), err)
	}
	replay := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/ecommerce/items/"+itoa(parentItem.ID)+"/retry", map[string]any{}, cookies, map[string]string{"Idempotency-Key": "retry-child-batch"})
	if replay.Code != http.StatusCreated {
		t.Fatalf("retry replay = %d: %s", replay.Code, replay.Body.String())
	}
	var replayBody struct {
		Batch struct {
			ID uint `json:"id"`
		} `json:"batch"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayBody); err != nil || replayBody.Batch.ID != retryBody.Batch.ID {
		t.Fatalf("retry replay batch = %d, want %d, err=%v", replayBody.Batch.ID, retryBody.Batch.ID, err)
	}
	conflictingParent := parentItem
	conflictingParent.ID = 0
	conflictingParent.IdempotencyKey = "retry-conflicting-parent"
	if err := db.Create(&conflictingParent).Error; err != nil {
		t.Fatalf("create conflicting retry parent: %v", err)
	}
	conflict := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/ecommerce/items/"+itoa(conflictingParent.ID)+"/retry", map[string]any{}, cookies, map[string]string{"Idempotency-Key": "retry-child-batch"})
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "idempotency_conflict") {
		t.Fatalf("retry idempotency conflict = %d: %s", conflict.Code, conflict.Body.String())
	}
	var child ecommerce.CommerceGenerationItem
	if err := db.Where("parent_item_id = ? AND user_id = ?", parentItem.ID, user.ID).First(&child).Error; err != nil {
		t.Fatalf("load retry child: %v", err)
	}
	if child.BatchID == parentBatch.ID || child.ReservationID == parentItem.ReservationID || child.Status != ecommerce.CommerceItemQueued {
		t.Fatalf("retry child = %#v", child)
	}
}

func TestCommerceCreditLedgerConcurrentReservations(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "commerce-ledger-concurrent", "password123")
	setUserCredits(t, testApp, user.ID, 10)
	sqlDB, err := testApp.db.DB()
	if err != nil {
		t.Fatalf("db connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	ledger := newCommerceCreditLedger()
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index := 0; index < 2; index++ {
		go func(index int) {
			ready.Done()
			<-start
			results <- testApp.db.Transaction(func(tx *gorm.DB) error {
				_, err := ledger.ReserveTx(context.Background(), tx, ecommerce.ReserveCreditsRequest{
					UserID: user.ID, ProjectID: 1, ScopeType: "batch", ScopeKey: itoa(uint(index + 1)),
					Amount: 8, IdempotencyKey: "concurrent-" + itoa(uint(index+1)),
				})
				return err
			})
		}(index)
	}
	ready.Wait()
	close(start)
	succeeded, insufficient := 0, 0
	for index := 0; index < 2; index++ {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, ecommerce.ErrCreditsInsufficient) {
			insufficient++
		} else {
			t.Fatalf("reservation error = %v", err)
		}
	}
	if succeeded != 1 || insufficient != 1 {
		t.Fatalf("reservation outcomes succeeded=%d insufficient=%d", succeeded, insufficient)
	}
	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 2 || balance.ReservedCredits != 8 {
		t.Fatalf("balance = available %d reserved %d", balance.AvailableCredits, balance.ReservedCredits)
	}
}

func TestCommerceCreditLedgerConcurrentDuplicateSettlement(t *testing.T) {
	db, ledger, item, reservation := newConcurrentSettlementFixture(t, "settle")
	installSettlementReadBarrier(t, db, "settle")
	req := ecommerce.SettleCreditsRequest{
		UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
		ReservationID: reservation.ReservationID, GenerationItemID: item.ID,
		HeldCredits: item.ReservedCredits, ActualCredits: item.ReservedCredits,
		IdempotencyKey: "concurrent-settle",
	}
	runConcurrentLedgerCalls(t, db, func(tx *gorm.DB) error {
		return ledger.SettleItemTx(context.Background(), tx, req)
	})
	assertConcurrentSettlementTotals(t, db, item.UserID, item.ID, reservation.ReservationID, 8, 0, 2, 0)
}

func TestCommerceCreditLedgerConcurrentDuplicateRelease(t *testing.T) {
	db, ledger, item, reservation := newConcurrentSettlementFixture(t, "release")
	installSettlementReadBarrier(t, db, "release")
	req := ecommerce.ReleaseCreditsRequest{
		UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
		ReservationID: reservation.ReservationID, GenerationItemID: item.ID,
		HeldCredits: item.ReservedCredits, Reason: "failed", IdempotencyKey: "concurrent-release",
	}
	runConcurrentLedgerCalls(t, db, func(tx *gorm.DB) error {
		return ledger.ReleaseItemTx(context.Background(), tx, req)
	})
	assertConcurrentSettlementTotals(t, db, item.UserID, item.ID, reservation.ReservationID, 10, 0, 0, 2)
}

func installSettlementReadBarrier(t *testing.T, db *gorm.DB, suffix string) {
	t.Helper()
	var arrivals atomic.Int32
	release := make(chan struct{})
	name := "test:settlement-read-barrier:" + suffix
	if err := db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "commerce_credit_settlements" || tx.RowsAffected != 0 {
			return
		}
		if arrivals.Add(1) == 2 {
			close(release)
			return
		}
		<-release
	}); err != nil {
		t.Fatalf("register settlement read barrier: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

func newConcurrentSettlementFixture(t *testing.T, suffix string) (*gorm.DB, ecommerce.CreditLedger, ecommerce.CommerceGenerationItem, ecommerce.CreditReservationSnapshot) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "concurrent-"+suffix+".sqlite")
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open concurrent sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("load concurrent sqlite connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&CreditBalance{}, &CreditTransaction{}); err != nil {
		t.Fatalf("migrate credit models: %v", err)
	}
	if err := ecommerce.MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate commerce models: %v", err)
	}
	const userID = 41
	if err := db.Create(&CreditBalance{UserID: userID, AvailableCredits: 10}).Error; err != nil {
		t.Fatalf("create balance: %v", err)
	}
	batch := ecommerce.CommerceGenerationBatch{
		UserID: userID, ProjectID: 51, Status: ecommerce.CommerceBatchRunning,
		IdempotencyKey: "concurrent-" + suffix + "-batch", RequestDigest: "digest",
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ledger := newCommerceCreditLedger()
	var reservation ecommerce.CreditReservationSnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var reserveErr error
		reservation, reserveErr = ledger.ReserveTx(context.Background(), tx, ecommerce.ReserveCreditsRequest{
			UserID: userID, ProjectID: batch.ProjectID, BatchID: &batch.ID,
			ScopeType: "batch", ScopeKey: itoa(batch.ID), Amount: 2,
			IdempotencyKey: "concurrent-" + suffix + "-reserve",
		})
		return reserveErr
	}); err != nil {
		t.Fatalf("reserve concurrent fixture: %v", err)
	}
	item := ecommerce.CommerceGenerationItem{
		UserID: userID, ProjectID: batch.ProjectID, BatchID: batch.ID, ReservationID: reservation.ReservationID,
		SKUID: 1, Pipeline: "general", RecipeKey: "poster", Status: ecommerce.CommerceItemRunning,
		IdempotencyKey: "concurrent-" + suffix + "-item", ReservedCredits: 2, EstimatedCredits: 2,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create concurrent item: %v", err)
	}
	return db, ledger, item, reservation
}

func runConcurrentLedgerCalls(t *testing.T, db *gorm.DB, call func(*gorm.DB) error) {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index := 0; index < 2; index++ {
		go func() {
			ready.Done()
			<-start
			errs <- db.Transaction(call)
		}()
	}
	ready.Wait()
	close(start)
	for index := 0; index < 2; index++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent ledger call %d: %v", index, err)
		}
	}
}

func assertConcurrentSettlementTotals(t *testing.T, db *gorm.DB, userID, itemID, reservationID uint, available, reserved, settled, released int) {
	t.Helper()
	var balance CreditBalance
	if err := db.Where("user_id = ?", userID).First(&balance).Error; err != nil {
		t.Fatalf("load concurrent balance: %v", err)
	}
	var reservation ecommerce.CommerceCreditReservation
	if err := db.First(&reservation, reservationID).Error; err != nil {
		t.Fatalf("load concurrent reservation: %v", err)
	}
	var settlements int64
	if err := db.Model(&ecommerce.CommerceCreditSettlement{}).Where("generation_item_id = ?", itemID).Count(&settlements).Error; err != nil {
		t.Fatalf("count concurrent settlements: %v", err)
	}
	if balance.AvailableCredits != available || balance.ReservedCredits != reserved || reservation.SettledCredits != settled || reservation.ReleasedCredits != released || settlements != 1 {
		t.Fatalf("concurrent totals balance=%#v reservation=%#v settlements=%d", balance, reservation, settlements)
	}
}

func TestCommerceCreditLedgerSettlesAndReleasesEachItemOnce(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "commerce-ledger-settlement", "password123")
	setUserCredits(t, testApp, user.ID, 14)
	batch := ecommerce.CommerceGenerationBatch{UserID: user.ID, ProjectID: 1, Status: ecommerce.CommerceBatchQueued, IdempotencyKey: "settlement-batch", RequestDigest: "digest"}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ledger := newCommerceCreditLedger()
	var reservation ecommerce.CreditReservationSnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		reservation, err = ledger.ReserveTx(context.Background(), tx, ecommerce.ReserveCreditsRequest{
			UserID: user.ID, ProjectID: 1, BatchID: &batch.ID, ScopeType: "batch", ScopeKey: itoa(batch.ID), Amount: 12, IdempotencyKey: "reserve-12",
		})
		return err
	}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := db.Model(&batch).Update("reservation_id", reservation.ReservationID).Error; err != nil {
		t.Fatalf("update batch reservation: %v", err)
	}
	items := make([]ecommerce.CommerceGenerationItem, 6)
	for index := range items {
		items[index] = ecommerce.CommerceGenerationItem{
			UserID: user.ID, ProjectID: 1, BatchID: batch.ID, ReservationID: reservation.ReservationID,
			SKUID: 1, Pipeline: "general", RecipeKey: "poster", Status: ecommerce.CommerceItemRunning,
			IdempotencyKey: "settlement-item-" + itoa(uint(index+1)), ReservedCredits: 2, EstimatedCredits: 2,
		}
		if err := db.Create(&items[index]).Error; err != nil {
			t.Fatalf("create item %d: %v", index, err)
		}
	}
	for index := 0; index < 4; index++ {
		req := ecommerce.SettleCreditsRequest{
			UserID: user.ID, ProjectID: 1, BatchID: batch.ID, ReservationID: reservation.ReservationID,
			GenerationItemID: items[index].ID, HeldCredits: 2, ActualCredits: 2,
			IdempotencyKey: "settle-" + itoa(items[index].ID),
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return ledger.SettleItemTx(context.Background(), tx, req) }); err != nil {
			t.Fatalf("settle item %d: %v", index, err)
		}
		if index == 0 {
			if err := db.Transaction(func(tx *gorm.DB) error { return ledger.SettleItemTx(context.Background(), tx, req) }); err != nil {
				t.Fatalf("duplicate settle item: %v", err)
			}
		}
	}
	for index := 4; index < 6; index++ {
		req := ecommerce.ReleaseCreditsRequest{
			UserID: user.ID, ProjectID: 1, BatchID: batch.ID, ReservationID: reservation.ReservationID,
			GenerationItemID: items[index].ID, HeldCredits: 2, Reason: "failed",
			IdempotencyKey: "release-" + itoa(items[index].ID),
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return ledger.ReleaseItemTx(context.Background(), tx, req) }); err != nil {
			t.Fatalf("release item %d: %v", index, err)
		}
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	var storedReservation ecommerce.CommerceCreditReservation
	if err := db.First(&storedReservation, reservation.ReservationID).Error; err != nil {
		t.Fatalf("load reservation: %v", err)
	}
	if balance.AvailableCredits != 6 || balance.ReservedCredits != 0 || storedReservation.SettledCredits != 8 || storedReservation.ReleasedCredits != 4 {
		t.Fatalf("settlement totals balance=%#v reservation=%#v", balance, storedReservation)
	}
	var settlementCount int64
	if err := db.Model(&ecommerce.CommerceCreditSettlement{}).Where("reservation_id = ?", reservation.ReservationID).Count(&settlementCount).Error; err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if settlementCount != 6 {
		t.Fatalf("settlement count = %d, want 6", settlementCount)
	}

	anomalyBatch := ecommerce.CommerceGenerationBatch{UserID: user.ID, ProjectID: 1, Status: ecommerce.CommerceBatchRunning, IdempotencyKey: "anomaly-batch", RequestDigest: "anomaly"}
	if err := db.Create(&anomalyBatch).Error; err != nil {
		t.Fatalf("create anomaly batch: %v", err)
	}
	var anomalyReservation ecommerce.CreditReservationSnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		anomalyReservation, err = ledger.ReserveTx(context.Background(), tx, ecommerce.ReserveCreditsRequest{
			UserID: user.ID, ProjectID: 1, BatchID: &anomalyBatch.ID, ScopeType: "batch", ScopeKey: itoa(anomalyBatch.ID), Amount: 2, IdempotencyKey: "anomaly-reserve",
		})
		return err
	}); err != nil {
		t.Fatalf("reserve anomaly item: %v", err)
	}
	anomalyItem := ecommerce.CommerceGenerationItem{
		UserID: user.ID, ProjectID: 1, BatchID: anomalyBatch.ID, ReservationID: anomalyReservation.ReservationID,
		SKUID: 1, Pipeline: "general", RecipeKey: "poster", Status: ecommerce.CommerceItemRunning,
		IdempotencyKey: "anomaly-item", ReservedCredits: 2, EstimatedCredits: 2,
	}
	if err := db.Create(&anomalyItem).Error; err != nil {
		t.Fatalf("create anomaly item: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return ledger.SettleItemTx(context.Background(), tx, ecommerce.SettleCreditsRequest{
			UserID: user.ID, ProjectID: 1, BatchID: anomalyBatch.ID, ReservationID: anomalyReservation.ReservationID,
			GenerationItemID: anomalyItem.ID, HeldCredits: 2, ActualCredits: 5, IdempotencyKey: "anomaly-settle",
		})
	}); err != nil {
		t.Fatalf("settle anomaly item: %v", err)
	}
	var anomalySettlement ecommerce.CommerceCreditSettlement
	if err := db.Where("generation_item_id = ?", anomalyItem.ID).First(&anomalySettlement).Error; err != nil {
		t.Fatalf("load anomaly settlement: %v", err)
	}
	var anomalyEvents int64
	if err := db.Model(&ecommerce.CommerceEvent{}).Where("event_type = ? AND entity_id = ?", "billing_actual_exceeds_hold", anomalyItem.ID).Count(&anomalyEvents).Error; err != nil {
		t.Fatalf("count anomaly events: %v", err)
	}
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("reload anomaly balance: %v", err)
	}
	if anomalySettlement.SettledCredits != 2 || anomalySettlement.ReleasedCredits != 0 || anomalySettlement.AnomalyCode != "actual_exceeds_hold" || anomalyEvents != 1 {
		t.Fatalf("anomaly settlement = %#v events=%d", anomalySettlement, anomalyEvents)
	}
	if balance.AvailableCredits != 4 || balance.ReservedCredits != 0 {
		t.Fatalf("anomaly charged beyond hold: available=%d reserved=%d", balance.AvailableCredits, balance.ReservedCredits)
	}
}

func TestCommerceCompleteGenerationItemDiscardsLateResultAfterCancel(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "commerce-late-result", "password123")
	setUserCredits(t, testApp, user.ID, 4)
	batch := ecommerce.CommerceGenerationBatch{
		UserID: user.ID, ProjectID: 1, Status: ecommerce.CommerceBatchRunning,
		IdempotencyKey: "late-result-batch", RequestDigest: "digest", TotalItems: 1, RunningItems: 1,
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ledger := newCommerceCreditLedger()
	var reservation ecommerce.CreditReservationSnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		reservation, err = ledger.ReserveTx(context.Background(), tx, ecommerce.ReserveCreditsRequest{
			UserID: user.ID, ProjectID: 1, BatchID: &batch.ID, ScopeType: "batch", ScopeKey: itoa(batch.ID), Amount: 2, IdempotencyKey: "late-result-reserve",
		})
		return err
	}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	now := time.Now().UTC()
	item := ecommerce.CommerceGenerationItem{
		UserID: user.ID, ProjectID: 1, BatchID: batch.ID, ReservationID: reservation.ReservationID,
		SKUID: 1, Pipeline: "general", RecipeKey: "poster", Status: ecommerce.CommerceItemRunning,
		IdempotencyKey: "late-result-item", ReservedCredits: 2, EstimatedCredits: 2, CancelRequestedAt: &now,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	batchID, itemID := batch.ID, item.ID
	job := ecommerce.CommerceJob{
		UserID: user.ID, ProjectID: 1, BatchID: &batchID, GenerationItemID: &itemID,
		Kind: ecommerce.CommerceJobKindGenerateItem, Pipeline: "general", RecipeKey: "poster",
		Status: ecommerce.CommerceJobRunning, IdempotencyKey: "late-result-job",
		LeaseOwner: "worker-1", LeaseToken: "lease-token", CancelRequestedAt: &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := testApp.commerceService.CompleteGenerationItem(context.Background(), ecommerce.LeaseIdentity{
		JobID: job.ID, LeaseOwner: job.LeaseOwner, LeaseToken: job.LeaseToken,
	}, item.ID, ecommerce.ExecutionResult{ActualCredits: 2, GenerationRecordID: 99}); err != nil {
		t.Fatalf("complete canceled item: %v", err)
	}
	if err := db.First(&item, item.ID).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if item.Status != ecommerce.CommerceItemCanceled || item.GenerationRecordID != nil || item.SettledCredits != 0 || item.ReleasedCredits != 2 {
		t.Fatalf("late result was not discarded: %#v", item)
	}
	if balance.AvailableCredits != 4 || balance.ReservedCredits != 0 {
		t.Fatalf("late result balance = available %d reserved %d", balance.AvailableCredits, balance.ReservedCredits)
	}
}

func commerceResponseID(t *testing.T, response *httptest.ResponseRecorder) uint {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	value, ok := body["id"].(float64)
	if !ok || value <= 0 {
		t.Fatalf("response missing id: %s", response.Body.String())
	}
	return uint(value)
}
