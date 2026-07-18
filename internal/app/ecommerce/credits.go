package ecommerce

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrCreditsInsufficient   = errors.New("commerce credits insufficient")
	ErrIdempotencyConflict   = errors.New("commerce idempotency conflict")
	ErrPricingSnapshotStale  = errors.New("commerce pricing snapshot stale")
	ErrCreditInvariant       = errors.New("commerce credit invariant violated")
	ErrLeaseMismatch         = errors.New("commerce job lease mismatch")
	ErrInvalidItemTransition = errors.New("commerce item transition invalid")
)

type GormPricingSnapshotStore struct{}

func NewGormPricingSnapshotStore() *GormPricingSnapshotStore {
	return &GormPricingSnapshotStore{}
}

func (s *GormPricingSnapshotStore) IssueTx(ctx context.Context, tx *gorm.DB, snapshot PricingSnapshot) (PricingSnapshot, error) {
	if tx == nil || snapshot.UserID == 0 || snapshot.ProjectID == 0 || strings.TrimSpace(snapshot.RequestDigest) == "" {
		return PricingSnapshot{}, ErrInvalidInput
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	if snapshot.ExpiresAt.IsZero() || !snapshot.ExpiresAt.After(snapshot.CreatedAt) {
		return PricingSnapshot{}, invalidField("pricing_expires_at", "pricing snapshot expiration must be after creation")
	}
	if snapshot.Status == "" {
		snapshot.Status = "issued"
	}
	if snapshot.Status != "issued" {
		return PricingSnapshot{}, invalidField("pricing_status", "new pricing snapshot must be issued")
	}
	if snapshot.ID == "" {
		id, err := opaqueSnapshotID()
		if err != nil {
			return PricingSnapshot{}, err
		}
		snapshot.ID = id
	}
	payload, err := EncodeJSON(snapshotPayload(snapshot))
	if err != nil {
		return PricingSnapshot{}, fmt.Errorf("encode pricing snapshot: %w", err)
	}
	hash := sha256.Sum256([]byte(payload))
	snapshot.SnapshotHash = hex.EncodeToString(hash[:])
	row := CommercePricingSnapshot{
		ID: snapshot.ID, UserID: snapshot.UserID, ProjectID: snapshot.ProjectID,
		RequestDigest: snapshot.RequestDigest, SnapshotJSON: payload, SnapshotHash: snapshot.SnapshotHash,
		Version: snapshot.Version, Status: snapshot.Status, ExpiresAt: snapshot.ExpiresAt, CreatedAt: snapshot.CreatedAt,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return PricingSnapshot{}, err
	}
	return snapshot, nil
}

func (s *GormPricingSnapshotStore) ResolveForSubmitTx(
	ctx context.Context,
	tx *gorm.DB,
	userID, projectID uint,
	snapshotID, requestDigest string,
	now time.Time,
) (PricingSnapshot, error) {
	if tx == nil || userID == 0 || projectID == 0 || strings.TrimSpace(snapshotID) == "" || strings.TrimSpace(requestDigest) == "" {
		logPricingSnapshotResolutionFailure("invalid_input", userID, projectID, snapshotID, requestDigest)
		return PricingSnapshot{}, ErrPricingSnapshotStale
	}
	var row CommercePricingSnapshot
	err := tx.WithContext(ctx).Where("id = ?", snapshotID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logPricingSnapshotResolutionFailure("not_found", userID, projectID, snapshotID, requestDigest)
		return PricingSnapshot{}, ErrPricingSnapshotStale
	}
	if err != nil {
		return PricingSnapshot{}, err
	}
	if reason := classifyPricingSnapshotResolutionFailure(true, row, userID, projectID, requestDigest, now); reason != "valid" {
		logPricingSnapshotResolutionFailure(reason, userID, projectID, snapshotID, requestDigest)
		return PricingSnapshot{}, ErrPricingSnapshotStale
	}
	var payload persistedPricingSnapshot
	hash := sha256.Sum256([]byte(row.SnapshotJSON))
	if hex.EncodeToString(hash[:]) != row.SnapshotHash {
		logPricingSnapshotResolutionFailure("hash_mismatch", userID, projectID, snapshotID, requestDigest)
		return PricingSnapshot{}, ErrPricingSnapshotStale
	}
	if err := DecodeJSON(row.SnapshotJSON, &payload); err != nil {
		return PricingSnapshot{}, fmt.Errorf("decode pricing snapshot %s: %w", row.ID, err)
	}
	if payload.Version != row.Version || payload.RequestDigest != row.RequestDigest || payload.UserID != row.UserID || payload.ProjectID != row.ProjectID {
		logPricingSnapshotResolutionFailure("payload_mismatch", userID, projectID, snapshotID, requestDigest)
		return PricingSnapshot{}, ErrPricingSnapshotStale
	}
	snapshot := PricingSnapshot{
		ID: row.ID, Version: payload.Version, RequestDigest: payload.RequestDigest, SnapshotHash: row.SnapshotHash,
		Status: row.Status, UserID: payload.UserID, ProjectID: payload.ProjectID,
		Entries: payload.Entries, CreatedAt: payload.CreatedAt, ExpiresAt: payload.ExpiresAt,
	}
	return snapshot, nil
}

func classifyPricingSnapshotResolutionFailure(found bool, row CommercePricingSnapshot, userID, projectID uint, requestDigest string, now time.Time) string {
	if !found {
		return "not_found"
	}
	if row.UserID != userID || row.ProjectID != projectID {
		return "ownership_mismatch"
	}
	if row.RequestDigest != requestDigest {
		return "request_digest_mismatch"
	}
	if row.Status != "issued" {
		if row.Status == "consumed" {
			return "status_consumed"
		}
		return "status_unavailable"
	}
	if !row.ExpiresAt.After(now) {
		return "expired"
	}
	return "valid"
}

func logPricingSnapshotResolutionFailure(reason string, userID, projectID uint, snapshotID, requestDigest string) {
	digestPrefix := strings.TrimSpace(requestDigest)
	if len(digestPrefix) > 12 {
		digestPrefix = digestPrefix[:12]
	}
	log.Printf("commerce_pricing_snapshot_stale reason=%s user_id=%d project_id=%d snapshot_id=%q request_digest_prefix=%q", reason, userID, projectID, strings.TrimSpace(snapshotID), digestPrefix)
}

type persistedPricingSnapshot struct {
	Version       string                 `json:"version"`
	RequestDigest string                 `json:"request_digest"`
	UserID        uint                   `json:"user_id"`
	ProjectID     uint                   `json:"project_id"`
	Entries       []PricingSnapshotEntry `json:"entries"`
	CreatedAt     time.Time              `json:"created_at"`
	ExpiresAt     time.Time              `json:"expires_at"`
}

func snapshotPayload(snapshot PricingSnapshot) persistedPricingSnapshot {
	return persistedPricingSnapshot{
		Version: snapshot.Version, RequestDigest: snapshot.RequestDigest,
		UserID: snapshot.UserID, ProjectID: snapshot.ProjectID,
		Entries:   append([]PricingSnapshotEntry(nil), snapshot.Entries...),
		CreatedAt: snapshot.CreatedAt, ExpiresAt: snapshot.ExpiresAt,
	}
}

func opaqueSnapshotID() (string, error) {
	var value [18]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate pricing snapshot id: %w", err)
	}
	return "ps_" + hex.EncodeToString(value[:]), nil
}
