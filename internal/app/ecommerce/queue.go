package ecommerce

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	mathrand "math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CommerceEventJobClaimed        = "job_claimed"
	CommerceEventJobHeartbeat      = "job_heartbeat"
	CommerceEventJobRetryScheduled = "job_retry_scheduled"
	CommerceEventJobSucceeded      = "job_succeeded"
	CommerceEventJobFailed         = "job_failed"
	CommerceEventItemSettled       = "item_settled"
	CommerceEventItemReleased      = "item_released"
)

var errLeaseRecoveryLost = errors.New("commerce lease recovery lost")

type JobTerminalHook interface {
	OnJobTerminalTx(context.Context, *gorm.DB, CommerceJob, time.Time) error
}

type Queue struct {
	DB                  *gorm.DB
	Service             *Service
	WorkerID            string
	Now                 func() time.Time
	Jitter              func() float64
	TerminalHooks       map[JobKind]JobTerminalHook
	LateResultDiscarder func(context.Context, *gorm.DB, CommerceJob, ExecutionResult) error
}

func NewQueue(db *gorm.DB, service *Service, workerID string) *Queue {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "commerce-worker-" + randomLeaseToken()
	}
	return &Queue{
		DB: db, Service: service, WorkerID: workerID,
		Now:           func() time.Time { return time.Now().UTC() },
		Jitter:        func() float64 { return mathrand.Float64() * 0.20 },
		TerminalHooks: make(map[JobKind]JobTerminalHook),
	}
}

func (q *Queue) Claim(ctx context.Context, limit int, leaseDuration time.Duration) ([]JobSnapshot, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []JobSnapshot{}, nil
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("commerce queue lease duration must be positive")
	}
	if err := q.RecoverExpired(ctx, limit); err != nil {
		return nil, err
	}
	if limit > 100 {
		limit = 100
	}
	now := q.currentTime()
	claimed := make([]JobSnapshot, 0, limit)
	err := q.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids, err := q.claimableJobIDs(tx, limit, now)
		if err != nil {
			return err
		}
		for _, id := range ids {
			token := randomLeaseToken()
			updates := map[string]any{
				"status": CommerceJobRunning, "lease_owner": q.WorkerID, "lease_token": token,
				"lease_expires_at": now.Add(leaseDuration), "heartbeat_at": now,
				"attempt_count": gorm.Expr("attempt_count + 1"), "started_at": gorm.Expr("COALESCE(started_at, ?)", now),
				"next_attempt_at": nil, "error_code": "", "error_message": "",
			}
			result := tx.Model(&CommerceJob{}).
				Where("id = ? AND status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?) AND (lease_expires_at IS NULL OR lease_expires_at < ?)",
					id, []CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying}, now, now).
				Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			var job CommerceJob
			if err := tx.Preload("GenerationItem").First(&job, id).Error; err != nil {
				return err
			}
			if job.Kind == CommerceJobKindGenerateItem {
				if job.GenerationItem == nil {
					return ErrGenerationItemRequired
				}
				itemResult := tx.Model(&CommerceGenerationItem{}).
					Where("id = ? AND user_id = ? AND status IN ? AND cancel_requested_at IS NULL", job.GenerationItem.ID, job.UserID, []CommerceItemStatus{CommerceItemQueued, CommerceItemRetrying}).
					Updates(map[string]any{"status": CommerceItemRunning, "progress_percent": gorm.Expr("CASE WHEN progress_percent < 10 THEN 10 ELSE progress_percent END"), "started_at": gorm.Expr("COALESCE(started_at, ?)", now), "error_code": "", "error_message": ""})
				if itemResult.Error != nil {
					return itemResult.Error
				}
				if itemResult.RowsAffected != 1 {
					return ErrInvalidItemTransition
				}
				if err := tx.First(job.GenerationItem, job.GenerationItem.ID).Error; err != nil {
					return err
				}
			}
			if err := emitCommerceEventTx(tx, job, CommerceEventJobClaimed, map[string]any{"attempt": job.AttemptCount, "worker_id": q.WorkerID}); err != nil {
				return err
			}
			if job.BatchID != nil {
				if err := refreshBatchCounters(ctx, tx, job.UserID, *job.BatchID, now); err != nil {
					return err
				}
			}
			claimed = append(claimed, JobSnapshot{Job: job, Item: job.GenerationItem})
		}
		return nil
	})
	return claimed, err
}

func (q *Queue) claimableJobIDs(tx *gorm.DB, limit int, now time.Time) ([]uint, error) {
	var ids []uint
	if tx.Dialector.Name() == "postgres" {
		err := tx.Raw(`SELECT id FROM commerce_jobs
WHERE status IN ('queued','retrying')
  AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
  AND (lease_expires_at IS NULL OR lease_expires_at < NOW())
ORDER BY priority DESC, id ASC
FOR UPDATE SKIP LOCKED
LIMIT ?`, limit).Scan(&ids).Error
		return ids, err
	}
	err := tx.Model(&CommerceJob{}).
		Where("status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?) AND (lease_expires_at IS NULL OR lease_expires_at < ?)",
			[]CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying}, now, now).
		Order("priority DESC, id ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (q *Queue) Heartbeat(ctx context.Context, lease LeaseIdentity, leaseDuration time.Duration) (bool, error) {
	if err := q.validate(); err != nil {
		return false, err
	}
	if leaseDuration <= 0 {
		return false, fmt.Errorf("commerce queue lease duration must be positive")
	}
	now := q.currentTime()
	canceled := false
	err := q.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := loadRunningLeasedJob(tx, lease)
		if err != nil {
			return err
		}
		canceled = job.CancelRequestedAt != nil
		if job.BatchID != nil {
			var batch CommerceGenerationBatch
			if err := tx.Select("cancel_requested_at").Where("id = ? AND user_id = ?", *job.BatchID, job.UserID).First(&batch).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			canceled = canceled || batch.CancelRequestedAt != nil
		}
		if job.GenerationItemID != nil {
			var item CommerceGenerationItem
			if err := tx.Select("cancel_requested_at").Where("id = ? AND user_id = ?", *job.GenerationItemID, job.UserID).First(&item).Error; err != nil {
				return err
			}
			canceled = canceled || item.CancelRequestedAt != nil
		}
		result := tx.Model(&CommerceJob{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", lease.JobID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
			Updates(map[string]any{"heartbeat_at": now, "lease_expires_at": now.Add(leaseDuration)})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseMismatch
		}
		return emitCommerceEventTx(tx, job, CommerceEventJobHeartbeat, map[string]any{"cancel_requested": canceled})
	})
	return canceled, err
}

func (q *Queue) Complete(ctx context.Context, lease LeaseIdentity, result JobResult) error {
	if err := q.validate(); err != nil {
		return err
	}
	now := q.currentTime()
	var terminalJob CommerceJob
	err := q.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := loadRunningLeasedJob(tx, lease)
		if err != nil {
			return err
		}
		if job.CancelRequestedAt != nil {
			if err := q.discardLateResultTx(ctx, tx, job, result); err != nil {
				return err
			}
			if err := q.cancelLeasedJobTx(ctx, tx, job, lease, "late_result_discarded", now, nil); err != nil {
				return err
			}
			terminalJob = job
			return nil
		}
		if job.Kind == CommerceJobKindGenerateItem {
			if job.GenerationItemID == nil || result.Execution == nil {
				return ErrGenerationItemRequired
			}
			var item CommerceGenerationItem
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", *job.GenerationItemID, job.UserID).First(&item).Error; err != nil {
				return err
			}
			if item.CancelRequestedAt != nil {
				if err := q.discardLateResultTx(ctx, tx, job, result); err != nil {
					return err
				}
				if err := q.cancelLeasedJobTx(ctx, tx, job, lease, "late_result_discarded", now, nil); err != nil {
					return err
				}
				terminalJob = job
				return nil
			}
			updates := map[string]any{"status": CommerceItemSucceeded, "progress_percent": 100, "finished_at": now, "error_code": "", "error_message": ""}
			if result.Execution.GenerationRecordID != 0 {
				updates["generation_record_id"] = result.Execution.GenerationRecordID
			}
			if result.Execution.WorkID != 0 {
				updates["work_id"] = result.Execution.WorkID
			}
			itemResult := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, item.UserID, CommerceItemRunning).Updates(updates)
			if itemResult.Error != nil {
				return itemResult.Error
			}
			if itemResult.RowsAffected != 1 {
				var current CommerceGenerationItem
				if err := tx.Where("id = ? AND user_id = ?", item.ID, item.UserID).First(&current).Error; err != nil {
					return err
				}
				if current.CancelRequestedAt != nil {
					if err := q.discardLateResultTx(ctx, tx, job, result); err != nil {
						return err
					}
					if err := q.cancelLeasedJobTx(ctx, tx, job, lease, "late_result_discarded", now, nil); err != nil {
						return err
					}
					terminalJob = job
					return nil
				}
				return ErrInvalidItemTransition
			}
			if q.Service != nil && q.Service.creditLedger != nil {
				if err := q.Service.creditLedger.SettleItemTx(ctx, tx, SettleCreditsRequest{
					UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID, ReservationID: item.ReservationID,
					GenerationItemID: item.ID, HeldCredits: item.ReservedCredits, ActualCredits: result.Execution.ActualCredits,
					IdempotencyKey: fmt.Sprintf("commerce:item:%d:settle", item.ID),
				}); err != nil {
					return err
				}
			} else {
				settled := result.Execution.ActualCredits
				if settled > item.ReservedCredits {
					settled = item.ReservedCredits
				}
				if settled < 0 {
					settled = 0
				}
				if err := tx.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Updates(map[string]any{"settled_credits": settled, "released_credits": item.ReservedCredits - settled}).Error; err != nil {
					return err
				}
			}
			if err := emitCommerceEventTx(tx, job, CommerceEventItemSettled, map[string]any{"item_id": item.ID, "actual_credits": result.Execution.ActualCredits}); err != nil {
				return err
			}
		}
		jobUpdate := tx.Model(&CommerceJob{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", lease.JobID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
			Updates(map[string]any{"status": CommerceJobSucceeded, "finished_at": now, "result_json": result.MetadataJSON, "lease_owner": "", "lease_token": "", "lease_expires_at": nil})
		if jobUpdate.Error != nil {
			return jobUpdate.Error
		}
		if jobUpdate.RowsAffected != 1 {
			return ErrLeaseMismatch
		}
		if err := emitCommerceEventTx(tx, job, CommerceEventJobSucceeded, nil); err != nil {
			return err
		}
		terminalJob = job
		job.Status = CommerceJobSucceeded
		return q.afterJobTerminalTx(ctx, tx, job, now)
	})
	if err != nil {
		return err
	}
	q.finalizeProjectDeletionBestEffort(ctx, terminalJob.UserID, terminalJob.ProjectID, now)
	return nil
}

func (q *Queue) discardLateResultTx(ctx context.Context, tx *gorm.DB, job CommerceJob, result JobResult) error {
	if result.Execution == nil || q.LateResultDiscarder == nil {
		return nil
	}
	return q.LateResultDiscarder(ctx, tx, job, *result.Execution)
}

func (q *Queue) Fail(ctx context.Context, lease LeaseIdentity, failure ExecutionFailure) error {
	if err := q.validate(); err != nil {
		return err
	}
	now := q.currentTime()
	var terminalJob CommerceJob
	err := q.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := loadRunningLeasedJob(tx, lease)
		if err != nil {
			return err
		}
		if job.CancelRequestedAt != nil {
			if err := q.cancelLeasedJobTx(ctx, tx, job, lease, "worker_canceled", now, nil); err != nil {
				return err
			}
			terminalJob = job
			return nil
		}
		if failure.Retryable && job.AttemptCount < job.MaxAttempts {
			next := now.Add(q.retryDelay(job.AttemptCount))
			if job.GenerationItemID != nil {
				itemUpdate := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status = ?", *job.GenerationItemID, job.UserID, CommerceItemRunning).
					Updates(map[string]any{"status": CommerceItemRetrying, "error_code": failure.Code, "error_message": failure.Message})
				if itemUpdate.Error != nil || itemUpdate.RowsAffected != 1 {
					if itemUpdate.Error != nil {
						return itemUpdate.Error
					}
					return ErrInvalidItemTransition
				}
			}
			jobUpdate := tx.Model(&CommerceJob{}).
				Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", lease.JobID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
				Updates(map[string]any{"status": CommerceJobRetrying, "next_attempt_at": next, "error_code": failure.Code, "error_message": failure.Message, "lease_owner": "", "lease_token": "", "lease_expires_at": nil})
			if jobUpdate.Error != nil || jobUpdate.RowsAffected != 1 {
				if jobUpdate.Error != nil {
					return jobUpdate.Error
				}
				return ErrLeaseMismatch
			}
			if err := emitCommerceEventTx(tx, job, CommerceEventJobRetryScheduled, map[string]any{"attempt": job.AttemptCount, "next_attempt_at": next}); err != nil {
				return err
			}
			if job.BatchID != nil {
				return refreshBatchCounters(ctx, tx, job.UserID, *job.BatchID, now)
			}
			return nil
		}
		if err := q.deadLetterLeasedJobTx(ctx, tx, job, lease, failure, now, nil); err != nil {
			return err
		}
		terminalJob = job
		return nil
	})
	if err != nil {
		return err
	}
	if terminalJob.ID != 0 {
		q.finalizeProjectDeletionBestEffort(ctx, terminalJob.UserID, terminalJob.ProjectID, now)
	}
	return nil
}

func (q *Queue) Cancel(ctx context.Context, lease LeaseIdentity, reason string) error {
	if err := q.validate(); err != nil {
		return err
	}
	now := q.currentTime()
	var terminalJob CommerceJob
	err := q.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := loadRunningLeasedJob(tx, lease)
		if err != nil {
			return err
		}
		if err := q.cancelLeasedJobTx(ctx, tx, job, lease, reason, now, nil); err != nil {
			return err
		}
		terminalJob = job
		return nil
	})
	if err != nil {
		return err
	}
	q.finalizeProjectDeletionBestEffort(ctx, terminalJob.UserID, terminalJob.ProjectID, now)
	return nil
}

func (q *Queue) RecoverExpired(ctx context.Context, limit int) error {
	if err := q.validate(); err != nil {
		return err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	now := q.currentTime()
	var ids []uint
	if err := q.DB.WithContext(ctx).Model(&CommerceJob{}).
		Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", CommerceJobRunning, now).
		Order("lease_expires_at ASC, id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		var terminalJob CommerceJob
		err := q.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var job CommerceJob
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", id, CommerceJobRunning, now).First(&job).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			lease := LeaseIdentity{JobID: job.ID, LeaseOwner: job.LeaseOwner, LeaseToken: job.LeaseToken}
			canceled := job.CancelRequestedAt != nil
			if job.BatchID != nil {
				var batch CommerceGenerationBatch
				if err := tx.Select("cancel_requested_at").Where("id = ? AND user_id = ?", *job.BatchID, job.UserID).First(&batch).Error; err != nil {
					return err
				}
				canceled = canceled || batch.CancelRequestedAt != nil
			}
			if job.GenerationItemID != nil {
				var item CommerceGenerationItem
				if err := tx.Select("cancel_requested_at").Where("id = ? AND user_id = ?", *job.GenerationItemID, job.UserID).First(&item).Error; err != nil {
					return err
				}
				canceled = canceled || item.CancelRequestedAt != nil
			}
			if canceled {
				if err := q.cancelLeasedJobTx(ctx, tx, job, lease, "cancel_requested", now, &now); err != nil {
					return err
				}
				terminalJob = job
				return nil
			}
			if job.AttemptCount < job.MaxAttempts {
				next := now.Add(q.retryDelay(job.AttemptCount))
				if job.GenerationItemID != nil {
					if err := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status = ?", *job.GenerationItemID, job.UserID, CommerceItemRunning).
						Updates(map[string]any{"status": CommerceItemRetrying, "error_code": "lease_expired", "error_message": "worker lease expired"}).Error; err != nil {
						return err
					}
				}
				result := tx.Model(&CommerceJob{}).
					Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", job.ID, job.LeaseOwner, job.LeaseToken, CommerceJobRunning, now).
					Updates(map[string]any{"status": CommerceJobRetrying, "next_attempt_at": next, "error_code": "lease_expired", "error_message": "worker lease expired", "lease_owner": "", "lease_token": "", "lease_expires_at": nil})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return errLeaseRecoveryLost
				}
				if err := emitCommerceEventTx(tx, job, CommerceEventJobRetryScheduled, map[string]any{"attempt": job.AttemptCount, "next_attempt_at": next, "reason": "lease_expired"}); err != nil {
					return err
				}
				if job.BatchID != nil {
					return refreshBatchCounters(ctx, tx, job.UserID, *job.BatchID, now)
				}
				return nil
			}
			if err := q.deadLetterLeasedJobTx(ctx, tx, job, lease, ExecutionFailure{Code: "max_attempts_exceeded", Message: "worker lease expired after maximum attempts"}, now, &now); err != nil {
				return err
			}
			terminalJob = job
			return nil
		})
		if errors.Is(err, errLeaseRecoveryLost) || errors.Is(err, ErrLeaseMismatch) {
			continue
		}
		if err != nil {
			return err
		}
		if terminalJob.ID != 0 {
			q.finalizeProjectDeletionBestEffort(ctx, terminalJob.UserID, terminalJob.ProjectID, now)
		}
	}
	_ = q.ReconcileProjectDeletions(ctx, limit)
	return nil
}

func (q *Queue) ReconcileProjectDeletions(ctx context.Context, limit int) error {
	if err := q.validate(); err != nil {
		return err
	}
	if q.Service == nil {
		return nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var projects []CommerceProject
	if err := q.DB.WithContext(ctx).Where("status = ? AND deletion_requested_at IS NOT NULL", "deletion_requested").Order("id ASC").Limit(limit).Find(&projects).Error; err != nil {
		return err
	}
	now := q.currentTime()
	for _, project := range projects {
		if err := runProjectMutationTransaction(ctx, q.DB, func(tx *gorm.DB) error {
			return q.Service.finalizeProjectDeletionTx(ctx, tx, project.UserID, project.ID, now)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) deadLetterLeasedJobTx(ctx context.Context, tx *gorm.DB, job CommerceJob, lease LeaseIdentity, failure ExecutionFailure, now time.Time, leaseExpiredBefore *time.Time) error {
	errorCode := strings.TrimSpace(failure.Code)
	deadLettered := job.AttemptCount >= job.MaxAttempts
	if deadLettered {
		errorCode = "max_attempts_exceeded"
	}
	if errorCode == "" {
		errorCode = "job_failed"
	}
	if job.GenerationItemID != nil {
		var item CommerceGenerationItem
		if err := tx.Where("id = ? AND user_id = ?", *job.GenerationItemID, job.UserID).First(&item).Error; err != nil {
			return err
		}
		itemUpdate := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status = ?", item.ID, item.UserID, CommerceItemRunning).
			Updates(map[string]any{"status": CommerceItemFailed, "progress_percent": 100, "finished_at": now, "error_code": errorCode, "error_message": failure.Message})
		if itemUpdate.Error != nil {
			return itemUpdate.Error
		}
		if itemUpdate.RowsAffected != 1 {
			return ErrInvalidItemTransition
		}
		if err := q.releaseItemTx(ctx, tx, item, "generation_failed"); err != nil {
			return err
		}
		if err := emitCommerceEventTx(tx, job, CommerceEventItemReleased, map[string]any{"item_id": item.ID, "reason": errorCode}); err != nil {
			return err
		}
	}
	updates := map[string]any{"status": CommerceJobFailed, "finished_at": now, "error_code": errorCode, "error_message": failure.Message, "lease_owner": "", "lease_token": "", "lease_expires_at": nil}
	if deadLettered {
		updates["dead_lettered_at"] = now
	}
	jobUpdate := tx.Model(&CommerceJob{}).
		Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", lease.JobID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning)
	if leaseExpiredBefore != nil {
		jobUpdate = jobUpdate.Where("lease_expires_at IS NOT NULL AND lease_expires_at < ?", *leaseExpiredBefore)
	}
	result := jobUpdate.Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseMismatch
	}
	if err := emitCommerceEventTx(tx, job, CommerceEventJobFailed, map[string]any{"error_code": errorCode, "dead_lettered": deadLettered}); err != nil {
		return err
	}
	job.Status = CommerceJobFailed
	job.ErrorCode = errorCode
	job.ErrorMessage = failure.Message
	if deadLettered {
		job.DeadLetteredAt = &now
	}
	return q.afterJobTerminalTx(ctx, tx, job, now)
}

func (q *Queue) cancelLeasedJobTx(ctx context.Context, tx *gorm.DB, job CommerceJob, lease LeaseIdentity, reason string, now time.Time, leaseExpiredBefore *time.Time) error {
	if reason == "" {
		reason = "worker_canceled"
	}
	if job.GenerationItemID != nil {
		var item CommerceGenerationItem
		if err := tx.Where("id = ? AND user_id = ?", *job.GenerationItemID, job.UserID).First(&item).Error; err != nil {
			return err
		}
		if item.Status == CommerceItemRunning {
			if err := q.releaseItemTx(ctx, tx, item, reason); err != nil {
				return err
			}
			if err := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ? AND status = ?", item.ID, item.UserID, CommerceItemRunning).
				Updates(map[string]any{"status": CommerceItemCanceled, "progress_percent": 100, "finished_at": now, "error_code": "canceled", "error_message": reason}).Error; err != nil {
				return err
			}
			if err := emitCommerceEventTx(tx, job, CommerceEventItemReleased, map[string]any{"item_id": item.ID, "reason": reason}); err != nil {
				return err
			}
		}
	}
	jobUpdate := tx.Model(&CommerceJob{}).
		Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", lease.JobID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning)
	if leaseExpiredBefore != nil {
		jobUpdate = jobUpdate.Where("lease_expires_at IS NOT NULL AND lease_expires_at < ?", *leaseExpiredBefore)
	}
	result := jobUpdate.Updates(map[string]any{"status": CommerceJobCanceled, "finished_at": now, "error_code": "canceled", "error_message": reason, "lease_owner": "", "lease_token": "", "lease_expires_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseMismatch
	}
	job.Status = CommerceJobCanceled
	job.ErrorCode = "canceled"
	job.ErrorMessage = reason
	return q.afterJobTerminalTx(ctx, tx, job, now)
}

func (q *Queue) releaseItemTx(ctx context.Context, tx *gorm.DB, item CommerceGenerationItem, reason string) error {
	if q.Service != nil && q.Service.creditLedger != nil {
		return q.Service.creditLedger.ReleaseItemTx(ctx, tx, ReleaseCreditsRequest{
			UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID, ReservationID: item.ReservationID,
			GenerationItemID: item.ID, HeldCredits: item.ReservedCredits, Reason: reason,
			IdempotencyKey: fmt.Sprintf("commerce:item:%d:release", item.ID),
		})
	}
	return tx.Model(&CommerceGenerationItem{}).Where("id = ?", item.ID).Updates(map[string]any{"released_credits": item.ReservedCredits}).Error
}

func (q *Queue) afterJobTerminalTx(ctx context.Context, tx *gorm.DB, job CommerceJob, now time.Time) error {
	if hook := q.TerminalHooks[job.Kind]; hook != nil {
		if err := hook.OnJobTerminalTx(ctx, tx, job, now); err != nil {
			return err
		}
	}
	if job.BatchID != nil {
		if err := refreshBatchCounters(ctx, tx, job.UserID, *job.BatchID, now); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) finalizeProjectDeletionBestEffort(ctx context.Context, userID, projectID uint, now time.Time) {
	if q.Service == nil || projectID == 0 {
		return
	}
	_ = runProjectMutationTransaction(ctx, q.DB, func(tx *gorm.DB) error {
		return q.Service.finalizeProjectDeletionTx(ctx, tx, userID, projectID, now)
	})
}

func (q *Queue) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exponent := attempt - 1
	if exponent > 16 {
		exponent = 16
	}
	base := time.Duration(math.Pow(2, float64(exponent))) * 5 * time.Second
	if base > 5*time.Minute {
		base = 5 * time.Minute
	}
	jitter := 0.0
	if q.Jitter != nil {
		jitter = q.Jitter()
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 0.20 {
		jitter = 0.20
	}
	delay := base + time.Duration(float64(base)*jitter)
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func (q *Queue) currentTime() time.Time {
	if q.Now == nil {
		return time.Now().UTC()
	}
	return q.Now().UTC()
}

func (q *Queue) validate() error {
	if q == nil || q.DB == nil {
		return fmt.Errorf("commerce queue database is unavailable")
	}
	if strings.TrimSpace(q.WorkerID) == "" {
		return fmt.Errorf("commerce queue worker ID is required")
	}
	return nil
}

func loadRunningLeasedJob(tx *gorm.DB, lease LeaseIdentity) (CommerceJob, error) {
	if lease.JobID == 0 || strings.TrimSpace(lease.LeaseOwner) == "" || strings.TrimSpace(lease.LeaseToken) == "" {
		return CommerceJob{}, ErrLeaseMismatch
	}
	var job CommerceJob
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", lease.JobID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CommerceJob{}, ErrLeaseMismatch
	}
	return job, err
}

func emitCommerceEventTx(tx *gorm.DB, job CommerceJob, eventType string, metadata any) error {
	metadataJSON := "{}"
	if metadata != nil {
		encoded, err := EncodeJSON(metadata)
		if err != nil {
			return err
		}
		metadataJSON = encoded
	}
	entityType, entityID := "job", job.ID
	return tx.Create(&CommerceEvent{
		UserID: job.UserID, ProjectID: job.ProjectID, BatchID: job.BatchID, JobID: &job.ID,
		EntityType: entityType, EntityID: entityID, Pipeline: job.Pipeline, RecipeKey: job.RecipeKey,
		EventType: eventType, MetadataJSON: metadataJSON,
	}).Error
}

func randomLeaseToken() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
