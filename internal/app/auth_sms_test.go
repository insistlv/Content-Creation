package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm/logger"
)

type stubSMSSender struct {
	err   error
	sent  []sentSMSCode
	codes map[string]string
}

type sentSMSCode struct {
	Phone   string
	Purpose string
	Code    string
}

func (s *stubSMSSender) SendVerificationCode(ctx context.Context, phone, purpose, code string) error {
	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, sentSMSCode{Phone: phone, Purpose: purpose, Code: code})
	if s.codes == nil {
		s.codes = map[string]string{}
	}
	s.codes[phone+"|"+purpose] = code
	return nil
}

func TestPhoneAuthSMSCodeAllowsWeixinMiniProgramMissingOrigin(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.smsSender = &stubSMSSender{}

	missingOriginResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "12800138000",
		"purpose": "register",
	}, nil, map[string]string{
		"Origin": "",
	})
	if missingOriginResp.Code != http.StatusForbidden {
		t.Fatalf("expected missing origin 403, got %d: %s", missingOriginResp.Code, missingOriginResp.Body.String())
	}
	if !strings.Contains(missingOriginResp.Body.String(), "origin_required") {
		t.Fatalf("expected origin_required error, got %s", missingOriginResp.Body.String())
	}

	mpResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "12800138000",
		"purpose": "register",
	}, nil, map[string]string{
		"Origin":               "",
		"X-Image-Agent-Client": "mp-weixin",
	})
	if mpResp.Code != http.StatusBadRequest {
		t.Fatalf("expected mini program request to reach SMS handler and return invalid phone 400, got %d: %s", mpResp.Code, mpResp.Body.String())
	}
	if !strings.Contains(mpResp.Body.String(), "invalid_phone") {
		t.Fatalf("expected invalid_phone error, got %s", mpResp.Body.String())
	}
}

func TestPhoneAuthFirstSMSCodeSendDoesNotLogRecordNotFound(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.smsSender = &stubSMSSender{}

	var logBuffer bytes.Buffer
	db.Logger = logger.New(log.New(&logBuffer, "", 0), logger.Config{
		LogLevel:                  logger.Error,
		IgnoreRecordNotFoundError: false,
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800138000",
		"purpose": "register",
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected first SMS send 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if bytes.Contains(logBuffer.Bytes(), []byte("record not found")) {
		t.Fatalf("expected first SMS send not to emit gorm record-not-found log, got %s", logBuffer.String())
	}
}

func TestPhoneRegistrationLoginAndResetPasswordFlow(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	sms := &stubSMSSender{}
	testApp.smsSender = sms

	sendRegister := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800138000",
		"purpose": "register",
	}, nil)
	if sendRegister.Code != http.StatusOK {
		t.Fatalf("expected send register code 200, got %d: %s", sendRegister.Code, sendRegister.Body.String())
	}
	registerCode := sms.codes["13800138000|register"]
	if registerCode == "" {
		t.Fatal("expected SMS sender to receive generated register code")
	}

	registerResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13800138000",
		"verification_code": registerCode,
		"username":          "phone_user",
		"password":          "test-password",
	}, nil)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("expected phone register 201, got %d: %s", registerResp.Code, registerResp.Body.String())
	}
	var registerPayload struct {
		AvailableCredits int `json:"available_credits"`
	}
	if err := json.Unmarshal(registerResp.Body.Bytes(), &registerPayload); err != nil {
		t.Fatalf("decode phone register payload: %v", err)
	}
	if registerPayload.AvailableCredits != 5 {
		t.Fatalf("expected signup bonus credits 5, got %s", registerResp.Body.String())
	}

	var user User
	if err := db.Where("username = ?", "phone_user").First(&user).Error; err != nil {
		t.Fatalf("load registered user: %v", err)
	}
	if user.Phone == nil || *user.Phone != "13800138000" {
		t.Fatalf("expected persisted phone, got %#v", user.Phone)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load signup bonus balance: %v", err)
	}
	if balance.AvailableCredits != 5 {
		t.Fatalf("expected signup bonus balance 5, got %+v", balance)
	}
	assertSignupBonusTransaction(t, db, user.ID)

	loginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "13800138000", "test-password"), nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected phone login 200, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	loginCookies := loginResp.Result().Cookies()
	if err := db.Model(&AuthVerificationCode{}).
		Where("phone = ?", "13800138000").
		Update("created_at", time.Now().Add(-verificationCodeResendWindow-time.Second)).Error; err != nil {
		t.Fatalf("age register code for reset send: %v", err)
	}

	sendReset := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800138000",
		"purpose": "reset_password",
	}, nil)
	if sendReset.Code != http.StatusOK {
		t.Fatalf("expected send reset code 200, got %d: %s", sendReset.Code, sendReset.Body.String())
	}
	resetCode := sms.codes["13800138000|reset_password"]

	resetResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/reset-password", map[string]any{
		"phone":             "13800138000",
		"verification_code": resetCode,
		"new_password":      "Newtest-password",
	}, nil)
	if resetResp.Code != http.StatusOK {
		t.Fatalf("expected reset password 200, got %d: %s", resetResp.Code, resetResp.Body.String())
	}

	oldSessionMe := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, loginCookies)
	if oldSessionMe.Code != http.StatusUnauthorized {
		t.Fatalf("expected reset to invalidate existing sessions, got %d: %s", oldSessionMe.Code, oldSessionMe.Body.String())
	}

	oldPasswordLogin := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "phone_user", "test-password"), nil)
	if oldPasswordLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password rejected, got %d: %s", oldPasswordLogin.Code, oldPasswordLogin.Body.String())
	}

	newPasswordLogin := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "phone_user", "Newtest-password"), nil)
	if newPasswordLogin.Code != http.StatusOK {
		t.Fatalf("expected new password login 200, got %d: %s", newPasswordLogin.Code, newPasswordLogin.Body.String())
	}
}

func TestPhoneRegistrationRejectsUnsafeUsernames(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	createSMSVerificationCodeForTest(t, testApp, "13800139108", smsPurposeRegister, "123456")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13800139108",
		"verification_code": "123456",
		"username":          "../../etc/passwd",
		"password":          "test-password",
	}, nil)
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "invalid_username") {
		t.Fatalf("expected invalid_username 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestMiniProgramPhoneRegistrationReturnsBearerToken(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	sms := &stubSMSSender{}
	testApp.smsSender = sms

	sendRegister := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13700137000",
		"purpose": "register",
	}, nil, map[string]string{
		"X-Image-Agent-Client": "mp-weixin",
	})
	if sendRegister.Code != http.StatusOK {
		t.Fatalf("expected send register code 200, got %d: %s", sendRegister.Code, sendRegister.Body.String())
	}
	registerCode := sms.codes["13700137000|register"]
	if registerCode == "" {
		t.Fatal("expected SMS sender to receive generated register code")
	}

	registerResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13700137000",
		"verification_code": registerCode,
		"username":          "mp_phone_user",
		"password":          "test-password",
	}, nil, map[string]string{
		"X-Image-Agent-Client": "mp-weixin",
	})
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("expected phone register 201, got %d: %s", registerResp.Code, registerResp.Body.String())
	}
	var payload struct {
		AuthToken     string `json:"auth_token"`
		AuthExpiresAt string `json:"auth_expires_at"`
	}
	if err := json.Unmarshal(registerResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode phone register payload: %v", err)
	}
	if strings.TrimSpace(payload.AuthToken) == "" || strings.TrimSpace(payload.AuthExpiresAt) == "" {
		t.Fatalf("expected phone register auth token fields, got %s", registerResp.Body.String())
	}
}

func TestLoggedInLegacyUserCanBindPhoneWithSMSCode(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	sms := &stubSMSSender{}
	testApp.smsSender = sms
	user, cookies := createLoggedInUser(t, testApp, "legacy_phone_user", "test-password")

	sendBind := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13600136000",
		"purpose": "bind_phone",
	}, nil)
	if sendBind.Code != http.StatusOK {
		t.Fatalf("expected send bind code 200, got %d: %s", sendBind.Code, sendBind.Body.String())
	}
	bindCode := sms.codes["13600136000|bind_phone"]
	if bindCode == "" {
		t.Fatal("expected SMS sender to receive generated bind phone code")
	}

	bindResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/phone", map[string]any{
		"phone":             "13600136000",
		"verification_code": bindCode,
	}, cookies)
	if bindResp.Code != http.StatusOK {
		t.Fatalf("expected phone bind 200, got %d: %s", bindResp.Code, bindResp.Body.String())
	}
	if !strings.Contains(bindResp.Body.String(), `"phone":"13600136000"`) {
		t.Fatalf("expected bound phone in response, got %s", bindResp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Phone == nil || *reloaded.Phone != "13600136000" {
		t.Fatalf("expected persisted bound phone, got %#v", reloaded.Phone)
	}
}

func TestLoggedInUserCanUnbindPhoneWithCurrentPassword(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "phone_unbind_user", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13600136002")

	unbindResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/phone", map[string]any{
		"current_password": "test-password",
	}, cookies)
	if unbindResp.Code != http.StatusOK {
		t.Fatalf("expected phone unbind 200, got %d: %s", unbindResp.Code, unbindResp.Body.String())
	}
	if !strings.Contains(unbindResp.Body.String(), `"phone":null`) {
		t.Fatalf("expected null phone in response, got %s", unbindResp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Phone != nil {
		t.Fatalf("expected persisted phone to be cleared, got %#v", reloaded.Phone)
	}

	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected session to remain active after phone unbind, got %d: %s", meResp.Code, meResp.Body.String())
	}
}

func TestLoggedInUserPhoneUnbindRejectsWrongPasswordAndMissingBinding(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "phone_unbind_reject_user", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13600136003")

	wrongPasswordResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/phone", map[string]any{
		"current_password": "WrongPass123",
	}, cookies)
	if wrongPasswordResp.Code != http.StatusUnauthorized || !strings.Contains(wrongPasswordResp.Body.String(), "password_mismatch") {
		t.Fatalf("expected wrong password 401, got %d: %s", wrongPasswordResp.Code, wrongPasswordResp.Body.String())
	}

	var afterWrongPassword User
	if err := db.First(&afterWrongPassword, user.ID).Error; err != nil {
		t.Fatalf("reload user after wrong password: %v", err)
	}
	if afterWrongPassword.Phone == nil || *afterWrongPassword.Phone != "13600136003" {
		t.Fatalf("expected phone to remain bound after wrong password, got %#v", afterWrongPassword.Phone)
	}

	okResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/phone", map[string]any{
		"current_password": "test-password",
	}, cookies)
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected first phone unbind 200, got %d: %s", okResp.Code, okResp.Body.String())
	}

	missingBindingResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/phone", map[string]any{
		"current_password": "test-password",
	}, cookies)
	if missingBindingResp.Code != http.StatusConflict || !strings.Contains(missingBindingResp.Body.String(), "phone_not_bound") {
		t.Fatalf("expected missing phone binding 409, got %d: %s", missingBindingResp.Code, missingBindingResp.Body.String())
	}
}

func TestPhoneAuthRejectsInvalidCodesDuplicatePhoneAndSendLimits(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	sms := &stubSMSSender{}
	testApp.smsSender = sms

	badPhone := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "12800138000",
		"purpose": "register",
	}, nil)
	if badPhone.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid phone 400, got %d: %s", badPhone.Code, badPhone.Body.String())
	}

	sendResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13900139000",
		"purpose": "register",
	}, nil)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("expected first code send 200, got %d: %s", sendResp.Code, sendResp.Body.String())
	}
	cooldownResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13900139000",
		"purpose": "register",
	}, nil)
	if cooldownResp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected resend cooldown 429, got %d: %s", cooldownResp.Code, cooldownResp.Body.String())
	}

	wrongCodeResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13900139000",
		"verification_code": "000000",
		"username":          "wrong_code_user",
		"password":          "test-password",
	}, nil)
	if wrongCodeResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong code 401, got %d: %s", wrongCodeResp.Code, wrongCodeResp.Body.String())
	}

	code := sms.codes["13900139000|register"]
	registerResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13900139000",
		"verification_code": code,
		"username":          "phone_once",
		"password":          "test-password",
	}, nil)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("expected phone register 201, got %d: %s", registerResp.Code, registerResp.Body.String())
	}

	consumedAt := time.Now()
	consumedRecord := AuthVerificationCode{
		Phone:      "13500135000",
		Purpose:    "register",
		CodeHash:   hashVerificationCode("13500135000", "register", "654321", testApp.cfg.JWTSecret),
		ExpiresAt:  time.Now().Add(time.Minute),
		ConsumedAt: &consumedAt,
		IPAddress:  "192.0.2.1",
	}
	if err := db.Create(&consumedRecord).Error; err != nil {
		t.Fatalf("seed consumed verification code: %v", err)
	}
	reuseResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13500135000",
		"verification_code": "654321",
		"username":          "phone_twice",
		"password":          "test-password",
	}, nil)
	if reuseResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected consumed code rejected, got %d: %s", reuseResp.Code, reuseResp.Body.String())
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	duplicatePhone := "13700137000"
	if err := db.Create(&User{
		Username:                 "existing_phone",
		DisplayName:              "existing_phone",
		Phone:                    &duplicatePhone,
		PasswordHash:             string(passwordHash),
		Status:                   UserStatusActive,
		LoginNotificationEnabled: true,
		RiskNotificationEnabled:  true,
	}).Error; err != nil {
		t.Fatalf("seed duplicate phone user: %v", err)
	}
	duplicateSend := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   duplicatePhone,
		"purpose": "register",
	}, nil)
	if duplicateSend.Code != http.StatusConflict {
		t.Fatalf("expected duplicate phone register code conflict, got %d: %s", duplicateSend.Code, duplicateSend.Body.String())
	}
}

func TestPhoneAuthRejectsExpiredCodesAndSMSProviderFailures(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	sms := &stubSMSSender{err: errors.New("provider down")}
	testApp.smsSender = sms

	sendFailure := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13600136000",
		"purpose": "register",
	}, nil)
	if sendFailure.Code != http.StatusBadGateway {
		t.Fatalf("expected provider failure 502, got %d: %s", sendFailure.Code, sendFailure.Body.String())
	}

	expiredCode := AuthVerificationCode{
		Phone:        "13600136001",
		Purpose:      "register",
		CodeHash:     hashVerificationCode("13600136001", "register", "123456", testApp.cfg.JWTSecret),
		ExpiresAt:    time.Now().Add(-time.Minute),
		IPAddress:    "192.0.2.1",
		AttemptCount: 0,
	}
	if err := db.Create(&expiredCode).Error; err != nil {
		t.Fatalf("seed expired verification code: %v", err)
	}

	expiredResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13600136001",
		"verification_code": "123456",
		"username":          "expired_code_user",
		"password":          "test-password",
	}, nil)
	if expiredResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired code rejected, got %d: %s", expiredResp.Code, expiredResp.Body.String())
	}
}
