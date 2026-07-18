package ecommerce

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var (
	ErrNotFound          = errors.New("commerce resource not found")
	ErrConflict          = errors.New("commerce resource conflict")
	ErrOwnershipMismatch = errors.New("commerce ownership mismatch")
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) WithDB(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *gorm.DB {
	return r.db
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (r *Repository) CreateBrand(ctx context.Context, brand *CommerceBrand) error {
	return r.db.WithContext(ctx).Create(brand).Error
}

func (r *Repository) ListBrands(ctx context.Context, userID uint) ([]CommerceBrand, error) {
	var brands []CommerceBrand
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id desc").Find(&brands).Error
	return brands, err
}

func (r *Repository) GetBrand(ctx context.Context, userID, brandID uint) (CommerceBrand, error) {
	var brand CommerceBrand
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", brandID, userID).First(&brand).Error
	return brand, mapNotFound(err)
}

func (r *Repository) SaveBrand(ctx context.Context, userID uint, brand *CommerceBrand) error {
	result := r.db.WithContext(ctx).Model(&CommerceBrand{}).
		Where("id = ? AND user_id = ?", brand.ID, userID).
		Select("logo_reference_asset_id", "name", "color_palette_json", "fonts_json", "forbidden_terms_json", "visual_rules_json", "updated_at").
		Updates(brand)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CreateProduct(ctx context.Context, product *CommerceProduct) error {
	return r.db.WithContext(ctx).Create(product).Error
}

func (r *Repository) ListProducts(ctx context.Context, userID uint) ([]CommerceProduct, error) {
	var products []CommerceProduct
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id desc").Find(&products).Error
	return products, err
}

func (r *Repository) GetProduct(ctx context.Context, userID, productID uint) (CommerceProduct, error) {
	var product CommerceProduct
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", productID, userID).First(&product).Error
	return product, mapNotFound(err)
}

func (r *Repository) SaveProduct(ctx context.Context, userID uint, product *CommerceProduct) error {
	result := r.db.WithContext(ctx).Model(&CommerceProduct{}).
		Where("id = ? AND user_id = ?", product.ID, userID).
		Select("brand_id", "name", "category", "category_id", "category_source", "category_path", "spu_code", "selling_points_json", "target_channels_json", "status", "updated_at").
		Updates(product)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CreateSKU(ctx context.Context, sku *CommerceSKU) error {
	return r.db.WithContext(ctx).Create(sku).Error
}

func (r *Repository) ListSKUs(ctx context.Context, userID, productID uint) ([]CommerceSKU, error) {
	var skus []CommerceSKU
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND product_id = ?", userID, productID).
		Order("id asc").
		Find(&skus).Error
	return skus, err
}

func (r *Repository) GetSKU(ctx context.Context, userID, skuID uint) (CommerceSKU, error) {
	var sku CommerceSKU
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", skuID, userID).First(&sku).Error
	return sku, mapNotFound(err)
}

func (r *Repository) GetSKUByProductCode(ctx context.Context, userID, productID uint, code string) (CommerceSKU, error) {
	var sku CommerceSKU
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND product_id = ? AND code = ?", userID, productID, code).
		First(&sku).Error
	return sku, mapNotFound(err)
}

func (r *Repository) SaveSKU(ctx context.Context, userID uint, sku *CommerceSKU) error {
	result := r.db.WithContext(ctx).Model(&CommerceSKU{}).
		Where("id = ? AND user_id = ?", sku.ID, userID).
		Select("code", "color", "style", "size", "attributes_json", "status", "updated_at").
		Updates(sku)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CreateProject(ctx context.Context, project *CommerceProject) error {
	return r.db.WithContext(ctx).Create(project).Error
}

func (r *Repository) ListProjects(ctx context.Context, userID uint) ([]CommerceProject, error) {
	var projects []CommerceProject
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id desc").Find(&projects).Error
	return projects, err
}

func (r *Repository) GetProject(ctx context.Context, userID, projectID uint) (CommerceProject, error) {
	var project CommerceProject
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", projectID, userID).
		First(&project).Error
	return project, mapNotFound(err)
}

func (r *Repository) CreateCreativeSpec(ctx context.Context, spec *CommerceCreativeSpec) error {
	return r.db.WithContext(ctx).Create(spec).Error
}

func (r *Repository) GetCreativeSpec(ctx context.Context, userID, creativeSpecID uint) (CommerceCreativeSpec, error) {
	var spec CommerceCreativeSpec
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", creativeSpecID, userID).
		First(&spec).Error
	return spec, mapNotFound(err)
}

func (r *Repository) GetProjectCreativeSpec(ctx context.Context, userID, projectID, creativeSpecID uint) (CommerceCreativeSpec, error) {
	var spec CommerceCreativeSpec
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND project_id = ?", creativeSpecID, userID, projectID).
		First(&spec).Error
	return spec, mapNotFound(err)
}

func (r *Repository) GetLatestProjectCreativeSpec(ctx context.Context, userID, projectID uint) (CommerceCreativeSpec, error) {
	var spec CommerceCreativeSpec
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND project_id = ?", userID, projectID).
		Order("created_at desc").
		Order("id desc").
		First(&spec).Error
	return spec, mapNotFound(err)
}

func (r *Repository) SaveCreativeSpec(ctx context.Context, userID uint, spec *CommerceCreativeSpec) error {
	result := r.db.WithContext(ctx).Model(&CommerceCreativeSpec{}).
		Where("id = ? AND user_id = ?", spec.ID, userID).
		Select("version", "source", "status", "product_facts_json", "common_facts_json", "sku_overrides_json", "sku_context_sha256", "selling_points_json", "forbidden_changes_json", "brand_tone_json", "shot_plan_json", "copy_blocks_json", "risk_notices_json", "source_asset_ids_json", "observed_facts_json", "user_overrides_json", "missing_fields_json", "suggested_sections_json", "analysis_error", "analysis_request_hash", "locked_at", "updated_at").
		Updates(spec)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
