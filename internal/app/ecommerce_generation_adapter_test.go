package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"image-agent/internal/app/ecommerce"
)

func TestCommerceImageTransformProductDetailFourByFive(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 1024, 1536))); err != nil {
		t.Fatal(err)
	}
	doc := ecommerce.DefaultProductDetailLayout("hero", "clean", "4:5", nil)
	docJSON, err := ecommerce.EncodeJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(source.Bytes())
	transformed, mimeType, metadataJSON, err := transformCommerceProductDetailResult(encoded, "image/png", docJSON)
	if err != nil {
		t.Fatal(err)
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(transformed)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(decodedBytes))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 1024 || decoded.Bounds().Dy() != 1280 || mimeType != "image/png" {
		t.Fatalf("bounds/mime = %v %q", decoded.Bounds(), mimeType)
	}
	if !strings.Contains(metadataJSON, `"source_size":"1024x1536"`) || !strings.Contains(metadataJSON, `"output_size":"1024x1280"`) || strings.Contains(metadataJSON, encoded) {
		t.Fatalf("metadata = %s", metadataJSON)
	}
	if _, _, _, err := transformCommerceProductDetailResult(encoded, "image/png", `{"version":1,"unknown":true}`); !errors.Is(err, ecommerce.ErrLayoutInvalid) {
		t.Fatalf("invalid layout error=%v", err)
	}
	backend := &commerceGenerationBackend{app: &App{}}
	_, failure := backend.Execute(context.Background(), ecommerce.ItemExecutionRequest{Item: ecommerce.CommerceGenerationItem{ID: 1}, Compiled: ecommerce.CompiledGenerationItem{RecipeKey: ecommerce.ProductDetailSetRecipeKey, AspectRatio: "4:5", LayoutDocumentJSON: docJSON, LayoutDocumentSHA256: "tampered"}})
	if failure == nil || failure.Code != "layout_invalid" {
		t.Fatalf("first transform SHA failure=%#v", failure)
	}
}

func TestCommerceImageTransformAcceptsJPEGProviderBytes(t *testing.T) {
	var source bytes.Buffer
	if err := jpeg.Encode(&source, image.NewRGBA(image.Rect(0, 0, 1024, 1024)), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	doc := ecommerce.DefaultProductDetailLayout("hero", "clean", "1:1", []string{"便携水杯"})
	docJSON, err := ecommerce.EncodeJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	transformed, mimeType, metadataJSON, err := transformCommerceProductDetailResult(base64.StdEncoding.EncodeToString(source.Bytes()), "image/png", docJSON)
	if err != nil {
		t.Fatal(err)
	}
	if mimeType != "image/png" || !strings.Contains(metadataJSON, `"source_size":"1024x1024"`) {
		t.Fatalf("mime=%q metadata=%s", mimeType, metadataJSON)
	}
	decoded, _, err := image.Decode(bytes.NewReader(mustDecodeBase64(t, transformed)))
	if err != nil || decoded.Bounds().Dx() != 1024 || decoded.Bounds().Dy() != 1024 {
		t.Fatalf("decoded=%v err=%v", decoded, err)
	}
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestCommerceGenerationAdapterUsesExternalReservationAndReusesSucceededWork(t *testing.T) {
	provider := &stubProvider{result: ImageGenerationResult{Base64Image: base64.StdEncoding.EncodeToString([]byte("image")), MIMEType: "image/png", ProviderRequestID: "req-1"}}
	a, db := newTestApp(t, provider)
	user := User{Username: "commerce-adapter", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CreditBalance{UserID: user.ID, AvailableCredits: 9}).Error; err != nil {
		t.Fatal(err)
	}
	item := ecommerce.CommerceGenerationItem{UserID: user.ID, ProjectID: 1, BatchID: 1, Status: ecommerce.CommerceItemRunning, IdempotencyKey: "item-key", ReservedCredits: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := ecommerce.CommerceJob{ID: 7, UserID: user.ID, GenerationItemID: &item.ID, LeaseOwner: "worker", LeaseToken: "lease"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	compiled := ecommerce.CompiledGenerationItem{Prompt: "stable prompt", AspectRatio: "1:1", WorkCategory: WorkCategoryProductMain, EstimatedCredits: 1}
	backend := &commerceGenerationBackend{app: a}
	req := ecommerce.ItemExecutionRequest{Lease: ecommerce.LeaseIdentity{JobID: job.ID, LeaseOwner: job.LeaseOwner, LeaseToken: job.LeaseToken}, Job: job, Item: item, Compiled: compiled, IdempotencyKey: item.IdempotencyKey}

	first, failure := backend.Execute(context.Background(), req)
	if failure != nil {
		t.Fatalf("Execute failure: %+v", failure)
	}
	second, failure := backend.Execute(context.Background(), req)
	if failure != nil {
		t.Fatalf("replay failure: %+v", failure)
	}
	if first.GenerationRecordID == 0 || first.WorkID == 0 || second != first {
		t.Fatalf("results first=%+v second=%+v", first, second)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d want 1", provider.calls)
	}
	doc := ecommerce.DefaultProductDetailLayout("hero", "clean", "4:5", nil)
	docJSON, _ := ecommerce.EncodeJSON(doc)
	req.Compiled = ecommerce.CompiledGenerationItem{RecipeKey: ecommerce.ProductDetailSetRecipeKey, AspectRatio: "4:5", LayoutDocumentJSON: docJSON}
	replayed, failure := backend.Execute(context.Background(), req)
	if failure != nil || !strings.Contains(replayed.MetadataJSON, `"output_size":"1024x1280"`) || provider.calls != 1 {
		t.Fatalf("durable replay=%#v failure=%#v calls=%d", replayed, failure, provider.calls)
	}
	var balance CreditBalance
	if err := db.Where("user_id = ?", user.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.AvailableCredits != 9 {
		t.Fatalf("balance=%d want 9", balance.AvailableCredits)
	}
	var charges int64
	if err := db.Model(&CreditTransaction{}).Where("user_id = ? AND type = ?", user.ID, CreditTransactionTypeGenerationCharge).Count(&charges).Error; err != nil {
		t.Fatal(err)
	}
	if charges != 0 {
		t.Fatalf("generation charges=%d want 0", charges)
	}
	var record GenerationRecord
	if err := db.First(&record, first.GenerationRecordID).Error; err != nil {
		t.Fatal(err)
	}
	var work Work
	if err := db.First(&work, first.WorkID).Error; err != nil {
		t.Fatal(err)
	}
	if record.StorageScope != StorageScopeCommercePrivate || work.StorageScope != StorageScopeCommercePrivate || record.ExecutionKey == nil || *record.ExecutionKey != fmt.Sprintf("commerce:item:%d", item.ID) {
		t.Fatalf("record=%+v work scope=%q", record, work.StorageScope)
	}
	if work.Category != WorkCategoryProductMain || work.Visibility != WorkVisibilityPrivate {
		t.Fatalf("work category/visibility = %q/%q", work.Category, work.Visibility)
	}
	var mark AIContentMark
	if err := db.Where("generation_record_id = ?", record.ID).First(&mark).Error; err != nil {
		t.Fatal(err)
	}
	if mark.VisibleLabel != "AI生成" || mark.TraceID == "" {
		t.Fatalf("mark=%+v", mark)
	}
}

func TestCommerceGenerationAdapterHonorsCanceledParent(t *testing.T) {
	a, db := newTestApp(t, &stubProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &commerceGenerationBackend{app: a}
	_, failure := backend.Execute(ctx, ecommerce.ItemExecutionRequest{Item: ecommerce.CommerceGenerationItem{ID: 9, UserID: 1}, Compiled: ecommerce.CompiledGenerationItem{Prompt: "x", AspectRatio: "1:1"}})
	if failure == nil || failure.Code != "user_cancelled" {
		t.Fatalf("failure=%+v", failure)
	}
	var works int64
	_ = db.Model(&Work{}).Count(&works).Error
	if works != 0 {
		t.Fatalf("works=%d want 0", works)
	}
}

func TestCommerceGenerationAdapterIdempotencyHeaderRequiresCapability(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer server.Close()
	p := NewOpenAIProvider(Config{OpenAIBaseURL: server.URL, OpenAIAPIKey: "test"})
	input := ImageGenerationInput{Prompt: "x", Size: "1024x1024", IdempotencyKey: "secret-item-key"}
	if _, err := p.Generate(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("header without capability=%q", got)
	}
	input.SupportsIdempotencyKey = true
	if _, err := p.Generate(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if got != input.IdempotencyKey {
		t.Fatalf("header=%q want key", got)
	}
}

func TestCommerceGenerationAdapterMixedCandidatesUsesPersistedActualChannel(t *testing.T) {
	record := GenerationRecord{Stage: GenerationStageRequestingProvider, ProviderRequestStarted: true, ProviderIdempotencySupported: false, ChannelID: 11}
	job := &generationJob{ModelCenterCandidates: []modelCenterCandidate{
		{Model: ModelCatalog{CapabilityTags: []string{"image"}}, Channel: ModelChannel{ID: 11}},
		{Model: ModelCatalog{CapabilityTags: []string{"image", "idempotency_key"}}, Channel: ModelChannel{ID: 12}},
	}}
	if commerceGenerationReplaySafe(record, job) {
		t.Fatal("mixed candidates must not use a later channel capability to replay the actual non-idempotent channel")
	}
}

func TestCommerceGenerationAdapterReplayPinsPersistedChannelAndDerivedKey(t *testing.T) {
	provider := &stubProvider{result: ImageGenerationResult{Base64Image: base64.StdEncoding.EncodeToString([]byte("pinned")), MIMEType: "image/png"}}
	a, db := newTestApp(t, provider)
	record := GenerationRecord{UserID: 1, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider, ProviderRequestStarted: true, ProviderIdempotencySupported: true, ChannelID: 302}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	job := &generationJob{Settings: AppSettings{RequestTimeoutSeconds: 30}, ModelCenterCandidates: []modelCenterCandidate{
		{Model: ModelCatalog{ID: 1, CapabilityTags: []string{"image", "idempotency_key"}}, Channel: ModelChannel{ID: 301, RuntimeModel: "reordered-first"}},
		{Model: ModelCatalog{ID: 2, CapabilityTags: []string{"image", "idempotency_key"}}, Channel: ModelChannel{ID: 302, RuntimeModel: "actual-channel"}},
	}}
	if !commerceGenerationReplaySafe(record, job) {
		t.Fatal("persisted channel should be replayable")
	}
	if len(job.ModelCenterCandidates) != 1 || job.ModelCenterCandidates[0].Channel.ID != 302 {
		t.Fatalf("candidates=%+v", job.ModelCenterCandidates)
	}
	_, providerErr, _, err := a.generateImageWithModelCenterFailover(context.Background(), time.Minute, &record, job, ImageGenerationInput{ExternalReservation: true, IdempotencyKey: "commerce:item:55"})
	if err != nil || providerErr != nil {
		t.Fatalf("err=%v providerErr=%+v", err, providerErr)
	}
	if provider.calls != 1 || len(provider.inputs) != 1 || provider.inputs[0].Model != "actual-channel" || provider.inputs[0].IdempotencyKey != "commerce:item:55:channel-302" {
		t.Fatalf("inputs=%+v calls=%d", provider.inputs, provider.calls)
	}
	var event GenerationEventLog
	if err := db.Where("generation_record_id = ? AND event = ?", record.ID, "provider_idempotency_key_applied").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	wantSum := sha256.Sum256([]byte("commerce:item:55:channel-302"))
	if !strings.Contains(event.MetadataJSON, hex.EncodeToString(wantSum[:])[:12]) || strings.Contains(event.MetadataJSON, "commerce:item:55") {
		t.Fatalf("metadata=%s", event.MetadataJSON)
	}
}

func TestCommerceGenerationAdapterReplayFailsClosedWhenPersistedChannelMissing(t *testing.T) {
	record := GenerationRecord{ProviderRequestStarted: true, ProviderIdempotencySupported: true, ChannelID: 999}
	job := &generationJob{ModelCenterCandidates: []modelCenterCandidate{{Channel: ModelChannel{ID: 1}}}}
	if commerceGenerationReplaySafe(record, job) {
		t.Fatal("missing persisted channel must fail closed")
	}
}

func TestCommerceExternalReservationNonIdempotentTimeoutDoesNotRetryOrFailover(t *testing.T) {
	provider := &stubProvider{errs: []*ProviderError{{Code: "provider_timeout", Message: "timed out", FailureStage: providerFailureStageImageGenerationRequest}}}
	a, db := newTestApp(t, provider)
	record := GenerationRecord{UserID: 1, Status: GenerationStatusRunning, Stage: GenerationStageRequestingProvider}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	job := &generationJob{Settings: AppSettings{RequestTimeoutSeconds: 30}, ModelCenterCandidates: []modelCenterCandidate{
		{Model: ModelCatalog{ID: 101, CapabilityTags: []string{"image"}}, Channel: ModelChannel{ID: 201, RuntimeModel: "first"}},
		{Model: ModelCatalog{ID: 102, CapabilityTags: []string{"image", "idempotency_key"}}, Channel: ModelChannel{ID: 202, RuntimeModel: "second"}},
	}}
	_, providerErr, _, err := a.generateImageWithModelCenterFailover(context.Background(), time.Minute, &record, job, ImageGenerationInput{Prompt: "x", ExternalReservation: true, IdempotencyKey: "commerce:item:1"})
	if err != nil {
		t.Fatal(err)
	}
	if providerErr == nil || providerErr.Code != "provider_result_unknown" {
		t.Fatalf("providerErr=%+v", providerErr)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d want 1", provider.calls)
	}
	var persisted GenerationRecord
	if err := db.First(&persisted, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.ChannelID != 201 || !persisted.ProviderRequestStarted || persisted.ProviderIdempotencySupported {
		t.Fatalf("persisted=%+v", persisted)
	}
	var idempotencyEvents int64
	if err := db.Model(&GenerationEventLog{}).Where("generation_record_id = ? AND event = ?", record.ID, "provider_idempotency_key_applied").Count(&idempotencyEvents).Error; err != nil {
		t.Fatal(err)
	}
	if idempotencyEvents != 0 {
		t.Fatalf("idempotency events=%d want 0", idempotencyEvents)
	}
}

func TestCommerceExternalReservationUnknownFailureStopsRetryAndFailover(t *testing.T) {
	input := ImageGenerationInput{ExternalReservation: true, SupportsIdempotencyKey: false}
	for _, code := range []string{"provider_timeout", "provider_request_failed"} {
		if commerceProviderFailureAllowsRetry(input, &ProviderError{Code: code, FailureStage: providerFailureStageImageGenerationRequest}) {
			t.Fatalf("%s unexpectedly allowed retry/failover", code)
		}
	}
	input.ExternalReservation = false
	if !commerceProviderFailureAllowsRetry(input, &ProviderError{Code: "provider_timeout", FailureStage: providerFailureStageImageGenerationRequest}) {
		t.Fatal("direct billing retry behavior changed")
	}
}

func TestCommerceGenerationAdapterCancelAfterObjectGuardCleansWithoutWork(t *testing.T) {
	provider := &stubProvider{result: ImageGenerationResult{Base64Image: base64.StdEncoding.EncodeToString([]byte("guard-cancel")), MIMEType: "image/png"}}
	a, db := newTestApp(t, provider)
	user := User{Username: "guard-cancel-user", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	style, weight := 65, 75
	job := &generationJob{User: user, Settings: AppSettings{RequestTimeoutSeconds: 30}, Request: generationRequest{Prompt: "cancel after guard", AspectRatio: "1:1", Size: "1024x1024", ToolMode: GenerationToolModeGenerate, StyleStrength: &style, ReferenceWeight: &weight, Num: 1}}
	key := "commerce:item:guard-cancel"
	record := GenerationRecord{UserID: user.ID, Prompt: job.Request.Prompt, AspectRatio: "1:1", Status: GenerationStatusQueued, Stage: GenerationStageQueued, CreditsCost: 1, ExecutionKey: &key}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, _, err := a.executeGenerationRecordWithOptions(&record, job, generationExecutionOptions{Context: ctx, BillingMode: generationBillingExternalReservation, ResultStorageScope: StorageScopeCommercePrivate, IdempotencyKey: key, CommerceProjectID: 77, AfterObjectGuard: cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want canceled", err)
	}
	var works, marks int64
	_ = db.Model(&Work{}).Where("generation_record_id = ?", record.ID).Count(&works).Error
	_ = db.Model(&AIContentMark{}).Where("generation_record_id = ?", record.ID).Count(&marks).Error
	if works != 0 || marks != 0 {
		t.Fatalf("works=%d marks=%d", works, marks)
	}
	var persisted GenerationRecord
	if err := db.First(&persisted, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status == GenerationStatusSucceeded || persisted.WorkID != nil || persisted.AssetKey != "" {
		t.Fatalf("persisted=%+v", persisted)
	}
	var cleanup ecommerce.CommerceObjectCleanup
	if err := db.Where("user_id = ? AND reason = ?", user.ID, "generation_canceled").First(&cleanup).Error; err != nil {
		t.Fatalf("cleanup missing: %v", err)
	}
}

func TestCommerceGenerationAdapterPersistenceFailureCleansObject(t *testing.T) {
	provider := &stubProvider{result: ImageGenerationResult{Base64Image: base64.StdEncoding.EncodeToString([]byte("persist-fail")), MIMEType: "image/png"}}
	a, db := newTestApp(t, provider)
	user := User{Username: "persist-fail-user", Status: UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	style, weight := 65, 75
	job := &generationJob{User: user, Settings: AppSettings{RequestTimeoutSeconds: 30}, Request: generationRequest{Prompt: "persist fail", AspectRatio: "1:1", Size: "1024x1024", ToolMode: GenerationToolModeGenerate, StyleStrength: &style, ReferenceWeight: &weight, Num: 1}}
	key := "commerce:item:persist-fail"
	record := GenerationRecord{UserID: user.ID, Status: GenerationStatusQueued, Stage: GenerationStageQueued, ExecutionKey: &key, CreditsCost: 1}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	callback := "test:fail-ai-content-mark"
	if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ai_content_marks" {
			tx.AddError(errors.New("injected mark failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })
	_, _, err := a.executeGenerationRecordWithOptions(&record, job, generationExecutionOptions{Context: context.Background(), BillingMode: generationBillingExternalReservation, ResultStorageScope: StorageScopeCommercePrivate, IdempotencyKey: key, CommerceProjectID: 88})
	if err == nil || !strings.Contains(err.Error(), "injected mark failure") {
		t.Fatalf("err=%v", err)
	}
	var works, marks int64
	_ = db.Model(&Work{}).Where("generation_record_id = ?", record.ID).Count(&works).Error
	_ = db.Model(&AIContentMark{}).Where("generation_record_id = ?", record.ID).Count(&marks).Error
	if works != 0 || marks != 0 {
		t.Fatalf("works=%d marks=%d", works, marks)
	}
	var persisted GenerationRecord
	if err := db.First(&persisted, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status == GenerationStatusSucceeded || persisted.WorkID != nil || persisted.AssetKey != "" {
		t.Fatalf("persisted=%+v", persisted)
	}
	var cleanup ecommerce.CommerceObjectCleanup
	if err := db.Where("user_id = ? AND reason = ?", user.ID, "generation_canceled").First(&cleanup).Error; err != nil {
		t.Fatalf("cleanup missing: %v", err)
	}
}
