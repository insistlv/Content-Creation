package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestBatchCanonicalRequestDigestIgnoresMapInsertionOrder(t *testing.T) {
	first := EstimateBatchRequest{
		RecipeKey: "poster", RecipeVersion: 1, OutputCount: 1, CreativeSpecID: 9, PrimarySKUID: 10,
		QualityTier: "standard", AspectRatio: "1:1",
		AssetBindings: map[string][]uint{"product": {4}, "background": {5}},
		Parameters:    map[string]any{"seed": float64(7), "prompt": "hello"},
	}
	second := first
	second.AssetBindings = map[string][]uint{"background": {5}, "product": {4}}
	second.Parameters = map[string]any{"prompt": "hello", "seed": float64(7)}

	firstDigest, firstJSON, err := canonicalBatchRequestDigest(first)
	if err != nil {
		t.Fatalf("first digest: %v", err)
	}
	secondDigest, secondJSON, err := canonicalBatchRequestDigest(second)
	if err != nil {
		t.Fatalf("second digest: %v", err)
	}
	if firstDigest != secondDigest || firstJSON != secondJSON {
		t.Fatalf("canonical request differs by insertion order: %s/%s %q/%q", firstDigest, secondDigest, firstJSON, secondJSON)
	}
}

func TestProductDetailGetLoadsItemOutputSnapshot(t *testing.T) {
	_, db := newCommerceServiceTest(t)
	batch := CommerceGenerationBatch{UserID: 91, ProjectID: 92, Status: CommerceBatchSucceeded}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}
	item := CommerceGenerationItem{UserID: 91, ProjectID: 92, BatchID: batch.ID, Status: CommerceItemSucceeded}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := CommerceJob{UserID: 91, ProjectID: 92, BatchID: &batch.ID, GenerationItemID: &item.ID, Kind: CommerceJobKindGenerateItem, Status: CommerceJobSucceeded, ResultJSON: `{"layout_version":1,"output_size":"1024x1280"}`}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	items, err := loadBatchItems(context.Background(), db, 91, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OutputSnapshotJSON != job.ResultJSON {
		t.Fatalf("items = %#v", items)
	}
}

func TestIdempotencySameKeyDifferentDigestConflicts(t *testing.T) {
	if !isIdempotencyReplay("key-1", "digest-1", "key-1", "digest-1") {
		t.Fatal("same key and digest must replay")
	}
	if err := validateIdempotencyReplay("key-1", "digest-1", "key-1", "digest-2"); err != ErrIdempotencyConflict {
		t.Fatalf("different digest error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestRetryItemRejectsDeletingOrSoftDeletedProject(t *testing.T) {
	for _, state := range []string{"deletion_requested", "soft_deleted"} {
		t.Run(state, func(t *testing.T) {
			service, db, _, _, project, request := newBatchTestService(t)
			estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
			if err != nil {
				t.Fatalf("EstimateBatch: %v", err)
			}
			submitted, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "retry-project-state-source", SubmitBatchRequest{
				EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
			})
			if err != nil {
				t.Fatalf("SubmitBatch: %v", err)
			}
			item := submitted.Items[0]
			if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Update("status", CommerceItemFailed).Error; err != nil {
				t.Fatalf("mark item failed: %v", err)
			}
			now := time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)
			switch state {
			case "deletion_requested":
				if err := db.Model(&CommerceProject{}).Where("id = ?", project.ID).Updates(map[string]any{"status": "deletion_requested", "deletion_requested_at": now}).Error; err != nil {
					t.Fatalf("mark project deleting: %v", err)
				}
			case "soft_deleted":
				if err := db.Delete(&CommerceProject{}, project.ID).Error; err != nil {
					t.Fatalf("soft delete project: %v", err)
				}
			}
			var before int64
			if err := db.Model(&CommerceGenerationBatch{}).Count(&before).Error; err != nil {
				t.Fatalf("count batches before retry: %v", err)
			}
			_, err = service.RetryItem(context.Background(), project.UserID, item.ID, "retry-project-state-new")
			if !errors.Is(err, ErrProjectDeletionRequested) {
				t.Fatalf("RetryItem error = %v, want ErrProjectDeletionRequested", err)
			}
			var after int64
			if err := db.Model(&CommerceGenerationBatch{}).Count(&after).Error; err != nil {
				t.Fatalf("count batches after retry: %v", err)
			}
			if after != before {
				t.Fatalf("retry created batch after project deletion: before=%d after=%d", before, after)
			}
		})
	}
}

func TestSubmitBatchRejectsSoftDeletedProjectWithoutSideEffects(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	if err := db.Delete(&CommerceProject{}, project.ID).Error; err != nil {
		t.Fatalf("soft delete project: %v", err)
	}
	_, err = service.SubmitBatch(context.Background(), project.UserID, project.ID, "submit-soft-deleted", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	})
	if !errors.Is(err, ErrProjectDeletionRequested) {
		t.Fatalf("SubmitBatch error = %v, want ErrProjectDeletionRequested", err)
	}
	var batches int64
	if err := db.Model(&CommerceGenerationBatch{}).Count(&batches).Error; err != nil {
		t.Fatalf("count batches: %v", err)
	}
	if batches != 0 {
		t.Fatalf("soft-deleted project created %d batches", batches)
	}
}

func TestBatchEstimatePersistsAndSubmitUsesFrozenSnapshot(t *testing.T) {
	service, db, registry, ledger, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), 11, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	if estimate.PricingSnapshotID == "" || estimate.PricingExpiresAt.IsZero() || estimate.EstimatedCredits != 2 {
		t.Fatalf("estimate = %#v", estimate)
	}

	restarted := NewService(NewRepository(db))
	restarted.ConfigureBatchInfrastructure(registry, ledger, NewGormPricingSnapshotStore(), nil)
	restarted.now = service.now
	result, err := restarted.SubmitBatch(context.Background(), 11, project.ID, "batch-key-1", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	})
	if err != nil {
		t.Fatalf("SubmitBatch after restart: %v", err)
	}
	if result.Batch.ReservationID == nil || len(result.Items) != 1 || result.Items[0].ReservationID != *result.Batch.ReservationID {
		t.Fatalf("batch reservation propagation = %#v items=%#v", result.Batch, result.Items)
	}
	var frozenJob CommerceJob
	if err := db.Where("generation_item_id = ?", result.Items[0].ID).First(&frozenJob).Error; err != nil {
		t.Fatalf("load frozen job: %v", err)
	}
	if frozenJob.MaxAttempts != 7 {
		t.Fatalf("job max attempts = %d, want recipe-frozen 7", frozenJob.MaxAttempts)
	}
	compiled, err := DecodeGenerationItemSnapshot(result.Items[0].OutputSpecJSON)
	if err != nil || compiled.EstimatedCredits != 2 || compiled.PricingSnapshotID != estimate.PricingSnapshotID {
		t.Fatalf("stored output spec = %#v err=%v", compiled, err)
	}

	replay, err := restarted.SubmitBatch(context.Background(), 11, project.ID, "batch-key-1", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	})
	if err != nil || replay.Batch.ID != result.Batch.ID {
		t.Fatalf("idempotent replay = %#v err=%v", replay, err)
	}
	changed := request
	changed.AspectRatio = "3:4"
	if _, err := restarted.SubmitBatch(context.Background(), 11, project.ID, "batch-key-1", SubmitBatchRequest{
		EstimateBatchRequest: changed, PricingSnapshotID: estimate.PricingSnapshotID,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key different digest error = %v", err)
	}
}

func TestBatchStaleSnapshotCreatesNoBatchOrReservation(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), 11, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	service.now = func() time.Time { return estimate.PricingExpiresAt }
	if _, err := service.SubmitBatch(context.Background(), 11, project.ID, "expired-key", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	}); !errors.Is(err, ErrPricingSnapshotStale) {
		t.Fatalf("expired snapshot error = %v", err)
	}
	var batches, reservations int64
	if err := db.Model(&CommerceGenerationBatch{}).Count(&batches).Error; err != nil {
		t.Fatalf("count batches: %v", err)
	}
	if err := db.Model(&CommerceCreditReservation{}).Count(&reservations).Error; err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if batches != 0 || reservations != 0 {
		t.Fatalf("stale submit side effects: batches=%d reservations=%d", batches, reservations)
	}
}

func TestIdempotencyReplayRejectsCrossProjectPath(t *testing.T) {
	service, _, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), 11, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	if _, err := service.SubmitBatch(context.Background(), 11, project.ID, "cross-project-key", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	}); err != nil {
		t.Fatalf("initial SubmitBatch: %v", err)
	}
	if _, err := service.SubmitBatch(context.Background(), 11, project.ID+1000, "cross-project-key", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("cross-project replay error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestIdempotencyConcurrentSameKeyReplaysOriginalBatch(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), 11, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	installBatchReplayReadBarrier(t, db)
	start := make(chan struct{})
	results := make(chan BatchSnapshot, 2)
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index := 0; index < 2; index++ {
		go func() {
			ready.Done()
			<-start
			result, submitErr := service.SubmitBatch(context.Background(), 11, project.ID, "concurrent-batch-key", SubmitBatchRequest{
				EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
			})
			results <- result
			errs <- submitErr
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results
	for index := 0; index < 2; index++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent SubmitBatch %d: %v", index, err)
		}
	}
	if first.Batch.ID == 0 || first.Batch.ID != second.Batch.ID {
		t.Fatalf("concurrent batch IDs = %d and %d", first.Batch.ID, second.Batch.ID)
	}
	var batches, reservations int64
	if err := db.Model(&CommerceGenerationBatch{}).Where("user_id = ? AND idempotency_key = ?", 11, "concurrent-batch-key").Count(&batches).Error; err != nil {
		t.Fatalf("count concurrent batches: %v", err)
	}
	if err := db.Model(&CommerceCreditReservation{}).Count(&reservations).Error; err != nil {
		t.Fatalf("count concurrent reservations: %v", err)
	}
	if batches != 1 || reservations != 0 {
		t.Fatalf("concurrent side effects batches=%d reservations=%d", batches, reservations)
	}
}

func installBatchReplayReadBarrier(t *testing.T, db *gorm.DB) {
	t.Helper()
	var arrivals atomic.Int32
	release := make(chan struct{})
	const name = "test:batch-replay-read-barrier"
	if err := db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "commerce_generation_batches" || tx.RowsAffected != 0 {
			return
		}
		if arrivals.Add(1) == 2 {
			close(release)
			return
		}
		<-release
	}); err != nil {
		t.Fatalf("register batch replay read barrier: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

func TestBatchCancelRacePreventsLateCompleteSettlement(t *testing.T) {
	service, db, lease, itemID := newCancelRaceService(t, "complete")
	installCancelBetweenLeaseReadAndTransition(t, db, itemID, lease.JobID, "complete")
	if err := service.CompleteGenerationItem(context.Background(), lease, itemID, ExecutionResult{ActualCredits: 2, GenerationRecordID: 99}); err != nil {
		t.Fatalf("CompleteGenerationItem: %v", err)
	}
	assertCancelWonRace(t, db, itemID)
}

func TestBatchCancelRacePreventsLateFailureSettlement(t *testing.T) {
	service, db, lease, itemID := newCancelRaceService(t, "fail")
	installCancelBetweenLeaseReadAndTransition(t, db, itemID, lease.JobID, "fail")
	if err := service.FailGenerationItem(context.Background(), lease, itemID, ExecutionFailure{Code: "provider_failed", Message: "failed"}); err != nil {
		t.Fatalf("FailGenerationItem: %v", err)
	}
	assertCancelWonRace(t, db, itemID)
}

func TestCancelBatchRollsBackAllItemsWhenSecondCancellationFails(t *testing.T) {
	service, db, registry, _, project, request := newBatchTestService(t)
	compiler := fakeRecipe{
		definition: RecipeDefinition{
			Key: "cancel-two", Pipeline: "general", Version: 1,
			AllowedOutputCounts: []int{2}, AspectRatios: []string{"1:1"}, QualityTiers: []string{"standard"}, MaxAttempts: 3,
		},
		resolver: &fakeCostResolver{price: ItemPrice{Credits: 2, Version: "price-v1"}},
		items: []CompiledGenerationItem{
			{SKUID: request.PrimarySKUID, Pipeline: "general", RecipeKey: "cancel-two", RecipeVersion: 1, SlotKey: "hero", AspectRatio: "1:1"},
			{SKUID: request.PrimarySKUID, Pipeline: "general", RecipeKey: "cancel-two", RecipeVersion: 1, SlotKey: "detail", AspectRatio: "1:1"},
		},
	}
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("register cancel recipe: %v", err)
	}
	service.ConfigureBatchInfrastructure(registry, &cancelRaceCreditLedger{}, NewGormPricingSnapshotStore(), nil)
	request.RecipeKey, request.OutputCount = "cancel-two", 2
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	batch, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "cancel-two-batch", SubmitBatchRequest{
		EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID,
	})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}

	triggerSQL := fmt.Sprintf(`CREATE TRIGGER fail_second_batch_item_cancel
		BEFORE UPDATE OF status ON commerce_generation_items
		WHEN OLD.id = %d AND NEW.status = 'canceled'
		BEGIN SELECT RAISE(ABORT, 'injected second item cancellation failure'); END`, batch.Items[1].ID)
	if err := db.Exec(triggerSQL).Error; err != nil {
		t.Fatalf("register cancellation fault: %v", err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER IF EXISTS fail_second_batch_item_cancel").Error })

	if _, err := service.CancelBatch(context.Background(), project.UserID, batch.Batch.ID); err == nil {
		t.Fatal("CancelBatch succeeded despite injected second item failure")
	}
	var items []CommerceGenerationItem
	if err := db.Where("batch_id = ?", batch.Batch.ID).Order("id ASC").Find(&items).Error; err != nil {
		t.Fatalf("load items after rollback: %v", err)
	}
	for _, item := range items {
		if item.Status != CommerceItemQueued || item.CancelRequestedAt != nil || item.ReleasedCredits != 0 {
			t.Fatalf("item %d partially canceled: %#v", item.ID, item)
		}
	}
	var storedBatch CommerceGenerationBatch
	if err := db.First(&storedBatch, batch.Batch.ID).Error; err != nil {
		t.Fatalf("load batch after rollback: %v", err)
	}
	if storedBatch.CancelRequestedAt != nil || storedBatch.CanceledItems != 0 {
		t.Fatalf("batch partially canceled: %#v", storedBatch)
	}
}

type cancelRaceCreditLedger struct{}

func (*cancelRaceCreditLedger) ReserveTx(context.Context, *gorm.DB, ReserveCreditsRequest) (CreditReservationSnapshot, error) {
	return CreditReservationSnapshot{}, nil
}

func (*cancelRaceCreditLedger) SettleItemTx(_ context.Context, tx *gorm.DB, req SettleCreditsRequest) error {
	return tx.Model(&CommerceGenerationItem{}).Where("id = ?", req.GenerationItemID).
		Updates(map[string]any{"settled_credits": req.HeldCredits, "released_credits": 0}).Error
}

func (*cancelRaceCreditLedger) ReleaseItemTx(_ context.Context, tx *gorm.DB, req ReleaseCreditsRequest) error {
	return tx.Model(&CommerceGenerationItem{}).Where("id = ?", req.GenerationItemID).
		Updates(map[string]any{"settled_credits": 0, "released_credits": req.HeldCredits}).Error
}

func newCancelRaceService(t *testing.T, suffix string) (*Service, *gorm.DB, LeaseIdentity, uint) {
	t.Helper()
	db := openCreditTestDB(t)
	batch := CommerceGenerationBatch{
		UserID: 61, ProjectID: 71, Status: CommerceBatchRunning,
		IdempotencyKey: "cancel-race-" + suffix + "-batch", RequestDigest: "digest",
		TotalItems: 1, RunningItems: 1,
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create cancel race batch: %v", err)
	}
	item := CommerceGenerationItem{
		UserID: batch.UserID, ProjectID: batch.ProjectID, BatchID: batch.ID, ReservationID: 81,
		SKUID: 1, Pipeline: "general", RecipeKey: "poster", Status: CommerceItemRunning,
		IdempotencyKey: "cancel-race-" + suffix + "-item", EstimatedCredits: 2, ReservedCredits: 2,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create cancel race item: %v", err)
	}
	batchID, itemID := batch.ID, item.ID
	job := CommerceJob{
		UserID: batch.UserID, ProjectID: batch.ProjectID, BatchID: &batchID, GenerationItemID: &itemID,
		Kind: CommerceJobKindGenerateItem, Pipeline: "general", RecipeKey: "poster", Status: CommerceJobRunning,
		IdempotencyKey: "cancel-race-" + suffix + "-job", LeaseOwner: "worker", LeaseToken: "token",
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create cancel race job: %v", err)
	}
	service := NewService(NewRepository(db))
	service.ConfigureBatchInfrastructure(NewRegistry(), &cancelRaceCreditLedger{}, NewGormPricingSnapshotStore(), nil)
	return service, db, LeaseIdentity{JobID: job.ID, LeaseOwner: job.LeaseOwner, LeaseToken: job.LeaseToken}, item.ID
}

func installCancelBetweenLeaseReadAndTransition(t *testing.T, db *gorm.DB, itemID, jobID uint, suffix string) {
	t.Helper()
	var injected atomic.Bool
	name := "test:inject-cancel-race:" + suffix
	if err := db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "commerce_jobs" || tx.RowsAffected != 1 || !injected.CompareAndSwap(false, true) {
			return
		}
		now := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
		clean := tx.Session(&gorm.Session{NewDB: true})
		if err := clean.Model(&CommerceGenerationItem{}).Where("id = ? AND status = ?", itemID, CommerceItemRunning).
			Update("cancel_requested_at", now).Error; err != nil {
			tx.AddError(err)
			return
		}
		if err := clean.Model(&CommerceJob{}).Where("id = ? AND status = ?", jobID, CommerceJobRunning).
			Update("cancel_requested_at", now).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatalf("register cancel race callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

func assertCancelWonRace(t *testing.T, db *gorm.DB, itemID uint) {
	t.Helper()
	var item CommerceGenerationItem
	if err := db.First(&item, itemID).Error; err != nil {
		t.Fatalf("load cancel race item: %v", err)
	}
	if item.Status != CommerceItemCanceled || item.SettledCredits != 0 || item.ReleasedCredits != item.ReservedCredits || item.GenerationRecordID != nil {
		t.Fatalf("cancel race item = %#v", item)
	}
}

func TestBatchETAFollowsPersistedItemProgress(t *testing.T) {
	service, db, registry, _, project, request := newBatchTestService(t)
	compiler := fakeRecipe{
		definition: RecipeDefinition{Key: "poster-two", Pipeline: "general", Version: 1, AllowedOutputCounts: []int{2}, AspectRatios: []string{"1:1"}, QualityTiers: []string{"standard"}, MaxAttempts: 3},
		resolver:   &fakeCostResolver{price: ItemPrice{Credits: 2, Version: "price-v1"}},
		items: []CompiledGenerationItem{
			{SKUID: 31, Pipeline: "general", RecipeKey: "poster-two", RecipeVersion: 1, SlotKey: "hero", AspectRatio: "1:1"},
			{SKUID: 31, Pipeline: "general", RecipeKey: "poster-two", RecipeVersion: 1, SlotKey: "detail", AspectRatio: "1:1"},
		},
	}
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("register two-item recipe: %v", err)
	}
	request.RecipeKey, request.OutputCount = "poster-two", 2
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	submitted, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "eta-flow", SubmitBatchRequest{EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	if submitted.Batch.ETASeconds <= 0 {
		t.Fatalf("submitted ETASeconds = %d, want positive", submitted.Batch.ETASeconds)
	}
	initialETA := submitted.Batch.ETASeconds
	created := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	updated := created.Add(20 * time.Second)
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", submitted.Items[0].ID).UpdateColumns(map[string]any{"status": CommerceItemSucceeded, "created_at": created, "updated_at": updated, "finished_at": updated}).Error; err != nil {
		t.Fatalf("complete first item: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return refreshBatchCounters(context.Background(), tx, project.UserID, submitted.Batch.ID, updated)
	}); err != nil {
		t.Fatalf("refresh first completion: %v", err)
	}
	var progressed CommerceGenerationBatch
	if err := db.First(&progressed, submitted.Batch.ID).Error; err != nil {
		t.Fatalf("load progressed batch: %v", err)
	}
	if progressed.ETASeconds <= 0 || progressed.ETASeconds >= initialETA {
		t.Fatalf("progressed ETASeconds = %d, initial %d", progressed.ETASeconds, initialETA)
	}
	updated2 := created.Add(25 * time.Second)
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", submitted.Items[1].ID).UpdateColumns(map[string]any{"status": CommerceItemSucceeded, "created_at": created, "updated_at": updated2, "finished_at": updated2}).Error; err != nil {
		t.Fatalf("complete second item: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return refreshBatchCounters(context.Background(), tx, project.UserID, submitted.Batch.ID, updated2)
	}); err != nil {
		t.Fatalf("refresh terminal: %v", err)
	}
	var terminal CommerceGenerationBatch
	if err := db.First(&terminal, submitted.Batch.ID).Error; err != nil {
		t.Fatalf("load terminal batch: %v", err)
	}
	if terminal.ETASeconds != 0 {
		t.Fatalf("terminal ETASeconds = %d, want 0", terminal.ETASeconds)
	}
}

func TestRetryItemImmediatelyPersistsQueuedETA(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
	if err != nil {
		t.Fatalf("EstimateBatch: %v", err)
	}
	parent, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "retry-eta-parent", SubmitBatchRequest{EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID})
	if err != nil {
		t.Fatalf("SubmitBatch: %v", err)
	}
	now := time.Date(2026, 7, 11, 10, 1, 0, 0, time.UTC)
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", parent.Items[0].ID).Updates(map[string]any{"status": CommerceItemFailed, "finished_at": now}).Error; err != nil {
		t.Fatalf("fail parent item: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return refreshBatchCounters(context.Background(), tx, project.UserID, parent.Batch.ID, now)
	}); err != nil {
		t.Fatalf("refresh parent: %v", err)
	}
	var terminalParent CommerceGenerationBatch
	if err := db.First(&terminalParent, parent.Batch.ID).Error; err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if terminalParent.ETASeconds != 0 {
		t.Fatalf("parent ETASeconds = %d, want 0", terminalParent.ETASeconds)
	}
	retried, err := service.RetryItem(context.Background(), project.UserID, parent.Items[0].ID, "retry-eta-child")
	if err != nil {
		t.Fatalf("RetryItem: %v", err)
	}
	if retried.Batch.Status != CommerceBatchQueued || retried.Batch.ETASeconds <= 0 {
		t.Fatalf("retry response batch = %#v", retried.Batch)
	}
	var stored CommerceGenerationBatch
	if err := db.First(&stored, retried.Batch.ID).Error; err != nil {
		t.Fatalf("load retry batch: %v", err)
	}
	if stored.ETASeconds != retried.Batch.ETASeconds {
		t.Fatalf("stored ETASeconds = %d, response %d", stored.ETASeconds, retried.Batch.ETASeconds)
	}
	listed, err := service.ListBatches(context.Background(), project.UserID, project.ID)
	if err != nil {
		t.Fatalf("ListBatches: %v", err)
	}
	got, err := service.GetBatch(context.Background(), project.UserID, retried.Batch.ID)
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if len(listed) == 0 || listed[0].ETASeconds != retried.Batch.ETASeconds || got.Batch.ETASeconds != retried.Batch.ETASeconds {
		t.Fatalf("list/get ETA mismatch: list=%#v get=%#v", listed, got.Batch)
	}
}

func TestRetryItemResetsTerminalProgressAndAllowsMonotonicProgress(t *testing.T) {
	for _, parentStatus := range []CommerceItemStatus{CommerceItemFailed, CommerceItemCanceled} {
		t.Run(string(parentStatus), func(t *testing.T) {
			service, db, _, _, project, request := newBatchTestService(t)
			estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
			if err != nil {
				t.Fatal(err)
			}
			parent, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "retry-progress-parent-"+string(parentStatus), SubmitBatchRequest{EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", parent.Items[0].ID).Updates(map[string]any{"status": parentStatus, "progress_percent": 100}).Error; err != nil {
				t.Fatal(err)
			}

			retried, err := service.RetryItem(context.Background(), project.UserID, parent.Items[0].ID, "retry-progress-child-"+string(parentStatus))
			if err != nil {
				t.Fatal(err)
			}
			if len(retried.Items) != 1 || retried.Items[0].Status != CommerceItemQueued || retried.Items[0].ProgressPercent != 0 {
				t.Fatalf("queued retry payload = %#v, want progress 0", retried.Items)
			}
			child := retried.Items[0]
			for _, percent := range []int{25, 60, 90, 100} {
				result := db.Model(&CommerceGenerationItem{}).Where("id = ? AND progress_percent < ?", child.ID, percent).Update("progress_percent", percent)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("advance to %d: rows=%d err=%v", percent, result.RowsAffected, result.Error)
				}
				if err := db.First(&child, child.ID).Error; err != nil {
					t.Fatal(err)
				}
				if child.ProgressPercent != percent {
					t.Fatalf("progress=%d, want %d", child.ProgressPercent, percent)
				}
			}
		})
	}
}

func TestRetryKeepsOriginalSKUSnapshotAfterRenameAndDisable(t *testing.T) {
	service, db, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "snapshot-parent", SubmitBatchRequest{EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID})
	if err != nil {
		t.Fatal(err)
	}
	original := parent.Items[0].OutputSpecJSON
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", parent.Items[0].ID).Update("status", CommerceItemFailed).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceSKU{}).Where("id = ?", request.PrimarySKUID).Updates(map[string]any{"code": "RENAMED", "status": "disabled"}).Error; err != nil {
		t.Fatal(err)
	}
	retried, err := service.RetryItem(context.Background(), project.UserID, parent.Items[0].ID, "snapshot-retry")
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.Items) != 1 || retried.Items[0].OutputSpecJSON != original {
		t.Fatalf("retry changed snapshot")
	}
}

func TestEstimateReservationAndPersistedTaskCountStayConsistent(t *testing.T) {
	service, _, _, _, project, request := newBatchTestService(t)
	estimate, err := service.EstimateBatch(context.Background(), project.UserID, project.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SubmitBatch(context.Background(), project.UserID, project.ID, "count-consistency", SubmitBatchRequest{EstimateBatchRequest: request, PricingSnapshotID: estimate.PricingSnapshotID})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.TotalItems != len(estimate.Items) || result.Batch.TotalItems != len(result.Items) || estimate.TotalItems != result.Batch.TotalItems {
		t.Fatalf("item counts estimate=%d batch=%d persisted=%d", estimate.TotalItems, result.Batch.TotalItems, len(result.Items))
	}
	if estimate.EstimatedCredits != result.Batch.EstimatedCredits || result.Batch.ReservedCredits != result.Batch.EstimatedCredits {
		t.Fatalf("credits estimate=%d batch=%d reserved=%d", estimate.EstimatedCredits, result.Batch.EstimatedCredits, result.Batch.ReservedCredits)
	}
}

func newBatchTestService(t *testing.T) (*Service, *gorm.DB, *Registry, CreditLedger, CommerceProject, EstimateBatchRequest) {
	t.Helper()
	db := openCreditTestDB(t)
	product := CommerceProduct{ID: 21, UserID: 11, Name: "Product", Status: "active"}
	sku := CommerceSKU{ID: 31, UserID: 11, ProductID: product.ID, Code: "SKU-1", Status: "active"}
	project := CommerceProject{ID: 41, UserID: 11, ProductID: product.ID, Pipeline: "general", Status: "active"}
	lockedAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	spec := CommerceCreativeSpec{
		ID: 51, UserID: 11, ProjectID: project.ID, Version: 1, Status: "confirmed", LockedAt: &lockedAt,
		ProductFactsJSON: "{}", SellingPointsJSON: "[]", ForbiddenChangesJSON: "[]", BrandToneJSON: "{}",
		ShotPlanJSON: "[]", CopyBlocksJSON: "[]", RiskNoticesJSON: "[]", SourceAssetIDsJSON: "[]",
	}
	for _, value := range []any{&product, &sku, &project, &spec} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create batch fixture %T: %v", value, err)
		}
	}
	registry := NewRegistry()
	resolver := &fakeCostResolver{price: ItemPrice{Credits: 2, Version: "price-v1"}}
	compiler := fakeRecipe{
		definition: RecipeDefinition{
			Key: "poster", Pipeline: "general", Version: 1,
			AllowedOutputCounts: []int{1}, AspectRatios: []string{"1:1", "3:4"}, QualityTiers: []string{"standard"},
			MaxAttempts: 7,
		},
		resolver: resolver,
		items: []CompiledGenerationItem{{
			SKUID: sku.ID, SKUCode: "SKU-1", SKUSnapshotJSON: `{"id":31,"code":"SKU-1","specification_path":"","attributes_json":"{}"}`, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, SlotKey: "hero", AspectRatio: "1:1",
		}},
	}
	if err := registry.Register(compiler); err != nil {
		t.Fatalf("register compiler: %v", err)
	}
	ledger := newTestAtomicCreditLedger(20)
	service := NewService(NewRepository(db))
	service.ConfigureBatchInfrastructure(registry, ledger, NewGormPricingSnapshotStore(), nil)
	fixedNow := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	request := EstimateBatchRequest{
		RecipeKey: "poster", RecipeVersion: 1, OutputCount: 1, CreativeSpecID: spec.ID, PrimarySKUID: sku.ID,
		QualityTier: "standard", AspectRatio: "1:1",
	}
	return service, db, registry, ledger, project, request
}
