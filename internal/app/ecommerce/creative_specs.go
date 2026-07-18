package ecommerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gorm.io/gorm"
)

var ErrCreativeSpecNotConfirmed = errors.New("creative spec is not confirmed")

type ManualCreativeSpecInput struct {
	ProductFacts, SellingPoints, ForbiddenChanges []byte
	BrandTone, CopyBlocks, RiskNotices            []byte
}

type PatchCreativeSpecInput struct {
	ExpectedVersion                               int
	ProductFacts, SellingPoints, ForbiddenChanges *[]byte
	BrandTone, CopyBlocks, RiskNotices            *[]byte
	UserOverrides                                 *[]byte
}

func (s *Service) CreateManualCreativeSpec(ctx context.Context, userID, projectID uint, input ManualCreativeSpecInput) (CommerceCreativeSpec, error) {
	if _, err := s.ValidateProjectWritable(ctx, userID, projectID); err != nil {
		return CommerceCreativeSpec{}, err
	}
	fields, err := normalizeManualCreativeSpecInput(input)
	if err != nil {
		return CommerceCreativeSpec{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	spec := CommerceCreativeSpec{
		UserID:                userID,
		ProjectID:             projectID,
		Version:               1,
		Source:                "manual",
		Status:                "draft",
		ProductFactsJSON:      fields.productFacts,
		CommonFactsJSON:       fields.productFacts,
		SKUOverridesJSON:      "{}",
		SellingPointsJSON:     fields.sellingPoints,
		ForbiddenChangesJSON:  fields.forbiddenChanges,
		BrandToneJSON:         fields.brandTone,
		ShotPlanJSON:          "[]",
		CopyBlocksJSON:        fields.copyBlocks,
		RiskNoticesJSON:       fields.riskNotices,
		SourceAssetIDsJSON:    "[]",
		ObservedFactsJSON:     "[]",
		UserOverridesJSON:     "{}",
		MissingFieldsJSON:     "[]",
		SuggestedSectionsJSON: "[]",
	}
	if err := s.repository.CreateCreativeSpec(ctx, &spec); err != nil {
		return CommerceCreativeSpec{}, err
	}
	return spec, nil
}

func (s *Service) GetCreativeSpec(ctx context.Context, userID, creativeSpecID uint) (CommerceCreativeSpec, error) {
	return s.repository.GetCreativeSpec(ctx, userID, creativeSpecID)
}

func (s *Service) GetLatestCreativeSpec(ctx context.Context, userID, projectID uint) (CommerceCreativeSpec, error) {
	project, err := s.repository.GetProject(ctx, userID, projectID)
	if err != nil {
		return CommerceCreativeSpec{}, err
	}
	if project.Status == "deletion_requested" || project.DeletionRequestedAt != nil {
		return CommerceCreativeSpec{}, ErrProjectDeletionRequested
	}
	return s.repository.GetLatestProjectCreativeSpec(ctx, userID, projectID)
}

func (s *Service) PatchCreativeSpec(ctx context.Context, userID, creativeSpecID uint, input PatchCreativeSpecInput) (CommerceCreativeSpec, error) {
	if input.ExpectedVersion <= 0 {
		return CommerceCreativeSpec{}, invalidField("expected_version", "positive expected_version is required")
	}
	var updated CommerceCreativeSpec
	err := s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := s.repository.WithDB(tx)
		spec, err := repo.GetCreativeSpec(ctx, userID, creativeSpecID)
		if err != nil {
			return err
		}
		if spec.Version != input.ExpectedVersion {
			return ErrVersionConflict
		}
		if _, err := repo.GetProject(ctx, userID, spec.ProjectID); err != nil {
			return err
		}
		if spec.Source == "vision" && input.ProductFacts != nil {
			return invalidField("product_facts", "vision product facts are read-only; use user_overrides")
		}
		if err := applyCreativeSpecPatch(&spec, input); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		spec.Version++
		spec.Status = "draft"
		spec.LockedAt = nil
		result := tx.WithContext(ctx).Model(&CommerceCreativeSpec{}).
			Where("id = ? AND user_id = ? AND version = ?", spec.ID, userID, input.ExpectedVersion).
			Where("EXISTS (?)", tx.Model(&CommerceProject{}).
				Select("1").
				Where("commerce_projects.id = ? AND commerce_projects.user_id = ? AND commerce_projects.status <> ? AND commerce_projects.deletion_requested_at IS NULL", spec.ProjectID, userID, "deletion_requested")).
			Select("version", "status", "product_facts_json", "selling_points_json", "forbidden_changes_json", "brand_tone_json", "copy_blocks_json", "risk_notices_json", "user_overrides_json", "locked_at", "updated_at").
			Updates(&spec)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			project, projectErr := repo.GetProject(ctx, userID, spec.ProjectID)
			if projectErr != nil {
				return projectErr
			}
			if project.Status == "deletion_requested" || project.DeletionRequestedAt != nil {
				return ErrProjectDeletionRequested
			}
			return ErrVersionConflict
		}
		if err := tx.WithContext(ctx).Model(&CommerceProject{}).
			Where("id = ? AND user_id = ? AND active_creative_spec_id = ?", spec.ProjectID, userID, spec.ID).
			Update("active_creative_spec_id", nil).Error; err != nil {
			return err
		}
		updated = spec
		return nil
	})
	if err != nil {
		return CommerceCreativeSpec{}, err
	}
	return updated, nil
}

func (s *Service) ConfirmCreativeSpec(ctx context.Context, userID, creativeSpecID uint) (CommerceCreativeSpec, error) {
	spec, err := s.repository.GetCreativeSpec(ctx, userID, creativeSpecID)
	if err != nil {
		return CommerceCreativeSpec{}, err
	}
	project, err := s.repository.GetProject(ctx, userID, spec.ProjectID)
	if err != nil {
		return CommerceCreativeSpec{}, err
	}
	if project.Status == "deletion_requested" || project.DeletionRequestedAt != nil {
		return CommerceCreativeSpec{}, ErrProjectDeletionRequested
	}
	facts, err := mergedCreativeSpecFacts(spec)
	if err != nil {
		return CommerceCreativeSpec{}, fmt.Errorf("normalize product facts: %w", err)
	}
	if !hasJSONContent(facts) {
		return CommerceCreativeSpec{}, invalidField("product_facts", "product facts are required")
	}
	var merged map[string]any
	if err := json.Unmarshal([]byte(facts), &merged); err != nil {
		return CommerceCreativeSpec{}, fmt.Errorf("decode merged product facts: %w", err)
	}
	if !hasFactValue(merged["name"]) {
		return CommerceCreativeSpec{}, invalidField("name", "product name is required")
	}
	var missing []string
	if err := json.Unmarshal([]byte(defaultJSON(spec.MissingFieldsJSON, "[]")), &missing); err != nil {
		return CommerceCreativeSpec{}, fmt.Errorf("decode missing fields: %w", err)
	}
	for _, field := range missing {
		field = strings.TrimSpace(field)
		if field != "" && !hasFactValue(merged[field]) {
			return CommerceCreativeSpec{}, invalidField("missing_fields", fmt.Sprintf("%s must be resolved before confirm", field))
		}
	}
	now := s.now().UTC()
	skuContextSHA, err := creativeSpecSKUContextDigest(ctx, s.repository.DB(), project)
	if err != nil {
		return CommerceCreativeSpec{}, err
	}
	err = s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"status": "confirmed", "product_facts_json": facts, "sku_context_sha256": skuContextSHA,
			"locked_at": now, "updated_at": now,
		}
		if spec.Source != "vision" {
			updates["common_facts_json"] = facts
		}
		result := tx.WithContext(ctx).Model(&CommerceCreativeSpec{}).
			Where("id = ? AND user_id = ? AND version = ?", spec.ID, userID, spec.Version).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionConflict
		}
		projectResult := tx.WithContext(ctx).Model(&CommerceProject{}).
			Where("id = ? AND user_id = ? AND status <> ? AND deletion_requested_at IS NULL", project.ID, userID, "deletion_requested").
			Update("active_creative_spec_id", spec.ID)
		if projectResult.Error != nil {
			return projectResult.Error
		}
		if projectResult.RowsAffected != 1 {
			return ErrProjectDeletionRequested
		}
		spec.ProductFactsJSON = facts
		if spec.Source != "vision" {
			spec.CommonFactsJSON = facts
		}
		spec.SKUContextSHA256 = skuContextSHA
		spec.Status = "confirmed"
		spec.LockedAt = &now
		return nil
	})
	if err != nil {
		return CommerceCreativeSpec{}, err
	}
	return spec, nil
}

func hasFactValue(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func mergedCreativeSpecFacts(spec CommerceCreativeSpec) (string, error) {
	merged := make(map[string]any)
	if spec.Source == "vision" {
		var observed []ObservedFact
		if err := json.Unmarshal([]byte(defaultJSON(spec.ObservedFactsJSON, "[]")), &observed); err != nil {
			return "", fmt.Errorf("decode observed facts: %w", err)
		}
		for _, fact := range observed {
			field := strings.TrimSpace(fact.Field)
			if field != "" {
				merged[field] = fact.Value
			}
		}
	} else if err := json.Unmarshal([]byte(defaultJSON(spec.ProductFactsJSON, "{}")), &merged); err != nil {
		return "", fmt.Errorf("decode product facts: %w", err)
	}
	if merged == nil {
		merged = make(map[string]any)
	}
	var overrides map[string]any
	if err := json.Unmarshal([]byte(defaultJSON(spec.UserOverridesJSON, "{}")), &overrides); err != nil {
		return "", fmt.Errorf("decode user overrides: %w", err)
	}
	for field, value := range overrides {
		if strings.TrimSpace(field) != "" {
			merged[field] = value
		}
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *Service) BuildConfirmedCreativeSpecSnapshot(ctx context.Context, userID, projectID, creativeSpecID uint) (CreativeSpecSnapshot, error) {
	spec, err := s.repository.GetProjectCreativeSpec(ctx, userID, projectID, creativeSpecID)
	if err != nil {
		return CreativeSpecSnapshot{}, err
	}
	if spec.Status != "confirmed" || spec.LockedAt == nil {
		return CreativeSpecSnapshot{}, ErrCreativeSpecNotConfirmed
	}
	snapshot := CreativeSpecSnapshot{
		ID:                   spec.ID,
		Version:              uint(spec.Version),
		Status:               spec.Status,
		ProductFactsJSON:     spec.ProductFactsJSON,
		CommonFactsJSON:      defaultJSON(spec.CommonFactsJSON, spec.ProductFactsJSON),
		SKUOverridesJSON:     defaultJSON(spec.SKUOverridesJSON, "{}"),
		SKUContextSHA256:     spec.SKUContextSHA256,
		SellingPointsJSON:    spec.SellingPointsJSON,
		ForbiddenChangesJSON: spec.ForbiddenChangesJSON,
		BrandToneJSON:        spec.BrandToneJSON,
		ShotPlanJSON:         spec.ShotPlanJSON,
		CopyBlocksJSON:       spec.CopyBlocksJSON,
		RiskNoticesJSON:      spec.RiskNoticesJSON,
		SourceAssetIDsJSON:   spec.SourceAssetIDsJSON,
	}
	if err := normalizeCreativeSpecSnapshot(&snapshot); err != nil {
		return CreativeSpecSnapshot{}, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return CreativeSpecSnapshot{}, fmt.Errorf("marshal creative spec snapshot: %w", err)
	}
	digest := sha256.Sum256(payload)
	snapshot.ContentSHA256 = hex.EncodeToString(digest[:])
	return snapshot, nil
}

type normalizedManualSpecFields struct {
	productFacts, sellingPoints, forbiddenChanges string
	brandTone, copyBlocks, riskNotices            string
}

func normalizeManualCreativeSpecInput(input ManualCreativeSpecInput) (normalizedManualSpecFields, error) {
	var fields normalizedManualSpecFields
	var err error
	if fields.productFacts, err = normalizeJSONObjectBytes(input.ProductFacts, "{}"); err != nil {
		return fields, fmt.Errorf("invalid product facts: %w", err)
	}
	if fields.sellingPoints, err = normalizeJSONBytes(input.SellingPoints, "[]"); err != nil {
		return fields, fmt.Errorf("invalid selling points: %w", err)
	}
	if fields.forbiddenChanges, err = normalizeJSONBytes(input.ForbiddenChanges, "[]"); err != nil {
		return fields, fmt.Errorf("invalid forbidden changes: %w", err)
	}
	if fields.brandTone, err = normalizeJSONBytes(input.BrandTone, "{}"); err != nil {
		return fields, fmt.Errorf("invalid brand tone: %w", err)
	}
	if fields.copyBlocks, err = normalizeJSONBytes(input.CopyBlocks, "[]"); err != nil {
		return fields, fmt.Errorf("invalid copy blocks: %w", err)
	}
	if fields.riskNotices, err = normalizeJSONBytes(input.RiskNotices, "[]"); err != nil {
		return fields, fmt.Errorf("invalid risk notices: %w", err)
	}
	return fields, nil
}

func applyCreativeSpecPatch(spec *CommerceCreativeSpec, input PatchCreativeSpecInput) error {
	if input.SellingPoints != nil {
		normalized, err := normalizeJSONStringArrayBytes(*input.SellingPoints)
		if err != nil {
			return fmt.Errorf("invalid selling points: %w", err)
		}
		spec.SellingPointsJSON = normalized
	}
	if input.ForbiddenChanges != nil {
		normalized, err := normalizeJSONStringArrayBytes(*input.ForbiddenChanges)
		if err != nil {
			return fmt.Errorf("invalid forbidden changes: %w", err)
		}
		spec.ForbiddenChangesJSON = normalized
	}
	if input.BrandTone != nil {
		normalized, err := normalizeBrandToneBytes(*input.BrandTone)
		if err != nil {
			return fmt.Errorf("invalid brand tone: %w", err)
		}
		spec.BrandToneJSON = normalized
	}
	for _, field := range []struct {
		input    *[]byte
		target   *string
		fallback string
		name     string
		object   bool
	}{
		{input.ProductFacts, &spec.ProductFactsJSON, "{}", "product facts", true},
		{input.CopyBlocks, &spec.CopyBlocksJSON, "[]", "copy blocks", false},
		{input.RiskNotices, &spec.RiskNoticesJSON, "[]", "risk notices", false},
		{input.UserOverrides, &spec.UserOverridesJSON, "{}", "user overrides", true},
	} {
		if field.input == nil {
			continue
		}
		var normalized string
		var err error
		if field.object {
			normalized, err = normalizeJSONObjectBytes(*field.input, field.fallback)
		} else {
			normalized, err = normalizeJSONBytes(*field.input, field.fallback)
		}
		if err != nil {
			return fmt.Errorf("invalid %s: %w", field.name, err)
		}
		*field.target = normalized
	}
	return nil
}

func normalizeJSONStringArrayBytes(raw []byte) (string, error) {
	normalized, err := normalizeJSONBytes(raw, "[]")
	if err != nil {
		return "", err
	}
	var values []string
	if err := json.Unmarshal([]byte(normalized), &values); err != nil || values == nil {
		return "", fmt.Errorf("must be a JSON string array")
	}
	for index, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("item %d must not be empty", index)
		}
		values[index] = value
	}
	encoded, err := json.Marshal(values)
	return string(encoded), err
}

func normalizeBrandToneBytes(raw []byte) (string, error) {
	normalized, err := normalizeJSONBytes(raw, "{}")
	if err != nil {
		return "", err
	}
	if normalized == "null" {
		return "", fmt.Errorf("must be an object with description")
	}
	decoder := json.NewDecoder(strings.NewReader(normalized))
	decoder.DisallowUnknownFields()
	var tone ProductBrandTone
	if err := decoder.Decode(&tone); err != nil {
		return "", fmt.Errorf("must be an object with description: %w", err)
	}
	tone.Description = strings.TrimSpace(tone.Description)
	encoded, err := json.Marshal(tone)
	return string(encoded), err
}

func normalizeJSONObjectBytes(raw []byte, fallback string) (string, error) {
	normalized, err := normalizeJSONBytes(raw, fallback)
	if err != nil {
		return "", err
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(normalized), &object); err != nil || object == nil {
		return "", fmt.Errorf("must be a JSON object")
	}
	return normalized, nil
}

func normalizeCreativeSpecSnapshot(snapshot *CreativeSpecSnapshot) error {
	for _, field := range []struct {
		target   *string
		fallback string
	}{
		{&snapshot.ProductFactsJSON, "{}"},
		{&snapshot.CommonFactsJSON, snapshot.ProductFactsJSON},
		{&snapshot.SKUOverridesJSON, "{}"},
		{&snapshot.SellingPointsJSON, "[]"},
		{&snapshot.ForbiddenChangesJSON, "[]"},
		{&snapshot.BrandToneJSON, "{}"},
		{&snapshot.ShotPlanJSON, "[]"},
		{&snapshot.CopyBlocksJSON, "[]"},
		{&snapshot.RiskNoticesJSON, "[]"},
		{&snapshot.SourceAssetIDsJSON, "[]"},
	} {
		normalized, err := normalizeJSON(*field.target, field.fallback)
		if err != nil {
			return fmt.Errorf("normalize creative spec JSON: %w", err)
		}
		*field.target = normalized
	}
	return nil
}

func creativeSpecSKUContextDigest(ctx context.Context, db *gorm.DB, project CommerceProject) (string, error) {
	var product CommerceProduct
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", project.ProductID, project.UserID).First(&product).Error; err != nil {
		return "", err
	}
	var skus []CommerceSKU
	if err := db.WithContext(ctx).Where("product_id = ? AND user_id = ?", project.ProductID, project.UserID).Order("id").Find(&skus).Error; err != nil {
		return "", err
	}
	var assets []CommerceAsset
	if err := db.WithContext(ctx).Where("project_id = ? AND user_id = ? AND sku_id IS NOT NULL", project.ID, project.UserID).Order("id").Find(&assets).Error; err != nil {
		return "", err
	}
	var dimensions []CommerceSKUDimension
	if err := db.WithContext(ctx).Where("product_id = ? AND user_id = ?", project.ProductID, project.UserID).Order("id").Find(&dimensions).Error; err != nil {
		return "", err
	}
	var values []CommerceSKUValue
	if err := db.WithContext(ctx).Where("product_id = ? AND user_id = ?", project.ProductID, project.UserID).Order("id").Find(&values).Error; err != nil {
		return "", err
	}
	var links []CommerceSKUValueLink
	if err := db.WithContext(ctx).Where("product_id = ? AND user_id = ?", project.ProductID, project.UserID).Order("id").Find(&links).Error; err != nil {
		return "", err
	}
	payload, err := EncodeJSON(struct {
		SKUVersion   int
		DefaultSKUID *uint
		SKUs         []CommerceSKU
		Assets       []CommerceAsset
		Dimensions   []CommerceSKUDimension
		Values       []CommerceSKUValue
		Links        []CommerceSKUValueLink
	}{product.SKUVersion, project.DefaultSKUID, skus, assets, dimensions, values, links})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:]), nil
}

func normalizeJSONBytes(raw []byte, fallback string) (string, error) {
	if len(raw) == 0 {
		return fallback, nil
	}
	return normalizeJSON(string(raw), fallback)
}

func normalizeJSON(raw, fallback string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("multiple JSON values")
		}
		return "", fmt.Errorf("trailing JSON data: %w", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func hasJSONContent(raw string) bool {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return false
	}
	switch typed := value.(type) {
	case nil:
		return false
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}
