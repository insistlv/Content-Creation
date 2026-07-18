package app

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	systemRequestLogRetentionDays    = 30
	requestLogErrorCodeKey           = "requestLogErrorCode"
	requestLogErrorMessageKey        = "requestLogErrorMessage"
	requestLogErrorDetailKey         = "requestLogErrorDetail"
	systemLogCategoryUserLogin       = "user_login"
	systemLogCategoryUserOperation   = "user_operation"
	systemLogCategorySystemOperation = "system_operation"
)

var (
	userLoginLogPaths       = []string{"/api/auth/login", "/api/auth/wechat-login", "/api/auth/wechat-phone-login"}
	userOperationLogMethods = []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete}
)

type adminSystemLogsSummary struct {
	Total            int64      `json:"total"`
	ErrorTotal       int64      `json:"error_total"`
	RecentErrorTotal int64      `json:"recent_error_total"`
	LastErrorAt      *time.Time `json:"last_error_at"`
}

type adminSystemLogCategorySummary struct {
	Total        int64      `json:"total"`
	SuccessTotal int64      `json:"success_total"`
	FailedTotal  int64      `json:"failed_total"`
	RecentTotal  int64      `json:"recent_total"`
	LastEventAt  *time.Time `json:"last_event_at"`
}

type adminSystemLogItem struct {
	ID            uint      `json:"id"`
	Category      string    `json:"category"`
	RequestID     string    `json:"request_id,omitempty"`
	Level         string    `json:"level,omitempty"`
	Method        string    `json:"method,omitempty"`
	Path          string    `json:"path,omitempty"`
	StatusCode    int       `json:"status_code,omitempty"`
	DurationMs    int64     `json:"duration_ms,omitempty"`
	IPAddress     string    `json:"ip_address,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
	UserID        *uint     `json:"user_id,omitempty"`
	UserUsername  string    `json:"user_username,omitempty"`
	AdminUserID   *uint     `json:"admin_user_id,omitempty"`
	AdminUsername string    `json:"admin_username,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	ErrorDetail   string    `json:"error_detail,omitempty"`
	Action        string    `json:"action,omitempty"`
	TargetType    string    `json:"target_type,omitempty"`
	TargetID      uint      `json:"target_id,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (a *App) requestLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}

		startedAt := time.Now()
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set("requestID", requestID)
		c.Header("X-Request-ID", requestID)

		defer func() {
			if recovered := recover(); recovered != nil {
				c.Set(requestLogErrorCodeKey, "panic")
				c.Set(requestLogErrorMessageKey, fmt.Sprint(recovered))
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": gin.H{"code": "internal_server_error", "message": "服务器内部错误"},
					})
				} else {
					c.Abort()
				}
			}
			a.writeSystemRequestLog(c, requestID, startedAt)
		}()

		c.Next()
	}
}

func (a *App) writeSystemRequestLog(c *gin.Context, requestID string, startedAt time.Time) {
	status := c.Writer.Status()
	if status <= 0 {
		status = http.StatusOK
	}
	entry := SystemRequestLog{
		RequestID:    requestID,
		Level:        systemRequestLogLevel(status),
		Method:       c.Request.Method,
		Path:         c.Request.URL.Path,
		StatusCode:   status,
		DurationMs:   time.Since(startedAt).Milliseconds(),
		IPAddress:    clientIP(c.Request),
		UserAgent:    c.Request.UserAgent(),
		ErrorCode:    stringContextValue(c, requestLogErrorCodeKey),
		ErrorMessage: stringContextValue(c, requestLogErrorMessageKey),
		ErrorDetail:  stringContextValue(c, requestLogErrorDetailKey),
		CreatedAt:    startedAt,
	}
	if userValue, exists := c.Get("currentUser"); exists {
		if user, ok := userValue.(*User); ok && user != nil {
			entry.UserID = &user.ID
			entry.UserUsername = fallbackString(user.DisplayName, user.Username)
		}
	}
	if adminValue, exists := c.Get("currentAdmin"); exists {
		if admin, ok := adminValue.(*AdminUser); ok && admin != nil {
			entry.AdminUserID = &admin.ID
			entry.AdminUsername = fallbackString(admin.DisplayName, admin.Username)
		}
	}
	if err := a.db.Create(&entry).Error; err != nil {
		log.Printf("system request log write failed: %v", err)
	}
}

func systemRequestLogLevel(status int) string {
	switch {
	case status >= 500:
		return SystemRequestLogLevelError
	case status >= 400:
		return SystemRequestLogLevelWarn
	default:
		return SystemRequestLogLevelInfo
	}
}

func stringContextValue(c *gin.Context, key string) string {
	value, exists := c.Get(key)
	if !exists {
		return ""
	}
	text, _ := value.(string)
	return text
}

func (a *App) handleListSystemLogs(c *gin.Context) {
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 30)
	if pageSize > 100 {
		pageSize = 100
	}

	category := strings.TrimSpace(c.Query("category"))
	if category != "" {
		a.handleListCategorizedSystemLogs(c, category, page, pageSize)
		return
	}

	query := a.db.Model(&SystemRequestLog{})
	query = applySystemLogFilters(query, c)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_count_failed", "系统日志统计失败")
		return
	}

	var items []SystemRequestLog
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_load_failed", "系统日志读取失败")
		return
	}

	summary, err := a.systemRequestLogSummary()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_summary_failed", "系统日志汇总失败")
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

func (a *App) handleListCategorizedSystemLogs(c *gin.Context, category string, page, pageSize int) {
	if !isKnownSystemLogCategory(category) {
		writeError(c, http.StatusBadRequest, "invalid_system_log_category", "日志分类无效")
		return
	}
	if category == systemLogCategorySystemOperation {
		a.handleListAdminAuditSystemLogs(c, page, pageSize)
		return
	}

	query := a.db.Model(&SystemRequestLog{})
	query = applySystemRequestLogCategory(query, category)
	query = applySystemLogFilters(query, c)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_count_failed", "系统日志统计失败")
		return
	}

	var logs []SystemRequestLog
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_load_failed", "系统日志读取失败")
		return
	}

	summary, err := a.systemRequestLogCategorySummary(category)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_summary_failed", "系统日志汇总失败")
		return
	}

	items := make([]adminSystemLogItem, 0, len(logs))
	for _, item := range logs {
		items = append(items, requestLogAdminItem(category, item))
	}

	writeJSON(c, http.StatusOK, gin.H{
		"category":  category,
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"summary":   summary,
	})
}

func (a *App) handleListAdminAuditSystemLogs(c *gin.Context, page, pageSize int) {
	query := a.applyAdminAuditLogFilters(a.db.Model(&AdminAuditLog{}), c)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_count_failed", "系统日志统计失败")
		return
	}

	var logs []AdminAuditLog
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_load_failed", "系统日志读取失败")
		return
	}

	summary, err := a.adminAuditLogSummary()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_summary_failed", "系统日志汇总失败")
		return
	}

	adminNames := a.adminUsernamesForAuditLogs(logs)
	items := make([]adminSystemLogItem, 0, len(logs))
	for _, item := range logs {
		items = append(items, adminAuditLogItem(item, adminNames[item.AdminUserID]))
	}

	writeJSON(c, http.StatusOK, gin.H{
		"category":  systemLogCategorySystemOperation,
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"summary":   summary,
	})
}

func applySystemLogFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	if level := strings.TrimSpace(c.Query("level")); level != "" {
		query = query.Where("level = ?", level)
	}
	if method := strings.ToUpper(strings.TrimSpace(c.Query("method"))); method != "" {
		query = query.Where("method = ?", method)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		if value, err := strconv.Atoi(status); err == nil {
			query = query.Where("status_code = ?", value)
		}
	}
	if keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword"))); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(path) LIKE ? OR LOWER(error_code) LIKE ? OR LOWER(error_message) LIKE ? OR LOWER(error_detail) LIKE ? OR LOWER(request_id) LIKE ? OR LOWER(user_username) LIKE ? OR LOWER(admin_username) LIKE ? OR LOWER(ip_address) LIKE ? OR LOWER(user_agent) LIKE ?", like, like, like, like, like, like, like, like, like)
	}
	if from, ok := parseSystemLogDate(c.Query("date_from"), false); ok {
		query = query.Where("created_at >= ?", from)
	}
	if to, ok := parseSystemLogDate(c.Query("date_to"), true); ok {
		query = query.Where("created_at < ?", to)
	}
	return query
}

func applySystemRequestLogCategory(query *gorm.DB, category string) *gorm.DB {
	switch category {
	case systemLogCategoryUserLogin:
		return query.Where("path IN ?", userLoginLogPaths)
	case systemLogCategoryUserOperation:
		return query.Where("user_id IS NOT NULL").Where("method IN ?", userOperationLogMethods).Where("path NOT IN ?", userLoginLogPaths)
	default:
		return query
	}
}

func (a *App) applyAdminAuditLogFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	if keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword"))); keyword != "" {
		like := "%" + keyword + "%"
		var adminIDs []uint
		_ = a.db.Model(&AdminUser{}).
			Where("LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?", like, like).
			Pluck("id", &adminIDs).Error
		if len(adminIDs) > 0 {
			query = query.Where("LOWER(action) LIKE ? OR LOWER(target_type) LIKE ? OR LOWER(detail) LIKE ? OR LOWER(ip_address) LIKE ? OR admin_user_id IN ?", like, like, like, like, adminIDs)
		} else {
			query = query.Where("LOWER(action) LIKE ? OR LOWER(target_type) LIKE ? OR LOWER(detail) LIKE ? OR LOWER(ip_address) LIKE ?", like, like, like, like)
		}
	}
	if from, ok := parseSystemLogDate(c.Query("date_from"), false); ok {
		query = query.Where("created_at >= ?", from)
	}
	if to, ok := parseSystemLogDate(c.Query("date_to"), true); ok {
		query = query.Where("created_at < ?", to)
	}
	return query
}

func isKnownSystemLogCategory(category string) bool {
	switch category {
	case systemLogCategoryUserLogin, systemLogCategoryUserOperation, systemLogCategorySystemOperation:
		return true
	default:
		return false
	}
}

func (a *App) systemRequestLogSummary() (adminSystemLogsSummary, error) {
	var summary adminSystemLogsSummary
	if err := a.db.Model(&SystemRequestLog{}).Count(&summary.Total).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&SystemRequestLog{}).Where("level = ?", SystemRequestLogLevelError).Count(&summary.ErrorTotal).Error; err != nil {
		return summary, err
	}
	if err := a.db.Model(&SystemRequestLog{}).Where("level = ? AND created_at >= ?", SystemRequestLogLevelError, time.Now().Add(-24*time.Hour)).Count(&summary.RecentErrorTotal).Error; err != nil {
		return summary, err
	}
	var last SystemRequestLog
	err := a.db.Where("level = ?", SystemRequestLogLevelError).Order("created_at desc").First(&last).Error
	if err == nil {
		summary.LastErrorAt = &last.CreatedAt
		return summary, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, nil
	}
	return summary, err
}

func (a *App) systemRequestLogCategorySummary(category string) (adminSystemLogCategorySummary, error) {
	var summary adminSystemLogCategorySummary
	base := applySystemRequestLogCategory(a.db.Model(&SystemRequestLog{}), category)
	if err := base.Count(&summary.Total).Error; err != nil {
		return summary, err
	}
	if err := applySystemRequestLogCategory(a.db.Model(&SystemRequestLog{}), category).Where("status_code < ?", http.StatusBadRequest).Count(&summary.SuccessTotal).Error; err != nil {
		return summary, err
	}
	if err := applySystemRequestLogCategory(a.db.Model(&SystemRequestLog{}), category).Where("status_code >= ?", http.StatusBadRequest).Count(&summary.FailedTotal).Error; err != nil {
		return summary, err
	}
	if err := applySystemRequestLogCategory(a.db.Model(&SystemRequestLog{}), category).Where("created_at >= ?", time.Now().Add(-24*time.Hour)).Count(&summary.RecentTotal).Error; err != nil {
		return summary, err
	}
	var last SystemRequestLog
	err := applySystemRequestLogCategory(a.db.Model(&SystemRequestLog{}), category).Order("created_at desc").First(&last).Error
	if err == nil {
		summary.LastEventAt = &last.CreatedAt
		return summary, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, nil
	}
	return summary, err
}

func (a *App) adminAuditLogSummary() (adminSystemLogCategorySummary, error) {
	var summary adminSystemLogCategorySummary
	if err := a.db.Model(&AdminAuditLog{}).Count(&summary.Total).Error; err != nil {
		return summary, err
	}
	summary.SuccessTotal = summary.Total
	if err := a.db.Model(&AdminAuditLog{}).Where("created_at >= ?", time.Now().Add(-24*time.Hour)).Count(&summary.RecentTotal).Error; err != nil {
		return summary, err
	}
	var last AdminAuditLog
	err := a.db.Order("created_at desc").First(&last).Error
	if err == nil {
		summary.LastEventAt = &last.CreatedAt
		return summary, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, nil
	}
	return summary, err
}

func requestLogAdminItem(category string, item SystemRequestLog) adminSystemLogItem {
	return adminSystemLogItem{
		ID:            item.ID,
		Category:      category,
		RequestID:     item.RequestID,
		Level:         item.Level,
		Method:        item.Method,
		Path:          item.Path,
		StatusCode:    item.StatusCode,
		DurationMs:    item.DurationMs,
		IPAddress:     item.IPAddress,
		UserAgent:     item.UserAgent,
		UserID:        item.UserID,
		UserUsername:  item.UserUsername,
		AdminUserID:   item.AdminUserID,
		AdminUsername: item.AdminUsername,
		ErrorCode:     item.ErrorCode,
		ErrorMessage:  item.ErrorMessage,
		ErrorDetail:   item.ErrorDetail,
		CreatedAt:     item.CreatedAt,
	}
}

func adminAuditLogItem(item AdminAuditLog, adminUsername string) adminSystemLogItem {
	adminID := item.AdminUserID
	return adminSystemLogItem{
		ID:            item.ID,
		Category:      systemLogCategorySystemOperation,
		Level:         SystemRequestLogLevelInfo,
		IPAddress:     item.IPAddress,
		AdminUserID:   &adminID,
		AdminUsername: adminUsername,
		Action:        item.Action,
		TargetType:    item.TargetType,
		TargetID:      item.TargetID,
		Detail:        item.Detail,
		CreatedAt:     item.CreatedAt,
	}
}

func (a *App) adminUsernamesForAuditLogs(logs []AdminAuditLog) map[uint]string {
	ids := make([]uint, 0, len(logs))
	seen := map[uint]bool{}
	for _, item := range logs {
		if item.AdminUserID == 0 || seen[item.AdminUserID] {
			continue
		}
		seen[item.AdminUserID] = true
		ids = append(ids, item.AdminUserID)
	}
	if len(ids) == 0 {
		return map[uint]string{}
	}
	var admins []AdminUser
	if err := a.db.Where("id IN ?", ids).Find(&admins).Error; err != nil {
		return map[uint]string{}
	}
	names := make(map[uint]string, len(admins))
	for _, admin := range admins {
		names[admin.ID] = fallbackString(admin.DisplayName, admin.Username)
	}
	return names
}

func (a *App) handleExportSystemLogs(c *gin.Context) {
	category := strings.TrimSpace(c.Query("category"))
	if !isKnownSystemLogCategory(category) {
		writeError(c, http.StatusBadRequest, "invalid_system_log_category", "日志分类无效")
		return
	}
	if category == systemLogCategorySystemOperation {
		a.exportAdminAuditSystemLogs(c)
		return
	}
	a.exportSystemRequestLogs(c, category)
}

func (a *App) exportSystemRequestLogs(c *gin.Context, category string) {
	query := a.db.Model(&SystemRequestLog{})
	query = applySystemRequestLogCategory(query, category)
	query = applySystemLogFilters(query, c)

	var logs []SystemRequestLog
	if err := query.Order("created_at desc").Find(&logs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_export_failed", "日志导出失败")
		return
	}

	if category == systemLogCategoryUserLogin {
		rows := make([][]string, 0, len(logs))
		for _, item := range logs {
			rows = append(rows, []string{
				formatCSVTime(&item.CreatedAt),
				requestLogResultLabel(item.StatusCode),
				strconv.Itoa(item.StatusCode),
				item.UserUsername,
				item.RequestID,
				item.IPAddress,
				item.UserAgent,
				item.ErrorCode,
				item.ErrorMessage,
			})
		}
		writeCSV(c, "user-login-logs.csv", []string{"时间", "结果", "状态码", "用户", "请求ID", "IP", "User-Agent", "错误码", "错误消息"}, rows)
		return
	}

	rows := make([][]string, 0, len(logs))
	for _, item := range logs {
		rows = append(rows, []string{
			formatCSVTime(&item.CreatedAt),
			levelCSVLabel(item.Level),
			item.Method,
			item.Path,
			strconv.Itoa(item.StatusCode),
			item.UserUsername,
			strconv.FormatInt(item.DurationMs, 10),
			item.IPAddress,
			item.RequestID,
			item.ErrorCode,
			item.ErrorDetail,
		})
	}
	writeCSV(c, "user-operation-logs.csv", []string{"时间", "级别", "方法", "路径", "状态码", "用户", "耗时(ms)", "IP", "请求ID", "错误码", "诊断信息"}, rows)
}

func (a *App) exportAdminAuditSystemLogs(c *gin.Context) {
	query := a.applyAdminAuditLogFilters(a.db.Model(&AdminAuditLog{}), c)

	var logs []AdminAuditLog
	if err := query.Order("created_at desc").Find(&logs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "system_logs_export_failed", "日志导出失败")
		return
	}

	adminNames := a.adminUsernamesForAuditLogs(logs)
	rows := make([][]string, 0, len(logs))
	for _, item := range logs {
		rows = append(rows, []string{
			formatCSVTime(&item.CreatedAt),
			adminNames[item.AdminUserID],
			item.Action,
			item.TargetType,
			strconv.FormatUint(uint64(item.TargetID), 10),
			item.IPAddress,
			item.Detail,
		})
	}
	writeCSV(c, "system-operation-logs.csv", []string{"时间", "管理员", "动作", "目标类型", "目标ID", "IP", "详情"}, rows)
}

func requestLogResultLabel(status int) string {
	if status >= http.StatusBadRequest {
		return "失败"
	}
	return "成功"
}

func levelCSVLabel(level string) string {
	switch level {
	case SystemRequestLogLevelError:
		return "错误"
	case SystemRequestLogLevelWarn:
		return "警告"
	default:
		return "信息"
	}
}

func parseSystemLogDate(value string, exclusiveEnd bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		if exclusiveEnd {
			parsed = parsed.AddDate(0, 0, 1)
		}
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (a *App) cleanupOldSystemRequestLogs(now time.Time) error {
	cutoff := now.AddDate(0, 0, -systemRequestLogRetentionDays)
	err := a.db.Where("created_at < ?", cutoff).Delete(&SystemRequestLog{}).Error
	if isMissingDatabaseObjectError(err) {
		return nil
	}
	return err
}

func (a *App) startSystemRequestLogCleanupTask() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-a.cleanupStop:
				return
			}
			if err := a.cleanupOldSystemRequestLogs(time.Now()); err != nil {
				log.Printf("system request log cleanup failed: %v", err)
			}
		}
	}()
}
