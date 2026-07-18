package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type stubProvider struct {
	result  ImageGenerationResult
	results []ImageGenerationResult
	err     *ProviderError
	errs    []*ProviderError
	calls   int
	waitCh  <-chan struct{}
	inputs  []ImageGenerationInput
}

type stubVideoProvider struct {
	submitResult VideoSubmitResult
	submitErr    *ProviderError
	pollResults  []VideoTaskResult
	submitInputs []VideoGenerationInput
	pollInputs   []string
}

type blockingImageProvider struct {
	mu       sync.Mutex
	release  chan struct{}
	calls    int
	finished int32
	inputs   []ImageGenerationInput
}

type panicOnceImageProvider struct {
	mu    sync.Mutex
	calls int
}

type promptRoutingImageProvider struct {
	mu     sync.Mutex
	calls  int
	inputs []ImageGenerationInput
}

type delayedSuccessImageProvider struct {
	mu           sync.Mutex
	started      chan struct{}
	release      chan struct{}
	startedOnce  sync.Once
	calls        int
	result       ImageGenerationResult
	beforeReturn func()
}

type blockingVideoProvider struct {
	mu           sync.Mutex
	release      chan struct{}
	submitCalls  int
	submitInputs []VideoGenerationInput
}

type publicURLAssetStore struct {
	key         string
	mimeType    string
	publicURL   string
	content     []byte
	deleteCalls int
}

func (s *publicURLAssetStore) SaveBase64(base64Image, mimeType string) (string, string, error) {
	content, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		return "", "", err
	}
	return s.SaveBytes(content, mimeType)
}

func (s *publicURLAssetStore) SaveBytes(content []byte, mimeType string) (string, string, error) {
	s.content = content
	if s.mimeType == "" {
		s.mimeType = normalizeAssetMimeType(mimeType)
	}
	return s.key, s.mimeType, nil
}

func (s *publicURLAssetStore) SaveStream(content io.Reader, mimeType string) (string, string, error) {
	value, err := io.ReadAll(content)
	if err != nil {
		return "", "", err
	}
	return s.SaveBytes(value, mimeType)
}

func (s *publicURLAssetStore) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func (s *publicURLAssetStore) Read(string) ([]byte, error) {
	return s.content, nil
}

func (s *publicURLAssetStore) ObjectMeta(string) (AssetObjectMeta, error) {
	return AssetObjectMeta{
		ContentLength: int64(len(s.content)),
		MIMEType:      s.mimeType,
	}, nil
}

func (s *publicURLAssetStore) ReadRange(_ string, start, end int64) ([]byte, error) {
	if start < 0 || end < start || start >= int64(len(s.content)) {
		return nil, io.EOF
	}
	if end >= int64(len(s.content)) {
		end = int64(len(s.content)) - 1
	}
	return s.content[start : end+1], nil
}

func (s *publicURLAssetStore) Delete(string) error {
	s.deleteCalls++
	return nil
}

func (s *publicURLAssetStore) PublicURL(key string) string {
	if strings.HasSuffix(s.publicURL, "/") {
		return buildOSSPublicURL(s.publicURL, key)
	}
	return s.publicURL
}

func (s *stubProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	s.calls++
	s.inputs = append(s.inputs, input)
	if s.waitCh != nil {
		select {
		case <-s.waitCh:
		case <-ctx.Done():
			return ImageGenerationResult{}, &ProviderError{
				Code:    "provider_request_failed",
				Message: ctx.Err().Error(),
			}
		}
	}
	if s.err != nil {
		return ImageGenerationResult{}, s.err
	}
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		if err != nil {
			return ImageGenerationResult{}, err
		}
	}
	if len(s.results) > 0 {
		result := s.results[0]
		s.results = s.results[1:]
		return result, nil
	}
	return s.result, nil
}

func newBlockingImageProvider() *blockingImageProvider {
	return &blockingImageProvider{release: make(chan struct{})}
}

func (p *blockingImageProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	defer atomic.AddInt32(&p.finished, 1)

	p.mu.Lock()
	p.calls++
	callNumber := p.calls
	p.inputs = append(p.inputs, input)
	p.mu.Unlock()

	select {
	case <-p.release:
	case <-ctx.Done():
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_failed", Message: ctx.Err().Error()}
	}

	return ImageGenerationResult{
		Base64Image:       base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("image-%d", callNumber))),
		MIMEType:          "image/png",
		ProviderRequestID: fmt.Sprintf("req_concurrent_%d", callNumber),
	}, nil
}

func (p *blockingImageProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *blockingImageProvider) finishedCount() int {
	return int(atomic.LoadInt32(&p.finished))
}

func (p *panicOnceImageProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	p.mu.Lock()
	p.calls++
	callNumber := p.calls
	p.mu.Unlock()

	if callNumber == 1 {
		panic("provider task panic")
	}
	return ImageGenerationResult{
		Base64Image:       base64.StdEncoding.EncodeToString([]byte("panic-recovered-next-image")),
		MIMEType:          "image/png",
		ProviderRequestID: "req_after_panic",
	}, nil
}

func (p *panicOnceImageProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *promptRoutingImageProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	p.mu.Lock()
	p.calls++
	p.inputs = append(p.inputs, input)
	p.mu.Unlock()

	if strings.Contains(input.Prompt, "fail") {
		return ImageGenerationResult{}, &ProviderError{
			HTTPStatus:        http.StatusBadGateway,
			Code:              "provider_http_502",
			Message:           "upstream failed",
			ProviderRequestID: "req_" + strings.ReplaceAll(input.Prompt, " ", "_"),
			FailureStage:      providerFailureStageImageGenerationRequest,
		}
	}
	return ImageGenerationResult{
		Base64Image:       base64.StdEncoding.EncodeToString([]byte(input.Prompt)),
		MIMEType:          "image/png",
		ProviderRequestID: "req_" + strings.ReplaceAll(input.Prompt, " ", "_"),
	}, nil
}

func (p *promptRoutingImageProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func newDelayedSuccessImageProvider() *delayedSuccessImageProvider {
	return &delayedSuccessImageProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("late-success-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_late_success",
		},
	}
}

func (p *delayedSuccessImageProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.startedOnce.Do(func() { close(p.started) })

	select {
	case <-p.release:
		if p.beforeReturn != nil {
			p.beforeReturn()
		}
		return p.result, nil
	case <-ctx.Done():
		return ImageGenerationResult{}, &ProviderError{Code: "provider_timeout", Message: ctx.Err().Error()}
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatalf("condition was not met within %s", timeout)
}

func waitForVideoGenerationTasksToSettle(t *testing.T, testApp *App) {
	t.Helper()
	waitFor(t, 3*time.Second, func() bool {
		var pending int64
		if err := testApp.db.Model(&GenerationRecord{}).
			Where("tool_mode = ? AND status NOT IN ?", "video", []string{GenerationStatusSucceeded, GenerationStatusFailed}).
			Count(&pending).Error; err != nil {
			return false
		}
		return pending == 0
	})
}

func (s *stubVideoProvider) SubmitVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError) {
	s.submitInputs = append(s.submitInputs, input)
	if s.submitErr != nil {
		return VideoSubmitResult{}, s.submitErr
	}
	return s.submitResult, nil
}

func (s *stubVideoProvider) PollVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	s.pollInputs = append(s.pollInputs, taskID)
	if len(s.pollResults) == 0 {
		return VideoTaskResult{TaskID: taskID, Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("video-bytes")), MIMEType: "video/mp4"}, nil
	}
	result := s.pollResults[0]
	s.pollResults = s.pollResults[1:]
	return result, nil
}

func newBlockingVideoProvider() *blockingVideoProvider {
	return &blockingVideoProvider{release: make(chan struct{})}
}

func (p *blockingVideoProvider) SubmitVideo(ctx context.Context, input VideoGenerationInput) (VideoSubmitResult, *ProviderError) {
	p.mu.Lock()
	p.submitCalls++
	p.submitInputs = append(p.submitInputs, input)
	p.mu.Unlock()

	select {
	case <-p.release:
	case <-ctx.Done():
		return VideoSubmitResult{}, &ProviderError{Code: "provider_request_failed", Message: ctx.Err().Error()}
	}
	return VideoSubmitResult{TaskID: "video-blocking-task"}, nil
}

func (p *blockingVideoProvider) PollVideo(ctx context.Context, taskID string, input VideoGenerationInput) (VideoTaskResult, *ProviderError) {
	return VideoTaskResult{
		TaskID:       taskID,
		Status:       VideoTaskSucceeded,
		OutputBase64: base64.StdEncoding.EncodeToString([]byte("video-bytes")),
		MIMEType:     "video/mp4",
	}, nil
}

func (p *blockingVideoProvider) submitCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.submitCalls
}

func TestLegacyUsernameRegisterRequiresPhoneVerification(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register", map[string]any{
		"username": "bot_like_user",
		"password": "test-password",
	}, nil)
	if resp.Code != http.StatusBadRequest || !bytes.Contains(resp.Body.Bytes(), []byte(`"phone_verification_required"`)) {
		t.Fatalf("expected phone_verification_required 400, got %d: %s", resp.Code, resp.Body.String())
	}
	var count int64
	if err := db.Model(&User{}).Where("username = ?", "bot_like_user").Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("legacy register should not create users, got %d", count)
	}
}

func TestRegisterLoginLogoutAndMeFlow(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	_, cookies := createLoggedInUser(t, testApp, "creator_01", "test-password")
	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}

	var mePayload struct {
		UserID                   uint      `json:"user_id"`
		Username                 string    `json:"username"`
		DisplayName              string    `json:"display_name"`
		Email                    string    `json:"email"`
		Status                   string    `json:"status"`
		AvailableCredits         int       `json:"available_credits"`
		LoginNotificationEnabled bool      `json:"login_notification_enabled"`
		RiskNotificationEnabled  bool      `json:"risk_notification_enabled"`
		CreatedAt                time.Time `json:"created_at"`
		UpdatedAt                time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(meResp.Body.Bytes(), &mePayload); err != nil {
		t.Fatalf("decode me payload: %v", err)
	}
	if mePayload.UserID == 0 || mePayload.Username != "creator_01" {
		t.Fatalf("unexpected me payload: %+v", mePayload)
	}
	if mePayload.DisplayName != "creator_01" {
		t.Fatalf("expected display name, got %+v", mePayload)
	}
	if mePayload.Status != UserStatusActive {
		t.Fatalf("expected active status, got %+v", mePayload)
	}
	if mePayload.Email != "" {
		t.Fatalf("expected empty email by default, got %+v", mePayload)
	}
	if !mePayload.LoginNotificationEnabled || !mePayload.RiskNotificationEnabled {
		t.Fatalf("expected notification preferences enabled by default, got %+v", mePayload)
	}
	if mePayload.CreatedAt.IsZero() || mePayload.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps in me payload, got %+v", mePayload)
	}
	if mePayload.AvailableCredits != 5 {
		t.Fatalf("expected signup bonus credits 5, got %d", mePayload.AvailableCredits)
	}

	logoutResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/logout", nil, cookies)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("expected logout 200, got %d", logoutResp.Code)
	}

	meAfterLogout := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("expected me after logout 401, got %d", meAfterLogout.Code)
	}

	loginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "creator_01", "test-password"), nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
}

func TestMiniProgramLoginReturnsBearerTokenAndAuthenticatesMe(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	_, _ = createLoggedInUser(t, testApp, "mp_creator", "test-password")

	webLoginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "mp_creator", "test-password"), nil)
	if webLoginResp.Code != http.StatusOK {
		t.Fatalf("expected web login 200, got %d: %s", webLoginResp.Code, webLoginResp.Body.String())
	}
	var webPayload map[string]any
	if err := json.Unmarshal(webLoginResp.Body.Bytes(), &webPayload); err != nil {
		t.Fatalf("decode web login payload: %v", err)
	}
	if _, ok := webPayload["auth_token"]; ok {
		t.Fatalf("web login must not expose auth_token: %s", webLoginResp.Body.String())
	}
	if _, ok := webPayload["auth_expires_at"]; ok {
		t.Fatalf("web login must not expose auth_expires_at: %s", webLoginResp.Body.String())
	}

	mpLoginResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "mp_creator",
		"password": "test-password",
	}, nil, map[string]string{
		"X-Image-Agent-Client": "mp-weixin",
	})
	if mpLoginResp.Code != http.StatusOK {
		t.Fatalf("expected mp login 200, got %d: %s", mpLoginResp.Code, mpLoginResp.Body.String())
	}
	var mpPayload struct {
		AuthToken     string `json:"auth_token"`
		AuthExpiresAt string `json:"auth_expires_at"`
	}
	if err := json.Unmarshal(mpLoginResp.Body.Bytes(), &mpPayload); err != nil {
		t.Fatalf("decode mp login payload: %v", err)
	}
	if strings.TrimSpace(mpPayload.AuthToken) == "" {
		t.Fatalf("expected mp login auth_token, got %s", mpLoginResp.Body.String())
	}
	if strings.TrimSpace(mpPayload.AuthExpiresAt) == "" {
		t.Fatalf("expected mp login auth_expires_at, got %s", mpLoginResp.Body.String())
	}

	meResp := performJSONRequestWithHeaders(t, testApp, http.MethodGet, "/api/me", nil, nil, map[string]string{
		"Authorization": "Bearer " + mpPayload.AuthToken,
	})
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected bearer me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}
}

func TestMiniProgramBearerLogoutInvalidatesSession(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	_, _ = createLoggedInUser(t, testApp, "mp_logout_creator", "test-password")
	loginResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "mp_logout_creator",
		"password": "test-password",
	}, nil, map[string]string{"X-Image-Agent-Client": "mp-weixin"})
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected mp login 200, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	var loginPayload struct {
		AuthToken string `json:"auth_token"`
	}
	if err := json.Unmarshal(loginResp.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("decode mp login payload: %v", err)
	}
	if strings.TrimSpace(loginPayload.AuthToken) == "" {
		t.Fatalf("expected mp login auth_token, got %s", loginResp.Body.String())
	}

	logoutResp := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/logout", nil, nil, map[string]string{
		"Authorization": "Bearer " + loginPayload.AuthToken,
	})
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("expected bearer logout 200, got %d: %s", logoutResp.Code, logoutResp.Body.String())
	}

	meAfterLogout := performJSONRequestWithHeaders(t, testApp, http.MethodGet, "/api/me", nil, nil, map[string]string{
		"Authorization": "Bearer " + loginPayload.AuthToken,
	})
	if meAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("expected bearer me after logout 401, got %d: %s", meAfterLogout.Code, meAfterLogout.Body.String())
	}
}

func TestAccountProfileEmailAndPreferencesPersist(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "creator_account", "test-password")

	profileResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/account/profile", map[string]any{
		"display_name": "视觉主理人",
	}, cookies)
	if profileResp.Code != http.StatusOK {
		t.Fatalf("expected profile update 200, got %d: %s", profileResp.Code, profileResp.Body.String())
	}
	if !bytes.Contains(profileResp.Body.Bytes(), []byte(`"display_name":"视觉主理人"`)) {
		t.Fatalf("expected updated display name, got %s", profileResp.Body.String())
	}

	emailResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/account/email", map[string]any{
		"email": "creator@example.com",
	}, cookies)
	if emailResp.Code != http.StatusOK {
		t.Fatalf("expected email update 200, got %d: %s", emailResp.Code, emailResp.Body.String())
	}
	if !bytes.Contains(emailResp.Body.Bytes(), []byte(`"email":"creator@example.com"`)) {
		t.Fatalf("expected updated email, got %s", emailResp.Body.String())
	}

	preferencesResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/account/preferences", map[string]any{
		"login_notification_enabled": false,
		"risk_notification_enabled":  true,
	}, cookies)
	if preferencesResp.Code != http.StatusOK {
		t.Fatalf("expected preferences update 200, got %d: %s", preferencesResp.Code, preferencesResp.Body.String())
	}

	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}
	var mePayload struct {
		DisplayName              string `json:"display_name"`
		Email                    string `json:"email"`
		LoginNotificationEnabled bool   `json:"login_notification_enabled"`
		RiskNotificationEnabled  bool   `json:"risk_notification_enabled"`
	}
	if err := json.Unmarshal(meResp.Body.Bytes(), &mePayload); err != nil {
		t.Fatalf("decode me payload: %v", err)
	}
	if mePayload.DisplayName != "视觉主理人" || mePayload.Email != "creator@example.com" {
		t.Fatalf("expected persisted profile and email, got %+v", mePayload)
	}
	if mePayload.LoginNotificationEnabled || !mePayload.RiskNotificationEnabled {
		t.Fatalf("expected persisted notification preferences, got %+v", mePayload)
	}

	invalidEmailResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/account/email", map[string]any{
		"email": "not-an-email",
	}, cookies)
	if invalidEmailResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid email 400, got %d: %s", invalidEmailResp.Code, invalidEmailResp.Body.String())
	}

	clearEmailResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/account/email", map[string]any{
		"email": "",
	}, cookies)
	if clearEmailResp.Code != http.StatusOK {
		t.Fatalf("expected clear email 200, got %d: %s", clearEmailResp.Code, clearEmailResp.Body.String())
	}
	if !bytes.Contains(clearEmailResp.Body.Bytes(), []byte(`"email":""`)) {
		t.Fatalf("expected cleared email, got %s", clearEmailResp.Body.String())
	}
}

func TestRegisterAllowsOriginMatchingRequestHost(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.smsSender = &stubSMSSender{}

	payload, err := json.Marshal(map[string]any{
		"phone":   "13800139005",
		"purpose": "register",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/sms-code", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8888"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8888")
	csrfToken := "host-origin-csrf-token"
	req.Header.Set(csrfHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken, Path: "/"})

	resp := httptest.NewRecorder()
	testApp.Router().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected SMS send 200, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestAdminSPAPagesRequireAdminSession(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	distPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(distPath, "index.html"), []byte("<!doctype html><title>app</title>"), 0o644); err != nil {
		t.Fatalf("write test index: %v", err)
	}
	testApp.cfg.FrontendDistPath = distPath
	testApp.router = testApp.setupRouter()

	adminPaths := []string{
		"/admin",
		"/admin/settings",
		"/admin/settings/models/3",
		"/admin/prompt-templates",
		"/admin/system-settings",
		"/admin/invites",
		"/admin/generations",
		"/admin/users",
		"/admin/packages",
		"/admin/customer-service",
		"/admin/finance-orders",
		"/admin/permissions",
		"/admin/forbidden",
	}

	for _, path := range adminPaths {
		t.Run(path, func(t *testing.T) {
			resp := performRequest(t, testApp, http.MethodGet, path, nil, nil)
			if resp.Code != http.StatusFound {
				t.Fatalf("expected unauthenticated admin SPA path to redirect, got %d: %s", resp.Code, resp.Body.String())
			}
			if got := resp.Header().Get("Location"); got != "/admin/login" {
				t.Fatalf("expected redirect to admin login, got %q", got)
			}
		})
	}

	loginResp := performRequest(t, testApp, http.MethodGet, "/admin/login", nil, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected admin login SPA path to remain public, got %d: %s", loginResp.Code, loginResp.Body.String())
	}

	adminCookies := createAdminSession(t, testApp)
	for _, path := range adminPaths {
		t.Run(path+" authenticated", func(t *testing.T) {
			resp := performRequest(t, testApp, http.MethodGet, path, nil, adminCookies)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected authenticated admin SPA path to serve app shell, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestFrontendAssetsRouteKeepsSPARouteAndServesHashedFiles(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	distPath := t.TempDir()
	assetsPath := filepath.Join(distPath, "assets")
	appAssetsPath := filepath.Join(distPath, "app-assets")
	if err := os.MkdirAll(assetsPath, 0o755); err != nil {
		t.Fatalf("create assets dir: %v", err)
	}
	if err := os.MkdirAll(appAssetsPath, 0o755); err != nil {
		t.Fatalf("create app assets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distPath, "index.html"), []byte("<!doctype html><title>workspace</title>"), 0o644); err != nil {
		t.Fatalf("write test index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distPath, "build-info.json"), []byte(`{"git_commit":"abc123"}`), 0o644); err != nil {
		t.Fatalf("write build info: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsPath, "index-test.js"), []byte("console.log('asset')"), 0o644); err != nil {
		t.Fatalf("write test asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appAssetsPath, "index-test.js"), []byte("console.log('app-asset')"), 0o644); err != nil {
		t.Fatalf("write test app asset: %v", err)
	}
	testApp.cfg.FrontendDistPath = distPath
	testApp.router = testApp.setupRouter()

	for _, path := range []string{"/assets", "/assets/"} {
		t.Run(path, func(t *testing.T) {
			resp := performRequest(t, testApp, http.MethodGet, path, nil, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected SPA route 200, got %d: %s", resp.Code, resp.Body.String())
			}
			if body := resp.Body.String(); !strings.Contains(body, "<title>workspace</title>") {
				t.Fatalf("expected SPA index, got %q", body)
			}
		})
	}

	assetResp := performRequest(t, testApp, http.MethodGet, "/assets/index-test.js", nil, nil)
	if assetResp.Code != http.StatusOK {
		t.Fatalf("expected hashed asset 200, got %d: %s", assetResp.Code, assetResp.Body.String())
	}
	if body := assetResp.Body.String(); body != "console.log('asset')" {
		t.Fatalf("expected static asset body, got %q", body)
	}
	if got := assetResp.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable asset cache header, got %q", got)
	}

	appAssetResp := performRequest(t, testApp, http.MethodGet, "/app-assets/index-test.js", nil, nil)
	if appAssetResp.Code != http.StatusOK {
		t.Fatalf("expected app asset 200, got %d: %s", appAssetResp.Code, appAssetResp.Body.String())
	}
	if body := appAssetResp.Body.String(); body != "console.log('app-asset')" {
		t.Fatalf("expected app asset body, got %q", body)
	}
	if got := appAssetResp.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable app asset cache header, got %q", got)
	}
	appAssetHeadResp := performRequest(t, testApp, http.MethodHead, "/app-assets/index-test.js", nil, nil)
	if appAssetHeadResp.Code != http.StatusOK {
		t.Fatalf("expected app asset HEAD 200, got %d: %s", appAssetHeadResp.Code, appAssetHeadResp.Body.String())
	}
	if got := appAssetHeadResp.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable app asset HEAD cache header, got %q", got)
	}

	buildInfoResp := performRequest(t, testApp, http.MethodGet, "/build-info.json", nil, nil)
	if buildInfoResp.Code != http.StatusOK {
		t.Fatalf("expected build info 200, got %d: %s", buildInfoResp.Code, buildInfoResp.Body.String())
	}
	if body := buildInfoResp.Body.String(); body != `{"git_commit":"abc123"}` {
		t.Fatalf("expected build info body, got %q", body)
	}
	if got := buildInfoResp.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	if got := buildInfoResp.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected no-store build info cache header, got %q", got)
	}

	gzipResp := performRequestWithHeaders(t, testApp, http.MethodGet, "/assets/index-test.js", nil, nil, map[string]string{
		"Accept-Encoding": "gzip",
	})
	if gzipResp.Code != http.StatusOK {
		t.Fatalf("expected gzip asset 200, got %d: %s", gzipResp.Code, gzipResp.Body.String())
	}
	if got := gzipResp.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected gzip content encoding, got %q", got)
	}
	if got := gzipResp.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("expected Accept-Encoding vary header, got %q", got)
	}
	reader, err := gzip.NewReader(bytes.NewReader(gzipResp.Body.Bytes()))
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close gzip reader: %v", err)
	}
	if string(decoded) != "console.log('asset')" {
		t.Fatalf("expected decoded gzip asset body, got %q", string(decoded))
	}

	missingResp := performRequest(t, testApp, http.MethodGet, "/assets/missing.js", nil, nil)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing asset 404, got %d: %s", missingResp.Code, missingResp.Body.String())
	}
	if strings.Contains(strings.ToLower(missingResp.Body.String()), "index of") {
		t.Fatalf("expected no directory index, got %q", missingResp.Body.String())
	}
}

func TestAdminAPIRoutesRequireAdminSession(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/logout"},
		{http.MethodPost, "/api/admin/password"},
		{http.MethodGet, "/api/admin/me"},
		{http.MethodGet, "/api/admin/dashboard"},
		{http.MethodGet, "/api/admin/settings/image"},
		{http.MethodPut, "/api/admin/settings/image"},
		{http.MethodGet, "/api/admin/system-settings"},
		{http.MethodPatch, "/api/admin/system-settings"},
		{http.MethodGet, "/api/admin/system-settings/export"},
		{http.MethodGet, "/api/admin/models"},
		{http.MethodPost, "/api/admin/models"},
		{http.MethodGet, "/api/admin/models/1"},
		{http.MethodPut, "/api/admin/models/1"},
		{http.MethodDelete, "/api/admin/models/1"},
		{http.MethodGet, "/api/admin/prompt-templates"},
		{http.MethodPost, "/api/admin/prompt-templates"},
		{http.MethodPut, "/api/admin/prompt-templates/1"},
		{http.MethodDelete, "/api/admin/prompt-templates/1"},
		{http.MethodPost, "/api/admin/prompt-templates/1/preview"},
		{http.MethodPost, "/api/admin/prompt-templates/previews/generate"},
		{http.MethodGet, "/api/admin/couple-album-options"},
		{http.MethodPost, "/api/admin/couple-album-options"},
		{http.MethodPut, "/api/admin/couple-album-options/1"},
		{http.MethodDelete, "/api/admin/couple-album-options/1"},
		{http.MethodPost, "/api/admin/couple-album-options/assets"},
		{http.MethodGet, "/api/admin/model-routing"},
		{http.MethodPut, "/api/admin/model-routing"},
		{http.MethodGet, "/api/admin/invites"},
		{http.MethodGet, "/api/admin/invites/export"},
		{http.MethodPost, "/api/admin/invites"},
		{http.MethodPost, "/api/admin/invites/batch"},
		{http.MethodPut, "/api/admin/invites/1"},
		{http.MethodGet, "/api/admin/invite-redemptions"},
		{http.MethodGet, "/api/admin/invite-redemptions/export"},
		{http.MethodGet, "/api/admin/generations"},
		{http.MethodGet, "/api/admin/generations/export"},
		{http.MethodGet, "/api/admin/generations/1"},
		{http.MethodGet, "/api/admin/users"},
		{http.MethodGet, "/api/admin/credit-transactions"},
		{http.MethodPost, "/api/admin/users/1/credits"},
		{http.MethodPost, "/api/admin/users/1/credit-adjustments"},
		{http.MethodPatch, "/api/admin/users/1/wechat-binding"},
		{http.MethodDelete, "/api/admin/users/1/wechat-binding"},
		{http.MethodDelete, "/api/admin/users/1/phone-binding"},
		{http.MethodGet, "/api/admin/packages"},
		{http.MethodPost, "/api/admin/packages"},
		{http.MethodPut, "/api/admin/packages/1"},
		{http.MethodDelete, "/api/admin/packages/1"},
		{http.MethodGet, "/api/admin/customer-service"},
		{http.MethodPatch, "/api/admin/customer-service"},
		{http.MethodPost, "/api/admin/customer-service/qrcode"},
		{http.MethodGet, "/api/admin/purchase-intents"},
		{http.MethodPut, "/api/admin/purchase-intents/1"},
		{http.MethodGet, "/api/admin/finance-orders"},
		{http.MethodGet, "/api/admin/finance-orders/export"},
		{http.MethodGet, "/api/admin/finance-orders/1"},
		{http.MethodPatch, "/api/admin/finance-refunds/1"},
		{http.MethodPatch, "/api/admin/finance-invoices/1"},
		{http.MethodGet, "/api/admin/announcements"},
		{http.MethodPost, "/api/admin/announcements"},
		{http.MethodPut, "/api/admin/announcements/1"},
		{http.MethodPatch, "/api/admin/announcements/1/status"},
		{http.MethodGet, "/api/admin/admin-users"},
		{http.MethodPost, "/api/admin/admin-users"},
		{http.MethodPatch, "/api/admin/admin-users/1"},
		{http.MethodPut, "/api/admin/admin-users/1/roles"},
		{http.MethodPost, "/api/admin/admin-users/1/reset-password"},
		{http.MethodGet, "/api/admin/roles"},
		{http.MethodPost, "/api/admin/roles"},
		{http.MethodPatch, "/api/admin/roles/1"},
		{http.MethodPut, "/api/admin/roles/1/permissions"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			resp := performJSONRequest(t, testApp, route.method, route.path, map[string]any{}, nil)
			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected unauthenticated admin API route to return 401, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestAccountPhoneUnbindRequiresUserSession(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/phone", map[string]any{
		"current_password": "test-password",
	}, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated account phone unbind 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestAccountPasswordChangeRequiresCurrentPasswordAndAppliesNewPassword(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "creator_pwd", "OldPass123")

	badResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/password", map[string]any{
		"current_password": "wrong-pass",
		"new_password":     "NewPass456",
	}, cookies)
	if badResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", badResp.Code, badResp.Body.String())
	}

	okResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/password", map[string]any{
		"current_password": "OldPass123",
		"new_password":     "NewPass456",
	}, cookies)
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", okResp.Code, okResp.Body.String())
	}

	oldLogin := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "creator_pwd", "OldPass123"), nil)
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password login 401, got %d", oldLogin.Code)
	}

	newLogin := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "creator_pwd", "NewPass456"), nil)
	if newLogin.Code != http.StatusOK {
		t.Fatalf("expected new password login 200, got %d", newLogin.Code)
	}
}

func TestAccountPaymentPasswordCanBeSetResetAndCleared(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "creator_pay_pwd", "LoginPass123")

	meBefore := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meBefore.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meBefore.Code, meBefore.Body.String())
	}
	if !bytes.Contains(meBefore.Body.Bytes(), []byte(`"payment_password_enabled":false`)) {
		t.Fatalf("expected payment password disabled in me payload, got %s", meBefore.Body.String())
	}

	invalidResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/payment-password", map[string]any{
		"current_password": "LoginPass123",
		"payment_password": "12345a",
	}, cookies)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid payment password 400, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}

	wrongPasswordResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/payment-password", map[string]any{
		"current_password": "wrong-password",
		"payment_password": "123456",
	}, cookies)
	if wrongPasswordResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong current password 401, got %d: %s", wrongPasswordResp.Code, wrongPasswordResp.Body.String())
	}

	setResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/payment-password", map[string]any{
		"current_password": "LoginPass123",
		"payment_password": "123456",
	}, cookies)
	if setResp.Code != http.StatusOK {
		t.Fatalf("expected set payment password 200, got %d: %s", setResp.Code, setResp.Body.String())
	}

	resetResp := performJSONRequest(t, testApp, http.MethodPost, "/api/account/payment-password", map[string]any{
		"current_password": "LoginPass123",
		"payment_password": "654321",
	}, cookies)
	if resetResp.Code != http.StatusOK {
		t.Fatalf("expected reset payment password 200, got %d: %s", resetResp.Code, resetResp.Body.String())
	}

	meAfterSet := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meAfterSet.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meAfterSet.Code, meAfterSet.Body.String())
	}
	if !bytes.Contains(meAfterSet.Body.Bytes(), []byte(`"payment_password_enabled":true`)) {
		t.Fatalf("expected payment password enabled in me payload, got %s", meAfterSet.Body.String())
	}

	wrongClearResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/payment-password", map[string]any{
		"current_password": "wrong-password",
	}, cookies)
	if wrongClearResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong clear password 401, got %d: %s", wrongClearResp.Code, wrongClearResp.Body.String())
	}

	clearResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/payment-password", map[string]any{
		"current_password": "LoginPass123",
	}, cookies)
	if clearResp.Code != http.StatusOK {
		t.Fatalf("expected clear payment password 200, got %d: %s", clearResp.Code, clearResp.Body.String())
	}

	meAfterClear := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, cookies)
	if meAfterClear.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meAfterClear.Code, meAfterClear.Body.String())
	}
	if !bytes.Contains(meAfterClear.Body.Bytes(), []byte(`"payment_password_enabled":false`)) {
		t.Fatalf("expected payment password disabled after clear, got %s", meAfterClear.Body.String())
	}
}

func TestAccountSessionsCanListAndRevokeOtherDevices(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, firstCookies := createLoggedInUser(t, testApp, "creator_sessions", "test-password")
	secondLogin := performJSONRequestWithHeaders(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "creator_sessions", "test-password"), nil, map[string]string{
		"User-Agent":      "Second Browser",
		"X-Forwarded-For": "203.0.113.10",
	})
	if secondLogin.Code != http.StatusOK {
		t.Fatalf("expected second login 200, got %d: %s", secondLogin.Code, secondLogin.Body.String())
	}
	secondCookies := secondLogin.Result().Cookies()

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/sessions", nil, secondCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected sessions list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID        uint   `json:"id"`
			Current   bool   `json:"current"`
			IPAddress string `json:"ip_address"`
			UserAgent string `json:"user_agent"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode sessions payload: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected two sessions, got %+v", payload)
	}
	var currentID, otherID uint
	for _, item := range payload.Items {
		if item.Current {
			currentID = item.ID
			if item.UserAgent != "Second Browser" || item.IPAddress != "203.0.113.10" {
				t.Fatalf("expected current session source metadata, got %+v", item)
			}
		} else {
			otherID = item.ID
		}
	}
	if currentID == 0 || otherID == 0 {
		t.Fatalf("expected current and other sessions, got %+v", payload)
	}

	deleteCurrentResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/sessions/"+itoa(currentID), nil, secondCookies)
	if deleteCurrentResp.Code != http.StatusBadRequest {
		t.Fatalf("expected deleting current session 400, got %d: %s", deleteCurrentResp.Code, deleteCurrentResp.Body.String())
	}

	deleteOtherResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/sessions/"+itoa(otherID), nil, secondCookies)
	if deleteOtherResp.Code != http.StatusOK {
		t.Fatalf("expected deleting other session 200, got %d: %s", deleteOtherResp.Code, deleteOtherResp.Body.String())
	}
	firstMeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, firstCookies)
	if firstMeResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked first session 401, got %d: %s", firstMeResp.Code, firstMeResp.Body.String())
	}

	thirdLogin := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, user.Username, "test-password"), nil)
	if thirdLogin.Code != http.StatusOK {
		t.Fatalf("expected third login 200, got %d: %s", thirdLogin.Code, thirdLogin.Body.String())
	}
	deleteOthersResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/account/sessions/others", nil, secondCookies)
	if deleteOthersResp.Code != http.StatusOK {
		t.Fatalf("expected deleting other sessions 200, got %d: %s", deleteOthersResp.Code, deleteOthersResp.Body.String())
	}
	thirdMeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, thirdLogin.Result().Cookies())
	if thirdMeResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected third session revoked 401, got %d: %s", thirdMeResp.Code, thirdMeResp.Body.String())
	}
	secondMeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, secondCookies)
	if secondMeResp.Code != http.StatusOK {
		t.Fatalf("expected current second session to remain active, got %d: %s", secondMeResp.Code, secondMeResp.Body.String())
	}
}

func TestGenerationSuccessPersistsWorkAndConsumesCredits(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "ZmFrZS1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_123",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_gen", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "cinematic paper collage of a koi fish in clouds",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}

	var payload struct {
		GenerationID      uint   `json:"generation_id"`
		WorkID            uint   `json:"work_id"`
		Status            string `json:"status"`
		PreviewURL        string `json:"preview_url"`
		DownloadURL       string `json:"download_url"`
		AvailableCredits  int    `json:"available_credits"`
		CreditsCost       int    `json:"credits_cost"`
		CreditsDeducted   bool   `json:"credits_deducted"`
		ProviderRequestID string `json:"provider_request_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	if payload.GenerationID == 0 || payload.WorkID == 0 {
		t.Fatalf("expected persisted generation/work ids, got %+v", payload)
	}
	if payload.Status != GenerationStatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", payload)
	}
	if payload.AvailableCredits != 1 {
		t.Fatalf("expected remaining credits 1, got %+v", payload)
	}
	if payload.CreditsCost != 1 || !payload.CreditsDeducted {
		t.Fatalf("expected response credits_cost=1 and credits_deducted=true, got %+v", payload)
	}
	if payload.ProviderRequestID != "req_123" {
		t.Fatalf("expected provider request id req_123, got %+v", payload)
	}

	workResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works/"+itoa(payload.WorkID), nil, cookies)
	if workResp.Code != http.StatusOK {
		t.Fatalf("expected work detail 200, got %d: %s", workResp.Code, workResp.Body.String())
	}
	if !bytes.Contains(workResp.Body.Bytes(), []byte(`"preview_url":"`)) {
		t.Fatalf("expected preview url in work payload: %s", workResp.Body.String())
	}

	fileResp := performRequest(t, testApp, http.MethodGet, payload.PreviewURL, nil, cookies)
	if fileResp.Code != http.StatusOK {
		t.Fatalf("expected preview file 200, got %d: %s", fileResp.Code, fileResp.Body.String())
	}
	if got := fileResp.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected image/png, got %q", got)
	}
	if body := fileResp.Body.Bytes(); len(body) == 0 {
		t.Fatal("expected persisted image bytes")
	}

	downloadResp := performRequest(t, testApp, http.MethodGet, payload.DownloadURL, nil, cookies)
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected download 200, got %d", downloadResp.Code)
	}
	if disposition := downloadResp.Header().Get("Content-Disposition"); disposition == "" {
		t.Fatal("expected attachment content disposition")
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 1 {
		t.Fatalf("expected available credits 1, got %d", balance.AvailableCredits)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.Status != GenerationStatusSucceeded || !record.CreditsDeducted {
		t.Fatalf("unexpected generation record: %+v", record)
	}

	var work Work
	if err := testApp.db.First(&work, payload.WorkID).Error; err != nil {
		t.Fatalf("load work: %v", err)
	}
	if work.UserID != user.ID || work.AssetKey == "" || work.Visibility != WorkVisibilityPrivate {
		t.Fatalf("unexpected work payload: %+v", work)
	}
}

func TestImageToImageReferenceAssetsChargeOneCredit(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "ZmFrZS1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_i2i_cost",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_i2i_cost", "test-password")
	setUserCredits(t, testApp, user.ID, 4)
	first := seedReferenceAsset(t, testApp, user.ID, "first.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "second.png", "image/png", []byte("second-reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "combine uploaded references into a poster",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}

	var payload struct {
		GenerationID     uint `json:"generation_id"`
		AvailableCredits int  `json:"available_credits"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	if payload.AvailableCredits != 3 {
		t.Fatalf("expected remaining credits 3 after one-credit image-to-image charge, got %+v", payload)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.CreditsCost != 1 || !record.CreditsDeducted {
		t.Fatalf("expected credits_cost=1 and deducted=true, got %+v", record)
	}

	var transaction CreditTransaction
	if err := testApp.db.Where("user_id = ? AND related_type = ? AND related_id = ?", user.ID, "generation", record.ID).First(&transaction).Error; err != nil {
		t.Fatalf("load credit transaction: %v", err)
	}
	if transaction.Amount != -1 || transaction.BalanceAfter != 3 {
		t.Fatalf("expected transaction amount -1 and balance 3, got %+v", transaction)
	}
}

func TestEstimateImageGenerationWithReferenceAsset(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_estimate_reference", "test-password")
	setUserCredits(t, testApp, user.ID, 4)
	asset := seedReferenceAsset(t, testApp, user.ID, "reference.png", "image/png", []byte("reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/estimate", map[string]any{
		"prompt":              "estimate uploaded reference",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{asset.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		RequiredCredits  int  `json:"required_credits"`
		AvailableCredits int  `json:"available_credits"`
		Enough           bool `json:"enough"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode estimate payload: %v", err)
	}
	if payload.RequiredCredits <= 0 || payload.AvailableCredits != 4 || !payload.Enough {
		t.Fatalf("unexpected estimate payload: %+v", payload)
	}
}

func TestEstimateImageGenerationReturnsReferenceAssetNotFound(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_estimate_missing_reference", "test-password")
	setUserCredits(t, testApp, user.ID, 4)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/estimate", map[string]any{
		"prompt":              "estimate missing uploaded reference",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{999999},
	}, cookies)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected estimate 404, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"reference_asset_not_found"`)) {
		t.Fatalf("expected reference_asset_not_found payload, got %s", resp.Body.String())
	}
}

func TestImageToImageRejectsWhenCreditsDoNotCoverOneCredit(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "ZmFrZS1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_i2i_insufficient",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_i2i_cost_low_balance", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	first := seedReferenceAsset(t, testApp, user.ID, "first.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "second.png", "image/png", []byte("second-reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "combine uploaded references with too few credits",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
	}, cookies)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"credits_insufficient"`)) {
		t.Fatalf("expected credits_insufficient payload, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called when credits cannot cover one-credit image generation, got %d", provider.calls)
	}
}

func TestAsyncGenerationCreateResponseIncludesCreditDeductionState(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "ZmFrZS1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_async_credits",
		},
	})
	user, cookies := createLoggedInUser(t, testApp, "creator_async_credit_state", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "async response credit state",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected async create 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID     uint `json:"generation_id"`
		AvailableCredits int  `json:"available_credits"`
		CreditsCost      int  `json:"credits_cost"`
		CreditsDeducted  bool `json:"credits_deducted"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode async generation payload: %v", err)
	}
	if payload.GenerationID == 0 || payload.CreditsCost != 1 || payload.CreditsDeducted {
		t.Fatalf("expected queued async payload with cost=1 and deducted=false, got %+v", payload)
	}
	if payload.AvailableCredits != 2 {
		t.Fatalf("expected queued async create to reserve one available credit, got %+v", payload)
	}
}

func TestImageToImageBatchChargesGeneratedImagesIndividually(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "ZmFrZS1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_i2i_batch_cost",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_i2i_batch_cost", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	first := seedReferenceAsset(t, testApp, user.ID, "first.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "second.png", "image/png", []byte("second-reference"))
	batchID := "batch-cost-references-once"

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "first image in a reference batch",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
		"batch_id":            batchID,
		"batch_index":         0,
		"batch_total":         2,
	}, cookies)
	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "second image in a reference batch",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
		"batch_id":            batchID,
		"batch_index":         1,
		"batch_total":         2,
	}, cookies)
	if firstResp.Code != http.StatusAccepted || secondResp.Code != http.StatusAccepted {
		t.Fatalf("expected both batch tasks accepted, got first=%d %s second=%d %s", firstResp.Code, firstResp.Body.String(), secondResp.Code, secondResp.Body.String())
	}

	waitFor(t, 2*time.Second, func() bool {
		var succeeded int64
		if err := testApp.db.Model(&GenerationRecord{}).Where("batch_id = ? AND status = ?", batchID, GenerationStatusSucceeded).Count(&succeeded).Error; err != nil {
			return false
		}
		return succeeded == 2
	})

	var records []GenerationRecord
	if err := testApp.db.Where("batch_id = ?", batchID).Order("batch_index asc").Find(&records).Error; err != nil {
		t.Fatalf("load batch records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected two batch records, got %d", len(records))
	}
	if records[0].CreditsCost != 1 || records[1].CreditsCost != 1 {
		t.Fatalf("expected each batch record to cost 1 credit, got %+v", records)
	}
	if len(provider.inputs) != 2 {
		t.Fatalf("expected two provider inputs, got %d", len(provider.inputs))
	}
	for index, input := range provider.inputs {
		if len(input.ReferenceImages) != 2 {
			t.Fatalf("expected full reference list on batch provider input %d, got %+v", index, input.ReferenceImages)
		}
		if input.ReferenceImages[0].InputURL != "data:image/png;base64,Zmlyc3QtcmVmZXJlbmNl" ||
			input.ReferenceImages[1].InputURL != "data:image/png;base64,c2Vjb25kLXJlZmVyZW5jZQ==" {
			t.Fatalf("expected ordered references on batch provider input %d, got %+v", index, input.ReferenceImages)
		}
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 0 {
		t.Fatalf("expected 2 total points charged for 2 generated images, got balance %+v", balance)
	}
}

func TestPromptOptimizationUsesDeepSeekAndSanitizesUnsafeTerms(t *testing.T) {
	var captured struct {
		Path          string
		Authorization string
		Model         string `json:"model"`
		Messages      []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"一张血腥暴力风格的裸露商品海报，电影级光影，画面完整"}}]}`))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_opt", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt":       "血腥暴力 裸露 商品海报",
		"mode":         "commercial",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		OriginalPrompt  string   `json:"original_prompt"`
		OptimizedPrompt string   `json:"optimized_prompt"`
		SafetyNotes     []string `json:"safety_notes"`
		Model           string   `json:"model"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	if captured.Path != "/chat/completions" || captured.Authorization != "Bearer deepseek-key" {
		t.Fatalf("unexpected deepseek request: path=%q auth=%q", captured.Path, captured.Authorization)
	}
	if captured.Model != "deepseek-v4" {
		t.Fatalf("expected deepseek-v4 model, got %q", captured.Model)
	}
	if len(captured.Messages) < 2 || !strings.Contains(captured.Messages[0].Content, "规避") || !strings.Contains(captured.Messages[1].Content, "commercial") {
		t.Fatalf("expected safety and mode instructions in request, got %+v", captured.Messages)
	}
	for _, unsafe := range []string{"血腥", "暴力", "裸露"} {
		if strings.Contains(payload.OptimizedPrompt, unsafe) {
			t.Fatalf("optimized prompt still contains unsafe term %q: %q", unsafe, payload.OptimizedPrompt)
		}
	}
	if !strings.Contains(payload.OptimizedPrompt, "商品海报") || len(payload.SafetyNotes) == 0 || payload.Model != "deepseek-v4" {
		t.Fatalf("unexpected optimized payload: %+v", payload)
	}
}

func TestPromptOptimizationSafeModeAvoidsRecognizablePrincessIP(t *testing.T) {
	const recognizablePrincessPrompt = "一位年轻美丽的公主，肌肤白皙细腻，黑色短发，红唇，身着蓝黄配色经典公主裙，置身森林中。阳光穿过树叶洒下柔和光斑，周围小动物环绕，画面唯美，童话风格，高清细节"

	var captured struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, recognizablePrincessPrompt)
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_ip_guard", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt":       recognizablePrincessPrompt,
		"mode":         "safe",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		OptimizedPrompt string   `json:"optimized_prompt"`
		SafetyNotes     []string `json:"safety_notes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	if len(captured.Messages) < 1 || !strings.Contains(captured.Messages[0].Content, "版权") || !strings.Contains(captured.Messages[0].Content, "知名角色") {
		t.Fatalf("expected copyright and character-likeness instruction, got %+v", captured.Messages)
	}
	for _, risky := range []string{"黑色短发", "红唇", "蓝黄配色", "经典公主裙", "小动物环绕"} {
		if strings.Contains(payload.OptimizedPrompt, risky) {
			t.Fatalf("optimized prompt still contains recognizable IP signal %q: %q", risky, payload.OptimizedPrompt)
		}
	}
	if !strings.Contains(payload.OptimizedPrompt, "原创") || !strings.Contains(payload.OptimizedPrompt, "童话") {
		t.Fatalf("expected original fairy-tale rewrite, got %q", payload.OptimizedPrompt)
	}
	if len(payload.SafetyNotes) == 0 || !strings.Contains(strings.Join(payload.SafetyNotes, ","), "知名角色") {
		t.Fatalf("expected IP safety note, got %+v", payload.SafetyNotes)
	}
}

func TestPromptOptimizationPortraitDetailAddsFaceMacroRule(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"成年女性人像特写，面部自然清晰，电影级柔和布光"}}]}`))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_portrait_detail", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt":       "成年女性人像特写，皮肤自然，电影光影",
		"mode":         "portrait_detail",
		"aspect_ratio": "2:3",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		OptimizedPrompt string   `json:"optimized_prompt"`
		SafetyNotes     []string `json:"safety_notes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	if len(captured.Messages) < 2 || !strings.Contains(captured.Messages[0].Content, "人脸高清") || !strings.Contains(captured.Messages[0].Content, "面部绒毛") || !strings.Contains(captured.Messages[1].Content, "portrait_detail") {
		t.Fatalf("expected portrait detail optimization instructions, got %+v", captured.Messages)
	}
	for _, expected := range []string{"真实皮肤毛孔", "细微面部绒毛", "RAW 摄影", "超写实"} {
		if !strings.Contains(payload.OptimizedPrompt, expected) {
			t.Fatalf("expected portrait detail phrase %q in %q", expected, payload.OptimizedPrompt)
		}
	}
	if len(payload.SafetyNotes) == 0 || !strings.Contains(strings.Join(payload.SafetyNotes, ","), "人脸高清") {
		t.Fatalf("expected portrait detail safety note, got %+v", payload.SafetyNotes)
	}
}

func TestPromptOptimizationPortraitDetailSkipsChildSkinMacroTerms(t *testing.T) {
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"儿童人像特写，明亮笑容，柔和自然光"}}]}`))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_portrait_child_guard", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt": "儿童人像特写，笑容自然",
		"mode":   "portrait_detail",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		OptimizedPrompt string   `json:"optimized_prompt"`
		SafetyNotes     []string `json:"safety_notes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	for _, risky := range []string{"真实皮肤毛孔", "细微面部绒毛", "微光泽皮肤", "RAW 摄影"} {
		if strings.Contains(payload.OptimizedPrompt, risky) {
			t.Fatalf("child portrait prompt should not contain macro skin term %q: %q", risky, payload.OptimizedPrompt)
		}
	}
	if !strings.Contains(strings.Join(payload.SafetyNotes, ","), "未成年人") {
		t.Fatalf("expected child guard note, got %+v", payload.SafetyNotes)
	}
}

func TestPromptOptimizationChatModeReturnsStructuredPrompt(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request: %v", err)
		}
		response := map[string]any{
			"reply":            "明白了！我为你整理了提示词，你也可以继续补充或调整。",
			"optimized_prompt": "一只橘白相间的猫坐在花园的石板路上，周围开满色彩丰富的花朵，阳光明媚，写实风格，温暖治愈。",
			"structured_prompt": map[string]string{
				"subject": "橘白相间的猫",
				"scene":   "花园整体，石板路，周围有花",
				"style":   "写实，自然光，温暖治愈",
				"usage":   "社交媒体配图",
			},
			"safety_notes": []string{"已按图片生成场景优化为安全描述"},
		}
		content, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, string(content))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_chat", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt":       "一直小猫",
		"mode":         "chat",
		"aspect_ratio": "1:1",
		"message":      "偏写实，花园整体，温暖治愈。",
		"history": []map[string]string{
			{"role": "user", "content": "一直小猫"},
			{"role": "assistant", "content": "好的！我们先确定几个方向。"},
			{"role": "user", "content": "偏写实，花园整体，温暖治愈。"},
		},
		"structured_prompt": map[string]string{
			"subject": "猫",
			"scene":   "花园",
		},
		"action": "continue",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Reply            string                 `json:"reply"`
		OptimizedPrompt  string                 `json:"optimized_prompt"`
		StructuredPrompt promptStructuredFields `json:"structured_prompt"`
		SafetyNotes      []string               `json:"safety_notes"`
		Model            string                 `json:"model"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	if len(captured.Messages) < 2 ||
		!strings.Contains(captured.Messages[0].Content, "JSON") ||
		!strings.Contains(captured.Messages[0].Content, "基于原始描述推断结构化字段") ||
		!strings.Contains(captured.Messages[0].Content, "不要返回占位文案") ||
		!strings.Contains(captured.Messages[0].Content, "用途采用温和推断") ||
		!strings.Contains(captured.Messages[0].Content, "低置信可空") ||
		!strings.Contains(captured.Messages[0].Content, "字段必须与 optimized_prompt 保持一致") ||
		!strings.Contains(captured.Messages[1].Content, "action=continue") ||
		!strings.Contains(captured.Messages[1].Content, "偏写实") {
		t.Fatalf("expected chat JSON instructions and user context, got %+v", captured.Messages)
	}
	if payload.Reply == "" || !strings.Contains(payload.OptimizedPrompt, "橘白相间的猫") {
		t.Fatalf("unexpected chat response text: %+v", payload)
	}
	if payload.StructuredPrompt.Subject != "橘白相间的猫" || payload.StructuredPrompt.Scene == "" || payload.StructuredPrompt.Style == "" || payload.StructuredPrompt.Usage == "" {
		t.Fatalf("expected structured prompt fields, got %+v", payload.StructuredPrompt)
	}
	if payload.Model != "deepseek-v4" || len(payload.SafetyNotes) == 0 {
		t.Fatalf("unexpected model or safety notes: %+v", payload)
	}
}

func TestPromptOptimizationChangeDirectionReturnsDirections(t *testing.T) {
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"reply":            "可以换成下面 3 个方向。",
			"optimized_prompt": "橘白猫在花园中，写实摄影，自然光。",
			"structured_prompt": map[string]string{
				"subject": "橘白猫",
				"scene":   "花园",
				"style":   "写实摄影，自然光",
				"usage":   "社交媒体配图",
			},
			"directions": []map[string]any{
				{
					"title":   "温暖写实",
					"summary": "自然光下的花园猫咪",
					"prompt":  "一只橘白猫坐在晨光花园里，写实摄影，柔和浅景深。",
					"structured_prompt": map[string]string{
						"subject": "橘白猫",
						"scene":   "晨光花园",
						"style":   "写实摄影，柔和浅景深",
						"usage":   "",
					},
				},
				{
					"title":   "清新插画",
					"summary": "色彩轻盈的花园插画",
					"prompt":  "一只橘白猫在花园里散步，清新插画风格，明亮色彩。",
					"structured_prompt": map[string]string{
						"subject": "橘白猫",
						"scene":   "花园散步",
						"style":   "清新插画，明亮色彩",
						"usage":   "儿童读物插图",
					},
				},
				{
					"title":   "电影夜景",
					"summary": "夜晚花园的电影感画面",
					"prompt":  "一只橘白猫穿过夜晚花园，电影感光影，蓝金色调。",
					"structured_prompt": map[string]string{
						"subject": "橘白猫",
						"scene":   "夜晚花园",
						"style":   "电影感光影，蓝金色调",
						"usage":   "",
					},
				},
			},
		}
		content, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, string(content))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_direction", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt":  "花园里的猫",
		"mode":    "direction",
		"message": "换个方向",
		"action":  "change_direction",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Reply           string `json:"reply"`
		OptimizedPrompt string `json:"optimized_prompt"`
		Directions      []struct {
			Title            string                 `json:"title"`
			Summary          string                 `json:"summary"`
			Prompt           string                 `json:"prompt"`
			StructuredPrompt promptStructuredFields `json:"structured_prompt"`
		} `json:"directions"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	if payload.Reply == "" || len(payload.Directions) != 3 {
		t.Fatalf("expected reply and 3 directions, got %+v", payload)
	}
	if payload.Directions[1].Title != "清新插画" || !strings.Contains(payload.Directions[1].Prompt, "清新插画") {
		t.Fatalf("unexpected directions: %+v", payload.Directions)
	}
	if payload.Directions[1].StructuredPrompt.Scene != "花园散步" || payload.Directions[1].StructuredPrompt.Usage != "儿童读物插图" {
		t.Fatalf("expected structured prompt on direction, got %+v", payload.Directions[1].StructuredPrompt)
	}
}

func TestPromptOptimizationFallbackTextReturnsEmptyStructuredFields(t *testing.T) {
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"未来城市夜景，霓虹灯闪烁，电影感构图"}}]}`))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	_, cookies := createLoggedInUser(t, testApp, "creator_prompt_plain_fallback", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompts/optimize", map[string]any{
		"prompt": "未来城市夜景",
		"mode":   "chat",
		"action": "start",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prompt optimization payload: %v", err)
	}
	structured, ok := payload["structured_prompt"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured_prompt object, got %+v", payload["structured_prompt"])
	}
	for _, key := range []string{"subject", "scene", "style", "usage"} {
		if value, exists := structured[key]; !exists || value != "" {
			t.Fatalf("expected empty structured_prompt.%s, got value=%#v exists=%v in %+v", key, value, exists, structured)
		}
	}
}

func TestPromptTemplatesListAndUseDeductsCredits(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_prompt_template_use", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/prompt-templates", nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected template list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			ID          uint   `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			PreviewURL  string `json:"preview_url"`
			Prompt      string `json:"prompt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode template list: %v", err)
	}
	if len(listPayload.Items) < 8 {
		t.Fatalf("expected seeded prompt templates, got %+v", listPayload.Items)
	}
	first := listPayload.Items[0]
	if first.ID == 0 || first.Title == "" || first.Description == "" {
		t.Fatalf("expected list item with basic template fields, got %+v", first)
	}
	if first.PreviewURL != "" {
		t.Fatalf("template list must not synthesize simulated preview URLs, got %+v", first)
	}
	if first.Prompt != "" {
		t.Fatalf("template list must not expose full prompt before paid use: %+v", first)
	}

	useResp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompt-templates/"+itoa(first.ID)+"/use", nil, cookies)
	if useResp.Code != http.StatusOK {
		t.Fatalf("expected use template 200, got %d: %s", useResp.Code, useResp.Body.String())
	}
	var usePayload struct {
		Prompt           string `json:"prompt"`
		AvailableCredits int    `json:"available_credits"`
	}
	if err := json.Unmarshal(useResp.Body.Bytes(), &usePayload); err != nil {
		t.Fatalf("decode use template payload: %v", err)
	}
	if usePayload.Prompt == "" || usePayload.AvailableCredits != 1 {
		t.Fatalf("expected prompt and remaining credits 1, got %+v", usePayload)
	}

	secondUseResp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompt-templates/"+itoa(first.ID)+"/use", nil, cookies)
	if secondUseResp.Code != http.StatusOK {
		t.Fatalf("expected repeated use 200, got %d: %s", secondUseResp.Code, secondUseResp.Body.String())
	}
	var secondUsePayload struct {
		AvailableCredits int `json:"available_credits"`
	}
	if err := json.Unmarshal(secondUseResp.Body.Bytes(), &secondUsePayload); err != nil {
		t.Fatalf("decode second use payload: %v", err)
	}
	if secondUsePayload.AvailableCredits != 0 {
		t.Fatalf("expected repeated template use to deduct another credit, got %+v", secondUsePayload)
	}

	var transactions []CreditTransaction
	if err := testApp.db.Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypePromptTemplateUse).Order("id asc").Find(&transactions).Error; err != nil {
		t.Fatalf("load template transactions: %v", err)
	}
	if len(transactions) != 2 || transactions[0].Amount != -1 || transactions[1].Amount != -1 {
		t.Fatalf("expected two template deduction transactions, got %+v", transactions)
	}
}

func TestPromptTemplateUseRequiresCredit(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_prompt_template_no_credit", "test-password")
	setUserCredits(t, testApp, user.ID, 0)

	var template PromptTemplate
	if err := testApp.db.Where("is_active = ?", true).Order("sort_order asc, id asc").First(&template).Error; err != nil {
		t.Fatalf("load template: %v", err)
	}
	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/prompt-templates/"+itoa(template.ID)+"/use", nil, cookies)
	if resp.Code != http.StatusConflict || !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"credits_insufficient"`)) {
		t.Fatalf("expected credits_insufficient, got %d: %s", resp.Code, resp.Body.String())
	}
	var txCount int64
	if err := testApp.db.Model(&CreditTransaction{}).
		Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypePromptTemplateUse).
		Count(&txCount).Error; err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if txCount != 0 {
		t.Fatalf("expected no transaction for failed template use, got %d", txCount)
	}
}

func TestWorkspaceDiscoveryReturnsActiveTemplatesWithoutDeductingCredits(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, cookies := createLoggedInUser(t, testApp, "creator_workspace_discovery", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	modelID := configureImageModelCenterRouteForEstimateTest(t, testApp, adminCookies, 2)

	if err := testApp.db.Create(&PromptTemplate{
		Slug:              "workspace-hot-template",
		Title:             "工作台热门模板",
		Category:          "电商产品海报",
		Description:       "后台配置的热门模板",
		Prompt:            "生成一个工作台热门模板提示词",
		AspectRatio:       "4:3",
		StylePreset:       "海报",
		Theme:             "workspace-hot",
		WorkspaceSection:  "hot",
		WorkspaceToolMode: GenerationToolModeGenerate,
		WorkspaceSort:     5,
		IsActive:          true,
	}).Error; err != nil {
		t.Fatalf("create hot template: %v", err)
	}
	if err := testApp.db.Create(&PromptTemplate{
		Slug:              "workspace-inspiration-template",
		Title:             "工作台灵感模板",
		Category:          "角色灵感",
		Description:       "后台配置的灵感模板",
		Prompt:            "生成一个工作台灵感模板提示词",
		AspectRatio:       "9:16",
		StylePreset:       "写实",
		Theme:             "workspace-inspiration",
		WorkspaceSection:  "inspiration",
		WorkspaceToolMode: GenerationToolModeRemoveBackground,
		WorkspaceSort:     6,
		IsActive:          true,
	}).Error; err != nil {
		t.Fatalf("create inspiration template: %v", err)
	}
	disabledTemplate := PromptTemplate{
		Slug:             "workspace-disabled-template",
		Title:            "停用模板",
		Prompt:           "不应返回",
		WorkspaceSection: "hot",
		IsActive:         false,
	}
	if err := testApp.db.Create(&disabledTemplate).Error; err != nil {
		t.Fatalf("create disabled template: %v", err)
	}
	if err := testApp.db.Model(&disabledTemplate).Update("is_active", false).Error; err != nil {
		t.Fatalf("disable template: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/workspace/discovery", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Tools []struct {
			Mode           string           `json:"mode"`
			Title          string           `json:"title"`
			Enabled        bool             `json:"enabled"`
			RequiresSource bool             `json:"requires_source"`
			SourceLimit    int              `json:"source_limit"`
			FormSchema     []workspaceField `json:"form_schema"`
		} `json:"tools"`
		Models []struct {
			ID                 uint     `json:"id"`
			Name               string   `json:"name"`
			DefaultCreditsCost int      `json:"default_credits_cost"`
			CapabilityTags     []string `json:"capability_tags"`
		} `json:"models"`
		Hot []struct {
			Title       string `json:"title"`
			Prompt      string `json:"prompt"`
			AspectRatio string `json:"aspect_ratio"`
			StylePreset string `json:"style_preset"`
			ToolMode    string `json:"tool_mode"`
			SortOrder   int    `json:"sort_order"`
		} `json:"hot"`
		Inspiration []struct {
			Title    string `json:"title"`
			Prompt   string `json:"prompt"`
			ToolMode string `json:"tool_mode"`
		} `json:"inspiration"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode discovery payload: %v", err)
	}
	if len(payload.Tools) == 0 {
		t.Fatalf("expected backend configured workspace tools, got none")
	}
	if got, want := len(payload.Tools), 5; got != want {
		t.Fatalf("expected exactly %d supported workspace tools, got %d: %+v", want, got, payload.Tools)
	}
	expectedToolModes := []string{
		GenerationToolModeExpand,
		GenerationToolModeErase,
		GenerationToolModeRemoveBackground,
		GenerationToolModeUpscale,
		GenerationToolModePrecisionEdit,
	}
	for index, mode := range expectedToolModes {
		if payload.Tools[index].Mode != mode {
			t.Fatalf("expected tool %d to be %q, got %+v", index, mode, payload.Tools[index])
		}
	}
	var upscaleTool *struct {
		Mode           string           `json:"mode"`
		Title          string           `json:"title"`
		Enabled        bool             `json:"enabled"`
		RequiresSource bool             `json:"requires_source"`
		SourceLimit    int              `json:"source_limit"`
		FormSchema     []workspaceField `json:"form_schema"`
	}
	var removeBackgroundTool *struct {
		Mode           string           `json:"mode"`
		Title          string           `json:"title"`
		Enabled        bool             `json:"enabled"`
		RequiresSource bool             `json:"requires_source"`
		SourceLimit    int              `json:"source_limit"`
		FormSchema     []workspaceField `json:"form_schema"`
	}
	for index := range payload.Tools {
		if payload.Tools[index].Mode == GenerationToolModeUpscale {
			upscaleTool = &payload.Tools[index]
		}
		if payload.Tools[index].Mode == GenerationToolModeRemoveBackground {
			removeBackgroundTool = &payload.Tools[index]
		}
	}
	if upscaleTool == nil || !upscaleTool.Enabled || len(upscaleTool.FormSchema) == 0 {
		t.Fatalf("expected enabled upscale tool with schema, got %+v", payload.Tools)
	}
	if !upscaleTool.RequiresSource || upscaleTool.SourceLimit != 1 {
		t.Fatalf("expected upscale to require exactly one source, got %+v", *upscaleTool)
	}
	var hasUpscaleScale bool
	var hasUpscaleEditInstruction bool
	for _, field := range upscaleTool.FormSchema {
		if field.Key == "scale" && field.Type == "select" && reflect.DeepEqual(field.Options, []string{"2x", "4x", "8x"}) {
			hasUpscaleScale = true
		}
		if field.Key == "edit_instruction" && field.Type == "textarea" {
			hasUpscaleEditInstruction = true
		}
	}
	if !hasUpscaleScale || !hasUpscaleEditInstruction {
		t.Fatalf("expected upscale scale and edit instruction fields, got %+v", upscaleTool.FormSchema)
	}
	if removeBackgroundTool == nil || !removeBackgroundTool.Enabled || !removeBackgroundTool.RequiresSource ||
		removeBackgroundTool.SourceLimit != 1 || len(removeBackgroundTool.FormSchema) == 0 {
		t.Fatalf("expected enabled remove background tool with optional schema, got %+v", payload.Tools)
	}
	var discoveredModel *struct {
		ID                 uint     `json:"id"`
		Name               string   `json:"name"`
		DefaultCreditsCost int      `json:"default_credits_cost"`
		CapabilityTags     []string `json:"capability_tags"`
	}
	for index := range payload.Models {
		if payload.Models[index].ID == modelID {
			discoveredModel = &payload.Models[index]
			break
		}
	}
	if discoveredModel == nil || discoveredModel.DefaultCreditsCost != 1 {
		t.Fatalf("expected online image model in discovery, got %+v", payload.Models)
	}
	var hotTemplate *struct {
		Title       string `json:"title"`
		Prompt      string `json:"prompt"`
		AspectRatio string `json:"aspect_ratio"`
		StylePreset string `json:"style_preset"`
		ToolMode    string `json:"tool_mode"`
		SortOrder   int    `json:"sort_order"`
	}
	for index := range payload.Hot {
		if payload.Hot[index].Title == "工作台热门模板" {
			hotTemplate = &payload.Hot[index]
			break
		}
	}
	if hotTemplate == nil || hotTemplate.Prompt == "" {
		t.Fatalf("expected active hot template with prompt, got %+v", payload.Hot)
	}
	if hotTemplate.AspectRatio != "4:3" || hotTemplate.StylePreset != "海报" || hotTemplate.ToolMode != GenerationToolModeGenerate {
		t.Fatalf("expected fillable hot template fields, got %+v", *hotTemplate)
	}
	var inspirationTemplate *struct {
		Title    string `json:"title"`
		Prompt   string `json:"prompt"`
		ToolMode string `json:"tool_mode"`
	}
	for index := range payload.Inspiration {
		if payload.Inspiration[index].Title == "工作台灵感模板" {
			inspirationTemplate = &payload.Inspiration[index]
			break
		}
	}
	if inspirationTemplate == nil || inspirationTemplate.ToolMode != GenerationToolModeRemoveBackground {
		t.Fatalf("expected active inspiration template, got %+v", payload.Inspiration)
	}
	if bytes.Contains(resp.Body.Bytes(), []byte("停用模板")) {
		t.Fatalf("discovery must not return disabled templates: %s", resp.Body.String())
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 2 {
		t.Fatalf("workspace discovery must not deduct credits, got %d", balance.AvailableCredits)
	}
	var transactionCount int64
	if err := testApp.db.Model(&CreditTransaction{}).Where("user_id = ?", user.ID).Count(&transactionCount).Error; err != nil {
		t.Fatalf("count credit transactions: %v", err)
	}
	if transactionCount != 1 {
		t.Fatalf("expected only signup bonus transaction, got %d", transactionCount)
	}
}

func TestWorkspaceDiscoveryAllowsAnonymousRead(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	modelID := configureImageModelCenterRouteForEstimateTest(t, testApp, adminCookies, 2)

	if err := testApp.db.Create(&PromptTemplate{
		Slug:              "anonymous-workspace-hot-template",
		Title:             "游客可见热门案例",
		Category:          "公开案例",
		Description:       "无需登录即可浏览",
		Prompt:            "生成一个公开热门案例",
		AspectRatio:       "4:3",
		StylePreset:       "海报",
		Theme:             "anonymous-hot",
		WorkspaceSection:  "hot",
		WorkspaceToolMode: GenerationToolModeGenerate,
		WorkspaceModelID:  modelID,
		WorkspaceSort:     5,
		IsActive:          true,
	}).Error; err != nil {
		t.Fatalf("create anonymous hot template: %v", err)
	}
	if err := testApp.db.Create(&PromptTemplate{
		Slug:              "anonymous-workspace-inspiration-template",
		Title:             "游客可见灵感案例",
		Category:          "公开灵感",
		Description:       "无需登录即可浏览灵感",
		Prompt:            "生成一个公开灵感案例",
		AspectRatio:       "9:16",
		StylePreset:       "写实",
		Theme:             "anonymous-inspiration",
		WorkspaceSection:  "inspiration",
		WorkspaceToolMode: GenerationToolModeRemoveBackground,
		WorkspaceSort:     6,
		IsActive:          true,
	}).Error; err != nil {
		t.Fatalf("create anonymous inspiration template: %v", err)
	}
	disabledTemplate := PromptTemplate{
		Slug:             "anonymous-workspace-disabled-template",
		Title:            "游客不可见停用案例",
		Prompt:           "不应返回",
		WorkspaceSection: "hot",
		IsActive:         false,
	}
	if err := testApp.db.Create(&disabledTemplate).Error; err != nil {
		t.Fatalf("create anonymous disabled template: %v", err)
	}
	if err := testApp.db.Model(&disabledTemplate).Update("is_active", false).Error; err != nil {
		t.Fatalf("disable anonymous template: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/workspace/discovery", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected anonymous discovery 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Tools  []workspaceTool `json:"tools"`
		Models []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
		Hot []struct {
			Title       string `json:"title"`
			Prompt      string `json:"prompt"`
			AspectRatio string `json:"aspect_ratio"`
			StylePreset string `json:"style_preset"`
			ToolMode    string `json:"tool_mode"`
			ModelID     uint   `json:"model_id"`
		} `json:"hot"`
		Inspiration []struct {
			Title    string `json:"title"`
			Prompt   string `json:"prompt"`
			ToolMode string `json:"tool_mode"`
		} `json:"inspiration"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode anonymous discovery payload: %v", err)
	}
	if len(payload.Tools) == 0 {
		t.Fatalf("expected public workspace tools, got none")
	}
	var discoveredModel bool
	for _, model := range payload.Models {
		if model.ID == modelID && model.Name != "" {
			discoveredModel = true
			break
		}
	}
	if !discoveredModel {
		t.Fatalf("expected public workspace models to include configured model %d, got %+v", modelID, payload.Models)
	}
	var hotFound bool
	for _, template := range payload.Hot {
		if template.Title == "游客可见热门案例" {
			hotFound = template.Prompt != "" &&
				template.AspectRatio == "4:3" &&
				template.StylePreset == "海报" &&
				template.ToolMode == GenerationToolModeGenerate &&
				template.ModelID == modelID
			break
		}
	}
	if !hotFound {
		t.Fatalf("expected active anonymous hot template with fillable fields, got %+v", payload.Hot)
	}
	var inspirationFound bool
	for _, template := range payload.Inspiration {
		if template.Title == "游客可见灵感案例" {
			inspirationFound = template.Prompt != "" && template.ToolMode == GenerationToolModeRemoveBackground
			break
		}
	}
	if !inspirationFound {
		t.Fatalf("expected active anonymous inspiration template, got %+v", payload.Inspiration)
	}
	if bytes.Contains(resp.Body.Bytes(), []byte("游客不可见停用案例")) {
		t.Fatalf("anonymous discovery must not return disabled templates: %s", resp.Body.String())
	}
}

func TestPromptTemplateListReturnsStoredPreviewURL(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_prompt_template_preview_url", "test-password")
	setUserCredits(t, testApp, user.ID, 1)

	var template PromptTemplate
	if err := testApp.db.Where("is_active = ?", true).Order("sort_order asc, id asc").First(&template).Error; err != nil {
		t.Fatalf("load template: %v", err)
	}
	const previewURL = "https://oss.example.com/templates/preview.png"
	if err := testApp.db.Model(&template).Updates(map[string]any{
		"preview_asset_key": "templates/preview.png",
		"preview_url":       previewURL,
		"preview_mime_type": "image/png",
	}).Error; err != nil {
		t.Fatalf("update template preview fields: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/prompt-templates", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected template list 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID         uint   `json:"id"`
			PreviewURL string `json:"preview_url"`
			Prompt     string `json:"prompt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode template list: %v", err)
	}
	if len(payload.Items) == 0 || payload.Items[0].PreviewURL != previewURL {
		t.Fatalf("expected stored preview URL in list, got %+v", payload.Items)
	}
	if payload.Items[0].Prompt != "" {
		t.Fatalf("template list must still hide paid prompt, got %+v", payload.Items[0])
	}
}

func TestPromptTemplatePreviewRedirectsToStoredPublicAsset(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.assetStore = &publicURLAssetStore{
		key:       "templates/preview.png",
		mimeType:  "image/png",
		publicURL: "https://oss.example.com/templates/preview.png",
		content:   []byte("stored-preview"),
	}
	var template PromptTemplate
	if err := testApp.db.Where("is_active = ?", true).Order("sort_order asc, id asc").First(&template).Error; err != nil {
		t.Fatalf("load template: %v", err)
	}
	if err := testApp.db.Model(&template).Updates(map[string]any{
		"preview_asset_key": "templates/preview.png",
		"preview_url":       "https://oss.example.com/templates/preview.png",
		"preview_mime_type": "image/png",
	}).Error; err != nil {
		t.Fatalf("update template preview fields: %v", err)
	}

	resp := performRequest(t, testApp, http.MethodGet, "/api/public/prompt-templates/"+itoa(template.ID)+"/preview", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected public preview redirect, got %d: %s", resp.Code, resp.Body.String())
	}
	if location := resp.Header().Get("Location"); location != "https://oss.example.com/templates/preview.png" {
		t.Fatalf("expected OSS preview redirect, got %q", location)
	}
}

func TestPromptTemplatePreviewRequiresGeneratedAsset(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	var template PromptTemplate
	if err := testApp.db.Where("is_active = ?", true).Order("sort_order asc, id asc").First(&template).Error; err != nil {
		t.Fatalf("load template: %v", err)
	}

	resp := performRequest(t, testApp, http.MethodGet, "/api/public/prompt-templates/"+itoa(template.ID)+"/preview", nil, nil)
	if resp.Code != http.StatusNotFound || !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"prompt_template_preview_not_generated"`)) {
		t.Fatalf("expected missing preview error, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestGenerateMissingPromptTemplatePreviewsPersistsOSSURL(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("generated-template-preview")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_template_preview",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "templates/generated.png",
		publicURL: "https://oss.example.com/templates/generated.png",
	}

	report, err := testApp.GenerateMissingPromptTemplatePreviews(context.Background(), PromptTemplatePreviewGenerationOptions{Limit: 1})
	if err != nil {
		t.Fatalf("generate template previews: %v", err)
	}
	if report.Generated != 1 || report.Failed != 0 || provider.calls != 1 {
		t.Fatalf("expected one generated preview, report=%+v provider calls=%d", report, provider.calls)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected provider input")
	}
	if provider.inputs[0].Prompt == "" || provider.inputs[0].AspectRatio == "" || provider.inputs[0].Size == "" || provider.inputs[0].Model != testApp.cfg.DefaultImageModel {
		t.Fatalf("expected template generation input with model/prompt/aspect ratio/size, got %+v", provider.inputs[0])
	}

	var template PromptTemplate
	if err := testApp.db.Where("preview_asset_key = ?", "templates/generated.png").First(&template).Error; err != nil {
		t.Fatalf("load generated template preview: %v", err)
	}
	if template.PreviewURL != "https://oss.example.com/templates/generated.png" || template.PreviewMIMEType != "image/png" || template.PreviewProviderRequestID != "req_template_preview" || template.PreviewGeneratedAt == nil {
		t.Fatalf("expected stored generated preview fields, got %+v", template)
	}
}

func TestGenerateMissingPromptTemplatePreviewsRetriesTransientProviderFailures(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{HTTPStatus: http.StatusServiceUnavailable, Code: "provider_unavailable", Message: "系统繁忙，请稍后再试", FailureStage: providerFailureStageImageGenerationRequest},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("retried-template-preview")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_template_preview_retry",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "templates/retried.png",
		publicURL: "https://oss.example.com/templates/retried.png",
	}

	report, err := testApp.GenerateMissingPromptTemplatePreviews(context.Background(), PromptTemplatePreviewGenerationOptions{Limit: 1})
	if err != nil {
		t.Fatalf("generate template previews: %v", err)
	}
	if report.Generated != 1 || report.Failed != 0 || provider.calls != 2 {
		t.Fatalf("expected transient failure retried into success, report=%+v calls=%d", report, provider.calls)
	}

	var template PromptTemplate
	if err := testApp.db.Where("preview_asset_key = ?", "templates/retried.png").First(&template).Error; err != nil {
		t.Fatalf("load retried generated template preview: %v", err)
	}
	if template.PreviewStatus != promptTemplatePreviewStatusGenerated || template.PreviewErrorMessage != "" || template.PreviewProviderRequestID != "req_template_preview_retry" {
		t.Fatalf("expected retried preview success to clear failure state, got %+v", template)
	}
}

func TestGenerateMissingPromptTemplatePreviewsSkipsPreviouslyFailedTemplates(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("next-missing-template-preview")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_next_missing_template_preview",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "templates/next-missing.png",
		publicURL: "https://oss.example.com/templates/next-missing.png",
	}

	var failedTemplate PromptTemplate
	if err := testApp.db.Where("slug = ?", "chang-an-night-market-app").First(&failedTemplate).Error; err != nil {
		t.Fatalf("load failed template candidate: %v", err)
	}
	if err := testApp.db.Model(&failedTemplate).Updates(map[string]any{
		"preview_status":        promptTemplatePreviewStatusFailed,
		"preview_error_message": "模型通道认证已失效",
		"preview_asset_key":     "",
		"preview_url":           "",
	}).Error; err != nil {
		t.Fatalf("mark template failed: %v", err)
	}

	report, err := testApp.GenerateMissingPromptTemplatePreviews(context.Background(), PromptTemplatePreviewGenerationOptions{Limit: 1})
	if err != nil {
		t.Fatalf("generate missing template previews: %v", err)
	}
	if report.Generated != 1 || report.Failed != 0 || provider.calls != 1 {
		t.Fatalf("expected one non-failed missing template generated, report=%+v calls=%d", report, provider.calls)
	}
	if len(provider.inputs) != 1 || provider.inputs[0].Prompt == failedTemplate.Prompt {
		t.Fatalf("batch missing generation must skip previously failed template, inputs=%+v", provider.inputs)
	}

	var stillFailed PromptTemplate
	if err := testApp.db.First(&stillFailed, failedTemplate.ID).Error; err != nil {
		t.Fatalf("reload failed template: %v", err)
	}
	if stillFailed.PreviewStatus != promptTemplatePreviewStatusFailed || stillFailed.PreviewErrorMessage != "模型通道认证已失效" {
		t.Fatalf("expected previously failed template to remain untouched, got %+v", stillFailed)
	}
}

func TestAdminPromptTemplatesCRUDAndPreviewGeneration(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("admin-template-preview")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_admin_template_preview",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "templates/admin-generated.png",
		publicURL: "https://oss.example.com/templates/admin-generated.png",
	}
	adminCookies := createAdminSession(t, testApp)

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/prompt-templates?page=1&page_size=5", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected admin template list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			ID            uint   `json:"id"`
			Prompt        string `json:"prompt"`
			PreviewURL    string `json:"preview_url"`
			PreviewStatus string `json:"preview_status"`
			IsActive      bool   `json:"is_active"`
			CostCredits   int    `json:"cost_credits"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode admin template list: %v", err)
	}
	if listPayload.Total < 12 || len(listPayload.Items) == 0 || listPayload.Items[0].Prompt == "" || listPayload.Items[0].CostCredits != 1 {
		t.Fatalf("expected admin list to expose editable template fields, got %+v", listPayload)
	}

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/prompt-templates", map[string]any{
		"slug":                "admin-template-test",
		"title":               "后台新增模板",
		"category":            "测试分类",
		"description":         "后台创建的模板",
		"prompt":              "生成一张后台模板预览图",
		"aspect_ratio":        "1:1",
		"style_preset":        "写实",
		"theme":               "admin-test",
		"workspace_section":   "hot",
		"workspace_tool_mode": GenerationToolModeRemoveBackground,
		"workspace_sort":      77,
		"sort_order":          999,
		"is_active":           true,
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create template 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created PromptTemplate
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created template: %v", err)
	}
	if created.ID == 0 || created.Slug != "admin-template-test" || created.Prompt == "" ||
		created.WorkspaceSection != "hot" || created.WorkspaceToolMode != GenerationToolModeRemoveBackground || created.WorkspaceSort != 77 {
		t.Fatalf("unexpected created template: %+v", created)
	}

	disabledCreateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/prompt-templates", map[string]any{
		"slug":      "admin-disabled-template-test",
		"title":     "后台停用模板",
		"prompt":    "这个模板不应启用",
		"is_active": false,
	}, adminCookies)
	if disabledCreateResp.Code != http.StatusCreated {
		t.Fatalf("expected create disabled template 201, got %d: %s", disabledCreateResp.Code, disabledCreateResp.Body.String())
	}
	var disabledCreated PromptTemplate
	if err := json.Unmarshal(disabledCreateResp.Body.Bytes(), &disabledCreated); err != nil {
		t.Fatalf("decode disabled template: %v", err)
	}
	if disabledCreated.IsActive {
		t.Fatalf("expected created template to remain inactive, got %+v", disabledCreated)
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/prompt-templates/"+itoa(created.ID), map[string]any{
		"title":               "后台更新模板",
		"workspace_section":   "inspiration",
		"workspace_tool_mode": GenerationToolModeExpand,
		"workspace_sort":      88,
		"is_active":           false,
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update template 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	var updated PromptTemplate
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated template: %v", err)
	}
	if updated.WorkspaceSection != "inspiration" || updated.WorkspaceToolMode != GenerationToolModeExpand || updated.WorkspaceSort != 88 {
		t.Fatalf("expected updated workspace template fields, got %+v", updated)
	}

	previewResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/prompt-templates/"+itoa(created.ID)+"/preview", map[string]any{"force": true}, adminCookies)
	if previewResp.Code != http.StatusAccepted {
		t.Fatalf("expected generate template preview 202, got %d: %s", previewResp.Code, previewResp.Body.String())
	}
	var previewPayload struct {
		Status string `json:"status"`
		Queued int    `json:"queued"`
	}
	if err := json.Unmarshal(previewResp.Body.Bytes(), &previewPayload); err != nil {
		t.Fatalf("decode preview generation payload: %v", err)
	}
	if previewPayload.Status != "queued" || previewPayload.Queued != 1 {
		t.Fatalf("expected queued preview payload, got %+v", previewPayload)
	}
	var generatedTemplate PromptTemplate
	waitForCondition(t, time.Second, func() bool {
		err := testApp.db.First(&generatedTemplate, created.ID).Error
		return err == nil && generatedTemplate.PreviewURL == "https://oss.example.com/templates/admin-generated.png" && generatedTemplate.PreviewStatus == "generated"
	})
	if generatedTemplate.PreviewProviderRequestID != "req_admin_template_preview" || generatedTemplate.PreviewGeneratedAt == nil || generatedTemplate.PreviewLastFinishedAt == nil {
		t.Fatalf("expected background preview generation to persist status and metadata, got %+v", generatedTemplate)
	}
	if provider.calls != 1 || provider.inputs[0].Prompt != "生成一张后台模板预览图" {
		t.Fatalf("expected provider to generate from template prompt, calls=%d inputs=%+v", provider.calls, provider.inputs)
	}

	batchResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/prompt-templates/previews/generate", map[string]any{"limit": 1}, adminCookies)
	if batchResp.Code != http.StatusAccepted {
		t.Fatalf("expected batch preview generation 202, got %d: %s", batchResp.Code, batchResp.Body.String())
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/prompt-templates/"+itoa(created.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete template 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	var deleted PromptTemplate
	if err := testApp.db.Unscoped().First(&deleted, created.ID).Error; err != nil {
		t.Fatalf("load deleted template: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected prompt template to be soft deleted")
	}
}

func TestAdminPromptTemplatePreviewStoresAsyncFailure(t *testing.T) {
	provider := &stubProvider{
		err: &ProviderError{
			Code:    "provider_busy",
			Message: "系统繁忙，请稍后再试",
		},
	}
	testApp, _ := newTestApp(t, provider)
	adminCookies := createAdminSession(t, testApp)

	var template PromptTemplate
	if err := testApp.db.Where("slug = ?", "chang-an-night-market-app").First(&template).Error; err != nil {
		t.Fatalf("load template: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/prompt-templates/"+itoa(template.ID)+"/preview", map[string]any{"force": true}, adminCookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected async preview generation 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated PromptTemplate
	waitForCondition(t, time.Second, func() bool {
		err := testApp.db.First(&updated, template.ID).Error
		return err == nil && updated.PreviewStatus == promptTemplatePreviewStatusFailed
	})
	if updated.PreviewErrorMessage != "系统繁忙，请稍后再试" || updated.PreviewLastFinishedAt == nil {
		t.Fatalf("expected async preview failure to be stored on template, got %+v", updated)
	}
}

func TestGenerationProviderFailureReturnsRawProviderMessage(t *testing.T) {
	const providerMessage = "提交中含有违反平台政策的内容，请你立即停止或调整你的提交内容（traceid: sync-policy-1）"

	provider := &stubProvider{
		err: &ProviderError{
			HTTPStatus:        http.StatusInternalServerError,
			Code:              "<nil>",
			Message:           providerMessage,
			ProviderRequestID: "req_sync_policy",
			FailureStage:      providerFailureStageImageGenerationRequest,
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_sync_raw_error", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "policy rejected image",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload.Error.Code != "provider_policy_rejected" || payload.Error.Message != providerMessage {
		t.Fatalf("expected raw provider error response, got %+v", payload.Error)
	}
}

func TestGenerationPersistsStorePublicURLWhenAvailable(t *testing.T) {
	const publicURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/generated.png"

	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "ZmFrZS1pbWFnZQ==",
			MIMEType:          "image/png",
			ProviderRequestID: "req_oss",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/generated.png",
		mimeType:  "image/png",
		publicURL: publicURL,
	}
	user, cookies := createLoggedInUser(t, testApp, "creator_gen_oss", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "oss public image url",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint   `json:"generation_id"`
		WorkID       uint   `json:"work_id"`
		PreviewURL   string `json:"preview_url"`
		DownloadURL  string `json:"download_url"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	if payload.PreviewURL != publicURL || payload.DownloadURL != publicURL {
		t.Fatalf("expected OSS public urls in response, got %+v", payload)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.PreviewURL != publicURL || record.DownloadURL != publicURL {
		t.Fatalf("expected OSS public urls on record, got %+v", record)
	}

	var work Work
	if err := testApp.db.First(&work, payload.WorkID).Error; err != nil {
		t.Fatalf("load work: %v", err)
	}
	if work.PreviewURL != publicURL || work.DownloadURL != publicURL {
		t.Fatalf("expected OSS public urls on work, got %+v", work)
	}
}

func TestLegacyWorkURLsResolveToStorePublicURLWhenAvailable(t *testing.T) {
	const publicURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/legacy.png"

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/legacy.png",
		mimeType:  "image/png",
		publicURL: publicURL,
	}
	user, cookies := createLoggedInUser(t, testApp, "creator_legacy_oss", "test-password")

	record := GenerationRecord{
		UserID:      user.ID,
		Prompt:      "legacy oss record",
		AspectRatio: "1:1",
		Model:       "gpt-image-2",
		Status:      GenerationStatusSucceeded,
		Stage:       GenerationStageSucceeded,
		AssetKey:    "assets/2026/05/legacy.png",
		MIMEType:    "image/png",
	}
	if err := testApp.db.Create(&record).Error; err != nil {
		t.Fatalf("create generation record: %v", err)
	}
	work := Work{
		UserID:             user.ID,
		GenerationRecordID: record.ID,
		Prompt:             record.Prompt,
		AspectRatio:        record.AspectRatio,
		Model:              record.Model,
		Status:             GenerationStatusSucceeded,
		Visibility:         WorkVisibilityPrivate,
		AssetKey:           record.AssetKey,
		MIMEType:           record.MIMEType,
		PreviewURL:         "/api/works/1/file",
		DownloadURL:        "/api/works/1/download",
	}
	if err := testApp.db.Create(&work).Error; err != nil {
		t.Fatalf("create legacy work: %v", err)
	}
	record.WorkID = &work.ID
	record.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
	record.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
	if err := testApp.db.Save(&record).Error; err != nil {
		t.Fatalf("save legacy record urls: %v", err)
	}
	if err := testApp.db.Model(&work).Updates(map[string]any{
		"preview_url":  record.PreviewURL,
		"download_url": record.DownloadURL,
	}).Error; err != nil {
		t.Fatalf("save legacy work urls: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works", nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected works list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []Work `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode works list: %v", err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].PreviewURL != publicURL || listPayload.Items[0].DownloadURL != publicURL {
		t.Fatalf("expected public urls in works list, got %+v", listPayload.Items)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/works/%d", work.ID), nil, cookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected work detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var detail Work
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode work detail: %v", err)
	}
	if detail.PreviewURL != publicURL || detail.DownloadURL != publicURL {
		t.Fatalf("expected public urls in work detail, got %+v", detail)
	}

	generationResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/images/generations/%d", record.ID), nil, cookies)
	if generationResp.Code != http.StatusOK {
		t.Fatalf("expected generation detail 200, got %d: %s", generationResp.Code, generationResp.Body.String())
	}
	if !bytes.Contains(generationResp.Body.Bytes(), []byte(`"preview_url":"`+publicURL+`"`)) {
		t.Fatalf("expected public urls in generation payload: %s", generationResp.Body.String())
	}

	fileResp := performRequest(t, testApp, http.MethodGet, record.PreviewURL, nil, cookies)
	if fileResp.Code != http.StatusFound {
		t.Fatalf("expected legacy file route redirect, got %d: %s", fileResp.Code, fileResp.Body.String())
	}
	if location := fileResp.Header().Get("Location"); location != publicURL {
		t.Fatalf("expected redirect to %q, got %q", publicURL, location)
	}
}

func TestReferenceAssetsUploadListServeDeleteAndOwnership(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_refs", "test-password")
	_, viewerCookies := createLoggedInUser(t, testApp, "creator_refs_other", "test-password")

	uploadResp := performMultipartRequest(t, testApp, http.MethodPost, "/api/reference-assets", "file", "lamp.png", mustBase64Decode(t, fakePNGBase64), ownerCookies)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}

	var created struct {
		ID               uint   `json:"id"`
		PreviewURL       string `json:"preview_url"`
		OriginalFilename string `json:"original_filename"`
		MIMEType         string `json:"mime_type"`
	}
	if err := json.Unmarshal(uploadResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode upload payload: %v", err)
	}
	if created.ID == 0 || created.PreviewURL == "" {
		t.Fatalf("unexpected upload payload: %+v", created)
	}
	if created.OriginalFilename != "lamp.png" {
		t.Fatalf("expected original filename lamp.png, got %+v", created)
	}
	if created.MIMEType != "image/png" {
		t.Fatalf("expected mime type image/png, got %+v", created)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/reference-assets", nil, ownerCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if !bytes.Contains(listResp.Body.Bytes(), []byte(`"original_filename":"lamp.png"`)) {
		t.Fatalf("expected uploaded asset in list: %s", listResp.Body.String())
	}

	fileResp := performRequest(t, testApp, http.MethodGet, created.PreviewURL, nil, ownerCookies)
	if fileResp.Code != http.StatusOK {
		t.Fatalf("expected file 200, got %d: %s", fileResp.Code, fileResp.Body.String())
	}
	if got := fileResp.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected image/png, got %q", got)
	}

	otherFileResp := performRequest(t, testApp, http.MethodGet, created.PreviewURL, nil, viewerCookies)
	if otherFileResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user file 404, got %d", otherFileResp.Code)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/reference-assets/"+itoa(created.ID), nil, ownerCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	var deleted ReferenceAsset
	if err := testApp.db.First(&deleted, created.ID).Error; err == nil {
		t.Fatalf("expected reference asset deleted, got %+v", deleted)
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", owner.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
}

func TestReferenceAssetMultipartUploadRejectsFilesOver50MB(t *testing.T) {
	const maxReferenceAssetBytes = int64(50 * 1024 * 1024)

	testApp, _ := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "creator_refs_multipart_large", "test-password")

	resp := performMultipartRequest(t, testApp, http.MethodPost, "/api/reference-assets", "file", "large.png", oversizedPNGBytes(t, maxReferenceAssetBytes+1), cookies)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected multipart upload 413 for oversized file, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"reference_asset_too_large"`)) {
		t.Fatalf("expected reference_asset_too_large error, got %s", resp.Body.String())
	}
}

func TestReferenceAssetDisplayNameCanBeUpdatedClearedAndListed(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_ref_rename", "test-password")
	asset := seedReferenceAsset(t, testApp, user.ID, "original-reference.png", "image/png", mustBase64Decode(t, fakePNGBase64))

	updateResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/reference-assets/"+itoa(asset.ID), map[string]any{
		"display_name": "  商品参考图  ",
	}, cookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected display name update 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	var updated struct {
		ID               uint   `json:"id"`
		DisplayName      string `json:"display_name"`
		OriginalFilename string `json:"original_filename"`
	}
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode display name update payload: %v", err)
	}
	if updated.ID != asset.ID || updated.DisplayName != "商品参考图" || updated.OriginalFilename != "original-reference.png" {
		t.Fatalf("unexpected display name update payload: %+v", updated)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/reference-assets", nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected reference asset list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if !bytes.Contains(listResp.Body.Bytes(), []byte(`"display_name":"商品参考图"`)) {
		t.Fatalf("expected display name in list payload: %s", listResp.Body.String())
	}
	if !bytes.Contains(listResp.Body.Bytes(), []byte(`"original_filename":"original-reference.png"`)) {
		t.Fatalf("expected original filename to remain unchanged: %s", listResp.Body.String())
	}

	clearResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/reference-assets/"+itoa(asset.ID), map[string]any{
		"display_name": "   ",
	}, cookies)
	if clearResp.Code != http.StatusOK {
		t.Fatalf("expected display name clear 200, got %d: %s", clearResp.Code, clearResp.Body.String())
	}
	var cleared struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(clearResp.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decode display name clear payload: %v", err)
	}
	if cleared.DisplayName != "" {
		t.Fatalf("expected display name to be cleared, got %+v", cleared)
	}
}

func TestReferenceAssetDisplayNameValidationAndOwnership(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_ref_rename_owner", "test-password")
	_, otherCookies := createLoggedInUser(t, testApp, "creator_ref_rename_other", "test-password")
	asset := seedReferenceAsset(t, testApp, owner.ID, "owned-reference.png", "image/png", mustBase64Decode(t, fakePNGBase64))

	longResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/reference-assets/"+itoa(asset.ID), map[string]any{
		"display_name": strings.Repeat("名", 81),
	}, ownerCookies)
	if longResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected long display name 422, got %d: %s", longResp.Code, longResp.Body.String())
	}

	otherResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/reference-assets/"+itoa(asset.ID), map[string]any{
		"display_name": "其他用户的名称",
	}, otherCookies)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("expected other user display name update 404, got %d: %s", otherResp.Code, otherResp.Body.String())
	}
}

func TestDeleteUnreferencedReferenceAssetRemovesRowAndObject(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	store := &publicURLAssetStore{
		key:      "assets/reference-assets/11/2026/05/unreferenced.png",
		mimeType: "image/png",
		content:  mustBase64Decode(t, fakePNGBase64),
	}
	testApp.assetStore = store
	user, cookies := createLoggedInUser(t, testApp, "creator_unreferenced_delete", "test-password")
	asset := seedReferenceAsset(t, testApp, user.ID, "unreferenced.png", "image/png", mustBase64Decode(t, fakePNGBase64))

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/reference-assets/"+itoa(asset.ID), nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected unreferenced asset delete 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if store.deleteCalls != 1 {
		t.Fatalf("expected unreferenced asset object to be deleted once, delete calls=%d", store.deleteCalls)
	}

	var deleted ReferenceAsset
	if err := testApp.db.Unscoped().First(&deleted, asset.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected unreferenced reference asset row to be physically deleted, got asset=%+v err=%v", deleted, err)
	}
}

func TestReferenceAssetUploadPersistsStorePublicURLWhenAvailable(t *testing.T) {
	const publicURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/reference.png"

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/reference.png",
		mimeType:  "image/png",
		publicURL: publicURL,
	}
	_, cookies := createLoggedInUser(t, testApp, "creator_refs_oss", "test-password")

	resp := performMultipartRequest(t, testApp, http.MethodPost, "/api/reference-assets", "file", "reference.png", mustBase64Decode(t, fakePNGBase64), cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		ID         uint   `json:"id"`
		PreviewURL string `json:"preview_url"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode upload payload: %v", err)
	}
	if created.PreviewURL != publicURL {
		t.Fatalf("expected OSS public preview url, got %+v", created)
	}

	var asset ReferenceAsset
	if err := testApp.db.First(&asset, created.ID).Error; err != nil {
		t.Fatalf("load reference asset: %v", err)
	}
	if asset.PreviewURL != publicURL {
		t.Fatalf("expected OSS public preview url on asset, got %+v", asset)
	}
}

func TestReferenceAssetDirectUploadPolicyReturnsRestrictedOSSPostFields(t *testing.T) {
	const maxReferenceAssetBytes = int64(50 * 1024 * 1024)

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.StorageType = "oss"
	testApp.cfg.OSSEndpoint = "https://oss-cn-shenzhen.aliyuncs.com"
	testApp.cfg.OSSBucket = "example-assets"
	testApp.cfg.OSSPublicBaseURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com"
	testApp.cfg.OSSBasePath = "assets/"
	testApp.cfg.OSSAccessKeyID = "test-access-key"
	testApp.cfg.OSSAccessKeySecret = "test-access-secret"
	user, cookies := createLoggedInUser(t, testApp, "creator_refs_policy", "test-password")

	unauthorizedResp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/upload-policy", map[string]any{
		"filename":  "reference.png",
		"mime_type": "image/png",
		"size":      len(mustBase64Decode(t, fakePNGBase64)),
	}, nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected upload policy to require login, got %d: %s", unauthorizedResp.Code, unauthorizedResp.Body.String())
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/upload-policy", map[string]any{
		"filename":  "reference.png",
		"mime_type": "image/png",
		"size":      maxReferenceAssetBytes,
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected upload policy 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		UploadURL   string            `json:"upload_url"`
		ObjectKey   string            `json:"object_key"`
		PublicURL   string            `json:"public_url"`
		FormData    map[string]string `json:"form_data"`
		ExpiresAt   time.Time         `json:"expires_at"`
		UploadToken string            `json:"upload_token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload policy payload: %v", err)
	}
	expectedPrefix := fmt.Sprintf("assets/reference-assets/%d/", user.ID)
	if payload.UploadURL != "https://example-assets.oss-cn-shenzhen.aliyuncs.com" ||
		!strings.HasPrefix(payload.ObjectKey, expectedPrefix) ||
		!strings.HasSuffix(payload.ObjectKey, ".png") ||
		payload.PublicURL != "https://example-assets.oss-cn-shenzhen.aliyuncs.com/"+payload.ObjectKey ||
		payload.UploadToken == "" ||
		payload.ExpiresAt.IsZero() {
		t.Fatalf("unexpected upload policy payload: %+v", payload)
	}
	if payload.FormData["key"] != payload.ObjectKey ||
		payload.FormData["OSSAccessKeyId"] != "test-access-key" ||
		payload.FormData["Content-Type"] != "image/png" ||
		payload.FormData["success_action_status"] != "201" ||
		payload.FormData["x-oss-forbid-overwrite"] != "true" ||
		payload.FormData["policy"] == "" ||
		payload.FormData["Signature"] == "" {
		t.Fatalf("unexpected OSS form data: %+v", payload.FormData)
	}
	policyJSON, err := base64.StdEncoding.DecodeString(payload.FormData["policy"])
	if err != nil {
		t.Fatalf("decode OSS policy: %v", err)
	}
	var decodedPolicy struct {
		Conditions []any `json:"conditions"`
	}
	if err := json.Unmarshal(policyJSON, &decodedPolicy); err != nil {
		t.Fatalf("unmarshal OSS policy: %v", err)
	}
	var foundContentLengthLimit bool
	for _, condition := range decodedPolicy.Conditions {
		values, ok := condition.([]any)
		if !ok || len(values) != 3 || values[0] != "content-length-range" {
			continue
		}
		if values[1] != float64(1) || values[2] != float64(maxReferenceAssetBytes) {
			t.Fatalf("unexpected content-length-range condition: %#v", values)
		}
		foundContentLengthLimit = true
	}
	if !foundContentLengthLimit {
		t.Fatalf("expected OSS policy to include 50MB content-length-range, got %s", string(policyJSON))
	}
	for _, marker := range []string{payload.ObjectKey, "content-length-range", "image/png", "x-oss-forbid-overwrite"} {
		if !bytes.Contains(policyJSON, []byte(marker)) {
			t.Fatalf("expected OSS policy to contain %q, got %s", marker, string(policyJSON))
		}
	}

	tooLargeResp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/upload-policy", map[string]any{
		"filename":  "too-large.png",
		"mime_type": "image/png",
		"size":      maxReferenceAssetBytes + 1,
	}, cookies)
	if tooLargeResp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected upload policy 413 for oversized file, got %d: %s", tooLargeResp.Code, tooLargeResp.Body.String())
	}
	if !bytes.Contains(tooLargeResp.Body.Bytes(), []byte(`"code":"reference_asset_too_large"`)) {
		t.Fatalf("expected reference_asset_too_large error, got %s", tooLargeResp.Body.String())
	}
}

func TestReferenceAssetCompleteDirectUploadCreatesReferenceAsset(t *testing.T) {
	const publicBaseURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com"

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.StorageType = "oss"
	testApp.cfg.OSSEndpoint = "https://oss-cn-shenzhen.aliyuncs.com"
	testApp.cfg.OSSBucket = "example-assets"
	testApp.cfg.OSSPublicBaseURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com"
	testApp.cfg.OSSBasePath = "assets/"
	testApp.cfg.OSSAccessKeyID = "test-access-key"
	testApp.cfg.OSSAccessKeySecret = "test-access-secret"
	testApp.assetStore = &publicURLAssetStore{
		key:       "unused-by-complete-upload",
		mimeType:  "image/png",
		publicURL: publicBaseURL + "/",
		content:   mustBase64Decode(t, fakePNGBase64),
	}
	_, cookies := createLoggedInUser(t, testApp, "creator_refs_complete", "test-password")

	policyResp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/upload-policy", map[string]any{
		"filename":  "reference.png",
		"mime_type": "image/png",
		"size":      len(mustBase64Decode(t, fakePNGBase64)),
	}, cookies)
	if policyResp.Code != http.StatusOK {
		t.Fatalf("expected upload policy 200, got %d: %s", policyResp.Code, policyResp.Body.String())
	}
	var policy struct {
		ObjectKey   string `json:"object_key"`
		UploadToken string `json:"upload_token"`
	}
	if err := json.Unmarshal(policyResp.Body.Bytes(), &policy); err != nil {
		t.Fatalf("decode policy payload: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/complete-upload", map[string]any{
		"object_key":    policy.ObjectKey,
		"upload_token":  policy.UploadToken,
		"original_name": "ignored-client-name.png",
	}, cookies)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected complete upload 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var created ReferenceAsset
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode completed reference asset: %v", err)
	}
	if created.ID == 0 ||
		created.AssetKey != policy.ObjectKey ||
		created.MIMEType != "image/png" ||
		created.OriginalFilename != "reference.png" ||
		created.PreviewURL != "https://example-assets.oss-cn-shenzhen.aliyuncs.com/"+policy.ObjectKey {
		t.Fatalf("unexpected completed reference asset: %+v", created)
	}

	var persisted ReferenceAsset
	if err := testApp.db.First(&persisted, created.ID).Error; err != nil {
		t.Fatalf("load created reference asset: %v", err)
	}
	if persisted.AssetKey != policy.ObjectKey || persisted.PreviewURL != created.PreviewURL {
		t.Fatalf("unexpected persisted reference asset: %+v", persisted)
	}

	mismatchResp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/complete-upload", map[string]any{
		"object_key":   policy.ObjectKey + ".evil",
		"upload_token": policy.UploadToken,
	}, cookies)
	if mismatchResp.Code != http.StatusForbidden {
		t.Fatalf("expected mismatched object key 403, got %d: %s", mismatchResp.Code, mismatchResp.Body.String())
	}
}

func TestReferenceAssetCompleteDirectUploadRejectsObjectOver50MB(t *testing.T) {
	const maxReferenceAssetBytes = int64(50 * 1024 * 1024)
	const publicBaseURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com"

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.cfg.StorageType = "oss"
	testApp.cfg.OSSEndpoint = "https://oss-cn-shenzhen.aliyuncs.com"
	testApp.cfg.OSSBucket = "example-assets"
	testApp.cfg.OSSPublicBaseURL = publicBaseURL
	testApp.cfg.OSSBasePath = "assets/"
	testApp.cfg.OSSAccessKeyID = "test-access-key"
	testApp.cfg.OSSAccessKeySecret = "test-access-secret"
	testApp.assetStore = &publicURLAssetStore{
		key:       "unused-by-complete-upload",
		mimeType:  "image/png",
		publicURL: publicBaseURL + "/",
		content:   oversizedPNGBytes(t, maxReferenceAssetBytes+1),
	}
	_, cookies := createLoggedInUser(t, testApp, "creator_refs_complete_large", "test-password")

	policyResp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/upload-policy", map[string]any{
		"filename":  "reference.png",
		"mime_type": "image/png",
		"size":      maxReferenceAssetBytes,
	}, cookies)
	if policyResp.Code != http.StatusOK {
		t.Fatalf("expected upload policy 200, got %d: %s", policyResp.Code, policyResp.Body.String())
	}
	var policy struct {
		ObjectKey   string `json:"object_key"`
		UploadToken string `json:"upload_token"`
	}
	if err := json.Unmarshal(policyResp.Body.Bytes(), &policy); err != nil {
		t.Fatalf("decode policy payload: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/reference-assets/complete-upload", map[string]any{
		"object_key":   policy.ObjectKey,
		"upload_token": policy.UploadToken,
	}, cookies)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected complete upload 413 for oversized object, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"reference_asset_too_large"`)) {
		t.Fatalf("expected reference_asset_too_large error, got %s", resp.Body.String())
	}
}

func TestLegacyReferenceAssetURLsResolveToStorePublicURLWhenAvailable(t *testing.T) {
	const publicURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/reference-legacy.png"

	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/reference-legacy.png",
		mimeType:  "image/png",
		publicURL: publicURL,
	}
	user, cookies := createLoggedInUser(t, testApp, "creator_refs_legacy_oss", "test-password")
	asset := ReferenceAsset{
		UserID:           user.ID,
		AssetKey:         "assets/2026/05/reference-legacy.png",
		PreviewURL:       "/api/reference-assets/1/file",
		MIMEType:         "image/png",
		OriginalFilename: "legacy.png",
	}
	if err := testApp.db.Create(&asset).Error; err != nil {
		t.Fatalf("create legacy reference asset: %v", err)
	}
	asset.PreviewURL = fmt.Sprintf("/api/reference-assets/%d/file", asset.ID)
	if err := testApp.db.Save(&asset).Error; err != nil {
		t.Fatalf("save legacy reference asset url: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/reference-assets", nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected reference asset list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []ReferenceAsset `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode reference asset list: %v", err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].PreviewURL != publicURL {
		t.Fatalf("expected public url in reference asset list, got %+v", listPayload.Items)
	}

	fileResp := performRequest(t, testApp, http.MethodGet, asset.PreviewURL, nil, cookies)
	if fileResp.Code != http.StatusFound {
		t.Fatalf("expected legacy reference file route redirect, got %d: %s", fileResp.Code, fileResp.Body.String())
	}
	if location := fileResp.Header().Get("Location"); location != publicURL {
		t.Fatalf("expected redirect to %q, got %q", publicURL, location)
	}
}

func TestGenerationWithReferenceAssetsPersistsRelationsAndProviderInputs(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_refs",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_refs_gen", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	first := seedReferenceAsset(t, testApp, user.ID, "mood-board.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "product.jpg", "image/jpeg", []byte("second-reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "create a premium poster using both references",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{second.ID, first.ID},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected exactly one captured input, got %d", len(provider.inputs))
	}
	if len(provider.inputs[0].ReferenceImages) != 2 {
		t.Fatalf("expected two reference images, got %+v", provider.inputs[0])
	}
	if provider.inputs[0].ReferenceImages[0].MIMEType != "image/jpeg" {
		t.Fatalf("expected first reference to preserve request order, got %+v", provider.inputs[0].ReferenceImages)
	}
	if provider.inputs[0].ReferenceImages[1].MIMEType != "image/png" {
		t.Fatalf("expected second reference to preserve request order, got %+v", provider.inputs[0].ReferenceImages)
	}
	if provider.inputs[0].ReferenceImages[0].InputURL != "data:image/jpeg;base64,c2Vjb25kLXJlZmVyZW5jZQ==" {
		t.Fatalf("expected first reference input URL data URL, got %+v", provider.inputs[0].ReferenceImages[0])
	}

	var requestStartEvent GenerationEventLog
	if err := testApp.db.Where("generation_record_id = ? AND event = ?", payload.GenerationID, "provider_request_start").First(&requestStartEvent).Error; err != nil {
		t.Fatalf("load provider request start event: %v", err)
	}
	var requestStartMetadata map[string]any
	if err := json.Unmarshal([]byte(requestStartEvent.MetadataJSON), &requestStartMetadata); err != nil {
		t.Fatalf("decode provider request metadata: %v", err)
	}
	if requestStartMetadata["reference_transport_mode"] != "images_edits_multipart" {
		t.Fatalf("expected edits multipart transport metadata, got %+v", requestStartMetadata)
	}
	if requestStartMetadata["provider_image_payload_count"] != float64(2) {
		t.Fatalf("expected provider image payload count 2, got %+v", requestStartMetadata)
	}

	var links []GenerationReferenceAsset
	if err := testApp.db.Where("generation_record_id = ?", payload.GenerationID).Order("sort_order asc").Find(&links).Error; err != nil {
		t.Fatalf("load generation reference links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected two reference links, got %+v", links)
	}
	if links[0].ReferenceAssetID != second.ID || links[0].SortOrder != 0 {
		t.Fatalf("unexpected first link: %+v", links[0])
	}
	if links[1].ReferenceAssetID != first.ID || links[1].SortOrder != 1 {
		t.Fatalf("unexpected second link: %+v", links[1])
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?category=image&page_size=10", nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected works list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			WorkID            uint   `json:"work_id"`
			ReferenceAssetIDs []uint `json:"reference_asset_ids"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode works list: %v", err)
	}
	if len(listPayload.Items) != 1 {
		t.Fatalf("expected one work item, got %+v", listPayload.Items)
	}
	if !reflect.DeepEqual(listPayload.Items[0].ReferenceAssetIDs, []uint{second.ID, first.ID}) {
		t.Fatalf("expected works list to expose ordered reference asset ids for image-to-image display, got %+v", listPayload.Items[0].ReferenceAssetIDs)
	}
}

func TestAsyncImageGenerationPropagatesComposeReferenceIntent(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-compose-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_compose_refs",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_compose_refs", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	first := seedReferenceAsset(t, testApp, user.ID, "person-one.png", "image/png", []byte("person-one"))
	second := seedReferenceAsset(t, testApp, user.ID, "person-two-scene.jpg", "image/jpeg", []byte("person-two-scene"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":                     "把两个人合成到图2背景",
		"aspect_ratio":               "1:1",
		"reference_asset_ids":        []uint{first.ID, second.ID},
		"reference_intent":           "compose",
		"background_reference_index": 1,
		"variation_mode":             "balanced",
		"variation_prompt":           "改成广角并增加背景人群",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.ReferenceIntent != GenerationReferenceIntentCompose || input.BackgroundReferenceIndex == nil || *input.BackgroundReferenceIndex != 1 {
		t.Fatalf("expected compose reference intent and second image background, got %+v", input)
	}
	if len(input.ReferenceImages) != 2 {
		t.Fatalf("expected two provider reference images, got %+v", input.ReferenceImages)
	}

	var requestStartEvent GenerationEventLog
	if err := testApp.db.Where("generation_record_id = ? AND event = ?", payload.GenerationID, "provider_request_start").First(&requestStartEvent).Error; err != nil {
		t.Fatalf("load provider request start event: %v", err)
	}
	var requestStartMetadata map[string]any
	if err := json.Unmarshal([]byte(requestStartEvent.MetadataJSON), &requestStartMetadata); err != nil {
		t.Fatalf("decode provider request metadata: %v", err)
	}
	if requestStartMetadata["reference_transport_mode"] != "images_edits_multipart" {
		t.Fatalf("expected compose references to use edits multipart transport, got %+v", requestStartMetadata)
	}
	if requestStartMetadata["provider_image_payload_count"] != float64(2) {
		t.Fatalf("expected two provider image payloads, got %+v", requestStartMetadata)
	}
	if requestStartMetadata["reference_intent"] != GenerationReferenceIntentCompose {
		t.Fatalf("expected reference intent metadata, got %+v", requestStartMetadata)
	}
}

func TestAsyncImageGenerationComposePlannerUsesDeepSeekPlan(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-compose-plan-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_compose_plan",
		},
	}
	var deepSeekCalls atomic.Int32
	var capturedBody string
	var capturedMu sync.Mutex
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deepSeekCalls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read deepseek request: %v", err)
		}
		capturedMu.Lock()
		capturedBody = string(body)
		capturedMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"background_reference_index\":2,\"reference_roles\":[{\"reference_index\":1,\"use\":\"保留人物主体\"},{\"reference_index\":2,\"use\":\"作为室内背景\"}],\"final_prompt\":\"AI合成规划：以【图2】作为室内背景，保留【图1】人物主体。\"}"}}]}`))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, provider)
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	user, cookies := createLoggedInUser(t, testApp, "creator_compose_plan", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	first := seedReferenceAsset(t, testApp, user.ID, "person-one.png", "image/png", []byte("person-one"))
	second := seedReferenceAsset(t, testApp, user.ID, "room-scene.jpg", "image/jpeg", []byte("room-scene"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "把人物合成到合适的室内背景",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
		"reference_intent":    "compose",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if deepSeekCalls.Load() != 1 {
		t.Fatalf("expected compose planner to call deepseek once, got %d", deepSeekCalls.Load())
	}
	capturedMu.Lock()
	deepSeekRequestBody := capturedBody
	capturedMu.Unlock()
	if !strings.Contains(deepSeekRequestBody, "reference_count") || strings.Contains(deepSeekRequestBody, "person-one") || strings.Contains(deepSeekRequestBody, "room-scene") {
		t.Fatalf("expected text-only compose planning request with reference metadata, got %s", deepSeekRequestBody)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.BackgroundReferenceIndex == nil || *input.BackgroundReferenceIndex != 1 {
		t.Fatalf("expected deepseek plan to select second reference as background, got %+v", input.BackgroundReferenceIndex)
	}
	promptText := composeImagePrompt(input)
	for _, expected := range []string{"AI合成规划", "背景/场景严格取【图2】", "只保留参考图中已经出现的人物"} {
		if !strings.Contains(promptText, expected) {
			t.Fatalf("expected provider prompt to contain %q, got %q", expected, promptText)
		}
	}
}

func TestAsyncImageGenerationComposePlannerFallbackUsesPromptBackground(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-compose-fallback-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_compose_fallback",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_compose_prompt_bg", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	first := seedReferenceAsset(t, testApp, user.ID, "background.png", "image/png", []byte("background"))
	second := seedReferenceAsset(t, testApp, user.ID, "person.jpg", "image/jpeg", []byte("person"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "请用图1作为背景，把图2的人物自然合成进去",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
		"reference_intent":    "compose",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.BackgroundReferenceIndex == nil || *input.BackgroundReferenceIndex != 0 {
		t.Fatalf("expected prompt fallback to select first reference as background, got %+v", input.BackgroundReferenceIndex)
	}
	if promptText := composeImagePrompt(input); !strings.Contains(promptText, "背景/场景严格取【图1】") {
		t.Fatalf("expected provider prompt to use first reference as background, got %q", promptText)
	}
}

func TestAsyncImageGenerationComposePlannerInvalidJSONFallsBackWithoutBlocking(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-compose-invalid-plan-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_compose_invalid_plan",
		},
	}
	var deepSeekCalls atomic.Int32
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deepSeekCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"不是 JSON"}}]}`))
	}))
	defer deepSeekServer.Close()

	testApp, _ := newTestApp(t, provider)
	testApp.cfg.DeepSeekAPIKey = "deepseek-key"
	testApp.cfg.DeepSeekBaseURL = deepSeekServer.URL
	testApp.cfg.DeepSeekPromptModel = "deepseek-v4"
	user, cookies := createLoggedInUser(t, testApp, "creator_compose_invalid_plan", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	first := seedReferenceAsset(t, testApp, user.ID, "person.png", "image/png", []byte("person"))
	second := seedReferenceAsset(t, testApp, user.ID, "scene.jpg", "image/jpeg", []byte("scene"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "把两张参考图自然合成",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{first.ID, second.ID},
		"reference_intent":    "compose",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if deepSeekCalls.Load() != 1 {
		t.Fatalf("expected compose planner to attempt deepseek once, got %d", deepSeekCalls.Load())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected provider to still be called once, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.BackgroundReferenceIndex == nil || *input.BackgroundReferenceIndex != 1 {
		t.Fatalf("expected invalid planner response to fall back to last reference background, got %+v", input.BackgroundReferenceIndex)
	}
}

func TestGenerationWithReferenceWorksPassesProviderInputs(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_work_refs",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_work_refs_gen", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	asset := seedReferenceAsset(t, testApp, user.ID, "mood-board.png", "image/png", []byte("asset-reference"))
	work := seedSucceededWork(t, testApp, user.ID, "existing source image", "1:1")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "create with uploaded and work references",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{asset.ID},
		"reference_work_ids":  []uint{work.ID},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected exactly one captured input, got %d", len(provider.inputs))
	}
	if len(provider.inputs[0].ReferenceImages) != 2 {
		t.Fatalf("expected uploaded asset plus work reference, got %+v", provider.inputs[0].ReferenceImages)
	}
	if provider.inputs[0].ReferenceImages[0].MIMEType != "image/png" || provider.inputs[0].ReferenceImages[1].MIMEType != "image/png" {
		t.Fatalf("expected image references, got %+v", provider.inputs[0].ReferenceImages)
	}
}

func TestGenerationReferenceAssetsPassPublicInputURLToProvider(t *testing.T) {
	const publicURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/reference.png"

	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("fake-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_ref_public_url",
		},
	}
	testApp, _ := newTestApp(t, provider)
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/reference.png",
		mimeType:  "image/png",
		publicURL: publicURL,
	}
	user, cookies := createLoggedInUser(t, testApp, "creator_refs_public_url_gen", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	asset := seedReferenceAsset(t, testApp, user.ID, "mood-board.png", "image/png", []byte("asset-reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "create with public reference",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{asset.ID},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})

	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}
	if len(provider.inputs) != 1 || len(provider.inputs[0].ReferenceImages) != 1 {
		t.Fatalf("expected one provider reference image, got %+v", provider.inputs)
	}
	if provider.inputs[0].ReferenceImages[0].InputURL != publicURL {
		t.Fatalf("expected public reference input URL, got %+v", provider.inputs[0].ReferenceImages[0])
	}
}

func TestAsyncGenerationPreservesSpecificExecutionFailure(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       "not-base64",
			MIMEType:          "image/png",
			ProviderRequestID: "req_bad_asset",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_async_specific_failure", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "provider returns undecodable asset",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	var payload struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	waitFor(t, time.Second, func() bool {
		getResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/images/generations/%d", created.GenerationID), nil, cookies)
		if getResp.Code != http.StatusOK {
			return false
		}
		if err := json.Unmarshal(getResp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode generation payload: %v", err)
		}
		return payload.Status == GenerationStatusFailed
	})

	if payload.Error.Code != "asset_store_failed" || payload.Error.Message != "作品保存失败" {
		t.Fatalf("expected specific asset store failure, got %+v", payload.Error)
	}
}

func TestAsyncGenerationFailoversProviderTimeouts(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{Code: "provider_timeout", Message: "context deadline exceeded", FailureStage: providerFailureStageImageGenerationRequest},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("would-be-duplicate-image")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_should_not_be_used",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_no_timeout_resubmit", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "single submit timeout",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	var payload struct {
		Status            string `json:"status"`
		AvailableCredits  int    `json:"available_credits"`
		ProviderRequestID string `json:"provider_request_id"`
	}
	waitFor(t, 2*time.Second, func() bool {
		getResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/images/generations/%d", created.GenerationID), nil, cookies)
		if getResp.Code != http.StatusOK {
			return false
		}
		if err := json.Unmarshal(getResp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode generation payload: %v", err)
		}
		return payload.Status == GenerationStatusSucceeded
	})

	if provider.calls != 2 {
		t.Fatalf("expected provider to fail over after timeout, got %d calls", provider.calls)
	}
	if payload.ProviderRequestID != "req_should_not_be_used" || payload.AvailableCredits != 1 {
		t.Fatalf("expected successful failover payload, got %+v", payload)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ProviderAttemptCount != 2 || record.Status != GenerationStatusSucceeded {
		t.Fatalf("expected second provider attempt to succeed, got %+v", record)
	}
}

func TestAsyncGenerationFailoversProviderDecodeFailures(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{Code: "provider_decode_failed", Message: "provider returned empty response body", ProviderRequestID: "req_decode_1", FailureStage: providerFailureStageImageGenerationRequest},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("decode-resubmit-would-duplicate")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_decode_should_not_be_used",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_no_decode_resubmit", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "single submit decode failure",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	var payload struct {
		Status            string `json:"status"`
		ProviderRequestID string `json:"provider_request_id"`
	}
	waitFor(t, 2*time.Second, func() bool {
		getResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/images/generations/%d", created.GenerationID), nil, cookies)
		if getResp.Code != http.StatusOK {
			return false
		}
		if err := json.Unmarshal(getResp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode generation payload: %v", err)
		}
		return payload.Status == GenerationStatusSucceeded
	})

	if provider.calls != 2 {
		t.Fatalf("expected provider to fail over after decode failure, got %d calls", provider.calls)
	}
	if payload.ProviderRequestID != "req_decode_should_not_be_used" {
		t.Fatalf("expected successful failover provider request id, got %q", payload.ProviderRequestID)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ProviderAttemptCount != 2 || record.Status != GenerationStatusSucceeded {
		t.Fatalf("expected second provider attempt to succeed, got %+v", record)
	}
}

func TestAsyncGenerationStoresProviderDiagnosticsWithoutExposingToUserPayload(t *testing.T) {
	provider := &stubProvider{
		err: &ProviderError{
			HTTPStatus:        http.StatusBadGateway,
			Code:              "provider_http_502",
			Message:           "upstream gateway failed",
			ProviderRequestID: "req_gateway_502",
			FailureStage:      providerFailureStageImageGenerationRequest,
		},
	}
	testApp, _ := newTestApp(t, provider)
	disableSeedImageRoutes(t, testApp)
	route := seedImageRoutingModel(t, testApp, "diagnostic single route", "https://diagnostic-route.example", 101)
	saveDefaultImageRoutes(t, testApp, route.ID, route.ID)
	user, cookies := createLoggedInUser(t, testApp, "creator_provider_diagnostics", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "provider gateway diagnostics",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	var payload struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	waitFor(t, 2*time.Second, func() bool {
		getResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/images/generations/%d", created.GenerationID), nil, cookies)
		if getResp.Code != http.StatusOK {
			return false
		}
		if bytes.Contains(getResp.Body.Bytes(), []byte("provider_http_status")) ||
			bytes.Contains(getResp.Body.Bytes(), []byte("provider_error_message")) {
			t.Fatalf("user generation payload must not expose raw provider diagnostics: %s", getResp.Body.String())
		}
		if err := json.Unmarshal(getResp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode generation payload: %v", err)
		}
		return payload.Status == GenerationStatusFailed
	})

	if payload.Error.Code != "provider_unavailable" || payload.Error.Message != "upstream gateway failed" {
		t.Fatalf("expected raw unavailable provider payload, got %+v", payload.Error)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ProviderHTTPStatus != http.StatusBadGateway ||
		record.ProviderErrorCode != "provider_http_502" ||
		record.ProviderErrorMessage != "upstream gateway failed" ||
		record.ProviderFailureStage != providerFailureStageImageGenerationRequest ||
		record.ProviderAttemptCount != 2 ||
		record.ProviderRequestID != "req_gateway_502" {
		t.Fatalf("expected raw provider diagnostics on record, got %+v", record)
	}

	var events []GenerationEventLog
	if err := testApp.db.Where("generation_record_id = ?", created.GenerationID).Order("created_at asc, id asc").Find(&events).Error; err != nil {
		t.Fatalf("load generation event logs: %v", err)
	}
	eventNames := make([]string, 0, len(events))
	for _, event := range events {
		eventNames = append(eventNames, event.Event)
		if strings.Contains(event.MetadataJSON, "Bearer ") || strings.Contains(event.MetadataJSON, "test-key") || strings.Contains(event.MetadataJSON, "base64") {
			t.Fatalf("expected metadata to be sanitized, got event %+v", event)
		}
	}
	for _, want := range []string{"task_created", "provider_request_start", "provider_request_failed", "generation_failed"} {
		if !containsEventName(eventNames, want) {
			t.Fatalf("expected generation event %q in %v", want, eventNames)
		}
	}
}

func TestPublicProviderFailureUsesRawProviderAssetFetchMessage(t *testing.T) {
	code, message, retryable := publicProviderFailure(&ProviderError{
		HTTPStatus:   http.StatusBadGateway,
		Code:         "provider_asset_http_502",
		Message:      "asset gateway failed",
		FailureStage: providerFailureStageProviderAssetFetch,
	})
	if code != "provider_asset_fetch_failed" ||
		message != "asset gateway failed" ||
		!retryable {
		t.Fatalf("unexpected public asset fetch failure: code=%q message=%q retryable=%v", code, message, retryable)
	}
}

func containsEventName(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}

func TestPublicProviderFailureClassifiesPolicyRejections(t *testing.T) {
	const providerMessage = "提交中含有违反平台政策的内容，请你立即停止或调整你的提交内容（traceid: trace_policy_1）"
	code, message, retryable := publicProviderFailure(&ProviderError{
		HTTPStatus:   http.StatusInternalServerError,
		Code:         "<nil>",
		Message:      providerMessage,
		FailureStage: providerFailureStageImageGenerationRequest,
	})
	if code != "provider_policy_rejected" ||
		message != providerMessage ||
		retryable {
		t.Fatalf("unexpected policy rejection mapping: code=%q message=%q retryable=%v", code, message, retryable)
	}
}

func TestPublicProviderFailureHidesTechnicalTransportMessages(t *testing.T) {
	code, message, retryable := publicProviderFailure(&ProviderError{
		Code:         "provider_timeout",
		Message:      `Post "https://bailinai.net/v1/images/edits": context deadline exceeded`,
		FailureStage: providerFailureStageImageGenerationRequest,
	})
	if code != "provider_timeout" ||
		message != "网络超时，生成失败，请点击重试。" ||
		!retryable {
		t.Fatalf("unexpected timeout mapping: code=%q message=%q retryable=%v", code, message, retryable)
	}
	if strings.Contains(message, "https://") ||
		strings.Contains(message, "context deadline") ||
		strings.Contains(message, "Post ") {
		t.Fatalf("public message leaked transport details: %q", message)
	}
}

func TestAsyncGenerationDoesNotAmplifyProviderHTTPFailures(t *testing.T) {
	provider := &stubProvider{
		err: &ProviderError{HTTPStatus: http.StatusBadGateway, Code: "provider_http_502", Message: "bad gateway", FailureStage: providerFailureStageImageGenerationRequest},
	}
	testApp, _ := newTestApp(t, provider)
	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "http failure route a", "https://http-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "http failure route b", "https://http-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)
	user, cookies := createLoggedInUser(t, testApp, "creator_no_http_amplify", "test-password")
	setUserCredits(t, testApp, user.ID, 4)

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "first provider http failure",
		"aspect_ratio": "1:1",
	}, cookies)
	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "second provider http failure",
		"aspect_ratio": "1:1",
	}, cookies)
	if firstResp.Code != http.StatusAccepted || secondResp.Code != http.StatusAccepted {
		t.Fatalf("expected both tasks accepted, got first=%d %s second=%d %s", firstResp.Code, firstResp.Body.String(), secondResp.Code, secondResp.Body.String())
	}

	waitFor(t, 2*time.Second, func() bool {
		var failed int64
		if err := testApp.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusFailed).Count(&failed).Error; err != nil {
			return false
		}
		return failed == 2
	})

	if provider.calls != 8 {
		t.Fatalf("expected each task to try two model candidates with one same-channel retry, got %d calls", provider.calls)
	}
	var records []GenerationRecord
	if err := testApp.db.Where("status = ?", GenerationStatusFailed).Find(&records).Error; err != nil {
		t.Fatalf("load failed records: %v", err)
	}
	for _, record := range records {
		if record.ProviderAttemptCount != 4 {
			t.Fatalf("expected four provider attempts per failed task, got %+v", record)
		}
	}
}

func TestAsyncGenerationMixedFailuresDoNotBlockOtherTasks(t *testing.T) {
	provider := &promptRoutingImageProvider{}
	testApp, _ := newTestApp(t, provider)
	settings, err := testApp.loadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	candidates, err := testApp.modelConfigCandidatesForGeneration(settings)
	if err != nil {
		t.Fatalf("load image candidates: %v", err)
	}
	user, cookies := createLoggedInUser(t, testApp, "creator_mixed_failure_isolation", "test-password")
	setUserCredits(t, testApp, user.ID, 8)

	prompts := []string{"success one", "fail one", "success two", "fail two"}
	for _, prompt := range prompts {
		resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
			"prompt":       prompt,
			"aspect_ratio": "1:1",
		}, cookies)
		if resp.Code != http.StatusAccepted {
			t.Fatalf("expected %q accepted, got %d: %s", prompt, resp.Code, resp.Body.String())
		}
	}

	waitFor(t, 2*time.Second, func() bool {
		var terminal int64
		if err := testApp.db.Model(&GenerationRecord{}).
			Where("status IN ?", []string{GenerationStatusSucceeded, GenerationStatusFailed}).
			Count(&terminal).Error; err != nil {
			return false
		}
		return terminal == int64(len(prompts))
	})

	expectedCalls := 2 + 4*len(candidates)
	if provider.callCount() != expectedCalls {
		t.Fatalf("expected failover calls for failed tasks, got %d want %d", provider.callCount(), expectedCalls)
	}
	var succeeded, failed int64
	if err := testApp.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusSucceeded).Count(&succeeded).Error; err != nil {
		t.Fatalf("count succeeded records: %v", err)
	}
	if err := testApp.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusFailed).Count(&failed).Error; err != nil {
		t.Fatalf("count failed records: %v", err)
	}
	if succeeded != 2 || failed != 2 {
		t.Fatalf("expected 2 succeeded and 2 failed isolated tasks, got succeeded=%d failed=%d", succeeded, failed)
	}
}

func TestAsyncGenerationRecoversProviderPanicAndContinuesProcessing(t *testing.T) {
	provider := &panicOnceImageProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_panic_isolation", "test-password")
	setUserCredits(t, testApp, user.ID, 4)

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "panic first task",
		"aspect_ratio": "1:1",
	}, cookies)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("expected first task 202, got %d: %s", firstResp.Code, firstResp.Body.String())
	}
	var firstCreated struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(firstResp.Body.Bytes(), &firstCreated); err != nil {
		t.Fatalf("decode first generation: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, firstCreated.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusFailed
	})
	var failedRecord GenerationRecord
	if err := testApp.db.First(&failedRecord, firstCreated.GenerationID).Error; err != nil {
		t.Fatalf("load failed panic record: %v", err)
	}
	if failedRecord.ErrorCode != "generation_task_panic" || failedRecord.ErrorMessage != "生成任务异常中断，请重新生成" {
		t.Fatalf("expected panic to be isolated on current record, got %+v", failedRecord)
	}

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "second task still runs",
		"aspect_ratio": "1:1",
	}, cookies)
	if secondResp.Code != http.StatusAccepted {
		t.Fatalf("expected second task 202 after panic, got %d: %s", secondResp.Code, secondResp.Body.String())
	}
	var secondCreated struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(secondResp.Body.Bytes(), &secondCreated); err != nil {
		t.Fatalf("decode second generation: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, secondCreated.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusSucceeded
	})
	if provider.callCount() != 2 {
		t.Fatalf("expected provider called for both isolated tasks, got %d", provider.callCount())
	}
}

func TestAsyncGenerationDoesNotRetryProviderParameterErrors(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{HTTPStatus: http.StatusBadRequest, Code: "invalid_request", Message: "参数无效"},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_no_retry", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "do not retry bad request",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		var record GenerationRecord
		if err := testApp.db.First(&record, created.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusFailed
	})

	if provider.calls != 1 {
		t.Fatalf("expected non-retryable provider error called once, got %d", provider.calls)
	}
}

func TestAsyncVideoGenerationCreatesVideoWork(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "sora-task-123"},
		pollResults: []VideoTaskResult{
			{TaskID: "sora-task-123", Status: VideoTaskSucceeded, Progress: "100%", OutputBase64: base64.StdEncoding.EncodeToString([]byte("video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-user", "secret123")
	setUserCredits(t, testApp, user.ID, 10)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "生成一个产品发布会短视频",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "sora-2",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted video generation, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}
	if created.GenerationID == 0 {
		t.Fatalf("expected generation id")
	}

	var payload struct {
		Status           string `json:"status"`
		WorkID           uint   `json:"work_id"`
		DownloadURL      string `json:"download_url"`
		MIMEType         string `json:"mime_type"`
		AvailableCredits int    `json:"available_credits"`
	}
	waitFor(t, time.Second, func() bool {
		getResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/videos/generations/%d", created.GenerationID), nil, cookies)
		if getResp.Code != http.StatusOK {
			return false
		}
		if err := json.Unmarshal(getResp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode generation payload: %v", err)
		}
		return payload.Status == GenerationStatusSucceeded
	})

	if payload.WorkID == 0 || payload.DownloadURL == "" || payload.MIMEType != "video/mp4" || payload.AvailableCredits != 5 {
		t.Fatalf("unexpected video generation payload: %+v", payload)
	}
	if len(videoProvider.submitInputs) != 1 || videoProvider.submitInputs[0].Prompt != "生成一个产品发布会短视频" || videoProvider.submitInputs[0].AspectRatio != "16:9" {
		t.Fatalf("unexpected video provider input: %+v", videoProvider.submitInputs)
	}

	var work Work
	if err := db.First(&work, payload.WorkID).Error; err != nil {
		t.Fatalf("load video work: %v", err)
	}
	if work.Category != WorkCategoryVideo || work.MIMEType != "video/mp4" {
		t.Fatalf("expected video work, got %+v", work)
	}
}

func TestAsyncVideoGenerationUsesDefaultWuyinModelConfig(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "grok-task-1"},
		pollResults: []VideoTaskResult{
			{TaskID: "grok-task-1", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("grok-video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-default", "secret123")
	setUserCredits(t, testApp, user.ID, 20)
	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "default-wuyin-key")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "default grok video",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Wuyin video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.Model != wuyinGrokImagineRuntimeModel || input.Duration != "6" || input.AspectRatio != "9:16" {
		t.Fatalf("expected default Wuyin video parameters, got %+v", input)
	}
	if input.ProviderBaseURL != wuyinGrokImagineProviderBaseURL || input.ProviderAPIEndpoint != wuyinGrokImagineSubmitEndpoint || input.ProviderAPIKey != "default-wuyin-key" {
		t.Fatalf("expected Wuyin model center key, got %+v", input)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)
}

func TestAsyncVideoGenerationRejectsWuyinWithoutReferenceImageBeforeProvider(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "grok-task-no-reference"}}
	testApp.videoProvider = videoProvider

	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "no-reference-wuyin-key")

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-no-reference", "secret123")
	setUserCredits(t, testApp, user.ID, 10)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "text only grok video should be blocked",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected Wuyin reference image 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "reference_image_required") || !strings.Contains(resp.Body.String(), "当前模型需要至少 1 张参考图") {
		t.Fatalf("expected reference_image_required response, got %s", resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("expected missing reference image to stop before provider call, got %+v", videoProvider.submitInputs)
	}
	var count int64
	if err := db.Model(&GenerationRecord{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count generation records: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no generation record for blocked Wuyin request, got %d", count)
	}
}

func TestAsyncVideoGenerationDefaultsWuyinDurationToThreeSeconds(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "grok-task-default-duration"},
		pollResults: []VideoTaskResult{
			{TaskID: "grok-task-default-duration", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("grok-video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "default-duration-wuyin-key")

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-default-duration", "secret123")
	setUserCredits(t, testApp, user.ID, 10)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "default three second grok video",
		"aspect_ratio": "9:16",
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Wuyin video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.Model != wuyinGrokImagineRuntimeModel || input.Duration != "3" || input.AspectRatio != "9:16" {
		t.Fatalf("expected default Wuyin duration to be 3 seconds, got %+v", input)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)
}

func TestAsyncVideoGenerationAcceptsWuyinShortDurations(t *testing.T) {
	for _, duration := range []string{"1", "3"} {
		t.Run(duration, func(t *testing.T) {
			testApp, db := newTestApp(t, &stubProvider{})
			videoProvider := &stubVideoProvider{
				submitResult: VideoSubmitResult{TaskID: "grok-task-" + duration},
				pollResults: []VideoTaskResult{
					{TaskID: "grok-task-" + duration, Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("grok-video-bytes")), MIMEType: "video/mp4"},
				},
			}
			testApp.videoProvider = videoProvider

			var grok ModelConfig
			if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
				t.Fatalf("load Grok model config: %v", err)
			}
			setWuyinModelCenterProviderKey(t, testApp, grok.ID, "short-duration-wuyin-key")

			user, cookies := createLoggedInUser(t, testApp, "video-wuyin-short-"+duration, "secret123")
			setUserCredits(t, testApp, user.ID, 10)

			resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
				"prompt":       "short grok video " + duration,
				"aspect_ratio": "16:9",
				"duration":     duration,
				"model":        wuyinGrokImagineRuntimeModel,
				"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
			}, cookies)
			if resp.Code != http.StatusAccepted {
				t.Fatalf("expected accepted Wuyin video generation for duration %s, got %d: %s", duration, resp.Code, resp.Body.String())
			}
			waitForCondition(t, time.Second, func() bool {
				return len(videoProvider.submitInputs) == 1
			})
			input := videoProvider.submitInputs[0]
			if input.Model != wuyinGrokImagineRuntimeModel || input.Duration != duration {
				t.Fatalf("expected Wuyin duration %s to reach provider, got %+v", duration, input)
			}
			waitForVideoGenerationTasksToSettle(t, testApp)
		})
	}
}

func TestAsyncVideoGenerationUsesWuyinModelCenterProviderKey(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "grok-task-model-center-key"},
		pollResults: []VideoTaskResult{
			{TaskID: "grok-task-model-center-key", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("grok-video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	if strings.TrimSpace(grok.APIKey) != "" {
		t.Fatalf("test expects legacy Grok key to be empty")
	}
	channel := setWuyinModelCenterProviderKey(t, testApp, grok.ID, "model-center-wuyin-key")

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-model-center-key", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "model center grok video",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Wuyin video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.ProviderAPIKey != "model-center-wuyin-key" {
		t.Fatalf("expected Wuyin provider key from model center, got %+v", input)
	}

	var record GenerationRecord
	if err := db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ModelID != channel.ModelID || record.ChannelID != channel.ID || record.RuntimeModel != wuyinGrokImagineRuntimeModel {
		t.Fatalf("expected generation record model center fields, got %+v", record)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)
}

func TestAsyncVideoGenerationSkipsWuyinModelCenterProviderWithoutKey(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "grok-task-selected-key"},
		pollResults: []VideoTaskResult{
			{TaskID: "grok-task-selected-key", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("grok-video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	emptyChannel := setWuyinModelCenterProviderKey(t, testApp, grok.ID, "")
	keyedProvider := ModelProvider{
		Name:                  "Wuyin keyed account",
		Provider:              "Wuyin",
		BaseURL:               wuyinGrokImagineProviderBaseURL,
		APIKey:                "selected-wuyin-key",
		DefaultTimeoutSeconds: defaultRequestTimeoutSeconds,
		Status:                ModelCenterStatusOnline,
	}
	if err := db.Create(&keyedProvider).Error; err != nil {
		t.Fatalf("create keyed Wuyin provider: %v", err)
	}
	keyedChannel := ModelChannel{
		ModelID:             emptyChannel.ModelID,
		ProviderID:          keyedProvider.ID,
		Name:                "Grok Imagine keyed channel",
		RuntimeModel:        wuyinGrokImagineRuntimeModel,
		Endpoint:            wuyinGrokImagineSubmitEndpoint,
		Weight:              50,
		Priority:            emptyChannel.Priority + 1,
		Status:              ModelCenterStatusOnline,
		HealthStatus:        ModelChannelHealthHealthy,
		LegacyModelConfigID: grok.ID,
	}
	if err := db.Create(&keyedChannel).Error; err != nil {
		t.Fatalf("create keyed Wuyin channel: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-select-keyed", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "select keyed grok video",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Wuyin video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.ProviderAPIKey != "selected-wuyin-key" {
		t.Fatalf("expected keyed Wuyin model center provider, got %+v", input)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)
}

func TestAsyncVideoGenerationUsesKeyedWuyinProviderAccountWhenBoundChannelKeyMissing(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "grok-task-keyed-provider"}}
	testApp.videoProvider = videoProvider

	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "")
	keyedProvider := ModelProvider{
		Name:                  "Wuyin",
		Provider:              "wuyin",
		BaseURL:               wuyinGrokImagineProviderBaseURL,
		APIKey:                "provider-account-key",
		DefaultTimeoutSeconds: defaultRequestTimeoutSeconds,
		Status:                ModelCenterStatusOnline,
	}
	if err := db.Create(&keyedProvider).Error; err != nil {
		t.Fatalf("create keyed Wuyin provider account: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-provider-account", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "provider account grok video",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Wuyin video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.ProviderAPIKey != "provider-account-key" {
		t.Fatalf("expected keyed Wuyin provider account, got %+v", input)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)
}

func TestAsyncVideoGenerationRejectsWuyinWhenProviderKeyMissing(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "grok-task-no-key"}}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-key-missing", "secret123")
	setUserCredits(t, testApp, user.ID, 10)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "missing key grok video",
		"aspect_ratio": "9:16",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected missing Wuyin key 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "provider_key_missing") {
		t.Fatalf("expected provider_key_missing response, got %s", resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("expected missing Wuyin key to stop before provider call, got %+v", videoProvider.submitInputs)
	}
}

func TestAsyncVideoGenerationRejectsWuyinUnsupportedDuration(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "grok-task-1"}}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-duration", "secret123")
	setUserCredits(t, testApp, user.ID, 10)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "unsupported duration",
		"aspect_ratio": "16:9",
		"duration":     "25",
		"model":        wuyinGrokImagineRuntimeModel,
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected Wuyin unsupported duration 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "invalid_video_duration") {
		t.Fatalf("expected invalid_video_duration response, got %s", resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("expected invalid Wuyin duration to stop before provider call, got %+v", videoProvider.submitInputs)
	}
}

func TestAsyncVideoGenerationLocalizesWuyinReferenceProviderError(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	providerMessage := "This model requires an input image. Text-to-video is not supported for this model."
	expectedMessage := "当前模型需要至少 1 张参考图，暂不支持纯文字生成视频。请先上传参考图后再提交。"
	videoProvider := &stubVideoProvider{
		submitErr: &ProviderError{
			HTTPStatus:        http.StatusBadRequest,
			Code:              "invalid_request",
			Message:           providerMessage,
			ProviderRequestID: "req-grok-reference-required",
			FailureStage:      providerFailureStageVideoSubmitRequest,
		},
	}
	testApp.videoProvider = videoProvider

	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "localized-provider-error-key")

	user, cookies := createLoggedInUser(t, testApp, "video-wuyin-provider-reference-error", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "provider still rejects this referenced grok video",
		"aspect_ratio": "16:9",
		"duration":     "6",
		"model":        wuyinGrokImagineRuntimeModel,
		"images":       []string{"data:image/png;base64,cmVmZXJlbmNl"},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Wuyin task before provider failure, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		var record GenerationRecord
		if err := db.First(&record, created.GenerationID).Error; err != nil {
			return false
		}
		if record.Status != GenerationStatusFailed {
			return false
		}
		var videoRecord VideoGenerationRecord
		if err := db.Where("generation_record_id = ?", created.GenerationID).First(&videoRecord).Error; err != nil {
			return false
		}
		return videoRecord.Status == GenerationStatusFailed &&
			videoRecord.ErrorMessage == expectedMessage &&
			videoRecord.ProviderErrorMessage == providerMessage
	})

	var record GenerationRecord
	if err := db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ErrorMessage != expectedMessage {
		t.Fatalf("expected localized user error message, got %q", record.ErrorMessage)
	}
	if strings.Contains(strings.ToLower(record.ErrorMessage), "requires an input image") ||
		strings.Contains(strings.ToLower(record.ErrorMessage), "text-to-video is not supported") {
		t.Fatalf("expected user-visible error to hide provider English, got %q", record.ErrorMessage)
	}
	if record.ProviderErrorMessage != providerMessage {
		t.Fatalf("expected raw provider error retained for diagnostics, got %q", record.ProviderErrorMessage)
	}

	var videoRecord VideoGenerationRecord
	if err := db.Where("generation_record_id = ?", created.GenerationID).First(&videoRecord).Error; err != nil {
		t.Fatalf("load video audit record: %v", err)
	}
	if videoRecord.ErrorMessage != expectedMessage || videoRecord.ProviderErrorMessage != providerMessage {
		t.Fatalf("expected localized audit error and raw provider diagnostic, got %+v", videoRecord)
	}
}

func TestAsyncVideoGenerationRejectsUnavailableDoubaoModelBeforeProvider(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")

	testApp, _ := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "ark-task-1"}}
	testApp.videoProvider = videoProvider

	if err := testApp.db.Model(&ModelConfig{}).
		Where("runtime_model = ?", "doubao-seed-2-0-mini-260428").
		Update("permission", ModelConfigPermissionPublic).Error; err != nil {
		t.Fatalf("publish Doubao without key: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-doubao-unavailable", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "seedance unavailable dry run",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "doubao-seed-2-0-mini-260428",
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected unavailable Doubao 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "provider_key_missing") || !strings.Contains(resp.Body.String(), "ARK_API_KEY") {
		t.Fatalf("expected provider_key_missing response, got %s", resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("expected unavailable Doubao to stop before provider call, got %+v", videoProvider.submitInputs)
	}
}

func TestAsyncVideoGenerationCanonicalizesDoubaoMiniAlias(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "ark-task-1"},
		pollResults: []VideoTaskResult{
			{TaskID: "ark-task-1", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("ark-video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var doubao ModelConfig
	if err := db.Where("runtime_model = ?", arkSeedanceMiniRuntimeModel).First(&doubao).Error; err != nil {
		t.Fatalf("load Doubao model: %v", err)
	}
	if err := db.Model(&doubao).Updates(map[string]any{
		"permission":                 ModelConfigPermissionPublic,
		"api_key":                    "ark-test-key",
		"video_readiness_status":     "passed",
		"video_readiness_reason":     "",
		"video_readiness_checked_at": time.Now(),
	}).Error; err != nil {
		t.Fatalf("publish Doubao model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-doubao-alias", "secret123")
	setUserCredits(t, testApp, user.ID, 200)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "seedance alias dry run",
		"aspect_ratio": "21:9",
		"duration":     "12",
		"model":        "doubao-seed-2-0-mini",
		"hd":           true,
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Doubao alias generation, got %d: %s", resp.Code, resp.Body.String())
	}

	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.Model != arkSeedanceMiniRuntimeModel ||
		input.Resolution != "720p" ||
		input.ProviderBaseURL != arkVideoProviderBaseURL ||
		input.ProviderAPIEndpoint != arkVideoTasksEndpoint ||
		input.ProviderAPIKey != "ark-test-key" {
		t.Fatalf("expected canonical Doubao provider input, got %+v", input)
	}

	var record GenerationRecord
	if err := db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.RuntimeModel != arkSeedanceMiniRuntimeModel || record.Model != arkSeedanceMiniRuntimeModel || record.ModelConfigID != doubao.ID {
		t.Fatalf("expected canonical Doubao generation record, got %+v", record)
	}
}

func TestAsyncVideoGenerationRejectsInsufficientCreditsWithRequiredVideoCost(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "ark-expensive-task"}}
	testApp.videoProvider = videoProvider
	now := time.Now()
	if err := db.Create(&ModelConfig{
		Name:                    "Doubao Seedance 2.0",
		Type:                    ModelConfigTypeVideo,
		Provider:                arkVideoProviderName,
		Status:                  ModelConfigStatusOnline,
		Priority:                1,
		CostLabel:               "30-50 点/秒",
		Permission:              ModelConfigPermissionPublic,
		RuntimeModel:            arkSeedance2RuntimeModel,
		APIBaseURL:              arkVideoProviderBaseURL,
		APIEndpoint:             arkVideoTasksEndpoint,
		APIKey:                  "ark-seedance-2-key",
		VideoReadinessStatus:    arkVideoReadinessStatusPassed,
		VideoReadinessReason:    "",
		VideoReadinessCheckedAt: &now,
	}).Error; err != nil {
		t.Fatalf("seed Seedance 2.0 model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-seedance2-low-balance", "secret123")
	setUserCredits(t, testApp, user.ID, 499)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "seedance 2.0 1080p needs five hundred credits",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        arkSeedance2RuntimeModel,
		"resolution":   "1080p",
	}, cookies)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected credits insufficient 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"credits_insufficient"`)) ||
		!bytes.Contains(resp.Body.Bytes(), []byte(`"required_credits":500`)) ||
		!bytes.Contains(resp.Body.Bytes(), []byte(`"available_credits":499`)) {
		t.Fatalf("expected required video credits payload, got %s", resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("expected insufficient credits to stop before provider call, got %+v", videoProvider.submitInputs)
	}
}

func TestEstimateVideoGenerationPricesVideoDS2AsSeedance20Fast(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "estimate-video-ds-provider-call"}}
	testApp.videoProvider = videoProvider

	var zz ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&zz).Error; err != nil {
		t.Fatalf("load ZZ video model: %v", err)
	}
	if err := db.Model(&zz).Updates(map[string]any{
		"name":          "Video DS 2.0",
		"runtime_model": "video-ds-2.0",
		"permission":    ModelConfigPermissionPublic,
		"api_key":       "zz-video-ds-key",
	}).Error; err != nil {
		t.Fatalf("publish Video DS 2.0 alias: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-ds-estimate", "secret123")
	setUserCredits(t, testApp, user.ID, 300)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/estimate", map[string]any{
		"prompt":       "estimate video ds 480p",
		"aspect_ratio": "16:9",
		"duration":     "15",
		"model":        "video-ds-2.0",
		"resolution":   "480p",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected Video DS estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload := decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 270 || payload.AvailableCredits != 300 || payload.MissingCredits != 0 || !payload.Enough {
		t.Fatalf("expected 480p estimate required=270 available=300 enough=true, got %+v", payload)
	}

	resp = performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/estimate", map[string]any{
		"prompt":       "estimate video ds 720p",
		"aspect_ratio": "16:9",
		"duration":     "15",
		"model":        "video-ds-2.0",
		"resolution":   "720p",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected Video DS 720p estimate 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload = decodeCreditEstimateTestPayload(t, resp.Body.Bytes())
	if payload.RequiredCredits != 360 || payload.AvailableCredits != 300 || payload.MissingCredits != 60 || payload.Enough {
		t.Fatalf("expected 720p estimate required=360 available=300 missing=60 enough=false, got %+v", payload)
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("estimate must not call provider, got %+v", videoProvider.submitInputs)
	}
	assertNoGenerationRecordsForUser(t, db, user.ID)
}

func TestAsyncVideoGenerationDeductsVideoDS2Seedance20FastCost(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "video-ds-alias-task"},
		pollResults: []VideoTaskResult{
			{TaskID: "video-ds-alias-task", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("video-ds-alias-video")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var zz ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&zz).Error; err != nil {
		t.Fatalf("load ZZ video model: %v", err)
	}
	if err := db.Model(&zz).Updates(map[string]any{
		"name":          "Video DS 2.0",
		"runtime_model": "video-ds-2.0",
		"permission":    ModelConfigPermissionPublic,
		"api_key":       "zz-video-ds-key",
	}).Error; err != nil {
		t.Fatalf("publish Video DS 2.0 alias: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-ds-generate", "secret123")
	setUserCredits(t, testApp, user.ID, 270)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "generate video ds 480p",
		"aspect_ratio": "16:9",
		"duration":     "15",
		"model":        "video-ds-2.0",
		"resolution":   "480p",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Video DS generation, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)

	var record GenerationRecord
	if err := db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	var videoRecord VideoGenerationRecord
	if err := db.Where("generation_record_id = ?", created.GenerationID).First(&videoRecord).Error; err != nil {
		t.Fatalf("load video audit record: %v", err)
	}
	if record.Status != GenerationStatusSucceeded || !record.CreditsDeducted || record.CreditsCost != 270 {
		t.Fatalf("expected successful generation record with 270 credits deducted, got %+v", record)
	}
	if videoRecord.Status != GenerationStatusSucceeded || !videoRecord.CreditsDeducted || videoRecord.CreditsCost != 270 {
		t.Fatalf("expected matching video audit record with 270 credits deducted, got %+v", videoRecord)
	}
	if len(videoProvider.submitInputs) != 1 || videoProvider.submitInputs[0].Resolution != "480p" {
		t.Fatalf("expected provider input to keep 480p resolution, got %+v", videoProvider.submitInputs)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 0 {
		t.Fatalf("expected balance 0 after 270-credit video, got %d", balance.AvailableCredits)
	}
}

func TestAsyncVideoGenerationPassesSeedance2MultimodalReferences(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "ark-multimodal-task"},
		pollResults: []VideoTaskResult{
			{TaskID: "ark-multimodal-task", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("ark-multimodal-video")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	now := time.Now()
	if err := db.Model(&ModelConfig{}).
		Where("runtime_model = ?", arkSeedance2RuntimeModel).
		Updates(map[string]any{
			"permission":                 ModelConfigPermissionPublic,
			"api_key":                    "ark-seedance-2-key",
			"video_readiness_status":     arkVideoReadinessStatusPassed,
			"video_readiness_reason":     "",
			"video_readiness_checked_at": &now,
		}).Error; err != nil {
		t.Fatalf("publish Seedance 2.0 model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-seedance2-multimodal", "secret123")
	setUserCredits(t, testApp, user.ID, 1000)
	imageA := seedReferenceAsset(t, testApp, user.ID, "image-a.png", "image/png", []byte("image-a"))
	imageB := seedReferenceAsset(t, testApp, user.ID, "image-b.jpg", "image/jpeg", []byte("image-b"))
	video := seedReferenceAsset(t, testApp, user.ID, "motion.mp4", "video/mp4", []byte("video-ref"))
	audio := seedReferenceAsset(t, testApp, user.ID, "music.mp3", "audio/mpeg", []byte("audio-ref"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":                    "seedance 2 multimodal references",
		"aspect_ratio":              "16:9",
		"duration":                  "11",
		"model":                     arkSeedance2RuntimeModel,
		"resolution":                "1080p",
		"reference_asset_ids":       []uint{imageA.ID, imageB.ID},
		"reference_video_asset_ids": []uint{video.ID},
		"reference_audio_asset_ids": []uint{audio.ID},
		"generate_audio":            true,
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Seedance 2.0 multimodal generation, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.Model != arkSeedance2RuntimeModel ||
		input.Duration != "11" ||
		input.Resolution != "1080p" ||
		!input.GenerateAudio ||
		len(input.Images) != 2 ||
		len(input.ReferenceVideos) != 1 ||
		len(input.ReferenceAudios) != 1 {
		t.Fatalf("expected multimodal provider input, got %+v", input)
	}
}

func TestAsyncVideoGenerationUsesZZModelCenterChannel(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "zz-task-1"},
		pollResults: []VideoTaskResult{
			{TaskID: "zz-task-1", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("zz-video-bytes")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var zz ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&zz).Error; err != nil {
		t.Fatalf("load ZZ video model: %v", err)
	}
	if err := db.Model(&zz).Updates(map[string]any{
		"permission": ModelConfigPermissionPublic,
	}).Error; err != nil {
		t.Fatalf("publish ZZ model with model center key only: %v", err)
	}
	channel := setVideoModelCenterProviderKey(t, testApp, zz.ID, "ZZ API", "zz", "https://model-center.zz.example", "zz-channel-key")
	if err := db.Model(&ModelChannel{}).Where("id = ?", channel.ID).Update("endpoint", "/v1/videos").Error; err != nil {
		t.Fatalf("update ZZ channel endpoint: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-zz-model-center", "secret123")
	setUserCredits(t, testApp, user.ID, 300)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":                    "zz model center video",
		"aspect_ratio":              "1:1",
		"duration":                  "15",
		"model":                     zzVideoDSFastRuntimeModel,
		"images":                    []string{"https://assets.example/ref.png"},
		"reference_video_asset_ids": []uint{},
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted ZZ video generation, got %d: %s", resp.Code, resp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return len(videoProvider.submitInputs) == 1
	})
	input := videoProvider.submitInputs[0]
	if input.Model != zzVideoDSFastRuntimeModel ||
		input.Duration != "15" ||
		input.AspectRatio != "1:1" ||
		input.ProviderBaseURL != "https://model-center.zz.example" ||
		input.ProviderAPIEndpoint != "/v1/videos" ||
		input.ProviderAPIKey != "zz-channel-key" {
		t.Fatalf("expected ZZ provider input from model center channel, got %+v", input)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)
}

func TestEstimateVideoGenerationValidatesSeedance2ReferenceMedia(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "estimate-should-not-submit"}}
	testApp.videoProvider = videoProvider

	now := time.Now()
	if err := db.Model(&ModelConfig{}).
		Where("runtime_model = ?", arkSeedance2RuntimeModel).
		Updates(map[string]any{
			"permission":                 ModelConfigPermissionPublic,
			"api_key":                    "ark-seedance-2-key",
			"video_readiness_status":     arkVideoReadinessStatusPassed,
			"video_readiness_reason":     "",
			"video_readiness_checked_at": &now,
		}).Error; err != nil {
		t.Fatalf("publish Seedance 2.0 model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-seedance2-estimate-media", "secret123")
	setUserCredits(t, testApp, user.ID, 1000)
	video := seedReferenceAsset(t, testApp, user.ID, "motion.mp4", "video/mp4", []byte("video-ref"))
	audio := seedReferenceAsset(t, testApp, user.ID, "music.mp3", "audio/mpeg", []byte("audio-ref"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/estimate", map[string]any{
		"prompt":                    "estimate seedance 2 multimodal references",
		"aspect_ratio":              "16:9",
		"duration":                  "11",
		"model":                     arkSeedance2RuntimeModel,
		"resolution":                "720p",
		"reference_video_asset_ids": []uint{video.ID},
		"reference_audio_asset_ids": []uint{audio.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected estimate 200 with video/audio references, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("estimate must not submit provider task, got %+v", videoProvider.submitInputs)
	}

	image := seedReferenceAsset(t, testApp, user.ID, "not-a-video.png", "image/png", []byte("image-ref"))
	badResp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/estimate", map[string]any{
		"prompt":                    "bad media kind",
		"aspect_ratio":              "16:9",
		"duration":                  "11",
		"model":                     arkSeedance2RuntimeModel,
		"reference_video_asset_ids": []uint{image.ID},
	}, cookies)
	if badResp.Code != http.StatusBadRequest || !strings.Contains(badResp.Body.String(), "invalid_reference_asset_type") {
		t.Fatalf("expected invalid video reference MIME type, got %d: %s", badResp.Code, badResp.Body.String())
	}
}

func TestVideoGenerationRejectsReferenceMediaForUnsupportedModel(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "unsupported-media-task"}}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-ref-media-unsupported", "secret123")
	setUserCredits(t, testApp, user.ID, 1000)
	video := seedReferenceAsset(t, testApp, user.ID, "motion.mp4", "video/mp4", []byte("video-ref"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":                    "sora cannot use video references",
		"aspect_ratio":              "16:9",
		"duration":                  "10",
		"model":                     "sora-2",
		"reference_video_asset_ids": []uint{video.ID},
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "reference_video_unsupported") {
		t.Fatalf("expected unsupported reference video error, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("unsupported reference media must stop before provider call, got %+v", videoProvider.submitInputs)
	}
}

func TestAsyncVideoGenerationDeductsPerSecondCostAfterSuccess(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitResult: VideoSubmitResult{TaskID: "ark-mini-480p"},
		pollResults: []VideoTaskResult{
			{TaskID: "ark-mini-480p", Status: VideoTaskSucceeded, OutputBase64: base64.StdEncoding.EncodeToString([]byte("ark-mini-video")), MIMEType: "video/mp4"},
		},
	}
	testApp.videoProvider = videoProvider

	var doubao ModelConfig
	if err := db.Where("runtime_model = ?", arkSeedanceMiniRuntimeModel).First(&doubao).Error; err != nil {
		t.Fatalf("load Doubao model: %v", err)
	}
	if err := db.Model(&doubao).Updates(map[string]any{
		"permission":                 ModelConfigPermissionPublic,
		"api_key":                    "ark-mini-key",
		"video_readiness_status":     "passed",
		"video_readiness_reason":     "",
		"video_readiness_checked_at": time.Now(),
	}).Error; err != nil {
		t.Fatalf("publish Doubao model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-mini-480p-billing", "secret123")
	setUserCredits(t, testApp, user.ID, 40)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "seedance mini 480p success",
		"aspect_ratio": "16:9",
		"duration":     "4",
		"model":        arkSeedanceMiniRuntimeModel,
		"resolution":   "480p",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted Doubao generation, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}
	waitForVideoGenerationTasksToSettle(t, testApp)

	var record GenerationRecord
	if err := db.First(&record, created.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	var videoRecord VideoGenerationRecord
	if err := db.Where("generation_record_id = ?", created.GenerationID).First(&videoRecord).Error; err != nil {
		t.Fatalf("load video audit record: %v", err)
	}
	if record.Status != GenerationStatusSucceeded || !record.CreditsDeducted || record.CreditsCost != 40 {
		t.Fatalf("expected successful generation record with 40 credits deducted, got %+v", record)
	}
	if videoRecord.Status != GenerationStatusSucceeded || !videoRecord.CreditsDeducted || videoRecord.CreditsCost != 40 {
		t.Fatalf("expected matching video audit record with 40 credits deducted, got %+v", videoRecord)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 0 {
		t.Fatalf("expected balance 0 after 40-credit video, got %d", balance.AvailableCredits)
	}
}

func TestAsyncVideoGenerationDisablesArkModelAfterUnsupportedContentGeneration(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{
		submitErr: &ProviderError{
			HTTPStatus:        http.StatusBadRequest,
			Code:              "InvalidParameter",
			Message:           "The parameter `model` specified in the request is not valid: the specified model doubao-seed-2-0-mini does not support content generation.",
			ProviderRequestID: "req-unsupported-content-generation",
			FailureStage:      providerFailureStageVideoSubmitRequest,
		},
	}
	testApp.videoProvider = videoProvider

	var doubao ModelConfig
	if err := db.Where("runtime_model = ?", arkSeedanceMiniRuntimeModel).First(&doubao).Error; err != nil {
		t.Fatalf("load Doubao model: %v", err)
	}
	if err := db.Model(&doubao).Updates(map[string]any{
		"permission":                 ModelConfigPermissionPublic,
		"api_key":                    "ark-test-key",
		"video_readiness_status":     "passed",
		"video_readiness_reason":     "",
		"video_readiness_checked_at": time.Now(),
	}).Error; err != nil {
		t.Fatalf("publish ready Doubao model: %v", err)
	}

	user, cookies := createLoggedInUser(t, testApp, "video-doubao-unsupported", "secret123")
	setUserCredits(t, testApp, user.ID, 50)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "seedance unsupported dry run",
		"aspect_ratio": "16:9",
		"duration":     "4",
		"model":        arkSeedanceMiniRuntimeModel,
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected first Doubao request accepted before provider failure, got %d: %s", resp.Code, resp.Body.String())
	}
	var created struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode video generation: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		var record GenerationRecord
		if err := db.First(&record, created.GenerationID).Error; err != nil {
			return false
		}
		return record.Status == GenerationStatusFailed
	})
	if len(videoProvider.submitInputs) != 1 {
		t.Fatalf("expected one provider call before readiness downgrade, got %+v", videoProvider.submitInputs)
	}

	var refreshed ModelConfig
	if err := db.First(&refreshed, doubao.ID).Error; err != nil {
		t.Fatalf("reload Doubao model: %v", err)
	}
	if refreshed.VideoReadinessStatus != "failed" || !strings.Contains(refreshed.VideoReadinessReason, "不支持视频生成 API") || refreshed.VideoReadinessCheckedAt == nil {
		t.Fatalf("expected Doubao readiness failed after Ark unsupported error, got %+v", refreshed)
	}

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "seedance should be blocked",
		"aspect_ratio": "16:9",
		"duration":     "4",
		"model":        arkSeedanceMiniRuntimeModel,
	}, cookies)
	if secondResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected second Doubao request blocked with 422, got %d: %s", secondResp.Code, secondResp.Body.String())
	}
	if !strings.Contains(secondResp.Body.String(), "video_model_unavailable") || !strings.Contains(secondResp.Body.String(), "不支持视频生成 API") {
		t.Fatalf("expected unavailable business error, got %s", secondResp.Body.String())
	}
	if len(videoProvider.submitInputs) != 1 {
		t.Fatalf("expected downgraded Doubao to stop before provider call, got %+v", videoProvider.submitInputs)
	}

	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load credit balance: %v", err)
	}
	if balance.AvailableCredits != 50 {
		t.Fatalf("failed/blocked Doubao requests must not deduct credits, got %d", balance.AvailableCredits)
	}
}

func TestAsyncVideoGenerationRejectsUnknownVideoModelBeforeProvider(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	videoProvider := &stubVideoProvider{submitResult: VideoSubmitResult{TaskID: "unknown-task-1"}}
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-unknown-model", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "unknown model should fail before provider",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "doubao-seed-2-0-pro",
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected unknown video model 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "video_model_unavailable") {
		t.Fatalf("expected video_model_unavailable response, got %s", resp.Body.String())
	}
	if len(videoProvider.submitInputs) != 0 {
		t.Fatalf("expected unknown model to stop before provider call, got %+v", videoProvider.submitInputs)
	}
}

func TestAsyncVideoGenerationStillRejectsSameUserConcurrentTask(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	videoProvider := newBlockingVideoProvider()
	testApp.videoProvider = videoProvider

	user, cookies := createLoggedInUser(t, testApp, "video-concurrent-user", "secret123")
	setUserCredits(t, testApp, user.ID, 20)

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "first video",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "sora-2",
	}, cookies)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("expected first video task 202, got %d: %s", firstResp.Code, firstResp.Body.String())
	}
	waitFor(t, time.Second, func() bool {
		return videoProvider.submitCallCount() == 1
	})

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/videos/generations/async", map[string]any{
		"prompt":       "second video",
		"aspect_ratio": "16:9",
		"duration":     "10",
		"model":        "sora-2",
	}, cookies)
	if secondResp.Code != http.StatusConflict {
		t.Fatalf("expected second video task conflict, got %d: %s", secondResp.Code, secondResp.Body.String())
	}
	if !bytes.Contains(secondResp.Body.Bytes(), []byte(`"code":"generation_in_progress"`)) {
		t.Fatalf("expected generation_in_progress payload, got %s", secondResp.Body.String())
	}

	close(videoProvider.release)
	// 等待放行后的视频生成 goroutine 完成落库，避免其在 t.TempDir() 清理 SQLite 时仍在写入，
	// 触发 "directory not empty" 的清理失败（teardown 竞态）。
	waitFor(t, 3*time.Second, func() bool {
		var pending int64
		if err := testApp.db.Model(&GenerationRecord{}).
			Where("status NOT IN ?", []string{GenerationStatusSucceeded, GenerationStatusFailed}).
			Count(&pending).Error; err != nil {
			return false
		}
		return pending == 0
	})
}

func TestGenerationAcceptsAdvancedParametersAndSourceWork(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("advanced-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_advanced",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_advanced", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	sourceWork := seedSucceededWork(t, testApp, user.ID, "原始校园夜景", "16:9")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":           "增强夜景灯光层次",
		"negative_prompt":  "不要文字、不要水印",
		"aspect_ratio":     "1:1",
		"quality":          "high",
		"style_preset":     "写实",
		"tool_mode":        "upscale",
		"style_strength":   65,
		"reference_weight": 75,
		"seed":             "campus-001",
		"source_work_id":   sourceWork.ID,
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected advanced generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.NegativePrompt != "不要文字、不要水印" || input.Quality != "high" || input.StylePreset != "写实" {
		t.Fatalf("expected advanced textual parameters, got %+v", input)
	}
	if input.ToolMode != "upscale" || input.StyleStrength != 65 || input.ReferenceWeight != 75 || input.Seed != "campus-001" {
		t.Fatalf("expected advanced numeric/tool parameters, got %+v", input)
	}
	if input.SourceImage == nil || input.SourceImage.MIMEType != "image/png" || input.SourceImage.Base64Data == "" {
		t.Fatalf("expected source work image in provider input, got %+v", input.SourceImage)
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
		Parameters   struct {
			NegativePrompt  string `json:"negative_prompt"`
			Quality         string `json:"quality"`
			StylePreset     string `json:"style_preset"`
			ToolMode        string `json:"tool_mode"`
			StyleStrength   int    `json:"style_strength"`
			ReferenceWeight int    `json:"reference_weight"`
			Seed            string `json:"seed"`
			SourceWorkID    uint   `json:"source_work_id"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode advanced payload: %v", err)
	}
	if payload.Parameters.Quality != "high" || payload.Parameters.ToolMode != "upscale" || payload.Parameters.SourceWorkID != sourceWork.ID {
		t.Fatalf("expected parameters in response, got %+v", payload.Parameters)
	}

	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.NegativePrompt != "不要文字、不要水印" || record.Quality != "high" || record.ToolMode != "upscale" {
		t.Fatalf("expected persisted advanced parameters, got %+v", record)
	}
	if record.SourceWorkID == nil || *record.SourceWorkID != sourceWork.ID {
		t.Fatalf("expected persisted source work id, got %+v", record.SourceWorkID)
	}
}

func TestGenerationPassesSelectedModelAPIConfigToProvider(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("configured-model-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_model_config",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_model_config", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	if err := testApp.db.Model(&ModelConfig{}).
		Where("runtime_model = ?", "gpt-image-2").
		Updates(map[string]any{
			"api_base_url": "https://model-api.example",
			"api_endpoint": "/v1/images/generations",
			"api_key":      "model-specific-key",
		}).Error; err != nil {
		t.Fatalf("seed model api config: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "使用后台模型配置",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.Model != "gpt-image-2" || input.ProviderBaseURL != "https://model-api.example" || input.ProviderAPIEndpoint != "/v1/images/generations" || input.ProviderAPIKey != "model-specific-key" {
		t.Fatalf("expected selected model API config in provider input, got %+v", input)
	}
}

func TestGenerationUsesDefaultImageModelIDWhenRuntimeModelsDuplicate(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("duplicate-runtime-configured-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_duplicate_runtime_config",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_duplicate_runtime_model", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	first := ModelConfig{
		Name:         "gpt-image official expensive",
		Type:         ModelConfigTypeImage,
		Provider:     "OpenAI",
		Status:       ModelConfigStatusOnline,
		Priority:     1,
		CostLabel:    "5 点/次",
		Permission:   ModelConfigPermissionPublic,
		Weight:       0,
		SortOrder:    5,
		RuntimeModel: "gpt-image-2",
		APIBaseURL:   "https://wrong-route.example",
		APIEndpoint:  "/v1/images/generations",
		APIKey:       "wrong-key",
	}
	second := ModelConfig{
		Name:         "gpt-image distributor cheap",
		Type:         ModelConfigTypeImage,
		Provider:     "OpenAI",
		Status:       ModelConfigStatusOnline,
		Priority:     2,
		CostLabel:    "1 点/次",
		Permission:   ModelConfigPermissionPublic,
		Weight:       0,
		SortOrder:    6,
		RuntimeModel: "gpt-image-2",
		APIBaseURL:   "https://selected-route.example",
		APIEndpoint:  "/v1/images/generations",
		APIKey:       "selected-key",
	}
	if err := testApp.db.Create(&first).Error; err != nil {
		t.Fatalf("create first duplicate runtime route: %v", err)
	}
	if err := testApp.db.Create(&second).Error; err != nil {
		t.Fatalf("create second duplicate runtime route: %v", err)
	}
	if err := testApp.db.Model(&AppSettings{}).Where("id = ?", 1).Updates(map[string]any{
		"default_image_model_id": second.ID,
		"active_image_model":     "gpt-image-2",
	}).Error; err != nil {
		t.Fatalf("select duplicate runtime route: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "使用同运行时模型的指定线路",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.Model != "gpt-image-2" || input.ProviderBaseURL != "https://selected-route.example" || input.ProviderAPIKey != "selected-key" {
		t.Fatalf("expected provider input from selected model id, got %+v", input)
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ModelConfigID != second.ID || record.Model != "gpt-image-2" {
		t.Fatalf("expected generation record linked to selected model config, got %+v", record)
	}
}

func TestGenerationSpeedFirstRoutingUsesFastestRecentSuccessfulModel(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("speed-first-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_speed_first_route",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_speed_first_route", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	adminCookies := createAdminSession(t, testApp)

	disableSeedImageRoutes(t, testApp)
	slow := seedImageRoutingModel(t, testApp, "slow image route", "https://slow-route.example", 101)
	fast := seedImageRoutingModel(t, testApp, "fast image route", "https://fast-route.example", 102)
	saveImageRoutingStrategy(t, testApp, adminCookies, "speed_first", slow.ID, slow.ID, []map[string]any{
		{"id": slow.ID, "weight": 50},
		{"id": fast.ID, "weight": 50},
	})

	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: slow.ID,
		Model:         slow.RuntimeModel,
		Status:        GenerationStatusSucceeded,
		LatencyMS:     2200,
		CreatedAt:     time.Now().Add(-2 * time.Hour),
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: fast.ID,
		Model:         fast.RuntimeModel,
		Status:        GenerationStatusSucceeded,
		LatencyMS:     350,
		CreatedAt:     time.Now().Add(-time.Hour),
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "选择最近响应最快的线路",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	if provider.inputs[0].ProviderBaseURL != "https://fast-route.example" {
		t.Fatalf("expected speed-first routing to use fastest model, got %+v", provider.inputs[0])
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.ModelConfigID != fast.ID {
		t.Fatalf("expected record linked to fastest model, got %+v", record)
	}
}

func TestGenerationSpeedFirstRoutingFallsBackToDefaultWithoutLatencyStats(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("speed-first-default-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_speed_first_default_route",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_speed_first_default_route", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	adminCookies := createAdminSession(t, testApp)

	disableSeedImageRoutes(t, testApp)
	defaultModel := seedImageRoutingModel(t, testApp, "default image route", "https://default-route.example", 101)
	alternate := seedImageRoutingModel(t, testApp, "alternate image route", "https://alternate-route.example", 102)
	saveImageRoutingStrategy(t, testApp, adminCookies, "speed_first", defaultModel.ID, defaultModel.ID, []map[string]any{
		{"id": defaultModel.ID, "weight": 50},
		{"id": alternate.ID, "weight": 50},
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "没有速度统计时使用默认线路",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	if provider.inputs[0].ProviderBaseURL != "https://default-route.example" {
		t.Fatalf("expected default route without latency stats, got %+v", provider.inputs[0])
	}
}

func TestGenerationRoundRobinRoutingUsesLeastUsedOnlineModel(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("round-robin-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_round_robin_route",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_round_robin_route", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	adminCookies := createAdminSession(t, testApp)

	disableSeedImageRoutes(t, testApp)
	first := seedImageRoutingModel(t, testApp, "first round robin route", "https://first-route.example", 101)
	second := seedImageRoutingModel(t, testApp, "second round robin route", "https://second-route.example", 102)
	saveImageRoutingStrategy(t, testApp, adminCookies, "round_robin", first.ID, first.ID, []map[string]any{
		{"id": first.ID, "weight": 50},
		{"id": second.ID, "weight": 50},
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: first.ID,
		Model:         first.RuntimeModel,
		Status:        GenerationStatusSucceeded,
		LatencyMS:     900,
		CreatedAt:     time.Now().Add(-time.Hour),
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "轮询选择使用次数更少的线路",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected one provider input, got %d", len(provider.inputs))
	}
	if provider.inputs[0].ProviderBaseURL != "https://second-route.example" {
		t.Fatalf("expected round-robin routing to use least-used model, got %+v", provider.inputs[0])
	}
}

func TestAsyncImageGenerationAllowsSameUserConcurrentTasks(t *testing.T) {
	provider := newBlockingImageProvider()
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_image_concurrent", "test-password")
	setUserCredits(t, testApp, user.ID, 4)

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "first concurrent image",
		"aspect_ratio": "1:1",
	}, cookies)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("expected first task 202, got %d: %s", firstResp.Code, firstResp.Body.String())
	}

	waitFor(t, time.Second, func() bool {
		return provider.callCount() == 1
	})

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "second concurrent image",
		"aspect_ratio": "1:1",
	}, cookies)
	if secondResp.Code != http.StatusAccepted {
		t.Fatalf("expected second concurrent task 202, got %d: %s", secondResp.Code, secondResp.Body.String())
	}

	var firstCreated, secondCreated struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(firstResp.Body.Bytes(), &firstCreated); err != nil {
		t.Fatalf("decode first generation: %v", err)
	}
	if err := json.Unmarshal(secondResp.Body.Bytes(), &secondCreated); err != nil {
		t.Fatalf("decode second generation: %v", err)
	}
	close(provider.release)

	waitFor(t, 2*time.Second, func() bool {
		var records []GenerationRecord
		if err := testApp.db.Where("id IN ?", []uint{firstCreated.GenerationID, secondCreated.GenerationID}).Find(&records).Error; err != nil {
			return false
		}
		if len(records) != 2 {
			return false
		}
		for _, record := range records {
			if record.Status != GenerationStatusSucceeded {
				return false
			}
		}
		return true
	})
	if provider.callCount() != 2 {
		t.Fatalf("expected two provider calls, got %d", provider.callCount())
	}
}

func TestAsyncImageGenerationPersistsBatchMetadata(t *testing.T) {
	provider := newBlockingImageProvider()
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_batch_images", "test-password")
	setUserCredits(t, testApp, user.ID, 4)

	batchID := "batch-test-20260515"
	variationPrompts := []string{"正面构图，主体居中，干净商业摄影风格", "侧向构图，突出产品材质和高光细节"}
	for index := 0; index < 2; index++ {
		resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
			"prompt":           "same task multi image",
			"aspect_ratio":     "1:1",
			"batch_id":         batchID,
			"batch_index":      index,
			"batch_total":      2,
			"seed":             fmt.Sprintf("batch-seed-%d", index),
			"variation_mode":   "balanced",
			"variation_prompt": variationPrompts[index],
		}, cookies)
		if resp.Code != http.StatusAccepted {
			t.Fatalf("expected batch task %d to be accepted, got %d: %s", index, resp.Code, resp.Body.String())
		}
	}
	close(provider.release)

	waitFor(t, 2*time.Second, func() bool {
		var count int64
		if err := testApp.db.Model(&Work{}).Where("user_id = ? AND prompt = ?", user.ID, "same task multi image").Count(&count).Error; err != nil {
			return false
		}
		return count == 2
	})

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?category=image&page_size=30", nil, cookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected works list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode works list: %v", err)
	}

	seenIndexes := map[int]bool{}
	for _, item := range payload.Items {
		if item["prompt"] != "same task multi image" {
			continue
		}
		if item["batch_id"] != batchID {
			t.Fatalf("expected batch_id %q in works payload, got %+v", batchID, item)
		}
		if int(item["batch_total"].(float64)) != 2 {
			t.Fatalf("expected batch_total 2 in works payload, got %+v", item)
		}
		seenIndexes[int(item["batch_index"].(float64))] = true
	}
	if !seenIndexes[0] || !seenIndexes[1] {
		t.Fatalf("expected both batch indexes in works payload, got %+v from %s", seenIndexes, listResp.Body.String())
	}

	if provider.callCount() != 2 {
		t.Fatalf("expected two provider calls, got %d", provider.callCount())
	}
	if len(provider.inputs) != 2 {
		t.Fatalf("expected two provider inputs, got %d", len(provider.inputs))
	}
	seenSeeds := map[string]bool{}
	for _, input := range provider.inputs {
		if input.VariationMode != "balanced" {
			t.Fatalf("expected provider variation mode balanced, got %+v", input)
		}
		matchedVariationPrompt := false
		for _, expectedPrompt := range variationPrompts {
			if input.VariationPrompt == expectedPrompt {
				matchedVariationPrompt = true
				break
			}
		}
		if !matchedVariationPrompt {
			t.Fatalf("expected provider variation prompt from %+v, got %+v", variationPrompts, input)
		}
		seenSeeds[input.Seed] = true
	}
	if !seenSeeds["batch-seed-0"] || !seenSeeds["batch-seed-1"] {
		t.Fatalf("expected distinct provider seeds, got %+v", seenSeeds)
	}

	var records []GenerationRecord
	if err := testApp.db.Where("user_id = ? AND prompt = ?", user.ID, "same task multi image").Order("batch_index asc").Find(&records).Error; err != nil {
		t.Fatalf("load batch records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected two batch records, got %d", len(records))
	}
	for index, record := range records {
		if record.Seed != fmt.Sprintf("batch-seed-%d", index) || record.VariationMode != "balanced" || record.VariationPrompt != variationPrompts[index] {
			t.Fatalf("expected persisted variation metadata at %d, got %+v", index, record)
		}
	}
}

func TestAsyncImageGenerationAllowsDifferentUsersConcurrentTasks(t *testing.T) {
	provider := newBlockingImageProvider()
	testApp, _ := newTestApp(t, provider)
	firstUser, firstCookies := createLoggedInUser(t, testApp, "creator_first_concurrent", "test-password")
	secondUser, secondCookies := createLoggedInUser(t, testApp, "creator_second_concurrent", "test-password")
	setUserCredits(t, testApp, firstUser.ID, 2)
	setUserCredits(t, testApp, secondUser.ID, 2)

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "first user image",
		"aspect_ratio": "1:1",
	}, firstCookies)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("expected first user task 202, got %d: %s", firstResp.Code, firstResp.Body.String())
	}
	waitFor(t, time.Second, func() bool {
		return provider.callCount() == 1
	})

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "second user image",
		"aspect_ratio": "1:1",
	}, secondCookies)
	if secondResp.Code != http.StatusAccepted {
		t.Fatalf("expected second user task 202, got %d: %s", secondResp.Code, secondResp.Body.String())
	}

	close(provider.release)
	waitFor(t, 2*time.Second, func() bool {
		var succeeded int64
		if err := testApp.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusSucceeded).Count(&succeeded).Error; err != nil {
			return false
		}
		return succeeded == 2
	})
}

func TestConcurrentImageGenerationWithSingleCreditOnlyDeductsOnce(t *testing.T) {
	provider := newBlockingImageProvider()
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_single_credit_concurrent", "test-password")
	setUserCredits(t, testApp, user.ID, 1)

	firstResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "single credit first image",
		"aspect_ratio": "1:1",
	}, cookies)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("expected first task 202, got %d: %s", firstResp.Code, firstResp.Body.String())
	}
	waitFor(t, time.Second, func() bool {
		return provider.callCount() == 1
	})

	secondResp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "single credit second image",
		"aspect_ratio": "1:1",
	}, cookies)
	if secondResp.Code != http.StatusConflict {
		t.Fatalf("expected second task to fail credit reservation with 409, got %d: %s", secondResp.Code, secondResp.Body.String())
	}
	if !bytes.Contains(secondResp.Body.Bytes(), []byte(`"code":"credits_insufficient"`)) {
		t.Fatalf("expected credits_insufficient response, got %s", secondResp.Body.String())
	}
	close(provider.release)

	waitFor(t, 2*time.Second, func() bool {
		var terminalCount int64
		if err := testApp.db.Model(&GenerationRecord{}).
			Where("status IN ?", []string{GenerationStatusSucceeded, GenerationStatusFailed}).
			Count(&terminalCount).Error; err != nil {
			return false
		}
		return terminalCount == 1
	})

	var succeededCount int64
	if err := testApp.db.Model(&GenerationRecord{}).Where("status = ?", GenerationStatusSucceeded).Count(&succeededCount).Error; err != nil {
		t.Fatalf("count succeeded records: %v", err)
	}
	if succeededCount != 1 {
		t.Fatalf("expected exactly one succeeded generation, got %d", succeededCount)
	}

	if provider.callCount() != 1 {
		t.Fatalf("expected only the reserved task to reach provider, got %d calls", provider.callCount())
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 0 {
		t.Fatalf("expected balance 0, got %d", balance.AvailableCredits)
	}
}

func TestAsyncImageGenerationRateLimitAllowsTwentyRequestsPerMinute(t *testing.T) {
	provider := &stubProvider{err: &ProviderError{HTTPStatus: http.StatusBadRequest, Code: "invalid_model", Message: "bad model"}}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_rate_twenty", "test-password")
	setUserCredits(t, testApp, user.ID, 25)

	for index := 0; index < 20; index++ {
		resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
			"prompt":       fmt.Sprintf("rate limited image %d", index),
			"aspect_ratio": "1:1",
		}, cookies)
		if resp.Code != http.StatusAccepted {
			t.Fatalf("expected request %d to be accepted, got %d: %s", index+1, resp.Code, resp.Body.String())
		}
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "rate limited image 21",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected request 21 to be rate limited, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"too_many_requests"`)) {
		t.Fatalf("expected too_many_requests payload, got %s", resp.Body.String())
	}
}

func TestGenerationRejectsInvalidAdvancedParameters(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_bad_params", "test-password")
	setUserCredits(t, testApp, user.ID, 1)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":           "bad quality",
		"aspect_ratio":     "1:1",
		"quality":          "ultra",
		"tool_mode":        "generate",
		"style_strength":   101,
		"reference_weight": -1,
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid params 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestEditGenerationRejectsWithoutSourceWorkOrReferenceAsset(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_edit_without_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "移除背景",
		"aspect_ratio": "1:1",
		"tool_mode":    GenerationToolModeRemoveBackground,
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected edit source validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestUpscaleGenerationRejectsWithoutSourceImage(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_upscale_without_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "提升图片清晰度",
		"aspect_ratio": "1:1",
		"tool_mode":    GenerationToolModeUpscale,
		"tool_options": map[string]any{"scale": "2x"},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected upscale source validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestUpscaleGenerationRejectsMultipleSourceImages(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_upscale_multi_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	first := seedReferenceAsset(t, testApp, user.ID, "first-upscale.png", "image/png", []byte("first-image"))
	second := seedReferenceAsset(t, testApp, user.ID, "second-upscale.png", "image/png", []byte("second-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "提升图片清晰度",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeUpscale,
		"tool_options":        map[string]any{"scale": "2x"},
		"reference_asset_ids": []uint{first.ID, second.ID},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected upscale source count validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestUpscaleGenerationRejectsInvalidScale(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_upscale_bad_scale", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	source := seedReferenceAsset(t, testApp, user.ID, "upscale-source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "提升图片清晰度",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeUpscale,
		"tool_options":        map[string]any{"scale": "16x"},
		"reference_asset_ids": []uint{source.ID},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid upscale scale 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestUpscaleGenerationUsesSingleSourceAndAugmentedPrompt(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("upscale-result")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_upscale",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_upscale_source", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	source := seedReferenceAsset(t, testApp, user.ID, "upscale-source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "提升图片清晰度、纹理细节和边缘质量，保持主体、颜色、构图和内容不变",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeUpscale,
		"tool_options":        map[string]any{"scale": "4x"},
		"reference_asset_ids": []uint{source.ID},
		"edit_instruction":    "增强织物纹理和金属边缘",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected upscale 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || len(provider.inputs) != 1 {
		t.Fatalf("expected provider called once, got calls=%d inputs=%d", provider.calls, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.ToolMode != GenerationToolModeUpscale {
		t.Fatalf("expected upscale tool mode, got %+v", input)
	}
	if input.SourceImage == nil || input.SourceImage.Base64Data == "" || input.SourceImage.MIMEType != "image/png" {
		t.Fatalf("expected source image in provider input, got %+v", input.SourceImage)
	}
	if len(input.ReferenceImages) != 0 {
		t.Fatalf("expected upscale source not to remain in references, got %+v", input.ReferenceImages)
	}
	for _, expected := range []string{"放大倍率：4x", "AI 高清增强", "目标倍率", "保持主体", "颜色", "构图", "不新增物体", "不要重绘成新图", "增强说明：增强织物纹理和金属边缘"} {
		if !strings.Contains(input.Prompt, expected) {
			t.Fatalf("expected upscale prompt to contain %q, got %q", expected, input.Prompt)
		}
	}
}

func TestRemoveBackgroundGenerationRejectsMultipleSourceImages(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_remove_background_multi_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	first := seedReferenceAsset(t, testApp, user.ID, "first.png", "image/png", []byte("first-image"))
	second := seedReferenceAsset(t, testApp, user.ID, "second.png", "image/png", []byte("second-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "移除背景",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeRemoveBackground,
		"reference_asset_ids": []uint{first.ID, second.ID},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected remove background source count validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestPrecisionEditRejectsWithoutMaskOrRegion(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_precision_edit_without_mask", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	asset := seedReferenceAsset(t, testApp, user.ID, "source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "把圈选区域改成红色礼盒",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModePrecisionEdit,
		"reference_asset_ids": []uint{asset.ID},
		"edit_instruction":    "只修改圈选区域",
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected precision edit validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestPrecisionEditRejectsWithoutSourceImage(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_precision_edit_without_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":           "把圈选区域改成红色礼盒",
		"aspect_ratio":     "1:1",
		"tool_mode":        GenerationToolModePrecisionEdit,
		"edit_instruction": "把圈选区域改成红色礼盒",
		"tool_options": map[string]any{
			"mask_regions": []map[string]any{
				{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4},
			},
		},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected precision edit source validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestPrecisionEditRejectsMultipleSourceImages(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_precision_edit_multi_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	first := seedReferenceAsset(t, testApp, user.ID, "precision-first.png", "image/png", []byte("first-image"))
	second := seedReferenceAsset(t, testApp, user.ID, "precision-second.png", "image/png", []byte("second-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "把圈选区域改成红色礼盒",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModePrecisionEdit,
		"reference_asset_ids": []uint{first.ID, second.ID},
		"edit_instruction":    "把圈选区域改成红色礼盒",
		"tool_options": map[string]any{
			"mask_regions": []map[string]any{
				{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4},
			},
		},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected precision edit source count validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestPrecisionEditRejectsWithoutEditInstruction(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_precision_edit_without_instruction", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	source := seedReferenceAsset(t, testApp, user.ID, "precision-source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "局部改图",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModePrecisionEdit,
		"reference_asset_ids": []uint{source.ID},
		"tool_options": map[string]any{
			"mask_regions": []map[string]any{
				{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4},
			},
		},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected precision edit instruction validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestPrecisionEditRejectsInvalidMaskRegions(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_precision_edit_invalid_region", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	source := seedReferenceAsset(t, testApp, user.ID, "precision-source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "把圈选区域改成红色礼盒",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModePrecisionEdit,
		"reference_asset_ids": []uint{source.ID},
		"edit_instruction":    "把圈选区域改成红色礼盒",
		"tool_options": map[string]any{
			"mask_regions": []map[string]any{
				{"x": 0.8, "y": 0.2, "width": 0.4, "height": 0.4},
			},
		},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected precision edit region validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestPrecisionEditWithMaskUsesSourceMaskAndStrictLocalPrompt(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("precision-edit-result")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_precision_edit",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_precision_edit_mask", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	source := seedReferenceAsset(t, testApp, user.ID, "precision-source.png", "image/png", []byte("source-image"))
	mask := seedReferenceAsset(t, testApp, user.ID, "precision-mask.png", "image/png", []byte("mask-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "把圈选区域改成红色礼盒",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModePrecisionEdit,
		"reference_asset_ids": []uint{source.ID},
		"mask_asset_id":       mask.ID,
		"edit_instruction":    "把圈选区域改成红色礼盒",
		"tool_options": map[string]any{
			"mask_regions": []map[string]any{
				{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4},
			},
		},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected precision edit 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || len(provider.inputs) != 1 {
		t.Fatalf("expected provider called once, got calls=%d inputs=%d", provider.calls, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.ToolMode != GenerationToolModePrecisionEdit {
		t.Fatalf("expected precision edit tool mode, got %+v", input)
	}
	if input.SourceImage == nil || input.SourceImage.Base64Data == "" {
		t.Fatalf("expected source image in provider input, got %+v", input.SourceImage)
	}
	if input.MaskImage == nil || input.MaskImage.Base64Data == "" {
		t.Fatalf("expected mask image in provider input, got %+v", input.MaskImage)
	}
	if len(input.MaskRegions) != 1 {
		t.Fatalf("expected one mask region, got %+v", input.MaskRegions)
	}
	if len(input.ReferenceImages) != 0 {
		t.Fatalf("expected no ordinary reference images, got %+v", input.ReferenceImages)
	}
	for _, expected := range []string{"仅修改圈选区域", "保持未选区域不变", "不要重绘整张图", "主体", "颜色", "构图", "材质", "文字", "边缘"} {
		if !strings.Contains(input.Prompt, expected) {
			t.Fatalf("expected precision edit prompt to contain %q, got %q", expected, input.Prompt)
		}
	}
}

func TestEraseGenerationRejectsWithoutSourceImage(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_erase_without_source", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":           "移除画面左侧路人",
		"aspect_ratio":     "1:1",
		"tool_mode":        GenerationToolModeErase,
		"edit_instruction": "移除画面左侧路人",
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected erase source validation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestEraseGenerationRejectsWithoutInstructionOrMask(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_erase_empty", "test-password")
	setUserCredits(t, testApp, user.ID, 2)
	source := seedReferenceAsset(t, testApp, user.ID, "erase-source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "移除物体",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeErase,
		"reference_asset_ids": []uint{source.ID},
	}, cookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected erase empty operation 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestEraseGenerationWithTextInstructionUsesSourceAndAugmentedPrompt(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("erase-result")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_erase_text",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_erase_text", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	source := seedReferenceAsset(t, testApp, user.ID, "erase-source.png", "image/png", []byte("source-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "移除画面左侧路人",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeErase,
		"reference_asset_ids": []uint{source.ID},
		"edit_instruction":    "移除画面左侧路人",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected erase 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || len(provider.inputs) != 1 {
		t.Fatalf("expected provider called once, got calls=%d inputs=%d", provider.calls, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.ToolMode != GenerationToolModeErase {
		t.Fatalf("expected erase tool mode, got %+v", input)
	}
	if input.SourceImage == nil || input.SourceImage.Base64Data == "" {
		t.Fatalf("expected first reference asset to be source image, got %+v", input.SourceImage)
	}
	if !strings.Contains(input.Prompt, "移除指定物体") || !strings.Contains(input.Prompt, "自然补全背景") {
		t.Fatalf("expected erase prompt instructions, got %q", input.Prompt)
	}
	if !strings.Contains(input.Prompt, "移除说明：移除画面左侧路人") {
		t.Fatalf("expected edit instruction in prompt, got %q", input.Prompt)
	}
}

func TestEraseGenerationWithMaskPersistsAndPassesProviderContext(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("erase-mask-result")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_erase_mask",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_erase_mask", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	source := seedReferenceAsset(t, testApp, user.ID, "erase-source.png", "image/png", []byte("source-image"))
	mask := seedReferenceAsset(t, testApp, user.ID, "erase-mask.png", "image/png", []byte("mask-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "移除圈选区域中的干扰物",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeErase,
		"reference_asset_ids": []uint{source.ID},
		"mask_asset_id":       mask.ID,
		"tool_options": map[string]any{
			"mask_regions": []map[string]any{
				{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4},
			},
		},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected erase mask 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || len(provider.inputs) != 1 {
		t.Fatalf("expected provider called once, got calls=%d inputs=%d", provider.calls, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.MaskImage == nil || input.MaskImage.Base64Data == "" {
		t.Fatalf("expected mask image in provider input, got %+v", input.MaskImage)
	}
	if !strings.Contains(input.Prompt, "仅处理圈选区域") || !strings.Contains(input.Prompt, "白色区域为待移除区域") {
		t.Fatalf("expected mask constraints in prompt, got %q", input.Prompt)
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	var record GenerationRecord
	if err := testApp.db.First(&record, payload.GenerationID).Error; err != nil {
		t.Fatalf("load generation record: %v", err)
	}
	if record.MaskAssetID == nil || *record.MaskAssetID != mask.ID {
		t.Fatalf("expected mask asset id %d, got %+v", mask.ID, record.MaskAssetID)
	}
	if !strings.Contains(record.ToolOptionsJSON, "mask_regions") {
		t.Fatalf("expected mask regions persisted, got %q", record.ToolOptionsJSON)
	}
}

func TestEraseGenerationRejectsMaskAssetFromAnotherUser(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	owner, _ := createLoggedInUser(t, testApp, "creator_mask_owner", "test-password")
	viewer, viewerCookies := createLoggedInUser(t, testApp, "creator_mask_viewer", "test-password")
	setUserCredits(t, testApp, viewer.ID, 2)
	source := seedReferenceAsset(t, testApp, viewer.ID, "erase-source.png", "image/png", []byte("source-image"))
	mask := seedReferenceAsset(t, testApp, owner.ID, "erase-mask.png", "image/png", []byte("mask-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "移除圈选区域中的干扰物",
		"aspect_ratio":        "1:1",
		"tool_mode":           GenerationToolModeErase,
		"reference_asset_ids": []uint{source.ID},
		"mask_asset_id":       mask.ID,
	}, viewerCookies)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected foreign mask 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"invalid_generation_parameter"`)) {
		t.Fatalf("expected invalid_generation_parameter, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestEditGenerationUsesFirstReferenceAssetAsSourceImage(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("remove-background-result")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_remove_background",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_edit_reference_source", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	asset := seedReferenceAsset(t, testApp, user.ID, "subject.png", "image/png", []byte("subject-image"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":              "移除背景，只保留商品主体",
		"aspect_ratio":        "1:1",
		"quality":             "high",
		"tool_mode":           GenerationToolModeRemoveBackground,
		"reference_asset_ids": []uint{asset.ID},
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected remove background 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || len(provider.inputs) != 1 {
		t.Fatalf("expected provider called once, got calls=%d inputs=%d", provider.calls, len(provider.inputs))
	}
	input := provider.inputs[0]
	if input.ToolMode != GenerationToolModeRemoveBackground {
		t.Fatalf("expected remove_background tool mode, got %+v", input)
	}
	if input.SourceImage == nil || input.SourceImage.Base64Data == "" || input.SourceImage.MIMEType != "image/png" {
		t.Fatalf("expected first reference asset to be promoted as source image, got source=%+v references=%d", input.SourceImage, len(input.ReferenceImages))
	}
	if len(input.ReferenceImages) != 0 {
		t.Fatalf("expected promoted source image to be removed from reference images, got %d", len(input.ReferenceImages))
	}
	for _, expected := range []string{"透明背景", "保留主体", "不要改变主体", "材质", "颜色", "构图"} {
		if !strings.Contains(input.Prompt, expected) {
			t.Fatalf("expected remove background prompt to contain %q, got %q", expected, input.Prompt)
		}
	}

	var payload struct {
		GenerationID uint `json:"generation_id"`
		Parameters   struct {
			ToolMode          string `json:"tool_mode"`
			Quality           string `json:"quality"`
			ReferenceAssetIDs []uint `json:"reference_asset_ids"`
			SourceWorkID      uint   `json:"source_work_id"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	if payload.Parameters.ToolMode != GenerationToolModeRemoveBackground || payload.Parameters.Quality != "high" {
		t.Fatalf("expected response parameters, got %+v", payload.Parameters)
	}
	if len(payload.Parameters.ReferenceAssetIDs) != 1 || payload.Parameters.ReferenceAssetIDs[0] != asset.ID || payload.Parameters.SourceWorkID != 0 {
		t.Fatalf("expected reference asset source metadata in response, got %+v", payload.Parameters)
	}
}

func TestGenerationRejectsSourceWorkFromAnotherUser(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	owner, _ := createLoggedInUser(t, testApp, "creator_source_owner", "test-password")
	viewer, viewerCookies := createLoggedInUser(t, testApp, "creator_source_viewer", "test-password")
	setUserCredits(t, testApp, viewer.ID, 1)
	sourceWork := seedSucceededWork(t, testApp, owner.ID, "private source", "1:1")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":         "try source",
		"aspect_ratio":   "1:1",
		"tool_mode":      "upscale",
		"source_work_id": sourceWork.ID,
	}, viewerCookies)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected source work 404, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestGenerationRejectsMoreThanFourReferenceAssets(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_refs_limit", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	var ids []uint
	for idx := 0; idx < 5; idx++ {
		asset := seedReferenceAsset(t, testApp, user.ID, fmt.Sprintf("ref-%d.png", idx), "image/png", []byte(fmt.Sprintf("ref-%d", idx)))
		ids = append(ids, asset.ID)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "too many references",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": ids,
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"reference_asset_limit_exceeded"`)) {
		t.Fatalf("expected limit error, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestGenerationRejectsMoreThanFourCombinedReferenceImages(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_combined_refs_limit", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	var assetIDs []uint
	for idx := 0; idx < 3; idx++ {
		asset := seedReferenceAsset(t, testApp, user.ID, fmt.Sprintf("ref-%d.png", idx), "image/png", []byte(fmt.Sprintf("ref-%d", idx)))
		assetIDs = append(assetIDs, asset.ID)
	}
	firstWork := seedSucceededWork(t, testApp, user.ID, "first work reference", "1:1")
	secondWork := seedSucceededWork(t, testApp, user.ID, "second work reference", "1:1")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "too many combined references",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": assetIDs,
		"reference_work_ids":  []uint{firstWork.ID, secondWork.ID},
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"reference_asset_limit_exceeded"`)) {
		t.Fatalf("expected limit error, got %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestGenerationRejectsReferenceAssetFromAnotherUser(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	owner, _ := createLoggedInUser(t, testApp, "creator_refs_owner", "test-password")
	viewer, viewerCookies := createLoggedInUser(t, testApp, "creator_refs_viewer", "test-password")
	setUserCredits(t, testApp, viewer.ID, 1)
	asset := seedReferenceAsset(t, testApp, owner.ID, "shared.png", "image/png", []byte("shared-reference"))

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":              "use someone else's reference",
		"aspect_ratio":        "1:1",
		"reference_asset_ids": []uint{asset.ID},
	}, viewerCookies)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestGenerationRejectsWhenCreditsAreInsufficient(t *testing.T) {
	provider := &stubProvider{}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_empty", "test-password")
	setUserCredits(t, testApp, user.ID, 0)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "a bright studio portrait of a robot florist",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"credits_insufficient"`)) {
		t.Fatalf("expected credits_insufficient: %s", resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called, got %d", provider.calls)
	}
}

func TestDeductGenerationCreditsRejectsSecondChargeWithoutNegativeBalance(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "creator_atomic_credit", "test-password")
	setUserCredits(t, testApp, user.ID, 1)

	remaining, err := deductGenerationCredits(testApp.db, user.ID, 1)
	if err != nil {
		t.Fatalf("first deduct failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected remaining credits 0, got %d", remaining)
	}

	_, err = deductGenerationCredits(testApp.db, user.ID, 1)
	if !errors.Is(err, errCreditsInsufficient) {
		t.Fatalf("expected second deduct credits_insufficient, got %v", err)
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 0 {
		t.Fatalf("expected balance to stay at 0, got %d", balance.AvailableCredits)
	}
}

func TestWorksAreScopedReusableAndDeletable(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_owner", "test-password")
	viewer, viewerCookies := createLoggedInUser(t, testApp, "creator_viewer", "test-password")
	work := seedSucceededWork(t, testApp, owner.ID, "mist over bamboo lake", "3:4")

	ownerList := performJSONRequest(t, testApp, http.MethodGet, "/api/works?q=bamboo", nil, ownerCookies)
	if ownerList.Code != http.StatusOK {
		t.Fatalf("expected owner list 200, got %d", ownerList.Code)
	}
	if !bytes.Contains(ownerList.Body.Bytes(), []byte(`"work_id":`)) {
		t.Fatalf("expected work item in list: %s", ownerList.Body.String())
	}

	otherList := performJSONRequest(t, testApp, http.MethodGet, "/api/works?q=bamboo", nil, viewerCookies)
	if otherList.Code != http.StatusOK {
		t.Fatalf("expected other list 200, got %d", otherList.Code)
	}
	if bytes.Contains(otherList.Body.Bytes(), []byte(`"work_id":`)) {
		t.Fatalf("expected viewer list empty, got %s", otherList.Body.String())
	}

	reuseResp := performJSONRequest(t, testApp, http.MethodPost, "/api/works/"+itoa(work.ID)+"/reuse", nil, ownerCookies)
	if reuseResp.Code != http.StatusOK {
		t.Fatalf("expected reuse 200, got %d: %s", reuseResp.Code, reuseResp.Body.String())
	}
	if !bytes.Contains(reuseResp.Body.Bytes(), []byte(`"prompt":"mist over bamboo lake"`)) {
		t.Fatalf("unexpected reuse payload: %s", reuseResp.Body.String())
	}

	notFoundResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works/"+itoa(work.ID), nil, viewerCookies)
	if notFoundResp.Code != http.StatusNotFound {
		t.Fatalf("expected viewer detail 404, got %d", notFoundResp.Code)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/works/"+itoa(work.ID), nil, ownerCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	var deleted Work
	if err := testApp.db.Unscoped().First(&deleted, work.ID).Error; err != nil {
		t.Fatalf("load deleted work: %v", err)
	}
	if deleted.DeletedAt.Time.IsZero() {
		t.Fatalf("expected work soft deleted: %+v", deleted)
	}

	_ = viewer
}

func TestReuseWorkReturnsGenerationParametersAndReferences(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_reuse_full", "test-password")
	first := seedReferenceAsset(t, testApp, user.ID, "first-reference.png", "image/png", []byte("first-reference"))
	second := seedReferenceAsset(t, testApp, user.ID, "second-reference.jpg", "image/jpeg", []byte("second-reference"))
	work := seedSucceededWork(t, testApp, user.ID, "premium product poster", "9:16")

	var record GenerationRecord
	if err := testApp.db.First(&record, work.GenerationRecordID).Error; err != nil {
		t.Fatalf("load seeded record: %v", err)
	}
	record.NegativePrompt = "不要文字、不要水印"
	record.StylePreset = "电商"
	record.ToolMode = GenerationToolModeGenerate
	record.StyleStrength = 72
	record.ReferenceWeight = 81
	record.Seed = "reuse-seed"
	if err := testApp.db.Save(&record).Error; err != nil {
		t.Fatalf("save reusable record parameters: %v", err)
	}
	if err := testApp.db.Create(&[]GenerationReferenceAsset{
		{GenerationRecordID: record.ID, ReferenceAssetID: second.ID, SortOrder: 0},
		{GenerationRecordID: record.ID, ReferenceAssetID: first.ID, SortOrder: 1},
	}).Error; err != nil {
		t.Fatalf("create generation reference links: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/works/"+itoa(work.ID)+"/reuse", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected reuse 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		WorkID            uint             `json:"work_id"`
		Prompt            string           `json:"prompt"`
		NegativePrompt    string           `json:"negative_prompt"`
		AspectRatio       string           `json:"aspect_ratio"`
		StylePreset       string           `json:"style_preset"`
		ToolMode          string           `json:"tool_mode"`
		StyleStrength     int              `json:"style_strength"`
		ReferenceWeight   int              `json:"reference_weight"`
		Seed              string           `json:"seed"`
		SourceWorkID      uint             `json:"source_work_id"`
		ReferenceAssetIDs []uint           `json:"reference_asset_ids"`
		ReferenceAssets   []ReferenceAsset `json:"reference_assets"`
		ReferenceWorkIDs  []uint           `json:"reference_work_ids"`
		ReferenceWorks    []struct {
			ID         uint   `json:"id"`
			WorkID     uint   `json:"work_id"`
			PreviewURL string `json:"preview_url"`
		} `json:"reference_works"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode reuse payload: %v", err)
	}
	if payload.WorkID != work.ID || payload.Prompt != "premium product poster" || payload.AspectRatio != "9:16" {
		t.Fatalf("unexpected base reuse payload: %+v", payload)
	}
	if payload.NegativePrompt != "不要文字、不要水印" || payload.StylePreset != "电商" || payload.ToolMode != GenerationToolModeGenerate {
		t.Fatalf("expected generation parameters in reuse payload, got %+v", payload)
	}
	if payload.StyleStrength != 72 || payload.ReferenceWeight != 81 || payload.Seed != "reuse-seed" {
		t.Fatalf("expected numeric generation parameters in reuse payload, got %+v", payload)
	}
	if payload.SourceWorkID != work.ID {
		t.Fatalf("expected current work as default source id, got %+v", payload)
	}
	if !reflect.DeepEqual(payload.ReferenceAssetIDs, []uint{second.ID, first.ID}) {
		t.Fatalf("expected ordered reference asset ids, got %+v", payload.ReferenceAssetIDs)
	}
	if len(payload.ReferenceAssets) != 2 || payload.ReferenceAssets[0].ID != second.ID || payload.ReferenceAssets[1].ID != first.ID {
		t.Fatalf("expected ordered reference asset payloads, got %+v", payload.ReferenceAssets)
	}
	if payload.ReferenceAssets[0].PreviewURL == "" || payload.ReferenceAssets[0].OriginalFilename != "second-reference.jpg" {
		t.Fatalf("expected reference preview and filename, got %+v", payload.ReferenceAssets[0])
	}
	if !reflect.DeepEqual(payload.ReferenceWorkIDs, []uint{work.ID}) {
		t.Fatalf("expected current work as image-to-image reference id, got %+v", payload.ReferenceWorkIDs)
	}
	if len(payload.ReferenceWorks) != 1 || (payload.ReferenceWorks[0].ID != work.ID && payload.ReferenceWorks[0].WorkID != work.ID) || payload.ReferenceWorks[0].PreviewURL == "" {
		t.Fatalf("expected current work reference payload, got %+v", payload.ReferenceWorks)
	}
}

func TestWorksCanBeUpdatedByOwnerOnly(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_update_owner", "test-password")
	_, viewerCookies := createLoggedInUser(t, testApp, "creator_update_viewer", "test-password")
	work := seedSucceededWork(t, testApp, owner.ID, "collectible public poster", "16:9")

	updateResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{
		"is_favorite": true,
		"visibility":  WorkVisibilityPublic,
	}, ownerCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected owner update 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	var updated Work
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated work: %v", err)
	}
	if !updated.IsFavorite || updated.Visibility != WorkVisibilityPublic {
		t.Fatalf("expected favorite public work, got %+v", updated)
	}

	viewerResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{
		"is_favorite": false,
	}, viewerCookies)
	if viewerResp.Code != http.StatusNotFound {
		t.Fatalf("expected viewer update 404, got %d: %s", viewerResp.Code, viewerResp.Body.String())
	}

	invalidResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{
		"visibility": "team",
	}, ownerCookies)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid visibility 400, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}

	emptyResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{}, ownerCookies)
	if emptyResp.Code != http.StatusBadRequest {
		t.Fatalf("expected empty patch 400, got %d: %s", emptyResp.Code, emptyResp.Body.String())
	}
}

func TestPublicWorkFileOnlyServesPublicWorks(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_public_file", "test-password")
	privateWork := seedSucceededWork(t, testApp, owner.ID, "private render", "1:1")
	publicWork := seedSucceededWork(t, testApp, owner.ID, "public render", "1:1")

	privateResp := performJSONRequest(t, testApp, http.MethodGet, "/api/public/works/"+itoa(privateWork.ID)+"/file", nil, nil)
	if privateResp.Code != http.StatusNotFound {
		t.Fatalf("expected private public-file 404, got %d: %s", privateResp.Code, privateResp.Body.String())
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(publicWork.ID), map[string]any{
		"visibility": WorkVisibilityPublic,
	}, ownerCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected publish 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	publicResp := performJSONRequest(t, testApp, http.MethodGet, "/api/public/works/"+itoa(publicWork.ID)+"/file", nil, nil)
	if publicResp.Code != http.StatusOK {
		t.Fatalf("expected public file 200, got %d: %s", publicResp.Code, publicResp.Body.String())
	}
	if publicResp.Body.String() != "fake" {
		t.Fatalf("expected public asset bytes, got %q", publicResp.Body.String())
	}
}

func TestPublicWorksListReturnsOnlyPublicWorksInRequestedOrder(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_public_list", "test-password")
	privateWork := seedSucceededWork(t, testApp, owner.ID, "private render", "1:1")
	firstPublic := seedSucceededWork(t, testApp, owner.ID, "first public render", "16:9")
	secondPublic := seedSucceededWork(t, testApp, owner.ID, "second public render", "3:4")

	for _, work := range []Work{firstPublic, secondPublic} {
		updateResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/works/"+itoa(work.ID), map[string]any{
			"visibility": WorkVisibilityPublic,
		}, ownerCookies)
		if updateResp.Code != http.StatusOK {
			t.Fatalf("expected publish 200, got %d: %s", updateResp.Code, updateResp.Body.String())
		}
	}

	resp := performJSONRequest(
		t,
		testApp,
		http.MethodGet,
		"/api/public/works?ids="+itoa(secondPublic.ID)+","+itoa(privateWork.ID)+","+itoa(firstPublic.ID),
		nil,
		nil,
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected public works list 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Items []struct {
			WorkID             uint   `json:"work_id"`
			UserID             uint   `json:"user_id"`
			GenerationRecordID uint   `json:"generation_record_id"`
			Prompt             string `json:"prompt"`
			AspectRatio        string `json:"aspect_ratio"`
			Visibility         string `json:"visibility"`
			AssetKey           string `json:"asset_key"`
			DownloadURL        string `json:"download_url"`
			PreviewURL         string `json:"preview_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode public works list: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected only public works, got %+v from %s", payload.Items, resp.Body.String())
	}
	if payload.Items[0].WorkID != secondPublic.ID || payload.Items[1].WorkID != firstPublic.ID {
		t.Fatalf("expected requested public order, got %+v", payload.Items)
	}
	if payload.Items[0].PreviewURL != "/api/public/works/"+itoa(secondPublic.ID)+"/file" {
		t.Fatalf("expected public preview URL, got %+v", payload.Items[0])
	}
	for _, item := range payload.Items {
		if item.UserID != 0 || item.GenerationRecordID != 0 || item.AssetKey != "" || item.DownloadURL != "" || item.Visibility != "" {
			t.Fatalf("expected public payload to omit private fields, got %+v", item)
		}
		if item.Prompt == "" || item.AspectRatio == "" || item.PreviewURL == "" {
			t.Fatalf("expected display fields and preview URL, got %+v", item)
		}
	}
}

func TestPublicWorksListValidatesIDs(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	cases := []struct {
		name string
		path string
		code string
	}{
		{name: "empty", path: "/api/public/works", code: "invalid_public_work_ids"},
		{name: "invalid", path: "/api/public/works?ids=1,nope", code: "invalid_public_work_ids"},
		{name: "too many", path: "/api/public/works?ids=1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17", code: "too_many_public_work_ids"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performJSONRequest(t, testApp, http.MethodGet, tc.path, nil, nil)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid ids 400, got %d: %s", resp.Code, resp.Body.String())
			}
			if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"`+tc.code+`"`)) {
				t.Fatalf("expected %s error code, got %s", tc.code, resp.Body.String())
			}
		})
	}
}

func TestMissingOwnedWorkReturns404WithoutRecordNotFoundLog(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	_, cookies := createLoggedInUser(t, testApp, "creator_missing_work", "test-password")

	var logBuffer bytes.Buffer
	db.Logger = logger.New(log.New(&logBuffer, "", 0), logger.Config{
		LogLevel:                  logger.Error,
		IgnoreRecordNotFoundError: false,
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/works/2", nil, cookies)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected missing work 404, got %d: %s", resp.Code, resp.Body.String())
	}
	if bytes.Contains(logBuffer.Bytes(), []byte("record not found")) {
		t.Fatalf("expected missing work lookup not to emit gorm record-not-found log, got %s", logBuffer.String())
	}
}

func TestWorksListSupportsFiltersSortingAndSummary(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_library", "test-password")

	imageWork := seedSucceededWork(t, testApp, owner.ID, "生成小猫钓鱼图片", "1:1")
	posterWork := seedSucceededWork(t, testApp, owner.ID, "宣传片主视觉KV", "16:9")
	productWork := seedSucceededWork(t, testApp, owner.ID, "护肤品精华液主图", "1:1")
	failedFavoriteWork := seedSucceededWork(t, testApp, owner.ID, "失败的收藏草稿", "1:1")

	if err := testApp.db.Model(&Work{}).Where("id = ?", posterWork.ID).Updates(map[string]any{
		"category":   WorkCategoryPosterKV,
		"created_at": time.Now().AddDate(0, 0, -2),
	}).Error; err != nil {
		t.Fatalf("tag poster work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", productWork.ID).Updates(map[string]any{
		"category":   WorkCategoryProductMain,
		"created_at": time.Now().AddDate(0, 0, -10),
	}).Error; err != nil {
		t.Fatalf("tag product work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", imageWork.ID).Update("category", "").Error; err != nil {
		t.Fatalf("clear image category to simulate legacy work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", posterWork.ID).Update("is_favorite", true).Error; err != nil {
		t.Fatalf("favorite poster work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", failedFavoriteWork.ID).Updates(map[string]any{
		"is_favorite": true,
		"status":      GenerationStatusFailed,
	}).Error; err != nil {
		t.Fatalf("favorite failed work: %v", err)
	}

	filteredResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?q=主视觉&category=poster_kv&time_range=week&sort=oldest&page_size=30", nil, ownerCookies)
	if filteredResp.Code != http.StatusOK {
		t.Fatalf("expected filtered works 200, got %d: %s", filteredResp.Code, filteredResp.Body.String())
	}

	var filtered struct {
		Items   []Work `json:"items"`
		Total   int64  `json:"total"`
		Summary struct {
			Total          int64            `json:"total"`
			WeekNew        int64            `json:"week_new"`
			StoredPercent  int              `json:"stored_percent"`
			PrivateCount   int64            `json:"private_count"`
			CategoryCounts map[string]int64 `json:"category_counts"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(filteredResp.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered works: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].ID != posterWork.ID {
		t.Fatalf("expected only poster work, got %+v", filtered)
	}
	if filtered.Items[0].Category != WorkCategoryPosterKV {
		t.Fatalf("expected poster category, got %+v", filtered.Items[0])
	}
	if filtered.Summary.Total != 4 || filtered.Summary.PrivateCount != 4 || filtered.Summary.StoredPercent != 100 {
		t.Fatalf("unexpected summary totals: %+v", filtered.Summary)
	}
	if filtered.Summary.WeekNew != 3 {
		t.Fatalf("expected three works from the last week, got %+v", filtered.Summary)
	}
	if filtered.Summary.CategoryCounts[WorkCategoryImage] != 2 || filtered.Summary.CategoryCounts[WorkCategoryPosterKV] != 1 || filtered.Summary.CategoryCounts[WorkCategoryProductMain] != 1 {
		t.Fatalf("expected category counts including legacy image work, got %+v", filtered.Summary.CategoryCounts)
	}

	imageResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?category=image&page_size=30", nil, ownerCookies)
	if imageResp.Code != http.StatusOK {
		t.Fatalf("expected image category works 200, got %d: %s", imageResp.Code, imageResp.Body.String())
	}
	if !bytes.Contains(imageResp.Body.Bytes(), []byte(`"prompt":"生成小猫钓鱼图片"`)) {
		t.Fatalf("expected legacy image work in image filter: %s", imageResp.Body.String())
	}

	favoriteStatusResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?favorite=true&status=succeeded&page_size=30", nil, ownerCookies)
	if favoriteStatusResp.Code != http.StatusOK {
		t.Fatalf("expected favorite status works 200, got %d: %s", favoriteStatusResp.Code, favoriteStatusResp.Body.String())
	}
	var favoriteStatus struct {
		Items []Work `json:"items"`
		Total int64  `json:"total"`
	}
	if err := json.Unmarshal(favoriteStatusResp.Body.Bytes(), &favoriteStatus); err != nil {
		t.Fatalf("decode favorite status works: %v", err)
	}
	if favoriteStatus.Total != 1 || len(favoriteStatus.Items) != 1 || favoriteStatus.Items[0].ID != posterWork.ID {
		t.Fatalf("expected only succeeded favorite poster, got %+v", favoriteStatus)
	}
	if !favoriteStatus.Items[0].IsFavorite {
		t.Fatalf("expected favorite flag in list item, got %+v", favoriteStatus.Items[0])
	}
}

func TestWorksListSupportsWorkspacePageSizeEighteen(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_workspace_pages", "test-password")
	baseTime := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		work := seedSucceededWork(t, testApp, owner.ID, fmt.Sprintf("分页作品 %02d", i), "1:1")
		if err := testApp.db.Model(&Work{}).Where("id = ?", work.ID).Update("created_at", baseTime.Add(time.Duration(i)*time.Minute)).Error; err != nil {
			t.Fatalf("set work created_at: %v", err)
		}
	}

	var pageOne struct {
		Items    []Work `json:"items"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
		Total    int64  `json:"total"`
	}
	pageOneResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?page=1&page_size=18", nil, ownerCookies)
	if pageOneResp.Code != http.StatusOK {
		t.Fatalf("expected page one works list 200, got %d: %s", pageOneResp.Code, pageOneResp.Body.String())
	}
	if err := json.Unmarshal(pageOneResp.Body.Bytes(), &pageOne); err != nil {
		t.Fatalf("decode page one works: %v", err)
	}
	if pageOne.Page != 1 || pageOne.PageSize != 18 || pageOne.Total != 20 || len(pageOne.Items) != 18 {
		t.Fatalf("unexpected page one pagination payload: %+v", pageOne)
	}
	if pageOne.Items[0].Prompt != "分页作品 19" || pageOne.Items[17].Prompt != "分页作品 02" {
		t.Fatalf("expected page one newest-first ordering, got first=%q last=%q", pageOne.Items[0].Prompt, pageOne.Items[17].Prompt)
	}

	var pageTwo struct {
		Items    []Work `json:"items"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
		Total    int64  `json:"total"`
	}
	pageTwoResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?page=2&page_size=18", nil, ownerCookies)
	if pageTwoResp.Code != http.StatusOK {
		t.Fatalf("expected page two works list 200, got %d: %s", pageTwoResp.Code, pageTwoResp.Body.String())
	}
	if err := json.Unmarshal(pageTwoResp.Body.Bytes(), &pageTwo); err != nil {
		t.Fatalf("decode page two works: %v", err)
	}
	if pageTwo.Page != 2 || pageTwo.PageSize != 18 || pageTwo.Total != 20 || len(pageTwo.Items) != 2 {
		t.Fatalf("unexpected page two pagination payload: %+v", pageTwo)
	}
	if pageTwo.Items[0].Prompt != "分页作品 01" || pageTwo.Items[1].Prompt != "分页作品 00" {
		t.Fatalf("expected page two newest-first ordering, got %+v", pageTwo.Items)
	}
}

func TestWorksListCanExcludeCoupleAlbumPageWorks(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_album_library", "test-password")
	otherUser, _ := createLoggedInUser(t, testApp, "creator_album_other", "test-password")

	regularWork := seedSucceededWork(t, testApp, owner.ID, "普通作品", "1:1")
	albumPageWork := seedSucceededWork(t, testApp, owner.ID, "情侣相册第一页", "3:4")
	otherUserAlbumWork := seedSucceededWork(t, testApp, otherUser.ID, "其他用户相册页", "3:4")

	album := CoupleAlbum{
		UserID:       owner.ID,
		Title:        "情侣旅行相册",
		Location:     "大理",
		Status:       CoupleAlbumStatusSucceeded,
		ShareToken:   "owner-album-token",
		ShareEnabled: false,
	}
	if err := testApp.db.Create(&album).Error; err != nil {
		t.Fatalf("seed couple album: %v", err)
	}
	albumPage := CoupleAlbumPage{
		AlbumID:    album.ID,
		PageNumber: 1,
		PageTitle:  "封面",
		Caption:    "旅途开始",
		Status:     GenerationStatusSucceeded,
		WorkID:     &albumPageWork.ID,
	}
	if err := testApp.db.Create(&albumPage).Error; err != nil {
		t.Fatalf("seed couple album page: %v", err)
	}
	otherAlbum := CoupleAlbum{
		UserID:       otherUser.ID,
		Title:        "其他用户相册",
		Location:     "杭州",
		Status:       CoupleAlbumStatusSucceeded,
		ShareToken:   "other-album-token",
		ShareEnabled: false,
	}
	if err := testApp.db.Create(&otherAlbum).Error; err != nil {
		t.Fatalf("seed other couple album: %v", err)
	}
	otherAlbumPage := CoupleAlbumPage{
		AlbumID:    otherAlbum.ID,
		PageNumber: 1,
		PageTitle:  "封面",
		Caption:    "其他用户相册页",
		Status:     GenerationStatusSucceeded,
		WorkID:     &otherUserAlbumWork.ID,
	}
	if err := testApp.db.Create(&otherAlbumPage).Error; err != nil {
		t.Fatalf("seed other couple album page: %v", err)
	}

	defaultResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?page_size=30", nil, ownerCookies)
	if defaultResp.Code != http.StatusOK {
		t.Fatalf("expected default works list 200, got %d: %s", defaultResp.Code, defaultResp.Body.String())
	}
	var defaultPayload struct {
		Items []Work `json:"items"`
		Total int64  `json:"total"`
	}
	if err := json.Unmarshal(defaultResp.Body.Bytes(), &defaultPayload); err != nil {
		t.Fatalf("decode default works: %v", err)
	}
	if defaultPayload.Total != 2 || !workListContainsID(defaultPayload.Items, regularWork.ID) || !workListContainsID(defaultPayload.Items, albumPageWork.ID) {
		t.Fatalf("expected default list to include regular and album page works, got %+v", defaultPayload)
	}

	filteredResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?exclude_album_pages=true&page_size=30", nil, ownerCookies)
	if filteredResp.Code != http.StatusOK {
		t.Fatalf("expected filtered works list 200, got %d: %s", filteredResp.Code, filteredResp.Body.String())
	}
	var filteredPayload struct {
		Items   []Work `json:"items"`
		Total   int64  `json:"total"`
		Summary struct {
			Total          int64            `json:"total"`
			WeekNew        int64            `json:"week_new"`
			PrivateCount   int64            `json:"private_count"`
			CategoryCounts map[string]int64 `json:"category_counts"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(filteredResp.Body.Bytes(), &filteredPayload); err != nil {
		t.Fatalf("decode filtered works: %v", err)
	}
	if filteredPayload.Total != 1 || len(filteredPayload.Items) != 1 || filteredPayload.Items[0].ID != regularWork.ID {
		t.Fatalf("expected filtered list to hide only current user's album page work, got %+v", filteredPayload)
	}
	if filteredPayload.Summary.Total != 1 || filteredPayload.Summary.PrivateCount != 1 || filteredPayload.Summary.WeekNew != 1 {
		t.Fatalf("expected filtered summary to exclude current user's album page work, got %+v", filteredPayload.Summary)
	}
	if filteredPayload.Summary.CategoryCounts[WorkCategoryImage] != 1 {
		t.Fatalf("expected filtered category counts to exclude current user's album page work, got %+v", filteredPayload.Summary.CategoryCounts)
	}
}

func TestWorksListSupportsAudioCategoryAndRejectsUnknownCategory(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_audio_library", "test-password")

	imageWork := seedSucceededWork(t, testApp, owner.ID, "图片作品", "1:1")
	audioWork := seedSucceededWork(t, testApp, owner.ID, "音效作品", "1:1")
	if err := testApp.db.Model(&Work{}).Where("id = ?", audioWork.ID).Updates(map[string]any{
		"category":  "audio",
		"mime_type": "audio/mpeg",
	}).Error; err != nil {
		t.Fatalf("tag audio work: %v", err)
	}

	audioResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?category=audio&page_size=30", nil, ownerCookies)
	if audioResp.Code != http.StatusOK {
		t.Fatalf("expected audio category works 200, got %d: %s", audioResp.Code, audioResp.Body.String())
	}
	var audioPayload struct {
		Items []Work `json:"items"`
		Total int64  `json:"total"`
	}
	if err := json.Unmarshal(audioResp.Body.Bytes(), &audioPayload); err != nil {
		t.Fatalf("decode audio works: %v", err)
	}
	if audioPayload.Total != 1 || len(audioPayload.Items) != 1 || audioPayload.Items[0].ID != audioWork.ID || audioPayload.Items[0].Category != "audio" {
		t.Fatalf("expected only audio work, got %+v", audioPayload)
	}
	if bytes.Contains(audioResp.Body.Bytes(), []byte(imageWork.Prompt)) {
		t.Fatalf("expected audio filter to exclude image work: %s", audioResp.Body.String())
	}

	invalidResp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?category=document&page_size=30", nil, ownerCookies)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown category 400, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}
	if !bytes.Contains(invalidResp.Body.Bytes(), []byte(`"code":"invalid_work_category"`)) {
		t.Fatalf("expected invalid_work_category payload, got %s", invalidResp.Body.String())
	}
}

func TestWorksListMediaTypeImageIncludesOnlyImageMedia(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_media_type_image", "test-password")

	imageWork := seedSucceededWork(t, testApp, owner.ID, "plain image media", "1:1")
	posterWork := seedSucceededWork(t, testApp, owner.ID, "poster image media", "16:9")
	productWork := seedSucceededWork(t, testApp, owner.ID, "product image media", "1:1")
	coverWork := seedSucceededWork(t, testApp, owner.ID, "cover image media", "3:4")
	legacyImageWork := seedSucceededWork(t, testApp, owner.ID, "legacy empty category image media", "4:3")
	videoWork := seedSucceededWork(t, testApp, owner.ID, "category video media", "16:9")
	audioWork := seedSucceededWork(t, testApp, owner.ID, "category audio media", "1:1")
	legacyMimeVideoWork := seedSucceededWork(t, testApp, owner.ID, "legacy mime video media", "16:9")
	legacyToolModeVideoWork := seedSucceededWork(t, testApp, owner.ID, "legacy tool mode video media", "9:16")

	if err := testApp.db.Model(&Work{}).Where("id = ?", posterWork.ID).Update("category", WorkCategoryPosterKV).Error; err != nil {
		t.Fatalf("tag poster work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", productWork.ID).Update("category", WorkCategoryProductMain).Error; err != nil {
		t.Fatalf("tag product work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", coverWork.ID).Update("category", WorkCategoryCover).Error; err != nil {
		t.Fatalf("tag cover work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", legacyImageWork.ID).Update("category", "").Error; err != nil {
		t.Fatalf("clear legacy image category: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", videoWork.ID).Updates(map[string]any{
		"category":  WorkCategoryVideo,
		"mime_type": "video/mp4",
	}).Error; err != nil {
		t.Fatalf("tag category video work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", audioWork.ID).Updates(map[string]any{
		"category":  WorkCategoryAudio,
		"mime_type": "audio/mpeg",
	}).Error; err != nil {
		t.Fatalf("tag category audio work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", legacyMimeVideoWork.ID).Updates(map[string]any{
		"category":  "",
		"mime_type": "video/mp4",
	}).Error; err != nil {
		t.Fatalf("tag legacy mime video work: %v", err)
	}
	if err := testApp.db.Model(&Work{}).Where("id = ?", legacyToolModeVideoWork.ID).Update("category", "").Error; err != nil {
		t.Fatalf("clear legacy tool mode video work category: %v", err)
	}
	if err := testApp.db.Model(&GenerationRecord{}).Where("id = ?", legacyToolModeVideoWork.GenerationRecordID).Update("tool_mode", "video").Error; err != nil {
		t.Fatalf("tag legacy video generation record: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/works?media_type=image&page_size=30", nil, ownerCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected image media works 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []Work `json:"items"`
		Total int64  `json:"total"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode image media works: %v", err)
	}
	expectedIDs := []uint{imageWork.ID, posterWork.ID, productWork.ID, coverWork.ID, legacyImageWork.ID}
	if payload.Total != int64(len(expectedIDs)) || len(payload.Items) != len(expectedIDs) {
		t.Fatalf("expected only image media works, got total=%d items=%+v body=%s", payload.Total, payload.Items, resp.Body.String())
	}
	for _, id := range expectedIDs {
		if !workListContainsID(payload.Items, id) {
			t.Fatalf("expected image media work %d in payload, got %+v", id, payload.Items)
		}
	}
	for _, id := range []uint{videoWork.ID, audioWork.ID, legacyMimeVideoWork.ID, legacyToolModeVideoWork.ID} {
		if workListContainsID(payload.Items, id) {
			t.Fatalf("expected non-image media work %d to be excluded, got %+v", id, payload.Items)
		}
	}
}

func workListContainsID(items []Work, id uint) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestPackagesAndAdminTopUpFlow(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_pay", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	setUserPhoneForTest(t, testApp, user.ID, "13800139301")
	adminCookies := createAdminSession(t, testApp)

	packagesResp := performJSONRequest(t, testApp, http.MethodGet, "/api/packages", nil, nil)
	if packagesResp.Code != http.StatusOK {
		t.Fatalf("expected packages 200, got %d: %s", packagesResp.Code, packagesResp.Body.String())
	}

	var packageList struct {
		Items []Package `json:"items"`
	}
	if err := json.Unmarshal(packagesResp.Body.Bytes(), &packageList); err != nil {
		t.Fatalf("decode packages: %v", err)
	}
	if len(packageList.Items) == 0 {
		t.Fatal("expected seeded packages")
	}

	topUpResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/credits", map[string]any{
		"amount": 30,
		"note":   "manual recharge",
	}, adminCookies)
	if topUpResp.Code != http.StatusOK {
		t.Fatalf("expected top up 200, got %d: %s", topUpResp.Code, topUpResp.Body.String())
	}

	creditsResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credits", nil, cookies)
	if creditsResp.Code != http.StatusOK {
		t.Fatalf("expected credits 200, got %d: %s", creditsResp.Code, creditsResp.Body.String())
	}
	if !bytes.Contains(creditsResp.Body.Bytes(), []byte(`"available_credits":30`)) {
		t.Fatalf("expected available_credits 30, got %s", creditsResp.Body.String())
	}

	transactionsResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credit-transactions", nil, cookies)
	if transactionsResp.Code != http.StatusOK {
		t.Fatalf("expected transactions 200, got %d: %s", transactionsResp.Code, transactionsResp.Body.String())
	}
	if !bytes.Contains(transactionsResp.Body.Bytes(), []byte(`"type":"manual_topup"`)) {
		t.Fatalf("expected manual_topup transaction, got %s", transactionsResp.Body.String())
	}
}

func TestAccountCreditsReturnsCreditSummary(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_credit_summary", "test-password")
	setUserCredits(t, testApp, user.ID, 22)

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	transactions := []CreditTransaction{
		{UserID: user.ID, Type: CreditTransactionTypePaymentTopUp, Amount: 50, BalanceAfter: 50, Reason: "payment", CreatedAt: monthStart.AddDate(0, -1, 0), UpdatedAt: monthStart.AddDate(0, -1, 0)},
		{UserID: user.ID, Type: CreditTransactionTypeGenerationCharge, Amount: -8, BalanceAfter: 42, Reason: "this month generation", CreatedAt: monthStart.Add(2 * time.Hour), UpdatedAt: monthStart.Add(2 * time.Hour)},
		{UserID: user.ID, Type: CreditTransactionTypeManualDeduct, Amount: -5, BalanceAfter: 37, Reason: "this month deduct", CreatedAt: monthStart.Add(3 * time.Hour), UpdatedAt: monthStart.Add(3 * time.Hour)},
		{UserID: user.ID, Type: CreditTransactionTypeGenerationCharge, Amount: -9, BalanceAfter: 31, Reason: "old generation", CreatedAt: monthStart.AddDate(0, -2, 0), UpdatedAt: monthStart.AddDate(0, -2, 0)},
	}
	if err := db.Create(&transactions).Error; err != nil {
		t.Fatalf("create credit summary transactions: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credits", nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected credits summary 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		UserID              uint   `json:"user_id"`
		AvailableCredits    int    `json:"available_credits"`
		MonthlyConsumption  int    `json:"monthly_consumption"`
		TotalRecharged      int    `json:"total_recharged"`
		LatestTransactionAt string `json:"latest_transaction_at"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode credits summary: %v", err)
	}
	if payload.UserID != user.ID || payload.AvailableCredits != 22 {
		t.Fatalf("expected user credits fields, got %+v", payload)
	}
	if payload.MonthlyConsumption != 13 || payload.TotalRecharged != signupBonusCredits+50 {
		t.Fatalf("expected monthly consumption 13 and total recharged %d, got %+v", signupBonusCredits+50, payload)
	}
	if payload.LatestTransactionAt == "" {
		t.Fatalf("expected latest transaction timestamp, got %+v", payload)
	}
}

func TestAccountCreditTransactionsSupportPaginationAndKindFilters(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_credit_flow", "test-password")
	otherUser, _ := createLoggedInUser(t, testApp, "creator_credit_other", "test-password")

	baseTime := time.Now().Add(-time.Hour).UTC()
	transactions := []CreditTransaction{
		{UserID: user.ID, Type: CreditTransactionTypeManualTopUp, Amount: 30, BalanceAfter: 30, Reason: "first recharge", CreatedAt: baseTime.Add(1 * time.Minute), UpdatedAt: baseTime.Add(1 * time.Minute)},
		{UserID: user.ID, Type: CreditTransactionTypeGenerationCharge, Amount: -3, BalanceAfter: 27, Reason: "generation", CreatedAt: baseTime.Add(2 * time.Minute), UpdatedAt: baseTime.Add(2 * time.Minute)},
		{UserID: user.ID, Type: CreditTransactionTypeManualTopUp, Amount: 12, BalanceAfter: 39, Reason: "second recharge", CreatedAt: baseTime.Add(3 * time.Minute), UpdatedAt: baseTime.Add(3 * time.Minute)},
		{UserID: user.ID, Type: CreditTransactionTypePromptTemplateUse, Amount: -1, BalanceAfter: 38, Reason: "template", CreatedAt: baseTime.Add(4 * time.Minute), UpdatedAt: baseTime.Add(4 * time.Minute)},
		{UserID: otherUser.ID, Type: CreditTransactionTypeManualTopUp, Amount: 99, BalanceAfter: 99, Reason: "other user", CreatedAt: baseTime.Add(5 * time.Minute), UpdatedAt: baseTime.Add(5 * time.Minute)},
	}
	if err := db.Create(&transactions).Error; err != nil {
		t.Fatalf("create credit transactions: %v", err)
	}

	pageResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credit-transactions?page=1&page_size=2", nil, cookies)
	if pageResp.Code != http.StatusOK {
		t.Fatalf("expected paged transactions 200, got %d: %s", pageResp.Code, pageResp.Body.String())
	}
	var pagePayload struct {
		Items    []CreditTransaction `json:"items"`
		Total    int64               `json:"total"`
		Page     int                 `json:"page"`
		PageSize int                 `json:"page_size"`
		HasMore  bool                `json:"has_more"`
	}
	if err := json.Unmarshal(pageResp.Body.Bytes(), &pagePayload); err != nil {
		t.Fatalf("decode paged transactions: %v", err)
	}
	if pagePayload.Total != 5 || pagePayload.Page != 1 || pagePayload.PageSize != 2 || !pagePayload.HasMore {
		t.Fatalf("expected page metadata total=5 page=1 page_size=2 has_more=true, got %+v", pagePayload)
	}
	if len(pagePayload.Items) != 2 ||
		pagePayload.Items[0].Type != CreditTransactionTypeSignupBonus ||
		pagePayload.Items[1].Amount != -1 {
		t.Fatalf("expected latest two current-user transactions, got %+v", pagePayload.Items)
	}

	rechargeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credit-transactions?kind=recharge&page=1&page_size=10", nil, cookies)
	if rechargeResp.Code != http.StatusOK {
		t.Fatalf("expected recharge transactions 200, got %d: %s", rechargeResp.Code, rechargeResp.Body.String())
	}
	var rechargePayload struct {
		Items   []CreditTransaction `json:"items"`
		Total   int64               `json:"total"`
		HasMore bool                `json:"has_more"`
	}
	if err := json.Unmarshal(rechargeResp.Body.Bytes(), &rechargePayload); err != nil {
		t.Fatalf("decode recharge transactions: %v", err)
	}
	if rechargePayload.Total != 3 || rechargePayload.HasMore || len(rechargePayload.Items) != 3 {
		t.Fatalf("expected three recharge transactions without more pages, got %+v", rechargePayload)
	}
	for _, item := range rechargePayload.Items {
		if item.Amount <= 0 || item.UserID != user.ID {
			t.Fatalf("expected only current-user positive transactions, got %+v", rechargePayload.Items)
		}
	}

	consumeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credit-transactions?kind=consume&page=1&page_size=10", nil, cookies)
	if consumeResp.Code != http.StatusOK {
		t.Fatalf("expected consume transactions 200, got %d: %s", consumeResp.Code, consumeResp.Body.String())
	}
	var consumePayload struct {
		Items []CreditTransaction `json:"items"`
		Total int64               `json:"total"`
	}
	if err := json.Unmarshal(consumeResp.Body.Bytes(), &consumePayload); err != nil {
		t.Fatalf("decode consume transactions: %v", err)
	}
	if consumePayload.Total != 2 || len(consumePayload.Items) != 2 {
		t.Fatalf("expected two consume transactions, got %+v", consumePayload)
	}
	for _, item := range consumePayload.Items {
		if item.Amount >= 0 || item.UserID != user.ID {
			t.Fatalf("expected only current-user negative transactions, got %+v", consumePayload.Items)
		}
	}

	unauthorizedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/credit-transactions?page=1&page_size=2", nil, nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated transactions 401, got %d: %s", unauthorizedResp.Code, unauthorizedResp.Body.String())
	}
}

func TestPurchaseIntentEndpointsAreDisabled(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "intent_disabled", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139302")
	adminCookies := createAdminSession(t, testApp)
	endpoints := []struct {
		method  string
		path    string
		body    map[string]any
		cookies []*http.Cookie
	}{
		{http.MethodPost, "/api/purchase-intents", map[string]any{"package_id": 1}, cookies},
		{http.MethodGet, "/api/admin/purchase-intents", nil, adminCookies},
		{http.MethodPut, "/api/admin/purchase-intents/1", map[string]any{"status": "completed"}, adminCookies},
	}
	for _, endpoint := range endpoints {
		resp := performJSONRequest(t, testApp, endpoint.method, endpoint.path, endpoint.body, endpoint.cookies)
		if resp.Code != http.StatusGone || !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"purchase_intents_disabled"`)) {
			t.Fatalf("expected disabled purchase intent endpoint for %s %s, got %d: %s", endpoint.method, endpoint.path, resp.Code, resp.Body.String())
		}
	}
}

func TestBackfillCompletedPurchaseIntentsCreatesFinanceOrders(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	now := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	intent := PurchaseIntent{
		UserID:         1,
		PackageID:      2,
		PackageName:    "历史套餐",
		PackageCredits: 60,
		PackagePrice:   "99 元",
		CustomerName:   "历史客户",
		Status:         PurchaseIntentStatusCompleted,
		ConvertedAt:    &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&intent).Error; err != nil {
		t.Fatalf("seed completed intent: %v", err)
	}

	if err := testApp.backfillFinanceOrders(); err != nil {
		t.Fatalf("backfill finance orders: %v", err)
	}
	if err := testApp.backfillFinanceOrders(); err != nil {
		t.Fatalf("second backfill finance orders: %v", err)
	}

	var count int64
	if err := db.Model(&FinanceOrder{}).Where("purchase_intent_id = ?", intent.ID).Count(&count).Error; err != nil {
		t.Fatalf("count finance orders: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one backfilled finance order, got %d", count)
	}
}

func TestAdminFinanceOrdersSupportFiltersKPIExportAndStatusUpdates(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	now := time.Now().Truncate(time.Second)
	user := User{Username: "finance-customer", DisplayName: "财务客户", Email: "finance@example.com", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	paidAt := now
	order := FinanceOrder{
		OrderNumber:    "FO-TEST-001",
		UserID:         user.ID,
		PackageID:      8,
		PackageName:    "团队包",
		PackageCredits: 320,
		AmountCents:    39900,
		OrderType:      FinanceOrderTypePackage,
		PaymentMethod:  FinancePaymentMethodOffline,
		PaymentStatus:  FinancePaymentStatusPaid,
		InvoiceStatus:  FinanceInvoiceStatusPending,
		PaidAt:         &paidAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed finance order: %v", err)
	}
	refund := FinanceRefund{
		RefundNumber:   "FR-TEST-001",
		FinanceOrderID: order.ID,
		AmountCents:    9900,
		Reason:         "客户申请退款",
		Status:         FinanceRefundStatusPending,
		RequestedAt:    now,
	}
	if err := db.Create(&refund).Error; err != nil {
		t.Fatalf("seed finance refund: %v", err)
	}
	invoice := FinanceInvoice{
		InvoiceNumber:  "FI-TEST-001",
		FinanceOrderID: order.ID,
		AmountCents:    order.AmountCents,
		Title:          "财务客户",
		Status:         FinanceInvoiceStatusPending,
	}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatalf("seed finance invoice: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders?type=package&payment_status=paid&q=finance-customer&page=1&page_size=5", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected finance orders 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID            uint   `json:"id"`
			OrderNumber   string `json:"order_number"`
			AmountCents   int64  `json:"amount_cents"`
			PaymentStatus string `json:"payment_status"`
			InvoiceStatus string `json:"invoice_status"`
			User          struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"items"`
		KPIs struct {
			TodayRevenueCents int64 `json:"today_revenue_cents"`
			MonthRevenueCents int64 `json:"month_revenue_cents"`
			PendingOrders     int64 `json:"pending_orders"`
			RefundingCount    int64 `json:"refunding_count"`
		} `json:"kpis"`
		Trend []struct {
			Date         string `json:"date"`
			RevenueCents int64  `json:"revenue_cents"`
			OrderCount   int64  `json:"order_count"`
		} `json:"trend"`
		RefundOverview struct {
			PendingCount int64 `json:"pending_count"`
		} `json:"refund_overview"`
		InvoiceOverview struct {
			PendingCount int64 `json:"pending_count"`
		} `json:"invoice_overview"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode finance orders: %v", err)
	}
	if payload.Total != 1 || payload.Page != 1 || payload.PageSize != 5 || len(payload.Items) != 1 {
		t.Fatalf("unexpected finance order pagination: %+v", payload)
	}
	if payload.Items[0].OrderNumber != order.OrderNumber || payload.Items[0].AmountCents != order.AmountCents ||
		payload.Items[0].User.Username != user.Username {
		t.Fatalf("unexpected finance order item: %+v", payload.Items[0])
	}
	if payload.KPIs.TodayRevenueCents != order.AmountCents || payload.KPIs.MonthRevenueCents != order.AmountCents ||
		payload.KPIs.RefundingCount != 1 || len(payload.Trend) != 30 ||
		payload.RefundOverview.PendingCount != 1 || payload.InvoiceOverview.PendingCount != 1 {
		t.Fatalf("unexpected finance summary: %+v", payload)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders/"+itoa(order.ID), nil, adminCookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	if !bytes.Contains(detailResp.Body.Bytes(), []byte(`"refund_number":"FR-TEST-001"`)) {
		t.Fatalf("expected refund in detail: %s", detailResp.Body.String())
	}

	refundResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/finance-refunds/"+itoa(refund.ID), map[string]any{
		"status": "completed",
	}, adminCookies)
	if refundResp.Code != http.StatusOK {
		t.Fatalf("expected refund update 200, got %d: %s", refundResp.Code, refundResp.Body.String())
	}
	var updatedRefund FinanceRefund
	if err := db.First(&updatedRefund, refund.ID).Error; err != nil {
		t.Fatalf("load updated refund: %v", err)
	}
	if updatedRefund.Status != FinanceRefundStatusCompleted || updatedRefund.ProcessedAt == nil {
		t.Fatalf("unexpected updated refund: %+v", updatedRefund)
	}

	invoiceResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/finance-invoices/"+itoa(invoice.ID), map[string]any{
		"status": "issued",
	}, adminCookies)
	if invoiceResp.Code != http.StatusOK {
		t.Fatalf("expected invoice update 200, got %d: %s", invoiceResp.Code, invoiceResp.Body.String())
	}
	var updatedInvoice FinanceInvoice
	if err := db.First(&updatedInvoice, invoice.ID).Error; err != nil {
		t.Fatalf("load updated invoice: %v", err)
	}
	if updatedInvoice.Status != FinanceInvoiceStatusIssued || updatedInvoice.IssuedAt == nil {
		t.Fatalf("unexpected updated invoice: %+v", updatedInvoice)
	}
	var updatedOrder FinanceOrder
	if err := db.First(&updatedOrder, order.ID).Error; err != nil {
		t.Fatalf("load updated order: %v", err)
	}
	if updatedOrder.InvoiceStatus != FinanceInvoiceStatusIssued {
		t.Fatalf("expected order invoice status to follow invoice, got %+v", updatedOrder)
	}

	invalidRefundResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/finance-refunds/"+itoa(refund.ID), map[string]any{
		"status": "unknown",
	}, adminCookies)
	if invalidRefundResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid refund status 400, got %d: %s", invalidRefundResp.Code, invalidRefundResp.Body.String())
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders/export?q=finance-customer", nil, adminCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if disposition := exportResp.Header().Get("Content-Disposition"); !strings.Contains(disposition, "finance-orders.csv") {
		t.Fatalf("expected finance csv disposition, got %q", disposition)
	}
	if !bytes.Contains(exportResp.Body.Bytes(), []byte("FO-TEST-001")) {
		t.Fatalf("expected order in csv export: %s", exportResp.Body.String())
	}
}

func TestAdminFinanceOrdersRequireReadPermission(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	unauthorizedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders", nil, nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized finance orders 401, got %d: %s", unauthorizedResp.Code, unauthorizedResp.Body.String())
	}

	admin := createDatabaseAdminUser(t, db, "finance-limited", "LimitedPass123")
	role := Role{Code: "finance_limited_empty", Name: "Finance Limited Empty", Status: RoleStatusActive}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create limited role: %v", err)
	}
	if err := db.Model(&admin).Association("Roles").Append(&role); err != nil {
		t.Fatalf("assign limited role: %v", err)
	}
	cookies := loginAdminAs(t, testApp, "finance-limited", "LimitedPass123")

	allowedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders", nil, cookies)
	if allowedResp.Code != http.StatusForbidden {
		t.Fatalf("expected finance orders without permission 403, got %d: %s", allowedResp.Code, allowedResp.Body.String())
	}

	assignAdminRoleByCode(t, db, &admin, "finance")
	allowedCookies := loginAdminAs(t, testApp, "finance-limited", "LimitedPass123")
	allowedResp = performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders", nil, allowedCookies)
	if allowedResp.Code != http.StatusOK {
		t.Fatalf("expected finance orders with finance role 200, got %d: %s", allowedResp.Code, allowedResp.Body.String())
	}
}

func TestAdminFinanceOrderSyncPaymentQueriesAlipayAndCreditsOnce(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, userCookies := createLoggedInUser(t, testApp, "finance-sync-buyer", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	privateKey, publicKey := generateAlipayKeyPair(t)
	testApp.cfg.AlipayAppID = "app-finance-sync"
	testApp.cfg.AlipayPrivateKey = privateKey
	testApp.cfg.AlipayPublicKey = publicKey
	testApp.cfg.AlipayGateway = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	orderNumber := createTestAlipayOrder(t, testApp, userCookies)

	var order FinanceOrder
	if err := db.Where("order_number = ?", orderNumber).First(&order).Error; err != nil {
		t.Fatalf("load pending order: %v", err)
	}
	testApp.alipayQuerier = fakeAlipayQuerier{
		result: alipayTradeQueryResult{
			OutTradeNo:  orderNumber,
			TradeNo:     "2026070122000000000099",
			BuyerID:     "2088000000000099",
			TradeStatus: "TRADE_SUCCESS",
			TotalAmount: formatAlipayAmount(order.AmountCents),
		},
	}

	syncResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/finance-orders/"+itoa(order.ID)+"/sync-payment", nil, adminCookies)
	if syncResp.Code != http.StatusOK {
		t.Fatalf("expected sync payment 200, got %d: %s", syncResp.Code, syncResp.Body.String())
	}
	if !bytes.Contains(syncResp.Body.Bytes(), []byte(`"payment_status":"paid"`)) ||
		!bytes.Contains(syncResp.Body.Bytes(), []byte(`"available_credits":`)) {
		t.Fatalf("expected paid sync response with balance: %s", syncResp.Body.String())
	}

	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != order.PackageCredits {
		t.Fatalf("expected credited balance %d, got %d", order.PackageCredits, balance.AvailableCredits)
	}
	var transactionCount int64
	if err := db.Model(&CreditTransaction{}).
		Where("user_id = ? AND type = ? AND related_type = ? AND related_id = ?", user.ID, CreditTransactionTypePaymentTopUp, "finance_order", order.ID).
		Count(&transactionCount).Error; err != nil {
		t.Fatalf("count payment topup: %v", err)
	}
	if transactionCount != 1 {
		t.Fatalf("expected one payment topup after sync, got %d", transactionCount)
	}
	var payment PaymentRecord
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("load payment record after sync: %v", err)
	}
	if payment.Status != PaymentRecordStatusPaid || payment.QueryCount != 1 || payment.NotifyCount != 0 || payment.LastEvent != "admin_sync" {
		t.Fatalf("expected admin sync payment record query event, got status=%s query=%d notify=%d last=%s", payment.Status, payment.QueryCount, payment.NotifyCount, payment.LastEvent)
	}

	repeatResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/finance-orders/"+itoa(order.ID)+"/sync-payment", nil, adminCookies)
	if repeatResp.Code != http.StatusOK {
		t.Fatalf("expected repeat sync payment 200, got %d: %s", repeatResp.Code, repeatResp.Body.String())
	}
	if err := db.Model(&CreditTransaction{}).
		Where("user_id = ? AND type = ? AND related_type = ? AND related_id = ?", user.ID, CreditTransactionTypePaymentTopUp, "finance_order", order.ID).
		Count(&transactionCount).Error; err != nil {
		t.Fatalf("count repeated payment topup: %v", err)
	}
	if transactionCount != 1 {
		t.Fatalf("expected repeat sync to keep one payment topup, got %d", transactionCount)
	}
	if err := db.Where("finance_order_id = ?", order.ID).First(&payment).Error; err != nil {
		t.Fatalf("reload payment record after repeat sync: %v", err)
	}
	if payment.QueryCount != 1 || payment.NotifyCount != 0 {
		t.Fatalf("expected repeat sync to skip duplicate query bookkeeping after paid, got query=%d notify=%d", payment.QueryCount, payment.NotifyCount)
	}
}

func TestAdminInvitesSupportBatchFiltersRedemptionsAndExport(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	expiresAt := time.Now().AddDate(0, 0, 14).UTC().Truncate(time.Second)

	batchResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/invites/batch", map[string]any{
		"prefix":      "OPS",
		"quantity":    3,
		"expires_at":  expiresAt,
		"total_quota": 2,
	}, adminCookies)
	if batchResp.Code != http.StatusOK {
		t.Fatalf("expected batch create 200, got %d: %s", batchResp.Code, batchResp.Body.String())
	}
	var batchPayload struct {
		Items []Invite `json:"items"`
	}
	if err := json.Unmarshal(batchResp.Body.Bytes(), &batchPayload); err != nil {
		t.Fatalf("decode batch payload: %v", err)
	}
	if len(batchPayload.Items) != 3 {
		t.Fatalf("expected three invites, got %+v", batchPayload.Items)
	}
	seenCodes := map[string]bool{}
	for _, invite := range batchPayload.Items {
		if !strings.HasPrefix(invite.Code, "OPS-") || len(strings.Split(invite.Code, "-")) != 3 {
			t.Fatalf("expected OPS-XXXX-XXXX code, got %q", invite.Code)
		}
		if invite.TotalQuota != 2 || invite.Status != InviteStatusActive || invite.ExpiresAt == nil {
			t.Fatalf("unexpected batch invite defaults: %+v", invite)
		}
		if seenCodes[invite.Code] {
			t.Fatalf("expected unique invite codes, got duplicate %q", invite.Code)
		}
		seenCodes[invite.Code] = true
	}

	createSMSVerificationCodeForTest(t, testApp, "13800139006", smsPurposeRegister, "123456")
	registerResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13800139006",
		"verification_code": "123456",
		"username":          "invited_user",
		"password":          "test-password",
		"invite_code":       batchPayload.Items[0].Code,
	}, nil)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("expected invited phone register 201, got %d: %s", registerResp.Code, registerResp.Body.String())
	}
	var registerPayload struct {
		AvailableCredits int `json:"available_credits"`
	}
	if err := json.Unmarshal(registerResp.Body.Bytes(), &registerPayload); err != nil {
		t.Fatalf("decode invited register payload: %v", err)
	}
	if registerPayload.AvailableCredits != 5 {
		t.Fatalf("expected invited register signup bonus credits 5, got %s", registerResp.Body.String())
	}

	var invited User
	if err := db.Where("username = ?", "invited_user").First(&invited).Error; err != nil {
		t.Fatalf("load invited user: %v", err)
	}
	if invited.Phone == nil || *invited.Phone != "13800139006" {
		t.Fatalf("expected register to persist phone, got %+v", invited)
	}
	var redeemed Invite
	if err := db.First(&redeemed, batchPayload.Items[0].ID).Error; err != nil {
		t.Fatalf("load redeemed invite: %v", err)
	}
	if redeemed.UsedQuota != 1 {
		t.Fatalf("expected invite used quota 1, got %+v", redeemed)
	}
	var redemption InviteRedemption
	if err := db.Where("invite_code = ? AND user_id = ?", batchPayload.Items[0].Code, invited.ID).First(&redemption).Error; err != nil {
		t.Fatalf("load invite redemption: %v", err)
	}
	if redemption.Username != "invited_user" || redemption.DisplayName != "invited_user" || redemption.Email != "" || redemption.RegisteredAt.IsZero() {
		t.Fatalf("expected redemption snapshot fields, got %+v", redemption)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", invited.ID).First(&balance).Error; err != nil {
		t.Fatalf("load invited user credit balance: %v", err)
	}
	if balance.AvailableCredits != 5 {
		t.Fatalf("expected invited user signup bonus balance 5, got %+v", balance)
	}
	assertSignupBonusTransaction(t, db, invited.ID)

	completedAt := time.Now()
	if err := db.Create(&FinanceOrder{
		OrderNumber:    "FO-INVITE-CONVERTED",
		UserID:         invited.ID,
		PackageID:      1,
		PackageName:    "邀请转化包",
		PackageCredits: 50,
		AmountCents:    1000,
		OrderType:      FinanceOrderTypePackage,
		PaymentMethod:  FinancePaymentMethodOffline,
		PaymentStatus:  FinancePaymentStatusPaid,
		InvoiceStatus:  FinanceInvoiceStatusPending,
		PaidAt:         &completedAt,
		CreatedAt:      completedAt,
		UpdatedAt:      completedAt,
	}).Error; err != nil {
		t.Fatalf("seed paid finance order: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/invites?q=OPS&status=partial&page=1&page_size=2", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected invite list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items   []Invite `json:"items"`
		Summary struct {
			AvailableInvites                 int64   `json:"available_invites"`
			AvailableInvitesDeltaPercent     float64 `json:"available_invites_delta_percent"`
			UsedInvites                      int64   `json:"used_invites"`
			UsedInvitesDeltaPercent          float64 `json:"used_invites_delta_percent"`
			TodayNewInviteUsers              int64   `json:"today_new_invite_users"`
			TodayNewInviteUsersDeltaPercent  float64 `json:"today_new_invite_users_delta_percent"`
			InviteConversionRate             int64   `json:"invite_conversion_rate"`
			InviteConversionRateDeltaPercent float64 `json:"invite_conversion_rate_delta_percent"`
		} `json:"summary"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode invite list: %v", err)
	}
	if listPayload.Total != 1 || listPayload.Page != 1 || listPayload.PageSize != 2 || len(listPayload.Items) != 1 {
		t.Fatalf("unexpected invite pagination payload: %+v", listPayload)
	}
	if listPayload.Items[0].Code != batchPayload.Items[0].Code {
		t.Fatalf("expected partial invite in filtered list, got %+v", listPayload.Items)
	}
	if listPayload.Summary.AvailableInvites != 3 || listPayload.Summary.UsedInvites != 1 ||
		listPayload.Summary.TodayNewInviteUsers != 1 || listPayload.Summary.InviteConversionRate != 100 {
		t.Fatalf("unexpected invite summary: %+v", listPayload.Summary)
	}

	redemptionsResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/invite-redemptions?result=converted&page=1&page_size=10", nil, adminCookies)
	if redemptionsResp.Code != http.StatusOK {
		t.Fatalf("expected redemptions 200, got %d: %s", redemptionsResp.Code, redemptionsResp.Body.String())
	}
	var redemptionsPayload struct {
		Items []struct {
			InviteCode       string `json:"invite_code"`
			Username         string `json:"username"`
			DisplayName      string `json:"display_name"`
			Email            string `json:"email"`
			ConversionResult string `json:"conversion_result"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(redemptionsResp.Body.Bytes(), &redemptionsPayload); err != nil {
		t.Fatalf("decode redemptions: %v", err)
	}
	if redemptionsPayload.Total != 1 || len(redemptionsPayload.Items) != 1 ||
		redemptionsPayload.Items[0].InviteCode != batchPayload.Items[0].Code ||
		redemptionsPayload.Items[0].Username != "invited_user" ||
		redemptionsPayload.Items[0].DisplayName != "invited_user" ||
		redemptionsPayload.Items[0].Email != "" ||
		redemptionsPayload.Items[0].ConversionResult != "converted" {
		t.Fatalf("unexpected redemption payload: %+v", redemptionsPayload)
	}

	invitesCSV := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/invites/export?status=partial&q=OPS", nil, adminCookies)
	if invitesCSV.Code != http.StatusOK || !bytes.Contains(invitesCSV.Body.Bytes(), []byte(batchPayload.Items[0].Code)) {
		t.Fatalf("expected invite CSV export with code, got %d: %s", invitesCSV.Code, invitesCSV.Body.String())
	}
	redemptionsCSV := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/invite-redemptions/export?result=converted", nil, adminCookies)
	if redemptionsCSV.Code != http.StatusOK || !bytes.Contains(redemptionsCSV.Body.Bytes(), []byte("invited_user")) {
		t.Fatalf("expected redemption CSV export with user, got %d: %s", redemptionsCSV.Code, redemptionsCSV.Body.String())
	}
}

func TestPhoneRegisterRejectsUnavailableInviteCodes(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	expiredAt := time.Now().AddDate(0, 0, -1)
	invites := []Invite{
		{Code: "DISABLED-0001", Status: InviteStatusDisabled, TotalQuota: 1},
		{Code: "EXPIRED-0001", Status: InviteStatusActive, TotalQuota: 1, ExpiresAt: &expiredAt},
		{Code: "USED-0001", Status: InviteStatusActive, TotalQuota: 1, UsedQuota: 1},
	}
	if err := db.Create(&invites).Error; err != nil {
		t.Fatalf("seed unavailable invites: %v", err)
	}

	cases := []struct {
		name       string
		code       string
		wantStatus int
		wantCode   string
	}{
		{name: "missing", code: "NO-SUCH-CODE", wantStatus: http.StatusBadRequest, wantCode: "invite_not_found"},
		{name: "disabled", code: "DISABLED-0001", wantStatus: http.StatusBadRequest, wantCode: "invite_disabled"},
		{name: "expired", code: "EXPIRED-0001", wantStatus: http.StatusBadRequest, wantCode: "invite_expired"},
		{name: "exhausted", code: "USED-0001", wantStatus: http.StatusBadRequest, wantCode: "invite_quota_exhausted"},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phone := fmt.Sprintf("13800139%03d", 100+index)
			createSMSVerificationCodeForTest(t, testApp, phone, smsPurposeRegister, "123456")
			resp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
				"phone":             phone,
				"verification_code": "123456",
				"username":          fmt.Sprintf("blocked_invite_%d", index),
				"password":          "test-password",
				"invite_code":       tc.code,
			}, nil)
			if resp.Code != tc.wantStatus {
				t.Fatalf("expected phone register %d, got %d: %s", tc.wantStatus, resp.Code, resp.Body.String())
			}
			if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":"`+tc.wantCode+`"`)) {
				t.Fatalf("expected error code %q, got %s", tc.wantCode, resp.Body.String())
			}
		})
	}
}

func TestAdminPackagesSupportExtendedCRUDSummaryAndSoftDelete(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "package_buyer", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139304")

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/packages", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected admin packages 200, got %d: %s", listResp.Code, listResp.Body.String())
	}

	var listPayload struct {
		Items   []Package `json:"items"`
		Summary struct {
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
		} `json:"summary"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode admin packages: %v", err)
	}
	if len(listPayload.Items) != 6 {
		t.Fatalf("expected seeded admin packages, got %+v", listPayload.Items)
	}
	if listPayload.Items[0].PriceCents <= 0 || listPayload.Items[0].ValidDays <= 0 || len(listPayload.Items[0].Tags) == 0 {
		t.Fatalf("expected extended package fields, got %+v", listPayload.Items[0])
	}
	if listPayload.Summary.ActivePackages != 6 || len(listPayload.Summary.ActivePackagesSparkline) != 7 ||
		len(listPayload.Summary.RevenueShareSparkline) != 7 ||
		len(listPayload.Summary.AverageOrderSparkline) != 7 ||
		len(listPayload.Summary.MonthlyOrdersSparkline) != 7 {
		t.Fatalf("unexpected package summary: %+v", listPayload.Summary)
	}

	invalidResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/packages", map[string]any{
		"name":        "无效套餐",
		"price_cents": 0,
		"credits":     0,
		"valid_days":  0,
	}, adminCookies)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid package 400, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/packages", map[string]any{
		"name":        "商业加速包",
		"description": "面向商业海报和产品主图的短周期加速包",
		"price_cents": 12800,
		"credits":     88,
		"valid_days":  45,
		"audience":    "商业创作者",
		"tags":        []string{"商用", "加急"},
		"icon":        "rocket",
		"theme":       "teal",
		"badge":       "商用推荐",
		"recommended": true,
		"features":    []string{"商用授权", "加急排队", "高清下载"},
		"benefits": []map[string]any{
			{"label": "点数", "value": "88 点"},
			{"label": "商用授权", "value": "支持"},
			{"label": "专属权益", "value": "加急排队"},
		},
		"is_active": false,
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected package create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}

	var created Package
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created package: %v", err)
	}
	if created.ID == 0 || created.PriceCents != 12800 || created.PriceLabel == "" || created.ValidDays != 45 ||
		created.Audience != "商业创作者" || len(created.Tags) != 2 || created.IsActive ||
		created.Badge != "商用推荐" || !created.Recommended || len(created.Features) != 3 || len(created.Benefits) != 3 {
		t.Fatalf("unexpected created package: %+v", created)
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/packages/"+itoa(created.ID), map[string]any{
		"is_active": true,
		"features":  []string{"团队协作", "商用授权"},
		"benefits": []map[string]any{
			{"label": "点数", "value": "100 点"},
			{"label": "团队席位", "value": "3 人"},
		},
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected package update 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	var updated Package
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated package: %v", err)
	}
	if !updated.IsActive || updated.Name != created.Name || updated.PriceCents != created.PriceCents ||
		updated.ValidDays != created.ValidDays || len(updated.Tags) != 2 ||
		len(updated.Features) != 2 || updated.Features[0] != "团队协作" ||
		len(updated.Benefits) != 2 || updated.Benefits[1].Label != "团队席位" {
		t.Fatalf("expected partial update to preserve omitted fields, got %+v", updated)
	}

	paidAt := time.Now()
	if err := db.Create(&FinanceOrder{
		OrderNumber:    "FO-PACKAGE-HISTORY",
		UserID:         user.ID,
		PackageID:      updated.ID,
		PackageName:    updated.Name,
		PackageCredits: updated.Credits,
		AmountCents:    updated.PriceCents,
		OrderType:      FinanceOrderTypePackage,
		PaymentMethod:  FinancePaymentMethodOffline,
		PaymentStatus:  FinancePaymentStatusPaid,
		InvoiceStatus:  FinanceInvoiceStatusPending,
		PaidAt:         &paidAt,
		CreatedAt:      paidAt,
		UpdatedAt:      paidAt,
	}).Error; err != nil {
		t.Fatalf("seed finance order before delete: %v", err)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/packages/"+itoa(created.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected package delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	var deletedVisible int64
	if err := db.Model(&Package{}).Where("id = ?", created.ID).Count(&deletedVisible).Error; err != nil {
		t.Fatalf("count visible deleted package: %v", err)
	}
	if deletedVisible != 0 {
		t.Fatalf("expected deleted package hidden from default queries, got %d", deletedVisible)
	}

	publicResp := performJSONRequest(t, testApp, http.MethodGet, "/api/packages", nil, nil)
	if publicResp.Code != http.StatusOK {
		t.Fatalf("expected public packages 200, got %d: %s", publicResp.Code, publicResp.Body.String())
	}
	if bytes.Contains(publicResp.Body.Bytes(), []byte(`"name":"商业加速包"`)) {
		t.Fatalf("expected deleted package hidden from public list: %s", publicResp.Body.String())
	}
	if !bytes.Contains(updateResp.Body.Bytes(), []byte(`"features":["团队协作","商用授权"]`)) ||
		!bytes.Contains(updateResp.Body.Bytes(), []byte(`"label":"团队席位"`)) {
		t.Fatalf("expected presentation fields in package response: %s", updateResp.Body.String())
	}

	adminOrders := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/finance-orders?q=%E5%95%86%E4%B8%9A%E5%8A%A0%E9%80%9F%E5%8C%85", nil, adminCookies)
	if adminOrders.Code != http.StatusOK {
		t.Fatalf("expected admin finance orders 200, got %d: %s", adminOrders.Code, adminOrders.Body.String())
	}
	if !bytes.Contains(adminOrders.Body.Bytes(), []byte(`"package_name":"商业加速包"`)) {
		t.Fatalf("expected finance order history preserved after package deletion: %s", adminOrders.Body.String())
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).Where("target_type = ? AND target_id = ? AND action IN ?", "package", created.ID, []string{"package.create", "package.update", "package.delete"}).Count(&auditCount).Error; err != nil {
		t.Fatalf("count package audit logs: %v", err)
	}
	if auditCount != 3 {
		t.Fatalf("expected create/update/delete audit logs, got %d", auditCount)
	}
}

func TestCustomerServiceConfigPublicReadAndAdminUpdate(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	publicResp := performJSONRequest(t, testApp, http.MethodGet, "/api/customer-service", nil, nil)
	if publicResp.Code != http.StatusOK {
		t.Fatalf("expected public customer service 200, got %d: %s", publicResp.Code, publicResp.Body.String())
	}
	if !bytes.Contains(publicResp.Body.Bytes(), []byte(`"wechat"`)) || !bytes.Contains(publicResp.Body.Bytes(), []byte(`"faqs"`)) {
		t.Fatalf("expected default customer service payload, got %s", publicResp.Body.String())
	}

	adminMissingResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/customer-service", map[string]any{}, nil)
	if adminMissingResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected customer service admin update to require login, got %d: %s", adminMissingResp.Code, adminMissingResp.Body.String())
	}

	adminCookies := createAdminSession(t, testApp)
	updatePayload := map[string]any{
		"title":       "联系客服",
		"eyebrow":     "CUSTOMER SERVICE",
		"subtitle":    "微信 / QQ 快速联系，移动端支持长按二维码添加微信",
		"description": "如您在使用过程中遇到账户问题、充值相关、生成异常或合作咨询等需求，请随时联系我们的客服团队。",
		"wechat": map[string]any{
			"label":   "微信客服",
			"account": "bailin_ai",
			"qr_url":  "https://cdn.example.com/wechat.png",
		},
		"qq": map[string]any{
			"label":   "QQ客服",
			"account": "123456789",
			"qr_url":  "https://cdn.example.com/qq.png",
		},
		"service_tags": []string{"账号问题", "充值咨询", "作品下载"},
		"stats": []map[string]any{
			{"label": "在线时间", "value": "09:00 - 22:00"},
			{"label": "平均响应", "value": "5 分钟内"},
			{"label": "服务范围", "value": "支持账号 / 支付 / 作品 / 合作咨询"},
		},
		"features": []map[string]any{
			{"title": "快速响应", "text": "专属客服团队在线服务，平均 5 分钟内响应"},
		},
		"faqs": []map[string]any{
			{"title": "充值未到账怎么办？", "url": "/pricing"},
		},
	}
	updateResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/customer-service", updatePayload, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected customer service update 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	reloadResp := performJSONRequest(t, testApp, http.MethodGet, "/api/customer-service", nil, nil)
	if reloadResp.Code != http.StatusOK {
		t.Fatalf("expected public customer service reload 200, got %d: %s", reloadResp.Code, reloadResp.Body.String())
	}
	if !bytes.Contains(reloadResp.Body.Bytes(), []byte(`"account":"bailin_ai"`)) ||
		!bytes.Contains(reloadResp.Body.Bytes(), []byte(`"title":"充值未到账怎么办？"`)) {
		t.Fatalf("expected updated customer service config, got %s", reloadResp.Body.String())
	}
}

func TestAdminCustomerServiceQRCodeUploadUsesAssetStorePublicURL(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	testApp.assetStore = &publicURLAssetStore{
		key:       "assets/2026/05/contact-qr.png",
		publicURL: "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/contact-qr.png",
	}

	unauthResp := performMultipartRequest(t, testApp, http.MethodPost, "/api/admin/customer-service/qrcode", "file", "wechat.png", mustBase64Decode(t, fakePNGBase64), nil)
	if unauthResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated upload 401, got %d: %s", unauthResp.Code, unauthResp.Body.String())
	}

	adminCookies := createAdminSession(t, testApp)
	uploadResp := performMultipartRequest(t, testApp, http.MethodPost, "/api/admin/customer-service/qrcode", "file", "wechat.png", mustBase64Decode(t, fakePNGBase64), adminCookies)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}

	var payload struct {
		URL      string `json:"url"`
		AssetKey string `json:"asset_key"`
		MIMEType string `json:"mime_type"`
	}
	if err := json.Unmarshal(uploadResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload payload: %v", err)
	}
	if payload.URL != "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/contact-qr.png" ||
		payload.AssetKey != "assets/2026/05/contact-qr.png" ||
		payload.MIMEType != "image/png" {
		t.Fatalf("unexpected upload payload: %+v", payload)
	}
}

func TestAdminUsersListSupportsSearchStatusPaginationAndStats(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	alice, _ := createLoggedInUser(t, testApp, "alice_ops", "test-password")
	bob, _ := createLoggedInUser(t, testApp, "bob_ops", "test-password")
	carol, _ := createLoggedInUser(t, testApp, "carol_ops", "test-password")
	setUserCredits(t, testApp, carol.ID, 0)

	yesterday := time.Now().Add(-24 * time.Hour)
	if err := db.Model(&User{}).Where("id = ?", alice.ID).Updates(map[string]any{
		"display_name": "Alice Designer",
		"phone":        "13800139011",
		"updated_at":   time.Now(),
	}).Error; err != nil {
		t.Fatalf("update alice: %v", err)
	}
	if err := db.Model(&User{}).Where("id = ?", bob.ID).Updates(map[string]any{
		"display_name": "Bob Analyst",
		"updated_at":   time.Now(),
	}).Error; err != nil {
		t.Fatalf("update bob: %v", err)
	}
	if err := db.Model(&User{}).Where("id = ?", carol.ID).Updates(map[string]any{
		"status":     "disabled",
		"created_at": yesterday,
		"updated_at": yesterday,
	}).Error; err != nil {
		t.Fatalf("update carol: %v", err)
	}
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", alice.ID).Update("available_credits", 15).Error; err != nil {
		t.Fatalf("update alice balance: %v", err)
	}
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", bob.ID).Update("available_credits", 25).Error; err != nil {
		t.Fatalf("update bob balance: %v", err)
	}
	if err := db.Create(&CreditTransaction{
		UserID:       alice.ID,
		Type:         CreditTransactionTypeManualTopUp,
		Amount:       15,
		BalanceAfter: 15,
		Reason:       "后台人工充值",
	}).Error; err != nil {
		t.Fatalf("create credit transaction: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?q=ops&status=active&page=1&page_size=2", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected admin users 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Items []struct {
			UserID           uint   `json:"user_id"`
			Username         string `json:"username"`
			Account          string `json:"account"`
			Phone            string `json:"phone"`
			DisplayName      string `json:"display_name"`
			AvatarURL        string `json:"avatar_url"`
			Status           string `json:"status"`
			Online           bool   `json:"online"`
			AvailableCredits int    `json:"available_credits"`
			TotalRecharged   int    `json:"total_recharged"`
			LastLoginAt      string `json:"last_login_at"`
			Role             struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"role"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Summary  struct {
			UsersTotal                int64   `json:"users_total"`
			ActiveUsers               int64   `json:"active_users"`
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
		} `json:"summary"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin users payload: %v", err)
	}
	if payload.Total != 2 || payload.Page != 1 || payload.PageSize != 2 || len(payload.Items) != 2 {
		t.Fatalf("unexpected pagination payload: %+v", payload)
	}
	if payload.Items[0].Status != UserStatusActive || payload.Items[1].Status != UserStatusActive {
		t.Fatalf("expected only active users, got %+v", payload.Items)
	}
	if payload.Items[0].Account == "" || payload.Items[0].Role.Code != "standard_user" || payload.Items[0].Role.Name != "普通用户" {
		t.Fatalf("expected enriched account and default role fields, got %+v", payload.Items[0])
	}
	var aliceItem *struct {
		UserID           uint   `json:"user_id"`
		Username         string `json:"username"`
		Account          string `json:"account"`
		Phone            string `json:"phone"`
		DisplayName      string `json:"display_name"`
		AvatarURL        string `json:"avatar_url"`
		Status           string `json:"status"`
		Online           bool   `json:"online"`
		AvailableCredits int    `json:"available_credits"`
		TotalRecharged   int    `json:"total_recharged"`
		LastLoginAt      string `json:"last_login_at"`
		Role             struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"role"`
	}
	for i := range payload.Items {
		if payload.Items[i].Username == "alice_ops" {
			aliceItem = &payload.Items[i]
			break
		}
	}
	if aliceItem == nil || aliceItem.TotalRecharged != 15 || aliceItem.LastLoginAt == "" {
		t.Fatalf("expected alice recharge and last login fields, got %+v", payload.Items)
	}
	if payload.Summary.UsersTotal != 3 || payload.Summary.ActiveUsers != 2 || payload.Summary.TodayNewUsers != 2 {
		t.Fatalf("unexpected user summary: %+v", payload.Summary)
	}
	if payload.Summary.TotalCredits != 40 || payload.Summary.TotalManualTopUp != 15 {
		t.Fatalf("unexpected credit summary: %+v", payload.Summary)
	}
	if len(payload.Summary.UsersTotalSparkline) != 7 || len(payload.Summary.ActiveUsersSparkline) != 7 ||
		len(payload.Summary.TodayNewUsersSparkline) != 7 || len(payload.Summary.TotalCreditsSparkline) != 7 {
		t.Fatalf("expected seven day sparklines, got %+v", payload.Summary)
	}

	phoneResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?q=39011&page=1&page_size=10", nil, adminCookies)
	if phoneResp.Code != http.StatusOK {
		t.Fatalf("expected phone search 200, got %d: %s", phoneResp.Code, phoneResp.Body.String())
	}
	var phonePayload struct {
		Items []struct {
			UserID uint   `json:"user_id"`
			Phone  string `json:"phone"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(phoneResp.Body.Bytes(), &phonePayload); err != nil {
		t.Fatalf("decode phone search payload: %v", err)
	}
	if phonePayload.Total != 1 || len(phonePayload.Items) != 1 || phonePayload.Items[0].UserID != alice.ID || phonePayload.Items[0].Phone != "13800139011" {
		t.Fatalf("expected alice by phone search with phone field, got %+v", phonePayload)
	}

	roleResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?role=content_reviewer", nil, adminCookies)
	if roleResp.Code != http.StatusOK {
		t.Fatalf("expected role filter 200, got %d: %s", roleResp.Code, roleResp.Body.String())
	}
	var rolePayload struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(roleResp.Body.Bytes(), &rolePayload); err != nil {
		t.Fatalf("decode role filter payload: %v", err)
	}
	if rolePayload.Total != 0 {
		t.Fatalf("expected content_reviewer role filter to exclude default users, got %d", rolePayload.Total)
	}
}

func TestAdminUsersListSortsHighlightedColumns(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	low, _ := createLoggedInUser(t, testApp, "sort_ops_low", "test-password")
	mid, _ := createLoggedInUser(t, testApp, "sort_ops_mid", "test-password")
	high, _ := createLoggedInUser(t, testApp, "sort_ops_high", "test-password")
	setUserCredits(t, testApp, low.ID, 5)
	setUserCredits(t, testApp, mid.ID, 30)
	setUserCredits(t, testApp, high.ID, 90)

	baseTime := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	lowCreatedAt := baseTime.Add(3 * time.Hour)
	midCreatedAt := baseTime.Add(2 * time.Hour)
	highCreatedAt := baseTime.Add(time.Hour)
	lowLoginAt := baseTime.Add(4 * time.Hour)
	midLoginAt := baseTime.Add(6 * time.Hour)
	highLoginAt := baseTime.Add(5 * time.Hour)
	if err := db.Model(&User{}).Where("id = ?", low.ID).Updates(map[string]any{
		"created_at":    lowCreatedAt,
		"updated_at":    lowCreatedAt,
		"last_login_at": lowLoginAt,
	}).Error; err != nil {
		t.Fatalf("update low user: %v", err)
	}
	if err := db.Model(&User{}).Where("id = ?", mid.ID).Updates(map[string]any{
		"created_at":    midCreatedAt,
		"updated_at":    midCreatedAt,
		"last_login_at": midLoginAt,
	}).Error; err != nil {
		t.Fatalf("update mid user: %v", err)
	}
	if err := db.Model(&User{}).Where("id = ?", high.ID).Updates(map[string]any{
		"status":        UserStatusDisabled,
		"created_at":    highCreatedAt,
		"updated_at":    highCreatedAt,
		"last_login_at": highLoginAt,
	}).Error; err != nil {
		t.Fatalf("update high user: %v", err)
	}
	if err := db.Create(&CreditTransaction{
		UserID:       mid.ID,
		Type:         CreditTransactionTypeManualTopUp,
		Amount:       20,
		BalanceAfter: 30,
		Reason:       "排序测试充值",
	}).Error; err != nil {
		t.Fatalf("create mid recharge: %v", err)
	}
	if err := db.Create(&CreditTransaction{
		UserID:       high.ID,
		Type:         CreditTransactionTypeManualTopUp,
		Amount:       200,
		BalanceAfter: 90,
		Reason:       "排序测试充值",
	}).Error; err != nil {
		t.Fatalf("create high recharge: %v", err)
	}

	now := time.Now()
	if err := db.Model(&UserSession{}).Where("user_id = ?", mid.ID).Updates(map[string]any{
		"expires_at":   now.Add(2 * time.Hour),
		"last_seen_at": now.Add(-2 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("mark mid online: %v", err)
	}
	if err := db.Model(&UserSession{}).Where("user_id IN ?", []uint{low.ID, high.ID}).Updates(map[string]any{
		"expires_at":   now.Add(-time.Hour),
		"last_seen_at": now.Add(-2 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("mark other users offline: %v", err)
	}

	assertAdminUsersOrder := func(path string, expected []uint) []struct {
		UserID           uint   `json:"user_id"`
		Username         string `json:"username"`
		Online           bool   `json:"online"`
		AvailableCredits int    `json:"available_credits"`
		TotalRecharged   int    `json:"total_recharged"`
		LastLoginAt      string `json:"last_login_at"`
	} {
		t.Helper()
		resp := performJSONRequest(t, testApp, http.MethodGet, path, nil, adminCookies)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected admin users 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var payload struct {
			Items []struct {
				UserID           uint   `json:"user_id"`
				Username         string `json:"username"`
				Online           bool   `json:"online"`
				AvailableCredits int    `json:"available_credits"`
				TotalRecharged   int    `json:"total_recharged"`
				LastLoginAt      string `json:"last_login_at"`
			} `json:"items"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode sorted users payload: %v", err)
		}
		if len(payload.Items) != len(expected) {
			t.Fatalf("expected %d sorted users, got %+v", len(expected), payload.Items)
		}
		for i, expectedID := range expected {
			if payload.Items[i].UserID != expectedID {
				t.Fatalf("expected order %v for %s, got %+v", expected, path, payload.Items)
			}
		}
		return payload.Items
	}

	assertAdminUsersOrder("/api/admin/users?q=sort_ops&sort_by=available_credits&sort_dir=desc&page=1&page_size=10", []uint{high.ID, mid.ID, low.ID})
	assertAdminUsersOrder("/api/admin/users?q=sort_ops&sort_by=total_recharged&sort_dir=asc&page=1&page_size=10", []uint{low.ID, mid.ID, high.ID})
	assertAdminUsersOrder("/api/admin/users?q=sort_ops&sort_by=last_login_at&sort_dir=desc&page=1&page_size=10", []uint{mid.ID, high.ID, low.ID})
	presenceItems := assertAdminUsersOrder("/api/admin/users?q=sort_ops&sort_by=presence&sort_dir=desc&page=1&page_size=10", []uint{mid.ID, high.ID, low.ID})
	if !presenceItems[0].Online || presenceItems[1].Online || presenceItems[2].Online {
		t.Fatalf("expected presence desc to put online user first, got %+v", presenceItems)
	}
	assertAdminUsersOrder("/api/admin/users?q=sort_ops&sort_by=unknown&sort_dir=sideways&page=1&page_size=10", []uint{low.ID, mid.ID, high.ID})
}

func TestAdminUsersWechatBindingCanBeViewedUpdatedAndUnbound(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "wechat_managed", "test-password")
	other, _ := createLoggedInUser(t, testApp, "wechat_taken", "test-password")
	if err := db.Model(&user).Update("wechat_open_id", "wx-existing-openid").Error; err != nil {
		t.Fatalf("bind user openid: %v", err)
	}
	if err := db.Model(&other).Update("wechat_open_id", "wx-taken-openid").Error; err != nil {
		t.Fatalf("bind other openid: %v", err)
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?q=wechat_managed", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected users list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			UserID        uint   `json:"user_id"`
			WechatBound   bool   `json:"wechat_bound"`
			WechatOpenID  string `json:"wechat_open_id"`
			WechatBinding struct {
				Bound  bool   `json:"bound"`
				OpenID string `json:"openid"`
			} `json:"wechat_binding"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode users list: %v", err)
	}
	if len(listPayload.Items) != 1 || !listPayload.Items[0].WechatBound ||
		listPayload.Items[0].WechatOpenID != "wx-existing-openid" ||
		!listPayload.Items[0].WechatBinding.Bound ||
		listPayload.Items[0].WechatBinding.OpenID != "wx-existing-openid" {
		t.Fatalf("expected visible wechat binding in admin users list, got %+v", listPayload.Items)
	}

	conflictResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/users/"+itoa(user.ID)+"/wechat-binding", map[string]any{
		"openid": "wx-taken-openid",
		"note":   "尝试绑定冲突 openid",
	}, adminCookies)
	if conflictResp.Code != http.StatusConflict {
		t.Fatalf("expected openid conflict 409, got %d: %s", conflictResp.Code, conflictResp.Body.String())
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/users/"+itoa(user.ID)+"/wechat-binding", map[string]any{
		"openid": " wx-updated-openid ",
		"note":   "客服核验后修正",
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update binding 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.WechatOpenID != "wx-updated-openid" {
		t.Fatalf("expected updated openid, got %q", user.WechatOpenID)
	}

	unbindResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/users/"+itoa(user.ID)+"/wechat-binding", map[string]any{
		"note": "用户要求解绑",
	}, adminCookies)
	if unbindResp.Code != http.StatusOK {
		t.Fatalf("expected unbind 200, got %d: %s", unbindResp.Code, unbindResp.Body.String())
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatalf("reload unbound user: %v", err)
	}
	if user.WechatOpenID != "" {
		t.Fatalf("expected openid cleared, got %q", user.WechatOpenID)
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).Where("target_type = ? AND target_id = ? AND action IN ?", "user", user.ID, []string{"users.wechat.update", "users.wechat.unbind"}).Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("expected update and unbind audit logs, got %d", auditCount)
	}
}

func TestAdminUserPhoneBindingCanBeUnboundAndAudited(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "phone_managed", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139013")

	unbindResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/users/"+itoa(user.ID)+"/phone-binding", map[string]any{
		"note": "用户要求解绑手机号",
	}, adminCookies)
	if unbindResp.Code != http.StatusOK {
		t.Fatalf("expected phone binding unbind 200, got %d: %s", unbindResp.Code, unbindResp.Body.String())
	}
	if !strings.Contains(unbindResp.Body.String(), `"phone":null`) || !strings.Contains(unbindResp.Body.String(), `"phone_bound":false`) {
		t.Fatalf("expected phone unbound payload, got %s", unbindResp.Body.String())
	}

	var reloaded User
	if err := db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Phone != nil {
		t.Fatalf("expected phone cleared, got %#v", reloaded.Phone)
	}

	var audit AdminAuditLog
	if err := db.Where("target_type = ? AND target_id = ? AND action = ?", "user", user.ID, "users.phone.unbind").First(&audit).Error; err != nil {
		t.Fatalf("load phone unbind audit log: %v", err)
	}
	if !strings.Contains(audit.Detail, "13800139013") || !strings.Contains(audit.Detail, "用户要求解绑手机号") {
		t.Fatalf("expected old phone and note in audit detail, got %s", audit.Detail)
	}
}

func TestAdminUserPhoneBindingUnbindReturnsNotFound(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/users/999999/phone-binding", map[string]any{
		"note": "清理手机号",
	}, adminCookies)
	if resp.Code != http.StatusNotFound || !strings.Contains(resp.Body.String(), "user_not_found") {
		t.Fatalf("expected missing user 404, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestAdminUserDeleteSoftDeletesUserAndRetainsHistory(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "delete_target", "test-password")
	setUserPhoneForTest(t, testApp, user.ID, "13800139012")
	if err := db.Create(&CreditTransaction{
		UserID:       user.ID,
		Type:         CreditTransactionTypeManualTopUp,
		Amount:       18,
		BalanceAfter: 18,
		Reason:       "删除前历史流水",
	}).Error; err != nil {
		t.Fatalf("create historical transaction: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/users/"+itoa(user.ID), nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected delete user 200, got %d: %s", resp.Code, resp.Body.String())
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?q=delete_target", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected users list 200 after delete, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Total int64 `json:"total"`
		Items []struct {
			UserID uint `json:"user_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode deleted user list: %v", err)
	}
	if listPayload.Total != 0 || len(listPayload.Items) != 0 {
		t.Fatalf("expected deleted user hidden from list, got %+v", listPayload)
	}

	loginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "delete_target", "test-password"), nil)
	if loginResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected deleted user login 401, got %d: %s", loginResp.Code, loginResp.Body.String())
	}

	var sessionCount int64
	if err := db.Model(&UserSession{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count deleted user sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected user sessions removed, got %d", sessionCount)
	}

	var softDeletedCount int64
	if err := db.Unscoped().Model(&User{}).Where("id = ? AND deleted_at IS NOT NULL", user.ID).Count(&softDeletedCount).Error; err != nil {
		t.Fatalf("count soft deleted user: %v", err)
	}
	if softDeletedCount != 1 {
		t.Fatalf("expected user to be soft deleted, got %d", softDeletedCount)
	}

	var transactionCount int64
	if err := db.Model(&CreditTransaction{}).
		Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypeManualTopUp).
		Count(&transactionCount).Error; err != nil {
		t.Fatalf("count historical transactions: %v", err)
	}
	if transactionCount != 1 {
		t.Fatalf("expected historical transaction retained, got %d", transactionCount)
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).Where("target_type = ? AND target_id = ? AND action = ?", "user", user.ID, "users.delete").Count(&auditCount).Error; err != nil {
		t.Fatalf("count delete audit logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected delete audit log, got %d", auditCount)
	}
}

func TestAdminUsersBatchDeleteDeduplicatesAndRejectsInvalidInputAtomically(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	first, _ := createLoggedInUser(t, testApp, "batch_delete_one", "test-password")
	second, _ := createLoggedInUser(t, testApp, "batch_delete_two", "test-password")
	kept, _ := createLoggedInUser(t, testApp, "batch_delete_kept", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/batch-delete", map[string]any{
		"user_ids": []uint{first.ID, first.ID, second.ID},
	}, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected batch delete 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var deletedCount int64
	if err := db.Unscoped().Model(&User{}).Where("id IN ? AND deleted_at IS NOT NULL", []uint{first.ID, second.ID}).Count(&deletedCount).Error; err != nil {
		t.Fatalf("count batch deleted users: %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("expected two unique users soft deleted, got %d", deletedCount)
	}
	var keptCount int64
	if err := db.Model(&User{}).Where("id = ?", kept.ID).Count(&keptCount).Error; err != nil {
		t.Fatalf("count kept user: %v", err)
	}
	if keptCount != 1 {
		t.Fatalf("expected unselected user to remain visible, got %d", keptCount)
	}

	emptyResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/batch-delete", map[string]any{
		"user_ids": []uint{},
	}, adminCookies)
	if emptyResp.Code != http.StatusBadRequest {
		t.Fatalf("expected empty batch delete 400, got %d: %s", emptyResp.Code, emptyResp.Body.String())
	}

	atomicOne, _ := createLoggedInUser(t, testApp, "batch_delete_atomic_one", "test-password")
	atomicTwo, _ := createLoggedInUser(t, testApp, "batch_delete_atomic_two", "test-password")
	missingResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/batch-delete", map[string]any{
		"user_ids": []uint{atomicOne.ID, 999999, atomicTwo.ID},
	}, adminCookies)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing batch delete 404, got %d: %s", missingResp.Code, missingResp.Body.String())
	}
	var atomicCount int64
	if err := db.Model(&User{}).Where("id IN ?", []uint{atomicOne.ID, atomicTwo.ID}).Count(&atomicCount).Error; err != nil {
		t.Fatalf("count atomic users: %v", err)
	}
	if atomicCount != 2 {
		t.Fatalf("expected missing batch delete to leave all requested users visible, got %d", atomicCount)
	}
}

func TestAdminUserDeleteRequiresDeletePermission(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "delete_forbidden_target", "test-password")
	admin := createDatabaseAdminUser(t, db, "delete-limited", "LimitedPass123")
	assignAdminRoleByCode(t, db, &admin, "auditor")
	cookies := loginAdminAs(t, testApp, "delete-limited", "LimitedPass123")

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/users/"+itoa(user.ID), nil, cookies)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected delete without users.delete permission 403, got %d: %s", resp.Code, resp.Body.String())
	}
	var count int64
	if err := db.Model(&User{}).Where("id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count user after forbidden delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected forbidden delete to leave user visible, got %d", count)
	}
}

func TestAdminUserResetPasswordRequiresPermissionAndValidatesInput(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "reset_forbidden_target", "test-password")

	unauthResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/reset-password", map[string]any{
		"password": "NewPass456",
	}, nil)
	if unauthResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated reset 401, got %d: %s", unauthResp.Code, unauthResp.Body.String())
	}

	limitedAdmin := createDatabaseAdminUser(t, db, "reset-limited", "LimitedPass123")
	assignAdminRoleByCode(t, db, &limitedAdmin, "auditor")
	limitedCookies := loginAdminAs(t, testApp, "reset-limited", "LimitedPass123")
	forbiddenResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/reset-password", map[string]any{
		"password": "NewPass456",
	}, limitedCookies)
	if forbiddenResp.Code != http.StatusForbidden {
		t.Fatalf("expected reset without users.password.reset permission 403, got %d: %s", forbiddenResp.Code, forbiddenResp.Body.String())
	}

	adminCookies := createAdminSession(t, testApp)
	shortResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/reset-password", map[string]any{
		"password": "short",
	}, adminCookies)
	if shortResp.Code != http.StatusBadRequest {
		t.Fatalf("expected short password reset 400, got %d: %s", shortResp.Code, shortResp.Body.String())
	}

	missingResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/999999/reset-password", map[string]any{
		"password": "NewPass456",
	}, adminCookies)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing user reset 404, got %d: %s", missingResp.Code, missingResp.Body.String())
	}

	deletedUser, _ := createLoggedInUser(t, testApp, "reset_deleted_target", "test-password")
	if err := db.Delete(&deletedUser).Error; err != nil {
		t.Fatalf("soft delete user: %v", err)
	}
	deletedResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(deletedUser.ID)+"/reset-password", map[string]any{
		"password": "NewPass456",
	}, adminCookies)
	if deletedResp.Code != http.StatusNotFound {
		t.Fatalf("expected deleted user reset 404, got %d: %s", deletedResp.Code, deletedResp.Body.String())
	}
}

func TestAdminUserResetPasswordUpdatesHashRevokesSessionsAndAudits(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, oldUserCookies := createLoggedInUser(t, testApp, "reset_success_target", "test-password")
	adminCookies := createAdminSession(t, testApp)

	resetResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/reset-password", map[string]any{
		"password": "NewPass456",
	}, adminCookies)
	if resetResp.Code != http.StatusOK {
		t.Fatalf("expected password reset 200, got %d: %s", resetResp.Code, resetResp.Body.String())
	}
	if !strings.Contains(resetResp.Body.String(), `"ok":true`) {
		t.Fatalf("expected ok payload, got %s", resetResp.Body.String())
	}

	oldSessionResp := performJSONRequest(t, testApp, http.MethodGet, "/api/me", nil, oldUserCookies)
	if oldSessionResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected existing user session revoked, got %d: %s", oldSessionResp.Code, oldSessionResp.Body.String())
	}

	var sessionCount int64
	if err := db.Model(&UserSession{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count reset user sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected reset to remove user sessions, got %d", sessionCount)
	}

	oldPasswordResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, user.Username, "test-password"), nil)
	if oldPasswordResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password login 401, got %d: %s", oldPasswordResp.Code, oldPasswordResp.Body.String())
	}
	newPasswordResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, user.Username, "NewPass456"), nil)
	if newPasswordResp.Code != http.StatusOK {
		t.Fatalf("expected new password login 200, got %d: %s", newPasswordResp.Code, newPasswordResp.Body.String())
	}

	var audit AdminAuditLog
	if err := db.Where("target_type = ? AND target_id = ? AND action = ?", "user", user.ID, "users.password.reset").First(&audit).Error; err != nil {
		t.Fatalf("load reset password audit log: %v", err)
	}
	if strings.Contains(audit.Detail, "NewPass456") || strings.Contains(audit.Detail, "test-password") {
		t.Fatalf("reset password audit log must not contain plaintext passwords: %s", audit.Detail)
	}
}

func TestAdminUserPasswordResetPermissionSeededForSuperAdminOnly(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	var permission Permission
	if err := db.Where("code = ?", "users.password.reset").First(&permission).Error; err != nil {
		t.Fatalf("expected users.password.reset permission seed: %v", err)
	}
	if permission.Name != "重置用户密码" {
		t.Fatalf("unexpected users.password.reset permission name: %q", permission.Name)
	}

	adminCookies := createAdminSession(t, testApp)
	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, adminCookies)
	if meResp.Code != http.StatusOK || !strings.Contains(meResp.Body.String(), `"users.password.reset"`) {
		t.Fatalf("expected super admin session to include users.password.reset, got %d: %s", meResp.Code, meResp.Body.String())
	}

	for _, roleCode := range []string{"operator", "auditor"} {
		var role Role
		if err := db.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
			t.Fatalf("load role %s: %v", roleCode, err)
		}
		for _, rolePermission := range role.Permissions {
			if rolePermission.Code == "users.password.reset" {
				t.Fatalf("expected %s role not to include users.password.reset by default", roleCode)
			}
		}
	}
}

func TestAdminUsersListFiltersOnlineAndOfflineFromSessions(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	onlineUser, _ := createLoggedInUser(t, testApp, "online_user", "test-password")
	offlineUser, _ := createLoggedInUser(t, testApp, "offline_user", "test-password")
	staleUser, _ := createLoggedInUser(t, testApp, "stale_online_user", "test-password")
	now := time.Now()
	if err := db.Model(&UserSession{}).Where("user_id = ?", onlineUser.ID).Update("last_seen_at", now.Add(-2*time.Minute)).Error; err != nil {
		t.Fatalf("mark online user seen recently: %v", err)
	}
	if err := db.Model(&UserSession{}).Where("user_id = ?", offlineUser.ID).Updates(map[string]any{
		"expires_at":   now.Add(-time.Hour),
		"last_seen_at": now.Add(-2 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("expire offline user session: %v", err)
	}
	if err := db.Model(&UserSession{}).Where("user_id = ?", staleUser.ID).Updates(map[string]any{
		"expires_at":   now.Add(2 * time.Hour),
		"last_seen_at": now.Add(-6 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("stale active session: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?status=online&page=1&page_size=10", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected online filter 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			UserID uint `json:"user_id"`
			Online bool `json:"online"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode online filter payload: %v", err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].UserID != onlineUser.ID || !payload.Items[0].Online {
		t.Fatalf("expected only active session user online, got %+v", payload)
	}

	resp = performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?status=offline&page=1&page_size=10", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected offline filter 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload = struct {
		Items []struct {
			UserID uint `json:"user_id"`
			Online bool `json:"online"`
		} `json:"items"`
		Total int64 `json:"total"`
	}{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode offline filter payload: %v", err)
	}
	if payload.Total != 2 || len(payload.Items) != 2 {
		t.Fatalf("expected expired and stale-session users offline, got %+v", payload)
	}
	offlineByID := map[uint]bool{}
	for _, item := range payload.Items {
		if item.Online {
			t.Fatalf("expected offline filter to return only offline users, got %+v", payload)
		}
		offlineByID[item.UserID] = true
	}
	if !offlineByID[offlineUser.ID] || !offlineByID[staleUser.ID] {
		t.Fatalf("expected expired and stale-session users offline, got %+v", payload)
	}
}

func TestUserSessionLastSeenAndPresenceHeartbeat(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	user, cookies := createLoggedInUser(t, testApp, "presence_user", "test-password")
	var session UserSession
	if err := db.Where("user_id = ?", user.ID).First(&session).Error; err != nil {
		t.Fatalf("load created session: %v", err)
	}
	if session.LastSeenAt == nil || time.Since(*session.LastSeenAt) > time.Minute {
		t.Fatalf("expected login to write last_seen_at, got %+v", session.LastSeenAt)
	}

	staleSeen := time.Now().Add(-6 * time.Minute)
	if err := db.Model(&UserSession{}).Where("id = ?", session.ID).Update("last_seen_at", staleSeen).Error; err != nil {
		t.Fatalf("make session stale: %v", err)
	}
	staleList := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?status=online&q=presence_user", nil, adminCookies)
	if staleList.Code != http.StatusOK {
		t.Fatalf("expected stale list 200, got %d: %s", staleList.Code, staleList.Body.String())
	}
	var stalePayload struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(staleList.Body.Bytes(), &stalePayload); err != nil {
		t.Fatalf("decode stale online payload: %v", err)
	}
	if stalePayload.Total != 0 {
		t.Fatalf("expected stale session to be offline, got %+v", stalePayload)
	}

	presenceResp := performJSONRequest(t, testApp, http.MethodGet, "/api/account/presence", nil, cookies)
	if presenceResp.Code != http.StatusOK {
		t.Fatalf("expected presence heartbeat 200, got %d: %s", presenceResp.Code, presenceResp.Body.String())
	}
	var presencePayload struct {
		OK                  bool   `json:"ok"`
		LastSeenAt          string `json:"last_seen_at"`
		OnlineWindowSeconds int    `json:"online_window_seconds"`
	}
	if err := json.Unmarshal(presenceResp.Body.Bytes(), &presencePayload); err != nil {
		t.Fatalf("decode presence payload: %v", err)
	}
	if !presencePayload.OK || strings.TrimSpace(presencePayload.LastSeenAt) == "" || presencePayload.OnlineWindowSeconds != 300 {
		t.Fatalf("unexpected presence payload: %+v", presencePayload)
	}

	freshList := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?status=online&q=presence_user", nil, adminCookies)
	if freshList.Code != http.StatusOK {
		t.Fatalf("expected fresh list 200, got %d: %s", freshList.Code, freshList.Body.String())
	}
	var freshPayload struct {
		Items []struct {
			UserID uint `json:"user_id"`
			Online bool `json:"online"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(freshList.Body.Bytes(), &freshPayload); err != nil {
		t.Fatalf("decode fresh online payload: %v", err)
	}
	if freshPayload.Total != 1 || len(freshPayload.Items) != 1 || freshPayload.Items[0].UserID != user.ID || !freshPayload.Items[0].Online {
		t.Fatalf("expected presence heartbeat to restore online status, got %+v", freshPayload)
	}
}

func TestAdminUsersSummaryCountsRealtimeOnlineUsersOnce(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	onlineUser, _ := createLoggedInUser(t, testApp, "summary_online_user", "test-password")
	disabledUser, _ := createLoggedInUser(t, testApp, "summary_disabled_user", "test-password")
	staleUser, _ := createLoggedInUser(t, testApp, "summary_stale_user", "test-password")
	now := time.Now()
	if err := db.Model(&UserSession{}).Where("user_id = ?", onlineUser.ID).Update("last_seen_at", now.Add(-time.Minute)).Error; err != nil {
		t.Fatalf("mark online user active: %v", err)
	}
	secondSessionLastSeen := now.Add(-30 * time.Second)
	if err := db.Create(&UserSession{
		UserID:     onlineUser.ID,
		TokenID:    "summary-online-second-session",
		ExpiresAt:  now.Add(time.Hour),
		LastSeenAt: &secondSessionLastSeen,
	}).Error; err != nil {
		t.Fatalf("create second online session: %v", err)
	}
	if err := db.Model(&User{}).Where("id = ?", disabledUser.ID).Update("status", UserStatusDisabled).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if err := db.Model(&UserSession{}).Where("user_id = ?", disabledUser.ID).Update("last_seen_at", now.Add(-time.Minute)).Error; err != nil {
		t.Fatalf("mark disabled user active: %v", err)
	}
	if err := db.Model(&UserSession{}).Where("user_id = ?", staleUser.ID).Update("last_seen_at", now.Add(-6*time.Minute)).Error; err != nil {
		t.Fatalf("mark stale user inactive: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users?page=1&page_size=10", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected admin users 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Summary struct {
			ActiveUsers int64 `json:"active_users"`
			OnlineUsers int64 `json:"online_users"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin users summary: %v", err)
	}
	if payload.Summary.ActiveUsers != 2 {
		t.Fatalf("expected active_users to remain active account count, got %+v", payload.Summary)
	}
	if payload.Summary.OnlineUsers != 1 {
		t.Fatalf("expected online_users to count one active user despite multiple sessions and disabled active session, got %+v", payload.Summary)
	}
}

func TestRegisterAndLoginUpdateLastLoginAt(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	createSMSVerificationCodeForTest(t, testApp, "13800139007", smsPurposeRegister, "123456")
	registerResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/register-phone", map[string]any{
		"phone":             "13800139007",
		"verification_code": "123456",
		"username":          "login_time_user",
		"password":          "test-password",
	}, nil)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("expected phone register 201, got %d: %s", registerResp.Code, registerResp.Body.String())
	}

	var registered sql.NullTime
	if err := db.Raw("SELECT last_login_at FROM users WHERE username = ?", "login_time_user").Scan(&registered).Error; err != nil {
		t.Fatalf("read registered last_login_at: %v", err)
	}
	if !registered.Valid {
		t.Fatal("expected registration to set last_login_at")
	}

	older := time.Now().Add(-2 * time.Hour)
	if err := db.Exec("UPDATE users SET last_login_at = ? WHERE username = ?", older, "login_time_user").Error; err != nil {
		t.Fatalf("force old last_login_at: %v", err)
	}

	loginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, testApp, "login_time_user", "test-password"), nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginResp.Code, loginResp.Body.String())
	}

	var afterLogin sql.NullTime
	if err := db.Raw("SELECT last_login_at FROM users WHERE username = ?", "login_time_user").Scan(&afterLogin).Error; err != nil {
		t.Fatalf("read login last_login_at: %v", err)
	}
	if !afterLogin.Valid || !afterLogin.Time.After(older) {
		t.Fatalf("expected login to refresh last_login_at, got %v", afterLogin)
	}
}

func TestAdminCreditTransactionsListShowsRecentTopUps(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "creator_credit_log", "test-password")
	adminCookies := createAdminSession(t, testApp)

	topUpResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/credits", map[string]any{
		"amount": 12,
		"note":   "补发活动点数",
	}, adminCookies)
	if topUpResp.Code != http.StatusOK {
		t.Fatalf("expected top up 200, got %d: %s", topUpResp.Code, topUpResp.Body.String())
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/credit-transactions?page=1&page_size=5", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected admin credit transactions 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Items []struct {
			UserID       uint   `json:"user_id"`
			Username     string `json:"username"`
			Type         string `json:"type"`
			Amount       int    `json:"amount"`
			BalanceAfter int    `json:"balance_after"`
			AdminNote    string `json:"admin_note"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin credit transactions payload: %v", err)
	}
	if payload.Total != 2 || payload.Page != 1 || payload.PageSize != 5 || len(payload.Items) != 2 {
		t.Fatalf("unexpected transactions pagination: %+v", payload)
	}
	item := payload.Items[0]
	if item.UserID != user.ID || item.Username != user.Username || item.Type != CreditTransactionTypeManualTopUp {
		t.Fatalf("unexpected transaction identity: %+v", item)
	}
	if item.Amount != 12 || item.BalanceAfter != 17 || item.AdminNote != "补发活动点数" {
		t.Fatalf("unexpected transaction details: %+v", item)
	}
	if payload.Items[1].Type != CreditTransactionTypeSignupBonus || payload.Items[1].Amount != signupBonusCredits {
		t.Fatalf("expected signup bonus transaction to remain visible, got %+v", payload.Items)
	}
}

func TestAdminCreditAdjustmentsAddAndDeductCreditsWithAudit(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "creator_adjust", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	adminCookies := createAdminSession(t, testApp)

	addResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/credit-adjustments", map[string]any{
		"type":   "add",
		"amount": 20,
		"note":   "补发点数",
	}, adminCookies)
	if addResp.Code != http.StatusOK {
		t.Fatalf("expected add adjustment 200, got %d: %s", addResp.Code, addResp.Body.String())
	}

	deductResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/credit-adjustments", map[string]any{
		"type":   "deduct",
		"amount": 8,
		"note":   "人工扣减",
	}, adminCookies)
	if deductResp.Code != http.StatusOK {
		t.Fatalf("expected deduct adjustment 200, got %d: %s", deductResp.Code, deductResp.Body.String())
	}

	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load adjusted balance: %v", err)
	}
	if balance.AvailableCredits != 12 {
		t.Fatalf("expected balance 12 after add and deduct, got %d", balance.AvailableCredits)
	}

	var transactions []CreditTransaction
	if err := db.Where("user_id = ? AND type IN ?", user.ID, []string{CreditTransactionTypeManualTopUp, CreditTransactionTypeManualDeduct}).
		Order("id asc").Find(&transactions).Error; err != nil {
		t.Fatalf("load adjustment transactions: %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("expected two adjustment transactions, got %+v", transactions)
	}
	if transactions[0].Type != CreditTransactionTypeManualTopUp || transactions[0].Amount != 20 || transactions[0].BalanceAfter != 20 {
		t.Fatalf("unexpected add transaction: %+v", transactions[0])
	}
	if transactions[1].Type != "manual_deduct" || transactions[1].Amount != -8 || transactions[1].BalanceAfter != 12 {
		t.Fatalf("unexpected deduct transaction: %+v", transactions[1])
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).Where("target_type = ? AND target_id = ? AND action IN ?", "user", user.ID, []string{"users.credits.add", "users.credits.deduct"}).Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("expected add and deduct audit logs, got %d", auditCount)
	}
}

func TestAdminCreditAdjustmentRejectsInsufficientDeductWithoutTransaction(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "creator_insufficient", "test-password")
	setUserCredits(t, testApp, user.ID, 0)
	adminCookies := createAdminSession(t, testApp)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/users/"+itoa(user.ID)+"/credit-adjustments", map[string]any{
		"type":   "deduct",
		"amount": 3,
		"note":   "余额不足扣点",
	}, adminCookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected insufficient deduct 422, got %d: %s", resp.Code, resp.Body.String())
	}

	var transactionCount int64
	if err := db.Model(&CreditTransaction{}).
		Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypeManualDeduct).
		Count(&transactionCount).Error; err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if transactionCount != 0 {
		t.Fatalf("expected no transaction for failed deduct, got %d", transactionCount)
	}
}

func TestDefaultPackagesIncludeExtendedMetadataAndRespectSoftDeletedDefaults(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	var initial []Package
	if err := db.Order("sort_order asc, id asc").Find(&initial).Error; err != nil {
		t.Fatalf("load seeded packages: %v", err)
	}
	if len(initial) != 6 {
		t.Fatalf("expected six default packages, got %+v", initial)
	}
	if initial[0].Name != "体验包" || initial[0].Credits != 50 || initial[0].PriceLabel != "10 元" ||
		initial[0].PriceCents != 1000 || initial[0].ValidDays != 30 {
		t.Fatalf("unexpected experience package: %+v", initial[0])
	}
	if initial[2].Name != "常用包" || initial[2].Credits != 688 || initial[2].PriceLabel != "100 元" ||
		initial[2].PriceCents != 10000 || initial[2].ValidDays != 180 {
		t.Fatalf("unexpected common package: %+v", initial[2])
	}
	if initial[4].Name != "专业包" || initial[4].Credits != 2588 || initial[4].PriceLabel != "298 元" ||
		initial[4].PriceCents != 29800 || initial[4].ValidDays != 365 || !initial[4].Recommended ||
		initial[4].Badge != "推荐" {
		t.Fatalf("unexpected professional package: %+v", initial[4])
	}
	expectedVirtualProducts := map[string]string{
		"体验包": "pointspack10",
		"入门包": "pointspack30",
		"常用包": "pointspack100",
		"进阶包": "pointspack198",
		"专业包": "pointspack298",
		"旗舰包": "pointspack648",
	}
	for _, pkg := range initial {
		if pkg.WechatVirtualProductID != expectedVirtualProducts[pkg.Name] {
			t.Fatalf("expected %s virtual product id %q, got %q", pkg.Name, expectedVirtualProducts[pkg.Name], pkg.WechatVirtualProductID)
		}
	}
	if initial[5].Name != "旗舰包" || initial[5].Credits != 6188 || initial[5].Badge != "最划算" ||
		initial[5].Theme != "gold" || len(initial[5].Tags) == 0 {
		t.Fatalf("expected flagship package extended metadata, got %+v", initial[5])
	}

	if err := db.Where("name = ?", "进阶包").Delete(&Package{}).Error; err != nil {
		t.Fatalf("soft delete advanced package: %v", err)
	}
	if err := db.Model(&Package{}).Where("name = ?", "体验包").Updates(map[string]any{
		"price_label": "自定义价格",
		"credits":     88,
		"is_active":   false,
		"theme":       "custom",
	}).Error; err != nil {
		t.Fatalf("customize package: %v", err)
	}

	var customized Package
	if err := db.Where("name = ?", "体验包").First(&customized).Error; err != nil {
		t.Fatalf("load customized package before reseed: %v", err)
	}
	if err := db.Model(&customized).Update("tags_json", "").Error; err != nil {
		t.Fatalf("clear tags json before reseed: %v", err)
	}

	var inspiration Package
	if err := db.Where("name = ?", "体验包").First(&inspiration).Error; err != nil {
		t.Fatalf("load customized inspiration package: %v", err)
	}
	if err := testApp.seedPackages(); err != nil {
		t.Fatalf("seed packages: %v", err)
	}

	if err := db.Where("name = ?", "体验包").First(&inspiration).Error; err != nil {
		t.Fatalf("load customized inspiration package after reseed: %v", err)
	}
	if inspiration.PriceLabel != "自定义价格" || inspiration.Credits != 88 || inspiration.IsActive || inspiration.Theme != "custom" {
		t.Fatalf("expected existing package preserved, got %+v", inspiration)
	}
	if inspiration.ValidDays != 30 || inspiration.Audience == "" || inspiration.Icon == "" || len(inspiration.Tags) == 0 {
		t.Fatalf("expected missing extended fields backfilled without overriding custom values, got %+v", inspiration)
	}
	if inspiration.WechatVirtualProductID != "pointspack10" {
		t.Fatalf("expected missing virtual product id backfilled, got %+v", inspiration)
	}

	var advanced Package
	err := db.Where("name = ?", "进阶包").First(&advanced).Error
	if err == nil {
		t.Fatalf("expected soft-deleted default package to stay hidden, got %+v", advanced)
	}
	var allAdvancedCount int64
	if err := db.Unscoped().Model(&Package{}).Where("name = ?", "进阶包").Count(&allAdvancedCount).Error; err != nil {
		t.Fatalf("count unscoped advanced package: %v", err)
	}
	if allAdvancedCount != 1 {
		t.Fatalf("expected soft-deleted advanced package not recreated, got %d records", allAdvancedCount)
	}
}

func TestSeedPackagesUpgradesLegacyPricingWithoutDuplicates(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})

	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Package{}).Error; err != nil {
		t.Fatalf("clear seeded packages: %v", err)
	}
	legacy := []Package{
		{Name: "灵感包", PriceLabel: "10 元", PriceCents: 1000, Credits: 50, ValidDays: 30, SortOrder: 10, IsActive: true},
		{Name: "创作包", PriceLabel: "30 元", PriceCents: 3000, Credits: 188, ValidDays: 90, SortOrder: 20, IsActive: true},
		{Name: "高频包", PriceLabel: "100 元", PriceCents: 10000, Credits: 688, ValidDays: 180, SortOrder: 30, IsActive: true},
		{Name: "团队包", PriceLabel: "399 元", PriceCents: 39900, Credits: 320, ValidDays: 365, SortOrder: 40, IsActive: true},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy packages: %v", err)
	}

	if err := testApp.seedPackages(); err != nil {
		t.Fatalf("seed packages: %v", err)
	}

	var active []Package
	if err := db.Where("is_active = ?", true).Order("sort_order asc, id asc").Find(&active).Error; err != nil {
		t.Fatalf("load active packages: %v", err)
	}
	if len(active) != 6 {
		t.Fatalf("expected six active packages after legacy upgrade, got %+v", active)
	}
	expected := []struct {
		name       string
		priceCents int64
		credits    int
	}{
		{"体验包", 1000, 50},
		{"入门包", 3000, 188},
		{"常用包", 10000, 688},
		{"进阶包", 19800, 1488},
		{"专业包", 29800, 2588},
		{"旗舰包", 64800, 6188},
	}
	for index, want := range expected {
		got := active[index]
		if got.Name != want.name || got.PriceCents != want.priceCents || got.Credits != want.credits {
			t.Fatalf("unexpected package at %d: got %+v want %+v", index, got, want)
		}
	}
	var oldDefaultActiveCount int64
	if err := db.Model(&Package{}).Where("name IN ? AND is_active = ?", []string{"灵感包", "创作包", "高频包", "团队包"}, true).Count(&oldDefaultActiveCount).Error; err != nil {
		t.Fatalf("count old active packages: %v", err)
	}
	if oldDefaultActiveCount != 0 {
		t.Fatalf("expected old default package names not to stay active, got %d", oldDefaultActiveCount)
	}
}

func TestAdminSettingsRejectsModelOutsideAllowList(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	resp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/settings/image", map[string]any{
		"active_model": "gpt-image-999",
	}, adminCookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.Code)
	}
}

func TestAdminModelConfigsListFiltersAndPaginatesSeedData(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	var dalle ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&dalle).Error; err != nil {
		t.Fatalf("load DALL-E model: %v", err)
	}
	now := time.Now()
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: dalle.ID,
		Model:         "gpt-image-2",
		Status:        GenerationStatusSucceeded,
		LatencyMS:     1200,
		CreatedAt:     now,
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: dalle.ID,
		Model:         "gpt-image-2",
		Status:        GenerationStatusFailed,
		LatencyMS:     0,
		CreatedAt:     now.Add(-26 * time.Hour),
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models?page=1&page_size=3", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected admin models 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Items []struct {
			ID           uint   `json:"id"`
			Name         string `json:"name"`
			Type         string `json:"type"`
			Provider     string `json:"provider"`
			Status       string `json:"status"`
			Priority     int    `json:"priority"`
			CostLabel    string `json:"cost_label"`
			Permission   string `json:"permission"`
			Weight       int    `json:"weight"`
			SortOrder    int    `json:"sort_order"`
			RuntimeModel string `json:"runtime_model"`
			Usage        struct {
				TotalCalls     int64 `json:"total_calls"`
				SucceededCalls int64 `json:"succeeded_calls"`
				FailedCalls    int64 `json:"failed_calls"`
				TodayCalls     int64 `json:"today_calls"`
				Last7DaysCalls int64 `json:"last_7d_calls"`
				AverageLatency int64 `json:"average_latency_ms"`
			} `json:"usage"`
		} `json:"items"`
		Summary struct {
			OnlineModels   int64  `json:"online_models"`
			ImageModels    int64  `json:"image_models"`
			VideoModels    int64  `json:"video_models"`
			DefaultModel   string `json:"default_model"`
			DefaultModelID uint   `json:"default_model_id"`
			TotalCalls     int64  `json:"total_calls"`
		} `json:"summary"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin models payload: %v", err)
	}
	if payload.Total != 16 || payload.Page != 1 || payload.PageSize != 3 || len(payload.Items) != 3 {
		t.Fatalf("expected paginated seed data, got %+v", payload)
	}
	if payload.Summary.OnlineModels != 14 || payload.Summary.ImageModels != 5 || payload.Summary.VideoModels != 10 || payload.Summary.DefaultModel != "DALL-E 3" {
		t.Fatalf("unexpected model summary: %+v", payload.Summary)
	}
	if payload.Summary.TotalCalls != 2 {
		t.Fatalf("expected model summary total calls from generation records, got %+v", payload.Summary)
	}
	var dalleUsage struct {
		TotalCalls     int64
		SucceededCalls int64
		FailedCalls    int64
		TodayCalls     int64
		Last7DaysCalls int64
		AverageLatency int64
	}
	for _, item := range payload.Items {
		if item.Name == "DALL-E 3" {
			dalleUsage.TotalCalls = item.Usage.TotalCalls
			dalleUsage.SucceededCalls = item.Usage.SucceededCalls
			dalleUsage.FailedCalls = item.Usage.FailedCalls
			dalleUsage.TodayCalls = item.Usage.TodayCalls
			dalleUsage.Last7DaysCalls = item.Usage.Last7DaysCalls
			dalleUsage.AverageLatency = item.Usage.AverageLatency
		}
	}
	if dalleUsage.TotalCalls != 2 || dalleUsage.SucceededCalls != 1 || dalleUsage.FailedCalls != 1 || dalleUsage.TodayCalls != 1 || dalleUsage.Last7DaysCalls != 2 || dalleUsage.AverageLatency != 1200 {
		t.Fatalf("unexpected DALL-E usage stats: %+v", dalleUsage)
	}

	filtered := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models?type=image&provider=OpenAI&status=online&q=dall&page=1&page_size=10", nil, adminCookies)
	if filtered.Code != http.StatusOK {
		t.Fatalf("expected filtered admin models 200, got %d: %s", filtered.Code, filtered.Body.String())
	}
	var filteredPayload struct {
		Items []struct {
			Name         string `json:"name"`
			Type         string `json:"type"`
			Provider     string `json:"provider"`
			Status       string `json:"status"`
			RuntimeModel string `json:"runtime_model"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredPayload); err != nil {
		t.Fatalf("decode filtered admin models payload: %v", err)
	}
	if filteredPayload.Total != 1 || len(filteredPayload.Items) != 1 {
		t.Fatalf("expected one filtered model, got %+v", filteredPayload)
	}
	if filteredPayload.Items[0].Name != "DALL-E 3" || filteredPayload.Items[0].RuntimeModel != "gpt-image-2" {
		t.Fatalf("expected DALL-E runtime mapping, got %+v", filteredPayload.Items[0])
	}

	realModelResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models?q=gpt-image-2-2026-04-21&page=1&page_size=10", nil, adminCookies)
	if realModelResp.Code != http.StatusOK {
		t.Fatalf("expected runtime model search 200, got %d: %s", realModelResp.Code, realModelResp.Body.String())
	}
	var realModelPayload struct {
		Items []struct {
			Name         string `json:"name"`
			RuntimeModel string `json:"runtime_model"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(realModelResp.Body.Bytes(), &realModelPayload); err != nil {
		t.Fatalf("decode runtime model payload: %v", err)
	}
	if realModelPayload.Total != 1 || len(realModelPayload.Items) != 1 || realModelPayload.Items[0].RuntimeModel != "gpt-image-2-2026-04-21" {
		t.Fatalf("expected real allowed runtime model in model list, got %+v", realModelPayload)
	}
}

func TestAdminModelUsageStatsStaySeparateForDuplicateRuntimeModels(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	first := ModelConfig{
		Name:         "生图模型2",
		Type:         ModelConfigTypeImage,
		Provider:     "OpenAI",
		Status:       ModelConfigStatusOnline,
		Priority:     1,
		CostLabel:    "5 点/次",
		Permission:   ModelConfigPermissionPublic,
		Weight:       0,
		SortOrder:    1,
		RuntimeModel: "gpt-image-2",
		APIEndpoint:  "/v1/images/generations",
	}
	second := ModelConfig{
		Name:         "gpt-image-2",
		Type:         ModelConfigTypeImage,
		Provider:     "OpenAI",
		Status:       ModelConfigStatusOnline,
		Priority:     2,
		CostLabel:    "1 点/次",
		Permission:   ModelConfigPermissionPublic,
		Weight:       0,
		SortOrder:    30,
		RuntimeModel: "gpt-image-2",
		APIEndpoint:  "/v1/images/generations",
	}
	if err := testApp.db.Create(&first).Error; err != nil {
		t.Fatalf("create first duplicate runtime model: %v", err)
	}
	if err := testApp.db.Create(&second).Error; err != nil {
		t.Fatalf("create second duplicate runtime model: %v", err)
	}

	now := time.Now()
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: first.ID,
		Model:         "gpt-image-2",
		Status:        GenerationStatusSucceeded,
		LatencyMS:     800,
		CreatedAt:     now,
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		ModelConfigID: second.ID,
		Model:         "gpt-image-2",
		Status:        GenerationStatusFailed,
		CreatedAt:     now,
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		Model:     "gpt-image-2",
		Status:    GenerationStatusSucceeded,
		CreatedAt: now,
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models?q=gpt-image-2&page=1&page_size=100", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected duplicate runtime model list 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID    uint   `json:"id"`
			Name  string `json:"name"`
			Usage struct {
				TotalCalls     int64 `json:"total_calls"`
				SucceededCalls int64 `json:"succeeded_calls"`
				FailedCalls    int64 `json:"failed_calls"`
			} `json:"usage"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode duplicate runtime model list: %v", err)
	}
	usageByID := map[uint]struct {
		Total     int64
		Succeeded int64
		Failed    int64
	}{}
	for _, item := range payload.Items {
		usageByID[item.ID] = struct {
			Total     int64
			Succeeded int64
			Failed    int64
		}{item.Usage.TotalCalls, item.Usage.SucceededCalls, item.Usage.FailedCalls}
	}
	if usageByID[first.ID].Total != 1 || usageByID[first.ID].Succeeded != 1 || usageByID[first.ID].Failed != 0 {
		t.Fatalf("expected first duplicate runtime model to keep only its own usage, got %+v", usageByID[first.ID])
	}
	if usageByID[second.ID].Total != 1 || usageByID[second.ID].Succeeded != 0 || usageByID[second.ID].Failed != 1 {
		t.Fatalf("expected second duplicate runtime model to exclude legacy string-only usage, got %+v", usageByID[second.ID])
	}
}

func TestAdminModelDetailReturnsUsageTrendAndRecentGenerations(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var dalle ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&dalle).Error; err != nil {
		t.Fatalf("load DALL-E model: %v", err)
	}
	now := time.Now()
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:        7,
		Prompt:        "今日海报",
		ModelConfigID: dalle.ID,
		Model:         "gpt-image-2",
		Status:        GenerationStatusSucceeded,
		LatencyMS:     900,
		CreatedAt:     now.Add(-3 * time.Hour),
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:        8,
		Prompt:        "失败任务",
		ModelConfigID: dalle.ID,
		Model:         "gpt-image-2",
		Status:        GenerationStatusFailed,
		ErrorMessage:  "provider timeout",
		CreatedAt:     now.AddDate(0, 0, -1),
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:    18,
		Prompt:    "历史字符串记录",
		Model:     "gpt-image-2",
		Status:    GenerationStatusSucceeded,
		CreatedAt: now,
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:    9,
		Prompt:    "其他模型",
		Model:     "gpt-image-2-2026-04-21",
		Status:    GenerationStatusSucceeded,
		CreatedAt: now,
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/models/%d", dalle.ID), nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected model detail 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Model struct {
			ID           uint   `json:"id"`
			Name         string `json:"name"`
			RuntimeModel string `json:"runtime_model"`
		} `json:"model"`
		Usage struct {
			TotalCalls     int64 `json:"total_calls"`
			SucceededCalls int64 `json:"succeeded_calls"`
			FailedCalls    int64 `json:"failed_calls"`
			AverageLatency int64 `json:"average_latency_ms"`
		} `json:"usage"`
		StatusBreakdown []struct {
			Status string `json:"status"`
			Count  int64  `json:"count"`
		} `json:"status_breakdown"`
		DailyTrend []struct {
			Date      string `json:"date"`
			Calls     int64  `json:"calls"`
			Succeeded int64  `json:"succeeded"`
			Failed    int64  `json:"failed"`
		} `json:"daily_trend"`
		RecentGenerations []struct {
			ID            uint   `json:"id"`
			Model         string `json:"model"`
			Status        string `json:"status"`
			PromptSummary string `json:"prompt_summary"`
		} `json:"recent_generations"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode model detail payload: %v", err)
	}
	if payload.Model.ID != dalle.ID || payload.Model.Name != "DALL-E 3" || payload.Model.RuntimeModel != "gpt-image-2" {
		t.Fatalf("unexpected model detail header: %+v", payload.Model)
	}
	if payload.Usage.TotalCalls != 2 || payload.Usage.SucceededCalls != 1 || payload.Usage.FailedCalls != 1 || payload.Usage.AverageLatency != 900 {
		t.Fatalf("unexpected detail usage stats: %+v", payload.Usage)
	}
	if len(payload.DailyTrend) != 14 {
		t.Fatalf("expected 14 day trend, got %d", len(payload.DailyTrend))
	}
	if len(payload.RecentGenerations) != 2 || payload.RecentGenerations[0].Model != "gpt-image-2" || payload.RecentGenerations[0].PromptSummary == "" {
		t.Fatalf("unexpected recent generations: %+v", payload.RecentGenerations)
	}

	missing := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models/999999", nil, adminCookies)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected missing model 404, got %d", missing.Code)
	}
}

func TestAdminModelConfigsCreateAndPartialUpdate(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/models", map[string]any{
		"name":          "Test Video",
		"type":          "video",
		"provider":      "Test Labs",
		"status":        "online",
		"priority":      5,
		"cost_label":    "12 点/次",
		"permission":    "internal",
		"weight":        0,
		"sort_order":    90,
		"api_base_url":  "https://api.vendor.example",
		"api_endpoint":  "/v1/videos/generations",
		"api_key":       "secret-key",
		"runtime_model": "",
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create model 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID          uint   `json:"id"`
		Name        string `json:"name"`
		Provider    string `json:"provider"`
		APIBaseURL  string `json:"api_base_url"`
		APIEndpoint string `json:"api_endpoint"`
		APIKey      string `json:"api_key"`
		APIKeySet   bool   `json:"api_key_set"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created model: %v", err)
	}
	if created.ID == 0 || created.Provider != "Test Labs" || created.APIBaseURL != "https://api.vendor.example" || created.APIEndpoint != "/v1/videos/generations" || !created.APIKeySet || created.APIKey != "" {
		t.Fatalf("unexpected created model: %+v", created)
	}

	updateResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/models/"+itoa(created.ID), map[string]any{
		"status":   "offline",
		"priority": 6,
		"api_key":  "rotated-key",
	}, adminCookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update model 200, got %d: %s", updateResp.Code, updateResp.Body.String())
	}

	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models?q=Test%20Video", nil, adminCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list updated model 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Items []struct {
			Provider string `json:"provider"`
			Status   string `json:"status"`
			Priority int    `json:"priority"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode updated model list: %v", err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].Provider != "Test Labs" || listPayload.Items[0].Status != "offline" || listPayload.Items[0].Priority != 6 {
		t.Fatalf("expected partial update to preserve unspecified fields, got %+v", listPayload.Items)
	}
	var stored ModelConfig
	if err := testApp.db.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("load stored model: %v", err)
	}
	if stored.APIKey != "rotated-key" {
		t.Fatalf("expected rotated API key stored without returning plaintext, got %q", stored.APIKey)
	}

	invalidResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/models", map[string]any{
		"name":       "Broken",
		"type":       "text",
		"provider":   "Test Labs",
		"permission": "public",
	}, adminCookies)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid model type 400, got %d: %s", invalidResp.Code, invalidResp.Body.String())
	}
}

func TestAdminModelConfigBlocksDoubaoPublicWithoutAPIKey(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var doubao ModelConfig
	if err := db.Where("runtime_model = ?", "doubao-seed-2-0-mini-260428").First(&doubao).Error; err != nil {
		t.Fatalf("load Doubao model: %v", err)
	}

	blockedResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/models/"+itoa(doubao.ID), map[string]any{
		"permission": "public",
	}, adminCookies)
	if blockedResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected Doubao publish without key 422, got %d: %s", blockedResp.Code, blockedResp.Body.String())
	}
	if !strings.Contains(blockedResp.Body.String(), "video_model_publish_key_missing") || !strings.Contains(blockedResp.Body.String(), "ARK_API_KEY") {
		t.Fatalf("expected publish key readiness error, got %s", blockedResp.Body.String())
	}

	publishResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/models/"+itoa(doubao.ID), map[string]any{
		"permission": "public",
		"api_key":    "ark-admin-key",
	}, adminCookies)
	if publishResp.Code != http.StatusOK {
		t.Fatalf("expected Doubao publish with key 200, got %d: %s", publishResp.Code, publishResp.Body.String())
	}
	var published struct {
		Permission string `json:"permission"`
		APIKey     string `json:"api_key"`
		APIKeySet  bool   `json:"api_key_set"`
	}
	if err := json.Unmarshal(publishResp.Body.Bytes(), &published); err != nil {
		t.Fatalf("decode published Doubao model: %v", err)
	}
	if published.Permission != ModelConfigPermissionPublic || !published.APIKeySet || published.APIKey != "" {
		t.Fatalf("expected public Doubao with masked key status, got %+v", published)
	}
}

func TestAdminModelConfigBlocksZZPublicWithoutAPIKey(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var zz ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&zz).Error; err != nil {
		t.Fatalf("load ZZ video model: %v", err)
	}

	blockedResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/models/"+itoa(zz.ID), map[string]any{
		"permission": ModelConfigPermissionPublic,
	}, adminCookies)
	if blockedResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected ZZ publish without key 422, got %d: %s", blockedResp.Code, blockedResp.Body.String())
	}
	if !strings.Contains(blockedResp.Body.String(), "video_model_publish_key_missing") || !strings.Contains(blockedResp.Body.String(), "ZZ_API_KEY") {
		t.Fatalf("expected ZZ key readiness error, got %s", blockedResp.Body.String())
	}

	publishResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/models/"+itoa(zz.ID), map[string]any{
		"permission": ModelConfigPermissionPublic,
		"api_key":    "zz-admin-key",
	}, adminCookies)
	if publishResp.Code != http.StatusOK {
		t.Fatalf("expected ZZ publish with key 200, got %d: %s", publishResp.Code, publishResp.Body.String())
	}
	var published struct {
		Permission string `json:"permission"`
		APIKey     string `json:"api_key"`
		APIKeySet  bool   `json:"api_key_set"`
	}
	if err := json.Unmarshal(publishResp.Body.Bytes(), &published); err != nil {
		t.Fatalf("decode published ZZ model: %v", err)
	}
	if published.Permission != ModelConfigPermissionPublic || !published.APIKeySet || published.APIKey != "" {
		t.Fatalf("expected public ZZ with masked key status, got %+v", published)
	}
}

func TestAdminModelConfigPublishesZZWithModelCenterProviderKey(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var zz ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&zz).Error; err != nil {
		t.Fatalf("load ZZ video model: %v", err)
	}
	setVideoModelCenterProviderKey(t, testApp, zz.ID, "ZZ API", "zz", "https://model-center.zz.example", "zz-provider-key")

	publishResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/models/"+itoa(zz.ID), map[string]any{
		"permission": ModelConfigPermissionPublic,
	}, adminCookies)
	if publishResp.Code != http.StatusOK {
		t.Fatalf("expected ZZ publish with model center provider key 200, got %d: %s", publishResp.Code, publishResp.Body.String())
	}
	var published struct {
		Permission string `json:"permission"`
		APIKey     string `json:"api_key"`
		APIKeySet  bool   `json:"api_key_set"`
	}
	if err := json.Unmarshal(publishResp.Body.Bytes(), &published); err != nil {
		t.Fatalf("decode published ZZ model: %v", err)
	}
	if published.Permission != ModelConfigPermissionPublic || !published.APIKeySet || published.APIKey != "" {
		t.Fatalf("expected public ZZ with masked model center key status, got %+v", published)
	}
}

func TestAdminModelConfigsDeleteModelAndPreserveDeletionAcrossSeed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	cfg := testConfig(t)
	cfg.AssetStoragePath = filepath.Join(t.TempDir(), "assets")
	testApp, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("new test app: %v", err)
	}
	adminCookies := createAdminSession(t, testApp)

	var defaultImage ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&defaultImage).Error; err != nil {
		t.Fatalf("load default image model: %v", err)
	}
	defaultDeleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(defaultImage.ID), nil, adminCookies)
	if defaultDeleteResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected default model delete blocked, got %d: %s", defaultDeleteResp.Code, defaultDeleteResp.Body.String())
	}

	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/models", map[string]any{
		"name":          "Temporary GPT-Best",
		"type":          "image",
		"provider":      "GPT-Best",
		"status":        "offline",
		"priority":      5,
		"cost_label":    "1 点/次",
		"permission":    "public",
		"weight":        0,
		"sort_order":    95,
		"runtime_model": "temporary-gpt-best",
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected custom model create 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created model: %v", err)
	}
	var settings AppSettings
	if err := testApp.db.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if err := ensureAllowedImageModel(&settings, "temporary-gpt-best"); err != nil {
		t.Fatalf("add allowed image model: %v", err)
	}
	if err := testApp.db.Save(&settings).Error; err != nil {
		t.Fatalf("save settings: %v", err)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(created.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected custom model delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	missingResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/models/"+itoa(created.ID), nil, adminCookies)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected deleted model hidden from detail, got %d: %s", missingResp.Code, missingResp.Body.String())
	}
	var reloadedSettings AppSettings
	if err := testApp.db.First(&reloadedSettings, 1).Error; err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if contains(reloadedSettings.AllowedImageModels(), "temporary-gpt-best") {
		t.Fatalf("expected deleted runtime removed from allowed image models, got %v", reloadedSettings.AllowedImageModels())
	}

	var flux ModelConfig
	if err := testApp.db.Where("name = ?", "FLUX.1 [dev]").First(&flux).Error; err != nil {
		t.Fatalf("load seeded non-default model: %v", err)
	}
	seedDeleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(flux.ID), nil, adminCookies)
	if seedDeleteResp.Code != http.StatusOK {
		t.Fatalf("expected seeded non-default model delete 200, got %d: %s", seedDeleteResp.Code, seedDeleteResp.Body.String())
	}
	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("new app after delete: %v", err)
	}
	var fluxCount int64
	if err := db.Model(&ModelConfig{}).Where("name = ?", "FLUX.1 [dev]").Count(&fluxCount).Error; err != nil {
		t.Fatalf("count flux models: %v", err)
	}
	if fluxCount != 0 {
		t.Fatalf("expected deleted seeded model not to be restored on startup, got %d", fluxCount)
	}
}

func TestAdminModelConfigsDeleteDuplicateRuntimeModelNotBlockedByActiveRuntime(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	duplicate := ModelConfig{
		Name:         "生图模型2 ithinkai",
		Type:         ModelConfigTypeImage,
		Provider:     "IThinkAI",
		Status:       ModelConfigStatusOnline,
		Priority:     4,
		CostLabel:    "1 点/次",
		Permission:   ModelConfigPermissionPublic,
		Weight:       0,
		SortOrder:    35,
		RuntimeModel: "gpt-image-2",
		APIBaseURL:   "https://ithinkai.example",
		APIEndpoint:  "/v1/images/generations",
	}
	if err := testApp.db.Create(&duplicate).Error; err != nil {
		t.Fatalf("create duplicate runtime image model: %v", err)
	}
	var settings AppSettings
	if err := testApp.db.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	settings.ActiveImageModel = "gpt-image-2"
	if err := ensureAllowedImageModel(&settings, "gpt-image-2"); err != nil {
		t.Fatalf("ensure active runtime is allowed: %v", err)
	}
	if err := testApp.db.Save(&settings).Error; err != nil {
		t.Fatalf("save settings: %v", err)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(duplicate.ID), nil, adminCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected non-routed duplicate runtime model delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	var reloadedSettings AppSettings
	if err := testApp.db.First(&reloadedSettings, 1).Error; err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if !contains(reloadedSettings.AllowedImageModels(), "gpt-image-2") {
		t.Fatalf("expected shared runtime to remain allowed while another model uses it, got %v", reloadedSettings.AllowedImageModels())
	}
}

func TestAdminModelConfigsForceDeleteReassignsImageDefaults(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var target ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&target).Error; err != nil {
		t.Fatalf("load default image model: %v", err)
	}

	normalDeleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(target.ID), nil, adminCookies)
	if normalDeleteResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected normal delete to keep protecting defaults, got %d: %s", normalDeleteResp.Code, normalDeleteResp.Body.String())
	}

	forceDeleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(target.ID)+"?force=true", nil, adminCookies)
	if forceDeleteResp.Code != http.StatusOK {
		t.Fatalf("expected force delete default image model 200, got %d: %s", forceDeleteResp.Code, forceDeleteResp.Body.String())
	}

	var deleted ModelConfig
	if err := testApp.db.Unscoped().First(&deleted, target.ID).Error; err != nil {
		t.Fatalf("load soft-deleted model: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected force-deleted model to be soft deleted, got %+v", deleted)
	}

	var settings AppSettings
	if err := testApp.db.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.DefaultImageModelID == nil || *settings.DefaultImageModelID == target.ID {
		t.Fatalf("expected default image model reassigned, got %+v", settings)
	}
	if strings.TrimSpace(settings.ActiveImageModel) == "" || settings.ActiveImageModel == target.RuntimeModel {
		t.Fatalf("expected active image model reassigned, got %+v", settings)
	}
	if contains(settings.AllowedImageModels(), target.RuntimeModel) || contains(settings.AllowedImageModels(), target.Name) {
		t.Fatalf("expected deleted image model removed from allowed list, got %v", settings.AllowedImageModels())
	}
	if !contains(settings.AllowedImageModels(), settings.ActiveImageModel) {
		t.Fatalf("expected replacement active model added to allowed list, got active=%q allowed=%v", settings.ActiveImageModel, settings.AllowedImageModels())
	}
}

func TestAdminModelConfigsForceDeletedDefaultDoesNotBreakStartupSeed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	cfg := testConfig(t)
	cfg.AssetStoragePath = filepath.Join(t.TempDir(), "assets")
	testApp, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("new test app: %v", err)
	}
	adminCookies := createAdminSession(t, testApp)

	var target ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&target).Error; err != nil {
		t.Fatalf("load default image model: %v", err)
	}
	forceDeleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(target.ID)+"?force=true", nil, adminCookies)
	if forceDeleteResp.Code != http.StatusOK {
		t.Fatalf("expected force delete default image model 200, got %d: %s", forceDeleteResp.Code, forceDeleteResp.Body.String())
	}

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("expected app startup after force delete default image model, got %v", err)
	}

	var activeCount int64
	if err := db.Model(&ModelConfig{}).Where("name = ?", "DALL-E 3").Count(&activeCount).Error; err != nil {
		t.Fatalf("count active DALL-E 3 models: %v", err)
	}
	if activeCount != 0 {
		t.Fatalf("expected force-deleted DALL-E 3 not restored on startup, got %d active rows", activeCount)
	}
}

func TestAdminModelConfigsForceDeleteReassignsFallbackAndVideoDefaults(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var settings AppSettings
	if err := testApp.db.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	fallbackID := uintPointerValue(settings.FallbackModelID)
	videoID := uintPointerValue(settings.DefaultVideoModelID)
	if fallbackID == 0 || videoID == 0 {
		t.Fatalf("expected seeded fallback and video defaults, got %+v", settings)
	}

	fallbackResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(fallbackID)+"?force=true", nil, adminCookies)
	if fallbackResp.Code != http.StatusOK {
		t.Fatalf("expected force delete fallback image model 200, got %d: %s", fallbackResp.Code, fallbackResp.Body.String())
	}
	var afterFallback AppSettings
	if err := testApp.db.First(&afterFallback, 1).Error; err != nil {
		t.Fatalf("load settings after fallback delete: %v", err)
	}
	if afterFallback.FallbackModelID == nil || *afterFallback.FallbackModelID == fallbackID {
		t.Fatalf("expected fallback model reassigned, got %+v", afterFallback)
	}

	videoResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(videoID)+"?force=true", nil, adminCookies)
	if videoResp.Code != http.StatusOK {
		t.Fatalf("expected force delete default video model 200, got %d: %s", videoResp.Code, videoResp.Body.String())
	}
	var afterVideo AppSettings
	if err := testApp.db.First(&afterVideo, 1).Error; err != nil {
		t.Fatalf("load settings after video delete: %v", err)
	}
	if afterVideo.DefaultVideoModelID == nil || *afterVideo.DefaultVideoModelID == videoID {
		t.Fatalf("expected default video model reassigned, got %+v", afterVideo)
	}
	var replacementVideo ModelConfig
	if err := testApp.db.First(&replacementVideo, *afterVideo.DefaultVideoModelID).Error; err != nil {
		t.Fatalf("load replacement video model: %v", err)
	}
	if replacementVideo.Type != ModelConfigTypeVideo {
		t.Fatalf("expected video replacement, got %+v", replacementVideo)
	}
}

func TestAdminModelConfigsForceDeleteRequiresReplacement(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var target ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&target).Error; err != nil {
		t.Fatalf("load default image model: %v", err)
	}
	if err := testApp.db.Where("type = ? AND id <> ?", ModelConfigTypeImage, target.ID).Delete(&ModelConfig{}).Error; err != nil {
		t.Fatalf("delete image replacements: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodDelete, "/api/admin/models/"+itoa(target.ID)+"?force=true", nil, adminCookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected missing replacement 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "model_replacement_required") {
		t.Fatalf("expected model_replacement_required response, got %s", resp.Body.String())
	}
	var stillPresent ModelConfig
	if err := testApp.db.First(&stillPresent, target.ID).Error; err != nil {
		t.Fatalf("expected target model not deleted: %v", err)
	}
}

func TestAdminModelRoutingSavesDefaultsWeightsAndCompatibilityModel(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	routeResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/model-routing", nil, adminCookies)
	if routeResp.Code != http.StatusOK {
		t.Fatalf("expected model routing 200, got %d: %s", routeResp.Code, routeResp.Body.String())
	}
	var routing struct {
		DefaultImageModelID uint   `json:"default_image_model_id"`
		DefaultVideoModelID uint   `json:"default_video_model_id"`
		FallbackModelID     uint   `json:"fallback_model_id"`
		RoutingEnabled      bool   `json:"routing_enabled"`
		RoutingStrategy     string `json:"routing_strategy"`
		ConcurrencyLimit    int    `json:"concurrency_limit"`
		ImageModels         []struct {
			ID           uint   `json:"id"`
			Name         string `json:"name"`
			Type         string `json:"type"`
			RuntimeModel string `json:"runtime_model"`
			Weight       int    `json:"weight"`
		} `json:"image_models"`
		VideoModels []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"video_models"`
	}
	if err := json.Unmarshal(routeResp.Body.Bytes(), &routing); err != nil {
		t.Fatalf("decode routing payload: %v", err)
	}
	if routing.DefaultImageModelID == 0 || routing.DefaultVideoModelID == 0 || routing.FallbackModelID == 0 || len(routing.ImageModels) == 0 || len(routing.VideoModels) == 0 {
		t.Fatalf("expected seeded routing options, got %+v", routing)
	}
	if routing.RoutingStrategy != "default" {
		t.Fatalf("expected default routing strategy, got %+v", routing)
	}

	var dallID, sdxlID, soraID, grokID uint
	for _, model := range routing.ImageModels {
		if model.Name == "DALL-E 3" {
			dallID = model.ID
		}
		if model.Name == "SDXL 1.0" {
			sdxlID = model.ID
		}
	}
	for _, model := range routing.VideoModels {
		if model.Name == "Sora2" {
			soraID = model.ID
		}
		if model.Name == "Grok Imagine" {
			grokID = model.ID
		}
	}
	if dallID == 0 || sdxlID == 0 || soraID == 0 || grokID == 0 {
		t.Fatalf("expected seed model ids, got routing %+v", routing)
	}
	if routing.DefaultVideoModelID != grokID {
		t.Fatalf("expected Grok Imagine as default video model, got routing %+v", routing)
	}

	weights := make([]map[string]any, 0, len(routing.ImageModels))
	for i, model := range routing.ImageModels {
		weight := 0
		if i == 0 {
			weight = 100
		}
		weights = append(weights, map[string]any{"id": model.ID, "weight": weight})
	}
	saveResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-routing", map[string]any{
		"default_image_model_id": dallID,
		"default_video_model_id": soraID,
		"fallback_model_id":      sdxlID,
		"routing_enabled":        false,
		"routing_strategy":       "speed_first",
		"concurrency_limit":      7,
		"image_weights":          weights,
	}, adminCookies)
	if saveResp.Code != http.StatusOK {
		t.Fatalf("expected model routing save 200, got %d: %s", saveResp.Code, saveResp.Body.String())
	}

	reloadRoutingResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/model-routing", nil, adminCookies)
	if reloadRoutingResp.Code != http.StatusOK {
		t.Fatalf("expected reloaded model routing 200, got %d: %s", reloadRoutingResp.Code, reloadRoutingResp.Body.String())
	}
	var reloadedRouting struct {
		DefaultImageModelID uint   `json:"default_image_model_id"`
		DefaultVideoModelID uint   `json:"default_video_model_id"`
		FallbackModelID     uint   `json:"fallback_model_id"`
		RoutingEnabled      bool   `json:"routing_enabled"`
		RoutingStrategy     string `json:"routing_strategy"`
		ConcurrencyLimit    int    `json:"concurrency_limit"`
	}
	if err := json.Unmarshal(reloadRoutingResp.Body.Bytes(), &reloadedRouting); err != nil {
		t.Fatalf("decode reloaded routing payload: %v", err)
	}
	if reloadedRouting.DefaultImageModelID != dallID || reloadedRouting.DefaultVideoModelID != soraID || reloadedRouting.FallbackModelID != sdxlID {
		t.Fatalf("expected saved model ids, got %+v", reloadedRouting)
	}
	if reloadedRouting.RoutingEnabled || reloadedRouting.ConcurrencyLimit != 7 {
		t.Fatalf("expected saved routing state and concurrency, got %+v", reloadedRouting)
	}
	if reloadedRouting.RoutingStrategy != "speed_first" {
		t.Fatalf("expected saved routing strategy, got %+v", reloadedRouting)
	}

	imageSettingsResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/settings/image", nil, adminCookies)
	if imageSettingsResp.Code != http.StatusOK {
		t.Fatalf("expected image settings 200, got %d: %s", imageSettingsResp.Code, imageSettingsResp.Body.String())
	}
	var imageSettings struct {
		ActiveModel string `json:"active_model"`
	}
	if err := json.Unmarshal(imageSettingsResp.Body.Bytes(), &imageSettings); err != nil {
		t.Fatalf("decode image settings: %v", err)
	}
	if imageSettings.ActiveModel != "gpt-image-2" {
		t.Fatalf("expected runtime-compatible active image model preserved, got %s", imageSettings.ActiveModel)
	}

	typeMismatchResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-routing", map[string]any{
		"default_image_model_id": dallID,
		"default_video_model_id": dallID,
		"fallback_model_id":      sdxlID,
		"concurrency_limit":      7,
		"image_weights":          weights,
	}, adminCookies)
	if typeMismatchResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected video default type mismatch 422, got %d: %s", typeMismatchResp.Code, typeMismatchResp.Body.String())
	}

	badWeights := append([]map[string]any(nil), weights...)
	badWeights[0] = map[string]any{"id": routing.ImageModels[0].ID, "weight": 99}
	badWeightsResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-routing", map[string]any{
		"default_image_model_id": dallID,
		"default_video_model_id": soraID,
		"fallback_model_id":      sdxlID,
		"concurrency_limit":      7,
		"image_weights":          badWeights,
	}, adminCookies)
	if badWeightsResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected bad weights 422, got %d: %s", badWeightsResp.Code, badWeightsResp.Body.String())
	}

	badConcurrencyResp := performJSONRequest(t, testApp, http.MethodPut, "/api/admin/model-routing", map[string]any{
		"default_image_model_id": dallID,
		"default_video_model_id": soraID,
		"fallback_model_id":      sdxlID,
		"concurrency_limit":      0,
		"image_weights":          weights,
	}, adminCookies)
	if badConcurrencyResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected bad concurrency 422, got %d: %s", badConcurrencyResp.Code, badConcurrencyResp.Body.String())
	}
}

func TestListVideoModelsReturnsPublicCapabilities(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")

	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			Name           string   `json:"name"`
			RuntimeModel   string   `json:"runtime_model"`
			Provider       string   `json:"provider"`
			Permission     string   `json:"permission"`
			Available      bool     `json:"available"`
			DisabledReason string   `json:"disabled_reason"`
			APIKeySet      bool     `json:"api_key_set"`
			AspectRatios   []string `json:"aspect_ratios"`
			Durations      []string `json:"durations"`
			SupportsHD     bool     `json:"supports_hd"`
			MaxReferences  int      `json:"max_reference_images"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	var internalDoubao *struct {
		Name           string   `json:"name"`
		RuntimeModel   string   `json:"runtime_model"`
		Provider       string   `json:"provider"`
		Permission     string   `json:"permission"`
		Available      bool     `json:"available"`
		DisabledReason string   `json:"disabled_reason"`
		APIKeySet      bool     `json:"api_key_set"`
		AspectRatios   []string `json:"aspect_ratios"`
		Durations      []string `json:"durations"`
		SupportsHD     bool     `json:"supports_hd"`
		MaxReferences  int      `json:"max_reference_images"`
	}
	for _, item := range payload.Items {
		if item.RuntimeModel == "doubao-seed-2-0-mini-260428" {
			internalDoubao = &item
			break
		}
	}
	if internalDoubao == nil {
		t.Fatalf("expected internal Doubao model to be visible as disabled: %+v", payload.Items)
	}
	if internalDoubao.Available || internalDoubao.Permission != ModelConfigPermissionInternal || internalDoubao.APIKeySet {
		t.Fatalf("expected internal Doubao to be disabled without exposing a key, got %+v", *internalDoubao)
	}
	if !strings.Contains(internalDoubao.DisabledReason, "\u5185\u6d4b") || !strings.Contains(internalDoubao.DisabledReason, "\u706b\u5c71\u65b9\u821f") {
		t.Fatalf("expected internal/key readiness reason for Doubao, got %q", internalDoubao.DisabledReason)
	}

	if err := testApp.db.Model(&ModelConfig{}).
		Where("runtime_model = ?", "doubao-seed-2-0-mini-260428").
		Updates(map[string]any{
			"permission": ModelConfigPermissionPublic,
			"api_key":    "ark-test-key",
		}).Error; err != nil {
		t.Fatalf("publish Doubao model: %v", err)
	}
	resp = performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models after publishing Doubao 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload.Items = nil
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode published video models payload: %v", err)
	}
	var doubaoAwaitingReadiness *struct {
		Name           string   `json:"name"`
		RuntimeModel   string   `json:"runtime_model"`
		Provider       string   `json:"provider"`
		Permission     string   `json:"permission"`
		Available      bool     `json:"available"`
		DisabledReason string   `json:"disabled_reason"`
		APIKeySet      bool     `json:"api_key_set"`
		AspectRatios   []string `json:"aspect_ratios"`
		Durations      []string `json:"durations"`
		SupportsHD     bool     `json:"supports_hd"`
		MaxReferences  int      `json:"max_reference_images"`
	}
	for index := range payload.Items {
		if payload.Items[index].RuntimeModel == "doubao-seed-2-0-mini-260428" {
			doubaoAwaitingReadiness = &payload.Items[index]
			break
		}
	}
	if doubaoAwaitingReadiness == nil {
		t.Fatalf("expected published Doubao model in payload: %+v", payload.Items)
	}
	if doubaoAwaitingReadiness.Provider != "Volcengine Ark" ||
		doubaoAwaitingReadiness.Permission != ModelConfigPermissionPublic ||
		doubaoAwaitingReadiness.Available ||
		!strings.Contains(doubaoAwaitingReadiness.DisabledReason, "不支持视频生成 API") ||
		!doubaoAwaitingReadiness.APIKeySet {
		t.Fatalf("expected public/keyed Doubao to stay disabled after failed readiness, got %+v", *doubaoAwaitingReadiness)
	}

	if err := testApp.db.Model(&ModelConfig{}).
		Where("runtime_model = ?", "doubao-seed-2-0-mini-260428").
		Updates(map[string]any{
			"video_readiness_status":     "passed",
			"video_readiness_reason":     "",
			"video_readiness_checked_at": time.Now(),
		}).Error; err != nil {
		t.Fatalf("mark Doubao readiness passed: %v", err)
	}
	resp = performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models after Doubao readiness passed 200, got %d: %s", resp.Code, resp.Body.String())
	}
	payload.Items = nil
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode ready video models payload: %v", err)
	}
	var doubao *struct {
		Name           string   `json:"name"`
		RuntimeModel   string   `json:"runtime_model"`
		Provider       string   `json:"provider"`
		Permission     string   `json:"permission"`
		Available      bool     `json:"available"`
		DisabledReason string   `json:"disabled_reason"`
		APIKeySet      bool     `json:"api_key_set"`
		AspectRatios   []string `json:"aspect_ratios"`
		Durations      []string `json:"durations"`
		SupportsHD     bool     `json:"supports_hd"`
		MaxReferences  int      `json:"max_reference_images"`
	}
	for index := range payload.Items {
		if payload.Items[index].RuntimeModel == "doubao-seed-2-0-mini-260428" {
			doubao = &payload.Items[index]
			break
		}
	}
	if doubao == nil {
		t.Fatalf("expected ready Doubao model in payload: %+v", payload.Items)
	}
	if doubao.Provider != "Volcengine Ark" ||
		doubao.Permission != ModelConfigPermissionPublic ||
		!doubao.Available ||
		doubao.DisabledReason != "" ||
		!doubao.APIKeySet ||
		doubao.SupportsHD != true ||
		doubao.MaxReferences != 9 ||
		!reflect.DeepEqual(doubao.Durations, []string{"4", "5", "6", "8", "10", "12", "15", "-1"}) ||
		!reflect.DeepEqual(doubao.AspectRatios, []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}) {
		t.Fatalf("unexpected ready Doubao model capabilities: %+v", *doubao)
	}
}

func TestListVideoModelsReturnsWuyinShortDurations(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel string   `json:"runtime_model"`
			Durations    []string `json:"durations"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel != wuyinGrokImagineRuntimeModel {
			continue
		}
		if !reflect.DeepEqual(item.Durations, []string{"1", "3", "6", "10", "15"}) {
			t.Fatalf("expected Wuyin short durations, got %+v", item.Durations)
		}
		return
	}
	t.Fatalf("expected Wuyin Grok model in payload: %+v", payload.Items)
}

func TestListVideoModelsReturnsReferenceImageRequirement(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	if err := testApp.db.Model(&ModelConfig{}).
		Where("runtime_model = ?", arkSeedanceMiniRuntimeModel).
		Updates(map[string]any{
			"permission":                 ModelConfigPermissionPublic,
			"api_key":                    "ark-test-key",
			"video_readiness_status":     "passed",
			"video_readiness_reason":     "",
			"video_readiness_checked_at": time.Now(),
		}).Error; err != nil {
		t.Fatalf("publish Doubao model: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel           string `json:"runtime_model"`
			RequiresReferenceImage bool   `json:"requires_reference_image"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}

	requiresByModel := map[string]bool{}
	for _, item := range payload.Items {
		requiresByModel[item.RuntimeModel] = item.RequiresReferenceImage
	}
	if requiresByModel[wuyinGrokImagineRuntimeModel] != true {
		t.Fatalf("expected Wuyin/Grok to require a reference image, got %+v", requiresByModel)
	}
	if requiresByModel[arkSeedanceMiniRuntimeModel] != false {
		t.Fatalf("expected Seedance to allow text-to-video, got %+v", requiresByModel)
	}
	if requiresByModel["sora-2"] != false {
		t.Fatalf("expected Sora to allow text-to-video, got %+v", requiresByModel)
	}
}

func TestListVideoModelsReturnsSeedance2MultimodalCapabilities(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})

	if err := testApp.db.Model(&ModelConfig{}).
		Where("runtime_model = ?", arkSeedance2RuntimeModel).
		Updates(map[string]any{
			"permission":                 ModelConfigPermissionPublic,
			"api_key":                    "ark-test-key",
			"video_readiness_status":     arkVideoReadinessStatusPassed,
			"video_readiness_reason":     "",
			"video_readiness_checked_at": time.Now(),
		}).Error; err != nil {
		t.Fatalf("publish Seedance 2.0 model: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel           string   `json:"runtime_model"`
			Durations              []string `json:"durations"`
			MaxReferenceImages     int      `json:"max_reference_images"`
			SupportsReferenceVideo bool     `json:"supports_reference_video"`
			SupportsReferenceAudio bool     `json:"supports_reference_audio"`
			MaxReferenceVideos     int      `json:"max_reference_videos"`
			MaxReferenceAudios     int      `json:"max_reference_audios"`
			SupportsGenerateAudio  bool     `json:"supports_generate_audio"`
			RequiresReferenceImage bool     `json:"requires_reference_image"`
			ResolutionOptions      []string `json:"resolution_options"`
			DefaultResolution      string   `json:"default_resolution"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel != arkSeedance2RuntimeModel {
			continue
		}
		if !item.SupportsReferenceVideo ||
			!item.SupportsReferenceAudio ||
			item.MaxReferenceImages != 9 ||
			item.MaxReferenceVideos != 3 ||
			item.MaxReferenceAudios != 3 ||
			!item.SupportsGenerateAudio ||
			item.RequiresReferenceImage ||
			item.DefaultResolution != "720p" ||
			!reflect.DeepEqual(item.ResolutionOptions, []string{"720p", "1080p"}) ||
			!reflect.DeepEqual(item.Durations, []string{"4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "-1"}) {
			t.Fatalf("unexpected Seedance 2.0 multimodal capabilities: %+v", item)
		}
		return
	}
	t.Fatalf("expected Seedance 2.0 in video models payload: %+v", payload.Items)
}

func TestListVideoModelsReturnsZZCapabilitiesWhenKeyed(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "zz-env-key")
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			Name                   string           `json:"name"`
			RuntimeModel           string           `json:"runtime_model"`
			Permission             string           `json:"permission"`
			Available              bool             `json:"available"`
			APIKeySet              bool             `json:"api_key_set"`
			AspectRatios           []string         `json:"aspect_ratios"`
			Durations              []string         `json:"durations"`
			ResolutionOptions      []string         `json:"resolution_options"`
			DefaultResolution      string           `json:"default_resolution"`
			PriceRules             []videoPriceRule `json:"price_rules"`
			MaxReferenceImages     int              `json:"max_reference_images"`
			SupportsReferenceVideo bool             `json:"supports_reference_video"`
			SupportsReferenceAudio bool             `json:"supports_reference_audio"`
			MaxReferenceVideos     int              `json:"max_reference_videos"`
			MaxReferenceAudios     int              `json:"max_reference_audios"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel != zzVideoDSFastRuntimeModel {
			continue
		}
		if item.Name != "DS 2.0" ||
			item.Permission != ModelConfigPermissionPublic ||
			!item.Available ||
			!item.APIKeySet ||
			!reflect.DeepEqual(item.AspectRatios, []string{"16:9", "9:16", "1:1"}) ||
			!reflect.DeepEqual(item.Durations, []string{"15"}) ||
			!reflect.DeepEqual(item.ResolutionOptions, []string{"480p", "720p"}) ||
			item.DefaultResolution != "480p" ||
			len(item.PriceRules) != 2 ||
			item.PriceRules[0].Resolution != "480p" ||
			item.PriceRules[0].CreditsPerSecond != 18 ||
			item.PriceRules[1].Resolution != "720p" ||
			item.PriceRules[1].CreditsPerSecond != 24 ||
			item.MaxReferenceImages != 4 ||
			!item.SupportsReferenceVideo ||
			!item.SupportsReferenceAudio ||
			item.MaxReferenceVideos != 3 ||
			item.MaxReferenceAudios != 3 {
			t.Fatalf("unexpected ZZ capabilities: %+v", item)
		}
		return
	}
	t.Fatalf("expected ZZ video model in payload: %+v", payload.Items)
}

func TestListVideoModelsHidesZZWhenKeyMissing(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")
	testApp, _ := newTestApp(t, &stubProvider{})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), zzVideoDSFastRuntimeModel) {
		t.Fatalf("expected ZZ model hidden without key, got %s", resp.Body.String())
	}
}

func TestListVideoModelsReturnsAvailableZZFromModelCenterProviderKey(t *testing.T) {
	t.Setenv("ZZ_API_KEY", "")

	testApp, db := newTestApp(t, &stubProvider{})
	var zz ModelConfig
	if err := db.Where("runtime_model = ?", zzVideoDSFastRuntimeModel).First(&zz).Error; err != nil {
		t.Fatalf("load ZZ video model: %v", err)
	}
	if strings.TrimSpace(zz.APIKey) != "" {
		t.Fatalf("test expects legacy ZZ key to be empty")
	}
	if err := db.Model(&zz).Update("permission", ModelConfigPermissionPublic).Error; err != nil {
		t.Fatalf("publish ZZ model: %v", err)
	}
	setVideoModelCenterProviderKey(t, testApp, zz.ID, "ZZ API", "zz", "https://model-center.zz.example", "zz-provider-key")

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel string `json:"runtime_model"`
			Available    bool   `json:"available"`
			APIKeySet    bool   `json:"api_key_set"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel == zzVideoDSFastRuntimeModel {
			if !item.Available || !item.APIKeySet {
				t.Fatalf("expected ZZ available from model center provider key, got %+v", item)
			}
			return
		}
	}
	t.Fatalf("expected ZZ model in video models payload: %+v", payload.Items)
}

func TestListVideoModelsMarksWuyinAPIKeySetFromModelCenterProvider(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	if strings.TrimSpace(grok.APIKey) != "" {
		t.Fatalf("test expects legacy Grok key to be empty")
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "list-models-wuyin-key")

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel string `json:"runtime_model"`
			APIKeySet    bool   `json:"api_key_set"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel == wuyinGrokImagineRuntimeModel {
			if !item.APIKeySet {
				t.Fatalf("expected Wuyin api_key_set from model center provider, got %+v", item)
			}
			return
		}
	}
	t.Fatalf("expected Grok model in video models payload: %+v", payload.Items)
}

func TestListVideoModelsMarksWuyinAPIKeySetFromKeyedProviderAccount(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	var grok ModelConfig
	if err := db.Where("runtime_model = ?", wuyinGrokImagineRuntimeModel).First(&grok).Error; err != nil {
		t.Fatalf("load Grok model config: %v", err)
	}
	setWuyinModelCenterProviderKey(t, testApp, grok.ID, "")
	keyedProvider := ModelProvider{
		Name:                  "Wuyin",
		Provider:              "wuyin",
		BaseURL:               wuyinGrokImagineProviderBaseURL,
		APIKey:                "list-provider-account-key",
		DefaultTimeoutSeconds: defaultRequestTimeoutSeconds,
		Status:                ModelCenterStatusOnline,
	}
	if err := db.Create(&keyedProvider).Error; err != nil {
		t.Fatalf("create keyed Wuyin provider account: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/videos/models", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected video models 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			RuntimeModel string `json:"runtime_model"`
			APIKeySet    bool   `json:"api_key_set"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode video models payload: %v", err)
	}
	for _, item := range payload.Items {
		if item.RuntimeModel == wuyinGrokImagineRuntimeModel {
			if !item.APIKeySet {
				t.Fatalf("expected Wuyin api_key_set from keyed provider account, got %+v", item)
			}
			return
		}
	}
	t.Fatalf("expected Grok model in video models payload: %+v", payload.Items)
}

func TestAdminDashboardReturnsOperationalPayload(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user, _ := createLoggedInUser(t, testApp, "dashboard_creator", "test-password")

	paidAt := time.Now()
	if err := db.Create(&FinanceOrder{
		OrderNumber:    "FO-DASH-PAID",
		UserID:         user.ID,
		PackageID:      1,
		PackageName:    "创作包",
		PackageCredits: 60,
		AmountCents:    9900,
		OrderType:      FinanceOrderTypePackage,
		PaymentMethod:  FinancePaymentMethodOffline,
		PaymentStatus:  FinancePaymentStatusPaid,
		InvoiceStatus:  FinanceInvoiceStatusPending,
		PaidAt:         &paidAt,
		CreatedAt:      paidAt,
		UpdatedAt:      paidAt,
	}).Error; err != nil {
		t.Fatalf("seed paid finance order: %v", err)
	}
	if err := db.Create(&PurchaseIntent{
		UserID:         user.ID,
		PackageName:    "待处理包",
		PackageCredits: 20,
		PackagePrice:   "39 元",
		Status:         PurchaseIntentStatusSubmitted,
	}).Error; err != nil {
		t.Fatalf("seed pending intent: %v", err)
	}
	if err := db.Create(&Invite{
		Code:       "DASH1234",
		Label:      "概览测试",
		Status:     InviteStatusActive,
		TotalQuota: 12,
		UsedQuota:  3,
	}).Error; err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	now := time.Now()
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:     user.ID,
		Prompt:     "今日趋势",
		Model:      "gpt-image-2",
		Status:     GenerationStatusSucceeded,
		PreviewURL: "/api/works/1/file",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:    user.ID,
		Prompt:    "昨日失败",
		Model:     "gpt-image-2",
		Status:    GenerationStatusFailed,
		CreatedAt: now.AddDate(0, 0, -1),
		UpdatedAt: now.AddDate(0, 0, -1),
	})

	if err := db.Create(&SystemAnnouncement{
		Title:         "维护公告",
		Content:       "今晚 23 点短暂维护",
		Level:         AnnouncementLevelImportant,
		Status:        AnnouncementStatusPublished,
		PublishedAt:   &now,
		CreatedByID:   1,
		CreatedByName: "admin",
	}).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/dashboard", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		UsersTotal          int64 `json:"users_total"`
		GenerationSucceeded int64 `json:"generation_succeeded"`
		KPIs                struct {
			RevenueCompleted string `json:"revenue_completed"`
			GenerationFailed int64  `json:"generation_failed"`
		} `json:"kpis"`
		Packages []Package `json:"packages"`
		Models   []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		} `json:"models"`
		GenerationTrend []struct {
			Date      string `json:"date"`
			Total     int64  `json:"total"`
			Succeeded int64  `json:"succeeded"`
			Failed    int64  `json:"failed"`
		} `json:"generation_trend"`
		InviteSummary struct {
			Active    int64 `json:"active"`
			Remaining int64 `json:"remaining"`
		} `json:"invite_summary"`
		RecentGenerations []struct {
			Prompt     string `json:"prompt"`
			PreviewURL string `json:"preview_url"`
		} `json:"recent_generations"`
		Announcements []struct {
			Title string `json:"title"`
		} `json:"announcements"`
		OperationLogs []struct {
			Action string `json:"action"`
		} `json:"operation_logs"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode dashboard payload: %v", err)
	}
	if payload.UsersTotal == 0 || payload.GenerationSucceeded != 1 {
		t.Fatalf("expected legacy count fields preserved, got %+v", payload)
	}
	if payload.KPIs.RevenueCompleted != "￥99.00" || payload.KPIs.GenerationFailed != 1 {
		t.Fatalf("unexpected kpis: %+v", payload.KPIs)
	}
	if len(payload.Packages) == 0 || len(payload.Models) == 0 {
		t.Fatalf("expected dashboard lists, got packages=%d models=%d", len(payload.Packages), len(payload.Models))
	}
	if len(payload.GenerationTrend) != 30 {
		t.Fatalf("expected 30 trend points, got %d", len(payload.GenerationTrend))
	}
	lastTrend := payload.GenerationTrend[len(payload.GenerationTrend)-1]
	if lastTrend.Total != 1 || lastTrend.Succeeded != 1 {
		t.Fatalf("expected today's trend counts, got %+v", lastTrend)
	}
	if payload.InviteSummary.Active == 0 || payload.InviteSummary.Remaining == 0 {
		t.Fatalf("expected invite summary from seeded invites, got %+v", payload.InviteSummary)
	}
	if len(payload.RecentGenerations) == 0 || payload.RecentGenerations[0].Prompt != "今日趋势" || payload.RecentGenerations[0].PreviewURL == "" {
		t.Fatalf("unexpected recent generations: %+v", payload.RecentGenerations)
	}
	if len(payload.Announcements) == 0 || payload.Announcements[0].Title != "维护公告" {
		t.Fatalf("expected recent announcement, got %+v", payload.Announcements)
	}
	if len(payload.OperationLogs) == 0 {
		t.Fatalf("expected operation logs from admin login")
	}
}

func TestAdminAnnouncementsRequirePermissionsAndWriteAuditLogs(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	admin := createDatabaseAdminUser(t, db, "announcement-admin", "LimitedPass123")
	viewer := createDatabaseAdminUser(t, db, "announcement-viewer", "LimitedPass123")

	role := Role{Code: "announcement_manager", Name: "Announcement manager", Status: RoleStatusActive}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	var permissions []Permission
	if err := db.Where("code IN ?", []string{"announcements.create", "announcements.update"}).Find(&permissions).Error; err != nil {
		t.Fatalf("load announcement permissions: %v", err)
	}
	if len(permissions) != 2 {
		t.Fatalf("expected seeded announcement permissions, got %d", len(permissions))
	}
	if err := db.Model(&role).Association("Permissions").Append(permissions); err != nil {
		t.Fatalf("assign permissions: %v", err)
	}
	if err := db.Model(&admin).Association("Roles").Append(&role); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	viewerCookies := loginAdminAs(t, testApp, viewer.Username, "LimitedPass123")
	viewerCreateResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/announcements", map[string]any{
		"title":   "普通管理员公告",
		"content": "可发布",
		"level":   AnnouncementLevelInfo,
	}, viewerCookies)
	if viewerCreateResp.Code != http.StatusForbidden {
		t.Fatalf("expected create without announcement permission 403, got %d: %s", viewerCreateResp.Code, viewerCreateResp.Body.String())
	}

	adminCookies := loginAdminAs(t, testApp, admin.Username, "LimitedPass123")
	createResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/announcements", map[string]any{
		"title":   "版本更新",
		"content": "新模型已上线",
		"level":   AnnouncementLevelWarning,
	}, adminCookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create announcement 201, got %d: %s", createResp.Code, createResp.Body.String())
	}

	var created SystemAnnouncement
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created announcement: %v", err)
	}
	if created.Status != AnnouncementStatusPublished || created.PublishedAt == nil || created.CreatedByID != admin.ID {
		t.Fatalf("unexpected created announcement: %+v", created)
	}

	statusResp := performJSONRequest(t, testApp, http.MethodPatch, "/api/admin/announcements/"+itoa(created.ID)+"/status", map[string]any{
		"status": AnnouncementStatusOffline,
	}, adminCookies)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected status update 200, got %d: %s", statusResp.Code, statusResp.Body.String())
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).
		Where("target_type = ? AND target_id = ? AND action IN ?", "announcement", created.ID, []string{"announcement.create", "announcement.status.update"}).
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("expected two announcement audit logs, got %d", auditCount)
	}
}

func TestAdminGenerationsListReturnsPaginatedLightweightItemsInStableOrder(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	createdAt := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

	var ids []uint
	for i := 1; i <= 25; i++ {
		workID := uint(i * 10)
		record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
			UserID:          uint(i),
			WorkID:          &workID,
			Model:           "gpt-image-2",
			Status:          GenerationStatusSucceeded,
			LatencyMS:       int64(i * 100),
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
			CreditsDeducted: true,
		})
		ids = append(ids, record.ID)
	}

	pageOneResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?page=1&page_size=20", nil, adminCookies)
	if pageOneResp.Code != http.StatusOK {
		t.Fatalf("expected page 1 status 200, got %d: %s", pageOneResp.Code, pageOneResp.Body.String())
	}

	var pageOne struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(pageOneResp.Body.Bytes(), &pageOne); err != nil {
		t.Fatalf("decode page 1 payload: %v", err)
	}
	if pageOne.Page != 1 || pageOne.PageSize != 20 || pageOne.Total != 25 {
		t.Fatalf("unexpected page 1 metadata: %+v", pageOne)
	}
	if len(pageOne.Items) != 20 {
		t.Fatalf("expected 20 page 1 items, got %d", len(pageOne.Items))
	}
	if got := uint(pageOne.Items[0]["id"].(float64)); got != ids[24] {
		t.Fatalf("expected first item id %d, got %d", ids[24], got)
	}
	if got := uint(pageOne.Items[19]["id"].(float64)); got != ids[5] {
		t.Fatalf("expected twentieth item id %d, got %d", ids[5], got)
	}
	if _, ok := pageOne.Items[0]["prompt"]; ok {
		t.Fatalf("expected lightweight generation item without prompt: %+v", pageOne.Items[0])
	}
	if _, ok := pageOne.Items[0]["error_message"]; ok {
		t.Fatalf("expected lightweight generation item without error_message: %+v", pageOne.Items[0])
	}

	pageTwoResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?page=2&page_size=20", nil, adminCookies)
	if pageTwoResp.Code != http.StatusOK {
		t.Fatalf("expected page 2 status 200, got %d: %s", pageTwoResp.Code, pageTwoResp.Body.String())
	}

	var pageTwo struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(pageTwoResp.Body.Bytes(), &pageTwo); err != nil {
		t.Fatalf("decode page 2 payload: %v", err)
	}
	if pageTwo.Page != 2 || pageTwo.PageSize != 20 || pageTwo.Total != 25 {
		t.Fatalf("unexpected page 2 metadata: %+v", pageTwo)
	}
	if len(pageTwo.Items) != 5 {
		t.Fatalf("expected 5 page 2 items, got %d", len(pageTwo.Items))
	}
	if got := uint(pageTwo.Items[0]["id"].(float64)); got != ids[4] {
		t.Fatalf("expected page 2 first item id %d, got %d", ids[4], got)
	}
	if got := uint(pageTwo.Items[4]["id"].(float64)); got != ids[0] {
		t.Fatalf("expected page 2 last item id %d, got %d", ids[0], got)
	}
}

func TestAdminGenerationsListSupportsFiltersSummaryAndRichItems(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	userA := seedAdminGenerationUser(t, db, "creator_alpha", "阿尔法设计师", "alpha@example.com")
	userB := seedAdminGenerationUser(t, db, "creator_beta", "Beta", "beta@example.com")
	setUserPhoneForTest(t, testApp, userA.ID, "13800138001")
	setUserPhoneForTest(t, testApp, userB.ID, "13800138002")
	var dalle ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&dalle).Error; err != nil {
		t.Fatalf("load DALL-E model: %v", err)
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	workID := uint(501)
	matched := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:            userA.ID,
		WorkID:            &workID,
		Prompt:            "赛博森林玻璃塔，蓝绿色光线",
		NegativePrompt:    "低清晰度",
		AspectRatio:       "1:1",
		Quality:           GenerationQualityHigh,
		StylePreset:       "cinematic",
		ToolMode:          GenerationToolModeGenerate,
		StyleStrength:     70,
		ReferenceWeight:   45,
		Seed:              "seed-42",
		ModelConfigID:     dalle.ID,
		Model:             "gpt-image-2",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		LatencyMS:         1200,
		PreviewURL:        "/api/works/501/file",
		DownloadURL:       "/api/works/501/download",
		MIMEType:          "image/png",
		CreatedAt:         todayStart.Add(10 * time.Hour),
		UpdatedAt:         todayStart.Add(10 * time.Hour),
		CreditsDeducted:   true,
		ProviderRequestID: "req_success",
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:       userB.ID,
		Prompt:       "水彩城市街景",
		Model:        "gpt-image-2-2026-04-21",
		Status:       GenerationStatusFailed,
		Stage:        GenerationStageFailed,
		ErrorCode:    "provider_error",
		ErrorMessage: "供应商超时",
		LatencyMS:    2400,
		CreatedAt:    todayStart.Add(11 * time.Hour),
		UpdatedAt:    todayStart.Add(11 * time.Hour),
	})
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:    userA.ID,
		Prompt:    "昨日成功任务",
		Model:     "gpt-image-2",
		Status:    GenerationStatusSucceeded,
		LatencyMS: 900,
		CreatedAt: yesterdayStart.Add(9 * time.Hour),
		UpdatedAt: yesterdayStart.Add(9 * time.Hour),
	})

	path := fmt.Sprintf(
		"/api/admin/generations?q=%s&model_config_id=%d&user_keyword=%s&status=succeeded&date_from=%s&date_to=%s&page=1&page_size=10",
		"赛博",
		dalle.ID,
		"CREATOR_ALPHA",
		todayStart.Format("2006-01-02"),
		todayStart.Format("2006-01-02"),
	)
	resp := performJSONRequest(t, testApp, http.MethodGet, path, nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected filtered generation list 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Items []struct {
			ID            uint   `json:"id"`
			UserID        uint   `json:"user_id"`
			PromptSummary string `json:"prompt_summary"`
			ModelConfigID uint   `json:"model_config_id"`
			ModelName     string `json:"model_name"`
			RuntimeModel  string `json:"runtime_model"`
			Model         string `json:"model"`
			Status        string `json:"status"`
			LatencyMS     int64  `json:"latency_ms"`
			CreditsCost   int    `json:"credits_cost"`
			User          struct {
				ID          uint   `json:"id"`
				Username    string `json:"username"`
				DisplayName string `json:"display_name"`
				Email       string `json:"email"`
			} `json:"user"`
			PreviewImages []struct {
				PreviewURL  string `json:"preview_url"`
				DownloadURL string `json:"download_url"`
			} `json:"preview_images"`
		} `json:"items"`
		Summary struct {
			TodayGenerations             int64   `json:"today_generations"`
			TodayGenerationsDeltaPercent float64 `json:"today_generations_delta_percent"`
			SuccessRate                  float64 `json:"success_rate"`
			SuccessRateDeltaPercent      float64 `json:"success_rate_delta_percent"`
			AverageLatencyMS             int64   `json:"average_latency_ms"`
			AverageLatencyDeltaPercent   float64 `json:"average_latency_delta_percent"`
			FailedTasks                  int64   `json:"failed_tasks"`
			FailedTasksDeltaPercent      float64 `json:"failed_tasks_delta_percent"`
		} `json:"summary"`
		Filters struct {
			Query         string `json:"q"`
			Model         string `json:"model"`
			ModelConfigID uint   `json:"model_config_id"`
			UserID        uint   `json:"user_id"`
			UserKeyword   string `json:"user_keyword"`
			Status        string `json:"status"`
			DateFrom      string `json:"date_from"`
			DateTo        string `json:"date_to"`
		} `json:"filters"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode filtered generation list: %v", err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].ID != matched.ID {
		t.Fatalf("unexpected filtered items: %+v", payload)
	}
	item := payload.Items[0]
	if item.User.Username != "creator_alpha" || item.User.DisplayName != "阿尔法设计师" || item.User.Email != "alpha@example.com" {
		t.Fatalf("expected user snapshot on list item, got %+v", item.User)
	}
	if item.ModelConfigID != dalle.ID || item.ModelName != "DALL-E 3" || item.RuntimeModel != "gpt-image-2" || item.Model != "gpt-image-2" {
		t.Fatalf("expected structured model identity on list item, got %+v", item)
	}
	if item.PromptSummary == "" || item.CreditsCost != 1 || len(item.PreviewImages) != 1 || item.PreviewImages[0].PreviewURL != "/api/works/501/file" {
		t.Fatalf("expected prompt summary, credit cost and real preview image, got %+v", item)
	}
	if payload.Filters.Query != "赛博" || payload.Filters.ModelConfigID != dalle.ID || payload.Filters.Model != "" || payload.Filters.UserID != 0 || payload.Filters.UserKeyword != "CREATOR_ALPHA" || payload.Filters.Status != "succeeded" {
		t.Fatalf("expected echoed filters, got %+v", payload.Filters)
	}
	if payload.Summary.TodayGenerations != 2 || payload.Summary.SuccessRate != 50 || payload.Summary.AverageLatencyMS != 1800 || payload.Summary.FailedTasks != 1 {
		t.Fatalf("unexpected generation summary: %+v", payload.Summary)
	}
	if payload.Summary.TodayGenerationsDeltaPercent != 100 || payload.Summary.SuccessRateDeltaPercent != -50 || payload.Summary.FailedTasksDeltaPercent != 100 {
		t.Fatalf("unexpected summary deltas: %+v", payload.Summary)
	}

	phoneResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?user_keyword=13800138002&page=1&page_size=10", nil, adminCookies)
	if phoneResp.Code != http.StatusOK {
		t.Fatalf("expected phone filtered generation list 200, got %d: %s", phoneResp.Code, phoneResp.Body.String())
	}
	var phonePayload struct {
		Items []struct {
			ID     uint `json:"id"`
			UserID uint `json:"user_id"`
		} `json:"items"`
		Filters struct {
			UserID      uint   `json:"user_id"`
			UserKeyword string `json:"user_keyword"`
		} `json:"filters"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(phoneResp.Body.Bytes(), &phonePayload); err != nil {
		t.Fatalf("decode phone filtered generation list: %v", err)
	}
	if phonePayload.Total != 1 || len(phonePayload.Items) != 1 || phonePayload.Items[0].UserID != userB.ID {
		t.Fatalf("expected exact phone search to match beta user only, got %+v", phonePayload)
	}
	if phonePayload.Filters.UserID != 0 || phonePayload.Filters.UserKeyword != "13800138002" {
		t.Fatalf("expected echoed phone keyword filter, got %+v", phonePayload.Filters)
	}

	partialPhoneResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?user_keyword=138001380&page=1&page_size=10", nil, adminCookies)
	if partialPhoneResp.Code != http.StatusOK {
		t.Fatalf("expected partial phone generation list 200, got %d: %s", partialPhoneResp.Code, partialPhoneResp.Body.String())
	}
	var partialPhonePayload struct {
		Items []struct {
			ID uint `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(partialPhoneResp.Body.Bytes(), &partialPhonePayload); err != nil {
		t.Fatalf("decode partial phone generation list: %v", err)
	}
	if partialPhonePayload.Total != 0 || len(partialPhonePayload.Items) != 0 {
		t.Fatalf("expected partial phone search to match nothing, got %+v", partialPhonePayload)
	}
}

func TestAdminGlobalSearchValidatesQueryAndReturnsPermissionFilteredSections(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user := seedAdminGenerationUser(t, db, "global_creator", "全局搜索用户", "global@example.com")
	setUserPhoneForTest(t, testApp, user.ID, "13800139011")
	generation := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:          user.ID,
		Prompt:          "global-search-product-poster",
		NegativePrompt:  "no blur",
		Model:           "gpt-image-2",
		Status:          GenerationStatusSucceeded,
		Stage:           GenerationStageSucceeded,
		PreviewURL:      "/api/works/901/file",
		DownloadURL:     "/api/works/901/download",
		MIMEType:        "image/png",
		CreditsDeducted: true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	blankResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/search?q=%20", nil, adminCookies)
	if blankResp.Code != http.StatusBadRequest {
		t.Fatalf("expected blank search query 400, got %d: %s", blankResp.Code, blankResp.Body.String())
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/search?q=global", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected global admin search 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Query    string `json:"query"`
		Sections []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
			Items []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Subtitle string `json:"subtitle"`
				To       string `json:"to"`
			} `json:"items"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode global search payload: %v", err)
	}
	if payload.Query != "global" {
		t.Fatalf("expected echoed query, got %+v", payload)
	}
	sections := map[string][]struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		To       string `json:"to"`
	}{}
	for _, section := range payload.Sections {
		sections[section.Key] = section.Items
	}
	if len(sections["users"]) != 1 || sections["users"][0].Title != "全局搜索用户" || sections["users"][0].To != "/admin/users?q=global" {
		t.Fatalf("expected user search result routed to admin users, got %+v", sections["users"])
	}
	if len(sections["generations"]) != 1 || sections["generations"][0].ID != itoa(generation.ID) || sections["generations"][0].To != "/admin/generations?q=global" {
		t.Fatalf("expected generation search result routed to admin generations, got %+v", sections["generations"])
	}

	configResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/search?q=%E6%A8%A1%E6%9D%BF", nil, adminCookies)
	if configResp.Code != http.StatusOK {
		t.Fatalf("expected config entry search 200, got %d: %s", configResp.Code, configResp.Body.String())
	}
	var configPayload struct {
		Sections []struct {
			Key   string `json:"key"`
			Items []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				To    string `json:"to"`
			} `json:"items"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(configResp.Body.Bytes(), &configPayload); err != nil {
		t.Fatalf("decode config global search payload: %v", err)
	}
	foundPromptTemplate := false
	for _, section := range configPayload.Sections {
		if section.Key != "config" {
			continue
		}
		for _, item := range section.Items {
			if item.ID == "prompt_templates.read" && item.Title == "提示词模板" && item.To == "/admin/prompt-templates" {
				foundPromptTemplate = true
			}
		}
	}
	if !foundPromptTemplate {
		t.Fatalf("expected prompt template config entry in search payload, got %+v", configPayload.Sections)
	}

	limitedAdmin := createDatabaseAdminUser(t, db, "generation-search-admin", "LimitedPass123")
	var generationPermission Permission
	if err := db.Where("code = ?", "generations.read").First(&generationPermission).Error; err != nil {
		t.Fatalf("load generation permission: %v", err)
	}
	limitedRole := Role{Code: "generation_search_viewer", Name: "Generation search viewer", Status: RoleStatusActive}
	if err := db.Create(&limitedRole).Error; err != nil {
		t.Fatalf("create limited search role: %v", err)
	}
	if err := db.Model(&limitedRole).Association("Permissions").Append(&generationPermission); err != nil {
		t.Fatalf("assign generation search permission: %v", err)
	}
	if err := db.Model(&limitedAdmin).Association("Roles").Append(&limitedRole); err != nil {
		t.Fatalf("assign limited search role: %v", err)
	}
	limitedCookies := loginAdminAs(t, testApp, limitedAdmin.Username, "LimitedPass123")
	limitedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/search?q=global", nil, limitedCookies)
	if limitedResp.Code != http.StatusOK {
		t.Fatalf("expected limited global search 200, got %d: %s", limitedResp.Code, limitedResp.Body.String())
	}
	var limitedPayload struct {
		Sections []struct {
			Key   string `json:"key"`
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(limitedResp.Body.Bytes(), &limitedPayload); err != nil {
		t.Fatalf("decode limited global search payload: %v", err)
	}
	for _, section := range limitedPayload.Sections {
		if section.Key == "users" || section.Key == "config" {
			t.Fatalf("expected limited admin to receive only permitted generation section, got %+v", limitedPayload.Sections)
		}
	}
	if len(limitedPayload.Sections) != 1 || limitedPayload.Sections[0].Key != "generations" || len(limitedPayload.Sections[0].Items) != 1 {
		t.Fatalf("expected exactly one generation result for limited admin, got %+v", limitedPayload.Sections)
	}
}

func TestAdminGenerationDetailAndExportReturnRealImagesAndErrors(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user := seedAdminGenerationUser(t, db, "creator_detail", "详情用户", "detail@example.com")
	setUserPhoneForTest(t, testApp, user.ID, "13800138003")
	otherUser := seedAdminGenerationUser(t, db, "creator_export_other", "导出干扰用户", "export-other@example.com")
	setUserPhoneForTest(t, testApp, otherUser.ID, "13800138004")
	var dalle ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&dalle).Error; err != nil {
		t.Fatalf("load DALL-E model: %v", err)
	}
	now := time.Now()

	workID := uint(701)
	success := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:            user.ID,
		WorkID:            &workID,
		Prompt:            "产品海报，白色背景，金属质感",
		NegativePrompt:    "模糊",
		AspectRatio:       "1:1",
		Quality:           GenerationQualityHigh,
		StylePreset:       "product",
		ToolMode:          GenerationToolModeRedraw,
		StyleStrength:     64,
		ReferenceWeight:   52,
		Seed:              "seed-detail",
		ModelConfigID:     dalle.ID,
		Model:             "gpt-image-2",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		LatencyMS:         3200,
		PreviewURL:        "/api/works/701/file",
		DownloadURL:       "/api/works/701/download",
		MIMEType:          "image/png",
		CreatedAt:         now,
		UpdatedAt:         now,
		CreditsDeducted:   true,
		ProviderRequestID: "req_detail",
	})
	failed := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:               user.ID,
		Prompt:               "失败的产品海报",
		Model:                "gpt-image-2",
		Status:               GenerationStatusFailed,
		Stage:                GenerationStageFailed,
		ErrorCode:            "provider_unavailable",
		ErrorMessage:         "upstream gateway failed",
		ProviderHTTPStatus:   http.StatusBadGateway,
		ProviderErrorCode:    "provider_http_502",
		ProviderErrorMessage: "upstream gateway failed",
		ProviderFailureStage: providerFailureStageImageGenerationRequest,
		ProviderAttemptCount: 1,
		LatencyMS:            4500,
		CreatedAt:            now.Add(-time.Hour),
		UpdatedAt:            now.Add(-time.Hour),
	})
	otherWorkID := uint(702)
	seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:          otherUser.ID,
		WorkID:          &otherWorkID,
		Prompt:          "产品海报，白色背景，金属质感",
		Model:           "gpt-image-2",
		Status:          GenerationStatusSucceeded,
		Stage:           GenerationStageSucceeded,
		LatencyMS:       1800,
		PreviewURL:      "/api/works/702/file",
		DownloadURL:     "/api/works/702/download",
		MIMEType:        "image/png",
		CreatedAt:       now.Add(-2 * time.Hour),
		UpdatedAt:       now.Add(-2 * time.Hour),
		CreditsDeducted: true,
	})
	if err := db.Create(&GenerationEventLog{
		GenerationRecordID: failed.ID,
		TraceID:            "gen-trace-502",
		Level:              "error",
		Stage:              providerFailureStageImageGenerationRequest,
		Event:              "provider_request_failed",
		Message:            "供应商图片生成请求失败",
		MetadataJSON:       `{"provider_http_status":502,"provider_trace_id":"trace-502"}`,
		CreatedAt:          now.Add(-time.Hour + time.Second),
	}).Error; err != nil {
		t.Fatalf("seed generation event log: %v", err)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/generations/%d", success.ID), nil, adminCookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected generation detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var detail struct {
		ID            uint   `json:"id"`
		UserID        uint   `json:"user_id"`
		Prompt        string `json:"prompt"`
		ModelConfigID uint   `json:"model_config_id"`
		ModelName     string `json:"model_name"`
		RuntimeModel  string `json:"runtime_model"`
		Model         string `json:"model"`
		Status        string `json:"status"`
		LatencyMS     int64  `json:"latency_ms"`
		CreditsCost   int    `json:"credits_cost"`
		User          struct {
			Username string `json:"username"`
		} `json:"user"`
		Params struct {
			NegativePrompt  string `json:"negative_prompt"`
			AspectRatio     string `json:"aspect_ratio"`
			Quality         string `json:"quality"`
			StylePreset     string `json:"style_preset"`
			ToolMode        string `json:"tool_mode"`
			StyleStrength   int    `json:"style_strength"`
			ReferenceWeight int    `json:"reference_weight"`
			Seed            string `json:"seed"`
		} `json:"params"`
		ResultImages []struct {
			PreviewURL  string `json:"preview_url"`
			DownloadURL string `json:"download_url"`
		} `json:"result_images"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode generation detail: %v", err)
	}
	if detail.ID != success.ID || detail.User.Username != "creator_detail" || detail.CreditsCost != 1 {
		t.Fatalf("unexpected detail basics: %+v", detail)
	}
	if detail.ModelConfigID != dalle.ID || detail.ModelName != "DALL-E 3" || detail.RuntimeModel != "gpt-image-2" || detail.Model != "gpt-image-2" {
		t.Fatalf("expected structured model identity in detail, got %+v", detail)
	}
	if detail.Params.NegativePrompt != "模糊" || detail.Params.AspectRatio != "1:1" || detail.Params.ToolMode != GenerationToolModeRedraw || detail.Params.Seed != "seed-detail" {
		t.Fatalf("expected generation params in detail, got %+v", detail.Params)
	}
	if len(detail.ResultImages) != 1 || detail.ResultImages[0].DownloadURL != "/api/works/701/download" {
		t.Fatalf("expected real result image in detail, got %+v", detail.ResultImages)
	}

	failedResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/generations/%d", failed.ID), nil, adminCookies)
	if failedResp.Code != http.StatusOK {
		t.Fatalf("expected failed generation detail 200, got %d: %s", failedResp.Code, failedResp.Body.String())
	}
	if !bytes.Contains(failedResp.Body.Bytes(), []byte(`"code":"provider_unavailable"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"message":"upstream gateway failed"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"provider_http_status":502`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"provider_error_code":"provider_http_502"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"provider_error_message":"upstream gateway failed"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"provider_failure_stage":"image_generation_request"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"provider_attempt_count":1`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"events":[`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"trace_id":"gen-trace-502"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"event":"provider_request_failed"`)) ||
		!bytes.Contains(failedResp.Body.Bytes(), []byte(`"provider_trace_id":"trace-502"`)) {
		t.Fatalf("expected failure detail error fields, got %s", failedResp.Body.String())
	}

	exportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations/export?q=产品海报&status=succeeded&user_keyword=creator_detail", nil, adminCookies)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected generation CSV export 200, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if !bytes.Contains(exportResp.Body.Bytes(), []byte("产品海报")) || !bytes.Contains(exportResp.Body.Bytes(), []byte("/api/works/701/download")) {
		t.Fatalf("expected exported CSV to include success record and real download URL, got %s", exportResp.Body.String())
	}
	if bytes.Contains(exportResp.Body.Bytes(), []byte("provider_timeout")) {
		t.Fatalf("expected export filters to exclude failed record, got %s", exportResp.Body.String())
	}
	if bytes.Contains(exportResp.Body.Bytes(), []byte("/api/works/702/download")) {
		t.Fatalf("expected export user keyword filter to exclude other user record, got %s", exportResp.Body.String())
	}

	failedExportResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations/export?q=失败的产品海报&status=failed", nil, adminCookies)
	if failedExportResp.Code != http.StatusOK {
		t.Fatalf("expected failed generation CSV export 200, got %d: %s", failedExportResp.Code, failedExportResp.Body.String())
	}
	for _, marker := range [][]byte{
		[]byte("provider_unavailable"),
		[]byte("502"),
		[]byte("provider_http_502"),
		[]byte("upstream gateway failed"),
		[]byte("image_generation_request"),
		[]byte(",1"),
	} {
		if !bytes.Contains(failedExportResp.Body.Bytes(), marker) {
			t.Fatalf("expected failed export to include provider diagnostic marker %q, got %s", marker, failedExportResp.Body.String())
		}
	}
}

func TestAdminGenerationDetailKeepsReferenceImagesAfterAssetDeletion(t *testing.T) {
	const publicURL = "https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/reference-assets/9/2026/05/ref.png"

	testApp, _ := newTestApp(t, &stubProvider{})
	store := &publicURLAssetStore{
		key:       "assets/reference-assets/9/2026/05/ref.png",
		mimeType:  "image/png",
		publicURL: publicURL,
		content:   mustBase64Decode(t, fakePNGBase64),
	}
	testApp.assetStore = store
	adminCookies := createAdminSession(t, testApp)
	user, userCookies := createLoggedInUser(t, testApp, "creator_ref_retention", "test-password")
	asset := seedReferenceAsset(t, testApp, user.ID, "retained-reference.png", "image/png", mustBase64Decode(t, fakePNGBase64))
	record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:       user.ID,
		Prompt:       "失败任务也要留参考图",
		AspectRatio:  "1:1",
		Model:        "gpt-image-2",
		Status:       GenerationStatusFailed,
		Stage:        GenerationStageFailed,
		ErrorCode:    "provider_error",
		ErrorMessage: "供应商失败",
	})
	if err := testApp.db.Create(&GenerationReferenceAsset{
		GenerationRecordID: record.ID,
		ReferenceAssetID:   asset.ID,
		SortOrder:          0,
	}).Error; err != nil {
		t.Fatalf("link generation reference asset: %v", err)
	}

	deleteResp := performJSONRequest(t, testApp, http.MethodDelete, "/api/reference-assets/"+itoa(asset.ID), nil, userCookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected referenced asset delete 200, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	if store.deleteCalls != 0 {
		t.Fatalf("expected referenced asset object to be retained, delete calls=%d", store.deleteCalls)
	}
	listResp := performJSONRequest(t, testApp, http.MethodGet, "/api/reference-assets", nil, userCookies)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected reference list 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if bytes.Contains(listResp.Body.Bytes(), []byte("retained-reference.png")) {
		t.Fatalf("expected deleted referenced asset hidden from user list: %s", listResp.Body.String())
	}
	var deleted ReferenceAsset
	if err := testApp.db.First(&deleted, asset.ID).Error; err == nil {
		t.Fatalf("expected default query to hide soft-deleted reference asset, got %+v", deleted)
	}
	if err := testApp.db.Unscoped().First(&deleted, asset.ID).Error; err != nil {
		t.Fatalf("expected soft-deleted reference asset row to remain: %v", err)
	}

	detailResp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/generations/%d", record.ID), nil, adminCookies)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected admin generation detail 200, got %d: %s", detailResp.Code, detailResp.Body.String())
	}
	var payload struct {
		ReferenceImages []struct {
			ReferenceAssetID uint   `json:"reference_asset_id"`
			PreviewURL       string `json:"preview_url"`
			DownloadURL      string `json:"download_url"`
			MIMEType         string `json:"mime_type"`
			OriginalFilename string `json:"original_filename"`
			SortOrder        int    `json:"sort_order"`
		} `json:"reference_images"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin detail payload: %v", err)
	}
	if len(payload.ReferenceImages) != 1 ||
		payload.ReferenceImages[0].ReferenceAssetID != asset.ID ||
		payload.ReferenceImages[0].PreviewURL != publicURL ||
		payload.ReferenceImages[0].DownloadURL != publicURL ||
		payload.ReferenceImages[0].MIMEType != "image/png" ||
		payload.ReferenceImages[0].OriginalFilename != "retained-reference.png" ||
		payload.ReferenceImages[0].SortOrder != 0 {
		t.Fatalf("expected retained reference image evidence, got %+v", payload.ReferenceImages)
	}
}

func TestAdminGenerationsListComputesRunningLatencyFromCreatedAt(t *testing.T) {
	testApp, db := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)
	user := seedAdminGenerationUser(t, db, "creator_running_latency", "运行用户", "running@example.com")
	createdAt := time.Now().Add(-2 * time.Minute)

	record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:    user.ID,
		Prompt:    "running task should show elapsed time",
		Model:     "gpt-image-2",
		Status:    GenerationStatusRunning,
		Stage:     GenerationStageRequestingProvider,
		LatencyMS: 0,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?page=1&page_size=10", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generations list 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID        uint  `json:"id"`
			LatencyMS int64 `json:"latency_ms"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generations list: %v", err)
	}
	for _, item := range payload.Items {
		if item.ID == record.ID {
			if item.LatencyMS < int64(time.Minute/time.Millisecond) {
				t.Fatalf("expected running task latency to use elapsed time, got %d", item.LatencyMS)
			}
			return
		}
	}
	t.Fatalf("running generation %d not found in list: %+v", record.ID, payload.Items)
}

func TestAdminGenerationsListRejectsNonAdminAndClampsPagination(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	createdAt := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 25; i++ {
		workID := uint(i)
		seedAdminGenerationRecord(t, testApp, GenerationRecord{
			UserID:          uint(i),
			WorkID:          &workID,
			Model:           "gpt-image-2",
			Status:          GenerationStatusSucceeded,
			LatencyMS:       int64(i),
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
			CreditsDeducted: true,
		})
	}

	unauthorizedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations", nil, nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status 401, got %d", unauthorizedResp.Code)
	}

	adminCookies := createAdminSession(t, testApp)
	clampedResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?page=0&page_size=1000", nil, adminCookies)
	if clampedResp.Code != http.StatusOK {
		t.Fatalf("expected clamped list status 200, got %d: %s", clampedResp.Code, clampedResp.Body.String())
	}

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(clampedResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode clamped payload: %v", err)
	}
	if payload.Page != 1 || payload.PageSize != 100 || payload.Total != 25 {
		t.Fatalf("unexpected clamped metadata: %+v", payload)
	}
	if len(payload.Items) != 25 {
		t.Fatalf("expected 25 clamped items, got %d", len(payload.Items))
	}
}

func TestAdminLoginUsesDatabaseBackedSessionsAndLogout(t *testing.T) {
	testApp, db := newTestApp(t, nil)

	cookies := createAdminSession(t, testApp)

	var sessions int64
	if err := db.Model(&AdminSession{}).Count(&sessions).Error; err != nil {
		t.Fatalf("count admin sessions: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("expected one admin session, got %d", sessions)
	}

	meResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, cookies)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expected admin me 200, got %d: %s", meResp.Code, meResp.Body.String())
	}
	if !bytes.Contains(meResp.Body.Bytes(), []byte(`"permissions"`)) {
		t.Fatalf("expected permissions in admin me payload: %s", meResp.Body.String())
	}

	logoutResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/logout", nil, cookies)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("expected admin logout 200, got %d: %s", logoutResp.Code, logoutResp.Body.String())
	}

	dashboardResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/dashboard", nil, cookies)
	if dashboardResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected dashboard after logout 401, got %d", dashboardResp.Code)
	}
}

func TestAdminPasswordChangeRequiresCurrentPasswordInvalidatesSessionsAndAudits(t *testing.T) {
	testApp, db := newTestApp(t, nil)

	primaryCookies := createAdminSession(t, testApp)
	secondaryCookies := createAdminSession(t, testApp)

	var admin AdminUser
	if err := db.Where("username = ?", testApp.cfg.AdminUsername).First(&admin).Error; err != nil {
		t.Fatalf("load admin: %v", err)
	}

	badCurrentResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/password", map[string]any{
		"current_password": "wrong-password",
		"new_password":     "AdminPass456",
	}, primaryCookies)
	if badCurrentResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong current password 401, got %d: %s", badCurrentResp.Code, badCurrentResp.Body.String())
	}

	shortPasswordResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/password", map[string]any{
		"current_password": testApp.cfg.AdminPassword,
		"new_password":     "short",
	}, primaryCookies)
	if shortPasswordResp.Code != http.StatusBadRequest {
		t.Fatalf("expected short password 400, got %d: %s", shortPasswordResp.Code, shortPasswordResp.Body.String())
	}

	okResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/password", map[string]any{
		"current_password": testApp.cfg.AdminPassword,
		"new_password":     "AdminPass456",
	}, primaryCookies)
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected password change 200, got %d: %s", okResp.Code, okResp.Body.String())
	}
	if got := okResp.Result().Cookies(); len(got) == 0 || got[0].Name != adminSessionCookie || got[0].MaxAge >= 0 {
		t.Fatalf("expected admin session cookie to be cleared, got %+v", got)
	}

	var sessions int64
	if err := db.Model(&AdminSession{}).Where("admin_user_id = ?", admin.ID).Count(&sessions).Error; err != nil {
		t.Fatalf("count admin sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("expected all admin sessions invalidated, got %d", sessions)
	}

	meWithPrimarySession := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, primaryCookies)
	if meWithPrimarySession.Code != http.StatusUnauthorized {
		t.Fatalf("expected primary session invalidated 401, got %d", meWithPrimarySession.Code)
	}
	meWithSecondarySession := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/me", nil, secondaryCookies)
	if meWithSecondarySession.Code != http.StatusUnauthorized {
		t.Fatalf("expected secondary session invalidated 401, got %d", meWithSecondarySession.Code)
	}

	oldLoginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, testApp, testApp.cfg.AdminUsername, testApp.cfg.AdminPassword), nil)
	if oldLoginResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected old admin password 401, got %d", oldLoginResp.Code)
	}

	newLoginResp := performJSONRequest(t, testApp, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, testApp, testApp.cfg.AdminUsername, "AdminPass456"), nil)
	if newLoginResp.Code != http.StatusOK {
		t.Fatalf("expected new admin password 200, got %d: %s", newLoginResp.Code, newLoginResp.Body.String())
	}

	var auditCount int64
	if err := db.Model(&AdminAuditLog{}).
		Where("admin_user_id = ? AND action = ? AND target_type = ? AND target_id = ?", admin.ID, "admin.password.change", "admin_user", admin.ID).
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one password change audit log, got %d", auditCount)
	}
}

func TestAdminLoginDoesNotUseChangedEnvPasswordWhenDatabaseAdminExists(t *testing.T) {
	testApp, db := newTestApp(t, nil)

	cfg := testApp.cfg
	cfg.AdminPassword = "changed-env-password"
	reloaded, err := NewWithDependencies(cfg, db, nil)
	if err != nil {
		t.Fatalf("reload app: %v", err)
	}

	changedPasswordResp := performJSONRequest(t, reloaded, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, reloaded, cfg.AdminUsername, cfg.AdminPassword), nil)
	if changedPasswordResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected changed env password to fail, got %d", changedPasswordResp.Code)
	}

	originalPasswordResp := performJSONRequest(t, reloaded, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, reloaded, cfg.AdminUsername, "admin-pass"), nil)
	if originalPasswordResp.Code != http.StatusOK {
		t.Fatalf("expected original database password to pass, got %d: %s", originalPasswordResp.Code, originalPasswordResp.Body.String())
	}
}

func TestAdminPermissionChecksRejectAuthenticatedAdminsWithoutRolePermissions(t *testing.T) {
	testApp, db := newTestApp(t, nil)
	admin := createDatabaseAdminUser(t, db, "limited-admin", "LimitedPass123")

	noPermissionCookies := loginAdminAs(t, testApp, "limited-admin", "LimitedPass123")
	noPermissionResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/dashboard", nil, noPermissionCookies)
	if noPermissionResp.Code != http.StatusForbidden {
		t.Fatalf("expected dashboard for admin without permission 403, got %d: %s", noPermissionResp.Code, noPermissionResp.Body.String())
	}

	var permission Permission
	if err := db.Where("code = ?", "dashboard.read").First(&permission).Error; err != nil {
		t.Fatalf("load dashboard permission: %v", err)
	}
	role := Role{Code: "dashboard_viewer", Name: "Dashboard viewer", Status: RoleStatusActive}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Model(&role).Association("Permissions").Append(&permission); err != nil {
		t.Fatalf("assign permission: %v", err)
	}
	if err := db.Model(&admin).Association("Roles").Append(&role); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	allowedCookies := loginAdminAs(t, testApp, "limited-admin", "LimitedPass123")
	dashboardResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/dashboard", nil, allowedCookies)
	if dashboardResp.Code != http.StatusOK {
		t.Fatalf("expected dashboard with permission 200, got %d: %s", dashboardResp.Code, dashboardResp.Body.String())
	}

	usersResp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/users", nil, allowedCookies)
	if usersResp.Code != http.StatusForbidden {
		t.Fatalf("expected users without users.read permission 403, got %d: %s", usersResp.Code, usersResp.Body.String())
	}
}

func TestStartupDoesNotMisclassifyLegacyImageGenerationsAsProviderTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)

	cfg := testConfig(t)
	cfg.AssetStoragePath = filepath.Join(t.TempDir(), "assets")
	firstApp, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("new first app: %v", err)
	}
	user, _ := createLoggedInUser(t, firstApp, "creator_stale_cleanup", "test-password")
	staleTime := time.Now().Add(-time.Duration(cfg.RequestTimeoutSeconds+60) * time.Second)
	freshTime := time.Now()

	staleRunning := seedAdminGenerationRecord(t, firstApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "stale running",
		Model:       "gpt-image-2",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
		CreatedAt:   staleTime,
		UpdatedAt:   staleTime,
	})
	staleQueued := seedAdminGenerationRecord(t, firstApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "stale queued",
		Model:       "gpt-image-2",
		Status:      GenerationStatusQueued,
		Stage:       GenerationStageQueued,
		CreditsCost: 1,
		CreatedAt:   staleTime,
		UpdatedAt:   staleTime,
	})
	freshRunning := seedAdminGenerationRecord(t, firstApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "fresh running",
		Model:       "gpt-image-2",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
		CreatedAt:   freshTime,
		UpdatedAt:   freshTime,
	})

	if _, err := NewWithDependencies(cfg, db, &stubProvider{}); err != nil {
		t.Fatalf("new second app: %v", err)
	}

	var runningRecord, queuedRecord, freshRecord GenerationRecord
	if err := db.First(&runningRecord, staleRunning.ID).Error; err != nil {
		t.Fatalf("load stale running record: %v", err)
	}
	if err := db.First(&queuedRecord, staleQueued.ID).Error; err != nil {
		t.Fatalf("load stale queued record: %v", err)
	}
	if err := db.First(&freshRecord, freshRunning.ID).Error; err != nil {
		t.Fatalf("load fresh running record: %v", err)
	}
	if runningRecord.Status != GenerationStatusRunning || runningRecord.ErrorCode != "" {
		t.Fatalf("expected legacy running record not to be misclassified as provider timeout, got %+v", runningRecord)
	}
	if queuedRecord.Status != GenerationStatusQueued || queuedRecord.ErrorCode != "" {
		t.Fatalf("expected legacy queued record not to be misclassified as provider timeout, got %+v", queuedRecord)
	}
	if freshRecord.Status != GenerationStatusRunning {
		t.Fatalf("expected fresh running record to remain running, got %+v", freshRecord)
	}
}

func TestGetImageGenerationDoesNotUseCreatedAtAsProviderTimeout(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_expire_image", "test-password")
	now := time.Now()
	staleTime := now.Add(-10*time.Minute - time.Second)
	freshTime := now.Add(-10*time.Minute + time.Second)

	staleImage := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "stale image",
		Model:       "gpt-image-2",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
		CreatedAt:   staleTime,
		UpdatedAt:   staleTime,
	})
	freshImage := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "fresh image",
		Model:       "gpt-image-2",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
		CreatedAt:   freshTime,
		UpdatedAt:   freshTime,
	})
	staleVideo := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "stale video",
		Model:       "grok-imagine",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
		CreatedAt:   staleTime,
		UpdatedAt:   staleTime,
	})
	if err := testApp.db.Create(&VideoGenerationRecord{
		GenerationRecordID: staleVideo.ID,
		UserID:             user.ID,
		Prompt:             staleVideo.Prompt,
		Status:             GenerationStatusRunning,
		Stage:              GenerationStageRequestingProvider,
		CreatedAt:          staleTime,
		UpdatedAt:          staleTime,
	}).Error; err != nil {
		t.Fatalf("seed video generation record: %v", err)
	}

	resp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/images/generations/%d", staleImage.ID), nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected get generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Status string `json:"status"`
		Error  struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
		CreditsDeducted bool `json:"credits_deducted"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generation payload: %v", err)
	}
	if payload.Status != GenerationStatusRunning || payload.Error.Code != "" || payload.CreditsDeducted {
		t.Fatalf("expected stale legacy image to remain unchanged, got %+v", payload)
	}

	var reloadedStale, reloadedFresh, reloadedVideo GenerationRecord
	if err := testApp.db.First(&reloadedStale, staleImage.ID).Error; err != nil {
		t.Fatalf("load stale image: %v", err)
	}
	if err := testApp.db.First(&reloadedFresh, freshImage.ID).Error; err != nil {
		t.Fatalf("load fresh image: %v", err)
	}
	if err := testApp.db.First(&reloadedVideo, staleVideo.ID).Error; err != nil {
		t.Fatalf("load stale video: %v", err)
	}
	if reloadedStale.Status != GenerationStatusRunning || reloadedStale.ErrorCode != "" {
		t.Fatalf("expected stale legacy image not to be marked provider timeout, got %+v", reloadedStale)
	}
	if reloadedFresh.Status != GenerationStatusRunning {
		t.Fatalf("expected fresh image to remain running, got %+v", reloadedFresh)
	}
	if reloadedVideo.Status != GenerationStatusRunning {
		t.Fatalf("expected stale video to remain running, got %+v", reloadedVideo)
	}
}

func TestAdminGenerationsListDoesNotInventProviderTimeoutFailures(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, testApp, "creator_admin_expire_image", "test-password")
	adminCookies := loginAdminAs(t, testApp, testApp.cfg.AdminUsername, testApp.cfg.AdminPassword)
	staleTime := time.Now().Add(-12 * time.Minute)
	staleImage := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "admin stale image",
		Model:       "gpt-image-2",
		Status:      GenerationStatusQueued,
		Stage:       GenerationStageQueued,
		CreditsCost: 1,
		CreatedAt:   staleTime,
		UpdatedAt:   staleTime,
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, "/api/admin/generations?page=1&page_size=20", nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected admin generations 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []struct {
			ID        uint   `json:"id"`
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
			LatencyMS int64  `json:"latency_ms"`
		} `json:"items"`
		Summary struct {
			FailedTasks int64 `json:"failed_tasks"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin generations payload: %v", err)
	}
	if len(payload.Items) == 0 ||
		payload.Items[0].ID != staleImage.ID ||
		payload.Items[0].Status != GenerationStatusQueued ||
		payload.Items[0].ErrorCode != "" {
		t.Fatalf("expected stale legacy image unchanged in admin list, got %+v", payload.Items)
	}
	if payload.Summary.FailedTasks != 0 {
		t.Fatalf("expected summary not to invent provider timeout failures, got %+v", payload.Summary)
	}
}

func TestLateImageProviderResultDoesNotReviveTimedOutTask(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	provider := newDelayedSuccessImageProvider()
	close(provider.release)
	cfg := testConfig(t)
	cfg.AssetStoragePath = filepath.Join(t.TempDir(), "assets")
	testApp, err := NewWithDependencies(cfg, db, provider)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	user, _ := createLoggedInUser(t, testApp, "creator_late_timeout", "test-password")
	settings, err := testApp.loadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	styleStrength := 65
	referenceWeight := 75
	job := &generationJob{
		User:     user,
		Settings: settings,
		Request: generationRequest{
			Prompt:          "late provider result",
			AspectRatio:     "1:1",
			Quality:         GenerationQualityMedium,
			ToolMode:        GenerationToolModeGenerate,
			Size:            "1024x1024",
			StyleStrength:   &styleStrength,
			ReferenceWeight: &referenceWeight,
		},
	}
	record, err := testApp.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
	if err != nil {
		t.Fatalf("create generation record: %v", err)
	}
	provider.beforeReturn = func() {
		if err := testApp.db.Model(&GenerationRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
			"status":           GenerationStatusFailed,
			"stage":            GenerationStageFailed,
			"error_code":       "provider_timeout",
			"error_message":    "图片生成超过10分钟未完成，已自动判定失败，请重新生成。",
			"credits_deducted": false,
			"latency_ms":       int64(10 * time.Minute / time.Millisecond),
		}).Error; err != nil {
			t.Errorf("mark timed out: %v", err)
		}
	}
	_, _, _ = testApp.executeGenerationRecord(&record, job)

	var reloaded GenerationRecord
	if err := testApp.db.First(&reloaded, record.ID).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if reloaded.Status != GenerationStatusFailed ||
		reloaded.ErrorCode != "provider_timeout" ||
		reloaded.WorkID != nil ||
		reloaded.CreditsDeducted {
		t.Fatalf("expected timed out record to remain failed, got %+v", reloaded)
	}
	var workCount int64
	if err := testApp.db.Model(&Work{}).Where("generation_record_id = ?", record.ID).Count(&workCount).Error; err != nil {
		t.Fatalf("count work: %v", err)
	}
	if workCount != 0 {
		t.Fatalf("expected no work created for late result, got %d", workCount)
	}
	var txCount int64
	if err := testApp.db.Model(&CreditTransaction{}).Where("user_id = ? AND related_type = ? AND related_id = ?", user.ID, "generation", record.ID).Count(&txCount).Error; err != nil {
		t.Fatalf("count credit transactions: %v", err)
	}
	if txCount != 0 {
		t.Fatalf("expected no generation charge for late result, got %d", txCount)
	}
}

func TestCancelImageGenerationMarksRunningTaskWithoutCredits(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_cancel_running", "test-password")
	startedAt := time.Now().Add(-90 * time.Second)
	record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      user.ID,
		Prompt:      "wrong prompt",
		AspectRatio: "1:1",
		Model:       "gpt-image-2",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
		AssetKey:    "stale/asset.png",
		PreviewURL:  "https://oss.example.com/stale.png",
		DownloadURL: "https://oss.example.com/stale.png",
		MIMEType:    "image/png",
		CreatedAt:   startedAt,
		UpdatedAt:   startedAt,
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/images/generations/%d/cancel", record.ID), nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected cancel generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		GenerationID     uint   `json:"generation_id"`
		Status           string `json:"status"`
		Stage            string `json:"stage"`
		CreditsDeducted  bool   `json:"credits_deducted"`
		AvailableCredits int    `json:"available_credits"`
		Error            struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode cancel payload: %v", err)
	}
	if payload.GenerationID != record.ID ||
		payload.Status != GenerationStatusFailed ||
		payload.Stage != GenerationStageFailed ||
		payload.Error.Code != "user_cancelled" ||
		payload.Error.Message != "已取消生成，未扣点。" ||
		!payload.Error.Retryable ||
		payload.CreditsDeducted ||
		payload.AvailableCredits <= 0 {
		t.Fatalf("unexpected cancel payload: %+v", payload)
	}

	var reloaded GenerationRecord
	if err := testApp.db.First(&reloaded, record.ID).Error; err != nil {
		t.Fatalf("load cancelled generation: %v", err)
	}
	if reloaded.Status != GenerationStatusFailed ||
		reloaded.Stage != GenerationStageFailed ||
		reloaded.ErrorCode != "user_cancelled" ||
		reloaded.ErrorMessage != "已取消生成，未扣点。" ||
		reloaded.CreditsDeducted ||
		reloaded.AssetKey != "" ||
		reloaded.PreviewURL != "" ||
		reloaded.DownloadURL != "" ||
		reloaded.MIMEType != "" ||
		reloaded.LatencyMS <= 0 {
		t.Fatalf("expected running record cancelled without result, got %+v", reloaded)
	}
	var workCount int64
	if err := testApp.db.Model(&Work{}).Where("generation_record_id = ?", record.ID).Count(&workCount).Error; err != nil {
		t.Fatalf("count work: %v", err)
	}
	if workCount != 0 {
		t.Fatalf("expected no work for cancelled generation, got %d", workCount)
	}
	var txCount int64
	if err := testApp.db.Model(&CreditTransaction{}).Where("user_id = ? AND related_type = ? AND related_id = ?", user.ID, "generation", record.ID).Count(&txCount).Error; err != nil {
		t.Fatalf("count credit transactions: %v", err)
	}
	if txCount != 0 {
		t.Fatalf("expected no generation charge for cancelled generation, got %d", txCount)
	}
}

func TestCancelImageGenerationRejectsOtherUsersAndVideoTasks(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	owner, ownerCookies := createLoggedInUser(t, testApp, "creator_cancel_owner", "test-password")
	other, otherCookies := createLoggedInUser(t, testApp, "creator_cancel_other", "test-password")
	_ = other
	record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      owner.ID,
		Prompt:      "owner running image",
		Model:       "gpt-image-2",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/images/generations/%d/cancel", record.ID), nil, otherCookies)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected other user cancel 404, got %d: %s", resp.Code, resp.Body.String())
	}
	var reloaded GenerationRecord
	if err := testApp.db.First(&reloaded, record.ID).Error; err != nil {
		t.Fatalf("load owner record: %v", err)
	}
	if reloaded.Status != GenerationStatusRunning {
		t.Fatalf("expected other-user cancel to leave record running, got %+v", reloaded)
	}

	videoRecord := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:      owner.ID,
		Prompt:      "video running",
		Model:       "grok-imagine",
		Status:      GenerationStatusRunning,
		Stage:       GenerationStageRequestingProvider,
		CreditsCost: 1,
	})
	if err := testApp.db.Create(&VideoGenerationRecord{
		GenerationRecordID: videoRecord.ID,
		UserID:             owner.ID,
		Prompt:             videoRecord.Prompt,
		Status:             GenerationStatusRunning,
		Stage:              GenerationStageRequestingProvider,
	}).Error; err != nil {
		t.Fatalf("seed video generation record: %v", err)
	}
	videoResp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/images/generations/%d/cancel", videoRecord.ID), nil, ownerCookies)
	if videoResp.Code != http.StatusNotFound {
		t.Fatalf("expected video cancel through image endpoint 404, got %d: %s", videoResp.Code, videoResp.Body.String())
	}
	var reloadedVideo GenerationRecord
	if err := testApp.db.First(&reloadedVideo, videoRecord.ID).Error; err != nil {
		t.Fatalf("load video record: %v", err)
	}
	if reloadedVideo.Status != GenerationStatusRunning {
		t.Fatalf("expected image cancel endpoint to leave video record running, got %+v", reloadedVideo)
	}
}

func TestCancelImageGenerationReturnsTerminalTaskWithoutMutation(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	user, cookies := createLoggedInUser(t, testApp, "creator_cancel_done", "test-password")
	workID := uint(42)
	record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:            user.ID,
		WorkID:            &workID,
		Prompt:            "finished image",
		Model:             "gpt-image-2",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		CreditsCost:       1,
		CreditsDeducted:   true,
		AssetKey:          "works/done.png",
		PreviewURL:        "/api/works/42/file",
		DownloadURL:       "/api/works/42/download",
		MIMEType:          "image/png",
		ProviderRequestID: "req_done",
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/images/generations/%d/cancel", record.ID), nil, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected cancel completed generation 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Status          string `json:"status"`
		WorkID          uint   `json:"work_id"`
		CreditsDeducted bool   `json:"credits_deducted"`
		Error           any    `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode completed cancel payload: %v", err)
	}
	if payload.Status != GenerationStatusSucceeded || payload.WorkID != workID || !payload.CreditsDeducted || payload.Error != nil {
		t.Fatalf("expected completed task payload unchanged, got %+v", payload)
	}
	var reloaded GenerationRecord
	if err := testApp.db.First(&reloaded, record.ID).Error; err != nil {
		t.Fatalf("load completed record: %v", err)
	}
	if reloaded.Status != GenerationStatusSucceeded || reloaded.ErrorCode != "" || reloaded.WorkID == nil || *reloaded.WorkID != workID {
		t.Fatalf("expected completed record unchanged, got %+v", reloaded)
	}
}

func TestCancelImageGenerationStopsBlockingProviderAndIgnoresLateResult(t *testing.T) {
	provider := newBlockingImageProvider()
	defer close(provider.release)
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_cancel_provider", "test-password")

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations/async", map[string]any{
		"prompt":       "prompt that should be cancelled",
		"aspect_ratio": "1:1",
		"tool_mode":    "generate",
	}, cookies)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected async generation 202, got %d: %s", resp.Code, resp.Body.String())
	}
	var createPayload struct {
		GenerationID uint `json:"generation_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode async payload: %v", err)
	}
	waitForCondition(t, time.Second, func() bool {
		return provider.callCount() == 1
	})

	cancelResp := performJSONRequest(t, testApp, http.MethodPost, fmt.Sprintf("/api/images/generations/%d/cancel", createPayload.GenerationID), nil, cookies)
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("expected cancel blocking generation 200, got %d: %s", cancelResp.Code, cancelResp.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		return provider.finishedCount() >= 1
	})

	var reloaded GenerationRecord
	if err := testApp.db.First(&reloaded, createPayload.GenerationID).Error; err != nil {
		t.Fatalf("load cancelled generation: %v", err)
	}
	if reloaded.Status != GenerationStatusFailed ||
		reloaded.ErrorCode != "user_cancelled" ||
		reloaded.WorkID != nil ||
		reloaded.CreditsDeducted {
		t.Fatalf("expected provider-cancelled task to remain user_cancelled, got %+v", reloaded)
	}
	var workCount int64
	if err := testApp.db.Model(&Work{}).Where("generation_record_id = ?", createPayload.GenerationID).Count(&workCount).Error; err != nil {
		t.Fatalf("count work: %v", err)
	}
	if workCount != 0 {
		t.Fatalf("expected no work created for cancelled provider result, got %d", workCount)
	}
	var txCount int64
	if err := testApp.db.Model(&CreditTransaction{}).Where("user_id = ? AND related_type = ? AND related_id = ?", user.ID, "generation", createPayload.GenerationID).Count(&txCount).Error; err != nil {
		t.Fatalf("count credit transactions: %v", err)
	}
	if txCount != 0 {
		t.Fatalf("expected no generation charge for cancelled provider result, got %d", txCount)
	}
}

func TestExistingSettingsLegacyTimeoutIsUpgradedToConfiguredBaseline(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db := openTestSQLiteDB(t, dbPath)
	if err := db.AutoMigrate(&AppSettings{}, &Invite{}, &GenerationRecord{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}

	settings := AppSettings{
		ID:                     1,
		ActiveImageModel:       "gpt-image-2",
		RequestTimeoutSeconds:  90,
		DefaultInviteQuota:     10,
		RateLimitWindowSeconds: 60,
		RateLimitMaxRequests:   5,
	}
	if err := settings.SetAllowedImageModels([]string{"gpt-image-2"}); err != nil {
		t.Fatalf("set allowed models: %v", err)
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	cfg := testConfig(t)
	cfg.DatabaseURL = "postgres://test:test@localhost:5432/test?sslmode=disable"
	cfg.RequestTimeoutSeconds = 600
	app, err := NewWithDependencies(cfg, db, &stubProvider{})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	loaded, err := app.loadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if loaded.RequestTimeoutSeconds != 600 {
		t.Fatalf("expected upgraded timeout 600, got %d", loaded.RequestTimeoutSeconds)
	}
	if loaded.RateLimitMaxRequests != 20 {
		t.Fatalf("expected upgraded rate limit 20, got %d", loaded.RateLimitMaxRequests)
	}
}

func newTestApp(t *testing.T, provider ImageProvider) (*App, *gorm.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	cfg := testConfig(t)
	cfg.DatabaseURL = "postgres://test:test@localhost:5432/test?sslmode=disable"
	cfg.AssetStoragePath = filepath.Join(t.TempDir(), "assets")

	app, err := NewWithDependencies(cfg, db, provider)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	// 电商 handler 测试默认覆盖启用态，feature guard 用例会显式关闭。
	app.cfg.AICommerceEnabled = true
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Fatalf("close app: %v", err)
		}
	})
	return app, db
}

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		AppBaseURL:               "http://localhost:3000",
		OpenAIAPIKey:             "test-key",
		OpenAIBaseURL:            "https://api.openai.com",
		JWTSecret:                "test-secret",
		AdminUsername:            "admin",
		AdminPassword:            "admin-pass",
		DatabaseURL:              "postgres://test:test@localhost:5432/test?sslmode=disable",
		AssetStoragePath:         filepath.Join(t.TempDir(), "assets"),
		DefaultInviteQuota:       10,
		DefaultImageModel:        "gpt-image-2",
		AllowedImageModels:       []string{"gpt-image-2", "gpt-image-2-2026-04-21"},
		RequestTimeoutSeconds:    600,
		RateLimitWindowSeconds:   60,
		RateLimitMaxRequests:     20,
		UserSessionHours:         72,
		AdminSessionHours:        12,
		FrontendDistPath:         "web/dist",
		StartupDatabaseBootstrap: true,
	}
}

func createLoggedInUser(t *testing.T, app *App, username, password string) (User, []*http.Cookie) {
	t.Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	now := time.Now()
	user := User{
		Username:                 username,
		DisplayName:              username,
		PasswordHash:             string(passwordHash),
		Status:                   UserStatusActive,
		LoginNotificationEnabled: true,
		RiskNotificationEnabled:  true,
		LastLoginAt:              &now,
	}
	if err := app.createRegisteredUserWithInvite(&user, "", now); err != nil {
		t.Fatalf("create user: %v", err)
	}

	loginResp := performJSONRequest(t, app, http.MethodPost, "/api/auth/login", userLoginPayloadWithCaptcha(t, app, username, password), nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	return user, loginResp.Result().Cookies()
}

func createSMSVerificationCodeForTest(t *testing.T, app *App, phone, purpose, code string) {
	t.Helper()
	record := AuthVerificationCode{
		Phone:     phone,
		Purpose:   purpose,
		CodeHash:  hashVerificationCode(phone, purpose, code, app.cfg.JWTSecret),
		ExpiresAt: time.Now().Add(verificationCodeTTL),
		IPAddress: "127.0.0.1",
	}
	if err := app.db.Create(&record).Error; err != nil {
		t.Fatalf("create verification code: %v", err)
	}
}

func setUserPhoneForTest(t *testing.T, app *App, userID uint, phone string) {
	t.Helper()
	normalized := normalizeMainlandPhone(phone)
	if !isValidMainlandPhone(normalized) {
		t.Fatalf("invalid test phone: %s", phone)
	}
	if err := app.db.Model(&User{}).Where("id = ?", userID).Update("phone", normalized).Error; err != nil {
		t.Fatalf("set user phone: %v", err)
	}
}

func setUserPhoneForCookiesForTest(t *testing.T, app *App, cookies []*http.Cookie, phone string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	claims, err := app.parseSessionCookie(req, userSessionCookie)
	if err != nil {
		t.Fatalf("parse session cookie: %v", err)
	}
	setUserPhoneForTest(t, app, claims.UserID, phone)
}

func createAdminSession(t *testing.T, app *App) []*http.Cookie {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, app, app.cfg.AdminUsername, app.cfg.AdminPassword), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("admin login failed: %d %s", resp.Code, resp.Body.String())
	}
	return resp.Result().Cookies()
}

func loginAdminAs(t *testing.T, app *App, username, password string) []*http.Cookie {
	t.Helper()
	resp := performJSONRequest(t, app, http.MethodPost, "/api/admin/login", adminLoginPayloadWithCaptcha(t, app, username, password), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("admin login failed: %d %s", resp.Code, resp.Body.String())
	}
	return resp.Result().Cookies()
}

func createDatabaseAdminUser(t *testing.T, db *gorm.DB, username, password string) AdminUser {
	t.Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	admin := AdminUser{
		Username:     username,
		DisplayName:  username,
		PasswordHash: string(passwordHash),
		Status:       AdminUserStatusActive,
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	return admin
}

func setUserCredits(t *testing.T, app *App, userID uint, amount int) {
	t.Helper()
	var balance CreditBalance
	if err := app.db.Where("user_id = ?", userID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	balance.AvailableCredits = amount
	if err := app.db.Save(&balance).Error; err != nil {
		t.Fatalf("save balance: %v", err)
	}
}

func seedSucceededWork(t *testing.T, app *App, userID uint, prompt, aspectRatio string) Work {
	t.Helper()
	record := GenerationRecord{
		UserID:            userID,
		Prompt:            prompt,
		AspectRatio:       aspectRatio,
		Model:             "gpt-image-2",
		Status:            GenerationStatusSucceeded,
		Stage:             GenerationStageSucceeded,
		MIMEType:          "image/png",
		AssetKey:          "seed/work.png",
		PreviewURL:        fmt.Sprintf("/api/works/%d/file", 1),
		DownloadURL:       fmt.Sprintf("/api/works/%d/download", 1),
		CreditsDeducted:   true,
		ProviderRequestID: "req_seed",
	}
	if err := app.db.Create(&record).Error; err != nil {
		t.Fatalf("seed record: %v", err)
	}

	work := Work{
		UserID:             userID,
		GenerationRecordID: record.ID,
		Prompt:             prompt,
		AspectRatio:        aspectRatio,
		Model:              record.Model,
		Status:             GenerationStatusSucceeded,
		Visibility:         WorkVisibilityPrivate,
		AssetKey:           "seed/work.png",
		MIMEType:           "image/png",
		ProviderRequestID:  "req_seed",
	}
	if err := app.db.Create(&work).Error; err != nil {
		t.Fatalf("seed work: %v", err)
	}

	record.WorkID = &work.ID
	record.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
	record.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
	if err := app.db.Save(&record).Error; err != nil {
		t.Fatalf("link record: %v", err)
	}

	work.PreviewURL = record.PreviewURL
	work.DownloadURL = record.DownloadURL
	if err := app.db.Save(&work).Error; err != nil {
		t.Fatalf("save urls: %v", err)
	}

	assetPath := filepath.Join(app.cfg.AssetStoragePath, "seed", "work.png")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write asset file: %v", err)
	}

	return work
}

func seedReferenceAsset(t *testing.T, app *App, userID uint, filename, mimeType string, content []byte) ReferenceAsset {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString(content)
	assetKey, normalizedMimeType, err := app.assetStore.SaveBase64(encoded, mimeType)
	if err != nil {
		t.Fatalf("save reference asset: %v", err)
	}

	asset := ReferenceAsset{
		UserID:           userID,
		AssetKey:         assetKey,
		MIMEType:         normalizedMimeType,
		OriginalFilename: filename,
	}
	if err := app.db.Create(&asset).Error; err != nil {
		t.Fatalf("create reference asset: %v", err)
	}
	asset.PreviewURL = fmt.Sprintf("/api/reference-assets/%d/file", asset.ID)
	if err := app.db.Save(&asset).Error; err != nil {
		t.Fatalf("save reference asset preview url: %v", err)
	}
	return asset
}

func seedAdminGenerationRecord(t *testing.T, app *App, record GenerationRecord) GenerationRecord {
	t.Helper()
	createdAt := record.CreatedAt
	updatedAt := record.UpdatedAt
	if err := app.db.Create(&record).Error; err != nil {
		t.Fatalf("seed generation record: %v", err)
	}
	if !createdAt.IsZero() || !updatedAt.IsZero() {
		updates := map[string]any{}
		if !createdAt.IsZero() {
			updates["created_at"] = createdAt
		}
		if !updatedAt.IsZero() {
			updates["updated_at"] = updatedAt
		}
		if err := app.db.Model(&record).Updates(updates).Error; err != nil {
			t.Fatalf("backfill generation timestamps: %v", err)
		}
		record.CreatedAt = createdAt
		record.UpdatedAt = updatedAt
	}
	return record
}

func seedImageRoutingModel(t *testing.T, app *App, name, baseURL string, sortOrder int) ModelConfig {
	t.Helper()
	model := ModelConfig{
		Name:         name,
		Type:         ModelConfigTypeImage,
		Provider:     "OpenAI",
		Status:       ModelConfigStatusOnline,
		Priority:     sortOrder,
		CostLabel:    "按配置扣点",
		Permission:   ModelConfigPermissionPublic,
		Weight:       50,
		SortOrder:    sortOrder,
		RuntimeModel: "gpt-image-2",
		APIBaseURL:   baseURL,
		APIEndpoint:  "/v1/images/generations",
		APIKey:       name + "-key",
	}
	if err := app.db.Create(&model).Error; err != nil {
		t.Fatalf("create image route model: %v", err)
	}
	return model
}

func setWuyinModelCenterProviderKey(t *testing.T, app *App, legacyModelConfigID uint, apiKey string) ModelChannel {
	t.Helper()
	if err := app.ensureModelCenter(); err != nil {
		t.Fatalf("ensure model center: %v", err)
	}
	var channel ModelChannel
	if err := app.db.Preload("Provider").
		Where("legacy_model_config_id = ?", legacyModelConfigID).
		Order("id asc").
		First(&channel).Error; err != nil {
		t.Fatalf("load Wuyin model center channel: %v", err)
	}
	if err := app.db.Model(&ModelProvider{}).
		Where("id = ?", channel.ProviderID).
		Updates(map[string]any{
			"api_key": strings.TrimSpace(apiKey),
			"status":  ModelCenterStatusOnline,
		}).Error; err != nil {
		t.Fatalf("update Wuyin provider key: %v", err)
	}
	if err := app.db.Model(&ModelChannel{}).
		Where("id = ?", channel.ID).
		Updates(map[string]any{
			"status":        ModelCenterStatusOnline,
			"health_status": ModelChannelHealthHealthy,
		}).Error; err != nil {
		t.Fatalf("update Wuyin channel status: %v", err)
	}
	if err := app.db.Preload("Provider").First(&channel, channel.ID).Error; err != nil {
		t.Fatalf("reload Wuyin model center channel: %v", err)
	}
	return channel
}

func setVideoModelCenterProviderKey(t *testing.T, app *App, legacyModelConfigID uint, name, providerCode, baseURL, apiKey string) ModelChannel {
	t.Helper()
	if err := app.ensureModelCenter(); err != nil {
		t.Fatalf("ensure model center: %v", err)
	}
	var channel ModelChannel
	if err := app.db.Preload("Provider").
		Where("legacy_model_config_id = ?", legacyModelConfigID).
		Order("id asc").
		First(&channel).Error; err != nil {
		t.Fatalf("load video model center channel: %v", err)
	}
	if err := app.db.Model(&ModelProvider{}).
		Where("id = ?", channel.ProviderID).
		Updates(map[string]any{
			"name":     strings.TrimSpace(name),
			"provider": strings.TrimSpace(providerCode),
			"base_url": strings.TrimRight(strings.TrimSpace(baseURL), "/"),
			"api_key":  strings.TrimSpace(apiKey),
			"status":   ModelCenterStatusOnline,
		}).Error; err != nil {
		t.Fatalf("update video provider key: %v", err)
	}
	if err := app.db.Model(&ModelChannel{}).
		Where("id = ?", channel.ID).
		Updates(map[string]any{
			"status":        ModelCenterStatusOnline,
			"health_status": ModelChannelHealthHealthy,
		}).Error; err != nil {
		t.Fatalf("update video channel status: %v", err)
	}
	if err := app.db.Preload("Provider").First(&channel, channel.ID).Error; err != nil {
		t.Fatalf("reload video model center channel: %v", err)
	}
	return channel
}

func disableSeedImageRoutes(t *testing.T, app *App) {
	t.Helper()
	if err := app.db.Model(&ModelConfig{}).
		Where("type = ?", ModelConfigTypeImage).
		Update("status", ModelConfigStatusOffline).Error; err != nil {
		t.Fatalf("disable seeded image routes: %v", err)
	}
}

func saveImageRoutingStrategy(t *testing.T, app *App, adminCookies []*http.Cookie, strategy string, defaultImageID, fallbackID uint, imageWeights []map[string]any) {
	t.Helper()
	var video ModelConfig
	if err := app.db.Where("type = ?", ModelConfigTypeVideo).Order("sort_order asc, id asc").First(&video).Error; err != nil {
		t.Fatalf("load video route model: %v", err)
	}
	resp := performJSONRequest(t, app, http.MethodPut, "/api/admin/model-routing", map[string]any{
		"default_image_model_id": defaultImageID,
		"default_video_model_id": video.ID,
		"fallback_model_id":      fallbackID,
		"routing_enabled":        true,
		"routing_strategy":       strategy,
		"concurrency_limit":      4,
		"image_weights":          imageWeights,
	}, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected model routing save 200, got %d: %s", resp.Code, resp.Body.String())
	}
}

func seedAdminGenerationUser(t *testing.T, db *gorm.DB, username, displayName, email string) User {
	t.Helper()
	user := User{
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Status:      UserStatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed admin generation user: %v", err)
	}
	return user
}

func performJSONRequest(t *testing.T, app *App, method, path string, body map[string]any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return performJSONRequestWithHeaders(t, app, method, path, body, cookies, nil)
}

func performJSONRequestWithHeaders(t *testing.T, app *App, method, path string, body map[string]any, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	return performRequestWithHeaders(t, app, method, path, payload, cookies, headers)
}

func performRequest(t *testing.T, app *App, method, path string, body []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return performRequestWithHeaders(t, app, method, path, body, cookies, nil)
}

func performRequestWithHeaders(t *testing.T, app *App, method, path string, body []byte, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", app.cfg.AppBaseURL)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	addDefaultCSRFForTest(req)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp := httptest.NewRecorder()
	app.Router().ServeHTTP(resp, req)
	return resp
}

func performMultipartRequest(t *testing.T, app *App, method, path, fieldName, filename string, body []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create multipart field: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(body)); err != nil {
		t.Fatalf("write multipart body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", app.cfg.AppBaseURL)
	addDefaultCSRFForTest(req)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp := httptest.NewRecorder()
	app.Router().ServeHTTP(resp, req)
	return resp
}

func oversizedPNGBytes(t *testing.T, size int64) []byte {
	t.Helper()
	header := mustBase64Decode(t, fakePNGBase64)
	if size < int64(len(header)) {
		size = int64(len(header))
	}
	content := make([]byte, size)
	copy(content, header)
	return content
}

func addDefaultCSRFForTest(req *http.Request) {
	if req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions {
		return
	}
	if req.Header.Get("X-Image-Agent-Client") == "mp-weixin" || req.Header.Get("Origin") == "" {
		return
	}
	if req.Header.Get(csrfHeaderName) != "" {
		return
	}
	token := "test-csrf-token"
	req.Header.Set(csrfHeaderName, token)
	req.AddCookie(&http.Cookie{
		Name:  csrfCookieName,
		Value: token,
		Path:  "/",
	})
}

const fakePNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+X2S0AAAAASUVORK5CYII="

func mustBase64Decode(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64 fixture: %v", err)
	}
	return decoded
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func itoa[T ~uint | ~uint64](value T) string {
	return fmt.Sprintf("%d", value)
}
