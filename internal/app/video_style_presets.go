package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminVideoStylePresetRequest struct {
	Slug            *string   `json:"slug"`
	Title           *string   `json:"title"`
	Category        *string   `json:"category"`
	Description     *string   `json:"description"`
	Tags            *[]string `json:"tags"`
	PreviewAssetKey *string   `json:"preview_asset_key"`
	PreviewURL      *string   `json:"preview_url"`
	StylePrompt     *string   `json:"style_prompt"`
	SortOrder       *int      `json:"sort_order"`
	IsActive        *bool     `json:"is_active"`
}

type userVideoStyleTemplateRequest struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	ReferenceAssetID uint   `json:"reference_asset_id"`
	StylePrompt      string `json:"style_prompt"`
}

func (a *App) seedVideoStylePresets() error {
	defaults := []VideoStylePreset{
		{
			Slug:        "cinematic-realism",
			Title:       "电影感写实",
			Category:    "电影",
			Description: "适合品牌短片、产品发布和人物叙事，强调镜头质感与自然光影。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/precision-edit.png",
			StylePrompt: "cinematic realism, natural lensing, soft film grain, balanced contrast, realistic motion, premium commercial lighting",
			SortOrder:   10,
			IsActive:    true,
		},
		{
			Slug:        "japanese-hand-drawn-animation",
			Title:       "日系手绘动画",
			Category:    "动画",
			Description: "适合温柔叙事、治愈系场景和梦幻自然环境。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/childhood-dream-album.png",
			StylePrompt: "Japanese hand-drawn animation feeling, soft watercolor background, warm sunlight, gentle camera movement, detailed environment, whimsical mood",
			SortOrder:   20,
			IsActive:    true,
		},
		{
			Slug:        "cyber-neon",
			Title:       "赛博霓虹",
			Category:    "科幻",
			Description: "适合未来城市、科技产品、游戏宣传和夜景氛围。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/expand.png",
			StylePrompt: "cyberpunk neon city, wet reflective streets, high contrast blue and magenta lighting, futuristic atmosphere, dramatic camera movement",
			SortOrder:   30,
			IsActive:    true,
		},
		{
			Slug:        "ink-wash-chinese",
			Title:       "国风水墨",
			Category:    "国风",
			Description: "适合古风人物、山水意境、文化宣传和东方奇幻。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/upscale.png",
			StylePrompt: "Chinese ink wash style, flowing brush texture, misty mountains, elegant composition, restrained color palette, poetic motion",
			SortOrder:   40,
			IsActive:    true,
		},
		{
			Slug:        "retro-film",
			Title:       "复古胶片",
			Category:    "摄影",
			Description: "适合生活方式、旅行、怀旧故事和社媒短片。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/remove-background.png",
			StylePrompt: "retro film look, warm tones, soft highlight bloom, visible grain, nostalgic atmosphere, handheld documentary camera",
			SortOrder:   50,
			IsActive:    true,
		},
		{
			Slug:        "commercial-blockbuster",
			Title:       "商业大片",
			Category:    "商业",
			Description: "适合电商产品、品牌广告和高质感宣传片。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/product.png",
			StylePrompt: "premium commercial blockbuster style, clean studio lighting, high-end product texture, smooth dolly shot, polished advertising look",
			SortOrder:   60,
			IsActive:    true,
		},
		{
			Slug:        "clay-stop-motion",
			Title:       "黏土定格",
			Category:    "动画",
			Description: "适合可爱角色、手作产品和趣味短片。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/couple-album.png",
			StylePrompt: "clay stop motion style, handcrafted miniature set, tactile material, playful lighting, charming imperfect motion",
			SortOrder:   70,
			IsActive:    true,
		},
		{
			Slug:        "fantasy-epic",
			Title:       "幻想史诗",
			Category:    "奇幻",
			Description: "适合游戏角色、奇幻世界观和英雄叙事。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/city.png",
			StylePrompt: "epic fantasy cinematic style, grand scale environment, dramatic rim light, magical particles, heroic camera movement",
			SortOrder:   80,
			IsActive:    true,
		},
	}
	for i := range defaults {
		switch defaults[i].Slug {
		case "cinematic-realism":
			_ = defaults[i].SetTags([]string{"推荐", "电影感"})
		case "japanese-hand-drawn-animation":
			_ = defaults[i].SetTags([]string{"新手", "动画"})
		case "commercial-blockbuster":
			_ = defaults[i].SetTags([]string{"电商", "广告"})
		default:
			_ = defaults[i].SetTags([]string{defaults[i].Category})
		}
		var existing VideoStylePreset
		err := a.db.Where("slug = ?", defaults[i].Slug).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := a.db.Create(&defaults[i]).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleListVideoStylePresets(c *gin.Context) {
	var presets []VideoStylePreset
	if err := a.db.Where("is_active = ?", true).Order("sort_order asc, id asc").Find(&presets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_presets_load_failed", "视频风格预设读取失败")
		return
	}
	items := make([]gin.H, 0, len(presets))
	for _, preset := range presets {
		items = append(items, videoStylePresetPayload(preset, false))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleListUserVideoStyleTemplates(c *gin.Context) {
	user := currentUser(c)
	var templates []UserVideoStyleTemplate
	if err := a.db.Where("user_id = ? AND is_active = ?", user.ID, true).Order("created_at desc, id desc").Find(&templates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_templates_load_failed", "视频风格模板读取失败")
		return
	}
	items := make([]gin.H, 0, len(templates))
	for _, template := range templates {
		items = append(items, userVideoStyleTemplatePayload(template))
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleCreateUserVideoStyleTemplate(c *gin.Context) {
	user := currentUser(c)
	var req userVideoStyleTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" || req.ReferenceAssetID == 0 {
		writeError(c, http.StatusUnprocessableEntity, "invalid_video_style_template", "模板名称和风格参考图不能为空")
		return
	}
	if len(title) > 128 {
		writeError(c, http.StatusUnprocessableEntity, "video_style_template_title_too_long", "模板名称最多 128 个字符")
		return
	}
	var asset ReferenceAsset
	if err := a.db.Where("id = ? AND user_id = ?", req.ReferenceAssetID, user.ID).First(&asset).Error; err != nil {
		writeError(c, http.StatusNotFound, "reference_asset_not_found", "风格参考图不存在")
		return
	}
	template := UserVideoStyleTemplate{
		UserID:           user.ID,
		Title:            title,
		Description:      strings.TrimSpace(req.Description),
		ReferenceAssetID: asset.ID,
		PreviewURL:       asset.PreviewURL,
		StylePrompt:      strings.TrimSpace(req.StylePrompt),
		IsActive:         true,
	}
	if template.StylePrompt == "" {
		template.StylePrompt = title
	}
	if err := a.db.Create(&template).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_template_create_failed", "视频风格模板保存失败")
		return
	}
	writeJSON(c, http.StatusCreated, userVideoStyleTemplatePayload(template))
}

func (a *App) handleDeleteUserVideoStyleTemplate(c *gin.Context) {
	user := currentUser(c)
	var template UserVideoStyleTemplate
	if err := a.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&template).Error; err != nil {
		writeError(c, http.StatusNotFound, "video_style_template_not_found", "视频风格模板不存在")
		return
	}
	if err := a.db.Delete(&template).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_template_delete_failed", "视频风格模板删除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (a *App) handleListAdminVideoStylePresets(c *gin.Context) {
	query := a.db.Model(&VideoStylePreset{})
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		like := "%" + q + "%"
		query = query.Where("slug LIKE ? OR title LIKE ? OR category LIKE ? OR style_prompt LIKE ?", like, like, like, like)
	}
	if active := strings.TrimSpace(c.Query("active")); active != "" {
		query = query.Where("is_active = ?", active == "true")
	}
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_presets_count_failed", "视频风格预设统计失败")
		return
	}
	var presets []VideoStylePreset
	if err := query.Order("sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&presets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_presets_load_failed", "视频风格预设读取失败")
		return
	}
	items := make([]gin.H, 0, len(presets))
	for _, preset := range presets {
		items = append(items, videoStylePresetPayload(preset, true))
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleCreateAdminVideoStylePreset(c *gin.Context) {
	var req adminVideoStylePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	preset := VideoStylePreset{IsActive: true}
	if err := applyAdminVideoStylePresetRequest(&preset, req, true); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "invalid_video_style_preset", err.Error())
		return
	}
	if err := a.db.Create(&preset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_preset_create_failed", "视频风格预设保存失败")
		return
	}
	writeJSON(c, http.StatusCreated, videoStylePresetPayload(preset, true))
}

func (a *App) handleUpdateAdminVideoStylePreset(c *gin.Context) {
	var preset VideoStylePreset
	if err := a.db.First(&preset, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "video_style_preset_not_found", "视频风格预设不存在")
		return
	}
	var req adminVideoStylePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyAdminVideoStylePresetRequest(&preset, req, false); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "invalid_video_style_preset", err.Error())
		return
	}
	if err := a.db.Save(&preset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_preset_update_failed", "视频风格预设更新失败")
		return
	}
	writeJSON(c, http.StatusOK, videoStylePresetPayload(preset, true))
}

func (a *App) handleDeleteAdminVideoStylePreset(c *gin.Context) {
	var preset VideoStylePreset
	if err := a.db.First(&preset, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "video_style_preset_not_found", "视频风格预设不存在")
		return
	}
	if err := a.db.Delete(&preset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "video_style_preset_delete_failed", "视频风格预设删除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"deleted": true})
}

func applyAdminVideoStylePresetRequest(preset *VideoStylePreset, req adminVideoStylePresetRequest, create bool) error {
	if req.Slug != nil {
		preset.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.Title != nil {
		preset.Title = strings.TrimSpace(*req.Title)
	}
	if req.Category != nil {
		preset.Category = strings.TrimSpace(*req.Category)
	}
	if req.Description != nil {
		preset.Description = strings.TrimSpace(*req.Description)
	}
	if req.Tags != nil {
		if err := preset.SetTags(*req.Tags); err != nil {
			return err
		}
	}
	if req.PreviewAssetKey != nil {
		preset.PreviewAssetKey = strings.TrimSpace(*req.PreviewAssetKey)
	}
	if req.PreviewURL != nil {
		preset.PreviewURL = strings.TrimSpace(*req.PreviewURL)
	}
	if req.StylePrompt != nil {
		preset.StylePrompt = strings.TrimSpace(*req.StylePrompt)
	}
	if req.SortOrder != nil {
		preset.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		preset.IsActive = *req.IsActive
	}
	if strings.TrimSpace(preset.Slug) == "" || strings.TrimSpace(preset.Title) == "" || strings.TrimSpace(preset.StylePrompt) == "" {
		return errors.New("视频风格标识、标题和风格提示词不能为空")
	}
	if create && strings.TrimSpace(preset.PreviewURL) == "" && strings.TrimSpace(preset.PreviewAssetKey) == "" {
		return errors.New("视频风格预览图不能为空")
	}
	return nil
}

func videoStylePresetPayload(preset VideoStylePreset, includeAdminFields bool) gin.H {
	payload := gin.H{
		"id":           preset.ID,
		"slug":         preset.Slug,
		"title":        preset.Title,
		"category":     preset.Category,
		"description":  preset.Description,
		"tags":         preset.Tags(),
		"preview_url":  preset.PreviewURL,
		"style_prompt": preset.StylePrompt,
		"sort_order":   preset.SortOrder,
		"use_count":    preset.UseCount,
	}
	if includeAdminFields {
		payload["preview_asset_key"] = preset.PreviewAssetKey
		payload["is_active"] = preset.IsActive
		payload["created_at"] = preset.CreatedAt
		payload["updated_at"] = preset.UpdatedAt
	}
	return payload
}

func userVideoStyleTemplatePayload(template UserVideoStyleTemplate) gin.H {
	return gin.H{
		"id":                 template.ID,
		"user_id":            template.UserID,
		"title":              template.Title,
		"description":        template.Description,
		"reference_asset_id": template.ReferenceAssetID,
		"preview_url":        template.PreviewURL,
		"style_prompt":       template.StylePrompt,
		"is_active":          template.IsActive,
		"use_count":          template.UseCount,
		"created_at":         template.CreatedAt,
		"updated_at":         template.UpdatedAt,
	}
}
