package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestLoggingCreatesInfoWarnAndErrorLogsWithoutBodies(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	user, userCookies := createLoggedInUser(t, testApp, "log-user", "UserPass123")
	adminCookies := createAdminSession(t, testApp)

	okResp := performJSONRequestWithHeaders(t, testApp, http.MethodGet, "/api/me", nil, userCookies, map[string]string{
		"User-Agent":      "ImageAgentTest/1.0",
		"X-Forwarded-For": "203.0.113.9, 10.0.0.1",
	})
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected /api/me 200, got %d: %s", okResp.Code, okResp.Body.String())
	}

	unauthorizedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credits", nil, nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized request 401, got %d", unauthorizedResp.Code)
	}

	settingsResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/system-settings", map[string]any{
		"platform": map[string]any{"name": ""},
		"password": "secret-password",
		"token":    "secret-token",
	}, adminCookies)
	if settingsResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid settings 400, got %d: %s", settingsResp.Code, settingsResp.Body.String())
	}

	var logs []SystemRequestLog
	if err := db.Order("created_at asc").Find(&logs).Error; err != nil {
		t.Fatalf("load request logs: %v", err)
	}
	if len(logs) < 3 {
		t.Fatalf("expected at least 3 logs, got %d", len(logs))
	}

	var okLog, warnLog, errorLog SystemRequestLog
	for _, logItem := range logs {
		switch logItem.Path {
		case "/api/me":
			okLog = logItem
		case "/api/account/credits":
			warnLog = logItem
		case "/api/admin/system-settings":
			errorLog = logItem
		}
	}
	if okLog.Level != "info" || okLog.StatusCode != http.StatusOK || okLog.UserID == nil || *okLog.UserID != user.ID {
		t.Fatalf("expected info user log for /api/me, got %+v", okLog)
	}
	if okLog.IPAddress != "203.0.113.9" || okLog.UserAgent != "ImageAgentTest/1.0" {
		t.Fatalf("expected client headers to be captured, got ip=%q ua=%q", okLog.IPAddress, okLog.UserAgent)
	}
	if warnLog.Level != "warn" || warnLog.StatusCode != http.StatusUnauthorized || warnLog.ErrorCode != "unauthorized" {
		t.Fatalf("expected warn unauthorized log, got %+v", warnLog)
	}
	if errorLog.Level != "warn" || errorLog.StatusCode != http.StatusBadRequest || errorLog.ErrorCode != "invalid_system_settings" || errorLog.ErrorMessage != "系统设置无效" {
		t.Fatalf("expected writeError details on bad request log, got %+v", errorLog)
	}
	serialized, _ := json.Marshal(logs)
	if strings.Contains(string(serialized), "secret-password") || strings.Contains(string(serialized), "secret-token") {
		t.Fatalf("request logs must not contain request body secrets: %s", string(serialized))
	}
}

func TestRequestLoggingCapturesAlipayNotConfiguredAndPanic(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	user, cookies := createLoggedInUser(t, testApp, "pay-log-user", "UserPass123")
	setUserPhoneForTest(t, testApp, user.ID, "13800139305")
	setUserCredits(t, testApp, user.ID, 100)
	var pkg Package
	if err := db.Where("is_active = ?", true).First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	orderResp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders", map[string]any{
		"package_id": pkg.ID,
	}, cookies)
	if orderResp.Code != http.StatusCreated {
		t.Fatalf("expected order create 201, got %d: %s", orderResp.Code, orderResp.Body.String())
	}
	var orderPayload struct {
		OrderNumber string `json:"order_number"`
	}
	if err := json.Unmarshal(orderResp.Body.Bytes(), &orderPayload); err != nil {
		t.Fatalf("decode order payload: %v", err)
	}

	payResp := performJSONRequest(t, testApp, http.MethodPost, "/api/payments/alipay/orders/"+orderPayload.OrderNumber+"/pay", map[string]any{}, cookies)
	if payResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected alipay pay 503, got %d: %s", payResp.Code, payResp.Body.String())
	}

	testApp.router.GET("/api/test-panic", func(c *gin.Context) {
		panic("boom")
	})
	panicResp := performJSONRequest(t, testApp, http.MethodGet, "/api/test-panic", nil, nil)
	if panicResp.Code != http.StatusInternalServerError {
		t.Fatalf("expected panic response 500, got %d: %s", panicResp.Code, panicResp.Body.String())
	}

	var payLog SystemRequestLog
	if err := db.Where("path LIKE ? AND status_code = ?", "/api/payments/alipay/orders/%/pay", http.StatusServiceUnavailable).First(&payLog).Error; err != nil {
		t.Fatalf("load alipay pay log: %v", err)
	}
	if payLog.Level != "error" || payLog.ErrorCode != "alipay_not_configured" || payLog.ErrorMessage != alipayMaintenanceMessage {
		t.Fatalf("expected alipay business error in request log, got %+v", payLog)
	}

	var panicLog SystemRequestLog
	if err := db.Where("path = ?", "/api/test-panic").First(&panicLog).Error; err != nil {
		t.Fatalf("load panic log: %v", err)
	}
	if panicLog.Level != "error" || panicLog.StatusCode != http.StatusInternalServerError || panicLog.ErrorCode != "panic" || !strings.Contains(panicLog.ErrorMessage, "boom") {
		t.Fatalf("expected panic log, got %+v", panicLog)
	}
}

func TestAdminSystemLogsEndpointFiltersSummarizesAndRequiresPermission(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	now := time.Now()
	logs := []SystemRequestLog{
		{RequestID: "req-old", Level: "error", Method: "GET", Path: "/api/old", StatusCode: 500, DurationMs: 1, CreatedAt: now.Add(-48 * time.Hour), ErrorCode: "old_error"},
		{RequestID: "req-pay", Level: "error", Method: "POST", Path: "/api/payments/alipay/orders/FO-1/pay", StatusCode: 503, DurationMs: 42, IPAddress: "198.51.100.7", UserAgent: "UA", ErrorCode: "alipay_not_configured", ErrorMessage: alipayMaintenanceMessage, ErrorDetail: "alipay_private_key_invalid_or_sign_failed: illegal base64 data", CreatedAt: now.Add(-1 * time.Hour)},
		{RequestID: "req-login", Level: "warn", Method: "POST", Path: "/api/auth/login", StatusCode: 401, DurationMs: 8, ErrorCode: "login_failed", ErrorMessage: "账号或密码错误", CreatedAt: now.Add(-2 * time.Hour)},
		{RequestID: "req-ok", Level: "info", Method: "GET", Path: "/api/me", StatusCode: 200, DurationMs: 3, CreatedAt: now.Add(-3 * time.Hour)},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	limited := createDatabaseAdminUser(t, db, "logs-limited", "LimitedPass123")
	limitedCookies := loginAdminAs(t, testApp, limited.Username, "LimitedPass123")
	limitedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs", nil, limitedCookies)
	if limitedResp.Code != http.StatusForbidden {
		t.Fatalf("expected limited admin 403, got %d: %s", limitedResp.Code, limitedResp.Body.String())
	}

	// 系统日志为敏感能力，受 requireStrictAdminPermission 保护，仅 super_admin 可访问；
	// auditor 等自定义角色即便拥有读权限也应被拒绝。
	auditor := createDatabaseAdminUser(t, db, "logs-auditor", "LimitedPass123")
	assignAdminRoleByCode(t, db, &auditor, "auditor")
	auditorCookies := loginAdminAs(t, testApp, auditor.Username, "LimitedPass123")
	auditorResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs", nil, auditorCookies)
	if auditorResp.Code != http.StatusForbidden {
		t.Fatalf("expected auditor system logs 403 (super_admin only), got %d: %s", auditorResp.Code, auditorResp.Body.String())
	}

	superAdmin := createDatabaseAdminUser(t, db, "logs-superadmin", "SuperPass123")
	assignAdminRoleByCode(t, db, &superAdmin, "super_admin")
	superCookies := loginAdminAs(t, testApp, superAdmin.Username, "SuperPass123")
	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs?level=error&method=POST&status=503&keyword=alipay&page=1&page_size=30", nil, superCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected super_admin system logs 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Items   []SystemRequestLog `json:"items"`
		Total   int64              `json:"total"`
		Page    int                `json:"page"`
		Summary struct {
			Total            int64      `json:"total"`
			ErrorTotal       int64      `json:"error_total"`
			RecentErrorTotal int64      `json:"recent_error_total"`
			LastErrorAt      *time.Time `json:"last_error_at"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode system logs payload: %v", err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].RequestID != "req-pay" {
		t.Fatalf("expected filtered pay log only, got %+v", payload)
	}
	if payload.Items[0].ErrorDetail != "alipay_private_key_invalid_or_sign_failed: illegal base64 data" {
		t.Fatalf("expected system logs endpoint to include error_detail, got %+v", payload.Items[0])
	}
	if payload.Summary.Total < 4 || payload.Summary.ErrorTotal < 2 || payload.Summary.RecentErrorTotal < 1 || payload.Summary.LastErrorAt == nil {
		t.Fatalf("expected summary counts and last error, got %+v", payload.Summary)
	}

	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, superCookies)
	if !containsString(meResp.Body.String(), "system_logs.read") || !containsString(meResp.Body.String(), "/admin/system-logs") {
		t.Fatalf("expected super_admin session to include system logs permission and menu, got %s", meResp.Body.String())
	}

	auditorMeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, auditorCookies)
	if containsString(auditorMeResp.Body.String(), "system_logs.read") {
		t.Fatalf("expected auditor session NOT to expose system_logs.read, got %s", auditorMeResp.Body.String())
	}
}

func TestUserLoginLogsCaptureSuccessfulUserContextAndHideFailureBody(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	user, _ := createLoggedInUser(t, testApp, "login-audit-user", "UserPass123")
	failedResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "login-audit-user", "WrongPass123"), nil, map[string]string{
		"User-Agent":      "LoginAuditTest/1.0",
		"X-Forwarded-For": "198.51.100.45",
	})
	if failedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected failed login 401, got %d: %s", failedResp.Code, failedResp.Body.String())
	}
	if err := db.Model(&User{}).Where("id = ?", user.ID).Update("wechat_open_id", "openid-login-audit").Error; err != nil {
		t.Fatalf("set user openid: %v", err)
	}
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-audit-code": "openid-login-audit",
	}}
	wechatResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/wechat-login", map[string]any{
		"code": "login-audit-code",
	}, nil)
	if wechatResp.Code != http.StatusOK {
		t.Fatalf("expected wechat login 200, got %d: %s", wechatResp.Code, wechatResp.Body.String())
	}

	var logs []SystemRequestLog
	if err := db.Where("path = ?", "/api/auth/login").Order("created_at asc").Find(&logs).Error; err != nil {
		t.Fatalf("load login logs: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected successful and failed login logs, got %+v", logs)
	}

	var successLog, failureLog SystemRequestLog
	for _, item := range logs {
		switch item.StatusCode {
		case http.StatusOK:
			successLog = item
		case http.StatusUnauthorized:
			failureLog = item
		}
	}
	if successLog.UserID == nil || *successLog.UserID != user.ID || successLog.UserUsername != user.Username {
		t.Fatalf("expected successful login log to include user context, got %+v", successLog)
	}
	if failureLog.UserID != nil || failureLog.UserUsername != "" || failureLog.ErrorCode != "login_failed" {
		t.Fatalf("expected failed login log to expose only failure metadata, got %+v", failureLog)
	}
	if failureLog.IPAddress != "198.51.100.45" || failureLog.UserAgent != "LoginAuditTest/1.0" {
		t.Fatalf("expected failed login log to retain IP and UA, got %+v", failureLog)
	}
	serialized, _ := json.Marshal(logs)
	if strings.Contains(string(serialized), "UserPass123") || strings.Contains(string(serialized), "WrongPass123") {
		t.Fatalf("login request logs must not contain passwords: %s", string(serialized))
	}

	var wechatLog SystemRequestLog
	if err := db.Where("path = ? AND status_code = ?", "/api/auth/wechat-login", http.StatusOK).First(&wechatLog).Error; err != nil {
		t.Fatalf("load wechat login log: %v", err)
	}
	if wechatLog.UserID == nil || *wechatLog.UserID != user.ID || wechatLog.UserUsername != user.Username {
		t.Fatalf("expected successful wechat login log to include user context, got %+v", wechatLog)
	}
}

func TestAdminSystemLogsEndpointCategoriesAndCSVExport(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "category-log-user", "UserPass123")
	// 系统日志仅 super_admin 可访问（requireStrictAdminPermission）。
	auditor := createDatabaseAdminUser(t, db, "category-auditor", "LimitedPass123")
	assignAdminRoleByCode(t, db, &auditor, "super_admin")
	auditorCookies := loginAdminAs(t, testApp, auditor.Username, "LimitedPass123")

	if err := db.Exec("DELETE FROM system_request_logs").Error; err != nil {
		t.Fatalf("clear request logs: %v", err)
	}
	if err := db.Exec("DELETE FROM admin_audit_logs").Error; err != nil {
		t.Fatalf("clear audit logs: %v", err)
	}

	now := time.Now()
	userID := user.ID
	if err := db.Create(&[]SystemRequestLog{
		{RequestID: "req-login-success", Level: "info", Method: "POST", Path: "/api/auth/login", StatusCode: 200, DurationMs: 21, IPAddress: "203.0.113.10", UserAgent: "Browser/1", UserID: &userID, UserUsername: user.Username, CreatedAt: now.Add(-4 * time.Hour)},
		{RequestID: "req-login-fail", Level: "warn", Method: "POST", Path: "/api/auth/login", StatusCode: 401, DurationMs: 9, IPAddress: "203.0.113.11", UserAgent: "Browser/2", ErrorCode: "login_failed", ErrorMessage: "账号或密码错误", CreatedAt: now.Add(-3 * time.Hour)},
		{RequestID: "req-image-create", Level: "error", Method: "POST", Path: "/api/images/generations/async", StatusCode: 503, DurationMs: 420, IPAddress: "203.0.113.12", UserAgent: "Browser/3", UserID: &userID, UserUsername: user.Username, ErrorCode: "provider_timeout", ErrorDetail: "gpt-image-2 upstream timeout", CreatedAt: now.Add(-2 * time.Hour)},
		{RequestID: "req-user-read", Level: "info", Method: "GET", Path: "/api/me", StatusCode: 200, DurationMs: 3, UserID: &userID, UserUsername: user.Username, CreatedAt: now.Add(-1 * time.Hour)},
	}).Error; err != nil {
		t.Fatalf("seed request logs: %v", err)
	}
	if err := db.Create(&AdminAuditLog{
		AdminUserID: auditor.ID,
		Action:      "model.update",
		TargetType:  "model_config",
		TargetID:    7,
		Detail:      `{"model":"gpt-image-2","enabled":true}`,
		IPAddress:   "198.51.100.8",
		CreatedAt:   now.Add(-30 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("seed audit log: %v", err)
	}

	var categoryPayload struct {
		Items []struct {
			Category      string `json:"category"`
			RequestID     string `json:"request_id"`
			Path          string `json:"path"`
			Method        string `json:"method"`
			StatusCode    int    `json:"status_code"`
			UserID        *uint  `json:"user_id"`
			UserUsername  string `json:"user_username"`
			AdminUsername string `json:"admin_username"`
			Action        string `json:"action"`
			TargetType    string `json:"target_type"`
			Detail        string `json:"detail"`
			ErrorDetail   string `json:"error_detail"`
		} `json:"items"`
		Total   int64 `json:"total"`
		Summary struct {
			Total        int64      `json:"total"`
			SuccessTotal int64      `json:"success_total"`
			FailedTotal  int64      `json:"failed_total"`
			RecentTotal  int64      `json:"recent_total"`
			LastEventAt  *time.Time `json:"last_event_at"`
		} `json:"summary"`
	}

	loginResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs?category=user_login&page_size=10", nil, auditorCookies)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected user login logs 200, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	if err := json.Unmarshal(loginResp.Body.Bytes(), &categoryPayload); err != nil {
		t.Fatalf("decode login category payload: %v", err)
	}
	if categoryPayload.Total != 2 || len(categoryPayload.Items) != 2 || categoryPayload.Items[0].Category != "user_login" || categoryPayload.Items[0].RequestID != "req-login-fail" {
		t.Fatalf("expected only login logs ordered newest first, got %+v", categoryPayload)
	}
	if categoryPayload.Items[0].UserID != nil || categoryPayload.Items[1].UserID == nil || *categoryPayload.Items[1].UserID != user.ID {
		t.Fatalf("expected failed login without user and success with user context, got %+v", categoryPayload.Items)
	}
	if categoryPayload.Summary.Total != 2 || categoryPayload.Summary.SuccessTotal != 1 || categoryPayload.Summary.FailedTotal != 1 || categoryPayload.Summary.LastEventAt == nil {
		t.Fatalf("expected login category summary, got %+v", categoryPayload.Summary)
	}

	operationResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs?category=user_operation&keyword=gpt-image-2", nil, auditorCookies)
	if operationResp.Code != http.StatusOK {
		t.Fatalf("expected user operation logs 200, got %d: %s", operationResp.Code, operationResp.Body.String())
	}
	categoryPayload = struct {
		Items []struct {
			Category      string `json:"category"`
			RequestID     string `json:"request_id"`
			Path          string `json:"path"`
			Method        string `json:"method"`
			StatusCode    int    `json:"status_code"`
			UserID        *uint  `json:"user_id"`
			UserUsername  string `json:"user_username"`
			AdminUsername string `json:"admin_username"`
			Action        string `json:"action"`
			TargetType    string `json:"target_type"`
			Detail        string `json:"detail"`
			ErrorDetail   string `json:"error_detail"`
		} `json:"items"`
		Total   int64 `json:"total"`
		Summary struct {
			Total        int64      `json:"total"`
			SuccessTotal int64      `json:"success_total"`
			FailedTotal  int64      `json:"failed_total"`
			RecentTotal  int64      `json:"recent_total"`
			LastEventAt  *time.Time `json:"last_event_at"`
		} `json:"summary"`
	}{}
	if err := json.Unmarshal(operationResp.Body.Bytes(), &categoryPayload); err != nil {
		t.Fatalf("decode operation category payload: %v", err)
	}
	if categoryPayload.Total != 1 || len(categoryPayload.Items) != 1 || categoryPayload.Items[0].RequestID != "req-image-create" || categoryPayload.Items[0].Category != "user_operation" {
		t.Fatalf("expected only mutating user operation logs, got %+v", categoryPayload)
	}

	systemResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs?category=system_operation&keyword=model.update", nil, auditorCookies)
	if systemResp.Code != http.StatusOK {
		t.Fatalf("expected system operation logs 200, got %d: %s", systemResp.Code, systemResp.Body.String())
	}
	categoryPayload = struct {
		Items []struct {
			Category      string `json:"category"`
			RequestID     string `json:"request_id"`
			Path          string `json:"path"`
			Method        string `json:"method"`
			StatusCode    int    `json:"status_code"`
			UserID        *uint  `json:"user_id"`
			UserUsername  string `json:"user_username"`
			AdminUsername string `json:"admin_username"`
			Action        string `json:"action"`
			TargetType    string `json:"target_type"`
			Detail        string `json:"detail"`
			ErrorDetail   string `json:"error_detail"`
		} `json:"items"`
		Total   int64 `json:"total"`
		Summary struct {
			Total        int64      `json:"total"`
			SuccessTotal int64      `json:"success_total"`
			FailedTotal  int64      `json:"failed_total"`
			RecentTotal  int64      `json:"recent_total"`
			LastEventAt  *time.Time `json:"last_event_at"`
		} `json:"summary"`
	}{}
	if err := json.Unmarshal(systemResp.Body.Bytes(), &categoryPayload); err != nil {
		t.Fatalf("decode system category payload: %v", err)
	}
	if categoryPayload.Total != 1 || len(categoryPayload.Items) != 1 || categoryPayload.Items[0].Action != "model.update" || categoryPayload.Items[0].AdminUsername != auditor.Username {
		t.Fatalf("expected admin audit log with admin username, got %+v", categoryPayload)
	}

	adminKeywordResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs?category=system_operation&keyword=category-auditor", nil, auditorCookies)
	if adminKeywordResp.Code != http.StatusOK {
		t.Fatalf("expected admin keyword system operation logs 200, got %d: %s", adminKeywordResp.Code, adminKeywordResp.Body.String())
	}
	categoryPayload = struct {
		Items []struct {
			Category      string `json:"category"`
			RequestID     string `json:"request_id"`
			Path          string `json:"path"`
			Method        string `json:"method"`
			StatusCode    int    `json:"status_code"`
			UserID        *uint  `json:"user_id"`
			UserUsername  string `json:"user_username"`
			AdminUsername string `json:"admin_username"`
			Action        string `json:"action"`
			TargetType    string `json:"target_type"`
			Detail        string `json:"detail"`
			ErrorDetail   string `json:"error_detail"`
		} `json:"items"`
		Total   int64 `json:"total"`
		Summary struct {
			Total        int64      `json:"total"`
			SuccessTotal int64      `json:"success_total"`
			FailedTotal  int64      `json:"failed_total"`
			RecentTotal  int64      `json:"recent_total"`
			LastEventAt  *time.Time `json:"last_event_at"`
		} `json:"summary"`
	}{}
	if err := json.Unmarshal(adminKeywordResp.Body.Bytes(), &categoryPayload); err != nil {
		t.Fatalf("decode admin keyword payload: %v", err)
	}
	if categoryPayload.Total != 1 || len(categoryPayload.Items) != 1 || categoryPayload.Items[0].AdminUsername != auditor.Username {
		t.Fatalf("expected system operation keyword to search admin username, got %+v", categoryPayload)
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-logs/export?category=system_operation&keyword=model.update", nil, auditorCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected system operation CSV export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if disposition := exportResp.Header().Get("Content-Disposition"); !strings.Contains(disposition, "system-operation-logs.csv") {
		t.Fatalf("expected system operation csv disposition, got %q", disposition)
	}
	if !strings.Contains(exportResp.Body.String(), "model.update") || !strings.Contains(exportResp.Body.String(), "gpt-image-2") || !strings.Contains(exportResp.Body.String(), auditor.Username) {
		t.Fatalf("expected system operation csv to include audit fields, got %s", exportResp.Body.String())
	}
}

func TestCleanupOldSystemRequestLogs(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	now := time.Now()
	if err := db.Create(&[]SystemRequestLog{
		{RequestID: "expired", Level: "info", Method: "GET", Path: "/api/old", StatusCode: 200, CreatedAt: now.AddDate(0, 0, -31)},
		{RequestID: "fresh", Level: "info", Method: "GET", Path: "/api/new", StatusCode: 200, CreatedAt: now.AddDate(0, 0, -29)},
	}).Error; err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	if err := testApp.cleanupOldSystemRequestLogs(now); err != nil {
		t.Fatalf("cleanup logs: %v", err)
	}

	var ids []string
	if err := db.Model(&SystemRequestLog{}).Order("request_id asc").Pluck("request_id", &ids).Error; err != nil {
		t.Fatalf("load remaining request IDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "fresh" {
		t.Fatalf("expected only fresh log to remain, got %#v", ids)
	}
}
