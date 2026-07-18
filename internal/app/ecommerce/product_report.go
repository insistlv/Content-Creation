package ecommerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const CommerceJobKindProductAnalysis JobKind = "product_analysis"
const CommerceSubjectCreativeSpec = "creative_spec"

var ErrVisionNotConfigured = errors.New("commerce vision analyzer is not configured")

var productAnalysisAssetRoles = []string{"product", "product_front", "product_back", "product_detail"}

// ProductAnalysisAssetRoles returns the controlled asset roles accepted by
// product analysis. Callers receive a copy so the shared policy cannot be mutated.
func ProductAnalysisAssetRoles() []string {
	return append([]string(nil), productAnalysisAssetRoles...)
}

type BootstrapProjectInput struct {
	Title, Category, SKUCode, Pipeline string
	CategoryID                         uint
	CategorySource, CategoryPath       string
}

type BootstrapProjectResult struct {
	Product  CommerceProduct
	SKU      CommerceSKU
	Project  CommerceProject
	Replayed bool
}

type ObservedFact struct {
	Field          string  `json:"field"`
	Value          string  `json:"value"`
	Confidence     float64 `json:"confidence"`
	SourceAssetIDs []uint  `json:"source_asset_ids"`
}

type ProductBrandTone struct {
	Description string `json:"description"`
}

type ProductReport struct {
	ObservedFacts     []ObservedFact            `json:"observed_facts"`
	CommonFacts       []ObservedFact            `json:"common_facts"`
	SKUOverrides      map[string][]ObservedFact `json:"sku_overrides"`
	SellingPoints     []string                  `json:"selling_points"`
	ForbiddenChanges  []string                  `json:"forbidden_changes"`
	BrandTone         *ProductBrandTone         `json:"brand_tone"`
	MissingFields     []string                  `json:"missing_fields"`
	RiskNotices       []string                  `json:"risk_notices"`
	SuggestedSections []string                  `json:"suggested_sections"`
}

type AnalyzeProductInput struct {
	SourceAssetIDs   []uint
	UserRequirements string
}

type productAnalysisJobPayload struct {
	SourceAssetIDs      []uint `json:"source_asset_ids"`
	UserRequirements    string `json:"user_requirements"`
	AnalysisRequestHash string `json:"analysis_request_hash"`
	CreativeSpecVersion int    `json:"creative_spec_version"`
	ProjectID           uint   `json:"project_id"`
	CreativeSpecID      uint   `json:"creative_spec_id"`
}

type AnalyzeProductResult struct {
	CreativeSpec CommerceCreativeSpec
	Job          CommerceJob
}

type ProductAnalysisRequest struct {
	JobID, UserID, ProjectID, CreativeSpecID uint
	SourceAssetIDs                           []uint
	UserRequirements                         string
	AssetContexts                            []ProductAnalysisAssetContext
}

type ProductAnalysisAssetContext struct {
	AssetID uint  `json:"asset_id"`
	SKUID   *uint `json:"sku_id,omitempty"`
	Shared  bool  `json:"shared"`
}

type CommerceVisionAnalyzer interface {
	AnalyzeProduct(context.Context, ProductAnalysisRequest) (string, error)
}

type CommerceVisionAvailability interface {
	CommerceVisionConfigured(context.Context) (bool, error)
}

func (s *Service) ConfigureVisionAnalyzer(analyzer CommerceVisionAnalyzer) {
	if s != nil {
		s.visionMu.Lock()
		defer s.visionMu.Unlock()
		s.visionAnalyzer = analyzer
	}
}

func (s *Service) VisionAnalyzer() CommerceVisionAnalyzer {
	if s == nil {
		return nil
	}
	s.visionMu.RLock()
	defer s.visionMu.RUnlock()
	return s.visionAnalyzer
}

func (s *Service) BootstrapProject(ctx context.Context, userID uint, key string, input BootstrapProjectInput) (BootstrapProjectResult, error) {
	key = strings.TrimSpace(key)
	input.Title, input.Category, input.SKUCode, input.Pipeline = strings.TrimSpace(input.Title), strings.TrimSpace(input.Category), strings.TrimSpace(input.SKUCode), strings.TrimSpace(input.Pipeline)
	input.CategorySource, input.CategoryPath = strings.TrimSpace(input.CategorySource), strings.TrimSpace(input.CategoryPath)
	if key == "" {
		return BootstrapProjectResult{}, invalidField("idempotency_key", "Idempotency-Key is required")
	}
	if input.Title == "" {
		return BootstrapProjectResult{}, invalidField("title", "title is required")
	}
	if input.Pipeline != "general" {
		return BootstrapProjectResult{}, ErrInvalidPipeline
	}
	if input.SKUCode == "" {
		input.SKUCode = "DEFAULT"
	}
	if input.CategoryID != 0 || input.CategorySource != "" {
		if input.CategoryID == 0 || input.CategorySource == "" {
			return BootstrapProjectResult{}, ErrCategoryUnavailable
		}
		path, resolveErr := s.ResolveCategorySelection(ctx, userID, input.CategoryID, input.CategorySource)
		if resolveErr != nil {
			return BootstrapProjectResult{}, resolveErr
		}
		input.Category, input.CategoryPath = path, path
	}
	digest, err := canonicalDigest(input)
	if err != nil {
		return BootstrapProjectResult{}, err
	}
	scope := "project_bootstrap"
	var result BootstrapProjectResult
	err = runProjectMutationTransaction(ctx, s.repository.DB(), func(tx *gorm.DB) error {
		var record CommerceIdempotencyRecord
		err := tx.Where("user_id = ? AND scope = ? AND idempotency_key = ?", userID, scope, key).First(&record).Error
		if err == nil {
			if record.RequestDigest != digest {
				return ErrIdempotencyConflict
			}
			if err := loadBootstrapResult(tx, userID, record, &result); err != nil {
				return err
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		product := CommerceProduct{UserID: userID, Name: input.Title, Category: input.Category, CategorySource: input.CategorySource, CategoryPath: input.CategoryPath, Status: "active", SellingPointsJSON: "[]", TargetChannelsJSON: "[]"}
		if input.CategoryID != 0 {
			product.CategoryID = &input.CategoryID
		}
		if err := tx.Create(&product).Error; err != nil {
			return err
		}
		sku := CommerceSKU{UserID: userID, ProductID: product.ID, Code: input.SKUCode, Status: "active", AttributesJSON: "{}"}
		if err := tx.Create(&sku).Error; err != nil {
			return err
		}
		project := CommerceProject{UserID: userID, ProductID: product.ID, DefaultSKUID: &sku.ID, Title: input.Title, Pipeline: "general", Status: "active"}
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		record = CommerceIdempotencyRecord{UserID: userID, Scope: scope, IdempotencyKey: key, RequestDigest: digest, ProductID: &product.ID, SKUID: &sku.ID, ProjectID: &project.ID}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		result = BootstrapProjectResult{Product: product, SKU: sku, Project: project}
		return nil
	})
	if err != nil && isUniqueConstraintError(err) {
		var record CommerceIdempotencyRecord
		if loadErr := s.repository.DB().WithContext(ctx).Where("user_id = ? AND scope = ? AND idempotency_key = ?", userID, scope, key).First(&record).Error; loadErr != nil {
			return BootstrapProjectResult{}, err
		}
		if record.RequestDigest != digest {
			return BootstrapProjectResult{}, ErrIdempotencyConflict
		}
		if loadErr := loadBootstrapResult(s.repository.DB().WithContext(ctx), userID, record, &result); loadErr != nil {
			return BootstrapProjectResult{}, loadErr
		}
		result.Replayed = true
		return result, nil
	}
	return result, err
}

func loadBootstrapResult(tx *gorm.DB, userID uint, record CommerceIdempotencyRecord, target *BootstrapProjectResult) error {
	if record.ProductID == nil || record.SKUID == nil || record.ProjectID == nil {
		return fmt.Errorf("invalid bootstrap idempotency record")
	}
	if err := tx.Where("id = ? AND user_id = ?", *record.ProductID, userID).First(&target.Product).Error; err != nil {
		return mapNotFound(err)
	}
	if err := tx.Where("id = ? AND user_id = ?", *record.SKUID, userID).First(&target.SKU).Error; err != nil {
		return mapNotFound(err)
	}
	if err := tx.Where("id = ? AND user_id = ?", *record.ProjectID, userID).First(&target.Project).Error; err != nil {
		return mapNotFound(err)
	}
	return nil
}

func ParseProductReport(raw string, allowedAssetIDs []uint) (ProductReport, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var report ProductReport
	if err := decoder.Decode(&report); err != nil {
		return ProductReport{}, fmt.Errorf("decode product report: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ProductReport{}, fmt.Errorf("decode product report: trailing JSON")
	}
	legacyFacts := report.CommonFacts == nil && report.SKUOverrides == nil
	if (legacyFacts && report.ObservedFacts == nil) || report.SellingPoints == nil || report.ForbiddenChanges == nil || report.BrandTone == nil || report.MissingFields == nil || report.RiskNotices == nil || report.SuggestedSections == nil {
		return ProductReport{}, fmt.Errorf("product report fields must not be null")
	}
	for name, values := range map[string][]string{
		"selling_points": report.SellingPoints, "forbidden_changes": report.ForbiddenChanges,
		"missing_fields": report.MissingFields, "risk_notices": report.RiskNotices, "suggested_sections": report.SuggestedSections,
	} {
		for index, value := range values {
			if strings.TrimSpace(value) == "" {
				return ProductReport{}, fmt.Errorf("%s[%d] must not be empty", name, index)
			}
			values[index] = strings.TrimSpace(value)
		}
	}
	report.BrandTone.Description = strings.TrimSpace(report.BrandTone.Description)
	allowed := make(map[uint]struct{}, len(allowedAssetIDs))
	for _, id := range allowedAssetIDs {
		allowed[id] = struct{}{}
	}
	requiredMissingFields := []string{"price", "capacity", "material", "certification", "efficacy"}
	prohibited := make(map[string]struct{}, len(requiredMissingFields))
	for _, field := range requiredMissingFields {
		prohibited[field] = struct{}{}
	}
	missing := make(map[string]struct{}, len(report.MissingFields))
	for _, field := range report.MissingFields {
		missing[strings.ToLower(strings.TrimSpace(field))] = struct{}{}
	}
	allFacts := append([]ObservedFact(nil), report.ObservedFacts...)
	allFacts = append(allFacts, report.CommonFacts...)
	for _, facts := range report.SKUOverrides {
		allFacts = append(allFacts, facts...)
	}
	for _, fact := range allFacts {
		field := strings.ToLower(strings.TrimSpace(fact.Field))
		if field == "" || fact.Confidence < 0 || fact.Confidence > 1 || len(fact.SourceAssetIDs) == 0 {
			return ProductReport{}, fmt.Errorf("invalid observed fact %q", fact.Field)
		}
		if _, blocked := prohibited[field]; blocked {
			return ProductReport{}, fmt.Errorf("unverified fact %q is prohibited", field)
		}
		for _, id := range fact.SourceAssetIDs {
			if _, ok := allowed[id]; !ok {
				return ProductReport{}, fmt.Errorf("unknown source asset %d", id)
			}
		}
	}
	for _, field := range requiredMissingFields {
		if _, ok := missing[field]; ok {
			continue
		}
		report.MissingFields = append(report.MissingFields, field)
		missing[field] = struct{}{}
	}
	sections := map[string]struct{}{"hero": {}, "selling_points": {}, "material": {}, "detail": {}, "usage": {}, "specification": {}, "closing": {}}
	for _, section := range report.SuggestedSections {
		if _, ok := sections[section]; !ok {
			return ProductReport{}, fmt.Errorf("unsupported suggested section %q", section)
		}
	}
	return report, nil
}

func (s *Service) AnalyzeProduct(ctx context.Context, userID, projectID uint, key string, input AnalyzeProductInput) (AnalyzeProductResult, error) {
	analyzer := s.VisionAnalyzer()
	if analyzer == nil {
		return AnalyzeProductResult{}, ErrVisionNotConfigured
	}
	if availability, ok := analyzer.(CommerceVisionAvailability); ok {
		configured, err := availability.CommerceVisionConfigured(ctx)
		if err != nil {
			return AnalyzeProductResult{}, err
		}
		if !configured {
			return AnalyzeProductResult{}, ErrVisionNotConfigured
		}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return AnalyzeProductResult{}, invalidField("idempotency_key", "Idempotency-Key is required")
	}
	input.UserRequirements = strings.TrimSpace(input.UserRequirements)
	input.SourceAssetIDs = uniqueSortedIDs(input.SourceAssetIDs)
	if len(input.SourceAssetIDs) == 0 {
		return AnalyzeProductResult{}, invalidField("source_asset_ids", "at least one product asset is required")
	}
	digest, err := productAnalysisRequestDigest(projectID, input.SourceAssetIDs, input.UserRequirements)
	if err != nil {
		return AnalyzeProductResult{}, err
	}
	scope := "product_analysis"
	var result AnalyzeProductResult
	err = runProjectMutationTransaction(ctx, s.repository.DB(), func(tx *gorm.DB) error {
		if _, err := lockWritableProjectTx(ctx, tx, userID, projectID); err != nil {
			return err
		}
		var record CommerceIdempotencyRecord
		err := tx.Where("user_id = ? AND scope = ? AND idempotency_key = ?", userID, scope, key).First(&record).Error
		if err == nil {
			if record.RequestDigest != digest {
				return ErrIdempotencyConflict
			}
			if record.CreativeSpecID == nil || record.JobID == nil {
				return fmt.Errorf("invalid analysis idempotency record")
			}
			if err := tx.Where("id = ? AND user_id = ?", *record.CreativeSpecID, userID).First(&result.CreativeSpec).Error; err != nil {
				return mapNotFound(err)
			}
			if err := tx.Where("id = ? AND user_id = ?", *record.JobID, userID).First(&result.Job).Error; err != nil {
				return mapNotFound(err)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var assets []CommerceAsset
		if err := tx.Where("id IN ? AND user_id = ? AND project_id = ? AND role IN ?", input.SourceAssetIDs, userID, projectID, productAnalysisAssetRoles).Find(&assets).Error; err != nil {
			return err
		}
		if len(assets) != len(input.SourceAssetIDs) {
			return ErrOwnershipMismatch
		}
		assetJSON, _ := EncodeJSON(input.SourceAssetIDs)
		spec := CommerceCreativeSpec{UserID: userID, ProjectID: projectID, Version: 1, Source: "vision", Status: "analyzing", ProductFactsJSON: "{}", CommonFactsJSON: "[]", SKUOverridesJSON: "{}", SellingPointsJSON: "[]", ForbiddenChangesJSON: "[]", BrandToneJSON: "{}", ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: assetJSON, ObservedFactsJSON: "[]", UserOverridesJSON: "{}", MissingFieldsJSON: "[]", SuggestedSectionsJSON: "[]", AnalysisRequestHash: digest}
		if err := tx.Create(&spec).Error; err != nil {
			return err
		}
		payload, _ := EncodeJSON(productAnalysisJobPayload{SourceAssetIDs: input.SourceAssetIDs, UserRequirements: input.UserRequirements, AnalysisRequestHash: digest, CreativeSpecVersion: spec.Version, ProjectID: projectID, CreativeSpecID: spec.ID})
		specID := spec.ID
		job := CommerceJob{UserID: userID, ProjectID: projectID, SubjectID: &specID, SubjectType: CommerceSubjectCreativeSpec, Kind: CommerceJobKindProductAnalysis, Status: CommerceJobQueued, IdempotencyKey: "product-analysis:" + key, MaxAttempts: 3, PayloadJSON: payload}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		record = CommerceIdempotencyRecord{UserID: userID, Scope: scope, IdempotencyKey: key, RequestDigest: digest, ProjectID: &projectID, CreativeSpecID: &spec.ID, JobID: &job.ID}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		result = AnalyzeProductResult{CreativeSpec: spec, Job: job}
		return nil
	})
	if err != nil && isUniqueConstraintError(err) {
		var record CommerceIdempotencyRecord
		if loadErr := s.repository.DB().WithContext(ctx).Where("user_id = ? AND scope = ? AND idempotency_key = ?", userID, scope, key).First(&record).Error; loadErr != nil {
			return AnalyzeProductResult{}, err
		}
		if record.RequestDigest != digest {
			return AnalyzeProductResult{}, ErrIdempotencyConflict
		}
		if record.CreativeSpecID == nil || record.JobID == nil {
			return AnalyzeProductResult{}, fmt.Errorf("invalid analysis idempotency record")
		}
		if loadErr := s.repository.DB().WithContext(ctx).Where("id = ? AND user_id = ?", *record.CreativeSpecID, userID).First(&result.CreativeSpec).Error; loadErr != nil {
			return AnalyzeProductResult{}, mapNotFound(loadErr)
		}
		if loadErr := s.repository.DB().WithContext(ctx).Where("id = ? AND user_id = ?", *record.JobID, userID).First(&result.Job).Error; loadErr != nil {
			return AnalyzeProductResult{}, mapNotFound(loadErr)
		}
		return result, nil
	}
	return result, err
}

type ProductAnalysisJobHandler struct {
	Service  *Service
	Analyzer CommerceVisionAnalyzer
}

func NewProductAnalysisJobHandler(service *Service, analyzer CommerceVisionAnalyzer) *ProductAnalysisJobHandler {
	return &ProductAnalysisJobHandler{Service: service, Analyzer: analyzer}
}

func (*ProductAnalysisJobHandler) Kind() JobKind { return CommerceJobKindProductAnalysis }

func (h *ProductAnalysisJobHandler) Handle(ctx context.Context, snapshot JobSnapshot) (JobResult, error) {
	job := snapshot.Job
	if h == nil || h.Service == nil || h.Analyzer == nil {
		return JobResult{}, NewJobError("commerce_vision_not_configured", "commerce vision analyzer is unavailable", true)
	}
	if job.SubjectType != CommerceSubjectCreativeSpec || job.SubjectID == nil {
		return JobResult{}, NewJobError("invalid_analysis_subject", "product analysis subject is invalid", false)
	}
	spec, err := h.Service.repository.GetProjectCreativeSpec(ctx, job.UserID, job.ProjectID, *job.SubjectID)
	if err != nil {
		return JobResult{}, NewJobError("analysis_subject_not_found", "product analysis subject is unavailable", false)
	}
	input, err := decodeProductAnalysisJobPayload(job.PayloadJSON)
	if err != nil || input.ProjectID != job.ProjectID || input.CreativeSpecID != spec.ID || !productAnalysisPayloadAssetsMatchSpec(input.SourceAssetIDs, spec.SourceAssetIDsJSON) {
		h.markFailed(ctx, spec, "invalid analysis payload")
		return JobResult{}, NewJobError("invalid_analysis_payload", "product analysis payload is invalid", false)
	}
	if spec.Status != "analyzing" || input.CreativeSpecVersion != spec.Version {
		return JobResult{}, NewJobError("analysis_superseded", "product analysis result was superseded", false)
	}
	requestHash, err := productAnalysisRequestDigest(input.ProjectID, input.SourceAssetIDs, input.UserRequirements)
	if err != nil || requestHash != input.AnalysisRequestHash || requestHash != spec.AnalysisRequestHash {
		h.markFailed(ctx, spec, "invalid analysis payload")
		return JobResult{}, NewJobError("invalid_analysis_payload", "product analysis payload is invalid", false)
	}
	var analysisAssets []CommerceAsset
	if err := h.Service.repository.DB().WithContext(ctx).
		Where("id IN ? AND user_id = ? AND project_id = ? AND role IN ?", input.SourceAssetIDs, job.UserID, job.ProjectID, productAnalysisAssetRoles).
		Find(&analysisAssets).Error; err != nil {
		return JobResult{}, err
	}
	if len(analysisAssets) != len(input.SourceAssetIDs) {
		h.markFailed(ctx, spec, "analysis assets unavailable")
		return JobResult{}, NewJobError("analysis_assets_unavailable", "product analysis assets are unavailable", false)
	}
	contexts := make([]ProductAnalysisAssetContext, 0, len(analysisAssets))
	for _, asset := range analysisAssets {
		contexts = append(contexts, ProductAnalysisAssetContext{AssetID: asset.ID, SKUID: asset.SKUID, Shared: asset.SKUID == nil})
	}
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].AssetID < contexts[j].AssetID })
	raw, err := h.Analyzer.AnalyzeProduct(ctx, ProductAnalysisRequest{JobID: job.ID, UserID: job.UserID, ProjectID: job.ProjectID, CreativeSpecID: spec.ID, SourceAssetIDs: input.SourceAssetIDs, UserRequirements: input.UserRequirements, AssetContexts: contexts})
	if err != nil {
		retryable := false
		var classified interface{ Retryable() bool }
		if errors.As(err, &classified) {
			retryable = classified.Retryable()
		}
		if retryable && (job.MaxAttempts <= 0 || job.AttemptCount < job.MaxAttempts) {
			_ = h.Service.repository.DB().WithContext(ctx).Model(&CommerceCreativeSpec{}).
				Where("id = ? AND user_id = ? AND status = ? AND version = ? AND analysis_request_hash = ?", spec.ID, spec.UserID, "analyzing", spec.Version, spec.AnalysisRequestHash).
				Update("analysis_error", "analysis temporarily failed").Error
		} else {
			h.markFailed(ctx, spec, "analysis failed")
		}
		return JobResult{}, NewJobError("analysis_failed", "product analysis failed", retryable)
	}
	report, err := ParseProductReport(raw, input.SourceAssetIDs)
	if err != nil {
		log.Printf("commerce_product_analysis_invalid_response job_id=%d project_id=%d error=%v", job.ID, job.ProjectID, err)
		h.markFailed(ctx, spec, "invalid analysis response")
		return JobResult{}, NewJobError("invalid_analysis_response", "product analysis response was invalid", false)
	}
	if report.CommonFacts == nil && report.SKUOverrides == nil {
		report.CommonFacts, report.SKUOverrides = partitionLegacyFactsByAssetContext(report.ObservedFacts, contexts)
	}
	var analysisProject CommerceProject
	if err := h.Service.repository.DB().WithContext(ctx).Where("id = ? AND user_id = ?", job.ProjectID, job.UserID).First(&analysisProject).Error; err != nil {
		return JobResult{}, err
	}
	var projectSKUs []CommerceSKU
	if err := h.Service.repository.DB().WithContext(ctx).Where("product_id = ? AND user_id = ?", analysisProject.ProductID, job.UserID).Find(&projectSKUs).Error; err != nil {
		return JobResult{}, err
	}
	validSKUs := map[uint]struct{}{}
	for _, sku := range projectSKUs {
		validSKUs[sku.ID] = struct{}{}
	}
	if err := validateReportAssetScopes(report, contexts, validSKUs); err != nil {
		h.markFailed(ctx, spec, "invalid analysis response")
		return JobResult{}, NewJobError("invalid_analysis_response", "product analysis response was invalid", false)
	}
	observed, _ := EncodeJSON(report.ObservedFacts)
	commonFacts, _ := EncodeJSON(report.CommonFacts)
	skuOverrides, _ := EncodeJSON(report.SKUOverrides)
	sellingPoints, _ := EncodeJSON(report.SellingPoints)
	forbiddenChanges, _ := EncodeJSON(report.ForbiddenChanges)
	brandTone, _ := EncodeJSON(report.BrandTone)
	missing, _ := EncodeJSON(report.MissingFields)
	risks, _ := EncodeJSON(report.RiskNotices)
	sections, _ := EncodeJSON(report.SuggestedSections)
	result := h.Service.repository.DB().WithContext(ctx).Model(&CommerceCreativeSpec{}).
		Where("id = ? AND user_id = ? AND status = ? AND version = ? AND analysis_request_hash = ?", spec.ID, job.UserID, "analyzing", spec.Version, spec.AnalysisRequestHash).
		Updates(map[string]any{"status": "draft", "observed_facts_json": observed, "common_facts_json": commonFacts, "sku_overrides_json": skuOverrides, "selling_points_json": sellingPoints, "forbidden_changes_json": forbiddenChanges, "brand_tone_json": brandTone, "missing_fields_json": missing, "risk_notices_json": risks, "suggested_sections_json": sections, "analysis_error": ""})
	if result.Error != nil {
		return JobResult{}, result.Error
	}
	if result.RowsAffected != 1 {
		return JobResult{}, NewJobError("analysis_superseded", "product analysis result was superseded", false)
	}
	return JobResult{MetadataJSON: fmt.Sprintf(`{"creative_spec_id":%d}`, spec.ID)}, nil
}

func partitionLegacyFactsByAssetContext(facts []ObservedFact, contexts []ProductAnalysisAssetContext) ([]ObservedFact, map[string][]ObservedFact) {
	assetSKU := map[uint]*uint{}
	for _, context := range contexts {
		assetSKU[context.AssetID] = context.SKUID
	}
	common := []ObservedFact{}
	overrides := map[string][]ObservedFact{}
	for _, fact := range facts {
		var skuID uint
		skuSpecific, mixed := false, false
		for _, assetID := range fact.SourceAssetIDs {
			owner := assetSKU[assetID]
			if owner == nil {
				continue
			}
			if !skuSpecific {
				skuID, skuSpecific = *owner, true
			} else if skuID != *owner {
				mixed = true
			}
		}
		if !skuSpecific || mixed {
			common = append(common, fact)
			continue
		}
		key := fmt.Sprintf("%d", skuID)
		overrides[key] = append(overrides[key], fact)
	}
	return common, overrides
}

func validateReportAssetScopes(report ProductReport, contexts []ProductAnalysisAssetContext, validSKUs map[uint]struct{}) error {
	owners := map[uint]*uint{}
	for _, context := range contexts {
		owners[context.AssetID] = context.SKUID
	}
	for _, fact := range report.CommonFacts {
		for _, id := range fact.SourceAssetIDs {
			if owners[id] != nil {
				return fmt.Errorf("SKU-specific fact cannot be common")
			}
		}
	}
	for key, facts := range report.SKUOverrides {
		skuID64, err := strconv.ParseUint(key, 10, 64)
		if err != nil || skuID64 == 0 {
			return fmt.Errorf("invalid SKU override key")
		}
		if _, ok := validSKUs[uint(skuID64)]; !ok {
			return fmt.Errorf("SKU override does not belong to project product")
		}
		for _, fact := range facts {
			hasOwnedSource := false
			for _, id := range fact.SourceAssetIDs {
				owner := owners[id]
				if owner != nil && uint64(*owner) == skuID64 {
					hasOwnedSource = true
				}
				if owner != nil && uint64(*owner) != skuID64 {
					return fmt.Errorf("SKU override source mismatch")
				}
			}
			if !hasOwnedSource {
				return fmt.Errorf("SKU override requires SKU-specific source")
			}
		}
	}
	return nil
}

func decodeProductAnalysisJobPayload(raw string) (productAnalysisJobPayload, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload productAnalysisJobPayload
	if err := decoder.Decode(&payload); err != nil {
		return productAnalysisJobPayload{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return productAnalysisJobPayload{}, fmt.Errorf("product analysis payload has trailing JSON")
	}
	payload.UserRequirements = strings.TrimSpace(payload.UserRequirements)
	if payload.AnalysisRequestHash == "" || payload.CreativeSpecVersion <= 0 || payload.ProjectID == 0 || payload.CreativeSpecID == 0 || len(payload.SourceAssetIDs) == 0 {
		return productAnalysisJobPayload{}, fmt.Errorf("product analysis payload is incomplete")
	}
	normalized := uniqueSortedIDs(payload.SourceAssetIDs)
	if len(normalized) != len(payload.SourceAssetIDs) {
		return productAnalysisJobPayload{}, fmt.Errorf("product analysis asset IDs are invalid")
	}
	for index := range normalized {
		if normalized[index] != payload.SourceAssetIDs[index] {
			return productAnalysisJobPayload{}, fmt.Errorf("product analysis asset IDs are not canonical")
		}
	}
	return payload, nil
}

func productAnalysisPayloadAssetsMatchSpec(payloadIDs []uint, rawSpecIDs string) bool {
	var specIDs []uint
	if err := json.Unmarshal([]byte(rawSpecIDs), &specIDs); err != nil || len(specIDs) != len(payloadIDs) {
		return false
	}
	for index := range payloadIDs {
		if payloadIDs[index] != specIDs[index] {
			return false
		}
	}
	return true
}

type productAnalysisTerminalHook struct{}

func NewProductAnalysisTerminalHook() JobTerminalHook { return productAnalysisTerminalHook{} }

func (productAnalysisTerminalHook) OnJobTerminalTx(ctx context.Context, tx *gorm.DB, job CommerceJob, _ time.Time) error {
	if job.Status != CommerceJobFailed || job.ErrorCode != "max_attempts_exceeded" || job.DeadLetteredAt == nil || job.SubjectType != CommerceSubjectCreativeSpec || job.SubjectID == nil {
		return nil
	}
	payload, err := decodeProductAnalysisJobPayload(job.PayloadJSON)
	if err != nil || payload.ProjectID != job.ProjectID || payload.CreativeSpecID != *job.SubjectID {
		return nil
	}
	return tx.WithContext(ctx).Model(&CommerceCreativeSpec{}).
		Where("id = ? AND user_id = ? AND project_id = ? AND status = ? AND version = ? AND analysis_request_hash = ?", *job.SubjectID, job.UserID, job.ProjectID, "analyzing", payload.CreativeSpecVersion, payload.AnalysisRequestHash).
		Updates(map[string]any{"status": "analysis_failed", "analysis_error": "analysis retries exhausted"}).Error
}

func (h *ProductAnalysisJobHandler) markFailed(ctx context.Context, spec CommerceCreativeSpec, message string) {
	_ = h.Service.repository.DB().WithContext(ctx).Model(&CommerceCreativeSpec{}).
		Where("id = ? AND user_id = ? AND status = ? AND version = ? AND analysis_request_hash = ?", spec.ID, spec.UserID, "analyzing", spec.Version, spec.AnalysisRequestHash).
		Updates(map[string]any{"status": "analysis_failed", "analysis_error": message}).Error
}

func canonicalDigest(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func productAnalysisRequestDigest(projectID uint, sourceAssetIDs []uint, userRequirements string) (string, error) {
	input := AnalyzeProductInput{
		SourceAssetIDs:   uniqueSortedIDs(sourceAssetIDs),
		UserRequirements: strings.TrimSpace(userRequirements),
	}
	return canonicalDigest(struct {
		ProjectID uint
		Input     AnalyzeProductInput
	}{ProjectID: projectID, Input: input})
}

func uniqueSortedIDs(values []uint) []uint {
	seen := map[uint]struct{}{}
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
