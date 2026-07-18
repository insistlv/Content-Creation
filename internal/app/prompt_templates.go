package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (a *App) seedPromptTemplates() error {
	defaults := []PromptTemplate{
		{
			Slug: "chang-an-night-market-app", Title: "长安夜市点单界面", Category: "跨时代 UI", Description: "古代市井和现代移动端产品界面融合，适合生成高信息密度 App 首屏。", AspectRatio: "9:16", StylePreset: "插画", Theme: "tang-ui", SortOrder: 10, IsActive: true,
			Prompt: "长安夜市点单 App 首页界面，古代市井生活与现代移动产品设计融合，顶部显示长安坊市定位，主视觉为胡饼、炙羊肉、葡萄酿推荐卡，商家头像为工笔画掌柜，评分使用铜钱图标，底部导航为首页、食单、跑腿、订单、我的，配色为赭石、石绿、金箔红，碑刻感标题字搭配现代无衬线正文，真实产品设计稿质感，幽默穿越感，9:16。",
		},
		{
			Slug: "city-sleepless-index", Title: "城市失眠指数海报", Category: "数据海报", Description: "把城市夜景、情绪数据和信息图结合成可视化海报。", AspectRatio: "4:3", StylePreset: "海报", Theme: "city-data", SortOrder: 20, IsActive: true,
			Prompt: "一座现代城市的失眠指数数据海报，俯瞰午夜街区与高楼灯光，用热力图标记不同区域的清醒程度，画面包含时间轴、睡眠曲线、咖啡因浓度、夜班人群注释，蓝紫夜色与暖黄色窗光形成对比，版式像城市研究报告封面，信息密度高但层级清晰，4:3。",
		},
		{
			Slug: "song-travel-storyboard", Title: "宋代旅行分镜板", Category: "分镜叙事", Description: "历史人物旅行 Vlog 的分镜板，适合故事感画面。", AspectRatio: "16:9", StylePreset: "国风", Theme: "song-board", SortOrder: 30, IsActive: true,
			Prompt: "北宋文人江南旅行 Vlog 分镜板，六格连续画面展示登船、过桥、听雨、题诗、夜宿、返程，每格含镜头编号、手写旁白和天气标记，水墨山水与现代视频分镜标注融合，淡墨、青绿、米纸底色，画面有真实手稿与影视前期设计感，16:9。",
		},
		{
			Slug: "future-market-wayfinding", Title: "未来菜场导视系统", Category: "导视系统", Description: "社区菜场和未来信息导视结合，适合空间视觉设计。", AspectRatio: "4:3", StylePreset: "赛博朋克", Theme: "market-wayfinding", SortOrder: 40, IsActive: true,
			Prompt: "未来社区菜市场导视系统设计图，湿润地面、透明货架、电子价签与悬浮方向标并存，摊位分区为鲜蔬、海产、熟食、修理铺，导视牌使用霓虹绿与番茄红，包含人流箭头、今日时价、拥挤度和气味提示，真实空间导视提案效果，4:3。",
		},
		{
			Slug: "writer-room-archive", Title: "作家的房间物件档案", Category: "室内陈列", Description: "用档案化方式呈现房间、物品和人物状态。", AspectRatio: "1:1", StylePreset: "写实", Theme: "room-archive", SortOrder: 50, IsActive: true,
			Prompt: "一个作家的房间物件档案图，正午斜光照进狭小书房，桌上有手稿、旧打字机、便利贴、半杯冷咖啡和地图，画面以俯视陈列方式标注物品编号和情绪备注，像博物馆档案与生活摄影结合，安静、真实、细节丰富，1:1。",
		},
		{
			Slug: "moon-store-night-shift", Title: "月球便利店夜班记录", Category: "科幻零售", Description: "月球生活场景和零售小票/监控界面结合。", AspectRatio: "9:16", StylePreset: "写实", Theme: "moon-store", SortOrder: 60, IsActive: true,
			Prompt: "月球社区便利店夜班记录海报，窗外是灰白月面和远处地球，店内售卖氧气罐、压缩饭团、低重力拖鞋和维修胶带，画面叠加监控时间码、夜班小票、库存提醒和店员手写备注，冷白灯与蓝色阴影，科幻生活感强，9:16。",
		},
		{
			Slug: "cyber-herbal-identity", Title: "赛博中药铺品牌提案", Category: "品牌视觉", Description: "传统药铺与赛博品牌识别融合，适合品牌 KV。", AspectRatio: "16:9", StylePreset: "赛博朋克", Theme: "herbal-brand", SortOrder: 70, IsActive: true,
			Prompt: "赛博朋克中药铺品牌识别提案，老木柜、药斗标签、电子脉诊屏和霓虹招牌融合，展示 logo、包装袋、药方票据、门头灯箱和会员卡，色彩为靛蓝、草本绿、朱砂红，传统纹样转译为数字网格，像完整品牌视觉展示板，16:9。",
		},
		{
			Slug: "showa-fan-manual", Title: "昭和风陪伴电风扇说明书", Category: "复古说明书", Description: "复古家电说明书与拟人化陪伴功能结合。", AspectRatio: "3:4", StylePreset: "插画", Theme: "showa-manual", SortOrder: 80, IsActive: true,
			Prompt: "昭和风家电说明书页面，一台会陪人发呆的复古电风扇被拆解成零件图，页面包含开关档位、风向情绪、使用注意、夏夜场景小插图和旧纸张折痕，配色为奶油黄、薄荷绿、褪色红，带轻微印刷网点与怀旧说明书质感，3:4。",
		},
		{
			Slug: "antarctic-kitchen-sop", Title: "南极厨房 SOP 图解", Category: "流程图解", Description: "极地科考站厨房流程，强调图解、步骤和真实环境。", AspectRatio: "4:3", StylePreset: "写实", Theme: "antarctic-sop", SortOrder: 90, IsActive: true,
			Prompt: "南极科考站厨房 SOP 图解海报，不锈钢台面、保温箱、冻干食材和窗外暴风雪，画面用步骤编号展示解冻、配餐、记录热量、清洁消毒流程，包含温度计、库存表和安全提示，真实纪实摄影叠加信息图设计，4:3。",
		},
		{
			Slug: "midnight-bakery-last-batch", Title: "午夜面包房最后一炉", Category: "氛围叙事", Description: "适合生成强氛围、带故事的商业海报。", AspectRatio: "3:4", StylePreset: "写实", Theme: "bakery-night", SortOrder: 100, IsActive: true,
			Prompt: "午夜面包房最后一炉的橱窗海报，街道空旷潮湿，店内暖光照着刚出炉的可颂和吐司，玻璃上有手写今日售罄、凌晨 1:20、明天见，画面带轻微水汽和反射，真实商业摄影与电影海报构图，温暖但孤独，3:4。",
		},
		{
			Slug: "mars-first-week-shopping", Title: "火星新移民购物清单", Category: "清单设计", Description: "科幻生活方式清单，适合社媒图和长图。", AspectRatio: "9:16", StylePreset: "海报", Theme: "mars-list", SortOrder: 110, IsActive: true,
			Prompt: "火星新移民第一周购物清单长图，红色尘土背景上排列生存物资、植物种子、过滤芯、家庭照片框和低重力餐具，每项带价格、重量、必要程度和备注，版式像未来超市购物 App 与纸质清单结合，橙红、银灰、白色高对比，9:16。",
		},
		{
			Slug: "emotion-first-aid-kit", Title: "情绪急救箱说明页", Category: "治愈设计", Description: "把情绪工具做成产品说明页，适合治愈系封面。", AspectRatio: "1:1", StylePreset: "插画", Theme: "emotion-kit", SortOrder: 120, IsActive: true,
			Prompt: "当代人的情绪急救箱说明页，打开的透明收纳盒中有呼吸卡片、耳塞、小夜灯、便利贴、热茶包和空白求助信，画面用柔和图标标注各物品用途，版式像温柔的产品说明书，浅蓝、奶白、柔粉色，干净留白，治愈但不幼稚，1:1。",
		},
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		for index, seed := range defaults {
			if seed.WorkspaceSection == "" {
				if index < 8 {
					seed.WorkspaceSection = "hot"
				} else {
					seed.WorkspaceSection = "inspiration"
				}
			}
			if seed.WorkspaceToolMode == "" {
				seed.WorkspaceToolMode = GenerationToolModeGenerate
			}
			if seed.WorkspaceSort == 0 {
				seed.WorkspaceSort = seed.SortOrder
			}
			var existing PromptTemplate
			err := tx.Unscoped().Where("slug = ?", seed.Slug).First(&existing).Error
			if err == nil {
				if existing.DeletedAt.Valid {
					continue
				}
				updates := map[string]any{
					"title":               seed.Title,
					"category":            seed.Category,
					"description":         seed.Description,
					"prompt":              seed.Prompt,
					"aspect_ratio":        seed.AspectRatio,
					"style_preset":        seed.StylePreset,
					"theme":               seed.Theme,
					"workspace_section":   seed.WorkspaceSection,
					"workspace_tool_mode": seed.WorkspaceToolMode,
					"workspace_sort":      seed.WorkspaceSort,
					"sort_order":          seed.SortOrder,
					"is_active":           seed.IsActive,
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

type promptTemplateListItem struct {
	ID          uint   `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Description string `json:"description"`
	PreviewURL  string `json:"preview_url"`
	AspectRatio string `json:"aspect_ratio"`
	StylePreset string `json:"style_preset"`
	Theme       string `json:"theme"`
	CostCredits int    `json:"cost_credits"`
}

func (a *App) handleListPromptTemplates(c *gin.Context) {
	category := strings.TrimSpace(c.Query("category"))
	query := strings.TrimSpace(c.Query("q"))
	dbQuery := a.db.Where("is_active = ?", true)
	if category != "" && category != "all" {
		dbQuery = dbQuery.Where("category = ?", category)
	}
	if query != "" {
		like := "%" + query + "%"
		dbQuery = dbQuery.Where("title LIKE ? OR description LIKE ?", like, like)
	}

	var templates []PromptTemplate
	if err := dbQuery.Order("sort_order asc, id asc").Find(&templates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "prompt_templates_load_failed", "提示词模板读取失败")
		return
	}

	items := make([]promptTemplateListItem, 0, len(templates))
	for _, item := range templates {
		items = append(items, promptTemplateListItem{
			ID:          item.ID,
			Slug:        item.Slug,
			Title:       item.Title,
			Category:    item.Category,
			Description: item.Description,
			PreviewURL:  a.promptTemplatePreviewURL(item),
			AspectRatio: item.AspectRatio,
			StylePreset: item.StylePreset,
			Theme:       item.Theme,
			CostCredits: 1,
		})
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleUsePromptTemplate(c *gin.Context) {
	user := currentUser(c)
	var template PromptTemplate
	if err := a.db.Where("id = ? AND is_active = ?", c.Param("id"), true).First(&template).Error; err != nil {
		writeError(c, http.StatusNotFound, "prompt_template_not_found", "提示词模板不存在")
		return
	}

	var availableCredits int
	err := a.db.Transaction(func(tx *gorm.DB) error {
		remaining, err := deductGenerationCredits(tx, user.ID, 1)
		if err != nil {
			return err
		}
		availableCredits = remaining
		transaction := CreditTransaction{
			UserID:       user.ID,
			Type:         CreditTransactionTypePromptTemplateUse,
			Amount:       -1,
			BalanceAfter: remaining,
			Reason:       "使用提示词模板",
			RelatedType:  "prompt_template",
			RelatedID:    template.ID,
		}
		return tx.Create(&transaction).Error
	})
	if err != nil {
		if errors.Is(err, errCreditsInsufficient) {
			writeError(c, http.StatusConflict, "credits_insufficient", "点数不足，请先充值")
			return
		}
		writeError(c, http.StatusInternalServerError, "prompt_template_use_failed", "提示词模板使用失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"id":                template.ID,
		"title":             template.Title,
		"prompt":            template.Prompt,
		"aspect_ratio":      template.AspectRatio,
		"style_preset":      template.StylePreset,
		"preview_url":       a.promptTemplatePreviewURL(template),
		"available_credits": availableCredits,
	})
}

func (a *App) handleServePromptTemplatePreview(c *gin.Context) {
	var template PromptTemplate
	if err := a.db.Where("id = ? AND is_active = ?", c.Param("id"), true).First(&template).Error; err != nil {
		writeError(c, http.StatusNotFound, "prompt_template_not_found", "提示词模板不存在")
		return
	}
	if strings.TrimSpace(template.PreviewAssetKey) == "" && strings.TrimSpace(template.PreviewURL) == "" {
		writeError(c, http.StatusNotFound, "prompt_template_preview_not_generated", "模板预览图尚未生成")
		return
	}
	if publicURL := strings.TrimSpace(template.PreviewURL); publicURL != "" {
		c.Redirect(http.StatusFound, publicURL)
		return
	}
	assetKey := strings.TrimSpace(template.PreviewAssetKey)
	if publicURL := strings.TrimSpace(a.assetStore.PublicURL(assetKey)); publicURL != "" {
		c.Redirect(http.StatusFound, publicURL)
		return
	}
	data, err := a.assetStore.Read(assetKey)
	if err != nil {
		writeError(c, http.StatusNotFound, "prompt_template_preview_not_found", "模板预览图不存在")
		return
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, normalizeImageMimeType(template.PreviewMIMEType), data)
}

func (a *App) promptTemplatePreviewURL(template PromptTemplate) string {
	if previewURL := strings.TrimSpace(template.PreviewURL); previewURL != "" {
		return previewURL
	}
	if strings.TrimSpace(template.PreviewAssetKey) == "" {
		return ""
	}
	return fmt.Sprintf("/api/public/prompt-templates/%d/preview", template.ID)
}

type PromptTemplatePreviewGenerationOptions struct {
	Limit          int
	Force          bool
	TemplateIDs    []uint
	PerItemTimeout time.Duration
	Progress       func(PromptTemplatePreviewGenerationItem)
}

type PromptTemplatePreviewGenerationReport struct {
	Scanned   int                                   `json:"scanned"`
	Generated int                                   `json:"generated"`
	Skipped   int                                   `json:"skipped"`
	Failed    int                                   `json:"failed"`
	Items     []PromptTemplatePreviewGenerationItem `json:"items"`
}

type PromptTemplatePreviewGenerationItem struct {
	ID         uint   `json:"id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	PreviewURL string `json:"preview_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (a *App) GenerateMissingPromptTemplatePreviews(ctx context.Context, opts PromptTemplatePreviewGenerationOptions) (PromptTemplatePreviewGenerationReport, error) {
	var report PromptTemplatePreviewGenerationReport
	query := a.db.Model(&PromptTemplate{}).Order("sort_order asc, id asc")
	if len(opts.TemplateIDs) > 0 {
		query = query.Where("id IN ?", opts.TemplateIDs)
	} else {
		query = query.Where("is_active = ?", true)
	}
	if !opts.Force && len(opts.TemplateIDs) == 0 {
		query = query.
			Where("COALESCE(preview_asset_key, '') = ''").
			Where("COALESCE(preview_status, '') <> ?", promptTemplatePreviewStatusFailed)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	var templates []PromptTemplate
	if err := query.Find(&templates).Error; err != nil {
		return report, err
	}
	report.Scanned = len(templates)

	settings, err := a.loadSettings()
	if err != nil {
		return report, err
	}
	modelConfig, err := a.modelConfigForGeneration(settings)
	if err != nil {
		return report, err
	}
	runtimeModel := generationRuntimeModel(settings, modelConfig)
	if runtimeModel == "" {
		runtimeModel = a.cfg.DefaultImageModel
	}

	for _, template := range templates {
		item := PromptTemplatePreviewGenerationItem{ID: template.ID, Slug: template.Slug, Title: template.Title}
		if !opts.Force && strings.TrimSpace(template.PreviewAssetKey) != "" {
			report.Skipped++
			item.Status = "skipped"
			item.PreviewURL = a.promptTemplatePreviewURL(template)
			report.Items = append(report.Items, item)
			notifyPromptTemplatePreviewProgress(opts, item)
			continue
		}
		notifyPromptTemplatePreviewProgress(opts, PromptTemplatePreviewGenerationItem{
			ID:     template.ID,
			Slug:   template.Slug,
			Title:  template.Title,
			Status: "running",
		})

		aspectRatio := fallbackString(strings.TrimSpace(template.AspectRatio), "1:1")
		size, ok := aspectRatioToSize(aspectRatio)
		if !ok {
			aspectRatio = "1:1"
			size, _ = aspectRatioToSize(aspectRatio)
		}
		input := ImageGenerationInput{
			Model:               runtimeModel,
			Prompt:              template.Prompt,
			AspectRatio:         aspectRatio,
			Size:                size,
			Quality:             GenerationQualityMedium,
			StylePreset:         strings.TrimSpace(template.StylePreset),
			ToolMode:            GenerationToolModeGenerate,
			ProviderBaseURL:     modelConfigProviderBaseURL(modelConfig),
			ProviderAPIKey:      modelConfigProviderAPIKey(modelConfig),
			ProviderAPIEndpoint: modelConfigProviderAPIEndpoint(modelConfig),
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
			notifyPromptTemplatePreviewProgress(opts, item)
			continue
		}
		assetKey, mimeType, err := a.assetStore.SaveBase64(result.Base64Image, result.MIMEType)
		if err != nil {
			report.Failed++
			item.Status = "failed"
			item.Error = err.Error()
			report.Items = append(report.Items, item)
			notifyPromptTemplatePreviewProgress(opts, item)
			continue
		}
		now := time.Now()
		previewURL := strings.TrimSpace(a.assetStore.PublicURL(assetKey))
		if err := a.db.Model(&PromptTemplate{}).Where("id = ?", template.ID).Updates(map[string]any{
			"preview_asset_key":           assetKey,
			"preview_url":                 previewURL,
			"preview_mime_type":           normalizeImageMimeType(mimeType),
			"preview_provider_request_id": strings.TrimSpace(result.ProviderRequestID),
			"preview_generated_at":        &now,
			"preview_status":              promptTemplatePreviewStatusGenerated,
			"preview_error_message":       "",
			"preview_last_finished_at":    &now,
		}).Error; err != nil {
			report.Failed++
			item.Status = "failed"
			item.Error = err.Error()
			report.Items = append(report.Items, item)
			notifyPromptTemplatePreviewProgress(opts, item)
			continue
		}
		report.Generated++
		item.Status = "generated"
		item.PreviewURL = previewURL
		report.Items = append(report.Items, item)
		notifyPromptTemplatePreviewProgress(opts, item)
	}
	return report, nil
}

func (a *App) generatePromptTemplatePreviewWithRetries(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	const maxAttempts = 3
	var lastErr *ProviderError
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		leaseToken, err := a.acquireImageExecutionLease(ctx, "prompt-template-preview-"+uuid.NewString(), generationExecutionLeaseMeta{EntryPoint: "prompt_template_preview"})
		if err != nil {
			return ImageGenerationResult{}, &ProviderError{Code: "generation_capacity_unavailable", Message: err.Error(), FailureStage: "waiting_capacity", AttemptCount: attempt}
		}
		stopRenew := make(chan struct{})
		go a.renewImageExecutionLease(leaseToken, stopRenew)
		result, providerErr := a.provider.Generate(ctx, input)
		close(stopRenew)
		a.releaseImageExecutionLease(leaseToken)
		if providerErr == nil {
			if result.ProviderAttemptCount <= 0 {
				result.ProviderAttemptCount = attempt
			}
			return result, nil
		}
		if providerErr.AttemptCount <= 0 {
			providerErr.AttemptCount = attempt
		}
		lastErr = providerErr
		if attempt == maxAttempts || !isRetryableProviderError(providerErr) {
			return ImageGenerationResult{}, providerErr
		}
		select {
		case <-ctx.Done():
			return ImageGenerationResult{}, &ProviderError{
				Code:         "provider_timeout",
				Message:      ctx.Err().Error(),
				FailureStage: providerErr.FailureStage,
				AttemptCount: attempt,
			}
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return ImageGenerationResult{}, lastErr
}

func notifyPromptTemplatePreviewProgress(opts PromptTemplatePreviewGenerationOptions, item PromptTemplatePreviewGenerationItem) {
	if opts.Progress != nil {
		opts.Progress(item)
	}
}
