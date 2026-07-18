package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidInput                      = errors.New("invalid commerce input")
	ErrInvalidPipeline                   = errors.New("invalid commerce pipeline")
	ErrVersionConflict                   = errors.New("commerce version conflict")
	ErrProjectDeletionRequested          = errors.New("project deletion requested")
	ErrReferenceAssetResolverUnavailable = errors.New("reference asset ownership resolver unavailable")
)

type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *FieldError) Unwrap() error {
	return ErrInvalidInput
}

func invalidField(field, message string) error {
	return &FieldError{Field: field, Message: message}
}

var allowedPipelines = map[string]struct{}{
	"general": {},
	"fashion": {},
	"mixed":   {},
}

type Service struct {
	repository             *Repository
	referenceAssetResolver ReferenceAssetOwnershipResolver
	batchRegistry          *Registry
	creditLedger           CreditLedger
	pricingSnapshots       PricingSnapshotStore
	pricingProvider        PricingSnapshotProvider
	visionAnalyzer         CommerceVisionAnalyzer
	visionMu               sync.RWMutex
	skuMutationMu          sync.Mutex
	now                    func() time.Time
}

type ReferenceAssetOwnershipResolver interface {
	OwnsReferenceAsset(context.Context, uint, uint) (bool, error)
}

type ReferenceAssetOwnershipResolverFunc func(context.Context, uint, uint) (bool, error)

func (f ReferenceAssetOwnershipResolverFunc) OwnsReferenceAsset(ctx context.Context, userID, assetID uint) (bool, error) {
	return f(ctx, userID, assetID)
}

func NewService(repository *Repository, resolvers ...ReferenceAssetOwnershipResolver) *Service {
	service := &Service{repository: repository, now: time.Now}
	if len(resolvers) > 0 {
		service.referenceAssetResolver = resolvers[0]
	}
	return service
}

type CreateBrandInput struct {
	Name, ColorPaletteJSON, FontsJSON, ForbiddenTermsJSON, VisualRulesJSON string
	LogoReferenceAssetID                                                   *uint
}

type PatchBrandInput struct {
	Name, ColorPaletteJSON, FontsJSON, ForbiddenTermsJSON, VisualRulesJSON *string
	LogoReferenceAssetID                                                   *uint
	ClearLogoReferenceAssetID                                              bool
}

type CreateProductInput struct {
	BrandID                               *uint
	Name, Category, SPUCode               string
	SellingPointsJSON, TargetChannelsJSON string
	Status                                string
}

type PatchProductInput struct {
	BrandID, CategoryID                                   *uint
	ClearBrandID                                          bool
	Name, Category, CategorySource, CategoryPath, SPUCode *string
	SellingPointsJSON, TargetChannelsJSON, Status         *string
}

type CreateSKUInput struct {
	Code, Color, Style, Size, AttributesJSON, Status string
}

type PatchSKUInput struct {
	Code, Color, Style, Size, AttributesJSON, Status *string
}

type CreateProjectInput struct {
	ProductID                                      uint
	BrandID, DefaultSKUID                          *uint
	Title, Pipeline, Status, DefaultChannelProfile string
}

type PatchProjectInput struct {
	ProductID                                      *uint
	BrandID, DefaultSKUID                          *uint
	ClearBrandID, ClearDefaultSKUID                bool
	Title, Pipeline, Status, DefaultChannelProfile *string
}

func (s *Service) CreateBrand(ctx context.Context, userID uint, input CreateBrandInput) (CommerceBrand, error) {
	if err := s.validateLogoReferenceAsset(ctx, userID, input.LogoReferenceAssetID); err != nil {
		return CommerceBrand{}, err
	}
	brand := CommerceBrand{
		UserID:               userID,
		LogoReferenceAssetID: input.LogoReferenceAssetID,
		Name:                 strings.TrimSpace(input.Name),
		ColorPaletteJSON:     defaultJSON(input.ColorPaletteJSON, "[]"),
		FontsJSON:            defaultJSON(input.FontsJSON, "[]"),
		ForbiddenTermsJSON:   defaultJSON(input.ForbiddenTermsJSON, "[]"),
		VisualRulesJSON:      defaultJSON(input.VisualRulesJSON, "{}"),
	}
	if brand.Name == "" {
		return CommerceBrand{}, invalidField("name", "brand name is required")
	}
	if err := s.repository.CreateBrand(ctx, &brand); err != nil {
		return CommerceBrand{}, err
	}
	return brand, nil
}

func (s *Service) ListBrands(ctx context.Context, userID uint) ([]CommerceBrand, error) {
	return s.repository.ListBrands(ctx, userID)
}

func (s *Service) GetBrand(ctx context.Context, userID, brandID uint) (CommerceBrand, error) {
	return s.repository.GetBrand(ctx, userID, brandID)
}

func (s *Service) PatchBrand(ctx context.Context, userID, brandID uint, input PatchBrandInput) (CommerceBrand, error) {
	brand, err := s.repository.GetBrand(ctx, userID, brandID)
	if err != nil {
		return CommerceBrand{}, err
	}
	if input.Name != nil {
		brand.Name = strings.TrimSpace(*input.Name)
		if brand.Name == "" {
			return CommerceBrand{}, invalidField("name", "brand name is required")
		}
	}
	assignString(&brand.ColorPaletteJSON, input.ColorPaletteJSON)
	assignString(&brand.FontsJSON, input.FontsJSON)
	assignString(&brand.ForbiddenTermsJSON, input.ForbiddenTermsJSON)
	assignString(&brand.VisualRulesJSON, input.VisualRulesJSON)
	if input.ClearLogoReferenceAssetID {
		brand.LogoReferenceAssetID = nil
	} else if input.LogoReferenceAssetID != nil {
		if err := s.validateLogoReferenceAsset(ctx, userID, input.LogoReferenceAssetID); err != nil {
			return CommerceBrand{}, err
		}
		brand.LogoReferenceAssetID = input.LogoReferenceAssetID
	}
	if err := s.repository.SaveBrand(ctx, userID, &brand); err != nil {
		return CommerceBrand{}, err
	}
	return s.repository.GetBrand(ctx, userID, brandID)
}

func (s *Service) validateLogoReferenceAsset(ctx context.Context, userID uint, assetID *uint) error {
	if assetID == nil {
		return nil
	}
	if *assetID == 0 {
		return invalidField("logo_reference_asset_id", "reference asset ID must be non-zero")
	}
	if s.referenceAssetResolver == nil {
		return ErrReferenceAssetResolverUnavailable
	}
	owned, err := s.referenceAssetResolver.OwnsReferenceAsset(ctx, userID, *assetID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrOwnershipMismatch
	}
	return nil
}

func (s *Service) CreateProduct(ctx context.Context, userID uint, input CreateProductInput) (CommerceProduct, error) {
	if input.BrandID != nil {
		if _, err := s.repository.GetBrand(ctx, userID, *input.BrandID); err != nil {
			return CommerceProduct{}, err
		}
	}
	product := CommerceProduct{
		UserID:             userID,
		BrandID:            input.BrandID,
		Name:               strings.TrimSpace(input.Name),
		Category:           strings.TrimSpace(input.Category),
		SPUCode:            strings.TrimSpace(input.SPUCode),
		SellingPointsJSON:  defaultJSON(input.SellingPointsJSON, "[]"),
		TargetChannelsJSON: defaultJSON(input.TargetChannelsJSON, "[]"),
		Status:             defaultString(input.Status, "active"),
	}
	if product.Name == "" {
		return CommerceProduct{}, invalidField("name", "product name is required")
	}
	if err := s.repository.CreateProduct(ctx, &product); err != nil {
		return CommerceProduct{}, err
	}
	return product, nil
}

func (s *Service) ListProducts(ctx context.Context, userID uint) ([]CommerceProduct, error) {
	return s.repository.ListProducts(ctx, userID)
}

func (s *Service) GetProduct(ctx context.Context, userID, productID uint) (CommerceProduct, error) {
	return s.repository.GetProduct(ctx, userID, productID)
}

func (s *Service) PatchProduct(ctx context.Context, userID, productID uint, input PatchProductInput) (CommerceProduct, error) {
	product, err := s.repository.GetProduct(ctx, userID, productID)
	if err != nil {
		return CommerceProduct{}, err
	}
	if input.ClearBrandID {
		product.BrandID = nil
	} else if input.BrandID != nil {
		if _, err := s.repository.GetBrand(ctx, userID, *input.BrandID); err != nil {
			return CommerceProduct{}, err
		}
		product.BrandID = input.BrandID
	}
	assignTrimmed(&product.Name, input.Name)
	assignTrimmed(&product.Category, input.Category)
	if input.CategoryID != nil {
		if input.CategorySource == nil {
			return CommerceProduct{}, ErrCategoryUnavailable
		}
		path, resolveErr := s.ResolveCategorySelection(ctx, userID, *input.CategoryID, *input.CategorySource)
		if resolveErr != nil {
			return CommerceProduct{}, resolveErr
		}
		product.CategoryID, product.CategorySource, product.CategoryPath, product.Category = input.CategoryID, strings.TrimSpace(*input.CategorySource), path, path
	}
	assignTrimmed(&product.SPUCode, input.SPUCode)
	assignString(&product.SellingPointsJSON, input.SellingPointsJSON)
	assignString(&product.TargetChannelsJSON, input.TargetChannelsJSON)
	assignTrimmed(&product.Status, input.Status)
	if product.Name == "" {
		return CommerceProduct{}, invalidField("name", "product name is required")
	}
	if err := s.repository.SaveProduct(ctx, userID, &product); err != nil {
		return CommerceProduct{}, err
	}
	return s.repository.GetProduct(ctx, userID, productID)
}

func (s *Service) CreateSKU(ctx context.Context, userID, productID uint, input CreateSKUInput) (CommerceSKU, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return CommerceSKU{}, invalidField("code", "SKU code is required")
	}
	sku := CommerceSKU{
		UserID:         userID,
		ProductID:      productID,
		Code:           code,
		Color:          strings.TrimSpace(input.Color),
		Style:          strings.TrimSpace(input.Style),
		Size:           strings.TrimSpace(input.Size),
		AttributesJSON: defaultJSON(input.AttributesJSON, "{}"),
		Status:         defaultString(input.Status, "active"),
	}
	err := s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var product CommerceProduct
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", productID, userID).First(&product).Error; err != nil {
			return mapNotFound(err)
		}
		if err := tx.Create(&sku).Error; err != nil {
			return err
		}
		return tx.Model(&CommerceProduct{}).Where("id=? AND user_id=?", productID, userID).UpdateColumn("sku_version", gorm.Expr("sku_version + 1")).Error
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return CommerceSKU{}, ErrConflict
		}
		return CommerceSKU{}, err
	}
	return sku, nil
}

func (s *Service) ListSKUs(ctx context.Context, userID, productID uint) ([]CommerceSKU, error) {
	config, err := s.GetSKUConfig(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	return config.SKUs, nil
}

func (s *Service) GetSKU(ctx context.Context, userID, skuID uint) (CommerceSKU, error) {
	return s.repository.GetSKU(ctx, userID, skuID)
}

func (s *Service) PatchSKU(ctx context.Context, userID, skuID uint, input PatchSKUInput) (CommerceSKU, error) {
	s.skuMutationMu.Lock()
	defer s.skuMutationMu.Unlock()
	current, err := s.repository.GetSKU(ctx, userID, skuID)
	if err != nil {
		return CommerceSKU{}, err
	}
	err = s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var product CommerceProduct
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", current.ProductID, userID).First(&product).Error; err != nil {
			return mapNotFound(err)
		}
		var projects []CommerceProject
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND product_id=?", userID, current.ProductID).Order("id").Find(&projects).Error; err != nil {
			return err
		}
		var sku CommerceSKU
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", skuID, userID).First(&sku).Error; err != nil {
			return mapNotFound(err)
		}
		if input.Code != nil {
			code := strings.TrimSpace(*input.Code)
			if code == "" {
				return invalidField("code", "SKU code is required")
			}
			sku.Code = code
		}
		assignTrimmed(&sku.Color, input.Color)
		assignTrimmed(&sku.Style, input.Style)
		assignTrimmed(&sku.Size, input.Size)
		assignString(&sku.AttributesJSON, input.AttributesJSON)
		assignTrimmed(&sku.Status, input.Status)
		if sku.Status == "disabled" {
			for _, project := range projects {
				if project.DefaultSKUID != nil && *project.DefaultSKUID == sku.ID {
					return ErrDefaultSKUDisable
				}
			}
		}
		result := tx.Model(&CommerceSKU{}).Where("id=? AND user_id=?", sku.ID, userID).Select("code", "color", "style", "size", "attributes_json", "status", "updated_at").Updates(&sku)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Model(&CommerceProduct{}).Where("id=? AND user_id=?", sku.ProductID, userID).UpdateColumn("sku_version", gorm.Expr("sku_version + 1")).Error
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return CommerceSKU{}, ErrConflict
		}
		return CommerceSKU{}, err
	}
	return s.repository.GetSKU(ctx, userID, skuID)
}

func (s *Service) CreateProject(ctx context.Context, userID uint, input CreateProjectInput) (CommerceProject, error) {
	s.skuMutationMu.Lock()
	defer s.skuMutationMu.Unlock()
	product, err := s.repository.GetProduct(ctx, userID, input.ProductID)
	if err != nil {
		return CommerceProject{}, err
	}
	pipeline := strings.TrimSpace(input.Pipeline)
	if pipeline == "" {
		pipeline = "general"
	}
	if !isAllowedPipeline(pipeline) {
		return CommerceProject{}, ErrInvalidPipeline
	}
	brandID := input.BrandID
	if brandID == nil {
		brandID = product.BrandID
	}
	if brandID != nil {
		if _, err := s.repository.GetBrand(ctx, userID, *brandID); err != nil {
			return CommerceProject{}, err
		}
	}
	if err := s.validateDefaultSKU(ctx, userID, input.ProductID, input.DefaultSKUID); err != nil {
		return CommerceProject{}, err
	}
	project := CommerceProject{
		UserID:                userID,
		ProductID:             input.ProductID,
		BrandID:               brandID,
		DefaultSKUID:          input.DefaultSKUID,
		Title:                 strings.TrimSpace(input.Title),
		Pipeline:              pipeline,
		Status:                defaultString(input.Status, "active"),
		DefaultChannelProfile: strings.TrimSpace(input.DefaultChannelProfile),
	}
	err = s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedProduct CommerceProduct
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", input.ProductID, userID).First(&lockedProduct).Error; err != nil {
			return mapNotFound(err)
		}
		if input.DefaultSKUID != nil {
			var sku CommerceSKU
			if err := tx.Where("id=? AND user_id=? AND product_id=? AND status=?", *input.DefaultSKUID, userID, input.ProductID, "active").First(&sku).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrDefaultSKUDisable
				}
				return err
			}
		}
		return tx.Create(&project).Error
	})
	if err != nil {
		return CommerceProject{}, err
	}
	return project, nil
}

func (s *Service) ListProjects(ctx context.Context, userID uint) ([]CommerceProject, error) {
	return s.repository.ListProjects(ctx, userID)
}

func (s *Service) GetProject(ctx context.Context, userID, projectID uint) (CommerceProject, error) {
	return s.repository.GetProject(ctx, userID, projectID)
}

func (s *Service) PatchProject(ctx context.Context, userID, projectID uint, input PatchProjectInput) (CommerceProject, error) {
	if input.DefaultSKUID != nil {
		s.skuMutationMu.Lock()
		defer s.skuMutationMu.Unlock()
	}
	project, err := s.repository.GetProject(ctx, userID, projectID)
	if err != nil {
		return CommerceProject{}, err
	}
	if project.Status == "deletion_requested" {
		return CommerceProject{}, ErrProjectDeletionRequested
	}
	updates := make(map[string]any)
	if input.ProductID != nil {
		if _, err := s.repository.GetProduct(ctx, userID, *input.ProductID); err != nil {
			return CommerceProject{}, err
		}
		project.ProductID = *input.ProductID
		updates["product_id"] = project.ProductID
	}
	if input.ClearBrandID {
		project.BrandID = nil
		updates["brand_id"] = nil
	} else if input.BrandID != nil {
		if _, err := s.repository.GetBrand(ctx, userID, *input.BrandID); err != nil {
			return CommerceProject{}, err
		}
		project.BrandID = input.BrandID
		updates["brand_id"] = *input.BrandID
	}
	if input.ClearDefaultSKUID {
		project.DefaultSKUID = nil
		updates["default_sku_id"] = nil
	} else if input.DefaultSKUID != nil {
		project.DefaultSKUID = input.DefaultSKUID
		updates["default_sku_id"] = *input.DefaultSKUID
	}
	if err := s.validateDefaultSKU(ctx, userID, project.ProductID, project.DefaultSKUID); err != nil {
		return CommerceProject{}, err
	}
	assignTrimmed(&project.Title, input.Title)
	if input.Title != nil {
		updates["title"] = project.Title
	}
	if input.Pipeline != nil {
		pipeline := strings.TrimSpace(*input.Pipeline)
		if !isAllowedPipeline(pipeline) {
			return CommerceProject{}, ErrInvalidPipeline
		}
		project.Pipeline = pipeline
		updates["pipeline"] = pipeline
	}
	assignTrimmed(&project.Status, input.Status)
	assignTrimmed(&project.DefaultChannelProfile, input.DefaultChannelProfile)
	if input.Status != nil {
		updates["status"] = project.Status
	}
	if input.DefaultChannelProfile != nil {
		updates["default_channel_profile"] = project.DefaultChannelProfile
	}
	updates["updated_at"] = s.now().UTC()
	if input.DefaultSKUID != nil {
		err := s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var lockedProduct CommerceProduct
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", project.ProductID, userID).First(&lockedProduct).Error; err != nil {
				return mapNotFound(err)
			}
			var lockedProject CommerceProject
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", projectID, userID).First(&lockedProject).Error; err != nil {
				return mapNotFound(err)
			}
			var activeSKU CommerceSKU
			if err := tx.Where("id=? AND user_id=? AND product_id=? AND status=?", *input.DefaultSKUID, userID, project.ProductID, "active").First(&activeSKU).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrDefaultSKUDisable
				}
				return err
			}
			result := tx.Model(&CommerceProject{}).Where("id=? AND user_id=? AND status<>? AND deletion_requested_at IS NULL", projectID, userID, "deletion_requested").Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrProjectDeletionRequested
			}
			return nil
		})
		if err != nil {
			return CommerceProject{}, err
		}
		return s.repository.GetProject(ctx, userID, projectID)
	}
	query := s.repository.DB().WithContext(ctx).Model(&CommerceProject{}).
		Where("id = ? AND user_id = ? AND status <> ? AND deletion_requested_at IS NULL", projectID, userID, "deletion_requested")
	result := query.Updates(updates)
	if result.Error != nil {
		return CommerceProject{}, result.Error
	}
	if result.RowsAffected != 1 {
		latest, loadErr := s.repository.GetProject(ctx, userID, projectID)
		if loadErr != nil {
			return CommerceProject{}, loadErr
		}
		if latest.Status == "deletion_requested" || latest.DeletionRequestedAt != nil {
			return CommerceProject{}, ErrProjectDeletionRequested
		}
		return CommerceProject{}, ErrNotFound
	}
	return s.repository.GetProject(ctx, userID, projectID)
}

func (s *Service) RequestProjectDeletion(ctx context.Context, userID, projectID uint) (CommerceProject, error) {
	var accepted CommerceProject
	err := runProjectMutationTransaction(ctx, s.repository.DB(), func(tx *gorm.DB) error {
		project, err := lockProjectRowTx(ctx, tx, userID, projectID)
		if err != nil {
			return err
		}
		if project.DeletedAt.Valid {
			return ErrNotFound
		}
		now := s.now().UTC()
		result := tx.WithContext(ctx).Model(&CommerceProject{}).
			Where("id = ? AND user_id = ?", projectID, userID).
			Updates(map[string]any{
				"status":                "deletion_requested",
				"deletion_requested_at": now,
				"updated_at":            now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		accepted = project
		accepted.Status = "deletion_requested"
		accepted.DeletionRequestedAt = &now
		accepted.UpdatedAt = now
		var jobs []CommerceJob
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND project_id = ? AND status IN ?", userID, projectID, []CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying, CommerceJobRunning}).
			Order("id ASC").Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			if job.Status == CommerceJobRunning {
				if err := tx.Model(&CommerceJob{}).Where("id = ? AND user_id = ? AND status = ?", job.ID, userID, CommerceJobRunning).
					Update("cancel_requested_at", now).Error; err != nil {
					return err
				}
				if job.GenerationItemID != nil {
					if err := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status = ?", *job.GenerationItemID, userID, CommerceItemRunning).
						Update("cancel_requested_at", now).Error; err != nil {
						return err
					}
				}
				continue
			}
			if job.GenerationItemID != nil {
				var item CommerceGenerationItem
				if err := tx.Where("id = ? AND user_id = ?", *job.GenerationItemID, userID).First(&item).Error; err != nil {
					return err
				}
				if item.Status == CommerceItemQueued || item.Status == CommerceItemRetrying {
					if s.creditLedger != nil {
						if err := s.creditLedger.ReleaseItemTx(ctx, tx, ReleaseCreditsRequest{
							UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
							ReservationID: item.ReservationID, GenerationItemID: item.ID, HeldCredits: item.ReservedCredits,
							Reason: "project_deleted", IdempotencyKey: fmt.Sprintf("commerce:item:%d:project-delete", item.ID),
						}); err != nil {
							return err
						}
					} else {
						if err := tx.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Update("released_credits", item.ReservedCredits).Error; err != nil {
							return err
						}
					}
					if err := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status IN ?", item.ID, userID, []CommerceItemStatus{CommerceItemQueued, CommerceItemRetrying}).
						Updates(map[string]any{"status": CommerceItemCanceled, "progress_percent": 100, "cancel_requested_at": now, "finished_at": now, "error_code": "canceled", "error_message": "project_deleted"}).Error; err != nil {
						return err
					}
					if err := emitCommerceEventTx(tx, job, CommerceEventItemReleased, map[string]any{"item_id": item.ID, "reason": "project_deleted"}); err != nil {
						return err
					}
				}
			}
			if err := tx.Model(&CommerceJob{}).Where("id = ? AND user_id = ? AND status IN ?", job.ID, userID, []CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying}).
				Updates(map[string]any{"status": CommerceJobCanceled, "cancel_requested_at": now, "finished_at": now, "error_code": "canceled", "error_message": "project_deleted"}).Error; err != nil {
				return err
			}
			if job.BatchID != nil {
				if err := refreshBatchCounters(ctx, tx, userID, *job.BatchID, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return CommerceProject{}, err
	}
	return accepted, nil
}

func (s *Service) ListBatchEvents(ctx context.Context, userID, batchID, afterID uint) ([]CommerceEvent, error) {
	if s == nil || s.repository == nil || s.repository.DB() == nil {
		return nil, fmt.Errorf("commerce repository is unavailable")
	}
	var count int64
	if err := s.repository.DB().WithContext(ctx).Model(&CommerceGenerationBatch{}).
		Where("id = ? AND user_id = ?", batchID, userID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrNotFound
	}
	var events []CommerceEvent
	err := s.repository.DB().WithContext(ctx).
		Where("user_id = ? AND batch_id = ? AND id > ?", userID, batchID, afterID).
		Order("id ASC").Limit(500).Find(&events).Error
	return events, err
}

func (s *Service) finalizeProjectDeletionTx(ctx context.Context, tx *gorm.DB, userID, projectID uint, now time.Time) error {
	if projectID == 0 {
		return nil
	}
	project, err := lockProjectRowTx(ctx, tx, userID, projectID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if project.DeletedAt.Valid || project.Status != "deletion_requested" || project.DeletionRequestedAt == nil {
		return nil
	}
	var active int64
	if err := tx.Model(&CommerceJob{}).Where("user_id = ? AND project_id = ? AND status IN ?", userID, projectID,
		[]CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying, CommerceJobRunning}).Count(&active).Error; err != nil {
		return err
	}
	if active != 0 {
		return nil
	}
	type projectAssetObject struct {
		CommerceAssetID  uint
		ReferenceAssetID uint
		StorageScope     string
		ObjectKey        string
	}
	var objects []projectAssetObject
	if tx.Migrator().HasTable("reference_assets") {
		if err := tx.Table("commerce_assets AS ca").
			Select("ca.id AS commerce_asset_id, ca.reference_asset_id, ra.storage_scope, ra.asset_key AS object_key").
			Joins("JOIN reference_assets AS ra ON ra.id = ca.reference_asset_id").
			Where("ca.user_id = ? AND ca.project_id = ? AND ca.deleted_at IS NULL", userID, projectID).
			Scan(&objects).Error; err != nil {
			return err
		}
	}
	for _, object := range objects {
		assetID, referenceID := object.CommerceAssetID, object.ReferenceAssetID
		var existing int64
		if err := tx.Model(&CommerceObjectCleanup{}).Where("commerce_asset_id = ? AND object_deleted_at IS NULL", assetID).Count(&existing).Error; err != nil {
			return err
		}
		if existing == 0 && strings.TrimSpace(object.ObjectKey) != "" {
			next := now
			if err := tx.Create(&CommerceObjectCleanup{
				UserID: userID, ProjectID: projectID, CommerceAssetID: &assetID, ReferenceAssetID: &referenceID,
				StorageScope: normalizedStorageScope(object.StorageScope), ObjectKey: object.ObjectKey,
				Reason: "project_deleted", Status: CleanupStatusQueued, MaxAttempts: defaultCleanupMaxAttempts,
				NextAttemptAt: &next, DeleteAfter: now,
			}).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Where("user_id = ? AND project_id = ?", userID, projectID).Delete(&CommerceAsset{}).Error; err != nil {
		return err
	}
	return tx.Where("id = ? AND user_id = ?", projectID, userID).Delete(&CommerceProject{}).Error
}

func lockWritableProjectTx(ctx context.Context, tx *gorm.DB, userID, projectID uint) (CommerceProject, error) {
	project, err := lockProjectRowTx(ctx, tx, userID, projectID)
	if err != nil {
		return CommerceProject{}, err
	}
	if project.DeletedAt.Valid || project.Status == "deletion_requested" || project.DeletionRequestedAt != nil {
		return CommerceProject{}, ErrProjectDeletionRequested
	}
	return project, nil
}

func lockProjectRowTx(ctx context.Context, tx *gorm.DB, userID, projectID uint) (CommerceProject, error) {
	if tx == nil || userID == 0 || projectID == 0 {
		return CommerceProject{}, ErrNotFound
	}
	query := tx.WithContext(ctx).Unscoped().Where("id = ? AND user_id = ?", projectID, userID)
	if tx.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	} else {
		claimed := tx.WithContext(ctx).Unscoped().Model(&CommerceProject{}).
			Where("id = ? AND user_id = ?", projectID, userID).
			UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if claimed.Error != nil {
			return CommerceProject{}, claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return CommerceProject{}, ErrNotFound
		}
	}
	var project CommerceProject
	if err := query.First(&project).Error; err != nil {
		return CommerceProject{}, mapNotFound(err)
	}
	return project, nil
}

func runProjectMutationTransaction(ctx context.Context, db *gorm.DB, mutation func(*gorm.DB) error) error {
	const sqliteLockAttempts = 50
	for attempt := 0; ; attempt++ {
		err := db.WithContext(ctx).Transaction(mutation)
		if db.Dialector.Name() != "sqlite" || !isSQLiteLockConflict(err) || attempt+1 >= sqliteLockAttempts {
			return err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isSQLiteLockConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
}

func (s *Service) ValidateProjectWritable(ctx context.Context, userID, projectID uint) (CommerceProject, error) {
	project, err := s.repository.GetProject(ctx, userID, projectID)
	if err != nil {
		return CommerceProject{}, err
	}
	if project.Status == "deletion_requested" || project.DeletionRequestedAt != nil {
		return CommerceProject{}, ErrProjectDeletionRequested
	}
	return project, nil
}

func (s *Service) ValidateProjectSKUs(ctx context.Context, userID, productID uint, skuIDs []uint) error {
	if _, err := s.repository.GetProduct(ctx, userID, productID); err != nil {
		return err
	}
	seen := make(map[uint]struct{}, len(skuIDs))
	for _, skuID := range skuIDs {
		if skuID == 0 {
			return ErrOwnershipMismatch
		}
		if _, ok := seen[skuID]; ok {
			continue
		}
		seen[skuID] = struct{}{}
		sku, err := s.repository.GetSKU(ctx, userID, skuID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrOwnershipMismatch
			}
			return err
		}
		if sku.ProductID != productID {
			return ErrOwnershipMismatch
		}
	}
	return nil
}

func (s *Service) validateDefaultSKU(ctx context.Context, userID, productID uint, skuID *uint) error {
	if skuID == nil {
		return nil
	}
	if err := s.ValidateProjectSKUs(ctx, userID, productID, []uint{*skuID}); err != nil {
		return err
	}
	sku, err := s.repository.GetSKU(ctx, userID, *skuID)
	if err != nil {
		return err
	}
	if sku.Status != "active" {
		return ErrDefaultSKUDisable
	}
	return nil
}

func isAllowedPipeline(pipeline string) bool {
	_, ok := allowedPipelines[pipeline]
	return ok
}

func defaultJSON(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func assignString(target *string, value *string) {
	if value != nil {
		*target = *value
	}
}

func assignTrimmed(target *string, value *string) {
	if value != nil {
		*target = strings.TrimSpace(*value)
	}
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}
