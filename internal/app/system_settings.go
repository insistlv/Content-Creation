package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminSystemSettings struct {
	Platform      adminPlatformSettings     `json:"platform"`
	Storage       adminStorageSettings      `json:"storage"`
	Generation    adminGenerationSettings   `json:"generation"`
	Notifications adminNotificationSettings `json:"notifications"`
	Security      adminSecuritySettings     `json:"security"`
}

type adminPlatformSettings struct {
	Name            string `json:"name"`
	ShortName       string `json:"short_name"`
	LogoURL         string `json:"logo_url"`
	Timezone        string `json:"timezone"`
	Language        string `json:"language"`
	Currency        string `json:"currency"`
	ICPRecordNumber string `json:"icp_record_number"`
	PlatformDomain  string `json:"platform_domain"`
}

type adminStorageSettings struct {
	StorageMode     string `json:"storage_mode"`
	Provider        string `json:"provider"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	CDNDomain       string `json:"cdn_domain"`
	CDNAcceleration bool   `json:"cdn_acceleration"`
}

type adminGenerationSettings struct {
	UploadLimit               int    `json:"upload_limit"`
	DefaultAspectRatio        string `json:"default_aspect_ratio"`
	RetentionDays             int    `json:"retention_days"`
	ConcurrencyLimit          int    `json:"concurrency_limit"`
	ReviewPolicy              string `json:"review_policy"`
	NegativePromptEnabled     bool   `json:"negative_prompt_enabled"`
	AdvancedParametersEnabled bool   `json:"advanced_parameters_enabled"`
}

type adminNotificationSettings struct {
	NotificationEmail  string `json:"notification_email"`
	TaskCompleteNotice bool   `json:"task_complete_notice"`
	SystemAlertNotice  bool   `json:"system_alert_notice"`
	DailySummaryNotice bool   `json:"daily_summary_notice"`
	WebhookURL         string `json:"webhook_url"`
}

type adminSecuritySettings struct {
	LoginPolicy                      string `json:"login_policy"`
	PasswordMinLength                int    `json:"password_min_length"`
	TwoFactorEnabled                 bool   `json:"two_factor_enabled"`
	FailedLoginLockEnabled           bool   `json:"failed_login_lock_enabled"`
	AdminPermissionManagementEnabled bool   `json:"admin_permission_management_enabled"`
}

type adminSystemStatus struct {
	RuntimeStatus        string                 `json:"runtime_status"`
	DatabaseStatus       string                 `json:"database_status"`
	Version              string                 `json:"version"`
	StartedAt            time.Time              `json:"started_at"`
	StorageMode          string                 `json:"storage_mode"`
	StorageProvider      string                 `json:"storage_provider"`
	StorageBucket        string                 `json:"storage_bucket"`
	StorageUsedBytes     int64                  `json:"storage_used_bytes"`
	StorageCapacityBytes int64                  `json:"storage_capacity_bytes"`
	CDNStatus            string                 `json:"cdn_status"`
	CDNTrafficBytes      int64                  `json:"cdn_traffic_bytes"`
	CDNTrafficLimitBytes int64                  `json:"cdn_traffic_limit_bytes"`
	TodayGenerations     int64                  `json:"today_generations"`
	DailyGenerationLimit int64                  `json:"daily_generation_limit"`
	QueueStatus          adminSystemQueueStatus `json:"queue_status"`
	Payment              adminPaymentStatus     `json:"payment"`
	TotalUsers           int64                  `json:"total_users"`
	TotalWorks           int64                  `json:"total_works"`
	TotalGenerations     int64                  `json:"total_generations"`
}

type adminPaymentStatus struct {
	Alipay alipayConfigStatus `json:"alipay"`
}

type adminSystemQueueStatus struct {
	Queued  int64 `json:"queued"`
	Running int64 `json:"running"`
}

type adminSystemSettingsPatch struct {
	Platform      *adminPlatformSettingsPatch     `json:"platform"`
	Storage       *adminStorageSettingsPatch      `json:"storage"`
	Generation    *adminGenerationSettingsPatch   `json:"generation"`
	Notifications *adminNotificationSettingsPatch `json:"notifications"`
	Security      *adminSecuritySettingsPatch     `json:"security"`
}

type adminPlatformSettingsPatch struct {
	Name            *string `json:"name"`
	ShortName       *string `json:"short_name"`
	LogoURL         *string `json:"logo_url"`
	Timezone        *string `json:"timezone"`
	Language        *string `json:"language"`
	Currency        *string `json:"currency"`
	ICPRecordNumber *string `json:"icp_record_number"`
	PlatformDomain  *string `json:"platform_domain"`
}

type adminStorageSettingsPatch struct {
	StorageMode     *string `json:"storage_mode"`
	Provider        *string `json:"provider"`
	Region          *string `json:"region"`
	Bucket          *string `json:"bucket"`
	CDNDomain       *string `json:"cdn_domain"`
	CDNAcceleration *bool   `json:"cdn_acceleration"`
}

type adminGenerationSettingsPatch struct {
	UploadLimit               *int    `json:"upload_limit"`
	DefaultAspectRatio        *string `json:"default_aspect_ratio"`
	RetentionDays             *int    `json:"retention_days"`
	ConcurrencyLimit          *int    `json:"concurrency_limit"`
	ReviewPolicy              *string `json:"review_policy"`
	NegativePromptEnabled     *bool   `json:"negative_prompt_enabled"`
	AdvancedParametersEnabled *bool   `json:"advanced_parameters_enabled"`
}

type adminNotificationSettingsPatch struct {
	NotificationEmail  *string `json:"notification_email"`
	TaskCompleteNotice *bool   `json:"task_complete_notice"`
	SystemAlertNotice  *bool   `json:"system_alert_notice"`
	DailySummaryNotice *bool   `json:"daily_summary_notice"`
	WebhookURL         *string `json:"webhook_url"`
}

type adminSecuritySettingsPatch struct {
	LoginPolicy                      *string `json:"login_policy"`
	PasswordMinLength                *int    `json:"password_min_length"`
	TwoFactorEnabled                 *bool   `json:"two_factor_enabled"`
	FailedLoginLockEnabled           *bool   `json:"failed_login_lock_enabled"`
	AdminPermissionManagementEnabled *bool   `json:"admin_permission_management_enabled"`
}

func defaultAdminSystemSettings(cfg Config) adminSystemSettings {
	return adminSystemSettings{
		Platform: adminPlatformSettings{
			Name:           "白霖共享",
			ShortName:      "IA",
			Timezone:       "Asia/Shanghai",
			Language:       "zh-CN",
			Currency:       "CNY",
			PlatformDomain: strings.TrimSpace(cfg.AppBaseURL),
		},
		Storage: adminStorageSettings{
			StorageMode: "local",
			Provider:    "local",
			Bucket:      strings.TrimSpace(cfg.AssetStoragePath),
		},
		Generation: adminGenerationSettings{
			UploadLimit:               6,
			DefaultAspectRatio:        "1:1",
			RetentionDays:             30,
			ConcurrencyLimit:          4,
			ReviewPolicy:              "standard",
			NegativePromptEnabled:     true,
			AdvancedParametersEnabled: true,
		},
		Notifications: adminNotificationSettings{
			TaskCompleteNotice: true,
			SystemAlertNotice:  true,
		},
		Security: adminSecuritySettings{
			LoginPolicy:                      "standard",
			PasswordMinLength:                8,
			FailedLoginLockEnabled:           true,
			AdminPermissionManagementEnabled: true,
		},
	}
}

func (a *App) initializeSystemSettings(settings *AppSettings) {
	values := defaultAdminSystemSettings(a.cfg)
	applyAdminSystemSettingsToModel(settings, values)
	settings.SystemSettingsInitialized = true
}

func adminSystemSettingsFromModel(settings AppSettings) adminSystemSettings {
	return adminSystemSettings{
		Platform: adminPlatformSettings{
			Name:            settings.PlatformName,
			ShortName:       settings.PlatformShortName,
			LogoURL:         settings.PlatformLogoURL,
			Timezone:        settings.PlatformTimezone,
			Language:        settings.PlatformLanguage,
			Currency:        settings.PlatformCurrency,
			ICPRecordNumber: settings.PlatformICPRecordNumber,
			PlatformDomain:  settings.PlatformDomain,
		},
		Storage: adminStorageSettings{
			StorageMode:     settings.StorageMode,
			Provider:        settings.StorageProvider,
			Region:          settings.StorageRegion,
			Bucket:          settings.StorageBucket,
			CDNDomain:       settings.StorageCDNDomain,
			CDNAcceleration: settings.StorageCDNAcceleration,
		},
		Generation: adminGenerationSettings{
			UploadLimit:               settings.GenerationUploadLimit,
			DefaultAspectRatio:        settings.GenerationDefaultAspectRatio,
			RetentionDays:             settings.GenerationRetentionDays,
			ConcurrencyLimit:          settings.GenerationConcurrencyLimit,
			ReviewPolicy:              settings.GenerationReviewPolicy,
			NegativePromptEnabled:     settings.GenerationNegativePromptEnabled,
			AdvancedParametersEnabled: settings.GenerationAdvancedParametersEnabled,
		},
		Notifications: adminNotificationSettings{
			NotificationEmail:  settings.NotificationEmail,
			TaskCompleteNotice: settings.NotificationTaskCompleteNotice,
			SystemAlertNotice:  settings.NotificationSystemAlertNotice,
			DailySummaryNotice: settings.NotificationDailySummaryNotice,
			WebhookURL:         settings.NotificationWebhookURL,
		},
		Security: adminSecuritySettings{
			LoginPolicy:                      settings.SecurityLoginPolicy,
			PasswordMinLength:                settings.SecurityPasswordMinLength,
			TwoFactorEnabled:                 settings.SecurityTwoFactorEnabled,
			FailedLoginLockEnabled:           settings.SecurityFailedLoginLockEnabled,
			AdminPermissionManagementEnabled: settings.SecurityAdminPermissionManagementEnabled,
		},
	}
}

func applyAdminSystemSettingsToModel(settings *AppSettings, values adminSystemSettings) {
	settings.PlatformName = values.Platform.Name
	settings.PlatformShortName = values.Platform.ShortName
	settings.PlatformLogoURL = values.Platform.LogoURL
	settings.PlatformTimezone = values.Platform.Timezone
	settings.PlatformLanguage = values.Platform.Language
	settings.PlatformCurrency = values.Platform.Currency
	settings.PlatformICPRecordNumber = values.Platform.ICPRecordNumber
	settings.PlatformDomain = values.Platform.PlatformDomain
	settings.StorageMode = values.Storage.StorageMode
	settings.StorageProvider = values.Storage.Provider
	settings.StorageRegion = values.Storage.Region
	settings.StorageBucket = values.Storage.Bucket
	settings.StorageCDNDomain = values.Storage.CDNDomain
	settings.StorageCDNAcceleration = values.Storage.CDNAcceleration
	settings.GenerationUploadLimit = values.Generation.UploadLimit
	settings.GenerationDefaultAspectRatio = values.Generation.DefaultAspectRatio
	settings.GenerationRetentionDays = values.Generation.RetentionDays
	settings.GenerationConcurrencyLimit = values.Generation.ConcurrencyLimit
	settings.GenerationReviewPolicy = values.Generation.ReviewPolicy
	settings.GenerationNegativePromptEnabled = values.Generation.NegativePromptEnabled
	settings.GenerationAdvancedParametersEnabled = values.Generation.AdvancedParametersEnabled
	settings.NotificationEmail = values.Notifications.NotificationEmail
	settings.NotificationTaskCompleteNotice = values.Notifications.TaskCompleteNotice
	settings.NotificationSystemAlertNotice = values.Notifications.SystemAlertNotice
	settings.NotificationDailySummaryNotice = values.Notifications.DailySummaryNotice
	settings.NotificationWebhookURL = values.Notifications.WebhookURL
	settings.SecurityLoginPolicy = values.Security.LoginPolicy
	settings.SecurityPasswordMinLength = values.Security.PasswordMinLength
	settings.SecurityTwoFactorEnabled = values.Security.TwoFactorEnabled
	settings.SecurityFailedLoginLockEnabled = values.Security.FailedLoginLockEnabled
	settings.SecurityAdminPermissionManagementEnabled = values.Security.AdminPermissionManagementEnabled
}

func applyAdminSystemSettingsPatch(values *adminSystemSettings, patch adminSystemSettingsPatch) {
	if patch.Platform != nil {
		if patch.Platform.Name != nil {
			values.Platform.Name = strings.TrimSpace(*patch.Platform.Name)
		}
		if patch.Platform.ShortName != nil {
			values.Platform.ShortName = strings.TrimSpace(*patch.Platform.ShortName)
		}
		if patch.Platform.LogoURL != nil {
			values.Platform.LogoURL = strings.TrimSpace(*patch.Platform.LogoURL)
		}
		if patch.Platform.Timezone != nil {
			values.Platform.Timezone = strings.TrimSpace(*patch.Platform.Timezone)
		}
		if patch.Platform.Language != nil {
			values.Platform.Language = strings.TrimSpace(*patch.Platform.Language)
		}
		if patch.Platform.Currency != nil {
			values.Platform.Currency = strings.TrimSpace(*patch.Platform.Currency)
		}
		if patch.Platform.ICPRecordNumber != nil {
			values.Platform.ICPRecordNumber = strings.TrimSpace(*patch.Platform.ICPRecordNumber)
		}
		if patch.Platform.PlatformDomain != nil {
			values.Platform.PlatformDomain = strings.TrimSpace(*patch.Platform.PlatformDomain)
		}
	}
	if patch.Storage != nil {
		if patch.Storage.StorageMode != nil {
			values.Storage.StorageMode = strings.TrimSpace(*patch.Storage.StorageMode)
		}
		if patch.Storage.Provider != nil {
			values.Storage.Provider = strings.TrimSpace(*patch.Storage.Provider)
		}
		if patch.Storage.Region != nil {
			values.Storage.Region = strings.TrimSpace(*patch.Storage.Region)
		}
		if patch.Storage.Bucket != nil {
			values.Storage.Bucket = strings.TrimSpace(*patch.Storage.Bucket)
		}
		if patch.Storage.CDNDomain != nil {
			values.Storage.CDNDomain = strings.TrimSpace(*patch.Storage.CDNDomain)
		}
		if patch.Storage.CDNAcceleration != nil {
			values.Storage.CDNAcceleration = *patch.Storage.CDNAcceleration
		}
	}
	if patch.Generation != nil {
		if patch.Generation.UploadLimit != nil {
			values.Generation.UploadLimit = *patch.Generation.UploadLimit
		}
		if patch.Generation.DefaultAspectRatio != nil {
			values.Generation.DefaultAspectRatio = strings.TrimSpace(*patch.Generation.DefaultAspectRatio)
		}
		if patch.Generation.RetentionDays != nil {
			values.Generation.RetentionDays = *patch.Generation.RetentionDays
		}
		if patch.Generation.ConcurrencyLimit != nil {
			values.Generation.ConcurrencyLimit = *patch.Generation.ConcurrencyLimit
		}
		if patch.Generation.ReviewPolicy != nil {
			values.Generation.ReviewPolicy = strings.TrimSpace(*patch.Generation.ReviewPolicy)
		}
		if patch.Generation.NegativePromptEnabled != nil {
			values.Generation.NegativePromptEnabled = *patch.Generation.NegativePromptEnabled
		}
		if patch.Generation.AdvancedParametersEnabled != nil {
			values.Generation.AdvancedParametersEnabled = *patch.Generation.AdvancedParametersEnabled
		}
	}
	if patch.Notifications != nil {
		if patch.Notifications.NotificationEmail != nil {
			values.Notifications.NotificationEmail = strings.TrimSpace(*patch.Notifications.NotificationEmail)
		}
		if patch.Notifications.TaskCompleteNotice != nil {
			values.Notifications.TaskCompleteNotice = *patch.Notifications.TaskCompleteNotice
		}
		if patch.Notifications.SystemAlertNotice != nil {
			values.Notifications.SystemAlertNotice = *patch.Notifications.SystemAlertNotice
		}
		if patch.Notifications.DailySummaryNotice != nil {
			values.Notifications.DailySummaryNotice = *patch.Notifications.DailySummaryNotice
		}
		if patch.Notifications.WebhookURL != nil {
			values.Notifications.WebhookURL = strings.TrimSpace(*patch.Notifications.WebhookURL)
		}
	}
	if patch.Security != nil {
		if patch.Security.LoginPolicy != nil {
			values.Security.LoginPolicy = strings.TrimSpace(*patch.Security.LoginPolicy)
		}
		if patch.Security.PasswordMinLength != nil {
			values.Security.PasswordMinLength = *patch.Security.PasswordMinLength
		}
		if patch.Security.TwoFactorEnabled != nil {
			values.Security.TwoFactorEnabled = *patch.Security.TwoFactorEnabled
		}
		if patch.Security.FailedLoginLockEnabled != nil {
			values.Security.FailedLoginLockEnabled = *patch.Security.FailedLoginLockEnabled
		}
		if patch.Security.AdminPermissionManagementEnabled != nil {
			values.Security.AdminPermissionManagementEnabled = *patch.Security.AdminPermissionManagementEnabled
		}
	}
}

func validateAdminSystemSettings(values adminSystemSettings) error {
	if strings.TrimSpace(values.Platform.Name) == "" {
		return errors.New("platform name is required")
	}
	if strings.TrimSpace(values.Platform.ShortName) == "" {
		return errors.New("platform short name is required")
	}
	if _, err := time.LoadLocation(values.Platform.Timezone); err != nil {
		return errors.New("timezone is invalid")
	}
	if strings.TrimSpace(values.Platform.Language) == "" || strings.TrimSpace(values.Platform.Currency) == "" {
		return errors.New("language and currency are required")
	}
	if !validOptionalHTTPURL(values.Platform.LogoURL) || !validOptionalHTTPURL(values.Platform.PlatformDomain) || !validOptionalHTTPURL(values.Storage.CDNDomain) || !validOptionalHTTPURL(values.Notifications.WebhookURL) {
		return errors.New("url is invalid")
	}
	if !oneOf(values.Storage.StorageMode, "local", "object") {
		return errors.New("storage mode is invalid")
	}
	if values.Storage.Provider == "" {
		return errors.New("storage provider is required")
	}
	if values.Generation.UploadLimit < 1 || values.Generation.UploadLimit > 20 {
		return errors.New("upload limit is invalid")
	}
	if !oneOf(values.Generation.DefaultAspectRatio, "21:9", "16:9", "4:3", "3:2", "1:1", "2:3", "3:4", "9:16", "9:21") {
		return errors.New("default aspect ratio is invalid")
	}
	if values.Generation.RetentionDays < 1 || values.Generation.RetentionDays > 3650 {
		return errors.New("retention days is invalid")
	}
	if values.Generation.ConcurrencyLimit < 1 || values.Generation.ConcurrencyLimit > 100 {
		return errors.New("concurrency limit is invalid")
	}
	if !oneOf(values.Generation.ReviewPolicy, "standard", "manual", "auto", "off") {
		return errors.New("review policy is invalid")
	}
	if values.Notifications.NotificationEmail != "" {
		if _, err := mail.ParseAddress(values.Notifications.NotificationEmail); err != nil {
			return errors.New("notification email is invalid")
		}
	}
	if !oneOf(values.Security.LoginPolicy, "standard", "strict", "relaxed") {
		return errors.New("login policy is invalid")
	}
	if values.Security.PasswordMinLength < 8 || values.Security.PasswordMinLength > 64 {
		return errors.New("password minimum length is invalid")
	}
	return nil
}

func validOptionalHTTPURL(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func (a *App) handleGetSystemSettings(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	status, err := a.adminSystemStatus(settings)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "system_status_failed", "系统状态读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"settings":   adminSystemSettingsFromModel(settings),
		"defaults":   defaultAdminSystemSettings(a.cfg),
		"status":     status,
		"updated_at": settings.UpdatedAt,
	})
}

func (a *App) handlePatchSystemSettings(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	var req adminSystemSettingsPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	values := adminSystemSettingsFromModel(settings)
	applyAdminSystemSettingsPatch(&values, req)
	if err := validateAdminSystemSettings(values); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_system_settings", "系统设置无效")
		return
	}
	applyAdminSystemSettingsToModel(&settings, values)
	settings.SystemSettingsInitialized = true
	if err := a.db.Save(&settings).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "settings_save_failed", "配置保存失败")
		return
	}
	a.writeAdminAudit(c, "system_settings.update", "settings", settings.ID, gin.H{
		"platform_name": settings.PlatformName,
		"storage_mode":  settings.StorageMode,
	})
	writeJSON(c, http.StatusOK, gin.H{
		"ok":       true,
		"settings": adminSystemSettingsFromModel(settings),
	})
}

func (a *App) handleExportSystemSettings(c *gin.Context) {
	settings, err := a.loadSettings()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_load_failed", "配置读取失败")
		return
	}
	payload := gin.H{
		"settings":    adminSystemSettingsFromModel(settings),
		"exported_at": time.Now(),
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		writeError(c, http.StatusInternalServerError, "settings_export_failed", "配置导出失败")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="system-settings.json"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", content)
}

func (a *App) adminSystemStatus(settings AppSettings) (adminSystemStatus, error) {
	var status adminSystemStatus
	status.RuntimeStatus = "running"
	status.DatabaseStatus = "connected"
	status.Version = fallbackString(a.cfg.AppVersion, "local")
	status.StartedAt = a.startedAt
	if status.StartedAt.IsZero() {
		status.StartedAt = time.Now()
	}
	status.StorageMode = settings.StorageMode
	status.StorageProvider = settings.StorageProvider
	status.StorageBucket = settings.StorageBucket
	status.StorageCapacityBytes = a.cfg.SystemStorageCapacityBytes
	status.CDNTrafficBytes = a.cfg.SystemCDNTrafficBytes
	status.CDNTrafficLimitBytes = a.cfg.SystemCDNTrafficLimitBytes
	status.DailyGenerationLimit = a.cfg.SystemDailyGenerationLimit
	status.Payment = adminPaymentStatus{Alipay: alipayRuntimeConfigStatus(a.cfg)}
	if settings.StorageCDNAcceleration {
		status.CDNStatus = "enabled"
	} else {
		status.CDNStatus = "disabled"
	}
	storageUsed, err := storageUsedBytes(a.cfg.AssetStoragePath)
	if err != nil {
		return status, err
	}
	status.StorageUsedBytes = storageUsed
	if err := a.db.Model(&User{}).Count(&status.TotalUsers).Error; err != nil {
		return status, err
	}
	if err := a.db.Model(&Work{}).Count(&status.TotalWorks).Error; err != nil {
		return status, err
	}
	if err := a.db.Model(&GenerationRecord{}).Count(&status.TotalGenerations).Error; err != nil {
		return status, err
	}
	start, end := systemStatusDayRange(settings.PlatformTimezone)
	if err := a.db.Model(&GenerationRecord{}).Where("created_at >= ? AND created_at < ?", start, end).Count(&status.TodayGenerations).Error; err != nil {
		return status, err
	}
	if err := a.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusQueued).Count(&status.QueueStatus.Queued).Error; err != nil {
		return status, err
	}
	if err := a.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusRunning).Count(&status.QueueStatus.Running).Error; err != nil {
		return status, err
	}
	return status, nil
}

func storageUsedBytes(root string) (int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, nil
	}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func systemStatusDayRange(timezone string) (time.Time, time.Time) {
	loc, err := time.LoadLocation(fallbackString(timezone, "Local"))
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 0, 1)
}
