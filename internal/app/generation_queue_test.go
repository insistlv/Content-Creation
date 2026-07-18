package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type queueConcurrencyProvider struct {
	release chan struct{}
	calls   int32
	active  int32
	max     int32
}

func newQueueConcurrencyProvider() *queueConcurrencyProvider {
	return &queueConcurrencyProvider{release: make(chan struct{})}
}

func (p *queueConcurrencyProvider) Generate(ctx context.Context, _ ImageGenerationInput) (ImageGenerationResult, *ProviderError) {
	call := atomic.AddInt32(&p.calls, 1)
	active := atomic.AddInt32(&p.active, 1)
	for {
		current := atomic.LoadInt32(&p.max)
		if active <= current || atomic.CompareAndSwapInt32(&p.max, current, active) {
			break
		}
	}
	defer atomic.AddInt32(&p.active, -1)
	select {
	case <-p.release:
	case <-ctx.Done():
		return ImageGenerationResult{}, &ProviderError{Code: "provider_request_failed", Message: ctx.Err().Error()}
	}
	return ImageGenerationResult{
		Base64Image: base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("image-%d", call))),
		MIMEType:    "image/png", ProviderRequestID: fmt.Sprintf("queue-%d", call),
	}, nil
}

func TestPersistentGenerationQueueAcceptsSixteenAndCapsExecutionAtFour(t *testing.T) {
	provider := newQueueConcurrencyProvider()
	application, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, application, "queue_batch_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Updates(map[string]any{"available_credits": 100, "reserved_credits": 0}).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		if err := db.Create(&ImageExecutionLease{Token: fmt.Sprintf("submission-gate-%d", index), Owner: "test", EntryPoint: "submission_gate", ExpiresAt: time.Now().UTC().Add(time.Minute)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	submissionStarted := time.Now()
	for index := 0; index < 16; index++ {
		response := performJSONRequest(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{
			"prompt": fmt.Sprintf("queue task %d", index), "aspect_ratio": "1:1", "batch_id": "queue-batch-16",
			"batch_index": index, "batch_total": 16,
		}, cookies)
		if response.Code != http.StatusAccepted {
			t.Fatalf("task %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
	if elapsed := time.Since(submissionStarted); elapsed > 3*time.Second {
		t.Fatalf("16 queued submissions exceeded 3 seconds: %s", elapsed)
	}

	var jobs int64
	if err := db.Model(&ImageGenerationJob{}).Count(&jobs).Error; err != nil || jobs != 16 {
		t.Fatalf("queued jobs=%d err=%v", jobs, err)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.AvailableCredits != 84 || balance.ReservedCredits != 16 {
		t.Fatalf("reserved balance available=%d reserved=%d", balance.AvailableCredits, balance.ReservedCredits)
	}
	if err := db.Where("owner = ?", "test").Delete(&ImageExecutionLease{}).Error; err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return atomic.LoadInt32(&provider.calls) == 4 })
	if got := atomic.LoadInt32(&provider.max); got > 4 {
		t.Fatalf("active provider calls exceeded global limit: %d", got)
	}

	close(provider.release)
	waitFor(t, 8*time.Second, func() bool {
		var succeeded int64
		_ = db.Model(&ImageGenerationJob{}).Where("status = ?", ImageGenerationJobStatusSucceeded).Count(&succeeded).Error
		return succeeded == 16
	})
	if got := atomic.LoadInt32(&provider.max); got > 4 {
		t.Fatalf("peak provider calls exceeded global limit: %d", got)
	}
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.AvailableCredits != 84 || balance.ReservedCredits != 0 {
		t.Fatalf("settled balance available=%d reserved=%d", balance.AvailableCredits, balance.ReservedCredits)
	}
}

func TestProviderConcurrencyZeroInheritsGlobalLimit(t *testing.T) {
	application, db := newTestApp(t, &stubProvider{})
	provider := ModelProvider{Name: "queue-limit-provider", Provider: "test", Status: ModelCenterStatusOnline, ConcurrencyLimit: 0}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	if got := application.providerExecutionLimit(provider.ID, 4); got != 4 {
		t.Fatalf("zero concurrency should inherit global 4, got %d", got)
	}
	if err := db.Model(&provider).Update("concurrency_limit", 2).Error; err != nil {
		t.Fatal(err)
	}
	if got := application.providerExecutionLimit(provider.ID, 4); got != 2 {
		t.Fatalf("provider concurrency should use min(provider, global)=2, got %d", got)
	}
}

func TestGenerationQueueFairSelectionPreservesPriorityAndUserFIFO(t *testing.T) {
	now := time.Now().UTC()
	candidates := []ImageGenerationJob{
		{ID: 1, UserID: 10, Priority: 5, QueuedAt: now},
		{ID: 2, UserID: 10, Priority: 5, QueuedAt: now.Add(time.Millisecond)},
		{ID: 3, UserID: 20, Priority: 5, QueuedAt: now.Add(2 * time.Millisecond)},
		{ID: 4, UserID: 30, Priority: 4, QueuedAt: now.Add(3 * time.Millisecond)},
	}
	selected, ok := selectFairGenerationCandidate(candidates, map[uint]int64{10: 2, 20: 0, 30: 0})
	if !ok || selected.ID != 3 {
		t.Fatalf("expected less-active user at the highest priority, got %+v", selected)
	}
	selected, ok = selectFairGenerationCandidate(candidates, map[uint]int64{10: 0, 20: 0, 30: 0})
	if !ok || selected.ID != 1 {
		t.Fatalf("expected FIFO candidate when user activity is equal, got %+v", selected)
	}
}

func TestGenerationQueueOldLeaseCannotFinalizeJob(t *testing.T) {
	application, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, application, "queue_lease_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Updates(map[string]any{"available_credits": 9, "reserved_credits": 1}).Error; err != nil {
		t.Fatal(err)
	}
	record := GenerationRecord{UserID: user.ID, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, CreditsCost: 1}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	queueJob := ImageGenerationJob{GenerationRecordID: record.ID, UserID: user.ID, Status: ImageGenerationJobStatusRunning, LeaseToken: "new-token", ReservedCredits: 1, QueuedAt: time.Now(), QueueDeadlineAt: time.Now().Add(time.Minute)}
	if err := db.Create(&queueJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := application.failClaimedGenerationJob(queueJob, "old-token", "generation_failed", "late result"); err != errGenerationQueueLeaseLost {
		t.Fatalf("old lease error=%v", err)
	}
	var stored ImageGenerationJob
	if err := db.First(&stored, queueJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != ImageGenerationJobStatusRunning || stored.CreditsReleased {
		t.Fatalf("old lease changed job: %+v", stored)
	}
}

func TestGenerationQueueLeaseRenewalUsesCurrentTokenCAS(t *testing.T) {
	application, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, application, "queue_renew_user", "test-password")
	record := GenerationRecord{UserID: user.ID, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, CreditsCost: 1}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	originalExpiry := time.Now().UTC().Add(time.Second)
	queueJob := ImageGenerationJob{
		GenerationRecordID: record.ID, UserID: user.ID, Status: ImageGenerationJobStatusRunning,
		LeaseToken: "current-token", LeaseExpiresAt: &originalExpiry, QueuedAt: time.Now().UTC(), QueueDeadlineAt: time.Now().UTC().Add(time.Minute),
	}
	if err := db.Create(&queueJob).Error; err != nil {
		t.Fatal(err)
	}
	if application.renewGenerationQueueLeaseOnce(queueJob.ID, "old-token", time.Now().UTC()) {
		t.Fatal("old lease token must not renew queue job")
	}
	if !application.renewGenerationQueueLeaseOnce(queueJob.ID, "current-token", time.Now().UTC()) {
		t.Fatal("current lease token should renew queue job")
	}
	if err := db.First(&queueJob, queueJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if queueJob.LeaseExpiresAt == nil || !queueJob.LeaseExpiresAt.After(originalExpiry) {
		t.Fatalf("queue lease was not extended: %+v", queueJob.LeaseExpiresAt)
	}
}

func TestGenerationQueueIdempotencyReturnsOriginalAndRejectsConflict(t *testing.T) {
	provider := newQueueConcurrencyProvider()
	application, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, application, "queue_idempotency_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Update("available_credits", 10).Error; err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Idempotency-Key": "same-generation-request"}
	request := map[string]any{"prompt": "same prompt", "aspect_ratio": "1:1"}
	first := performJSONRequestWithHeaders(t, application, http.MethodPost, "/api/images/generations/async", request, cookies, headers)
	second := performJSONRequestWithHeaders(t, application, http.MethodPost, "/api/images/generations/async", request, cookies, headers)
	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted {
		t.Fatalf("idempotent responses=%d/%d first=%s second=%s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	var firstPayload, secondPayload struct {
		GenerationID uint `json:"generation_id"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &firstPayload)
	_ = json.Unmarshal(second.Body.Bytes(), &secondPayload)
	if firstPayload.GenerationID == 0 || firstPayload.GenerationID != secondPayload.GenerationID {
		t.Fatalf("generation ids differ: %d/%d", firstPayload.GenerationID, secondPayload.GenerationID)
	}
	conflict := performJSONRequestWithHeaders(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{"prompt": "different prompt", "aspect_ratio": "1:1"}, cookies, headers)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	var count int64
	if err := db.Model(&ImageGenerationJob{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("idempotent queue rows=%d err=%v", count, err)
	}
	if err := db.Model(&ImageGenerationJob{}).Where("user_id = ? AND idempotency_key = ?", user.ID, headers["Idempotency-Key"]).Update("idempotency_expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	afterExpiry := performJSONRequestWithHeaders(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{"prompt": "different after retention", "aspect_ratio": "1:1"}, cookies, headers)
	if afterExpiry.Code != http.StatusAccepted {
		t.Fatalf("expired idempotency status=%d body=%s", afterExpiry.Code, afterExpiry.Body.String())
	}
	if err := db.Model(&ImageGenerationJob{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("queue rows after retention expiry=%d err=%v", count, err)
	}
	close(provider.release)
}

func TestGenerationQueueCapacityReturnsRetryable429(t *testing.T) {
	provider := newQueueConcurrencyProvider()
	application, db := newTestApp(t, provider)
	application.cfg.GenerationQueueCapacity = 2
	user, cookies := createLoggedInUser(t, application, "queue_capacity_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Update("available_credits", 10).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		response := performJSONRequest(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{"prompt": fmt.Sprintf("capacity-%d", index), "aspect_ratio": "1:1"}, cookies)
		if response.Code != http.StatusAccepted {
			t.Fatalf("accepted status=%d body=%s", response.Code, response.Body.String())
		}
	}
	full := performJSONRequest(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{"prompt": "capacity-full", "aspect_ratio": "1:1"}, cookies)
	if full.Code != http.StatusTooManyRequests || full.Header().Get("Retry-After") != "15" {
		t.Fatalf("queue full status=%d retry-after=%q body=%s", full.Code, full.Header().Get("Retry-After"), full.Body.String())
	}
	close(provider.release)
}

func TestGenerationQueueFinalFailureReleasesReservedCreditsOnce(t *testing.T) {
	application, db := newTestApp(t, &stubProvider{err: &ProviderError{HTTPStatus: http.StatusBadRequest, Code: "invalid_request", Message: "bad request"}})
	user, cookies := createLoggedInUser(t, application, "queue_release_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Updates(map[string]any{"available_credits": 10, "reserved_credits": 0}).Error; err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{"prompt": "release credits", "aspect_ratio": "1:1"}, cookies)
	if response.Code != http.StatusAccepted {
		t.Fatalf("enqueue status=%d body=%s", response.Code, response.Body.String())
	}
	waitFor(t, 3*time.Second, func() bool {
		var failed int64
		_ = db.Model(&ImageGenerationJob{}).Where("status = ?", ImageGenerationJobStatusFailed).Count(&failed).Error
		return failed == 1
	})
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.AvailableCredits != 10 || balance.ReservedCredits != 0 {
		t.Fatalf("released balance available=%d reserved=%d", balance.AvailableCredits, balance.ReservedCredits)
	}
	var releases int64
	if err := db.Model(&CreditTransaction{}).Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypeGenerationRelease).Count(&releases).Error; err != nil || releases != 1 {
		t.Fatalf("release transactions=%d err=%v", releases, err)
	}
}

func TestGenerationQueueRecoveryStopsUnknownNonIdempotentRequest(t *testing.T) {
	application, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, application, "queue_recovery_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Updates(map[string]any{"available_credits": 9, "reserved_credits": 1}).Error; err != nil {
		t.Fatal(err)
	}
	record := GenerationRecord{UserID: user.ID, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, CreditsCost: 1, ProviderRequestStarted: true, ProviderIdempotencySupported: false}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Minute)
	queueJob := ImageGenerationJob{GenerationRecordID: record.ID, UserID: user.ID, IdempotencyKey: "recovery-key", Status: ImageGenerationJobStatusRunning, Stage: GenerationStageRequestingProvider, LeaseToken: "dead-worker", LeaseExpiresAt: &expired, ProviderRequestStarted: true, ReservedCredits: 1, QueuedAt: expired, QueueDeadlineAt: time.Now().Add(time.Minute)}
	if err := db.Create(&queueJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := application.recoverGenerationQueue(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&record, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.Status != GenerationStatusFailed || record.ErrorCode != "provider_result_unknown" {
		t.Fatalf("unexpected recovered record: %+v", record)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.AvailableCredits != 10 || balance.ReservedCredits != 0 {
		t.Fatalf("recovery did not release credits: %+v", balance)
	}
}

func TestGenerationQueueRecoveryFinalizesAlreadyPersistedResult(t *testing.T) {
	application, db := newTestApp(t, &stubProvider{})
	user, _ := createLoggedInUser(t, application, "queue_persisted_recovery_user", "test-password")
	record := GenerationRecord{UserID: user.ID, Status: GenerationStatusSucceeded, Stage: GenerationStageSucceeded, CreditsCost: 1, CreditsDeducted: true}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	spoolPath := filepath.Join(t.TempDir(), "completed.png")
	if err := os.WriteFile(spoolPath, []byte("complete-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	queueJob := ImageGenerationJob{
		GenerationRecordID: record.ID, UserID: user.ID, Status: ImageGenerationJobStatusRunning,
		Stage: "persisting", LeaseToken: "expired-token", LeaseExpiresAt: &expired, SpoolPath: spoolPath,
		CreditsSettled: true, QueuedAt: time.Now().UTC().Add(-time.Minute), QueueDeadlineAt: time.Now().UTC().Add(time.Minute),
	}
	if err := db.Create(&queueJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := application.recoverGenerationQueue(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&queueJob, queueJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if queueJob.Status != ImageGenerationJobStatusSucceeded || queueJob.SpoolPath != "" {
		t.Fatalf("persisted queue job not finalized: %+v", queueJob)
	}
	if _, err := os.Stat(spoolPath); !os.IsNotExist(err) {
		t.Fatalf("completed spool file should be removed, stat err=%v", err)
	}
}

func TestGenerationSpoolPersistenceExpiresAfterFiveMinutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.png")
	if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if generationSpoolPersistenceExpired(path, now) {
		t.Fatal("fresh spool file must remain retryable")
	}
	old := now.Add(-imageGenerationPersistenceLimit - time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if !generationSpoolPersistenceExpired(path, now) {
		t.Fatal("old spool file must exceed persistence deadline")
	}
}

func TestReferenceImageUsesSpoolFileAndMultipartStreamsIt(t *testing.T) {
	application, _ := newTestApp(t, &stubProvider{})
	assetKey, _, err := application.assetStore.SaveBytes([]byte("streamed-reference-image"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	input, err := application.buildReferenceImageInput(assetKey, "image/png", StorageScopeDefault)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(input.FilePath)
	if input.FilePath == "" || input.Base64Data != "" {
		t.Fatalf("reference input should use a spool file: %+v", input)
	}
	body, _, cleanup, err := buildImagesEditsMultipartTempFile(application.cfg.GenerationSpoolPath, ImageGenerationInput{Model: "test-image", Size: "1024x1024", Quality: GenerationQualityMedium}, "stream it", []ReferenceImageInput{input})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	payload, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "streamed-reference-image") {
		t.Fatal("multipart request did not contain streamed reference payload")
	}
}

func TestImageProviderRetryAfterAndResponseLimits(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"37"}}}
	providerErr := providerHTTPErrorWithResponse(response, []byte(`{"error":{"message":"busy"}}`), "request-429", providerFailureStageImageGenerationRequest)
	if providerErr.RetryAfter != 37*time.Second {
		t.Fatalf("retry after=%s", providerErr.RetryAfter)
	}
	tooLarge := &http.Response{StatusCode: http.StatusOK, ContentLength: maxImageProviderSuccessResponseBytes + 1, Body: io.NopCloser(strings.NewReader("{}"))}
	if _, sizeErr := readLimitedImageProviderResponse(tooLarge); sizeErr == nil || sizeErr.Code != "generation_payload_too_large" {
		t.Fatalf("unexpected response limit error: %+v", sizeErr)
	}
}

func TestGenerationQueue429SchedulesRetryAndReleasesExecutionLease(t *testing.T) {
	provider := &stubProvider{err: &ProviderError{HTTPStatus: http.StatusTooManyRequests, Code: "provider_http_429", Message: "busy", RetryAfter: time.Minute}}
	application, db := newTestApp(t, provider)
	user, cookies := createLoggedInUser(t, application, "queue_429_user", "test-password")
	if err := db.Model(&CreditBalance{}).Where("user_id = ?", user.ID).Update("available_credits", 10).Error; err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(t, application, http.MethodPost, "/api/images/generations/async", map[string]any{"prompt": "rate limited", "aspect_ratio": "1:1"}, cookies)
	if response.Code != http.StatusAccepted {
		t.Fatalf("enqueue status=%d body=%s", response.Code, response.Body.String())
	}
	waitFor(t, 3*time.Second, func() bool {
		var retrying int64
		_ = db.Model(&ImageGenerationJob{}).Where("status = ?", ImageGenerationJobStatusRetryWait).Count(&retrying).Error
		return retrying == 1
	})
	if provider.calls != 1 {
		t.Fatalf("429 should not be retried inside one execution lease, calls=%d", provider.calls)
	}
	var activeLeases int64
	if err := db.Model(&ImageExecutionLease{}).Count(&activeLeases).Error; err != nil || activeLeases != 0 {
		t.Fatalf("429 retry wait retained execution lease: count=%d err=%v", activeLeases, err)
	}
}
