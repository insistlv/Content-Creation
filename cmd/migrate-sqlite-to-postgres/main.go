package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"image-agent/internal/app"
	"image-agent/internal/app/ecommerce"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type copiedTable struct {
	name  string
	count int64
}

func main() {
	sqlitePath := os.Getenv("SQLITE_PATH")
	databaseURL := os.Getenv("DATABASE_URL")
	if sqlitePath == "" || databaseURL == "" {
		log.Fatal("SQLITE_PATH and DATABASE_URL are required")
	}

	src, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	dst, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}

	if err := migrate(dst); err != nil {
		log.Fatalf("migrate postgres schema: %v", err)
	}

	copied, err := copyBusinessData(src, dst)
	if err != nil {
		log.Fatalf("copy data: %v", err)
	}

	if err := app.SeedRBACAndBootstrapAdmin(dst, os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")); err != nil {
		log.Fatalf("seed rbac/bootstrap admin: %v", err)
	}

	if err := resetSequences(dst); err != nil {
		log.Fatalf("reset postgres sequences: %v", err)
	}
	if err := verifyMigration(src, dst, copied); err != nil {
		log.Fatalf("verify migration: %v", err)
	}

	log.Printf("migration completed: %d tables copied", len(copied))
	for _, table := range copied {
		log.Printf("  %s: %d rows", table.name, table.count)
	}
}

func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&app.AppSettings{},
		&app.ModelConfig{},
		&app.Permission{},
		&app.Role{},
		&app.AdminUser{},
		&app.AdminSession{},
		&app.AdminAuditLog{},
		&app.SystemAnnouncement{},
		&app.Invite{},
		&app.InviteRedemption{},
		&app.UserRole{},
		&app.User{},
		&app.UserSession{},
		&app.CreditBalance{},
		&app.CreditTransaction{},
		&app.Package{},
		&app.PurchaseIntent{},
		&app.PurchaseIntentNote{},
		&app.FinanceOrder{},
		&app.FinanceRefund{},
		&app.FinanceInvoice{},
		&app.Work{},
		&app.ReferenceAsset{},
		&app.GenerationRecord{},
		&app.VideoGenerationRecord{},
		&app.GenerationReferenceAsset{},
	); err != nil {
		return err
	}
	if err := ecommerce.ApplyFoundationMigrations(context.Background(), db); err != nil {
		return fmt.Errorf("apply commerce foundation migrations: %w", err)
	}
	return nil
}

func copyBusinessData(src, dst *gorm.DB) ([]copiedTable, error) {
	var copied []copiedTable
	copyOne := func(table copiedTable, err error) error {
		if err != nil {
			return err
		}
		copied = append(copied, table)
		return nil
	}

	if err := copyOne(copyNamedTable[app.AppSettings](src, dst, "app_settings")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.ModelConfig](src, dst, "model_configs")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.SystemAnnouncement](src, dst, "system_announcements")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.Invite](src, dst, "invites")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.InviteRedemption](src, dst, "invite_redemptions")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.UserRole](src, dst, "user_roles")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.User](src, dst, "users")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.UserSession](src, dst, "user_sessions")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.CreditBalance](src, dst, "credit_balances")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.CreditTransaction](src, dst, "credit_transactions")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.Package](src, dst, "packages")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.PurchaseIntent](src, dst, "purchase_intents")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.PurchaseIntentNote](src, dst, "purchase_intent_notes")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.FinanceOrder](src, dst, "finance_orders")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.FinanceRefund](src, dst, "finance_refunds")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.FinanceInvoice](src, dst, "finance_invoices")); err != nil {
		return nil, err
	}
	if err := copyOne(copyCommerceObjectGuards(src, dst)); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.Work](src, dst, "works")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.ReferenceAsset](src, dst, "reference_assets")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.GenerationRecord](src, dst, "generation_records")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.VideoGenerationRecord](src, dst, "video_generation_records")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[app.GenerationReferenceAsset](src, dst, "generation_reference_assets")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceBrand](src, dst, "commerce_brands")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceProduct](src, dst, "commerce_products")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceSKU](src, dst, "commerce_skus")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceProject](src, dst, "commerce_projects")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceAsset](src, dst, "commerce_assets")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceCreativeSpec](src, dst, "commerce_creative_specs")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceIdempotencyRecord](src, dst, "commerce_idempotency_records")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceAIInvocation](src, dst, "commerce_ai_invocations")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceGenerationBatch](src, dst, "commerce_generation_batches")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceGenerationItem](src, dst, "commerce_generation_items")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceJob](src, dst, "commerce_jobs")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceCreditReservation](src, dst, "commerce_credit_reservations")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommercePricingSnapshot](src, dst, "commerce_pricing_snapshots")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceCreditSettlement](src, dst, "commerce_credit_settlements")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceEvent](src, dst, "commerce_events")); err != nil {
		return nil, err
	}
	if err := copyOne(copyNamedTable[ecommerce.CommerceObjectCleanup](src, dst, "commerce_object_cleanups")); err != nil {
		return nil, err
	}
	return copied, nil
}

func copyCommerceObjectGuards(src, dst *gorm.DB) (copiedTable, error) {
	guards := make(map[string]ecommerce.CommerceObjectGuard)
	explicitGuards := make(map[string]struct{})
	if src.Migrator().HasTable("commerce_object_guards") {
		var existing []ecommerce.CommerceObjectGuard
		if err := src.Find(&existing).Error; err != nil {
			return copiedTable{}, fmt.Errorf("commerce_object_guards: %w", err)
		}
		for _, guard := range existing {
			identity := fmt.Sprintf("%d\x00%s\x00%s", guard.UserID, guard.StorageScope, guard.ObjectKey)
			guards[identity] = guard
			explicitGuards[identity] = struct{}{}
		}
	}
	candidates, err := collectCommerceObjectGuardCandidates(src)
	if err != nil {
		return copiedTable{}, err
	}
	for identity, candidate := range candidates {
		if _, explicit := explicitGuards[identity]; explicit {
			continue
		}
		state := ecommerce.ObjectGuardStateDeleted
		if candidate.Active {
			state = ecommerce.ObjectGuardStateActive
		}
		guards[identity] = ecommerce.CommerceObjectGuard{
			UserID: candidate.UserID, StorageScope: ecommerce.StorageScopeCommercePrivate,
			ObjectKey: candidate.ObjectKey, State: state,
		}
	}
	rows := make([]ecommerce.CommerceObjectGuard, 0, len(guards))
	for _, guard := range guards {
		rows = append(rows, guard)
	}
	if len(rows) > 0 {
		if err := dst.CreateInBatches(rows, 100).Error; err != nil {
			return copiedTable{}, fmt.Errorf("commerce_object_guards: %w", err)
		}
	}
	return copiedTable{name: "commerce_object_guards", count: int64(len(rows))}, nil
}

type commerceObjectGuardCandidate struct {
	UserID    uint
	ObjectKey string
	Active    bool
}

func collectCommerceObjectGuardCandidates(src *gorm.DB) (map[string]commerceObjectGuardCandidate, error) {
	candidates := make(map[string]commerceObjectGuardCandidate)
	merge := func(userID uint, objectKey string, active bool) {
		if userID == 0 || strings.TrimSpace(objectKey) == "" {
			return
		}
		objectKey = strings.TrimSpace(objectKey)
		identity := fmt.Sprintf("%d\x00%s\x00%s", userID, ecommerce.StorageScopeCommercePrivate, objectKey)
		current := candidates[identity]
		current.UserID = userID
		current.ObjectKey = objectKey
		current.Active = current.Active || active
		candidates[identity] = current
	}
	if src.Migrator().HasTable("reference_assets") && src.Migrator().HasColumn("reference_assets", "storage_scope") {
		var references []app.ReferenceAsset
		if err := src.Unscoped().Where("storage_scope = ?", ecommerce.StorageScopeCommercePrivate).Find(&references).Error; err != nil {
			return nil, fmt.Errorf("backfill guards from reference_assets: %w", err)
		}
		for _, reference := range references {
			merge(reference.UserID, reference.AssetKey, !reference.DeletedAt.Valid)
		}
	}
	if src.Migrator().HasTable("works") && src.Migrator().HasColumn("works", "storage_scope") {
		var works []app.Work
		if err := src.Unscoped().Where("storage_scope = ?", ecommerce.StorageScopeCommercePrivate).Find(&works).Error; err != nil {
			return nil, fmt.Errorf("backfill guards from works: %w", err)
		}
		for _, work := range works {
			merge(work.UserID, work.AssetKey, !work.DeletedAt.Valid)
		}
	}
	if src.Migrator().HasTable("generation_records") && src.Migrator().HasColumn("generation_records", "storage_scope") {
		var records []app.GenerationRecord
		if err := src.Where("storage_scope = ?", ecommerce.StorageScopeCommercePrivate).Find(&records).Error; err != nil {
			return nil, fmt.Errorf("backfill guards from generation_records: %w", err)
		}
		for _, record := range records {
			merge(record.UserID, record.AssetKey, true)
		}
	}
	if !src.Migrator().HasTable("reference_assets") {
		return candidates, nil
	}
	consumerQueries := []struct {
		table, column string
		softDelete    bool
		objectDelete  bool
	}{
		{"generation_reference_assets", "reference_asset_id", false, false},
		{"commerce_assets", "reference_asset_id", true, true},
		{"user_video_style_templates", "reference_asset_id", true, false},
		{"couple_albums", "male_reference_asset_id", true, false},
		{"couple_albums", "female_reference_asset_id", true, false},
		{"novel_video_shots", "reference_asset_id", false, false},
		{"commerce_brands", "logo_reference_asset_id", true, false},
	}
	for _, consumer := range consumerQueries {
		if !src.Migrator().HasTable(consumer.table) || !src.Migrator().HasColumn(consumer.table, consumer.column) {
			continue
		}
		var rows []struct {
			UserID    uint
			ObjectKey string
			Active    bool
		}
		activeConditions := make([]string, 0, 2)
		if consumer.softDelete && src.Migrator().HasColumn(consumer.table, "deleted_at") {
			activeConditions = append(activeConditions, "consumer.deleted_at IS NULL")
		}
		if consumer.objectDelete && src.Migrator().HasColumn(consumer.table, "object_deleted_at") {
			activeConditions = append(activeConditions, "consumer.object_deleted_at IS NULL")
		}
		activeCondition := "1 = 1"
		if len(activeConditions) > 0 {
			activeCondition = strings.Join(activeConditions, " AND ")
		}
		query := fmt.Sprintf(`SELECT ra.user_id, ra.asset_key AS object_key,
			CASE WHEN %s THEN 1 ELSE 0 END AS active
			FROM %s AS consumer
			JOIN reference_assets AS ra ON ra.id = consumer.%s
			WHERE ra.storage_scope = ? AND TRIM(ra.asset_key) <> ''`, activeCondition, consumer.table, consumer.column)
		if err := src.Unscoped().Raw(query, ecommerce.StorageScopeCommercePrivate).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("backfill guards from %s.%s: %w", consumer.table, consumer.column, err)
		}
		for _, row := range rows {
			merge(row.UserID, row.ObjectKey, row.Active)
		}
	}
	return candidates, nil
}

func copyNamedTable[T any](src, dst *gorm.DB, name string) (copiedTable, error) {
	if !src.Migrator().HasTable(name) {
		return copiedTable{name: name, count: 0}, nil
	}
	count, err := copyTable[T](src, dst)
	if err != nil {
		return copiedTable{}, fmt.Errorf("%s: %w", name, err)
	}
	return copiedTable{name: name, count: count}, nil
}

func copyTable[T any](src, dst *gorm.DB) (int64, error) {
	var rows []T
	if err := src.Unscoped().Find(&rows).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := dst.CreateInBatches(rows, 100).Error; err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

func resetSequences(db *gorm.DB) error {
	for _, table := range sequenceTables {

		var sequence sql.NullString
		if err := db.Raw("SELECT pg_get_serial_sequence(?, 'id')", table).Scan(&sequence).Error; err != nil {
			return fmt.Errorf("lookup %s sequence: %w", table, err)
		}
		if !sequence.Valid || sequence.String == "" {
			continue
		}
		statement := fmt.Sprintf("SELECT setval(?::regclass, COALESCE((SELECT MAX(id) FROM %s), 1), (SELECT MAX(id) IS NOT NULL FROM %s))", table, table)
		if err := db.Exec(statement, sequence.String).Error; err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	return nil
}

var sequenceTables = []string{
	"app_settings",
	"model_configs",
	"permissions",
	"roles",
	"admin_users",
	"admin_sessions",
	"admin_audit_logs",
	"system_announcements",
	"invites",
	"invite_redemptions",
	"user_roles",
	"users",
	"user_sessions",
	"credit_balances",
	"credit_transactions",
	"packages",
	"purchase_intents",
	"purchase_intent_notes",
	"finance_orders",
	"finance_refunds",
	"finance_invoices",
	"works",
	"reference_assets",
	"generation_records",
	"generation_reference_assets",
	"commerce_brands",
	"commerce_products",
	"commerce_skus",
	"commerce_projects",
	"commerce_assets",
	"commerce_creative_specs",
	"commerce_idempotency_records",
	"commerce_ai_invocations",
	"commerce_generation_batches",
	"commerce_generation_items",
	"commerce_jobs",
	"commerce_credit_reservations",
	"commerce_pricing_snapshots",
	"commerce_credit_settlements",
	"commerce_events",
	"commerce_object_cleanups",
	"commerce_object_guards",
}

func verifyMigration(src, dst *gorm.DB, copied []copiedTable) error {
	for _, table := range copied {
		var dstCount int64
		if err := dst.Table(table.name).Count(&dstCount).Error; err != nil {
			return fmt.Errorf("count %s: %w", table.name, err)
		}
		if dstCount != table.count {
			return fmt.Errorf("%s count mismatch: sqlite=%d postgres=%d", table.name, table.count, dstCount)
		}
	}
	var orphanBalances int64
	if err := dst.Table("credit_balances").
		Joins("LEFT JOIN users ON users.id = credit_balances.user_id").
		Where("users.id IS NULL").
		Count(&orphanBalances).Error; err != nil {
		return err
	}
	if orphanBalances > 0 {
		return fmt.Errorf("credit_balances has %d orphan rows", orphanBalances)
	}
	var mismatchCount int64
	if err := dst.Raw(`
		SELECT COUNT(*)
		FROM credit_balances cb
		JOIN (
			SELECT user_id, MAX(id) AS last_id
			FROM credit_transactions
			GROUP BY user_id
		) last_tx ON last_tx.user_id = cb.user_id
		JOIN credit_transactions ct ON ct.id = last_tx.last_id
		WHERE ct.balance_after <> cb.available_credits
	`).Scan(&mismatchCount).Error; err != nil {
		return err
	}
	if mismatchCount > 0 {
		return fmt.Errorf("credit balance mismatch for %d users", mismatchCount)
	}
	for _, table := range commerceBusinessTables {
		var missingUserID int64
		if err := dst.Table(table).Where("user_id IS NULL").Count(&missingUserID).Error; err != nil {
			return fmt.Errorf("verify %s user_id: %w", table, err)
		}
		if missingUserID > 0 {
			return fmt.Errorf("%s has %d rows without user_id", table, missingUserID)
		}
	}
	return nil
}

var commerceBusinessTables = []string{
	"commerce_brands",
	"commerce_products",
	"commerce_skus",
	"commerce_projects",
	"commerce_assets",
	"commerce_creative_specs",
	"commerce_idempotency_records",
	"commerce_ai_invocations",
	"commerce_generation_batches",
	"commerce_generation_items",
	"commerce_jobs",
	"commerce_credit_reservations",
	"commerce_pricing_snapshots",
	"commerce_credit_settlements",
	"commerce_events",
	"commerce_object_cleanups",
	"commerce_object_guards",
}
