package ecommerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrRecipeReferenceLimitExceeded = errors.New("recipe_reference_limit_exceeded")
	ErrRecipeModelUnavailable       = errors.New("recipe_model_unavailable")
)

const (
	ProductDetailSetRecipeKey = "product_detail_set"
	ProductDetailSetVersion   = 1
)

type SnapshotCostResolver struct{}

func NewSnapshotCostResolver() *SnapshotCostResolver { return &SnapshotCostResolver{} }

func (*SnapshotCostResolver) EstimateImageItem(_ context.Context, snapshot PricingSnapshot, qualityTier string, capabilities []string) (ItemPrice, error) {
	for _, entry := range snapshot.Entries {
		if entry.Pipeline == "general" && entry.RecipeKey == ProductDetailSetRecipeKey && entry.QualityTier == qualityTier && entry.Credits > 0 {
			capable := true
			for _, capability := range capabilities {
				if !containsString(entry.RequiredCapabilities, capability) {
					capable = false
					break
				}
			}
			if !capable {
				continue
			}
			return ItemPrice{Credits: entry.Credits, Version: snapshot.Version}, nil
		}
	}
	return ItemPrice{}, ErrRecipeModelUnavailable
}

type ProductDetailSetCompiler struct{ costs CostResolver }

func NewProductDetailSetCompiler(costs CostResolver) *ProductDetailSetCompiler {
	return &ProductDetailSetCompiler{costs: costs}
}

func (*ProductDetailSetCompiler) Definition() RecipeDefinition {
	return ProductDetailSetDefinition()
}

func ProductDetailSetDefinition() RecipeDefinition {
	return RecipeDefinition{
		Key: ProductDetailSetRecipeKey, Title: "商品详情页套图", Pipeline: "general", Version: ProductDetailSetVersion,
		RequiredAssets: []AssetRequirement{{Role: "product_front", MinCount: 1, MaxCount: 101, MediaKind: "image", Required: true}},
		OptionalAssets: []AssetRequirement{
			{Role: "product_back", MinCount: 0, MaxCount: 101, MediaKind: "image"},
			{Role: "product_detail", MinCount: 0, MaxCount: 800, MediaKind: "image"},
			{Role: "logo", MinCount: 0, MaxCount: 101, MediaKind: "image"},
			{Role: "pattern", MinCount: 0, MaxCount: 400, MediaKind: "image"},
		},
		AllowedOutputCounts: []int{1, 2, 3, 4, 5, 6, 7},
		AspectRatios:        []string{"1:1", "3:4", "4:5", "9:16"},
		QualityTiers:        []string{"standard", "high_fidelity"}, MaxAttempts: 3,
		QualityOptions: []RecipeDisplayOption{
			{Value: "standard", Label: "标准"},
			{Value: "high_fidelity", Label: "高清"},
		},
		DefaultParameters: map[string]any{"layout_template": "clean"},
		Capabilities:      []string{"image"},
		Sections:          []string{"hero", "selling_points", "material", "detail", "usage", "specification", "closing"},
		SectionOptions: []RecipeDisplayOption{
			{Value: "hero", Label: "首屏主视觉"},
			{Value: "selling_points", Label: "核心卖点"},
			{Value: "material", Label: "材质工艺"},
			{Value: "detail", Label: "细节展示"},
			{Value: "usage", Label: "使用场景"},
			{Value: "specification", Label: "规格参数"},
			{Value: "closing", Label: "收尾转化"},
		},
		SectionScopes: map[string]RecipeSectionScope{
			"hero": {Scope: "sku"}, "selling_points": {Scope: "shared"},
			"material": {Scope: "shared"}, "detail": {Scope: "sku", Configurable: true},
			"usage": {Scope: "shared"}, "specification": {Scope: "sku"}, "closing": {Scope: "shared"},
		},
		LayoutTemplates: []string{"clean", "dark_gradient", "brand_band"},
		LayoutTemplateOptions: []RecipeDisplayOption{
			{Value: "clean", Label: "简洁留白"},
			{Value: "dark_gradient", Label: "深色渐变"},
			{Value: "brand_band", Label: "品牌色带"},
		},
	}
}

func (c *ProductDetailSetCompiler) Compile(ctx context.Context, input CompileInput) ([]CompiledGenerationItem, error) {
	definition := c.Definition()
	if len(input.SelectedSKUIDs) == 0 && input.PrimarySKUID != 0 {
		input.SelectedSKUIDs = []uint{input.PrimarySKUID}
	}
	sections, scopes, template, err := productDetailParameters(input.Parameters, definition)
	if err != nil {
		return nil, err
	}
	wanted := 0
	for _, section := range sections {
		if scopes[section] == "shared" {
			wanted++
		} else {
			wanted += len(input.SelectedSKUIDs)
		}
	}
	if wanted != input.OutputCount {
		return nil, fmt.Errorf("%w: output_count must equal compiled section count", ErrRecipeConstraint)
	}
	if c == nil || c.costs == nil {
		return nil, ErrRecipeModelUnavailable
	}
	price, err := c.costs.EstimateImageItem(ctx, input.PricingSnapshot, input.QualityTier, []string{"image"})
	if err != nil {
		return nil, err
	}
	modelSnapshot, err := modelSnapshotJSON(input.PricingSnapshot, input.QualityTier)
	if err != nil {
		return nil, err
	}
	items := make([]CompiledGenerationItem, 0, wanted)
	for index, section := range sections {
		skuIDs := input.SelectedSKUIDs
		if scopes[section] == "shared" {
			skuIDs = []uint{0}
		}
		for _, skuID := range skuIDs {
			scope := "sku"
			if skuID == 0 {
				scope = "shared"
			}
			texts := confirmedLayoutTexts(input.CreativeSpec, skuID, scope)
			refs, referenceSnapshots := orderedDetailReferences(input, index, skuID)
			if len(refs) > 4 {
				return nil, ErrRecipeReferenceLimitExceeded
			}
			doc := DefaultProductDetailLayout(section, template, input.AspectRatio, texts)
			if len(texts) > 20 {
				return nil, ErrLayoutTextOverflow
			}
			if err := ValidateLayoutDocument(doc); err != nil {
				return nil, err
			}
			docJSON, err := EncodeJSON(doc)
			if err != nil {
				return nil, err
			}
			docSHA := sha256.Sum256([]byte(docJSON))
			refJSON, err := EncodeJSON(ExecutionReferenceSetSnapshot{References: referenceSnapshots})
			if err != nil {
				return nil, err
			}
			inherited := skuID != 0 && !hasSKUOwnedAsset(input.Assets, skuID)
			prompt := confirmedProductDetailPrompt(section, input.CreativeSpec, skuID, scope)
			if inherited {
				prompt += "；缺少 SKU 专属素材，当前仅继承公共素材，不得猜测规格差异"
			}
			slot := fmt.Sprintf("product_detail_set:v1:sku-%d:%s", skuID, section)
			if skuID == 0 {
				scope, slot = "shared", fmt.Sprintf("product_detail_set:v1:shared:%s", section)
			}
			skuSnapshot := input.SKUSnapshots[skuID]
			skuJSON, _ := EncodeJSON(skuSnapshot)
			items = append(items, CompiledGenerationItem{
				SKUID: skuID, Scope: scope, Pipeline: "general", RecipeKey: ProductDetailSetRecipeKey, RecipeVersion: ProductDetailSetVersion,
				SlotKey: slot,
				Prompt:  prompt, NegativePrompt: "禁止生成任何文字、数字、价格、标签、水印；禁止重绘 Logo；禁止补造规格、认证、功效、成分。",
				ToolMode: "generate", ReferenceIntent: "product_consistency", AssetIDs: refs,
				AspectRatio: input.AspectRatio, WorkCategory: "product_main", PostProcessJSON: docJSON,
				PricingVersion: price.Version, EstimatedCredits: price.Credits, Section: section,
				CreativeSpecSnapshot: input.CreativeSpec, ModelSnapshotJSON: modelSnapshot,
				SKUCode: skuSnapshot.Code, SpecificationPath: skuSnapshot.SpecificationPath, SKUSnapshotJSON: skuJSON, AssetSnapshotJSON: refJSON,
				InheritedSharedAssets: inherited,
				LayoutDocumentJSON:    docJSON, LayoutDocumentSHA256: hex.EncodeToString(docSHA[:]), ExecutionReferenceSnapshotJSON: refJSON,
			})
		}
	}
	return items, nil
}

func productDetailParameters(parameters map[string]any, definition RecipeDefinition) ([]string, map[string]string, string, error) {
	for key := range parameters {
		if key != "detail_sections" && key != "layout_template" && key != "section_scopes" {
			return nil, nil, "", fmt.Errorf("%w: unknown parameter %q", ErrRecipeConstraint, key)
		}
	}
	raw, ok := parameters["detail_sections"]
	if !ok {
		return nil, nil, "", fmt.Errorf("%w: detail_sections is required", ErrRecipeConstraint)
	}
	var sections []string
	switch values := raw.(type) {
	case []string:
		sections = append(sections, values...)
	case []any:
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, nil, "", fmt.Errorf("%w: detail section must be a string", ErrRecipeConstraint)
			}
			sections = append(sections, text)
		}
	default:
		return nil, nil, "", fmt.Errorf("%w: detail_sections must be an array", ErrRecipeConstraint)
	}
	if len(sections) < 1 || len(sections) > 7 {
		return nil, nil, "", fmt.Errorf("%w: detail_sections length is outside 1..7", ErrRecipeConstraint)
	}
	seen := map[string]struct{}{}
	for i, section := range sections {
		section = strings.TrimSpace(section)
		sections[i] = section
		if !containsString(definition.Sections, section) {
			return nil, nil, "", fmt.Errorf("%w: unknown detail section %q", ErrRecipeConstraint, section)
		}
		if _, ok := seen[section]; ok {
			return nil, nil, "", fmt.Errorf("%w: duplicate detail section %q", ErrRecipeConstraint, section)
		}
		seen[section] = struct{}{}
	}
	template, _ := parameters["layout_template"].(string)
	template = strings.TrimSpace(template)
	if template == "" {
		template, _ = definition.DefaultParameters["layout_template"].(string)
		template = strings.TrimSpace(template)
	}
	if !containsString(definition.LayoutTemplates, template) {
		return nil, nil, "", fmt.Errorf("%w: unknown layout_template %q", ErrRecipeConstraint, template)
	}
	scopes := make(map[string]string, len(definition.SectionScopes))
	for section, metadata := range definition.SectionScopes {
		scopes[section] = metadata.Scope
	}
	if rawOverrides, ok := parameters["section_scopes"]; ok {
		overrides, ok := rawOverrides.(map[string]any)
		if !ok {
			return nil, nil, "", fmt.Errorf("%w: section_scopes must be an object", ErrRecipeConstraint)
		}
		for section, rawScope := range overrides {
			metadata, exists := definition.SectionScopes[section]
			scope, stringOK := rawScope.(string)
			if !exists || !metadata.Configurable || !stringOK || (scope != "shared" && scope != "sku") {
				return nil, nil, "", fmt.Errorf("%w: section scope override is not allowed", ErrRecipeConstraint)
			}
			scopes[section] = scope
		}
	}
	return sections, scopes, template, nil
}

func orderedDetailReferences(input CompileInput, sectionIndex int, targetSKUID uint) ([]uint, []ExecutionReferenceSnapshot) {
	eligible := func(role string) []uint {
		result := []uint{}
		for _, id := range input.AssetBindings[role] {
			for _, asset := range input.Assets {
				if asset.CommerceAssetID == id && (asset.SKUID == nil || (targetSKUID != 0 && *asset.SKUID == targetSKUID)) {
					result = append(result, id)
					break
				}
			}
		}
		return result
	}
	ordered := eligible("product_front")
	if details := eligible("product_detail"); sectionIndex < len(details) {
		ordered = append(ordered, details[sectionIndex])
	}
	ordered = append(ordered, eligible("logo")...)
	ordered = append(ordered, eligible("pattern")...)
	referenceByAsset := make(map[uint]uint, len(input.Assets))
	roleByAsset := make(map[uint]string, len(input.Assets))
	for _, asset := range input.Assets {
		if targetSKUID == 0 && asset.SKUID != nil {
			continue
		}
		if targetSKUID != 0 && asset.SKUID != nil && *asset.SKUID != targetSKUID {
			continue
		}
		referenceByAsset[asset.CommerceAssetID] = asset.ReferenceAssetID
		roleByAsset[asset.CommerceAssetID] = asset.Role
	}
	seen, result := map[uint]struct{}{}, make([]uint, 0, len(ordered))
	snapshots := make([]ExecutionReferenceSnapshot, 0, len(ordered))
	for _, id := range ordered {
		if _, eligible := referenceByAsset[id]; !eligible {
			continue
		}
		dedupeID := referenceByAsset[id]
		if dedupeID == 0 {
			dedupeID = id
		}
		if _, ok := seen[dedupeID]; ok {
			continue
		}
		seen[dedupeID] = struct{}{}
		result = append(result, id)
		snapshots = append(snapshots, ExecutionReferenceSnapshot{CommerceAssetID: id, ReferenceAssetID: dedupeID, Role: roleByAsset[id], Order: len(snapshots)})
	}
	return result, snapshots
}

func hasSKUOwnedAsset(assets []AssetBindingSnapshot, skuID uint) bool {
	for _, asset := range assets {
		if asset.SKUID != nil && *asset.SKUID == skuID {
			return true
		}
	}
	return false
}

func DecodeExecutionReferenceSetSnapshot(raw string) (ExecutionReferenceSetSnapshot, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot ExecutionReferenceSetSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, fmt.Errorf("decode execution reference snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return snapshot, fmt.Errorf("execution reference snapshot has trailing data")
	}
	// Shared sections may intentionally have no SKU-bound input assets. The
	// compiler emits that empty frozen set, while SKU sections still carry the
	// references selected for their own context.
	if len(snapshot.References) > 4 {
		return snapshot, ErrRecipeReferenceLimitExceeded
	}
	seenCommerce, seenReference := map[uint]struct{}{}, map[uint]struct{}{}
	for index, reference := range snapshot.References {
		if reference.CommerceAssetID == 0 || reference.ReferenceAssetID == 0 || strings.TrimSpace(reference.Role) == "" || reference.Order != index {
			return snapshot, fmt.Errorf("%w: invalid frozen reference at order %d", ErrRecipeConstraint, index)
		}
		if _, ok := seenCommerce[reference.CommerceAssetID]; ok {
			return snapshot, fmt.Errorf("%w: duplicate commerce asset", ErrRecipeConstraint)
		}
		seenCommerce[reference.CommerceAssetID] = struct{}{}
		if _, ok := seenReference[reference.ReferenceAssetID]; ok {
			return snapshot, fmt.Errorf("%w: duplicate reference asset", ErrRecipeConstraint)
		}
		seenReference[reference.ReferenceAssetID] = struct{}{}
	}
	canonical, err := EncodeJSON(snapshot)
	if err != nil {
		return snapshot, err
	}
	if canonical != raw {
		return snapshot, fmt.Errorf("%w: execution reference snapshot is not canonical", ErrRecipeConstraint)
	}
	return snapshot, nil
}

func modelSnapshotJSON(snapshot PricingSnapshot, quality string) (string, error) {
	type modelCandidate struct {
		ModelID              uint     `json:"model_id"`
		ModelName            string   `json:"model_name"`
		ChannelID            uint     `json:"channel_id"`
		ProviderID           uint     `json:"provider_id"`
		RuntimeModel         string   `json:"runtime_model"`
		Endpoint             string   `json:"endpoint"`
		RouteOrder           int      `json:"route_order"`
		Credits              int      `json:"credits"`
		RequiredCapabilities []string `json:"required_capabilities"`
	}
	candidates := []modelCandidate{}
	var selectedModelID uint
	for _, entry := range snapshot.Entries {
		if entry.Pipeline == "general" && entry.RecipeKey == ProductDetailSetRecipeKey && entry.QualityTier == quality {
			if selectedModelID == 0 {
				selectedModelID = entry.ModelID
			}
			if entry.ModelID != selectedModelID {
				continue
			}
			caps := append([]string(nil), entry.RequiredCapabilities...)
			sort.Strings(caps)
			candidates = append(candidates, modelCandidate{entry.ModelID, entry.ModelName, entry.ChannelID, entry.ProviderID, entry.RuntimeModel, entry.Endpoint, entry.RouteOrder, entry.Credits, caps})
		}
	}
	if len(candidates) == 0 {
		return "", ErrRecipeModelUnavailable
	}
	return EncodeJSON(struct {
		PricingVersion string           `json:"pricing_version"`
		QualityTier    string           `json:"quality_tier"`
		Candidates     []modelCandidate `json:"candidates"`
	}{snapshot.Version, quality, candidates})
}

var forbiddenFactKeys = map[string]struct{}{"price": {}, "certification": {}, "efficacy": {}, "ingredients": {}, "价格": {}, "认证": {}, "功效": {}, "成分": {}}

func confirmedProductDetailPrompt(section string, spec CreativeSpecSnapshot, skuID uint, scope string) string {
	parts := []string{"为商品详情页生成无文字摄影底图", "章节=" + section}
	for _, raw := range append(creativeSpecFactsForTarget(spec, skuID, scope), spec.SellingPointsJSON, spec.ForbiddenChangesJSON, spec.BrandToneJSON) {
		var value any
		if json.Unmarshal([]byte(raw), &value) != nil {
			continue
		}
		value = stripForbiddenFacts(value)
		encoded, _ := EncodeJSON(value)
		if encoded != "null" && encoded != "{}" && encoded != "[]" {
			parts = append(parts, encoded)
		}
	}
	return strings.Join(parts, "；")
}

func creativeSpecFactsForTarget(spec CreativeSpecSnapshot, skuID uint, scope string) []string {
	common := strings.TrimSpace(spec.CommonFactsJSON)
	overrides := strings.TrimSpace(spec.SKUOverridesJSON)
	if !hasJSONContent(common) && !hasJSONContent(overrides) {
		return []string{spec.ProductFactsJSON}
	}
	result := []string{defaultJSON(common, "[]")}
	if scope == "sku" && skuID != 0 {
		var values map[string]json.RawMessage
		if json.Unmarshal([]byte(defaultJSON(overrides, "{}")), &values) == nil {
			if value, ok := values[strconv.FormatUint(uint64(skuID), 10)]; ok {
				result = append(result, string(value))
			}
		}
	}
	return result
}

func stripForbiddenFacts(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range typed {
			if _, blocked := forbiddenFactKeys[strings.ToLower(strings.TrimSpace(key))]; !blocked {
				out[key] = stripForbiddenFacts(child)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = stripForbiddenFacts(typed[i])
		}
		return out
	default:
		return value
	}
}

func confirmedLayoutTexts(spec CreativeSpecSnapshot, skuID uint, scope string) []string {
	texts := []string{}
	var blocks []map[string]any
	if json.Unmarshal([]byte(spec.CopyBlocksJSON), &blocks) == nil {
		for _, block := range blocks {
			if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
		}
	}
	var points []string
	if json.Unmarshal([]byte(spec.SellingPointsJSON), &points) == nil {
		texts = append(texts, points...)
	}
	for _, rawFacts := range creativeSpecFactsForTarget(spec, skuID, scope) {
		var facts map[string]any
		if json.Unmarshal([]byte(rawFacts), &facts) == nil {
			keys := make([]string, 0, len(facts))
			for key := range facts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if _, blocked := forbiddenFactKeys[strings.ToLower(key)]; blocked {
					continue
				}
				if value, ok := facts[key].(string); ok && value != "" {
					texts = append(texts, key+"："+value)
				}
			}
		}
		var observed []ObservedFact
		if json.Unmarshal([]byte(rawFacts), &observed) == nil {
			for _, fact := range observed {
				if strings.TrimSpace(fact.Value) != "" {
					texts = append(texts, fact.Field+"："+fact.Value)
				}
			}
		}
	}
	return texts
}
