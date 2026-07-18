package main

import (
	"testing"

	"image-agent/internal/app"
	"image-agent/internal/app/ecommerce"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateCreatesTablesForCurrentApplicationModels(t *testing.T) {
	db := openMigrationTestDB(t)

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tests := []struct {
		name  string
		model any
	}{
		{name: "model_configs", model: &app.ModelConfig{}},
		{name: "system_announcements", model: &app.SystemAnnouncement{}},
		{name: "user_roles", model: &app.UserRole{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !db.Migrator().HasTable(tt.model) {
				t.Fatalf("expected table %s to be migrated", tt.name)
			}
		})
	}
	for _, model := range ecommerce.FoundationModels() {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("expected Commerce table for %T to be migrated", model)
		}
	}
}

func TestCopyNamedTableSkipsMissingSQLiteSourceTable(t *testing.T) {
	src := openMigrationTestDB(t)
	dst := openMigrationTestDB(t)
	if err := dst.AutoMigrate(&app.ModelConfig{}); err != nil {
		t.Fatalf("migrate destination: %v", err)
	}

	copied, err := copyNamedTable[app.ModelConfig](src, dst, "model_configs")
	if err != nil {
		t.Fatalf("copy missing source table: %v", err)
	}
	if copied.count != 0 {
		t.Fatalf("expected missing table to copy 0 rows, got %d", copied.count)
	}

	var dstCount int64
	if err := dst.Model(&app.ModelConfig{}).Count(&dstCount).Error; err != nil {
		t.Fatalf("count destination rows: %v", err)
	}
	if dstCount != 0 {
		t.Fatalf("expected no destination rows, got %d", dstCount)
	}
}

func TestCopyGenerationRecordsPreservesProviderRequestState(t *testing.T) {
	src, dst := openMigrationTestDB(t), openMigrationTestDB(t)
	if err := src.AutoMigrate(&app.GenerationRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := dst.AutoMigrate(&app.GenerationRecord{}); err != nil {
		t.Fatal(err)
	}
	record := app.GenerationRecord{UserID: 9, ChannelID: 17, ProviderRequestStarted: true, ProviderIdempotencySupported: true, Status: app.GenerationStatusRunning}
	if err := src.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := copyNamedTable[app.GenerationRecord](src, dst, "generation_records"); err != nil {
		t.Fatal(err)
	}
	var copied app.GenerationRecord
	if err := dst.First(&copied, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !copied.ProviderRequestStarted || !copied.ProviderIdempotencySupported || copied.ChannelID != record.ChannelID {
		t.Fatalf("copied=%+v", copied)
	}
}

func TestCopyBusinessDataIncludesCommerceTables(t *testing.T) {
	src := openMigrationTestDB(t)
	dst := openMigrationTestDB(t)
	if err := src.AutoMigrate(&app.ModelConfig{}, &app.SystemAnnouncement{}, &app.UserRole{}, &ecommerce.CommerceProduct{}, &ecommerce.CommerceObjectGuard{}); err != nil {
		t.Fatalf("migrate source: %v", err)
	}
	if err := src.Create(&app.ModelConfig{
		Name:       "Custom Image",
		Type:       app.ModelConfigTypeImage,
		Provider:   "Custom",
		Status:     app.ModelConfigStatusOnline,
		Permission: app.ModelConfigPermissionPublic,
	}).Error; err != nil {
		t.Fatalf("seed source model config: %v", err)
	}
	if err := src.Create(&app.SystemAnnouncement{
		Title:   "Maintenance",
		Content: "Scheduled maintenance",
		Level:   app.AnnouncementLevelInfo,
		Status:  app.AnnouncementStatusPublished,
	}).Error; err != nil {
		t.Fatalf("seed source system announcement: %v", err)
	}
	if err := src.Create(&app.UserRole{
		Code: "beta_user",
		Name: "Beta user",
	}).Error; err != nil {
		t.Fatalf("seed source user role: %v", err)
	}
	if err := src.Create(&ecommerce.CommerceProduct{
		UserID: 101,
		Name:   "Commerce product",
		Status: "active",
	}).Error; err != nil {
		t.Fatalf("seed source commerce product: %v", err)
	}
	if err := src.Create(&ecommerce.CommerceObjectGuard{
		UserID: 101, StorageScope: ecommerce.StorageScopeCommercePrivate,
		ObjectKey: "commerce/101/1/product.png", State: ecommerce.ObjectGuardStateActive,
	}).Error; err != nil {
		t.Fatalf("seed source commerce object guard: %v", err)
	}
	if err := migrate(dst); err != nil {
		t.Fatalf("migrate destination: %v", err)
	}

	copied, err := copyBusinessData(src, dst)
	if err != nil {
		t.Fatalf("copy business data: %v", err)
	}

	copiedCounts := map[string]int64{}
	for _, table := range copied {
		copiedCounts[table.name] = table.count
	}
	for _, table := range []string{"model_configs", "system_announcements", "user_roles", "commerce_products", "commerce_object_guards"} {
		if copiedCounts[table] != 1 {
			t.Fatalf("expected %s to copy 1 row, got %d", table, copiedCounts[table])
		}
	}

	assertDestinationCount(t, dst, &app.ModelConfig{}, 1)
	assertDestinationCount(t, dst, &app.SystemAnnouncement{}, 1)
	assertDestinationCount(t, dst, &app.UserRole{}, 1)
	assertDestinationCount(t, dst, &ecommerce.CommerceProduct{}, 1)
	assertDestinationCount(t, dst, &ecommerce.CommerceObjectGuard{}, 1)
}

func TestCommerceMigrationCoverageIncludesEveryFoundationTableAndSequence(t *testing.T) {
	src, dst := openMigrationTestDB(t), openMigrationTestDB(t)
	if err := ecommerce.MigrateSQLiteFoundationSchema(t.Context(), src); err != nil {
		t.Fatal(err)
	}
	if err := ecommerce.MigrateSQLiteFoundationSchema(t.Context(), dst); err != nil {
		t.Fatal(err)
	}
	copied, err := copyBusinessData(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	copiedNames := map[string]bool{}
	for _, table := range copied {
		copiedNames[table.name] = true
	}
	sequenceNames := map[string]bool{}
	for _, table := range sequenceTables {
		sequenceNames[table] = true
	}
	for _, table := range commerceBusinessTables {
		if !copiedNames[table] {
			t.Errorf("commerce table %s missing from copy list", table)
		}
		if !sequenceNames[table] {
			t.Errorf("commerce table %s missing from sequence calibration", table)
		}
	}
	for table, columns := range map[string][]string{
		"credit_balances":     {"reserved_credits"},
		"credit_transactions": {"idempotency_key", "reserved_after"},
		"reference_assets":    {"storage_scope"},
		"works":               {"storage_scope"},
		"generation_records":  {"storage_scope", "execution_key", "provider_request_started", "provider_idempotency_supported"},
	} {
		if !dst.Migrator().HasTable(table) {
			continue
		}
		for _, column := range columns {
			if !dst.Migrator().HasColumn(table, column) {
				t.Errorf("parent column %s.%s missing", table, column)
			}
		}
	}
}

func TestCopyBusinessDataBackfillsGuardsBeforePrivateReferences(t *testing.T) {
	src := openMigrationTestDB(t)
	dst := openMigrationTestDB(t)
	if err := src.AutoMigrate(&app.ReferenceAsset{}); err != nil {
		t.Fatalf("migrate legacy reference assets: %v", err)
	}
	reference := app.ReferenceAsset{
		UserID: 77, AssetKey: "commerce/77/9/legacy.png",
		StorageScope: ecommerce.StorageScopeCommercePrivate, MIMEType: "image/png",
	}
	if err := src.Create(&reference).Error; err != nil {
		t.Fatalf("seed legacy private reference: %v", err)
	}
	deletedReference := app.ReferenceAsset{
		UserID: 77, AssetKey: "commerce/77/9/deleted.png",
		StorageScope: ecommerce.StorageScopeCommercePrivate, MIMEType: "image/png",
	}
	if err := src.Create(&deletedReference).Error; err != nil {
		t.Fatalf("seed deleted legacy private reference: %v", err)
	}
	if err := src.Delete(&deletedReference).Error; err != nil {
		t.Fatalf("soft delete legacy private reference: %v", err)
	}
	if err := migrate(dst); err != nil {
		t.Fatalf("migrate destination: %v", err)
	}

	if _, err := copyBusinessData(src, dst); err != nil {
		t.Fatalf("copy business data: %v", err)
	}
	var guard ecommerce.CommerceObjectGuard
	if err := dst.Where("user_id = ? AND storage_scope = ? AND object_key = ?", reference.UserID, reference.StorageScope, reference.AssetKey).First(&guard).Error; err != nil {
		t.Fatalf("load backfilled object guard: %v", err)
	}
	if guard.State != ecommerce.ObjectGuardStateActive {
		t.Fatalf("backfilled guard state = %q, want active", guard.State)
	}
	var deletedGuard ecommerce.CommerceObjectGuard
	if err := dst.Where("user_id = ? AND storage_scope = ? AND object_key = ?", deletedReference.UserID, deletedReference.StorageScope, deletedReference.AssetKey).First(&deletedGuard).Error; err != nil {
		t.Fatalf("load deleted object guard: %v", err)
	}
	if deletedGuard.State != ecommerce.ObjectGuardStateDeleted {
		t.Fatalf("deleted guard state = %q, want deleted", deletedGuard.State)
	}
	var copied app.ReferenceAsset
	if err := dst.First(&copied, reference.ID).Error; err != nil {
		t.Fatalf("load copied private reference: %v", err)
	}
	var copiedDeleted app.ReferenceAsset
	if err := dst.Unscoped().First(&copiedDeleted, deletedReference.ID).Error; err != nil {
		t.Fatalf("load copied deleted private reference: %v", err)
	}
}

func TestCopyCommerceObjectGuardsUsesAllActiveConsumers(t *testing.T) {
	src := openMigrationTestDB(t)
	dst := openMigrationTestDB(t)
	if err := src.AutoMigrate(
		&app.ReferenceAsset{}, &app.Work{}, &app.GenerationRecord{}, &app.GenerationReferenceAsset{},
		&app.UserVideoStyleTemplate{}, &app.CoupleAlbum{}, &app.NovelVideoShot{},
		&ecommerce.CommerceAsset{}, &ecommerce.CommerceBrand{},
	); err != nil {
		t.Fatalf("migrate source consumer tables: %v", err)
	}
	if err := dst.AutoMigrate(&ecommerce.CommerceObjectGuard{}); err != nil {
		t.Fatalf("migrate destination guards: %v", err)
	}

	createDeletedReference := func(key string) app.ReferenceAsset {
		reference := app.ReferenceAsset{UserID: 91, AssetKey: key, StorageScope: ecommerce.StorageScopeCommercePrivate}
		if err := src.Create(&reference).Error; err != nil {
			t.Fatalf("create reference %s: %v", key, err)
		}
		if err := src.Delete(&reference).Error; err != nil {
			t.Fatalf("soft delete reference %s: %v", key, err)
		}
		return reference
	}

	consumerReferences := map[string]app.ReferenceAsset{
		"generation_reference": createDeletedReference("commerce/91/1/generation-reference.png"),
		"commerce_asset":       createDeletedReference("commerce/91/1/commerce-asset.png"),
		"style_template":       createDeletedReference("commerce/91/1/style-template.png"),
		"couple_album":         createDeletedReference("commerce/91/1/couple-album.png"),
		"novel_shot":           createDeletedReference("commerce/91/1/novel-shot.png"),
		"brand_logo":           createDeletedReference("commerce/91/1/brand-logo.png"),
	}
	if err := src.Create(&app.GenerationReferenceAsset{GenerationRecordID: 1, ReferenceAssetID: consumerReferences["generation_reference"].ID}).Error; err != nil {
		t.Fatalf("create generation reference consumer: %v", err)
	}
	if err := src.Create(&ecommerce.CommerceAsset{UserID: 91, ProjectID: 1, ReferenceAssetID: consumerReferences["commerce_asset"].ID}).Error; err != nil {
		t.Fatalf("create commerce asset consumer: %v", err)
	}
	if err := src.Create(&app.UserVideoStyleTemplate{UserID: 91, ReferenceAssetID: consumerReferences["style_template"].ID}).Error; err != nil {
		t.Fatalf("create style template consumer: %v", err)
	}
	if err := src.Create(&app.CoupleAlbum{UserID: 91, MaleReferenceAssetID: consumerReferences["couple_album"].ID}).Error; err != nil {
		t.Fatalf("create couple album consumer: %v", err)
	}
	novelReferenceID := consumerReferences["novel_shot"].ID
	if err := src.Create(&app.NovelVideoShot{UserID: 91, ReferenceAssetID: &novelReferenceID}).Error; err != nil {
		t.Fatalf("create novel shot consumer: %v", err)
	}
	brandReferenceID := consumerReferences["brand_logo"].ID
	if err := src.Create(&ecommerce.CommerceBrand{UserID: 91, LogoReferenceAssetID: &brandReferenceID}).Error; err != nil {
		t.Fatalf("create brand logo consumer: %v", err)
	}

	activeWork := app.Work{UserID: 91, AssetKey: "commerce/91/1/work-active.png", StorageScope: ecommerce.StorageScopeCommercePrivate}
	if err := src.Create(&activeWork).Error; err != nil {
		t.Fatalf("create active Work: %v", err)
	}
	deletedWork := app.Work{UserID: 91, AssetKey: "commerce/91/1/work-deleted.png", StorageScope: ecommerce.StorageScopeCommercePrivate}
	if err := src.Create(&deletedWork).Error; err != nil {
		t.Fatalf("create deleted Work: %v", err)
	}
	if err := src.Delete(&deletedWork).Error; err != nil {
		t.Fatalf("soft delete Work: %v", err)
	}
	generation := app.GenerationRecord{UserID: 91, AssetKey: "commerce/91/1/generation.png", StorageScope: ecommerce.StorageScopeCommercePrivate}
	if err := src.Create(&generation).Error; err != nil {
		t.Fatalf("create GenerationRecord: %v", err)
	}

	copied, err := copyCommerceObjectGuards(src, dst)
	if err != nil {
		t.Fatalf("copyCommerceObjectGuards: %v", err)
	}
	if copied.count != int64(len(consumerReferences)+3) {
		t.Fatalf("copied guard count = %d, want %d", copied.count, len(consumerReferences)+3)
	}
	for name, reference := range consumerReferences {
		var guard ecommerce.CommerceObjectGuard
		if err := dst.Where("user_id = ? AND object_key = ?", reference.UserID, reference.AssetKey).First(&guard).Error; err != nil {
			t.Fatalf("load %s guard: %v", name, err)
		}
		if guard.State != ecommerce.ObjectGuardStateActive {
			t.Fatalf("%s guard state = %q, want active", name, guard.State)
		}
	}
	for key, want := range map[string]string{
		activeWork.AssetKey:  ecommerce.ObjectGuardStateActive,
		deletedWork.AssetKey: ecommerce.ObjectGuardStateDeleted,
		generation.AssetKey:  ecommerce.ObjectGuardStateActive,
	} {
		var guard ecommerce.CommerceObjectGuard
		if err := dst.Where("user_id = ? AND object_key = ?", 91, key).First(&guard).Error; err != nil {
			t.Fatalf("load direct guard %s: %v", key, err)
		}
		if guard.State != want {
			t.Fatalf("direct guard %s state = %q, want %q", key, guard.State, want)
		}
	}
}

func TestVerifyMigrationRejectsCommerceRowsWithoutUserID(t *testing.T) {
	src := openMigrationTestDB(t)
	dst := openMigrationTestDB(t)
	if err := migrate(dst); err != nil {
		t.Fatalf("migrate destination: %v", err)
	}
	if err := dst.Exec("INSERT INTO commerce_products (user_id, name) VALUES (NULL, 'invalid')").Error; err != nil {
		t.Fatalf("seed invalid commerce row: %v", err)
	}
	if err := verifyMigration(src, dst, nil); err == nil {
		t.Fatal("verifyMigration accepted a Commerce row without user_id")
	}
}

func TestCopyNamedTableReadsLegacySoftDeleteModelsWithoutDeletedAtColumn(t *testing.T) {
	src := openMigrationTestDB(t)
	dst := openMigrationTestDB(t)
	if err := src.Exec(`
		CREATE TABLE packages (
			id integer primary key,
			name text,
			description text,
			price_label text,
			price_cents integer,
			credits integer,
			valid_days integer,
			audience text,
			tags_json text,
			icon text,
			theme text,
			badge text,
			sort_order integer,
			is_active boolean,
			created_at datetime,
			updated_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create legacy source table: %v", err)
	}
	if err := src.Exec(`
		INSERT INTO packages (
			id, name, description, price_label, price_cents, credits, valid_days,
			audience, tags_json, icon, theme, badge, sort_order, is_active,
			created_at, updated_at
		) VALUES (
			1, 'Starter', 'Starter package', '9.90', 990, 20, 30,
			'standard', '[]', 'sparkles', 'blue', '', 10, true,
			'2026-01-01 00:00:00', '2026-01-01 00:00:00'
		)
	`).Error; err != nil {
		t.Fatalf("seed legacy source table: %v", err)
	}
	if err := dst.AutoMigrate(&app.Package{}); err != nil {
		t.Fatalf("migrate destination: %v", err)
	}

	copied, err := copyNamedTable[app.Package](src, dst, "packages")
	if err != nil {
		t.Fatalf("copy legacy source table: %v", err)
	}
	if copied.count != 1 {
		t.Fatalf("expected 1 copied row, got %d", copied.count)
	}
	assertDestinationCount(t, dst, &app.Package{}, 1)
}

func openMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func assertDestinationCount(t *testing.T, db *gorm.DB, model any, want int64) {
	t.Helper()
	var got int64
	if err := db.Model(model).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	if got != want {
		t.Fatalf("count %T: got %d, want %d", model, got, want)
	}
}
