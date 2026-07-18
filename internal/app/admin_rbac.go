package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type permissionSeed struct {
	Code string
	Name string
}

var defaultPermissionSeeds = []permissionSeed{
	{"inspiration_recommendations.read", "查看灵感推荐"},
	{"inspiration_recommendations.update", "管理灵感推荐"},
	{"dashboard.read", "查看概览"},
	{"users.read", "查看用户"},
	{"users.update", "修改用户"},
	{"users.delete", "删除用户"},
	{"users.credits.add", "人工加点"},
	{"users.password.reset", "重置用户密码"},
	{"packages.read", "查看套餐"},
	{"packages.create", "新增套餐"},
	{"packages.update", "更新套餐"},
	{"packages.delete", "删除套餐"},
	{"finance_orders.read", "查看财务订单"},
	{"finance_orders.update", "更新财务订单"},
	{"settings.image.read", "查看模型配置"},
	{"settings.image.update", "更新模型配置"},
	{"prompt_templates.read", "查看提示词模板"},
	{"prompt_templates.update", "管理提示词模板"},
	{"video_style_presets.read", "查看视频风格预设"},
	{"video_style_presets.update", "管理视频风格预设"},
	{"couple_album_options.read", "查看情侣相册配置"},
	{"couple_album_options.update", "管理情侣相册配置"},
	{"system_settings.read", "查看系统设置"},
	{"system_settings.update", "更新系统设置"},
	{"system_resources.read", "查看资源监控"},
	{"system_logs.read", "查看系统日志"},
	{"customer_service.read", "查看客服配置"},
	{"customer_service.update", "更新客服配置"},
	{"invites.read", "查看邀请码"},
	{"invites.create", "新增邀请码"},
	{"invites.update", "更新邀请码"},
	{"generations.read", "查看生成记录"},
	{"content_reviews.read", "查看内容审核"},
	{"content_reviews.update", "处置内容审核"},
	{"content_reports.read", "查看内容举报"},
	{"content_reports.update", "处置内容举报"},
	{"algorithm_compliance.read", "查看算法合规"},
	{"algorithm_compliance.update", "维护算法合规"},
	{"algorithm_incidents.read", "查看算法应急事件"},
	{"algorithm_incidents.update", "维护算法应急事件"},
	{"announcements.read", "查看公告"},
	{"announcements.create", "发布公告"},
	{"announcements.update", "更新公告"},
	{"admin_users.read", "查看管理员"},
	{"admin_users.create", "新增管理员"},
	{"admin_users.update", "更新管理员"},
	{"admin_users.roles.update", "分配管理员角色"},
	{"admin_users.password.reset", "重置管理员密码"},
	{"roles.read", "查看角色"},
	{"roles.create", "新增角色"},
	{"roles.update", "更新角色"},
	{"roles.permissions.update", "分配角色权限"},
}

func (a *App) seedRBACAndBootstrapAdmin() error {
	// Dependency-injected test/legacy callers keep their explicit bootstrap
	// credentials. The production path always has SecretStore enabled and must
	// use the interactive `admin create` command instead.
	if a.secretStore == nil && strings.TrimSpace(a.cfg.AdminUsername) != "" && strings.TrimSpace(a.cfg.AdminPassword) != "" {
		return SeedRBACAndBootstrapAdmin(a.db, a.cfg.AdminUsername, a.cfg.AdminPassword)
	}
	return a.seedPermissionsAndRoles()
}

func SeedRBACAndBootstrapAdmin(db *gorm.DB, adminUsername, adminPassword string) error {
	if err := db.AutoMigrate(&Permission{}, &Role{}, &AdminUser{}); err != nil {
		return err
	}
	a := &App{
		db: db,
		cfg: Config{
			AdminUsername: adminUsername,
			AdminPassword: adminPassword,
		},
	}
	if err := a.seedPermissionsAndRoles(); err != nil {
		return err
	}

	var adminCount int64
	if err := a.db.Model(&AdminUser{}).Count(&adminCount).Error; err != nil {
		return err
	}
	if adminCount > 0 {
		return nil
	}
	if strings.TrimSpace(a.cfg.AdminUsername) == "" || strings.TrimSpace(a.cfg.AdminPassword) == "" {
		return errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required to bootstrap the first admin user")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(a.cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	var superAdmin Role
	if err := a.db.Where("code = ?", "super_admin").First(&superAdmin).Error; err != nil {
		return err
	}

	admin := AdminUser{
		Username:     strings.TrimSpace(a.cfg.AdminUsername),
		DisplayName:  strings.TrimSpace(a.cfg.AdminUsername),
		PasswordHash: string(passwordHash),
		Status:       AdminUserStatusActive,
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&admin).Error; err != nil {
			return err
		}
		return tx.Model(&admin).Association("Roles").Append(&superAdmin)
	})
}

func CreateInitialAdmin(db *gorm.DB, username, password string) error {
	if db == nil {
		return errors.New("database is required")
	}
	if strings.TrimSpace(username) == "" {
		return errors.New("username is required")
	}
	if len(password) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	return SeedRBACAndBootstrapAdmin(db, username, password)
}

func (a *App) seedPermissionsAndRoles() error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		for _, seed := range defaultPermissionSeeds {
			permission := Permission{Code: seed.Code, Name: seed.Name}
			if err := tx.Where("code = ?", seed.Code).FirstOrCreate(&permission, permission).Error; err != nil {
				return err
			}
			if permission.Name == "" {
				permission.Name = seed.Name
				if err := tx.Save(&permission).Error; err != nil {
					return err
				}
			}
		}

		roles := []Role{
			{Code: "super_admin", Name: "超级管理员", Description: "拥有全部后台权限", Status: RoleStatusActive},
			{Code: "operator", Name: "运营", Description: "日常运营管理", Status: RoleStatusActive},
			{Code: "finance", Name: "财务", Description: "套餐和财务订单管理", Status: RoleStatusActive},
			{Code: "auditor", Name: "审计", Description: "只读审计访问", Status: RoleStatusActive},
		}
		for _, seed := range roles {
			var role Role
			if err := tx.Where("code = ?", seed.Code).First(&role).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				role = seed
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			}
			updates := map[string]any{}
			if role.Name == "" {
				updates["name"] = seed.Name
			}
			if role.Status == "" {
				updates["status"] = seed.Status
			}
			if seed.Description != "" && role.Description != seed.Description {
				updates["description"] = seed.Description
			}
			if len(updates) > 0 {
				if err := tx.Model(&role).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var superAdmin Role
		if err := tx.Where("code = ?", "super_admin").First(&superAdmin).Error; err != nil {
			return err
		}
		var permissions []Permission
		if err := tx.Order("code asc").Find(&permissions).Error; err != nil {
			return err
		}
		if err := tx.Model(&superAdmin).Association("Permissions").Replace(permissions); err != nil {
			return err
		}

		if err := ensureRolePermissions(tx, "finance", []string{
			"packages.read",
			"finance_orders.read",
			"finance_orders.update",
		}); err != nil {
			return err
		}
		if err := ensureRolePermissions(tx, "operator", []string{
			"system_settings.read",
			"system_settings.update",
			"system_resources.read",
			"customer_service.read",
			"customer_service.update",
			"prompt_templates.read",
			"prompt_templates.update",
			"inspiration_recommendations.read",
			"inspiration_recommendations.update",
			"video_style_presets.read",
			"video_style_presets.update",
			"couple_album_options.read",
			"couple_album_options.update",
			"announcements.read",
			"announcements.create",
			"announcements.update",
			"content_reviews.read",
			"content_reviews.update",
			"content_reports.read",
			"content_reports.update",
			"algorithm_compliance.read",
			"algorithm_compliance.update",
			"algorithm_incidents.read",
			"algorithm_incidents.update",
		}); err != nil {
			return err
		}
		// system_logs.read 为敏感能力，受 requireStrictAdminPermission 保护，仅 super_admin 可用，
		// 不再下放给 auditor 等自定义角色。
		return ensureRolePermissions(tx, "auditor", []string{
			"dashboard.read",
			"users.read",
			"packages.read",
			"finance_orders.read",
			"generations.read",
			"system_settings.read",
			"system_resources.read",
			"customer_service.read",
			"couple_album_options.read",
			"content_reviews.read",
			"content_reports.read",
			"algorithm_compliance.read",
			"algorithm_incidents.read",
		})
	})
}

func ensureRolePermissions(tx *gorm.DB, roleCode string, permissionCodes []string) error {
	var role Role
	if err := tx.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
		return err
	}
	var permissions []Permission
	if err := tx.Where("code IN ?", permissionCodes).Find(&permissions).Error; err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, permission := range role.Permissions {
		existing[permission.Code] = true
	}
	missing := make([]Permission, 0, len(permissions))
	for _, permission := range permissions {
		if !existing[permission.Code] {
			missing = append(missing, permission)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return tx.Model(&role).Association("Permissions").Append(missing)
}

func (a *App) authenticateAdmin(req *http.Request) (*SessionClaims, AdminUser, map[string]bool, error) {
	claims, err := a.parseSessionCookie(req, adminSessionCookie)
	if err != nil || claims.Role != "admin" || claims.AdminUserID == 0 || strings.TrimSpace(claims.SessionID) == "" {
		return nil, AdminUser{}, nil, errors.New("invalid admin session")
	}

	var session AdminSession
	if err := a.db.Where("token_id = ? AND admin_user_id = ?", claims.SessionID, claims.AdminUserID).First(&session).Error; err != nil {
		return nil, AdminUser{}, nil, err
	}
	if time.Now().After(session.ExpiresAt) {
		_ = a.db.Delete(&session).Error
		return nil, AdminUser{}, nil, errors.New("admin session expired")
	}

	var admin AdminUser
	if err := a.db.Preload("Roles.Permissions").First(&admin, claims.AdminUserID).Error; err != nil {
		return nil, AdminUser{}, nil, err
	}
	if admin.Status != AdminUserStatusActive {
		return nil, AdminUser{}, nil, errors.New("admin disabled")
	}
	return claims, admin, permissionsForAdmin(admin), nil
}

func permissionsForAdmin(admin AdminUser) map[string]bool {
	permissions := map[string]bool{}
	for _, role := range admin.Roles {
		if role.Status != RoleStatusActive {
			continue
		}
		if role.Code == "super_admin" {
			for _, seed := range defaultPermissionSeeds {
				permissions[seed.Code] = true
			}
			continue
		}
		for _, permission := range role.Permissions {
			permissions[permission.Code] = true
		}
	}
	return permissions
}

func permissionCodesFromMap(values map[string]bool) []string {
	codes := make([]string, 0, len(values))
	for code, ok := range values {
		if ok {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	return codes
}

func currentAdmin(c interface{ MustGet(any) any }) *AdminUser {
	return c.MustGet("currentAdmin").(*AdminUser)
}

func (a *App) writeAdminAudit(c interface {
	ClientIP() string
	MustGet(any) any
}, action, targetType string, targetID uint, detail any) {
	admin := currentAdmin(c)
	var text string
	switch value := detail.(type) {
	case string:
		text = value
	default:
		payload, _ := json.Marshal(value)
		text = string(payload)
	}
	_ = a.db.Create(&AdminAuditLog{
		AdminUserID: admin.ID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Detail:      text,
		IPAddress:   c.ClientIP(),
	}).Error
}
