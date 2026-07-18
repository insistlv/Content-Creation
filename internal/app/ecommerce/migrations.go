package ecommerce

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/0001_foundation.up.sql
var foundationUpSQL string

//go:embed migrations/0001_foundation.down.sql
var foundationDownSQL string

//go:embed migrations/0002_product_reports.up.sql
var productReportsUpSQL string

//go:embed migrations/0002_product_reports.down.sql
var productReportsDownSQL string

//go:embed migrations/0003_categories.up.sql
var categoriesUpSQL string

//go:embed migrations/0003_categories.down.sql
var categoriesDownSQL string

//go:embed migrations/0004_sku_matrix.up.sql
var skuMatrixUpSQL string

//go:embed migrations/0004_sku_matrix.down.sql
var skuMatrixDownSQL string

//go:embed migrations/0005_sku_generation.up.sql
var skuGenerationUpSQL string

//go:embed migrations/0005_sku_generation.down.sql
var skuGenerationDownSQL string

//go:embed migrations/0006_generation_progress.up.sql
var generationProgressUpSQL string

//go:embed migrations/0006_generation_progress.down.sql
var generationProgressDownSQL string

var foundationSQLiteIndexStatements = []string{
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_skus_product_code ON commerce_skus(product_id, code) WHERE deleted_at IS NULL",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_batches_user_idempotency ON commerce_generation_batches(user_id, idempotency_key)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_jobs_user_idempotency ON commerce_jobs(user_id, idempotency_key)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_items_user_idempotency ON commerce_generation_items(user_id, idempotency_key)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_jobs_generation_item ON commerce_jobs(generation_item_id) WHERE generation_item_id IS NOT NULL",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_reservations_user_idempotency ON commerce_credit_reservations(user_id, idempotency_key)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_reservations_scope ON commerce_credit_reservations(user_id, scope_type, scope_key)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_items_reservation ON commerce_generation_items(reservation_id)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_pricing_snapshots_lookup ON commerce_pricing_snapshots(user_id, project_id, request_digest, status)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_pricing_snapshots_expiry ON commerce_pricing_snapshots(expires_at)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_jobs_claim ON commerce_jobs(status, next_attempt_at, priority DESC, id)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_jobs_lease ON commerce_jobs(lease_expires_at)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_jobs_subject ON commerce_jobs(user_id, subject_type, subject_id, status)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_settlements_item ON commerce_credit_settlements(generation_item_id)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_events_metrics ON commerce_events(created_at, event_type, pipeline)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_creative_specs_analysis_request_hash ON commerce_creative_specs(analysis_request_hash)",
	"CREATE INDEX IF NOT EXISTS ix_commerce_ai_invocations_job ON commerce_ai_invocations(job_id, created_at)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_system_categories_parent_name ON commerce_system_categories(COALESCE(parent_id, 0), name)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_system_categories_seed_key ON commerce_system_categories(seed_key) WHERE seed_key <> ''",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_user_categories_owner_parent_name ON commerce_user_categories(user_id, parent_id, name)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_dimensions_product_name ON commerce_sku_dimensions(product_id, name)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_values_dimension_name ON commerce_sku_values(dimension_id, name)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_value_links_sku_value ON commerce_sku_value_links(sku_id, value_id)",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_matrix_requests_owner_key ON commerce_sku_matrix_requests(user_id, product_id, idempotency_key)",
}

const commerceJobGenerationItemConstraint = "fk_commerce_jobs_generation_item"

var workerRequiredCommerceJobColumns = []string{
	"id", "user_id", "project_id", "batch_id", "generation_item_id", "subject_id", "subject_type",
	"kind", "pipeline", "recipe_key", "status", "idempotency_key", "priority", "attempt_count", "max_attempts",
	"next_attempt_at", "lease_owner", "lease_token", "lease_expires_at", "heartbeat_at", "cancel_requested_at",
	"payload_json", "result_json", "error_code", "error_message", "started_at", "finished_at", "dead_lettered_at",
	"created_at", "updated_at",
}

func FoundationModels() []any {
	return []any{
		&CommerceBrand{},
		&CommerceProduct{},
		&CommerceSystemCategory{},
		&CommerceUserCategory{},
		&CommerceSKU{},
		&CommerceSKUDimension{},
		&CommerceSKUValue{},
		&CommerceSKUValueLink{},
		&CommerceSKUMatrixRequest{},
		&CommerceProject{},
		&CommerceAsset{},
		&CommerceCreativeSpec{},
		&CommerceIdempotencyRecord{},
		&CommerceAIInvocation{},
		&CommerceGenerationBatch{},
		&CommerceGenerationItem{},
		&CommerceJob{},
		&CommerceCreditReservation{},
		&CommercePricingSnapshot{},
		&CommerceCreditSettlement{},
		&CommerceEvent{},
		&CommerceObjectCleanup{},
		&CommerceObjectGuard{},
	}
}

// ApplyFoundationMigrations uses the embedded SQL as PostgreSQL's source of
// truth. SQLite tests use the same model set and compatible explicit indexes.
func ApplyFoundationMigrations(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("apply foundation migrations: nil database")
	}
	if db.Dialector.Name() != "postgres" {
		return MigrateSQLiteFoundationSchema(ctx, db)
	}
	if err := db.WithContext(ctx).Exec(foundationUpSQL).Error; err != nil {
		return fmt.Errorf("apply foundation PostgreSQL migration: %w", err)
	}
	if err := db.WithContext(ctx).Exec(productReportsUpSQL).Error; err != nil {
		return fmt.Errorf("apply product reports PostgreSQL migration: %w", err)
	}
	if err := db.WithContext(ctx).Exec(categoriesUpSQL).Error; err != nil {
		return fmt.Errorf("apply categories PostgreSQL migration: %w", err)
	}
	if err := db.WithContext(ctx).Exec(skuMatrixUpSQL).Error; err != nil {
		return fmt.Errorf("apply SKU matrix PostgreSQL migration: %w", err)
	}
	if err := db.WithContext(ctx).Exec(skuGenerationUpSQL).Error; err != nil {
		return fmt.Errorf("apply SKU generation PostgreSQL migration: %w", err)
	}
	if err := db.WithContext(ctx).Exec(generationProgressUpSQL).Error; err != nil {
		return fmt.Errorf("apply generation progress PostgreSQL migration: %w", err)
	}
	if err := SeedDefaultCategories(ctx, db); err != nil {
		return fmt.Errorf("seed commerce categories: %w", err)
	}
	if err := VerifyFoundationSchema(ctx, db); err != nil {
		return fmt.Errorf("verify foundation PostgreSQL migration: %w", err)
	}
	return nil
}

func VerifyFoundationSchema(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("verify foundation schema: nil database")
	}
	db = db.WithContext(ctx)
	migrator := db.Migrator()

	for _, model := range FoundationModels() {
		if !migrator.HasTable(model) {
			return fmt.Errorf("missing foundation table for %T", model)
		}
	}

	for _, check := range []struct {
		model   any
		columns []string
	}{
		{&CommerceProject{}, []string{"default_sku_id"}},
		{&CommerceProduct{}, []string{"category_id", "category_source", "category_path"}},
		{&CommerceProduct{}, []string{"sku_version"}},
		{&CommerceSKUDimension{}, []string{"user_id", "product_id", "version", "sort_order", "status"}},
		{&CommerceSKUValue{}, []string{"user_id", "product_id", "dimension_id", "sort_order", "status"}},
		{&CommerceSKUValueLink{}, []string{"user_id", "product_id", "sku_id", "value_id"}},
		{&CommerceSystemCategory{}, []string{"parent_id", "level", "seed_key", "search_aliases_json", "catalog_version"}},
		{&CommerceUserCategory{}, []string{"user_id", "parent_id", "search_aliases_json", "status"}},
		{&CommerceAsset{}, []string{"sku_id"}},
		{&CommerceGenerationBatch{}, []string{"primary_sku_id", "reserved_credits", "idempotency_key", "pricing_snapshot_json"}},
		{&CommerceJob{}, workerRequiredCommerceJobColumns},
		{&CommerceGenerationItem{}, []string{"sku_id", "scope", "input_snapshot_json", "output_spec_json", "reserved_credits", "progress_percent"}},
		{&CommerceCreditReservation{}, []string{"reserved_credits", "idempotency_key"}},
		{&CommercePricingSnapshot{}, []string{"request_digest", "expires_at"}},
		{&CommerceCreditSettlement{}, []string{"generation_item_id"}},
		{&CommerceEvent{}, []string{"event_type", "metadata_json"}},
		{&CommerceObjectCleanup{}, []string{"storage_scope", "object_key"}},
		{&CommerceObjectGuard{}, []string{"user_id", "storage_scope", "object_key", "state", "delete_token"}},
		{&CommerceCreativeSpec{}, []string{"observed_facts_json", "user_overrides_json", "common_facts_json", "sku_overrides_json", "sku_context_sha256", "missing_fields_json", "suggested_sections_json", "analysis_error", "analysis_request_hash"}},
		{&CommerceIdempotencyRecord{}, []string{"user_id", "scope", "idempotency_key", "request_digest", "sku_id"}},
		{&CommerceAIInvocation{}, []string{"job_id", "user_id", "project_id", "purpose", "model_id", "channel_id", "status", "request_asset_ids_json", "response_schema_version"}},
	} {
		for _, column := range check.columns {
			if !migrator.HasColumn(check.model, column) {
				return fmt.Errorf("missing column %T.%s", check.model, column)
			}
		}
	}
	if !migrator.HasConstraint(&CommerceJob{}, commerceJobGenerationItemConstraint) {
		return fmt.Errorf("missing foundation constraint %s", commerceJobGenerationItemConstraint)
	}
	for _, check := range []struct {
		model any
		name  string
	}{
		{&CommerceSKUDimension{}, "fk_commerce_sku_dimensions_product"},
		{&CommerceSKUValue{}, "fk_commerce_sku_values_dimension"},
		{&CommerceSKUValue{}, "fk_commerce_sku_values_product"},
		{&CommerceSKUValueLink{}, "fk_commerce_sku_value_links_sku"},
		{&CommerceSKUValueLink{}, "fk_commerce_sku_value_links_value"},
		{&CommerceSKUValueLink{}, "fk_commerce_sku_value_links_product"},
		{&CommerceSKUMatrixRequest{}, "fk_commerce_sku_matrix_requests_product"},
	} {
		if !migrator.HasConstraint(check.model, check.name) {
			return fmt.Errorf("missing foundation constraint %s", check.name)
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
		{&CommerceObjectGuard{}, "ux_commerce_object_guards_identity"},
		{&CommerceIdempotencyRecord{}, "ux_commerce_idempotency_scope_key"},
		{&CommerceCreativeSpec{}, "ix_commerce_creative_specs_analysis_request_hash"},
		{&CommerceAIInvocation{}, "ix_commerce_ai_invocations_job"},
		{&CommerceSystemCategory{}, "ux_commerce_system_categories_parent_name"},
		{&CommerceSystemCategory{}, "ux_commerce_system_categories_seed_key"},
		{&CommerceUserCategory{}, "ux_commerce_user_categories_owner_parent_name"},
		{&CommerceSKUDimension{}, "ux_commerce_sku_dimensions_product_name"},
		{&CommerceSKUValue{}, "ux_commerce_sku_values_dimension_name"},
		{&CommerceSKUValueLink{}, "ux_commerce_sku_value_links_sku_value"},
		{&CommerceSKUMatrixRequest{}, "ux_commerce_sku_matrix_requests_owner_key"},
	} {
		if !migrator.HasIndex(check.model, check.name) {
			return fmt.Errorf("missing foundation index %s", check.name)
		}
	}

	for _, check := range []struct {
		table   string
		columns []string
	}{
		{"credit_balances", []string{"reserved_credits"}},
		{"credit_transactions", []string{"idempotency_key", "reserved_after"}},
		{"reference_assets", []string{"storage_scope"}},
		{"works", []string{"storage_scope"}},
		{"generation_records", []string{"storage_scope", "execution_key", "provider_request_started", "provider_idempotency_supported"}},
	} {
		if !migrator.HasTable(check.table) {
			continue
		}
		for _, column := range check.columns {
			if !migrator.HasColumn(check.table, column) {
				return fmt.Errorf("missing parent column %s.%s", check.table, column)
			}
		}
	}
	if migrator.HasTable("generation_records") && !migrator.HasIndex("generation_records", "ux_generation_records_execution_key") {
		return fmt.Errorf("missing foundation index ux_generation_records_execution_key")
	}
	if migrator.HasTable("reference_assets") && !migrator.HasIndex("reference_assets", "ux_reference_assets_storage_object") {
		return fmt.Errorf("missing foundation index ux_reference_assets_storage_object")
	}
	if err := verifyCommerceObjectGuardTriggers(db); err != nil {
		return err
	}

	return nil
}

// FoundationMigrationStatus reports whether the commerce foundation schema is present.
// It never changes database state.
func FoundationMigrationStatus(ctx context.Context, db *gorm.DB) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("foundation migration status: nil database")
	}
	var exists bool
	if db.Dialector.Name() == "postgres" {
		if err := db.WithContext(ctx).Raw(`SELECT to_regclass('commerce_projects') IS NOT NULL`).Scan(&exists).Error; err != nil {
			return false, fmt.Errorf("foundation migration status: %w", err)
		}
		if !exists {
			return false, nil
		}
		if err := VerifyFoundationSchema(ctx, db); err != nil {
			if strings.Contains(err.Error(), "missing ") {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	if !db.Migrator().HasTable(&CommerceProject{}) {
		return false, nil
	}
	if err := VerifyFoundationSchema(ctx, db); err != nil {
		if strings.Contains(err.Error(), "missing ") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DownFoundationMigration executes the embedded controlled rollback SQL.
// The SQL refuses rollback while any commerce business table contains data.
func DownFoundationMigration(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("down foundation migration: nil database")
	}
	if db.Dialector.Name() != "postgres" {
		return fmt.Errorf("down foundation migration is only supported for PostgreSQL")
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for index, statement := range controlledFoundationDownStatements() {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("down foundation PostgreSQL migration step %d: %w", index+1, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("down foundation PostgreSQL migration: %w", err)
	}
	return nil
}

func controlledFoundationDownStatements() []string {
	return []string{generationProgressDownSQL, skuGenerationDownSQL, skuMatrixDownSQL, categoriesDownSQL, productReportsDownSQL, foundationDownSQL}
}

// RollbackFoundationMigration is deliberately a no-op. The SQL down migration
// is retained for controlled PostgreSQL operations, while application rollback
// must preserve compatibility tables and existing parent columns.
func RollbackFoundationMigration(context.Context, *gorm.DB) error {
	return nil
}

func MigrateSQLiteFoundationSchema(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("migrate SQLite foundation schema: nil database")
	}
	db = db.WithContext(ctx)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return fmt.Errorf("enable SQLite foreign keys: %w", err)
	}
	if err := db.AutoMigrate(FoundationModels()...); err != nil {
		return fmt.Errorf("migrate SQLite foundation models: %w", err)
	}
	if db.Migrator().HasTable("generation_records") {
		for _, column := range []struct{ name, definition string }{
			{"provider_request_started", "BOOLEAN NOT NULL DEFAULT 0"},
			{"provider_idempotency_supported", "BOOLEAN NOT NULL DEFAULT 0"},
		} {
			if !db.Migrator().HasColumn("generation_records", column.name) {
				if err := db.Exec(fmt.Sprintf("ALTER TABLE generation_records ADD COLUMN %s %s", column.name, column.definition)).Error; err != nil {
					return fmt.Errorf("add SQLite generation request state column %s: %w", column.name, err)
				}
			}
		}
	}
	if !db.Migrator().HasConstraint(&CommerceJob{}, commerceJobGenerationItemConstraint) {
		if err := db.Migrator().CreateConstraint(&CommerceJob{}, commerceJobGenerationItemConstraint); err != nil {
			return fmt.Errorf("create SQLite commerce job generation-item constraint: %w", err)
		}
	}
	for _, statement := range foundationSQLiteIndexStatements {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("create SQLite foundation index: %w", err)
		}
	}
	if db.Migrator().HasTable("generation_records") && db.Migrator().HasColumn("generation_records", "execution_key") {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_generation_records_execution_key ON generation_records(execution_key) WHERE execution_key IS NOT NULL").Error; err != nil {
			return fmt.Errorf("create SQLite generation record execution-key index: %w", err)
		}
	}
	if db.Migrator().HasTable("reference_assets") {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_reference_assets_storage_object ON reference_assets(user_id, storage_scope, asset_key) WHERE deleted_at IS NULL AND storage_scope = 'commerce_private'").Error; err != nil {
			return fmt.Errorf("create SQLite reference asset storage-object index: %w", err)
		}
	}
	if err := migrateSQLiteCommerceObjectGuards(db); err != nil {
		return err
	}
	if err := SeedDefaultCategories(ctx, db); err != nil {
		return fmt.Errorf("seed SQLite commerce categories: %w", err)
	}
	if err := VerifyFoundationSchema(ctx, db); err != nil {
		return fmt.Errorf("verify SQLite foundation migration: %w", err)
	}
	return nil
}

type commerceObjectGuardReferenceTrigger struct {
	table        string
	column       string
	softDelete   bool
	objectDelete bool
}

type commerceObjectGuardDirectTrigger struct {
	table      string
	softDelete bool
}

var commerceObjectGuardDirectTriggers = []commerceObjectGuardDirectTrigger{
	{table: "reference_assets", softDelete: true},
	{table: "works", softDelete: true},
	{table: "generation_records"},
}

var commerceObjectGuardReferenceTriggers = []commerceObjectGuardReferenceTrigger{
	{table: "commerce_assets", column: "reference_asset_id", softDelete: true, objectDelete: true},
	{table: "generation_reference_assets", column: "reference_asset_id"},
	{table: "user_video_style_templates", column: "reference_asset_id", softDelete: true},
	{table: "couple_albums", column: "male_reference_asset_id", softDelete: true},
	{table: "couple_albums", column: "female_reference_asset_id", softDelete: true},
	{table: "novel_video_shots", column: "reference_asset_id"},
	{table: "commerce_brands", column: "logo_reference_asset_id", softDelete: true},
}

func migrateSQLiteCommerceObjectGuards(db *gorm.DB) error {
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	queries := sqliteCommerceObjectGuardCandidateQueries(db)
	if len(queries) > 0 {
		statement := fmt.Sprintf(`
			INSERT OR IGNORE INTO commerce_object_guards(user_id, storage_scope, object_key, state, delete_token, created_at, updated_at)
			SELECT user_id, 'commerce_private', object_key,
			       CASE WHEN MAX(is_active) = 1 THEN 'active' ELSE 'deleted' END,
			       '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			FROM (%s) AS candidates
			WHERE user_id <> 0 AND TRIM(object_key) <> ''
			GROUP BY user_id, object_key`, strings.Join(queries, " UNION ALL "))
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("backfill SQLite commerce object guards: %w", err)
		}
	}
	for _, target := range commerceObjectGuardDirectTriggers {
		if !db.Migrator().HasTable(target.table) || !db.Migrator().HasColumn(target.table, "user_id") || !db.Migrator().HasColumn(target.table, "storage_scope") || !db.Migrator().HasColumn(target.table, "asset_key") {
			continue
		}
		condition := `NEW.storage_scope = 'commerce_private' AND NOT EXISTS (
			SELECT 1 FROM commerce_object_guards guard
			WHERE guard.user_id = NEW.user_id AND guard.storage_scope = 'commerce_private'
			  AND guard.object_key = NEW.asset_key AND guard.state = 'active'
		)`
		updateColumns := "user_id, storage_scope, asset_key"
		if target.softDelete && db.Migrator().HasColumn(target.table, "deleted_at") {
			condition = "NEW.deleted_at IS NULL AND " + condition
			updateColumns += ", deleted_at"
		}
		for _, event := range []string{"INSERT", "UPDATE"} {
			name := commerceObjectGuardDirectTriggerName(target.table, event)
			if err := db.Exec("DROP TRIGGER IF EXISTS " + name).Error; err != nil {
				return fmt.Errorf("drop SQLite commerce object guard trigger %s: %w", name, err)
			}
			statement := fmt.Sprintf(`CREATE TRIGGER %s BEFORE %s%s ON %s
				WHEN %s BEGIN SELECT RAISE(ABORT, 'commerce private object is not active'); END`,
				name, event, commerceObjectGuardUpdateColumns(event, updateColumns), target.table, condition)
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("create SQLite commerce object guard trigger %s: %w", name, err)
			}
		}
	}
	if !db.Migrator().HasTable("reference_assets") {
		return nil
	}
	for _, target := range commerceObjectGuardReferenceTriggers {
		if !db.Migrator().HasTable(target.table) || !db.Migrator().HasColumn(target.table, target.column) {
			continue
		}
		condition := fmt.Sprintf(`NEW.%s IS NOT NULL AND EXISTS (
			SELECT 1 FROM reference_assets ra
			WHERE ra.id = NEW.%s AND ra.storage_scope = 'commerce_private'
			  AND NOT EXISTS (
				SELECT 1 FROM commerce_object_guards guard
				WHERE guard.user_id = ra.user_id AND guard.storage_scope = 'commerce_private'
				  AND guard.object_key = ra.asset_key AND guard.state = 'active'
			  )
		)`, target.column, target.column)
		updateColumns := target.column
		if target.softDelete && db.Migrator().HasColumn(target.table, "deleted_at") {
			condition = "NEW.deleted_at IS NULL AND " + condition
			updateColumns += ", deleted_at"
		}
		if target.objectDelete && db.Migrator().HasColumn(target.table, "object_deleted_at") {
			condition = "NEW.object_deleted_at IS NULL AND " + condition
			updateColumns += ", object_deleted_at"
		}
		for _, event := range []string{"INSERT", "UPDATE"} {
			name := commerceObjectGuardReferenceTriggerName(target, event)
			if err := db.Exec("DROP TRIGGER IF EXISTS " + name).Error; err != nil {
				return fmt.Errorf("drop SQLite commerce object guard trigger %s: %w", name, err)
			}
			statement := fmt.Sprintf(`CREATE TRIGGER %s BEFORE %s%s ON %s
				WHEN %s BEGIN SELECT RAISE(ABORT, 'commerce private object is not active'); END`,
				name, event, commerceObjectGuardUpdateColumns(event, updateColumns), target.table, condition)
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("create SQLite commerce object guard trigger %s: %w", name, err)
			}
		}
	}
	return nil
}

func sqliteCommerceObjectGuardCandidateQueries(db *gorm.DB) []string {
	queries := make([]string, 0, 12)
	directTables := []struct {
		table      string
		softDelete bool
	}{
		{"reference_assets", true},
		{"works", true},
		{"generation_records", false},
	}
	for _, target := range directTables {
		if !db.Migrator().HasTable(target.table) || !db.Migrator().HasColumn(target.table, "user_id") || !db.Migrator().HasColumn(target.table, "storage_scope") || !db.Migrator().HasColumn(target.table, "asset_key") {
			continue
		}
		activeExpression := "1"
		if target.softDelete && db.Migrator().HasColumn(target.table, "deleted_at") {
			activeExpression = "CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END"
		}
		queries = append(queries, fmt.Sprintf(`SELECT user_id, asset_key AS object_key, %s AS is_active
			FROM %s WHERE storage_scope = 'commerce_private' AND TRIM(asset_key) <> ''`, activeExpression, target.table))
	}
	if !db.Migrator().HasTable("reference_assets") {
		return queries
	}
	consumers := []struct {
		table, column string
		softDelete    bool
		objectDeleted bool
	}{
		{"generation_reference_assets", "reference_asset_id", false, false},
		{"commerce_assets", "reference_asset_id", true, true},
		{"user_video_style_templates", "reference_asset_id", true, false},
		{"couple_albums", "male_reference_asset_id", true, false},
		{"couple_albums", "female_reference_asset_id", true, false},
		{"novel_video_shots", "reference_asset_id", false, false},
		{"commerce_brands", "logo_reference_asset_id", true, false},
	}
	for _, consumer := range consumers {
		if !db.Migrator().HasTable(consumer.table) || !db.Migrator().HasColumn(consumer.table, consumer.column) {
			continue
		}
		activeConditions := make([]string, 0, 2)
		if consumer.softDelete && db.Migrator().HasColumn(consumer.table, "deleted_at") {
			activeConditions = append(activeConditions, "consumer.deleted_at IS NULL")
		}
		if consumer.objectDeleted && db.Migrator().HasColumn(consumer.table, "object_deleted_at") {
			activeConditions = append(activeConditions, "consumer.object_deleted_at IS NULL")
		}
		activeExpression := "1"
		if len(activeConditions) > 0 {
			activeExpression = "CASE WHEN " + strings.Join(activeConditions, " AND ") + " THEN 1 ELSE 0 END"
		}
		queries = append(queries, fmt.Sprintf(`SELECT ra.user_id, ra.asset_key AS object_key, %s AS is_active
			FROM %s AS consumer JOIN reference_assets AS ra ON ra.id = consumer.%s
			WHERE ra.storage_scope = 'commerce_private' AND TRIM(ra.asset_key) <> ''`, activeExpression, consumer.table, consumer.column))
	}
	return queries
}

func commerceObjectGuardUpdateColumns(event, columns string) string {
	if event == "UPDATE" {
		return " OF " + columns
	}
	return ""
}

func commerceObjectGuardDirectTriggerName(table, event string) string {
	return fmt.Sprintf("trg_%s_commerce_guard_%s", table, strings.ToLower(event))
}

func commerceObjectGuardReferenceTriggerName(target commerceObjectGuardReferenceTrigger, event string) string {
	return fmt.Sprintf("trg_%s_%s_commerce_guard_%s", target.table, target.column, strings.ToLower(event))
}

func verifyCommerceObjectGuardTriggers(db *gorm.DB) error {
	expected := make([]struct {
		table string
		name  string
	}, 0)
	for _, target := range commerceObjectGuardDirectTriggers {
		if !db.Migrator().HasTable(target.table) || !db.Migrator().HasColumn(target.table, "user_id") || !db.Migrator().HasColumn(target.table, "storage_scope") || !db.Migrator().HasColumn(target.table, "asset_key") {
			continue
		}
		for _, event := range []string{"INSERT", "UPDATE"} {
			expected = append(expected, struct{ table, name string }{target.table, commerceObjectGuardDirectTriggerName(target.table, event)})
		}
	}
	if db.Migrator().HasTable("reference_assets") {
		for _, target := range commerceObjectGuardReferenceTriggers {
			if !db.Migrator().HasTable(target.table) || !db.Migrator().HasColumn(target.table, target.column) {
				continue
			}
			for _, event := range []string{"INSERT", "UPDATE"} {
				expected = append(expected, struct{ table, name string }{target.table, commerceObjectGuardReferenceTriggerName(target, event)})
			}
		}
	}
	for _, trigger := range expected {
		var count int64
		var err error
		if db.Dialector.Name() == "postgres" {
			err = db.Raw(`SELECT COUNT(*) FROM pg_trigger WHERE tgname = ? AND NOT tgisinternal`, trigger.name).Scan(&count).Error
		} else {
			err = db.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = ? AND tbl_name = ?`, trigger.name, trigger.table).Scan(&count).Error
		}
		if err != nil {
			return fmt.Errorf("verify commerce object guard trigger %s: %w", trigger.name, err)
		}
		if count != 1 {
			return fmt.Errorf("missing commerce object guard trigger %s", trigger.name)
		}
	}
	return nil
}
