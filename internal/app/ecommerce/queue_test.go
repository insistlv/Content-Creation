package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestQueueClaimLease(t *testing.T) {
	db := newQueueTestDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	job, item := seedQueueJob(t, db, "claim", 3)
	queue := NewQueue(db, nil, "worker-a")
	queue.Now = func() time.Time { return now }

	claimed, err := queue.Claim(context.Background(), 1, 30*time.Second)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Job.ID != job.ID || claimed[0].Item == nil || claimed[0].Item.ID != item.ID {
		t.Fatalf("claimed = %#v", claimed)
	}
	if claimed[0].Item.ProgressPercent != 10 {
		t.Fatalf("claimed item progress=%d want 10", claimed[0].Item.ProgressPercent)
	}
	lease := claimed[0].Job
	if lease.Status != CommerceJobRunning || lease.LeaseOwner != "worker-a" || lease.LeaseToken == "" || lease.LeaseExpiresAt == nil || !lease.LeaseExpiresAt.Equal(now.Add(30*time.Second)) || lease.AttemptCount != 1 {
		t.Fatalf("lease = %#v", lease)
	}

	second := NewQueue(db, nil, "worker-b")
	second.Now = queue.Now
	other, err := second.Claim(context.Background(), 1, 30*time.Second)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("second worker claimed active lease: %#v", other)
	}
}

func TestQueueRetryDelayNeverExceedsFiveMinutes(t *testing.T) {
	queue := &Queue{Jitter: func() float64 { return 0.20 }}
	if got := queue.retryDelay(99); got != 5*time.Minute {
		t.Fatalf("retryDelay(99) = %s, want final cap 5m", got)
	}
}

func TestQueueHeartbeatExtendsLeaseAndReportsCancellation(t *testing.T) {
	db := newQueueTestDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, _ = seedQueueJob(t, db, "heartbeat", 3)
	queue := NewQueue(db, nil, "worker-heartbeat")
	queue.Now = func() time.Time { return now }
	claimed, err := queue.Claim(context.Background(), 1, 20*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim = %#v, %v", claimed, err)
	}
	lease := claimed[0].Lease()

	now = now.Add(10 * time.Second)
	canceled, err := queue.Heartbeat(context.Background(), lease, 20*time.Second)
	if err != nil || canceled {
		t.Fatalf("Heartbeat canceled=%v err=%v", canceled, err)
	}
	var reloaded CommerceJob
	if err := db.First(&reloaded, lease.JobID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if reloaded.HeartbeatAt == nil || !reloaded.HeartbeatAt.Equal(now) || reloaded.LeaseExpiresAt == nil || !reloaded.LeaseExpiresAt.Equal(now.Add(20*time.Second)) {
		t.Fatalf("heartbeat job = %#v", reloaded)
	}

	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", *reloaded.GenerationItemID).Update("cancel_requested_at", now).Error; err != nil {
		t.Fatalf("request cancellation: %v", err)
	}
	canceled, err = queue.Heartbeat(context.Background(), lease, 20*time.Second)
	if err != nil || !canceled {
		t.Fatalf("cancel heartbeat canceled=%v err=%v", canceled, err)
	}
}

func TestQueueExpiredLeaseRetriesThenDeadLetters(t *testing.T) {
	db := newQueueTestDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	job, _ := seedQueueJob(t, db, "expired-retry", 2)
	expired := now.Add(-time.Second)
	if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "old", "lease_token": "old-token", "lease_expires_at": expired,
	}).Error; err != nil {
		t.Fatalf("seed expired lease: %v", err)
	}
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", *job.GenerationItemID).Update("status", CommerceItemRunning).Error; err != nil {
		t.Fatalf("seed running item: %v", err)
	}
	queue := NewQueue(db, nil, "recoverer")
	queue.Now = func() time.Time { return now }
	queue.Jitter = func() float64 { return 0 }

	if err := queue.RecoverExpired(context.Background(), 10); err != nil {
		t.Fatalf("RecoverExpired retry: %v", err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatalf("reload retry job: %v", err)
	}
	if job.Status != CommerceJobRetrying || job.NextAttemptAt == nil || !job.NextAttemptAt.Equal(now.Add(5*time.Second)) || job.LeaseOwner != "" || job.LeaseToken != "" {
		t.Fatalf("retry job = %#v", job)
	}

	job2, item2 := seedQueueJob(t, db, "expired-dead", 1)
	if err := db.Model(&CommerceJob{}).Where("id = ?", job2.ID).Updates(map[string]any{
		"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "old", "lease_token": "old-token", "lease_expires_at": expired,
	}).Error; err != nil {
		t.Fatalf("seed dead lease: %v", err)
	}
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item2.ID).Updates(map[string]any{"status": CommerceItemRunning, "reserved_credits": 2}).Error; err != nil {
		t.Fatalf("seed dead item: %v", err)
	}
	if err := queue.RecoverExpired(context.Background(), 10); err != nil {
		t.Fatalf("RecoverExpired dead letter: %v", err)
	}
	if err := db.First(&job2, job2.ID).Error; err != nil {
		t.Fatalf("reload dead job: %v", err)
	}
	if err := db.First(&item2, item2.ID).Error; err != nil {
		t.Fatalf("reload released item: %v", err)
	}
	if job2.Status != CommerceJobFailed || job2.ErrorCode != "max_attempts_exceeded" || job2.DeadLetteredAt == nil || item2.Status != CommerceItemFailed {
		t.Fatalf("dead job=%#v item=%#v", job2, item2)
	}
}

func TestQueueRecoverExpiredCanceledLeaseFinalizesAndReleases(t *testing.T) {
	for _, cancelSource := range []string{"job", "batch", "item"} {
		t.Run(cancelSource, func(t *testing.T) {
			db := newQueueTestDB(t)
			now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
			job, item := seedQueueJob(t, db, "expired-canceled-"+cancelSource, 3)
			expired := now.Add(-time.Second)
			jobUpdates := map[string]any{
				"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "old", "lease_token": "old-token", "lease_expires_at": expired,
			}
			if cancelSource == "job" {
				jobUpdates["cancel_requested_at"] = now
			}
			if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(jobUpdates).Error; err != nil {
				t.Fatalf("seed expired job: %v", err)
			}
			itemUpdates := map[string]any{"status": CommerceItemRunning, "reserved_credits": 4}
			if cancelSource == "item" {
				itemUpdates["cancel_requested_at"] = now
			}
			if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Updates(itemUpdates).Error; err != nil {
				t.Fatalf("seed running item: %v", err)
			}
			if cancelSource == "batch" {
				if err := db.Model(&CommerceGenerationBatch{}).Where("id = ?", item.BatchID).Update("cancel_requested_at", now).Error; err != nil {
					t.Fatalf("seed canceled batch: %v", err)
				}
			}
			queue := NewQueue(db, nil, "recover-canceled")
			queue.Now = func() time.Time { return now }
			queue.Jitter = func() float64 { return 0 }

			if err := queue.RecoverExpired(context.Background(), 10); err != nil {
				t.Fatalf("RecoverExpired: %v", err)
			}
			if err := db.First(&job, job.ID).Error; err != nil {
				t.Fatalf("reload job: %v", err)
			}
			if err := db.First(&item, item.ID).Error; err != nil {
				t.Fatalf("reload item: %v", err)
			}
			if job.Status != CommerceJobCanceled || item.Status != CommerceItemCanceled || item.ReleasedCredits != 4 {
				t.Fatalf("canceled recovery job=%#v item=%#v", job, item)
			}
		})
	}
}

func TestQueueCanceledExpiredLeaseDoesNotBlockOtherClaims(t *testing.T) {
	db := newQueueTestDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	canceledJob, canceledItem := seedQueueJob(t, db, "expired-canceled-first", 3)
	otherJob, _ := seedQueueJob(t, db, "claim-after-canceled", 3)
	expired := now.Add(-time.Second)
	if err := db.Model(&CommerceJob{}).Where("id = ?", canceledJob.ID).Updates(map[string]any{
		"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "old", "lease_token": "old-token", "lease_expires_at": expired,
	}).Error; err != nil {
		t.Fatalf("seed expired job: %v", err)
	}
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", canceledItem.ID).Updates(map[string]any{
		"status": CommerceItemRunning, "cancel_requested_at": now,
	}).Error; err != nil {
		t.Fatalf("seed canceled item: %v", err)
	}
	queue := NewQueue(db, nil, "recover-then-claim")
	queue.Now = func() time.Time { return now }
	queue.Jitter = func() float64 { return 0 }
	if err := queue.RecoverExpired(context.Background(), 10); err != nil {
		t.Fatalf("first RecoverExpired: %v", err)
	}
	now = now.Add(5 * time.Second)
	claimed, err := queue.Claim(context.Background(), 2, time.Minute)
	if err != nil {
		t.Fatalf("Claim after canceled recovery: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Job.ID != otherJob.ID {
		t.Fatalf("claimed = %#v, want only job %d", claimed, otherJob.ID)
	}
}

func TestProductDetailLateCanceledWorkIsDiscardedBeforeQueueTerminal(t *testing.T) {
	db := newQueueTestDB(t)
	job, item := seedQueueJob(t, db, "late-product-detail", 3)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": CommerceJobRunning, "lease_owner": "worker", "lease_token": "token", "lease_expires_at": now.Add(time.Minute), "cancel_requested_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Update("status", CommerceItemRunning).Error; err != nil {
		t.Fatal(err)
	}
	discarded := uint(0)
	queue := NewQueue(db, nil, "worker")
	queue.LateResultDiscarder = func(_ context.Context, _ *gorm.DB, _ CommerceJob, result ExecutionResult) error {
		discarded = result.WorkID
		return nil
	}
	err := queue.Complete(context.Background(), LeaseIdentity{JobID: job.ID, LeaseOwner: "worker", LeaseToken: "token"}, JobResult{Execution: &ExecutionResult{WorkID: 99, GenerationRecordID: 88, ActualCredits: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 99 {
		t.Fatalf("discarded work = %d", discarded)
	}
}

func TestQueueLeaseCASRejectsStaleWorker(t *testing.T) {
	db := newQueueTestDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, _ = seedQueueJob(t, db, "cas", 3)
	queue := NewQueue(db, nil, "worker-cas")
	queue.Now = func() time.Time { return now }
	claimed, err := queue.Claim(context.Background(), 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim = %#v, %v", claimed, err)
	}
	stale := claimed[0].Lease()
	stale.LeaseToken = "stale-token"
	if _, err := queue.Heartbeat(context.Background(), stale, time.Minute); !errors.Is(err, ErrLeaseMismatch) {
		t.Fatalf("Heartbeat stale error = %v, want ErrLeaseMismatch", err)
	}
}

func TestProjectDeletionCancelsQueuedWorkAndFinalizesAfterRunningLease(t *testing.T) {
	db := newQueueTestDB(t)
	service := NewService(NewRepository(db))
	service.ConfigureBatchInfrastructure(NewRegistry(), newTestAtomicCreditLedger(20), NewGormPricingSnapshotStore(), nil)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	queuedJob, queuedItem := seedQueueJob(t, db, "delete-queued", 3)
	runningJob, runningItem := seedQueueJob(t, db, "delete-running", 3)
	if err := db.Model(&CommerceJob{}).Where("id = ?", runningJob.ID).Updates(map[string]any{
		"status": CommerceJobRunning, "lease_owner": "delete-worker", "lease_token": "delete-token", "lease_expires_at": now.Add(time.Minute), "attempt_count": 1,
	}).Error; err != nil {
		t.Fatalf("seed running job: %v", err)
	}
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", runningItem.ID).Update("status", CommerceItemRunning).Error; err != nil {
		t.Fatalf("seed running item: %v", err)
	}

	if _, err := service.RequestProjectDeletion(context.Background(), queuedJob.UserID, queuedJob.ProjectID); err != nil {
		t.Fatalf("delete queued project: %v", err)
	}
	if err := db.First(&queuedJob, queuedJob.ID).Error; err != nil {
		t.Fatalf("reload queued job: %v", err)
	}
	if err := db.First(&queuedItem, queuedItem.ID).Error; err != nil {
		t.Fatalf("reload queued item: %v", err)
	}
	if queuedJob.Status != CommerceJobCanceled || queuedItem.Status != CommerceItemCanceled {
		t.Fatalf("queued deletion job=%#v item=%#v", queuedJob, queuedItem)
	}
	queue := NewQueue(db, service, "delete-worker")
	queue.Now = func() time.Time { return now }
	if err := queue.ReconcileProjectDeletions(context.Background(), 10); err != nil {
		t.Fatalf("reconcile queued project deletion: %v", err)
	}
	var queuedProject CommerceProject
	if err := db.Unscoped().First(&queuedProject, queuedJob.ProjectID).Error; err != nil {
		t.Fatalf("load queued finalized project: %v", err)
	}
	if !queuedProject.DeletedAt.Valid {
		t.Fatalf("queued project was not finalized: %#v", queuedProject)
	}

	if _, err := service.RequestProjectDeletion(context.Background(), runningJob.UserID, runningJob.ProjectID); err != nil {
		t.Fatalf("delete running project: %v", err)
	}
	if err := db.First(&runningJob, runningJob.ID).Error; err != nil {
		t.Fatalf("reload running job: %v", err)
	}
	if err := db.First(&runningItem, runningItem.ID).Error; err != nil {
		t.Fatalf("reload running item: %v", err)
	}
	if runningJob.CancelRequestedAt == nil || runningItem.CancelRequestedAt == nil {
		t.Fatalf("running cancellation job=%#v item=%#v", runningJob, runningItem)
	}
	if err := queue.Cancel(context.Background(), LeaseIdentity{JobID: runningJob.ID, LeaseOwner: "delete-worker", LeaseToken: "delete-token"}, "project_deleted"); err != nil {
		t.Fatalf("cancel running deletion: %v", err)
	}
	var project CommerceProject
	if err := db.Unscoped().First(&project, runningJob.ProjectID).Error; err != nil {
		t.Fatalf("load finalized project: %v", err)
	}
	if !project.DeletedAt.Valid {
		t.Fatalf("project was not soft deleted: %#v", project)
	}
}

func TestListBatchEventsIsUserScopedAndUsesAfterID(t *testing.T) {
	db := newQueueTestDB(t)
	job, _ := seedQueueJob(t, db, "events-owner", 3)
	other, _ := seedQueueJob(t, db, "events-other", 3)
	first := CommerceEvent{UserID: job.UserID, ProjectID: job.ProjectID, BatchID: job.BatchID, JobID: &job.ID, EntityType: "job", EntityID: job.ID, EventType: CommerceEventJobClaimed, MetadataJSON: "{}"}
	second := CommerceEvent{UserID: job.UserID, ProjectID: job.ProjectID, BatchID: job.BatchID, JobID: &job.ID, EntityType: "job", EntityID: job.ID, EventType: CommerceEventJobHeartbeat, MetadataJSON: "{}"}
	leak := CommerceEvent{UserID: other.UserID, ProjectID: other.ProjectID, BatchID: other.BatchID, JobID: &other.ID, EntityType: "job", EntityID: other.ID, EventType: CommerceEventJobFailed, MetadataJSON: "{}"}
	for _, event := range []*CommerceEvent{&first, &second, &leak} {
		if err := db.Create(event).Error; err != nil {
			t.Fatalf("create event: %v", err)
		}
	}
	service := NewService(NewRepository(db))
	events, err := service.ListBatchEvents(context.Background(), job.UserID, *job.BatchID, first.ID)
	if err != nil {
		t.Fatalf("ListBatchEvents: %v", err)
	}
	if len(events) != 1 || events[0].ID != second.ID {
		t.Fatalf("events = %#v", events)
	}
	if _, err := service.ListBatchEvents(context.Background(), 999, *job.BatchID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user events error = %v", err)
	}
}

func newQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate foundation: %v", err)
	}
	return db
}

func seedQueueJob(t *testing.T, db *gorm.DB, prefix string, maxAttempts int) (CommerceJob, CommerceGenerationItem) {
	t.Helper()
	project := CommerceProject{UserID: 801, ProductID: 1, Title: prefix, Pipeline: "general", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	batch := CommerceGenerationBatch{UserID: project.UserID, ProjectID: project.ID, PrimarySKUID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, Status: CommerceBatchQueued, IdempotencyKey: prefix + ":batch", TotalItems: 1, QueuedItems: 1}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	compiled, err := EncodeJSON(CompiledGenerationItem{SKUID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, SlotKey: "hero"})
	if err != nil {
		t.Fatalf("encode compiled item: %v", err)
	}
	item := CommerceGenerationItem{UserID: project.UserID, ProjectID: project.ID, BatchID: batch.ID, ReservationID: 1, SKUID: 1, Pipeline: "general", RecipeKey: "poster", RecipeVersion: 1, Status: CommerceItemQueued, IdempotencyKey: prefix + ":item", OutputSpecJSON: compiled}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	batchID, itemID := batch.ID, item.ID
	job := CommerceJob{UserID: project.UserID, ProjectID: project.ID, BatchID: &batchID, GenerationItemID: &itemID, Kind: CommerceJobKindGenerateItem, Pipeline: "general", RecipeKey: "poster", Status: CommerceJobQueued, IdempotencyKey: prefix + ":job", MaxAttempts: maxAttempts}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job, item
}
