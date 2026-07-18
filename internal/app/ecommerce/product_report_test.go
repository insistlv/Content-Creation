package ecommerce

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCommerceVisionAnalyzer struct {
	raw string
	err error
}

type countingCommerceVisionAnalyzer struct{ calls int }

func (f *countingCommerceVisionAnalyzer) AnalyzeProduct(context.Context, ProductAnalysisRequest) (string, error) {
	f.calls++
	return `{"observed_facts":[],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":[]}`, nil
}

type retryableVisionError struct{}

func (retryableVisionError) Error() string   { return "temporary provider detail" }
func (retryableVisionError) Retryable() bool { return true }

func (f fakeCommerceVisionAnalyzer) AnalyzeProduct(context.Context, ProductAnalysisRequest) (string, error) {
	return f.raw, f.err
}

func TestProductAnalysisAssetRolesAreControlled(t *testing.T) {
	roles := ProductAnalysisAssetRoles()
	if got, want := strings.Join(roles, ","), "product,product_front,product_back,product_detail"; got != want {
		t.Fatalf("analysis roles = %q, want %q", got, want)
	}
	roles[0] = "logo"
	if got := strings.Join(ProductAnalysisAssetRoles(), ","); got != "product,product_front,product_back,product_detail" {
		t.Fatalf("caller mutated analysis roles: %q", got)
	}
}

func TestBootstrapProjectAtomicIdempotency(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	input := BootstrapProjectInput{Title: "保温杯", Category: "家居", SKUCode: "CUP-001", Pipeline: "general"}

	first, err := service.BootstrapProject(ctx, 10, "bootstrap-key", input)
	if err != nil {
		t.Fatalf("BootstrapProject: %v", err)
	}
	replay, err := service.BootstrapProject(ctx, 10, "bootstrap-key", input)
	if err != nil {
		t.Fatalf("BootstrapProject replay: %v", err)
	}
	if replay.Product.ID != first.Product.ID || replay.SKU.ID != first.SKU.ID || replay.Project.ID != first.Project.ID {
		t.Fatalf("replay changed result: first=%#v replay=%#v", first, replay)
	}
	if _, err := service.BootstrapProject(ctx, 10, "bootstrap-key", BootstrapProjectInput{Title: "另一个商品", Pipeline: "general"}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v, want ErrIdempotencyConflict", err)
	}
	other, err := service.BootstrapProject(ctx, 11, "bootstrap-key", input)
	if err != nil || other.Project.UserID != 11 || other.Project.ID == first.Project.ID {
		t.Fatalf("cross-user bootstrap = %#v, %v", other, err)
	}

	before := map[string]int64{}
	for name, model := range map[string]any{"products": &CommerceProduct{}, "skus": &CommerceSKU{}, "projects": &CommerceProject{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		before[name] = count
	}
	if _, err := service.BootstrapProject(ctx, 10, "bad-pipeline", BootstrapProjectInput{Title: "失败", Pipeline: "fashion"}); !errors.Is(err, ErrInvalidPipeline) {
		t.Fatalf("invalid pipeline error = %v", err)
	}
	for name, model := range map[string]any{"products": &CommerceProduct{}, "skus": &CommerceSKU{}, "projects": &CommerceProject{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != before[name] {
			t.Fatalf("%s count after rollback = %d, want %d (err=%v)", name, count, before[name], err)
		}
	}
}

func TestProductReportParserStrictEvidence(t *testing.T) {
	valid := `{"observed_facts":[{"field":"color","value":"白色","confidence":0.92,"source_asset_ids":[7]}],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":["容量需用户确认"],"suggested_sections":["hero","detail"]}`
	report, err := ParseProductReport(valid, []uint{7})
	if err != nil {
		t.Fatalf("ParseProductReport: %v", err)
	}
	if len(report.ObservedFacts) != 1 || len(report.MissingFields) != 5 {
		t.Fatalf("unexpected report: %#v", report)
	}

	withoutEfficacy := `{"observed_facts":[],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification"],"risk_notices":[],"suggested_sections":[]}`
	normalized, err := ParseProductReport(withoutEfficacy, nil)
	if err != nil {
		t.Fatalf("ParseProductReport normalizes required unknown fields: %v", err)
	}
	if got := strings.Join(normalized.MissingFields, ","); got != "price,capacity,material,certification,efficacy" {
		t.Fatalf("normalized missing fields = %q", got)
	}

	cases := []string{
		`{"observed_facts":[],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":[],"risk_notices":[],"suggested_sections":[],"extra":true}`,
		`{"observed_facts":[{"field":"color","value":"白色","confidence":0.9,"source_asset_ids":[99]}],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":["hero"]}`,
		`{"observed_facts":[{"field":"material","value":"纯银","confidence":0.9,"source_asset_ids":[7]}],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","certification","efficacy"],"risk_notices":[],"suggested_sections":["hero"]}`,
		`{"observed_facts":[],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":["unknown"]}`,
	}
	for _, raw := range cases {
		if _, err := ParseProductReport(raw, []uint{7}); err == nil {
			t.Fatalf("ParseProductReport(%s) unexpectedly succeeded", raw)
		}
	}
}

func TestProductReportParserRequiresStructuredCreativeFields(t *testing.T) {
	raw := `{"observed_facts":[],"selling_points":["轻便"],"forbidden_changes":["不得改变杯盖"],"brand_tone":{"description":"简洁克制"},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":["selling_points"]}`
	report, err := ParseProductReport(raw, nil)
	if err != nil {
		t.Fatalf("ParseProductReport: %v", err)
	}
	if strings.Join(report.SellingPoints, ",") != "轻便" || strings.Join(report.ForbiddenChanges, ",") != "不得改变杯盖" || report.BrandTone.Description != "简洁克制" {
		t.Fatalf("structured creative fields = %#v", report)
	}

	for _, invalid := range []string{
		`{"observed_facts":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":[]}`,
		`{"observed_facts":[],"selling_points":null,"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":[]}`,
		`{"observed_facts":[],"selling_points":[""],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":[]}`,
	} {
		if _, err := ParseProductReport(invalid, nil); err == nil {
			t.Fatalf("invalid structured report accepted: %s", invalid)
		}
	}
}

func TestProductAnalysisCreateReplayAndNoAnalyzerRollback(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 20, CreateProductInput{Name: "杯子"})
	project, _ := service.CreateProject(ctx, 20, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 20, ProjectID: project.ID, ReferenceAssetID: 200, Role: "product", Lifecycle: AssetLifecycleProject}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.AnalyzeProduct(ctx, 20, project.ID, "analysis-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}}); !errors.Is(err, ErrVisionNotConfigured) {
		t.Fatalf("without analyzer error = %v", err)
	}
	var specs, jobs int64
	db.Model(&CommerceCreativeSpec{}).Count(&specs)
	db.Model(&CommerceJob{}).Count(&jobs)
	if specs != 0 || jobs != 0 {
		t.Fatalf("unconfigured analyzer left rows: specs=%d jobs=%d", specs, jobs)
	}

	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	first, err := service.AnalyzeProduct(ctx, 20, project.ID, "analysis-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}, UserRequirements: "简洁"})
	if err != nil {
		t.Fatalf("AnalyzeProduct: %v", err)
	}
	if first.CreativeSpec.Status != "analyzing" || first.Job.Kind != CommerceJobKindProductAnalysis || first.Job.BatchID != nil {
		t.Fatalf("unexpected analysis creation: %#v", first)
	}
	replay, err := service.AnalyzeProduct(ctx, 20, project.ID, "analysis-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}, UserRequirements: "简洁"})
	if err != nil || replay.CreativeSpec.ID != first.CreativeSpec.ID || replay.Job.ID != first.Job.ID {
		t.Fatalf("analysis replay = %#v, %v", replay, err)
	}
	if _, err := service.AnalyzeProduct(ctx, 20, project.ID, "analysis-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}, UserRequirements: "冲突"}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("analysis conflict = %v", err)
	}

	otherProduct, _ := service.CreateProduct(ctx, 20, CreateProductInput{Name: "另一个"})
	otherProject, _ := service.CreateProject(ctx, 20, CreateProjectInput{ProductID: otherProduct.ID, Pipeline: "general"})
	if _, err := service.AnalyzeProduct(ctx, 20, otherProject.ID, "cross-project", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}}); !errors.Is(err, ErrOwnershipMismatch) {
		t.Fatalf("cross-project asset error = %v", err)
	}
}

func TestProductAnalysisAcceptsCreatorProductRoles(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 21, CreateProductInput{Name: "商品角色"})
	project, _ := service.CreateProject(ctx, 21, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	front := CommerceAsset{UserID: 21, ProjectID: project.ID, ReferenceAssetID: 211, Role: "product_front", Lifecycle: AssetLifecycleProject}
	detail := CommerceAsset{UserID: 21, ProjectID: project.ID, ReferenceAssetID: 212, Role: "product_detail", Lifecycle: AssetLifecycleProject}
	back := CommerceAsset{UserID: 21, ProjectID: project.ID, ReferenceAssetID: 213, Role: "product_back", Lifecycle: AssetLifecycleProject}
	logo := CommerceAsset{UserID: 21, ProjectID: project.ID, ReferenceAssetID: 214, Role: "logo", Lifecycle: AssetLifecycleProject}
	pattern := CommerceAsset{UserID: 21, ProjectID: project.ID, ReferenceAssetID: 215, Role: "pattern", Lifecycle: AssetLifecycleProject}
	if err := db.Create(&[]CommerceAsset{front, detail, back, logo, pattern}).Error; err != nil {
		t.Fatal(err)
	}
	var assets []CommerceAsset
	if err := db.Where("project_id = ?", project.ID).Order("id asc").Find(&assets).Error; err != nil {
		t.Fatal(err)
	}
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	if _, err := service.AnalyzeProduct(ctx, 21, project.ID, "creator-product-roles", AnalyzeProductInput{SourceAssetIDs: []uint{assets[0].ID, assets[1].ID, assets[2].ID}}); err != nil {
		t.Fatalf("creator product roles rejected: %v", err)
	}
	for index, role := range []string{"logo", "pattern"} {
		if _, err := service.AnalyzeProduct(ctx, 21, project.ID, role+"-is-not-product", AnalyzeProductInput{SourceAssetIDs: []uint{assets[index+3].ID}}); !errors.Is(err, ErrOwnershipMismatch) {
			t.Fatalf("%s analysis error=%v, want ownership mismatch", role, err)
		}
	}
}

func TestProductAnalysisWorkerCASProtectsEditedVersion(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 30, CreateProductInput{Name: "灯"})
	project, _ := service.CreateProject(ctx, 30, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 30, ProjectID: project.ID, ReferenceAssetID: 300, Role: "product", Lifecycle: AssetLifecycleProject}
	db.Create(&asset)
	raw := `{"observed_facts":[{"field":"color","value":"黑色","confidence":0.91,"source_asset_ids":[` + itoaForReport(asset.ID) + `]}],"selling_points":["轻便"],"forbidden_changes":["不得改变轮廓"],"brand_tone":{"description":"克制"},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":["hero"]}`
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{raw: raw})
	created, err := service.AnalyzeProduct(ctx, 30, project.ID, "worker-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewProductAnalysisJobHandler(service, fakeCommerceVisionAnalyzer{raw: raw})
	if _, err := handler.Handle(ctx, JobSnapshot{Job: created.Job}); err != nil {
		t.Fatalf("worker handle: %v", err)
	}
	loaded, _ := service.GetCreativeSpec(ctx, 30, created.CreativeSpec.ID)
	if loaded.Status != "draft" || loaded.ObservedFactsJSON == "[]" || loaded.SellingPointsJSON != `["轻便"]` || loaded.ForbiddenChangesJSON != `["不得改变轮廓"]` || loaded.BrandToneJSON != `{"description":"克制"}` {
		t.Fatalf("worker result not persisted: %#v", loaded)
	}

	second, _ := service.AnalyzeProduct(ctx, 30, project.ID, "cas-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	if err := db.Model(&CommerceCreativeSpec{}).Where("id = ?", second.CreativeSpec.ID).Updates(map[string]any{"version": 2, "status": "draft", "user_overrides_json": `{"name":"用户编辑"}`, "selling_points_json": `["用户卖点"]`, "forbidden_changes_json": `["用户禁改"]`, "brand_tone_json": `{"description":"用户调性"}`}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Handle(ctx, JobSnapshot{Job: second.Job}); JobErrorCode(err) != "analysis_superseded" {
		t.Fatalf("stale worker error = %v", err)
	}
	var stale CommerceCreativeSpec
	db.First(&stale, second.CreativeSpec.ID)
	if stale.Version != 2 || stale.UserOverridesJSON != `{"name":"用户编辑"}` || stale.SellingPointsJSON != `["用户卖点"]` || stale.ForbiddenChangesJSON != `["用户禁改"]` || stale.BrandToneJSON != `{"description":"用户调性"}` {
		t.Fatalf("stale analysis overwrote edit: %#v", stale)
	}
}

func TestProductReportPatchAndConfirmMergeSnapshot(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 40, CreateProductInput{Name: "水杯"})
	project, _ := service.CreateProject(ctx, 40, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	spec := CommerceCreativeSpec{
		UserID: 40, ProjectID: project.ID, Version: 1, Source: "vision", Status: "draft",
		ProductFactsJSON: "{}", SellingPointsJSON: "[]", ForbiddenChangesJSON: "[]", BrandToneJSON: "{}", ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: "[1]",
		ObservedFactsJSON: `[{"field":"color","value":"白色","confidence":0.9,"source_asset_ids":[1]}]`, UserOverridesJSON: "{}", MissingFieldsJSON: `["price","capacity","material","certification","efficacy"]`, SuggestedSectionsJSON: `["hero"]`,
	}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	overrides := []byte(`{"color":"米白色","name":"便携水杯","price":"用户未提供","capacity":"用户未提供","material":"用户未提供","certification":"用户未提供","efficacy":"用户未提供"}`)
	patched, err := service.PatchCreativeSpec(ctx, 40, spec.ID, PatchCreativeSpecInput{ExpectedVersion: 1, UserOverrides: &overrides})
	if err != nil {
		t.Fatalf("PatchCreativeSpec: %v", err)
	}
	if patched.ObservedFactsJSON != spec.ObservedFactsJSON || !strings.Contains(patched.UserOverridesJSON, `"name":"便携水杯"`) || patched.Version != 2 {
		t.Fatalf("patch overwrote analyzer evidence: %#v", patched)
	}
	confirmed, err := service.ConfirmCreativeSpec(ctx, 40, spec.ID)
	if err != nil {
		t.Fatalf("ConfirmCreativeSpec: %v", err)
	}
	if confirmed.Status != "confirmed" || !strings.Contains(confirmed.ProductFactsJSON, `"name":"便携水杯"`) {
		t.Fatalf("confirmed snapshot = %#v", confirmed)
	}
	loadedProject, _ := service.GetProject(ctx, 40, project.ID)
	if loadedProject.ActiveCreativeSpecID == nil || *loadedProject.ActiveCreativeSpecID != spec.ID {
		t.Fatalf("active spec = %v", loadedProject.ActiveCreativeSpecID)
	}
	directFacts := []byte(`{"color":"伪造覆盖"}`)
	if _, err := service.PatchCreativeSpec(ctx, 40, spec.ID, PatchCreativeSpecInput{ExpectedVersion: confirmed.Version, ProductFacts: &directFacts}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("vision product facts overwrite error = %v", err)
	}
}

func TestProductAnalysisRetryableFailureKeepsSpecAnalyzing(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 55, CreateProductInput{Name: "重试商品"})
	project, _ := service.CreateProject(ctx, 55, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 55, ProjectID: project.ID, ReferenceAssetID: 550, Role: "product", Lifecycle: AssetLifecycleProject}
	db.Create(&asset)
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	created, _ := service.AnalyzeProduct(ctx, 55, project.ID, "retryable-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	handler := NewProductAnalysisJobHandler(service, fakeCommerceVisionAnalyzer{err: retryableVisionError{}})
	_, err := handler.Handle(ctx, JobSnapshot{Job: created.Job})
	if !IsRetryableJobError(err) {
		t.Fatalf("worker error should be retryable: %v", err)
	}
	loaded, _ := service.GetCreativeSpec(ctx, 55, created.CreativeSpec.ID)
	if loaded.Status != "analyzing" || loaded.AnalysisError != "analysis temporarily failed" {
		t.Fatalf("retryable failure made spec terminal: %#v", loaded)
	}
}

func TestProductAnalysisWorkerFailureIsSafe(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 50, CreateProductInput{Name: "失败商品"})
	project, _ := service.CreateProject(ctx, 50, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 50, ProjectID: project.ID, ReferenceAssetID: 500, Role: "product", Lifecycle: AssetLifecycleProject}
	db.Create(&asset)
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	created, _ := service.AnalyzeProduct(ctx, 50, project.ID, "failure-key", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	handler := NewProductAnalysisJobHandler(service, fakeCommerceVisionAnalyzer{err: errors.New("provider secret detail")})
	if _, err := handler.Handle(ctx, JobSnapshot{Job: created.Job}); JobErrorCode(err) != "analysis_failed" {
		t.Fatalf("worker error = %v", err)
	}
	loaded, _ := service.GetCreativeSpec(ctx, 50, created.CreativeSpec.ID)
	if loaded.Status != "analysis_failed" || loaded.AnalysisError != "analysis failed" || strings.Contains(loaded.AnalysisError, "secret") {
		t.Fatalf("unsafe analysis failure: %#v", loaded)
	}
}

func TestProductAnalysisLeaseReclaim(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	job := CommerceJob{UserID: 60, ProjectID: 600, Kind: CommerceJobKindProductAnalysis, SubjectType: CommerceSubjectCreativeSpec, Status: CommerceJobRunning, IdempotencyKey: "lease-product-analysis", MaxAttempts: 3, AttemptCount: 1, LeaseOwner: "worker-a", LeaseToken: "expired-token", LeaseExpiresAt: &expired}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	secondQueue := NewQueue(db, service, "worker-b")
	secondQueue.Now = func() time.Time { return now }
	secondQueue.Jitter = func() float64 { return 0 }
	if err := secondQueue.RecoverExpired(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Update("next_attempt_at", expired).Error; err != nil {
		t.Fatal(err)
	}
	var recovered CommerceJob
	if err := db.First(&recovered, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.Status != CommerceJobRetrying {
		t.Fatalf("recovered status = %s", recovered.Status)
	}
	reclaimed, err := secondQueue.Claim(ctx, 1, time.Second)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].Job.ID != job.ID || reclaimed[0].Job.AttemptCount != 2 {
		t.Fatalf("reclaim = %#v, %v", reclaimed, err)
	}
}

func TestProductAnalysisJobPayloadIsStrictAndBoundToSpec(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload func(AnalyzeProductResult) string
	}{
		{name: "empty_object", payload: func(AnalyzeProductResult) string { return `{}` }},
		{name: "unknown_field", payload: func(created AnalyzeProductResult) string {
			return strings.TrimSuffix(created.Job.PayloadJSON, "}") + `,"unknown":true}`
		}},
		{name: "trailing_json", payload: func(created AnalyzeProductResult) string { return created.Job.PayloadJSON + `{}` }},
		{name: "empty_assets", payload: func(created AnalyzeProductResult) string {
			var payload map[string]any
			_ = json.Unmarshal([]byte(created.Job.PayloadJSON), &payload)
			payload["source_asset_ids"] = []uint{}
			encoded, _ := json.Marshal(payload)
			return string(encoded)
		}},
		{name: "wrong_spec_hash", payload: func(created AnalyzeProductResult) string {
			var payload map[string]any
			_ = json.Unmarshal([]byte(created.Job.PayloadJSON), &payload)
			payload["analysis_request_hash"] = "not-the-spec-hash"
			encoded, _ := json.Marshal(payload)
			return string(encoded)
		}},
		{name: "wrong_project", payload: func(created AnalyzeProductResult) string {
			var payload map[string]any
			_ = json.Unmarshal([]byte(created.Job.PayloadJSON), &payload)
			payload["project_id"] = created.Job.ProjectID + 1
			encoded, _ := json.Marshal(payload)
			return string(encoded)
		}},
		{name: "tampered_user_requirements", payload: func(created AnalyzeProductResult) string {
			var payload map[string]any
			_ = json.Unmarshal([]byte(created.Job.PayloadJSON), &payload)
			payload["user_requirements"] = "tampered requirement"
			encoded, _ := json.Marshal(payload)
			return string(encoded)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, db := newCommerceServiceTest(t)
			product, _ := service.CreateProduct(ctx, 61, CreateProductInput{Name: "payload product"})
			project, _ := service.CreateProject(ctx, 61, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
			asset := CommerceAsset{UserID: 61, ProjectID: project.ID, ReferenceAssetID: 611, Role: "product", Lifecycle: AssetLifecycleProject}
			if err := db.Create(&asset).Error; err != nil {
				t.Fatal(err)
			}
			service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
			created, err := service.AnalyzeProduct(ctx, 61, project.ID, "payload-"+tc.name, AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
			if err != nil {
				t.Fatal(err)
			}
			created.Job.PayloadJSON = tc.payload(created)
			analyzer := &countingCommerceVisionAnalyzer{}
			_, err = NewProductAnalysisJobHandler(service, analyzer).Handle(ctx, JobSnapshot{Job: created.Job})
			if JobErrorCode(err) != "invalid_analysis_payload" {
				t.Fatalf("Handle error = %v", err)
			}
			if analyzer.calls != 0 {
				t.Fatalf("analyzer calls = %d, want 0", analyzer.calls)
			}
			loaded, loadErr := service.GetCreativeSpec(ctx, 61, created.CreativeSpec.ID)
			if loadErr != nil || loaded.Status != "analysis_failed" {
				t.Fatalf("spec = %#v, err=%v", loaded, loadErr)
			}
		})
	}
}

func TestProductAnalysisExpiredFinalLeaseMarksSpecFailed(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 62, CreateProductInput{Name: "lease terminal product"})
	project, _ := service.CreateProject(ctx, 62, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 62, ProjectID: project.ID, ReferenceAssetID: 621, Role: "product", Lifecycle: AssetLifecycleProject}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	created, err := service.AnalyzeProduct(ctx, 62, project.ID, "lease-terminal", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)
	if err := db.Model(&CommerceJob{}).Where("id = ?", created.Job.ID).Updates(map[string]any{
		"status": CommerceJobRunning, "attempt_count": created.Job.MaxAttempts,
		"lease_owner": "expired-worker", "lease_token": "expired-token", "lease_expires_at": expired,
	}).Error; err != nil {
		t.Fatal(err)
	}
	queue := NewQueue(db, service, "recoverer")
	queue.Now = func() time.Time { return now }
	queue.TerminalHooks[CommerceJobKindProductAnalysis] = NewProductAnalysisTerminalHook()
	if err := queue.RecoverExpired(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var job CommerceJob
	if err := db.First(&job, created.Job.ID).Error; err != nil {
		t.Fatal(err)
	}
	loaded, err := service.GetCreativeSpec(ctx, 62, created.CreativeSpec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != CommerceJobFailed || job.ErrorCode != "max_attempts_exceeded" || job.DeadLetteredAt == nil {
		t.Fatalf("job = %#v", job)
	}
	if loaded.Status != "analysis_failed" || loaded.AnalysisError != "analysis retries exhausted" {
		t.Fatalf("spec = %#v", loaded)
	}
}

func TestBootstrapConcurrentIdempotency(t *testing.T) {
	service, db := newCommerceServiceTest(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(8)
	const workers = 8
	results := make(chan BootstrapProjectResult, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.BootstrapProject(context.Background(), 70, "concurrent-bootstrap", BootstrapProjectInput{Title: "并发商品", Pipeline: "general"})
			results <- result
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var productID, skuID, projectID uint
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent bootstrap: %v", err)
		}
	}
	for result := range results {
		if productID == 0 {
			productID, skuID, projectID = result.Product.ID, result.SKU.ID, result.Project.ID
		}
		if result.Product.ID != productID || result.SKU.ID != skuID || result.Project.ID != projectID {
			t.Fatalf("concurrent result diverged: %#v", result)
		}
	}
	for model, want := range map[any]int64{&CommerceProduct{}: 1, &CommerceSKU{}: 1, &CommerceProject{}: 1} {
		var count int64
		if err := db.Model(model).Where("user_id = ?", 70).Count(&count).Error; err != nil || count != want {
			t.Fatalf("count %T=%d want %d err=%v", model, count, want, err)
		}
	}
}

func TestProductAnalysisConcurrentIdempotency(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(8)
	product, _ := service.CreateProduct(ctx, 71, CreateProductInput{Name: "并发分析"})
	project, _ := service.CreateProject(ctx, 71, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 71, ProjectID: project.ID, ReferenceAssetID: 710, Role: "product", Lifecycle: AssetLifecycleProject}
	db.Create(&asset)
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	const workers = 8
	results := make(chan AnalyzeProductResult, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.AnalyzeProduct(context.Background(), 71, project.ID, "concurrent-analysis", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
			results <- result
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var specID, jobID uint
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent analyze: %v", err)
		}
	}
	for result := range results {
		if specID == 0 {
			specID, jobID = result.CreativeSpec.ID, result.Job.ID
		}
		if result.CreativeSpec.ID != specID || result.Job.ID != jobID {
			t.Fatalf("concurrent analysis diverged: %#v", result)
		}
	}
}

func TestProductReportParserRejectsAnalyzerOverrides(t *testing.T) {
	raw := `{"observed_facts":[],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"user_overrides":{"price":"9.9"},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":["hero"]}`
	if _, err := ParseProductReport(raw, []uint{1}); err == nil {
		t.Fatal("analyzer user_overrides must be rejected")
	}
}

func TestCreativeSpecConfirmRequiresMissingFactsResolved(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 72, CreateProductInput{Name: "待补商品"})
	project, _ := service.CreateProject(ctx, 72, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	spec := CommerceCreativeSpec{UserID: 72, ProjectID: project.ID, Version: 1, Source: "vision", Status: "draft", ProductFactsJSON: "{}", ObservedFactsJSON: `[{"field":"name","value":"待补商品","confidence":0.9,"source_asset_ids":[1]}]`, UserOverridesJSON: "{}", MissingFieldsJSON: `["material"]`, SellingPointsJSON: "[]", ForbiddenChangesJSON: "[]", BrandToneJSON: "{}", ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: "[1]", SuggestedSectionsJSON: "[]"}
	db.Create(&spec)
	if _, err := service.ConfirmCreativeSpec(ctx, 72, spec.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unresolved missing field confirm error=%v", err)
	}
	overrides := []byte(`{"material":"用户确认的不锈钢"}`)
	if _, err := service.PatchCreativeSpec(ctx, 72, spec.ID, PatchCreativeSpecInput{ExpectedVersion: 1, UserOverrides: &overrides}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmCreativeSpec(ctx, 72, spec.ID); err != nil {
		t.Fatalf("resolved confirm: %v", err)
	}
}

func TestProductAnalysisTerminalAndSupersededSemantics(t *testing.T) {
	ctx := context.Background()
	service, db := newCommerceServiceTest(t)
	product, _ := service.CreateProduct(ctx, 73, CreateProductInput{Name: "终态"})
	project, _ := service.CreateProject(ctx, 73, CreateProjectInput{ProductID: product.ID, Pipeline: "general"})
	asset := CommerceAsset{UserID: 73, ProjectID: project.ID, ReferenceAssetID: 730, Role: "product", Lifecycle: AssetLifecycleProject}
	db.Create(&asset)
	service.ConfigureVisionAnalyzer(fakeCommerceVisionAnalyzer{})
	created, _ := service.AnalyzeProduct(ctx, 73, project.ID, "invalid-payload", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	created.Job.PayloadJSON = "{"
	created.Job.AttemptCount = created.Job.MaxAttempts
	handler := NewProductAnalysisJobHandler(service, fakeCommerceVisionAnalyzer{err: retryableVisionError{}})
	if _, err := handler.Handle(ctx, JobSnapshot{Job: created.Job}); JobErrorCode(err) != "invalid_analysis_payload" {
		t.Fatalf("invalid payload error=%v", err)
	}
	loaded, _ := service.GetCreativeSpec(ctx, 73, created.CreativeSpec.ID)
	if loaded.Status != "analysis_failed" {
		t.Fatalf("invalid payload spec status=%s", loaded.Status)
	}

	created2, _ := service.AnalyzeProduct(ctx, 73, project.ID, "superseded", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	db.Model(&CommerceCreativeSpec{}).Where("id = ?", created2.CreativeSpec.ID).Updates(map[string]any{"version": 2, "status": "draft"})
	valid := `{"observed_facts":[{"field":"name","value":"终态","confidence":0.9,"source_asset_ids":[` + itoaForReport(asset.ID) + `]}],"selling_points":[],"forbidden_changes":[],"brand_tone":{"description":""},"missing_fields":["price","capacity","material","certification","efficacy"],"risk_notices":[],"suggested_sections":["hero"]}`
	if _, err := NewProductAnalysisJobHandler(service, fakeCommerceVisionAnalyzer{raw: valid}).Handle(ctx, JobSnapshot{Job: created2.Job}); JobErrorCode(err) != "analysis_superseded" {
		t.Fatalf("CAS miss error=%v", err)
	}
	created3, _ := service.AnalyzeProduct(ctx, 73, project.ID, "retry-exhausted", AnalyzeProductInput{SourceAssetIDs: []uint{asset.ID}})
	created3.Job.AttemptCount = created3.Job.MaxAttempts
	if _, err := NewProductAnalysisJobHandler(service, fakeCommerceVisionAnalyzer{err: retryableVisionError{}}).Handle(ctx, JobSnapshot{Job: created3.Job}); !IsRetryableJobError(err) {
		t.Fatalf("exhausted error=%v", err)
	}
	loaded3, _ := service.GetCreativeSpec(ctx, 73, created3.CreativeSpec.ID)
	if loaded3.Status != "analysis_failed" {
		t.Fatalf("exhausted retry status=%s", loaded3.Status)
	}
}

func TestConcurrentIdempotencyDifferentDigestConflicts(t *testing.T) {
	service, db := newCommerceServiceTest(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(4)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, title := range []string{"并发甲", "并发乙"} {
		title := title
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.BootstrapProject(context.Background(), 74, "concurrent-conflict", BootstrapProjectInput{Title: title, Pipeline: "general"})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var success, conflict int
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrIdempotencyConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected concurrent error=%v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func itoaForReport(value uint) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
