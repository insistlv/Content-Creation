package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func currentUser(c *gin.Context) *User {
	return c.MustGet("currentUser").(*User)
}

func currentUserSession(c *gin.Context) *UserSession {
	session, _ := c.Get("currentUserSession")
	if userSession, ok := session.(*UserSession); ok {
		return userSession
	}
	return nil
}

var errInsufficientCredits = errors.New("insufficient credits")

type adminGenerationListItem struct {
	ID            uint                          `json:"id"`
	UserID        uint                          `json:"user_id"`
	WorkID        *uint                         `json:"work_id"`
	User          adminGenerationUserSnapshot   `json:"user"`
	PromptSummary string                        `json:"prompt_summary"`
	PreviewImages []adminGenerationImagePayload `json:"preview_images"`
	ModelID       uint                          `json:"model_id"`
	ChannelID     uint                          `json:"channel_id"`
	ModelConfigID uint                          `json:"model_config_id"`
	ModelName     string                        `json:"model_name"`
	ChannelName   string                        `json:"channel_name"`
	RuntimeModel  string                        `json:"runtime_model"`
	Model         string                        `json:"model"`
	Status        string                        `json:"status"`
	LatencyMS     int64                         `json:"latency_ms"`
	CreditsCost   int                           `json:"credits_cost"`
	ErrorCode     string                        `json:"error_code"`
	CreatedAt     time.Time                     `json:"created_at"`
}

type adminGenerationListRow struct {
	ID                   uint
	UserID               uint
	WorkID               *uint
	Prompt               string
	ModelID              uint
	ChannelID            uint
	ModelConfigID        uint
	ModelName            string
	ChannelName          string
	RuntimeModel         string
	Model                string
	Status               string
	LatencyMS            int64
	ErrorCode            string
	ProviderHTTPStatus   int
	ProviderErrorCode    string
	ProviderErrorMessage string
	ProviderFailureStage string
	ProviderAttemptCount int
	PreviewURL           string
	DownloadURL          string
	MIMEType             string
	CreditsCost          int
	CreditsDeducted      bool
	CreatedAt            time.Time
	Username             string
	DisplayName          string
	Email                string
	AvatarURL            string
}

type adminGenerationUserSnapshot struct {
	ID          uint   `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
}

type adminGenerationImagePayload struct {
	WorkID      *uint  `json:"work_id,omitempty"`
	PreviewURL  string `json:"preview_url"`
	DownloadURL string `json:"download_url"`
	MIMEType    string `json:"mime_type"`
}

type adminGenerationReferenceImagePayload struct {
	ReferenceAssetID uint   `json:"reference_asset_id"`
	PreviewURL       string `json:"preview_url"`
	DownloadURL      string `json:"download_url"`
	MIMEType         string `json:"mime_type"`
	OriginalFilename string `json:"original_filename"`
	SortOrder        int    `json:"sort_order"`
}

type adminGenerationFilters struct {
	Query         string `json:"q"`
	Model         string `json:"model"`
	ModelID       uint   `json:"model_id"`
	ChannelID     uint   `json:"channel_id"`
	ModelConfigID uint   `json:"model_config_id"`
	UserID        uint   `json:"user_id"`
	UserKeyword   string `json:"user_keyword"`
	Status        string `json:"status"`
	DateFrom      string `json:"date_from"`
	DateTo        string `json:"date_to"`
}

type adminGenerationSummary struct {
	TodayGenerations             int64   `json:"today_generations"`
	TodayGenerationsDeltaPercent float64 `json:"today_generations_delta_percent"`
	SuccessRate                  float64 `json:"success_rate"`
	SuccessRateDeltaPercent      float64 `json:"success_rate_delta_percent"`
	AverageLatencyMS             int64   `json:"average_latency_ms"`
	AverageLatencyDeltaPercent   float64 `json:"average_latency_delta_percent"`
	FailedTasks                  int64   `json:"failed_tasks"`
	FailedTasksDeltaPercent      float64 `json:"failed_tasks_delta_percent"`
}

type adminGenerationDetailPayload struct {
	ID                  uint                                      `json:"id"`
	TaskID              string                                    `json:"task_id"`
	UserID              uint                                      `json:"user_id"`
	WorkID              *uint                                     `json:"work_id"`
	User                adminGenerationUserSnapshot               `json:"user"`
	CreatedAt           time.Time                                 `json:"created_at"`
	Status              string                                    `json:"status"`
	ModelID             uint                                      `json:"model_id"`
	ChannelID           uint                                      `json:"channel_id"`
	ModelConfigID       uint                                      `json:"model_config_id"`
	ModelName           string                                    `json:"model_name"`
	ChannelName         string                                    `json:"channel_name"`
	RuntimeModel        string                                    `json:"runtime_model"`
	Model               string                                    `json:"model"`
	LatencyMS           int64                                     `json:"latency_ms"`
	CreditsCost         int                                       `json:"credits_cost"`
	Prompt              string                                    `json:"prompt"`
	Params              adminGenerationParamsPayload              `json:"params"`
	ResultImages        []adminGenerationImagePayload             `json:"result_images"`
	ReferenceImages     []adminGenerationReferenceImagePayload    `json:"reference_images"`
	SourceImage         *adminGenerationImagePayload              `json:"source_image,omitempty"`
	Error               *adminGenerationErrorPayload              `json:"error"`
	ProviderDiagnostics adminGenerationProviderDiagnosticsPayload `json:"provider_diagnostics"`
	Events              []adminGenerationEventPayload             `json:"events"`
}

type adminGenerationParamsPayload struct {
	NegativePrompt  string `json:"negative_prompt"`
	AspectRatio     string `json:"aspect_ratio"`
	Quality         string `json:"quality"`
	StylePreset     string `json:"style_preset"`
	ToolMode        string `json:"tool_mode"`
	StyleStrength   int    `json:"style_strength"`
	ReferenceWeight int    `json:"reference_weight"`
	Seed            string `json:"seed"`
}

type adminGenerationErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type adminGenerationProviderDiagnosticsPayload struct {
	HTTPStatus   int    `json:"provider_http_status"`
	ErrorCode    string `json:"provider_error_code"`
	ErrorMessage string `json:"provider_error_message"`
	FailureStage string `json:"provider_failure_stage"`
	AttemptCount int    `json:"provider_attempt_count"`
}

type adminGenerationEventPayload struct {
	ID        uint           `json:"id"`
	TraceID   string         `json:"trace_id"`
	Level     string         `json:"level"`
	Stage     string         `json:"stage"`
	Event     string         `json:"event"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type workReuseReferenceAssetPayload struct {
	ID               uint   `json:"id"`
	PreviewURL       string `json:"preview_url"`
	MIMEType         string `json:"mime_type"`
	OriginalFilename string `json:"original_filename"`
}

type workReuseReferenceWorkPayload struct {
	ID         uint   `json:"id"`
	PreviewURL string `json:"preview_url"`
	MIMEType   string `json:"mime_type"`
}

type adminUserListItem struct {
	UserID           uint                 `json:"user_id"`
	Username         string               `json:"username"`
	Account          string               `json:"account"`
	Phone            *string              `json:"phone"`
	DisplayName      string               `json:"display_name"`
	Email            string               `json:"email"`
	AvatarURL        string               `json:"avatar_url"`
	Status           string               `json:"status"`
	Online           bool                 `json:"online"`
	WechatBound      bool                 `json:"wechat_bound"`
	WechatOpenID     string               `json:"wechat_open_id"`
	WechatBinding    adminWechatBinding   `json:"wechat_binding"`
	AvailableCredits int                  `json:"available_credits"`
	TotalRecharged   int                  `json:"total_recharged"`
	LastLoginAt      *time.Time           `json:"last_login_at"`
	Role             adminUserRolePayload `json:"role"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type adminWechatBinding struct {
	Bound  bool   `json:"bound"`
	OpenID string `json:"openid"`
}

type adminUserRolePayload struct {
	ID    uint   `json:"id"`
	Code  string `json:"code"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type adminUserListRow struct {
	UserID           uint
	Username         string
	Phone            *string
	DisplayName      string
	Email            string
	AvatarURL        string
	Status           string
	WechatOpenID     string
	AvailableCredits int
	TotalRecharged   int
	LastLoginAt      *time.Time
	Online           bool
	RoleID           uint
	RoleCode         string
	RoleName         string
	RoleColor        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type adminUsersSummary struct {
	UsersTotal                int64   `json:"users_total"`
	ActiveUsers               int64   `json:"active_users"`
	OnlineUsers               int64   `json:"online_users"`
	TodayNewUsers             int64   `json:"today_new_users"`
	TotalCredits              int64   `json:"total_credits"`
	TotalManualTopUp          int64   `json:"total_manual_topup"`
	UsersTotalDeltaPercent    float64 `json:"users_total_delta_percent"`
	ActiveUsersDeltaPercent   float64 `json:"active_users_delta_percent"`
	TodayNewUsersDeltaPercent float64 `json:"today_new_users_delta_percent"`
	TotalCreditsDeltaPercent  float64 `json:"total_credits_delta_percent"`
	UsersTotalSparkline       []int64 `json:"users_total_sparkline"`
	ActiveUsersSparkline      []int64 `json:"active_users_sparkline"`
	TodayNewUsersSparkline    []int64 `json:"today_new_users_sparkline"`
	TotalCreditsSparkline     []int64 `json:"total_credits_sparkline"`
}

type adminPackageSummary struct {
	ActivePackages             int64   `json:"active_packages"`
	ActivePackagesDeltaPercent float64 `json:"active_packages_delta_percent"`
	ActivePackagesSparkline    []int64 `json:"active_packages_sparkline"`
	RevenueSharePercent        int     `json:"revenue_share_percent"`
	RevenueShareDeltaPercent   float64 `json:"revenue_share_delta_percent"`
	RevenueShareSparkline      []int64 `json:"revenue_share_sparkline"`
	AverageOrderCents          int64   `json:"average_order_cents"`
	AverageOrderDeltaPercent   float64 `json:"average_order_delta_percent"`
	AverageOrderSparkline      []int64 `json:"average_order_sparkline"`
	MonthlyOrders              int64   `json:"monthly_orders"`
	MonthlyOrdersDeltaPercent  float64 `json:"monthly_orders_delta_percent"`
	MonthlyOrdersSparkline     []int64 `json:"monthly_orders_sparkline"`
}

type adminPackageWriteRequest struct {
	Name                   *string           `json:"name"`
	Description            *string           `json:"description"`
	PriceLabel             *string           `json:"price_label"`
	PriceCents             *int64            `json:"price_cents"`
	Credits                *int              `json:"credits"`
	ValidDays              *int              `json:"valid_days"`
	Audience               *string           `json:"audience"`
	Tags                   *[]string         `json:"tags"`
	Icon                   *string           `json:"icon"`
	Theme                  *string           `json:"theme"`
	Badge                  *string           `json:"badge"`
	Recommended            *bool             `json:"recommended"`
	Features               *[]string         `json:"features"`
	Benefits               *[]PackageBenefit `json:"benefits"`
	WechatVirtualProductID *string           `json:"wechat_virtual_product_id"`
	SortOrder              *int              `json:"sort_order"`
	IsActive               *bool             `json:"is_active"`
}

type adminPurchaseIntentSummary struct {
	PendingIntents                 int64   `json:"pending_intents"`
	PendingIntentsDeltaPercent     float64 `json:"pending_intents_delta_percent"`
	PendingIntentsSparkline        []int64 `json:"pending_intents_sparkline"`
	TodayNewIntents                int64   `json:"today_new_intents"`
	TodayNewIntentsDeltaPercent    float64 `json:"today_new_intents_delta_percent"`
	TodayNewIntentsSparkline       []int64 `json:"today_new_intents_sparkline"`
	ContactedIntents               int64   `json:"contacted_intents"`
	ContactedIntentsDeltaPercent   float64 `json:"contacted_intents_delta_percent"`
	ContactedIntentsSparkline      []int64 `json:"contacted_intents_sparkline"`
	MonthlyConversionRate          int64   `json:"monthly_conversion_rate"`
	MonthlyConversionDeltaPercent  float64 `json:"monthly_conversion_delta_percent"`
	MonthlyConversionRateSparkline []int64 `json:"monthly_conversion_rate_sparkline"`
}

type adminInviteSummary struct {
	AvailableInvites                 int64   `json:"available_invites"`
	AvailableInvitesDeltaPercent     float64 `json:"available_invites_delta_percent"`
	UsedInvites                      int64   `json:"used_invites"`
	UsedInvitesDeltaPercent          float64 `json:"used_invites_delta_percent"`
	TodayNewInviteUsers              int64   `json:"today_new_invite_users"`
	TodayNewInviteUsersDeltaPercent  float64 `json:"today_new_invite_users_delta_percent"`
	InviteConversionRate             int64   `json:"invite_conversion_rate"`
	InviteConversionRateDeltaPercent float64 `json:"invite_conversion_rate_delta_percent"`
}

type inviteRedemptionListItem struct {
	ID               uint       `json:"id"`
	InviteID         uint       `json:"invite_id"`
	InviteCode       string     `json:"invite_code"`
	InviterName      string     `json:"inviter_name"`
	UserID           uint       `json:"user_id"`
	Username         string     `json:"username"`
	DisplayName      string     `json:"display_name"`
	Email            string     `json:"email"`
	RegisteredAt     time.Time  `json:"registered_at"`
	ConversionResult string     `json:"conversion_result"`
	ConvertedAt      *time.Time `json:"converted_at"`
}

type adminPurchaseIntentUpdateRequest struct {
	Status        *string `json:"status"`
	OwnerName     *string `json:"owner_name"`
	Note          *string `json:"note"`
	CustomerName  *string `json:"customer_name"`
	CustomerEmail *string `json:"customer_email"`
	CustomerPhone *string `json:"customer_phone"`
	ContactType   *string `json:"contact_type"`
	ContactValue  *string `json:"contact_value"`
	Source        *string `json:"source"`
	BudgetRange   *string `json:"budget_range"`
	UseCase       *string `json:"use_case"`
	Region        *string `json:"region"`
	ClosedReason  *string `json:"closed_reason"`
}

type adminCreditTransactionListItem struct {
	ID           uint      `json:"id"`
	UserID       uint      `json:"user_id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name"`
	Type         string    `json:"type"`
	Amount       int       `json:"amount"`
	BalanceAfter int       `json:"balance_after"`
	Reason       string    `json:"reason"`
	RelatedType  string    `json:"related_type"`
	RelatedID    uint      `json:"related_id"`
	AdminNote    string    `json:"admin_note"`
	CreatedAt    time.Time `json:"created_at"`
}

type dashboardKPI struct {
	UsersTotal          int64  `json:"users_total"`
	WorksTotal          int64  `json:"works_total"`
	GenerationTotal     int64  `json:"generation_total"`
	GenerationSucceeded int64  `json:"generation_succeeded"`
	GenerationFailed    int64  `json:"generation_failed"`
	ActivePackages      int64  `json:"packages_active"`
	ActiveInvites       int64  `json:"invites_active"`
	RevenueCompleted    string `json:"revenue_completed"`
}

type dashboardModelItem struct {
	Name                  string `json:"name"`
	Active                bool   `json:"active"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

type dashboardTrendPoint struct {
	Date      string `json:"date"`
	Total     int64  `json:"total"`
	Succeeded int64  `json:"succeeded"`
	Failed    int64  `json:"failed"`
}

type dashboardInviteSummary struct {
	Active    int64 `json:"active"`
	Total     int64 `json:"total"`
	Used      int64 `json:"used"`
	Remaining int64 `json:"remaining"`
}

type dashboardRecentGeneration struct {
	ID         uint      `json:"id"`
	UserID     uint      `json:"user_id"`
	WorkID     *uint     `json:"work_id"`
	Prompt     string    `json:"prompt"`
	Model      string    `json:"model"`
	Status     string    `json:"status"`
	PreviewURL string    `json:"preview_url"`
	CreatedAt  time.Time `json:"created_at"`
}

type worksSummary struct {
	Total          int64            `json:"total"`
	WeekNew        int64            `json:"week_new"`
	StoredPercent  int              `json:"stored_percent"`
	PrivateCount   int64            `json:"private_count"`
	CategoryCounts map[string]int64 `json:"category_counts"`
}

const maxPublicWorksShareIDs = 16

type publicWorkPayload struct {
	WorkID      uint      `json:"work_id"`
	Prompt      string    `json:"prompt"`
	AspectRatio string    `json:"aspect_ratio"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	MIMEType    string    `json:"mime_type"`
	PreviewURL  string    `json:"preview_url"`
	CreatedAt   time.Time `json:"created_at"`
}

func accountPayload(user User, availableCredits int) gin.H {
	return gin.H{
		"user_id":                    user.ID,
		"username":                   user.Username,
		"phone":                      user.Phone,
		"display_name":               user.DisplayName,
		"email":                      user.Email,
		"status":                     user.Status,
		"wechat_openid_bound":        strings.TrimSpace(user.WechatOpenID) != "",
		"available_credits":          availableCredits,
		"payment_password_enabled":   strings.TrimSpace(user.PaymentPasswordHash) != "",
		"login_notification_enabled": user.LoginNotificationEnabled,
		"risk_notification_enabled":  user.RiskNotificationEnabled,
		"created_at":                 user.CreatedAt,
		"updated_at":                 user.UpdatedAt,
	}
}

func appendMiniProgramAuthPayload(c *gin.Context, payload gin.H, session *IssuedUserSession) gin.H {
	if c.GetHeader("X-Image-Agent-Client") != "mp-weixin" || session == nil {
		return payload
	}
	payload["auth_token"] = session.Token
	payload["auth_expires_at"] = session.ExpiresAt.UTC().Format(time.RFC3339)
	return payload
}

func (a *App) lookupBalance(userID uint) (CreditBalance, error) {
	var balance CreditBalance
	err := a.db.Where("user_id = ?", userID).First(&balance).Error
	return balance, err
}

func (a *App) findOwnedWork(c *gin.Context, userID uint) (Work, bool) {
	var work Work
	result := a.db.Where("id = ? AND user_id = ?", c.Param("id"), userID).Limit(1).Find(&work)
	if result.Error != nil {
		writeError(c, http.StatusInternalServerError, "work_load_failed", "作品读取失败")
		return Work{}, false
	}
	if result.RowsAffected == 0 {
		writeError(c, http.StatusNotFound, "work_not_found", "作品不存在")
		return Work{}, false
	}
	return work, true
}
