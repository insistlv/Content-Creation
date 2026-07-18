package app

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	InviteStatusActive   = "active"
	InviteStatusDisabled = "disabled"

	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"

	defaultRequestTimeoutSeconds = 600
	legacyRequestTimeoutSeconds  = 90
	defaultRateLimitMaxRequests  = 20
	legacyRateLimitMaxRequests   = 5

	GenerationStatusQueued     = "queued"
	GenerationStatusRunning    = "running"
	GenerationStatusProcessing = GenerationStatusRunning
	GenerationStatusSucceeded  = "succeeded"
	GenerationStatusFailed     = "failed"

	CoupleAlbumStatusDraft         = "draft"
	CoupleAlbumStatusGenerating    = "generating"
	CoupleAlbumStatusSucceeded     = GenerationStatusSucceeded
	CoupleAlbumStatusPartialFailed = "partial_failed"
	CoupleAlbumStatusFailed        = GenerationStatusFailed

	NovelVideoProjectStatusDraft           = "draft"
	NovelVideoProjectStatusAnalyzed        = "analyzed"
	NovelVideoProjectStatusPlanned         = "planned"
	NovelVideoProjectStatusRendering       = "rendering"
	NovelVideoProjectStatusSucceeded       = GenerationStatusSucceeded
	NovelVideoProjectStatusFailed          = GenerationStatusFailed
	NovelVideoProjectStatusPartial         = "partial_failed"
	NovelVideoReviewStatusDraft            = "draft"
	NovelVideoReviewStatusApproved         = "approved"
	NovelVideoReviewStatusNeedsReview      = "needs_review"
	NovelVideoContentModeNarration         = "narration"
	NovelVideoContentModeDrama             = "drama"
	NovelVideoContentModeAd                = "ad"
	NovelVideoContentModeShortFilmImage    = "short_film_image"
	NovelVideoGenerationModeStoryboard     = "storyboard"
	NovelVideoGenerationModeGrid           = "grid"
	NovelVideoGenerationModeReferenceVideo = "reference_video"
	NovelVideoGenerationModeImageSeries    = "image_series"
	NovelVideoAssetKindCharacter           = "character"
	NovelVideoAssetKindScene               = "scene"
	NovelVideoAssetKindProp                = "prop"
	NovelVideoAssetKindClue                = "clue"
	NovelVideoAssetKindStyle               = "style"
	NovelVideoAssetKindActorRef            = "actor_ref"
	NovelVideoAssetKindActorKeySheet       = "actor_key_sheet"
	NovelVideoAssetKindShotImage           = "shot_image"
	NovelVideoJobTypeAnalysis              = "analysis"
	NovelVideoJobTypeAssetImage            = "asset_image"
	NovelVideoJobTypeStoryboard            = "storyboard"
	NovelVideoJobTypeShotVideo             = "shot_video"
	NovelVideoJobTypeCompose               = "compose"
	NovelVideoJobTypeExport                = "export"

	GenerationStageQueued             = "queued"
	GenerationStageRequestingProvider = "requesting_provider"
	GenerationStagePersistingResult   = "persisting_result"
	GenerationStageSucceeded          = "succeeded"
	GenerationStageFailed             = "failed"

	VideoGenerationSourceWorkspace = "workspace"
	VideoGenerationSourceNovelShot = "novel_shot"

	GenerationQualityLow    = "low"
	GenerationQualityMedium = "medium"
	GenerationQualityHigh   = "high"
	GenerationQualityUltra  = "ultra"

	GenerationToolModeGenerate         = "generate"
	GenerationToolModeRedraw           = "redraw"
	GenerationToolModeErase            = "erase"
	GenerationToolModeExpand           = "expand"
	GenerationToolModeUpscale          = "upscale"
	GenerationToolModeRemoveBackground = "remove_background"
	GenerationToolModePrecisionEdit    = "precision_edit"
	GenerationToolModeVirtualTryOn     = "virtual_try_on"

	GenerationReferenceIntentCompose   = "compose"
	GenerationReferenceIntentCharacter = "character"
	GenerationReferenceIntentCreative  = "creative"

	WorkVisibilityPrivate   = "private"
	WorkVisibilityPublic    = "public"
	WorkCategoryImage       = "image"
	WorkCategoryVideo       = "video"
	WorkCategoryAudio       = "audio"
	WorkCategoryPosterKV    = "poster_kv"
	WorkCategoryProductMain = "product_main"
	WorkCategoryCover       = "cover"

	CreditTransactionTypeManualTopUp       = "manual_topup"
	CreditTransactionTypeManualDeduct      = "manual_deduct"
	CreditTransactionTypeGenerationCharge  = "generation_charge"
	CreditTransactionTypeGenerationReserve = "generation_reserve"
	CreditTransactionTypeGenerationRelease = "generation_release"
	CreditTransactionTypeGenerationSettle  = "generation_settle"
	CreditTransactionTypePaymentTopUp      = "payment_topup"
	CreditTransactionTypePromptTemplateUse = "prompt_template_use"
	CreditTransactionTypeSignupBonus       = "signup_bonus"
	CreditTransactionTypeCommerceReserve   = "commerce_reserve"
	CreditTransactionTypeCommerceRelease   = "commerce_release"

	PurchaseIntentStatusSubmitted  = "submitted"
	PurchaseIntentStatusProcessing = "processing"
	PurchaseIntentStatusContacted  = "contacted"
	PurchaseIntentStatusCompleted  = "completed"
	PurchaseIntentStatusInvalid    = "invalid"

	FinanceOrderTypePackage = "package"

	FinancePaymentMethodOffline            = "offline_transfer"
	FinancePaymentMethodAlipayPage         = "alipay_page"
	FinancePaymentMethodWechatJSAPI        = "wechat_jsapi"
	FinancePaymentMethodWechatVirtualGoods = "wechat_virtual_goods"

	FinancePaymentStatusPending  = "pending"
	FinancePaymentStatusPaid     = "paid"
	FinancePaymentStatusRefunded = "refunded"
	FinancePaymentStatusFailed   = "failed"
	FinancePaymentStatusExpired  = "expired"

	PaymentProviderAlipay                   = "alipay"
	PaymentProviderMethodAlipayPage         = "page_pay"
	PaymentProviderWechat                   = "wechat"
	PaymentProviderMethodWechatJSAPI        = "jsapi"
	PaymentProviderMethodWechatVirtualGoods = "virtual_goods"
	PaymentRecordStatusCreated              = "created"
	PaymentRecordStatusRequested            = "requested"
	PaymentRecordStatusPaid                 = "paid"
	PaymentRecordStatusFailed               = "failed"
	PaymentRecordStatusClosed               = "closed"

	FinanceInvoiceStatusPending  = "pending"
	FinanceInvoiceStatusIssued   = "issued"
	FinanceInvoiceStatusRejected = "rejected"
	FinanceInvoiceStatusVoided   = "voided"

	FinanceRefundStatusPending    = "pending"
	FinanceRefundStatusProcessing = "processing"
	FinanceRefundStatusApproved   = "approved"
	FinanceRefundStatusRejected   = "rejected"
	FinanceRefundStatusCompleted  = "completed"

	AdminUserStatusActive   = "active"
	AdminUserStatusDisabled = "disabled"

	RoleStatusActive   = "active"
	RoleStatusDisabled = "disabled"

	SystemRequestLogLevelInfo  = "info"
	SystemRequestLogLevelWarn  = "warn"
	SystemRequestLogLevelError = "error"

	AnnouncementLevelInfo      = "info"
	AnnouncementLevelImportant = "important"
	AnnouncementLevelWarning   = "warning"

	AnnouncementStatusDraft     = "draft"
	AnnouncementStatusPublished = "published"
	AnnouncementStatusOffline   = "offline"

	AnnouncementClientAll      = "all"
	AnnouncementClientWeb      = "web"
	AnnouncementClientMPWeixin = "mp-weixin"

	ModelConfigTypeImage = "image"
	ModelConfigTypeVideo = "video"
	ModelConfigTypeAudio = "audio"
	ModelConfigTypeChat  = "chat"

	ModelConfigStatusOnline  = "online"
	ModelConfigStatusOffline = "offline"

	ModelConfigPermissionPublic   = "public"
	ModelConfigPermissionInternal = "internal"

	ModelRoutingStrategyDefault    = "default"
	ModelRoutingStrategyRoundRobin = "round_robin"
	ModelRoutingStrategySpeedFirst = "speed_first"
	ModelRoutingStrategyWeighted   = "weighted"

	ModelCallAttemptStatusSucceeded = "succeeded"
	ModelCallAttemptStatusFailed    = "failed"

	CoupleAlbumOptionTypeLocation      = "location"
	CoupleAlbumOptionTypeStoryTemplate = "story_template"
	CoupleAlbumOptionTypeStyle         = "style"

	ModelCenterStatusOnline       = "online"
	ModelCenterStatusOffline      = "offline"
	ModelCenterVisibilityPublic   = "public"
	ModelCenterVisibilityInternal = "internal"
	ModelChannelHealthHealthy     = "healthy"
	ModelChannelHealthDegraded    = "degraded"
	ModelChannelHealthDown        = "down"
	ModelRoutingSourceLegacy      = "legacy"
	ModelRoutingSourceModelCenter = "model_center"

	ContentSafetyStatusPending      = "pending"
	ContentSafetyStatusPass         = "pass"
	ContentSafetyStatusReject       = "reject"
	ContentSafetyStatusManualReview = "manual_review"

	ContentReviewTypePrompt     = "prompt"
	ContentReviewTypeReference  = "reference_image"
	ContentReviewTypeGeneration = "generation_result"
	ContentReviewTypeShare      = "public_share"

	ContentReportStatusPending  = "pending"
	ContentReportStatusResolved = "resolved"
	ContentReportStatusRejected = "rejected"

	AlgorithmDisclosureStatusDraft     = "draft"
	AlgorithmDisclosureStatusPublished = "published"

	AlgorithmIncidentStatusOpen       = "open"
	AlgorithmIncidentStatusMitigating = "mitigating"
	AlgorithmIncidentStatusResolved   = "resolved"

	userSessionCookie  = "user_session"
	adminSessionCookie = "admin_session"
)

type StartupDatabaseMigrationsMode string

const (
	StartupDatabaseMigrationsSkip      StartupDatabaseMigrationsMode = "skip"
	StartupDatabaseMigrationsExisting  StartupDatabaseMigrationsMode = "existing"
	StartupDatabaseMigrationsBootstrap StartupDatabaseMigrationsMode = "bootstrap"
)

type Config struct {
	AppBaseURL                        string
	OpenAIAPIKey                      string
	OpenAIBaseURL                     string
	DeepSeekAPIKey                    string
	DeepSeekBaseURL                   string
	DeepSeekPromptModel               string
	DeepSeekPromptTimeoutSeconds      int
	DeepSeekComposePlanTimeoutSeconds int
	JWTSecret                         string
	AdminUsername                     string
	AdminPassword                     string
	DatabaseURL                       string
	SecretsMasterKey                  []byte
	SecretsKeyVersion                 int
	AssetStoragePath                  string
	AppVersion                        string
	SystemStorageCapacityBytes        int64
	SystemCDNTrafficBytes             int64
	SystemCDNTrafficLimitBytes        int64
	SystemDailyGenerationLimit        int64
	DefaultInviteQuota                int
	DefaultImageModel                 string
	AllowedImageModels                []string
	RequestTimeoutSeconds             int
	RateLimitWindowSeconds            int
	RateLimitMaxRequests              int
	UserSessionHours                  int
	AdminSessionHours                 int
	UserRememberSessionHours          int
	AdminRememberSessionHours         int
	FrontendDistPath                  string
	StartupDatabaseMigrations         StartupDatabaseMigrationsMode
	StartupDatabaseBootstrap          bool

	StorageType                          string
	OSSEndpoint                          string
	OSSAccessKeyID                       string
	OSSAccessKeySecret                   string
	OSSBucket                            string
	OSSPublicBaseURL                     string
	OSSBasePath                          string
	ReferenceAssetUploadMaxBytes         int64
	ReferenceAssetUploadPolicyTTLSeconds int
	AICommerceEnabled                    bool
	AICommerceWorkerEnabled              bool
	AICommercePrivateStorageType         string
	AICommercePrivateAssetPath           string
	AICommerceOSSEndpoint                string
	AICommerceOSSAccessKeyID             string
	AICommerceOSSAccessKeySecret         string
	AICommerceOSSBucket                  string
	AICommerceOSSBasePath                string
	AICommerceSignedURLTTLSeconds        int
	AICommerceTempRetentionHours         int
	GenerationQueueCapacity              int
	GenerationUserPendingLimit           int
	GenerationQueueTimeoutSeconds        int
	GenerationSpoolPath                  string
	GenerationSpoolMaxBytes              int64

	SMSProvider                   string
	AliyunSMSAccessKeyID          string
	AliyunSMSAccessKeySecret      string
	AliyunSMSSignName             string
	AliyunSMSRegisterTemplateCode string
	AliyunSMSResetTemplateCode    string
	AliyunSMSEndpoint             string

	AlipayAppID      string
	AlipayPrivateKey string
	AlipayPublicKey  string
	AlipayGateway    string
	AlipaySandbox    bool

	WechatPayAppID                string
	WechatPayMchID                string
	WechatPayMchCertSerialNo      string
	WechatPayMchPrivateKey        string
	WechatPayMchPrivateKeyPath    string
	WechatPayAPIv3Key             string
	WechatPayNotifyURL            string
	WechatPayPlatformPublicKey    string
	WechatAppSecret               string
	WechatVirtualPayOfferID       string
	WechatVirtualPayAppKey        string
	WechatVirtualPaySandboxAppKey string
	WechatVirtualPayEnv           int
	ArkAPIKey                     string
	ZZAPIKey                      string
}

// SecretRecord stores one encrypted application secret. Ciphertext never
// contains the plaintext value and the identity tuple is authenticated as AAD.
type SecretRecord struct {
	ID         uint      `json:"-" gorm:"primaryKey"`
	Namespace  string    `json:"namespace" gorm:"size:96;uniqueIndex:idx_secret_identity,priority:1"`
	OwnerID    string    `json:"owner_id" gorm:"size:96;uniqueIndex:idx_secret_identity,priority:2"`
	Name       string    `json:"name" gorm:"size:128;uniqueIndex:idx_secret_identity,priority:3"`
	Ciphertext []byte    `json:"-" gorm:"type:bytea"`
	Nonce      []byte    `json:"-" gorm:"type:bytea"`
	Algorithm  string    `json:"algorithm" gorm:"size:32"`
	KeyVersion int       `json:"key_version"`
	CreatedBy  string    `json:"created_by" gorm:"size:128"`
	UpdatedBy  string    `json:"updated_by" gorm:"size:128"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type AdminUser struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	Username     string     `json:"username" gorm:"uniqueIndex;size:64"`
	DisplayName  string     `json:"display_name" gorm:"size:128"`
	PasswordHash string     `json:"-" gorm:"type:text"`
	Status       string     `json:"status" gorm:"size:32;index"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	Roles        []Role     `json:"roles" gorm:"many2many:admin_user_roles;"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type AdminSession struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	AdminUserID uint      `json:"admin_user_id" gorm:"index"`
	TokenID     string    `json:"token_id" gorm:"uniqueIndex;size:64"`
	ExpiresAt   time.Time `json:"expires_at" gorm:"index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Role struct {
	ID          uint         `json:"id" gorm:"primaryKey"`
	Code        string       `json:"code" gorm:"uniqueIndex;size:96"`
	Name        string       `json:"name" gorm:"size:128"`
	Description string       `json:"description" gorm:"type:text"`
	Status      string       `json:"status" gorm:"size:32;index"`
	Permissions []Permission `json:"permissions" gorm:"many2many:role_permissions;"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Permission struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;size:128"`
	Name        string    `json:"name" gorm:"size:128"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AdminAuditLog struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	AdminUserID uint      `json:"admin_user_id" gorm:"index"`
	Action      string    `json:"action" gorm:"size:128;index"`
	TargetType  string    `json:"target_type" gorm:"size:64"`
	TargetID    uint      `json:"target_id" gorm:"index"`
	Detail      string    `json:"detail" gorm:"type:text"`
	IPAddress   string    `json:"ip_address" gorm:"size:64"`
	CreatedAt   time.Time `json:"created_at"`
}

type SystemRequestLog struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	RequestID     string         `json:"request_id" gorm:"uniqueIndex;size:64"`
	Level         string         `json:"level" gorm:"size:16;index"`
	Method        string         `json:"method" gorm:"size:16;index"`
	Path          string         `json:"path" gorm:"type:text;index"`
	StatusCode    int            `json:"status_code" gorm:"index"`
	DurationMs    int64          `json:"duration_ms"`
	IPAddress     string         `json:"ip_address" gorm:"size:64"`
	UserAgent     string         `json:"user_agent" gorm:"type:text"`
	UserID        *uint          `json:"user_id" gorm:"index"`
	UserUsername  string         `json:"user_username" gorm:"size:128"`
	AdminUserID   *uint          `json:"admin_user_id" gorm:"index"`
	AdminUsername string         `json:"admin_username" gorm:"size:128"`
	ErrorCode     string         `json:"error_code" gorm:"size:128;index"`
	ErrorMessage  string         `json:"error_message" gorm:"type:text"`
	ErrorDetail   string         `json:"error_detail" gorm:"type:text"`
	CreatedAt     time.Time      `json:"created_at" gorm:"index"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

type SystemAnnouncement struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	Title             string     `json:"title" gorm:"size:160"`
	Content           string     `json:"content" gorm:"type:text"`
	Level             string     `json:"level" gorm:"size:32;index"`
	Status            string     `json:"status" gorm:"size:32;index"`
	TargetClientsJSON string     `json:"-" gorm:"column:target_clients;type:text"`
	TargetClients     []string   `json:"target_clients" gorm:"-"`
	PopupEnabled      bool       `json:"popup_enabled"`
	StartsAt          *time.Time `json:"starts_at"`
	EndsAt            *time.Time `json:"ends_at"`
	Priority          int        `json:"priority" gorm:"index"`
	ActionText        string     `json:"action_text" gorm:"size:80"`
	ActionURL         string     `json:"action_url" gorm:"type:text"`
	PublishedAt       *time.Time `json:"published_at"`
	CreatedByID       uint       `json:"created_by_id" gorm:"index"`
	CreatedByName     string     `json:"created_by_name" gorm:"size:128"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (a *SystemAnnouncement) AfterFind(tx *gorm.DB) error {
	a.TargetClients = decodeAnnouncementTargetClients(a.TargetClientsJSON)
	return nil
}

func (a *SystemAnnouncement) BeforeSave(tx *gorm.DB) error {
	a.NormalizeTargetClients()
	return nil
}

func (a *SystemAnnouncement) NormalizeTargetClients() {
	if len(a.TargetClients) == 0 && strings.TrimSpace(a.TargetClientsJSON) != "" {
		a.TargetClients = decodeAnnouncementTargetClients(a.TargetClientsJSON)
	}
	a.TargetClients = normalizeAnnouncementTargetClients(a.TargetClients)
	payload, _ := json.Marshal(a.TargetClients)
	a.TargetClientsJSON = string(payload)
}

func decodeAnnouncementTargetClients(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{AnnouncementClientAll}
	}
	var clients []string
	if err := json.Unmarshal([]byte(value), &clients); err != nil {
		return []string{AnnouncementClientAll}
	}
	return normalizeAnnouncementTargetClients(clients)
}

func normalizeAnnouncementTargetClients(clients []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(clients))
	for _, client := range clients {
		client = strings.TrimSpace(strings.ToLower(client))
		if client == "" {
			continue
		}
		if client == AnnouncementClientAll {
			return []string{AnnouncementClientAll}
		}
		if !isKnownAnnouncementClient(client) || seen[client] {
			continue
		}
		seen[client] = true
		normalized = append(normalized, client)
	}
	if len(normalized) == 0 {
		return []string{AnnouncementClientAll}
	}
	return normalized
}

func isKnownAnnouncementClient(client string) bool {
	return client == AnnouncementClientAll || client == AnnouncementClientWeb || client == AnnouncementClientMPWeixin
}

type AnnouncementReceipt struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	UserID         uint       `json:"user_id" gorm:"uniqueIndex:idx_announcement_receipt_user_client;index"`
	AnnouncementID uint       `json:"announcement_id" gorm:"uniqueIndex:idx_announcement_receipt_user_client;index"`
	Client         string     `json:"client" gorm:"uniqueIndex:idx_announcement_receipt_user_client;size:32;index"`
	DismissedAt    *time.Time `json:"dismissed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type User struct {
	ID                       uint           `json:"user_id" gorm:"primaryKey"`
	Username                 string         `json:"username" gorm:"uniqueIndex;size:64"`
	Phone                    *string        `json:"phone" gorm:"uniqueIndex;size:20"`
	WechatOpenID             string         `json:"-" gorm:"column:wechat_open_id;index;size:128"`
	DisplayName              string         `json:"display_name" gorm:"size:128"`
	Email                    string         `json:"email" gorm:"size:255"`
	AvatarURL                string         `json:"avatar_url" gorm:"type:text"`
	PasswordHash             string         `json:"-" gorm:"type:text"`
	PaymentPasswordHash      string         `json:"-" gorm:"type:text"`
	Status                   string         `json:"status" gorm:"size:32"`
	LastLoginAt              *time.Time     `json:"last_login_at"`
	UserRoleID               *uint          `json:"user_role_id" gorm:"index"`
	UserRole                 UserRole       `json:"role" gorm:"foreignKey:UserRoleID"`
	LoginNotificationEnabled bool           `json:"login_notification_enabled" gorm:"default:true"`
	RiskNotificationEnabled  bool           `json:"risk_notification_enabled" gorm:"default:true"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	DeletedAt                gorm.DeletedAt `json:"-" gorm:"index"`
}

type AuthVerificationCode struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	Phone        string     `json:"phone" gorm:"index;size:20"`
	Purpose      string     `json:"purpose" gorm:"index;size:32"`
	CodeHash     string     `json:"-" gorm:"type:text"`
	ExpiresAt    time.Time  `json:"expires_at" gorm:"index"`
	ConsumedAt   *time.Time `json:"consumed_at" gorm:"index"`
	AttemptCount int        `json:"attempt_count"`
	IPAddress    string     `json:"ip_address" gorm:"index;size:64"`
	CreatedAt    time.Time  `json:"created_at" gorm:"index"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type AuthCaptchaChallenge struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	CaptchaID    string     `json:"captcha_id" gorm:"uniqueIndex;size:64"`
	Purpose      string     `json:"purpose" gorm:"index;size:32"`
	CodeHash     string     `json:"-" gorm:"type:text"`
	ExpiresAt    time.Time  `json:"expires_at" gorm:"index"`
	ConsumedAt   *time.Time `json:"consumed_at" gorm:"index"`
	AttemptCount int        `json:"attempt_count"`
	IPAddress    string     `json:"ip_address" gorm:"index;size:64"`
	CreatedAt    time.Time  `json:"created_at" gorm:"index"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type UserRole struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;size:96"`
	Name        string    `json:"name" gorm:"size:128"`
	Description string    `json:"description" gorm:"type:text"`
	Color       string    `json:"color" gorm:"size:32"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserSession struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	UserID     uint       `json:"user_id" gorm:"index"`
	TokenID    string     `json:"token_id" gorm:"uniqueIndex;size:64"`
	IPAddress  string     `json:"ip_address" gorm:"size:64"`
	UserAgent  string     `json:"user_agent" gorm:"type:text"`
	ExpiresAt  time.Time  `json:"expires_at" gorm:"index"`
	LastSeenAt *time.Time `json:"last_seen_at" gorm:"index"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreditBalance struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           uint      `json:"user_id" gorm:"uniqueIndex"`
	AvailableCredits int       `json:"available_credits"`
	ReservedCredits  int       `json:"reserved_credits"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CreditTransaction struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	UserID         uint      `json:"user_id" gorm:"index"`
	Type           string    `json:"type" gorm:"size:64"`
	Amount         int       `json:"amount"`
	BalanceAfter   int       `json:"balance_after"`
	ReservedAfter  int       `json:"reserved_after"`
	IdempotencyKey string    `json:"idempotency_key" gorm:"size:160"`
	Reason         string    `json:"reason" gorm:"size:255"`
	RelatedType    string    `json:"related_type" gorm:"size:64"`
	RelatedID      uint      `json:"related_id" gorm:"index"`
	AdminNote      string    `json:"admin_note" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Package struct {
	ID                     uint             `json:"id" gorm:"primaryKey"`
	Name                   string           `json:"name" gorm:"size:128"`
	Description            string           `json:"description" gorm:"type:text"`
	PriceLabel             string           `json:"price_label" gorm:"size:64"`
	PriceCents             int64            `json:"price_cents"`
	Credits                int              `json:"credits"`
	ValidDays              int              `json:"valid_days"`
	Audience               string           `json:"audience" gorm:"size:128"`
	TagsJSON               string           `json:"-" gorm:"column:tags_json;type:text"`
	Tags                   []string         `json:"tags" gorm:"-"`
	Icon                   string           `json:"icon" gorm:"size:64"`
	Theme                  string           `json:"theme" gorm:"size:64"`
	Badge                  string           `json:"badge" gorm:"size:64"`
	Recommended            bool             `json:"recommended"`
	FeaturesJSON           string           `json:"-" gorm:"column:features_json;type:text"`
	Features               []string         `json:"features" gorm:"-"`
	BenefitsJSON           string           `json:"-" gorm:"column:benefits_json;type:text"`
	Benefits               []PackageBenefit `json:"benefits" gorm:"-"`
	WechatVirtualProductID string           `json:"wechat_virtual_product_id" gorm:"size:128;index"`
	SortOrder              int              `json:"sort_order"`
	IsActive               bool             `json:"is_active"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
	DeletedAt              gorm.DeletedAt   `json:"-" gorm:"index"`
}

type PackageBenefit struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func (p *Package) AfterFind(tx *gorm.DB) error {
	p.Tags = decodePackageTags(p.TagsJSON)
	p.Features = decodePackageFeatures(p.FeaturesJSON)
	p.Benefits = decodePackageBenefits(p.BenefitsJSON)
	return nil
}

func (p *Package) BeforeSave(tx *gorm.DB) error {
	if p.Tags != nil {
		payload, err := json.Marshal(normalizePackageTags(p.Tags))
		if err != nil {
			return err
		}
		p.TagsJSON = string(payload)
	}
	if p.Features != nil {
		payload, err := json.Marshal(normalizePackageFeatures(p.Features))
		if err != nil {
			return err
		}
		p.FeaturesJSON = string(payload)
	}
	if p.Benefits != nil {
		payload, err := json.Marshal(normalizePackageBenefits(p.Benefits))
		if err != nil {
			return err
		}
		p.BenefitsJSON = string(payload)
	}
	return nil
}

func (p *Package) NormalizeTags() {
	p.Tags = normalizePackageTags(p.Tags)
	payload, _ := json.Marshal(p.Tags)
	p.TagsJSON = string(payload)
}

func (p *Package) NormalizePresentation() {
	p.Features = normalizePackageFeatures(p.Features)
	p.Benefits = normalizePackageBenefits(p.Benefits)
	featuresPayload, _ := json.Marshal(p.Features)
	benefitsPayload, _ := json.Marshal(p.Benefits)
	p.FeaturesJSON = string(featuresPayload)
	p.BenefitsJSON = string(benefitsPayload)
}

func decodePackageTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err != nil {
		return []string{}
	}
	return normalizePackageTags(tags)
}

func normalizePackageTags(tags []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		text := strings.TrimSpace(tag)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		normalized = append(normalized, text)
	}
	return normalized
}

func decodePackageFeatures(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	var features []string
	if err := json.Unmarshal([]byte(value), &features); err != nil {
		return []string{}
	}
	return normalizePackageFeatures(features)
}

func normalizePackageFeatures(features []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(features))
	for _, feature := range features {
		text := strings.TrimSpace(feature)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		normalized = append(normalized, text)
	}
	return normalized
}

func decodePackageBenefits(value string) []PackageBenefit {
	if strings.TrimSpace(value) == "" {
		return []PackageBenefit{}
	}
	var benefits []PackageBenefit
	if err := json.Unmarshal([]byte(value), &benefits); err != nil {
		return []PackageBenefit{}
	}
	return normalizePackageBenefits(benefits)
}

func normalizePackageBenefits(benefits []PackageBenefit) []PackageBenefit {
	seen := map[string]bool{}
	normalized := make([]PackageBenefit, 0, len(benefits))
	for _, benefit := range benefits {
		label := strings.TrimSpace(benefit.Label)
		value := strings.TrimSpace(benefit.Value)
		if label == "" || value == "" || seen[label] {
			continue
		}
		seen[label] = true
		normalized = append(normalized, PackageBenefit{Label: label, Value: value})
	}
	return normalized
}

type PromptTemplate struct {
	ID                       uint           `json:"id" gorm:"primaryKey"`
	Slug                     string         `json:"slug" gorm:"size:128;uniqueIndex"`
	Title                    string         `json:"title" gorm:"size:128"`
	Category                 string         `json:"category" gorm:"size:64;index"`
	Description              string         `json:"description" gorm:"type:text"`
	Prompt                   string         `json:"prompt" gorm:"type:text"`
	AspectRatio              string         `json:"aspect_ratio" gorm:"size:16"`
	StylePreset              string         `json:"style_preset" gorm:"size:64"`
	Theme                    string         `json:"theme" gorm:"size:64"`
	PreviewAssetKey          string         `json:"preview_asset_key" gorm:"size:255"`
	PreviewURL               string         `json:"preview_url" gorm:"type:text"`
	PreviewMIMEType          string         `json:"preview_mime_type" gorm:"size:64"`
	PreviewProviderRequestID string         `json:"preview_provider_request_id" gorm:"size:128"`
	PreviewGeneratedAt       *time.Time     `json:"preview_generated_at"`
	PreviewStatus            string         `json:"preview_status" gorm:"size:32;index"`
	PreviewErrorMessage      string         `json:"preview_error_message" gorm:"type:text"`
	PreviewLastStartedAt     *time.Time     `json:"preview_last_started_at"`
	PreviewLastFinishedAt    *time.Time     `json:"preview_last_finished_at"`
	WorkspaceSection         string         `json:"workspace_section" gorm:"size:32;index"`
	WorkspaceToolMode        string         `json:"workspace_tool_mode" gorm:"size:32"`
	WorkspaceModelID         uint           `json:"workspace_model_id" gorm:"index"`
	WorkspaceSort            int            `json:"workspace_sort" gorm:"index"`
	SortOrder                int            `json:"sort_order" gorm:"index"`
	IsActive                 bool           `json:"is_active" gorm:"default:true;index"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	DeletedAt                gorm.DeletedAt `json:"-" gorm:"index"`
}

type InspirationRecommendation struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	Slug             string         `json:"slug" gorm:"size:128;uniqueIndex"`
	Title            string         `json:"title" gorm:"size:128"`
	Category         string         `json:"category" gorm:"size:64;index"`
	Description      string         `json:"description" gorm:"type:text"`
	HeatTagsJSON     string         `json:"-" gorm:"type:text"`
	HeatTagsValue    []string       `json:"heat_tags,omitempty" gorm:"-"`
	PreviewAssetKey  string         `json:"preview_asset_key" gorm:"size:255"`
	PreviewURL       string         `json:"preview_url" gorm:"type:text"`
	Prompt           string         `json:"prompt" gorm:"type:text"`
	NegativePrompt   string         `json:"negative_prompt" gorm:"type:text"`
	AspectRatio      string         `json:"aspect_ratio" gorm:"size:16"`
	StylePreset      string         `json:"style_preset" gorm:"size:64"`
	Theme            string         `json:"theme" gorm:"size:64"`
	ToolMode         string         `json:"tool_mode" gorm:"size:32"`
	WorkspaceModelID uint           `json:"model_id" gorm:"index"`
	ParamsJSON       string         `json:"-" gorm:"type:text"`
	ParamsValue      map[string]any `json:"params,omitempty" gorm:"-"`
	SortOrder        int            `json:"sort_order" gorm:"index"`
	IsActive         bool           `json:"is_active" gorm:"index"`
	ViewCount        int            `json:"view_count"`
	UseCount         int            `json:"use_count"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

type VideoStylePreset struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	Slug            string         `json:"slug" gorm:"size:128;uniqueIndex"`
	Title           string         `json:"title" gorm:"size:128"`
	Category        string         `json:"category" gorm:"size:64;index"`
	Description     string         `json:"description" gorm:"type:text"`
	TagsJSON        string         `json:"-" gorm:"type:text"`
	TagsValue       []string       `json:"tags,omitempty" gorm:"-"`
	PreviewAssetKey string         `json:"preview_asset_key" gorm:"size:255"`
	PreviewURL      string         `json:"preview_url" gorm:"type:text"`
	StylePrompt     string         `json:"style_prompt" gorm:"type:text"`
	SortOrder       int            `json:"sort_order" gorm:"index"`
	IsActive        bool           `json:"is_active" gorm:"index"`
	UseCount        int            `json:"use_count"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

type UserVideoStyleTemplate struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	UserID           uint           `json:"user_id" gorm:"index"`
	Title            string         `json:"title" gorm:"size:128"`
	Description      string         `json:"description" gorm:"type:text"`
	ReferenceAssetID uint           `json:"reference_asset_id" gorm:"index"`
	PreviewURL       string         `json:"preview_url" gorm:"type:text"`
	StylePrompt      string         `json:"style_prompt" gorm:"type:text"`
	IsActive         bool           `json:"is_active" gorm:"index"`
	UseCount         int            `json:"use_count"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

func (recommendation *InspirationRecommendation) SetHeatTags(tags []string) error {
	normalized := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		normalized = append(normalized, tag)
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	recommendation.HeatTagsJSON = string(data)
	recommendation.HeatTagsValue = normalized
	return nil
}

func (preset *VideoStylePreset) SetTags(tags []string) error {
	normalized := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		normalized = append(normalized, tag)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	preset.TagsJSON = string(raw)
	preset.TagsValue = normalized
	return nil
}

func (preset VideoStylePreset) Tags() []string {
	if len(preset.TagsValue) > 0 {
		return append([]string(nil), preset.TagsValue...)
	}
	var tags []string
	if strings.TrimSpace(preset.TagsJSON) == "" {
		return []string{}
	}
	if err := json.Unmarshal([]byte(preset.TagsJSON), &tags); err != nil {
		return []string{}
	}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			normalized = append(normalized, tag)
		}
	}
	return normalized
}

func (recommendation InspirationRecommendation) HeatTags() []string {
	if len(recommendation.HeatTagsValue) > 0 {
		return normalizeRecommendationStringList(recommendation.HeatTagsValue)
	}
	if strings.TrimSpace(recommendation.HeatTagsJSON) == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(recommendation.HeatTagsJSON), &tags); err != nil {
		return []string{}
	}
	return normalizeRecommendationStringList(tags)
}

func (recommendation *InspirationRecommendation) SetParams(params map[string]any) error {
	normalized := normalizeGenerationToolOptions(params)
	if len(normalized) == 0 {
		recommendation.ParamsJSON = "{}"
		recommendation.ParamsValue = map[string]any{}
		return nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	recommendation.ParamsJSON = string(data)
	recommendation.ParamsValue = normalized
	return nil
}

func (recommendation InspirationRecommendation) Params() map[string]any {
	if len(recommendation.ParamsValue) > 0 {
		return recommendation.ParamsValue
	}
	if strings.TrimSpace(recommendation.ParamsJSON) == "" {
		return map[string]any{}
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(recommendation.ParamsJSON), &params); err != nil {
		return map[string]any{}
	}
	if params == nil {
		return map[string]any{}
	}
	return params
}

func normalizeRecommendationStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

type CoupleAlbumOption struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Type        string         `json:"type" gorm:"size:32;uniqueIndex:idx_couple_album_option_type_value;index"`
	Value       string         `json:"value" gorm:"size:96;uniqueIndex:idx_couple_album_option_type_value"`
	Label       string         `json:"label" gorm:"size:128"`
	Description string         `json:"description" gorm:"type:text"`
	ImageURL    string         `json:"image_url" gorm:"type:text"`
	IconURL     string         `json:"icon_url" gorm:"type:text"`
	PromptLabel string         `json:"prompt_label" gorm:"size:128"`
	SortOrder   int            `json:"sort_order" gorm:"index"`
	IsActive    bool           `json:"is_active" gorm:"default:true;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

type PurchaseIntent struct {
	ID              uint                 `json:"intent_id" gorm:"primaryKey"`
	UserID          uint                 `json:"user_id" gorm:"index"`
	User            User                 `json:"user,omitempty" gorm:"foreignKey:UserID"`
	PackageID       uint                 `json:"package_id" gorm:"index"`
	PackageName     string               `json:"package_name" gorm:"size:128"`
	PackageCredits  int                  `json:"package_credits"`
	PackagePrice    string               `json:"package_price" gorm:"size:64"`
	CustomerName    string               `json:"customer_name" gorm:"size:128"`
	CustomerEmail   string               `json:"customer_email" gorm:"size:255"`
	CustomerPhone   string               `json:"customer_phone" gorm:"size:64"`
	ContactType     string               `json:"contact_type" gorm:"size:64"`
	ContactValue    string               `json:"contact_value" gorm:"size:255"`
	Source          string               `json:"source" gorm:"size:128;index"`
	OwnerName       string               `json:"owner_name" gorm:"size:128;index"`
	BudgetRange     string               `json:"budget_range" gorm:"size:128"`
	UseCase         string               `json:"use_case" gorm:"type:text"`
	Region          string               `json:"region" gorm:"size:128"`
	Status          string               `json:"status" gorm:"size:32;index"`
	ClosedReason    string               `json:"closed_reason" gorm:"type:text"`
	LastContactedAt *time.Time           `json:"last_contacted_at"`
	ConvertedAt     *time.Time           `json:"converted_at"`
	Note            string               `json:"note" gorm:"type:text"`
	Notes           []PurchaseIntentNote `json:"notes,omitempty" gorm:"foreignKey:PurchaseIntentID"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

type PurchaseIntentNote struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	PurchaseIntentID uint      `json:"purchase_intent_id" gorm:"index"`
	AuthorAdminID    uint      `json:"author_admin_id" gorm:"index"`
	AuthorName       string    `json:"author_name" gorm:"size:128"`
	Event            string    `json:"event" gorm:"size:64"`
	Body             string    `json:"body" gorm:"type:text"`
	CreatedAt        time.Time `json:"created_at"`
}

type FinanceOrder struct {
	ID                     uint            `json:"id" gorm:"primaryKey"`
	OrderNumber            string          `json:"order_number" gorm:"uniqueIndex;size:64"`
	UserID                 uint            `json:"user_id" gorm:"index"`
	User                   User            `json:"user,omitempty" gorm:"foreignKey:UserID"`
	PurchaseIntentID       *uint           `json:"purchase_intent_id" gorm:"uniqueIndex"`
	PurchaseIntent         PurchaseIntent  `json:"purchase_intent,omitempty" gorm:"foreignKey:PurchaseIntentID"`
	PackageID              uint            `json:"package_id" gorm:"index"`
	PackageName            string          `json:"package_name" gorm:"size:128"`
	PackageCredits         int             `json:"package_credits"`
	AmountCents            int64           `json:"amount_cents"`
	OrderType              string          `json:"order_type" gorm:"size:64;index"`
	PaymentMethod          string          `json:"payment_method" gorm:"size:64;index"`
	PaymentStatus          string          `json:"payment_status" gorm:"size:32;index"`
	InvoiceStatus          string          `json:"invoice_status" gorm:"size:32;index"`
	PaidAt                 *time.Time      `json:"paid_at" gorm:"index"`
	AlipayTradeNo          string          `json:"alipay_trade_no" gorm:"size:128;index"`
	AlipayBuyerID          string          `json:"alipay_buyer_id" gorm:"size:128"`
	WechatTransactionID    string          `json:"wechat_transaction_id" gorm:"size:128;index"`
	WechatOpenID           string          `json:"wechat_open_id" gorm:"size:128;index"`
	WechatVirtualProductID string          `json:"wechat_virtual_product_id" gorm:"size:128;index"`
	IPAddress              string          `json:"-" gorm:"size:64;index"`
	PaymentRequestAt       *time.Time      `json:"payment_request_at"`
	AlipayNotifyAt         *time.Time      `json:"alipay_notify_at"`
	WechatNotifyAt         *time.Time      `json:"wechat_notify_at"`
	TransactionURL         string          `json:"transaction_url" gorm:"type:text"`
	EvidenceSnapshotJSON   string          `json:"-" gorm:"column:evidence_snapshot_json;type:text"`
	EvidenceSnapshot       map[string]any  `json:"evidence_snapshot,omitempty" gorm:"-"`
	RawNotificationSummary string          `json:"raw_notification_summary" gorm:"type:text"`
	PaymentRecord          *PaymentRecord  `json:"payment_record,omitempty" gorm:"foreignKey:FinanceOrderID"`
	Refunds                []FinanceRefund `json:"refunds,omitempty" gorm:"foreignKey:FinanceOrderID"`
	Invoice                FinanceInvoice  `json:"invoice,omitempty" gorm:"foreignKey:FinanceOrderID"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

func (o *FinanceOrder) AfterFind(tx *gorm.DB) error {
	if strings.TrimSpace(o.EvidenceSnapshotJSON) == "" {
		return nil
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(o.EvidenceSnapshotJSON), &snapshot); err != nil {
		o.EvidenceSnapshot = map[string]any{}
		return nil
	}
	o.EvidenceSnapshot = snapshot
	return nil
}

type PaymentRecord struct {
	ID               uint         `json:"id" gorm:"primaryKey"`
	PaymentNumber    string       `json:"payment_number" gorm:"uniqueIndex;size:64"`
	FinanceOrderID   uint         `json:"finance_order_id" gorm:"uniqueIndex"`
	FinanceOrder     FinanceOrder `json:"-" gorm:"foreignKey:FinanceOrderID"`
	OrderNumber      string       `json:"order_number" gorm:"size:64;index"`
	UserID           uint         `json:"user_id" gorm:"index"`
	Provider         string       `json:"provider" gorm:"size:64;index"`
	ProviderMethod   string       `json:"provider_method" gorm:"size:64"`
	OutTradeNo       string       `json:"out_trade_no" gorm:"size:128;index"`
	ProviderTradeNo  string       `json:"provider_trade_no" gorm:"size:128;index"`
	AmountCents      int64        `json:"amount_cents"`
	Status           string       `json:"status" gorm:"size:32;index"`
	RequestCount     int          `json:"request_count"`
	NotifyCount      int          `json:"notify_count"`
	QueryCount       int          `json:"query_count"`
	RequestedAt      *time.Time   `json:"requested_at"`
	NotifiedAt       *time.Time   `json:"notified_at"`
	QueriedAt        *time.Time   `json:"queried_at"`
	PaidAt           *time.Time   `json:"paid_at"`
	BuyerID          string       `json:"buyer_id" gorm:"size:128"`
	LastEvent        string       `json:"last_event" gorm:"size:64"`
	LastErrorCode    string       `json:"last_error_code" gorm:"size:128"`
	LastErrorMessage string       `json:"last_error_message" gorm:"type:text"`
	RequestSummary   string       `json:"request_summary" gorm:"type:text"`
	NotifySummary    string       `json:"notify_summary" gorm:"type:text"`
	QuerySummary     string       `json:"query_summary" gorm:"type:text"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

type FinanceRefund struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	RefundNumber   string     `json:"refund_number" gorm:"uniqueIndex;size:64"`
	FinanceOrderID uint       `json:"finance_order_id" gorm:"index"`
	AmountCents    int64      `json:"amount_cents"`
	Reason         string     `json:"reason" gorm:"type:text"`
	Status         string     `json:"status" gorm:"size:32;index"`
	RequestedAt    time.Time  `json:"requested_at" gorm:"index"`
	ProcessedAt    *time.Time `json:"processed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type FinanceInvoice struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	InvoiceNumber  string     `json:"invoice_number" gorm:"uniqueIndex;size:64"`
	FinanceOrderID uint       `json:"finance_order_id" gorm:"uniqueIndex"`
	AmountCents    int64      `json:"amount_cents"`
	Title          string     `json:"title" gorm:"size:160"`
	Status         string     `json:"status" gorm:"size:32;index"`
	IssuedAt       *time.Time `json:"issued_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Work struct {
	ID                 uint           `json:"work_id" gorm:"primaryKey"`
	UserID             uint           `json:"user_id" gorm:"index"`
	GenerationRecordID uint           `json:"generation_record_id" gorm:"index"`
	BatchID            string         `json:"batch_id" gorm:"size:64;index"`
	BatchIndex         int            `json:"batch_index"`
	BatchTotal         int            `json:"batch_total"`
	VariationMode      string         `json:"variation_mode" gorm:"size:32"`
	VariationPrompt    string         `json:"variation_prompt" gorm:"type:text"`
	Prompt             string         `json:"prompt" gorm:"type:text"`
	AspectRatio        string         `json:"aspect_ratio" gorm:"size:16"`
	Category           string         `json:"category" gorm:"size:32;default:image"`
	Model              string         `json:"model" gorm:"size:128"`
	Status             string         `json:"status" gorm:"size:32"`
	Visibility         string         `json:"visibility" gorm:"size:16"`
	IsFavorite         bool           `json:"is_favorite" gorm:"default:false"`
	AssetKey           string         `json:"asset_key" gorm:"size:255"`
	PreviewURL         string         `json:"preview_url" gorm:"type:text"`
	DownloadURL        string         `json:"download_url" gorm:"type:text"`
	MIMEType           string         `json:"mime_type" gorm:"size:64"`
	StorageScope       string         `json:"storage_scope" gorm:"size:32"`
	ProviderRequestID  string         `json:"provider_request_id" gorm:"size:128"`
	ReferenceAssetIDs  []uint         `json:"reference_asset_ids,omitempty" gorm:"-"`
	ErrorCode          string         `json:"error_code" gorm:"size:64"`
	ErrorMessage       string         `json:"error_message" gorm:"type:text"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

type VideoSoundtrack struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	UserID            uint      `json:"user_id" gorm:"index"`
	VideoWorkID       uint      `json:"video_work_id" gorm:"index"`
	AudioWorkID       uint      `json:"audio_work_id" gorm:"index"`
	Source            string    `json:"source" gorm:"size:32;index"`
	ProviderRequestID string    `json:"provider_request_id" gorm:"size:128"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CoupleAlbum struct {
	ID                     uint              `json:"id" gorm:"primaryKey"`
	UserID                 uint              `json:"user_id" gorm:"index"`
	Title                  string            `json:"title" gorm:"size:160"`
	Location               string            `json:"location" gorm:"size:96;index"`
	StoryTemplate          string            `json:"story_template" gorm:"size:64"`
	Style                  string            `json:"style" gorm:"size:64"`
	Status                 string            `json:"status" gorm:"size:32;index"`
	CoverPageID            *uint             `json:"cover_page_id" gorm:"index"`
	ShareToken             string            `json:"share_token" gorm:"uniqueIndex;size:64"`
	ShareEnabled           bool              `json:"share_enabled" gorm:"index;default:false"`
	MaleReferenceAssetID   uint              `json:"male_reference_asset_id" gorm:"index"`
	FemaleReferenceAssetID uint              `json:"female_reference_asset_id" gorm:"index"`
	Pages                  []CoupleAlbumPage `json:"pages,omitempty" gorm:"foreignKey:AlbumID"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
	DeletedAt              gorm.DeletedAt    `json:"-" gorm:"index"`
}

type CoupleAlbumPage struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	AlbumID            uint      `json:"album_id" gorm:"uniqueIndex:idx_couple_album_page"`
	PageNumber         int       `json:"page_number" gorm:"uniqueIndex:idx_couple_album_page"`
	PageTitle          string    `json:"page_title" gorm:"size:160"`
	Caption            string    `json:"caption" gorm:"type:text"`
	Prompt             string    `json:"prompt" gorm:"type:text"`
	Status             string    `json:"status" gorm:"size:32;index"`
	GenerationRecordID *uint     `json:"generation_record_id" gorm:"index"`
	WorkID             *uint     `json:"work_id" gorm:"index"`
	ErrorCode          string    `json:"error_code" gorm:"size:128"`
	ErrorMessage       string    `json:"error_message" gorm:"type:text"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NovelVideoProject struct {
	ID                 uint                 `json:"id" gorm:"primaryKey"`
	UserID             uint                 `json:"user_id" gorm:"index"`
	Title              string               `json:"title" gorm:"size:160"`
	SourceText         string               `json:"source_text" gorm:"type:text"`
	ContentMode        string               `json:"content_mode" gorm:"size:32;index;default:narration"`
	SchemaVersion      int                  `json:"schema_version" gorm:"default:1"`
	GenerationMode     string               `json:"generation_mode" gorm:"size:32;index;default:storyboard"`
	GridSize           int                  `json:"grid_size" gorm:"default:4"`
	StylePreset        string               `json:"style_preset" gorm:"size:160"`
	AspectRatio        string               `json:"aspect_ratio" gorm:"size:16"`
	Duration           string               `json:"duration" gorm:"size:16"`
	ImageModel         string               `json:"image_model" gorm:"size:128"`
	VideoModel         string               `json:"video_model" gorm:"size:128"`
	VideoSettingsJSON  string               `json:"-" gorm:"column:video_settings;type:text"`
	Status             string               `json:"status" gorm:"size:32;index"`
	StoryBibleJSON     string               `json:"-" gorm:"column:story_bible;type:text"`
	ContentRiskSummary string               `json:"content_risk_summary" gorm:"type:text"`
	PlanningDraftJSON  string               `json:"-" gorm:"column:planning_draft;type:text"`
	ErrorCode          string               `json:"error_code" gorm:"size:128"`
	ErrorMessage       string               `json:"error_message" gorm:"type:text"`
	Creatures          []NovelVideoCreature `json:"creatures,omitempty" gorm:"foreignKey:ProjectID"`
	Episodes           []NovelVideoEpisode  `json:"episodes,omitempty" gorm:"foreignKey:ProjectID"`
	Assets             []NovelVideoAsset    `json:"assets,omitempty" gorm:"foreignKey:ProjectID"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	DeletedAt          gorm.DeletedAt       `json:"-" gorm:"index"`
}

type NovelVideoAsset struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	ProjectID          uint           `json:"project_id" gorm:"index"`
	UserID             uint           `json:"user_id" gorm:"index"`
	Kind               string         `json:"kind" gorm:"size:48;index"`
	Name               string         `json:"name" gorm:"size:160"`
	Description        string         `json:"description" gorm:"type:text"`
	Prompt             string         `json:"prompt" gorm:"type:text"`
	ReferenceURL       string         `json:"reference_url" gorm:"type:text"`
	AssetURL           string         `json:"asset_url" gorm:"type:text"`
	Version            int            `json:"version" gorm:"default:1"`
	ReviewStatus       string         `json:"review_status" gorm:"size:32;index"`
	GenerationRecordID *uint          `json:"generation_record_id" gorm:"index"`
	WorkID             *uint          `json:"work_id" gorm:"index"`
	MetadataJSON       string         `json:"-" gorm:"column:metadata;type:text"`
	ErrorCode          string         `json:"error_code" gorm:"size:128"`
	ErrorMessage       string         `json:"error_message" gorm:"type:text"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

type NovelVideoCreature struct {
	ID                      uint           `json:"id" gorm:"primaryKey"`
	ProjectID               uint           `json:"project_id" gorm:"index"`
	UserID                  uint           `json:"user_id" gorm:"index"`
	Name                    string         `json:"name" gorm:"size:160"`
	CreatureType            string         `json:"creature_type" gorm:"size:96"`
	Appearance              string         `json:"appearance" gorm:"type:text"`
	Abilities               string         `json:"abilities" gorm:"type:text"`
	VisualConsistencyPrompt string         `json:"visual_consistency_prompt" gorm:"type:text"`
	ReviewStatus            string         `json:"review_status" gorm:"size:32;index"`
	GenerationRecordID      *uint          `json:"generation_record_id" gorm:"index"`
	WorkID                  *uint          `json:"work_id" gorm:"index"`
	AssetURL                string         `json:"asset_url" gorm:"type:text"`
	ErrorCode               string         `json:"error_code" gorm:"size:128"`
	ErrorMessage            string         `json:"error_message" gorm:"type:text"`
	WorkPreviewURL          string         `json:"work_preview_url" gorm:"-"`
	GenerationStatus        string         `json:"generation_status" gorm:"-"`
	LatestError             string         `json:"latest_error" gorm:"-"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	DeletedAt               gorm.DeletedAt `json:"-" gorm:"index"`
}

type NovelVideoEpisode struct {
	ID        uint             `json:"id" gorm:"primaryKey"`
	ProjectID uint             `json:"project_id" gorm:"uniqueIndex:idx_novel_video_episode"`
	UserID    uint             `json:"user_id" gorm:"index"`
	Number    int              `json:"number" gorm:"uniqueIndex:idx_novel_video_episode"`
	Title     string           `json:"title" gorm:"size:160"`
	Summary   string           `json:"summary" gorm:"type:text"`
	Status    string           `json:"status" gorm:"size:32;index"`
	Shots     []NovelVideoShot `json:"shots,omitempty" gorm:"foreignKey:EpisodeID"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type NovelVideoShot struct {
	ID                     uint                          `json:"id" gorm:"primaryKey"`
	ProjectID              uint                          `json:"project_id" gorm:"index"`
	EpisodeID              uint                          `json:"episode_id" gorm:"uniqueIndex:idx_novel_video_shot"`
	UserID                 uint                          `json:"user_id" gorm:"index"`
	Number                 int                           `json:"number" gorm:"uniqueIndex:idx_novel_video_shot"`
	Title                  string                        `json:"title" gorm:"size:160"`
	Prompt                 string                        `json:"prompt" gorm:"type:text"`
	ScriptUnitType         string                        `json:"script_unit_type" gorm:"size:48"`
	SourceExcerpt          string                        `json:"source_excerpt" gorm:"type:text"`
	DurationSeconds        int                           `json:"duration_seconds"`
	ImagePrompt            string                        `json:"image_prompt" gorm:"type:text"`
	VideoPrompt            string                        `json:"video_prompt" gorm:"type:text"`
	VoiceoverText          string                        `json:"voiceover_text" gorm:"type:text"`
	AssetRefsJSON          string                        `json:"-" gorm:"column:asset_refs;type:text"`
	ReferenceAssetID       *uint                         `json:"reference_asset_id" gorm:"index"`
	GenerationSettingsJSON string                        `json:"-" gorm:"column:generation_settings;type:text"`
	CreatureIDsJSON        string                        `json:"-" gorm:"column:creature_ids;type:text"`
	StoryboardURL          string                        `json:"storyboard_url" gorm:"type:text"`
	StoryboardStatus       string                        `json:"storyboard_status" gorm:"size:32;index"`
	SubtitleText           string                        `json:"subtitle_text" gorm:"type:text"`
	CameraPlanJSON         string                        `json:"-" gorm:"column:camera_plan;type:text"`
	Status                 string                        `json:"status" gorm:"size:32;index"`
	GenerationRecordID     *uint                         `json:"generation_record_id" gorm:"index"`
	WorkID                 *uint                         `json:"work_id" gorm:"index"`
	ErrorCode              string                        `json:"error_code" gorm:"size:128"`
	ErrorMessage           string                        `json:"error_message" gorm:"type:text"`
	WorkPreviewURL         string                        `json:"work_preview_url" gorm:"-"`
	WorkDownloadURL        string                        `json:"work_download_url" gorm:"-"`
	GenerationProgress     int                           `json:"generation_progress" gorm:"-"`
	EstimatedCredits       int                           `json:"estimated_credits" gorm:"-"`
	LatestError            string                        `json:"latest_error" gorm:"-"`
	RenderAttempts         []NovelVideoShotRenderAttempt `json:"generation_attempts,omitempty" gorm:"foreignKey:ShotID"`
	CreatedAt              time.Time                     `json:"created_at"`
	UpdatedAt              time.Time                     `json:"updated_at"`
}

type NovelVideoShotRenderAttempt struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ProjectID          uint      `json:"project_id" gorm:"index"`
	EpisodeID          uint      `json:"episode_id" gorm:"index"`
	ShotID             uint      `json:"shot_id" gorm:"index"`
	UserID             uint      `json:"user_id" gorm:"index"`
	GenerationRecordID *uint     `json:"generation_record_id" gorm:"index"`
	Status             string    `json:"status" gorm:"size:32;index"`
	Progress           int       `json:"progress"`
	ErrorCode          string    `json:"error_code" gorm:"size:128"`
	ErrorMessage       string    `json:"error_message" gorm:"type:text"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NovelVideoShotImage struct {
	ID                    uint           `json:"id" gorm:"primaryKey"`
	ProjectID             uint           `json:"project_id" gorm:"index"`
	EpisodeID             uint           `json:"episode_id" gorm:"index"`
	ShotID                uint           `json:"shot_id" gorm:"index"`
	UserID                uint           `json:"user_id" gorm:"index"`
	GenerationRecordID    *uint          `json:"generation_record_id" gorm:"index"`
	WorkID                *uint          `json:"work_id" gorm:"index"`
	Kind                  string         `json:"kind" gorm:"size:48;index"`
	Prompt                string         `json:"prompt" gorm:"type:text"`
	NegativePrompt        string         `json:"negative_prompt" gorm:"type:text"`
	ReferenceAssetIDsJSON string         `json:"-" gorm:"column:reference_asset_ids;type:text"`
	ActorIDsJSON          string         `json:"-" gorm:"column:actor_ids;type:text"`
	ReferenceIntent       string         `json:"reference_intent" gorm:"size:32;index"`
	Mode                  string         `json:"mode" gorm:"size:32;index"`
	LockLevel             string         `json:"lock_level" gorm:"size:32;index"`
	Version               int            `json:"version" gorm:"default:1"`
	Selected              bool           `json:"selected" gorm:"index"`
	ReviewStatus          string         `json:"review_status" gorm:"size:32;index"`
	ReviewNote            string         `json:"review_note" gorm:"type:text"`
	ErrorCode             string         `json:"error_code" gorm:"size:128"`
	ErrorMessage          string         `json:"error_message" gorm:"type:text"`
	PreviewURL            string         `json:"preview_url" gorm:"-"`
	DownloadURL           string         `json:"download_url" gorm:"-"`
	GenerationStatus      string         `json:"generation_status" gorm:"-"`
	GenerationStage       string         `json:"generation_stage" gorm:"-"`
	GenerationProgress    int            `json:"generation_progress" gorm:"-"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `json:"-" gorm:"index"`
}

type NovelVideoComposition struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProjectID    uint      `json:"project_id" gorm:"index"`
	UserID       uint      `json:"user_id" gorm:"index"`
	EpisodeID    *uint     `json:"episode_id" gorm:"index"`
	JobID        *uint     `json:"job_id" gorm:"index"`
	WorkID       *uint     `json:"work_id" gorm:"index"`
	OutputURL    string    `json:"output_url" gorm:"type:text"`
	SubtitleURL  string    `json:"subtitle_url" gorm:"type:text"`
	ManifestJSON string    `json:"-" gorm:"column:manifest;type:text"`
	Status       string    `json:"status" gorm:"size:32;index"`
	ErrorCode    string    `json:"error_code" gorm:"size:128"`
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type NovelVideoGrid struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ProjectID   uint      `json:"project_id" gorm:"index"`
	UserID      uint      `json:"user_id" gorm:"index"`
	EpisodeID   *uint     `json:"episode_id" gorm:"index"`
	GridType    string    `json:"grid_type" gorm:"size:32;index"`
	GridSize    int       `json:"grid_size"`
	ShotIDsJSON string    `json:"-" gorm:"column:shot_ids;type:text"`
	PromptJSON  string    `json:"-" gorm:"column:prompt;type:text"`
	Status      string    `json:"status" gorm:"size:32;index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type NovelVideoVersion struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProjectID    uint      `json:"project_id" gorm:"index"`
	UserID       uint      `json:"user_id" gorm:"index"`
	Label        string    `json:"label" gorm:"size:160"`
	SnapshotJSON string    `json:"-" gorm:"column:snapshot;type:text"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type NovelVideoJob struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	ProjectID    uint       `json:"project_id" gorm:"index"`
	UserID       uint       `json:"user_id" gorm:"index"`
	JobType      string     `json:"type" gorm:"column:job_type;size:48;index"`
	Status       string     `json:"status" gorm:"size:32;index"`
	EpisodeID    *uint      `json:"episode_id" gorm:"index"`
	ShotID       *uint      `json:"shot_id" gorm:"index"`
	AssetID      *uint      `json:"asset_id" gorm:"index"`
	DependsOnID  *uint      `json:"depends_on_id" gorm:"index"`
	Attempts     int        `json:"attempts"`
	MaxAttempts  int        `json:"max_attempts" gorm:"default:3"`
	Progress     int        `json:"progress"`
	PayloadJSON  string     `json:"-" gorm:"column:payload;type:text"`
	ResultJSON   string     `json:"-" gorm:"column:result;type:text"`
	ErrorCode    string     `json:"error_code" gorm:"size:128"`
	ErrorMessage string     `json:"error_message" gorm:"type:text"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type ReferenceAsset struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	UserID           uint           `json:"user_id" gorm:"index;uniqueIndex:ux_reference_assets_storage_object,priority:1,where:deleted_at IS NULL AND storage_scope = 'commerce_private'"`
	AssetKey         string         `json:"asset_key" gorm:"size:255;uniqueIndex:ux_reference_assets_storage_object,priority:3"`
	PreviewURL       string         `json:"preview_url" gorm:"type:text"`
	MIMEType         string         `json:"mime_type" gorm:"size:64"`
	Kind             string         `json:"kind" gorm:"-"`
	OriginalFilename string         `json:"original_filename" gorm:"type:text"`
	DisplayName      string         `json:"display_name" gorm:"type:text"`
	StorageScope     string         `json:"storage_scope" gorm:"size:32;uniqueIndex:ux_reference_assets_storage_object,priority:2"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

type GenerationReferenceAsset struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	GenerationRecordID uint      `json:"generation_record_id" gorm:"index"`
	ReferenceAssetID   uint      `json:"reference_asset_id" gorm:"index"`
	SortOrder          int       `json:"sort_order"`
	Role               string    `json:"role" gorm:"size:24;index"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type VideoConversation struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	UserID           uint           `json:"user_id" gorm:"index:idx_video_conversations_user_activity,priority:1;index:idx_video_conversations_user_favorite_activity,priority:1"`
	Title            string         `json:"title" gorm:"size:180"`
	IsFavorite       bool           `json:"is_favorite" gorm:"index:idx_video_conversations_user_favorite_activity,priority:2"`
	LastGenerationID *uint          `json:"last_generation_id" gorm:"index"`
	LastActivityAt   time.Time      `json:"last_activity_at" gorm:"index:idx_video_conversations_user_activity,priority:2,sort:desc;index:idx_video_conversations_user_favorite_activity,priority:3,sort:desc"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

type VideoConversationMessage struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ConversationID     uint      `json:"conversation_id" gorm:"index;uniqueIndex:ux_video_message_idempotency,priority:1"`
	UserID             uint      `json:"user_id" gorm:"index"`
	Role               string    `json:"role" gorm:"size:16;index"`
	Content            string    `json:"content" gorm:"type:text"`
	Status             string    `json:"status" gorm:"size:24;index"`
	ReplyToMessageID   *uint     `json:"reply_to_message_id" gorm:"index"`
	SuggestedPrompt    string    `json:"suggested_prompt" gorm:"type:text"`
	ReadyToGenerate    bool      `json:"ready_to_generate"`
	QuickRepliesJSON   string    `json:"-" gorm:"type:text"`
	QuickReplies       []string  `json:"quick_replies" gorm:"-"`
	IdempotencyKey     string    `json:"-" gorm:"size:160;uniqueIndex:ux_video_message_idempotency,priority:2"`
	RequestFingerprint string    `json:"-" gorm:"size:64"`
	ModelID            uint      `json:"-" gorm:"index"`
	ChannelID          uint      `json:"-" gorm:"index"`
	ProviderRequestID  string    `json:"-" gorm:"size:128"`
	LatencyMS          int64     `json:"-"`
	ErrorCode          string    `json:"-" gorm:"size:128"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (m *VideoConversationMessage) AfterFind(tx *gorm.DB) error {
	if strings.TrimSpace(m.QuickRepliesJSON) != "" {
		_ = json.Unmarshal([]byte(m.QuickRepliesJSON), &m.QuickReplies)
	}
	return nil
}

type Invite struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	Code       string     `json:"code" gorm:"uniqueIndex;size:64"`
	Label      string     `json:"label"`
	Status     string     `json:"status" gorm:"size:32"`
	TotalQuota int        `json:"total_quota"`
	UsedQuota  int        `json:"used_quota"`
	ExpiresAt  *time.Time `json:"expires_at"`
	Notes      string     `json:"notes"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (i Invite) RemainingQuota() int {
	remaining := i.TotalQuota - i.UsedQuota
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (i Invite) ValidateAvailability(now time.Time) error {
	if i.Status != InviteStatusActive {
		return errors.New("invite_disabled")
	}
	if i.ExpiresAt != nil && now.After(*i.ExpiresAt) {
		return errors.New("invite_expired")
	}
	if i.TotalQuota > 0 && i.UsedQuota >= i.TotalQuota {
		return errors.New("invite_quota_exhausted")
	}
	return nil
}

type InviteRedemption struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	InviteID     uint      `json:"invite_id" gorm:"index"`
	InviteCode   string    `json:"invite_code" gorm:"index;size:64"`
	InviterName  string    `json:"inviter_name" gorm:"size:128"`
	UserID       uint      `json:"user_id" gorm:"index"`
	Username     string    `json:"username" gorm:"size:64;index"`
	DisplayName  string    `json:"display_name" gorm:"size:128"`
	Email        string    `json:"email" gorm:"size:255"`
	RegisteredAt time.Time `json:"registered_at" gorm:"index"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AppSettings struct {
	ID                                       uint      `json:"id" gorm:"primaryKey"`
	ActiveImageModel                         string    `json:"active_image_model"`
	AllowedImageModelsJSON                   string    `json:"-"`
	DefaultImageModelID                      *uint     `json:"default_image_model_id"`
	DefaultVideoModelID                      *uint     `json:"default_video_model_id"`
	FallbackModelID                          *uint     `json:"fallback_model_id"`
	ModelRoutingEnabled                      bool      `json:"model_routing_enabled"`
	ModelRoutingStrategy                     string    `json:"model_routing_strategy" gorm:"size:32"`
	ModelConcurrencyLimit                    int       `json:"model_concurrency_limit"`
	RequestTimeoutSeconds                    int       `json:"request_timeout_seconds"`
	DefaultInviteQuota                       int       `json:"default_invite_quota"`
	RateLimitWindowSeconds                   int       `json:"rate_limit_window_seconds"`
	RateLimitMaxRequests                     int       `json:"rate_limit_max_requests"`
	SystemSettingsInitialized                bool      `json:"-"`
	PlatformName                             string    `json:"platform_name" gorm:"size:128"`
	PlatformShortName                        string    `json:"platform_short_name" gorm:"size:32"`
	PlatformLogoURL                          string    `json:"platform_logo_url" gorm:"type:text"`
	PlatformTimezone                         string    `json:"platform_timezone" gorm:"size:64"`
	PlatformLanguage                         string    `json:"platform_language" gorm:"size:32"`
	PlatformCurrency                         string    `json:"platform_currency" gorm:"size:16"`
	PlatformICPRecordNumber                  string    `json:"platform_icp_record_number" gorm:"size:128"`
	PlatformDomain                           string    `json:"platform_domain" gorm:"type:text"`
	StorageMode                              string    `json:"storage_mode" gorm:"size:32"`
	StorageProvider                          string    `json:"storage_provider" gorm:"size:96"`
	StorageRegion                            string    `json:"storage_region" gorm:"size:96"`
	StorageBucket                            string    `json:"storage_bucket" gorm:"size:160"`
	StorageCDNDomain                         string    `json:"storage_cdn_domain" gorm:"type:text"`
	StorageCDNAcceleration                   bool      `json:"storage_cdn_acceleration"`
	GenerationUploadLimit                    int       `json:"generation_upload_limit"`
	GenerationDefaultAspectRatio             string    `json:"generation_default_aspect_ratio" gorm:"size:16"`
	GenerationRetentionDays                  int       `json:"generation_retention_days"`
	GenerationConcurrencyLimit               int       `json:"generation_concurrency_limit"`
	GenerationReviewPolicy                   string    `json:"generation_review_policy" gorm:"size:32"`
	GenerationNegativePromptEnabled          bool      `json:"generation_negative_prompt_enabled"`
	GenerationAdvancedParametersEnabled      bool      `json:"generation_advanced_parameters_enabled"`
	NotificationEmail                        string    `json:"notification_email" gorm:"size:255"`
	NotificationTaskCompleteNotice           bool      `json:"notification_task_complete_notice"`
	NotificationSystemAlertNotice            bool      `json:"notification_system_alert_notice"`
	NotificationDailySummaryNotice           bool      `json:"notification_daily_summary_notice"`
	NotificationWebhookURL                   string    `json:"notification_webhook_url" gorm:"type:text"`
	SecurityLoginPolicy                      string    `json:"security_login_policy" gorm:"size:32"`
	SecurityPasswordMinLength                int       `json:"security_password_min_length"`
	SecurityTwoFactorEnabled                 bool      `json:"security_two_factor_enabled"`
	SecurityFailedLoginLockEnabled           bool      `json:"security_failed_login_lock_enabled"`
	SecurityAdminPermissionManagementEnabled bool      `json:"security_admin_permission_management_enabled"`
	CustomerServiceConfigJSON                string    `json:"-" gorm:"type:text"`
	CreatedAt                                time.Time `json:"created_at"`
	UpdatedAt                                time.Time `json:"updated_at"`
}

func (s *AppSettings) AllowedImageModels() []string {
	if s.AllowedImageModelsJSON == "" {
		return nil
	}
	var models []string
	_ = json.Unmarshal([]byte(s.AllowedImageModelsJSON), &models)
	return models
}

func (s *AppSettings) SetAllowedImageModels(models []string) error {
	payload, err := json.Marshal(models)
	if err != nil {
		return err
	}
	s.AllowedImageModelsJSON = string(payload)
	return nil
}

type ModelConfig struct {
	ID                      uint           `json:"id" gorm:"primaryKey"`
	Name                    string         `json:"name" gorm:"size:160;index"`
	Type                    string         `json:"type" gorm:"size:32;index"`
	Provider                string         `json:"provider" gorm:"size:96;index"`
	Status                  string         `json:"status" gorm:"size:32;index"`
	Priority                int            `json:"priority"`
	CostLabel               string         `json:"cost_label" gorm:"size:96"`
	Permission              string         `json:"permission" gorm:"size:32;index"`
	Weight                  int            `json:"weight"`
	SortOrder               int            `json:"sort_order" gorm:"column:sort_order;index"`
	RuntimeModel            string         `json:"runtime_model" gorm:"size:128"`
	APIBaseURL              string         `json:"api_base_url" gorm:"size:255"`
	APIEndpoint             string         `json:"api_endpoint" gorm:"size:128"`
	APIKey                  string         `json:"-" gorm:"type:text"`
	VideoReadinessStatus    string         `json:"video_readiness_status" gorm:"size:32;index"`
	VideoReadinessReason    string         `json:"video_readiness_reason" gorm:"type:text"`
	VideoReadinessCheckedAt *time.Time     `json:"video_readiness_checked_at"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	DeletedAt               gorm.DeletedAt `json:"-" gorm:"index"`
}

type ModelCatalog struct {
	ID                   uint           `json:"id" gorm:"primaryKey"`
	Name                 string         `json:"name" gorm:"size:160;index"`
	Modality             string         `json:"modality" gorm:"size:32;index"`
	Status               string         `json:"status" gorm:"size:32;index"`
	Visibility           string         `json:"visibility" gorm:"size:32;index"`
	DefaultCreditsCost   int            `json:"default_credits_cost"`
	CapabilityTagsJSON   string         `json:"-" gorm:"column:capability_tags_json;type:text"`
	CapabilityTags       []string       `json:"capability_tags" gorm:"-"`
	VideoDurationsJSON   string         `json:"-" gorm:"column:video_durations_json;type:text"`
	VideoDurations       []string       `json:"video_durations" gorm:"-"`
	DefaultVideoDuration string         `json:"default_video_duration" gorm:"size:16"`
	SortOrder            int            `json:"sort_order" gorm:"index"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

func (m *ModelCatalog) AfterFind(tx *gorm.DB) error {
	m.CapabilityTags = decodeStringList(m.CapabilityTagsJSON)
	m.VideoDurations = decodeVideoDurations(m.VideoDurationsJSON)
	return nil
}

func (m *ModelCatalog) BeforeSave(tx *gorm.DB) error {
	if m.CapabilityTags != nil {
		payload, err := json.Marshal(normalizeStringList(m.CapabilityTags))
		if err != nil {
			return err
		}
		m.CapabilityTagsJSON = string(payload)
	}
	if m.VideoDurations != nil {
		payload, err := json.Marshal(m.VideoDurations)
		if err != nil {
			return err
		}
		m.VideoDurationsJSON = string(payload)
	}
	return nil
}

type ModelProvider struct {
	ID                    uint           `json:"id" gorm:"primaryKey"`
	Name                  string         `json:"name" gorm:"size:160;index"`
	Provider              string         `json:"provider" gorm:"size:96;index"`
	BaseURL               string         `json:"base_url" gorm:"size:255"`
	APIKey                string         `json:"-" gorm:"type:text"`
	DefaultTimeoutSeconds int            `json:"default_timeout_seconds"`
	ConcurrencyLimit      int            `json:"concurrency_limit"`
	Status                string         `json:"status" gorm:"size:32;index"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `json:"-" gorm:"index"`
}

type ModelChannel struct {
	ID                      uint           `json:"id" gorm:"primaryKey"`
	ModelID                 uint           `json:"model_id" gorm:"index"`
	Model                   ModelCatalog   `json:"model,omitempty" gorm:"foreignKey:ModelID"`
	ProviderID              uint           `json:"provider_id" gorm:"index"`
	Provider                ModelProvider  `json:"provider_account,omitempty" gorm:"foreignKey:ProviderID"`
	LegacyModelConfigID     uint           `json:"legacy_model_config_id" gorm:"index"`
	Name                    string         `json:"name" gorm:"size:160;index"`
	RuntimeModel            string         `json:"runtime_model" gorm:"size:128;index"`
	VideoDurationsJSON      string         `json:"-" gorm:"column:video_durations_json;type:text"`
	VideoDurations          []string       `json:"video_durations" gorm:"-"`
	Endpoint                string         `json:"endpoint" gorm:"size:128"`
	Weight                  int            `json:"weight"`
	Priority                int            `json:"priority"`
	Status                  string         `json:"status" gorm:"size:32;index"`
	HealthStatus            string         `json:"health_status" gorm:"size:32;index"`
	FailCooldownUntil       *time.Time     `json:"fail_cooldown_until"`
	LastFailureAt           *time.Time     `json:"last_failure_at"`
	LastErrorCode           string         `json:"last_error_code" gorm:"size:128"`
	ConsecutiveFailureCount int            `json:"consecutive_failure_count"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	DeletedAt               gorm.DeletedAt `json:"-" gorm:"index"`
}

func (m *ModelChannel) AfterFind(tx *gorm.DB) error {
	m.VideoDurations = decodeVideoDurations(m.VideoDurationsJSON)
	return nil
}

func (m *ModelChannel) BeforeSave(tx *gorm.DB) error {
	if m.VideoDurations != nil {
		payload, err := json.Marshal(m.VideoDurations)
		if err != nil {
			return err
		}
		m.VideoDurationsJSON = string(payload)
	}
	return nil
}

type ModelRoutingPolicy struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	Modality        string    `json:"modality" gorm:"uniqueIndex;size:32"`
	DefaultModelID  uint      `json:"default_model_id" gorm:"index"`
	FallbackModelID uint      `json:"fallback_model_id" gorm:"index"`
	RoutingEnabled  bool      `json:"routing_enabled"`
	RoutingStrategy string    `json:"routing_strategy" gorm:"size:32"`
	Source          string    `json:"source" gorm:"size:32"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ModelRoutingEntry struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	PolicyID  uint      `json:"policy_id" gorm:"index"`
	ModelID   uint      `json:"model_id" gorm:"index"`
	ChannelID uint      `json:"channel_id" gorm:"index"`
	Enabled   bool      `json:"enabled" gorm:"index"`
	Weight    int       `json:"weight"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GenerationRecord struct {
	ID                           uint       `json:"id" gorm:"primaryKey;index:idx_generation_admin_list,priority:2,sort:desc"`
	UserID                       uint       `json:"user_id" gorm:"index"`
	InviteID                     uint       `json:"invite_id" gorm:"index"`
	InviteCode                   string     `json:"invite_code" gorm:"index;size:64"`
	WorkID                       *uint      `json:"work_id" gorm:"index"`
	Prompt                       string     `json:"prompt" gorm:"type:text"`
	NegativePrompt               string     `json:"negative_prompt" gorm:"type:text"`
	AspectRatio                  string     `json:"aspect_ratio" gorm:"size:16"`
	Quality                      string     `json:"quality" gorm:"size:16"`
	StylePreset                  string     `json:"style_preset" gorm:"size:128"`
	ToolMode                     string     `json:"tool_mode" gorm:"size:32"`
	ToolOptionsJSON              string     `json:"-" gorm:"column:tool_options;type:text"`
	BatchID                      string     `json:"batch_id" gorm:"size:64;index"`
	BatchIndex                   int        `json:"batch_index"`
	BatchTotal                   int        `json:"batch_total"`
	VariationMode                string     `json:"variation_mode" gorm:"size:32"`
	VariationPrompt              string     `json:"variation_prompt" gorm:"type:text"`
	StyleStrength                int        `json:"style_strength"`
	ReferenceWeight              int        `json:"reference_weight"`
	Seed                         string     `json:"seed" gorm:"size:128"`
	SourceWorkID                 *uint      `json:"source_work_id" gorm:"index"`
	MaskAssetID                  *uint      `json:"mask_asset_id" gorm:"index"`
	EditInstruction              string     `json:"edit_instruction" gorm:"type:text"`
	ModelID                      uint       `json:"model_id" gorm:"index"`
	ChannelID                    uint       `json:"channel_id" gorm:"index"`
	ModelName                    string     `json:"model_name" gorm:"size:160"`
	ChannelName                  string     `json:"channel_name" gorm:"size:160"`
	RuntimeModel                 string     `json:"runtime_model" gorm:"size:128;index"`
	ModelConfigID                uint       `json:"model_config_id" gorm:"index"`
	Model                        string     `json:"model" gorm:"size:128"`
	Status                       string     `json:"status" gorm:"size:32"`
	Stage                        string     `json:"stage" gorm:"size:64"`
	ErrorCode                    string     `json:"error_code" gorm:"size:64"`
	ErrorMessage                 string     `json:"error_message" gorm:"type:text"`
	AssetKey                     string     `json:"asset_key" gorm:"size:255"`
	PreviewURL                   string     `json:"preview_url" gorm:"type:text"`
	DownloadURL                  string     `json:"download_url" gorm:"type:text"`
	MIMEType                     string     `json:"mime_type" gorm:"size:64"`
	LatencyMS                    int64      `json:"latency_ms"`
	ProviderRequestID            string     `json:"provider_request_id" gorm:"size:128"`
	ProviderHTTPStatus           int        `json:"provider_http_status"`
	ProviderErrorCode            string     `json:"provider_error_code" gorm:"size:128"`
	ProviderErrorMessage         string     `json:"provider_error_message" gorm:"type:text"`
	ProviderFailureStage         string     `json:"provider_failure_stage" gorm:"size:64"`
	ProviderAttemptCount         int        `json:"provider_attempt_count"`
	ProviderRequestStarted       bool       `json:"provider_request_started"`
	ProviderIdempotencySupported bool       `json:"provider_idempotency_supported"`
	CreditsCost                  int        `json:"credits_cost"`
	CreditsDeducted              bool       `json:"credits_deducted"`
	StorageScope                 string     `json:"storage_scope" gorm:"size:32"`
	ExecutionKey                 *string    `json:"execution_key,omitempty" gorm:"size:160;uniqueIndex"`
	Progress                     int        `json:"progress"`
	RequestFingerprint           string     `json:"-" gorm:"size:64;index"`
	ReferenceAssetIDs            []uint     `json:"reference_asset_ids,omitempty" gorm:"-"`
	QueuePosition                int64      `json:"queue_position,omitempty" gorm:"-"`
	QueueWaitMS                  int64      `json:"queue_wait_ms,omitempty" gorm:"-"`
	ExecutionAttemptCount        int        `json:"execution_attempt_count,omitempty" gorm:"-"`
	NextAttemptAt                *time.Time `json:"next_attempt_at,omitempty" gorm:"-"`
	CreatedAt                    time.Time  `json:"created_at" gorm:"index:idx_generation_admin_list,priority:1,sort:desc"`
	UpdatedAt                    time.Time  `json:"updated_at"`
}

const (
	ImageGenerationJobStatusQueued     = "queued"
	ImageGenerationJobStatusRunning    = "running"
	ImageGenerationJobStatusRetryWait  = "retry_wait"
	ImageGenerationJobStatusPersisting = "persisting"
	ImageGenerationJobStatusSucceeded  = "succeeded"
	ImageGenerationJobStatusFailed     = "failed"
	ImageGenerationJobStatusCancelled  = "cancelled"
)

// ImageGenerationJob 是内部可靠队列；GenerationRecord 仍是面向用户的结果投影。
type ImageGenerationJob struct {
	ID                           uint       `json:"id" gorm:"primaryKey"`
	GenerationRecordID           uint       `json:"generation_record_id" gorm:"uniqueIndex;index"`
	UserID                       uint       `json:"user_id" gorm:"index:idx_image_jobs_dispatch,priority:2;index:idx_image_jobs_user_active,priority:1;uniqueIndex:ux_image_job_user_idempotency,priority:1"`
	EntryPoint                   string     `json:"entry_point" gorm:"size:64;index"`
	Priority                     int        `json:"priority" gorm:"index:idx_image_jobs_dispatch,priority:1,sort:desc"`
	RequestVersion               int        `json:"request_version"`
	RequestSnapshotJSON          string     `json:"-" gorm:"type:text"`
	RequestFingerprint           string     `json:"-" gorm:"size:64"`
	IdempotencyKey               string     `json:"-" gorm:"size:160;uniqueIndex:ux_image_job_user_idempotency,priority:2"`
	IdempotencyExpiresAt         time.Time  `json:"-" gorm:"index"`
	Status                       string     `json:"status" gorm:"size:32;index:idx_image_jobs_dispatch,priority:3;index:idx_image_jobs_user_active,priority:2"`
	Stage                        string     `json:"stage" gorm:"size:64"`
	AttemptCount                 int        `json:"attempt_count"`
	MaxAttempts                  int        `json:"max_attempts"`
	NextAttemptAt                *time.Time `json:"next_attempt_at" gorm:"index:idx_image_jobs_dispatch,priority:4"`
	QueueDeadlineAt              time.Time  `json:"queue_deadline_at" gorm:"index"`
	LeaseOwner                   string     `json:"-" gorm:"size:128"`
	LeaseToken                   string     `json:"-" gorm:"size:128;index"`
	LeaseExpiresAt               *time.Time `json:"lease_expires_at" gorm:"index"`
	CancelRequestedAt            *time.Time `json:"cancel_requested_at"`
	ReservedCredits              int        `json:"reserved_credits"`
	CreditsSettled               bool       `json:"credits_settled"`
	CreditsReleased              bool       `json:"credits_released"`
	ProviderRequestStarted       bool       `json:"provider_request_started"`
	ProviderIdempotencySupported bool       `json:"provider_idempotency_supported"`
	SpoolPath                    string     `json:"-" gorm:"size:512"`
	ErrorCode                    string     `json:"error_code" gorm:"size:64"`
	ErrorMessage                 string     `json:"error_message" gorm:"type:text"`
	QueuedAt                     time.Time  `json:"queued_at" gorm:"index"`
	ClaimedAt                    *time.Time `json:"claimed_at"`
	StartedAt                    *time.Time `json:"started_at"`
	CompletedAt                  *time.Time `json:"completed_at"`
	CreatedAt                    time.Time  `json:"created_at"`
	UpdatedAt                    time.Time  `json:"updated_at"`
}

// ImageExecutionLease 是跨进程统一的重型图片执行槽位。
type ImageExecutionLease struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	Token      string    `json:"-" gorm:"size:128;uniqueIndex"`
	Owner      string    `json:"-" gorm:"size:128;index"`
	JobID      *uint     `json:"job_id" gorm:"index"`
	RecordID   *uint     `json:"generation_record_id" gorm:"index"`
	UserID     uint      `json:"user_id" gorm:"index"`
	ProviderID uint      `json:"provider_id" gorm:"index"`
	ChannelID  uint      `json:"channel_id" gorm:"index"`
	EntryPoint string    `json:"entry_point" gorm:"size:64;index"`
	ExpiresAt  time.Time `json:"expires_at" gorm:"index"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type VideoGenerationRecord struct {
	ID                   uint           `json:"id" gorm:"primaryKey"`
	GenerationRecordID   uint           `json:"generation_record_id" gorm:"uniqueIndex;index"`
	ConversationID       *uint          `json:"conversation_id" gorm:"index"`
	Progress             int            `json:"progress"`
	UserID               uint           `json:"user_id" gorm:"index"`
	WorkID               *uint          `json:"work_id" gorm:"index"`
	Source               string         `json:"source" gorm:"size:32;index"`
	NovelVideoProjectID  *uint          `json:"novel_video_project_id" gorm:"index"`
	NovelVideoEpisodeID  *uint          `json:"novel_video_episode_id" gorm:"index"`
	NovelVideoShotID     *uint          `json:"novel_video_shot_id" gorm:"index"`
	NovelVideoAttemptID  *uint          `json:"novel_video_attempt_id" gorm:"index"`
	Prompt               string         `json:"prompt" gorm:"type:text"`
	AspectRatio          string         `json:"aspect_ratio" gorm:"size:16"`
	StylePreset          string         `json:"style_preset" gorm:"size:128;index"`
	DurationSeconds      int            `json:"duration_seconds"`
	InputImageCount      int            `json:"input_image_count"`
	ReferenceAssetCount  int            `json:"reference_asset_count"`
	ModelConfigID        uint           `json:"model_config_id" gorm:"index"`
	ModelName            string         `json:"model_name" gorm:"size:160"`
	RuntimeModel         string         `json:"runtime_model" gorm:"size:128;index"`
	Provider             string         `json:"provider" gorm:"size:96;index"`
	ProviderRequestID    string         `json:"provider_request_id" gorm:"size:128;index"`
	Status               string         `json:"status" gorm:"size:32;index"`
	Stage                string         `json:"stage" gorm:"size:64"`
	ErrorCode            string         `json:"error_code" gorm:"size:64"`
	ErrorMessage         string         `json:"error_message" gorm:"type:text"`
	ProviderHTTPStatus   int            `json:"provider_http_status"`
	ProviderErrorCode    string         `json:"provider_error_code" gorm:"size:128"`
	ProviderErrorMessage string         `json:"provider_error_message" gorm:"type:text"`
	ProviderFailureStage string         `json:"provider_failure_stage" gorm:"size:64"`
	LatencyMS            int64          `json:"latency_ms"`
	CreditsCost          int            `json:"credits_cost"`
	CreditsDeducted      bool           `json:"credits_deducted"`
	AssetKey             string         `json:"asset_key" gorm:"size:255"`
	PreviewURL           string         `json:"preview_url" gorm:"type:text"`
	DownloadURL          string         `json:"download_url" gorm:"type:text"`
	MIMEType             string         `json:"mime_type" gorm:"size:64"`
	MetadataJSON         string         `json:"metadata_json" gorm:"type:text"`
	CreatedAt            time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

type ModelCallAttempt struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	GenerationRecordID uint      `json:"generation_record_id" gorm:"index"`
	ChannelID          uint      `json:"channel_id" gorm:"index"`
	ModelConfigID      uint      `json:"model_config_id" gorm:"index"`
	AttemptIndex       int       `json:"attempt_index"`
	Status             string    `json:"status" gorm:"size:32;index"`
	LatencyMS          int64     `json:"latency_ms"`
	HTTPStatus         int       `json:"http_status"`
	ErrorCode          string    `json:"error_code" gorm:"size:128"`
	ErrorMessage       string    `json:"error_message" gorm:"type:text"`
	FailureStage       string    `json:"failure_stage" gorm:"size:64"`
	ProviderRequestID  string    `json:"provider_request_id" gorm:"size:128"`
	StartedAt          time.Time `json:"started_at" gorm:"index"`
	FinishedAt         time.Time `json:"finished_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type GenerationEventLog struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	GenerationRecordID uint      `json:"generation_record_id" gorm:"index"`
	TraceID            string    `json:"trace_id" gorm:"size:128;index"`
	Level              string    `json:"level" gorm:"size:16"`
	Stage              string    `json:"stage" gorm:"size:64"`
	Event              string    `json:"event" gorm:"size:96;index"`
	Message            string    `json:"message" gorm:"type:text"`
	MetadataJSON       string    `json:"metadata_json" gorm:"type:text"`
	CreatedAt          time.Time `json:"created_at" gorm:"index"`
}

type ContentSafetyReview struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	ReviewType         string     `json:"review_type" gorm:"size:64;index"`
	Status             string     `json:"status" gorm:"size:32;index"`
	RiskLevel          string     `json:"risk_level" gorm:"size:32;index"`
	Reason             string     `json:"reason" gorm:"type:text"`
	DecisionComment    string     `json:"decision_comment" gorm:"type:text"`
	TargetType         string     `json:"target_type" gorm:"size:64;index"`
	TargetID           uint       `json:"target_id" gorm:"index"`
	GenerationRecordID *uint      `json:"generation_record_id" gorm:"index"`
	WorkID             *uint      `json:"work_id" gorm:"index"`
	UserID             *uint      `json:"user_id" gorm:"index"`
	InputSummary       string     `json:"input_summary" gorm:"type:text"`
	Model              string     `json:"model" gorm:"size:128"`
	ProviderRequestID  string     `json:"provider_request_id" gorm:"size:128"`
	ReviewerAdminID    *uint      `json:"reviewer_admin_id" gorm:"index"`
	ReviewedAt         *time.Time `json:"reviewed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ContentReport struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	UserID             *uint      `json:"user_id" gorm:"index"`
	TargetType         string     `json:"target_type" gorm:"size:64;index"`
	TargetID           uint       `json:"target_id" gorm:"index"`
	GenerationRecordID *uint      `json:"generation_record_id" gorm:"index"`
	WorkID             *uint      `json:"work_id" gorm:"index"`
	Reason             string     `json:"reason" gorm:"type:text"`
	Description        string     `json:"description" gorm:"type:text"`
	Contact            string     `json:"contact" gorm:"size:255"`
	Status             string     `json:"status" gorm:"size:32;index"`
	Resolution         string     `json:"resolution" gorm:"type:text"`
	ContentReviewID    *uint      `json:"content_review_id" gorm:"index"`
	HandledByAdminID   *uint      `json:"handled_by_admin_id" gorm:"index"`
	HandledAt          *time.Time `json:"handled_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type AlgorithmDisclosure struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	AlgorithmName       string    `json:"algorithm_name" gorm:"size:160;index"`
	AlgorithmType       string    `json:"algorithm_type" gorm:"size:64;index"`
	ServiceDescription  string    `json:"service_description" gorm:"type:text"`
	ProviderDescription string    `json:"provider_description" gorm:"type:text"`
	GovernanceSummary   string    `json:"governance_summary" gorm:"type:text"`
	MarkingSummary      string    `json:"marking_summary" gorm:"type:text"`
	UserRightsSummary   string    `json:"user_rights_summary" gorm:"type:text"`
	DisclosureVersion   string    `json:"disclosure_version" gorm:"size:64"`
	Status              string    `json:"status" gorm:"size:32;index"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type UserConsent struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	UserID         uint      `json:"user_id" gorm:"index"`
	ConsentType    string    `json:"consent_type" gorm:"size:64;index"`
	Version        string    `json:"version" gorm:"size:64;index"`
	Source         string    `json:"source" gorm:"size:64"`
	IPAddress      string    `json:"ip_address" gorm:"size:64"`
	UserAgent      string    `json:"user_agent" gorm:"type:text"`
	GenerationID   *uint     `json:"generation_id" gorm:"index"`
	ReferenceAsset *uint     `json:"reference_asset_id" gorm:"index"`
	CreatedAt      time.Time `json:"created_at"`
}

type AIContentMark struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	GenerationRecordID uint      `json:"generation_record_id" gorm:"index"`
	UserID             uint      `json:"user_id" gorm:"index"`
	AssetKey           string    `json:"asset_key" gorm:"size:255;index"`
	VisibleLabel       string    `json:"visible_label" gorm:"size:128"`
	TraceID            string    `json:"trace_id" gorm:"uniqueIndex;size:128"`
	Model              string    `json:"model" gorm:"size:128"`
	ProviderRequestID  string    `json:"provider_request_id" gorm:"size:128"`
	CreatedAt          time.Time `json:"created_at"`
}

type AlgorithmIncident struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	Title       string     `json:"title" gorm:"size:160"`
	Severity    string     `json:"severity" gorm:"size:32;index"`
	Status      string     `json:"status" gorm:"size:32;index"`
	Description string     `json:"description" gorm:"type:text"`
	Action      string     `json:"action" gorm:"type:text"`
	Owner       string     `json:"owner" gorm:"size:128"`
	OccurredAt  *time.Time `json:"occurred_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ImageGenerationInput struct {
	Model                    string
	Prompt                   string
	NegativePrompt           string
	AspectRatio              string
	Size                     string
	Quality                  string
	StylePreset              string
	ToolMode                 string
	StyleStrength            int
	ReferenceWeight          int
	Seed                     string
	VariationMode            string
	VariationPrompt          string
	ReferenceIntent          string
	BackgroundReferenceIndex *int
	CompositionPlan          *ImageCompositionPlan
	ProviderBaseURL          string
	ProviderAPIKey           string
	ProviderAPIEndpoint      string
	SourceImage              *ReferenceImageInput
	MaskImage                *ReferenceImageInput
	MaskRegions              []ImageMaskRegion
	ReferenceImages          []ReferenceImageInput
	IdempotencyKey           string
	SupportsIdempotencyKey   bool
	ExternalReservation      bool
}

type ImageMaskRegion struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type ImageCompositionPlan struct {
	Prompt                   string
	Source                   string
	FallbackReason           string
	BackgroundReferenceIndex *int
	ReferenceUsages          []ImageCompositionReferenceUsage
}

type ImageCompositionReferenceUsage struct {
	ReferenceIndex int
	Use            string
}

type ReferenceImageInput struct {
	MIMEType   string
	Base64Data string
	InputURL   string
	FilePath   string
}

type ImageGenerationResult struct {
	Base64Image          string
	FilePath             string
	MIMEType             string
	ProviderRequestID    string
	ProviderAttemptCount int
}

const (
	VideoTaskNotStarted = "NOT_START"
	VideoTaskInProgress = "IN_PROGRESS"
	VideoTaskSucceeded  = "SUCCESS"
	VideoTaskFailed     = "FAILURE"
)

type VideoGenerationInput struct {
	Model               string
	Prompt              string
	AspectRatio         string
	Duration            string
	Resolution          string
	HD                  bool
	Watermark           bool
	Private             bool
	ProviderBaseURL     string
	ProviderAPIKey      string
	ProviderAPIEndpoint string
	NotifyHook          string
	Images              []string
	ReferenceVideos     []string
	ReferenceAudios     []string
	GenerateAudio       bool
}

type VideoSubmitResult struct {
	TaskID            string
	ProviderRequestID string
}

type VideoTaskResult struct {
	TaskID            string
	Status            string
	Progress          string
	FailReason        string
	OutputURL         string
	OutputBase64      string
	MIMEType          string
	ProviderRequestID string
	UsageTotalTokens  int
}

type MusicGenerationInput struct {
	Model               string `json:"model"`
	VideoURL            string `json:"video_url,omitempty"`
	VideoBase64         string `json:"video_base64,omitempty"`
	VideoMIMEType       string `json:"video_mime_type"`
	Prompt              string `json:"prompt"`
	Duration            string `json:"duration"`
	AspectRatio         string `json:"aspect_ratio"`
	Variation           string `json:"variation"`
	ProviderBaseURL     string `json:"-"`
	ProviderAPIKey      string `json:"-"`
	ProviderAPIEndpoint string `json:"-"`
}

type MusicGenerationResult struct {
	AudioURL          string `json:"audio_url"`
	AudioBase64       string `json:"audio_base64"`
	MIMEType          string `json:"mime_type"`
	ProviderRequestID string `json:"provider_request_id"`
}

type ProviderError struct {
	HTTPStatus        int
	Code              string
	Message           string
	ProviderRequestID string
	FailureStage      string
	AttemptCount      int
	RetryAfter        time.Duration
	RequestNotSent    bool
}
