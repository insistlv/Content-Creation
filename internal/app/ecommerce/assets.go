package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	StorageScopeDefault         = "default"
	StorageScopeCommercePrivate = "commerce_private"

	AssetLifecycleProject   = "project"
	AssetLifecycleTemporary = "temporary"

	CleanupStatusQueued    = "queued"
	CleanupStatusRetrying  = "retrying"
	CleanupStatusSucceeded = "succeeded"
	CleanupStatusFailed    = "failed"
)

const (
	defaultTemporaryRetention = 7 * 24 * time.Hour
	defaultCleanupMaxAttempts = 8
	maxCleanupRetryDelay      = 24 * time.Hour
)

type AssetService struct {
	repository *Repository
	now        func() time.Time
}

type CreateAssetInput struct {
	UserID, ProjectID, ReferenceAssetID uint
	SKUID                               *uint
	Role, Lifecycle                     string
	SortOrder                           int
	MetadataJSON                        string
	TemporaryRetention                  time.Duration
}

type OrphanCleanupInput struct {
	UserID, ProjectID uint
	ReferenceAssetID  *uint
	StorageScope      string
	ObjectKey, Reason string
	DeleteAfter       time.Time
}

type ObjectReferenceState struct {
	CommerceAssets       int64
	GenerationReferences int64
	Works                int64
	GenerationRecords    int64
	OtherActiveConsumers int64
}

func (s ObjectReferenceState) HasReferences() bool {
	return s.CommerceAssets+s.GenerationReferences+s.Works+s.GenerationRecords+s.OtherActiveConsumers > 0
}

func NewAssetService(repository *Repository) *AssetService {
	return &AssetService{repository: repository, now: time.Now}
}

func (s *AssetService) CreateAsset(ctx context.Context, input CreateAssetInput) (CommerceAsset, error) {
	if s == nil || s.repository == nil || s.repository.DB() == nil {
		return CommerceAsset{}, errors.New("commerce asset repository unavailable")
	}
	if input.UserID == 0 || input.ProjectID == 0 || input.ReferenceAssetID == 0 {
		return CommerceAsset{}, fmt.Errorf("%w: asset ownership fields are required", ErrInvalidInput)
	}
	if _, err := s.repository.GetProject(ctx, input.UserID, input.ProjectID); err != nil {
		return CommerceAsset{}, err
	}
	var referenceCount int64
	if err := s.repository.DB().WithContext(ctx).Table("reference_assets").
		Where("id = ? AND user_id = ?", input.ReferenceAssetID, input.UserID).
		Count(&referenceCount).Error; err != nil {
		return CommerceAsset{}, err
	}
	if referenceCount != 1 {
		return CommerceAsset{}, ErrOwnershipMismatch
	}

	lifecycle := strings.ToLower(strings.TrimSpace(input.Lifecycle))
	if lifecycle == "" {
		lifecycle = AssetLifecycleProject
	}
	if lifecycle != AssetLifecycleProject && lifecycle != AssetLifecycleTemporary {
		return CommerceAsset{}, fmt.Errorf("%w: unsupported asset lifecycle", ErrInvalidInput)
	}
	asset := CommerceAsset{
		UserID: input.UserID, ProjectID: input.ProjectID, ReferenceAssetID: input.ReferenceAssetID,
		SKUID: input.SKUID, Role: strings.TrimSpace(input.Role), Lifecycle: lifecycle,
		SortOrder: input.SortOrder, MetadataJSON: input.MetadataJSON,
	}
	if asset.Role == "" {
		asset.Role = "reference"
	}
	if lifecycle == AssetLifecycleTemporary {
		retention := input.TemporaryRetention
		if retention == 0 {
			retention = defaultTemporaryRetention
		}
		retainUntil := s.currentTime().Add(retention)
		asset.RetainUntil = &retainUntil
	}
	if err := s.repository.DB().WithContext(ctx).Create(&asset).Error; err != nil {
		return CommerceAsset{}, err
	}
	return asset, nil
}

func (s *AssetService) GetAsset(ctx context.Context, userID, assetID uint) (CommerceAsset, error) {
	var asset CommerceAsset
	err := s.repository.DB().WithContext(ctx).
		Where("id = ? AND user_id = ?", assetID, userID).
		First(&asset).Error
	return asset, mapNotFound(err)
}

func (s *AssetService) ListProjectAssets(ctx context.Context, userID, projectID uint) ([]CommerceAsset, error) {
	if _, err := s.repository.GetProject(ctx, userID, projectID); err != nil {
		return nil, err
	}
	var assets []CommerceAsset
	err := s.repository.DB().WithContext(ctx).
		Where("user_id = ? AND project_id = ?", userID, projectID).
		Order("sort_order asc, id asc").
		Find(&assets).Error
	return assets, err
}

func (s *AssetService) QueueExpiredTemporaryAssets(ctx context.Context) (int, error) {
	now := s.currentTime()
	type expiredAsset struct {
		CommerceAssetID  uint
		UserID           uint
		ProjectID        uint
		ReferenceAssetID uint
		StorageScope     string
		ObjectKey        string
	}
	var expired []expiredAsset
	err := s.repository.DB().WithContext(ctx).Table("commerce_assets AS ca").
		Select("ca.id AS commerce_asset_id, ca.user_id, ca.project_id, ca.reference_asset_id, ra.storage_scope, ra.asset_key AS object_key").
		Joins("JOIN reference_assets AS ra ON ra.id = ca.reference_asset_id").
		Where("ca.lifecycle = ? AND ca.retain_until IS NOT NULL AND ca.retain_until <= ? AND ca.object_deleted_at IS NULL AND ca.deleted_at IS NULL", AssetLifecycleTemporary, now).
		Scan(&expired).Error
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, candidate := range expired {
		references, err := s.InspectObjectReferences(ctx, candidate.StorageScope, candidate.ObjectKey, &candidate.CommerceAssetID)
		if err != nil {
			return queued, err
		}
		if references.HasReferences() {
			continue
		}
		err = s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&CommerceObjectCleanup{}).
				Where("commerce_asset_id = ? AND object_deleted_at IS NULL", candidate.CommerceAssetID).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			nextAttemptAt := now
			cleanup := CommerceObjectCleanup{
				UserID: candidate.UserID, ProjectID: candidate.ProjectID,
				CommerceAssetID: &candidate.CommerceAssetID, ReferenceAssetID: &candidate.ReferenceAssetID,
				StorageScope: normalizedStorageScope(candidate.StorageScope), ObjectKey: candidate.ObjectKey,
				Reason: "temporary_asset_expired", Status: CleanupStatusQueued,
				MaxAttempts: defaultCleanupMaxAttempts, NextAttemptAt: &nextAttemptAt, DeleteAfter: now,
			}
			if err := tx.Create(&cleanup).Error; err != nil {
				return err
			}
			queued++
			return nil
		})
		if err != nil {
			return queued, err
		}
	}
	return queued, nil
}

func (s *AssetService) InspectObjectReferences(ctx context.Context, storageScope, objectKey string, excludeCommerceAssetID *uint) (ObjectReferenceState, error) {
	var state ObjectReferenceState
	if s == nil || s.repository == nil || s.repository.DB() == nil {
		return state, errors.New("commerce asset repository unavailable")
	}
	return s.inspectObjectReferencesDB(ctx, s.repository.DB().WithContext(ctx), storageScope, objectKey, excludeCommerceAssetID)
}

func (s *AssetService) inspectObjectReferencesDB(ctx context.Context, db *gorm.DB, storageScope, objectKey string, excludeCommerceAssetID *uint) (ObjectReferenceState, error) {
	var state ObjectReferenceState
	db = db.WithContext(ctx)
	storageScope = normalizedStorageScope(storageScope)
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return state, fmt.Errorf("%w: object key is required", ErrInvalidInput)
	}
	scopes := []string{storageScope}
	if storageScope == StorageScopeDefault {
		scopes = append(scopes, "")
	}

	if db.Migrator().HasTable("commerce_assets") && db.Migrator().HasTable("reference_assets") {
		query := db.Table("commerce_assets AS ca").
			Joins("JOIN reference_assets AS ra ON ra.id = ca.reference_asset_id").
			Where("ca.deleted_at IS NULL AND ca.object_deleted_at IS NULL AND ra.asset_key = ? AND ra.storage_scope IN ?", objectKey, scopes)
		if excludeCommerceAssetID != nil && *excludeCommerceAssetID != 0 {
			query = query.Where("ca.id <> ?", *excludeCommerceAssetID)
		}
		if err := query.Count(&state.CommerceAssets).Error; err != nil {
			return state, err
		}
	}
	if db.Migrator().HasTable("generation_reference_assets") && db.Migrator().HasTable("reference_assets") {
		if err := db.Table("generation_reference_assets AS gra").
			Joins("JOIN reference_assets AS ra ON ra.id = gra.reference_asset_id").
			Where("ra.asset_key = ? AND ra.storage_scope IN ?", objectKey, scopes).
			Count(&state.GenerationReferences).Error; err != nil {
			return state, err
		}
	}
	if db.Migrator().HasTable("works") && db.Migrator().HasColumn("works", "storage_scope") {
		if err := db.Table("works").Where("deleted_at IS NULL AND asset_key = ? AND storage_scope IN ?", objectKey, scopes).Count(&state.Works).Error; err != nil {
			return state, err
		}
	}
	if db.Migrator().HasTable("generation_records") && db.Migrator().HasColumn("generation_records", "storage_scope") {
		if err := db.Table("generation_records").Where("asset_key = ? AND storage_scope IN ?", objectKey, scopes).Count(&state.GenerationRecords).Error; err != nil {
			return state, err
		}

	}
	if db.Migrator().HasTable("reference_assets") {
		consumerQueries := []struct {
			table     string
			condition string
		}{
			{"user_video_style_templates", "consumer.deleted_at IS NULL AND consumer.reference_asset_id = ra.id"},
			{"couple_albums", "consumer.deleted_at IS NULL AND (consumer.male_reference_asset_id = ra.id OR consumer.female_reference_asset_id = ra.id)"},
			{"novel_video_shots", "consumer.reference_asset_id = ra.id"},
			{"commerce_brands", "consumer.deleted_at IS NULL AND consumer.logo_reference_asset_id = ra.id"},
		}
		for _, consumer := range consumerQueries {
			if !db.Migrator().HasTable(consumer.table) {
				continue
			}
			var count int64
			if err := db.Table("reference_assets AS ra").
				Joins("JOIN "+consumer.table+" AS consumer ON "+consumer.condition).
				Where("ra.asset_key = ? AND ra.storage_scope IN ?", objectKey, scopes).
				Count(&count).Error; err != nil {
				return state, err
			}
			state.OtherActiveConsumers += count
		}
	}
	return state, nil
}

func (s *AssetService) ScheduleOrphanCleanup(ctx context.Context, input OrphanCleanupInput) (CommerceObjectCleanup, error) {
	now := s.currentTime()
	deleteAfter := input.DeleteAfter
	if deleteAfter.IsZero() {
		deleteAfter = now.Add(24 * time.Hour)
	}
	nextAttemptAt := deleteAfter
	cleanup := CommerceObjectCleanup{
		UserID: input.UserID, ProjectID: input.ProjectID, ReferenceAssetID: input.ReferenceAssetID,
		StorageScope: normalizedStorageScope(input.StorageScope), ObjectKey: strings.TrimSpace(input.ObjectKey),
		Reason: strings.TrimSpace(input.Reason), Status: CleanupStatusQueued,
		MaxAttempts: defaultCleanupMaxAttempts, NextAttemptAt: &nextAttemptAt, DeleteAfter: deleteAfter,
	}
	if cleanup.Reason == "" {
		cleanup.Reason = "orphaned_upload"
	}
	if cleanup.ObjectKey == "" {
		return CommerceObjectCleanup{}, fmt.Errorf("%w: cleanup object key is required", ErrInvalidInput)
	}
	if err := s.repository.DB().WithContext(ctx).Create(&cleanup).Error; err != nil {
		return CommerceObjectCleanup{}, err
	}
	return cleanup, nil
}

func (s *AssetService) DueCleanups(ctx context.Context, limit int) ([]CommerceObjectCleanup, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	now := s.currentTime()
	var cleanups []CommerceObjectCleanup
	err := s.repository.DB().WithContext(ctx).
		Where("object_deleted_at IS NULL AND delete_after <= ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?) AND status IN ?", now, now, []string{CleanupStatusQueued, CleanupStatusRetrying}).
		Order("delete_after asc, id asc").Limit(limit).Find(&cleanups).Error
	return cleanups, err
}

func (s *AssetService) RecordCleanupFailure(ctx context.Context, cleanupID uint, cleanupErr error) error {
	return s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cleanup CommerceObjectCleanup
		if err := tx.Where("id = ? AND object_deleted_at IS NULL", cleanupID).First(&cleanup).Error; err != nil {
			return mapNotFound(err)
		}
		cleanup.AttemptCount++
		if cleanup.MaxAttempts <= 0 {
			cleanup.MaxAttempts = defaultCleanupMaxAttempts
		}
		cleanup.LastError = ""
		if cleanupErr != nil {
			cleanup.LastError = cleanupErr.Error()
		}
		updates := map[string]any{
			"attempt_count": cleanup.AttemptCount,
			"max_attempts":  cleanup.MaxAttempts,
			"last_error":    cleanup.LastError,
		}
		if cleanup.AttemptCount >= cleanup.MaxAttempts {
			updates["status"] = CleanupStatusFailed
			updates["next_attempt_at"] = nil
		} else {
			delay := cleanupRetryDelay(cleanup.AttemptCount)
			nextAttemptAt := s.currentTime().Add(delay)
			updates["status"] = CleanupStatusRetrying
			updates["next_attempt_at"] = nextAttemptAt
		}
		return tx.Model(&CommerceObjectCleanup{}).Where("id = ?", cleanupID).Updates(updates).Error
	})
}

func (s *AssetService) RecordCleanupSuccess(ctx context.Context, cleanupID uint) error {
	return s.RecordCleanupSuccessWithGuard(ctx, cleanupID, "")
}

func (s *AssetService) RecordCleanupSuccessWithGuard(ctx context.Context, cleanupID uint, deleteToken string) error {
	now := s.currentTime()
	return s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cleanup CommerceObjectCleanup
		if err := tx.Where("id = ? AND object_deleted_at IS NULL", cleanupID).First(&cleanup).Error; err != nil {
			return mapNotFound(err)
		}
		if strings.TrimSpace(deleteToken) != "" {
			if err := s.CompleteObjectDeletionTx(tx, cleanup.UserID, cleanup.StorageScope, cleanup.ObjectKey, deleteToken); err != nil {
				return err
			}
		}
		if err := tx.Model(&CommerceObjectCleanup{}).Where("id = ?", cleanupID).Updates(map[string]any{
			"status": CleanupStatusSucceeded, "object_deleted_at": now, "next_attempt_at": nil, "last_error": "",
		}).Error; err != nil {
			return err
		}
		if cleanup.CommerceAssetID != nil {
			if err := tx.Model(&CommerceAsset{}).Where("id = ?", *cleanup.CommerceAssetID).Update("object_deleted_at", now).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *AssetService) RecordCleanupProtected(ctx context.Context, cleanupID uint, reason string) error {
	nextAttemptAt := s.currentTime().Add(24 * time.Hour)
	updates := map[string]any{
		"status": CleanupStatusRetrying, "next_attempt_at": nextAttemptAt,
		"last_error": strings.TrimSpace(reason),
	}
	result := s.repository.DB().WithContext(ctx).Model(&CommerceObjectCleanup{}).
		Where("id = ? AND object_deleted_at IS NULL", cleanupID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *AssetService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func normalizedStorageScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return StorageScopeDefault
	}
	return strings.TrimSpace(scope)
}

func cleanupRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exponent := math.Min(float64(attempt-1), 10)
	delay := time.Minute * time.Duration(math.Pow(2, exponent))
	if delay > maxCleanupRetryDelay {
		return maxCleanupRetryDelay
	}
	return delay
}
