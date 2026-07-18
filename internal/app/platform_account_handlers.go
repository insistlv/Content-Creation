package app

// 本文件从 platform_handlers.go 拆分：用户账户、登录会话、点数与偏好相关 handler。

import (
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func (a *App) handleRegister(c *gin.Context) {
	writeError(c, http.StatusBadRequest, "phone_verification_required", "注册必须使用手机号并完成短信验证码")
}

func (a *App) handleLogin(c *gin.Context) {
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
	if c.GetHeader("X-Image-Agent-Client") != "mp-weixin" && !a.validateAuthCaptcha(c, "user_login", req.CaptchaID, req.CaptchaCode) {
		return
	}

	var user User
	identifier := strings.TrimSpace(req.Username)
	query := a.db.Where("username = ?", identifier)
	if isValidMainlandPhone(identifier) {
		query = a.db.Where("username = ? OR phone = ?", identifier, identifier)
	}
	if err := query.First(&user).Error; err != nil {
		writeError(c, http.StatusUnauthorized, "login_failed", "账号或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(c, http.StatusUnauthorized, "login_failed", "账号或密码错误")
		return
	}

	now := time.Now()
	if err := a.db.Model(&user).Update("last_login_at", now).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "login_failed", "登录状态保存失败")
		return
	}
	user.LastLoginAt = &now
	c.Set("currentUser", &user)

	session, err := a.issueUserSession(c.Writer, c.Request, user, req.RememberLogin)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "session_issue_failed", "会话创建失败")
		return
	}

	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}

	writeJSON(c, http.StatusOK, appendMiniProgramAuthPayload(c, accountPayload(user, balance.AvailableCredits), session))
}

func (a *App) handleLogout(c *gin.Context) {
	claims := c.MustGet("claims").(*SessionClaims)
	if claims.SessionID != "" {
		_ = a.db.Where("token_id = ? AND user_id = ?", claims.SessionID, claims.UserID).Delete(&UserSession{}).Error
	}
	http.SetCookie(c.Writer, clearCookie(userSessionCookie))
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleMe(c *gin.Context) {
	user := currentUser(c)
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}

	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleUpdateProfile(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		writeError(c, http.StatusBadRequest, "display_name_required", "显示名称不能为空")
		return
	}
	if len([]rune(displayName)) > 64 {
		writeError(c, http.StatusBadRequest, "display_name_too_long", "显示名称不能超过 64 个字符")
		return
	}

	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("display_name", displayName).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "profile_save_failed", "资料保存失败")
		return
	}
	user.DisplayName = displayName
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleUpdateEmail(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			writeError(c, http.StatusBadRequest, "invalid_email", "邮箱格式不正确")
			return
		}
	}

	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("email", email).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "email_save_failed", "邮箱保存失败")
		return
	}
	user.Email = email
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleUpdatePreferences(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		LoginNotificationEnabled bool `json:"login_notification_enabled"`
		RiskNotificationEnabled  bool `json:"risk_notification_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	updates := map[string]any{
		"login_notification_enabled": req.LoginNotificationEnabled,
		"risk_notification_enabled":  req.RiskNotificationEnabled,
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "preferences_save_failed", "偏好保存失败")
		return
	}
	user.LoginNotificationEnabled = req.LoginNotificationEnabled
	user.RiskNotificationEnabled = req.RiskNotificationEnabled
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleGetCredits(c *gin.Context) {
	user := currentUser(c)
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonthStart := monthStart.AddDate(0, 1, 0)
	var monthlyConsumption int
	if err := a.db.Model(&CreditTransaction{}).
		Select("COALESCE(SUM(-amount), 0)").
		Where("user_id = ? AND amount < 0 AND created_at >= ? AND created_at < ?", user.ID, monthStart, nextMonthStart).
		Scan(&monthlyConsumption).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	var totalRecharged int
	if err := a.db.Model(&CreditTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND amount > 0", user.ID).
		Scan(&totalRecharged).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	var latestTransaction CreditTransaction
	latestTransactionAt := any(nil)
	err = a.db.Where("user_id = ?", user.ID).Order("created_at desc, id desc").First(&latestTransaction).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	if err == nil {
		latestTransactionAt = latestTransaction.CreatedAt
	}
	writeJSON(c, http.StatusOK, gin.H{
		"user_id":               user.ID,
		"available_credits":     balance.AvailableCredits,
		"monthly_consumption":   monthlyConsumption,
		"total_recharged":       totalRecharged,
		"latest_transaction_at": latestTransactionAt,
	})
}

func (a *App) handleAccountPresence(c *gin.Context) {
	session := currentUserSession(c)
	if session == nil || session.LastSeenAt == nil {
		writeError(c, http.StatusInternalServerError, "presence_unavailable", "在线状态读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"ok":                    true,
		"last_seen_at":          session.LastSeenAt,
		"online_window_seconds": int(userPresenceOnlineWindow.Seconds()),
	})
}

func (a *App) handleGetCreditTransactions(c *gin.Context) {
	user := currentUser(c)
	page := maxInt(getQueryInt(c, "page", 1), 1)
	pageSize := minInt(maxInt(getQueryInt(c, "page_size", 20), 1), 100)
	kind := strings.TrimSpace(c.Query("kind"))

	dbQuery := a.db.Model(&CreditTransaction{}).Where("user_id = ?", user.ID)
	switch kind {
	case "recharge":
		dbQuery = dbQuery.Where("amount > 0")
	case "consume":
		dbQuery = dbQuery.Where("amount < 0")
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "transactions_load_failed", "流水读取失败")
		return
	}

	var items []CreditTransaction
	itemQuery := dbQuery.Order("created_at desc, id desc")
	if c.Query("page") != "" || c.Query("page_size") != "" || c.Query("kind") != "" {
		itemQuery = itemQuery.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	if err := itemQuery.Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "transactions_load_failed", "流水读取失败")
		return
	}
	if c.Query("page") == "" && c.Query("page_size") == "" && c.Query("kind") == "" {
		pageSize = len(items)
	}
	writeJSON(c, http.StatusOK, gin.H{
		"user_id":   user.ID,
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"has_more":  int64(page*pageSize) < total,
	})
}

func (a *App) handleChangePassword(c *gin.Context) {
	user := currentUser(c)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeError(c, http.StatusUnauthorized, "password_mismatch", "当前密码错误")
		return
	}
	if len(strings.TrimSpace(req.NewPassword)) < 8 {
		writeError(c, http.StatusBadRequest, "password_too_short", "密码至少 8 位")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("password_hash", string(passwordHash)).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "password_save_failed", "密码更新失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleSetPaymentPassword(c *gin.Context) {
	user := currentUser(c)

	var req struct {
		CurrentPassword string `json:"current_password"`
		PaymentPassword string `json:"payment_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeError(c, http.StatusUnauthorized, "password_mismatch", "当前密码错误")
		return
	}
	if !isSixDigitPassword(req.PaymentPassword) {
		writeError(c, http.StatusBadRequest, "invalid_payment_password", "支付密码必须是 6 位数字")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.PaymentPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "payment_password_hash_failed", "支付密码处理失败")
		return
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("payment_password_hash", string(passwordHash)).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "payment_password_save_failed", "支付密码保存失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "payment_password_enabled": true})
}

func (a *App) handleClearPaymentPassword(c *gin.Context) {
	user := currentUser(c)

	var req struct {
		CurrentPassword string `json:"current_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeError(c, http.StatusUnauthorized, "password_mismatch", "当前密码错误")
		return
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("payment_password_hash", "").Error; err != nil {
		writeError(c, http.StatusInternalServerError, "payment_password_clear_failed", "支付密码清除失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "payment_password_enabled": false})
}

func isSixDigitPassword(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

type userSessionPayload struct {
	ID        uint      `json:"id"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Current   bool      `json:"current"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (a *App) handleListUserSessions(c *gin.Context) {
	user := currentUser(c)
	claims := c.MustGet("claims").(*SessionClaims)

	var sessions []UserSession
	if err := a.db.Where("user_id = ?", user.ID).Order("created_at desc, id desc").Find(&sessions).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "sessions_load_failed", "设备列表读取失败")
		return
	}
	items := make([]userSessionPayload, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, userSessionPayload{
			ID:        session.ID,
			IPAddress: session.IPAddress,
			UserAgent: session.UserAgent,
			Current:   session.TokenID == claims.SessionID,
			ExpiresAt: session.ExpiresAt,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.UpdatedAt,
		})
	}
	writeJSON(c, http.StatusOK, gin.H{
		"user_id": user.ID,
		"items":   items,
	})
}

func (a *App) handleDeleteUserSession(c *gin.Context) {
	user := currentUser(c)
	claims := c.MustGet("claims").(*SessionClaims)
	sessionID := strings.TrimSpace(c.Param("id"))

	if sessionID == "others" {
		if err := a.db.Where("user_id = ? AND token_id <> ?", user.ID, claims.SessionID).Delete(&UserSession{}).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "sessions_delete_failed", "设备下线失败")
			return
		}
		writeJSON(c, http.StatusOK, gin.H{"ok": true})
		return
	}

	id, err := strconv.ParseUint(sessionID, 10, 64)
	if err != nil || id == 0 {
		writeError(c, http.StatusBadRequest, "invalid_session_id", "设备会话无效")
		return
	}
	var session UserSession
	if err := a.db.Where("id = ? AND user_id = ?", uint(id), user.ID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "session_not_found", "设备会话不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "session_load_failed", "设备会话读取失败")
		return
	}
	if session.TokenID == claims.SessionID {
		writeError(c, http.StatusBadRequest, "current_session_cannot_be_deleted", "不能下线当前设备")
		return
	}
	if err := a.db.Delete(&session).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "session_delete_failed", "设备下线失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleListPackages(c *gin.Context) {
	if err := a.ensurePackagePresentationColumns(); err != nil {
		writeError(c, http.StatusInternalServerError, "packages_load_failed", "套餐读取失败")
		return
	}
	var items []Package
	if err := a.db.Where("is_active = ?", true).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "packages_load_failed", "套餐读取失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"items": items})
}

func (a *App) handlePurchaseIntentsDisabled(c *gin.Context) {
	writeError(c, http.StatusGone, "purchase_intents_disabled", "购买意向功能已下线，请直接购买套餐")
}
