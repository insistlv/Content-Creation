package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminPromptTemplateRequest struct {
	Slug              *string `json:"slug"`
	Title             *string `json:"title"`
	Category          *string `json:"category"`
	Description       *string `json:"description"`
	Prompt            *string `json:"prompt"`
	AspectRatio       *string `json:"aspect_ratio"`
	StylePreset       *string `json:"style_preset"`
	Theme             *string `json:"theme"`
	WorkspaceSection  *string `json:"workspace_section"`
	WorkspaceToolMode *string `json:"workspace_tool_mode"`
	WorkspaceModelID  *uint   `json:"workspace_model_id"`
	WorkspaceSort     *int    `json:"workspace_sort"`
	SortOrder         *int    `json:"sort_order"`
	IsActive          *bool   `json:"is_active"`
}

const (
	promptTemplatePreviewStatusQueued    = "queued"
	promptTemplatePreviewStatusRunning   = "running"
	promptTemplatePreviewStatusGenerated = "generated"
	promptTemplatePreviewStatusFailed    = "failed"
	promptTemplatePreviewStatusSkipped   = "skipped"
)

func (a *App) handleListAdminPromptTemplates(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 12), 1), 100)
	query := a.db.Model(&PromptTemplate{})

	if active := strings.TrimSpace(c.Query("active")); active != "" && active != "all" {
		query = query.Where("is_active = ?", active == "true" || active == "1")
	}
	if preview := strings.TrimSpace(c.Query("preview")); preview != "" && preview != "all" {
		if preview == "missing" {
			query = query.Where("COALESCE(preview_asset_key, '') = ''")
		}
		if preview == "generated" {
			query = query.Where("COALESCE(preview_asset_key, '') <> ''")
		}
		if preview == "running" {
			query = query.Where("preview_status IN ?", []string{promptTemplatePreviewStatusQueued, promptTemplatePreviewStatusRunning})
		}
		if preview == "failed" {
			query = query.Where("preview_status = ?", promptTemplatePreviewStatusFailed)
		}
	}
	if search := strings.ToLower(strings.TrimSpace(c.Query("q"))); search != "" {
		like := "%" + search + "%"
		query = query.Where("(LOWER(title) LIKE ? OR LOWER(slug) LIKE ? OR LOWER(category) LIKE ? OR LOWER(description) LIKE ?)", like, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_templates_count_failed", "模板统计失败")
		return
	}
	var templates []PromptTemplate
	if err := query.Order("sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&templates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_templates_load_failed", "模板读取失败")
		return
	}
	items := make([]gin.H, 0, len(templates))
	for _, template := range templates {
		items = append(items, a.adminPromptTemplatePayload(template))
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleCreateAdminPromptTemplate(c *gin.Context) {
	var req adminPromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	template := PromptTemplate{IsActive: true}
	if err := applyAdminPromptTemplateRequest(&template, req, true); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_prompt_template", err.Error())
		return
	}
	if err := a.db.Create(&template).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_template_create_failed", "模板创建失败")
		return
	}
	if req.IsActive != nil && !*req.IsActive {
		if err := a.db.Model(&template).Update("is_active", false).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "prompt_template_create_failed", "模板创建失败")
			return
		}
		template.IsActive = false
	}
	a.writeAdminAudit(c, "prompt_template.create", "prompt_template", template.ID, gin.H{"slug": template.Slug})
	writeJSON(c, http.StatusCreated, a.adminPromptTemplatePayload(template))
}

func (a *App) handleUpdateAdminPromptTemplate(c *gin.Context) {
	var template PromptTemplate
	if err := a.db.First(&template, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "prompt_template_not_found", "模板不存在")
		return
	}
	var req adminPromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyAdminPromptTemplateRequest(&template, req, false); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_prompt_template", err.Error())
		return
	}
	if err := a.db.Save(&template).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_template_save_failed", "模板保存失败")
		return
	}
	a.writeAdminAudit(c, "prompt_template.update", "prompt_template", template.ID, gin.H{"slug": template.Slug, "active": template.IsActive})
	writeJSON(c, http.StatusOK, a.adminPromptTemplatePayload(template))
}

func (a *App) handleDeleteAdminPromptTemplate(c *gin.Context) {
	var template PromptTemplate
	if err := a.db.First(&template, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "prompt_template_not_found", "模板不存在")
		return
	}
	if err := a.db.Delete(&template).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_template_delete_failed", "模板删除失败")
		return
	}
	a.writeAdminAudit(c, "prompt_template.delete", "prompt_template", template.ID, gin.H{"slug": template.Slug})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleGenerateAdminPromptTemplatePreview(c *gin.Context) {
	var template PromptTemplate
	if err := a.db.First(&template, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "prompt_template_not_found", "模板不存在")
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)
	queue, err := a.enqueuePromptTemplatePreviewGeneration(promptTemplatePreviewQueueOptions{
		TemplateIDs: []uint{template.ID},
		Force:       req.Force,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_template_preview_generate_failed", "模板预览生成失败")
		return
	}
	a.writeAdminAudit(c, "prompt_template.preview.generate", "prompt_template", template.ID, gin.H{"force": req.Force, "queued": queue.Queued})
	writeJSON(c, http.StatusAccepted, queue)
}

func (a *App) handleBatchGenerateAdminPromptTemplatePreviews(c *gin.Context) {
	var req struct {
		Limit int  `json:"limit"`
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)
	limit := req.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	queue, err := a.enqueuePromptTemplatePreviewGeneration(promptTemplatePreviewQueueOptions{
		Limit: limit,
		Force: req.Force,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_template_previews_generate_failed", "模板预览生成失败")
		return
	}
	a.writeAdminAudit(c, "prompt_template.previews.generate", "prompt_template", 0, gin.H{"limit": limit, "force": req.Force, "queued": queue.Queued})
	writeJSON(c, http.StatusAccepted, queue)
}

type promptTemplatePreviewQueueOptions struct {
	TemplateIDs []uint
	Limit       int
	Force       bool
}

type promptTemplatePreviewQueuePayload struct {
	Status      string `json:"status"`
	Queued      int    `json:"queued"`
	TemplateIDs []uint `json:"template_ids"`
}

func (a *App) enqueuePromptTemplatePreviewGeneration(opts promptTemplatePreviewQueueOptions) (promptTemplatePreviewQueuePayload, error) {
	query := a.db.Model(&PromptTemplate{}).Order("sort_order asc, id asc")
	if len(opts.TemplateIDs) > 0 {
		query = query.Where("id IN ?", opts.TemplateIDs)
	} else {
		query = query.Where("is_active = ?", true)
	}
	if !opts.Force {
		query = query.Where("COALESCE(preview_asset_key, '') = ''")
		if len(opts.TemplateIDs) == 0 {
			query = query.Where("COALESCE(preview_status, '') <> ?", promptTemplatePreviewStatusFailed)
		}
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	var templates []PromptTemplate
	if err := query.Find(&templates).Error; err != nil {
		return promptTemplatePreviewQueuePayload{}, err
	}
	ids := make([]uint, 0, len(templates))
	for _, template := range templates {
		ids = append(ids, template.ID)
	}
	if len(ids) == 0 {
		return promptTemplatePreviewQueuePayload{Status: "noop", Queued: 0, TemplateIDs: ids}, nil
	}

	now := time.Now()
	if err := a.db.Model(&PromptTemplate{}).Where("id IN ?", ids).Updates(map[string]any{
		"preview_status":           promptTemplatePreviewStatusQueued,
		"preview_error_message":    "",
		"preview_last_started_at":  &now,
		"preview_last_finished_at": nil,
	}).Error; err != nil {
		return promptTemplatePreviewQueuePayload{}, err
	}

	go a.runPromptTemplatePreviewGeneration(ids, opts.Force)
	return promptTemplatePreviewQueuePayload{Status: promptTemplatePreviewStatusQueued, Queued: len(ids), TemplateIDs: ids}, nil
}

func (a *App) runPromptTemplatePreviewGeneration(templateIDs []uint, force bool) {
	ctx := context.Background()
	timeout := time.Duration(len(templateIDs))*4*time.Minute + time.Minute
	if timeout < 5*time.Minute {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := a.GenerateMissingPromptTemplatePreviews(ctx, PromptTemplatePreviewGenerationOptions{
		TemplateIDs:    templateIDs,
		Force:          force,
		PerItemTimeout: 4 * time.Minute,
		Progress:       a.updatePromptTemplatePreviewProgress,
	})
	if err != nil {
		now := time.Now()
		_ = a.db.Model(&PromptTemplate{}).Where("id IN ?", templateIDs).Updates(map[string]any{
			"preview_status":           promptTemplatePreviewStatusFailed,
			"preview_error_message":    err.Error(),
			"preview_last_finished_at": &now,
		}).Error
	}
}

func (a *App) updatePromptTemplatePreviewProgress(item PromptTemplatePreviewGenerationItem) {
	updates := map[string]any{}
	now := time.Now()
	switch item.Status {
	case promptTemplatePreviewStatusRunning:
		updates["preview_status"] = promptTemplatePreviewStatusRunning
		updates["preview_error_message"] = ""
		updates["preview_last_started_at"] = &now
	case promptTemplatePreviewStatusGenerated:
		updates["preview_status"] = promptTemplatePreviewStatusGenerated
		updates["preview_error_message"] = ""
		updates["preview_last_finished_at"] = &now
	case promptTemplatePreviewStatusFailed:
		updates["preview_status"] = promptTemplatePreviewStatusFailed
		updates["preview_error_message"] = strings.TrimSpace(item.Error)
		updates["preview_last_finished_at"] = &now
	case promptTemplatePreviewStatusSkipped:
		updates["preview_status"] = promptTemplatePreviewStatusSkipped
		updates["preview_last_finished_at"] = &now
	default:
		return
	}
	_ = a.db.Model(&PromptTemplate{}).Where("id = ?", item.ID).Updates(updates).Error
}

func applyAdminPromptTemplateRequest(template *PromptTemplate, req adminPromptTemplateRequest, create bool) error {
	if req.Slug != nil {
		template.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.Title != nil {
		template.Title = strings.TrimSpace(*req.Title)
	}
	if req.Category != nil {
		template.Category = strings.TrimSpace(*req.Category)
	}
	if req.Description != nil {
		template.Description = strings.TrimSpace(*req.Description)
	}
	if req.Prompt != nil {
		template.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.AspectRatio != nil {
		template.AspectRatio = strings.TrimSpace(*req.AspectRatio)
	}
	if req.StylePreset != nil {
		template.StylePreset = strings.TrimSpace(*req.StylePreset)
	}
	if req.Theme != nil {
		template.Theme = strings.TrimSpace(*req.Theme)
	}
	if req.WorkspaceSection != nil {
		template.WorkspaceSection = normalizeWorkspaceTemplateSection(*req.WorkspaceSection)
	}
	if req.WorkspaceToolMode != nil {
		template.WorkspaceToolMode = normalizeGenerationToolMode(*req.WorkspaceToolMode)
	}
	if req.WorkspaceModelID != nil {
		template.WorkspaceModelID = *req.WorkspaceModelID
	}
	if req.WorkspaceSort != nil {
		template.WorkspaceSort = *req.WorkspaceSort
	}
	if req.SortOrder != nil {
		template.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		template.IsActive = *req.IsActive
	}
	if create || template.Slug == "" || template.Title == "" || template.Prompt == "" {
		if template.Slug == "" || template.Title == "" || template.Prompt == "" {
			return errInvalidPromptTemplate()
		}
	}
	if template.AspectRatio == "" {
		template.AspectRatio = "1:1"
	}
	if template.WorkspaceToolMode == "" {
		template.WorkspaceToolMode = GenerationToolModeGenerate
	}
	if !isValidWorkspaceTemplateSection(template.WorkspaceSection) {
		return adminPromptTemplateValidationError("工作台展示分区无效")
	}
	if !isValidGenerationToolMode(template.WorkspaceToolMode) {
		return adminPromptTemplateValidationError("工作台工具模式无效")
	}
	return nil
}

func errInvalidPromptTemplate() error {
	return adminPromptTemplateValidationError("模板标识、标题和提示词不能为空")
}

type adminPromptTemplateValidationError string

func (e adminPromptTemplateValidationError) Error() string {
	return string(e)
}

func (a *App) adminPromptTemplatePayload(template PromptTemplate) gin.H {
	return gin.H{
		"id":                          template.ID,
		"slug":                        template.Slug,
		"title":                       template.Title,
		"category":                    template.Category,
		"description":                 template.Description,
		"prompt":                      template.Prompt,
		"aspect_ratio":                template.AspectRatio,
		"style_preset":                template.StylePreset,
		"theme":                       template.Theme,
		"preview_asset_key":           template.PreviewAssetKey,
		"preview_url":                 a.promptTemplatePreviewURL(template),
		"preview_mime_type":           template.PreviewMIMEType,
		"preview_provider_request_id": template.PreviewProviderRequestID,
		"preview_generated_at":        template.PreviewGeneratedAt,
		"preview_status":              a.promptTemplatePreviewStatus(template),
		"preview_error_message":       template.PreviewErrorMessage,
		"preview_last_started_at":     template.PreviewLastStartedAt,
		"preview_last_finished_at":    template.PreviewLastFinishedAt,
		"workspace_section":           template.WorkspaceSection,
		"workspace_tool_mode":         fallbackString(template.WorkspaceToolMode, GenerationToolModeGenerate),
		"workspace_model_id":          template.WorkspaceModelID,
		"workspace_sort":              template.WorkspaceSort,
		"sort_order":                  template.SortOrder,
		"is_active":                   template.IsActive,
		"cost_credits":                1,
		"created_at":                  template.CreatedAt,
		"updated_at":                  template.UpdatedAt,
	}
}

func (a *App) promptTemplatePreviewStatus(template PromptTemplate) string {
	status := strings.TrimSpace(template.PreviewStatus)
	if status != "" {
		return status
	}
	if strings.TrimSpace(template.PreviewAssetKey) != "" || strings.TrimSpace(template.PreviewURL) != "" {
		return promptTemplatePreviewStatusGenerated
	}
	return ""
}
