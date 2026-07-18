package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestAdminSystemSettingsGetPatchExportAndAudit(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.cfg.AppVersion = "2026.05-test"
	testApp.cfg.SystemStorageCapacityBytes = 1024
	testApp.cfg.SystemCDNTrafficBytes = 256
	testApp.cfg.SystemCDNTrafficLimitBytes = 2048
	testApp.cfg.SystemDailyGenerationLimit = 50

	if err := os.MkdirAll(filepath.Join(testApp.cfg.AssetStoragePath, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir asset fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testApp.cfg.AssetStoragePath, "one.bin"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write asset fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testApp.cfg.AssetStoragePath, "nested", "two.bin"), []byte("1234567"), 0o644); err != nil {
		t.Fatalf("write nested asset fixture: %v", err)
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load test location: %v", err)
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)
	seedAdminGenerationRecord(t, testApp, GenerationRecord{Status: GenerationStatusQueued, CreatedAt: today})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{Status: GenerationStatusRunning, CreatedAt: today.Add(time.Hour)})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{Status: GenerationStatusSucceeded, CreatedAt: today.Add(-24 * time.Hour)})

	adminCookies := createAdminSession(t, testApp)

	getResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, adminCookies)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected system settings 200, got %d: %s", getResp.Code, getResp.Body.String())
	}

	var initial struct {
		Settings struct {
			Platform struct {
				Name           string `json:"name"`
				PlatformDomain string `json:"platform_domain"`
			} `json:"platform"`
			Generation struct {
				UploadLimit        int    `json:"upload_limit"`
				DefaultAspectRatio string `json:"default_aspect_ratio"`
				ConcurrencyLimit   int    `json:"concurrency_limit"`
			} `json:"generation"`
		} `json:"settings"`
		Defaults map[string]any `json:"defaults"`
		Status   struct {
			RuntimeStatus        string    `json:"runtime_status"`
			DatabaseStatus       string    `json:"database_status"`
			Version              string    `json:"version"`
			StartedAt            time.Time `json:"started_at"`
			StorageMode          string    `json:"storage_mode"`
			StorageUsedBytes     int64     `json:"storage_used_bytes"`
			StorageCapacityBytes int64     `json:"storage_capacity_bytes"`
			CDNTrafficBytes      int64     `json:"cdn_traffic_bytes"`
			CDNTrafficLimitBytes int64     `json:"cdn_traffic_limit_bytes"`
			TodayGenerations     int64     `json:"today_generations"`
			DailyGenerationLimit int64     `json:"daily_generation_limit"`
			TotalGenerations     int64     `json:"total_generations"`
			QueueStatus          struct {
				Queued  int64 `json:"queued"`
				Running int64 `json:"running"`
			} `json:"queue_status"`
		} `json:"status"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial system settings: %v", err)
	}
	if initial.Settings.Platform.Name != "白霖共享" {
		t.Fatalf("expected default platform name, got %+v", initial.Settings.Platform)
	}
	if initial.Settings.Platform.PlatformDomain != testApp.cfg.AppBaseURL {
		t.Fatalf("expected default platform domain from config, got %q", initial.Settings.Platform.PlatformDomain)
	}
	if initial.Settings.Generation.UploadLimit != 6 || initial.Settings.Generation.DefaultAspectRatio != "1:1" || initial.Settings.Generation.ConcurrencyLimit != 4 {
		t.Fatalf("unexpected default generation settings: %+v", initial.Settings.Generation)
	}
	if initial.Status.RuntimeStatus != "running" || initial.Status.DatabaseStatus != "connected" || initial.Status.StorageMode != "local" {
		t.Fatalf("unexpected status payload: %+v", initial.Status)
	}
	if initial.Status.Version != "2026.05-test" || initial.Status.StartedAt.IsZero() {
		t.Fatalf("expected configured version and app start time, got %+v", initial.Status)
	}
	if initial.Status.StorageUsedBytes != 12 || initial.Status.StorageCapacityBytes != 1024 {
		t.Fatalf("expected storage usage from asset directory, got used=%d capacity=%d", initial.Status.StorageUsedBytes, initial.Status.StorageCapacityBytes)
	}
	if initial.Status.CDNTrafficBytes != 256 || initial.Status.CDNTrafficLimitBytes != 2048 {
		t.Fatalf("expected configured CDN traffic, got used=%d limit=%d", initial.Status.CDNTrafficBytes, initial.Status.CDNTrafficLimitBytes)
	}
	if initial.Status.TodayGenerations != 2 || initial.Status.DailyGenerationLimit != 50 || initial.Status.TotalGenerations != 3 {
		t.Fatalf("unexpected generation counters: %+v", initial.Status)
	}
	if initial.Status.QueueStatus.Queued != 1 || initial.Status.QueueStatus.Running != 1 {
		t.Fatalf("unexpected queue status: %+v", initial.Status.QueueStatus)
	}
	if len(initial.Defaults) == 0 || initial.UpdatedAt == "" {
		t.Fatalf("expected defaults and updated_at, got defaults=%v updated_at=%q", initial.Defaults, initial.UpdatedAt)
	}

	updatePayload := map[string]any{
		"platform": map[string]any{
			"name":              "生成平台",
			"short_name":        "GP",
			"logo_url":          "https://cdn.example.com/logo.png",
			"timezone":          "Asia/Shanghai",
			"language":          "zh-CN",
			"currency":          "CNY",
			"icp_record_number": "沪ICP备20260501号",
			"platform_domain":   "https://images.example.com",
		},
		"storage": map[string]any{
			"storage_mode":     "object",
			"provider":         "aliyun-oss",
			"region":           "cn-shanghai",
			"bucket":           "image-agent-assets",
			"cdn_domain":       "https://cdn.example.com",
			"cdn_acceleration": true,
		},
		"generation": map[string]any{
			"upload_limit":                8,
			"default_aspect_ratio":        "21:9",
			"retention_days":              45,
			"concurrency_limit":           7,
			"review_policy":               "manual",
			"negative_prompt_enabled":     false,
			"advanced_parameters_enabled": true,
		},
		"notifications": map[string]any{
			"notification_email":   "ops@example.com",
			"task_complete_notice": true,
			"system_alert_notice":  true,
			"daily_summary_notice": true,
			"webhook_url":          "https://hooks.example.com/system",
		},
		"security": map[string]any{
			"login_policy":                        "strict",
			"password_min_length":                 12,
			"two_factor_enabled":                  true,
			"failed_login_lock_enabled":           true,
			"admin_permission_management_enabled": false,
		},
	}
	patchResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/system-settings", updatePayload, adminCookies)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected patch system settings 200, got %d: %s", patchResp.Code, patchResp.Body.String())
	}

	reloadResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, adminCookies)
	if reloadResp.Code != http.StatusOK {
		t.Fatalf("expected reload system settings 200, got %d: %s", reloadResp.Code, reloadResp.Body.String())
	}
	var reloaded struct {
		Settings struct {
			Platform struct {
				Name string `json:"name"`
			} `json:"platform"`
			Storage struct {
				Provider        string `json:"provider"`
				CDNAcceleration bool   `json:"cdn_acceleration"`
			} `json:"storage"`
			Generation struct {
				UploadLimit        int    `json:"upload_limit"`
				DefaultAspectRatio string `json:"default_aspect_ratio"`
				ConcurrencyLimit   int    `json:"concurrency_limit"`
				NegativePrompt     bool   `json:"negative_prompt_enabled"`
			} `json:"generation"`
			Notifications struct {
				NotificationEmail string `json:"notification_email"`
				WebhookURL        string `json:"webhook_url"`
			} `json:"notifications"`
			Security struct {
				PasswordMinLength                int  `json:"password_min_length"`
				TwoFactorEnabled                 bool `json:"two_factor_enabled"`
				AdminPermissionManagementEnabled bool `json:"admin_permission_management_enabled"`
			} `json:"security"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(reloadResp.Body.Bytes(), &reloaded); err != nil {
		t.Fatalf("decode reloaded system settings: %v", err)
	}
	if reloaded.Settings.Platform.Name != "生成平台" ||
		reloaded.Settings.Storage.Provider != "aliyun-oss" ||
		!reloaded.Settings.Storage.CDNAcceleration ||
		reloaded.Settings.Generation.UploadLimit != 8 ||
		reloaded.Settings.Generation.DefaultAspectRatio != "21:9" ||
		reloaded.Settings.Generation.ConcurrencyLimit != 7 ||
		reloaded.Settings.Generation.NegativePrompt ||
		reloaded.Settings.Notifications.NotificationEmail != "ops@example.com" ||
		reloaded.Settings.Notifications.WebhookURL != "https://hooks.example.com/system" ||
		reloaded.Settings.Security.PasswordMinLength != 12 ||
		!reloaded.Settings.Security.TwoFactorEnabled ||
		reloaded.Settings.Security.AdminPermissionManagementEnabled {
		t.Fatalf("settings were not persisted as expected: %+v", reloaded.Settings)
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).Where("action = ? AND target_type = ?", "system_settings.update", "settings").Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one system settings audit log, got %d", auditCount)
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings/export", nil, adminCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected system settings export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if got := exportResp.Header().Get("Content-Disposition"); got == "" {
		t.Fatalf("expected content disposition for exported settings")
	}
	if !json.Valid(exportResp.Body.Bytes()) || !containsString(exportResp.Body.String(), "生成平台") {
		t.Fatalf("expected exported JSON to contain current settings, got %s", exportResp.Body.String())
	}
}

func TestAdminSystemSettingsValidatesInvalidInputs(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "upload limit too low",
			payload: map[string]any{"generation": map[string]any{"upload_limit": 0}},
		},
		{
			name:    "concurrency too low",
			payload: map[string]any{"generation": map[string]any{"concurrency_limit": 0}},
		},
		{
			name:    "retention days too low",
			payload: map[string]any{"generation": map[string]any{"retention_days": 0}},
		},
		{
			name:    "password length too short",
			payload: map[string]any{"security": map[string]any{"password_min_length": 5}},
		},
		{
			name:    "invalid email",
			payload: map[string]any{"notifications": map[string]any{"notification_email": "not-an-email"}},
		},
		{
			name:    "invalid webhook scheme",
			payload: map[string]any{"notifications": map[string]any{"webhook_url": "ftp://hooks.example.com/system"}},
		},
		{
			name:    "invalid platform domain",
			payload: map[string]any{"platform": map[string]any{"platform_domain": "not a url"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/system-settings", tc.payload, adminCookies)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid input 400, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestAdminSystemSettingsReportsAlipayConfigStatusWithoutSecrets(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.AppBaseURL = "https://example.com"
	testApp.cfg.AlipayAppID = "2026000000000001"
	testApp.cfg.AlipayPrivateKey = "private-key-secret"
	testApp.cfg.AlipayPublicKey = ""
	testApp.cfg.AlipayGateway = defaultAlipayGateway(true)
	testApp.cfg.AlipaySandbox = true
	adminCookies := createAdminSession(t, testApp)

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected system settings 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Status struct {
			Payment struct {
				Alipay struct {
					Configured bool     `json:"configured"`
					Sandbox    bool     `json:"sandbox"`
					Gateway    string   `json:"gateway"`
					NotifyURL  string   `json:"notify_url"`
					Missing    []string `json:"missing"`
					Items      []struct {
						Key        string `json:"key"`
						Configured bool   `json:"configured"`
					} `json:"items"`
				} `json:"alipay"`
			} `json:"payment"`
		} `json:"status"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode system settings alipay status: %v", err)
	}
	alipay := payload.Status.Payment.Alipay
	if alipay.Configured || !alipay.Sandbox || alipay.Gateway != "https://openapi-sandbox.dl.alipaydev.com/gateway.do" ||
		alipay.NotifyURL != "https://example.com/api/payments/alipay/notify" {
		t.Fatalf("unexpected alipay status: %+v", alipay)
	}
	if !stringSliceContains(alipay.Missing, "ALIPAY_PUBLIC_KEY") {
		t.Fatalf("expected ALIPAY_PUBLIC_KEY missing, got %+v", alipay.Missing)
	}
	statusByKey := map[string]bool{}
	for _, item := range alipay.Items {
		statusByKey[item.Key] = item.Configured
	}
	for _, key := range []string{"ALIPAY_APP_ID", "ALIPAY_PRIVATE_KEY", "ALIPAY_GATEWAY", "APP_BASE_URL"} {
		if !statusByKey[key] {
			t.Fatalf("expected %s to be marked configured in %+v", key, alipay.Items)
		}
	}
	if statusByKey["ALIPAY_PUBLIC_KEY"] {
		t.Fatalf("expected ALIPAY_PUBLIC_KEY to be marked missing in %+v", alipay.Items)
	}
	body := resp.Body.String()
	if strings.Contains(body, "private-key-secret") || strings.Contains(body, "2026000000000001") {
		t.Fatalf("expected alipay status to omit configured values, got %s", body)
	}
}

func TestAdminSystemSettingsRequireRolePermissions(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	unauthorizedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request 401, got %d", unauthorizedResp.Code)
	}

	limited := createDatabaseAdminUser(t, db, "system-limited", "LimitedPass123")
	limitedCookies := loginAdminAs(t, testApp, limited.Username, "LimitedPass123")
	limitedReadResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, limitedCookies)
	if limitedReadResp.Code != http.StatusForbidden {
		t.Fatalf("expected admin without system setting permission read 403, got %d: %s", limitedReadResp.Code, limitedReadResp.Body.String())
	}

	auditor := createDatabaseAdminUser(t, db, "system-auditor", "LimitedPass123")
	assignAdminRoleByCode(t, db, &auditor, "auditor")
	auditorCookies := loginAdminAs(t, testApp, auditor.Username, "LimitedPass123")
	auditorReadResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, auditorCookies)
	if auditorReadResp.Code != http.StatusOK {
		t.Fatalf("expected auditor read 200, got %d: %s", auditorReadResp.Code, auditorReadResp.Body.String())
	}
	auditorWriteResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/system-settings", map[string]any{
		"platform": map[string]any{"name": "审计可保存"},
	}, auditorCookies)
	if auditorWriteResp.Code != http.StatusForbidden {
		t.Fatalf("expected auditor write without update permission 403, got %d: %s", auditorWriteResp.Code, auditorWriteResp.Body.String())
	}

	operator := createDatabaseAdminUser(t, db, "system-operator", "LimitedPass123")
	assignAdminRoleByCode(t, db, &operator, "operator")
	operatorCookies := loginAdminAs(t, testApp, operator.Username, "LimitedPass123")
	operatorWriteResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/system-settings", map[string]any{
		"platform": map[string]any{"name": "运营可保存"},
	}, operatorCookies)
	if operatorWriteResp.Code != http.StatusOK {
		t.Fatalf("expected operator write 200, got %d: %s", operatorWriteResp.Code, operatorWriteResp.Body.String())
	}

	finance := createDatabaseAdminUser(t, db, "system-finance", "LimitedPass123")
	assignAdminRoleByCode(t, db, &finance, "finance")
	financeCookies := loginAdminAs(t, testApp, finance.Username, "LimitedPass123")
	financeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/system-settings", nil, financeCookies)
	if financeResp.Code != http.StatusForbidden {
		t.Fatalf("expected finance admin without system settings read permission 403, got %d: %s", financeResp.Code, financeResp.Body.String())
	}

	superCookies := createAdminSession(t, testApp)
	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, superCookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected admin me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}
	if !containsString(meResp.Body.String(), "system_settings.read") || !containsString(meResp.Body.String(), "/admin/system-settings") {
		t.Fatalf("expected super admin permissions and menu to include system settings, got %s", meResp.Body.String())
	}
}

func assignAdminRoleByCode(t *testing.T, db *gorm.DB, admin *AdminUser, code string) {
	t.Helper()
	var role Role
	if err := db.Where("code = ?", code).First(&role).Error; err != nil {
		t.Fatalf("load role %s: %v", code, err)
	}
	if err := db.Model(admin).Association("Roles").Append(&role); err != nil {
		t.Fatalf("assign role %s: %v", code, err)
	}
}

func containsString(text, needle string) bool {
	return strings.Contains(text, needle)
}

func stringSliceContains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
