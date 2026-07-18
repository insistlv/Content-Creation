package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"image-agent/internal/app/ecommerce"
)

func (a *App) handleCommerceCapabilities(c *gin.Context) {
	enabled := a != nil && a.cfg.AICommerceEnabled
	workerConfigured := a != nil && a.cfg.AICommerceWorkerEnabled
	workerRunning := a != nil && a.commerceWorker != nil && a.commerceWorker.Running()
	writeJSON(c, http.StatusOK, gin.H{
		"enabled":               enabled,
		"worker_enabled":        enabled && workerConfigured && workerRunning,
		"worker_configured":     workerConfigured,
		"worker_running":        workerRunning,
		"pipelines":             []string{"general", "fashion", "mixed"},
		"recipes":               a.commerceRecipes.List(""),
		"private_storage_ready": a.assetStore != nil,
	})
}

func (a *App) requireCommerceEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a == nil || !a.cfg.AICommerceEnabled {
			writeError(c, http.StatusServiceUnavailable, "commerce_disabled", "AI 电商功能未启用")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (a *App) handleListCommerceRecipes(c *gin.Context) {
	writeJSON(c, http.StatusOK, gin.H{"items": a.commerceRecipes.List(strings.TrimSpace(c.Query("pipeline")))})
}

func commerceCategoryNodePayload(node ecommerce.CategoryNode) gin.H {
	children := make([]gin.H, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, commerceCategoryNodePayload(child))
	}
	return gin.H{"id": node.ID, "parent_id": node.ParentID, "source": node.Source, "name": node.Name, "path": node.Path, "status": node.Status, "aliases": node.Aliases, "sort_order": node.SortOrder, "children": children}
}

func (a *App) handleListCommerceCategories(c *gin.Context) {
	catalog, err := a.commerceService.ListCategories(c.Request.Context(), currentUser(c).ID)
	if !writeCommerceResult(c, err) {
		return
	}
	convert := func(nodes []ecommerce.CategoryNode) []gin.H {
		result := make([]gin.H, 0, len(nodes))
		for _, node := range nodes {
			result = append(result, commerceCategoryNodePayload(node))
		}
		return result
	}
	writeJSON(c, http.StatusOK, gin.H{"version": catalog.Version, "system_categories": convert(catalog.SystemCategories), "custom_categories": convert(catalog.CustomCategories), "recent_categories": convert(catalog.RecentCategories)})
}

func (a *App) handleCreateCommerceCustomCategory(c *gin.Context) {
	var req struct {
		ParentID uint   `json:"parent_id"`
		Name     string `json:"name"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	row, err := a.commerceService.CreateCustomCategory(c.Request.Context(), currentUser(c).ID, ecommerce.CreateCustomCategoryInput{ParentID: req.ParentID, Name: req.Name})
	if !writeCommerceResult(c, err) {
		return
	}
	path, _ := a.commerceService.ResolveCategorySelection(c.Request.Context(), currentUser(c).ID, row.ID, ecommerce.CategorySourceUser)
	writeJSON(c, http.StatusCreated, gin.H{"id": row.ID, "parent_id": row.ParentID, "source": ecommerce.CategorySourceUser, "name": row.Name, "path": path, "status": row.Status})
}

func (a *App) handlePatchCommerceCustomCategory(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		Name   *string `json:"name"`
		Status *string `json:"status"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	row, err := a.commerceService.PatchCustomCategory(c.Request.Context(), currentUser(c).ID, id, ecommerce.PatchCustomCategoryInput{Name: req.Name, Status: req.Status})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"id": row.ID, "parent_id": row.ParentID, "source": ecommerce.CategorySourceUser, "name": row.Name, "status": row.Status})
}

func (a *App) handleCreateCommerceBrand(c *gin.Context) {
	var req struct {
		Name                 string          `json:"name"`
		LogoReferenceAssetID *uint           `json:"logo_reference_asset_id"`
		ColorPalette         json.RawMessage `json:"color_palette"`
		Fonts                json.RawMessage `json:"fonts"`
		ForbiddenTerms       json.RawMessage `json:"forbidden_terms"`
		VisualRules          json.RawMessage `json:"visual_rules"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	brand, err := a.commerceService.CreateBrand(c.Request.Context(), currentUser(c).ID, ecommerce.CreateBrandInput{
		Name:                 req.Name,
		LogoReferenceAssetID: req.LogoReferenceAssetID,
		ColorPaletteJSON:     string(req.ColorPalette),
		FontsJSON:            string(req.Fonts),
		ForbiddenTermsJSON:   string(req.ForbiddenTerms),
		VisualRulesJSON:      string(req.VisualRules),
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceBrandPayload(brand))
}

func (a *App) handleListCommerceBrands(c *gin.Context) {
	brands, err := a.commerceService.ListBrands(c.Request.Context(), currentUser(c).ID)
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(brands))
	for _, brand := range brands {
		items = append(items, commerceBrandPayload(brand))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleGetCommerceBrand(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	brand, err := a.commerceService.GetBrand(c.Request.Context(), currentUser(c).ID, id)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceBrandPayload(brand))
}

func (a *App) handlePatchCommerceBrand(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		Name                 *string          `json:"name"`
		LogoReferenceAssetID *uint            `json:"logo_reference_asset_id"`
		ClearLogo            bool             `json:"clear_logo_reference_asset_id"`
		ColorPalette         *json.RawMessage `json:"color_palette"`
		Fonts                *json.RawMessage `json:"fonts"`
		ForbiddenTerms       *json.RawMessage `json:"forbidden_terms"`
		VisualRules          *json.RawMessage `json:"visual_rules"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	brand, err := a.commerceService.PatchBrand(c.Request.Context(), currentUser(c).ID, id, ecommerce.PatchBrandInput{
		Name:                      req.Name,
		LogoReferenceAssetID:      req.LogoReferenceAssetID,
		ClearLogoReferenceAssetID: req.ClearLogo,
		ColorPaletteJSON:          rawMessageString(req.ColorPalette),
		FontsJSON:                 rawMessageString(req.Fonts),
		ForbiddenTermsJSON:        rawMessageString(req.ForbiddenTerms),
		VisualRulesJSON:           rawMessageString(req.VisualRules),
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceBrandPayload(brand))
}

func (a *App) handleCreateCommerceProduct(c *gin.Context) {
	var req struct {
		BrandID        *uint           `json:"brand_id"`
		Name           string          `json:"name"`
		Category       string          `json:"category"`
		SPUCode        string          `json:"spu_code"`
		SellingPoints  json.RawMessage `json:"selling_points"`
		TargetChannels json.RawMessage `json:"target_channels"`
		Status         string          `json:"status"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	product, err := a.commerceService.CreateProduct(c.Request.Context(), currentUser(c).ID, ecommerce.CreateProductInput{
		BrandID:            req.BrandID,
		Name:               req.Name,
		Category:           req.Category,
		SPUCode:            req.SPUCode,
		SellingPointsJSON:  string(req.SellingPoints),
		TargetChannelsJSON: string(req.TargetChannels),
		Status:             req.Status,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceProductPayload(product))
}

func (a *App) handleListCommerceProducts(c *gin.Context) {
	products, err := a.commerceService.ListProducts(c.Request.Context(), currentUser(c).ID)
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(products))
	for _, product := range products {
		items = append(items, commerceProductPayload(product))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleGetCommerceProduct(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	product, err := a.commerceService.GetProduct(c.Request.Context(), currentUser(c).ID, id)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceProductPayload(product))
}

func (a *App) handlePatchCommerceProduct(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		BrandID        *uint            `json:"brand_id"`
		ClearBrandID   bool             `json:"clear_brand_id"`
		Name           *string          `json:"name"`
		Category       *string          `json:"category"`
		CategoryID     *uint            `json:"category_id"`
		CategorySource *string          `json:"category_source"`
		CategoryPath   *string          `json:"category_path"`
		SPUCode        *string          `json:"spu_code"`
		SellingPoints  *json.RawMessage `json:"selling_points"`
		TargetChannels *json.RawMessage `json:"target_channels"`
		Status         *string          `json:"status"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	product, err := a.commerceService.PatchProduct(c.Request.Context(), currentUser(c).ID, id, ecommerce.PatchProductInput{
		BrandID:            req.BrandID,
		ClearBrandID:       req.ClearBrandID,
		Name:               req.Name,
		Category:           req.Category,
		CategoryID:         req.CategoryID,
		CategorySource:     req.CategorySource,
		CategoryPath:       req.CategoryPath,
		SPUCode:            req.SPUCode,
		SellingPointsJSON:  rawMessageString(req.SellingPoints),
		TargetChannelsJSON: rawMessageString(req.TargetChannels),
		Status:             req.Status,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceProductPayload(product))
}

func (a *App) handleCreateCommerceSKU(c *gin.Context) {
	productID, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		Code       string          `json:"code"`
		Color      string          `json:"color"`
		Style      string          `json:"style"`
		Size       string          `json:"size"`
		Status     string          `json:"status"`
		Attributes json.RawMessage `json:"attributes"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	sku, err := a.commerceService.CreateSKU(c.Request.Context(), currentUser(c).ID, productID, ecommerce.CreateSKUInput{
		Code: req.Code, Color: req.Color, Style: req.Style, Size: req.Size,
		AttributesJSON: string(req.Attributes), Status: req.Status,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceSKUPayload(sku))
}

func (a *App) handleListCommerceSKUs(c *gin.Context) {
	productID, ok := commercePathID(c)
	if !ok {
		return
	}
	skus, err := a.commerceService.ListSKUs(c.Request.Context(), currentUser(c).ID, productID)
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(skus))
	for _, sku := range skus {
		items = append(items, commerceSKUPayload(sku))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleGetCommerceSKUConfig(c *gin.Context) {
	productID, ok := commercePathID(c)
	if !ok {
		return
	}
	config, err := a.commerceService.GetSKUConfig(c.Request.Context(), currentUser(c).ID, productID)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceSKUConfigPayload(config))
}

func bindSKUMatrixInput(c *gin.Context) (ecommerce.SKUMatrixInput, bool) {
	var req ecommerce.SKUMatrixInput
	if !bindCommerceJSON(c, &req) {
		return req, false
	}
	return req, true
}

func (a *App) handlePreviewCommerceSKUMatrix(c *gin.Context) {
	productID, ok := commercePathID(c)
	if !ok {
		return
	}
	req, ok := bindSKUMatrixInput(c)
	if !ok {
		return
	}
	preview, err := a.commerceService.PreviewSKUMatrix(c.Request.Context(), currentUser(c).ID, productID, req)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, preview)
}

func (a *App) handleApplyCommerceSKUMatrix(c *gin.Context) {
	productID, ok := commercePathID(c)
	if !ok {
		return
	}
	req, ok := bindSKUMatrixInput(c)
	if !ok {
		return
	}
	config, err := a.commerceService.ApplySKUMatrix(c.Request.Context(), currentUser(c).ID, productID, c.GetHeader("Idempotency-Key"), req)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceSKUConfigPayload(config))
}

func commerceSKUConfigPayload(config ecommerce.SKUConfig) gin.H {
	dimensions := make([]gin.H, 0, len(config.Dimensions))
	for _, d := range config.Dimensions {
		dimensions = append(dimensions, gin.H{"id": d.ID, "name": d.Name, "version": d.Version, "sort_order": d.SortOrder, "status": d.Status})
	}
	values := make([]gin.H, 0, len(config.Values))
	for _, v := range config.Values {
		values = append(values, gin.H{"id": v.ID, "dimension_id": v.DimensionID, "name": v.Name, "sort_order": v.SortOrder, "status": v.Status})
	}
	skus := make([]gin.H, 0, len(config.SKUs))
	for _, sku := range config.SKUs {
		skus = append(skus, commerceSKUPayload(sku))
	}
	payload := gin.H{"version": config.Version, "dimensions": dimensions, "values": values, "skus": skus}
	if config.DefaultKnown {
		payload["default_sku_id"] = config.DefaultSKUID
	}
	return payload
}

func (a *App) handlePatchCommerceSKU(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		Code       *string          `json:"code"`
		Color      *string          `json:"color"`
		Style      *string          `json:"style"`
		Size       *string          `json:"size"`
		Status     *string          `json:"status"`
		Attributes *json.RawMessage `json:"attributes"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	sku, err := a.commerceService.PatchSKU(c.Request.Context(), currentUser(c).ID, id, ecommerce.PatchSKUInput{
		Code: req.Code, Color: req.Color, Style: req.Style, Size: req.Size,
		AttributesJSON: rawMessageString(req.Attributes), Status: req.Status,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceSKUPayload(sku))
}

func (a *App) handleCreateCommerceProject(c *gin.Context) {
	var req struct {
		ProductID             uint   `json:"product_id"`
		BrandID               *uint  `json:"brand_id"`
		DefaultSKUID          *uint  `json:"default_sku_id"`
		Title                 string `json:"title"`
		Pipeline              string `json:"pipeline"`
		Status                string `json:"status"`
		DefaultChannelProfile string `json:"default_channel_profile"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	project, err := a.commerceService.CreateProject(c.Request.Context(), currentUser(c).ID, ecommerce.CreateProjectInput{
		ProductID: req.ProductID, BrandID: req.BrandID, DefaultSKUID: req.DefaultSKUID,
		Title: req.Title, Pipeline: req.Pipeline, Status: req.Status, DefaultChannelProfile: req.DefaultChannelProfile,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceProjectPayload(project))
}

func (a *App) handleBootstrapCommerceProject(c *gin.Context) {
	var req struct {
		Title          string `json:"title"`
		Category       string `json:"category"`
		CategoryID     uint   `json:"category_id"`
		CategorySource string `json:"category_source"`
		CategoryPath   string `json:"category_path"`
		SKUCode        string `json:"sku_code"`
		Pipeline       string `json:"pipeline"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	result, err := a.commerceService.BootstrapProject(c.Request.Context(), currentUser(c).ID, c.GetHeader("Idempotency-Key"), ecommerce.BootstrapProjectInput{
		Title: req.Title, Category: req.Category, CategoryID: req.CategoryID, CategorySource: req.CategorySource, CategoryPath: req.CategoryPath, SKUCode: req.SKUCode, Pipeline: req.Pipeline,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(c, status, gin.H{"product": commerceProductPayload(result.Product), "sku": commerceSKUPayload(result.SKU), "project": commerceProjectPayload(result.Project)})
}

func (a *App) handleAnalyzeCommerceProduct(c *gin.Context) {
	projectID, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		SourceAssetIDs   []uint `json:"source_asset_ids"`
		UserRequirements string `json:"user_requirements"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	result, err := a.commerceService.AnalyzeProduct(c.Request.Context(), currentUser(c).ID, projectID, c.GetHeader("Idempotency-Key"), ecommerce.AnalyzeProductInput{
		SourceAssetIDs: req.SourceAssetIDs, UserRequirements: req.UserRequirements,
	})
	if errors.Is(err, ecommerce.ErrVisionNotConfigured) {
		writeJSON(c, http.StatusServiceUnavailable, gin.H{"error": "commerce_vision_not_configured", "required_fields": []string{"source_asset_ids", "user_requirements"}})
		return
	}
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusAccepted, gin.H{"creative_spec": commerceCreativeSpecPayload(result.CreativeSpec), "job": commerceJobPayload(result.Job)})
}

func (a *App) handleListCommerceProjects(c *gin.Context) {
	projects, err := a.commerceService.ListProjects(c.Request.Context(), currentUser(c).ID)
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(projects))
	for _, project := range projects {
		items = append(items, commerceProjectPayload(project))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleGetCommerceProject(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	project, err := a.commerceService.GetProject(c.Request.Context(), currentUser(c).ID, id)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceProjectPayload(project))
}

func (a *App) handlePatchCommerceProject(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		ProductID             *uint   `json:"product_id"`
		BrandID               *uint   `json:"brand_id"`
		DefaultSKUID          *uint   `json:"default_sku_id"`
		ClearBrandID          bool    `json:"clear_brand_id"`
		ClearDefaultSKUID     bool    `json:"clear_default_sku_id"`
		Title                 *string `json:"title"`
		Pipeline              *string `json:"pipeline"`
		Status                *string `json:"status"`
		DefaultChannelProfile *string `json:"default_channel_profile"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	project, err := a.commerceService.PatchProject(c.Request.Context(), currentUser(c).ID, id, ecommerce.PatchProjectInput{
		ProductID: req.ProductID, BrandID: req.BrandID, DefaultSKUID: req.DefaultSKUID,
		ClearBrandID: req.ClearBrandID, ClearDefaultSKUID: req.ClearDefaultSKUID,
		Title: req.Title, Pipeline: req.Pipeline, Status: req.Status, DefaultChannelProfile: req.DefaultChannelProfile,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceProjectPayload(project))
}

func (a *App) handleDeleteCommerceProject(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	project, err := a.commerceService.RequestProjectDeletion(c.Request.Context(), currentUser(c).ID, id)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusAccepted, commerceProjectPayload(project))
}

func (a *App) handleCreateManualCommerceCreativeSpec(c *gin.Context) {
	projectID, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		ProductFacts     json.RawMessage `json:"product_facts"`
		SellingPoints    json.RawMessage `json:"selling_points"`
		ForbiddenChanges json.RawMessage `json:"forbidden_changes"`
		BrandTone        json.RawMessage `json:"brand_tone"`
		CopyBlocks       json.RawMessage `json:"copy_blocks"`
		RiskNotices      json.RawMessage `json:"risk_notices"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	spec, err := a.commerceService.CreateManualCreativeSpec(c.Request.Context(), currentUser(c).ID, projectID, ecommerce.ManualCreativeSpecInput{
		ProductFacts: req.ProductFacts, SellingPoints: req.SellingPoints, ForbiddenChanges: req.ForbiddenChanges,
		BrandTone: req.BrandTone, CopyBlocks: req.CopyBlocks, RiskNotices: req.RiskNotices,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceCreativeSpecPayload(spec))
}

func (a *App) handleGetCommerceCreativeSpec(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	spec, err := a.commerceService.GetCreativeSpec(c.Request.Context(), currentUser(c).ID, id)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceCreativeSpecPayload(spec))
}

func (a *App) handleGetLatestCommerceCreativeSpec(c *gin.Context) {
	projectID, ok := commercePathID(c)
	if !ok {
		return
	}
	spec, err := a.commerceService.GetLatestCreativeSpec(c.Request.Context(), currentUser(c).ID, projectID)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceCreativeSpecPayload(spec))
}

func (a *App) handlePatchCommerceCreativeSpec(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion  int                  `json:"expected_version"`
		ProductFacts     commerceOptionalJSON `json:"product_facts"`
		SellingPoints    commerceOptionalJSON `json:"selling_points"`
		ForbiddenChanges commerceOptionalJSON `json:"forbidden_changes"`
		BrandTone        commerceOptionalJSON `json:"brand_tone"`
		CopyBlocks       *json.RawMessage     `json:"copy_blocks"`
		RiskNotices      *json.RawMessage     `json:"risk_notices"`
		UserOverrides    commerceOptionalJSON `json:"user_overrides"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	spec, err := a.commerceService.PatchCreativeSpec(c.Request.Context(), currentUser(c).ID, id, ecommerce.PatchCreativeSpecInput{
		ExpectedVersion: req.ExpectedVersion,
		ProductFacts:    req.ProductFacts.bytes(), SellingPoints: req.SellingPoints.bytes(),
		ForbiddenChanges: req.ForbiddenChanges.bytes(), BrandTone: req.BrandTone.bytes(),
		CopyBlocks: rawMessageBytes(req.CopyBlocks), RiskNotices: rawMessageBytes(req.RiskNotices),
		UserOverrides: req.UserOverrides.bytes(),
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceCreativeSpecPayload(spec))
}

func (a *App) handleConfirmCommerceCreativeSpec(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	spec, err := a.commerceService.ConfirmCreativeSpec(c.Request.Context(), currentUser(c).ID, id)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceCreativeSpecPayload(spec))
}

type commerceBatchRequest struct {
	RecipeKey         string            `json:"recipe_key"`
	QualityTier       string            `json:"quality_tier"`
	RecipeVersion     int               `json:"recipe_version"`
	OutputCount       int               `json:"output_count"`
	CreativeSpecID    uint              `json:"creative_spec_id"`
	PrimarySKUID      uint              `json:"primary_sku_id"`
	SelectedSKUIDs    []uint            `json:"selected_sku_ids"`
	AspectRatio       string            `json:"aspect_ratio"`
	AssetBindings     map[string][]uint `json:"asset_bindings"`
	Parameters        map[string]any    `json:"parameters"`
	PricingSnapshotID string            `json:"pricing_snapshot_id"`
}

func (r commerceBatchRequest) estimateRequest() ecommerce.EstimateBatchRequest {
	return ecommerce.EstimateBatchRequest{
		RecipeKey: r.RecipeKey, QualityTier: r.QualityTier,
		RecipeVersion: r.RecipeVersion, OutputCount: r.OutputCount,
		CreativeSpecID: r.CreativeSpecID, PrimarySKUID: r.PrimarySKUID,
		SelectedSKUIDs: r.SelectedSKUIDs, AspectRatio: r.AspectRatio,
		AssetBindings: r.AssetBindings, Parameters: r.Parameters,
	}
}

func (a *App) handleEstimateCommerceBatch(c *gin.Context) {
	projectID, ok := commercePathID(c)
	if !ok {
		return
	}
	var req commerceBatchRequest
	if !bindCommerceJSON(c, &req) {
		return
	}
	estimate, err := a.commerceService.EstimateBatch(c.Request.Context(), currentUser(c).ID, projectID, req.estimateRequest())
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceBatchEstimatePayload(estimate))
}

func (a *App) handleSubmitCommerceBatch(c *gin.Context) {
	projectID, ok := commercePathID(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		writeError(c, http.StatusBadRequest, "idempotency_key_required", "缺少 Idempotency-Key")
		return
	}
	var req commerceBatchRequest
	if !bindCommerceJSON(c, &req) {
		return
	}
	batch, err := a.commerceService.SubmitBatch(c.Request.Context(), currentUser(c).ID, projectID, key, ecommerce.SubmitBatchRequest{
		EstimateBatchRequest: req.estimateRequest(), PricingSnapshotID: req.PricingSnapshotID,
	})
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceBatchSnapshotPayload(batch))
}

func (a *App) handleGetCommerceBatch(c *gin.Context) {
	batchID, ok := commercePathID(c)
	if !ok {
		return
	}
	batch, err := a.commerceService.GetBatch(c.Request.Context(), currentUser(c).ID, batchID)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceBatchSnapshotPayload(batch))
}

func (a *App) handleListCommerceBatchEvents(c *gin.Context) {
	batchID, ok := commercePathID(c)
	if !ok {
		return
	}
	var afterID uint
	if raw := strings.TrimSpace(c.Query("after_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_after_id", "after_id 无效")
			return
		}
		afterID = uint(value)
	}
	events, err := a.commerceService.ListBatchEvents(c.Request.Context(), currentUser(c).ID, batchID, afterID)
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(events))
	for _, event := range events {
		items = append(items, gin.H{
			"id": event.ID, "project_id": event.ProjectID, "batch_id": event.BatchID, "job_id": event.JobID,
			"entity_type": event.EntityType, "entity_id": event.EntityID, "pipeline": event.Pipeline,
			"recipe_key": event.RecipeKey, "event_type": event.EventType, "metadata_json": event.MetadataJSON,
			"created_at": event.CreatedAt,
		})
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleListCommerceBatches(c *gin.Context) {
	projectID, ok := commercePathID(c)
	if !ok {
		return
	}
	batches, err := a.commerceService.ListBatches(c.Request.Context(), currentUser(c).ID, projectID)
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(batches))
	for _, batch := range batches {
		items = append(items, commerceBatchPayload(batch))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleCancelCommerceBatch(c *gin.Context) {
	batchID, ok := commercePathID(c)
	if !ok {
		return
	}
	batch, err := a.commerceService.CancelBatch(c.Request.Context(), currentUser(c).ID, batchID)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceBatchSnapshotPayload(batch))
}

func (a *App) handleCancelCommerceItem(c *gin.Context) {
	itemID, ok := commercePathID(c)
	if !ok {
		return
	}
	item, err := a.commerceService.CancelItem(c.Request.Context(), currentUser(c).ID, itemID)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusOK, commerceItemPayload(item))
}

func (a *App) handleRetryCommerceItem(c *gin.Context) {
	itemID, ok := commercePathID(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		writeError(c, http.StatusBadRequest, "idempotency_key_required", "缺少 Idempotency-Key")
		return
	}
	batch, err := a.commerceService.RetryItem(c.Request.Context(), currentUser(c).ID, itemID, key)
	if !writeCommerceResult(c, err) {
		return
	}
	writeJSON(c, http.StatusCreated, commerceBatchSnapshotPayload(batch))
}

func commerceBatchEstimatePayload(estimate ecommerce.BatchEstimate) gin.H {
	items := make([]gin.H, 0, len(estimate.Items))
	for _, item := range estimate.Items {
		items = append(items, commerceCompiledItemPayload(item))
	}
	return gin.H{
		"items": items, "total_items": estimate.TotalItems, "estimated_credits": estimate.EstimatedCredits,
		"pricing_version": estimate.PricingVersion, "pricing_snapshot_id": estimate.PricingSnapshotID,
		"pricing_expires_at": estimate.PricingExpiresAt, "request_digest": estimate.RequestDigest,
		"eta_seconds": estimate.ETASeconds,
	}
}

func commerceBatchSnapshotPayload(snapshot ecommerce.BatchSnapshot) gin.H {
	items := make([]gin.H, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		items = append(items, commerceItemPayload(item))
	}
	return gin.H{"batch": commerceBatchPayload(snapshot.Batch), "items": items}
}

func commerceBatchPayload(batch ecommerce.CommerceGenerationBatch) gin.H {
	return gin.H{
		"id": batch.ID, "project_id": batch.ProjectID, "parent_batch_id": batch.ParentBatchID,
		"reservation_id": batch.ReservationID, "primary_sku_id": batch.PrimarySKUID,
		"pipeline": batch.Pipeline, "recipe_key": batch.RecipeKey, "recipe_version": batch.RecipeVersion,
		"quality_tier": batch.QualityTier, "status": batch.Status, "pricing_version": batch.PricingVersion,
		"eta_seconds":         batch.ETASeconds,
		"pricing_snapshot_id": batch.PricingSnapshotID, "total_items": batch.TotalItems,
		"queued_items": batch.QueuedItems, "running_items": batch.RunningItems, "retrying_items": batch.RetryingItems,
		"succeeded_items": batch.SucceededItems, "failed_items": batch.FailedItems, "canceled_items": batch.CanceledItems,
		"estimated_credits": batch.EstimatedCredits, "reserved_credits": batch.ReservedCredits,
		"settled_credits": batch.SettledCredits, "released_credits": batch.ReleasedCredits,
		"cancel_requested_at": batch.CancelRequestedAt, "created_at": batch.CreatedAt, "updated_at": batch.UpdatedAt,
	}
}

func commerceItemPayload(item ecommerce.CommerceGenerationItem) gin.H {
	payload := gin.H{
		"id": item.ID, "project_id": item.ProjectID, "batch_id": item.BatchID, "parent_item_id": item.ParentItemID,
		"reservation_id": item.ReservationID, "sku_id": item.SKUID, "scope": item.Scope, "slot_key": item.SlotKey,
		"candidate_index": item.CandidateIndex, "pipeline": item.Pipeline, "recipe_key": item.RecipeKey,
		"recipe_version": item.RecipeVersion, "quality_tier": item.QualityTier, "status": item.Status,
		"progress_percent": item.ProgressPercent,
		"pricing_version":  item.PricingVersion, "pricing_snapshot_id": item.PricingSnapshotID,
		"estimated_credits": item.EstimatedCredits, "reserved_credits": item.ReservedCredits,
		"settled_credits": item.SettledCredits, "released_credits": item.ReleasedCredits,
		"generation_record_id": item.GenerationRecordID, "work_id": item.WorkID,
		"output_snapshot": decodeCommerceJSON(item.OutputSnapshotJSON),
		"error_code":      item.ErrorCode, "error_message": item.ErrorMessage,
		"cancel_requested_at": item.CancelRequestedAt, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt,
	}
	if compiled, err := ecommerce.DecodeGenerationItemSnapshot(item.OutputSpecJSON); err == nil {
		addCommerceCompiledPresentation(payload, compiled)
	}
	return payload
}

func commerceCompiledItemPayload(item ecommerce.CompiledGenerationItem) gin.H {
	payload := gin.H{
		"sku_id": item.SKUID, "pipeline": item.Pipeline, "recipe_key": item.RecipeKey,
		"recipe_version": item.RecipeVersion, "slot_key": item.SlotKey, "aspect_ratio": item.AspectRatio,
		"pricing_version": item.PricingVersion, "pricing_snapshot_id": item.PricingSnapshotID,
		"estimated_credits": item.EstimatedCredits,
	}
	addCommerceCompiledPresentation(payload, item)
	return payload
}

func addCommerceCompiledPresentation(payload gin.H, item ecommerce.CompiledGenerationItem) {
	payload["scope"] = item.Scope
	payload["section"] = item.Section
	// 公共条目是明确的无 SKU 内容，不能携带主 SKU 或当前可变 SKU 的展示信息。
	if item.Scope == "shared" || item.SKUID == 0 {
		payload["sku_id"] = uint(0)
		payload["sku_code"] = ""
		payload["specification_path"] = ""
		payload["sku_snapshot"] = nil
		return
	}
	payload["sku_code"] = item.SKUCode
	payload["specification_path"] = item.SpecificationPath
	var snapshot ecommerce.SKUSnapshot
	if raw := strings.TrimSpace(item.SKUSnapshotJSON); raw != "" && json.Unmarshal([]byte(raw), &snapshot) == nil && snapshot.ID == item.SKUID {
		payload["sku_snapshot"] = gin.H{
			"id": snapshot.ID, "code": snapshot.Code, "specification_path": snapshot.SpecificationPath,
		}
	} else {
		payload["sku_snapshot"] = gin.H{
			"id": item.SKUID, "code": item.SKUCode, "specification_path": item.SpecificationPath,
		}
	}
}

func bindCommerceJSON(c *gin.Context, target any) bool {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil || rejectDuplicateJSONKeys(bytes.NewReader(raw)) != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求参数无效")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求参数无效")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求参数无效")
		return false
	}
	return true
}

func rejectDuplicateJSONKeys(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	if err := walkCommerceJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkCommerceJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := walkCommerceJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := walkCommerceJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("invalid JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func commercePathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		writeError(c, http.StatusBadRequest, "invalid_id", "ID 无效")
		return 0, false
	}
	return uint(value), true
}

func writeCommerceResult(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, ecommerce.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "资源不存在")
	case errors.Is(err, ecommerce.ErrInvalidInput):
		var fieldErr *ecommerce.FieldError
		if errors.As(err, &fieldErr) {
			c.Set(requestLogErrorCodeKey, "invalid_input")
			c.Set(requestLogErrorMessageKey, fieldErr.Message)
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
				"code": "invalid_input", "message": fieldErr.Message, "field": fieldErr.Field,
			}})
		} else {
			writeError(c, http.StatusUnprocessableEntity, "invalid_input", "请求字段无效")
		}
	case errors.Is(err, ecommerce.ErrCategoryConflict):
		writeError(c, http.StatusConflict, "category_conflict", "该品类已存在")
	case errors.Is(err, ecommerce.ErrCategoryUnavailable):
		writeError(c, http.StatusUnprocessableEntity, "category_unavailable", "所选商品品类不可用")
	case errors.Is(err, ecommerce.ErrConflict):
		writeError(c, http.StatusConflict, "commerce_conflict", "资源冲突")
	case errors.Is(err, ecommerce.ErrIdempotencyConflict):
		writeError(c, http.StatusConflict, "idempotency_conflict", "幂等键与请求内容冲突")
	case errors.Is(err, ecommerce.ErrSKUVersionConflict):
		writeError(c, http.StatusConflict, "sku_version_conflict", "SKU 版本已变化，请刷新后重试")
	case errors.Is(err, ecommerce.ErrDefaultSKUDisable):
		writeError(c, http.StatusConflict, "default_sku_disable_forbidden", "停用默认 SKU 前请先切换默认 SKU")
	case errors.Is(err, ecommerce.ErrCreditsInsufficient):
		writeError(c, http.StatusPaymentRequired, "credits_insufficient", "点数不足")
	case errors.Is(err, ecommerce.ErrPricingSnapshotStale):
		writeError(c, http.StatusConflict, "pricing_snapshot_stale", "价格快照已失效")
	case errors.Is(err, ecommerce.ErrInvalidItemTransition):
		writeError(c, http.StatusConflict, "invalid_item_transition", "当前状态不允许此操作")
	case errors.Is(err, ecommerce.ErrOwnershipMismatch):
		writeError(c, http.StatusUnprocessableEntity, "ownership_mismatch", "资源归属不匹配")
	case errors.Is(err, ecommerce.ErrInvalidPipeline):
		writeError(c, http.StatusUnprocessableEntity, "invalid_pipeline", "Pipeline 无效")
	case errors.Is(err, ecommerce.ErrVersionConflict):
		writeError(c, http.StatusConflict, "version_conflict", "版本已变化")
	case errors.Is(err, ecommerce.ErrCreativeSpecNotConfirmed):
		writeError(c, http.StatusConflict, "creative_spec_not_confirmed", "创意规格尚未确认")
	case errors.Is(err, ecommerce.ErrProjectDeletionRequested):
		writeError(c, http.StatusConflict, "project_deletion_requested", "项目正在删除")
	case errors.Is(err, ecommerce.ErrRecipeModelUnavailable):
		writeError(c, http.StatusUnprocessableEntity, "recipe_model_unavailable", "Recipe 所需模型不可用")
	case errors.Is(err, ecommerce.ErrRecipeReferenceLimitExceeded):
		writeError(c, http.StatusUnprocessableEntity, "recipe_reference_limit_exceeded", "Recipe 引用素材超过限制")
	case errors.Is(err, ecommerce.ErrRecipeConstraint), errors.Is(err, ecommerce.ErrRecipeNotFound):
		writeError(c, http.StatusUnprocessableEntity, "recipe_invalid", "Recipe 参数无效")
	default:
		writeErrorWithLogDetail(c, http.StatusInternalServerError, "commerce_internal_error", "电商服务处理失败", err.Error())
	}
	return false
}

func rawMessageString(value *json.RawMessage) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}

func rawMessageBytes(value *json.RawMessage) *[]byte {
	if value == nil {
		return nil
	}
	bytes := append([]byte(nil), (*value)...)
	return &bytes
}

type commerceOptionalJSON struct {
	present bool
	raw     json.RawMessage
}

func (value *commerceOptionalJSON) UnmarshalJSON(raw []byte) error {
	value.present = true
	value.raw = append(value.raw[:0], raw...)
	return nil
}

func (value commerceOptionalJSON) bytes() *[]byte {
	if !value.present {
		return nil
	}
	raw := append([]byte(nil), value.raw...)
	return &raw
}

func commerceBrandPayload(brand ecommerce.CommerceBrand) gin.H {
	return gin.H{"id": brand.ID, "name": brand.Name, "logo_reference_asset_id": brand.LogoReferenceAssetID,
		"color_palette": decodeCommerceJSON(brand.ColorPaletteJSON), "fonts": decodeCommerceJSON(brand.FontsJSON),
		"forbidden_terms": decodeCommerceJSON(brand.ForbiddenTermsJSON), "visual_rules": decodeCommerceJSON(brand.VisualRulesJSON),
		"created_at": brand.CreatedAt, "updated_at": brand.UpdatedAt}
}

func commerceProductPayload(product ecommerce.CommerceProduct) gin.H {
	return gin.H{"id": product.ID, "brand_id": product.BrandID, "name": product.Name, "category": product.Category,
		"category_id": product.CategoryID, "category_source": product.CategorySource, "category_path": product.CategoryPath,
		"spu_code": product.SPUCode, "selling_points": decodeCommerceJSON(product.SellingPointsJSON),
		"target_channels": decodeCommerceJSON(product.TargetChannelsJSON), "status": product.Status,
		"created_at": product.CreatedAt, "updated_at": product.UpdatedAt}
}

func commerceSKUPayload(sku ecommerce.CommerceSKU) gin.H {
	return gin.H{"id": sku.ID, "product_id": sku.ProductID, "code": sku.Code, "color": sku.Color, "style": sku.Style,
		"size": sku.Size, "attributes": decodeCommerceJSON(sku.AttributesJSON), "specification": sku.Specification, "is_default": sku.IsDefault, "asset_count": sku.AssetCount, "status": sku.Status,
		"created_at": sku.CreatedAt, "updated_at": sku.UpdatedAt}
}

func commerceProjectPayload(project ecommerce.CommerceProject) gin.H {
	return gin.H{"id": project.ID, "product_id": project.ProductID, "brand_id": project.BrandID,
		"default_sku_id": project.DefaultSKUID, "active_creative_spec_id": project.ActiveCreativeSpecID,
		"title": project.Title, "pipeline": project.Pipeline, "status": project.Status,
		"default_channel_profile": project.DefaultChannelProfile, "deletion_requested_at": project.DeletionRequestedAt,
		"created_at": project.CreatedAt, "updated_at": project.UpdatedAt}
}

func commerceCreativeSpecPayload(spec ecommerce.CommerceCreativeSpec) gin.H {
	return gin.H{"id": spec.ID, "project_id": spec.ProjectID, "version": spec.Version, "source": spec.Source, "status": spec.Status,
		"product_facts": decodeCommerceJSON(spec.ProductFactsJSON), "selling_points": decodeCommerceJSON(spec.SellingPointsJSON),
		"common_facts": decodeCommerceJSON(spec.CommonFactsJSON), "sku_overrides": decodeCommerceJSON(spec.SKUOverridesJSON),
		"forbidden_changes": decodeCommerceJSON(spec.ForbiddenChangesJSON), "brand_tone": decodeCommerceJSON(spec.BrandToneJSON),
		"copy_blocks": decodeCommerceJSON(spec.CopyBlocksJSON), "risk_notices": decodeCommerceJSON(spec.RiskNoticesJSON),
		"observed_facts": decodeCommerceJSON(spec.ObservedFactsJSON), "user_overrides": decodeCommerceJSON(spec.UserOverridesJSON),
		"missing_fields": decodeCommerceJSON(spec.MissingFieldsJSON), "suggested_sections": decodeCommerceJSON(spec.SuggestedSectionsJSON),
		"analysis_error": spec.AnalysisError, "analysis_request_hash": spec.AnalysisRequestHash,
		"source_asset_ids": decodeCommerceJSON(spec.SourceAssetIDsJSON),
		"locked_at":        spec.LockedAt, "created_at": spec.CreatedAt, "updated_at": spec.UpdatedAt}
}

func commerceJobPayload(job ecommerce.CommerceJob) gin.H {
	return gin.H{"id": job.ID, "project_id": job.ProjectID, "subject_id": job.SubjectID, "subject_type": job.SubjectType,
		"kind": job.Kind, "status": job.Status, "attempt_count": job.AttemptCount, "created_at": job.CreatedAt, "updated_at": job.UpdatedAt}
}

func decodeCommerceJSON(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}
