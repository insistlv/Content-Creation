package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestProductDetailSetDefinitionDeclaresSectionScopes(t *testing.T) {
	definition := ProductDetailSetDefinition()
	want := map[string]string{
		"hero": "sku", "selling_points": "shared", "material": "shared",
		"detail": "sku", "usage": "shared", "specification": "sku", "closing": "shared",
	}
	if len(definition.SectionScopes) != len(want) {
		t.Fatalf("section scopes = %#v", definition.SectionScopes)
	}
	for section, scope := range want {
		metadata, ok := definition.SectionScopes[section]
		if !ok || metadata.Scope != scope {
			t.Fatalf("scope[%s] = %#v", section, metadata)
		}
	}
}

func TestProductDetailSetCompilesSharedOnceAndPerSKU(t *testing.T) {
	compiler := NewProductDetailSetCompiler(NewSnapshotCostResolver())
	input := CompileInput{
		PrimarySKUID: 11, SelectedSKUIDs: []uint{11, 12}, OutputCount: 10,
		QualityTier: "standard", AspectRatio: "1:1",
		Parameters:      map[string]any{"detail_sections": []string{"hero", "selling_points", "material", "detail", "usage", "specification", "closing"}},
		CreativeSpec:    CreativeSpecSnapshot{ID: 3, Version: 1, ProductFactsJSON: `{"name":"杯子"}`, SellingPointsJSON: `[]`, ForbiddenChangesJSON: `[]`, BrandToneJSON: `{}`, ShotPlanJSON: `[]`, CopyBlocksJSON: `[]`, RiskNoticesJSON: `[]`, SourceAssetIDsJSON: `[]`},
		PricingSnapshot: PricingSnapshot{ID: "p", Version: "v1", Entries: []PricingSnapshotEntry{{Pipeline: "general", RecipeKey: ProductDetailSetRecipeKey, QualityTier: "standard", Credits: 2, RequiredCapabilities: []string{"image"}, ModelID: 1}}},
	}
	items, err := compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(items) != 10 {
		t.Fatalf("items = %d, want 10", len(items))
	}
	shared, perSKU := 0, map[uint]int{}
	for _, item := range items {
		switch item.Scope {
		case "shared":
			shared++
			if item.SKUID != 0 || !strings.Contains(item.SlotKey, ":shared:") {
				t.Fatalf("shared item = %#v", item)
			}
		case "sku":
			perSKU[item.SKUID]++
			if item.SKUID == 0 || !strings.Contains(item.SlotKey, ":sku-") {
				t.Fatalf("sku item = %#v", item)
			}
		default:
			t.Fatalf("scope = %q", item.Scope)
		}
	}
	if shared != 4 || perSKU[11] != 3 || perSKU[12] != 3 {
		t.Fatalf("shared=%d perSKU=%v", shared, perSKU)
	}
}

func TestProductDetailSetAllowsConfigurableScopeOverride(t *testing.T) {
	definition := ProductDetailSetDefinition()
	definition.SectionScopes["detail"] = RecipeSectionScope{Scope: "sku", Configurable: true}
	sections, scopes, _, err := productDetailParameters(map[string]any{
		"detail_sections": []string{"detail"},
		"section_scopes":  map[string]any{"detail": "shared"},
	}, definition)
	if err != nil {
		t.Fatalf("parameters: %v", err)
	}
	if len(sections) != 1 || scopes["detail"] != "shared" {
		t.Fatalf("sections=%v scopes=%v", sections, scopes)
	}
}

func TestProductDetailSetKeepsSKUAssetsIsolatedAndRecordsSharedFallback(t *testing.T) {
	compiler := NewProductDetailSetCompiler(NewSnapshotCostResolver())
	input := CompileInput{PrimarySKUID: 11, SelectedSKUIDs: []uint{11, 12}, OutputCount: 2, QualityTier: "standard", AspectRatio: "1:1",
		Parameters:      map[string]any{"detail_sections": []string{"detail"}},
		AssetBindings:   map[string][]uint{"product_front": {100, 111, 122}},
		Assets:          []AssetBindingSnapshot{{CommerceAssetID: 100, ReferenceAssetID: 1000, Role: "product_front"}, {CommerceAssetID: 111, ReferenceAssetID: 1110, Role: "product_front", SKUID: uintPtr(11)}, {CommerceAssetID: 122, ReferenceAssetID: 1220, Role: "product_front", SKUID: uintPtr(12)}},
		CreativeSpec:    CreativeSpecSnapshot{ID: 3, Status: "confirmed", ContentSHA256: "x", ProductFactsJSON: `{}`, CommonFactsJSON: `{}`, SKUOverridesJSON: `{}`, SellingPointsJSON: `[]`, ForbiddenChangesJSON: `[]`, BrandToneJSON: `{}`, ShotPlanJSON: `[]`, CopyBlocksJSON: `[]`, RiskNoticesJSON: `[]`, SourceAssetIDsJSON: `[]`},
		PricingSnapshot: PricingSnapshot{Version: "v1", Entries: []PricingSnapshotEntry{{Pipeline: "general", RecipeKey: ProductDetailSetRecipeKey, QualityTier: "standard", Credits: 1, RequiredCapabilities: []string{"image"}, ModelID: 1}}},
	}
	items, err := compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%d", len(items))
	}
	if got := items[0].AssetIDs; len(got) != 2 || got[0] != 100 || got[1] != 111 {
		t.Fatalf("sku11 assets=%v", got)
	}
	if got := items[1].AssetIDs; len(got) != 2 || got[0] != 100 || got[1] != 122 {
		t.Fatalf("sku12 assets=%v", got)
	}
	input.Assets = input.Assets[:2]
	input.AssetBindings["product_front"] = []uint{100, 111}
	items, err = compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !items[1].InheritedSharedAssets || !strings.Contains(items[1].Prompt, "缺少 SKU 专属素材") {
		t.Fatalf("fallback snapshot=%#v", items[1])
	}
}

func uintPtr(value uint) *uint { return &value }

func TestValidateProjectSKUsRejectsInvalidSelection(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	var primary CommerceSKU
	if err := db.First(&primary, request.PrimarySKUID).Error; err != nil {
		t.Fatal(err)
	}
	second := CommerceSKU{UserID: project.UserID, ProductID: project.ProductID, Code: "SECOND", Status: "disabled", AttributesJSON: `{}`}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	request.SelectedSKUIDs = []uint{primary.ID, second.ID}
	if _, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request); !errors.Is(err, ErrSKUDisabled) {
		t.Fatalf("disabled SKU error = %v", err)
	}
	request.SelectedSKUIDs = []uint{second.ID}
	request.PrimarySKUID = primary.ID
	if _, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request); !errors.Is(err, ErrPrimarySKUNotSelected) {
		t.Fatalf("primary selection error = %v", err)
	}
}

func TestProductDetailSetCompilesUpToOneHundredSKUs(t *testing.T) {
	for _, count := range []int{33, 100} {
		t.Run(fmt.Sprintf("%d_skus", count), func(t *testing.T) {
			ids := make([]uint, count)
			snapshots := map[uint]SKUSnapshot{}
			for i := range ids {
				ids[i] = uint(i + 1)
				snapshots[ids[i]] = SKUSnapshot{ID: ids[i], Code: fmt.Sprintf("SKU-%03d", i+1)}
			}
			input := CompileInput{PrimarySKUID: 1, SelectedSKUIDs: ids, OutputCount: 4 + 3*count, QualityTier: "standard", AspectRatio: "1:1", SKUSnapshots: snapshots,
				Parameters:      map[string]any{"detail_sections": []string{"hero", "selling_points", "material", "detail", "usage", "specification", "closing"}},
				CreativeSpec:    CreativeSpecSnapshot{ID: 1, Status: "confirmed", ContentSHA256: "x", ProductFactsJSON: `{}`, CommonFactsJSON: `{}`, SKUOverridesJSON: `{}`, SellingPointsJSON: `[]`, ForbiddenChangesJSON: `[]`, BrandToneJSON: `{}`, ShotPlanJSON: `[]`, CopyBlocksJSON: `[]`, RiskNoticesJSON: `[]`, SourceAssetIDsJSON: `[]`},
				PricingSnapshot: PricingSnapshot{Version: "v1", Entries: []PricingSnapshotEntry{{Pipeline: "general", RecipeKey: ProductDetailSetRecipeKey, QualityTier: "standard", Credits: 1, RequiredCapabilities: []string{"image"}, ModelID: 1}}},
			}
			items, err := NewProductDetailSetCompiler(NewSnapshotCostResolver()).Compile(context.Background(), input)
			if err != nil || len(items) != 4+3*count {
				t.Fatalf("items=%d err=%v", len(items), err)
			}
		})
	}
}

func TestEstimateRejectsConfirmedSpecAfterSKUContextChanges(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	digest, err := creativeSpecSKUContextDigest(context.Background(), db, project)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceCreativeSpec{}).Where("id = ?", request.CreativeSpecID).Update("sku_context_sha256", digest).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request); err != nil {
		t.Fatalf("baseline estimate: %v", err)
	}
	if err := db.Model(&CommerceSKU{}).Where("id = ?", request.PrimarySKUID).Update("attributes_json", `{"容量":"500ml"}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request); !errors.Is(err, ErrCreativeSpecNotConfirmed) {
		t.Fatalf("stale spec error=%v", err)
	}
}

func TestEstimateRejectsConfirmedSpecAfterAssetOwnershipChanges(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	asset := CommerceAsset{UserID: project.UserID, ProjectID: project.ID, ReferenceAssetID: 9001, Role: "product_front", Lifecycle: AssetLifecycleProject}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	digest, err := creativeSpecSKUContextDigest(context.Background(), db, project)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceCreativeSpec{}).Where("id = ?", request.CreativeSpecID).Update("sku_context_sha256", digest).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceAsset{}).Where("id = ?", asset.ID).Update("sku_id", request.PrimarySKUID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request); !errors.Is(err, ErrCreativeSpecNotConfirmed) {
		t.Fatalf("stale asset spec error=%v", err)
	}
}

func TestProductDetailPromptUsesOnlyTargetScopeFacts(t *testing.T) {
	spec := CreativeSpecSnapshot{ProductFactsJSON: `{"legacy":"旧"}`, CommonFactsJSON: `[{"field":"name","value":"公共杯子","confidence":1,"source_asset_ids":[1]}]`, SKUOverridesJSON: `{"11":[{"field":"color","value":"红色A","confidence":1,"source_asset_ids":[11]}],"12":[{"field":"color","value":"蓝色B","confidence":1,"source_asset_ids":[12]}]}`, SellingPointsJSON: `[]`, ForbiddenChangesJSON: `[]`, BrandToneJSON: `{}`}
	shared := confirmedProductDetailPrompt("selling_points", spec, 0, "shared")
	if !strings.Contains(shared, "公共杯子") || strings.Contains(shared, "红色A") || strings.Contains(shared, "蓝色B") {
		t.Fatalf("shared prompt=%s", shared)
	}
	skuA := confirmedProductDetailPrompt("hero", spec, 11, "sku")
	if !strings.Contains(skuA, "公共杯子") || !strings.Contains(skuA, "红色A") || strings.Contains(skuA, "蓝色B") {
		t.Fatalf("sku A prompt=%s", skuA)
	}
}

type skuTestPricingProvider struct{}

func (skuTestPricingProvider) SnapshotForEstimate(_ context.Context, _ *gorm.DB, userID, projectID uint, _ RecipeDefinition, _ EstimateBatchRequest) (PricingSnapshot, error) {
	return PricingSnapshot{UserID: userID, ProjectID: projectID, Version: "sku-test-v1", Entries: []PricingSnapshotEntry{{Pipeline: "general", RecipeKey: ProductDetailSetRecipeKey, QualityTier: "standard", Credits: 1, RequiredCapabilities: []string{"image"}, ModelID: 1}}}, nil
}

func TestServiceEstimateCompilesPublicAndTargetSKUAssetsWithoutLeakage(t *testing.T) {
	service, db, _, ledger, project, base := newBatchTestService(t)
	second := CommerceSKU{UserID: project.UserID, ProductID: project.ProductID, Code: "SKU-B", Status: "active", AttributesJSON: `{}`}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	assets := []CommerceAsset{{UserID: project.UserID, ProjectID: project.ID, ReferenceAssetID: 1001, Role: "product_front", Lifecycle: AssetLifecycleProject}, {UserID: project.UserID, ProjectID: project.ID, ReferenceAssetID: 1002, Role: "product_front", Lifecycle: AssetLifecycleProject, SKUID: uintPtr(base.PrimarySKUID)}, {UserID: project.UserID, ProjectID: project.ID, ReferenceAssetID: 1003, Role: "product_front", Lifecycle: AssetLifecycleProject, SKUID: uintPtr(second.ID)}}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(NewProductDetailSetCompiler(NewSnapshotCostResolver())); err != nil {
		t.Fatal(err)
	}
	service.ConfigureBatchInfrastructure(registry, ledger, NewGormPricingSnapshotStore(), skuTestPricingProvider{})
	req := EstimateBatchRequest{RecipeKey: ProductDetailSetRecipeKey, RecipeVersion: 1, OutputCount: 1, CreativeSpecID: base.CreativeSpecID, PrimarySKUID: base.PrimarySKUID, SelectedSKUIDs: []uint{base.PrimarySKUID, second.ID}, QualityTier: "standard", AspectRatio: "1:1", AssetBindings: map[string][]uint{"product_front": {assets[0].ID, assets[1].ID, assets[2].ID}}, Parameters: map[string]any{"detail_sections": []string{"hero"}}}
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(estimate.Items) != 2 {
		t.Fatalf("items=%d", len(estimate.Items))
	}
	if got := estimate.Items[0].AssetIDs; len(got) != 2 || got[0] != assets[0].ID || got[1] != assets[1].ID {
		t.Fatalf("A assets=%v", got)
	}
	if got := estimate.Items[1].AssetIDs; len(got) != 2 || got[0] != assets[0].ID || got[1] != assets[2].ID {
		t.Fatalf("B assets=%v", got)
	}
	created, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "service-sku-assets", SubmitBatchRequest{EstimateBatchRequest: req, PricingSnapshotID: estimate.PricingSnapshotID})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Items) != 2 {
		t.Fatalf("created items=%d", len(created.Items))
	}
	for index, item := range created.Items {
		compiled, err := DecodeGenerationItemSnapshot(item.OutputSpecJSON)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(compiled.AssetIDs, estimate.Items[index].AssetIDs) {
			t.Fatalf("created item %d assets=%v estimate=%v", index, compiled.AssetIDs, estimate.Items[index].AssetIDs)
		}
	}
}

func TestValidateReportAssetScopesRejectsInvalidSKUAndSharedOnlyOverride(t *testing.T) {
	shared, owned := uint(1), uint(2)
	contexts := []ProductAnalysisAssetContext{{AssetID: shared, Shared: true}, {AssetID: owned, SKUID: uintPtr(11)}}
	valid := map[uint]struct{}{11: {}}
	base := ProductReport{CommonFacts: []ObservedFact{}, SKUOverrides: map[string][]ObservedFact{"99": {{Field: "color", Value: "蓝", Confidence: 1, SourceAssetIDs: []uint{owned}}}}}
	if err := validateReportAssetScopes(base, contexts, valid); err == nil {
		t.Fatal("expected cross-product SKU rejection")
	}
	base.SKUOverrides = map[string][]ObservedFact{"11": {{Field: "color", Value: "红", Confidence: 1, SourceAssetIDs: []uint{shared}}}}
	if err := validateReportAssetScopes(base, contexts, valid); err == nil {
		t.Fatal("expected shared-only override rejection")
	}
	base.SKUOverrides = map[string][]ObservedFact{"11": {{Field: "color", Value: "红", Confidence: 1, SourceAssetIDs: []uint{shared, owned}}}}
	if err := validateReportAssetScopes(base, contexts, valid); err != nil {
		t.Fatalf("valid override: %v", err)
	}
}
