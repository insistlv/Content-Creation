package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type fakeWechatSessionExchanger struct {
	openidByCode map[string]string
	err          error
}

func (f fakeWechatSessionExchanger) Exchange(code string) (wechatSession, error) {
	if f.err != nil {
		return wechatSession{}, f.err
	}
	openid := f.openidByCode[code]
	if openid == "" {
		openid = "openid-" + code
	}
	return wechatSession{OpenID: openid, SessionKey: "session-key-" + code}, nil
}

type fakeWechatPhoneResolver struct {
	phoneByCode map[string]string
	err         error
}

func (f fakeWechatPhoneResolver) ResolvePhone(code string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.phoneByCode[code], nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withWechatHTTPRoundTripper(t *testing.T, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(fn)}
	t.Cleanup(func() {
		http.DefaultClient = originalClient
	})
}

func wechatJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type fakeWechatPaymentClient struct {
	createParams wechatPayRequestParams
	queryResult  wechatPayQueryResult
}

func (f fakeWechatPaymentClient) CreateJSAPIOrder(order FinanceOrder, openid string) (wechatPayRequestParams, string, error) {
	return f.createParams, "prepay-" + order.OrderNumber, nil
}

func (f fakeWechatPaymentClient) QueryOrder(orderNumber string) (wechatPayQueryResult, error) {
	result := f.queryResult
	if result.OutTradeNo == "" {
		result.OutTradeNo = orderNumber
	}
	return result, nil
}

type fakeWechatVirtualPayClient struct {
	queryResult wechatVirtualPayQueryResult
	queryErr    error
	notifyErr   error
	queryCalls  int
	notifyCalls int
}

func (f *fakeWechatVirtualPayClient) QueryOrder(order FinanceOrder) (wechatVirtualPayQueryResult, error) {
	f.queryCalls += 1
	if f.queryErr != nil {
		return wechatVirtualPayQueryResult{}, f.queryErr
	}
	result := f.queryResult
	if result.OrderID == "" {
		result.OrderID = order.OrderNumber
	}
	if result.OpenID == "" {
		result.OpenID = order.WechatOpenID
	}
	return result, nil
}

func (f *fakeWechatVirtualPayClient) NotifyProvideGoods(order FinanceOrder, result wechatVirtualPayQueryResult) error {
	f.notifyCalls += 1
	return f.notifyErr
}

func TestWechatLoginAutoCreatesAndReusesOpenIDUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"new-code": "wx-openid-1",
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-login", map[string]any{
		"code": "new-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected first wechat login 201, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"auth_token"`) || !strings.Contains(resp.Body.String(), `"wechat_openid_bound":true`) {
		t.Fatalf("expected mp auth payload and openid bound flag, got %s", resp.Body.String())
	}
	var payload struct {
		AvailableCredits int `json:"available_credits"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode wechat login payload: %v", err)
	}
	if payload.AvailableCredits != 5 {
		t.Fatalf("expected signup bonus credits 5, got %s", resp.Body.String())
	}

	var user User
	if err := db.Where("wechat_open_id = ?", "wx-openid-1").First(&user).Error; err != nil {
		t.Fatalf("expected user bound to openid: %v", err)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("expected initial credit balance: %v", err)
	}
	if balance.AvailableCredits != 5 {
		t.Fatalf("expected initial signup bonus balance 5, got %+v", balance)
	}
	assertSignupBonusTransaction(t, db, user.ID)

	repeatResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-login", map[string]any{
		"code": "new-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if repeatResp.Code != http.StatusOK {
		t.Fatalf("expected existing wechat login 200, got %d: %s", repeatResp.Code, repeatResp.Body.String())
	}
	if count := countSignupBonusTransactions(t, db, user.ID); count != 1 {
		t.Fatalf("expected repeat login not to grant signup bonus again, got %d", count)
	}
	var userCount int64
	if err := db.Model(&User{}).Where("wechat_open_id = ?", "wx-openid-1").Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected repeat login to reuse user, got %d users", userCount)
	}
}

func TestWechatPhoneLoginBindsExistingPhoneUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "phone_web_user", "test-password")
	phone := "13800138000"
	if err := db.Model(&user).Update("phone", phone).Error; err != nil {
		t.Fatalf("seed phone: %v", err)
	}
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-code": "wx-phone-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected wechat phone login 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"auth_token"`) ||
		!strings.Contains(resp.Body.String(), `"phone":"13800138000"`) ||
		!strings.Contains(resp.Body.String(), `"wechat_openid_bound":true`) {
		t.Fatalf("expected mp auth payload with phone and openid binding, got %s", resp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.WechatOpenID != "wx-phone-openid" {
		t.Fatalf("expected user openid bound, got %q", reloaded.WechatOpenID)
	}
	var userCount int64
	if err := db.Model(&User{}).Where("phone = ?", phone).Count(&userCount).Error; err != nil {
		t.Fatalf("count phone users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected no duplicate phone user, got %d", userCount)
	}
}

func TestWechatPhoneLoginAutoRegistersPhoneUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	phone := "13900139000"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-code": "wx-new-phone-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected wechat phone login to auto-register 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		UserID              uint    `json:"user_id"`
		Username            string  `json:"username"`
		DisplayName         string  `json:"display_name"`
		Phone               *string `json:"phone"`
		WechatOpenIDBound   bool    `json:"wechat_openid_bound"`
		AvailableCredits    int     `json:"available_credits"`
		AuthToken           string  `json:"auth_token"`
		AuthTokenExpiresAt  string  `json:"auth_expires_at"`
		PaymentPasswordFlag bool    `json:"payment_password_enabled"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode auth payload: %v", err)
	}
	if payload.UserID == 0 || payload.AuthToken == "" || payload.AuthTokenExpiresAt == "" {
		t.Fatalf("expected mini program auth token payload, got %s", resp.Body.String())
	}
	if payload.Phone == nil || *payload.Phone != phone || !payload.WechatOpenIDBound {
		t.Fatalf("expected phone and openid binding in payload, got %s", resp.Body.String())
	}
	if payload.DisplayName != "微信用户" || !strings.HasPrefix(payload.Username, "wxp_") {
		t.Fatalf("expected generated wxp user, got username=%q display=%q", payload.Username, payload.DisplayName)
	}
	if payload.AvailableCredits != 5 || payload.PaymentPasswordFlag {
		t.Fatalf("expected fresh account defaults, got %s", resp.Body.String())
	}

	var user User
	if err := db.Preload("UserRole").Where("phone = ?", phone).First(&user).Error; err != nil {
		t.Fatalf("expected persisted phone user: %v", err)
	}
	if user.WechatOpenID != "wx-new-phone-openid" || user.UserRole.Code != "standard_user" {
		t.Fatalf("expected standard user with openid, got openid=%q role=%q", user.WechatOpenID, user.UserRole.Code)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("expected initial credit balance: %v", err)
	}
	if balance.AvailableCredits != 5 {
		t.Fatalf("expected initial signup bonus balance 5, got %+v", balance)
	}
	assertSignupBonusTransaction(t, db, user.ID)

	repeatResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if repeatResp.Code != http.StatusOK {
		t.Fatalf("expected existing wechat phone login 200, got %d: %s", repeatResp.Code, repeatResp.Body.String())
	}
	if count := countSignupBonusTransactions(t, db, user.ID); count != 1 {
		t.Fatalf("expected repeat wechat phone login not to grant signup bonus again, got %d", count)
	}
	var repeatBalance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&repeatBalance).Error; err != nil {
		t.Fatalf("load repeat login balance: %v", err)
	}
	if repeatBalance.AvailableCredits != 5 {
		t.Fatalf("expected repeat login balance to remain 5, got %+v", repeatBalance)
	}
}

func TestWechatPhoneLoginRejectsPhoneBoundToOtherOpenID(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "phone_conflict_user", "test-password")
	phone := "13700137000"
	if err := db.Model(&user).Updates(map[string]any{
		"phone":          phone,
		"wechat_open_id": "wx-existing-phone-openid",
	}).Error; err != nil {
		t.Fatalf("seed phone openid: %v", err)
	}
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-code": "wx-different-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected phone openid conflict 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_openid_conflict"`) {
		t.Fatalf("expected clear conflict code, got %s", resp.Body.String())
	}
}

func TestWechatPhoneLoginUsesExistingOpenIDBindingWhenPhoneBelongsToAnotherUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "openid_owner", "test-password")
	if err := db.Model(&owner).Update("wechat_open_id", "wx-taken-openid").Error; err != nil {
		t.Fatalf("seed owner openid: %v", err)
	}
	target, _ := createLoggedInUser(t, testApp, "openid_target", "test-password")
	phone := "13600136000"
	if err := db.Model(&target).Update("phone", phone).Error; err != nil {
		t.Fatalf("seed target phone: %v", err)
	}
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-code": "wx-taken-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected existing openid binding to login 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		UserID            uint   `json:"user_id"`
		Username          string `json:"username"`
		WechatOpenIDBound bool   `json:"wechat_openid_bound"`
		AuthToken         string `json:"auth_token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode auth payload: %v", err)
	}
	if payload.UserID != owner.ID || payload.Username != owner.Username || !payload.WechatOpenIDBound || payload.AuthToken == "" {
		t.Fatalf("expected login as existing openid owner, got %s", resp.Body.String())
	}
	var reloadedTarget User
	if err := db.First(&reloadedTarget, target.ID).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if reloadedTarget.WechatOpenID != "" {
		t.Fatalf("expected target to remain unbound, got %q", reloadedTarget.WechatOpenID)
	}
}

func TestWechatPhoneLoginBindsPhoneToExistingOpenIDUserWhenPhoneIsFree(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "openid_without_phone", "test-password")
	if err := db.Model(&user).Update("wechat_open_id", "wx-existing-openid-no-phone").Error; err != nil {
		t.Fatalf("seed openid: %v", err)
	}
	phone := "13500135000"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-code": "wx-existing-openid-no-phone",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected existing openid login 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"phone":"13500135000"`) ||
		!strings.Contains(resp.Body.String(), `"auth_token"`) {
		t.Fatalf("expected auth payload with newly bound phone, got %s", resp.Body.String())
	}
	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Phone == nil || *reloaded.Phone != phone {
		t.Fatalf("expected phone to bind to existing openid user, got %#v", reloaded.Phone)
	}
}

func TestWechatPhoneLoginRejectsInvalidAuthorizedPhoneWithoutCreatingUser(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolver wechatPhoneResolver
	}{
		{
			name: "resolver failure",
			resolver: fakeWechatPhoneResolver{
				err: errors.New("wechat phone unavailable"),
			},
		},
		{
			name: "invalid phone",
			resolver: fakeWechatPhoneResolver{phoneByCode: map[string]string{
				"phone-code": "12800128000",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testApp, db := newTestApp(t, &stubProvider{})
			testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
				"login-code": "wx-invalid-phone-openid",
			}}
			testApp.wechatPhoneResolver = tc.resolver

			resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
				"code":       "login-code",
				"phone_code": "phone-code",
			}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
			if resp.Code != http.StatusBadGateway {
				t.Fatalf("expected phone authorization failure 502, got %d: %s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), `"code":"wechat_phone_failed"`) {
				t.Fatalf("expected clear phone failure code, got %s", resp.Body.String())
			}

			var userCount int64
			if err := db.Model(&User{}).Where("wechat_open_id = ?", "wx-invalid-phone-openid").Count(&userCount).Error; err != nil {
				t.Fatalf("count users: %v", err)
			}
			if userCount != 0 {
				t.Fatalf("expected invalid phone flow to create no users, got %d", userCount)
			}
		})
	}
}

func TestWechatPhoneLoginClassifiesWechatResolverFailureAndLogsDetail(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	testApp.cfg.WechatPayAppID = "wx-test-app"
	testApp.cfg.WechatAppSecret = "wechat-secret"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"login-code": "wx-invalid-phone-code-openid",
	}}
	testApp.wechatPhoneResolver = &httpWechatPhoneResolver{app: testApp}

	withWechatHTTPRoundTripper(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/cgi-bin/token":
			return wechatJSONResponse(http.StatusOK, `{"access_token":"token-1","expires_in":7200}`), nil
		case "/wxa/business/getuserphonenumber":
			if got := req.URL.Query().Get("access_token"); got != "token-1" {
				t.Fatalf("unexpected phone access token %q", got)
			}
			return wechatJSONResponse(http.StatusOK, `{"errcode":40029,"errmsg":"invalid code","rid":"rid-invalid-code"}`), nil
		default:
			t.Fatalf("unexpected wechat request path %s", req.URL.Path)
			return nil, nil
		}
	})

	const requestID = "req-wechat-phone-login-invalid-code"
	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-phone-login", map[string]any{
		"code":       "login-code",
		"phone_code": "phone-code-secret",
	}, nil, map[string]string{
		"X-Image-Agent-Client": "mp-weixin",
		"X-Request-ID":         requestID,
	})
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected phone authorization failure 502, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_phone_code_invalid"`) {
		t.Fatalf("expected invalid phone code error, got %s", resp.Body.String())
	}

	var logItem SystemRequestLog
	if err := db.Where("request_id = ?", requestID).First(&logItem).Error; err != nil {
		t.Fatalf("load request log: %v", err)
	}
	if logItem.ErrorCode != "wechat_phone_code_invalid" || logItem.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected classified request log, got %+v", logItem)
	}
	for _, marker := range []string{"errcode=40029", "errmsg=invalid code", "rid=rid-invalid-code", "http_status=200"} {
		if !strings.Contains(logItem.ErrorDetail, marker) {
			t.Fatalf("expected error detail to contain %q, got %q", marker, logItem.ErrorDetail)
		}
	}
	for _, secret := range []string{"token-1", "phone-code-secret", "13800138000"} {
		if strings.Contains(logItem.ErrorDetail, secret) {
			t.Fatalf("request log detail must not contain secret %q: %q", secret, logItem.ErrorDetail)
		}
	}
}

func TestBindWechatPhoneBindsCurrentAccount(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "wechat_phone_bind_current", "test-password")
	phone := "13800138500"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"bind-code": "wx-current-account-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/account/wechat-phone", map[string]any{
		"code":       "bind-code",
		"phone_code": "phone-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected current account wechat phone bind 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		UserID            uint    `json:"user_id"`
		Phone             *string `json:"phone"`
		WechatOpenIDBound bool    `json:"wechat_openid_bound"`
		AuthToken         string  `json:"auth_token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode bind payload: %v", err)
	}
	if payload.UserID != user.ID || payload.Phone == nil || *payload.Phone != phone || !payload.WechatOpenIDBound {
		t.Fatalf("expected current account payload with phone and openid, got %s", resp.Body.String())
	}
	if payload.AuthToken != "" {
		t.Fatalf("expected account payload without login auth token, got %s", resp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Phone == nil || *reloaded.Phone != phone || reloaded.WechatOpenID != "wx-current-account-openid" {
		t.Fatalf("expected current user phone/openid binding, got phone=%#v openid=%q", reloaded.Phone, reloaded.WechatOpenID)
	}

	var userCount int64
	if err := db.Model(&User{}).Where("phone = ?", phone).Count(&userCount).Error; err != nil {
		t.Fatalf("count phone users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected no new account for phone binding, got %d users", userCount)
	}
}

func TestBindWechatPhoneClassifiesCapabilityFailureAndLogsDetail(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "wechat_phone_bind_capability", "test-password")
	testApp.cfg.WechatPayAppID = "wx-test-app"
	testApp.cfg.WechatAppSecret = "wechat-secret"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"bind-code": "wx-capability-openid",
	}}
	testApp.wechatPhoneResolver = &httpWechatPhoneResolver{app: testApp}

	withWechatHTTPRoundTripper(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/cgi-bin/token":
			return wechatJSONResponse(http.StatusOK, `{"access_token":"token-capability","expires_in":7200}`), nil
		case "/wxa/business/getuserphonenumber":
			return wechatJSONResponse(http.StatusOK, `{"errcode":48001,"errmsg":"api unauthorized","rid":"rid-capability"}`), nil
		default:
			t.Fatalf("unexpected wechat request path %s", req.URL.Path)
			return nil, nil
		}
	})

	const requestID = "req-wechat-phone-bind-capability"
	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/account/wechat-phone", map[string]any{
		"code":       "bind-code",
		"phone_code": "phone-code-secret",
	}, cookies, map[string]string{
		"X-Image-Agent-Client": "mp-weixin",
		"X-Request-ID":         requestID,
	})
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected phone authorization failure 502, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_phone_capability_unavailable"`) {
		t.Fatalf("expected capability unavailable error, got %s", resp.Body.String())
	}

	var logItem SystemRequestLog
	if err := db.Where("request_id = ?", requestID).First(&logItem).Error; err != nil {
		t.Fatalf("load request log: %v", err)
	}
	if logItem.ErrorCode != "wechat_phone_capability_unavailable" || logItem.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected classified request log, got %+v", logItem)
	}
	for _, marker := range []string{"errcode=48001", "errmsg=api unauthorized", "rid=rid-capability", "http_status=200"} {
		if !strings.Contains(logItem.ErrorDetail, marker) {
			t.Fatalf("expected error detail to contain %q, got %q", marker, logItem.ErrorDetail)
		}
	}
	for _, secret := range []string{"token-capability", "phone-code-secret"} {
		if strings.Contains(logItem.ErrorDetail, secret) {
			t.Fatalf("request log detail must not contain secret %q: %q", secret, logItem.ErrorDetail)
		}
	}
}

func TestBindWechatPhoneRejectsAlreadyBoundCurrentAccount(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "wechat_phone_bind_already", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800138501")
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"bind-code": "wx-current-account-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": "13800138502",
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/account/wechat-phone", map[string]any{
		"code":       "bind-code",
		"phone_code": "phone-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected already bound account 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"phone_already_bound"`) {
		t.Fatalf("expected phone_already_bound code, got %s", resp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Phone == nil || *reloaded.Phone != "13800138501" || reloaded.WechatOpenID != "" {
		t.Fatalf("expected current account unchanged, got phone=%#v openid=%q", reloaded.Phone, reloaded.WechatOpenID)
	}
}

func TestBindWechatPhoneRejectsPhoneOwnedByOtherAccount(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "wechat_phone_bind_owner", "test-password")
	phone := "13800138503"
	setUserPhoneForTest(t, testApp, owner.ID, phone)
	current, cookies := createLoggedInUser(t, testApp, "wechat_phone_bind_other", "test-password")
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"bind-code": "wx-current-account-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": phone,
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/account/wechat-phone", map[string]any{
		"code":       "bind-code",
		"phone_code": "phone-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected phone owner conflict 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"phone_exists"`) {
		t.Fatalf("expected phone_exists code, got %s", resp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, current.ID).Error; err != nil {
		t.Fatalf("reload current user: %v", err)
	}
	if reloaded.Phone != nil || reloaded.WechatOpenID != "" {
		t.Fatalf("expected current account unchanged, got phone=%#v openid=%q", reloaded.Phone, reloaded.WechatOpenID)
	}
}

func TestBindWechatPhoneRejectsOpenIDOwnedByOtherAccount(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "wechat_openid_bind_owner", "test-password")
	if err := db.Model(&owner).Update("wechat_open_id", "wx-taken-bind-openid").Error; err != nil {
		t.Fatalf("seed owner openid: %v", err)
	}
	current, cookies := createLoggedInUser(t, testApp, "wechat_openid_bind_other", "test-password")
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"bind-code": "wx-taken-bind-openid",
	}}
	testApp.wechatPhoneResolver = fakeWechatPhoneResolver{phoneByCode: map[string]string{
		"phone-code": "13800138504",
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/account/wechat-phone", map[string]any{
		"code":       "bind-code",
		"phone_code": "phone-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected openid owner conflict 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_openid_conflict"`) {
		t.Fatalf("expected wechat_openid_conflict code, got %s", resp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, current.ID).Error; err != nil {
		t.Fatalf("reload current user: %v", err)
	}
	if reloaded.Phone != nil || reloaded.WechatOpenID != "" {
		t.Fatalf("expected current account unchanged, got phone=%#v openid=%q", reloaded.Phone, reloaded.WechatOpenID)
	}
}

func TestBindWechatPhoneRejectsInvalidAuthorizedPhone(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolver wechatPhoneResolver
	}{
		{
			name: "resolver failure",
			resolver: fakeWechatPhoneResolver{
				err: errors.New("wechat phone unavailable"),
			},
		},
		{
			name: "invalid phone",
			resolver: fakeWechatPhoneResolver{phoneByCode: map[string]string{
				"phone-code": "12800128000",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testApp, db := newTestApp(t, &stubProvider{})
			user, cookies := createLoggedInUser(t, testApp, "wechat_phone_bind_invalid_"+strings.ReplaceAll(tc.name, " ", "_"), "test-password")
			testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
				"bind-code": "wx-invalid-bind-openid",
			}}
			testApp.wechatPhoneResolver = tc.resolver

			resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/account/wechat-phone", map[string]any{
				"code":       "bind-code",
				"phone_code": "phone-code",
			}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
			if resp.Code != http.StatusBadGateway {
				t.Fatalf("expected phone authorization failure 502, got %d: %s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), `"code":"wechat_phone_failed"`) {
				t.Fatalf("expected clear phone failure code, got %s", resp.Body.String())
			}

			var reloaded User
			if err := db.First(&reloaded, user.ID).Error; err != nil {
				t.Fatalf("reload current user: %v", err)
			}
			if reloaded.Phone != nil || reloaded.WechatOpenID != "" {
				t.Fatalf("expected current account unchanged, got phone=%#v openid=%q", reloaded.Phone, reloaded.WechatOpenID)
			}
		})
	}
}

func TestWechatPhoneResolverRefreshesAccessTokenAndRetriesTokenInvalid(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.WechatPayAppID = "wx-test-app"
	testApp.cfg.WechatAppSecret = "wechat-secret"
	resolver := &httpWechatPhoneResolver{
		app:       testApp,
		token:     "stale-token",
		expiresAt: time.Now().Add(time.Hour),
	}
	var tokenRequests int
	var phoneTokens []string

	withWechatHTTPRoundTripper(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/cgi-bin/token":
			tokenRequests++
			return wechatJSONResponse(http.StatusOK, `{"access_token":"fresh-token","expires_in":7200}`), nil
		case "/wxa/business/getuserphonenumber":
			token := req.URL.Query().Get("access_token")
			phoneTokens = append(phoneTokens, token)
			if token == "stale-token" {
				return wechatJSONResponse(http.StatusOK, `{"errcode":40001,"errmsg":"invalid credential","rid":"rid-stale-token"}`), nil
			}
			if token == "fresh-token" {
				return wechatJSONResponse(http.StatusOK, `{"errcode":0,"phone_info":{"purePhoneNumber":"13800138000","countryCode":"86"}}`), nil
			}
			t.Fatalf("unexpected phone access token %q", token)
			return nil, nil
		default:
			t.Fatalf("unexpected wechat request path %s", req.URL.Path)
			return nil, nil
		}
	})

	phone, err := resolver.ResolvePhone("phone-code")
	if err != nil {
		t.Fatalf("expected phone resolve retry to succeed, got %v", err)
	}
	if phone != "13800138000" {
		t.Fatalf("expected resolved phone, got %q", phone)
	}
	if tokenRequests != 1 {
		t.Fatalf("expected one token refresh, got %d", tokenRequests)
	}
	if len(phoneTokens) != 2 || phoneTokens[0] != "stale-token" || phoneTokens[1] != "fresh-token" {
		t.Fatalf("expected stale then fresh token phone requests, got %#v", phoneTokens)
	}
}

func TestWechatBindRejectsOpenIDAlreadyBoundToOtherUser(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	owner, _ := createLoggedInUser(t, testApp, "wechat_owner", "test-password")
	if err := db.Model(&owner).Update("wechat_open_id", "wx-conflict").Error; err != nil {
		t.Fatalf("seed openid: %v", err)
	}
	_, cookies := createLoggedInUser(t, testApp, "wechat_other", "test-password")
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"conflict-code": "wx-conflict",
	}}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/wechat-bind", map[string]any{
		"code": "conflict-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected openid conflict 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_openid_conflict"`) {
		t.Fatalf("expected clear conflict code, got %s", resp.Body.String())
	}
}

func TestWechatPayOrderRequiresConfigAndBoundOpenID(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "wechat_pay_unbound", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139401")

	var packages struct {
		Items []Package `json:"items"`
	}
	packagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/packages", nil, nil)
	if err := json.Unmarshal(packagesResp.Body.Bytes(), &packages); err != nil || len(packages.Items) == 0 {
		t.Fatalf("load packages: %v %s", err, packagesResp.Body.String())
	}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/orders", map[string]any{
		"package_id": packages.Items[0].ID,
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing config 503 before order creation, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_pay_not_configured"`) {
		t.Fatalf("expected wechat_pay_not_configured, got %s", resp.Body.String())
	}

	testApp.cfg.WechatPayAppID = "wx-app"
	testApp.cfg.WechatPayMchID = "mch-1"
	testApp.cfg.WechatPayMchCertSerialNo = "serial-1"
	testApp.cfg.WechatPayMchPrivateKey = "private-key"
	testApp.cfg.WechatPayAPIv3Key = "api-v3-key"
	testApp.cfg.WechatPayNotifyURL = "https://example.com/api/payments/wechat/notify"
	resp = performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/orders", map[string]any{
		"package_id": packages.Items[0].ID,
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected unbound openid 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_openid_required"`) {
		t.Fatalf("expected openid required code, got %s", resp.Body.String())
	}
}

func TestWechatPayCreateOrderAndQueryCreditsIdempotently(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "wechat_pay_buyer", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	setUserPhoneForTest(t, testApp, user.ID, "13800139402")
	if err := db.Model(&user).Update("wechat_open_id", "wx-pay-openid").Error; err != nil {
		t.Fatalf("bind openid: %v", err)
	}
	testApp.cfg.WechatPayAppID = "wx-app"
	testApp.cfg.WechatPayMchID = "mch-1"
	testApp.cfg.WechatPayMchCertSerialNo = "serial-1"
	testApp.cfg.WechatPayMchPrivateKey = "private-key"
	testApp.cfg.WechatPayAPIv3Key = "api-v3-key"
	testApp.cfg.WechatPayNotifyURL = "https://example.com/api/payments/wechat/notify"
	testApp.wechatPayClient = fakeWechatPaymentClient{
		createParams: wechatPayRequestParams{
			TimeStamp: "1760000000",
			NonceStr:  "nonce-1",
			Package:   "prepay_id=prepay-1",
			SignType:  "RSA",
			PaySign:   "pay-sign-1",
		},
		queryResult: wechatPayQueryResult{
			AppID:         "wx-app",
			MchID:         "mch-1",
			TradeState:    "SUCCESS",
			TransactionID: "4200000001",
			PayerOpenID:   "wx-pay-openid",
			AmountCents:   3900,
			SuccessTime:   time.Now().UTC(),
		},
	}

	var packages struct {
		Items []Package `json:"items"`
	}
	packagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/packages", nil, nil)
	if err := json.Unmarshal(packagesResp.Body.Bytes(), &packages); err != nil || len(packages.Items) == 0 {
		t.Fatalf("load packages: %v %s", err, packagesResp.Body.String())
	}
	pkg := packages.Items[0]

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/orders", map[string]any{
		"package_id": pkg.ID,
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected wechat order 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Order         FinanceOrder           `json:"order"`
		PaymentParams wechatPayRequestParams `json:"payment_params"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if payload.Order.PaymentMethod != FinancePaymentMethodWechatJSAPI ||
		payload.Order.PaymentStatus != FinancePaymentStatusPending ||
		payload.Order.PackageCredits != pkg.Credits ||
		payload.PaymentParams.SignType != "RSA" ||
		payload.PaymentParams.Package == "" ||
		payload.PaymentParams.PaySign == "" {
		t.Fatalf("unexpected wechat create payload: %+v", payload)
	}

	queryPath := fmt.Sprintf("/api/payments/wechat/orders/%s/query", payload.Order.OrderNumber)
	queryResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, queryPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if queryResp.Code != http.StatusOK {
		t.Fatalf("expected wechat query 200, got %d: %s", queryResp.Code, queryResp.Body.String())
	}
	if !strings.Contains(queryResp.Body.String(), `"payment_status":"paid"`) ||
		!strings.Contains(queryResp.Body.String(), fmt.Sprintf(`"available_credits":%d`, pkg.Credits)) {
		t.Fatalf("expected paid order and credits in query response: %s", queryResp.Body.String())
	}
	repeatResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, queryPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if repeatResp.Code != http.StatusOK {
		t.Fatalf("expected repeat query 200, got %d: %s", repeatResp.Code, repeatResp.Body.String())
	}
	var transactions []CreditTransaction
	if err := db.Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypePaymentTopUp).Find(&transactions).Error; err != nil {
		t.Fatalf("load topup transactions: %v", err)
	}
	if len(transactions) != 1 || transactions[0].Amount != pkg.Credits {
		t.Fatalf("expected exactly one payment topup transaction, got %+v", transactions)
	}
}

func TestWechatVirtualPayOrderRequiresConfigProductAndMatchingOpenID(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "wechat_virtual_missing", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139403")
	if err := db.Model(&user).Update("wechat_open_id", "wx-bound-openid").Error; err != nil {
		t.Fatalf("bind openid: %v", err)
	}
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"buyer-code":    "wx-bound-openid",
		"mismatch-code": "wx-other-openid",
	}}

	var packages struct {
		Items []Package `json:"items"`
	}
	packagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/packages", nil, nil)
	if err := json.Unmarshal(packagesResp.Body.Bytes(), &packages); err != nil || len(packages.Items) == 0 {
		t.Fatalf("load packages: %v %s", err, packagesResp.Body.String())
	}
	pkg := packages.Items[0]
	if err := db.Model(&Package{}).Where("id = ?", pkg.ID).Update("wechat_virtual_product_id", "").Error; err != nil {
		t.Fatalf("clear virtual product id: %v", err)
	}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id": pkg.ID,
		"code":       "buyer-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing virtual config 503, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_virtual_pay_not_configured"`) {
		t.Fatalf("expected virtual config error, got %s", resp.Body.String())
	}

	testApp.cfg.WechatVirtualPayOfferID = "offer-1001"
	testApp.cfg.WechatVirtualPayAppKey = "virtual-app-key"
	testApp.cfg.WechatPayAppID = "wx-app"
	testApp.cfg.WechatAppSecret = "app-secret"
	resp = performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id": pkg.ID,
		"code":       "buyer-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing product id 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_virtual_product_required"`) {
		t.Fatalf("expected product id error, got %s", resp.Body.String())
	}

	if err := db.Model(&Package{}).Where("id = ?", pkg.ID).Update("wechat_virtual_product_id", "goods-1001").Error; err != nil {
		t.Fatalf("set virtual product id: %v", err)
	}
	resp = performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id": pkg.ID,
		"code":       "mismatch-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected openid mismatch 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_openid_mismatch"`) {
		t.Fatalf("expected openid mismatch error, got %s", resp.Body.String())
	}
}

func TestWechatVirtualPayCreateOrderSignsAndConfirmsIdempotently(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "wechat_virtual_buyer", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	setUserPhoneForTest(t, testApp, user.ID, "13800139404")
	if err := db.Model(&user).Update("wechat_open_id", "wx-virtual-openid").Error; err != nil {
		t.Fatalf("bind openid: %v", err)
	}
	testApp.cfg.WechatVirtualPayOfferID = "offer-1001"
	testApp.cfg.WechatVirtualPayAppKey = "virtual-app-key"
	testApp.cfg.WechatVirtualPayEnv = 0
	testApp.cfg.WechatPayAppID = "wx-app"
	testApp.cfg.WechatAppSecret = "app-secret"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"virtual-code": "wx-virtual-openid",
	}}
	virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
		Status:    wechatVirtualOrderStatusPaidPendingDelivery,
		PaidFee:   3900,
		OrderFee:  3900,
		PaidTime:  time.Now().Unix(),
		WXOrderID: "wx-virtual-order-1",
	}}
	testApp.wechatVirtualPayClient = virtualClient

	var pkg Package
	if err := db.Where("is_active = ?", true).Order("sort_order asc, id asc").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if err := db.Model(&pkg).Update("wechat_virtual_product_id", "goods-1001").Error; err != nil {
		t.Fatalf("set virtual product id: %v", err)
	}

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id": pkg.ID,
		"code":       "virtual-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected virtual order 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Order         FinanceOrder               `json:"order"`
		PaymentParams wechatVirtualPaymentParams `json:"payment_params"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if payload.Order.PaymentMethod != FinancePaymentMethodWechatVirtualGoods ||
		payload.Order.PaymentStatus != FinancePaymentStatusPending ||
		payload.Order.WechatVirtualProductID != "goods-1001" ||
		payload.PaymentParams.Mode != "short_series_goods" ||
		payload.PaymentParams.SignData == "" ||
		payload.PaymentParams.PaySig == "" ||
		payload.PaymentParams.Signature == "" {
		t.Fatalf("unexpected virtual create payload: %+v", payload)
	}
	if !strings.Contains(payload.PaymentParams.SignData, `"offerId":"offer-1001"`) ||
		!strings.Contains(payload.PaymentParams.SignData, `"productId":"goods-1001"`) ||
		!strings.Contains(payload.PaymentParams.SignData, `"env":0`) ||
		!strings.Contains(payload.PaymentParams.SignData, fmt.Sprintf(`"goodsPrice":%d`, pkg.PriceCents)) ||
		!strings.Contains(payload.PaymentParams.SignData, fmt.Sprintf(`"outTradeNo":"%s"`, payload.Order.OrderNumber)) {
		t.Fatalf("signData missing expected goods payload: %s", payload.PaymentParams.SignData)
	}
	expectedPaySig := testHMACHex("virtual-app-key", "requestVirtualPayment&"+payload.PaymentParams.SignData)
	expectedSignature := testHMACHex("session-key-virtual-code", payload.PaymentParams.SignData)
	if payload.PaymentParams.PaySig != expectedPaySig || payload.PaymentParams.Signature != expectedSignature {
		t.Fatalf("unexpected signatures paySig=%s signature=%s", payload.PaymentParams.PaySig, payload.PaymentParams.Signature)
	}

	confirmPath := fmt.Sprintf("/api/payments/wechat/virtual-orders/%s/confirm", payload.Order.OrderNumber)
	confirmResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, confirmPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if confirmResp.Code != http.StatusOK {
		t.Fatalf("expected confirm 200, got %d: %s", confirmResp.Code, confirmResp.Body.String())
	}
	if !strings.Contains(confirmResp.Body.String(), `"payment_status":"paid"`) ||
		!strings.Contains(confirmResp.Body.String(), fmt.Sprintf(`"available_credits":%d`, pkg.Credits)) {
		t.Fatalf("expected paid virtual order and credits: %s", confirmResp.Body.String())
	}
	repeatResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, confirmPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if repeatResp.Code != http.StatusOK {
		t.Fatalf("expected repeat confirm 200, got %d: %s", repeatResp.Code, repeatResp.Body.String())
	}
	if virtualClient.queryCalls != 1 || virtualClient.notifyCalls != 1 {
		t.Fatalf("expected one query and one provide-goods notification, got query=%d notify=%d", virtualClient.queryCalls, virtualClient.notifyCalls)
	}
	var transactions []CreditTransaction
	if err := db.Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypePaymentTopUp).Find(&transactions).Error; err != nil {
		t.Fatalf("load transactions: %v", err)
	}
	if len(transactions) != 1 || transactions[0].Amount != pkg.Credits || !strings.Contains(transactions[0].Reason, "虚拟支付") {
		t.Fatalf("expected exactly one virtual payment topup, got %+v", transactions)
	}
}

func TestWechatVirtualPayCreateOrderSyncsReusablePendingOrder(t *testing.T) {
	t.Run("closed reusable order creates replacement and marks stale order closed", func(t *testing.T) {
		testApp, db := newTestApp(t, &stubProvider{})
		_, cookies, staleOrder := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_closed_reuse", "wx-closed-reuse-openid")
		virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
			Status:   wechatVirtualOrderStatusClosed,
			OrderFee: staleOrder.AmountCents,
		}}
		testApp.wechatVirtualPayClient = virtualClient

		resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
			"package_id": staleOrder.PackageID,
			"code":       "virtual-code",
		}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
		if resp.Code != http.StatusCreated {
			t.Fatalf("expected replacement virtual order 201, got %d: %s", resp.Code, resp.Body.String())
		}
		var payload struct {
			Order                    FinanceOrder                `json:"order"`
			PaymentParams            *wechatVirtualPaymentParams `json:"payment_params"`
			PaymentState             string                      `json:"payment_state"`
			RecoveredFromOrderNumber string                      `json:"recovered_from_order_number"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode replacement payload: %v", err)
		}
		if payload.Order.OrderNumber == staleOrder.OrderNumber || payload.PaymentState != "pay_required" ||
			payload.RecoveredFromOrderNumber != staleOrder.OrderNumber || payload.PaymentParams == nil {
			t.Fatalf("expected replacement pay-required payload, got %+v", payload)
		}

		var reloadedStale FinanceOrder
		if err := db.Where("id = ?", staleOrder.ID).First(&reloadedStale).Error; err != nil {
			t.Fatalf("reload stale order: %v", err)
		}
		if reloadedStale.PaymentStatus != FinancePaymentStatusFailed {
			t.Fatalf("expected stale order failed, got %s", reloadedStale.PaymentStatus)
		}
		var stalePayment PaymentRecord
		if err := db.Where("finance_order_id = ?", staleOrder.ID).First(&stalePayment).Error; err != nil {
			t.Fatalf("load stale payment record: %v", err)
		}
		if stalePayment.Status != PaymentRecordStatusClosed || !strings.Contains(stalePayment.QuerySummary, "status=6") {
			t.Fatalf("expected closed payment record with query summary, got %+v", stalePayment)
		}
		if virtualClient.queryCalls != 1 {
			t.Fatalf("expected one stale order query, got %d", virtualClient.queryCalls)
		}
	})

	t.Run("created reusable order is re-signed without creating a new order", func(t *testing.T) {
		testApp, db := newTestApp(t, &stubProvider{})
		_, cookies, reusableOrder := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_created_reuse", "wx-created-reuse-openid")
		virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
			Status:   wechatVirtualOrderStatusCreated,
			OrderFee: reusableOrder.AmountCents,
		}}
		testApp.wechatVirtualPayClient = virtualClient

		resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
			"package_id": reusableOrder.PackageID,
			"code":       "virtual-code",
		}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
		if resp.Code != http.StatusOK {
			t.Fatalf("expected reused virtual order 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var payload struct {
			Order         FinanceOrder                `json:"order"`
			PaymentParams *wechatVirtualPaymentParams `json:"payment_params"`
			PaymentState  string                      `json:"payment_state"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode reused payload: %v", err)
		}
		if payload.Order.OrderNumber != reusableOrder.OrderNumber || payload.PaymentState != "pay_required" || payload.PaymentParams == nil {
			t.Fatalf("expected existing pay-required order, got %+v", payload)
		}
		var orderCount int64
		if err := db.Model(&FinanceOrder{}).Where("user_id = ? AND package_id = ? AND payment_method = ?", reusableOrder.UserID, reusableOrder.PackageID, FinancePaymentMethodWechatVirtualGoods).Count(&orderCount).Error; err != nil {
			t.Fatalf("count virtual orders: %v", err)
		}
		if orderCount != 1 {
			t.Fatalf("expected no replacement order, got %d orders", orderCount)
		}
		if virtualClient.queryCalls != 1 {
			t.Fatalf("expected one reusable order query, got %d", virtualClient.queryCalls)
		}
	})

	t.Run("paid reusable order grants credits and returns already paid", func(t *testing.T) {
		testApp, db := newTestApp(t, &stubProvider{})
		_, cookies, paidOrder := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_paid_reuse", "wx-paid-reuse-openid")
		virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
			Status:       wechatVirtualOrderStatusPaidPendingDelivery,
			OpenID:       "wx-paid-reuse-openid",
			PaidFee:      paidOrder.AmountCents,
			OrderFee:     paidOrder.AmountCents,
			PaidTime:     time.Now().Unix(),
			WXOrderID:    "wx-paid-reuse-order",
			WXPayOrderID: "wxpay-paid-reuse-order",
		}}
		testApp.wechatVirtualPayClient = virtualClient

		resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
			"package_id": paidOrder.PackageID,
			"code":       "virtual-code",
		}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
		if resp.Code != http.StatusOK {
			t.Fatalf("expected already-paid virtual order 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var payload struct {
			Order            FinanceOrder                `json:"order"`
			PaymentParams    *wechatVirtualPaymentParams `json:"payment_params"`
			PaymentState     string                      `json:"payment_state"`
			AvailableCredits int                         `json:"available_credits"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode already-paid payload: %v", err)
		}
		if payload.Order.OrderNumber != paidOrder.OrderNumber || payload.Order.PaymentStatus != FinancePaymentStatusPaid ||
			payload.PaymentState != "already_paid" || payload.AvailableCredits != paidOrder.PackageCredits || payload.PaymentParams != nil {
			t.Fatalf("expected already-paid payload without payment params, got %+v", payload)
		}
		var orderCount int64
		if err := db.Model(&FinanceOrder{}).Where("user_id = ? AND package_id = ? AND payment_method = ?", paidOrder.UserID, paidOrder.PackageID, FinancePaymentMethodWechatVirtualGoods).Count(&orderCount).Error; err != nil {
			t.Fatalf("count virtual orders: %v", err)
		}
		if orderCount != 1 {
			t.Fatalf("expected no new order for already-paid reusable order, got %d", orderCount)
		}
		var transactionCount int64
		if err := db.Model(&CreditTransaction{}).Where("user_id = ? AND type = ?", paidOrder.UserID, CreditTransactionTypePaymentTopUp).Count(&transactionCount).Error; err != nil {
			t.Fatalf("count transactions: %v", err)
		}
		if transactionCount != 1 || virtualClient.queryCalls != 1 || virtualClient.notifyCalls != 1 {
			t.Fatalf("expected one topup/query/notify, got transactions=%d query=%d notify=%d", transactionCount, virtualClient.queryCalls, virtualClient.notifyCalls)
		}
	})
}

func TestWechatVirtualPayForceNewClosesOwnStaleOrderBeforeRateLimit(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies, staleOrder := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_force_new", "wx-force-new-openid")
	packages := allPackagesForTest(t, testApp)
	if len(packages) < 3 {
		t.Fatalf("expected at least three packages, got %d", len(packages))
	}
	now := time.Now().UTC()
	for index, pkg := range packages[1:3] {
		order := FinanceOrder{
			OrderNumber:            nextFinanceOrderNumber(now.Add(time.Duration(index+1) * time.Second)),
			UserID:                 user.ID,
			PackageID:              pkg.ID,
			PackageName:            pkg.Name,
			PackageCredits:         pkg.Credits,
			AmountCents:            pkg.PriceCents,
			OrderType:              FinanceOrderTypePackage,
			PaymentMethod:          FinancePaymentMethodWechatVirtualGoods,
			PaymentStatus:          FinancePaymentStatusPending,
			InvoiceStatus:          FinanceInvoiceStatusPending,
			IPAddress:              "127.0.0.1",
			WechatOpenID:           staleOrder.WechatOpenID,
			WechatVirtualProductID: "goods-extra",
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		if err := db.Create(&order).Error; err != nil {
			t.Fatalf("create extra pending order: %v", err)
		}
	}
	virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
		Status:   wechatVirtualOrderStatusClosed,
		OrderFee: staleOrder.AmountCents,
	}}
	testApp.wechatVirtualPayClient = virtualClient

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id":         staleOrder.PackageID,
		"code":               "virtual-code",
		"force_new":          true,
		"stale_order_number": staleOrder.OrderNumber,
		"stale_reason":       "ORDER_CLOSED",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected forced replacement 201 without rate limit, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Order                    FinanceOrder `json:"order"`
		PaymentState             string       `json:"payment_state"`
		RecoveredFromOrderNumber string       `json:"recovered_from_order_number"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode forced replacement payload: %v", err)
	}
	if payload.Order.OrderNumber == staleOrder.OrderNumber || payload.PaymentState != "pay_required" ||
		payload.RecoveredFromOrderNumber != staleOrder.OrderNumber {
		t.Fatalf("expected forced replacement payload, got %+v", payload)
	}
	var reloadedStale FinanceOrder
	if err := db.Where("id = ?", staleOrder.ID).First(&reloadedStale).Error; err != nil {
		t.Fatalf("reload stale order: %v", err)
	}
	if reloadedStale.PaymentStatus != FinancePaymentStatusFailed {
		t.Fatalf("expected own stale order failed before replacement, got %s", reloadedStale.PaymentStatus)
	}
}

func TestWechatVirtualPayForceNewDoesNotTouchOtherUsersStaleOrder(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, _, otherOrder := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_other_stale", "wx-other-stale-openid")
	user, cookies := createLoggedInUser(t, testApp, "wechat_virtual_own_force_new", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139405")
	if err := db.Model(&user).Update("wechat_open_id", "wx-own-force-new-openid").Error; err != nil {
		t.Fatalf("bind own openid: %v", err)
	}
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"own-virtual-code": "wx-own-force-new-openid",
	}}
	virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
		Status:   wechatVirtualOrderStatusClosed,
		OrderFee: otherOrder.AmountCents,
	}}
	testApp.wechatVirtualPayClient = virtualClient

	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id":         otherOrder.PackageID,
		"code":               "own-virtual-code",
		"force_new":          true,
		"stale_order_number": otherOrder.OrderNumber,
		"stale_reason":       "ORDER_CLOSED",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected own replacement order 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Order                    FinanceOrder `json:"order"`
		RecoveredFromOrderNumber string       `json:"recovered_from_order_number"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode force-new payload: %v", err)
	}
	if payload.Order.UserID != user.ID || payload.Order.OrderNumber == otherOrder.OrderNumber || payload.RecoveredFromOrderNumber != "" {
		t.Fatalf("expected new own order without recovering from other user's stale order, got %+v", payload)
	}
	var reloadedOther FinanceOrder
	if err := db.Where("id = ?", otherOrder.ID).First(&reloadedOther).Error; err != nil {
		t.Fatalf("reload other order: %v", err)
	}
	if reloadedOther.PaymentStatus != FinancePaymentStatusPending || virtualClient.queryCalls != 0 {
		t.Fatalf("expected other user's stale order untouched and unqueried, order=%+v query=%d", reloadedOther, virtualClient.queryCalls)
	}
}

func TestWechatVirtualPayConfirmPendingDoesNotGrantCredits(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies, order := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_pending", "wx-pending-openid")
	virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
		Status:   wechatVirtualOrderStatusCreated,
		PaidFee:  0,
		OrderFee: order.AmountCents,
	}}
	testApp.wechatVirtualPayClient = virtualClient

	confirmPath := fmt.Sprintf("/api/payments/wechat/virtual-orders/%s/confirm", order.OrderNumber)
	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, confirmPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected pending confirm 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"wechat_virtual_pay_pending"`) ||
		!strings.Contains(resp.Body.String(), `"payment_status":"pending"`) ||
		!strings.Contains(resp.Body.String(), `"available_credits":0`) {
		t.Fatalf("expected pending order without credits, got %s", resp.Body.String())
	}
	if virtualClient.notifyCalls != 0 {
		t.Fatalf("expected no provide-goods notification for pending order, got %d", virtualClient.notifyCalls)
	}
	var transactionCount int64
	if err := db.Model(&CreditTransaction{}).Where("user_id = ? AND type = ?", order.UserID, CreditTransactionTypePaymentTopUp).Count(&transactionCount).Error; err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if transactionCount != 0 {
		t.Fatalf("expected no topup transaction, got %d", transactionCount)
	}
}

func TestWechatVirtualPayConfirmDeliveringStatusGrantsCredits(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies, order := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_delivering", "wx-delivering-openid")
	virtualClient := &fakeWechatVirtualPayClient{queryResult: wechatVirtualPayQueryResult{
		Status:       wechatVirtualOrderStatusDelivering,
		OpenID:       "wx-delivering-openid",
		PaidFee:      order.AmountCents,
		OrderFee:     order.AmountCents,
		PaidTime:     time.Now().Unix(),
		WXOrderID:    "wx-virtual-delivering-order",
		WXPayOrderID: "wxpay-virtual-delivering-order",
	}}
	testApp.wechatVirtualPayClient = virtualClient

	confirmPath := fmt.Sprintf("/api/payments/wechat/virtual-orders/%s/confirm", order.OrderNumber)
	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, confirmPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected delivering confirm 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"payment_status":"paid"`) ||
		!strings.Contains(resp.Body.String(), fmt.Sprintf(`"available_credits":%d`, order.PackageCredits)) {
		t.Fatalf("expected delivering status to grant credits, got %s", resp.Body.String())
	}

	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != order.PackageCredits {
		t.Fatalf("expected credited balance %d, got %d", order.PackageCredits, balance.AvailableCredits)
	}
}

func TestWechatVirtualPaySandboxUsesSandboxAppKeyForPaySig(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.WechatVirtualPayOfferID = "offer-1001"
	testApp.cfg.WechatVirtualPayAppKey = "production-app-key"
	testApp.cfg.WechatVirtualPaySandboxAppKey = "sandbox-app-key"
	testApp.cfg.WechatVirtualPayEnv = 1

	order := FinanceOrder{
		ID:                     42,
		OrderNumber:            "FO-SANDBOX-1",
		AmountCents:            1000,
		WechatVirtualProductID: "goods-1001",
	}
	params, err := testApp.buildWechatVirtualPaymentParams(order, "session-key")
	if err != nil {
		t.Fatalf("build sandbox params: %v", err)
	}
	if !strings.Contains(params.SignData, `"env":1`) {
		t.Fatalf("expected sandbox env in signData: %s", params.SignData)
	}
	expectedSandboxPaySig := testHMACHex("sandbox-app-key", "requestVirtualPayment&"+params.SignData)
	if params.PaySig != expectedSandboxPaySig {
		t.Fatalf("expected sandbox app key paySig, got %s want %s", params.PaySig, expectedSandboxPaySig)
	}
	productionPaySig := testHMACHex("production-app-key", "requestVirtualPayment&"+params.SignData)
	if params.PaySig == productionPaySig {
		t.Fatalf("sandbox paySig unexpectedly used production app key")
	}
}

func TestWechatVirtualPaySandboxUsesSandboxAppKeyForXPayPaySig(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.WechatVirtualPayAppKey = "production-app-key"
	testApp.cfg.WechatVirtualPaySandboxAppKey = "sandbox-app-key"
	testApp.cfg.WechatVirtualPayEnv = 1

	path := "/xpay/query_order"
	body := []byte(`{"openid":"openid-1","env":1,"order_id":"FO-SANDBOX-1"}`)
	got := testApp.wechatVirtualXPaySig(path, body)
	want := testHMACHex("sandbox-app-key", path+"&"+string(body))
	if got != want {
		t.Fatalf("expected sandbox xpay pay_sig, got %s want %s", got, want)
	}
	if got == testHMACHex("production-app-key", path+"&"+string(body)) {
		t.Fatalf("sandbox xpay pay_sig unexpectedly used production app key")
	}
}

func TestWechatVirtualPayConfirmRejectsMismatchAndQueryError(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies, order := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_reject", "wx-reject-openid")

	cases := []struct {
		name       string
		result     wechatVirtualPayQueryResult
		queryErr   error
		wantStatus int
		wantCode   string
	}{
		{
			name: "amount mismatch",
			result: wechatVirtualPayQueryResult{
				Status:   wechatVirtualOrderStatusPaidPendingDelivery,
				OpenID:   "wx-reject-openid",
				PaidFee:  order.AmountCents - 1,
				OrderFee: order.AmountCents,
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "wechat_amount_mismatch",
		},
		{
			name: "openid mismatch",
			result: wechatVirtualPayQueryResult{
				Status:   wechatVirtualOrderStatusPaidPendingDelivery,
				OpenID:   "wx-other-openid",
				PaidFee:  order.AmountCents,
				OrderFee: order.AmountCents,
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "wechat_openid_mismatch",
		},
		{
			name:       "query error",
			queryErr:   errors.New("wechat signature error"),
			wantStatus: http.StatusBadGateway,
			wantCode:   "wechat_virtual_query_failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			virtualClient := &fakeWechatVirtualPayClient{queryResult: tc.result, queryErr: tc.queryErr}
			testApp.wechatVirtualPayClient = virtualClient
			confirmPath := fmt.Sprintf("/api/payments/wechat/virtual-orders/%s/confirm", order.OrderNumber)
			resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, confirmPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
			if resp.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), fmt.Sprintf(`"code":"%s"`, tc.wantCode)) {
				t.Fatalf("expected %s, got %s", tc.wantCode, resp.Body.String())
			}
			if virtualClient.notifyCalls != 0 {
				t.Fatalf("expected no provide-goods notification, got %d", virtualClient.notifyCalls)
			}
		})
	}

	var transactionCount int64
	if err := db.Model(&CreditTransaction{}).Where("user_id = ? AND type = ?", order.UserID, CreditTransactionTypePaymentTopUp).Count(&transactionCount).Error; err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if transactionCount != 0 {
		t.Fatalf("expected no topup transaction after rejected confirms, got %d", transactionCount)
	}
}

func TestWechatVirtualPayConfirmNotifyFailureDoesNotRollbackCredits(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies, order := createWechatVirtualOrderForTest(t, testApp, db, "wechat_virtual_notify", "wx-notify-openid")
	virtualClient := &fakeWechatVirtualPayClient{
		queryResult: wechatVirtualPayQueryResult{
			Status:    wechatVirtualOrderStatusPaidPendingDelivery,
			OpenID:    "wx-notify-openid",
			PaidFee:   order.AmountCents,
			OrderFee:  order.AmountCents,
			PaidTime:  time.Now().Unix(),
			WXOrderID: "wx-notify-order",
		},
		notifyErr: errors.New("notify temporary failed"),
	}
	testApp.wechatVirtualPayClient = virtualClient

	confirmPath := fmt.Sprintf("/api/payments/wechat/virtual-orders/%s/confirm", order.OrderNumber)
	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, confirmPath, nil, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected confirm 200 despite notify failure, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"payment_status":"paid"`) ||
		!strings.Contains(resp.Body.String(), fmt.Sprintf(`"available_credits":%d`, order.PackageCredits)) {
		t.Fatalf("expected paid order and credits despite notify failure: %s", resp.Body.String())
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load payment record: %v", err)
	}
	if !strings.Contains(payment.NotifySummary, "notify_provide_goods_failed") ||
		payment.LastErrorCode != "wechat_virtual_notify_failed" {
		t.Fatalf("expected notify failure summary on payment record, got %+v", payment)
	}
}

func createWechatVirtualOrderForTest(t *testing.T, testApp *App, db *gorm.DB, username, openid string) (User, []*http.Cookie, FinanceOrder) {
	t.Helper()
	user, cookies := createLoggedInUser(t, testApp, username, "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	setUserPhoneForTest(t, testApp, user.ID, fmt.Sprintf("139%08d", 10000000+int(user.ID)%89999999))
	if err := db.Model(&user).Update("wechat_open_id", openid).Error; err != nil {
		t.Fatalf("bind openid: %v", err)
	}
	testApp.cfg.WechatVirtualPayOfferID = "offer-1001"
	testApp.cfg.WechatVirtualPayAppKey = "virtual-app-key"
	testApp.cfg.WechatVirtualPayEnv = 0
	testApp.cfg.WechatPayAppID = "wx-app"
	testApp.cfg.WechatAppSecret = "app-secret"
	testApp.wechatSessionExchanger = fakeWechatSessionExchanger{openidByCode: map[string]string{
		"virtual-code": openid,
	}}
	var pkg Package
	if err := db.Where("is_active = ?", true).Order("sort_order asc, id asc").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if err := db.Model(&pkg).Update("wechat_virtual_product_id", "goods-1001").Error; err != nil {
		t.Fatalf("set virtual product id: %v", err)
	}
	resp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/payments/wechat/virtual-orders", map[string]any{
		"package_id": pkg.ID,
		"code":       "virtual-code",
	}, cookies, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected virtual order 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Order FinanceOrder `json:"order"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return user, cookies, payload.Order
}

func TestWechatVirtualPayHMACHex(t *testing.T) {
	got := hmacSHA256Hex("key", "requestVirtualPayment&{\"outTradeNo\":\"order-1\"}")
	want := testHMACHex("key", "requestVirtualPayment&{\"outTradeNo\":\"order-1\"}")
	if got != want {
		t.Fatalf("unexpected hmac hex: got %s want %s", got, want)
	}
}

func testHMACHex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
