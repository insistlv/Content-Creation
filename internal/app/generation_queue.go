package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	generationRequestSnapshotVersion = 1
	imageExecutionLeaseTTL           = 90 * time.Second
	imageExecutionLeaseRenewInterval = 30 * time.Second
	imageQueuePollInterval           = 100 * time.Millisecond
	imageQueueMaxExternalAttempts    = 3
	imageQueueOrphanMaxAge           = 24 * time.Hour
	imageGenerationPersistenceLimit  = 5 * time.Minute
	imageExecutionLeaseAdvisoryLock  = int64(0x494D47454E4C4541)
	imageGenerationQueueAdvisoryLock = int64(0x494D47454E515545)
)

var (
	errGenerationQueueFull       = errors.New("generation queue full")
	errGenerationIdempotency     = errors.New("generation idempotency conflict")
	errGenerationQueueLeaseLost  = errors.New("generation queue lease lost")
	errGenerationPayloadTooLarge = errors.New("generation payload too large")
)

type generationQueueSnapshot struct {
	Version int               `json:"version"`
	Request generationRequest `json:"request"`
}

type generationExecutionLeaseMeta struct {
	JobID      *uint
	RecordID   *uint
	UserID     uint
	ProviderID uint
	ChannelID  uint
	EntryPoint string
}

func (a *App) generationQueueCapacity() int {
	if a.cfg.GenerationQueueCapacity <= 0 {
		return 500
	}
	return a.cfg.GenerationQueueCapacity
}

func (a *App) generationUserPendingLimit() int {
	if a.cfg.GenerationUserPendingLimit <= 0 {
		return 32
	}
	return a.cfg.GenerationUserPendingLimit
}

func (a *App) generationQueueTimeout() time.Duration {
	seconds := a.cfg.GenerationQueueTimeoutSeconds
	if seconds <= 0 {
		seconds = 30 * 60
	}
	return time.Duration(seconds) * time.Second
}

func (a *App) prepareGenerationSpool() error {
	path := strings.TrimSpace(a.cfg.GenerationSpoolPath)
	if path == "" {
		base := strings.TrimSpace(a.cfg.AssetStoragePath)
		if base == "" {
			base = "data/assets"
		}
		path = filepath.Join(filepath.Dir(base), "generation-spool")
		a.cfg.GenerationSpoolPath = path
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create generation spool: %w", err)
	}
	return filepath.WalkDir(path, func(name string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) > imageQueueOrphanMaxAge {
			if removeErr := os.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
		}
		return nil
	})
}

func generationQueueIdempotencyKey(request generationRequest, explicit string) string {
	if key := strings.TrimSpace(explicit); key != "" {
		return key
	}
	if strings.TrimSpace(request.BatchID) != "" {
		return fmt.Sprintf("batch:%s:%d", strings.TrimSpace(request.BatchID), request.BatchIndex)
	}
	// Optional idempotency means callers without a key still need an internal,
	// unique value so the composite database constraint never collapses two
	// independent submissions from the same user.
	return "auto:" + uuid.NewString()
}

func generationQueueSnapshotFor(job *generationJob) (string, string, error) {
	payload, err := json.Marshal(generationQueueSnapshot{Version: generationRequestSnapshotVersion, Request: job.Request})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(payload)
	return string(payload), hex.EncodeToString(sum[:]), nil
}

func (a *App) enqueueGenerationJob(job *generationJob, idempotencyKey, entryPoint string) (GenerationRecord, ImageGenerationJob, bool, error) {
	var record GenerationRecord
	var queueJob ImageGenerationJob
	if job == nil || job.User.ID == 0 {
		return record, queueJob, false, errors.New("invalid generation job")
	}
	snapshotJSON, fingerprint, err := generationQueueSnapshotFor(job)
	if err != nil {
		return record, queueJob, false, err
	}
	idempotencyKey = generationQueueIdempotencyKey(job.Request, idempotencyKey)
	now := time.Now().UTC()
	reused := false
	err = a.db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", imageGenerationQueueAdvisoryLock).Error; err != nil {
				return err
			}
		}
		if idempotencyKey != "" {
			err := tx.Where("user_id = ? AND idempotency_key = ? AND idempotency_expires_at > ?", job.User.ID, idempotencyKey, now).First(&queueJob).Error
			if err == nil {
				if queueJob.RequestFingerprint != fingerprint {
					return errGenerationIdempotency
				}
				if err := tx.First(&record, queueJob.GenerationRecordID).Error; err != nil {
					return err
				}
				reused = true
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var expired ImageGenerationJob
			if err := tx.Where("user_id = ? AND idempotency_key = ?", job.User.ID, idempotencyKey).First(&expired).Error; err == nil {
				if err := tx.Model(&ImageGenerationJob{}).Where("id = ?", expired.ID).Update("idempotency_key", fmt.Sprintf("expired:%d:%s", expired.ID, uuid.NewString())).Error; err != nil {
					return err
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		activeStatuses := []string{ImageGenerationJobStatusQueued, ImageGenerationJobStatusRunning, ImageGenerationJobStatusRetryWait, ImageGenerationJobStatusPersisting}
		var globalCount, userCount int64
		if err := tx.Model(&ImageGenerationJob{}).Where("status IN ?", activeStatuses).Count(&globalCount).Error; err != nil {
			return err
		}
		if globalCount >= int64(a.generationQueueCapacity()) {
			return errGenerationQueueFull
		}
		if err := tx.Model(&ImageGenerationJob{}).Where("user_id = ? AND status IN ?", job.User.ID, activeStatuses).Count(&userCount).Error; err != nil {
			return err
		}
		if userCount >= int64(a.generationUserPendingLimit()) {
			return errGenerationQueueFull
		}

		credits := generationJobCreditCost(job)
		result := tx.Model(&CreditBalance{}).
			Where("user_id = ? AND available_credits >= ?", job.User.ID, credits).
			Updates(map[string]any{
				"available_credits": gorm.Expr("available_credits - ?", credits),
				"reserved_credits":  gorm.Expr("reserved_credits + ?", credits),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errCreditsInsufficient
		}

		appInTx := *a
		appInTx.db = tx
		created, err := appInTx.createGenerationRecord(job, GenerationStatusQueued, GenerationStageQueued)
		if err != nil {
			return err
		}
		record = created
		record.RequestFingerprint = fingerprint
		if err := tx.Model(&GenerationRecord{}).Where("id = ?", record.ID).Update("request_fingerprint", fingerprint).Error; err != nil {
			return err
		}
		queueJob = ImageGenerationJob{
			GenerationRecordID:   record.ID,
			UserID:               job.User.ID,
			EntryPoint:           fallbackString(strings.TrimSpace(entryPoint), "workspace"),
			Priority:             0,
			RequestVersion:       generationRequestSnapshotVersion,
			RequestSnapshotJSON:  snapshotJSON,
			RequestFingerprint:   fingerprint,
			IdempotencyKey:       idempotencyKey,
			IdempotencyExpiresAt: now.Add(30 * 24 * time.Hour),
			Status:               ImageGenerationJobStatusQueued,
			Stage:                "waiting_capacity",
			MaxAttempts:          imageQueueMaxExternalAttempts,
			QueueDeadlineAt:      now.Add(a.generationQueueTimeout()),
			ReservedCredits:      credits,
			QueuedAt:             now,
		}
		if err := tx.Create(&queueJob).Error; err != nil {
			return err
		}
		var balance CreditBalance
		if err := tx.Where("user_id = ?", job.User.ID).First(&balance).Error; err != nil {
			return err
		}
		return tx.Create(&CreditTransaction{
			UserID: job.User.ID, Type: CreditTransactionTypeGenerationReserve, Amount: -credits,
			BalanceAfter: balance.AvailableCredits, ReservedAfter: balance.ReservedCredits,
			IdempotencyKey: fmt.Sprintf("generation:%d:reserve", queueJob.ID),
			Reason:         "图片生成预留点数", RelatedType: "image_generation_job", RelatedID: queueJob.ID,
		}).Error
	})
	if err == nil && !reused {
		a.logGenerationEvent(record.ID, generationEventLevelInfo, GenerationStageQueued, "job_queued", "图片生成任务已进入持久化队列", map[string]any{
			"job_id": queueJob.ID, "entry_point": queueJob.EntryPoint, "reserved_credits": queueJob.ReservedCredits,
		})
	}
	return record, queueJob, reused, err
}

func (a *App) writeGenerationEnqueueError(c *gin.Context, job *generationJob, err error) {
	switch {
	case errors.Is(err, errGenerationQueueFull):
		c.Header("Retry-After", "15")
		writeError(c, http.StatusTooManyRequests, "generation_queue_full", "图片生成队列已满，请稍后重试")
	case errors.Is(err, errGenerationIdempotency):
		writeError(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key 已用于不同的生成请求")
	case errors.Is(err, errCreditsInsufficient):
		estimate, estimateErr := a.buildCreditEstimate(job.User.ID, generationJobRequiredCredits(job))
		if estimateErr != nil {
			writeError(c, http.StatusConflict, "credits_insufficient", "点数不足，请先充值")
			return
		}
		writeCreditsInsufficientError(c, estimate)
	default:
		writeError(c, http.StatusInternalServerError, "generation_enqueue_failed", "图片生成任务入队失败")
	}
}

func (a *App) writeQueuedGenerationAccepted(c *gin.Context, record GenerationRecord, userID uint) {
	a.hydrateGenerationQueueProjection(&record)
	balance, _ := a.lookupBalance(userID)
	writeJSON(c, http.StatusAccepted, generationPayload(record, balance.AvailableCredits))
}

func (a *App) handleCreateAsyncGeneration(c *gin.Context) {
	job, ok := a.prepareGenerationJob(c)
	if !ok {
		return
	}
	record, _, _, err := a.enqueueGenerationJob(job, c.GetHeader("Idempotency-Key"), "workspace_async")
	if err != nil {
		a.writeGenerationEnqueueError(c, job, err)
		return
	}
	a.writeQueuedGenerationAccepted(c, record, job.User.ID)
}

func (a *App) handleGenerateImage(c *gin.Context) {
	job, ok := a.prepareGenerationJob(c)
	if !ok {
		return
	}
	record, _, _, err := a.enqueueGenerationJob(job, c.GetHeader("Idempotency-Key"), "workspace_sync")
	if err != nil {
		a.writeGenerationEnqueueError(c, job, err)
		return
	}
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := a.db.First(&record, record.ID).Error; err == nil && !isActiveGenerationRecordStatus(record.Status) {
			a.hydrateGenerationQueueProjection(&record)
			if record.Status == GenerationStatusFailed {
				providerErr := &ProviderError{
					HTTPStatus: record.ProviderHTTPStatus, Code: fallbackString(record.ProviderErrorCode, record.ErrorCode),
					Message: fallbackString(record.ProviderErrorMessage, record.ErrorMessage), ProviderRequestID: record.ProviderRequestID,
					FailureStage: record.ProviderFailureStage, AttemptCount: record.ProviderAttemptCount,
				}
				status, code, message := mapProviderError(providerErr)
				writeError(c, status, code, message)
				return
			}
			balance, _ := a.lookupBalance(job.User.ID)
			writeJSON(c, http.StatusOK, generationPayload(record, balance.AvailableCredits))
			return
		}
		select {
		case <-c.Request.Context().Done():
			a.writeQueuedGenerationAccepted(c, record, job.User.ID)
			return
		case <-deadline.C:
			a.writeQueuedGenerationAccepted(c, record, job.User.ID)
			return
		case <-ticker.C:
		}
	}
}

func (a *App) hydrateGenerationQueueProjection(record *GenerationRecord) {
	if record == nil || record.ID == 0 {
		return
	}
	queueJob, ok := a.generationQueueProjection(record.ID)
	if !ok {
		return
	}
	record.QueuePosition = a.queuePosition(queueJob)
	if !queueJob.QueuedAt.IsZero() {
		end := time.Now().UTC()
		if queueJob.StartedAt != nil {
			end = *queueJob.StartedAt
		}
		record.QueueWaitMS = end.Sub(queueJob.QueuedAt).Milliseconds()
		if record.QueueWaitMS < 0 {
			record.QueueWaitMS = 0
		}
	}
	record.ExecutionAttemptCount = queueJob.AttemptCount
	record.NextAttemptAt = queueJob.NextAttemptAt
	if isActiveGenerationRecordStatus(record.Status) && strings.TrimSpace(queueJob.Stage) != "" {
		record.Stage = queueJob.Stage
	}
}

func settleQueuedGenerationCredits(tx *gorm.DB, userID, recordID uint, amount int) (int, error) {
	var queueJob ImageGenerationJob
	if err := tx.Where("generation_record_id = ? AND user_id = ?", recordID, userID).First(&queueJob).Error; err != nil {
		return 0, err
	}
	if queueJob.CreditsSettled {
		var balance CreditBalance
		err := tx.Where("user_id = ?", userID).First(&balance).Error
		return balance.AvailableCredits, err
	}
	result := tx.Model(&CreditBalance{}).Where("user_id = ? AND reserved_credits >= ?", userID, amount).
		UpdateColumn("reserved_credits", gorm.Expr("reserved_credits - ?", amount))
	if result.Error != nil || result.RowsAffected == 0 {
		if result.Error != nil {
			return 0, result.Error
		}
		return 0, errors.New("generation reserved credit invariant violated")
	}
	if err := tx.Model(&ImageGenerationJob{}).Where("id = ? AND credits_settled = ?", queueJob.ID, false).
		Updates(map[string]any{"credits_settled": true, "reserved_credits": 0}).Error; err != nil {
		return 0, err
	}
	var balance CreditBalance
	if err := tx.Where("user_id = ?", userID).First(&balance).Error; err != nil {
		return 0, err
	}
	if err := tx.Create(&CreditTransaction{
		UserID: userID, Type: CreditTransactionTypeGenerationSettle, Amount: 0,
		BalanceAfter: balance.AvailableCredits, ReservedAfter: balance.ReservedCredits,
		IdempotencyKey: fmt.Sprintf("generation:%d:settle", queueJob.ID), Reason: "图片生成成功结算预留点数",
		RelatedType: "image_generation_job", RelatedID: queueJob.ID,
	}).Error; err != nil {
		return 0, err
	}
	return balance.AvailableCredits, nil
}

func releaseQueuedGenerationCredits(tx *gorm.DB, queueJob *ImageGenerationJob, reason string) error {
	if queueJob == nil || queueJob.CreditsReleased || queueJob.CreditsSettled || queueJob.ReservedCredits <= 0 {
		return nil
	}
	amount := queueJob.ReservedCredits
	result := tx.Model(&CreditBalance{}).Where("user_id = ? AND reserved_credits >= ?", queueJob.UserID, amount).
		Updates(map[string]any{
			"available_credits": gorm.Expr("available_credits + ?", amount),
			"reserved_credits":  gorm.Expr("reserved_credits - ?", amount),
		})
	if result.Error != nil || result.RowsAffected == 0 {
		if result.Error != nil {
			return result.Error
		}
		return errors.New("generation reserved credit release invariant violated")
	}
	if err := tx.Model(&ImageGenerationJob{}).Where("id = ? AND credits_released = ? AND credits_settled = ?", queueJob.ID, false, false).
		Updates(map[string]any{"credits_released": true, "reserved_credits": 0}).Error; err != nil {
		return err
	}
	var balance CreditBalance
	if err := tx.Where("user_id = ?", queueJob.UserID).First(&balance).Error; err != nil {
		return err
	}
	return tx.Create(&CreditTransaction{
		UserID: queueJob.UserID, Type: CreditTransactionTypeGenerationRelease, Amount: amount,
		BalanceAfter: balance.AvailableCredits, ReservedAfter: balance.ReservedCredits,
		IdempotencyKey: fmt.Sprintf("generation:%d:release", queueJob.ID), Reason: reason,
		RelatedType: "image_generation_job", RelatedID: queueJob.ID,
	}).Error
}

func (a *App) startImageGenerationQueueWorker() {
	if a == nil || a.cleanupStop == nil || a.imageQueueWorkerDone == nil {
		return
	}
	if !a.db.Migrator().HasTable(&ImageGenerationJob{}) || !a.db.Migrator().HasTable(&ImageExecutionLease{}) {
		close(a.imageQueueWorkerDone)
		return
	}
	owner := fmt.Sprintf("%s-%d", uuid.NewString(), os.Getpid())
	go func() {
		defer close(a.imageQueueWorkerDone)
		ticker := time.NewTicker(imageQueuePollInterval)
		defer ticker.Stop()
		var workers sync.WaitGroup
		for {
			select {
			case <-ticker.C:
				_ = a.expireQueuedGenerationJobs(time.Now().UTC())
				for i := 0; i < a.currentGenerationConcurrencyLimit(); i++ {
					queueJob, token, ok := a.claimGenerationQueueJob(owner, time.Now().UTC())
					if !ok {
						break
					}
					job, err := a.loadGenerationJobSnapshot(queueJob)
					if err != nil {
						_ = a.failClaimedGenerationJob(queueJob, token, "generation_snapshot_invalid", err.Error())
						continue
					}
					meta := generationExecutionLeaseMeta{JobID: &queueJob.ID, RecordID: &queueJob.GenerationRecordID, UserID: queueJob.UserID, ProviderID: generationJobProviderID(job), ChannelID: generationJobChannelID(job), EntryPoint: queueJob.EntryPoint}
					executionToken, ok := a.tryAcquireImageExecutionLease(owner, meta, time.Now().UTC())
					if !ok {
						_ = a.returnClaimedGenerationJobToQueue(queueJob.ID, token)
						break
					}
					workers.Add(1)
					go func(q ImageGenerationJob, prepared *generationJob, claimToken, leaseToken string) {
						defer workers.Done()
						a.runQueuedGenerationJob(q, prepared, claimToken, leaseToken)
					}(queueJob, job, token, executionToken)
				}
			case <-a.cleanupStop:
				workers.Wait()
				return
			}
		}
	}()
}

func (a *App) currentGenerationConcurrencyLimit() int {
	settings, err := a.loadSettings()
	if err == nil && settings.GenerationConcurrencyLimit > 0 {
		return settings.GenerationConcurrencyLimit
	}
	return 4
}

func generationJobProviderID(job *generationJob) uint {
	if candidate := selectedModelCenterCandidate(job); candidate != nil {
		return candidate.Provider.ID
	}
	return 0
}

func generationJobChannelID(job *generationJob) uint {
	if candidate := selectedModelCenterCandidate(job); candidate != nil {
		return candidate.Channel.ID
	}
	return 0
}

func (a *App) providerExecutionLimit(providerID uint, global int) int {
	if providerID == 0 {
		return global
	}
	var provider ModelProvider
	if err := a.db.Select("id, concurrency_limit").First(&provider, providerID).Error; err != nil || provider.ConcurrencyLimit <= 0 {
		return global
	}
	if provider.ConcurrencyLimit < global {
		return provider.ConcurrencyLimit
	}
	return global
}

func (a *App) tryAcquireImageExecutionLease(owner string, meta generationExecutionLeaseMeta, now time.Time) (string, bool) {
	token := uuid.NewString()
	globalLimit := a.currentGenerationConcurrencyLimit()
	providerLimit := a.providerExecutionLimit(meta.ProviderID, globalLimit)
	err := a.db.Transaction(func(tx *gorm.DB) error {
		// PostgreSQL 的 READ COMMITTED 不能让 count + insert 自动成为容量 CAS；
		// 事务级 advisory lock 将所有实例的槽位检查和创建串行化。
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", imageExecutionLeaseAdvisoryLock).Error; err != nil {
				return err
			}
		}
		var expired []ImageExecutionLease
		if err := tx.Where("expires_at <= ?", now).Find(&expired).Error; err != nil {
			return err
		}
		if err := tx.Where("expires_at <= ?", now).Delete(&ImageExecutionLease{}).Error; err != nil {
			return err
		}
		for _, lease := range expired {
			if lease.RecordID != nil {
				metadata, _ := json.Marshal(map[string]any{"job_id": lease.JobID, "entry_point": lease.EntryPoint})
				if err := tx.Create(&GenerationEventLog{GenerationRecordID: *lease.RecordID, TraceID: generationTraceID(*lease.RecordID), Level: generationEventLevelWarn, Stage: "recovering", Event: "lease_expired", Message: "图片执行租约已过期", MetadataJSON: string(metadata), CreatedAt: now}).Error; err != nil {
					return err
				}
			}
		}
		var active int64
		if err := tx.Model(&ImageExecutionLease{}).Where("expires_at > ?", now).Count(&active).Error; err != nil {
			return err
		}
		if active >= int64(globalLimit) {
			return gorm.ErrInvalidTransaction
		}
		if meta.ProviderID != 0 {
			var providerActive int64
			if err := tx.Model(&ImageExecutionLease{}).Where("provider_id = ? AND expires_at > ?", meta.ProviderID, now).Count(&providerActive).Error; err != nil {
				return err
			}
			if providerActive >= int64(providerLimit) {
				return gorm.ErrInvalidTransaction
			}
		}
		return tx.Create(&ImageExecutionLease{
			Token: token, Owner: owner, JobID: meta.JobID, RecordID: meta.RecordID, UserID: meta.UserID,
			ProviderID: meta.ProviderID, ChannelID: meta.ChannelID, EntryPoint: meta.EntryPoint, ExpiresAt: now.Add(imageExecutionLeaseTTL),
		}).Error
	})
	return token, err == nil
}

func (a *App) acquireImageExecutionLease(ctx context.Context, owner string, meta generationExecutionLeaseMeta) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if token, ok := a.tryAcquireImageExecutionLease(owner, meta, time.Now().UTC()); ok {
			return token, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-a.cleanupStop:
			return "", context.Canceled
		case <-ticker.C:
		}
	}
}

func (a *App) renewImageExecutionLease(token string, stop <-chan struct{}) {
	if token == "" {
		return
	}
	ticker := time.NewTicker(imageExecutionLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = a.db.Model(&ImageExecutionLease{}).Where("token = ?", token).Update("expires_at", time.Now().UTC().Add(imageExecutionLeaseTTL)).Error
		case <-stop:
			return
		}
	}
}

func (a *App) renewGenerationQueueLeaseOnce(jobID uint, token string, now time.Time) bool {
	if jobID == 0 || strings.TrimSpace(token) == "" {
		return false
	}
	result := a.db.Model(&ImageGenerationJob{}).
		Where("id = ? AND lease_token = ? AND status IN ?", jobID, token, []string{ImageGenerationJobStatusRunning, ImageGenerationJobStatusPersisting}).
		Update("lease_expires_at", now.Add(imageExecutionLeaseTTL))
	return result.Error == nil && result.RowsAffected == 1
}

func (a *App) renewGenerationQueueLease(jobID uint, token string, stop <-chan struct{}) {
	if jobID == 0 || strings.TrimSpace(token) == "" {
		return
	}
	ticker := time.NewTicker(imageExecutionLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.renewGenerationQueueLeaseOnce(jobID, token, time.Now().UTC())
		case <-stop:
			return
		}
	}
}

func (a *App) releaseImageExecutionLease(token string) {
	if token != "" {
		_ = a.db.Where("token = ?", token).Delete(&ImageExecutionLease{}).Error
	}
}

func selectFairGenerationCandidate(candidates []ImageGenerationJob, activeByUser map[uint]int64) (ImageGenerationJob, bool) {
	if len(candidates) == 0 {
		return ImageGenerationJob{}, false
	}
	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Priority != selected.Priority {
			break
		}
		if activeByUser[candidate.UserID] < activeByUser[selected.UserID] {
			selected = candidate
		}
	}
	return selected, true
}

func (a *App) claimGenerationQueueJob(owner string, now time.Time) (ImageGenerationJob, string, bool) {
	var selected ImageGenerationJob
	token := uuid.NewString()
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var candidates []ImageGenerationJob
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", []string{ImageGenerationJobStatusQueued, ImageGenerationJobStatusRetryWait}).
			Where("cancel_requested_at IS NULL AND queue_deadline_at > ?", now).
			Where("next_attempt_at IS NULL OR next_attempt_at <= ?", now).
			Order("priority DESC, queued_at ASC, id ASC").Limit(a.generationQueueCapacity())
		if err := query.Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return gorm.ErrRecordNotFound
		}
		activeByUser := map[uint]int64{}
		for _, candidate := range candidates {
			if _, found := activeByUser[candidate.UserID]; found {
				continue
			}
			var count int64
			if err := tx.Model(&ImageGenerationJob{}).Where("user_id = ? AND status IN ?", candidate.UserID, []string{ImageGenerationJobStatusRunning, ImageGenerationJobStatusPersisting}).Count(&count).Error; err != nil {
				return err
			}
			activeByUser[candidate.UserID] = count
		}
		selected, _ = selectFairGenerationCandidate(candidates, activeByUser)
		leaseUntil := now.Add(imageExecutionLeaseTTL)
		result := tx.Model(&ImageGenerationJob{}).
			Where("id = ? AND status IN ?", selected.ID, []string{ImageGenerationJobStatusQueued, ImageGenerationJobStatusRetryWait}).
			Updates(map[string]any{
				"status": ImageGenerationJobStatusRunning, "stage": "recovering", "lease_owner": owner,
				"lease_token": token, "lease_expires_at": leaseUntil, "claimed_at": now, "started_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		selected.LeaseToken, selected.LeaseOwner, selected.LeaseExpiresAt = token, owner, &leaseUntil
		return nil
	})
	return selected, token, err == nil
}

func (a *App) returnClaimedGenerationJobToQueue(jobID uint, token string) error {
	return a.db.Model(&ImageGenerationJob{}).Where("id = ? AND lease_token = ? AND status = ?", jobID, token, ImageGenerationJobStatusRunning).
		Updates(map[string]any{"status": ImageGenerationJobStatusQueued, "stage": "waiting_capacity", "lease_owner": "", "lease_token": "", "lease_expires_at": nil}).Error
}

func (a *App) loadGenerationJobSnapshot(queueJob ImageGenerationJob) (*generationJob, error) {
	var snapshot generationQueueSnapshot
	if err := json.Unmarshal([]byte(queueJob.RequestSnapshotJSON), &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != generationRequestSnapshotVersion {
		return nil, fmt.Errorf("unsupported generation request snapshot version %d", snapshot.Version)
	}
	request := snapshot.Request
	var user User
	if err := a.db.First(&user, queueJob.UserID).Error; err != nil {
		return nil, err
	}
	settings, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	var referenceAssets []ReferenceAsset
	if len(request.ReferenceAssetIDs) > 0 {
		if err := a.db.Where("user_id = ? AND id IN ?", user.ID, request.ReferenceAssetIDs).Find(&referenceAssets).Error; err != nil {
			return nil, err
		}
		if len(referenceAssets) != len(request.ReferenceAssetIDs) {
			return nil, errors.New("reference asset snapshot no longer belongs to user")
		}
		referenceAssets = orderReferenceAssets(referenceAssets, request.ReferenceAssetIDs)
	}
	var referenceWorks []Work
	if len(request.ReferenceWorkIDs) > 0 {
		if err := a.db.Where("user_id = ? AND id IN ?", user.ID, request.ReferenceWorkIDs).Find(&referenceWorks).Error; err != nil {
			return nil, err
		}
		if len(referenceWorks) != len(request.ReferenceWorkIDs) {
			return nil, errors.New("reference work snapshot no longer belongs to user")
		}
	}
	var sourceWork *Work
	if request.SourceWorkID != nil {
		var work Work
		if err := a.db.Where("user_id = ? AND id = ?", user.ID, *request.SourceWorkID).First(&work).Error; err != nil {
			return nil, err
		}
		sourceWork = &work
	}
	modelCenterCandidates, err := a.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, request.ModelID)
	if err != nil {
		return nil, err
	}
	prepared := &generationJob{User: user, Settings: settings, Request: request, SourceWork: sourceWork, ReferenceAssets: referenceAssets, ReferenceWorks: referenceWorks, ModelCenterCandidates: modelCenterCandidates}
	if len(modelCenterCandidates) > 0 {
		prepared.ModelCenterModel = &modelCenterCandidates[0].Model
		prepared.ModelCenterChannel = &modelCenterCandidates[0].Channel
	} else {
		prepared.ModelCandidates, err = a.modelConfigCandidatesForGeneration(settings)
		if err != nil {
			return nil, err
		}
		if len(prepared.ModelCandidates) > 0 {
			prepared.ModelConfig = &prepared.ModelCandidates[0]
		} else {
			prepared.ModelConfig, err = a.modelConfigForGeneration(settings)
			if err != nil {
				return nil, err
			}
		}
	}
	return prepared, nil
}

func orderReferenceAssets(found []ReferenceAsset, ids []uint) []ReferenceAsset {
	byID := make(map[uint]ReferenceAsset, len(found))
	for _, asset := range found {
		byID[asset.ID] = asset
	}
	ordered := make([]ReferenceAsset, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, byID[id])
	}
	return ordered
}

func (a *App) runQueuedGenerationJob(queueJob ImageGenerationJob, job *generationJob, claimToken, executionToken string) {
	stopRenew := make(chan struct{})
	go a.renewImageExecutionLease(executionToken, stopRenew)
	go a.renewGenerationQueueLease(queueJob.ID, claimToken, stopRenew)
	defer close(stopRenew)
	defer a.releaseImageExecutionLease(executionToken)

	var record GenerationRecord
	if err := a.db.First(&record, queueJob.GenerationRecordID).Error; err != nil {
		_ = a.failClaimedGenerationJob(queueJob, claimToken, "generation_record_missing", err.Error())
		return
	}
	if strings.TrimSpace(queueJob.SpoolPath) != "" {
		if err := a.resumeQueuedGenerationSpool(&record, &queueJob, job, claimToken); err == nil {
			a.logGenerationEvent(record.ID, generationEventLevelInfo, "persisting", "result_resumed", "已从完整临时结果继续持久化", map[string]any{"job_id": queueJob.ID})
			return
		} else {
			if generationSpoolPersistenceExpired(queueJob.SpoolPath, time.Now().UTC()) {
				_ = a.failClaimedGenerationJob(queueJob, claimToken, "generation_persist_timeout", "生成结果持久化超过5分钟")
				return
			}
			next := time.Now().UTC().Add(15 * time.Second)
			_ = a.db.Model(&ImageGenerationJob{}).Where("id = ? AND lease_token = ?", queueJob.ID, claimToken).Updates(map[string]any{
				"status": ImageGenerationJobStatusRetryWait, "stage": "persisting", "next_attempt_at": next,
				"lease_owner": "", "lease_token": "", "lease_expires_at": nil, "error_code": "generation_persist_failed", "error_message": err.Error(),
			}).Error
			return
		}
	}
	a.logGenerationEvent(record.ID, generationEventLevelInfo, "waiting_capacity", "job_claimed", "图片生成任务已取得执行槽位", map[string]any{"job_id": queueJob.ID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-a.cleanupStop:
			cancel()
		case <-ctx.Done():
		}
	}()
	result, providerErr, err := a.executeGenerationRecordWithOptions(&record, job, generationExecutionOptions{
		Context: ctx, BillingMode: generationBillingQueueReservation,
		IdempotencyKey: queueJob.IdempotencyKey, DeferProviderFailure: true, ExecutionLeaseToken: executionToken,
	})
	if providerErr != nil {
		_ = a.db.Model(&GenerationRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
			"model_id": record.ModelID, "channel_id": record.ChannelID, "model_name": record.ModelName,
			"channel_name": record.ChannelName, "runtime_model": record.RuntimeModel, "model_config_id": record.ModelConfigID, "model": record.Model,
			"latency_ms": record.LatencyMS, "provider_request_id": record.ProviderRequestID, "provider_http_status": record.ProviderHTTPStatus,
			"provider_error_code": record.ProviderErrorCode, "provider_error_message": record.ProviderErrorMessage,
			"provider_failure_stage": record.ProviderFailureStage, "provider_attempt_count": record.ProviderAttemptCount,
			"provider_request_started": record.ProviderRequestStarted, "provider_idempotency_supported": record.ProviderIdempotencySupported,
		}).Error
	}
	var attempts int64
	_ = a.db.Model(&ModelCallAttempt{}).Where("generation_record_id = ?", record.ID).Count(&attempts).Error
	if providerErr != nil && queuedProviderFailureRetryable(providerErr) && int(attempts) < imageQueueMaxExternalAttempts {
		delay := 15 * time.Second
		if attempts >= 2 {
			delay = 60 * time.Second
		}
		if providerErr.RetryAfter > 0 {
			delay = providerErr.RetryAfter
		}
		jitter := 0.8 + rand.Float64()*0.4
		if providerErr.RetryAfter > 0 {
			jitter = 1
		}
		next := time.Now().UTC().Add(time.Duration(float64(delay) * jitter))
		updates := map[string]any{
			"status": ImageGenerationJobStatusRetryWait, "stage": "retry_wait", "attempt_count": attempts,
			"next_attempt_at": next, "lease_owner": "", "lease_token": "", "lease_expires_at": nil,
			"provider_request_started": record.ProviderRequestStarted, "provider_idempotency_supported": record.ProviderIdempotencySupported,
			"error_code": providerErr.Code, "error_message": providerErr.Message,
		}
		if result := a.db.Model(&ImageGenerationJob{}).Where("id = ? AND lease_token = ?", queueJob.ID, claimToken).Updates(updates); result.Error == nil && result.RowsAffected == 1 {
			_ = a.db.Model(&GenerationRecord{}).Where("id = ? AND status IN ?", record.ID, []string{GenerationStatusQueued, GenerationStatusRunning}).Updates(map[string]any{"status": GenerationStatusQueued, "stage": "retry_wait", "error_code": "", "error_message": ""}).Error
			a.logGenerationEvent(record.ID, generationEventLevelWarn, "retry_wait", "retry_scheduled", "供应商调用将在退避后重试", map[string]any{"job_id": queueJob.ID, "next_attempt_at": next, "attempt_count": attempts})
			return
		}
	}
	if err != nil && errors.Is(err, errGenerationRecordAlreadyTerminal) {
		return
	}
	if err != nil {
		var current ImageGenerationJob
		if loadErr := a.db.First(&current, queueJob.ID).Error; loadErr == nil && strings.TrimSpace(current.SpoolPath) != "" {
			if generationSpoolPersistenceExpired(current.SpoolPath, time.Now().UTC()) {
				_ = a.failClaimedGenerationJob(current, claimToken, "generation_persist_timeout", "生成结果持久化超过5分钟")
				return
			}
			next := time.Now().UTC().Add(15 * time.Second)
			if update := a.db.Model(&ImageGenerationJob{}).Where("id = ? AND lease_token = ?", current.ID, claimToken).Updates(map[string]any{
				"status": ImageGenerationJobStatusRetryWait, "stage": "persisting", "next_attempt_at": next,
				"lease_owner": "", "lease_token": "", "lease_expires_at": nil, "error_code": "generation_persist_failed", "error_message": err.Error(),
			}); update.Error == nil && update.RowsAffected == 1 {
				_ = a.db.Model(&GenerationRecord{}).Where("id = ? AND status IN ?", record.ID, []string{GenerationStatusQueued, GenerationStatusRunning}).Updates(map[string]any{"status": GenerationStatusQueued, "stage": "persisting", "error_code": "", "error_message": ""}).Error
				return
			}
		}
	}
	if result != nil && record.Status == GenerationStatusSucceeded {
		now := time.Now().UTC()
		_ = a.db.Model(&ImageGenerationJob{}).Where("id = ? AND lease_token = ?", queueJob.ID, claimToken).Updates(map[string]any{
			"status": ImageGenerationJobStatusSucceeded, "stage": GenerationStageSucceeded, "attempt_count": attempts,
			"completed_at": now, "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "credits_settled": true, "reserved_credits": 0,
		}).Error
		return
	}
	code := strings.TrimSpace(record.ErrorCode)
	message := strings.TrimSpace(record.ErrorMessage)
	if providerErr != nil {
		code, message, _ = publicProviderFailure(providerErr)
	}
	if code == "" {
		code = "generation_failed"
	}
	if message == "" {
		message = "图片生成任务失败"
	}
	_ = a.failClaimedGenerationJob(queueJob, claimToken, code, message)
}

func generationSpoolPersistenceExpired(path string, now time.Time) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return true
	}
	return now.Sub(info.ModTime()) >= imageGenerationPersistenceLimit
}

func (a *App) resumeQueuedGenerationSpool(record *GenerationRecord, queueJob *ImageGenerationJob, job *generationJob, claimToken string) error {
	file, err := os.Open(queueJob.SpoolPath)
	if err != nil {
		return err
	}
	assetKey, mimeType, err := a.assetStore.SaveStream(file, fallbackString(record.MIMEType, "image/png"))
	_ = file.Close()
	if err != nil {
		return err
	}
	var work Work
	err = a.db.Transaction(func(tx *gorm.DB) error {
		var current ImageGenerationJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND lease_token = ?", queueJob.ID, claimToken).First(&current).Error; err != nil {
			return errGenerationQueueLeaseLost
		}
		work = Work{
			UserID: job.User.ID, GenerationRecordID: record.ID, Prompt: job.Request.Prompt, AspectRatio: job.Request.AspectRatio,
			BatchID: job.Request.BatchID, BatchIndex: job.Request.BatchIndex, BatchTotal: job.Request.BatchTotal,
			Category: WorkCategoryImage, Model: record.Model, Status: GenerationStatusSucceeded, Visibility: WorkVisibilityPrivate,
			AssetKey: assetKey, MIMEType: mimeType, ProviderRequestID: record.ProviderRequestID,
		}
		if err := tx.Create(&work).Error; err != nil {
			return err
		}
		if publicURL := a.assetStore.PublicURL(assetKey); publicURL != "" {
			work.PreviewURL, work.DownloadURL = publicURL, publicURL
		} else {
			work.PreviewURL = fmt.Sprintf("/api/works/%d/file", work.ID)
			work.DownloadURL = fmt.Sprintf("/api/works/%d/download", work.ID)
		}
		if err := tx.Save(&work).Error; err != nil {
			return err
		}
		if _, err := settleQueuedGenerationCredits(tx, job.User.ID, record.ID, generationRecordCreditCost(*record)); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&GenerationRecord{}).Where("id = ? AND status IN ?", record.ID, []string{GenerationStatusQueued, GenerationStatusRunning}).Updates(map[string]any{
			"work_id": work.ID, "status": GenerationStatusSucceeded, "stage": GenerationStageSucceeded,
			"asset_key": assetKey, "preview_url": work.PreviewURL, "download_url": work.DownloadURL, "mime_type": mimeType, "credits_deducted": true,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&ImageGenerationJob{}).Where("id = ? AND lease_token = ?", queueJob.ID, claimToken).Updates(map[string]any{
			"status": ImageGenerationJobStatusSucceeded, "stage": GenerationStageSucceeded, "completed_at": now,
			"spool_path": "", "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "credits_settled": true, "reserved_credits": 0,
		}).Error
	})
	if err != nil {
		_ = a.assetStore.Delete(assetKey)
		return err
	}
	_ = os.Remove(queueJob.SpoolPath)
	return nil
}

func queuedProviderFailureRetryable(providerErr *ProviderError) bool {
	if providerErr == nil || strings.TrimSpace(providerErr.Code) == "provider_result_unknown" {
		return false
	}
	if providerErr.RequestNotSent {
		return true
	}
	if providerErr.HTTPStatus == 429 || providerErr.HTTPStatus == 502 || providerErr.HTTPStatus == 503 || providerErr.HTTPStatus == 504 {
		return true
	}
	status, ok := providerHTTPStatusFromErrorCode(providerErr.Code)
	return ok && (status == 429 || status == 502 || status == 503 || status == 504)
}

func queuedProviderFailureWaits(providerErr *ProviderError) bool {
	if providerErr == nil {
		return false
	}
	if providerErr.RequestNotSent || providerErr.HTTPStatus == http.StatusTooManyRequests {
		return true
	}
	status, ok := providerHTTPStatusFromErrorCode(providerErr.Code)
	return ok && status == http.StatusTooManyRequests
}

func (a *App) failClaimedGenerationJob(queueJob ImageGenerationJob, token, code, message string) error {
	now := time.Now().UTC()
	spoolPath := ""
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var current ImageGenerationJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, queueJob.ID).Error; err != nil {
			return err
		}
		if current.Status != ImageGenerationJobStatusQueued && current.Status != ImageGenerationJobStatusRetryWait && current.Status != ImageGenerationJobStatusRunning && current.Status != ImageGenerationJobStatusPersisting {
			return errGenerationQueueLeaseLost
		}
		if token != "" && current.LeaseToken != token {
			return errGenerationQueueLeaseLost
		}
		spoolPath = current.SpoolPath
		if err := releaseQueuedGenerationCredits(tx, &current, code); err != nil {
			return err
		}
		status := ImageGenerationJobStatusFailed
		if code == imageGenerationCancelledErrorCode {
			status = ImageGenerationJobStatusCancelled
		}
		if err := tx.Model(&ImageGenerationJob{}).Where("id = ?", current.ID).Updates(map[string]any{
			"status": status, "stage": GenerationStageFailed, "error_code": code, "error_message": message,
			"completed_at": now, "lease_owner": "", "lease_token": "", "lease_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&GenerationRecord{}).Where("id = ? AND status IN ?", current.GenerationRecordID, []string{GenerationStatusQueued, GenerationStatusRunning}).Updates(map[string]any{
			"status": GenerationStatusFailed, "stage": GenerationStageFailed, "error_code": code, "error_message": message, "credits_deducted": false,
		}).Error
	})
	if err == nil && code == imageGenerationCancelledErrorCode {
		a.logGenerationEvent(queueJob.GenerationRecordID, generationEventLevelInfo, GenerationStageFailed, "job_cancelled", "用户取消图片生成队列任务", map[string]any{"job_id": queueJob.ID})
	}
	if err == nil && strings.TrimSpace(spoolPath) != "" {
		_ = os.Remove(spoolPath)
	}
	return err
}

func (a *App) expireQueuedGenerationJobs(now time.Time) error {
	var jobs []ImageGenerationJob
	if err := a.db.Where("status IN ? AND queue_deadline_at <= ?", []string{ImageGenerationJobStatusQueued, ImageGenerationJobStatusRetryWait}, now).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		if err := a.failClaimedGenerationJob(job, "", "queue_timeout", "图片生成排队超过30分钟，已自动取消"); err != nil && !errors.Is(err, errGenerationQueueLeaseLost) {
			return err
		}
	}
	return nil
}

func (a *App) recoverGenerationQueue(now time.Time) error {
	if !a.db.Migrator().HasTable(&ImageGenerationJob{}) {
		return nil
	}
	var jobs []ImageGenerationJob
	if err := a.db.Where("status IN ?", []string{ImageGenerationJobStatusRunning, ImageGenerationJobStatusPersisting}).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		if job.LeaseExpiresAt != nil && job.LeaseExpiresAt.After(now) {
			continue
		}
		var record GenerationRecord
		if err := a.db.First(&record, job.GenerationRecordID).Error; err != nil {
			return err
		}
		if record.Status == GenerationStatusSucceeded {
			if err := a.db.Model(&ImageGenerationJob{}).Where("id = ? AND status IN ?", job.ID, []string{ImageGenerationJobStatusRunning, ImageGenerationJobStatusPersisting}).Updates(map[string]any{
				"status": ImageGenerationJobStatusSucceeded, "stage": GenerationStageSucceeded, "completed_at": now,
				"lease_owner": "", "lease_token": "", "lease_expires_at": nil, "spool_path": "", "credits_settled": true, "reserved_credits": 0,
			}).Error; err != nil {
				return err
			}
			if strings.TrimSpace(job.SpoolPath) != "" {
				_ = os.Remove(job.SpoolPath)
			}
			continue
		}
		if !isActiveGenerationRecordStatus(record.Status) {
			if err := a.failClaimedGenerationJob(job, "", fallbackString(record.ErrorCode, "generation_failed"), fallbackString(record.ErrorMessage, "图片生成任务失败")); err != nil && !errors.Is(err, errGenerationQueueLeaseLost) {
				return err
			}
			continue
		}
		a.logGenerationEvent(record.ID, generationEventLevelWarn, "recovering", "lease_expired", "图片生成任务执行租约已过期", map[string]any{"job_id": job.ID, "lease_owner": job.LeaseOwner})
		if record.ProviderRequestStarted && !record.ProviderIdempotencySupported && strings.TrimSpace(job.SpoolPath) == "" {
			if err := a.failClaimedGenerationJob(job, "", "provider_result_unknown", "服务中断时供应商请求结果未知，为避免重复生成已停止重试"); err != nil {
				return err
			}
			continue
		}
		if err := a.db.Model(&ImageGenerationJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status": ImageGenerationJobStatusQueued, "stage": "recovering", "lease_owner": "", "lease_token": "", "lease_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		if err := a.db.Model(&GenerationRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"status": GenerationStatusQueued, "stage": "recovering", "error_code": "", "error_message": ""}).Error; err != nil {
			return err
		}
		a.logGenerationEvent(record.ID, generationEventLevelInfo, "recovering", "job_recovered", "服务重启后任务已恢复到队列", map[string]any{"job_id": job.ID})
	}
	return a.db.Where("expires_at <= ?", now).Delete(&ImageExecutionLease{}).Error
}

func (a *App) generationQueueProjection(recordID uint) (ImageGenerationJob, bool) {
	var job ImageGenerationJob
	err := a.db.Where("generation_record_id = ?", recordID).First(&job).Error
	return job, err == nil
}

func (a *App) queuePosition(job ImageGenerationJob) int64 {
	if job.Status != ImageGenerationJobStatusQueued && job.Status != ImageGenerationJobStatusRetryWait {
		return 0
	}
	var count int64
	_ = a.db.Model(&ImageGenerationJob{}).
		Where("status IN ?", []string{ImageGenerationJobStatusQueued, ImageGenerationJobStatusRetryWait}).
		Where("priority > ? OR (priority = ? AND (queued_at < ? OR (queued_at = ? AND id <= ?)))", job.Priority, job.Priority, job.QueuedAt, job.QueuedAt, job.ID).
		Count(&count).Error
	return count
}

func (a *App) logQueueWorkerError(message string, err error) {
	if err != nil {
		log.Printf("generation_queue %s: %v", message, err)
	}
}
