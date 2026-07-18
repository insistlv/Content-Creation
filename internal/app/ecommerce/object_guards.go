package ecommerce

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ObjectGuardStateActive   = "active"
	ObjectGuardStateDeleting = "deleting"
	ObjectGuardStateDeleted  = "deleted"
)

var (
	ErrObjectGuardUnavailable = errors.New("commerce object guard unavailable")
	ErrObjectDeletionBusy     = errors.New("commerce object deletion already in progress")
)

type ObjectDeletionLease struct {
	Token      string
	References ObjectReferenceState
	Acquired   bool
}

func (s *AssetService) EnsureObjectGuard(ctx context.Context, userID uint, storageScope, objectKey string) error {
	if normalizedStorageScope(storageScope) != StorageScopeCommercePrivate {
		return nil
	}
	if userID == 0 || strings.TrimSpace(objectKey) == "" {
		return fmt.Errorf("%w: object guard identity is required", ErrInvalidInput)
	}
	guard := CommerceObjectGuard{
		UserID: userID, StorageScope: StorageScopeCommercePrivate,
		ObjectKey: strings.TrimSpace(objectKey), State: ObjectGuardStateActive,
	}
	db := s.repository.DB().WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; err != nil {
		return err
	}
	var current CommerceObjectGuard
	if err := db.Where("user_id = ? AND storage_scope = ? AND object_key = ?", userID, StorageScopeCommercePrivate, guard.ObjectKey).First(&current).Error; err != nil {
		return err
	}
	if current.State != ObjectGuardStateActive {
		return ErrObjectGuardUnavailable
	}
	return nil
}

func (s *AssetService) BeginObjectDeletion(ctx context.Context, userID uint, storageScope, objectKey string, excludeCommerceAssetID *uint) (ObjectDeletionLease, error) {
	var lease ObjectDeletionLease
	storageScope = normalizedStorageScope(storageScope)
	objectKey = strings.TrimSpace(objectKey)
	if storageScope != StorageScopeCommercePrivate {
		references, err := s.inspectObjectReferencesDB(ctx, s.repository.DB(), storageScope, objectKey, excludeCommerceAssetID)
		if err != nil {
			return lease, err
		}
		lease.References = references
		lease.Acquired = true
		return lease, nil
	}
	if err := s.EnsureObjectGuard(ctx, userID, storageScope, objectKey); err != nil {
		return lease, err
	}
	token, err := newObjectDeletionToken()
	if err != nil {
		return lease, err
	}
	err = s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var guard CommerceObjectGuard
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND storage_scope = ? AND object_key = ?", userID, storageScope, objectKey).
			First(&guard).Error; err != nil {
			return err
		}
		if guard.State != ObjectGuardStateActive {
			return ErrObjectDeletionBusy
		}
		references, err := s.inspectObjectReferencesDB(ctx, tx, storageScope, objectKey, excludeCommerceAssetID)
		if err != nil {
			return err
		}
		lease.References = references
		if references.HasReferences() {
			return nil
		}
		result := tx.Model(&CommerceObjectGuard{}).
			Where("id = ? AND state = ?", guard.ID, ObjectGuardStateActive).
			Updates(map[string]any{"state": ObjectGuardStateDeleting, "delete_token": token})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrObjectDeletionBusy
		}
		lease.Token = token
		lease.Acquired = true
		return nil
	})
	return lease, err
}

func (s *AssetService) CompleteObjectDeletionTx(tx *gorm.DB, userID uint, storageScope, objectKey, token string) error {
	if normalizedStorageScope(storageScope) != StorageScopeCommercePrivate {
		return nil
	}
	result := tx.Model(&CommerceObjectGuard{}).
		Where("user_id = ? AND storage_scope = ? AND object_key = ? AND state = ? AND delete_token = ?",
			userID, StorageScopeCommercePrivate, strings.TrimSpace(objectKey), ObjectGuardStateDeleting, token).
		Updates(map[string]any{"state": ObjectGuardStateDeleted, "delete_token": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrObjectDeletionBusy
	}
	return nil
}

func (s *AssetService) ReleaseObjectDeletion(ctx context.Context, userID uint, storageScope, objectKey, token string) error {
	if normalizedStorageScope(storageScope) != StorageScopeCommercePrivate {
		return nil
	}
	result := s.repository.DB().WithContext(ctx).Model(&CommerceObjectGuard{}).
		Where("user_id = ? AND storage_scope = ? AND object_key = ? AND state = ? AND delete_token = ?",
			userID, StorageScopeCommercePrivate, strings.TrimSpace(objectKey), ObjectGuardStateDeleting, token).
		Updates(map[string]any{"state": ObjectGuardStateActive, "delete_token": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrObjectDeletionBusy
	}
	return nil
}

func newObjectDeletionToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
