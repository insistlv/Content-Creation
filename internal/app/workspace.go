package app

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	workspaceTemplateSectionHot         = "hot"
	workspaceTemplateSectionInspiration = "inspiration"
)

type workspaceDiscoveryTemplate struct {
	ID          uint   `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	PreviewURL  string `json:"preview_url"`
	AspectRatio string `json:"aspect_ratio"`
	StylePreset string `json:"style_preset"`
	Theme       string `json:"theme"`
	ToolMode    string `json:"tool_mode"`
	ModelID     uint   `json:"model_id"`
	SortOrder   int    `json:"sort_order"`
}

type workspaceTool struct {
	Mode           string           `json:"mode"`
	Title          string           `json:"title"`
	Description    string           `json:"description"`
	Icon           string           `json:"icon"`
	Enabled        bool             `json:"enabled"`
	SortOrder      int              `json:"sort_order"`
	RequiresSource bool             `json:"requires_source"`
	SourceLimit    int              `json:"source_limit,omitempty"`
	FormSchema     []workspaceField `json:"form_schema"`
}

type workspaceField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Default any      `json:"default,omitempty"`
	Min     *int     `json:"min,omitempty"`
	Max     *int     `json:"max,omitempty"`
	Options []string `json:"options,omitempty"`
	Step    int      `json:"step,omitempty"`
}

type workspaceModel struct {
	ID                 uint     `json:"id"`
	Name               string   `json:"name"`
	DefaultCreditsCost int      `json:"default_credits_cost"`
	CapabilityTags     []string `json:"capability_tags"`
	SortOrder          int      `json:"sort_order"`
}

func (a *App) handleWorkspaceDiscovery(c *gin.Context) {
	var templates []PromptTemplate
	if err := a.db.
		Where("is_active = ? AND workspace_section IN ?", true, []string{workspaceTemplateSectionHot, workspaceTemplateSectionInspiration}).
		Order("workspace_section asc, workspace_sort asc, sort_order asc, id asc").
		Find(&templates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "workspace_discovery_load_failed", "工作台模板读取失败")
		return
	}
	models, err := a.workspaceDiscoveryModels()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "workspace_discovery_load_failed", "工作台模型读取失败")
		return
	}

	recommendations, err := a.workspaceDiscoveryRecommendations()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "workspace_discovery_load_failed", "灵感推荐读取失败")
		return
	}

	hot := make([]workspaceDiscoveryTemplate, 0)
	inspiration := make([]workspaceDiscoveryTemplate, 0)
	for _, template := range templates {
		item := a.workspaceDiscoveryTemplatePayload(template)
		switch normalizeWorkspaceTemplateSection(template.WorkspaceSection) {
		case workspaceTemplateSectionHot:
			hot = append(hot, item)
		case workspaceTemplateSectionInspiration:
			inspiration = append(inspiration, item)
		}
	}

	writeJSON(c, http.StatusOK, gin.H{
		"tools":           workspaceDiscoveryTools(),
		"models":          models,
		"recommendations": recommendations,
		"hot":             hot,
		"inspiration":     inspiration,
	})
}

func (a *App) workspaceDiscoveryTemplatePayload(template PromptTemplate) workspaceDiscoveryTemplate {
	toolMode := normalizeGenerationToolMode(template.WorkspaceToolMode)
	if !isValidGenerationToolMode(toolMode) {
		toolMode = GenerationToolModeGenerate
	}
	sortOrder := template.WorkspaceSort
	if sortOrder == 0 {
		sortOrder = template.SortOrder
	}
	return workspaceDiscoveryTemplate{
		ID:          template.ID,
		Slug:        template.Slug,
		Title:       template.Title,
		Category:    template.Category,
		Description: template.Description,
		Prompt:      template.Prompt,
		PreviewURL:  a.promptTemplatePreviewURL(template),
		AspectRatio: fallbackString(template.AspectRatio, "1:1"),
		StylePreset: template.StylePreset,
		Theme:       template.Theme,
		ToolMode:    toolMode,
		ModelID:     template.WorkspaceModelID,
		SortOrder:   sortOrder,
	}
}

func workspaceDiscoveryTools() []workspaceTool {
	return []workspaceTool{
		{
			Mode:           GenerationToolModeExpand,
			Title:          "智能扩图",
			Description:    "按方向延展画面边界",
			Icon:           "maximize",
			Enabled:        true,
			SortOrder:      10,
			RequiresSource: true,
			SourceLimit:    1,
			FormSchema: []workspaceField{
				numberWorkspaceField("top", "上", 20, 0, 100, 5),
				numberWorkspaceField("bottom", "下", 20, 0, 100, 5),
				numberWorkspaceField("left", "左", 20, 0, 100, 5),
				numberWorkspaceField("right", "右", 20, 0, 100, 5),
			},
		},
		{
			Mode:           GenerationToolModeErase,
			Title:          "移除物体",
			Description:    "清理图中干扰元素",
			Icon:           "eraser",
			Enabled:        true,
			SortOrder:      20,
			RequiresSource: true,
			SourceLimit:    1,
			FormSchema: []workspaceField{
				{Key: "edit_instruction", Label: "移除说明", Type: "textarea"},
				{Key: "mask", Label: "蒙版", Type: "mask"},
			},
		},
		{
			Mode:           GenerationToolModeRemoveBackground,
			Title:          "移除背景",
			Description:    "保留主体轮廓",
			Icon:           "image",
			Enabled:        true,
			SortOrder:      30,
			RequiresSource: true,
			SourceLimit:    1,
			FormSchema: []workspaceField{
				{Key: "edit_instruction", Label: "主体保留说明（可选）", Type: "textarea"},
			},
		},
		{
			Mode:           GenerationToolModeUpscale,
			Title:          "高清放大",
			Description:    "选择倍率增强细节",
			Icon:           "sparkles",
			Enabled:        true,
			SortOrder:      40,
			RequiresSource: true,
			SourceLimit:    1,
			FormSchema: []workspaceField{
				{Key: "scale", Label: "倍率", Type: "select", Default: "2x", Options: []string{"2x", "4x", "8x"}},
				{Key: "edit_instruction", Label: "增强说明（可选）", Type: "textarea"},
			},
		},
		{
			Mode:           GenerationToolModePrecisionEdit,
			Title:          "精细编辑",
			Description:    "圈选局部并输入编辑指令",
			Icon:           "edit",
			Enabled:        true,
			SortOrder:      50,
			RequiresSource: true,
			SourceLimit:    1,
			FormSchema: []workspaceField{
				{Key: "edit_instruction", Label: "编辑指令", Type: "textarea"},
				{Key: "mask", Label: "蒙版", Type: "mask"},
			},
		},
	}
}

func numberWorkspaceField(key, label string, defaultValue, minValue, maxValue, step int) workspaceField {
	return workspaceField{
		Key:     key,
		Label:   label,
		Type:    "number",
		Default: defaultValue,
		Min:     &minValue,
		Max:     &maxValue,
		Step:    step,
	}
}

func (a *App) workspaceDiscoveryModels() ([]workspaceModel, error) {
	var models []ModelCatalog
	err := a.db.
		Where("modality = ? AND status = ? AND visibility = ?", ModelConfigTypeImage, ModelCenterStatusOnline, ModelCenterVisibilityPublic).
		Order("sort_order asc, id asc").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	items := make([]workspaceModel, 0, len(models))
	for _, model := range models {
		tags := model.CapabilityTags
		if tags == nil {
			tags = decodeStringList(model.CapabilityTagsJSON)
		}
		items = append(items, workspaceModel{
			ID:                 model.ID,
			Name:               model.Name,
			DefaultCreditsCost: modelCenterDefaultCreditsCost(&model),
			CapabilityTags:     tags,
			SortOrder:          model.SortOrder,
		})
	}
	return items, nil
}

func normalizeWorkspaceTemplateSection(section string) string {
	switch strings.ToLower(strings.TrimSpace(section)) {
	case workspaceTemplateSectionHot:
		return workspaceTemplateSectionHot
	case workspaceTemplateSectionInspiration:
		return workspaceTemplateSectionInspiration
	default:
		return strings.ToLower(strings.TrimSpace(section))
	}
}

func isValidWorkspaceTemplateSection(section string) bool {
	switch normalizeWorkspaceTemplateSection(section) {
	case "", workspaceTemplateSectionHot, workspaceTemplateSectionInspiration:
		return true
	default:
		return false
	}
}
