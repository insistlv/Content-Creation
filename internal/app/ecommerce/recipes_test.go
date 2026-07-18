package ecommerce

import (
	"context"
	"errors"
	"testing"
)

type fakeCostResolver struct {
	price ItemPrice
	calls int
}

func (f *fakeCostResolver) EstimateImageItem(_ context.Context, _ PricingSnapshot, _ string, _ []string) (ItemPrice, error) {
	f.calls++
	return f.price, nil
}

type fakeRecipe struct {
	definition RecipeDefinition
	resolver   CostResolver
	items      []CompiledGenerationItem
}

func (f fakeRecipe) Definition() RecipeDefinition { return f.definition }

func (f fakeRecipe) Compile(ctx context.Context, input CompileInput) ([]CompiledGenerationItem, error) {
	price, err := f.resolver.EstimateImageItem(ctx, input.PricingSnapshot, input.QualityTier, []string{"image"})
	if err != nil {
		return nil, err
	}
	items := append([]CompiledGenerationItem(nil), f.items...)
	for index := range items {
		items[index].PricingVersion = price.Version
		items[index].EstimatedCredits = price.Credits
	}
	return items, nil
}

func TestRegistryRejectsDuplicateAndInvalidCompiledIdentity(t *testing.T) {
	definition := RecipeDefinition{
		Key:                 "fake_recipe",
		Title:               "Fake",
		Pipeline:            "general",
		Version:             1,
		RequiredAssets:      []AssetRequirement{{Role: "product", MinCount: 1, MaxCount: 2, MediaKind: "image", Required: true}},
		AllowedOutputCounts: []int{1},
		AspectRatios:        []string{"1:1"},
		QualityTiers:        []string{"standard"},
	}
	resolver := &fakeCostResolver{price: ItemPrice{Credits: 3, Version: "price-v1"}}
	compiler := fakeRecipe{
		definition: definition,
		resolver:   resolver,
		items: []CompiledGenerationItem{{
			SKUID:         9,
			Pipeline:      "general",
			RecipeKey:     "fake_recipe",
			RecipeVersion: 1,
		}},
	}
	registry := NewRegistry()
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("Register: %v", err)
	}
	replacement := compiler
	replacement.definition.Title = "Replacement"
	if err := registry.Register(replacement); !errors.Is(err, ErrRecipeAlreadyRegistered) {
		t.Fatalf("duplicate Register error = %v, want ErrRecipeAlreadyRegistered", err)
	}
	loaded, ok := registry.Get("general", "fake_recipe", 1)
	if !ok || loaded.Definition().Title != "Fake" {
		t.Fatalf("registered compiler was overwritten or missing: ok=%v definition=%#v", ok, loaded.Definition())
	}

	input := CompileInput{
		UserID:         1,
		ProjectID:      2,
		PrimarySKUID:   9,
		CreativeSpecID: 8,
		CreativeSpec:   CreativeSpecSnapshot{ID: 8, Status: "confirmed", ContentSHA256: "hash"},
		RecipeKey:      "fake_recipe",
		Pipeline:       "general",
		RecipeVersion:  1,
		OutputCount:    1,
		AspectRatio:    "1:1",
		QualityTier:    "standard",
		AssetBindings:  map[string][]uint{"product": {3}},
		Assets:         []AssetBindingSnapshot{{CommerceAssetID: 3, ReferenceAssetID: 30, Role: "product"}},
	}
	items, err := registry.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(items) != 1 || items[0].EstimatedCredits != 3 || resolver.calls != 1 {
		t.Fatalf("compiled items=%#v resolver.calls=%d", items, resolver.calls)
	}

	badResolver := &fakeCostResolver{price: ItemPrice{Credits: 3, Version: "price-v1"}}
	bad := fakeRecipe{definition: RecipeDefinition{
		Key: "bad", Pipeline: "general", Version: 1, AllowedOutputCounts: []int{1}, AspectRatios: []string{"1:1"}, QualityTiers: []string{"standard"},
	}, resolver: badResolver, items: []CompiledGenerationItem{{SKUID: 9, Pipeline: "fashion", RecipeKey: "bad", RecipeVersion: 1}}}
	if err := registry.Register(bad); err != nil {
		t.Fatalf("Register bad: %v", err)
	}
	input.RecipeKey = "bad"
	input.AssetBindings = nil
	input.Assets = nil
	if _, err := registry.Compile(context.Background(), input); !errors.Is(err, ErrCompiledItemIdentityMismatch) {
		t.Fatalf("identity mismatch error = %v, want ErrCompiledItemIdentityMismatch", err)
	}
}

func TestRegistryValidatesCreativeSpecAndRecipeConstraints(t *testing.T) {
	resolver := &fakeCostResolver{price: ItemPrice{Credits: 1, Version: "v1"}}
	registry := NewRegistry()
	compiler := fakeRecipe{definition: RecipeDefinition{
		Key: "fake_recipe", Pipeline: "fashion", Version: 2,
		RequiredAssets:      []AssetRequirement{{Role: "garment", MinCount: 1, MaxCount: 1, Required: true}},
		AllowedOutputCounts: []int{1, 2}, AspectRatios: []string{"3:4"}, QualityTiers: []string{"hd"},
	}, resolver: resolver, items: []CompiledGenerationItem{{SKUID: 4, Pipeline: "fashion", RecipeKey: "fake_recipe", RecipeVersion: 2}}}
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("Register: %v", err)
	}
	input := CompileInput{
		PrimarySKUID: 4, CreativeSpecID: 5, CreativeSpec: CreativeSpecSnapshot{ID: 5, Status: "draft"},
		RecipeKey: "fake_recipe", Pipeline: "fashion", RecipeVersion: 2, OutputCount: 1,
		AspectRatio: "3:4", QualityTier: "hd", AssetBindings: map[string][]uint{"garment": {7}},
		Assets: []AssetBindingSnapshot{{CommerceAssetID: 7, ReferenceAssetID: 70, Role: "garment"}},
	}
	if _, err := registry.Compile(context.Background(), input); !errors.Is(err, ErrCreativeSpecNotConfirmed) {
		t.Fatalf("draft creative spec error = %v, want ErrCreativeSpecNotConfirmed", err)
	}
	input.CreativeSpec.Status = "confirmed"
	input.CreativeSpec.ContentSHA256 = "hash"
	input.OutputCount = 3
	if _, err := registry.Compile(context.Background(), input); !errors.Is(err, ErrRecipeConstraint) {
		t.Fatalf("invalid output count error = %v, want ErrRecipeConstraint", err)
	}
	input.OutputCount = 1
	delete(input.AssetBindings, "garment")
	if _, err := registry.Compile(context.Background(), input); !errors.Is(err, ErrRecipeConstraint) {
		t.Fatalf("missing asset role error = %v, want ErrRecipeConstraint", err)
	}
}

func TestRegistryAssetSnapshots(t *testing.T) {
	registry := NewRegistry()
	resolver := &fakeCostResolver{price: ItemPrice{Credits: 1, Version: "v1"}}
	compiler := fakeRecipe{
		definition: RecipeDefinition{
			Key: "asset_recipe", Pipeline: "fashion", Version: 1,
			RequiredAssets:      []AssetRequirement{{Role: "product", MinCount: 1, MaxCount: 2, Required: true}},
			OptionalAssets:      []AssetRequirement{{Role: "background", MinCount: 0, MaxCount: 1}},
			AllowedOutputCounts: []int{1}, AspectRatios: []string{"1:1"}, QualityTiers: []string{"standard"},
		},
		resolver: resolver,
		items:    []CompiledGenerationItem{{SKUID: 9, Pipeline: "fashion", RecipeKey: "asset_recipe", RecipeVersion: 1}},
	}
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("Register: %v", err)
	}
	base := CompileInput{
		PrimarySKUID: 9, SelectedSKUIDs: []uint{10},
		CreativeSpecID: 8, CreativeSpec: CreativeSpecSnapshot{ID: 8, Status: "confirmed", ContentSHA256: "hash"},
		RecipeKey: "asset_recipe", Pipeline: "fashion", RecipeVersion: 1, OutputCount: 1,
		AspectRatio: "1:1", QualityTier: "standard",
		AssetBindings: map[string][]uint{"product": {11}},
		Assets:        []AssetBindingSnapshot{{CommerceAssetID: 11, ReferenceAssetID: 101, Role: "product", SKUID: uintPointer(9)}},
	}
	if _, err := registry.Compile(context.Background(), base); err != nil {
		t.Fatalf("valid asset snapshot Compile: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*CompileInput)
	}{
		{"zero binding", func(input *CompileInput) { input.AssetBindings["product"] = []uint{0} }},
		{"duplicate binding", func(input *CompileInput) { input.AssetBindings["product"] = []uint{11, 11} }},
		{"missing snapshot", func(input *CompileInput) { input.Assets = nil }},
		{"role mismatch", func(input *CompileInput) { input.Assets[0].Role = "background" }},
		{"extra snapshot", func(input *CompileInput) {
			input.Assets = append(input.Assets, AssetBindingSnapshot{CommerceAssetID: 12, ReferenceAssetID: 102, Role: "product"})
		}},
		{"incompatible sku", func(input *CompileInput) { input.Assets[0].SKUID = uintPointer(99) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCompileInputForTest(base)
			test.mutate(&input)
			if _, err := registry.Compile(context.Background(), input); !errors.Is(err, ErrRecipeConstraint) {
				t.Fatalf("Compile error = %v, want ErrRecipeConstraint", err)
			}
		})
	}
}

func cloneCompileInputForTest(input CompileInput) CompileInput {
	cloned := input
	cloned.SelectedSKUIDs = append([]uint(nil), input.SelectedSKUIDs...)
	cloned.Assets = append([]AssetBindingSnapshot(nil), input.Assets...)
	cloned.AssetBindings = make(map[string][]uint, len(input.AssetBindings))
	for role, ids := range input.AssetBindings {
		cloned.AssetBindings[role] = append([]uint(nil), ids...)
	}
	return cloned
}

func uintPointer(value uint) *uint {
	return &value
}
