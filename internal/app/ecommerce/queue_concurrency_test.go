package ecommerce

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestQueueRecoverExpiredDoesNotReplaceRenewedLease(t *testing.T) {
	db := newQueueTestDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	job, item := seedQueueJob(t, db, "recover-heartbeat-race", 3)
	expired := now.Add(-time.Second)
	if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "worker-a",
		"lease_token": "lease-a", "lease_expires_at": expired,
	}).Error; err != nil {
		t.Fatalf("seed expired lease: %v", err)
	}
	if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Update("status", CommerceItemRunning).Error; err != nil {
		t.Fatalf("seed running item: %v", err)
	}

	var injected atomic.Bool
	const callbackName = "test:renew-lease-after-recovery-read"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if _, isJobRead := tx.Statement.Dest.(*CommerceJob); !isJobRead || tx.Statement.Table != "commerce_jobs" || tx.RowsAffected != 1 || !injected.CompareAndSwap(false, true) {
			return
		}
		renewedUntil := now.Add(time.Minute)
		clean := tx.Session(&gorm.Session{NewDB: true})
		if err := clean.Model(&CommerceJob{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", job.ID, "worker-a", "lease-a", CommerceJobRunning).
			Updates(map[string]any{"heartbeat_at": now, "lease_expires_at": renewedUntil}).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatalf("register lease renewal callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	queue := NewQueue(db, nil, "recovery-worker")
	queue.Now = func() time.Time { return now }
	queue.Jitter = func() float64 { return 0 }
	if err := queue.RecoverExpired(context.Background(), 10); err != nil {
		t.Fatalf("RecoverExpired: %v", err)
	}
	lease := LeaseIdentity{JobID: job.ID, LeaseOwner: "worker-a", LeaseToken: "lease-a"}
	if canceled, err := queue.Heartbeat(context.Background(), lease, time.Minute); err != nil || canceled {
		t.Fatalf("Heartbeat after recovery CAS rollback canceled=%v err=%v", canceled, err)
	}

	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if err := db.First(&item, item.ID).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if job.Status != CommerceJobRunning || job.LeaseOwner != "worker-a" || job.LeaseToken != "lease-a" || job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("renewed job was replaced by recovery: %#v", job)
	}
	if item.Status != CommerceItemRunning {
		t.Fatalf("item changed despite renewed lease: %#v", item)
	}
}

func TestQueueTerminalPathsLockJobBeforeItem(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Queue, LeaseIdentity) error
	}{
		{name: "complete", run: func(q *Queue, lease LeaseIdentity) error {
			return q.Complete(context.Background(), lease, JobResult{Execution: &ExecutionResult{ActualCredits: 1}})
		}},
		{name: "fail", run: func(q *Queue, lease LeaseIdentity) error {
			return q.Fail(context.Background(), lease, ExecutionFailure{Code: "retry", Message: "retry", Retryable: true})
		}},
		{name: "cancel", run: func(q *Queue, lease LeaseIdentity) error {
			return q.Cancel(context.Background(), lease, "test")
		}},
		{name: "recover", run: func(q *Queue, _ LeaseIdentity) error {
			return q.RecoverExpired(context.Background(), 1)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newQueueTestDB(t)
			now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
			job, item := seedQueueJob(t, db, "lock-order-"+tt.name, 3)
			leaseExpiresAt := now.Add(time.Minute)
			if tt.name == "recover" {
				leaseExpiresAt = now.Add(-time.Second)
			}
			if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(map[string]any{
				"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "worker-a",
				"lease_token": "lease-a", "lease_expires_at": leaseExpiresAt,
			}).Error; err != nil {
				t.Fatalf("seed running job: %v", err)
			}
			if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Update("status", CommerceItemRunning).Error; err != nil {
				t.Fatalf("seed running item: %v", err)
			}

			var mu sync.Mutex
			lockedTables := make([]string, 0, 2)
			callbackName := fmt.Sprintf("test:audit-lock-order:%s", tt.name)
			if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if _, locked := tx.Statement.Clauses["FOR"]; !locked {
					return
				}
				mu.Lock()
				lockedTables = append(lockedTables, tx.Statement.Table)
				mu.Unlock()
			}); err != nil {
				t.Fatalf("register lock audit callback: %v", err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

			queue := NewQueue(db, nil, "worker-a")
			queue.Now = func() time.Time { return now }
			queue.Jitter = func() float64 { return 0 }
			lease := LeaseIdentity{JobID: job.ID, LeaseOwner: "worker-a", LeaseToken: "lease-a"}
			if err := tt.run(queue, lease); err != nil {
				t.Fatalf("run %s: %v", tt.name, err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(lockedTables) == 0 || lockedTables[0] != "commerce_jobs" {
				t.Fatalf("row lock order = %v, want commerce_jobs first", lockedTables)
			}
		})
	}
}

func TestQueueTerminalTransitionSurvivesDeferredProjectFinalizationFailure(t *testing.T) {
	tests := []struct {
		name       string
		wantJob    CommerceJobStatus
		wantItem   CommerceItemStatus
		prepareJob func(*testing.T, *gorm.DB, CommerceJob, time.Time)
		run        func(*Queue, LeaseIdentity) error
	}{
		{
			name: "complete", wantJob: CommerceJobSucceeded, wantItem: CommerceItemSucceeded,
			run: func(q *Queue, lease LeaseIdentity) error {
				return q.Complete(context.Background(), lease, JobResult{Execution: &ExecutionResult{ActualCredits: 1}})
			},
		},
		{
			name: "fail", wantJob: CommerceJobFailed, wantItem: CommerceItemFailed,
			run: func(q *Queue, lease LeaseIdentity) error {
				return q.Fail(context.Background(), lease, ExecutionFailure{Code: "provider_failed", Message: "failed", Retryable: false})
			},
		},
		{
			name: "cancel", wantJob: CommerceJobCanceled, wantItem: CommerceItemCanceled,
			run: func(q *Queue, lease LeaseIdentity) error {
				return q.Cancel(context.Background(), lease, "test cancellation")
			},
		},
		{
			name: "recover", wantJob: CommerceJobFailed, wantItem: CommerceItemFailed,
			prepareJob: func(t *testing.T, db *gorm.DB, job CommerceJob, now time.Time) {
				t.Helper()
				if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(map[string]any{
					"attempt_count": job.MaxAttempts, "lease_expires_at": now.Add(-time.Second),
				}).Error; err != nil {
					t.Fatalf("expire recover lease: %v", err)
				}
			},
			run: func(q *Queue, _ LeaseIdentity) error {
				return q.RecoverExpired(context.Background(), 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newQueueTestDB(t)
			now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
			job, item := seedQueueJob(t, db, "deferred-project-finalize-"+tt.name, 3)
			if err := db.Model(&CommerceProject{}).Where("id = ?", job.ProjectID).Updates(map[string]any{
				"status": "deletion_requested", "deletion_requested_at": now,
			}).Error; err != nil {
				t.Fatalf("mark project deleting: %v", err)
			}
			if err := db.Model(&CommerceJob{}).Where("id = ?", job.ID).Updates(map[string]any{
				"status": CommerceJobRunning, "attempt_count": 1, "lease_owner": "worker-a",
				"lease_token": "lease-a", "lease_expires_at": now.Add(time.Minute),
			}).Error; err != nil {
				t.Fatalf("seed running job: %v", err)
			}
			if err := db.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Update("status", CommerceItemRunning).Error; err != nil {
				t.Fatalf("seed running item: %v", err)
			}
			if tt.prepareJob != nil {
				tt.prepareJob(t, db, job, now)
			}

			var injected atomic.Bool
			callbackName := "test:fail-deferred-project-finalize:" + tt.name
			if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == "commerce_projects" {
					injected.Store(true)
					tx.AddError(fmt.Errorf("injected project finalization failure"))
				}
			}); err != nil {
				t.Fatalf("register project failure callback: %v", err)
			}
			t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

			service := NewService(NewRepository(db))
			queue := NewQueue(db, service, "worker-a")
			queue.Now = func() time.Time { return now }
			queue.Jitter = func() float64 { return 0 }
			lease := LeaseIdentity{JobID: job.ID, LeaseOwner: "worker-a", LeaseToken: "lease-a"}
			if err := tt.run(queue, lease); err != nil {
				t.Fatalf("terminal transition returned deferred project error: %v", err)
			}
			if !injected.Load() {
				t.Fatal("project finalization failure was not injected")
			}

			if err := db.First(&job, job.ID).Error; err != nil {
				t.Fatalf("reload job: %v", err)
			}
			if err := db.First(&item, item.ID).Error; err != nil {
				t.Fatalf("reload item: %v", err)
			}
			if job.Status != tt.wantJob || item.Status != tt.wantItem {
				t.Fatalf("terminal state rolled back: job=%s item=%s, want job=%s item=%s", job.Status, item.Status, tt.wantJob, tt.wantItem)
			}
			var project CommerceProject
			if err := db.First(&project, job.ProjectID).Error; err != nil {
				t.Fatalf("project should remain for reconcile: %v", err)
			}
			if project.Status != "deletion_requested" || project.DeletedAt.Valid {
				t.Fatalf("project should remain deletion_requested after deferred failure: %#v", project)
			}
		})
	}
}
