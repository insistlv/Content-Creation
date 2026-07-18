package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"image-agent/internal/app/ecommerce"
)

func adminCommerceCategoryPayload(row ecommerce.CommerceSystemCategory) gin.H {
	aliases := []string{}
	_ = ecommerce.DecodeJSON(row.SearchAliasesJSON, &aliases)
	return gin.H{"id": row.ID, "parent_id": row.ParentID, "level": row.Level, "name": row.Name, "aliases": aliases, "sort_order": row.SortOrder, "status": row.Status, "catalog_version": row.CatalogVersion, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func (a *App) handleListAdminCommerceCategories(c *gin.Context) {
	rows, err := a.commerceService.ListAdminCategories(c.Request.Context())
	if !writeCommerceResult(c, err) {
		return
	}
	customCount, err := a.commerceService.CountUserCategories(c.Request.Context())
	if !writeCommerceResult(c, err) {
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminCommerceCategoryPayload(row))
	}
	writeJSON(c, http.StatusOK, gin.H{"version": ecommerce.CategoryCatalogVersion, "items": items, "summary": gin.H{"system_categories": len(items), "user_custom_categories": customCount}})
}

func (a *App) handleCreateAdminCommerceCategory(c *gin.Context) {
	var req struct {
		ParentID  *uint    `json:"parent_id"`
		Level     int      `json:"level"`
		Name      string   `json:"name"`
		Aliases   []string `json:"aliases"`
		SortOrder int      `json:"sort_order"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	row, err := a.commerceService.CreateSystemCategory(c.Request.Context(), ecommerce.CreateSystemCategoryInput{ParentID: req.ParentID, Level: req.Level, Name: req.Name, Aliases: req.Aliases, SortOrder: req.SortOrder})
	if !writeCommerceResult(c, err) {
		return
	}
	a.writeAdminAudit(c, "commerce_category.create", "commerce_category", row.ID, gin.H{"name": row.Name, "level": row.Level, "parent_id": row.ParentID})
	writeJSON(c, http.StatusCreated, adminCommerceCategoryPayload(row))
}

func (a *App) handlePatchAdminCommerceCategory(c *gin.Context) {
	id, ok := commercePathID(c)
	if !ok {
		return
	}
	var req struct {
		Name, Status *string
		Aliases      *[]string `json:"aliases"`
		SortOrder    *int      `json:"sort_order"`
	}
	if !bindCommerceJSON(c, &req) {
		return
	}
	row, err := a.commerceService.PatchSystemCategory(c.Request.Context(), id, ecommerce.PatchSystemCategoryInput{Name: req.Name, Status: req.Status, Aliases: req.Aliases, SortOrder: req.SortOrder})
	if !writeCommerceResult(c, err) {
		return
	}
	a.writeAdminAudit(c, "commerce_category.update", "commerce_category", row.ID, gin.H{"name": row.Name, "status": row.Status, "sort_order": row.SortOrder})
	writeJSON(c, http.StatusOK, adminCommerceCategoryPayload(row))
}
