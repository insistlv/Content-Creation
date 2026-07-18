package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	smsPurposeRegister      = "register"
	smsPurposeResetPassword = "reset_password"
	smsPurposeBindPhone     = "bind_phone"

	verificationCodeTTL          = 5 * time.Minute
	verificationCodeResendWindow = 60 * time.Second
	verificationPhoneHourLimit   = 5
	verificationIPHourLimit      = 20
	verificationMaxAttempts      = 5
)

var (
	mainlandPhonePattern    = regexp.MustCompile(`^1[3-9]\d{9}$`)
	verificationCodePattern = regexp.MustCompile(`^\d{6}$`)
	registerUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$`)
)

func normalizeMainlandPhone(phone string) string {
	return strings.TrimSpace(phone)
}

func isValidMainlandPhone(phone string) bool {
	return mainlandPhonePattern.MatchString(phone)
}

func isValidSMSPurpose(purpose string) bool {
	return purpose == smsPurposeRegister || purpose == smsPurposeResetPassword || purpose == smsPurposeBindPhone
}

func isValidRegisterUsername(username string) bool {
	return registerUsernamePattern.MatchString(username)
}

func hashVerificationCode(phone, purpose, code, secret string) string {
	key := strings.TrimSpace(secret)
	if key == "" {
		key = "image-agent-verification-code"
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(phone))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateVerificationCode() (string, error) {
	var digits [6]byte
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		digits[i] = byte('0' + n.Int64())
	}
	return string(digits[:]), nil
}

func (a *App) handleSendSMSCode(c *gin.Context) {
	var req struct {
		Phone   string `json:"phone"`
		Purpose string `json:"purpose"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	phone := normalizeMainlandPhone(req.Phone)
	purpose := strings.TrimSpace(req.Purpose)
	if !isValidMainlandPhone(phone) {
		writeError(c, http.StatusBadRequest, "invalid_phone", "手机号格式不正确")
		return
	}
	if !isValidSMSPurpose(purpose) {
		writeError(c, http.StatusBadRequest, "invalid_purpose", "验证码用途不正确")
		return
	}

	if err := a.validateSMSPurposeTarget(phone, purpose); err != nil {
		switch err.Error() {
		case "phone_exists":
			writeError(c, http.StatusConflict, "phone_exists", "手机号已注册")
		case "phone_not_found":
			writeError(c, http.StatusNotFound, "phone_not_found", "手机号未注册")
		default:
			writeError(c, http.StatusInternalServerError, "phone_lookup_failed", "手机号校验失败")
		}
		return
	}

	now := time.Now()
	ip := sourceIPAddress(c.Request)
	if retryAfter, limited, err := a.smsSendLimited(phone, ip, now); err != nil {
		writeError(c, http.StatusInternalServerError, "sms_limit_check_failed", "验证码发送校验失败")
		return
	} else if limited {
		c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(c, http.StatusTooManyRequests, "sms_rate_limited", "验证码发送过于频繁，请稍后再试")
		return
	}

	code, err := generateVerificationCode()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "code_generate_failed", "验证码生成失败")
		return
	}
	if a.smsSender == nil {
		a.smsSender = NewAliyunSMSSender(a.cfg)
	}
	if err := a.smsSender.SendVerificationCode(c.Request.Context(), phone, purpose, code); err != nil {
		writeError(c, http.StatusBadGateway, "sms_send_failed", "短信发送失败，请检查短信配置")
		return
	}

	record := AuthVerificationCode{
		Phone:     phone,
		Purpose:   purpose,
		CodeHash:  hashVerificationCode(phone, purpose, code, a.cfg.JWTSecret),
		ExpiresAt: now.Add(verificationCodeTTL),
		IPAddress: ip,
	}
	if err := a.db.Create(&record).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "code_store_failed", "验证码保存失败")
		return
	}

	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) validateSMSPurposeTarget(phone, purpose string) error {
	var count int64
	if err := a.db.Model(&User{}).Where("phone = ?", phone).Count(&count).Error; err != nil {
		return err
	}
	switch purpose {
	case smsPurposeRegister:
		if count > 0 {
			return errors.New("phone_exists")
		}
	case smsPurposeResetPassword:
		if count == 0 {
			return errors.New("phone_not_found")
		}
	case smsPurposeBindPhone:
		if count > 0 {
			return errors.New("phone_exists")
		}
	}
	return nil
}

func (a *App) smsSendLimited(phone, ip string, now time.Time) (time.Duration, bool, error) {
	var latest AuthVerificationCode
	result := a.db.Where("phone = ?", phone).Order("created_at desc").Limit(1).Find(&latest)
	if result.Error != nil {
		return 0, false, result.Error
	}
	if result.RowsAffected > 0 {
		elapsed := now.Sub(latest.CreatedAt)
		if elapsed >= 0 && elapsed < verificationCodeResendWindow {
			return verificationCodeResendWindow - elapsed, true, nil
		}
	}

	since := now.Add(-time.Hour)
	var phoneCount int64
	if err := a.db.Model(&AuthVerificationCode{}).Where("phone = ? AND created_at >= ?", phone, since).Count(&phoneCount).Error; err != nil {
		return 0, false, err
	}
	if phoneCount >= verificationPhoneHourLimit {
		return time.Hour, true, nil
	}
	var ipCount int64
	if err := a.db.Model(&AuthVerificationCode{}).Where("ip_address = ? AND created_at >= ?", ip, since).Count(&ipCount).Error; err != nil {
		return 0, false, err
	}
	if ipCount >= verificationIPHourLimit {
		return time.Hour, true, nil
	}
	return 0, false, nil
}

func (a *App) consumeVerificationCode(phone, purpose, code string) error {
	if !isValidMainlandPhone(phone) || !isValidSMSPurpose(purpose) || !verificationCodePattern.MatchString(code) {
		return errors.New("invalid_code")
	}
	var record AuthVerificationCode
	err := a.db.Where("phone = ? AND purpose = ? AND consumed_at IS NULL", phone, purpose).
		Order("created_at desc").
		First(&record).Error
	if err != nil {
		return errors.New("invalid_code")
	}
	if record.AttemptCount >= verificationMaxAttempts {
		return errors.New("too_many_attempts")
	}
	if time.Now().After(record.ExpiresAt) {
		_ = a.db.Model(&record).UpdateColumn("attempt_count", gorm.Expr("attempt_count + ?", 1)).Error
		return errors.New("invalid_code")
	}
	expected := hashVerificationCode(phone, purpose, code, a.cfg.JWTSecret)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(record.CodeHash)) != 1 {
		_ = a.db.Model(&record).UpdateColumn("attempt_count", gorm.Expr("attempt_count + ?", 1)).Error
		return errors.New("invalid_code")
	}
	now := time.Now()
	if err := a.db.Model(&record).Updates(map[string]any{"consumed_at": &now}).Error; err != nil {
		return err
	}
	return nil
}

func (a *App) handleRegisterPhone(c *gin.Context) {
	var req struct {
		Phone            string `json:"phone"`
		VerificationCode string `json:"verification_code"`
		Username         string `json:"username"`
		Password         string `json:"password"`
		InviteCode       string `json:"invite_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	phone := normalizeMainlandPhone(req.Phone)
	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	if !isValidMainlandPhone(phone) {
		writeError(c, http.StatusBadRequest, "invalid_phone", "手机号格式不正确")
		return
	}
	if username == "" || password == "" {
		writeError(c, http.StatusBadRequest, "credentials_required", "账号和密码不能为空")
		return
	}
	if !isValidRegisterUsername(username) {
		writeError(c, http.StatusBadRequest, "invalid_username", "账号只能使用 3-32 位字母、数字、下划线、点或横线")
		return
	}
	if len(password) < 8 {
		writeError(c, http.StatusBadRequest, "password_too_short", "密码至少 8 位")
		return
	}
	if exists, err := a.userExists("username = ?", username); err != nil {
		writeError(c, http.StatusInternalServerError, "user_lookup_failed", "账号校验失败")
		return
	} else if exists {
		writeError(c, http.StatusConflict, "username_taken", "账号已存在")
		return
	}
	if exists, err := a.userExists("phone = ?", phone); err != nil {
		writeError(c, http.StatusInternalServerError, "phone_lookup_failed", "手机号校验失败")
		return
	} else if exists {
		writeError(c, http.StatusConflict, "phone_exists", "手机号已注册")
		return
	}
	if err := a.consumeVerificationCode(phone, smsPurposeRegister, strings.TrimSpace(req.VerificationCode)); err != nil {
		writeVerificationCodeError(c, err)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}
	user := User{
		Username:                 username,
		DisplayName:              username,
		Phone:                    &phone,
		PasswordHash:             string(passwordHash),
		Status:                   UserStatusActive,
		LoginNotificationEnabled: true,
		RiskNotificationEnabled:  true,
	}
	now := time.Now()
	user.LastLoginAt = &now
	if err := a.createRegisteredUserWithInvite(&user, strings.ToUpper(strings.TrimSpace(req.InviteCode)), now); err != nil {
		writeRegisterError(c, err)
		return
	}
	session, err := a.issueUserSession(c.Writer, c.Request, user, false)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "session_issue_failed", "会话创建失败")
		return
	}
	writeJSON(c, http.StatusCreated, appendMiniProgramAuthPayload(c, accountPayload(user, signupBonusCredits), session))
}

func (a *App) handleResetPassword(c *gin.Context) {
	var req struct {
		Phone            string `json:"phone"`
		VerificationCode string `json:"verification_code"`
		NewPassword      string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	phone := normalizeMainlandPhone(req.Phone)
	newPassword := strings.TrimSpace(req.NewPassword)
	if !isValidMainlandPhone(phone) {
		writeError(c, http.StatusBadRequest, "invalid_phone", "手机号格式不正确")
		return
	}
	if len(newPassword) < 8 {
		writeError(c, http.StatusBadRequest, "password_too_short", "密码至少 8 位")
		return
	}
	var user User
	if err := a.db.Where("phone = ?", phone).First(&user).Error; err != nil {
		writeError(c, http.StatusNotFound, "phone_not_found", "手机号未注册")
		return
	}
	if err := a.consumeVerificationCode(phone, smsPurposeResetPassword, strings.TrimSpace(req.VerificationCode)); err != nil {
		writeVerificationCodeError(c, err)
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "密码处理失败")
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", user.ID).Update("password_hash", string(passwordHash)).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&UserSession{}).Error
	}); err != nil {
		writeError(c, http.StatusInternalServerError, "password_reset_failed", "密码重置失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

func (a *App) handleBindPhone(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		Phone            string `json:"phone"`
		VerificationCode string `json:"verification_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	phone := normalizeMainlandPhone(req.Phone)
	if !isValidMainlandPhone(phone) {
		writeError(c, http.StatusBadRequest, "invalid_phone", "手机号格式不正确")
		return
	}
	if user.Phone != nil && strings.TrimSpace(*user.Phone) != "" {
		writeError(c, http.StatusConflict, "phone_already_bound", "当前账号已绑定手机号")
		return
	}
	if exists, err := a.userExists("phone = ?", phone); err != nil {
		writeError(c, http.StatusInternalServerError, "phone_lookup_failed", "手机号校验失败")
		return
	} else if exists {
		writeError(c, http.StatusConflict, "phone_exists", "手机号已被其他账号绑定")
		return
	}
	if err := a.consumeVerificationCode(phone, smsPurposeBindPhone, strings.TrimSpace(req.VerificationCode)); err != nil {
		writeVerificationCodeError(c, err)
		return
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("phone", phone).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "phone_bind_failed", "手机号绑定失败")
		return
	}
	user.Phone = &phone
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) handleUnbindPhone(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		CurrentPassword string `json:"current_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}
	if user.Phone == nil || strings.TrimSpace(*user.Phone) == "" {
		writeError(c, http.StatusConflict, "phone_not_bound", "当前账号未绑定手机号")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeError(c, http.StatusUnauthorized, "password_mismatch", "当前密码错误")
		return
	}
	if err := a.db.Model(&User{}).Where("id = ?", user.ID).Update("phone", nil).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "phone_unbind_failed", "手机号解绑失败")
		return
	}
	user.Phone = nil
	balance, err := a.lookupBalance(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "balance_load_failed", "账户读取失败")
		return
	}
	writeJSON(c, http.StatusOK, accountPayload(*user, balance.AvailableCredits))
}

func (a *App) userExists(query string, args ...any) (bool, error) {
	var count int64
	if err := a.db.Model(&User{}).Where(query, args...).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (a *App) createRegisteredUserWithInvite(user *User, inviteCode string, now time.Time) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		var invite Invite
		if inviteCode != "" {
			if err := tx.Where("code = ?", inviteCode).First(&invite).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("invite_not_found")
				}
				return err
			}
			if err := invite.ValidateAvailability(now); err != nil {
				return err
			}
		}
		var standardRole UserRole
		if err := tx.Where("code = ?", "standard_user").First(&standardRole).Error; err != nil {
			return err
		}
		user.UserRoleID = &standardRole.ID
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := createSignupBonusTx(tx, user.ID); err != nil {
			return err
		}
		if err := createRegistrationConsents(tx, user.ID, now); err != nil {
			return err
		}
		if inviteCode == "" {
			return nil
		}
		if err := tx.Model(&Invite{}).Where("id = ?", invite.ID).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", 1)).Error; err != nil {
			return err
		}
		return tx.Create(&InviteRedemption{
			InviteID:     invite.ID,
			InviteCode:   invite.Code,
			InviterName:  fallbackString(invite.Label, "运营后台"),
			UserID:       user.ID,
			Username:     user.Username,
			DisplayName:  user.DisplayName,
			Email:        user.Email,
			RegisteredAt: now,
		}).Error
	})
}

func createRegistrationConsents(tx *gorm.DB, userID uint, now time.Time) error {
	for _, consentType := range []string{"terms", "privacy", "algorithm_disclosure"} {
		consent := UserConsent{
			UserID:      userID,
			ConsentType: consentType,
			Version:     "2026-06-04",
			Source:      "register",
			CreatedAt:   now,
		}
		if err := tx.Create(&consent).Error; err != nil {
			return err
		}
	}
	return nil
}

func writeRegisterError(c *gin.Context, err error) {
	switch err.Error() {
	case "invite_not_found":
		writeError(c, http.StatusBadRequest, "invite_not_found", "邀请码不存在")
	case "invite_disabled":
		writeError(c, http.StatusBadRequest, "invite_disabled", "邀请码已停用")
	case "invite_expired":
		writeError(c, http.StatusBadRequest, "invite_expired", "邀请码已过期")
	case "invite_quota_exhausted":
		writeError(c, http.StatusBadRequest, "invite_quota_exhausted", "邀请码可用次数已用完")
	default:
		writeError(c, http.StatusInternalServerError, "register_failed", "注册失败")
	}
}

func writeVerificationCodeError(c *gin.Context, err error) {
	if err != nil && err.Error() == "too_many_attempts" {
		writeError(c, http.StatusTooManyRequests, "verification_attempts_exceeded", "验证码错误次数过多，请重新获取")
		return
	}
	writeError(c, http.StatusUnauthorized, "verification_code_invalid", "验证码错误或已过期")
}
