package app

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminCoupleAlbumOptionRequest struct {
	Type        *string `json:"type"`
	Value       *string `json:"value"`
	Label       *string `json:"label"`
	Description *string `json:"description"`
	ImageURL    *string `json:"image_url"`
	IconURL     *string `json:"icon_url"`
	PromptLabel *string `json:"prompt_label"`
	SortOrder   *int    `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

func (a *App) seedCoupleAlbumOptions() error {
	defaults := []CoupleAlbumOption{
		{Type: CoupleAlbumOptionTypeLocation, Value: "大理", Label: "大理洱海", Description: "风吹洱海的蓝色午后", ImageURL: "/static/couple-album/dali-erhai.png", PromptLabel: "大理洱海", SortOrder: 10, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "京都", Label: "京都樱花", Description: "樱花雨里的慢镜头", ImageURL: "/static/couple-album/kyoto-sakura.png", PromptLabel: "京都樱花", SortOrder: 20, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "巴黎", Label: "巴黎街角", Description: "转角遇见电影感", ImageURL: "/static/couple-album/paris-corner.png", PromptLabel: "巴黎街角", SortOrder: 30, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "厦门", Label: "厦门海岸", Description: "海风、落日和并肩", ImageURL: "/static/couple-album/xiamen-coast.png", PromptLabel: "厦门海岸", SortOrder: 40, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "上海", Label: "上海夜景", Description: "灯光把故事点亮", ImageURL: "/static/couple-album/shanghai-night.png", PromptLabel: "上海夜景", SortOrder: 50, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "childhood_dream_stage", Label: "童年梦想舞台", Description: "六一职业梦想相册", ImageURL: "/static/home-replica/couple-album-book.png", PromptLabel: "六一儿童节梦想舞台", SortOrder: 60, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "childhood_space_adventure", Label: "星际探索之旅", Description: "火箭、星球和小小宇航员", ImageURL: "/static/childhood-dream-album/theme-space.png", PromptLabel: "星际探索之旅", SortOrder: 61, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "childhood_fairy_tale", Label: "童话奇遇记", Description: "城堡、魔法森林和奇妙伙伴", ImageURL: "/static/childhood-dream-album/theme-fairy-tale.png", PromptLabel: "童话奇遇记", SortOrder: 62, IsActive: true},
		{Type: CoupleAlbumOptionTypeLocation, Value: "childhood_nature_explorer", Label: "自然小达人", Description: "森林、昆虫和自然观察", ImageURL: "/static/childhood-dream-album/theme-nature.png", PromptLabel: "自然小达人", SortOrder: 63, IsActive: true},
		{Type: CoupleAlbumOptionTypeStoryTemplate, Value: "city_walk", Label: "城市漫游", Description: "街角、咖啡和夜色", IconURL: "/static/icons/works.png", PromptLabel: "城市漫游", SortOrder: 10, IsActive: true},
		{Type: CoupleAlbumOptionTypeStoryTemplate, Value: "first_trip", Label: "初次旅行", Description: "出发那天的心动", IconURL: "/static/icons/image.png", PromptLabel: "第一次旅行", SortOrder: 20, IsActive: true},
		{Type: CoupleAlbumOptionTypeStoryTemplate, Value: "anniversary", Label: "纪念日", Description: "520 与每个重要日子", IconURL: "/static/icons/favorite.png", PromptLabel: "周年纪念", SortOrder: 30, IsActive: true},
		{Type: CoupleAlbumOptionTypeStoryTemplate, Value: "proposal", Label: "求婚时刻", Description: "黄昏、灯光和仪式感", IconURL: "/static/icons/generate.png", PromptLabel: "求婚纪念", SortOrder: 40, IsActive: true},
		{Type: CoupleAlbumOptionTypeStoryTemplate, Value: "childhood_career_dream", Label: "童年职业梦想", Description: "六一 8 页职业梦想故事", IconURL: "/static/icons/logo-star.png", PromptLabel: "童年职业梦想相册", SortOrder: 50, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "film", Label: "旅行胶片", IconURL: "/static/icons/photo.png", PromptLabel: "旅行胶片", SortOrder: 10, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "cinematic", Label: "电影旅拍", IconURL: "/static/icons/image-image.png", PromptLabel: "电影旅拍", SortOrder: 20, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "watercolor", Label: "清透水彩", IconURL: "/static/icons/illustration.png", PromptLabel: "清透水彩", SortOrder: 30, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "storybook", Label: "绘本相册", IconURL: "/static/icons/prompt.png", PromptLabel: "绘本相册", SortOrder: 40, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "children_storybook", Label: "童话绘本", IconURL: "/static/icons/illustration.png", PromptLabel: "童话绘本", SortOrder: 50, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "dreamy_watercolor", Label: "梦幻水彩", IconURL: "/static/icons/guofeng.png", PromptLabel: "梦幻水彩", SortOrder: 60, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "animation_3d", Label: "3D 动画电影", IconURL: "/static/icons/image-image.png", PromptLabel: "3D 动画电影", SortOrder: 70, IsActive: true},
		{Type: CoupleAlbumOptionTypeStyle, Value: "children_photo_poster", Label: "儿童写真海报", IconURL: "/static/icons/photo.png", PromptLabel: "儿童写真海报", SortOrder: 80, IsActive: true},
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		for _, seed := range defaults {
			var existing CoupleAlbumOption
			err := tx.Unscoped().Where("type = ? AND value = ?", seed.Type, seed.Value).First(&existing).Error
			if err == nil {
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

func (a *App) handleListCoupleAlbumOptions(c *gin.Context) {
	payload, err := a.coupleAlbumOptionsPayload(true)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_options_load_failed", "相册配置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, payload)
}

func (a *App) handleListAdminCoupleAlbumOptions(c *gin.Context) {
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 100), 1), 100)
	query := a.db.Model(&CoupleAlbumOption{})
	if optionType := strings.TrimSpace(c.Query("type")); optionType != "" && optionType != "all" {
		query = query.Where("type = ?", optionType)
	}
	if active := strings.TrimSpace(c.Query("active")); active != "" && active != "all" {
		query = query.Where("is_active = ?", active == "true" || active == "1")
	}
	if search := strings.ToLower(strings.TrimSpace(c.Query("q"))); search != "" {
		like := "%" + search + "%"
		query = query.Where("(LOWER(value) LIKE ? OR LOWER(label) LIKE ? OR LOWER(description) LIKE ? OR LOWER(prompt_label) LIKE ?)", like, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_options_count_failed", "相册配置统计失败")
		return
	}
	var items []CoupleAlbumOption
	if err := query.Order("type asc, sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_options_load_failed", "相册配置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (a *App) handleCreateAdminCoupleAlbumOption(c *gin.Context) {
	var req adminCoupleAlbumOptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	option := CoupleAlbumOption{IsActive: true}
	if err := applyAdminCoupleAlbumOptionRequest(&option, req, true); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_couple_album_option", err.Error())
		return
	}
	if err := a.db.Create(&option).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_option_create_failed", "相册配置创建失败")
		return
	}
	a.writeAdminAudit(c, "couple_album_option.create", "couple_album_option", option.ID, gin.H{"type": option.Type, "value": option.Value})
	writeJSON(c, http.StatusCreated, option)
}

func (a *App) handleUpdateAdminCoupleAlbumOption(c *gin.Context) {
	var option CoupleAlbumOption
	if err := a.db.First(&option, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "couple_album_option_not_found", "相册配置不存在")
		return
	}
	var req adminCoupleAlbumOptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyAdminCoupleAlbumOptionRequest(&option, req, false); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_couple_album_option", err.Error())
		return
	}
	if err := a.db.Save(&option).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_option_save_failed", "相册配置保存失败")
		return
	}
	a.writeAdminAudit(c, "couple_album_option.update", "couple_album_option", option.ID, gin.H{"type": option.Type, "value": option.Value, "active": option.IsActive})
	writeJSON(c, http.StatusOK, option)
}

func (a *App) handleDeleteAdminCoupleAlbumOption(c *gin.Context) {
	var option CoupleAlbumOption
	if err := a.db.First(&option, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "couple_album_option_not_found", "相册配置不存在")
		return
	}
	if err := a.db.Delete(&option).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_option_delete_failed", "相册配置删除失败")
		return
	}
	a.writeAdminAudit(c, "couple_album_option.delete", "couple_album_option", option.ID, gin.H{"type": option.Type, "value": option.Value})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleUploadAdminCoupleAlbumOptionAsset(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "couple_album_option_asset_required", "请上传图片")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, "couple_album_option_asset_open_failed", "图片读取失败")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		writeError(c, http.StatusBadRequest, "couple_album_option_asset_read_failed", "图片读取失败")
		return
	}
	mimeType, ok := detectSupportedImageMimeType(content)
	if !ok {
		writeError(c, http.StatusBadRequest, "couple_album_option_asset_invalid_type", "仅支持 PNG、JPG、WEBP 图片")
		return
	}
	assetKey, normalizedMimeType, err := a.assetStore.SaveBase64(base64.StdEncoding.EncodeToString(content), mimeType)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "couple_album_option_asset_save_failed", "图片上传失败")
		return
	}
	publicURL := a.assetStore.PublicURL(assetKey)
	if publicURL == "" {
		_ = a.assetStore.Delete(assetKey)
		writeError(c, http.StatusInternalServerError, "couple_album_option_asset_public_url_missing", "图片已保存但缺少公网访问地址，请检查 OSS_PUBLIC_BASE_URL")
		return
	}
	a.writeAdminAudit(c, "couple_album_option.asset_upload", "asset", 0, gin.H{"asset_key": assetKey, "url": publicURL})
	writeJSON(c, http.StatusCreated, gin.H{
		"url":       publicURL,
		"asset_key": assetKey,
		"mime_type": normalizedMimeType,
	})
}

func applyAdminCoupleAlbumOptionRequest(option *CoupleAlbumOption, req adminCoupleAlbumOptionRequest, creating bool) error {
	if req.Type != nil || creating {
		option.Type = strings.TrimSpace(valueOrEmpty(req.Type))
	}
	if !validCoupleAlbumOptionType(option.Type) {
		return errors.New("选项分组无效")
	}
	if req.Value != nil || creating {
		option.Value = truncateRunes(strings.TrimSpace(valueOrEmpty(req.Value)), 96)
	}
	if option.Value == "" {
		return errors.New("选项值不能为空")
	}
	if req.Label != nil || creating {
		option.Label = truncateRunes(strings.TrimSpace(valueOrEmpty(req.Label)), 128)
	}
	if option.Label == "" {
		return errors.New("显示名称不能为空")
	}
	if req.Description != nil {
		option.Description = truncateRunes(strings.TrimSpace(*req.Description), 500)
	}
	if req.ImageURL != nil {
		option.ImageURL = strings.TrimSpace(*req.ImageURL)
	}
	if req.IconURL != nil {
		option.IconURL = strings.TrimSpace(*req.IconURL)
	}
	if req.PromptLabel != nil {
		option.PromptLabel = truncateRunes(strings.TrimSpace(*req.PromptLabel), 128)
	}
	if req.SortOrder != nil {
		option.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		option.IsActive = *req.IsActive
	}
	if option.Type == CoupleAlbumOptionTypeLocation && strings.TrimSpace(option.ImageURL) == "" {
		return errors.New("旅游地点需要封面图")
	}
	if option.Type != CoupleAlbumOptionTypeLocation && strings.TrimSpace(option.IconURL) == "" {
		return errors.New("故事模板和画面风格需要图标")
	}
	if strings.TrimSpace(option.PromptLabel) == "" {
		option.PromptLabel = option.Label
	}
	return nil
}

func (a *App) coupleAlbumOptionsPayload(activeOnly bool) (gin.H, error) {
	groups, err := a.coupleAlbumOptionGroups(activeOnly)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"locations":       groups[CoupleAlbumOptionTypeLocation],
		"story_templates": groups[CoupleAlbumOptionTypeStoryTemplate],
		"styles":          groups[CoupleAlbumOptionTypeStyle],
	}, nil
}

func (a *App) coupleAlbumOptionGroups(activeOnly bool) (map[string][]CoupleAlbumOption, error) {
	query := a.db.Model(&CoupleAlbumOption{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	var options []CoupleAlbumOption
	if err := query.Order("type asc, sort_order asc, id asc").Find(&options).Error; err != nil {
		return nil, err
	}
	groups := map[string][]CoupleAlbumOption{
		CoupleAlbumOptionTypeLocation:      {},
		CoupleAlbumOptionTypeStoryTemplate: {},
		CoupleAlbumOptionTypeStyle:         {},
	}
	for _, option := range options {
		groups[option.Type] = append(groups[option.Type], option)
	}
	return groups, nil
}

func (a *App) validateCoupleAlbumCreateOptions(req coupleAlbumCreateRequest) (map[string]CoupleAlbumOption, bool, string) {
	checks := []struct {
		optionType string
		value      string
		code       string
		message    string
	}{
		{CoupleAlbumOptionTypeLocation, req.Location, "album_location_invalid", "请选择有效旅游地点"},
		{CoupleAlbumOptionTypeStoryTemplate, req.StoryTemplate, "album_story_template_invalid", "请选择有效故事模板"},
		{CoupleAlbumOptionTypeStyle, req.Style, "album_style_invalid", "请选择有效画面风格"},
	}
	options := map[string]CoupleAlbumOption{}
	for _, check := range checks {
		var option CoupleAlbumOption
		if err := a.db.Where("type = ? AND value = ? AND is_active = ?", check.optionType, check.value, true).First(&option).Error; err != nil {
			return nil, false, check.code + "|" + check.message
		}
		options[check.optionType] = option
	}
	return options, true, ""
}

func (a *App) coupleAlbumOptionPromptLabel(optionType, value string) string {
	var option CoupleAlbumOption
	if err := a.db.Where("type = ? AND value = ?", optionType, value).First(&option).Error; err != nil {
		return ""
	}
	return fallbackString(strings.TrimSpace(option.PromptLabel), option.Label)
}

func validCoupleAlbumOptionType(value string) bool {
	switch strings.TrimSpace(value) {
	case CoupleAlbumOptionTypeLocation, CoupleAlbumOptionTypeStoryTemplate, CoupleAlbumOptionTypeStyle:
		return true
	default:
		return false
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
