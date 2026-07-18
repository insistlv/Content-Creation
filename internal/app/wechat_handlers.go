package app

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	errWechatPayNotConfigured = errors.New("wechat pay is not configured")
	errWechatAmountMismatch   = errors.New("wechat amount mismatch")
	errWechatOpenIDMismatch   = errors.New("wechat openid mismatch")
	errWechatInvalidStatus    = errors.New("wechat invalid status")
)

const (
	wechatVirtualOrderStatusInitialized         = 0
	wechatVirtualOrderStatusCreated             = 1
	wechatVirtualOrderStatusPaidPendingDelivery = 2
	wechatVirtualOrderStatusDelivering          = 3
	wechatVirtualOrderStatusDelivered           = 4
	wechatVirtualOrderStatusRefunded            = 5
	wechatVirtualOrderStatusClosed              = 6
	wechatVirtualOrderStatusRefundFailed        = 7
	wechatVirtualOrderStatusUserRefunded        = 8
)

const (
	wechatVirtualPaymentStatePayRequired = "pay_required"
	wechatVirtualPaymentStateAlreadyPaid = "already_paid"
)

type wechatVirtualPendingSyncAction string

const (
	wechatVirtualPendingSyncReuse       wechatVirtualPendingSyncAction = "reuse"
	wechatVirtualPendingSyncReplace     wechatVirtualPendingSyncAction = "replace"
	wechatVirtualPendingSyncAlreadyPaid wechatVirtualPendingSyncAction = "already_paid"
)

type wechatVirtualPendingSyncResult struct {
	Action           wechatVirtualPendingSyncAction
	Order            FinanceOrder
	AvailableCredits int
	Result           wechatVirtualPayQueryResult
}

type wechatSession struct {
	OpenID     string
	SessionKey string
	UnionID    string
}

type wechatSessionExchanger interface {
	Exchange(code string) (wechatSession, error)
}

type wechatPhoneResolver interface {
	ResolvePhone(code string) (string, error)
}

type wechatPayClient interface {
	CreateJSAPIOrder(order FinanceOrder, openid string) (wechatPayRequestParams, string, error)
	QueryOrder(orderNumber string) (wechatPayQueryResult, error)
}

type wechatVirtualPayClient interface {
	QueryOrder(order FinanceOrder) (wechatVirtualPayQueryResult, error)
	NotifyProvideGoods(order FinanceOrder, result wechatVirtualPayQueryResult) error
}

type wechatPayRequestParams struct {
	TimeStamp string `json:"timeStamp"`
	NonceStr  string `json:"nonceStr"`
	Package   string `json:"package"`
	SignType  string `json:"signType"`
	PaySign   string `json:"paySign"`
}

type wechatVirtualPaymentParams struct {
	Mode      string `json:"mode"`
	SignData  string `json:"signData"`
	PaySig    string `json:"paySig"`
	Signature string `json:"signature"`
}

type wechatVirtualSignData struct {
	OfferID      string `json:"offerId"`
	BuyQuantity  int    `json:"buyQuantity"`
	CurrencyType string `json:"currencyType"`
	ProductID    string `json:"productId"`
	GoodsPrice   int64  `json:"goodsPrice"`
	OutTradeNo   string `json:"outTradeNo"`
	Attach       string `json:"attach"`
	Env          int    `json:"env"`
}

type wechatPayQueryResult struct {
	AppID         string
	MchID         string
	OutTradeNo    string
	TransactionID string
	TradeState    string
	PayerOpenID   string
	AmountCents   int64
	SuccessTime   time.Time
	RawSummary    string
}

type wechatVirtualPayQueryResult struct {
	OrderID        string
	WXOrderID      string
	WXPayOrderID   string
	ChannelOrderID string
	Status         int
	OpenID         string
	OrderFee       int64
	PaidFee        int64
	PaidTime       int64
	ProvideTime    int64
	RawSummary     string
}

type httpWechatSessionExchanger struct {
	app *App
}

type httpWechatPhoneResolver struct {
	app       *App
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

const (
	wechatPhoneCodeFailed                = "wechat_phone_failed"
	wechatPhoneCodeInvalid               = "wechat_phone_code_invalid"
	wechatPhoneCodeCapabilityUnavailable = "wechat_phone_capability_unavailable"
	wechatPhoneCodeTokenFailed           = "wechat_phone_token_failed"
)

type wechatPhoneResolveError struct {
	apiCode        string
	errCode        int
	errMsg         string
	rid            string
	httpStatus     int
	detail         string
	retryableToken bool
	cause          error
}

func (e *wechatPhoneResolveError) Error() string {
	if e == nil {
		return "wechat phone failed"
	}
	if strings.TrimSpace(e.errMsg) != "" || e.errCode != 0 {
		return fmt.Sprintf("wechat phone failed: %d %s", e.errCode, e.errMsg)
	}
	if e.cause != nil {
		return "wechat phone failed: " + e.cause.Error()
	}
	return "wechat phone failed"
}

func (e *wechatPhoneResolveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *wechatPhoneResolveError) logDetail() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.detail) != "" {
		return sanitizeWechatLogText(e.detail)
	}
	return formatWechatErrorLogDetail("wechat phone", e.httpStatus, e.errCode, e.errMsg, e.rid)
}

type wechatAccessTokenError struct {
	errCode    int
	errMsg     string
	rid        string
	httpStatus int
	detail     string
	cause      error
}

func (e *wechatAccessTokenError) Error() string {
	if e == nil {
		return "wechat access_token failed"
	}
	if strings.TrimSpace(e.errMsg) != "" || e.errCode != 0 {
		return fmt.Sprintf("wechat access_token failed: %d %s", e.errCode, e.errMsg)
	}
	if e.cause != nil {
		return "wechat access_token failed: " + e.cause.Error()
	}
	return "wechat access_token failed"
}

func (e *wechatAccessTokenError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *wechatAccessTokenError) logDetail() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.detail) != "" {
		return sanitizeWechatLogText(e.detail)
	}
	return formatWechatErrorLogDetail("wechat access_token", e.httpStatus, e.errCode, e.errMsg, e.rid)
}

type httpWechatPayClient struct {
	app *App
}

type httpWechatVirtualPayClient struct {
	app       *App
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newWechatPhoneResponseError(httpStatus, errCode int, errMsg, rid string) *wechatPhoneResolveError {
	apiCode := wechatPhoneAPICodeForErrCode(errCode)
	return &wechatPhoneResolveError{
		apiCode:        apiCode,
		errCode:        errCode,
		errMsg:         strings.TrimSpace(errMsg),
		rid:            strings.TrimSpace(rid),
		httpStatus:     httpStatus,
		retryableToken: isWechatAccessTokenErrCode(errCode),
	}
}

func newWechatPhoneTokenError(err error) *wechatPhoneResolveError {
	detail := "wechat access_token failed"
	var tokenErr *wechatAccessTokenError
	if errors.As(err, &tokenErr) {
		detail = tokenErr.logDetail()
	} else if err != nil {
		detail = "wechat access_token failed error=" + sanitizeWechatLogText(err.Error())
	}
	return &wechatPhoneResolveError{
		apiCode: wechatPhoneCodeTokenFailed,
		detail:  detail,
		cause:   err,
	}
}

func newWechatPhoneResolverError(err error) *wechatPhoneResolveError {
	if err == nil {
		return &wechatPhoneResolveError{apiCode: wechatPhoneCodeFailed, detail: "wechat phone resolver failed"}
	}
	var phoneErr *wechatPhoneResolveError
	if errors.As(err, &phoneErr) {
		return phoneErr
	}
	return &wechatPhoneResolveError{
		apiCode: wechatPhoneCodeFailed,
		detail:  "wechat phone resolver failed error=" + sanitizeWechatLogText(err.Error()),
		cause:   err,
	}
}

func newWechatPhoneInvalidPhoneError() *wechatPhoneResolveError {
	return &wechatPhoneResolveError{
		apiCode: wechatPhoneCodeFailed,
		detail:  "wechat phone response invalid phone",
	}
}

func wechatPhoneAPICodeForErrCode(errCode int) string {
	switch {
	case isWechatAccessTokenErrCode(errCode):
		return wechatPhoneCodeTokenFailed
	}
	switch errCode {
	case 40029, 40163:
		return wechatPhoneCodeInvalid
	case 48001, 48002, 61004, 61007, 89002, 89003, 89006:
		return wechatPhoneCodeCapabilityUnavailable
	default:
		return wechatPhoneCodeFailed
	}
}

func isWechatAccessTokenErrCode(errCode int) bool {
	switch errCode {
	case 40001, 40014, 41001, 42001:
		return true
	default:
		return false
	}
}

func wechatPhoneErrorMessage(apiCode string) string {
	switch apiCode {
	case wechatPhoneCodeInvalid:
		return "微信手机号授权已失效，请重新授权"
	case wechatPhoneCodeCapabilityUnavailable:
		return "微信手机号服务暂不可用，请改用短信验证码"
	case wechatPhoneCodeTokenFailed:
		return "微信手机号授权服务异常，请稍后重试"
	default:
		return "微信手机号授权失败"
	}
}

func writeWechatPhoneResolveError(c *gin.Context, err error) {
	phoneErr := newWechatPhoneResolverError(err)
	apiCode := strings.TrimSpace(phoneErr.apiCode)
	if apiCode == "" {
		apiCode = wechatPhoneCodeFailed
	}
	detail := phoneErr.logDetail()
	if strings.TrimSpace(detail) != "" {
		writeErrorWithLogDetail(c, http.StatusBadGateway, apiCode, wechatPhoneErrorMessage(apiCode), detail)
		return
	}
	writeError(c, http.StatusBadGateway, apiCode, wechatPhoneErrorMessage(apiCode))
}

func formatWechatErrorLogDetail(scope string, httpStatus, errCode int, errMsg, rid string) string {
	parts := []string{sanitizeWechatLogText(scope)}
	if httpStatus > 0 {
		parts = append(parts, fmt.Sprintf("http_status=%d", httpStatus))
	}
	if errCode != 0 {
		parts = append(parts, fmt.Sprintf("errcode=%d", errCode))
	}
	if strings.TrimSpace(errMsg) != "" {
		parts = append(parts, "errmsg="+sanitizeWechatLogText(errMsg))
	}
	if strings.TrimSpace(rid) != "" {
		parts = append(parts, "rid="+sanitizeWechatLogText(rid))
	}
	return strings.Join(parts, " ")
}

func sanitizeWechatLogText(text string) string {
	text = sanitizeRequestLogDetail(text)
	for _, key := range []string{"access_token", "secret", "phone_code", "code"} {
		text = redactWechatLogValue(text, key)
	}
	return text
}

func redactWechatLogValue(text, key string) string {
	needle := strings.ToLower(key) + "="
	searchFrom := 0
	for {
		lower := strings.ToLower(text)
		relativeStart := strings.Index(lower[searchFrom:], needle)
		if relativeStart < 0 {
			return text
		}
		start := searchFrom + relativeStart
		if start > 0 && isWechatLogKeyChar(text[start-1]) {
			searchFrom = start + len(needle)
			continue
		}
		valueStart := start + len(needle)
		valueEnd := valueStart
		for valueEnd < len(text) {
			switch text[valueEnd] {
			case '&', ' ', '"', '\'', ',', '}':
				goto foundEnd
			default:
				valueEnd++
			}
		}
	foundEnd:
		text = text[:valueStart] + "[REDACTED]" + text[valueEnd:]
		searchFrom = valueStart + len("[REDACTED]")
	}
}

func isWechatLogKeyChar(char byte) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '_'
}

func (a *App) handleWechatLogin(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "微信登录 code 不能为空")
		return
	}
	session, err := a.wechatSessionExchanger.Exchange(strings.TrimSpace(req.Code))
	if err != nil || strings.TrimSpace(session.OpenID) == "" {
		writeError(c, http.StatusBadGateway, "wechat_login_failed", "微信登录失败")
		return
	}
	openid := strings.TrimSpace(session.OpenID)

	var user User
	status := http.StatusOK
	err = a.db.Where("wechat_open_id = ?", openid).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusCreated
		now := time.Now()
		username := "wx_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
		passwordHash, hashErr := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.DefaultCost)
		if hashErr != nil {
			writeError(c, http.StatusInternalServerError, "wechat_user_create_failed", "微信账号创建失败")
			return
		}
		user = User{
			Username:                 username,
			DisplayName:              "微信用户",
			PasswordHash:             string(passwordHash),
			WechatOpenID:             openid,
			Status:                   UserStatusActive,
			LoginNotificationEnabled: true,
			RiskNotificationEnabled:  true,
			LastLoginAt:              &now,
		}
		if err := a.db.Transaction(func(tx *gorm.DB) error {
			var standardRole UserRole
			if err := tx.Where("code = ?", "standard_user").First(&standardRole).Error; err != nil {
				return err
			}
			user.UserRoleID = &standardRole.ID
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
			return createSignupBonusTx(tx, user.ID)
		}); err != nil {
			writeError(c, http.StatusInternalServerError, "wechat_user_create_failed", "微信账号创建失败")
			return
		}
	} else if err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_login_failed", "微信登录失败")
		return
	} else {
		now := time.Now()
		_ = a.db.Model(&user).Update("last_login_at", now).Error
		user.LastLoginAt = &now
	}

	sessionToken, err := a.issueUserSession(c.Writer, c.Request, user, false)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "session_issue_failed", "会话创建失败")
		return
	}
	c.Set("currentUser", &user)
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, status, appendMiniProgramAuthPayload(c, accountPayload(user, balance.AvailableCredits), sessionToken))
}

func (a *App) handleWechatPhoneLogin(c *gin.Context) {
	var req struct {
		Code      string `json:"code"`
		PhoneCode string `json:"phone_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil ||
		strings.TrimSpace(req.Code) == "" ||
		strings.TrimSpace(req.PhoneCode) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "微信登录 code 和手机号授权 code 不能为空")
		return
	}
	session, err := a.wechatSessionExchanger.Exchange(strings.TrimSpace(req.Code))
	if err != nil || strings.TrimSpace(session.OpenID) == "" {
		writeError(c, http.StatusBadGateway, "wechat_login_failed", "微信登录失败")
		return
	}
	openid := strings.TrimSpace(session.OpenID)
	phone, err := a.wechatPhoneResolver.ResolvePhone(strings.TrimSpace(req.PhoneCode))
	if err != nil {
		writeWechatPhoneResolveError(c, err)
		return
	}
	if !isValidMainlandPhone(phone) {
		writeWechatPhoneResolveError(c, newWechatPhoneInvalidPhoneError())
		return
	}
	phone = normalizeMainlandPhone(phone)

	var openIDUser User
	err = a.db.Where("wechat_open_id = ?", openid).First(&openIDUser).Error
	if err == nil {
		now := time.Now()
		updates := map[string]any{"last_login_at": now}
		if openIDUser.Phone == nil || strings.TrimSpace(*openIDUser.Phone) == "" {
			var phoneOwner User
			phoneErr := a.db.Where("phone = ?", phone).First(&phoneOwner).Error
			if errors.Is(phoneErr, gorm.ErrRecordNotFound) {
				updates["phone"] = phone
				openIDUser.Phone = &phone
			} else if phoneErr != nil {
				writeError(c, http.StatusInternalServerError, "wechat_phone_login_failed", "手机号账号读取失败")
				return
			}
		}
		if err := a.db.Model(&openIDUser).Updates(updates).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "wechat_phone_login_failed", "微信登录状态更新失败")
			return
		}
		openIDUser.LastLoginAt = &now
		user := openIDUser
		sessionToken, err := a.issueUserSession(c.Writer, c.Request, user, false)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "session_issue_failed", "会话创建失败")
			return
		}
		c.Set("currentUser", &user)
		balance, err := a.lookupBalance(user.ID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
			return
		}
		writeJSON(c, http.StatusOK, appendMiniProgramAuthPayload(c, accountPayload(user, balance.AvailableCredits), sessionToken))
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusInternalServerError, "wechat_phone_login_failed", "微信绑定校验失败")
		return
	}

	var user User
	status := http.StatusOK
	createdUser := false
	if err := a.db.Where("phone = ?", phone).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			now := time.Now()
			username := "wxp_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
			passwordHash, hashErr := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.DefaultCost)
			if hashErr != nil {
				writeError(c, http.StatusInternalServerError, "wechat_user_create_failed", "微信账号创建失败")
				return
			}
			user = User{
				Username:                 username,
				DisplayName:              "微信用户",
				Phone:                    &phone,
				PasswordHash:             string(passwordHash),
				WechatOpenID:             openid,
				Status:                   UserStatusActive,
				LoginNotificationEnabled: true,
				RiskNotificationEnabled:  true,
				LastLoginAt:              &now,
			}
			if err := a.db.Transaction(func(tx *gorm.DB) error {
				var standardRole UserRole
				if err := tx.Where("code = ?", "standard_user").First(&standardRole).Error; err != nil {
					return err
				}
				user.UserRoleID = &standardRole.ID
				if err := tx.Create(&user).Error; err != nil {
					return err
				}
				return createSignupBonusTx(tx, user.ID)
			}); err != nil {
				writeError(c, http.StatusInternalServerError, "wechat_user_create_failed", "微信账号创建失败")
				return
			}
			status = http.StatusCreated
			createdUser = true
		}
		if !createdUser {
			writeError(c, http.StatusInternalServerError, "wechat_phone_login_failed", "手机号账号读取失败")
			return
		}
	}
	if !createdUser {
		if strings.TrimSpace(user.WechatOpenID) != "" && user.WechatOpenID != openid {
			writeError(c, http.StatusConflict, "wechat_openid_conflict", "该手机号已绑定其他微信")
			return
		}

		var existingOpenIDUser User
		err = a.db.Where("wechat_open_id = ? AND id <> ?", openid, user.ID).First(&existingOpenIDUser).Error
		if err == nil {
			writeError(c, http.StatusConflict, "wechat_openid_conflict", "该微信已绑定其他账号")
			return
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusInternalServerError, "wechat_phone_login_failed", "微信绑定校验失败")
			return
		}

		now := time.Now()
		updates := map[string]any{"last_login_at": now}
		if strings.TrimSpace(user.WechatOpenID) == "" {
			updates["wechat_open_id"] = openid
			user.WechatOpenID = openid
		}
		if err := a.db.Model(&user).Updates(updates).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "wechat_bind_failed", "微信绑定失败")
			return
		}
		user.LastLoginAt = &now
	}

	sessionToken, err := a.issueUserSession(c.Writer, c.Request, user, false)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "session_issue_failed", "会话创建失败")
		return
	}
	c.Set("currentUser", &user)
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, status, appendMiniProgramAuthPayload(c, accountPayload(user, balance.AvailableCredits), sessionToken))
}

func (a *App) handleBindWechatPhone(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		Code      string `json:"code"`
		PhoneCode string `json:"phone_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil ||
		strings.TrimSpace(req.Code) == "" ||
		strings.TrimSpace(req.PhoneCode) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "微信登录 code 和手机号授权 code 不能为空")
		return
	}
	if user.Phone != nil && strings.TrimSpace(*user.Phone) != "" {
		writeError(c, http.StatusConflict, "phone_already_bound", "当前账号已绑定手机号")
		return
	}

	session, err := a.wechatSessionExchanger.Exchange(strings.TrimSpace(req.Code))
	if err != nil || strings.TrimSpace(session.OpenID) == "" {
		writeError(c, http.StatusBadGateway, "wechat_login_failed", "微信登录失败")
		return
	}
	openid := strings.TrimSpace(session.OpenID)

	phone, err := a.wechatPhoneResolver.ResolvePhone(strings.TrimSpace(req.PhoneCode))
	if err != nil {
		writeWechatPhoneResolveError(c, err)
		return
	}
	if !isValidMainlandPhone(phone) {
		writeWechatPhoneResolveError(c, newWechatPhoneInvalidPhoneError())
		return
	}
	phone = normalizeMainlandPhone(phone)

	var phoneOwner User
	err = a.db.Where("phone = ? AND id <> ?", phone, user.ID).First(&phoneOwner).Error
	if err == nil {
		writeError(c, http.StatusConflict, "phone_exists", "手机号已被其他账号绑定")
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusInternalServerError, "phone_lookup_failed", "手机号校验失败")
		return
	}

	currentOpenID := strings.TrimSpace(user.WechatOpenID)
	if currentOpenID != "" && currentOpenID != openid {
		writeError(c, http.StatusConflict, "wechat_openid_conflict", "当前账号已绑定其他微信")
		return
	}
	var openIDOwner User
	err = a.db.Where("wechat_open_id = ? AND id <> ?", openid, user.ID).First(&openIDOwner).Error
	if err == nil {
		writeError(c, http.StatusConflict, "wechat_openid_conflict", "该微信已绑定其他账号")
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusInternalServerError, "wechat_phone_failed", "微信绑定校验失败")
		return
	}

	updates := map[string]any{"phone": phone}
	user.Phone = &phone
	if currentOpenID == "" {
		updates["wechat_open_id"] = openid
		user.WechatOpenID = openid
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "phone_bind_failed", "手机号绑定失败")
		return
	}
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleWechatBind(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "微信登录 code 不能为空")
		return
	}
	session, err := a.wechatSessionExchanger.Exchange(strings.TrimSpace(req.Code))
	if err != nil || strings.TrimSpace(session.OpenID) == "" {
		writeError(c, http.StatusBadGateway, "wechat_bind_failed", "微信绑定失败")
		return
	}
	openid := strings.TrimSpace(session.OpenID)
	var existing User
	err = a.db.Where("wechat_open_id = ? AND id <> ?", openid, user.ID).First(&existing).Error
	if err == nil {
		writeError(c, http.StatusConflict, "wechat_openid_conflict", "该微信已绑定其他账号")
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusInternalServerError, "wechat_bind_failed", "微信绑定失败")
		return
	}
	if err := a.db.Model(user).Update("wechat_open_id", openid).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_bind_failed", "微信绑定失败")
		return
	}
	user.WechatOpenID = openid
	balance, _ := a.lookupBalance(user.ID)
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleCreateWechatPayOrder(c *gin.Context) {
	user := currentUser(c)
	if !a.requireBoundPhoneForPayment(c, user) {
		return
	}
	if !wechatPayConfigured(a.cfg) {
		writeError(c, http.StatusServiceUnavailable, "wechat_pay_not_configured", "微信支付暂未配置")
		return
	}
	var req struct {
		PackageID uint `json:"package_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PackageID == 0 {
		writeError(c, http.StatusBadRequest, "invalid_package", "请选择有效套餐")
		return
	}
	openid := strings.TrimSpace(user.WechatOpenID)
	if openid == "" {
		writeError(c, http.StatusConflict, "wechat_openid_required", "请先完成微信登录绑定")
		return
	}
	var pkg Package
	if err := a.db.Where("id = ? AND is_active = ?", req.PackageID, true).First(&pkg).Error; err != nil {
		writeError(c, http.StatusNotFound, "package_not_found", "套餐不存在或已下架")
		return
	}
	now := time.Now().UTC()
	if err := a.expireStalePendingFinanceOrders(now); err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_order_create_failed", "微信支付订单创建失败")
		return
	}
	if !a.enforcePaymentOrderRateLimit(c, user.ID, now) {
		return
	}
	order := FinanceOrder{
		OrderNumber:      nextFinanceOrderNumber(now),
		UserID:           user.ID,
		PackageID:        pkg.ID,
		PackageName:      pkg.Name,
		PackageCredits:   pkg.Credits,
		AmountCents:      pkg.PriceCents,
		OrderType:        FinanceOrderTypePackage,
		PaymentMethod:    FinancePaymentMethodWechatJSAPI,
		PaymentStatus:    FinancePaymentStatusPending,
		InvoiceStatus:    FinanceInvoiceStatusPending,
		IPAddress:        sourceIPAddress(c.Request),
		WechatOpenID:     openid,
		TransactionURL:   strings.TrimRight(a.cfg.AppBaseURL, "/") + "/pricing",
		PaymentRequestAt: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		_, err := ensurePaymentRecordForFinanceOrder(tx, order, now)
		return err
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_order_create_failed", "微信支付订单创建失败")
		return
	}
	params, prepayID, err := a.wechatPayClient.CreateJSAPIOrder(order, openid)
	if err != nil {
		_ = a.db.Transaction(func(tx *gorm.DB) error {
			return markPaymentRecordError(tx, order, "wechat_pay_request_failed", safePaymentError(err), "", now, "pay_request")
		})
		writeError(c, http.StatusBadGateway, "wechat_pay_request_failed", "微信支付请求失败")
		return
	}
	summary := fmt.Sprintf("method=wechat.transactions.jsapi,out_trade_no=%s,prepay_id=%s,total=%d", order.OrderNumber, prepayID, order.AmountCents)
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		return markPaymentRecordRequested(tx, order, now, summary)
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_order_create_failed", "微信支付订单创建失败")
		return
	}
	writeJSON(c, http.StatusCreated, gin.H{"order": order, "payment_params": params})
}

func (a *App) handleCreateWechatVirtualPayOrder(c *gin.Context) {
	user := currentUser(c)
	if !a.requireBoundPhoneForPayment(c, user) {
		return
	}
	if !wechatVirtualPayConfigured(a.cfg) {
		writeError(c, http.StatusServiceUnavailable, "wechat_virtual_pay_not_configured", "微信虚拟支付暂未配置")
		return
	}
	var req struct {
		PackageID        uint   `json:"package_id"`
		Code             string `json:"code"`
		ForceNew         bool   `json:"force_new"`
		StaleOrderNumber string `json:"stale_order_number"`
		StaleReason      string `json:"stale_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PackageID == 0 || strings.TrimSpace(req.Code) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "请选择有效套餐并重新登录微信")
		return
	}
	session, err := a.wechatSessionExchanger.Exchange(strings.TrimSpace(req.Code))
	if err != nil || strings.TrimSpace(session.OpenID) == "" || strings.TrimSpace(session.SessionKey) == "" {
		writeError(c, http.StatusBadGateway, "wechat_login_failed", "微信登录失败")
		return
	}
	openid := strings.TrimSpace(session.OpenID)
	if strings.TrimSpace(user.WechatOpenID) != "" && user.WechatOpenID != openid {
		writeError(c, http.StatusConflict, "wechat_openid_mismatch", "当前账号绑定的微信与支付微信不一致")
		return
	}
	if strings.TrimSpace(user.WechatOpenID) == "" {
		if err := a.db.Model(user).Update("wechat_open_id", openid).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "wechat_bind_failed", "微信绑定失败")
			return
		}
		user.WechatOpenID = openid
	}

	var pkg Package
	if err := a.db.Where("id = ? AND is_active = ?", req.PackageID, true).First(&pkg).Error; err != nil {
		writeError(c, http.StatusNotFound, "package_not_found", "套餐不存在或已下架")
		return
	}
	productID := strings.TrimSpace(pkg.WechatVirtualProductID)
	if productID == "" {
		writeError(c, http.StatusBadRequest, "wechat_virtual_product_required", "套餐未配置微信虚拟支付道具 ID")
		return
	}

	now := time.Now().UTC()
	if err := a.expireStalePendingFinanceOrders(now); err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_virtual_order_create_failed", "微信虚拟支付订单创建失败")
		return
	}

	recoveredFromOrderNumber := ""
	if req.ForceNew {
		staleOrderNumber := strings.TrimSpace(req.StaleOrderNumber)
		if staleOrderNumber != "" {
			staleOrder, ok, err := a.findWechatVirtualOrderForRefresh(user.ID, pkg.ID, staleOrderNumber)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "wechat_virtual_order_create_failed", "微信虚拟支付订单创建失败")
				return
			}
			if ok {
				switch staleOrder.PaymentStatus {
				case FinancePaymentStatusPaid:
					writeJSON(c, http.StatusOK, gin.H{
						"order":             staleOrder,
						"payment_state":     wechatVirtualPaymentStateAlreadyPaid,
						"available_credits": a.currentAvailableCredits(staleOrder.UserID),
					})
					return
				case FinancePaymentStatusPending:
					syncResult, err := a.syncWechatVirtualPendingOrder(staleOrder, now)
					if err != nil {
						a.writeWechatVirtualCreateSyncError(c, err)
						return
					}
					switch syncResult.Action {
					case wechatVirtualPendingSyncAlreadyPaid:
						writeJSON(c, http.StatusOK, gin.H{
							"order":             syncResult.Order,
							"payment_state":     wechatVirtualPaymentStateAlreadyPaid,
							"available_credits": syncResult.AvailableCredits,
						})
						return
					case wechatVirtualPendingSyncReplace:
						recoveredFromOrderNumber = staleOrder.OrderNumber
					case wechatVirtualPendingSyncReuse:
						if isWechatVirtualOrderClosedReason(req.StaleReason) {
							summary := summarizeWechatVirtualQuery(syncResult.Result) + ",client_reason=" + sanitizeWechatLogText(req.StaleReason)
							if err := a.closeWechatVirtualOrderForReplacement(staleOrder, now, summary); err != nil {
								writeError(c, http.StatusInternalServerError, "wechat_virtual_order_create_failed", "微信虚拟支付订单创建失败")
								return
							}
							recoveredFromOrderNumber = staleOrder.OrderNumber
						}
					}
				}
			}
		}
	}

	if !req.ForceNew {
		if reusable, ok, err := a.reusablePendingFinanceOrder(user.ID, pkg.ID, FinancePaymentMethodWechatVirtualGoods, now); err != nil {
			writeError(c, http.StatusInternalServerError, "wechat_virtual_order_create_failed", "微信虚拟支付订单创建失败")
			return
		} else if ok {
			syncResult, err := a.syncWechatVirtualPendingOrder(reusable, now)
			if err != nil {
				a.writeWechatVirtualCreateSyncError(c, err)
				return
			}
			switch syncResult.Action {
			case wechatVirtualPendingSyncReuse:
				params, err := a.buildWechatVirtualPaymentParams(reusable, strings.TrimSpace(session.SessionKey))
				if err != nil {
					writeError(c, http.StatusInternalServerError, "wechat_virtual_sign_failed", "微信虚拟支付签名失败")
					return
				}
				writeJSON(c, http.StatusOK, gin.H{
					"order":          reusable,
					"payment_params": params,
					"payment_state":  wechatVirtualPaymentStatePayRequired,
				})
				return
			case wechatVirtualPendingSyncAlreadyPaid:
				writeJSON(c, http.StatusOK, gin.H{
					"order":             syncResult.Order,
					"payment_state":     wechatVirtualPaymentStateAlreadyPaid,
					"available_credits": syncResult.AvailableCredits,
				})
				return
			case wechatVirtualPendingSyncReplace:
				recoveredFromOrderNumber = reusable.OrderNumber
			}
		}
	}

	if !a.enforcePaymentOrderRateLimit(c, user.ID, now) {
		return
	}
	order := FinanceOrder{
		OrderNumber:            nextFinanceOrderNumber(now),
		UserID:                 user.ID,
		PackageID:              pkg.ID,
		PackageName:            pkg.Name,
		PackageCredits:         pkg.Credits,
		AmountCents:            pkg.PriceCents,
		OrderType:              FinanceOrderTypePackage,
		PaymentMethod:          FinancePaymentMethodWechatVirtualGoods,
		PaymentStatus:          FinancePaymentStatusPending,
		InvoiceStatus:          FinanceInvoiceStatusPending,
		IPAddress:              sourceIPAddress(c.Request),
		WechatOpenID:           openid,
		WechatVirtualProductID: productID,
		TransactionURL:         strings.TrimRight(a.cfg.AppBaseURL, "/") + "/pricing",
		PaymentRequestAt:       &now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		_, err := ensurePaymentRecordForFinanceOrder(tx, order, now)
		return err
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_virtual_order_create_failed", "微信虚拟支付订单创建失败")
		return
	}

	params, err := a.buildWechatVirtualPaymentParams(order, strings.TrimSpace(session.SessionKey))
	if err != nil {
		_ = a.db.Transaction(func(tx *gorm.DB) error {
			return markPaymentRecordError(tx, order, "wechat_virtual_sign_failed", safePaymentError(err), "", now, "pay_request")
		})
		writeError(c, http.StatusInternalServerError, "wechat_virtual_sign_failed", "微信虚拟支付签名失败")
		return
	}
	summary := fmt.Sprintf("method=wechat.requestVirtualPayment,mode=%s,out_trade_no=%s,product_id=%s,total=%d", params.Mode, order.OrderNumber, productID, order.AmountCents)
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		return markPaymentRecordRequested(tx, order, now, summary)
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "wechat_virtual_order_create_failed", "微信虚拟支付订单创建失败")
		return
	}
	response := gin.H{
		"order":          order,
		"payment_params": params,
		"payment_state":  wechatVirtualPaymentStatePayRequired,
	}
	if recoveredFromOrderNumber != "" {
		response["recovered_from_order_number"] = recoveredFromOrderNumber
	}
	writeJSON(c, http.StatusCreated, response)
}

func (a *App) findWechatVirtualOrderForRefresh(userID, packageID uint, orderNumber string) (FinanceOrder, bool, error) {
	var order FinanceOrder
	result := a.db.Where(
		"order_number = ? AND user_id = ? AND package_id = ? AND payment_method = ?",
		strings.TrimSpace(orderNumber),
		userID,
		packageID,
		FinancePaymentMethodWechatVirtualGoods,
	).Limit(1).Find(&order)
	if result.Error != nil {
		return FinanceOrder{}, false, result.Error
	}
	return order, result.RowsAffected > 0, nil
}

func (a *App) syncWechatVirtualPendingOrder(order FinanceOrder, now time.Time) (wechatVirtualPendingSyncResult, error) {
	result, err := a.wechatVirtualPayClient.QueryOrder(order)
	if err != nil {
		_ = a.markWechatVirtualQueryReceived(order, wechatVirtualPayQueryResult{}, now, "wechat_virtual_query_failed", safePaymentError(err))
		return wechatVirtualPendingSyncResult{}, err
	}
	if strings.TrimSpace(result.OrderID) != "" && strings.TrimSpace(result.OrderID) != order.OrderNumber {
		_ = a.markWechatVirtualQueryReceived(order, result, now, "wechat_virtual_query_mismatch", "order_id mismatch")
		return wechatVirtualPendingSyncResult{}, fmt.Errorf("wechat virtual query order mismatch")
	}
	if strings.TrimSpace(result.OpenID) != "" && strings.TrimSpace(order.WechatOpenID) != "" && strings.TrimSpace(result.OpenID) != strings.TrimSpace(order.WechatOpenID) {
		_ = a.markWechatVirtualQueryReceived(order, result, now, "wechat_openid_mismatch", "openid mismatch")
		return wechatVirtualPendingSyncResult{}, errWechatOpenIDMismatch
	}
	if isWechatVirtualOrderPending(result.Status) {
		_ = a.markWechatVirtualQueryReceived(order, result, now, "", "")
		return wechatVirtualPendingSyncResult{Action: wechatVirtualPendingSyncReuse, Order: order, Result: result}, nil
	}
	if code, message, failed := wechatVirtualTerminalError(result.Status); failed {
		if result.Status == wechatVirtualOrderStatusClosed || result.Status == wechatVirtualOrderStatusRefunded || result.Status == wechatVirtualOrderStatusUserRefunded {
			if err := a.closeWechatVirtualOrderForReplacement(order, now, summarizeWechatVirtualQuery(result)); err != nil {
				return wechatVirtualPendingSyncResult{}, err
			}
			return wechatVirtualPendingSyncResult{Action: wechatVirtualPendingSyncReplace, Order: order, Result: result}, nil
		}
		_ = a.markWechatVirtualQueryReceived(order, result, now, code, message)
		return wechatVirtualPendingSyncResult{}, fmt.Errorf("wechat virtual terminal status %d", result.Status)
	}
	if !isWechatVirtualOrderPaid(result.Status) {
		_ = a.markWechatVirtualQueryReceived(order, result, now, "wechat_virtual_invalid_status", "微信虚拟支付订单状态不可到账")
		return wechatVirtualPendingSyncResult{}, errWechatInvalidStatus
	}

	paidAt := now
	if result.PaidTime > 0 {
		paidAt = time.Unix(result.PaidTime, 0).UTC()
	}
	paidOrder, available, err := a.completeWechatVirtualOrder(order.OrderNumber, result, paidAt, summarizeWechatVirtualQuery(result))
	if err != nil {
		if errors.Is(err, errWechatAmountMismatch) {
			_ = a.markWechatVirtualQueryReceived(order, result, now, "wechat_amount_mismatch", "微信虚拟支付订单金额不一致")
		}
		if errors.Is(err, errWechatOpenIDMismatch) {
			_ = a.markWechatVirtualQueryReceived(order, result, now, "wechat_openid_mismatch", "openid mismatch")
		}
		return wechatVirtualPendingSyncResult{}, err
	}
	if result.Status == wechatVirtualOrderStatusPaidPendingDelivery {
		if err := a.wechatVirtualPayClient.NotifyProvideGoods(paidOrder, result); err != nil {
			_ = a.markWechatVirtualNotifyReceived(paidOrder, result, time.Now().UTC(), "wechat_virtual_notify_failed", "notify_provide_goods_failed: "+safePaymentError(err))
		} else {
			_ = a.markWechatVirtualNotifyReceived(paidOrder, result, time.Now().UTC(), "", "")
		}
	}
	return wechatVirtualPendingSyncResult{Action: wechatVirtualPendingSyncAlreadyPaid, Order: paidOrder, AvailableCredits: available, Result: result}, nil
}

func (a *App) closeWechatVirtualOrderForReplacement(order FinanceOrder, closedAt time.Time, summary string) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&FinanceOrder{}).Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).Updates(map[string]any{
			"payment_status": FinancePaymentStatusFailed,
			"updated_at":     closedAt,
		}).Error; err != nil {
			return err
		}
		return markPaymentRecordClosed(tx, order, closedAt, summary)
	})
}

func (a *App) writeWechatVirtualCreateSyncError(c *gin.Context, err error) {
	if errors.Is(err, errWechatAmountMismatch) {
		writeError(c, http.StatusBadGateway, "wechat_amount_mismatch", "微信虚拟支付订单金额不一致")
		return
	}
	if errors.Is(err, errWechatOpenIDMismatch) {
		writeError(c, http.StatusBadGateway, "wechat_openid_mismatch", "微信订单支付账号不匹配")
		return
	}
	if errors.Is(err, errWechatInvalidStatus) {
		writeError(c, http.StatusBadGateway, "wechat_virtual_invalid_status", "微信虚拟支付订单状态不可到账")
		return
	}
	writeError(c, http.StatusBadGateway, "wechat_virtual_query_failed", "微信虚拟支付订单查询失败")
}

func (a *App) handleQueryWechatPayOrder(c *gin.Context) {
	order, ok := a.findOwnedWechatOrder(c)
	if !ok {
		return
	}
	if order.PaymentStatus == FinancePaymentStatusPaid {
		writeJSON(c, http.StatusOK, gin.H{"order": order, "available_credits": a.currentAvailableCredits(order.UserID)})
		return
	}
	if !wechatPayConfigured(a.cfg) {
		writeError(c, http.StatusServiceUnavailable, "wechat_pay_not_configured", "微信支付暂未配置")
		return
	}
	result, err := a.wechatPayClient.QueryOrder(order.OrderNumber)
	if err != nil {
		_ = a.markWechatQueryReceived(order, wechatPayQueryResult{}, time.Now().UTC(), "wechat_query_failed", safePaymentError(err))
		writeError(c, http.StatusBadGateway, "wechat_query_failed", "微信支付订单查询失败")
		return
	}
	if result.OutTradeNo != "" && result.OutTradeNo != order.OrderNumber {
		_ = a.markWechatQueryReceived(order, result, time.Now().UTC(), "wechat_query_mismatch", "out_trade_no mismatch")
		writeError(c, http.StatusBadGateway, "wechat_query_mismatch", "微信支付订单查询结果不匹配")
		return
	}
	if result.TradeState != "SUCCESS" {
		queryAt := time.Now().UTC()
		_ = a.markWechatQueryReceived(order, result, queryAt, "", "")
		if result.TradeState == "CLOSED" || result.TradeState == "REVOKED" || result.TradeState == "PAYERROR" {
			_ = a.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&FinanceOrder{}).Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).Update("payment_status", FinancePaymentStatusFailed).Error; err != nil {
					return err
				}
				return markPaymentRecordClosed(tx, order, queryAt, summarizeWechatQuery(result))
			})
			order.PaymentStatus = FinancePaymentStatusFailed
		}
		writeJSON(c, http.StatusOK, gin.H{"order": order, "available_credits": a.currentAvailableCredits(order.UserID)})
		return
	}
	paidAt := result.SuccessTime
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	paidOrder, available, err := a.completeWechatOrder(order.OrderNumber, result, paidAt, "active_query")
	if err != nil {
		if errors.Is(err, errWechatAmountMismatch) {
			_ = a.markWechatQueryReceived(order, result, time.Now().UTC(), "wechat_amount_mismatch", "微信支付订单金额不一致")
			writeError(c, http.StatusBadGateway, "wechat_amount_mismatch", "微信支付订单金额不一致")
			return
		}
		writeError(c, http.StatusInternalServerError, "wechat_credit_failed", "微信支付到账处理失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"order": paidOrder, "available_credits": available})
}

func (a *App) handleConfirmWechatVirtualPayOrder(c *gin.Context) {
	order, ok := a.findOwnedWechatVirtualOrder(c)
	if !ok {
		return
	}
	if order.PaymentStatus == FinancePaymentStatusPaid {
		writeJSON(c, http.StatusOK, gin.H{"order": order, "available_credits": a.currentAvailableCredits(order.UserID)})
		return
	}
	if !wechatVirtualPayConfigured(a.cfg) {
		writeError(c, http.StatusServiceUnavailable, "wechat_virtual_pay_not_configured", "微信虚拟支付暂未配置")
		return
	}
	result, err := a.wechatVirtualPayClient.QueryOrder(order)
	queryAt := time.Now().UTC()
	if err != nil {
		_ = a.markWechatVirtualQueryReceived(order, wechatVirtualPayQueryResult{}, queryAt, "wechat_virtual_query_failed", safePaymentError(err))
		writeError(c, http.StatusBadGateway, "wechat_virtual_query_failed", "微信虚拟支付订单查询失败")
		return
	}
	if result.OrderID != "" && result.OrderID != order.OrderNumber {
		_ = a.markWechatVirtualQueryReceived(order, result, queryAt, "wechat_virtual_query_mismatch", "order_id mismatch")
		writeError(c, http.StatusBadGateway, "wechat_virtual_query_mismatch", "微信虚拟支付订单查询结果不匹配")
		return
	}
	if result.OpenID != "" && order.WechatOpenID != "" && result.OpenID != order.WechatOpenID {
		_ = a.markWechatVirtualQueryReceived(order, result, queryAt, "wechat_openid_mismatch", "openid mismatch")
		writeError(c, http.StatusBadGateway, "wechat_openid_mismatch", "微信虚拟支付订单 openid 不一致")
		return
	}
	if isWechatVirtualOrderPending(result.Status) {
		_ = a.markWechatVirtualQueryReceived(order, result, queryAt, "", "")
		writeJSON(c, http.StatusOK, gin.H{
			"code":              "wechat_virtual_pay_pending",
			"message":           "支付处理中，请稍后刷新",
			"order":             order,
			"available_credits": a.currentAvailableCredits(order.UserID),
		})
		return
	}
	if code, message, failed := wechatVirtualTerminalError(result.Status); failed {
		_ = a.markWechatVirtualQueryReceived(order, result, queryAt, code, message)
		if result.Status == wechatVirtualOrderStatusClosed || result.Status == wechatVirtualOrderStatusRefunded || result.Status == wechatVirtualOrderStatusUserRefunded {
			_ = a.db.Model(&FinanceOrder{}).Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).Update("payment_status", FinancePaymentStatusFailed).Error
		}
		writeError(c, http.StatusConflict, code, message)
		return
	}
	if !isWechatVirtualOrderPaid(result.Status) {
		_ = a.markWechatVirtualQueryReceived(order, result, queryAt, "wechat_virtual_invalid_status", "微信虚拟支付订单状态不可到账")
		writeError(c, http.StatusBadGateway, "wechat_virtual_invalid_status", "微信虚拟支付订单状态不可到账")
		return
	}
	paidAt := queryAt
	if result.PaidTime > 0 {
		paidAt = time.Unix(result.PaidTime, 0).UTC()
	}
	paidOrder, available, err := a.completeWechatVirtualOrder(order.OrderNumber, result, paidAt, summarizeWechatVirtualQuery(result))
	if err != nil {
		if errors.Is(err, errWechatAmountMismatch) {
			_ = a.markWechatVirtualQueryReceived(order, result, queryAt, "wechat_amount_mismatch", "微信虚拟支付订单金额不一致")
			writeError(c, http.StatusBadGateway, "wechat_amount_mismatch", "微信虚拟支付订单金额不一致")
			return
		}
		if errors.Is(err, errWechatOpenIDMismatch) {
			_ = a.markWechatVirtualQueryReceived(order, result, queryAt, "wechat_openid_mismatch", "openid mismatch")
			writeError(c, http.StatusBadGateway, "wechat_openid_mismatch", "微信虚拟支付订单 openid 不一致")
			return
		}
		writeError(c, http.StatusInternalServerError, "wechat_virtual_credit_failed", "微信虚拟支付到账处理失败")
		return
	}
	if result.Status == wechatVirtualOrderStatusPaidPendingDelivery {
		if err := a.wechatVirtualPayClient.NotifyProvideGoods(paidOrder, result); err != nil {
			_ = a.markWechatVirtualNotifyReceived(paidOrder, result, time.Now().UTC(), "wechat_virtual_notify_failed", "notify_provide_goods_failed: "+safePaymentError(err))
		} else {
			_ = a.markWechatVirtualNotifyReceived(paidOrder, result, time.Now().UTC(), "", "")
		}
	}
	writeJSON(c, http.StatusOK, gin.H{"order": paidOrder, "available_credits": available})
}

func (a *App) handleWechatPayNotify(c *gin.Context) {
	if !wechatPayConfigured(a.cfg) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "FAIL", "message": "wechat pay not configured"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "invalid body"})
		return
	}
	if err := a.verifyWechatNotifySignature(c.Request, body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "invalid signature"})
		return
	}
	result, err := a.parseWechatNotify(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "invalid notify"})
		return
	}
	if result.TradeState != "SUCCESS" {
		var order FinanceOrder
		if err := a.db.Where("order_number = ? AND payment_method = ?", result.OutTradeNo, FinancePaymentMethodWechatJSAPI).First(&order).Error; err == nil {
			_ = a.markWechatNotifyReceived(order, result, time.Now().UTC(), "", "")
		}
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
		return
	}
	paidAt := result.SuccessTime
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	if _, _, err := a.completeWechatOrder(result.OutTradeNo, result, paidAt, summarizeWechatQuery(result)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "process failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
}

func (a *App) findOwnedWechatOrder(c *gin.Context) (FinanceOrder, bool) {
	user := currentUser(c)
	var order FinanceOrder
	if err := a.db.Where("order_number = ? AND user_id = ? AND payment_method = ?", c.Param("order_number"), user.ID, FinancePaymentMethodWechatJSAPI).First(&order).Error; err != nil {
		writeError(c, http.StatusNotFound, "wechat_order_not_found", "微信支付订单不存在")
		return FinanceOrder{}, false
	}
	return order, true
}

func (a *App) findOwnedWechatVirtualOrder(c *gin.Context) (FinanceOrder, bool) {
	user := currentUser(c)
	var order FinanceOrder
	if err := a.db.Where("order_number = ? AND user_id = ? AND payment_method = ?", c.Param("order_number"), user.ID, FinancePaymentMethodWechatVirtualGoods).First(&order).Error; err != nil {
		writeError(c, http.StatusNotFound, "wechat_virtual_order_not_found", "微信虚拟支付订单不存在")
		return FinanceOrder{}, false
	}
	return order, true
}

func (a *App) completeWechatOrder(orderNumber string, result wechatPayQueryResult, paidAt time.Time, rawSummary string) (FinanceOrder, int, error) {
	var paidOrder FinanceOrder
	availableCredits := 0
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var order FinanceOrder
		if err := tx.Where("order_number = ? AND payment_method = ?", orderNumber, FinancePaymentMethodWechatJSAPI).First(&order).Error; err != nil {
			return err
		}
		if result.AmountCents != order.AmountCents {
			_ = markPaymentRecordError(tx, order, "wechat_amount_mismatch", "微信支付订单金额不一致", rawSummary, paidAt, "notify")
			return errWechatAmountMismatch
		}
		if result.AppID != "" && result.AppID != a.cfg.WechatPayAppID {
			return errors.New("wechat app_id mismatch")
		}
		if result.MchID != "" && result.MchID != a.cfg.WechatPayMchID {
			return errors.New("wechat mch_id mismatch")
		}
		if result.PayerOpenID != "" && order.WechatOpenID != "" && result.PayerOpenID != order.WechatOpenID {
			return errors.New("wechat openid mismatch")
		}
		if err := markWechatPaymentRecordPaid(tx, order, result, paidAt, rawSummary); err != nil {
			return err
		}
		if order.PaymentStatus == FinancePaymentStatusPaid {
			paidOrder = order
			availableCredits = a.currentAvailableCreditsTx(tx, order.UserID)
			return nil
		}
		if result.TradeState != "SUCCESS" {
			return errWechatInvalidStatus
		}
		// 原子 compare-and-set：仅当订单仍处于 pending 时才抢占到账，避免并发回调重复发放点数。
		settle := tx.Model(&FinanceOrder{}).
			Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).
			Update("payment_status", FinancePaymentStatusPaid)
		if settle.Error != nil {
			return settle.Error
		}
		if settle.RowsAffected == 0 {
			if err := tx.Where("id = ?", order.ID).First(&order).Error; err != nil {
				return err
			}
			paidOrder = order
			availableCredits = a.currentAvailableCreditsTx(tx, order.UserID)
			return nil
		}
		order.PaymentStatus = FinancePaymentStatusPaid
		order.PaidAt = &paidAt
		order.WechatTransactionID = result.TransactionID
		order.WechatOpenID = fallbackString(result.PayerOpenID, order.WechatOpenID)
		order.WechatNotifyAt = &paidAt
		order.RawNotificationSummary = rawSummary
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		var balance CreditBalance
		if err := tx.Where("user_id = ?", order.UserID).FirstOrCreate(&balance, CreditBalance{UserID: order.UserID}).Error; err != nil {
			return err
		}
		var existingTopUp CreditTransaction
		err := tx.Where("type = ? AND related_type = ? AND related_id = ?", CreditTransactionTypePaymentTopUp, "finance_order", order.ID).First(&existingTopUp).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			balance.AvailableCredits += order.PackageCredits
			availableCredits = balance.AvailableCredits
			if err := tx.Save(&balance).Error; err != nil {
				return err
			}
			transaction := CreditTransaction{
				UserID:       order.UserID,
				Type:         CreditTransactionTypePaymentTopUp,
				Amount:       order.PackageCredits,
				BalanceAfter: balance.AvailableCredits,
				Reason:       "微信支付到账",
				RelatedType:  "finance_order",
				RelatedID:    order.ID,
			}
			if err := tx.Create(&transaction).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			availableCredits = balance.AvailableCredits
		}
		paidOrder = order
		return nil
	})
	return paidOrder, availableCredits, err
}

func (a *App) completeWechatVirtualOrder(orderNumber string, result wechatVirtualPayQueryResult, paidAt time.Time, rawSummary string) (FinanceOrder, int, error) {
	var paidOrder FinanceOrder
	availableCredits := 0
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var order FinanceOrder
		if err := tx.Where("order_number = ? AND payment_method = ?", orderNumber, FinancePaymentMethodWechatVirtualGoods).First(&order).Error; err != nil {
			return err
		}
		if result.OrderFee != order.AmountCents || result.PaidFee != order.AmountCents {
			_ = markPaymentRecordError(tx, order, "wechat_amount_mismatch", "微信虚拟支付订单金额不一致", rawSummary, paidAt, "confirm")
			return errWechatAmountMismatch
		}
		if result.OpenID != "" && order.WechatOpenID != "" && result.OpenID != order.WechatOpenID {
			return errWechatOpenIDMismatch
		}
		if err := markWechatVirtualPaymentRecordPaid(tx, order, result, paidAt, rawSummary); err != nil {
			return err
		}
		if order.PaymentStatus == FinancePaymentStatusPaid {
			paidOrder = order
			availableCredits = a.currentAvailableCreditsTx(tx, order.UserID)
			return nil
		}
		if !isWechatVirtualOrderPaid(result.Status) {
			return errWechatInvalidStatus
		}
		// 原子 compare-and-set：仅当订单仍处于 pending 时才抢占到账，避免并发回调重复发放点数。
		settle := tx.Model(&FinanceOrder{}).
			Where("id = ? AND payment_status = ?", order.ID, FinancePaymentStatusPending).
			Update("payment_status", FinancePaymentStatusPaid)
		if settle.Error != nil {
			return settle.Error
		}
		if settle.RowsAffected == 0 {
			if err := tx.Where("id = ?", order.ID).First(&order).Error; err != nil {
				return err
			}
			paidOrder = order
			availableCredits = a.currentAvailableCreditsTx(tx, order.UserID)
			return nil
		}
		order.PaymentStatus = FinancePaymentStatusPaid
		order.PaidAt = &paidAt
		order.WechatTransactionID = wechatVirtualTradeNo(result, order)
		order.WechatOpenID = fallbackString(result.OpenID, order.WechatOpenID)
		order.WechatNotifyAt = &paidAt
		order.RawNotificationSummary = rawSummary
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		var balance CreditBalance
		if err := tx.Where("user_id = ?", order.UserID).FirstOrCreate(&balance, CreditBalance{UserID: order.UserID}).Error; err != nil {
			return err
		}
		var existingTopUp CreditTransaction
		err := tx.Where("type = ? AND related_type = ? AND related_id = ?", CreditTransactionTypePaymentTopUp, "finance_order", order.ID).First(&existingTopUp).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			balance.AvailableCredits += order.PackageCredits
			availableCredits = balance.AvailableCredits
			if err := tx.Save(&balance).Error; err != nil {
				return err
			}
			transaction := CreditTransaction{
				UserID:       order.UserID,
				Type:         CreditTransactionTypePaymentTopUp,
				Amount:       order.PackageCredits,
				BalanceAfter: balance.AvailableCredits,
				Reason:       "微信虚拟支付到账",
				RelatedType:  "finance_order",
				RelatedID:    order.ID,
			}
			if err := tx.Create(&transaction).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			availableCredits = balance.AvailableCredits
		}
		paidOrder = order
		return nil
	})
	return paidOrder, availableCredits, err
}

func markWechatPaymentRecordPaid(tx *gorm.DB, order FinanceOrder, result wechatPayQueryResult, paidAt time.Time, summary string) error {
	payment, err := ensurePaymentRecordForFinanceOrder(tx, order, paidAt)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status":             PaymentRecordStatusPaid,
		"provider_trade_no":  result.TransactionID,
		"buyer_id":           result.PayerOpenID,
		"paid_at":            &paidAt,
		"last_error_code":    "",
		"last_error_message": "",
	}
	if summary == "active_query" {
		updates["query_count"] = gorm.Expr("query_count + ?", 1)
		updates["queried_at"] = &paidAt
		updates["query_summary"] = summarizeWechatQuery(result)
		updates["last_event"] = "active_query"
	} else {
		updates["notify_count"] = gorm.Expr("notify_count + ?", 1)
		updates["notified_at"] = &paidAt
		updates["notify_summary"] = summary
		updates["last_event"] = "notify"
	}
	return tx.Model(&payment).Updates(updates).Error
}

func markWechatVirtualPaymentRecordPaid(tx *gorm.DB, order FinanceOrder, result wechatVirtualPayQueryResult, paidAt time.Time, summary string) error {
	payment, err := ensurePaymentRecordForFinanceOrder(tx, order, paidAt)
	if err != nil {
		return err
	}
	return tx.Model(&payment).Updates(map[string]any{
		"status":             PaymentRecordStatusPaid,
		"provider_trade_no":  wechatVirtualTradeNo(result, order),
		"buyer_id":           fallbackString(result.OpenID, order.WechatOpenID),
		"paid_at":            &paidAt,
		"query_count":        gorm.Expr("query_count + ?", 1),
		"queried_at":         &paidAt,
		"query_summary":      summary,
		"last_event":         "virtual_confirm",
		"last_error_code":    "",
		"last_error_message": "",
	}).Error
}

func (a *App) markWechatVirtualQueryReceived(order FinanceOrder, result wechatVirtualPayQueryResult, queriedAt time.Time, errorCode, errorMessage string) error {
	summary := summarizeWechatVirtualQuery(result)
	return a.db.Transaction(func(tx *gorm.DB) error {
		if errorCode != "" {
			return markPaymentRecordError(tx, order, errorCode, errorMessage, summary, queriedAt, "query")
		}
		payment, err := ensurePaymentRecordForFinanceOrder(tx, order, queriedAt)
		if err != nil {
			return err
		}
		return tx.Model(&payment).Updates(map[string]any{
			"query_count":       gorm.Expr("query_count + ?", 1),
			"queried_at":        &queriedAt,
			"provider_trade_no": wechatVirtualTradeNo(result, order),
			"buyer_id":          fallbackString(result.OpenID, order.WechatOpenID),
			"last_event":        "virtual_confirm",
			"query_summary":     summary,
		}).Error
	})
}

func (a *App) markWechatVirtualNotifyReceived(order FinanceOrder, result wechatVirtualPayQueryResult, notifiedAt time.Time, errorCode, errorMessage string) error {
	summary := summarizeWechatVirtualNotify(result)
	if errorCode != "" {
		summary = summary + ",error=" + errorMessage
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		if errorCode != "" {
			return markPaymentRecordError(tx, order, errorCode, errorMessage, summary, notifiedAt, "notify")
		}
		payment, err := ensurePaymentRecordForFinanceOrder(tx, order, notifiedAt)
		if err != nil {
			return err
		}
		return tx.Model(&payment).Updates(map[string]any{
			"notify_count":      gorm.Expr("notify_count + ?", 1),
			"notified_at":       &notifiedAt,
			"provider_trade_no": wechatVirtualTradeNo(result, order),
			"buyer_id":          fallbackString(result.OpenID, order.WechatOpenID),
			"last_event":        "virtual_notify_provide_goods",
			"notify_summary":    summary,
		}).Error
	})
}

func (a *App) markWechatQueryReceived(order FinanceOrder, result wechatPayQueryResult, queriedAt time.Time, errorCode, errorMessage string) error {
	summary := summarizeWechatQuery(result)
	return a.db.Transaction(func(tx *gorm.DB) error {
		if errorCode != "" {
			return markPaymentRecordError(tx, order, errorCode, errorMessage, summary, queriedAt, "query")
		}
		payment, err := ensurePaymentRecordForFinanceOrder(tx, order, queriedAt)
		if err != nil {
			return err
		}
		return tx.Model(&payment).Updates(map[string]any{
			"query_count":       gorm.Expr("query_count + ?", 1),
			"queried_at":        &queriedAt,
			"provider_trade_no": result.TransactionID,
			"buyer_id":          result.PayerOpenID,
			"last_event":        "active_query",
			"query_summary":     summary,
		}).Error
	})
}

func (a *App) markWechatNotifyReceived(order FinanceOrder, result wechatPayQueryResult, notifiedAt time.Time, errorCode, errorMessage string) error {
	summary := summarizeWechatQuery(result)
	return a.db.Transaction(func(tx *gorm.DB) error {
		if errorCode != "" {
			return markPaymentRecordError(tx, order, errorCode, errorMessage, summary, notifiedAt, "notify")
		}
		payment, err := ensurePaymentRecordForFinanceOrder(tx, order, notifiedAt)
		if err != nil {
			return err
		}
		return tx.Model(&payment).Updates(map[string]any{
			"notify_count":      gorm.Expr("notify_count + ?", 1),
			"notified_at":       &notifiedAt,
			"provider_trade_no": result.TransactionID,
			"buyer_id":          result.PayerOpenID,
			"last_event":        "notify",
			"notify_summary":    summary,
		}).Error
	})
}

func (q httpWechatSessionExchanger) Exchange(code string) (wechatSession, error) {
	if q.app == nil || strings.TrimSpace(q.app.cfg.WechatPayAppID) == "" || strings.TrimSpace(q.app.cfg.WechatAppSecret) == "" {
		return wechatSession{}, errors.New("wechat login is not configured")
	}
	endpoint := "https://api.weixin.qq.com/sns/jscode2session"
	values := url.Values{}
	values.Set("appid", q.app.cfg.WechatPayAppID)
	values.Set("secret", q.app.cfg.WechatAppSecret)
	values.Set("js_code", code)
	values.Set("grant_type", "authorization_code")
	resp, err := http.Get(endpoint + "?" + values.Encode())
	if err != nil {
		return wechatSession{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return wechatSession{}, err
	}
	var payload struct {
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
		UnionID    string `json:"unionid"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return wechatSession{}, err
	}
	if payload.ErrCode != 0 || strings.TrimSpace(payload.OpenID) == "" {
		return wechatSession{}, fmt.Errorf("wechat jscode2session failed: %d %s", payload.ErrCode, payload.ErrMsg)
	}
	return wechatSession{OpenID: payload.OpenID, SessionKey: payload.SessionKey, UnionID: payload.UnionID}, nil
}

func (r *httpWechatPhoneResolver) ResolvePhone(code string) (string, error) {
	if r == nil || r.app == nil || strings.TrimSpace(r.app.cfg.WechatPayAppID) == "" || strings.TrimSpace(r.app.cfg.WechatAppSecret) == "" {
		return "", &wechatPhoneResolveError{apiCode: wechatPhoneCodeTokenFailed, detail: "wechat phone resolver is not configured"}
	}
	accessToken, err := r.accessToken()
	if err != nil {
		return "", err
	}
	phone, err := r.resolvePhoneWithToken(code, accessToken)
	if err == nil {
		return phone, nil
	}
	var phoneErr *wechatPhoneResolveError
	if errors.As(err, &phoneErr) && phoneErr.retryableToken {
		freshToken, tokenErr := r.refreshAccessToken()
		if tokenErr != nil {
			return "", tokenErr
		}
		return r.resolvePhoneWithToken(code, freshToken)
	}
	return "", err
}

func (r *httpWechatPhoneResolver) resolvePhoneWithToken(code, accessToken string) (string, error) {
	body := map[string]string{"code": strings.TrimSpace(code)}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("access_token", accessToken)
	req, err := http.NewRequest(http.MethodPost, "https://api.weixin.qq.com/wxa/business/getuserphonenumber?"+values.Encode(), bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", &wechatPhoneResolveError{
			apiCode: wechatPhoneCodeFailed,
			detail:  "wechat phone request failed error=" + sanitizeWechatLogText(err.Error()),
			cause:   err,
		}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", &wechatPhoneResolveError{
			apiCode:    wechatPhoneCodeFailed,
			httpStatus: resp.StatusCode,
			detail:     fmt.Sprintf("wechat phone response read failed http_status=%d", resp.StatusCode),
			cause:      err,
		}
	}
	var payload struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		RID       string `json:"rid"`
		PhoneInfo struct {
			PhoneNumber     string `json:"phoneNumber"`
			PurePhoneNumber string `json:"purePhoneNumber"`
			CountryCode     string `json:"countryCode"`
		} `json:"phone_info"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", &wechatPhoneResolveError{
			apiCode:    wechatPhoneCodeFailed,
			httpStatus: resp.StatusCode,
			detail:     fmt.Sprintf("wechat phone response decode failed http_status=%d", resp.StatusCode),
			cause:      err,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.ErrCode != 0 || strings.TrimSpace(payload.ErrMsg) != "" || strings.TrimSpace(payload.RID) != "" {
			return "", newWechatPhoneResponseError(resp.StatusCode, payload.ErrCode, payload.ErrMsg, payload.RID)
		}
		return "", &wechatPhoneResolveError{
			apiCode:    wechatPhoneCodeFailed,
			httpStatus: resp.StatusCode,
			detail:     fmt.Sprintf("wechat phone http_status=%d", resp.StatusCode),
		}
	}
	if payload.ErrCode != 0 {
		return "", newWechatPhoneResponseError(resp.StatusCode, payload.ErrCode, payload.ErrMsg, payload.RID)
	}
	phone := normalizeMainlandPhone(payload.PhoneInfo.PurePhoneNumber)
	if phone == "" {
		phone = normalizeMainlandPhone(payload.PhoneInfo.PhoneNumber)
	}
	if !isValidMainlandPhone(phone) {
		return "", &wechatPhoneResolveError{
			apiCode: wechatPhoneCodeFailed,
			detail:  "wechat phone response invalid phone country=" + sanitizeWechatLogText(payload.PhoneInfo.CountryCode),
		}
	}
	return phone, nil
}

func (r *httpWechatPhoneResolver) accessToken() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if r.token != "" && now.Before(r.expiresAt) {
		return r.token, nil
	}
	return r.fetchAccessTokenLocked()
}

func (r *httpWechatPhoneResolver) refreshAccessToken() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fetchAccessTokenLocked()
}

func (r *httpWechatPhoneResolver) fetchAccessTokenLocked() (string, error) {
	token, expiresAt, err := fetchWechatAccessToken(r.app.cfg)
	if err != nil {
		return "", newWechatPhoneTokenError(err)
	}
	r.token = token
	r.expiresAt = expiresAt
	return r.token, nil
}

func fetchWechatAccessToken(cfg Config) (string, time.Time, error) {
	now := time.Now()
	values := url.Values{}
	values.Set("grant_type", "client_credential")
	values.Set("appid", strings.TrimSpace(cfg.WechatPayAppID))
	values.Set("secret", strings.TrimSpace(cfg.WechatAppSecret))
	req, err := http.NewRequest(http.MethodGet, "https://api.weixin.qq.com/cgi-bin/token?"+values.Encode(), nil)
	if err != nil {
		return "", time.Time{}, &wechatAccessTokenError{detail: "wechat access_token request build failed", cause: err}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", time.Time{}, &wechatAccessTokenError{
			detail: "wechat access_token request failed error=" + sanitizeWechatLogText(err.Error()),
			cause:  err,
		}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, &wechatAccessTokenError{
			httpStatus: resp.StatusCode,
			detail:     fmt.Sprintf("wechat access_token response read failed http_status=%d", resp.StatusCode),
			cause:      err,
		}
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		RID         string `json:"rid"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", time.Time{}, &wechatAccessTokenError{
			httpStatus: resp.StatusCode,
			detail:     fmt.Sprintf("wechat access_token response decode failed http_status=%d", resp.StatusCode),
			cause:      err,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, &wechatAccessTokenError{
			errCode:    payload.ErrCode,
			errMsg:     payload.ErrMsg,
			rid:        payload.RID,
			httpStatus: resp.StatusCode,
		}
	}
	if payload.ErrCode != 0 || strings.TrimSpace(payload.AccessToken) == "" {
		return "", time.Time{}, &wechatAccessTokenError{
			errCode:    payload.ErrCode,
			errMsg:     payload.ErrMsg,
			rid:        payload.RID,
			httpStatus: resp.StatusCode,
		}
	}
	ttl := time.Duration(payload.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	refreshSkew := 5 * time.Minute
	if ttl <= refreshSkew {
		refreshSkew = ttl / 10
		if refreshSkew <= 0 {
			refreshSkew = time.Second
		}
	}
	return strings.TrimSpace(payload.AccessToken), now.Add(ttl - refreshSkew), nil
}

func (c httpWechatPayClient) CreateJSAPIOrder(order FinanceOrder, openid string) (wechatPayRequestParams, string, error) {
	if c.app == nil || !wechatPayConfigured(c.app.cfg) {
		return wechatPayRequestParams{}, "", errWechatPayNotConfigured
	}
	body := map[string]any{
		"appid":        c.app.cfg.WechatPayAppID,
		"mchid":        c.app.cfg.WechatPayMchID,
		"description":  fmt.Sprintf("白霖共享 %s %d点", order.PackageName, order.PackageCredits),
		"out_trade_no": order.OrderNumber,
		"notify_url":   c.app.effectiveWechatNotifyURL(),
		"amount":       map[string]any{"total": order.AmountCents, "currency": "CNY"},
		"payer":        map[string]any{"openid": openid},
	}
	var response struct {
		PrepayID string `json:"prepay_id"`
	}
	if err := c.app.doWechatPayJSON(http.MethodPost, "/v3/pay/transactions/jsapi", body, &response); err != nil {
		return wechatPayRequestParams{}, "", err
	}
	if strings.TrimSpace(response.PrepayID) == "" {
		return wechatPayRequestParams{}, "", errors.New("wechat prepay_id missing")
	}
	params, err := c.app.buildWechatRequestPaymentParams(response.PrepayID, time.Now().UTC())
	return params, response.PrepayID, err
}

func (c httpWechatPayClient) QueryOrder(orderNumber string) (wechatPayQueryResult, error) {
	if c.app == nil || !wechatPayConfigured(c.app.cfg) {
		return wechatPayQueryResult{}, errWechatPayNotConfigured
	}
	var payload struct {
		AppID         string `json:"appid"`
		MchID         string `json:"mchid"`
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		SuccessTime   string `json:"success_time"`
		Payer         struct {
			OpenID string `json:"openid"`
		} `json:"payer"`
		Amount struct {
			Total int64 `json:"total"`
		} `json:"amount"`
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(orderNumber) + "?mchid=" + url.QueryEscape(c.app.cfg.WechatPayMchID)
	if err := c.app.doWechatPayJSON(http.MethodGet, path, nil, &payload); err != nil {
		return wechatPayQueryResult{}, err
	}
	successTime, _ := time.Parse(time.RFC3339, payload.SuccessTime)
	return wechatPayQueryResult{
		AppID:         payload.AppID,
		MchID:         payload.MchID,
		OutTradeNo:    payload.OutTradeNo,
		TransactionID: payload.TransactionID,
		TradeState:    payload.TradeState,
		PayerOpenID:   payload.Payer.OpenID,
		AmountCents:   payload.Amount.Total,
		SuccessTime:   successTime,
		RawSummary:    summarizeWechatQuery(wechatPayQueryResult{OutTradeNo: payload.OutTradeNo, TransactionID: payload.TransactionID, TradeState: payload.TradeState, AmountCents: payload.Amount.Total}),
	}, nil
}

func (c *httpWechatVirtualPayClient) QueryOrder(order FinanceOrder) (wechatVirtualPayQueryResult, error) {
	if c == nil || c.app == nil || !wechatVirtualPayConfigured(c.app.cfg) {
		return wechatVirtualPayQueryResult{}, errWechatPayNotConfigured
	}
	body := struct {
		OpenID  string `json:"openid"`
		Env     int    `json:"env"`
		OrderID string `json:"order_id"`
	}{
		OpenID:  order.WechatOpenID,
		Env:     c.app.cfg.WechatVirtualPayEnv,
		OrderID: order.OrderNumber,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return wechatVirtualPayQueryResult{}, err
	}
	var payload struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		OpenID  string `json:"openid"`
		Order   struct {
			OrderID        string `json:"order_id"`
			WXOrderID      string `json:"wx_order_id"`
			WXPayOrderID   string `json:"wxpay_order_id"`
			ChannelOrderID string `json:"channel_order_id"`
			Status         int    `json:"status"`
			OpenID         string `json:"openid"`
			OrderFee       int64  `json:"order_fee"`
			PaidFee        int64  `json:"paid_fee"`
			PaidTime       int64  `json:"paid_time"`
			ProvideTime    int64  `json:"provide_time"`
		} `json:"order"`
	}
	if err := c.doXPayJSON("/xpay/query_order", bodyBytes, true, &payload); err != nil {
		return wechatVirtualPayQueryResult{}, err
	}
	if payload.ErrCode != 0 {
		return wechatVirtualPayQueryResult{}, fmt.Errorf("wechat xpay query_order failed: %d %s", payload.ErrCode, payload.ErrMsg)
	}
	result := wechatVirtualPayQueryResult{
		OrderID:        payload.Order.OrderID,
		WXOrderID:      payload.Order.WXOrderID,
		WXPayOrderID:   payload.Order.WXPayOrderID,
		ChannelOrderID: payload.Order.ChannelOrderID,
		Status:         payload.Order.Status,
		OpenID:         fallbackString(payload.Order.OpenID, payload.OpenID),
		OrderFee:       payload.Order.OrderFee,
		PaidFee:        payload.Order.PaidFee,
		PaidTime:       payload.Order.PaidTime,
		ProvideTime:    payload.Order.ProvideTime,
	}
	result.RawSummary = summarizeWechatVirtualQuery(result)
	return result, nil
}

func (c *httpWechatVirtualPayClient) NotifyProvideGoods(order FinanceOrder, result wechatVirtualPayQueryResult) error {
	if c == nil || c.app == nil || !wechatVirtualPayConfigured(c.app.cfg) {
		return errWechatPayNotConfigured
	}
	body := struct {
		OrderID string `json:"order_id"`
		Env     int    `json:"env"`
	}{
		OrderID: order.OrderNumber,
		Env:     c.app.cfg.WechatVirtualPayEnv,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var payload struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := c.doXPayJSON("/xpay/notify_provide_goods", bodyBytes, false, &payload); err != nil {
		return err
	}
	if payload.ErrCode != 0 {
		return fmt.Errorf("wechat xpay notify_provide_goods failed: %d %s", payload.ErrCode, payload.ErrMsg)
	}
	return nil
}

func (c *httpWechatVirtualPayClient) doXPayJSON(path string, bodyBytes []byte, includePaySig bool, responseBody any) error {
	accessToken, err := c.accessToken()
	if err != nil {
		return err
	}
	values := url.Values{}
	values.Set("access_token", accessToken)
	if includePaySig {
		values.Set("pay_sig", c.app.wechatVirtualXPaySig(path, bodyBytes))
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.weixin.qq.com"+path+"?"+values.Encode(), bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wechat xpay status %d: %s", resp.StatusCode, string(body))
	}
	if len(strings.TrimSpace(string(body))) == 0 || responseBody == nil {
		return nil
	}
	return json.Unmarshal(body, responseBody)
}

func (a *App) wechatVirtualXPaySig(path string, bodyBytes []byte) string {
	return hmacSHA256Hex(a.wechatVirtualPayAppKey(), path+"&"+string(bodyBytes))
}

func (c *httpWechatVirtualPayClient) accessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.token != "" && now.Before(c.expiresAt) {
		return c.token, nil
	}
	token, expiresAt, err := fetchWechatAccessToken(c.app.cfg)
	if err != nil {
		return "", err
	}
	c.token = token
	c.expiresAt = expiresAt
	return c.token, nil
}

func (a *App) doWechatPayJSON(method, path string, requestBody any, responseBody any) error {
	var bodyBytes []byte
	var err error
	if requestBody != nil {
		bodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			return err
		}
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := strings.ReplaceAll(uuid.NewString(), "-", "")
	signature, err := a.signWechatPayMessage(method, path, timestamp, nonce, string(bodyBytes))
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, "https://api.mch.weixin.qq.com"+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		a.cfg.WechatPayMchID, nonce, signature, timestamp, a.cfg.WechatPayMchCertSerialNo))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wechat pay status %d: %s", resp.StatusCode, string(body))
	}
	if responseBody == nil {
		return nil
	}
	return json.Unmarshal(body, responseBody)
}

func (a *App) buildWechatRequestPaymentParams(prepayID string, now time.Time) (wechatPayRequestParams, error) {
	timestamp := fmt.Sprintf("%d", now.Unix())
	nonce := strings.ReplaceAll(uuid.NewString(), "-", "")
	packageValue := "prepay_id=" + prepayID
	message := strings.Join([]string{a.cfg.WechatPayAppID, timestamp, nonce, packageValue, ""}, "\n")
	signature, err := a.signWechatMessage(message)
	if err != nil {
		return wechatPayRequestParams{}, err
	}
	return wechatPayRequestParams{TimeStamp: timestamp, NonceStr: nonce, Package: packageValue, SignType: "RSA", PaySign: signature}, nil
}

func (a *App) buildWechatVirtualPaymentParams(order FinanceOrder, sessionKey string) (wechatVirtualPaymentParams, error) {
	signDataPayload := wechatVirtualSignData{
		OfferID:      strings.TrimSpace(a.cfg.WechatVirtualPayOfferID),
		BuyQuantity:  1,
		CurrencyType: "CNY",
		ProductID:    strings.TrimSpace(order.WechatVirtualProductID),
		GoodsPrice:   order.AmountCents,
		OutTradeNo:   order.OrderNumber,
		Attach:       fmt.Sprintf("finance_order:%d", order.ID),
		Env:          a.cfg.WechatVirtualPayEnv,
	}
	signDataBytes, err := json.Marshal(signDataPayload)
	if err != nil {
		return wechatVirtualPaymentParams{}, err
	}
	signData := string(signDataBytes)
	return wechatVirtualPaymentParams{
		Mode:      "short_series_goods",
		SignData:  signData,
		PaySig:    hmacSHA256Hex(a.wechatVirtualPayAppKey(), "requestVirtualPayment&"+signData),
		Signature: hmacSHA256Hex(sessionKey, signData),
	}, nil
}

func (a *App) wechatVirtualPayAppKey() string {
	return wechatVirtualPayAppKeyForConfig(a.cfg)
}

func hmacSHA256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) signWechatPayMessage(method, path, timestamp, nonce, body string) (string, error) {
	message := strings.Join([]string{method, path, timestamp, nonce, body, ""}, "\n")
	return a.signWechatMessage(message)
}

func (a *App) signWechatMessage(message string) (string, error) {
	privateKey, err := a.wechatPrivateKey()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (a *App) wechatPrivateKey() (*rsa.PrivateKey, error) {
	privateKeyText := strings.TrimSpace(a.cfg.WechatPayMchPrivateKey)
	if privateKeyText == "" && strings.TrimSpace(a.cfg.WechatPayMchPrivateKeyPath) != "" {
		data, err := os.ReadFile(strings.TrimSpace(a.cfg.WechatPayMchPrivateKeyPath))
		if err != nil {
			return nil, err
		}
		privateKeyText = string(data)
	}
	if privateKeyText == "" {
		return nil, errWechatPayNotConfigured
	}
	return parseAlipayPrivateKey(privateKeyText)
}

func (a *App) parseWechatNotify(body []byte) (wechatPayQueryResult, error) {
	var envelope struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			AssociatedData string `json:"associated_data"`
			Nonce          string `json:"nonce"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return wechatPayQueryResult{}, err
	}
	plaintext, err := decryptWechatResource(a.cfg.WechatPayAPIv3Key, envelope.Resource.Nonce, envelope.Resource.AssociatedData, envelope.Resource.Ciphertext)
	if err != nil {
		return wechatPayQueryResult{}, err
	}
	var payload struct {
		AppID         string `json:"appid"`
		MchID         string `json:"mchid"`
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		SuccessTime   string `json:"success_time"`
		Payer         struct {
			OpenID string `json:"openid"`
		} `json:"payer"`
		Amount struct {
			Total int64 `json:"total"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return wechatPayQueryResult{}, err
	}
	successTime, _ := time.Parse(time.RFC3339, payload.SuccessTime)
	return wechatPayQueryResult{
		AppID:         payload.AppID,
		MchID:         payload.MchID,
		OutTradeNo:    payload.OutTradeNo,
		TransactionID: payload.TransactionID,
		TradeState:    payload.TradeState,
		PayerOpenID:   payload.Payer.OpenID,
		AmountCents:   payload.Amount.Total,
		SuccessTime:   successTime,
	}, nil
}

func (a *App) verifyWechatNotifySignature(req *http.Request, body []byte) error {
	publicKeyText := strings.TrimSpace(a.cfg.WechatPayPlatformPublicKey)
	if publicKeyText == "" {
		return errors.New("wechat platform public key is not configured")
	}
	timestamp := strings.TrimSpace(req.Header.Get("Wechatpay-Timestamp"))
	nonce := strings.TrimSpace(req.Header.Get("Wechatpay-Nonce"))
	signatureText := strings.TrimSpace(req.Header.Get("Wechatpay-Signature"))
	if timestamp == "" || nonce == "" || signatureText == "" {
		return errors.New("wechat notify signature headers missing")
	}
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		return err
	}
	publicKey, err := parseAlipayPublicKey(publicKeyText)
	if err != nil {
		return err
	}
	message := strings.Join([]string{timestamp, nonce, string(body), ""}, "\n")
	digest := sha256.Sum256([]byte(message))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature)
}

func decryptWechatResource(apiV3Key, nonce, associatedData, ciphertextText string) ([]byte, error) {
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextText)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, []byte(nonce), ciphertext, []byte(associatedData))
}

func wechatPayConfigured(cfg Config) bool {
	return strings.TrimSpace(cfg.WechatPayAppID) != "" &&
		strings.TrimSpace(cfg.WechatPayMchID) != "" &&
		strings.TrimSpace(cfg.WechatPayMchCertSerialNo) != "" &&
		(strings.TrimSpace(cfg.WechatPayMchPrivateKey) != "" || strings.TrimSpace(cfg.WechatPayMchPrivateKeyPath) != "") &&
		strings.TrimSpace(cfg.WechatPayAPIv3Key) != ""
}

func wechatVirtualPayConfigured(cfg Config) bool {
	return strings.TrimSpace(cfg.WechatPayAppID) != "" &&
		strings.TrimSpace(cfg.WechatAppSecret) != "" &&
		strings.TrimSpace(cfg.WechatVirtualPayOfferID) != "" &&
		strings.TrimSpace(wechatVirtualPayAppKeyForConfig(cfg)) != ""
}

func wechatVirtualPayAppKeyForConfig(cfg Config) string {
	if cfg.WechatVirtualPayEnv == 1 {
		return strings.TrimSpace(cfg.WechatVirtualPaySandboxAppKey)
	}
	return strings.TrimSpace(cfg.WechatVirtualPayAppKey)
}

func (a *App) effectiveWechatNotifyURL() string {
	if strings.TrimSpace(a.cfg.WechatPayNotifyURL) != "" {
		return strings.TrimSpace(a.cfg.WechatPayNotifyURL)
	}
	return strings.TrimRight(a.cfg.AppBaseURL, "/") + "/api/payments/wechat/notify"
}

func summarizeWechatQuery(result wechatPayQueryResult) string {
	return fmt.Sprintf("out_trade_no=%s,transaction_id=%s,trade_state=%s,total=%d", result.OutTradeNo, result.TransactionID, result.TradeState, result.AmountCents)
}

func summarizeWechatVirtualQuery(result wechatVirtualPayQueryResult) string {
	return fmt.Sprintf("order_id=%s,wx_order_id=%s,wxpay_order_id=%s,status=%d,order_fee=%d,paid_fee=%d,openid=%s", result.OrderID, result.WXOrderID, result.WXPayOrderID, result.Status, result.OrderFee, result.PaidFee, result.OpenID)
}

func summarizeWechatVirtualNotify(result wechatVirtualPayQueryResult) string {
	return fmt.Sprintf("method=xpay.notify_provide_goods,order_id=%s,wx_order_id=%s,status=%d", result.OrderID, result.WXOrderID, result.Status)
}

func wechatVirtualTradeNo(result wechatVirtualPayQueryResult, order FinanceOrder) string {
	if strings.TrimSpace(result.WXPayOrderID) != "" {
		return strings.TrimSpace(result.WXPayOrderID)
	}
	if strings.TrimSpace(result.WXOrderID) != "" {
		return strings.TrimSpace(result.WXOrderID)
	}
	if strings.TrimSpace(result.ChannelOrderID) != "" {
		return strings.TrimSpace(result.ChannelOrderID)
	}
	return "virtual:" + order.OrderNumber
}

func isWechatVirtualOrderPending(status int) bool {
	return status == wechatVirtualOrderStatusInitialized ||
		status == wechatVirtualOrderStatusCreated
}

func isWechatVirtualOrderPaid(status int) bool {
	return status == wechatVirtualOrderStatusPaidPendingDelivery ||
		status == wechatVirtualOrderStatusDelivering ||
		status == wechatVirtualOrderStatusDelivered
}

func isWechatVirtualOrderClosedReason(reason string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(reason)), "ORDER_CLOSED")
}

func wechatVirtualTerminalError(status int) (string, string, bool) {
	switch status {
	case wechatVirtualOrderStatusClosed:
		return "wechat_virtual_order_closed", "微信虚拟支付订单已关闭", true
	case wechatVirtualOrderStatusRefunded, wechatVirtualOrderStatusUserRefunded:
		return "wechat_virtual_order_refunded", "微信虚拟支付订单已退款", true
	case wechatVirtualOrderStatusRefundFailed:
		return "wechat_virtual_refund_failed", "微信虚拟支付订单退款失败", true
	default:
		return "", "", false
	}
}
