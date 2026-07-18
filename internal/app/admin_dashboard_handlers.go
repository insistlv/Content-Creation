package app

// 本文件从 platform_handlers.go 拆分：管理端登录、仪表盘、公告与图片设置。

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func (a *App) handleAdminLogin(c *gin.Context) {
	var req struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		CaptchaID     string `json:"captcha_id"`
		CaptchaCode   string `json:"captcha_code"`
		RememberLogin bool   `json:"remember_login"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if !a.validateAuthCaptcha(c, "admin_login", req.CaptchaID, req.CaptchaCode) {
		return
	}
	var admin AdminUser
	if err := a.db.Preload("Roles.Permissions").Where("username = ?", strings.TrimSpace(req.Username)).First(&admin).Error; err != nil {
		writeError(c, http.StatusUnauthorized, "admin_login_failed", "账号或密码错误")
		return
	}
	if admin.Status != AdminUserStatusActive || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)) != nil {
		writeError(c, http.StatusUnauthorized, "admin_login_failed", "账号或密码错误")
		return
	}
	if err := a.issueAdminCookie(c.Writer, admin, req.RememberLogin); err != nil {
		writeError(c, http.StatusInternalServerError, "session_issue_failed", "会话创建失败")
		return
	}
	now := time.Now()
	_ = a.db.Model(&AdminUser{}).Where("id = ?", admin.ID).Update("last_login_at", &now).Error
	_ = a.db.Create(&AdminAuditLog{
		AdminUserID: admin.ID,
		Action:      "admin.login",
		TargetType:  "admin_user",
		TargetID:    admin.ID,
		IPAddress:   c.ClientIP(),
	}).Error
	writeJSON(c, http.StatusOK, adminMePayload(admin, permissionsForAdmin(admin)))
}

func (a *App) handleAdminLogout(c *gin.Context) {
	claims := c.MustGet("claims").(*SessionClaims)
	if claims.SessionID != "" {
		_ = a.db.Where("token_id = ? AND admin_user_id = ?", claims.SessionID, claims.AdminUserID).Delete(&AdminSession{}).Error
	}
	http.SetCookie(c.Writer, clearCookie(adminSessionCookie))
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleAdminMe(c *gin.Context) {
	admin := currentAdmin(c)
	permissions := c.MustGet("adminPermissions").(map[string]bool)
	writeJSON(c, http.StatusOK, adminMePayload(*admin, permissions))
}

func (a *App) handleAdminDashboard(c *gin.Context) {
	var usersTotal, worksTotal, generationTotal, generationSucceeded, generationFailed, activePackages, invitesActive int64
	if err := a.db.Model(&User{}).Count(&usersTotal).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	if err := a.db.Model(&Work{}).Count(&worksTotal).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	if err := a.db.Model(&GenerationRecord{}).Count(&generationTotal).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	if err := a.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusSucceeded).Count(&generationSucceeded).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	if err := a.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusFailed).Count(&generationFailed).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	if err := a.db.Model(&Package{}).Where("is_active = ?", true).Count(&activePackages).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	if err := a.db.Model(&Invite{}).Where("status = ?", InviteStatusActive).Count(&invitesActive).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	var packages []Package
	if err := a.db.Order("sort_order asc, id asc").Limit(8).Find(&packages).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	revenue, err := a.completedPurchaseRevenue()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	trend, err := a.dashboardGenerationTrend(time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	inviteSummary, err := a.dashboardInviteSummary()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	var recentGenerations []dashboardRecentGeneration
	if err := a.db.Model(&GenerationRecord{}).
		Select("id", "user_id", "work_id", "prompt", "model", "status", "preview_url", "created_at").
		Order("created_at desc, id desc").
		Limit(8).
		Find(&recentGenerations).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	var announcements []SystemAnnouncement
	if err := a.db.Where("status = ?", AnnouncementStatusPublished).
		Order("published_at desc, created_at desc, id desc").
		Limit(6).
		Find(&announcements).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	var operationLogs []AdminAuditLog
	if err := a.db.Order("created_at desc, id desc").Limit(8).Find(&operationLogs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "dashboard_load_failed", "概览读取失败")
		return
	}

	kpis := dashboardKPI{
		UsersTotal:          usersTotal,
		WorksTotal:          worksTotal,
		GenerationTotal:     generationTotal,
		GenerationSucceeded: generationSucceeded,
		GenerationFailed:    generationFailed,
		ActivePackages:      activePackages,
		ActiveInvites:       invitesActive,
		RevenueCompleted:    formatYuan(revenue),
	}

	writeJSON(c, http.StatusOK, gin.H{
		"users_total":             usersTotal,
		"works_total":             worksTotal,
		"generation_total":        generationTotal,
		"generation_succeeded":    generationSucceeded,
		"generation_failed":       generationFailed,
		"packages_active":         activePackages,
		"invites_active":          invitesActive,
		"active_image_model":      settings.ActiveImageModel,
		"rate_limit_max_requests": settings.RateLimitMaxRequests,
		"kpis":                    kpis,
		"packages":                packages,
		"models":                  dashboardModels(settings),
		"generation_trend":        trend,
		"invite_summary":          inviteSummary,
		"recent_generations":      recentGenerations,
		"announcements":           announcements,
		"operation_logs":          operationLogs,
	})
}

func dashboardModels(settings AppSettings) []dashboardModelItem {
	allowed := settings.AllowedImageModels()
	items := make([]dashboardModelItem, 0, len(allowed))
	for _, model := range allowed {
		items = append(items, dashboardModelItem{
			Name:                  model,
			Active:                model == settings.ActiveImageModel,
			RequestTimeoutSeconds: settings.RequestTimeoutSeconds,
		})
	}
	return items
}

func (a *App) completedPurchaseRevenue() (int64, error) {
	var totalCents int64
	if err := a.db.Model(&FinanceOrder{}).
		Select("COALESCE(SUM(amount_cents), 0)").
		Where("payment_status = ?", FinancePaymentStatusPaid).
		Scan(&totalCents).Error; err != nil {
		return 0, err
	}
	return totalCents, nil
}

func parsePriceCents(text string) (int64, bool) {
	var numeric strings.Builder
	seenDigit := false
	seenDot := false
	for _, r := range text {
		switch {
		case unicode.IsDigit(r):
			seenDigit = true
			numeric.WriteRune(r)
		case r == '.' && seenDigit && !seenDot:
			seenDot = true
			numeric.WriteRune(r)
		case seenDigit:
			value, err := strconv.ParseFloat(numeric.String(), 64)
			if err != nil {
				return 0, false
			}
			return int64(math.Round(value * 100)), true
		}
	}
	if !seenDigit {
		return 0, false
	}
	value, err := strconv.ParseFloat(numeric.String(), 64)
	if err != nil {
		return 0, false
	}
	return int64(math.Round(value * 100)), true
}

func formatYuan(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s￥%d.%02d", sign, cents/100, cents%100)
}

func (a *App) dashboardGenerationTrend(now time.Time) ([]dashboardTrendPoint, error) {
	startDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -29)
	points := make([]dashboardTrendPoint, 30)
	pointByDate := map[string]*dashboardTrendPoint{}
	for i := range points {
		date := startDay.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = dashboardTrendPoint{Date: date}
		pointByDate[date] = &points[i]
	}

	var records []GenerationRecord
	if err := a.db.Select("status", "created_at").Where("created_at >= ?", startDay).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		key := record.CreatedAt.In(now.Location()).Format("2006-01-02")
		point := pointByDate[key]
		if point == nil {
			continue
		}
		point.Total++
		switch record.Status {
		case GenerationStatusSucceeded:
			point.Succeeded++
		case GenerationStatusFailed:
			point.Failed++
		}
	}
	return points, nil
}

func (a *App) dashboardInviteSummary() (dashboardInviteSummary, error) {
	var invites []Invite
	if err := a.db.Find(&invites).Error; err != nil {
		return dashboardInviteSummary{}, err
	}
	var summary dashboardInviteSummary
	for _, invite := range invites {
		if invite.Status == InviteStatusActive {
			summary.Active++
		}
		summary.Total += int64(invite.TotalQuota)
		summary.Used += int64(invite.UsedQuota)
		summary.Remaining += int64(invite.RemainingQuota())
	}
	return summary, nil
}

func (a *App) handleListAdminAnnouncements(c *gin.Context) {
	page := minInt(maxInt(getQueryInt(c, "page", 1), 1), 100000)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 12), 1), 100)
	query := a.db.Model(&SystemAnnouncement{})

	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		if !validAnnouncementStatus(status) {
			writeError(c, http.StatusBadRequest, "invalid_status", "公告状态无效")
			return
		}
		query = query.Where("status = ?", status)
	}
	if level := strings.TrimSpace(c.Query("level")); level != "" && level != "all" {
		if !validAnnouncementLevel(level) {
			writeError(c, http.StatusBadRequest, "invalid_level", "公告级别无效")
			return
		}
		query = query.Where("level = ?", level)
	}
	if client := strings.TrimSpace(c.Query("client")); client != "" && client != "all" {
		client = normalizeAnnouncementClient(client)
		if client == "" {
			writeError(c, http.StatusBadRequest, "invalid_client", "公告投放端无效")
			return
		}
		query = applyAnnouncementTargetClientFilter(query, client)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR content LIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcements_count_failed", "公告统计失败")
		return
	}
	var items []SystemAnnouncement
	if err := query.Order("priority desc, published_at desc, created_at desc, id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcements_load_failed", "公告读取失败")
		return
	}
	summary, err := a.adminAnnouncementSummary()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "announcements_summary_failed", "公告统计失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"summary":   summary,
	})
}

func (a *App) handleCreateAnnouncement(c *gin.Context) {
	var req announcementPayloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	admin := currentAdmin(c)
	announcement := SystemAnnouncement{
		CreatedByID:   admin.ID,
		CreatedByName: fallbackString(admin.DisplayName, admin.Username),
	}
	if err := applyAnnouncementPayload(&announcement, req, true); err != nil {
		writeError(c, http.StatusBadRequest, err.code, err.message)
		return
	}
	if err := a.db.Create(&announcement).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcement_create_failed", "公告保存失败")
		return
	}
	a.writeAdminAudit(c, "announcement.create", "announcement", announcement.ID, gin.H{
		"title":  announcement.Title,
		"level":  announcement.Level,
		"status": announcement.Status,
	})
	writeJSON(c, http.StatusCreated, announcement)
}

func (a *App) handleUpdateAnnouncement(c *gin.Context) {
	var announcement SystemAnnouncement
	if err := a.db.First(&announcement, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "announcement_not_found", "公告不存在")
		return
	}
	var req announcementPayloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if err := applyAnnouncementPayload(&announcement, req, false); err != nil {
		writeError(c, http.StatusBadRequest, err.code, err.message)
		return
	}
	if err := a.db.Save(&announcement).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcement_save_failed", "公告保存失败")
		return
	}
	a.writeAdminAudit(c, "announcement.update", "announcement", announcement.ID, gin.H{
		"title":  announcement.Title,
		"level":  announcement.Level,
		"status": announcement.Status,
	})
	writeJSON(c, http.StatusOK, announcement)
}

func (a *App) handleUpdateAnnouncementStatus(c *gin.Context) {
	var announcement SystemAnnouncement
	if err := a.db.First(&announcement, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "announcement_not_found", "公告不存在")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !validAnnouncementStatus(status) || status == AnnouncementStatusDraft {
		writeError(c, http.StatusBadRequest, "invalid_status", "公告状态无效")
		return
	}
	announcement.Status = status
	if status == AnnouncementStatusPublished && announcement.PublishedAt == nil {
		now := time.Now()
		announcement.PublishedAt = &now
	}
	if err := a.db.Save(&announcement).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcement_save_failed", "公告状态保存失败")
		return
	}
	a.writeAdminAudit(c, "announcement.status.update", "announcement", announcement.ID, gin.H{"status": announcement.Status})
	writeJSON(c, http.StatusOK, announcement)
}

func (a *App) handleListPopupAnnouncements(c *gin.Context) {
	client := normalizeAnnouncementClient(c.Query("client"))
	if client == "" {
		writeError(c, http.StatusBadRequest, "invalid_client", "公告投放端无效")
		return
	}
	user := currentUser(c)
	now := time.Now()
	query := a.db.Model(&SystemAnnouncement{}).
		Joins("LEFT JOIN announcement_receipts ON announcement_receipts.announcement_id = system_announcements.id AND announcement_receipts.user_id = ? AND announcement_receipts.client = ? AND announcement_receipts.dismissed_at IS NOT NULL", user.ID, client).
		Where("system_announcements.status = ?", AnnouncementStatusPublished).
		Where("system_announcements.popup_enabled = ?", true).
		Where("(system_announcements.starts_at IS NULL OR system_announcements.starts_at <= ?)", now).
		Where("(system_announcements.ends_at IS NULL OR system_announcements.ends_at > ?)", now).
		Where("announcement_receipts.id IS NULL")
	query = applyAnnouncementTargetClientFilter(query, client)

	var items []SystemAnnouncement
	if err := query.Order("system_announcements.priority desc, system_announcements.published_at desc, system_announcements.created_at desc, system_announcements.id desc").
		Limit(10).
		Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcements_load_failed", "公告读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handleDismissAnnouncement(c *gin.Context) {
	var announcement SystemAnnouncement
	if err := a.db.First(&announcement, c.Param("id")).Error; err != nil {
		writeError(c, http.StatusNotFound, "announcement_not_found", "公告不存在")
		return
	}
	var req struct {
		Client string `json:"client"`
	}
	_ = c.ShouldBindJSON(&req)
	client := normalizeAnnouncementClient(fallbackString(req.Client, c.Query("client")))
	if client == "" {
		writeError(c, http.StatusBadRequest, "invalid_client", "公告投放端无效")
		return
	}
	now := time.Now()
	receipt := AnnouncementReceipt{
		UserID:         currentUser(c).ID,
		AnnouncementID: announcement.ID,
		Client:         client,
	}
	if err := a.db.Where("user_id = ? AND announcement_id = ? AND client = ?", receipt.UserID, receipt.AnnouncementID, receipt.Client).
		Assign(AnnouncementReceipt{DismissedAt: &now}).
		FirstOrCreate(&receipt).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "announcement_dismiss_failed", "公告关闭记录保存失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func validAnnouncementLevel(level string) bool {
	return level == AnnouncementLevelInfo || level == AnnouncementLevelImportant || level == AnnouncementLevelWarning
}

func validAnnouncementStatus(status string) bool {
	return status == AnnouncementStatusDraft || status == AnnouncementStatusPublished || status == AnnouncementStatusOffline
}

type announcementPayloadRequest struct {
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Level         string   `json:"level"`
	Status        string   `json:"status"`
	TargetClients []string `json:"target_clients"`
	PopupEnabled  *bool    `json:"popup_enabled"`
	StartsAt      *string  `json:"starts_at"`
	EndsAt        *string  `json:"ends_at"`
	Priority      *int     `json:"priority"`
	ActionText    string   `json:"action_text"`
	ActionURL     string   `json:"action_url"`
}

type announcementValidationError struct {
	code    string
	message string
}

func applyAnnouncementPayload(announcement *SystemAnnouncement, req announcementPayloadRequest, isCreate bool) *announcementValidationError {
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" || content == "" {
		return &announcementValidationError{code: "invalid_request", message: "公告标题和内容不能为空"}
	}

	level := strings.TrimSpace(req.Level)
	if level == "" {
		level = AnnouncementLevelInfo
	}
	if !validAnnouncementLevel(level) {
		return &announcementValidationError{code: "invalid_level", message: "公告级别无效"}
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		if isCreate {
			status = AnnouncementStatusPublished
		} else {
			status = announcement.Status
		}
	}
	if !validAnnouncementStatus(status) {
		return &announcementValidationError{code: "invalid_status", message: "公告状态无效"}
	}

	startsAt, err := parseOptionalAnnouncementTime(req.StartsAt)
	if err != nil {
		return &announcementValidationError{code: "invalid_time", message: "公告开始时间格式无效"}
	}
	endsAt, err := parseOptionalAnnouncementTime(req.EndsAt)
	if err != nil {
		return &announcementValidationError{code: "invalid_time", message: "公告结束时间格式无效"}
	}
	if startsAt != nil && endsAt != nil && !endsAt.After(*startsAt) {
		return &announcementValidationError{code: "invalid_time_window", message: "公告结束时间必须晚于开始时间"}
	}

	targetClients := announcement.TargetClients
	if isCreate || req.TargetClients != nil {
		normalized := normalizeAnnouncementTargetClients(req.TargetClients)
		if len(req.TargetClients) > 0 && !allAnnouncementClientsValid(req.TargetClients) {
			return &announcementValidationError{code: "invalid_client", message: "公告投放端无效"}
		}
		targetClients = normalized
	}

	announcement.Title = title
	announcement.Content = content
	announcement.Level = level
	announcement.Status = status
	announcement.TargetClients = targetClients
	if isCreate {
		announcement.PopupEnabled = true
	}
	if req.PopupEnabled != nil {
		announcement.PopupEnabled = *req.PopupEnabled
	}
	announcement.StartsAt = startsAt
	announcement.EndsAt = endsAt
	if req.Priority != nil {
		announcement.Priority = *req.Priority
	} else if isCreate {
		announcement.Priority = 0
	}
	announcement.ActionText = strings.TrimSpace(req.ActionText)
	announcement.ActionURL = strings.TrimSpace(req.ActionURL)
	if announcement.Status == AnnouncementStatusPublished && announcement.PublishedAt == nil {
		now := time.Now()
		announcement.PublishedAt = &now
	}
	if announcement.Status == AnnouncementStatusDraft {
		announcement.PublishedAt = nil
	}
	announcement.NormalizeTargetClients()
	return nil
}

func parseOptionalAnnouncementTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func allAnnouncementClientsValid(clients []string) bool {
	for _, client := range clients {
		if !isKnownAnnouncementClient(strings.TrimSpace(strings.ToLower(client))) {
			return false
		}
	}
	return true
}

func normalizeAnnouncementClient(client string) string {
	client = strings.TrimSpace(strings.ToLower(client))
	if client == "" {
		return AnnouncementClientWeb
	}
	if client == AnnouncementClientWeb || client == AnnouncementClientMPWeixin {
		return client
	}
	return ""
}

func applyAnnouncementTargetClientFilter(query *gorm.DB, client string) *gorm.DB {
	return query.Where(
		"system_announcements.target_clients IS NULL OR system_announcements.target_clients = '' OR system_announcements.target_clients LIKE ? OR system_announcements.target_clients LIKE ?",
		"%\""+AnnouncementClientAll+"\"%",
		"%\""+client+"\"%",
	)
}

func (a *App) adminAnnouncementSummary() (gin.H, error) {
	countStatus := func(status string) (int64, error) {
		var count int64
		err := a.db.Model(&SystemAnnouncement{}).Where("status = ?", status).Count(&count).Error
		return count, err
	}
	total := int64(0)
	if err := a.db.Model(&SystemAnnouncement{}).Count(&total).Error; err != nil {
		return nil, err
	}
	published, err := countStatus(AnnouncementStatusPublished)
	if err != nil {
		return nil, err
	}
	draft, err := countStatus(AnnouncementStatusDraft)
	if err != nil {
		return nil, err
	}
	offline, err := countStatus(AnnouncementStatusOffline)
	if err != nil {
		return nil, err
	}
	var popupEnabled int64
	if err := a.db.Model(&SystemAnnouncement{}).Where("popup_enabled = ?", true).Count(&popupEnabled).Error; err != nil {
		return nil, err
	}
	return gin.H{
		"total":         total,
		"published":     published,
		"draft":         draft,
		"offline":       offline,
		"popup_enabled": popupEnabled,
	}, nil
}

func (a *App) handleGetImageSettings(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"active_model":              settings.ActiveImageModel,
		"allowed_models":            settings.AllowedImageModels(),
		"request_timeout_seconds":   settings.RequestTimeoutSeconds,
		"default_invite_quota":      settings.DefaultInviteQuota,
		"rate_limit_window_seconds": settings.RateLimitWindowSeconds,
		"rate_limit_max_requests":   settings.RateLimitMaxRequests,
	})
}

func (a *App) handlePutImageSettings(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}

	var req struct {
		ActiveModel            string `json:"active_model"`
		RequestTimeoutSeconds  int    `json:"request_timeout_seconds"`
		DefaultInviteQuota     int    `json:"default_invite_quota"`
		RateLimitWindowSeconds int    `json:"rate_limit_window_seconds"`
		RateLimitMaxRequests   int    `json:"rate_limit_max_requests"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if req.ActiveModel != "" && !contains(settings.AllowedImageModels(), req.ActiveModel) {
		writeError(c, http.StatusUnprocessableEntity, "invalid_model", "模型不在白名单内")
		return
	}
	if req.ActiveModel != "" {
		settings.ActiveImageModel = req.ActiveModel
	}
	if req.RequestTimeoutSeconds > 0 {
		settings.RequestTimeoutSeconds = req.RequestTimeoutSeconds
	}
	if req.DefaultInviteQuota > 0 {
		settings.DefaultInviteQuota = req.DefaultInviteQuota
	}
	if req.RateLimitWindowSeconds > 0 {
		settings.RateLimitWindowSeconds = req.RateLimitWindowSeconds
	}
	if req.RateLimitMaxRequests > 0 {
		settings.RateLimitMaxRequests = req.RateLimitMaxRequests
	}
	if err := a.db.Save(&settings).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "settings_save_failed", "配置保存失败")
		return
	}
	a.writeAdminAudit(c, "settings.image.update", "settings", settings.ID, gin.H{
		"active_model": settings.ActiveImageModel,
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}
