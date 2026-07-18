package app

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type adminMenuItem struct {
	Label      string `json:"label"`
	Path       string `json:"path"`
	Permission string `json:"permission"`
}

type adminSearchItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	To       string `json:"to"`
}

type adminSearchSection struct {
	Key   string            `json:"key"`
	Label string            `json:"label"`
	Items []adminSearchItem `json:"items"`
}

var adminMenuItems = []adminMenuItem{
	{"AI 电商商品品类", "/admin/ecommerce-categories", "system_settings.read"},
	{"灵感推荐", "/admin/inspiration-recommendations", "inspiration_recommendations.read"},
	{"概览", "/admin", "dashboard.read"},
	{"用户与点数", "/admin/users", "users.read"},
	{"套餐配置", "/admin/packages", "packages.read"},
	{"财务订单", "/admin/finance-orders", "finance_orders.read"},
	{"系统设置", "/admin/system-settings", "system_settings.read"},
	{"资源监控", "/admin/system-resources", "system_resources.read"},
	{"系统日志", "/admin/system-logs", "system_logs.read"},
	{"客服配置", "/admin/customer-service", "customer_service.read"},
	{"提示词模板", "/admin/prompt-templates", "prompt_templates.read"},
	{"视频风格预设", "/admin/video-style-presets", "video_style_presets.read"},
	{"相册配置", "/admin/couple-album-options", "couple_album_options.read"},
	{"公告通知", "/admin/announcements", "announcements.read"},
	{"模型配置", "/admin/settings", "settings.image.read"},
	{"邀请码", "/admin/invites", "invites.read"},
	{"生成记录", "/admin/generations", "generations.read"},
	{"视频记录", "/admin/video-generations", "generations.read"},
	{"内容审核", "/admin/content-reviews", "content_reviews.read"},
	{"投诉举报", "/admin/content-reports", "content_reports.read"},
	{"算法合规", "/admin/algorithm-compliance", "algorithm_compliance.read"},
	{"应急事件", "/admin/incidents", "algorithm_incidents.read"},
	{"管理员与权限", "/admin/permissions", "admin_users.read"},
}

var adminSearchConfigEntries = []struct {
	ID         string
	Title      string
	Subtitle   string
	Path       string
	Permission string
	Keywords   []string
}{
	{"commerce_categories.read", "AI 电商商品品类", "维护 AI 电商两级商品品类目录", "/admin/ecommerce-categories", "system_settings.read", []string{"AI 电商", "商品", "品类", "分类", "配置"}},
	{"customer_service.read", "客服配置", "配置客服二维码与联系方式", "/admin/customer-service", "customer_service.read", []string{"客服", "联系", "二维码", "配置"}},
	{"prompt_templates.read", "提示词模板", "管理创作提示词模板", "/admin/prompt-templates", "prompt_templates.read", []string{"提示词", "模板", "prompt", "配置"}},
	{"video_style_presets.read", "视频风格预设", "管理视频生成视觉风格预设", "/admin/video-style-presets", "video_style_presets.read", []string{"视频", "风格", "预设", "模板", "配置"}},
	{"couple_album_options.read", "相册配置", "管理情侣相册地点、故事与风格", "/admin/couple-album-options", "couple_album_options.read", []string{"相册", "情侣", "地点", "风格", "配置"}},
	{"announcements.read", "公告通知", "发布与维护站内公告", "/admin/announcements", "announcements.read", []string{"公告", "通知", "配置"}},
	{"settings.image.read", "模型配置", "配置图片与视频生成模型", "/admin/settings", "settings.image.read", []string{"模型", "图片", "视频", "配置"}},
	{"system_settings.read", "系统设置", "维护系统限额与运行开关", "/admin/system-settings", "system_settings.read", []string{"系统", "设置", "限额", "配置"}},
}

func adminMePayload(admin AdminUser, permissions map[string]bool) gin.H {
	roles := make([]gin.H, 0, len(admin.Roles))
	for _, role := range admin.Roles {
		roles = append(roles, gin.H{"id": role.ID, "code": role.Code, "name": role.Name})
	}
	menus := make([]adminMenuItem, 0, len(adminMenuItems))
	for _, item := range adminMenuItems {
		if permissions[item.Permission] {
			menus = append(menus, item)
		}
	}
	return gin.H{
		"admin": gin.H{
			"id":            admin.ID,
			"username":      admin.Username,
			"display_name":  admin.DisplayName,
			"status":        admin.Status,
			"last_login_at": admin.LastLoginAt,
		},
		"roles":       roles,
		"permissions": permissionCodesFromMap(permissions),
		"menus":       menus,
	}
}

func (a *App) handleAdminSearch(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		writeError(c, http.StatusBadRequest, "search_query_required", "请输入搜索关键词")
		return
	}
	permissions := c.MustGet("adminPermissions").(map[string]bool)
	sections := make([]adminSearchSection, 0, 3)

	if permissions["users.read"] {
		if items, err := a.adminSearchUsers(query); err != nil {
			writeError(c, http.StatusInternalServerError, "admin_search_failed", "搜索失败")
			return
		} else if len(items) > 0 {
			sections = append(sections, adminSearchSection{Key: "users", Label: "用户", Items: items})
		}
	}

	if permissions["generations.read"] {
		if items, err := a.adminSearchGenerations(query); err != nil {
			writeError(c, http.StatusInternalServerError, "admin_search_failed", "搜索失败")
			return
		} else if len(items) > 0 {
			sections = append(sections, adminSearchSection{Key: "generations", Label: "生成记录", Items: items})
		}
		if items, err := a.adminSearchVideoGenerations(query); err != nil {
			writeError(c, http.StatusInternalServerError, "admin_search_failed", "搜索失败")
			return
		} else if len(items) > 0 {
			sections = append(sections, adminSearchSection{Key: "video_generations", Label: "视频记录", Items: items})
		}
	}

	configItems := adminSearchConfigItems(query, permissions)
	if len(configItems) > 0 {
		sections = append(sections, adminSearchSection{Key: "config", Label: "配置入口", Items: configItems})
	}

	writeJSON(c, http.StatusOK, gin.H{
		"query":    query,
		"sections": sections,
	})
}

func (a *App) adminSearchUsers(query string) ([]adminSearchItem, error) {
	like := "%" + query + "%"
	var users []User
	if err := a.db.
		Where("username LIKE ? OR display_name LIKE ? OR email LIKE ? OR phone LIKE ?", like, like, like, like).
		Order("updated_at desc, id desc").
		Limit(5).
		Find(&users).Error; err != nil {
		return nil, err
	}
	items := make([]adminSearchItem, 0, len(users))
	for _, user := range users {
		title := fallbackString(user.DisplayName, user.Username)
		if strings.TrimSpace(title) == "" {
			title = "用户 " + strconv.FormatUint(uint64(user.ID), 10)
		}
		phone := "未绑定手机"
		if user.Phone != nil && strings.TrimSpace(*user.Phone) != "" {
			phone = strings.TrimSpace(*user.Phone)
		}
		email := fallbackString(user.Email, "未绑定邮箱")
		items = append(items, adminSearchItem{
			ID:       strconv.FormatUint(uint64(user.ID), 10),
			Title:    title,
			Subtitle: phone + " / " + email,
			To:       "/admin/users?q=" + url.QueryEscape(query),
		})
	}
	return items, nil
}

func (a *App) adminSearchGenerations(query string) ([]adminSearchItem, error) {
	like := "%" + query + "%"
	var records []GenerationRecord
	if err := a.db.
		Where("prompt LIKE ? OR negative_prompt LIKE ? OR model LIKE ? OR runtime_model LIKE ?", like, like, like, like).
		Order("created_at desc, id desc").
		Limit(5).
		Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]adminSearchItem, 0, len(records))
	for _, record := range records {
		title := truncateRunes(strings.TrimSpace(record.Prompt), 42)
		if title == "" {
			title = "生成记录 " + strconv.FormatUint(uint64(record.ID), 10)
		}
		subtitleParts := []string{}
		if record.Status != "" {
			subtitleParts = append(subtitleParts, record.Status)
		}
		if fallbackString(record.RuntimeModel, record.Model) != "" {
			subtitleParts = append(subtitleParts, fallbackString(record.RuntimeModel, record.Model))
		}
		items = append(items, adminSearchItem{
			ID:       strconv.FormatUint(uint64(record.ID), 10),
			Title:    title,
			Subtitle: strings.Join(subtitleParts, " / "),
			To:       "/admin/generations?q=" + url.QueryEscape(query),
		})
	}
	return items, nil
}

func (a *App) adminSearchVideoGenerations(query string) ([]adminSearchItem, error) {
	like := "%" + query + "%"
	var records []VideoGenerationRecord
	if err := a.db.
		Where("prompt LIKE ? OR provider_request_id LIKE ? OR runtime_model LIKE ?", like, like, like).
		Order("created_at desc, id desc").
		Limit(5).
		Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]adminSearchItem, 0, len(records))
	for _, record := range records {
		title := truncateRunes(strings.TrimSpace(record.Prompt), 42)
		if title == "" {
			title = "视频记录 " + strconv.FormatUint(uint64(record.ID), 10)
		}
		subtitleParts := []string{}
		if record.Status != "" {
			subtitleParts = append(subtitleParts, record.Status)
		}
		if record.RuntimeModel != "" {
			subtitleParts = append(subtitleParts, record.RuntimeModel)
		}
		if record.ProviderRequestID != "" {
			subtitleParts = append(subtitleParts, record.ProviderRequestID)
		}
		items = append(items, adminSearchItem{
			ID:       strconv.FormatUint(uint64(record.ID), 10),
			Title:    title,
			Subtitle: strings.Join(subtitleParts, " / "),
			To:       "/admin/video-generations?q=" + url.QueryEscape(query),
		})
	}
	return items, nil
}

func adminSearchConfigItems(query string, permissions map[string]bool) []adminSearchItem {
	normalizedQuery := strings.ToLower(query)
	items := []adminSearchItem{}
	for _, entry := range adminSearchConfigEntries {
		if !permissions[entry.Permission] || !adminSearchConfigEntryMatches(entry.Title, entry.Path, entry.Keywords, normalizedQuery) {
			continue
		}
		items = append(items, adminSearchItem{
			ID:       entry.ID,
			Title:    entry.Title,
			Subtitle: entry.Subtitle,
			To:       entry.Path,
		})
	}
	return items
}

func adminSearchConfigEntryMatches(title, path string, keywords []string, normalizedQuery string) bool {
	if strings.Contains(strings.ToLower(title), normalizedQuery) || strings.Contains(strings.ToLower(path), normalizedQuery) {
		return true
	}
	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(keyword), normalizedQuery) {
			return true
		}
	}
	return false
}

func (a *App) handleListAdminUsers(c *gin.Context) {
	var admins []AdminUser
	if err := a.db.Preload("Roles").Order("created_at desc").Find(&admins).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "admin_users_load_failed", "管理员读取失败")
		return
	}
	items := make([]gin.H, 0, len(admins))
	for _, admin := range admins {
		roles := make([]gin.H, 0, len(admin.Roles))
		for _, role := range admin.Roles {
			roles = append(roles, gin.H{"id": role.ID, "code": role.Code, "name": role.Name})
		}
		items = append(items, gin.H{
			"id":            admin.ID,
			"username":      admin.Username,
			"display_name":  admin.DisplayName,
			"status":        admin.Status,
			"last_login_at": admin.LastLoginAt,
			"roles":         roles,
			"created_at":    admin.CreatedAt,
			"updated_at":    admin.UpdatedAt,
		})
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleCreateAdminUser(c *gin.Context) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Status      string `json:"status"`
		RoleIDs     []uint `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || len(strings.TrimSpace(req.Password)) < 8 {
		writeError(c, http.StatusBadRequest, "invalid_request", "管理员账号或密码无效")
		return
	}
	status := req.Status
	if status == "" {
		status = AdminUserStatusActive
	}
	if status != AdminUserStatusActive && status != AdminUserStatusDisabled {
		writeError(c, http.StatusBadRequest, "invalid_status", "管理员状态无效")
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}
	admin := AdminUser{Username: username, DisplayName: displayName, PasswordHash: string(passwordHash), Status: status}
	if err := a.db.Create(&admin).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "admin_user_create_failed", "管理员创建失败")
		return
	}
	if len(req.RoleIDs) > 0 {
		var roles []Role
		if err := a.db.Where("id IN ?", req.RoleIDs).Find(&roles).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "roles_load_failed", "角色读取失败")
			return
		}
		if err := a.db.Model(&admin).Association("Roles").Replace(roles); err != nil {
			writeError(c, http.StatusInternalServerError, "roles_save_failed", "角色保存失败")
			return
		}
	}
	a.writeAdminAudit(c, "admin_user.create", "admin_user", admin.ID, gin.H{"username": admin.Username})
	writeJSON(c, http.StatusCreated, gin.H{"id": admin.ID})
}

func (a *App) handleUpdateAdminUser(c *gin.Context) {
	var admin AdminUser
	if err := a.db.First(&admin, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "admin_user_not_found", "管理员不存在")
		return
	}
	var req struct {
		DisplayName *string `json:"display_name"`
		Status      *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.DisplayName != nil {
		admin.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Status != nil {
		if *req.Status != AdminUserStatusActive && *req.Status != AdminUserStatusDisabled {
			writeError(c, http.StatusBadRequest, "invalid_status", "管理员状态无效")
			return
		}
		admin.Status = *req.Status
	}
	if err := a.db.Save(&admin).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "admin_user_save_failed", "管理员保存失败")
		return
	}
	a.writeAdminAudit(c, "admin_user.update", "admin_user", admin.ID, gin.H{"status": admin.Status})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handlePutAdminUserRoles(c *gin.Context) {
	var admin AdminUser
	if err := a.db.First(&admin, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "admin_user_not_found", "管理员不存在")
		return
	}
	var req struct {
		RoleIDs []uint `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	var roles []Role
	if len(req.RoleIDs) > 0 {
		if err := a.db.Where("id IN ?", req.RoleIDs).Find(&roles).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "roles_load_failed", "角色读取失败")
			return
		}
	}
	if err := a.db.Model(&admin).Association("Roles").Replace(roles); err != nil {
		writeError(c, http.StatusInternalServerError, "roles_save_failed", "角色保存失败")
		return
	}
	a.writeAdminAudit(c, "admin_user.roles.update", "admin_user", admin.ID, gin.H{"role_ids": req.RoleIDs})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleResetAdminUserPassword(c *gin.Context) {
	var admin AdminUser
	if err := a.db.First(&admin, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "admin_user_not_found", "管理员不存在")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(strings.TrimSpace(req.Password)) < 8 {
		writeError(c, http.StatusBadRequest, "invalid_request", "新密码至少 8 位")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}
	if err := a.db.Model(&admin).Update("password_hash", string(passwordHash)).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "password_save_failed", "密码保存失败")
		return
	}
	_ = a.db.Where("admin_user_id = ?", admin.ID).Delete(&AdminSession{}).Error
	a.writeAdminAudit(c, "admin_user.password.reset", "admin_user", admin.ID, "")
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleChangeAdminPassword(c *gin.Context) {
	admin := currentAdmin(c)
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeError(c, http.StatusUnauthorized, "password_mismatch", "当前密码错误")
		return
	}
	if len(strings.TrimSpace(req.NewPassword)) < 8 {
		writeError(c, http.StatusBadRequest, "password_too_short", "新密码至少 8 位")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}
	if err := a.db.Model(&AdminUser{}).Where("id = ?", admin.ID).Update("password_hash", string(passwordHash)).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "password_save_failed", "密码保存失败")
		return
	}
	if err := a.db.Where("admin_user_id = ?", admin.ID).Delete(&AdminSession{}).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "session_revoke_failed", "会话清理失败")
		return
	}
	http.SetCookie(c.Writer, clearCookie(adminSessionCookie))
	a.writeAdminAudit(c, "admin.password.change", "admin_user", admin.ID, "")
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleListRoles(c *gin.Context) {
	var roles []Role
	if err := a.db.Preload("Permissions").Order("code asc").Find(&roles).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "roles_load_failed", "角色读取失败")
		return
	}
	var permissions []Permission
	if err := a.db.Order("code asc").Find(&permissions).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "permissions_load_failed", "权限读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": roles, "permissions": permissions})
}

func (a *App) handleCreateRole(c *gin.Context) {
	var req struct {
		Code        string `json:"code"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	role := Role{
		Code:        strings.TrimSpace(req.Code),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Status:      req.Status,
	}
	if role.Status == "" {
		role.Status = RoleStatusActive
	}
	if role.Code == "" || role.Name == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "角色编码和名称不能为空")
		return
	}
	if role.Status != RoleStatusActive && role.Status != RoleStatusDisabled {
		writeError(c, http.StatusBadRequest, "invalid_status", "角色状态无效")
		return
	}
	if err := a.db.Create(&role).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "role_create_failed", "角色创建失败")
		return
	}
	a.writeAdminAudit(c, "role.create", "role", role.ID, gin.H{"code": role.Code})
	writeJSON(c, http.StatusCreated, role)
}

func (a *App) handleUpdateRole(c *gin.Context) {
	var role Role
	if err := a.db.First(&role, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "role_not_found", "角色不存在")
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.Name != nil {
		role.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		role.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		if *req.Status != RoleStatusActive && *req.Status != RoleStatusDisabled {
			writeError(c, http.StatusBadRequest, "invalid_status", "角色状态无效")
			return
		}
		role.Status = *req.Status
	}
	if err := a.db.Save(&role).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "role_save_failed", "角色保存失败")
		return
	}
	a.writeAdminAudit(c, "role.update", "role", role.ID, gin.H{"status": role.Status})
	writeJSON(c, http.StatusOK, role)
}

func (a *App) handlePutRolePermissions(c *gin.Context) {
	var role Role
	if err := a.db.First(&role, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "role_not_found", "角色不存在")
		return
	}
	var req struct {
		PermissionIDs []uint `json:"permission_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	var permissions []Permission
	if len(req.PermissionIDs) > 0 {
		if err := a.db.Where("id IN ?", req.PermissionIDs).Find(&permissions).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "permissions_load_failed", "权限读取失败")
			return
		}
	}
	if err := a.db.Model(&role).Association("Permissions").Replace(permissions); err != nil {
		writeError(c, http.StatusInternalServerError, "permissions_save_failed", "权限保存失败")
		return
	}
	a.writeAdminAudit(c, "role.permissions.update", "role", role.ID, gin.H{"permission_ids": req.PermissionIDs})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}
