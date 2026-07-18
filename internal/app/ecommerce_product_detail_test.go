package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

func TestCommerceProductDetailRecipeDefinitionHTTPContract(t *testing.T) {
	a, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, a, "product-detail-recipe-definition", "password123")
	if err := a.commerceRecipes.Register(ecommerce.NewProductDetailSetCompiler(ecommerce.NewSnapshotCostResolver())); err != nil {
		t.Fatal(err)
	}

	response := performJSONRequest(t, a, http.MethodGet, "/api/ecommerce/recipes?pipeline=general", nil, cookies)
	if response.Code != http.StatusOK {
		t.Fatalf("recipes=%d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			Key             string         `json:"key"`
			Version         int            `json:"version"`
			Title           string         `json:"title"`
			Pipeline        string         `json:"pipeline"`
			AspectRatios    []string       `json:"aspect_ratios"`
			QualityTiers    []string       `json:"quality_tiers"`
			Parameters      map[string]any `json:"parameters"`
			Capabilities    []string       `json:"capabilities"`
			Sections        []string       `json:"sections"`
			LayoutTemplates []string       `json:"layout_templates"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items=%d body=%s", len(payload.Items), response.Body.String())
	}
	definition := payload.Items[0]
	if definition.Key != ecommerce.ProductDetailSetRecipeKey || definition.Version != ecommerce.ProductDetailSetVersion || definition.Title == "" || definition.Pipeline != "general" {
		t.Fatalf("identity=%+v", definition)
	}
	if got, want := strings.Join(definition.AspectRatios, ","), "1:1,3:4,4:5,9:16"; got != want {
		t.Fatalf("aspect_ratios=%q want %q", got, want)
	}
	if got, want := strings.Join(definition.QualityTiers, ","), "standard,high_fidelity"; got != want {
		t.Fatalf("quality_tiers=%q want %q", got, want)
	}
	if definition.Parameters["layout_template"] != "clean" {
		t.Fatalf("parameters=%v", definition.Parameters)
	}
	if got, want := strings.Join(definition.Capabilities, ","), "image"; got != want {
		t.Fatalf("capabilities=%q want %q", got, want)
	}
	if got, want := strings.Join(definition.Sections, ","), "hero,selling_points,material,detail,usage,specification,closing"; got != want {
		t.Fatalf("sections=%q want %q", got, want)
	}
	if got, want := strings.Join(definition.LayoutTemplates, ","), "clean,dark_gradient,brand_band"; got != want {
		t.Fatalf("layout_templates=%q want %q", got, want)
	}

	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, legacyKey := range []string{"Key", "Version", "Title", "Pipeline", "AspectRatios", "QualityTiers", "DefaultParameters"} {
		if _, exists := raw.Items[0][legacyKey]; exists {
			t.Fatalf("legacy JSON key %q leaked: %s", legacyKey, response.Body.String())
		}
	}
}

func TestCommerceProductDetailEstimatePresentationHTTPContract(t *testing.T) {
	a, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, a, "product-detail-estimate-presentation", "password123")
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "杯子", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	sku := ecommerce.CommerceSKU{UserID: user.ID, ProductID: product.ID, Code: "CUP-FROZEN", Color: "红色", Size: "标准", Status: "active"}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatal(err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	locked := time.Now().UTC()
	spec := ecommerce.CommerceCreativeSpec{UserID: user.ID, ProjectID: project.ID, Version: 1, Status: "confirmed", LockedAt: &locked, ProductFactsJSON: `{}`, SellingPointsJSON: `[]`, ForbiddenChangesJSON: `[]`, BrandToneJSON: `{}`, ShotPlanJSON: `[]`, CopyBlocksJSON: `[]`, RiskNoticesJSON: `[]`, SourceAssetIDsJSON: `[]`}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	asset := ecommerce.CommerceAsset{UserID: user.ID, ProjectID: project.ID, ReferenceAssetID: 991, Role: "product_front", Lifecycle: ecommerce.AssetLifecycleProject}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.commerceRecipes.Register(ecommerce.NewProductDetailSetCompiler(ecommerce.NewSnapshotCostResolver())); err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(t, a, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches/estimate", map[string]any{
		"recipe_key": ecommerce.ProductDetailSetRecipeKey, "recipe_version": 1, "output_count": 2,
		"creative_spec_id": spec.ID, "primary_sku_id": sku.ID, "selected_sku_ids": []uint{sku.ID},
		"quality_tier": "standard", "aspect_ratio": "4:5", "asset_bindings": map[string]any{"product_front": []uint{asset.ID}}, "parameters": map[string]any{"detail_sections": []string{"hero", "closing"}, "layout_template": "clean"},
	}, cookies)
	if response.Code != http.StatusOK {
		t.Fatalf("estimate=%d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			SKUID             uint           `json:"sku_id"`
			Scope             string         `json:"scope"`
			Section           string         `json:"section"`
			SKUCode           string         `json:"sku_code"`
			SpecificationPath string         `json:"specification_path"`
			SKUSnapshot       map[string]any `json:"sku_snapshot"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 2 || payload.Items[0].Scope != "sku" || payload.Items[0].Section != "hero" || payload.Items[0].SKUID != sku.ID || payload.Items[0].SKUCode != "CUP-FROZEN" || payload.Items[0].SpecificationPath != "红色/标准" || payload.Items[0].SKUSnapshot["code"] != "CUP-FROZEN" {
		t.Fatalf("SKU estimate=%+v body=%s", payload.Items, response.Body.String())
	}
	if payload.Items[1].Scope != "shared" || payload.Items[1].Section != "closing" || payload.Items[1].SKUID != 0 || len(payload.Items[1].SKUSnapshot) != 0 {
		t.Fatalf("shared estimate=%+v body=%s", payload.Items, response.Body.String())
	}
}

func TestCommerceProductDetailHTTPWorkerClosure(t *testing.T) {
	provider := &stubProvider{errs: []*ProviderError{{Code: "provider_policy_rejected", Message: "blocked"}}, results: []ImageGenerationResult{{Base64Image: productDetailTestPNG(t, 1024, 1536), MIMEType: "image/png", ProviderRequestID: "detail-1"}, {Base64Image: productDetailTestPNG(t, 1024, 1536), MIMEType: "image/png", ProviderRequestID: "detail-2"}}}
	a, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, a, "product-detail-e2e", "password123")
	setUserCredits(t, a, user.ID, 20)
	other, otherCookies := createLoggedInUser(t, a, "product-detail-other", "password123")
	_ = other
	product := ecommerce.CommerceProduct{UserID: user.ID, Name: "杯子", Status: "active"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	sku := ecommerce.CommerceSKU{UserID: user.ID, ProductID: product.ID, Code: "CUP-OLD", Color: "红色", Size: "标准", Status: "active"}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatal(err)
	}
	project := ecommerce.CommerceProject{UserID: user.ID, ProductID: product.ID, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	locked := time.Now().UTC()
	spec := ecommerce.CommerceCreativeSpec{UserID: user.ID, ProjectID: project.ID, Version: 1, Status: "confirmed", LockedAt: &locked, ProductFactsJSON: `{"name":"杯子"}`, SellingPointsJSON: `["保温"]`, ForbiddenChangesJSON: `[]`, BrandToneJSON: `{}`, ShotPlanJSON: `[]`, CopyBlocksJSON: `[{"text":"全天保温"}]`, RiskNoticesJSON: `[]`, SourceAssetIDsJSON: `[]`}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	originalReference := productDetailTestPNG(t, 1024, 1024)
	key, mime, err := a.assetStores.CommercePrivate.SaveBase64(originalReference, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.commerceAssets.EnsureObjectGuard(context.Background(), user.ID, StorageScopeCommercePrivate, key); err != nil {
		t.Fatal(err)
	}
	reference := ReferenceAsset{UserID: user.ID, AssetKey: key, MIMEType: mime, StorageScope: StorageScopeCommercePrivate}
	if err := db.Create(&reference).Error; err != nil {
		t.Fatal(err)
	}
	asset := ecommerce.CommerceAsset{UserID: user.ID, ProjectID: project.ID, ReferenceAssetID: reference.ID, SKUID: &sku.ID, Role: "product_front", Lifecycle: ecommerce.AssetLifecycleProject}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.commerceRecipes.Register(ecommerce.NewProductDetailSetCompiler(ecommerce.NewSnapshotCostResolver())); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"recipe_key": ecommerce.ProductDetailSetRecipeKey, "recipe_version": 1, "output_count": 2, "creative_spec_id": spec.ID, "primary_sku_id": sku.ID, "quality_tier": "standard", "aspect_ratio": "4:5", "asset_bindings": map[string]any{"product_front": []uint{asset.ID}}, "parameters": map[string]any{"detail_sections": []string{"hero", "closing"}, "layout_template": "clean"}}
	estimate := performJSONRequest(t, a, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches/estimate", body, cookies)
	if estimate.Code != http.StatusOK {
		t.Fatalf("estimate=%d %s", estimate.Code, estimate.Body.String())
	}
	var estimatePayload struct {
		PricingSnapshotID string `json:"pricing_snapshot_id"`
		Items             []struct {
			SKUID             uint           `json:"sku_id"`
			Scope             string         `json:"scope"`
			Section           string         `json:"section"`
			SKUCode           string         `json:"sku_code"`
			SpecificationPath string         `json:"specification_path"`
			SKUSnapshot       map[string]any `json:"sku_snapshot"`
		} `json:"items"`
	}
	if err := json.Unmarshal(estimate.Body.Bytes(), &estimatePayload); err != nil {
		t.Fatal(err)
	}
	if len(estimatePayload.Items) != 2 {
		t.Fatalf("estimate items=%+v body=%s", estimatePayload.Items, estimate.Body.String())
	}
	if got := estimatePayload.Items[0]; got.Scope != "sku" || got.Section != "hero" || got.SKUID != sku.ID || got.SKUCode != "CUP-OLD" || got.SpecificationPath != "红色/标准" || got.SKUSnapshot["code"] != "CUP-OLD" {
		t.Fatalf("estimate SKU item=%+v body=%s", got, estimate.Body.String())
	}
	if got := estimatePayload.Items[1]; got.Scope != "shared" || got.Section != "closing" || got.SKUID != 0 || len(got.SKUSnapshot) != 0 {
		t.Fatalf("estimate shared item=%+v body=%s", got, estimate.Body.String())
	}
	body["pricing_snapshot_id"] = estimatePayload.PricingSnapshotID
	if err := db.Model(&ecommerce.CommercePricingSnapshot{}).Where("id = ?", estimatePayload.PricingSnapshotID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	stale := performJSONRequestWithHeaders(t, a, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches", body, cookies, map[string]string{"Idempotency-Key": "detail-stale"})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "pricing_snapshot_stale") {
		t.Fatalf("stale submit=%d %s", stale.Code, stale.Body.String())
	}
	estimate = performJSONRequest(t, a, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches/estimate", body, cookies)
	if estimate.Code != http.StatusOK {
		t.Fatalf("re-estimate=%d %s", estimate.Code, estimate.Body.String())
	}
	if err := json.Unmarshal(estimate.Body.Bytes(), &estimatePayload); err != nil {
		t.Fatal(err)
	}
	body["pricing_snapshot_id"] = estimatePayload.PricingSnapshotID
	submit := performJSONRequestWithHeaders(t, a, http.MethodPost, "/api/ecommerce/projects/"+itoa(project.ID)+"/batches", body, cookies, map[string]string{"Idempotency-Key": "detail-e2e"})
	if submit.Code != http.StatusCreated {
		t.Fatalf("submit=%d %s", submit.Code, submit.Body.String())
	}
	var submitted struct {
		Batch struct {
			ID uint `json:"id"`
		} `json:"batch"`
	}
	if err := json.Unmarshal(submit.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	driftKey, driftMIME, err := a.assetStores.CommercePrivate.SaveBase64(productDetailTestPNG(t, 512, 512), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.commerceAssets.EnsureObjectGuard(context.Background(), user.ID, StorageScopeCommercePrivate, driftKey); err != nil {
		t.Fatal(err)
	}
	driftReference := ReferenceAsset{UserID: user.ID, AssetKey: driftKey, MIMEType: driftMIME, StorageScope: StorageScopeCommercePrivate}
	if err := db.Create(&driftReference).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ecommerce.CommerceAsset{}).Where("id = ?", asset.ID).Update("reference_asset_id", driftReference.ID).Error; err != nil {
		t.Fatal(err)
	}
	executors := ecommerce.NewExecutorRegistry()
	if err := executors.Register(ecommerce.NewKeyBoundExecutor(ecommerce.ExecutorKey{Pipeline: "general", RecipeKey: ecommerce.ProductDetailSetRecipeKey}, []int{1}, &commerceGenerationBackend{app: a})); err != nil {
		t.Fatal(err)
	}
	queue := ecommerce.NewQueue(db, a.commerceService, "detail-e2e")
	queue.LateResultDiscarder = a.discardLateCommerceResultTx
	// The real image post-process can take longer than SQLite's write-lock window.
	// A production-like lease avoids a test-only heartbeat lock race while still
	// exercising the real worker and requiring partial success before retrying.
	worker := &ecommerce.Worker{Queue: queue, Handlers: map[ecommerce.JobKind]ecommerce.JobHandler{ecommerce.CommerceJobKindGenerateItem: &ecommerce.GenerateItemJobHandler{Executors: executors}}, Concurrency: 1, Lease: 30 * time.Second, Poll: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()
	waitCommerceBatchStatus(t, db, submitted.Batch.ID, ecommerce.CommerceBatchPartialSucceeded)
	if err := db.Model(&ecommerce.CommerceSKU{}).Where("id = ?", sku.ID).Updates(map[string]any{"code": "CUP-RENAMED", "color": "蓝色", "size": "加大"}).Error; err != nil {
		t.Fatal(err)
	}
	var failed ecommerce.CommerceGenerationItem
	if err := db.Where("batch_id = ? AND status = ?", submitted.Batch.ID, ecommerce.CommerceItemFailed).First(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.ErrorCode != "provider_policy_rejected" {
		t.Fatalf("failed error code=%q", failed.ErrorCode)
	}
	get := performJSONRequest(t, a, http.MethodGet, "/api/ecommerce/batches/"+itoa(submitted.Batch.ID), nil, cookies)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"output_snapshot"`) || !strings.Contains(get.Body.String(), `"1024x1280"`) {
		t.Fatalf("get=%d %s", get.Code, get.Body.String())
	}
	var persisted struct {
		Items []struct {
			SKUID             uint           `json:"sku_id"`
			Scope             string         `json:"scope"`
			Section           string         `json:"section"`
			ProgressPercent   int            `json:"progress_percent"`
			SKUCode           string         `json:"sku_code"`
			SpecificationPath string         `json:"specification_path"`
			SKUSnapshot       map[string]any `json:"sku_snapshot"`
		} `json:"items"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Items) != 2 {
		t.Fatalf("persisted items=%+v", persisted.Items)
	}
	for _, item := range persisted.Items {
		if item.ProgressPercent != 100 {
			t.Fatalf("persisted progress=%+v", item)
		}
		if item.Scope == "sku" && (item.Section != "hero" || item.SKUCode != "CUP-OLD" || item.SpecificationPath != "红色/标准" || item.SKUSnapshot["code"] != "CUP-OLD") {
			t.Fatalf("persisted frozen SKU item=%+v", item)
		}
		if item.Scope == "shared" && (item.SKUID != 0 || item.Section != "closing" || len(item.SKUSnapshot) != 0) {
			t.Fatalf("persisted shared item=%+v", item)
		}
	}
	cross := performJSONRequest(t, a, http.MethodGet, "/api/ecommerce/batches/"+itoa(submitted.Batch.ID), nil, otherCookies)
	if cross.Code != http.StatusNotFound {
		t.Fatalf("cross user=%d %s", cross.Code, cross.Body.String())
	}
	retry := performJSONRequestWithHeaders(t, a, http.MethodPost, "/api/ecommerce/items/"+itoa(failed.ID)+"/retry", map[string]any{}, cookies, map[string]string{"Idempotency-Key": "detail-retry"})
	if retry.Code != http.StatusCreated {
		t.Fatalf("retry=%d %s", retry.Code, retry.Body.String())
	}
	var retryPayload struct {
		Batch struct {
			ID uint `json:"id"`
		} `json:"batch"`
	}
	if err := json.Unmarshal(retry.Body.Bytes(), &retryPayload); err != nil {
		t.Fatal(err)
	}
	waitCommerceBatchStatus(t, db, retryPayload.Batch.ID, ecommerce.CommerceBatchSucceeded)
	var work Work
	if err := db.Where("user_id = ?", user.ID).Order("id desc").First(&work).Error; err != nil {
		t.Fatal(err)
	}
	if work.Category != WorkCategoryProductMain || work.Visibility != WorkVisibilityPrivate || work.StorageScope != StorageScopeCommercePrivate {
		t.Fatalf("work=%#v", work)
	}
	var workCount int64
	if err := db.Model(&Work{}).Where("user_id = ? AND category = ?", user.ID, WorkCategoryProductMain).Count(&workCount).Error; err != nil {
		t.Fatal(err)
	}
	if workCount != 2 {
		t.Fatalf("work count=%d", workCount)
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls=%d, want policy reject + success + single retry", provider.calls)
	}
	for _, index := range []int{0, 2} {
		input := provider.inputs[index]
		if len(input.ReferenceImages) != 1 || input.ReferenceImages[0].InputURL != "data:image/png;base64,"+originalReference {
			t.Fatalf("SKU provider input %d used drifted reference: %#v", index, input.ReferenceImages)
		}
	}
	if len(provider.inputs[1].ReferenceImages) != 0 {
		t.Fatalf("shared provider input inherited SKU reference: %#v", provider.inputs[1].ReferenceImages)
	}
}

func TestCommerceProductDetailLateCancellationCompensation(t *testing.T) {
	a, db := newTestApp(t, &stubProvider{})
	user := User{Username: "late-compensation", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.commerceAssets.EnsureObjectGuard(context.Background(), user.ID, StorageScopeCommercePrivate, "commerce/late.png"); err != nil {
		t.Fatal(err)
	}
	record := GenerationRecord{UserID: user.ID, Status: GenerationStatusSucceeded, AssetKey: "commerce/late.png", StorageScope: StorageScopeCommercePrivate}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	work := Work{UserID: user.ID, GenerationRecordID: record.ID, Status: GenerationStatusSucceeded, Visibility: WorkVisibilityPrivate, Category: WorkCategoryProductMain, AssetKey: record.AssetKey, StorageScope: StorageScopeCommercePrivate}
	if err := db.Create(&work).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&record).Update("work_id", work.ID).Error; err != nil {
		t.Fatal(err)
	}
	job := ecommerce.CommerceJob{UserID: user.ID, ProjectID: 77}
	wrongJob := job
	wrongJob.UserID++
	if err := db.Transaction(func(tx *gorm.DB) error {
		return a.discardLateCommerceResultTx(context.Background(), tx, wrongJob, ecommerce.ExecutionResult{GenerationRecordID: record.ID, WorkID: work.ID})
	}); !errors.Is(err, ecommerce.ErrOwnershipMismatch) {
		t.Fatalf("wrong owner error=%v", err)
	}
	if err := db.First(&Work{}, work.ID).Error; err != nil {
		t.Fatalf("wrong-owner compensation deleted work: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return a.discardLateCommerceResultTx(context.Background(), tx, job, ecommerce.ExecutionResult{GenerationRecordID: record.ID, WorkID: work.ID})
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&Work{}, work.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("late work remains visible: %v", err)
	}
	if err := db.First(&record, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.WorkID != nil || record.AssetKey != "" || record.Status != GenerationStatusFailed || record.ErrorCode != imageGenerationCancelledErrorCode {
		t.Fatalf("record=%#v", record)
	}
	var cleanup ecommerce.CommerceObjectCleanup
	if err := db.Where("work_id = ?", work.ID).First(&cleanup).Error; err != nil {
		t.Fatal(err)
	}
	if cleanup.ProjectID != job.ProjectID || cleanup.ObjectKey != "commerce/late.png" || cleanup.Reason != "late_result_discarded" {
		t.Fatalf("cleanup=%#v", cleanup)
	}
}

func productDetailTestPNG(t *testing.T, width, height int) string {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(out.Bytes())
}

func waitCommerceItemStatus(t *testing.T, db *gorm.DB, batchID uint, status ecommerce.CommerceItemStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var item ecommerce.CommerceGenerationItem
		if err := db.Where("batch_id = ?", batchID).First(&item).Error; err == nil && item.Status == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var item ecommerce.CommerceGenerationItem
	_ = db.Where("batch_id = ?", batchID).First(&item).Error
	t.Fatalf("item status=%s error=%s", item.Status, item.ErrorMessage)
}

func waitCommerceBatchStatus(t *testing.T, db *gorm.DB, batchID uint, status ecommerce.CommerceBatchStatus) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var batch ecommerce.CommerceGenerationBatch
		if err := db.First(&batch, batchID).Error; err == nil && batch.Status == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var batch ecommerce.CommerceGenerationBatch
	_ = db.First(&batch, batchID).Error
	var items []ecommerce.CommerceGenerationItem
	_ = db.Where("batch_id = ?", batchID).Order("id").Find(&items).Error
	t.Fatalf("batch status=%s counts=%d/%d items=%+v", batch.Status, batch.SucceededItems, batch.FailedItems, items)
}

func TestCommerceGeneralExecutorUsesFrozenRouteAfterPolicyChange(t *testing.T) {
	a, db := newTestApp(t, &stubProvider{})
	user := User{Username: "frozen-route", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	key, mime, err := a.assetStores.CommercePrivate.SaveBase64(productDetailTestPNG(t, 32, 32), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.commerceAssets.EnsureObjectGuard(context.Background(), user.ID, StorageScopeCommercePrivate, key); err != nil {
		t.Fatal(err)
	}
	ref := ReferenceAsset{UserID: user.ID, AssetKey: key, MIMEType: mime, StorageScope: StorageScopeCommercePrivate}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatal(err)
	}
	binding := ecommerce.CommerceAsset{UserID: user.ID, ProjectID: 77, ReferenceAssetID: ref.ID, Role: "product_front", Lifecycle: ecommerce.AssetLifecycleProject}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	referenceSnapshot, _ := ecommerce.EncodeJSON(ecommerce.ExecutionReferenceSetSnapshot{References: []ecommerce.ExecutionReferenceSnapshot{{CommerceAssetID: binding.ID, ReferenceAssetID: ref.ID, Role: "product_front", Order: 0}}})
	settings, err := a.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	if err != nil || len(candidates) == 0 {
		t.Fatalf("candidates=%d err=%v", len(candidates), err)
	}
	frozen := candidates[0]
	snapshot, err := ecommerce.EncodeJSON(map[string]any{"candidates": []map[string]any{{"model_id": frozen.Model.ID, "channel_id": frozen.Channel.ID, "provider_id": frozen.Provider.ID, "runtime_model": "frozen-runtime", "endpoint": frozen.Channel.Endpoint, "route_order": 0}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Where("modality = ?", ModelConfigTypeImage).Delete(&ModelRoutingPolicy{}).Error; err != nil {
		t.Fatal(err)
	}
	job, err := a.commerceGenerationJob(context.Background(), ecommerce.ItemExecutionRequest{Item: ecommerce.CommerceGenerationItem{UserID: user.ID, ProjectID: 77}, Compiled: ecommerce.CompiledGenerationItem{RecipeKey: ecommerce.ProductDetailSetRecipeKey, AspectRatio: "1:1", ModelSnapshotJSON: snapshot, ExecutionReferenceSnapshotJSON: referenceSnapshot}})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.ModelCenterCandidates) != 1 || job.ModelCenterCandidates[0].Channel.ID != frozen.Channel.ID || job.ModelCenterCandidates[0].Channel.RuntimeModel != "frozen-runtime" {
		payload, _ := json.Marshal(job.ModelCenterCandidates)
		t.Fatalf("candidates=%s", payload)
	}
}

func TestCommerceGeneralRegistryAssemblyConditional(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:commerce-detail-assembly?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ModelConfig{}, &ModelCatalog{}, &ModelProvider{}, &ModelChannel{}, &ModelRoutingPolicy{}, &ModelRoutingEntry{}, &AppSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&AppSettings{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: Config{AICommerceEnabled: true}, db: db, commerceRecipes: ecommerce.NewRegistry()}
	if err := a.configureCommerceProductDetailRecipe(); err != nil {
		t.Fatal(err)
	}
	if definitions := a.commerceRecipes.List("general"); len(definitions) != 0 {
		t.Fatalf("recipes without model = %#v", definitions)
	}
	model := ModelCatalog{Name: "detail image", Modality: ModelConfigTypeImage, Status: ModelCenterStatusOnline, Visibility: ModelCenterVisibilityPublic, DefaultCreditsCost: 2}
	if err := db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.configureCommerceProductDetailRecipe(); err != nil {
		t.Fatal(err)
	}
	if definitions := a.commerceRecipes.List("general"); len(definitions) != 0 {
		t.Fatalf("catalog-only model exposed recipe = %#v", definitions)
	}
	provider := ModelProvider{Name: "provider", Provider: "openai", Status: ModelCenterStatusOnline}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	channel := ModelChannel{ModelID: model.ID, ProviderID: provider.ID, Name: "route", RuntimeModel: "detail-runtime", Endpoint: "/v1/images/generations", Status: ModelCenterStatusOnline, HealthStatus: ModelChannelHealthHealthy}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	var policy ModelRoutingPolicy
	if err := db.Where("modality = ?", ModelConfigTypeImage).First(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&policy).Updates(map[string]any{"default_model_id": model.ID, "routing_enabled": true, "routing_strategy": ModelRoutingStrategyDefault, "source": ModelRoutingSourceModelCenter}).Error; err != nil {
		t.Fatal(err)
	}
	entry := ModelRoutingEntry{PolicyID: policy.ID, ModelID: model.ID, ChannelID: channel.ID, Enabled: true, Priority: 1}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.configureCommerceProductDetailRecipe(); err != nil {
		t.Fatal(err)
	}
	definitions := a.commerceRecipes.List("general")
	if len(definitions) != 1 || definitions[0].Key != ecommerce.ProductDetailSetRecipeKey {
		t.Fatalf("recipes = %#v", definitions)
	}
}

func TestCommerceProductDetailStrictJSONRejectsDuplicateReservedKeys(t *testing.T) {
	for _, raw := range []string{
		`{"recipe_key":"product_detail_set","recipe_key":"other"}`,
		`{"parameters":{"detail_sections":["hero"],"detail_sections":["closing"]}}`,
		`{"asset_bindings":{"product_front":[1],"product_front":[2]}}`,
	} {
		if err := rejectDuplicateJSONKeys(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted duplicate JSON: %s", raw)
		}
	}
	if err := rejectDuplicateJSONKeys(strings.NewReader(`{"parameters":{"detail_sections":["hero"],"layout_template":"clean"}}`)); err != nil {
		t.Fatalf("valid JSON: %v", err)
	}
}
