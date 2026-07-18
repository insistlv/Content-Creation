package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"image-agent/internal/app/ecommerce"
)

type commerceCreditLedger struct{}

func newCommerceCreditLedger() ecommerce.CreditLedger {
	return &commerceCreditLedger{}
}

func (l *commerceCreditLedger) ReserveTx(ctx context.Context, tx *gorm.DB, req ecommerce.ReserveCreditsRequest) (ecommerce.CreditReservationSnapshot, error) {
	if tx == nil || req.UserID == 0 || req.ProjectID == 0 || req.Amount <= 0 || strings.TrimSpace(req.IdempotencyKey) == "" {
		return ecommerce.CreditReservationSnapshot{}, ecommerce.ErrInvalidInput
	}
	var existing ecommerce.CommerceCreditReservation
	err := tx.WithContext(ctx).Where("user_id = ? AND idempotency_key = ?", req.UserID, req.IdempotencyKey).First(&existing).Error
	if err == nil {
		if existing.ProjectID != req.ProjectID || existing.ScopeType != req.ScopeType || existing.ScopeKey != req.ScopeKey || existing.TotalCredits != req.Amount || !sameOptionalUint(existing.BatchID, req.BatchID) {
			return ecommerce.CreditReservationSnapshot{}, ecommerce.ErrIdempotencyConflict
		}
		return creditReservationSnapshot(ctx, tx, existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ecommerce.CreditReservationSnapshot{}, err
	}

	result := tx.WithContext(ctx).Model(&CreditBalance{}).
		Where("user_id = ? AND available_credits >= ?", req.UserID, req.Amount).
		Updates(map[string]any{
			"available_credits": gorm.Expr("available_credits - ?", req.Amount),
			"reserved_credits":  gorm.Expr("reserved_credits + ?", req.Amount),
		})
	if result.Error != nil {
		return ecommerce.CreditReservationSnapshot{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ecommerce.CreditReservationSnapshot{}, ecommerce.ErrCreditsInsufficient
	}
	reservation := ecommerce.CommerceCreditReservation{
		UserID: req.UserID, ProjectID: req.ProjectID, BatchID: req.BatchID,
		ScopeType: req.ScopeType, ScopeKey: req.ScopeKey, IdempotencyKey: req.IdempotencyKey,
		Status: "reserved", TotalCredits: req.Amount, ReservedCredits: req.Amount,
	}
	if err := tx.WithContext(ctx).Create(&reservation).Error; err != nil {
		if isCommerceUniqueError(err) {
			return ecommerce.CreditReservationSnapshot{}, ecommerce.ErrIdempotencyConflict
		}
		return ecommerce.CreditReservationSnapshot{}, err
	}
	var balance CreditBalance
	if err := tx.WithContext(ctx).Where("user_id = ?", req.UserID).First(&balance).Error; err != nil {
		return ecommerce.CreditReservationSnapshot{}, err
	}
	transaction := CreditTransaction{
		UserID: req.UserID, Type: CreditTransactionTypeCommerceReserve, Amount: -req.Amount,
		BalanceAfter: balance.AvailableCredits, ReservedAfter: balance.ReservedCredits,
		IdempotencyKey: req.IdempotencyKey, Reason: "commerce credit reservation",
		RelatedType: "commerce_credit_reservation", RelatedID: reservation.ID,
	}
	if err := tx.WithContext(ctx).Create(&transaction).Error; err != nil {
		return ecommerce.CreditReservationSnapshot{}, err
	}
	return ecommerce.CreditReservationSnapshot{
		ReservationID: reservation.ID, UserID: req.UserID, BatchID: req.BatchID,
		ScopeType: req.ScopeType, ScopeKey: req.ScopeKey, ReservedCredits: req.Amount,
		AvailableCredits: balance.AvailableCredits,
	}, nil
}

func (l *commerceCreditLedger) SettleItemTx(ctx context.Context, tx *gorm.DB, req ecommerce.SettleCreditsRequest) error {
	if tx == nil || req.UserID == 0 || req.ProjectID == 0 || req.BatchID == 0 || req.ReservationID == 0 || req.GenerationItemID == 0 || req.HeldCredits < 0 {
		return ecommerce.ErrInvalidInput
	}
	actual := req.ActualCredits
	if actual < 0 {
		actual = 0
	}
	settledCredits := actual
	releasedCredits := 0
	anomalyCode := ""
	if actual < req.HeldCredits {
		releasedCredits = req.HeldCredits - actual
	} else if actual > req.HeldCredits {
		settledCredits = req.HeldCredits
		anomalyCode = "actual_exceeds_hold"
	}
	settlement := ecommerce.CommerceCreditSettlement{
		UserID: req.UserID, ProjectID: req.ProjectID, BatchID: req.BatchID,
		ReservationID: req.ReservationID, GenerationItemID: req.GenerationItemID,
		IdempotencyKey: req.IdempotencyKey, HeldCredits: req.HeldCredits, ActualCredits: actual,
		SettledCredits: settledCredits, ReleasedCredits: releasedCredits, AnomalyCode: anomalyCode,
	}
	claimed, err := claimCommerceSettlement(ctx, tx, settlement)
	if err != nil || !claimed {
		return err
	}
	item, reservation, err := loadCommerceSettlementScope(ctx, tx, req.UserID, req.ProjectID, req.BatchID, req.ReservationID, req.GenerationItemID, req.HeldCredits)
	if err != nil {
		return err
	}
	if err := moveReservedCredits(ctx, tx, req.UserID, req.HeldCredits, releasedCredits); err != nil {
		return err
	}
	if err := updateCommerceReservationSettlement(ctx, tx, reservation, req.HeldCredits, settledCredits, releasedCredits); err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Model(&ecommerce.CommerceGenerationItem{}).
		Where("id = ? AND user_id = ? AND reservation_id = ? AND reserved_credits = ?", item.ID, req.UserID, req.ReservationID, req.HeldCredits).
		Updates(map[string]any{"settled_credits": settledCredits, "released_credits": releasedCredits}).Error; err != nil {
		return err
	}
	if releasedCredits > 0 {
		if err := createCommerceReleaseTransaction(ctx, tx, req.UserID, req.IdempotencyKey+":release", req.GenerationItemID, releasedCredits, "commerce unused hold released"); err != nil {
			return err
		}
	}
	if anomalyCode != "" {
		batchID := req.BatchID
		metadata, _ := json.Marshal(map[string]any{"held_credits": req.HeldCredits, "actual_credits": actual})
		if err := tx.WithContext(ctx).Create(&ecommerce.CommerceEvent{
			UserID: req.UserID, ProjectID: req.ProjectID, BatchID: &batchID,
			EntityType: "generation_item", EntityID: req.GenerationItemID,
			Pipeline: item.Pipeline, RecipeKey: item.RecipeKey,
			EventType: "billing_actual_exceeds_hold", MetadataJSON: string(metadata),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (l *commerceCreditLedger) ReleaseItemTx(ctx context.Context, tx *gorm.DB, req ecommerce.ReleaseCreditsRequest) error {
	if tx == nil || req.UserID == 0 || req.ProjectID == 0 || req.BatchID == 0 || req.ReservationID == 0 || req.GenerationItemID == 0 || req.HeldCredits < 0 {
		return ecommerce.ErrInvalidInput
	}
	settlement := ecommerce.CommerceCreditSettlement{
		UserID: req.UserID, ProjectID: req.ProjectID, BatchID: req.BatchID,
		ReservationID: req.ReservationID, GenerationItemID: req.GenerationItemID,
		IdempotencyKey: req.IdempotencyKey, HeldCredits: req.HeldCredits,
		SettledCredits: 0, ReleasedCredits: req.HeldCredits,
	}
	claimed, err := claimCommerceSettlement(ctx, tx, settlement)
	if err != nil || !claimed {
		return err
	}
	_, reservation, err := loadCommerceSettlementScope(ctx, tx, req.UserID, req.ProjectID, req.BatchID, req.ReservationID, req.GenerationItemID, req.HeldCredits)
	if err != nil {
		return err
	}
	if err := moveReservedCredits(ctx, tx, req.UserID, req.HeldCredits, req.HeldCredits); err != nil {
		return err
	}
	if err := updateCommerceReservationSettlement(ctx, tx, reservation, req.HeldCredits, 0, req.HeldCredits); err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Model(&ecommerce.CommerceGenerationItem{}).
		Where("id = ? AND user_id = ? AND reservation_id = ? AND reserved_credits = ?", req.GenerationItemID, req.UserID, req.ReservationID, req.HeldCredits).
		Updates(map[string]any{"settled_credits": 0, "released_credits": req.HeldCredits}).Error; err != nil {
		return err
	}
	return createCommerceReleaseTransaction(ctx, tx, req.UserID, req.IdempotencyKey, req.GenerationItemID, req.HeldCredits, req.Reason)
}

func creditReservationSnapshot(ctx context.Context, tx *gorm.DB, reservation ecommerce.CommerceCreditReservation) (ecommerce.CreditReservationSnapshot, error) {
	var balance CreditBalance
	if err := tx.WithContext(ctx).Where("user_id = ?", reservation.UserID).First(&balance).Error; err != nil {
		return ecommerce.CreditReservationSnapshot{}, err
	}
	return ecommerce.CreditReservationSnapshot{
		ReservationID: reservation.ID, UserID: reservation.UserID, BatchID: reservation.BatchID,
		ScopeType: reservation.ScopeType, ScopeKey: reservation.ScopeKey,
		ReservedCredits: reservation.ReservedCredits, SettledCredits: reservation.SettledCredits,
		ReleasedCredits: reservation.ReleasedCredits, AvailableCredits: balance.AvailableCredits,
	}, nil
}

func claimCommerceSettlement(ctx context.Context, tx *gorm.DB, settlement ecommerce.CommerceCreditSettlement) (bool, error) {
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "generation_item_id"}},
		DoNothing: true,
	}).Create(&settlement)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	var existing ecommerce.CommerceCreditSettlement
	if err := tx.WithContext(ctx).Where("generation_item_id = ?", settlement.GenerationItemID).First(&existing).Error; err != nil {
		return false, err
	}
	if existing.UserID != settlement.UserID || existing.ProjectID != settlement.ProjectID || existing.BatchID != settlement.BatchID || existing.ReservationID != settlement.ReservationID {
		return false, ecommerce.ErrCreditInvariant
	}
	return false, nil
}

func loadCommerceSettlementScope(ctx context.Context, tx *gorm.DB, userID, projectID, batchID, reservationID, itemID uint, held int) (ecommerce.CommerceGenerationItem, ecommerce.CommerceCreditReservation, error) {
	var item ecommerce.CommerceGenerationItem
	err := tx.WithContext(ctx).
		Where("id = ? AND user_id = ? AND project_id = ? AND batch_id = ? AND reservation_id = ? AND reserved_credits = ?", itemID, userID, projectID, batchID, reservationID, held).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ecommerce.CommerceGenerationItem{}, ecommerce.CommerceCreditReservation{}, ecommerce.ErrCreditInvariant
	}
	if err != nil {
		return ecommerce.CommerceGenerationItem{}, ecommerce.CommerceCreditReservation{}, err
	}
	var reservation ecommerce.CommerceCreditReservation
	err = tx.WithContext(ctx).
		Where("id = ? AND user_id = ? AND project_id = ?", reservationID, userID, projectID).
		First(&reservation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && reservation.BatchID != nil && *reservation.BatchID != batchID {
		return ecommerce.CommerceGenerationItem{}, ecommerce.CommerceCreditReservation{}, ecommerce.ErrCreditInvariant
	}
	return item, reservation, err
}

func moveReservedCredits(ctx context.Context, tx *gorm.DB, userID uint, held, released int) error {
	result := tx.WithContext(ctx).Model(&CreditBalance{}).
		Where("user_id = ? AND reserved_credits >= ?", userID, held).
		Updates(map[string]any{
			"available_credits": gorm.Expr("available_credits + ?", released),
			"reserved_credits":  gorm.Expr("reserved_credits - ?", held),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ecommerce.ErrCreditInvariant
	}
	return nil
}

func updateCommerceReservationSettlement(ctx context.Context, tx *gorm.DB, reservation ecommerce.CommerceCreditReservation, held, settled, released int) error {
	result := tx.WithContext(ctx).Model(&ecommerce.CommerceCreditReservation{}).
		Where("id = ? AND user_id = ? AND reserved_credits >= ?", reservation.ID, reservation.UserID, held).
		Updates(map[string]any{
			"reserved_credits": gorm.Expr("reserved_credits - ?", held),
			"settled_credits":  gorm.Expr("settled_credits + ?", settled),
			"released_credits": gorm.Expr("released_credits + ?", released),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ecommerce.ErrCreditInvariant
	}
	var updated ecommerce.CommerceCreditReservation
	if err := tx.WithContext(ctx).First(&updated, reservation.ID).Error; err != nil {
		return err
	}
	if updated.ReservedCredits == 0 {
		now := time.Now().UTC()
		status := "settled"
		if updated.SettledCredits == 0 {
			status = "released"
		}
		return tx.WithContext(ctx).Model(&ecommerce.CommerceCreditReservation{}).Where("id = ? AND user_id = ?", updated.ID, updated.UserID).
			Updates(map[string]any{"status": status, "completed_at": now}).Error
	}
	return nil
}

func createCommerceReleaseTransaction(ctx context.Context, tx *gorm.DB, userID uint, key string, itemID uint, released int, reason string) error {
	if released == 0 {
		return nil
	}
	var balance CreditBalance
	if err := tx.WithContext(ctx).Where("user_id = ?", userID).First(&balance).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Create(&CreditTransaction{
		UserID: userID, Type: CreditTransactionTypeCommerceRelease, Amount: released,
		BalanceAfter: balance.AvailableCredits, ReservedAfter: balance.ReservedCredits,
		IdempotencyKey: key, Reason: reason, RelatedType: "commerce_generation_item", RelatedID: itemID,
	}).Error
}

func sameOptionalUint(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func isCommerceUniqueError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}

type commercePricingSnapshotProvider struct {
	app *App
}

func (p commercePricingSnapshotProvider) SnapshotForEstimate(ctx context.Context, tx *gorm.DB, _ uint, _ uint, definition ecommerce.RecipeDefinition, req ecommerce.EstimateBatchRequest) (ecommerce.PricingSnapshot, error) {
	if p.app == nil {
		return ecommerce.PricingSnapshot{}, ecommerce.ErrRecipeModelUnavailable
	}
	settings, err := p.app.loadSettings()
	if err != nil {
		return ecommerce.PricingSnapshot{}, err
	}
	candidates, err := p.app.modelCenterCandidatesForGeneration(settings, ModelConfigTypeImage, 0)
	if err != nil {
		return ecommerce.PricingSnapshot{}, err
	}
	if len(candidates) == 0 {
		return ecommerce.PricingSnapshot{}, ecommerce.ErrRecipeModelUnavailable
	}
	entries := make([]ecommerce.PricingSnapshotEntry, 0, len(candidates))
	versionParts := make([]string, 0, len(candidates))
	for index, candidate := range candidates {
		model := candidate.Model
		if model.Status != ModelCenterStatusOnline || model.Visibility != ModelCenterVisibilityPublic || model.DefaultCreditsCost <= 0 {
			continue
		}
		var capabilities []string
		_ = json.Unmarshal([]byte(model.CapabilityTagsJSON), &capabilities)
		if !containsStringValue(capabilities, "image") {
			capabilities = append(capabilities, "image")
		}
		sort.Strings(capabilities)
		entries = append(entries, ecommerce.PricingSnapshotEntry{
			Pipeline: definition.Pipeline, RecipeKey: definition.Key, QualityTier: req.QualityTier,
			ModelID: model.ID, ModelName: model.Name, ChannelID: candidate.Channel.ID, ProviderID: candidate.Provider.ID,
			RuntimeModel: candidate.Channel.RuntimeModel, Endpoint: candidate.Channel.Endpoint, RouteOrder: index,
			RequiredCapabilities: capabilities, Credits: model.DefaultCreditsCost,
		})
		versionParts = append(versionParts, fmt.Sprintf("%d:%d:%d:%s:%d", model.ID, candidate.Channel.ID, candidate.Provider.ID, candidate.Channel.RuntimeModel, model.DefaultCreditsCost))
	}
	if len(entries) == 0 {
		return ecommerce.PricingSnapshot{}, ecommerce.ErrRecipeModelUnavailable
	}
	return ecommerce.PricingSnapshot{Version: fmt.Sprintf("catalog-%x", sha256Bytes(strings.Join(versionParts, "|"))[:8]), Entries: entries}, nil
}

func sha256Bytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
