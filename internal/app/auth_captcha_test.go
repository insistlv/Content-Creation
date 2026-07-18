package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAuthCaptchaEndpointReturnsDecodablePNGAndStoresChallenge(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/auth/captcha?purpose=user_login", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected captcha 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		CaptchaID   string `json:"captcha_id"`
		ImageBase64 string `json:"image_base64"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode captcha payload: %v", err)
	}
	if payload.CaptchaID == "" {
		t.Fatal("expected captcha_id")
	}
	if payload.ExpiresIn != int(authCaptchaTTL/time.Second) {
		t.Fatalf("expected expires_in %d, got %d", int(authCaptchaTTL/time.Second), payload.ExpiresIn)
	}
	raw, err := base64.StdEncoding.DecodeString(payload.ImageBase64)
	if err != nil {
		t.Fatalf("decode captcha image base64: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("decode captcha png: %v", err)
	}

	var challenge AuthCaptchaChallenge
	if err := db.Where("captcha_id = ?", payload.CaptchaID).First(&challenge).Error; err != nil {
		t.Fatalf("load stored challenge: %v", err)
	}
	if challenge.Purpose != "user_login" || challenge.CodeHash == "" || challenge.ConsumedAt != nil {
		t.Fatalf("unexpected challenge record: %+v", challenge)
	}
	if challenge.ExpiresAt.Before(time.Now().Add(4 * time.Minute)) {
		t.Fatalf("expected challenge to expire about 5 minutes later, got %s", challenge.ExpiresAt)
	}
}

func TestWebLoginRequiresValidOneTimeCaptchaAndConsumesOnPasswordFailure(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, _ = createLoggedInUser(t, testApp, "captcha_user", "test-password")

	missingCaptchaResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "captcha_user",
		"password": "test-password",
	}, nil)
	if missingCaptchaResp.Code != http.StatusBadRequest || !bytes.Contains(missingCaptchaResp.Body.Bytes(), []byte(`"captcha_required"`)) {
		t.Fatalf("expected captcha_required, got %d: %s", missingCaptchaResp.Code, missingCaptchaResp.Body.String())
	}

	wrong := createTestCaptchaChallenge(t, db, "user_login", "AB2CD")
	wrongResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username":     "captcha_user",
		"password":     "test-password",
		"captcha_id":   wrong,
		"captcha_code": "ZZZZZ",
	}, nil)
	if wrongResp.Code != http.StatusUnauthorized || !bytes.Contains(wrongResp.Body.Bytes(), []byte(`"captcha_invalid"`)) {
		t.Fatalf("expected captcha_invalid, got %d: %s", wrongResp.Code, wrongResp.Body.String())
	}

	passwordFailure := createTestCaptchaChallenge(t, db, "user_login", "Q7K9M")
	badPasswordResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username":     "captcha_user",
		"password":     "wrong-password",
		"captcha_id":   passwordFailure,
		"captcha_code": "Q7K9M",
	}, nil)
	if badPasswordResp.Code != http.StatusUnauthorized || !bytes.Contains(badPasswordResp.Body.Bytes(), []byte(`"login_failed"`)) {
		t.Fatalf("expected login_failed, got %d: %s", badPasswordResp.Code, badPasswordResp.Body.String())
	}
	reuseAfterPasswordFailure := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username":     "captcha_user",
		"password":     "test-password",
		"captcha_id":   passwordFailure,
		"captcha_code": "Q7K9M",
	}, nil)
	if reuseAfterPasswordFailure.Code != http.StatusUnauthorized || !bytes.Contains(reuseAfterPasswordFailure.Body.Bytes(), []byte(`"captcha_invalid"`)) {
		t.Fatalf("expected consumed captcha to fail, got %d: %s", reuseAfterPasswordFailure.Code, reuseAfterPasswordFailure.Body.String())
	}

	successID := createTestCaptchaChallenge(t, db, "user_login", "R8T6W")
	okResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username":     "captcha_user",
		"password":     "test-password",
		"captcha_id":   successID,
		"captcha_code": "r8t6w",
	}, nil)
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", okResp.Code, okResp.Body.String())
	}
	reuseAfterSuccess := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username":     "captcha_user",
		"password":     "test-password",
		"captcha_id":   successID,
		"captcha_code": "R8T6W",
	}, nil)
	if reuseAfterSuccess.Code != http.StatusUnauthorized || !bytes.Contains(reuseAfterSuccess.Body.Bytes(), []byte(`"captcha_invalid"`)) {
		t.Fatalf("expected one-time captcha reuse to fail, got %d: %s", reuseAfterSuccess.Code, reuseAfterSuccess.Body.String())
	}
}

func TestWebLoginRejectsExpiredAndAttemptExceededCaptchaButMpLoginSkipsCaptcha(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, _ = createLoggedInUser(t, testApp, "captcha_mp_user", "test-password")

	expiredID := createExpiredTestCaptchaChallenge(t, db, "user_login", "A3B4C")
	expiredResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username":     "captcha_mp_user",
		"password":     "test-password",
		"captcha_id":   expiredID,
		"captcha_code": "A3B4C",
	}, nil)
	if expiredResp.Code != http.StatusUnauthorized || !bytes.Contains(expiredResp.Body.Bytes(), []byte(`"captcha_invalid"`)) {
		t.Fatalf("expected expired captcha_invalid, got %d: %s", expiredResp.Code, expiredResp.Body.String())
	}

	exceededID := createTestCaptchaChallenge(t, db, "user_login", "B5C6D")
	for attempt := 1; attempt <= authCaptchaMaxAttempts; attempt++ {
		resp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
			"username":     "captcha_mp_user",
			"password":     "test-password",
			"captcha_id":   exceededID,
			"captcha_code": "WRONG",
		}, nil)
		if attempt < authCaptchaMaxAttempts && resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected invalid attempt %d to be 401, got %d: %s", attempt, resp.Code, resp.Body.String())
		}
		if attempt == authCaptchaMaxAttempts && (resp.Code != http.StatusTooManyRequests || !bytes.Contains(resp.Body.Bytes(), []byte(`"captcha_attempts_exceeded"`))) {
			t.Fatalf("expected attempts exceeded, got %d: %s", resp.Code, resp.Body.String())
		}
	}

	mpResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "captcha_mp_user",
		"password": "test-password",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if mpResp.Code != http.StatusOK {
		t.Fatalf("expected mp login without captcha 200, got %d: %s", mpResp.Code, mpResp.Body.String())
	}
}

func TestAdminLoginRequiresValidCaptcha(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	missingResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", map[string]any{
		"username": testApp.cfg.AdminUsername,
		"password": testApp.cfg.AdminPassword,
	}, nil)
	if missingResp.Code != http.StatusBadRequest || !bytes.Contains(missingResp.Body.Bytes(), []byte(`"captcha_required"`)) {
		t.Fatalf("expected admin captcha_required, got %d: %s", missingResp.Code, missingResp.Body.String())
	}

	wrongPurposeID := createTestCaptchaChallenge(t, db, "user_login", "C7D8E")
	wrongPurposeResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", map[string]any{
		"username":     testApp.cfg.AdminUsername,
		"password":     testApp.cfg.AdminPassword,
		"captcha_id":   wrongPurposeID,
		"captcha_code": "C7D8E",
	}, nil)
	if wrongPurposeResp.Code != http.StatusUnauthorized || !bytes.Contains(wrongPurposeResp.Body.Bytes(), []byte(`"captcha_invalid"`)) {
		t.Fatalf("expected wrong purpose captcha_invalid, got %d: %s", wrongPurposeResp.Code, wrongPurposeResp.Body.String())
	}

	successID := createTestCaptchaChallenge(t, db, "admin_login", "D9E7F")
	okResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", map[string]any{
		"username":     testApp.cfg.AdminUsername,
		"password":     testApp.cfg.AdminPassword,
		"captcha_id":   successID,
		"captcha_code": "D9E7F",
	}, nil)
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d: %s", okResp.Code, okResp.Body.String())
	}
}

func TestLoginRememberLoginExtendsSessionDurations(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.cfg.UserSessionHours = 2
	testApp.cfg.UserRememberSessionHours = 24
	testApp.cfg.AdminSessionHours = 1
	testApp.cfg.AdminRememberSessionHours = 12
	_, _ = createLoggedInUser(t, testApp, "remember_user", "test-password")

	userDefaultResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "remember_user", "test-password"), nil)
	if userDefaultResp.Code != http.StatusOK {
		t.Fatalf("expected default user login 200, got %d: %s", userDefaultResp.Code, userDefaultResp.Body.String())
	}
	assertCookieMaxAge(t, userDefaultResp, userSessionCookie, 2)

	rememberUserPayload := userLoginPayloadWithCaptcha(t, testApp, "remember_user", "test-password")
	rememberUserPayload["remember_login"] = true
	userRememberResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", rememberUserPayload, nil)
	if userRememberResp.Code != http.StatusOK {
		t.Fatalf("expected remembered user login 200, got %d: %s", userRememberResp.Code, userRememberResp.Body.String())
	}
	assertCookieMaxAge(t, userRememberResp, userSessionCookie, 24)
	var userSession UserSession
	if err := db.Order("id desc").First(&userSession).Error; err != nil {
		t.Fatalf("load remembered user session: %v", err)
	}
	assertExpiryAround(t, userSession.ExpiresAt, 24*time.Hour)

	adminDefaultResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, testApp, testApp.cfg.AdminUsername, testApp.cfg.AdminPassword), nil)
	if adminDefaultResp.Code != http.StatusOK {
		t.Fatalf("expected default admin login 200, got %d: %s", adminDefaultResp.Code, adminDefaultResp.Body.String())
	}
	assertCookieMaxAge(t, adminDefaultResp, adminSessionCookie, 1)

	rememberAdminPayload := adminLoginPayloadWithCaptcha(t, testApp, testApp.cfg.AdminUsername, testApp.cfg.AdminPassword)
	rememberAdminPayload["remember_login"] = true
	adminRememberResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", rememberAdminPayload, nil)
	if adminRememberResp.Code != http.StatusOK {
		t.Fatalf("expected remembered admin login 200, got %d: %s", adminRememberResp.Code, adminRememberResp.Body.String())
	}
	assertCookieMaxAge(t, adminRememberResp, adminSessionCookie, 12)
	var adminSession AdminSession
	if err := db.Order("id desc").First(&adminSession).Error; err != nil {
		t.Fatalf("load remembered admin session: %v", err)
	}
	assertExpiryAround(t, adminSession.ExpiresAt, 12*time.Hour)
}

func assertCookieMaxAge(t *testing.T, resp *httptest.ResponseRecorder, name string, hours int) {
	t.Helper()
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == name {
			if cookie.MaxAge != hours*int(time.Hour/time.Second) {
				t.Fatalf("expected %s MaxAge %d hours, got %d", name, hours, cookie.MaxAge)
			}
			return
		}
	}
	t.Fatalf("expected cookie %s", name)
}

func assertExpiryAround(t *testing.T, expiresAt time.Time, duration time.Duration) {
	t.Helper()
	remaining := time.Until(expiresAt)
	if remaining < duration-time.Minute || remaining > duration+time.Minute {
		t.Fatalf("expected expiry around %s, got remaining %s", duration, remaining)
	}
}

func createTestCaptchaChallenge(t *testing.T, db *gorm.DB, purpose, code string) string {
	t.Helper()
	return createTestCaptchaChallengeWithExpiry(t, db, purpose, code, time.Now().Add(authCaptchaTTL))
}

func createExpiredTestCaptchaChallenge(t *testing.T, db *gorm.DB, purpose, code string) string {
	t.Helper()
	return createTestCaptchaChallengeWithExpiry(t, db, purpose, code, time.Now().Add(-time.Minute))
}

func createTestCaptchaChallengeWithExpiry(t *testing.T, db *gorm.DB, purpose, code string, expiresAt time.Time) string {
	t.Helper()
	captchaID := "test-" + strings.ToLower(purpose) + "-" + uuid.NewString()
	challenge := AuthCaptchaChallenge{
		CaptchaID: captchaID,
		Purpose:   purpose,
		CodeHash:  hashAuthCaptchaCode(captchaID, purpose, code),
		ExpiresAt: expiresAt,
	}
	if err := db.Create(&challenge).Error; err != nil {
		t.Fatalf("create captcha challenge: %v", err)
	}
	return captchaID
}

func userLoginPayloadWithCaptcha(t *testing.T, app *App, username, password string) map[string]any {
	t.Helper()
	code := "A2B3C"
	return map[string]any{
		"username":     username,
		"password":     password,
		"captcha_id":   createTestCaptchaChallenge(t, app.db, "user_login", code),
		"captcha_code": code,
	}
}

func adminLoginPayloadWithCaptcha(t *testing.T, app *App, username, password string) map[string]any {
	t.Helper()
	code := "D4E5F"
	return map[string]any{
		"username":     username,
		"password":     password,
		"captcha_id":   createTestCaptchaChallenge(t, app.db, "admin_login", code),
		"captcha_code": code,
	}
}
