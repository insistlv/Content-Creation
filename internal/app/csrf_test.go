package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRFTokenEndpointSetsReadableCookie(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/auth/csrf-token", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected csrf token 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode csrf payload: %v", err)
	}
	if len(payload.CSRFToken) < 32 {
		t.Fatalf("expected strong csrf token, got %q", payload.CSRFToken)
	}

	var csrfCookie *http.Cookie
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("expected %s cookie, got %+v", csrfCookieName, resp.Result().Cookies())
	}
	if csrfCookie.Value != payload.CSRFToken {
		t.Fatalf("expected csrf cookie to match payload")
	}
	if csrfCookie.HttpOnly {
		t.Fatal("csrf cookie must be readable by the web client")
	}
	if csrfCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", csrfCookie.SameSite)
	}
}

func TestBrowserMutatingRequestsRequireCSRFHeaderAndCookie(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.smsSender = &stubSMSSender{}

	missingResp := performRawJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800139001",
		"purpose": "register",
	}, nil, map[string]string{"Origin": testApp.cfg.AppBaseURL})
	if missingResp.Code != http.StatusForbidden || !strings.Contains(missingResp.Body.String(), "csrf_required") {
		t.Fatalf("expected csrf_required 403, got %d: %s", missingResp.Code, missingResp.Body.String())
	}

	csrfCookie := csrfCookieForTest(t, testApp)
	mismatchResp := performRawJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800139002",
		"purpose": "register",
	}, []*http.Cookie{csrfCookie}, map[string]string{
		"Origin":       testApp.cfg.AppBaseURL,
		csrfHeaderName: "wrong-token",
	})
	if mismatchResp.Code != http.StatusForbidden || !strings.Contains(mismatchResp.Body.String(), "csrf_invalid") {
		t.Fatalf("expected csrf_invalid 403, got %d: %s", mismatchResp.Code, mismatchResp.Body.String())
	}

	okResp := performRawJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800139003",
		"purpose": "register",
	}, []*http.Cookie{csrfCookie}, map[string]string{
		"Origin":       testApp.cfg.AppBaseURL,
		csrfHeaderName: csrfCookie.Value,
	})
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected csrf-protected SMS send 200, got %d: %s", okResp.Code, okResp.Body.String())
	}
}

func TestMiniProgramMutatingRequestsBypassBrowserCSRF(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.smsSender = &stubSMSSender{}

	resp := performRawJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/sms-code", map[string]any{
		"phone":   "13800139004",
		"purpose": "register",
	}, nil, map[string]string{
		"Origin":               "",
		"X-Image-Agent-Client": "mp-weixin",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected mp SMS send without csrf 200, got %d: %s", resp.Code, resp.Body.String())
	}
}

func csrfCookieForTest(t *testing.T, app *App) *http.Cookie {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodGet, "/api/auth/csrf-token", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("csrf token failed: %d %s", resp.Code, resp.Body.String())
	}
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			return cookie
		}
	}
	t.Fatalf("csrf cookie missing: %+v", resp.Result().Cookies())
	return nil
}

func performRawJSONRequestWithHeaders(t *testing.T, app *App, method, path string, body map[string]any, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp := httptest.NewRecorder()
	app.Router().ServeHTTP(resp, req)
	return resp
}
