package ecommerce

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFoundationSchemaRequiresAllWorkerJobColumns(t *testing.T) {
	want := []string{
		"lease_owner", "lease_token", "lease_expires_at", "heartbeat_at", "attempt_count", "max_attempts",
		"next_attempt_at", "cancel_requested_at", "dead_lettered_at", "status", "priority", "batch_id",
		"generation_item_id", "user_id", "project_id", "kind", "pipeline", "recipe_key",
	}
	for _, column := range want {
		if !containsString(workerRequiredCommerceJobColumns, column) {
			t.Fatalf("worker readiness columns missing %q: %#v", column, workerRequiredCommerceJobColumns)
		}
	}
}

func TestVerifyFoundationSchemaFailsWhenWorkerColumnMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:foundation-worker-column?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate foundation: %v", err)
	}
	if err := db.Exec("ALTER TABLE commerce_jobs RENAME COLUMN heartbeat_at TO missing_heartbeat_at").Error; err != nil {
		t.Fatalf("rename heartbeat column: %v", err)
	}
	err = VerifyFoundationSchema(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "heartbeat_at") {
		t.Fatalf("VerifyFoundationSchema error = %v, want missing heartbeat_at", err)
	}
}

func TestVerifyFoundationSchemaRequiresPhysicalSKUColumns(t *testing.T) {
	tests := []struct {
		table  string
		column string
	}{
		{"commerce_projects", "default_sku_id"},
		{"commerce_assets", "sku_id"},
		{"commerce_idempotency_records", "sku_id"},
		{"commerce_generation_batches", "primary_sku_id"},
		{"commerce_generation_items", "sku_id"},
	}

	for _, test := range tests {
		t.Run(test.table+"."+test.column, func(t *testing.T) {
			dsn := "file:foundation-sku-column-" + test.table + "?mode=memory&cache=shared"
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
				t.Fatalf("migrate foundation: %v", err)
			}
			missingColumn := "missing_" + test.column
			if err := db.Exec("ALTER TABLE " + test.table + " RENAME COLUMN " + test.column + " TO " + missingColumn).Error; err != nil {
				t.Fatalf("rename %s.%s: %v", test.table, test.column, err)
			}
			err = VerifyFoundationSchema(context.Background(), db)
			if err == nil || !strings.Contains(err.Error(), test.column) {
				t.Fatalf("VerifyFoundationSchema error = %v, want missing %s", err, test.column)
			}
		})
	}
}

func TestVerifyFoundationSchemaFailsWhenGenerationRequestStateColumnsMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:foundation-generation-request-state?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE generation_records (id integer primary key, storage_scope text, execution_key text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(FoundationModels()...); err != nil {
		t.Fatal(err)
	}
	for _, statement := range foundationSQLiteIndexStatements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("CREATE UNIQUE INDEX ux_generation_records_execution_key ON generation_records(execution_key) WHERE execution_key IS NOT NULL").Error; err != nil {
		t.Fatal(err)
	}
	err = VerifyFoundationSchema(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "provider_request_started") {
		t.Fatalf("VerifyFoundationSchema error=%v", err)
	}
}

func TestFoundationMigrationSQLIncludesGenerationProviderRequestState(t *testing.T) {
	for _, column := range []string{"provider_request_started", "provider_idempotency_supported"} {
		if !strings.Contains(foundationUpSQL, "ADD COLUMN IF NOT EXISTS "+column+" BOOLEAN NOT NULL DEFAULT FALSE") {
			t.Fatalf("up migration missing %s", column)
		}
		if strings.Contains(foundationDownSQL, "DROP COLUMN IF EXISTS "+column) {
			t.Fatalf("down migration must preserve parent compatibility column %s", column)
		}
	}
}

func TestProductReportMigrationContract(t *testing.T) {
	for _, fragment := range []string{"observed_facts_json", "user_overrides_json", "missing_fields_json", "suggested_sections_json", "analysis_error", "analysis_request_hash", "commerce_idempotency_records", "ux_commerce_idempotency_scope_key", "commerce_ai_invocations", "request_asset_ids_json", "ix_commerce_ai_invocations_job"} {
		if !strings.Contains(productReportsUpSQL, fragment) {
			t.Fatalf("product report up migration missing %q", fragment)
		}
	}
	if !strings.Contains(productReportsDownSQL, "refusing to roll back product report migration") || !strings.Contains(productReportsDownSQL, "DROP TABLE IF EXISTS commerce_idempotency_records") {
		t.Fatal("product report down migration lacks data guard or cleanup")
	}
}

func TestSKUMatrixMigrationContract(t *testing.T) {
	for _, fragment := range []string{"sku_version", "commerce_sku_dimensions", "commerce_sku_values", "commerce_sku_value_links", "commerce_sku_matrix_requests", "REFERENCES commerce_products", "REFERENCES commerce_skus", "ux_commerce_sku_matrix_requests_owner_key"} {
		if !strings.Contains(skuMatrixUpSQL, fragment) {
			t.Fatalf("SKU matrix migration missing %q", fragment)
		}
	}
	db, err := gorm.Open(sqlite.Open("file:sku-matrix-repeatable?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSQLiteFoundationSchema(context.Background(), db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	for _, check := range []struct {
		model any
		name  string
	}{{&CommerceSKUDimension{}, "ux_commerce_sku_dimensions_product_name"}, {&CommerceSKUValue{}, "ux_commerce_sku_values_dimension_name"}, {&CommerceSKUValueLink{}, "ux_commerce_sku_value_links_sku_value"}, {&CommerceSKUMatrixRequest{}, "ux_commerce_sku_matrix_requests_owner_key"}} {
		if !db.Migrator().HasIndex(check.model, check.name) {
			t.Fatalf("missing %s", check.name)
		}
	}
	for _, check := range []struct {
		model any
		name  string
	}{
		{&CommerceSKUValue{}, "fk_commerce_sku_values_product"},
		{&CommerceSKUValueLink{}, "fk_commerce_sku_value_links_product"},
		{&CommerceSKUMatrixRequest{}, "fk_commerce_sku_matrix_requests_product"},
	} {
		if !db.Migrator().HasConstraint(check.model, check.name) {
			t.Fatalf("missing %s", check.name)
		}
	}
}

func TestSKUMatrixDownMigrationGuardsEveryBusinessTableAndCleansUpSymmetrically(t *testing.T) {
	for _, table := range []string{"commerce_sku_matrix_requests", "commerce_sku_value_links", "commerce_sku_values", "commerce_sku_dimensions"} {
		if !strings.Contains(skuMatrixDownSQL, "'"+table+"'") {
			t.Fatalf("SKU matrix down migration does not guard %s", table)
		}
	}
	for _, fragment := range []string{"to_regclass(table_name)", "SELECT EXISTS (SELECT 1 FROM %I LIMIT 1)", "contains business data"} {
		if !strings.Contains(skuMatrixDownSQL, fragment) {
			t.Fatalf("SKU matrix down migration missing safe guard fragment %q", fragment)
		}
	}
	wantOrder := []string{
		"DROP TABLE IF EXISTS commerce_sku_matrix_requests",
		"DROP TABLE IF EXISTS commerce_sku_value_links",
		"DROP TABLE IF EXISTS commerce_sku_values",
		"DROP TABLE IF EXISTS commerce_sku_dimensions",
		"ALTER TABLE commerce_products DROP COLUMN IF EXISTS sku_version",
	}
	last := -1
	for _, fragment := range wantOrder {
		index := strings.Index(skuMatrixDownSQL, fragment)
		if index < 0 || index <= last {
			t.Fatalf("SKU matrix down migration cleanup order invalid at %q", fragment)
		}
		last = index
	}
}

func TestGenerationProgressMigrationContract(t *testing.T) {
	for _, fragment := range []string{"ADD COLUMN IF NOT EXISTS progress_percent INTEGER NOT NULL DEFAULT 0", "status IN ('succeeded', 'failed', 'canceled') THEN 100", "status = 'running' THEN 10"} {
		if !strings.Contains(generationProgressUpSQL, fragment) {
			t.Fatalf("generation progress up migration missing %q", fragment)
		}
	}
	if !strings.Contains(generationProgressDownSQL, "DROP COLUMN IF EXISTS progress_percent") {
		t.Fatal("generation progress down migration missing progress_percent cleanup")
	}
}

func TestSKUMatrixMigrationRepairsConstraintsOnExistingTables(t *testing.T) {
	for _, name := range []string{"fk_commerce_sku_dimensions_product", "fk_commerce_sku_values_product", "fk_commerce_sku_values_dimension", "fk_commerce_sku_value_links_product", "fk_commerce_sku_value_links_sku", "fk_commerce_sku_value_links_value", "fk_commerce_sku_matrix_requests_product"} {
		if !strings.Contains(skuMatrixUpSQL, "ADD CONSTRAINT "+name) {
			t.Fatalf("migration cannot repair missing constraint %s", name)
		}
		if !strings.Contains(skuMatrixUpSQL, "conname = '"+name+"'") {
			t.Fatalf("migration lacks idempotent guard for %s", name)
		}
	}
}

func TestFoundationMigrationStatusRequiresLatestSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:foundation-status-latest?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE commerce_projects (id integer primary key)").Error; err != nil {
		t.Fatal(err)
	}
	applied, err := FoundationMigrationStatus(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("legacy commerce_projects alone must not report latest migration applied")
	}
}

func TestProductReportDownRunsInsideFoundationTransaction(t *testing.T) {
	statements := controlledFoundationDownStatements()
	if len(statements) != 6 || statements[0] != generationProgressDownSQL || statements[1] != skuGenerationDownSQL || statements[2] != skuMatrixDownSQL || statements[3] != categoriesDownSQL || statements[4] != productReportsDownSQL || statements[5] != foundationDownSQL {
		t.Fatalf("down statements=%#v", statements)
	}
}

func TestFoundationMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:foundation-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("CREATE TABLE generation_records (id integer primary key, user_id integer, asset_key text, storage_scope text, execution_key text)").Error; err != nil {
		t.Fatalf("create generation_records: %v", err)
	}
	if err := db.Exec("CREATE TABLE reference_assets (id integer primary key, user_id integer, asset_key text, storage_scope text, deleted_at datetime)").Error; err != nil {
		t.Fatalf("create reference_assets: %v", err)
	}
	if err := db.Exec("CREATE TABLE works (id integer primary key, user_id integer, asset_key text, storage_scope text, deleted_at datetime)").Error; err != nil {
		t.Fatalf("create works: %v", err)
	}
	if err := db.Exec("CREATE TABLE generation_reference_assets (id integer primary key, generation_record_id integer, reference_asset_id integer)").Error; err != nil {
		t.Fatalf("create generation_reference_assets: %v", err)
	}
	if err := db.Exec("INSERT INTO reference_assets(id, user_id, asset_key, storage_scope, deleted_at) VALUES (100, 2, 'commerce/2/1/consumer.png', 'commerce_private', CURRENT_TIMESTAMP)").Error; err != nil {
		t.Fatalf("seed deleted consumer reference: %v", err)
	}
	if err := db.Exec("INSERT INTO generation_reference_assets(generation_record_id, reference_asset_id) VALUES (50, 100)").Error; err != nil {
		t.Fatalf("seed active generation consumer: %v", err)
	}
	if err := db.Exec("INSERT INTO works(user_id, asset_key, storage_scope) VALUES (2, 'commerce/2/1/work.png', 'commerce_private')").Error; err != nil {
		t.Fatalf("seed active Work: %v", err)
	}
	if err := db.Exec("INSERT INTO works(user_id, asset_key, storage_scope, deleted_at) VALUES (2, 'commerce/2/1/deleted-work.png', 'commerce_private', CURRENT_TIMESTAMP)").Error; err != nil {
		t.Fatalf("seed deleted Work: %v", err)
	}
	if err := db.Exec("INSERT INTO generation_records(user_id, asset_key, storage_scope, execution_key) VALUES (2, 'commerce/2/1/generation.png', 'commerce_private', 'backfill-generation')").Error; err != nil {
		t.Fatalf("seed GenerationRecord: %v", err)
	}

	ctx := context.Background()
	if err := MigrateSQLiteFoundationSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSQLiteFoundationSchema: %v", err)
	}
	if err := VerifyFoundationSchema(ctx, db); err != nil {
		t.Fatalf("VerifyFoundationSchema: %v", err)
	}
	for _, column := range []string{"provider_request_started", "provider_idempotency_supported"} {
		if !db.Migrator().HasColumn("generation_records", column) {
			t.Fatalf("missing migrated generation_records.%s", column)
		}
	}
	if err := MigrateSQLiteFoundationSchema(ctx, db); err != nil {
		t.Fatalf("second MigrateSQLiteFoundationSchema: %v", err)
	}
	for key, want := range map[string]string{
		"commerce/2/1/consumer.png":     ObjectGuardStateActive,
		"commerce/2/1/work.png":         ObjectGuardStateActive,
		"commerce/2/1/deleted-work.png": ObjectGuardStateDeleted,
		"commerce/2/1/generation.png":   ObjectGuardStateActive,
	} {
		var guard CommerceObjectGuard
		if err := db.Where("user_id = ? AND object_key = ?", 2, key).First(&guard).Error; err != nil {
			t.Fatalf("load backfilled guard %s: %v", key, err)
		}
		if guard.State != want {
			t.Fatalf("backfilled guard %s state = %q, want %q", key, guard.State, want)
		}
	}

	for _, model := range []any{
		&CommerceBrand{},
		&CommerceProduct{},
		&CommerceSKU{},
		&CommerceProject{},
		&CommerceAsset{},
		&CommerceCreativeSpec{},
		&CommerceIdempotencyRecord{},
		&CommerceGenerationBatch{},
		&CommerceJob{},
		&CommerceGenerationItem{},
		&CommerceCreditReservation{},
		&CommercePricingSnapshot{},
		&CommerceCreditSettlement{},
		&CommerceEvent{},
		&CommerceObjectCleanup{},
	} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("missing table for %T", model)
		}
	}

	for _, check := range []struct {
		model any
		name  string
	}{
		{&CommerceSKU{}, "ux_commerce_skus_product_code"},
		{&CommerceGenerationBatch{}, "ux_commerce_batches_user_idempotency"},
		{&CommerceJob{}, "ux_commerce_jobs_user_idempotency"},
		{&CommerceGenerationItem{}, "ux_commerce_items_user_idempotency"},
		{&CommerceJob{}, "ux_commerce_jobs_generation_item"},
		{&CommerceCreditReservation{}, "ux_commerce_reservations_user_idempotency"},
		{&CommerceCreditReservation{}, "ix_commerce_reservations_scope"},
		{&CommerceGenerationItem{}, "ix_commerce_items_reservation"},
		{&CommercePricingSnapshot{}, "ix_commerce_pricing_snapshots_lookup"},
		{&CommercePricingSnapshot{}, "ix_commerce_pricing_snapshots_expiry"},
		{&CommerceJob{}, "ix_commerce_jobs_claim"},
		{&CommerceJob{}, "ix_commerce_jobs_lease"},
		{&CommerceJob{}, "ix_commerce_jobs_subject"},
		{&CommerceCreditSettlement{}, "ux_commerce_settlements_item"},
		{&CommerceEvent{}, "ix_commerce_events_metrics"},
	} {
		if !db.Migrator().HasIndex(check.model, check.name) {
			t.Fatalf("missing index %s for %T", check.name, check.model)
		}
	}
	if !db.Migrator().HasIndex("generation_records", "ux_generation_records_execution_key") {
		t.Fatal("missing generation-record execution key index")
	}
	if !db.Migrator().HasIndex("reference_assets", "ux_reference_assets_storage_object") {
		t.Fatal("missing reference-asset storage object index")
	}
	if !db.Migrator().HasIndex(&CommerceObjectGuard{}, "ux_commerce_object_guards_identity") {
		t.Fatal("missing commerce object guard identity index")
	}
	if err := db.Create(&CommerceObjectGuard{
		UserID: 1, StorageScope: StorageScopeCommercePrivate, ObjectKey: "commerce/1/1/a.png", State: ObjectGuardStateActive,
	}).Error; err != nil {
		t.Fatalf("insert active object guard: %v", err)
	}
	if err := db.Exec("INSERT INTO reference_assets(user_id, asset_key, storage_scope) VALUES (1, 'commerce/1/1/a.png', 'commerce_private')").Error; err != nil {
		t.Fatalf("insert first scoped reference asset: %v", err)
	}
	if err := db.Exec("INSERT INTO reference_assets(user_id, asset_key, storage_scope) VALUES (1, 'commerce/1/1/a.png', 'commerce_private')").Error; err == nil {
		t.Fatal("expected duplicate active scoped reference asset to be rejected")
	}
	if err := db.Model(&CommerceObjectGuard{}).
		Where("user_id = ? AND storage_scope = ? AND object_key = ?", 1, StorageScopeCommercePrivate, "commerce/1/1/a.png").
		Update("state", ObjectGuardStateDeleting).Error; err != nil {
		t.Fatalf("mark object guard deleting: %v", err)
	}
	if err := db.Exec("INSERT INTO generation_records(user_id, asset_key, storage_scope, execution_key) VALUES (1, 'commerce/1/1/a.png', 'commerce_private', 'private-race')").Error; err == nil || !strings.Contains(err.Error(), "commerce private object is not active") {
		t.Fatalf("expected guard to reject GenerationRecord reference, got %v", err)
	}
	if err := db.Exec("INSERT INTO generation_records(user_id, asset_key, storage_scope, execution_key) VALUES (1, 'public/a.png', 'default', 'default-compatible')").Error; err != nil {
		t.Fatalf("default storage scope was blocked by commerce object guard: %v", err)
	}
	var referenceID uint
	if err := db.Raw("SELECT id FROM reference_assets WHERE user_id = 1 AND asset_key = 'commerce/1/1/a.png'").Scan(&referenceID).Error; err != nil || referenceID == 0 {
		t.Fatalf("load guarded reference id: id=%d err=%v", referenceID, err)
	}
	if err := db.Create(&CommerceAsset{UserID: 1, ProjectID: 1, ReferenceAssetID: referenceID, Role: "reference", Lifecycle: AssetLifecycleProject}).Error; err == nil || !strings.Contains(err.Error(), "commerce private object is not active") {
		t.Fatalf("expected guard to reject CommerceAsset reference, got %v", err)
	}
	if !strings.Contains(foundationUpSQL, "CREATE UNIQUE INDEX IF NOT EXISTS ux_reference_assets_storage_object") {
		t.Fatal("PostgreSQL up migration missing reference-asset storage object index")
	}
	if !strings.Contains(foundationDownSQL, "DROP INDEX IF EXISTS ux_reference_assets_storage_object") {
		t.Fatal("PostgreSQL down migration missing reference-asset storage object index cleanup")
	}
	if !strings.Contains(foundationUpSQL, "CREATE TABLE IF NOT EXISTS commerce_object_guards") || !strings.Contains(foundationUpSQL, "commerce_assert_private_object_active") {
		t.Fatal("PostgreSQL up migration missing commerce object deletion guard protocol")
	}
	for _, fragment := range []string{
		"FROM works",
		"FROM generation_records",
		"FROM generation_reference_assets AS consumer",
		"FROM commerce_assets AS consumer",
		"FROM user_video_style_templates AS consumer",
		"FROM couple_albums AS consumer",
		"FROM novel_video_shots AS consumer",
		"FROM commerce_brands AS consumer",
		"('reference_assets', 'user_id, storage_scope, asset_key, deleted_at')",
		"('works', 'user_id, storage_scope, asset_key, deleted_at')",
		"to_jsonb(NEW) ? 'object_deleted_at'",
		"('commerce_assets', 'reference_asset_id', 'reference_asset_id, deleted_at, object_deleted_at')",
		"('user_video_style_templates', 'reference_asset_id', 'reference_asset_id, deleted_at')",
		"('couple_albums', 'male_reference_asset_id', 'male_reference_asset_id, deleted_at')",
		"('couple_albums', 'female_reference_asset_id', 'female_reference_asset_id, deleted_at')",
		"('commerce_brands', 'logo_reference_asset_id', 'logo_reference_asset_id, deleted_at')",
	} {
		if !strings.Contains(foundationUpSQL, fragment) {
			t.Fatalf("PostgreSQL guard migration missing %q", fragment)
		}
	}
	if !strings.Contains(foundationUpSQL, "FOR SHARE") {
		t.Fatal("PostgreSQL object guard does not lock writers against deletion state updates")
	}
	if !strings.Contains(foundationDownSQL, "DROP TABLE IF EXISTS commerce_object_guards") || !strings.Contains(foundationDownSQL, "DROP FUNCTION IF EXISTS commerce_assert_private_object_active") {
		t.Fatal("PostgreSQL down migration missing commerce object deletion guard cleanup")
	}
	for _, triggerName := range []string{
		"trg_works_commerce_guard_update",
		"trg_commerce_assets_reference_asset_id_commerce_guard_update",
	} {
		var triggerSQL string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?", triggerName).Scan(&triggerSQL).Error; err != nil {
			t.Fatalf("load SQLite trigger %s: %v", triggerName, err)
		}
		if !strings.Contains(strings.ToLower(triggerSQL), "deleted_at") {
			t.Fatalf("SQLite trigger %s does not validate deleted_at restore: %s", triggerName, triggerSQL)
		}
		if triggerName == "trg_commerce_assets_reference_asset_id_commerce_guard_update" && !strings.Contains(strings.ToLower(triggerSQL), "object_deleted_at") {
			t.Fatalf("SQLite CommerceAsset trigger does not validate object_deleted_at restore: %s", triggerSQL)
		}
	}
	if !db.Migrator().HasConstraint(&CommerceJob{}, "fk_commerce_jobs_generation_item") {
		t.Fatal("missing commerce_jobs generation-item foreign key")
	}
	orphanItemID := uint(999999)
	if err := db.Create(&CommerceJob{GenerationItemID: &orphanItemID}).Error; err == nil {
		t.Fatal("expected orphan commerce job generation_item_id to be rejected")
	}
	if !db.Migrator().HasColumn(&CommerceGenerationItem{}, "output_spec_json") {
		t.Fatal("missing commerce_generation_items.output_spec_json")
	}
	if !db.Migrator().HasColumn(&CommerceGenerationItem{}, "progress_percent") {
		t.Fatal("missing commerce_generation_items.progress_percent")
	}
	if !db.Migrator().HasColumn(&CommerceGenerationBatch{}, "reserved_credits") {
		t.Fatal("missing commerce_generation_batches.reserved_credits")
	}

	if err := RollbackFoundationMigration(ctx, db); err != nil {
		t.Fatalf("RollbackFoundationMigration: %v", err)
	}
	if !db.Migrator().HasTable(&CommerceGenerationItem{}) {
		t.Fatal("code rollback unexpectedly removed commerce tables")
	}
}
