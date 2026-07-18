package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrRecipeAlreadyRegistered      = errors.New("commerce recipe already registered")
	ErrRecipeDefinitionInvalid      = errors.New("commerce recipe definition invalid")
	ErrRecipeNotFound               = errors.New("commerce recipe not found")
	ErrRecipeConstraint             = errors.New("commerce recipe constraint violated")
	ErrCompiledItemIdentityMismatch = errors.New("compiled generation item identity mismatch")
)

const defaultCommerceJobMaxAttempts = 3

type AssetRequirement struct {
	Role      string `json:"role"`
	MinCount  int    `json:"min_count"`
	MaxCount  int    `json:"max_count"`
	MediaKind string `json:"media_kind"`
	Required  bool   `json:"required"`
}

type RecipeDisplayOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type RecipeSectionScope struct {
	Scope        string `json:"scope"`
	Configurable bool   `json:"configurable,omitempty"`
}

type RecipeDefinition struct {
	Key                   string                        `json:"key"`
	Version               int                           `json:"version"`
	Title                 string                        `json:"title"`
	Pipeline              string                        `json:"pipeline"`
	RequiredAssets        []AssetRequirement            `json:"required_assets"`
	OptionalAssets        []AssetRequirement            `json:"optional_assets"`
	AllowedOutputCounts   []int                         `json:"allowed_output_counts"`
	AspectRatios          []string                      `json:"aspect_ratios"`
	QualityTiers          []string                      `json:"quality_tiers"`
	QualityOptions        []RecipeDisplayOption         `json:"quality_options,omitempty"`
	DefaultParameters     map[string]any                `json:"parameters"`
	Capabilities          []string                      `json:"capabilities"`
	Sections              []string                      `json:"sections"`
	SectionOptions        []RecipeDisplayOption         `json:"section_options,omitempty"`
	SectionScopes         map[string]RecipeSectionScope `json:"section_scopes,omitempty"`
	LayoutTemplates       []string                      `json:"layout_templates"`
	LayoutTemplateOptions []RecipeDisplayOption         `json:"layout_template_options,omitempty"`
	MaxAttempts           int                           `json:"max_attempts"`
}

type CompileInput struct {
	UserID, ProjectID, PrimarySKUID, CreativeSpecID uint
	CreativeSpec                                    CreativeSpecSnapshot
	SelectedSKUIDs                                  []uint
	RecipeKey, Pipeline, QualityTier                string
	RecipeVersion, OutputCount                      int
	AspectRatio                                     string
	AssetBindings                                   map[string][]uint
	Assets                                          []AssetBindingSnapshot
	PricingSnapshot                                 PricingSnapshot
	Parameters                                      map[string]any
	SKUSnapshots                                    map[uint]SKUSnapshot
}

type SKUSnapshot struct {
	ID                uint   `json:"id"`
	Code              string `json:"code"`
	SpecificationPath string `json:"specification_path"`
	AttributesJSON    string `json:"attributes_json"`
}

type AssetBindingSnapshot struct {
	CommerceAssetID, ReferenceAssetID uint
	SKUID                             *uint
	Role, MetadataJSON                string
}

type Compiler interface {
	Definition() RecipeDefinition
	Compile(context.Context, CompileInput) ([]CompiledGenerationItem, error)
}

type ItemPrice struct {
	Credits int
	Version string
}

type CostResolver interface {
	EstimateImageItem(
		ctx context.Context,
		snapshot PricingSnapshot,
		qualityTier string,
		requiredCapabilities []string,
	) (ItemPrice, error)
}

type CommerceExecutionBackend interface {
	Execute(context.Context, ItemExecutionRequest) (ExecutionResult, *ExecutionFailure)
}

type EstimateBatchRequest struct {
	RecipeKey, QualityTier       string
	RecipeVersion, OutputCount   int
	CreativeSpecID, PrimarySKUID uint
	SelectedSKUIDs               []uint
	AspectRatio                  string
	AssetBindings                map[string][]uint
	Parameters                   map[string]any
}

type SubmitBatchRequest struct {
	EstimateBatchRequest
	PricingSnapshotID string
}

type BatchEstimate struct {
	Items             []CompiledGenerationItem
	TotalItems        int
	EstimatedCredits  int
	ETASeconds        int
	PricingVersion    string
	PricingSnapshotID string
	PricingExpiresAt  time.Time `json:"pricing_expires_at"`
	RequestDigest     string
}

type BatchSnapshot struct {
	Batch CommerceGenerationBatch
	Items []CommerceGenerationItem
}

type Registry struct {
	mu        sync.RWMutex
	compilers map[string]Compiler
}

func NewRegistry() *Registry {
	return &Registry{compilers: make(map[string]Compiler)}
}

func (r *Registry) Register(compiler Compiler) error {
	if compiler == nil {
		return fmt.Errorf("register commerce recipe: nil compiler")
	}
	definition := compiler.Definition()
	if definition.MaxAttempts <= 0 {
		definition.MaxAttempts = defaultCommerceJobMaxAttempts
	}
	if err := validateRecipeDefinition(definition); err != nil {
		return err
	}
	key := registryKey(definition.Pipeline, definition.Key, definition.Version)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.compilers == nil {
		r.compilers = make(map[string]Compiler)
	}
	if _, exists := r.compilers[key]; exists {
		return fmt.Errorf("%w: %s/%s@%d", ErrRecipeAlreadyRegistered, definition.Pipeline, definition.Key, definition.Version)
	}
	r.compilers[key] = registeredCompiler{compiler: compiler, definition: cloneRecipeDefinition(definition)}
	return nil
}

func (r *Registry) Get(pipeline, recipeKey string, version int) (Compiler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	compiler, ok := r.compilers[registryKey(pipeline, recipeKey, version)]
	return compiler, ok
}

func (r *Registry) List(pipeline string) []RecipeDefinition {
	r.mu.RLock()
	definitions := make([]RecipeDefinition, 0, len(r.compilers))
	for _, compiler := range r.compilers {
		definition := compiler.Definition()
		if pipeline == "" || definition.Pipeline == pipeline {
			definitions = append(definitions, cloneRecipeDefinition(definition))
		}
	}
	r.mu.RUnlock()
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Pipeline != definitions[j].Pipeline {
			return definitions[i].Pipeline < definitions[j].Pipeline
		}
		if definitions[i].Key != definitions[j].Key {
			return definitions[i].Key < definitions[j].Key
		}
		return definitions[i].Version < definitions[j].Version
	})
	return definitions
}

func (r *Registry) Compile(ctx context.Context, input CompileInput) ([]CompiledGenerationItem, error) {
	compiler, ok := r.Get(input.Pipeline, input.RecipeKey, input.RecipeVersion)
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s@%d", ErrRecipeNotFound, input.Pipeline, input.RecipeKey, input.RecipeVersion)
	}
	definition := compiler.Definition()
	if err := validateCompileInput(definition, input); err != nil {
		return nil, err
	}
	items, err := compiler.Compile(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(items) != input.OutputCount {
		return nil, fmt.Errorf("%w: compiler returned %d items, want %d", ErrRecipeConstraint, len(items), input.OutputCount)
	}
	allowedSKUs := map[uint]struct{}{input.PrimarySKUID: {}}
	for _, skuID := range input.SelectedSKUIDs {
		allowedSKUs[skuID] = struct{}{}
	}
	for index, item := range items {
		if item.Pipeline != definition.Pipeline || item.RecipeKey != definition.Key || item.RecipeVersion != definition.Version {
			return nil, fmt.Errorf("%w: item %d is %s/%s@%d, want %s/%s@%d",
				ErrCompiledItemIdentityMismatch,
				index,
				item.Pipeline,
				item.RecipeKey,
				item.RecipeVersion,
				definition.Pipeline,
				definition.Key,
				definition.Version,
			)
		}
		if item.Scope != "shared" && item.SKUID == 0 {
			return nil, fmt.Errorf("%w: item %d has no SKU", ErrRecipeConstraint, index)
		}
		if item.Scope != "shared" {
			if _, ok := allowedSKUs[item.SKUID]; !ok {
				return nil, fmt.Errorf("%w: item %d SKU %d is not selected", ErrRecipeConstraint, index, item.SKUID)
			}
		}
	}
	return items, nil
}

func registryKey(pipeline, recipeKey string, version int) string {
	return pipeline + "\x00" + recipeKey + "\x00" + strconv.Itoa(version)
}

type registeredCompiler struct {
	compiler   Compiler
	definition RecipeDefinition
}

func (r registeredCompiler) Definition() RecipeDefinition {
	return cloneRecipeDefinition(r.definition)
}

func (r registeredCompiler) Compile(ctx context.Context, input CompileInput) ([]CompiledGenerationItem, error) {
	return r.compiler.Compile(ctx, input)
}

func validateRecipeDefinition(definition RecipeDefinition) error {
	if strings.TrimSpace(definition.Pipeline) == "" || strings.TrimSpace(definition.Key) == "" || definition.Version <= 0 {
		return fmt.Errorf("%w: recipe pipeline, key and positive version are required", ErrRecipeConstraint)
	}
	if len(definition.AllowedOutputCounts) == 0 || len(definition.AspectRatios) == 0 || len(definition.QualityTiers) == 0 {
		return fmt.Errorf("%w: output counts, aspect ratios and quality tiers are required", ErrRecipeConstraint)
	}
	if len(definition.SectionOptions)+len(definition.QualityOptions)+len(definition.LayoutTemplateOptions) > 0 {
		for _, group := range []struct {
			name    string
			values  []string
			options []RecipeDisplayOption
		}{
			{"sections", definition.Sections, definition.SectionOptions},
			{"quality tiers", definition.QualityTiers, definition.QualityOptions},
			{"layout templates", definition.LayoutTemplates, definition.LayoutTemplateOptions},
		} {
			if err := validateRecipeDisplayOptions(group.name, group.values, group.options); err != nil {
				return err
			}
		}
	}
	seenRoles := make(map[string]struct{}, len(definition.RequiredAssets)+len(definition.OptionalAssets))
	for _, requirement := range append(append([]AssetRequirement(nil), definition.RequiredAssets...), definition.OptionalAssets...) {
		role := strings.TrimSpace(requirement.Role)
		if role == "" || requirement.MinCount < 0 || requirement.MaxCount < requirement.MinCount {
			return fmt.Errorf("%w: invalid asset requirement", ErrRecipeConstraint)
		}
		if _, exists := seenRoles[role]; exists {
			return fmt.Errorf("%w: duplicate asset role %s", ErrRecipeConstraint, role)
		}
		seenRoles[role] = struct{}{}
	}
	return nil
}

func validateRecipeDisplayOptions(name string, values []string, options []RecipeDisplayOption) error {
	if len(options) != len(values) {
		return fmt.Errorf("%w: %s display options length differs from values", ErrRecipeDefinitionInvalid, name)
	}
	seen := make(map[string]struct{}, len(options))
	for index, option := range options {
		if option.Value != values[index] {
			return fmt.Errorf("%w: %s display option %d value differs from values", ErrRecipeDefinitionInvalid, name, index)
		}
		if _, exists := seen[option.Value]; exists {
			return fmt.Errorf("%w: %s display option value %q is duplicated", ErrRecipeDefinitionInvalid, name, option.Value)
		}
		seen[option.Value] = struct{}{}
		if strings.TrimSpace(option.Label) == "" {
			return fmt.Errorf("%w: %s display option %q label is required", ErrRecipeDefinitionInvalid, name, option.Value)
		}
	}
	return nil
}

func validateCompileInput(definition RecipeDefinition, input CompileInput) error {
	if input.Pipeline != definition.Pipeline || input.RecipeKey != definition.Key || input.RecipeVersion != definition.Version {
		return fmt.Errorf("%w: compile input identity differs from recipe definition", ErrRecipeConstraint)
	}
	if input.CreativeSpecID == 0 || input.CreativeSpec.ID != input.CreativeSpecID || input.CreativeSpec.Status != "confirmed" || strings.TrimSpace(input.CreativeSpec.ContentSHA256) == "" {
		return ErrCreativeSpecNotConfirmed
	}
	allowsExpandedSKUSet := definition.Key == ProductDetailSetRecipeKey && input.OutputCount >= 1 && input.OutputCount <= 304
	if !containsInt(definition.AllowedOutputCounts, input.OutputCount) && !allowsExpandedSKUSet {
		return fmt.Errorf("%w: output count %d is not allowed", ErrRecipeConstraint, input.OutputCount)
	}
	if !containsString(definition.AspectRatios, input.AspectRatio) {
		return fmt.Errorf("%w: aspect ratio %q is not allowed", ErrRecipeConstraint, input.AspectRatio)
	}
	if !containsString(definition.QualityTiers, input.QualityTier) {
		return fmt.Errorf("%w: quality tier %q is not allowed", ErrRecipeConstraint, input.QualityTier)
	}
	allowedRoles := make(map[string]AssetRequirement, len(definition.RequiredAssets)+len(definition.OptionalAssets))
	for _, requirement := range definition.RequiredAssets {
		allowedRoles[requirement.Role] = requirement
	}
	for _, requirement := range definition.OptionalAssets {
		allowedRoles[requirement.Role] = requirement
	}
	for role := range input.AssetBindings {
		if _, ok := allowedRoles[role]; !ok {
			return fmt.Errorf("%w: asset role %q is not supported", ErrRecipeConstraint, role)
		}
	}
	for role, requirement := range allowedRoles {
		count := len(input.AssetBindings[role])
		minimum := requirement.MinCount
		if requirement.Required && minimum == 0 {
			minimum = 1
		}
		if count < minimum || (requirement.MaxCount > 0 && count > requirement.MaxCount) {
			return fmt.Errorf("%w: asset role %q count %d is outside [%d,%d]", ErrRecipeConstraint, role, count, minimum, requirement.MaxCount)
		}
	}
	if err := validateAssetBindingSnapshots(input); err != nil {
		return err
	}
	return nil
}

func validateAssetBindingSnapshots(input CompileInput) error {
	boundRoles := make(map[uint]string)
	for role, assetIDs := range input.AssetBindings {
		for _, assetID := range assetIDs {
			if assetID == 0 {
				return fmt.Errorf("%w: asset binding for role %q has zero ID", ErrRecipeConstraint, role)
			}
			if previousRole, exists := boundRoles[assetID]; exists {
				return fmt.Errorf("%w: asset %d is bound more than once to %q and %q", ErrRecipeConstraint, assetID, previousRole, role)
			}
			boundRoles[assetID] = role
		}
	}

	allowedSKUs := make(map[uint]struct{}, len(input.SelectedSKUIDs)+1)
	if input.PrimarySKUID != 0 {
		allowedSKUs[input.PrimarySKUID] = struct{}{}
	}
	for _, skuID := range input.SelectedSKUIDs {
		if skuID != 0 {
			allowedSKUs[skuID] = struct{}{}
		}
	}
	seenSnapshots := make(map[uint]struct{}, len(input.Assets))
	for _, snapshot := range input.Assets {
		if snapshot.CommerceAssetID == 0 || snapshot.ReferenceAssetID == 0 {
			return fmt.Errorf("%w: asset snapshot IDs must be non-zero", ErrRecipeConstraint)
		}
		if _, exists := seenSnapshots[snapshot.CommerceAssetID]; exists {
			return fmt.Errorf("%w: duplicate asset snapshot %d", ErrRecipeConstraint, snapshot.CommerceAssetID)
		}
		seenSnapshots[snapshot.CommerceAssetID] = struct{}{}
		expectedRole, bound := boundRoles[snapshot.CommerceAssetID]
		if !bound {
			return fmt.Errorf("%w: asset snapshot %d is not bound", ErrRecipeConstraint, snapshot.CommerceAssetID)
		}
		if snapshot.Role != expectedRole {
			return fmt.Errorf("%w: asset snapshot %d role %q does not match binding role %q", ErrRecipeConstraint, snapshot.CommerceAssetID, snapshot.Role, expectedRole)
		}
		if snapshot.SKUID != nil {
			if *snapshot.SKUID == 0 {
				return fmt.Errorf("%w: asset snapshot %d has zero SKU", ErrRecipeConstraint, snapshot.CommerceAssetID)
			}
			if _, allowed := allowedSKUs[*snapshot.SKUID]; !allowed {
				return fmt.Errorf("%w: asset snapshot %d SKU %d is not selected", ErrRecipeConstraint, snapshot.CommerceAssetID, *snapshot.SKUID)
			}
		}
	}
	for assetID := range boundRoles {
		if _, exists := seenSnapshots[assetID]; !exists {
			return fmt.Errorf("%w: bound asset %d has no snapshot", ErrRecipeConstraint, assetID)
		}
	}
	return nil
}

func cloneRecipeDefinition(definition RecipeDefinition) RecipeDefinition {
	definition.RequiredAssets = append([]AssetRequirement(nil), definition.RequiredAssets...)
	definition.OptionalAssets = append([]AssetRequirement(nil), definition.OptionalAssets...)
	definition.AllowedOutputCounts = append([]int(nil), definition.AllowedOutputCounts...)
	definition.AspectRatios = append([]string(nil), definition.AspectRatios...)
	definition.QualityTiers = append([]string(nil), definition.QualityTiers...)
	definition.QualityOptions = append([]RecipeDisplayOption(nil), definition.QualityOptions...)
	definition.Capabilities = append([]string(nil), definition.Capabilities...)
	definition.Sections = append([]string(nil), definition.Sections...)
	definition.SectionOptions = append([]RecipeDisplayOption(nil), definition.SectionOptions...)
	if definition.SectionScopes != nil {
		definition.SectionScopes = maps.Clone(definition.SectionScopes)
	}
	definition.LayoutTemplates = append([]string(nil), definition.LayoutTemplates...)
	definition.LayoutTemplateOptions = append([]RecipeDisplayOption(nil), definition.LayoutTemplateOptions...)
	if definition.DefaultParameters != nil {
		definition.DefaultParameters = cloneMap(definition.DefaultParameters)
	}
	return definition
}

func cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
