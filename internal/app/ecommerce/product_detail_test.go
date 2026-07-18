package ecommerce

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProductDetailCompilerSeparatesVisibleSellingPointsFromForbiddenConstraints(t *testing.T) {
	input := validProductDetailCompileInput()
	input.OutputCount = 1
	input.Parameters["detail_sections"] = []any{"selling_points"}
	input.CreativeSpec.CopyBlocksJSON = "[]"
	input.CreativeSpec.SellingPointsJSON = `["VISIBLE_SELLING_POINT"]`
	input.CreativeSpec.ForbiddenChangesJSON = `["PROMPT_ONLY_CONSTRAINT"]`
	items, err := NewProductDetailSetCompiler(NewSnapshotCostResolver()).Compile(context.Background(), input)
	if err != nil || len(items) != 1 {
		t.Fatalf("Compile = %#v, %v", items, err)
	}
	if !strings.Contains(items[0].Prompt, "VISIBLE_SELLING_POINT") || !strings.Contains(items[0].Prompt, "PROMPT_ONLY_CONSTRAINT") {
		t.Fatalf("prompt lost confirmed fields: %q", items[0].Prompt)
	}
	var document LayoutDocument
	if err := json.Unmarshal([]byte(items[0].LayoutDocumentJSON), &document); err != nil {
		t.Fatalf("layout document: %v", err)
	}
	visible := make([]string, 0, len(document.TextBlocks))
	for _, block := range document.TextBlocks {
		visible = append(visible, block.Text)
	}
	joined := strings.Join(visible, "\n")
	if !strings.Contains(joined, "VISIBLE_SELLING_POINT") {
		t.Fatalf("selling point missing from visible layout: %q", joined)
	}
	if strings.Contains(joined, "PROMPT_ONLY_CONSTRAINT") {
		t.Fatalf("forbidden constraint leaked into visible layout: %q", joined)
	}
}

func TestDetailSetDefinition(t *testing.T) {
	compiler := NewProductDetailSetCompiler(NewSnapshotCostResolver())
	definition := compiler.Definition()
	if definition.Key != "product_detail_set" || definition.Version != 1 || definition.Pipeline != "general" {
		t.Fatalf("identity = %s/%s@%d", definition.Pipeline, definition.Key, definition.Version)
	}
	if !reflect.DeepEqual(definition.AllowedOutputCounts, []int{1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("output counts = %#v", definition.AllowedOutputCounts)
	}
	if !reflect.DeepEqual(definition.AspectRatios, []string{"1:1", "3:4", "4:5", "9:16"}) {
		t.Fatalf("aspect ratios = %#v", definition.AspectRatios)
	}
	if !reflect.DeepEqual(definition.QualityTiers, []string{"standard", "high_fidelity"}) || definition.MaxAttempts < 1 || definition.MaxAttempts > 3 {
		t.Fatalf("quality/max attempts = %#v/%d", definition.QualityTiers, definition.MaxAttempts)
	}
	if !reflect.DeepEqual(definition.Capabilities, []string{"image"}) {
		t.Fatalf("capabilities = %#v", definition.Capabilities)
	}
	if !reflect.DeepEqual(definition.Sections, []string{"hero", "selling_points", "material", "detail", "usage", "specification", "closing"}) {
		t.Fatalf("sections = %#v", definition.Sections)
	}
	if !reflect.DeepEqual(definition.LayoutTemplates, []string{"clean", "dark_gradient", "brand_band"}) {
		t.Fatalf("layout templates = %#v", definition.LayoutTemplates)
	}
	wantRequired := []AssetRequirement{{Role: "product_front", MinCount: 1, MaxCount: 101, MediaKind: "image", Required: true}}
	if !reflect.DeepEqual(definition.RequiredAssets, wantRequired) {
		t.Fatalf("required assets = %#v", definition.RequiredAssets)
	}
}

func TestProductDetailDefinitionPublishesChineseDisplayOptions(t *testing.T) {
	definition := ProductDetailSetDefinition()
	if want := []RecipeDisplayOption{
		{Value: "hero", Label: "首屏主视觉"},
		{Value: "selling_points", Label: "核心卖点"},
		{Value: "material", Label: "材质工艺"},
		{Value: "detail", Label: "细节展示"},
		{Value: "usage", Label: "使用场景"},
		{Value: "specification", Label: "规格参数"},
		{Value: "closing", Label: "收尾转化"},
	}; !reflect.DeepEqual(definition.SectionOptions, want) {
		t.Fatalf("section options = %#v, want %#v", definition.SectionOptions, want)
	}
	if want := []RecipeDisplayOption{
		{Value: "standard", Label: "标准"},
		{Value: "high_fidelity", Label: "高清"},
	}; !reflect.DeepEqual(definition.QualityOptions, want) {
		t.Fatalf("quality options = %#v, want %#v", definition.QualityOptions, want)
	}
	if want := []RecipeDisplayOption{
		{Value: "clean", Label: "简洁留白"},
		{Value: "dark_gradient", Label: "深色渐变"},
		{Value: "brand_band", Label: "品牌色带"},
	}; !reflect.DeepEqual(definition.LayoutTemplateOptions, want) {
		t.Fatalf("layout template options = %#v, want %#v", definition.LayoutTemplateOptions, want)
	}
}

func TestRegistryValidatesAndClonesRecipeDisplayOptions(t *testing.T) {
	valid := ProductDetailSetDefinition()
	registry := NewRegistry()
	compiler := NewProductDetailSetCompiler(NewSnapshotCostResolver())
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("register valid definition: %v", err)
	}
	listed := registry.List("general")
	listed[0].SectionOptions[0].Label = "已修改"
	listed[0].QualityOptions[0].Label = "已修改"
	listed[0].LayoutTemplateOptions[0].Label = "已修改"
	if got := registry.List("general")[0].SectionOptions[0].Label; got != "首屏主视觉" {
		t.Fatalf("registered display options were not deeply cloned: %q", got)
	}
	if got := registry.List("general")[0].QualityOptions[0].Label; got != "标准" {
		t.Fatalf("registered quality options were not deeply cloned: %q", got)
	}
	if got := registry.List("general")[0].LayoutTemplateOptions[0].Label; got != "简洁留白" {
		t.Fatalf("registered layout template options were not deeply cloned: %q", got)
	}

	tests := []struct {
		name   string
		mutate func(*RecipeDefinition)
	}{
		{"length mismatch", func(definition *RecipeDefinition) { definition.SectionOptions = definition.SectionOptions[:1] }},
		{"order mismatch", func(definition *RecipeDefinition) {
			definition.SectionOptions[0], definition.SectionOptions[1] = definition.SectionOptions[1], definition.SectionOptions[0]
		}},
		{"value mismatch", func(definition *RecipeDefinition) { definition.QualityOptions[0].Value = "other" }},
		{"duplicate value", func(definition *RecipeDefinition) {
			definition.LayoutTemplateOptions[1].Value = definition.LayoutTemplateOptions[0].Value
		}},
		{"empty label", func(definition *RecipeDefinition) { definition.SectionOptions[0].Label = " " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := cloneRecipeDefinition(valid)
			tt.mutate(&definition)
			if err := NewRegistry().Register(recipeDefinitionCompiler{definition: definition}); !errors.Is(err, ErrRecipeDefinitionInvalid) {
				t.Fatalf("Register error = %v, want ErrRecipeDefinitionInvalid", err)
			}
		})
	}
}

type recipeDefinitionCompiler struct{ definition RecipeDefinition }

func (c recipeDefinitionCompiler) Definition() RecipeDefinition { return c.definition }
func (recipeDefinitionCompiler) Compile(context.Context, CompileInput) ([]CompiledGenerationItem, error) {
	return nil, nil
}

func TestProductDetailCompilerFreezesOrderedItemsAndReferences(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewProductDetailSetCompiler(NewSnapshotCostResolver())); err != nil {
		t.Fatal(err)
	}
	input := validProductDetailCompileInput()
	items, err := registry.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(items) != 2 || items[0].SlotKey != "product_detail_set:v1:sku-9:hero" || items[1].SlotKey != "product_detail_set:v1:sku-9:specification" {
		t.Fatalf("slots = %#v", []string{items[0].SlotKey, items[1].SlotKey})
	}
	if items[0].Section != "hero" || items[1].Section != "specification" {
		t.Fatalf("sections = %q, %q", items[0].Section, items[1].Section)
	}
	if !reflect.DeepEqual(items[0].AssetIDs, []uint{11, 31, 41}) || !reflect.DeepEqual(items[1].AssetIDs, []uint{11, 32, 41}) {
		t.Fatalf("references = %#v / %#v", items[0].AssetIDs, items[1].AssetIDs)
	}
	for _, item := range items {
		if item.CreativeSpecSnapshot.ID != input.CreativeSpec.ID || item.CreativeSpecSnapshot.Version != input.CreativeSpec.Version || item.CreativeSpecSnapshot.ContentSHA256 != input.CreativeSpec.ContentSHA256 {
			t.Fatalf("creative spec snapshot = %#v", item.CreativeSpecSnapshot)
		}
		if item.PricingVersion != "catalog-v1" || item.EstimatedCredits != 3 || !strings.Contains(item.ModelSnapshotJSON, `"model_id":77`) || item.LayoutDocumentJSON == "" || item.LayoutDocumentSHA256 == "" || item.ExecutionReferenceSnapshotJSON == "" {
			t.Fatalf("incomplete frozen item = %#v", item)
		}
		if !strings.Contains(item.NegativePrompt, "文字") || !strings.Contains(item.NegativePrompt, "价格") || !strings.Contains(item.NegativePrompt, "Logo") {
			t.Fatalf("negative prompt = %q", item.NegativePrompt)
		}
		if strings.Contains(item.Prompt, "99元") || strings.Contains(item.Prompt, "有机认证") {
			t.Fatalf("prompt invented forbidden facts = %q", item.Prompt)
		}
	}
}

func TestProductDetailCompilerRejectsStrictParametersAndReferenceOverflow(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewProductDetailSetCompiler(NewSnapshotCostResolver())); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*CompileInput)
		want   error
	}{
		{"unknown parameter", func(in *CompileInput) { in.Parameters["extra"] = true }, ErrRecipeConstraint},
		{"duplicate section", func(in *CompileInput) { in.Parameters["detail_sections"] = []any{"hero", "hero"} }, ErrRecipeConstraint},
		{"unknown section", func(in *CompileInput) { in.Parameters["detail_sections"] = []any{"hero", "price"} }, ErrRecipeConstraint},
		{"count differs", func(in *CompileInput) { in.OutputCount = 1 }, ErrRecipeConstraint},
		{"unknown template", func(in *CompileInput) { in.Parameters["layout_template"] = "loud" }, ErrRecipeConstraint},
		{"reference overflow", func(in *CompileInput) {
			in.AssetBindings["pattern"] = []uint{51, 52}
			in.Assets = append(in.Assets,
				AssetBindingSnapshot{CommerceAssetID: 51, ReferenceAssetID: 501, Role: "pattern"},
				AssetBindingSnapshot{CommerceAssetID: 52, ReferenceAssetID: 502, Role: "pattern"},
			)
		}, ErrRecipeReferenceLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := cloneCompileInputForTest(validProductDetailCompileInput())
			in.Parameters = cloneMap(in.Parameters)
			tt.mutate(&in)
			_, err := registry.Compile(context.Background(), in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDecodeExecutionReferenceSetSnapshotAllowsSharedEmptySet(t *testing.T) {
	snapshot, err := DecodeExecutionReferenceSetSnapshot(`{"references":[]}`)
	if err != nil || len(snapshot.References) != 0 {
		t.Fatalf("empty shared reference snapshot = %+v, err=%v", snapshot, err)
	}
}

func TestGeneralCompilerSnapshotCostIsPerItem(t *testing.T) {
	input := validProductDetailCompileInput()
	input.PricingSnapshot.Entries = []PricingSnapshotEntry{
		{Pipeline: "general", RecipeKey: "product_detail_set", QualityTier: "standard", Credits: 2, RequiredCapabilities: []string{"image"}},
	}
	items, err := NewProductDetailSetCompiler(NewSnapshotCostResolver()).Compile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, item := range items {
		total += item.EstimatedCredits
	}
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
}

func TestProductDetailCompilerDeduplicatesUnderlyingReferences(t *testing.T) {
	input := validProductDetailCompileInput()
	input.Assets[3].ReferenceAssetID = input.Assets[0].ReferenceAssetID
	items, err := NewProductDetailSetCompiler(NewSnapshotCostResolver()).Compile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(items[0].AssetIDs, []uint{11, 31}) {
		t.Fatalf("asset ids = %#v, want underlying reference dedupe", items[0].AssetIDs)
	}
}

func validProductDetailCompileInput() CompileInput {
	sku := uint(9)
	return CompileInput{
		UserID: 1, ProjectID: 2, PrimarySKUID: sku, CreativeSpecID: 8,
		CreativeSpec: CreativeSpecSnapshot{ID: 8, Version: 3, Status: "confirmed", ContentSHA256: "spec-sha",
			ProductFactsJSON:  `{"name":"保温杯","material":"不锈钢","price":"99元","certification":"有机认证"}`,
			SellingPointsJSON: `["保温","轻便"]`, ForbiddenChangesJSON: `["不得改变杯盖"]`, BrandToneJSON: `{"tone":"简洁"}`,
			CopyBlocksJSON: `[{"text":"全天保温"}]`},
		RecipeKey: "product_detail_set", Pipeline: "general", RecipeVersion: 1, OutputCount: 2,
		AspectRatio: "4:5", QualityTier: "standard",
		AssetBindings: map[string][]uint{"product_front": {11}, "product_detail": {31, 32}, "logo": {41}},
		Assets: []AssetBindingSnapshot{
			{CommerceAssetID: 11, ReferenceAssetID: 101, SKUID: &sku, Role: "product_front"},
			{CommerceAssetID: 31, ReferenceAssetID: 301, SKUID: &sku, Role: "product_detail"},
			{CommerceAssetID: 32, ReferenceAssetID: 302, SKUID: &sku, Role: "product_detail"},
			{CommerceAssetID: 41, ReferenceAssetID: 401, Role: "logo"},
		},
		PricingSnapshot: PricingSnapshot{
			Version: "catalog-v1",
			Entries: []PricingSnapshotEntry{{
				Pipeline: "general", RecipeKey: "product_detail_set", QualityTier: "standard",
				ModelID: 77, ModelName: "detail-image-v1", Credits: 3, RequiredCapabilities: []string{"image"},
			}},
		},
		Parameters: map[string]any{"detail_sections": []any{"hero", "specification"}, "layout_template": "clean"},
	}
}
