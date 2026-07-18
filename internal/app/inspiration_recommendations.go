package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var defaultInspirationRecommendationPreviewSlugs = []string{
	"weekly-cyberpunk-city",
	"ecommerce-luxury-product",
	"ancient-fantasy-portrait",
	"social-cafe-poster",
	"game-hero-character",
	"interior-morning-ritual",
	"fashion-key-visual",
	"flat-design-tech-poster",
}

const inspirationRecommendationPreviewPromptSuffix = "用于推荐卡封面，主体清晰，无文字，无水印，无 logo，适合前台卡片裁切。"

type adminInspirationRecommendationRequest struct {
	Slug             *string         `json:"slug"`
	Title            *string         `json:"title"`
	Category         *string         `json:"category"`
	Description      *string         `json:"description"`
	HeatTags         *[]string       `json:"heat_tags"`
	PreviewAssetKey  *string         `json:"preview_asset_key"`
	PreviewURL       *string         `json:"preview_url"`
	Prompt           *string         `json:"prompt"`
	NegativePrompt   *string         `json:"negative_prompt"`
	AspectRatio      *string         `json:"aspect_ratio"`
	StylePreset      *string         `json:"style_preset"`
	Theme            *string         `json:"theme"`
	ToolMode         *string         `json:"tool_mode"`
	WorkspaceModelID *uint           `json:"model_id"`
	Params           *map[string]any `json:"params"`
	SortOrder        *int            `json:"sort_order"`
	IsActive         *bool           `json:"is_active"`
}

func (a *App) seedInspirationRecommendations() error {
	defaults := []InspirationRecommendation{
		{
			Slug:        "weekly-cyberpunk-city",
			Title:       "赛博朋克城市",
			Category:    "概念场景",
			Description: "雨夜霓虹、未来街区与电影感构图。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/5d193370-a00f-4970-ad3e-963154cb6dec.png",
			Prompt:      "赛博朋克未来城市，雨夜街道，霓虹灯牌反射在湿润地面，高楼之间有悬浮交通，电影级广角构图，强对比光影，超清细节",
			AspectRatio: "16:9",
			StylePreset: "电影感",
			Theme:       "cyber-city",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   10,
			IsActive:    true,
		},
		{
			Slug:        "ecommerce-luxury-product",
			Title:       "高级产品主图",
			Category:    "电商营销",
			Description: "干净背景、商业摄影质感和精致布光。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/2ba7e4a8-db28-48be-86b7-6c8c8b998c0f.png",
			Prompt:      "高端护肤品商业主图，透明玻璃瓶，柔和棚拍灯光，水滴与丝绸背景，干净高级，电商详情页视觉，真实摄影质感",
			AspectRatio: "1:1",
			StylePreset: "电商",
			Theme:       "luxury-product",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   20,
			IsActive:    true,
		},
		{
			Slug:        "ancient-fantasy-portrait",
			Title:       "国风角色肖像",
			Category:    "角色设定",
			Description: "适合头像、角色设定和短剧封面。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/33c4510f-4009-47e1-bc4f-edab9aaeca59.png",
			Prompt:      "国风幻想女性角色肖像，银白长发，精致刺绣服饰，月光与竹影，微风飘动薄纱，细腻面部，唯美插画质感",
			AspectRatio: "9:16",
			StylePreset: "国风",
			Theme:       "fantasy-portrait",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   30,
			IsActive:    true,
		},
		{
			Slug:        "social-cafe-poster",
			Title:       "咖啡店社媒海报",
			Category:    "社媒海报",
			Description: "适合朋友圈、小红书和本地生活推广。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/14c67a02-9df6-4bb1-818a-5bea3fcf712b.png",
			Prompt:      "社区咖啡店晨间宣传海报，木质桌面，拿铁咖啡，阳光穿过窗帘，温暖生活方式摄影，留出文字排版空间",
			AspectRatio: "3:4",
			StylePreset: "写实",
			Theme:       "cafe-social",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   40,
			IsActive:    true,
		},
		{
			Slug:        "game-hero-character",
			Title:       "游戏英雄立绘",
			Category:    "动漫游戏",
			Description: "强动态、装备细节和战斗氛围。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/b69a1a9c-3cd3-4173-835b-2ebcbf21418b.png",
			Prompt:      "幻想游戏英雄角色立绘，蓝色能量铠甲，手持长剑，冰雪战场背景，动态姿势，精致装备细节，主机游戏宣传图质感",
			AspectRatio: "2:3",
			StylePreset: "动漫",
			Theme:       "game-hero",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   50,
			IsActive:    true,
		},
		{
			Slug:        "interior-morning-ritual",
			Title:       "室内生活方式图",
			Category:    "建筑及室内",
			Description: "家居、电商和生活方式内容通用。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/7529384d-f74d-47a6-affe-ea16a516be3a.png",
			Prompt:      "清晨卧室生活方式摄影，亚麻衣物挂在木衣架上，床头咖啡和绿植，柔和阳光，暖色调，真实家居杂志风格",
			AspectRatio: "4:3",
			StylePreset: "写实",
			Theme:       "morning-ritual",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   60,
			IsActive:    true,
		},
		{
			Slug:        "fashion-key-visual",
			Title:       "时装品牌大片",
			Category:    "摄影写真",
			Description: "高级模特姿态、材质细节和品牌感。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/887af50f-df93-408b-8498-84cabdc52c6a.png",
			Prompt:      "时装品牌广告大片，模特穿极简剪裁连衣裙，地中海石墙背景，自然硬光，高级商业摄影，胶片色彩",
			AspectRatio: "16:9",
			StylePreset: "摄影",
			Theme:       "fashion-kv",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   70,
			IsActive:    true,
		},
		{
			Slug:        "flat-design-tech-poster",
			Title:       "科技平面海报",
			Category:    "平面设计",
			Description: "适合活动封面、启动页和品牌 KV。",
			PreviewURL:  "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/06/aa052113-e62d-4ae9-a597-6241cfc4a27d.png",
			Prompt:      "科技产品发布会主视觉海报，抽象几何光束，深色背景，蓝绿色高光，中心留出产品标题区域，现代平面设计",
			AspectRatio: "16:9",
			StylePreset: "平面设计",
			Theme:       "tech-poster",
			ToolMode:    GenerationToolModeGenerate,
			SortOrder:   80,
			IsActive:    true,
		},
	}
	heatTags := [][]string{
		{"本周最热", "新手推荐"},
		{"电商爆款", "高转化"},
		{"热门角色", "短剧封面"},
		{"社媒常用", "生活方式"},
		{"游戏角色", "高细节"},
		{"室内灵感", "暖色调"},
		{"摄影写真", "品牌感"},
		{"平面设计", "活动封面"},
	}

	return a.db.Transaction(func(tx *gorm.DB) error {
		for index, seed := range defaults {
			if err := seed.SetHeatTags(heatTags[index]); err != nil {
				return err
			}
			if err := seed.SetParams(map[string]any{}); err != nil {
				return err
			}
			var existing InspirationRecommendation
			err := tx.Unscoped().Where("slug = ?", seed.Slug).First(&existing).Error
			if err == nil {
				if existing.DeletedAt.Valid {
					continue
				}
				updates := map[string]any{
					"title":           seed.Title,
					"category":        seed.Category,
					"description":     seed.Description,
					"heat_tags_json":  seed.HeatTagsJSON,
					"preview_url":     seed.PreviewURL,
					"prompt":          seed.Prompt,
					"negative_prompt": seed.NegativePrompt,
					"aspect_ratio":    seed.AspectRatio,
					"style_preset":    seed.StylePreset,
					"theme":           seed.Theme,
					"tool_mode":       seed.ToolMode,
					"params_json":     seed.ParamsJSON,
					"sort_order":      seed.SortOrder,
					"is_active":       seed.IsActive,
				}
				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return err
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(&seed).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *App) workspaceDiscoveryRecommendations() ([]gin.H, error) {
	var recommendations []InspirationRecommendation
	if err := a.db.
		Where("is_active = ?", true).
		Order("sort_order asc, id asc").
		Find(&recommendations).Error; err != nil {
		return nil, err
	}
	items := make([]gin.H, 0, len(recommendations))
	for _, recommendation := range recommendations {
		items = append(items, a.inspirationRecommendationPayload(recommendation, false))
	}
	return items, nil
}

func (a *App) handleUseInspirationRecommendation(c *gin.Context) {
	var recommendation InspirationRecommendation
	if err := a.db.Where("id = ? AND is_active = ?", c.Param("id"), true).First(&recommendation).Error; err != nil {
		writeError(c, http.StatusNotFound, "inspiration_recommendation_not_found", "灵感推荐不存在")
		return
	}
	if err := a.db.Model(&recommendation).UpdateColumn("use_count", gorm.Expr("use_count + ?", 1)).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "inspiration_recommendation_use_failed", "灵感推荐统计失败")
		return
	}
	recommendation.UseCount++
	writeJSON(c, http.StatusOK, gin.H{
		"id":        recommendation.ID,
		"use_count": recommendation.UseCount,
	})
}

func (a *App) handleListAdminInspirationRecommendations(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 12), 1), 100)
	query := a.db.Model(&InspirationRecommendation{})
	if active := strings.TrimSpace(c.Query("active")); active != "" && active != "all" {
		query = query.Where("is_active = ?", active == "true" || active == "1")
	}
	if search := strings.ToLower(strings.TrimSpace(c.Query("q"))); search != "" {
		like := "%" + search + "%"
		query = query.Where("(LOWER(title) LIKE ? OR LOWER(slug) LIKE ? OR LOWER(category) LIKE ? OR LOWER(description) LIKE ?)", like, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "inspiration_recommendations_count_failed", "灵感推荐统计失败")
		return
	}
	var recommendations []InspirationRecommendation
	if err := query.Order("sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&recommendations).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "inspiration_recommendations_load_failed", "灵感推荐读取失败")
		return
	}
	items := make([]gin.H, 0, len(recommendations))
	for _, recommendation := range recommendations {
		items = append(items, a.inspirationRecommendationPayload(recommendation, true))
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleCreateAdminInspirationRecommendation(c *gin.Context) {
	var req adminInspirationRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	recommendation := InspirationRecommendation{IsActive: true}
	if err := applyAdminInspirationRecommendationRequest(&recommendation, req, true); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_inspiration_recommendation", err.Error())
		return
	}
	if err := a.db.Create(&recommendation).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "inspiration_recommendation_create_failed", "灵感推荐创建失败")
		return
	}
	a.writeAdminAudit(c, "inspiration_recommendation.create", "inspiration_recommendation", recommendation.ID, gin.H{"slug": recommendation.Slug})
	writeJSON(c, http.StatusCreated, a.inspirationRecommendationPayload(recommendation, true))
}

func (a *App) handleUpdateAdminInspirationRecommendation(c *gin.Context) {
	var recommendation InspirationRecommendation
	if err := a.db.First(&recommendation, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "inspiration_recommendation_not_found", "灵感推荐不存在")
		return
	}
	var req adminInspirationRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyAdminInspirationRecommendationRequest(&recommendation, req, false); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_inspiration_recommendation", err.Error())
		return
	}
	if err := a.db.Save(&recommendation).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "inspiration_recommendation_save_failed", "灵感推荐保存失败")
		return
	}
	a.writeAdminAudit(c, "inspiration_recommendation.update", "inspiration_recommendation", recommendation.ID, gin.H{"slug": recommendation.Slug, "active": recommendation.IsActive})
	writeJSON(c, http.StatusOK, a.inspirationRecommendationPayload(recommendation, true))
}

func (a *App) handleDeleteAdminInspirationRecommendation(c *gin.Context) {
	var recommendation InspirationRecommendation
	if err := a.db.First(&recommendation, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "inspiration_recommendation_not_found", "灵感推荐不存在")
		return
	}
	if err := a.db.Delete(&recommendation).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "inspiration_recommendation_delete_failed", "灵感推荐删除失败")
		return
	}
	a.writeAdminAudit(c, "inspiration_recommendation.delete", "inspiration_recommendation", recommendation.ID, gin.H{"slug": recommendation.Slug})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func applyAdminInspirationRecommendationRequest(recommendation *InspirationRecommendation, req adminInspirationRecommendationRequest, create bool) error {
	if req.Slug != nil {
		recommendation.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.Title != nil {
		recommendation.Title = strings.TrimSpace(*req.Title)
	}
	if req.Category != nil {
		recommendation.Category = strings.TrimSpace(*req.Category)
	}
	if req.Description != nil {
		recommendation.Description = strings.TrimSpace(*req.Description)
	}
	if req.HeatTags != nil {
		if err := recommendation.SetHeatTags(*req.HeatTags); err != nil {
			return err
		}
	}
	if req.PreviewAssetKey != nil {
		recommendation.PreviewAssetKey = strings.TrimSpace(*req.PreviewAssetKey)
	}
	if req.PreviewURL != nil {
		recommendation.PreviewURL = strings.TrimSpace(*req.PreviewURL)
	}
	if req.Prompt != nil {
		recommendation.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.NegativePrompt != nil {
		recommendation.NegativePrompt = strings.TrimSpace(*req.NegativePrompt)
	}
	if req.AspectRatio != nil {
		recommendation.AspectRatio = strings.TrimSpace(*req.AspectRatio)
	}
	if req.StylePreset != nil {
		recommendation.StylePreset = strings.TrimSpace(*req.StylePreset)
	}
	if req.Theme != nil {
		recommendation.Theme = strings.TrimSpace(*req.Theme)
	}
	if req.ToolMode != nil {
		recommendation.ToolMode = normalizeGenerationToolMode(*req.ToolMode)
	}
	if req.WorkspaceModelID != nil {
		recommendation.WorkspaceModelID = *req.WorkspaceModelID
	}
	if req.Params != nil {
		if err := recommendation.SetParams(*req.Params); err != nil {
			return err
		}
	}
	if req.SortOrder != nil {
		recommendation.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		recommendation.IsActive = *req.IsActive
	}
	if create || recommendation.Slug == "" || recommendation.Title == "" || recommendation.Prompt == "" {
		if recommendation.Slug == "" || recommendation.Title == "" || recommendation.Prompt == "" {
			return adminInspirationRecommendationValidationError("推荐标识、标题和提示词不能为空")
		}
	}
	if strings.TrimSpace(recommendation.PreviewURL) == "" && strings.TrimSpace(recommendation.PreviewAssetKey) == "" {
		return adminInspirationRecommendationValidationError("推荐预览图不能为空")
	}
	if recommendation.AspectRatio == "" {
		recommendation.AspectRatio = "1:1"
	}
	if recommendation.ToolMode == "" {
		recommendation.ToolMode = GenerationToolModeGenerate
	}
	if !isValidGenerationToolMode(recommendation.ToolMode) {
		return adminInspirationRecommendationValidationError("推荐工具模式无效")
	}
	if strings.TrimSpace(recommendation.HeatTagsJSON) == "" {
		if err := recommendation.SetHeatTags(nil); err != nil {
			return err
		}
	}
	if strings.TrimSpace(recommendation.ParamsJSON) == "" {
		if err := recommendation.SetParams(nil); err != nil {
			return err
		}
	}
	return nil
}

type adminInspirationRecommendationValidationError string

func (e adminInspirationRecommendationValidationError) Error() string {
	return string(e)
}

func (a *App) inspirationRecommendationPayload(recommendation InspirationRecommendation, includeAdminFields bool) gin.H {
	previewURL := strings.TrimSpace(recommendation.PreviewURL)
	if previewURL == "" && strings.TrimSpace(recommendation.PreviewAssetKey) != "" && a.assetStore != nil {
		previewURL = strings.TrimSpace(a.assetStore.PublicURL(recommendation.PreviewAssetKey))
	}
	toolMode := normalizeGenerationToolMode(recommendation.ToolMode)
	if !isValidGenerationToolMode(toolMode) {
		toolMode = GenerationToolModeGenerate
	}
	payload := gin.H{
		"id":              recommendation.ID,
		"slug":            recommendation.Slug,
		"title":           recommendation.Title,
		"category":        recommendation.Category,
		"description":     recommendation.Description,
		"heat_tags":       recommendation.HeatTags(),
		"preview_url":     previewURL,
		"prompt":          recommendation.Prompt,
		"negative_prompt": recommendation.NegativePrompt,
		"aspect_ratio":    fallbackString(recommendation.AspectRatio, "1:1"),
		"style_preset":    recommendation.StylePreset,
		"theme":           recommendation.Theme,
		"tool_mode":       toolMode,
		"model_id":        recommendation.WorkspaceModelID,
		"params":          recommendation.Params(),
		"sort_order":      recommendation.SortOrder,
		"use_count":       recommendation.UseCount,
	}
	if includeAdminFields {
		payload["preview_asset_key"] = recommendation.PreviewAssetKey
		payload["is_active"] = recommendation.IsActive
		payload["view_count"] = recommendation.ViewCount
		payload["created_at"] = recommendation.CreatedAt
		payload["updated_at"] = recommendation.UpdatedAt
	}
	return payload
}

type InspirationRecommendationPreviewGenerationOptions struct {
	Limit          int
	Force          bool
	Slugs          []string
	Quality        string
	PerItemTimeout time.Duration
	DryRun         bool
	Progress       func(InspirationRecommendationPreviewGenerationItem)
}

type InspirationRecommendationPreviewGenerationReport struct {
	Scanned   int                                              `json:"scanned"`
	Generated int                                              `json:"generated"`
	Skipped   int                                              `json:"skipped"`
	Failed    int                                              `json:"failed"`
	Items     []InspirationRecommendationPreviewGenerationItem `json:"items"`
}

type InspirationRecommendationPreviewGenerationItem struct {
	ID         uint   `json:"id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	PreviewURL string `json:"preview_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

type inspirationRecommendationPreviewProviderRoute struct {
	RuntimeModel        string
	ProviderBaseURL     string
	ProviderAPIKey      string
	ProviderAPIEndpoint string
}

func (a *App) GenerateMissingInspirationRecommendationPreviews(ctx context.Context, opts InspirationRecommendationPreviewGenerationOptions) (InspirationRecommendationPreviewGenerationReport, error) {
	var report InspirationRecommendationPreviewGenerationReport
	slugs := normalizeInspirationRecommendationPreviewSlugs(opts.Slugs)
	if len(slugs) == 0 {
		slugs = append([]string(nil), defaultInspirationRecommendationPreviewSlugs...)
	}

	query := a.db.Model(&InspirationRecommendation{}).
		Where("is_active = ?", true).
		Where("slug IN ?", slugs).
		Order("sort_order asc, id asc")
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	var recommendations []InspirationRecommendation
	if err := query.Find(&recommendations).Error; err != nil {
		return report, err
	}
	report.Scanned = len(recommendations)

	quality := normalizeGenerationQuality(opts.Quality)
	if opts.Quality == "" {
		quality = GenerationQualityHigh
	}
	if !isValidGenerationQuality(quality) {
		quality = GenerationQualityHigh
	}

	var route inspirationRecommendationPreviewProviderRoute
	if !opts.DryRun {
		var err error
		settings, err := a.loadSettings()
		if err != nil {
			return report, err
		}
		route, err = a.inspirationRecommendationPreviewProviderRoute(settings)
		if err != nil {
			return report, err
		}
	}

	for _, recommendation := range recommendations {
		item := InspirationRecommendationPreviewGenerationItem{ID: recommendation.ID, Slug: recommendation.Slug, Title: recommendation.Title}
		if !opts.Force && hasInspirationRecommendationPreview(recommendation) {
			report.Skipped++
			item.Status = "skipped"
			item.PreviewURL = a.inspirationRecommendationPreviewURL(recommendation)
			report.Items = append(report.Items, item)
			notifyInspirationRecommendationPreviewProgress(opts, item)
			continue
		}
		if opts.DryRun {
			report.Skipped++
			item.Status = "dry_run"
			item.PreviewURL = a.inspirationRecommendationPreviewURL(recommendation)
			report.Items = append(report.Items, item)
			notifyInspirationRecommendationPreviewProgress(opts, item)
			continue
		}
		notifyInspirationRecommendationPreviewProgress(opts, InspirationRecommendationPreviewGenerationItem{
			ID:     recommendation.ID,
			Slug:   recommendation.Slug,
			Title:  recommendation.Title,
			Status: "running",
		})

		aspectRatio := fallbackString(strings.TrimSpace(recommendation.AspectRatio), "1:1")
		size, ok := aspectRatioToSize(aspectRatio)
		if !ok {
			aspectRatio = "1:1"
			size, _ = aspectRatioToSize(aspectRatio)
		}
		input := ImageGenerationInput{
			Model:               route.RuntimeModel,
			Prompt:              inspirationRecommendationPreviewPrompt(recommendation.Prompt),
			NegativePrompt:      strings.TrimSpace(recommendation.NegativePrompt),
			AspectRatio:         aspectRatio,
			Size:                size,
			Quality:             quality,
			StylePreset:         strings.TrimSpace(recommendation.StylePreset),
			ToolMode:            GenerationToolModeGenerate,
			ProviderBaseURL:     route.ProviderBaseURL,
			ProviderAPIKey:      route.ProviderAPIKey,
			ProviderAPIEndpoint: route.ProviderAPIEndpoint,
		}
		itemCtx := ctx
		cancel := func() {}
		if opts.PerItemTimeout > 0 {
			itemCtx, cancel = context.WithTimeout(ctx, opts.PerItemTimeout)
		}
		result, providerErr := a.generatePromptTemplatePreviewWithRetries(itemCtx, input)
		cancel()
		if providerErr != nil {
			report.Failed++
			item.Status = "failed"
			item.Error = strings.TrimSpace(providerErr.Message)
			if item.Error == "" {
				item.Error = providerErr.Code
			}
			report.Items = append(report.Items, item)
			notifyInspirationRecommendationPreviewProgress(opts, item)
			continue
		}
		assetKey, _, err := a.assetStore.SaveBase64(result.Base64Image, result.MIMEType)
		if err != nil {
			report.Failed++
			item.Status = "failed"
			item.Error = err.Error()
			report.Items = append(report.Items, item)
			notifyInspirationRecommendationPreviewProgress(opts, item)
			continue
		}
		previewURL := strings.TrimSpace(a.assetStore.PublicURL(assetKey))
		if err := a.db.Model(&InspirationRecommendation{}).Where("id = ?", recommendation.ID).Updates(map[string]any{
			"preview_asset_key": assetKey,
			"preview_url":       previewURL,
		}).Error; err != nil {
			report.Failed++
			item.Status = "failed"
			item.Error = err.Error()
			report.Items = append(report.Items, item)
			notifyInspirationRecommendationPreviewProgress(opts, item)
			continue
		}
		report.Generated++
		item.Status = "generated"
		item.PreviewURL = previewURL
		report.Items = append(report.Items, item)
		notifyInspirationRecommendationPreviewProgress(opts, item)
	}
	return report, nil
}

func (a *App) inspirationRecommendationPreviewProviderRoute(settings AppSettings) (inspirationRecommendationPreviewProviderRoute, error) {
	candidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	if err != nil {
		return inspirationRecommendationPreviewProviderRoute{}, err
	}
	if len(candidates) > 0 {
		candidate := candidates[0]
		runtimeModel := modelCenterRuntimeModel(&candidate)
		if runtimeModel == "" {
			runtimeModel = a.cfg.DefaultImageModel
		}
		return inspirationRecommendationPreviewProviderRoute{
			RuntimeModel:        runtimeModel,
			ProviderBaseURL:     modelCenterProviderBaseURL(&candidate),
			ProviderAPIKey:      modelCenterProviderAPIKey(&candidate),
			ProviderAPIEndpoint: modelCenterProviderEndpoint(&candidate),
		}, nil
	}

	modelConfig, err := a.modelConfigForGeneration(settings)
	if err != nil {
		return inspirationRecommendationPreviewProviderRoute{}, err
	}
	runtimeModel := generationRuntimeModel(settings, modelConfig)
	if runtimeModel == "" {
		runtimeModel = a.cfg.DefaultImageModel
	}
	return inspirationRecommendationPreviewProviderRoute{
		RuntimeModel:        runtimeModel,
		ProviderBaseURL:     modelConfigProviderBaseURL(modelConfig),
		ProviderAPIKey:      modelConfigProviderAPIKey(modelConfig),
		ProviderAPIEndpoint: modelConfigProviderAPIEndpoint(modelConfig),
	}, nil
}

func normalizeInspirationRecommendationPreviewSlugs(values []string) []string {
	seen := map[string]bool{}
	slugs := make([]string, 0, len(values))
	for _, value := range values {
		slug := strings.TrimSpace(value)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		slugs = append(slugs, slug)
	}
	return slugs
}

func hasInspirationRecommendationPreview(recommendation InspirationRecommendation) bool {
	return strings.TrimSpace(recommendation.PreviewAssetKey) != "" || strings.TrimSpace(recommendation.PreviewURL) != ""
}

func (a *App) inspirationRecommendationPreviewURL(recommendation InspirationRecommendation) string {
	if previewURL := strings.TrimSpace(recommendation.PreviewURL); previewURL != "" {
		return previewURL
	}
	if assetKey := strings.TrimSpace(recommendation.PreviewAssetKey); assetKey != "" && a.assetStore != nil {
		return strings.TrimSpace(a.assetStore.PublicURL(assetKey))
	}
	return ""
}

func inspirationRecommendationPreviewPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return inspirationRecommendationPreviewPromptSuffix
	}
	return prompt + "\n\n" + inspirationRecommendationPreviewPromptSuffix
}

func notifyInspirationRecommendationPreviewProgress(opts InspirationRecommendationPreviewGenerationOptions, item InspirationRecommendationPreviewGenerationItem) {
	if opts.Progress != nil {
		opts.Progress(item)
	}
}
