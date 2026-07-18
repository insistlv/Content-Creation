package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type deadlineCaptureProvider struct {
	result             ImageGenerationResult
	deadlineRemaining  []time.Duration
	deadlineConfigured []bool
}

func (p *deadlineCaptureProvider) Generate(ctx context.Context, input ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	deadline, ok := ctx.Deadline()
	p.deadlineConfigured = append(p.deadlineConfigured, ok)
	if ok {
		p.deadlineRemaining = append(p.deadlineRemaining, time.Until(deadline))
	}
	if strings.TrimSpace(p.result.Base64Image) == "" {
		p.result.Base64Image = base64.StdEncoding.EncodeToString([]byte("deadline-capture-image"))
	}
	if strings.TrimSpace(p.result.MIMEType) == "" {
		p.result.MIMEType = "image/png"
	}
	return p.result, nil
}

func TestImageGenerationFailoverRecordsAttemptsAndSucceeds(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusBadGateway,
				Code:              "provider_http_502",
				Message:           "upstream failed first",
				ProviderRequestID: "req_route_a_failed_1",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			{
				HTTPStatus:        http.StatusBadGateway,
				Code:              "provider_http_502",
				Message:           "upstream failed second",
				ProviderRequestID: "req_route_a_failed_2",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("route-b-image")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_route_b_success",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_failover_success", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "route a unavailable", "https://route-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "route b online", "https://route-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "切换到可用模型",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 3 {
		t.Fatalf("expected provider called for route A twice then route B, got %d inputs", len(provider.inputs))
	}
	if provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL || provider.inputs[1].ProviderBaseURL != routeA.APIBaseURL || provider.inputs[2].ProviderBaseURL != routeB.APIBaseURL {
		t.Fatalf("expected failover order A -> A -> B, got %+v", provider.inputs)
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
	if record.Status != GenerationStatusSucceeded || record.ModelConfigID != routeB.ID || record.ProviderRequestID != "req_route_b_success" {
		t.Fatalf("expected main record to keep final successful model, got %+v", record)
	}

	var attempts []ModelCallAttempt
	if err := testApp.db.Where("generation_record_id = ?", record.ID).Order("attempt_index asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load call attempts: %v", err)
	}
	if len(attempts) != 3 {
		t.Fatalf("expected three model call attempts, got %d", len(attempts))
	}
	if attempts[0].ModelConfigID != routeA.ID || attempts[0].Status != ModelCallAttemptStatusFailed || attempts[0].HTTPStatus != http.StatusBadGateway || attempts[0].ErrorCode != "provider_http_502" {
		t.Fatalf("unexpected failed attempt: %+v", attempts[0])
	}
	if attempts[1].ModelConfigID != routeA.ID || attempts[1].Status != ModelCallAttemptStatusFailed || attempts[1].HTTPStatus != http.StatusBadGateway || attempts[1].ErrorCode != "provider_http_502" {
		t.Fatalf("unexpected retry failed attempt: %+v", attempts[1])
	}
	if attempts[2].ModelConfigID != routeB.ID || attempts[2].Status != ModelCallAttemptStatusSucceeded || attempts[2].ProviderRequestID != "req_route_b_success" {
		t.Fatalf("unexpected successful attempt: %+v", attempts[2])
	}

	var events []GenerationEventLog
	if err := testApp.db.Where("generation_record_id = ?", record.ID).Order("created_at asc, id asc").Find(&events).Error; err != nil {
		t.Fatalf("load generation events: %v", err)
	}
	if countGenerationEvents(events, "model_call_attempt_start") != 3 {
		t.Fatalf("expected a start event for each model attempt, got events %+v", events)
	}
	if countGenerationEvents(events, "model_call_attempt_failed") != 2 {
		t.Fatalf("expected two failed attempt events, got events %+v", events)
	}
	if countGenerationEvents(events, "model_call_attempt_succeeded") != 1 {
		t.Fatalf("expected one successful attempt event, got events %+v", events)
	}
}

func TestModelCenterImageGenerationAttemptUsesProviderDefaultTimeout(t *testing.T) {
	provider := &deadlineCaptureProvider{
		result: ImageGenerationResult{ProviderRequestID: "req_model_center_timeout"},
	}
	testApp, _ := newTestApp(t, provider)
	record := GenerationRecord{Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, Prompt: "provider timeout"}
	if err := testApp.db.Create(&record).Error; err != nil {
		t.Fatalf("create generation record: %v", err)
	}

	candidate := modelCenterCandidate{
		Model: ModelCatalog{
			ID:                 11,
			Name:               "GPT Image 2",
			Modality:           ModelConfigTypeImage,
			DefaultCreditsCost: 1,
		},
		Channel: ModelChannel{
			ID:           22,
			ModelID:      11,
			ProviderID:   33,
			Name:         "timeout channel",
			RuntimeModel: "gpt-image-2",
			Endpoint:     "/v1/images/generations",
			Status:       ModelCenterStatusOnline,
		},
		Provider: ModelProvider{
			ID:                    33,
			Name:                  "timeout provider",
			BaseURL:               "https://provider-timeout.example",
			APIKey:                "provider-key",
			DefaultTimeoutSeconds: 75,
			Status:                ModelCenterStatusOnline,
		},
	}
	job := &generationJob{
		Settings:              AppSettings{RequestTimeoutSeconds: 600},
		ModelCenterCandidates: []modelCenterCandidate{candidate},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	result, providerErr, finalCandidate, err := testApp.generateImageWithModelCenterFailover(ctx, 600*time.Second, &record, job, ImageGenerationInput{})
	if err != nil {
		t.Fatalf("generate image with model center failover: %v", err)
	}
	if providerErr != nil {
		t.Fatalf("expected provider success, got %+v", providerErr)
	}
	if result.ProviderRequestID != "req_model_center_timeout" || finalCandidate == nil || finalCandidate.Provider.ID != candidate.Provider.ID {
		t.Fatalf("unexpected result/candidate: result=%+v candidate=%+v", result, finalCandidate)
	}
	assertCapturedAttemptTimeout(t, provider.deadlineConfigured, provider.deadlineRemaining, 75*time.Second)
	assertAttemptStartEventTimeout(t, testApp, record.ID, 75)
}

func TestModelCenterImageGenerationRetriesSameChannelOnceForHTTP502AndSucceeds(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusBadGateway,
				Code:              "provider_http_502",
				Message:           "upstream failed",
				ProviderRequestID: "req_bailinai_502",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("same-channel-retry-success")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_bailinai_retry_success",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	record := GenerationRecord{Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, Prompt: "same channel retry success"}
	if err := testApp.db.Create(&record).Error; err != nil {
		t.Fatalf("create generation record: %v", err)
	}
	candidate := seedModelCenterImageCandidate(t, testApp, "BailinAI GPT Image 2", "BailinAI primary")
	job := &generationJob{
		Settings:              AppSettings{RequestTimeoutSeconds: 600},
		ModelCenterCandidates: []modelCenterCandidate{candidate},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	result, providerErr, finalCandidate, err := testApp.generateImageWithModelCenterFailover(ctx, 600*time.Second, &record, job, ImageGenerationInput{})
	if err != nil {
		t.Fatalf("generate image with model center failover: %v", err)
	}
	if providerErr != nil {
		t.Fatalf("expected retry success, got provider error %+v", providerErr)
	}
	if finalCandidate == nil || finalCandidate.Channel.ID != candidate.Channel.ID {
		t.Fatalf("expected final candidate to stay on same channel, got %+v", finalCandidate)
	}
	if result.ProviderAttemptCount != 2 || result.ProviderRequestID != "req_bailinai_retry_success" {
		t.Fatalf("expected provider attempt count 2 with retry success, got %+v", result)
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ProviderBaseURL != candidate.Provider.BaseURL || provider.inputs[1].ProviderBaseURL != candidate.Provider.BaseURL {
		t.Fatalf("expected same channel to be called twice, got %+v", provider.inputs)
	}

	var attempts []ModelCallAttempt
	if err := testApp.db.Where("generation_record_id = ?", record.ID).Order("attempt_index asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load call attempts: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected two model call attempts, got %d", len(attempts))
	}
	if attempts[0].ChannelID != candidate.Channel.ID || attempts[0].Status != ModelCallAttemptStatusFailed || attempts[0].HTTPStatus != http.StatusBadGateway {
		t.Fatalf("unexpected failed first attempt: %+v", attempts[0])
	}
	if attempts[1].ChannelID != candidate.Channel.ID || attempts[1].Status != ModelCallAttemptStatusSucceeded || attempts[1].ProviderRequestID != "req_bailinai_retry_success" {
		t.Fatalf("unexpected retry success attempt: %+v", attempts[1])
	}

	var channel ModelChannel
	if err := testApp.db.First(&channel, candidate.Channel.ID).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	if channel.HealthStatus == ModelChannelHealthDegraded || channel.FailCooldownUntil != nil {
		t.Fatalf("expected successful retry not to degrade channel, got %+v", channel)
	}
}

func TestModelCenterImageGenerationDegradesChannelAfterSameChannelRetryFailsTwice(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusBadGateway,
				Code:              "provider_http_502",
				Message:           "upstream failed first",
				ProviderRequestID: "req_bailinai_502_first",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			{
				HTTPStatus:        http.StatusBadGateway,
				Code:              "provider_http_502",
				Message:           "upstream failed second",
				ProviderRequestID: "req_bailinai_502_second",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	record := GenerationRecord{Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, Prompt: "same channel retry fails twice"}
	if err := testApp.db.Create(&record).Error; err != nil {
		t.Fatalf("create generation record: %v", err)
	}
	candidate := seedModelCenterImageCandidate(t, testApp, "BailinAI GPT Image 2 fail", "BailinAI failover")
	job := &generationJob{
		Settings:              AppSettings{RequestTimeoutSeconds: 600},
		ModelCenterCandidates: []modelCenterCandidate{candidate},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	result, providerErr, finalCandidate, err := testApp.generateImageWithModelCenterFailover(ctx, 600*time.Second, &record, job, ImageGenerationInput{})
	if err != nil {
		t.Fatalf("generate image with model center failover: %v", err)
	}
	if providerErr == nil {
		t.Fatalf("expected provider error after retry exhausted, got result %+v", result)
	}
	if providerErr.AttemptCount != 2 || providerErr.ProviderRequestID != "req_bailinai_502_second" {
		t.Fatalf("expected second attempt error with attempt count 2, got %+v", providerErr)
	}
	if finalCandidate == nil || finalCandidate.Channel.ID != candidate.Channel.ID {
		t.Fatalf("expected final candidate to be failed channel, got %+v", finalCandidate)
	}
	if len(provider.inputs) != 2 {
		t.Fatalf("expected same channel to be called twice, got %d inputs", len(provider.inputs))
	}

	var attempts []ModelCallAttempt
	if err := testApp.db.Where("generation_record_id = ?", record.ID).Order("attempt_index asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load call attempts: %v", err)
	}
	if len(attempts) != 2 || attempts[0].Status != ModelCallAttemptStatusFailed || attempts[1].Status != ModelCallAttemptStatusFailed {
		t.Fatalf("expected two failed attempts, got %+v", attempts)
	}

	var channel ModelChannel
	if err := testApp.db.First(&channel, candidate.Channel.ID).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	if channel.HealthStatus != ModelChannelHealthDegraded || channel.FailCooldownUntil == nil || channel.LastErrorCode != "provider_http_502" {
		t.Fatalf("expected retry-exhausted channel degradation, got %+v", channel)
	}
}

func TestModelCenterImageGenerationDoesNotRetrySameChannelForAuthFailure(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusUnauthorized,
				Code:              "provider_auth_failed",
				Message:           "invalid api key",
				ProviderRequestID: "req_auth_failed",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("should-not-run")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_should_not_run",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	record := GenerationRecord{Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, Prompt: "auth failure no retry"}
	if err := testApp.db.Create(&record).Error; err != nil {
		t.Fatalf("create generation record: %v", err)
	}
	candidate := seedModelCenterImageCandidate(t, testApp, "BailinAI GPT Image 2 auth", "BailinAI auth")
	job := &generationJob{
		Settings:              AppSettings{RequestTimeoutSeconds: 600},
		ModelCenterCandidates: []modelCenterCandidate{candidate},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	_, providerErr, _, err := testApp.generateImageWithModelCenterFailover(ctx, 600*time.Second, &record, job, ImageGenerationInput{})
	if err != nil {
		t.Fatalf("generate image with model center failover: %v", err)
	}
	if providerErr == nil || providerErr.AttemptCount != 1 {
		t.Fatalf("expected one auth failure attempt, got %+v", providerErr)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("expected auth failure not to retry same channel, got %d inputs", len(provider.inputs))
	}
}

func TestLegacyImageGenerationAttemptUsesRequestTimeoutSetting(t *testing.T) {
	provider := &deadlineCaptureProvider{
		result: ImageGenerationResult{ProviderRequestID: "req_legacy_timeout"},
	}
	testApp, _ := newTestApp(t, provider)
	record := GenerationRecord{Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, Prompt: "settings timeout"}
	if err := testApp.db.Create(&record).Error; err != nil {
		t.Fatalf("create generation record: %v", err)
	}

	model := ModelConfig{
		ID:           44,
		Name:         "legacy timeout model",
		Type:         ModelConfigTypeImage,
		Status:       ModelConfigStatusOnline,
		RuntimeModel: "gpt-image-2",
		APIBaseURL:   "https://legacy-timeout.example",
		APIEndpoint:  "/v1/images/generations",
		APIKey:       "legacy-key",
	}
	job := &generationJob{
		Settings:        AppSettings{RequestTimeoutSeconds: 120},
		ModelCandidates: []ModelConfig{model},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, providerErr, finalModel, err := testApp.generateImageWithFailover(ctx, 120*time.Second, &record, job, ImageGenerationInput{})
	if err != nil {
		t.Fatalf("generate image with legacy failover: %v", err)
	}
	if providerErr != nil {
		t.Fatalf("expected provider success, got %+v", providerErr)
	}
	if result.ProviderRequestID != "req_legacy_timeout" || finalModel == nil || finalModel.ID != model.ID {
		t.Fatalf("unexpected result/model: result=%+v model=%+v", result, finalModel)
	}
	assertCapturedAttemptTimeout(t, provider.deadlineConfigured, provider.deadlineRemaining, 120*time.Second)
	assertAttemptStartEventTimeout(t, testApp, record.ID, 120)
}

func assertCapturedAttemptTimeout(t *testing.T, configured []bool, remaining []time.Duration, expected time.Duration) {
	t.Helper()
	if len(configured) != 1 || !configured[0] || len(remaining) != 1 {
		t.Fatalf("expected one configured provider deadline, configured=%v remaining=%v", configured, remaining)
	}
	lowerBound := expected - 5*time.Second
	upperBound := expected + time.Second
	if remaining[0] < lowerBound || remaining[0] > upperBound {
		t.Fatalf("expected provider deadline around %s, got %s", expected, remaining[0])
	}
}

func assertAttemptStartEventTimeout(t *testing.T, app *App, generationID uint, expectedSeconds int) {
	t.Helper()
	var event GenerationEventLog
	if err := app.db.Where("generation_record_id = ? AND event = ?", generationID, "model_call_attempt_start").First(&event).Error; err != nil {
		t.Fatalf("load attempt start event: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(event.MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode attempt start metadata: %v", err)
	}
	value, ok := metadata["attempt_timeout_seconds"].(float64)
	if !ok || int(value) != expectedSeconds {
		t.Fatalf("expected attempt_timeout_seconds=%d, got metadata %s", expectedSeconds, event.MetadataJSON)
	}
}

func seedModelCenterImageCandidate(t *testing.T, app *App, modelName, channelName string) modelCenterCandidate {
	t.Helper()
	model := ModelCatalog{
		Name:               modelName,
		Modality:           ModelConfigTypeImage,
		DefaultCreditsCost: 1,
		Status:             ModelCenterStatusOnline,
		Visibility:         ModelCenterVisibilityPublic,
	}
	if err := app.db.Create(&model).Error; err != nil {
		t.Fatalf("create model catalog: %v", err)
	}
	provider := ModelProvider{
		Name:                  channelName + " provider",
		BaseURL:               "https://" + strings.ToLower(strings.ReplaceAll(channelName, " ", "-")) + ".example",
		APIKey:                "provider-key",
		DefaultTimeoutSeconds: 75,
		Status:                ModelCenterStatusOnline,
	}
	if err := app.db.Create(&provider).Error; err != nil {
		t.Fatalf("create model provider: %v", err)
	}
	channel := ModelChannel{
		ModelID:      model.ID,
		ProviderID:   provider.ID,
		Name:         channelName,
		RuntimeModel: "gpt-image-2",
		Endpoint:     "/v1/images/generations",
		Weight:       100,
		Status:       ModelCenterStatusOnline,
		HealthStatus: ModelChannelHealthHealthy,
	}
	if err := app.db.Create(&channel).Error; err != nil {
		t.Fatalf("create model channel: %v", err)
	}
	return modelCenterCandidate{Model: model, Channel: channel, Provider: provider}
}

func TestImageGenerationFailoverForProviderUnavailableWithoutHTTPStatus(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				Code:              "provider_unavailable",
				Message:           "route unavailable",
				ProviderRequestID: "req_unavailable_no_status",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("provider-unavailable-failover")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_unavailable_route_b",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_unavailable_no_status", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "unavailable route a", "https://unavailable-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "unavailable route b", "https://unavailable-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "无状态不可用错误切换",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after provider_unavailable failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL || provider.inputs[1].ProviderBaseURL != routeB.APIBaseURL {
		t.Fatalf("expected failover order A -> B, got %+v", provider.inputs)
	}
}

func TestImageGenerationFailoverForProviderHTTPCodeWithoutHTTPStatus(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				Code:              "provider_http_502",
				Message:           "bad gateway without status first",
				ProviderRequestID: "req_http_code_no_status_1",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			{
				Code:              "provider_http_502",
				Message:           "bad gateway without status second",
				ProviderRequestID: "req_http_code_no_status_2",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("provider-http-code-failover")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_http_code_route_b",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_http_code_no_status", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "http code route a", "https://http-code-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "http code route b", "https://http-code-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "无状态 HTTP 代码切换",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after provider_http_502 failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 3 || provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL || provider.inputs[1].ProviderBaseURL != routeA.APIBaseURL || provider.inputs[2].ProviderBaseURL != routeB.APIBaseURL {
		t.Fatalf("expected failover order A -> A -> B, got %+v", provider.inputs)
	}
}

func TestImageGenerationFailoverForHTTP400InvalidModel(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusBadRequest,
				Code:              "invalid_model",
				Message:           "model is not available",
				ProviderRequestID: "req_invalid_model",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("invalid-model-failover")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_invalid_model_route_b",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_invalid_model_failover", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "invalid model route a", "https://invalid-model-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "invalid model route b", "https://invalid-model-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "模型无效时切换",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after invalid_model failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL || provider.inputs[1].ProviderBaseURL != routeB.APIBaseURL {
		t.Fatalf("expected failover order A -> B, got %+v", provider.inputs)
	}
}

func TestImageGenerationHTTP400ParameterErrorDoesNotFailover(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusBadRequest,
				Code:              "invalid_request",
				Message:           "prompt parameter is invalid",
				ProviderRequestID: "req_invalid_request",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("should-not-failover")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_invalid_request_route_b",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_invalid_request_no_failover", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "invalid request route a", "https://invalid-request-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "invalid request route b", "https://invalid-request-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "参数错误不切换",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected invalid request provider error without failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 || provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL {
		t.Fatalf("expected only route A to be attempted, got %+v", provider.inputs)
	}
}

func TestImageGenerationPolicyRejectionDoesNotFailover(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				HTTPStatus:        http.StatusBadRequest,
				Code:              "moderation_blocked",
				Message:           "rejected by the safety system",
				ProviderRequestID: "req_policy",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("should-not-run")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_should_not_run",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_policy_no_failover", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "policy route a", "https://policy-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "policy route b", "https://policy-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "触发安全策略",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected policy rejection 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 || provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL {
		t.Fatalf("expected no failover for policy rejection, got %+v", provider.inputs)
	}

	var attempts []ModelCallAttempt
	if err := testApp.db.Order("attempt_index asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load call attempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].ModelConfigID != routeA.ID || attempts[0].Status != ModelCallAttemptStatusFailed || attempts[0].ErrorCode != "moderation_blocked" {
		t.Fatalf("expected one failed policy attempt, got %+v", attempts)
	}

	var routeBAttempts int64
	if err := testApp.db.Model(&ModelCallAttempt{}).Where("model_config_id = ?", routeB.ID).Count(&routeBAttempts).Error; err != nil {
		t.Fatalf("count route B attempts: %v", err)
	}
	if routeBAttempts != 0 {
		t.Fatalf("expected no attempts for route B, got %d", routeBAttempts)
	}
}

func TestImageGenerationAllCandidatesFailDoesNotDeductCredits(t *testing.T) {
	provider := &stubProvider{
		err: &ProviderError{
			HTTPStatus:        http.StatusBadGateway,
			Code:              "provider_http_502",
			Message:           "upstream failed",
			ProviderRequestID: "req_all_failed",
			FailureStage:      providerFailureStageImageGenerationRequest,
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_all_fail_no_credit", "test-password")
	setUserCredits(t, testApp, user.ID, 2)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "all fail route a", "https://all-fail-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "all fail route b", "https://all-fail-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "全部模型都失败",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected bad gateway after all candidates fail, got %d: %s", resp.Code, resp.Body.String())
	}
	if provider.calls != imageQueueMaxExternalAttempts {
		t.Fatalf("expected external calls to stop at %d, got %d calls", imageQueueMaxExternalAttempts, provider.calls)
	}

	var record GenerationRecord
	if err := testApp.db.Where("user_id = ? AND prompt = ?", user.ID, "全部模型都失败").First(&record).Error; err != nil {
		t.Fatalf("load failed generation record: %v", err)
	}
	if record.Status != GenerationStatusFailed || record.ModelConfigID != routeB.ID || record.ProviderAttemptCount != imageQueueMaxExternalAttempts || record.CreditsDeducted {
		t.Fatalf("expected failed main record on last model without credit deduction, got %+v", record)
	}

	var balance CreditBalance
	if err := testApp.db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if balance.AvailableCredits != 2 {
		t.Fatalf("expected failed generation not to deduct credits, got %d", balance.AvailableCredits)
	}
}

func TestImageGenerationSkipsModelInRecentFailoverCooldown(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("cooldown-route-b-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_cooldown_route_b",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_cooldown_skip", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "cooldown route a", "https://cooldown-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "cooldown route b", "https://cooldown-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)
	seedModelCallAttempt(t, testApp, ModelCallAttempt{
		ModelConfigID: routeA.ID,
		Status:        ModelCallAttemptStatusFailed,
		ErrorCode:     "provider_timeout",
		StartedAt:     time.Now().Add(-30 * time.Second),
		FinishedAt:    time.Now().Add(-30 * time.Second),
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "跳过冷却中的模型",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 with cooldown skip, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 || provider.inputs[0].ProviderBaseURL != routeB.APIBaseURL {
		t.Fatalf("expected provider to skip route A and call route B, got %+v", provider.inputs)
	}
}

func TestImageGenerationFailoverCooldownDemotesButKeepsCandidate(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				Code:              "provider_unavailable",
				Message:           "non-cooling route failed",
				ProviderRequestID: "req_non_cooling_failed",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("cooling-route-recovered")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_cooling_route_recovered",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_cooldown_demote_keep", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "demoted cooldown route a", "https://demoted-cooldown-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "preferred route b", "https://preferred-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)
	seedModelCallAttempt(t, testApp, ModelCallAttempt{
		ModelConfigID: routeA.ID,
		Status:        ModelCallAttemptStatusFailed,
		ErrorCode:     "provider_timeout",
		StartedAt:     time.Now().Add(-30 * time.Second),
		FinishedAt:    time.Now().Add(-30 * time.Second),
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "冷却模型降级但保留",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after demoted candidate recovers, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ProviderBaseURL != routeB.APIBaseURL || provider.inputs[1].ProviderBaseURL != routeA.APIBaseURL {
		t.Fatalf("expected cooldown to demote route A behind route B without removing it, got %+v", provider.inputs)
	}
}

func TestImageGenerationSpeedFirstFailoverIncludesDefaultModelOutsideRoutingCandidates(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				Code:              "provider_unavailable",
				Message:           "speed route unavailable",
				ProviderRequestID: "req_speed_route_failed",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("speed-first-default-fallback")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_speed_default_success",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_speed_first_failover_default", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	adminCookies := createAdminSession(t, testApp)

	disableSeedImageRoutes(t, testApp)
	defaultModel := seedMinimalOnlineImageModel(t, testApp, "speed first default outside routing", 101)
	route := seedImageRoutingModel(t, testApp, "speed first route", "https://speed-first-route.example", 102)
	saveImageRoutingStrategy(t, testApp, adminCookies, ModelRoutingStrategySpeedFirst, defaultModel.ID, defaultModel.ID, []map[string]any{
		{"id": route.ID, "weight": 100},
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "速度优先失败后补默认模型",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after speed-first default failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ProviderBaseURL != route.APIBaseURL || provider.inputs[1].ProviderBaseURL != "" {
		t.Fatalf("expected speed-first route then default model outside routing candidates, got %+v", provider.inputs)
	}
}

func TestImageGenerationRoundRobinFailoverIncludesFallbackModelOutsideRoutingCandidates(t *testing.T) {
	provider := &stubProvider{
		errs: []*ProviderError{
			{
				Code:              "provider_unavailable",
				Message:           "round robin route unavailable",
				ProviderRequestID: "req_round_robin_route_failed",
				FailureStage:      providerFailureStageImageGenerationRequest,
			},
			nil,
		},
		results: []ImageGenerationResult{
			{
				Base64Image:       base64.StdEncoding.EncodeToString([]byte("round-robin-fallback")),
				MIMEType:          "image/png",
				ProviderRequestID: "req_round_robin_fallback_success",
			},
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_round_robin_failover_fallback", "test-password")
	setUserCredits(t, testApp, user.ID, 3)
	adminCookies := createAdminSession(t, testApp)

	disableSeedImageRoutes(t, testApp)
	route := seedImageRoutingModel(t, testApp, "round robin route", "https://round-robin-route.example", 101)
	fallbackModel := seedMinimalOnlineImageModel(t, testApp, "round robin fallback outside routing", 102)
	saveImageRoutingStrategy(t, testApp, adminCookies, ModelRoutingStrategyRoundRobin, route.ID, fallbackModel.ID, []map[string]any{
		{"id": route.ID, "weight": 100},
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "轮询失败后补回退模型",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after round-robin fallback failover, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ProviderBaseURL != route.APIBaseURL || provider.inputs[1].ProviderBaseURL != "" {
		t.Fatalf("expected round-robin route then fallback model outside routing candidates, got %+v", provider.inputs)
	}
}

func TestImageGenerationRestoresModelAfterFailoverCooldownExpires(t *testing.T) {
	provider := &stubProvider{
		result: ImageGenerationResult{
			Base64Image:       base64.StdEncoding.EncodeToString([]byte("expired-cooldown-route-a-image")),
			MIMEType:          "image/png",
			ProviderRequestID: "req_expired_cooldown_route_a",
		},
	}
	testApp, _ := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, testApp, "creator_cooldown_restore", "test-password")
	setUserCredits(t, testApp, user.ID, 3)

	disableSeedImageRoutes(t, testApp)
	routeA := seedImageRoutingModel(t, testApp, "expired cooldown route a", "https://expired-cooldown-a.example", 101)
	routeB := seedImageRoutingModel(t, testApp, "expired cooldown route b", "https://expired-cooldown-b.example", 102)
	saveDefaultImageRoutes(t, testApp, routeA.ID, routeB.ID)
	seedModelCallAttempt(t, testApp, ModelCallAttempt{
		ModelConfigID: routeA.ID,
		Status:        ModelCallAttemptStatusFailed,
		ErrorCode:     "provider_timeout",
		StartedAt:     time.Now().Add(-6 * time.Minute),
		FinishedAt:    time.Now().Add(-6 * time.Minute),
	})

	resp := performJSONRequest(t, testApp, http.MethodPost, "/api/images/generations", map[string]any{
		"prompt":       "冷却过期后恢复模型",
		"aspect_ratio": "1:1",
	}, cookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected generation 200 after cooldown expires, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(provider.inputs) != 1 || provider.inputs[0].ProviderBaseURL != routeA.APIBaseURL {
		t.Fatalf("expected route A to rejoin candidates after cooldown, got %+v", provider.inputs)
	}
}

func TestAdminModelDetailReturnsRecentCallAttempts(t *testing.T) {
	testApp, _ := newTestApp(t, &stubProvider{})
	adminCookies := createAdminSession(t, testApp)

	var model ModelConfig
	if err := testApp.db.Where("name = ?", "DALL-E 3").First(&model).Error; err != nil {
		t.Fatalf("load DALL-E model: %v", err)
	}
	now := time.Now()
	record := seedAdminGenerationRecord(t, testApp, GenerationRecord{
		UserID:        17,
		Prompt:        "后台查看尝试记录",
		ModelConfigID: model.ID,
		Model:         model.RuntimeModel,
		Status:        GenerationStatusSucceeded,
		LatencyMS:     1200,
		CreatedAt:     now,
	})
	seedModelCallAttempt(t, testApp, ModelCallAttempt{
		GenerationRecordID: record.ID,
		ModelConfigID:      model.ID,
		AttemptIndex:       1,
		Status:             ModelCallAttemptStatusFailed,
		LatencyMS:          540,
		HTTPStatus:         http.StatusBadGateway,
		ErrorCode:          "provider_http_502",
		ErrorMessage:       "upstream failed",
		ProviderRequestID:  "req_attempt_failed",
		StartedAt:          now.Add(-2 * time.Minute),
		FinishedAt:         now.Add(-2*time.Minute + 540*time.Millisecond),
	})
	seedModelCallAttempt(t, testApp, ModelCallAttempt{
		GenerationRecordID: record.ID,
		ModelConfigID:      model.ID,
		AttemptIndex:       2,
		Status:             ModelCallAttemptStatusSucceeded,
		LatencyMS:          320,
		ProviderRequestID:  "req_attempt_success",
		StartedAt:          now.Add(-time.Minute),
		FinishedAt:         now.Add(-time.Minute + 320*time.Millisecond),
	})

	resp := performJSONRequest(t, testApp, http.MethodGet, fmt.Sprintf("/api/admin/models/%d", model.ID), nil, adminCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected model detail 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Usage struct {
			TotalCalls     int64 `json:"total_calls"`
			SucceededCalls int64 `json:"succeeded_calls"`
			FailedCalls    int64 `json:"failed_calls"`
			AverageLatency int64 `json:"average_latency_ms"`
		} `json:"usage"`
		RecentCallAttempts []struct {
			GenerationRecordID uint   `json:"generation_record_id"`
			ModelConfigID      uint   `json:"model_config_id"`
			AttemptIndex       int    `json:"attempt_index"`
			Status             string `json:"status"`
			LatencyMS          int64  `json:"latency_ms"`
			HTTPStatus         int    `json:"http_status"`
			ErrorCode          string `json:"error_code"`
			ErrorMessage       string `json:"error_message"`
			ProviderRequestID  string `json:"provider_request_id"`
		} `json:"recent_call_attempts"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode model detail payload: %v", err)
	}
	if payload.Usage.TotalCalls != 2 || payload.Usage.SucceededCalls != 1 || payload.Usage.FailedCalls != 1 || payload.Usage.AverageLatency != 430 {
		t.Fatalf("expected usage to prefer attempt records, got %+v", payload.Usage)
	}
	if len(payload.RecentCallAttempts) != 2 {
		t.Fatalf("expected two recent call attempts, got %+v", payload.RecentCallAttempts)
	}
	if payload.RecentCallAttempts[0].Status != ModelCallAttemptStatusSucceeded || payload.RecentCallAttempts[0].ProviderRequestID != "req_attempt_success" {
		t.Fatalf("expected newest success attempt first, got %+v", payload.RecentCallAttempts[0])
	}
	if payload.RecentCallAttempts[1].Status != ModelCallAttemptStatusFailed || payload.RecentCallAttempts[1].HTTPStatus != http.StatusBadGateway || payload.RecentCallAttempts[1].ErrorCode != "provider_http_502" {
		t.Fatalf("expected failed attempt diagnostics, got %+v", payload.RecentCallAttempts[1])
	}
}

func saveDefaultImageRoutes(t *testing.T, app *App, defaultImageID, fallbackID uint) {
	t.Helper()
	if err := app.db.Model(&AppSettings{}).Where("id = ?", 1).Updates(map[string]any{
		"default_image_model_id": defaultImageID,
		"fallback_model_id":      fallbackID,
		"model_routing_enabled":  false,
		"model_routing_strategy": ModelRoutingStrategyDefault,
	}).Error; err != nil {
		t.Fatalf("save default image routes: %v", err)
	}
}

func seedModelCallAttempt(t *testing.T, app *App, attempt ModelCallAttempt) ModelCallAttempt {
	t.Helper()
	if attempt.StartedAt.IsZero() {
		attempt.StartedAt = time.Now()
	}
	if attempt.FinishedAt.IsZero() {
		attempt.FinishedAt = attempt.StartedAt
	}
	if err := app.db.Create(&attempt).Error; err != nil {
		t.Fatalf("seed model call attempt: %v", err)
	}
	return attempt
}

func seedMinimalOnlineImageModel(t *testing.T, app *App, name string, sortOrder int) ModelConfig {
	t.Helper()
	model := ModelConfig{
		Name:       name,
		Type:       ModelConfigTypeImage,
		Provider:   "OpenAI",
		Status:     ModelConfigStatusOnline,
		Priority:   sortOrder,
		CostLabel:  "按配置扣点",
		Permission: ModelConfigPermissionPublic,
		Weight:     50,
		SortOrder:  sortOrder,
	}
	if err := app.db.Create(&model).Error; err != nil {
		t.Fatalf("create minimal image model: %v", err)
	}
	return model
}

func countGenerationEvents(events []GenerationEventLog, name string) int {
	count := 0
	for _, event := range events {
		if event.Event == name {
			count++
		}
	}
	return count
}
