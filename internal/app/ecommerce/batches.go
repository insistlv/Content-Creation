package ecommerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PricingSnapshotProvider interface {
	SnapshotForEstimate(context.Context, *gorm.DB, uint, uint, RecipeDefinition, EstimateBatchRequest) (PricingSnapshot, error)
}

var (
	ErrSKUDisabled           = errors.New("SKU 已停用，不能创建新批次")
	ErrPrimarySKUNotSelected = errors.New("主 SKU 必须位于已选 SKU 集合中")
	ErrSKUSelectionRequired  = errors.New("至少选择一个 SKU")
	ErrSKUSelectionTooLarge  = errors.New("最多选择 100 个 SKU")
)

func (s *Service) ConfigureBatchInfrastructure(registry *Registry, ledger CreditLedger, snapshots PricingSnapshotStore, provider PricingSnapshotProvider) {
	s.batchRegistry = registry
	s.creditLedger = ledger
	s.pricingSnapshots = snapshots
	s.pricingProvider = provider
}

func (s *Service) EstimateBatch(ctx context.Context, userID, projectID uint, req EstimateBatchRequest) (BatchEstimate, error) {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return BatchEstimate{}, err
	}
	requestDigest, _, err := canonicalBatchRequestDigest(req)
	if err != nil {
		return BatchEstimate{}, err
	}
	project, compileInput, definition, err := s.batchCompileInput(ctx, s.repository.db, userID, projectID, req)
	if err != nil {
		return BatchEstimate{}, err
	}
	now := s.now().UTC()
	snapshot := PricingSnapshot{
		UserID: userID, ProjectID: projectID, RequestDigest: requestDigest,
		Status: "issued", CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}
	if s.pricingProvider != nil {
		snapshot, err = s.pricingProvider.SnapshotForEstimate(ctx, s.repository.db, userID, projectID, definition, req)
		if err != nil {
			return BatchEstimate{}, err
		}
		snapshot.UserID, snapshot.ProjectID = userID, projectID
		snapshot.RequestDigest, snapshot.Status = requestDigest, "issued"
		snapshot.CreatedAt, snapshot.ExpiresAt = now, now.Add(15*time.Minute)
	}
	compileInput.PricingSnapshot = snapshot
	items, err := s.batchRegistry.Compile(ctx, compileInput)
	if err != nil {
		return BatchEstimate{}, err
	}
	if len(items) == 0 {
		return BatchEstimate{}, invalidField("output_count", "compiled batch has no items")
	}
	if snapshot.Version == "" {
		snapshot.Version = items[0].PricingVersion
	}
	if snapshot.Version == "" {
		snapshot.Version = "pricing-v1"
	}
	if len(snapshot.Entries) == 0 {
		snapshot.Entries = pricingEntriesFromItems(items, req.QualityTier)
	}
	total := 0
	for index := range items {
		if items[index].EstimatedCredits <= 0 {
			return BatchEstimate{}, invalidField("estimated_credits", "compiled item cost must be positive")
		}
		total += items[index].EstimatedCredits
	}
	var issued PricingSnapshot
	err = s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var issueErr error
		issued, issueErr = s.pricingSnapshots.IssueTx(ctx, tx, snapshot)
		return issueErr
	})
	if err != nil {
		return BatchEstimate{}, err
	}
	for index := range items {
		items[index].PricingSnapshotID = issued.ID
		if items[index].PricingVersion == "" {
			items[index].PricingVersion = issued.Version
		}
	}
	_ = project
	return BatchEstimate{
		Items: items, TotalItems: len(items), EstimatedCredits: total,
		ETASeconds:     commerceCompiledItemsETASeconds(items),
		PricingVersion: issued.Version, PricingSnapshotID: issued.ID,
		PricingExpiresAt: issued.ExpiresAt, RequestDigest: requestDigest,
	}, nil
}

func (s *Service) SubmitBatch(ctx context.Context, userID, projectID uint, key string, req SubmitBatchRequest) (BatchSnapshot, error) {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return BatchSnapshot{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return BatchSnapshot{}, invalidField("idempotency_key", "idempotency key is required")
	}
	digest, requestJSON, err := canonicalBatchRequestDigest(req.EstimateBatchRequest)
	if err != nil {
		return BatchSnapshot{}, err
	}
	var result BatchSnapshot
	err = runProjectMutationTransaction(ctx, s.repository.db, func(tx *gorm.DB) error {
		var replay CommerceGenerationBatch
		replayErr := tx.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&replay).Error
		if replayErr == nil {
			if replay.ProjectID != projectID {
				return ErrIdempotencyConflict
			}
			if err := validateIdempotencyReplay(replay.IdempotencyKey, replay.RequestDigest, key, digest); err != nil {
				return err
			}
			if _, err := lockWritableProjectTx(ctx, tx, userID, replay.ProjectID); err != nil {
				return err
			}
			items, err := loadBatchItems(ctx, tx, userID, replay.ID)
			if err != nil {
				return err
			}
			result = BatchSnapshot{Batch: replay, Items: items}
			return nil
		}
		if !errors.Is(replayErr, gorm.ErrRecordNotFound) {
			return replayErr
		}
		if _, err := lockWritableProjectTx(ctx, tx, userID, projectID); err != nil {
			return err
		}
		batch := CommerceGenerationBatch{
			UserID: userID, ProjectID: projectID, CreativeSpecID: &req.CreativeSpecID,
			PrimarySKUID: req.PrimarySKUID, RecipeKey: req.RecipeKey, RecipeVersion: req.RecipeVersion,
			QualityTier: req.QualityTier, Status: CommerceBatchQueued, IdempotencyKey: key,
			RequestDigest: digest, RequestSnapshotJSON: requestJSON,
		}
		claim := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(&batch)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			var existing CommerceGenerationBatch
			if err := tx.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&existing).Error; err != nil {
				return err
			}
			if existing.ProjectID != projectID {
				return ErrIdempotencyConflict
			}
			if err := validateIdempotencyReplay(existing.IdempotencyKey, existing.RequestDigest, key, digest); err != nil {
				return err
			}
			items, err := loadBatchItems(ctx, tx, userID, existing.ID)
			if err != nil {
				return err
			}
			result = BatchSnapshot{Batch: existing, Items: items}
			return nil
		}
		snapshot, err := s.pricingSnapshots.ResolveForSubmitTx(ctx, tx, userID, projectID, req.PricingSnapshotID, digest, s.now().UTC())
		if err != nil {
			return err
		}
		project, compileInput, definition, err := s.batchCompileInput(ctx, tx, userID, projectID, req.EstimateBatchRequest)
		if err != nil {
			return err
		}
		compileInput.PricingSnapshot = snapshot
		compiledItems, err := s.batchRegistry.Compile(ctx, compileInput)
		if err != nil {
			return err
		}
		total := 0
		for index := range compiledItems {
			if compiledItems[index].EstimatedCredits <= 0 {
				return invalidField("estimated_credits", "compiled item cost must be positive")
			}
			compiledItems[index].PricingSnapshotID = snapshot.ID
			if compiledItems[index].PricingVersion == "" {
				compiledItems[index].PricingVersion = snapshot.Version
			}
			total += compiledItems[index].EstimatedCredits
		}
		batch.Pipeline = project.Pipeline
		batch.PricingVersion = snapshot.Version
		batch.PricingSnapshotID = snapshot.ID
		batch.TotalItems = len(compiledItems)
		batch.QueuedItems = len(compiledItems)
		batch.EstimatedCredits = total
		batch.ReservedCredits = total
		pricingJSON, err := EncodeJSON(snapshot)
		if err != nil {
			return err
		}
		batch.PricingSnapshotJSON = pricingJSON
		if err := tx.Save(&batch).Error; err != nil {
			return err
		}
		reservation, err := s.creditLedger.ReserveTx(ctx, tx, ReserveCreditsRequest{
			UserID: userID, ProjectID: projectID, BatchID: &batch.ID,
			ScopeType: "batch", ScopeKey: fmt.Sprintf("%d", batch.ID), Amount: total,
			IdempotencyKey: "commerce:batch:" + key,
		})
		if err != nil {
			return err
		}
		batch.ReservationID = &reservation.ReservationID
		if err := tx.Model(&CommerceGenerationBatch{}).Where("id = ? AND user_id = ?", batch.ID, userID).
			Update("reservation_id", reservation.ReservationID).Error; err != nil {
			return err
		}
		items := make([]CommerceGenerationItem, 0, len(compiledItems))
		for index, compiled := range compiledItems {
			outputJSON, err := EncodeJSON(compiled)
			if err != nil {
				return err
			}
			item := CommerceGenerationItem{
				UserID: userID, ProjectID: projectID, BatchID: batch.ID, ReservationID: reservation.ReservationID,
				SKUID: compiled.SKUID, Scope: compiled.Scope, SlotKey: compiled.SlotKey, CandidateIndex: index,
				Pipeline: compiled.Pipeline, RecipeKey: compiled.RecipeKey, RecipeVersion: compiled.RecipeVersion,
				QualityTier: req.QualityTier, PricingVersion: compiled.PricingVersion, PricingSnapshotID: snapshot.ID,
				IdempotencyKey: fmt.Sprintf("%s:item:%d", key, index), Status: CommerceItemQueued,
				InputSnapshotJSON: requestJSON, OutputSpecJSON: outputJSON,
				EstimatedCredits: compiled.EstimatedCredits, ReservedCredits: compiled.EstimatedCredits,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			itemID, batchID := item.ID, batch.ID
			job := CommerceJob{
				UserID: userID, ProjectID: projectID, BatchID: &batchID, GenerationItemID: &itemID,
				Kind: CommerceJobKindGenerateItem, Pipeline: compiled.Pipeline, RecipeKey: compiled.RecipeKey,
				Status: CommerceJobQueued, IdempotencyKey: fmt.Sprintf("%s:job:%d", key, index), MaxAttempts: definition.MaxAttempts,
				PayloadJSON: outputJSON,
			}
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			items = append(items, item)
		}
		now := s.now().UTC()
		consumed := tx.Model(&CommercePricingSnapshot{}).
			Where("id = ? AND user_id = ? AND project_id = ? AND status = ? AND expires_at > ?", snapshot.ID, userID, projectID, "issued", now).
			Updates(map[string]any{"status": "consumed", "consumed_at": now})
		if consumed.Error != nil {
			return consumed.Error
		}
		if consumed.RowsAffected != 1 {
			return ErrPricingSnapshotStale
		}
		if err := refreshBatchCounters(ctx, tx, userID, batch.ID, now); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", batch.ID, userID).First(&batch).Error; err != nil {
			return err
		}
		result = BatchSnapshot{Batch: batch, Items: items}
		return nil
	})
	return result, err
}

func (s *Service) GetBatch(ctx context.Context, userID, batchID uint) (BatchSnapshot, error) {
	var batch CommerceGenerationBatch
	err := s.repository.db.WithContext(ctx).Where("id = ? AND user_id = ?", batchID, userID).First(&batch).Error
	if err != nil {
		return BatchSnapshot{}, mapNotFound(err)
	}
	items, err := loadBatchItems(ctx, s.repository.db, userID, batchID)
	return BatchSnapshot{Batch: batch, Items: items}, err
}

func (s *Service) ListBatches(ctx context.Context, userID, projectID uint) ([]CommerceGenerationBatch, error) {
	var batches []CommerceGenerationBatch
	err := s.repository.db.WithContext(ctx).Where("user_id = ? AND project_id = ?", userID, projectID).Order("id desc").Find(&batches).Error
	return batches, err
}

func (s *Service) CompleteGenerationItem(ctx context.Context, lease LeaseIdentity, itemID uint, result ExecutionResult) error {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return err
	}
	return s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item, job, err := loadLeasedItem(ctx, tx, lease, itemID)
		if err != nil {
			return err
		}
		if item.Status == CommerceItemSucceeded {
			return nil
		}
		if item.Status != CommerceItemRunning || job.Status != CommerceJobRunning {
			return ErrInvalidItemTransition
		}
		if item.CancelRequestedAt != nil || job.CancelRequestedAt != nil {
			return s.cancelLeasedRunningItemTx(ctx, tx, lease, item, job, "late_result_discarded")
		}
		now := s.now().UTC()
		updates := map[string]any{"status": CommerceItemSucceeded, "progress_percent": 100, "finished_at": now, "error_code": "", "error_message": ""}
		if result.GenerationRecordID != 0 {
			updates["generation_record_id"] = result.GenerationRecordID
		}
		if result.WorkID != 0 {
			updates["work_id"] = result.WorkID
		}
		claimed := tx.Model(&CommerceGenerationItem{}).
			Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, item.UserID, CommerceItemRunning).
			Updates(updates)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return s.resolveLostWorkerTransition(ctx, tx, lease, item.ID, CommerceItemSucceeded, "late_result_discarded")
		}
		if err := s.creditLedger.SettleItemTx(ctx, tx, SettleCreditsRequest{
			UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
			ReservationID: item.ReservationID, GenerationItemID: item.ID,
			HeldCredits: item.ReservedCredits, ActualCredits: result.ActualCredits,
			IdempotencyKey: fmt.Sprintf("commerce:item:%d:settle", item.ID),
		}); err != nil {
			return err
		}
		jobResult := tx.Model(&CommerceJob{}).
			Where("id = ? AND user_id = ? AND lease_owner = ? AND lease_token = ? AND status = ? AND cancel_requested_at IS NULL", job.ID, item.UserID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
			Updates(map[string]any{"status": CommerceJobSucceeded, "finished_at": now, "result_json": result.MetadataJSON})
		if jobResult.Error != nil {
			return jobResult.Error
		}
		if jobResult.RowsAffected != 1 {
			return ErrInvalidItemTransition
		}
		return refreshBatchCounters(ctx, tx, item.UserID, item.BatchID, now)
	})
}

func (s *Service) FailGenerationItem(ctx context.Context, lease LeaseIdentity, itemID uint, failure ExecutionFailure) error {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return err
	}
	return s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item, job, err := loadLeasedItem(ctx, tx, lease, itemID)
		if err != nil {
			return err
		}
		if item.Status == CommerceItemFailed || item.Status == CommerceItemCanceled {
			return nil
		}
		if item.Status != CommerceItemRunning || job.Status != CommerceJobRunning {
			return ErrInvalidItemTransition
		}
		if item.CancelRequestedAt != nil || job.CancelRequestedAt != nil {
			return s.cancelLeasedRunningItemTx(ctx, tx, lease, item, job, "worker_canceled")
		}
		now := s.now().UTC()
		if failure.Retryable && job.AttemptCount < job.MaxAttempts {
			claimed := tx.Model(&CommerceGenerationItem{}).
				Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, item.UserID, CommerceItemRunning).
				Updates(map[string]any{"status": CommerceItemRetrying, "error_code": failure.Code, "error_message": failure.Message})
			if claimed.Error != nil {
				return claimed.Error
			}
			if claimed.RowsAffected != 1 {
				return s.resolveLostWorkerTransition(ctx, tx, lease, item.ID, CommerceItemRetrying, "worker_canceled")
			}
			jobResult := tx.Model(&CommerceJob{}).
				Where("id = ? AND user_id = ? AND lease_owner = ? AND lease_token = ? AND status = ? AND cancel_requested_at IS NULL", job.ID, item.UserID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
				Updates(map[string]any{"status": CommerceJobRetrying, "error_code": failure.Code, "error_message": failure.Message})
			if jobResult.Error != nil {
				return jobResult.Error
			}
			if jobResult.RowsAffected != 1 {
				return ErrInvalidItemTransition
			}
			return refreshBatchCounters(ctx, tx, item.UserID, item.BatchID, now)
		}
		claimed := tx.Model(&CommerceGenerationItem{}).
			Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, item.UserID, CommerceItemRunning).
			Updates(map[string]any{"status": CommerceItemFailed, "progress_percent": 100, "finished_at": now, "error_code": failure.Code, "error_message": failure.Message})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return s.resolveLostWorkerTransition(ctx, tx, lease, item.ID, CommerceItemFailed, "worker_canceled")
		}
		if err := s.creditLedger.ReleaseItemTx(ctx, tx, ReleaseCreditsRequest{
			UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
			ReservationID: item.ReservationID, GenerationItemID: item.ID, HeldCredits: item.ReservedCredits,
			Reason: "generation_failed", IdempotencyKey: fmt.Sprintf("commerce:item:%d:release", item.ID),
		}); err != nil {
			return err
		}
		jobResult := tx.Model(&CommerceJob{}).
			Where("id = ? AND user_id = ? AND lease_owner = ? AND lease_token = ? AND status = ? AND cancel_requested_at IS NULL", job.ID, item.UserID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
			Updates(map[string]any{"status": CommerceJobFailed, "finished_at": now, "error_code": failure.Code, "error_message": failure.Message})
		if jobResult.Error != nil {
			return jobResult.Error
		}
		if jobResult.RowsAffected != 1 {
			return ErrInvalidItemTransition
		}
		return refreshBatchCounters(ctx, tx, item.UserID, item.BatchID, now)
	})
}

func (s *Service) cancelLeasedRunningItemTx(ctx context.Context, tx *gorm.DB, lease LeaseIdentity, item CommerceGenerationItem, job CommerceJob, reason string) error {
	if err := s.creditLedger.ReleaseItemTx(ctx, tx, ReleaseCreditsRequest{
		UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
		ReservationID: item.ReservationID, GenerationItemID: item.ID, HeldCredits: item.ReservedCredits,
		Reason: reason, IdempotencyKey: fmt.Sprintf("commerce:item:%d:cancel", item.ID),
	}); err != nil {
		return err
	}
	now := s.now().UTC()
	if err := tx.Model(&CommerceGenerationItem{}).Where("id = ? AND user_id = ?", item.ID, item.UserID).
		Updates(map[string]any{"status": CommerceItemCanceled, "progress_percent": 100, "finished_at": now}).Error; err != nil {
		return err
	}
	jobResult := tx.Model(&CommerceJob{}).
		Where("id = ? AND user_id = ? AND lease_owner = ? AND lease_token = ? AND status = ?", job.ID, item.UserID, lease.LeaseOwner, lease.LeaseToken, CommerceJobRunning).
		Updates(map[string]any{"status": CommerceJobCanceled, "finished_at": now})
	if jobResult.Error != nil {
		return jobResult.Error
	}
	if jobResult.RowsAffected != 1 {
		return ErrLeaseMismatch
	}
	return refreshBatchCounters(ctx, tx, item.UserID, item.BatchID, now)
}

func (s *Service) resolveLostWorkerTransition(ctx context.Context, tx *gorm.DB, lease LeaseIdentity, itemID uint, target CommerceItemStatus, cancelReason string) error {
	item, job, err := loadLeasedItem(ctx, tx, lease, itemID)
	if err != nil {
		return err
	}
	if item.Status == target || item.Status == CommerceItemCanceled {
		return nil
	}
	if item.Status == CommerceItemRunning && (item.CancelRequestedAt != nil || job.CancelRequestedAt != nil) {
		return s.cancelLeasedRunningItemTx(ctx, tx, lease, item, job, cancelReason)
	}
	return ErrInvalidItemTransition
}

func (s *Service) CancelItem(ctx context.Context, userID, itemID uint) (CommerceGenerationItem, error) {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return CommerceGenerationItem{}, err
	}
	var result CommerceGenerationItem
	err := s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item CommerceGenerationItem
		if err := tx.Where("id = ? AND user_id = ?", itemID, userID).First(&item).Error; err != nil {
			return mapNotFound(err)
		}
		if item.Status == CommerceItemSucceeded || item.Status == CommerceItemFailed || item.Status == CommerceItemCanceled {
			result = item
			return nil
		}
		now := s.now().UTC()
		if item.Status == CommerceItemRunning {
			claimed := tx.Model(&CommerceGenerationItem{}).
				Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, userID, CommerceItemRunning).
				Update("cancel_requested_at", now)
			if claimed.Error != nil {
				return claimed.Error
			}
			if claimed.RowsAffected != 1 {
				if err := tx.Where("id = ? AND user_id = ?", item.ID, userID).First(&item).Error; err != nil {
					return mapNotFound(err)
				}
				result = item
				return nil
			}
			if err := tx.Model(&CommerceJob{}).
				Where("generation_item_id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, userID, CommerceJobRunning).
				Update("cancel_requested_at", now).Error; err != nil {
				return err
			}
			item.CancelRequestedAt = &now
			result = item
			return nil
		}
		claimed := tx.Model(&CommerceGenerationItem{}).
			Where("id = ? AND user_id = ? AND status IN ?", item.ID, userID, []CommerceItemStatus{CommerceItemQueued, CommerceItemRetrying}).
			Updates(map[string]any{"status": CommerceItemCanceled, "progress_percent": 100, "finished_at": now})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			if err := tx.Where("id = ? AND user_id = ?", item.ID, userID).First(&item).Error; err != nil {
				return mapNotFound(err)
			}
			result = item
			return nil
		}
		if err := s.creditLedger.ReleaseItemTx(ctx, tx, ReleaseCreditsRequest{
			UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
			ReservationID: item.ReservationID, GenerationItemID: item.ID, HeldCredits: item.ReservedCredits,
			Reason: "user_canceled", IdempotencyKey: fmt.Sprintf("commerce:item:%d:cancel", item.ID),
		}); err != nil {
			return err
		}
		if err := tx.Model(&CommerceJob{}).Where("generation_item_id = ? AND user_id = ? AND status IN ?", item.ID, userID, []CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying}).
			Updates(map[string]any{"status": CommerceJobCanceled, "finished_at": now}).Error; err != nil {
			return err
		}
		item.Status, item.FinishedAt = CommerceItemCanceled, &now
		result = item
		return refreshBatchCounters(ctx, tx, userID, item.BatchID, now)
	})
	return result, err
}

func (s *Service) CancelBatch(ctx context.Context, userID, batchID uint) (BatchSnapshot, error) {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return BatchSnapshot{}, err
	}
	var batchIdentity CommerceGenerationBatch
	if err := s.repository.db.WithContext(ctx).Select("id", "user_id", "project_id").
		Where("id = ? AND user_id = ?", batchID, userID).First(&batchIdentity).Error; err != nil {
		return BatchSnapshot{}, mapNotFound(err)
	}
	var result BatchSnapshot
	err := runProjectMutationTransaction(ctx, s.repository.db, func(tx *gorm.DB) error {
		if _, err := lockProjectRowTx(ctx, tx, userID, batchIdentity.ProjectID); err != nil {
			return err
		}
		var batch CommerceGenerationBatch
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND project_id = ?", batchID, userID, batchIdentity.ProjectID).First(&batch).Error; err != nil {
			return mapNotFound(err)
		}
		var jobs []CommerceJob
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND batch_id = ? AND status IN ?", userID, batchID,
				[]CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying, CommerceJobRunning}).
			Order("id ASC").Find(&jobs).Error; err != nil {
			return err
		}
		var items []CommerceGenerationItem
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND batch_id = ? AND status IN ?", userID, batchID,
				[]CommerceItemStatus{CommerceItemQueued, CommerceItemRetrying, CommerceItemRunning}).
			Order("id ASC").Find(&items).Error; err != nil {
			return err
		}
		now := s.now().UTC()
		jobsByItem := make(map[uint]CommerceJob, len(jobs))
		for _, job := range jobs {
			if job.GenerationItemID != nil {
				jobsByItem[*job.GenerationItemID] = job
			}
		}
		for _, item := range items {
			switch item.Status {
			case CommerceItemQueued, CommerceItemRetrying:
				if err := s.creditLedger.ReleaseItemTx(ctx, tx, ReleaseCreditsRequest{
					UserID: item.UserID, ProjectID: item.ProjectID, BatchID: item.BatchID,
					ReservationID: item.ReservationID, GenerationItemID: item.ID, HeldCredits: item.ReservedCredits,
					Reason: "user_canceled", IdempotencyKey: fmt.Sprintf("commerce:item:%d:cancel", item.ID),
				}); err != nil {
					return err
				}
				updated := tx.Model(&CommerceGenerationItem{}).
					Where("id = ? AND user_id = ? AND status IN ?", item.ID, userID, []CommerceItemStatus{CommerceItemQueued, CommerceItemRetrying}).
					Updates(map[string]any{"status": CommerceItemCanceled, "progress_percent": 100, "cancel_requested_at": now, "finished_at": now})
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return ErrInvalidItemTransition
				}
				if job, ok := jobsByItem[item.ID]; ok {
					if err := emitCommerceEventTx(tx, job, CommerceEventItemReleased, map[string]any{"item_id": item.ID, "reason": "user_canceled"}); err != nil {
						return err
					}
				}
			case CommerceItemRunning:
				updated := tx.Model(&CommerceGenerationItem{}).
					Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", item.ID, userID, CommerceItemRunning).
					Update("cancel_requested_at", now)
				if updated.Error != nil {
					return updated.Error
				}
			}
		}
		for _, job := range jobs {
			switch job.Status {
			case CommerceJobQueued, CommerceJobRetrying:
				updated := tx.Model(&CommerceJob{}).
					Where("id = ? AND user_id = ? AND status IN ?", job.ID, userID, []CommerceJobStatus{CommerceJobQueued, CommerceJobRetrying}).
					Updates(map[string]any{"status": CommerceJobCanceled, "cancel_requested_at": now, "finished_at": now})
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return ErrInvalidItemTransition
				}
			case CommerceJobRunning:
				if err := tx.Model(&CommerceJob{}).
					Where("id = ? AND user_id = ? AND status = ? AND cancel_requested_at IS NULL", job.ID, userID, CommerceJobRunning).
					Update("cancel_requested_at", now).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&CommerceGenerationBatch{}).Where("id = ? AND user_id = ?", batchID, userID).
			Update("cancel_requested_at", now).Error; err != nil {
			return err
		}
		if err := refreshBatchCounters(ctx, tx, userID, batchID, now); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", batchID, userID).First(&result.Batch).Error; err != nil {
			return err
		}
		loadedItems, err := loadBatchItems(ctx, tx, userID, batchID)
		if err != nil {
			return err
		}
		result.Items = loadedItems
		return nil
	})
	return result, err
}

func (s *Service) RetryItem(ctx context.Context, userID, itemID uint, key string) (BatchSnapshot, error) {
	if err := s.ensureBatchInfrastructure(); err != nil {
		return BatchSnapshot{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return BatchSnapshot{}, invalidField("idempotency_key", "idempotency key is required")
	}
	digest := fmt.Sprintf("retry:%d", itemID)
	var result BatchSnapshot
	err := runProjectMutationTransaction(ctx, s.repository.db, func(tx *gorm.DB) error {
		var parent CommerceGenerationItem
		if err := tx.Where("id = ? AND user_id = ?", itemID, userID).First(&parent).Error; err != nil {
			return mapNotFound(err)
		}
		if parent.Status != CommerceItemFailed && parent.Status != CommerceItemCanceled {
			return ErrInvalidItemTransition
		}
		if _, err := lockWritableProjectTx(ctx, tx, userID, parent.ProjectID); err != nil {
			return err
		}
		batchClaim := CommerceGenerationBatch{
			UserID: userID, Status: CommerceBatchQueued, IdempotencyKey: key, RequestDigest: digest,
		}
		claim := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(&batchClaim)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			var existing CommerceGenerationBatch
			if err := tx.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&existing).Error; err != nil {
				return err
			}
			if err := validateIdempotencyReplay(existing.IdempotencyKey, existing.RequestDigest, key, digest); err != nil {
				return err
			}
			items, err := loadBatchItems(ctx, tx, userID, existing.ID)
			if err != nil {
				return err
			}
			result = BatchSnapshot{Batch: existing, Items: items}
			return nil
		}
		var parentJob CommerceJob
		if err := tx.Where("generation_item_id = ? AND user_id = ?", parent.ID, userID).First(&parentJob).Error; err != nil {
			return mapNotFound(err)
		}
		var parentBatch CommerceGenerationBatch
		if err := tx.Where("id = ? AND user_id = ?", parent.BatchID, userID).First(&parentBatch).Error; err != nil {
			return mapNotFound(err)
		}
		batch := parentBatch
		batch.ID = batchClaim.ID
		batch.ParentBatchID = &parentBatch.ID
		batch.ReservationID = nil
		batch.IdempotencyKey = key
		batch.RequestDigest = digest
		batch.TotalItems, batch.QueuedItems = 1, 1
		batch.RunningItems, batch.RetryingItems, batch.SucceededItems, batch.FailedItems, batch.CanceledItems = 0, 0, 0, 0, 0
		batch.EstimatedCredits, batch.ReservedCredits = parent.EstimatedCredits, parent.EstimatedCredits
		batch.SettledCredits, batch.ReleasedCredits = 0, 0
		batch.Status = CommerceBatchQueued
		batch.StartedAt, batch.FinishedAt, batch.CancelRequestedAt = nil, nil, nil
		batch.CreatedAt, batch.UpdatedAt = batchClaim.CreatedAt, batchClaim.UpdatedAt
		if err := tx.Save(&batch).Error; err != nil {
			return err
		}
		reservation, err := s.creditLedger.ReserveTx(ctx, tx, ReserveCreditsRequest{
			UserID: parent.UserID, ProjectID: parent.ProjectID, BatchID: &batch.ID,
			ScopeType: "batch", ScopeKey: fmt.Sprintf("%d", batch.ID), Amount: parent.ReservedCredits,
			IdempotencyKey: "commerce:retry:" + key,
		})
		if err != nil {
			return err
		}
		batch.ReservationID = &reservation.ReservationID
		if err := tx.Model(&CommerceGenerationBatch{}).Where("id = ? AND user_id = ?", batch.ID, userID).Update("reservation_id", reservation.ReservationID).Error; err != nil {
			return err
		}
		item := parent
		item.ID = 0
		item.ParentItemID = &parent.ID
		item.BatchID, item.ReservationID = batch.ID, reservation.ReservationID
		item.IdempotencyKey, item.Status = key+":item:0", CommerceItemQueued
		item.ProgressPercent = 0
		item.SettledCredits, item.ReleasedCredits = 0, 0
		item.GenerationRecordID, item.WorkID = nil, nil
		item.ErrorCode, item.ErrorMessage = "", ""
		item.CancelRequestedAt, item.StartedAt, item.FinishedAt = nil, nil, nil
		item.CreatedAt, item.UpdatedAt = time.Time{}, time.Time{}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		itemIDCopy, batchIDCopy := item.ID, batch.ID
		job := CommerceJob{
			UserID: userID, ProjectID: item.ProjectID, BatchID: &batchIDCopy, GenerationItemID: &itemIDCopy,
			Kind: CommerceJobKindGenerateItem, Pipeline: item.Pipeline, RecipeKey: item.RecipeKey,
			Status: CommerceJobQueued, IdempotencyKey: key + ":job:0", MaxAttempts: parentJob.MaxAttempts, PayloadJSON: item.OutputSpecJSON,
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		now := s.now().UTC()
		if err := refreshBatchCounters(ctx, tx, userID, batch.ID, now); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", batch.ID, userID).First(&batch).Error; err != nil {
			return err
		}
		result = BatchSnapshot{Batch: batch, Items: []CommerceGenerationItem{item}}
		return nil
	})
	return result, err
}

func (s *Service) ensureBatchInfrastructure() error {
	if s == nil || s.repository == nil || s.repository.db == nil || s.batchRegistry == nil || s.creditLedger == nil || s.pricingSnapshots == nil {
		return fmt.Errorf("commerce batch infrastructure is not configured")
	}
	return nil
}

func (s *Service) batchCompileInput(ctx context.Context, db *gorm.DB, userID, projectID uint, req EstimateBatchRequest) (CommerceProject, CompileInput, RecipeDefinition, error) {
	var project CommerceProject
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, mapNotFound(err)
	}
	if project.Status == "deletion_requested" || project.DeletionRequestedAt != nil {
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, ErrProjectDeletionRequested
	}
	compiler, ok := s.batchRegistry.Get(project.Pipeline, req.RecipeKey, req.RecipeVersion)
	if !ok {
		if project.Pipeline == "general" && req.RecipeKey == ProductDetailSetRecipeKey && req.RecipeVersion == ProductDetailSetVersion {
			return CommerceProject{}, CompileInput{}, RecipeDefinition{}, ErrRecipeModelUnavailable
		}
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, fmt.Errorf("%w: %s/%s@%d", ErrRecipeNotFound, project.Pipeline, req.RecipeKey, req.RecipeVersion)
	}
	creativeSpec, err := buildConfirmedCreativeSpecSnapshotTx(ctx, db, userID, projectID, req.CreativeSpecID)
	if err != nil {
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, err
	}
	selectedSKUs := append([]uint(nil), req.SelectedSKUIDs...)
	if len(selectedSKUs) == 0 && req.PrimarySKUID != 0 {
		selectedSKUs = []uint{req.PrimarySKUID}
	}
	if err := validateProjectSKUsTx(ctx, db, userID, project.ProductID, req.PrimarySKUID, selectedSKUs); err != nil {
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, err
	}
	assets, err := loadAssetBindingSnapshots(ctx, db, userID, projectID, req.AssetBindings)
	if err != nil {
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, err
	}
	skuSnapshots, err := loadSKUSnapshots(ctx, db, userID, project.ProductID, selectedSKUs)
	if err != nil {
		return CommerceProject{}, CompileInput{}, RecipeDefinition{}, err
	}
	definition := compiler.Definition()
	outputCount := req.OutputCount
	if definition.Key == ProductDetailSetRecipeKey {
		sections, scopes, _, parseErr := productDetailParameters(req.Parameters, definition)
		if parseErr != nil {
			return CommerceProject{}, CompileInput{}, RecipeDefinition{}, parseErr
		}
		outputCount = 0
		for _, section := range sections {
			if scopes[section] == "shared" {
				outputCount++
			} else {
				outputCount += len(selectedSKUs)
			}
		}
	}
	return project, CompileInput{
		UserID: userID, ProjectID: projectID, PrimarySKUID: req.PrimarySKUID, CreativeSpecID: req.CreativeSpecID,
		CreativeSpec: creativeSpec, SelectedSKUIDs: selectedSKUs,
		RecipeKey: req.RecipeKey, Pipeline: project.Pipeline, RecipeVersion: req.RecipeVersion,
		OutputCount: outputCount, AspectRatio: req.AspectRatio, QualityTier: req.QualityTier,
		AssetBindings: cloneUintSliceMap(req.AssetBindings), Assets: assets, Parameters: cloneMap(req.Parameters), SKUSnapshots: skuSnapshots,
	}, definition, nil
}

func loadSKUSnapshots(ctx context.Context, db *gorm.DB, userID, productID uint, ids []uint) (map[uint]SKUSnapshot, error) {
	var skus []CommerceSKU
	if err := db.WithContext(ctx).Where("user_id = ? AND product_id = ? AND id IN ?", userID, productID, ids).Order("id").Find(&skus).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]SKUSnapshot, len(skus))
	for _, sku := range skus {
		parts := []string{}
		for _, value := range []string{sku.Color, sku.Style, sku.Size} {
			if strings.TrimSpace(value) != "" {
				parts = append(parts, strings.TrimSpace(value))
			}
		}
		result[sku.ID] = SKUSnapshot{ID: sku.ID, Code: sku.Code, SpecificationPath: strings.Join(parts, "/"), AttributesJSON: defaultJSON(sku.AttributesJSON, "{}")}
	}
	return result, nil
}

func buildConfirmedCreativeSpecSnapshotTx(ctx context.Context, db *gorm.DB, userID, projectID, creativeSpecID uint) (CreativeSpecSnapshot, error) {
	var spec CommerceCreativeSpec
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ? AND project_id = ?", creativeSpecID, userID, projectID).First(&spec).Error; err != nil {
		return CreativeSpecSnapshot{}, mapNotFound(err)
	}
	if spec.Status != "confirmed" || spec.LockedAt == nil {
		return CreativeSpecSnapshot{}, ErrCreativeSpecNotConfirmed
	}
	if spec.SKUContextSHA256 != "" {
		var project CommerceProject
		if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
			return CreativeSpecSnapshot{}, mapNotFound(err)
		}
		current, err := creativeSpecSKUContextDigest(ctx, db, project)
		if err != nil {
			return CreativeSpecSnapshot{}, err
		}
		if current != spec.SKUContextSHA256 {
			return CreativeSpecSnapshot{}, ErrCreativeSpecNotConfirmed
		}
	}
	snapshot := CreativeSpecSnapshot{
		ID: spec.ID, Version: uint(spec.Version), Status: spec.Status,
		ProductFactsJSON: spec.ProductFactsJSON, SellingPointsJSON: spec.SellingPointsJSON,
		CommonFactsJSON: defaultJSON(spec.CommonFactsJSON, spec.ProductFactsJSON), SKUOverridesJSON: defaultJSON(spec.SKUOverridesJSON, "{}"), SKUContextSHA256: spec.SKUContextSHA256,
		ForbiddenChangesJSON: spec.ForbiddenChangesJSON, BrandToneJSON: spec.BrandToneJSON,
		ShotPlanJSON: spec.ShotPlanJSON, CopyBlocksJSON: spec.CopyBlocksJSON,
		RiskNoticesJSON: spec.RiskNoticesJSON, SourceAssetIDsJSON: spec.SourceAssetIDsJSON,
	}
	if err := normalizeCreativeSpecSnapshot(&snapshot); err != nil {
		return CreativeSpecSnapshot{}, err
	}
	payload, err := EncodeJSON(snapshot)
	if err != nil {
		return CreativeSpecSnapshot{}, err
	}
	digest := sha256.Sum256([]byte(payload))
	snapshot.ContentSHA256 = hex.EncodeToString(digest[:])
	return snapshot, nil
}

func validateProjectSKUsTx(ctx context.Context, db *gorm.DB, userID, productID, primarySKUID uint, skuIDs []uint) error {
	var product CommerceProduct
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", productID, userID).First(&product).Error; err != nil {
		return mapNotFound(err)
	}
	if len(skuIDs) == 0 {
		return ErrSKUSelectionRequired
	}
	if len(skuIDs) > 100 {
		return ErrSKUSelectionTooLarge
	}
	seen := make(map[uint]struct{}, len(skuIDs))
	primarySelected := false
	for _, skuID := range skuIDs {
		if skuID == primarySKUID {
			primarySelected = true
			break
		}
	}
	if !primarySelected {
		return ErrPrimarySKUNotSelected
	}
	for _, skuID := range skuIDs {
		if skuID == 0 {
			return ErrOwnershipMismatch
		}
		if _, ok := seen[skuID]; ok {
			continue
		}
		seen[skuID] = struct{}{}
		var sku CommerceSKU
		if err := db.WithContext(ctx).Where("id = ? AND user_id = ? AND product_id = ?", skuID, userID, productID).First(&sku).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOwnershipMismatch
			}
			return err
		}
		if sku.Status != "active" {
			return ErrSKUDisabled
		}
	}
	return nil
}

func loadAssetBindingSnapshots(ctx context.Context, db *gorm.DB, userID, projectID uint, bindings map[string][]uint) ([]AssetBindingSnapshot, error) {
	ids := make([]uint, 0)
	roles := make(map[uint]string)
	for role, values := range bindings {
		for _, id := range values {
			if id == 0 {
				return nil, ErrOwnershipMismatch
			}
			if _, exists := roles[id]; exists {
				return nil, ErrOwnershipMismatch
			}
			roles[id], ids = role, append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var assets []CommerceAsset
	if err := db.WithContext(ctx).Where("user_id = ? AND project_id = ? AND id IN ?", userID, projectID, ids).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(ids) {
		return nil, ErrOwnershipMismatch
	}
	result := make([]AssetBindingSnapshot, 0, len(assets))
	for _, asset := range assets {
		result = append(result, AssetBindingSnapshot{
			CommerceAssetID: asset.ID, ReferenceAssetID: asset.ReferenceAssetID, SKUID: asset.SKUID,
			Role: roles[asset.ID], MetadataJSON: asset.MetadataJSON,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CommerceAssetID < result[j].CommerceAssetID })
	return result, nil
}

func canonicalBatchRequestDigest(req EstimateBatchRequest) (string, string, error) {
	canonical := struct {
		RecipeKey, QualityTier       string
		RecipeVersion, OutputCount   int
		CreativeSpecID, PrimarySKUID uint
		SelectedSKUIDs               []uint
		AspectRatio                  string
		AssetBindings                []canonicalUintBinding
		Parameters                   []canonicalParameter
	}{
		RecipeKey: req.RecipeKey, QualityTier: req.QualityTier, RecipeVersion: req.RecipeVersion,
		OutputCount: req.OutputCount, CreativeSpecID: req.CreativeSpecID, PrimarySKUID: req.PrimarySKUID,
		SelectedSKUIDs: append([]uint(nil), req.SelectedSKUIDs...), AspectRatio: req.AspectRatio,
	}
	for key, values := range req.AssetBindings {
		copied := append([]uint(nil), values...)
		canonical.AssetBindings = append(canonical.AssetBindings, canonicalUintBinding{Key: key, Values: copied})
	}
	sort.Slice(canonical.AssetBindings, func(i, j int) bool { return canonical.AssetBindings[i].Key < canonical.AssetBindings[j].Key })
	for key, value := range req.Parameters {
		encoded, err := EncodeJSON(value)
		if err != nil {
			return "", "", fmt.Errorf("encode batch parameter %s: %w", key, err)
		}
		canonical.Parameters = append(canonical.Parameters, canonicalParameter{Key: key, JSON: encoded})
	}
	sort.Slice(canonical.Parameters, func(i, j int) bool { return canonical.Parameters[i].Key < canonical.Parameters[j].Key })
	raw, err := EncodeJSON(canonical)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:]), raw, nil
}

type canonicalUintBinding struct {
	Key    string `json:"key"`
	Values []uint `json:"values"`
}

type canonicalParameter struct {
	Key  string `json:"key"`
	JSON string `json:"json"`
}

func isIdempotencyReplay(existingKey, existingDigest, key, digest string) bool {
	return existingKey == key && existingDigest == digest
}

func validateIdempotencyReplay(existingKey, existingDigest, key, digest string) error {
	if existingKey != key || existingDigest != digest {
		return ErrIdempotencyConflict
	}
	return nil
}

func cloneUintSliceMap(source map[string][]uint) map[string][]uint {
	if source == nil {
		return nil
	}
	result := make(map[string][]uint, len(source))
	for key, value := range source {
		result[key] = append([]uint(nil), value...)
	}
	return result
}

func pricingEntriesFromItems(items []CompiledGenerationItem, qualityTier string) []PricingSnapshotEntry {
	entries := make([]PricingSnapshotEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, PricingSnapshotEntry{
			Pipeline: item.Pipeline, RecipeKey: item.RecipeKey, QualityTier: qualityTier, Credits: item.EstimatedCredits,
		})
	}
	return entries
}

func loadBatchItems(ctx context.Context, db *gorm.DB, userID, batchID uint) ([]CommerceGenerationItem, error) {
	var items []CommerceGenerationItem
	if err := db.WithContext(ctx).Where("user_id = ? AND batch_id = ?", userID, batchID).Order("id asc").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	ids := make([]uint, len(items))
	for index := range items {
		ids[index] = items[index].ID
	}
	var jobs []CommerceJob
	if err := db.WithContext(ctx).Where("user_id = ? AND generation_item_id IN ? AND status = ?", userID, ids, CommerceJobSucceeded).Order("id desc").Find(&jobs).Error; err != nil {
		return nil, err
	}
	results := map[uint]string{}
	for _, job := range jobs {
		if job.GenerationItemID != nil && results[*job.GenerationItemID] == "" {
			results[*job.GenerationItemID] = job.ResultJSON
		}
	}
	for index := range items {
		items[index].OutputSnapshotJSON = results[items[index].ID]
	}
	return items, nil
}

func loadLeasedItem(ctx context.Context, tx *gorm.DB, lease LeaseIdentity, itemID uint) (CommerceGenerationItem, CommerceJob, error) {
	var item CommerceGenerationItem
	if err := tx.WithContext(ctx).Where("id = ?", itemID).First(&item).Error; err != nil {
		return CommerceGenerationItem{}, CommerceJob{}, mapNotFound(err)
	}
	var job CommerceJob
	if err := tx.WithContext(ctx).Where("id = ? AND generation_item_id = ?", lease.JobID, itemID).First(&job).Error; err != nil {
		return CommerceGenerationItem{}, CommerceJob{}, ErrLeaseMismatch
	}
	if job.LeaseOwner != lease.LeaseOwner || job.LeaseToken != lease.LeaseToken || strings.TrimSpace(lease.LeaseOwner) == "" || strings.TrimSpace(lease.LeaseToken) == "" {
		return CommerceGenerationItem{}, CommerceJob{}, ErrLeaseMismatch
	}
	return item, job, nil
}

func refreshBatchCounters(ctx context.Context, tx *gorm.DB, userID, batchID uint, now time.Time) error {
	var items []CommerceGenerationItem
	if err := tx.WithContext(ctx).Where("user_id = ? AND batch_id = ?", userID, batchID).Find(&items).Error; err != nil {
		return err
	}
	counters := map[CommerceItemStatus]int{}
	settled, released := 0, 0
	for _, item := range items {
		counters[item.Status]++
		settled += item.SettledCredits
		released += item.ReleasedCredits
	}
	status := CommerceBatchRunning
	terminal := counters[CommerceItemSucceeded] + counters[CommerceItemFailed] + counters[CommerceItemCanceled]
	if counters[CommerceItemQueued] == len(items) && len(items) > 0 {
		status = CommerceBatchQueued
	}
	if terminal == len(items) {
		switch {
		case counters[CommerceItemSucceeded] == len(items):
			status = CommerceBatchSucceeded
		case counters[CommerceItemCanceled] == len(items):
			status = CommerceBatchCanceled
		case counters[CommerceItemSucceeded] > 0:
			status = CommerceBatchPartialSucceeded
		default:
			status = CommerceBatchFailed
		}
	}
	updates := map[string]any{
		"status": status, "queued_items": counters[CommerceItemQueued], "running_items": counters[CommerceItemRunning],
		"retrying_items": counters[CommerceItemRetrying], "succeeded_items": counters[CommerceItemSucceeded],
		"failed_items": counters[CommerceItemFailed], "canceled_items": counters[CommerceItemCanceled],
		"settled_credits": settled, "released_credits": released, "eta_seconds": commerceBatchETASeconds(items),
	}
	if terminal == len(items) {
		updates["finished_at"] = now
	}
	return tx.Model(&CommerceGenerationBatch{}).Where("id = ? AND user_id = ?", batchID, userID).Updates(updates).Error
}

const defaultCommerceItemETASeconds = 60

func commerceCompiledItemsETASeconds(items []CompiledGenerationItem) int {
	pending := make([]CommerceGenerationItem, len(items))
	for index := range pending {
		pending[index].Status = CommerceItemQueued
	}
	return commerceBatchETASeconds(pending)
}

func commerceBatchETASeconds(items []CommerceGenerationItem) int {
	remaining := 0
	sampleSeconds := 0
	samples := 0
	for _, item := range items {
		switch item.Status {
		case CommerceItemQueued, CommerceItemRetrying, CommerceItemRunning:
			remaining++
		case CommerceItemSucceeded, CommerceItemFailed, CommerceItemCanceled:
			duration := item.UpdatedAt.Sub(item.CreatedAt)
			if duration <= 0 {
				continue
			}
			seconds := int((duration + time.Second - 1) / time.Second)
			if seconds < 10 {
				seconds = 10
			}
			if seconds > 600 {
				seconds = 600
			}
			sampleSeconds += seconds
			samples++
		}
	}
	if remaining == 0 {
		return 0
	}
	perItem := defaultCommerceItemETASeconds
	if samples > 0 {
		perItem = sampleSeconds / samples
	}
	return perItem * remaining
}
