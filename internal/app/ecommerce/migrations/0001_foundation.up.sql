ALTER TABLE IF EXISTS credit_balances
    ADD COLUMN IF NOT EXISTS reserved_credits INTEGER NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS credit_transactions
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(160) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS credit_transactions
    ADD COLUMN IF NOT EXISTS reserved_after INTEGER NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS reference_assets
    ADD COLUMN IF NOT EXISTS storage_scope VARCHAR(32) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS ux_reference_assets_storage_object
    ON reference_assets(user_id, storage_scope, asset_key)
    WHERE deleted_at IS NULL AND storage_scope = 'commerce_private';
ALTER TABLE IF EXISTS works
    ADD COLUMN IF NOT EXISTS storage_scope VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS generation_records
    ADD COLUMN IF NOT EXISTS storage_scope VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS generation_records
    ADD COLUMN IF NOT EXISTS execution_key VARCHAR(160);
ALTER TABLE IF EXISTS generation_records
    ADD COLUMN IF NOT EXISTS provider_request_started BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE IF EXISTS generation_records
    ADD COLUMN IF NOT EXISTS provider_idempotency_supported BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS commerce_brands (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    logo_reference_asset_id BIGINT,
    name VARCHAR(160) NOT NULL DEFAULT '',
    color_palette_json TEXT NOT NULL DEFAULT '',
    fonts_json TEXT NOT NULL DEFAULT '',
    forbidden_terms_json TEXT NOT NULL DEFAULT '',
    visual_rules_json TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS commerce_products (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    brand_id BIGINT,
    name TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    spu_code TEXT NOT NULL DEFAULT '',
    selling_points_json TEXT NOT NULL DEFAULT '',
    target_channels_json TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS commerce_skus (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    code TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    style TEXT NOT NULL DEFAULT '',
    size TEXT NOT NULL DEFAULT '',
    attributes_json TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS commerce_projects (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    brand_id BIGINT,
    default_sku_id BIGINT,
    active_creative_spec_id BIGINT,
    title TEXT NOT NULL DEFAULT '',
    pipeline TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    default_channel_profile TEXT NOT NULL DEFAULT '',
    deletion_requested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS commerce_assets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    reference_asset_id BIGINT NOT NULL,
    sku_id BIGINT,
    role TEXT NOT NULL DEFAULT '',
    lifecycle TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    metadata_json TEXT NOT NULL DEFAULT '',
    retain_until TIMESTAMPTZ,
    object_deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS commerce_creative_specs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    product_facts_json TEXT NOT NULL DEFAULT '',
    selling_points_json TEXT NOT NULL DEFAULT '',
    forbidden_changes_json TEXT NOT NULL DEFAULT '',
    brand_tone_json TEXT NOT NULL DEFAULT '',
    shot_plan_json TEXT NOT NULL DEFAULT '',
    copy_blocks_json TEXT NOT NULL DEFAULT '',
    risk_notices_json TEXT NOT NULL DEFAULT '',
    source_asset_ids_json TEXT NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_generation_batches (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    creative_spec_id BIGINT,
    parent_batch_id BIGINT,
    reservation_id BIGINT,
    primary_sku_id BIGINT NOT NULL,
    pipeline TEXT NOT NULL DEFAULT '',
    recipe_key TEXT NOT NULL DEFAULT '',
    recipe_version INTEGER NOT NULL DEFAULT 0,
    quality_tier TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    idempotency_key VARCHAR(160) NOT NULL DEFAULT '',
    request_digest VARCHAR(64) NOT NULL DEFAULT '',
    request_snapshot_json TEXT NOT NULL DEFAULT '',
    pricing_version VARCHAR(64) NOT NULL DEFAULT '',
    pricing_snapshot_id VARCHAR(160) NOT NULL DEFAULT '',
    pricing_snapshot_json TEXT NOT NULL DEFAULT '',
    total_items INTEGER NOT NULL DEFAULT 0,
    queued_items INTEGER NOT NULL DEFAULT 0,
    running_items INTEGER NOT NULL DEFAULT 0,
    retrying_items INTEGER NOT NULL DEFAULT 0,
    succeeded_items INTEGER NOT NULL DEFAULT 0,
    failed_items INTEGER NOT NULL DEFAULT 0,
    canceled_items INTEGER NOT NULL DEFAULT 0,
    estimated_credits INTEGER NOT NULL DEFAULT 0,
    reserved_credits INTEGER NOT NULL DEFAULT 0,
    settled_credits INTEGER NOT NULL DEFAULT 0,
    released_credits INTEGER NOT NULL DEFAULT 0,
    eta_seconds INTEGER NOT NULL DEFAULT 0,
    cancel_requested_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_generation_items (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    batch_id BIGINT NOT NULL,
    parent_item_id BIGINT,
    reservation_id BIGINT NOT NULL,
    sku_id BIGINT NOT NULL,
    slot_key VARCHAR(160) NOT NULL DEFAULT '',
    candidate_index INTEGER NOT NULL DEFAULT 0,
    pipeline TEXT NOT NULL DEFAULT '',
    recipe_key TEXT NOT NULL DEFAULT '',
    recipe_version INTEGER NOT NULL DEFAULT 0,
    quality_tier TEXT NOT NULL DEFAULT '',
    pricing_version TEXT NOT NULL DEFAULT '',
    pricing_snapshot_id TEXT NOT NULL DEFAULT '',
    idempotency_key VARCHAR(160) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    input_snapshot_json TEXT NOT NULL DEFAULT '',
    output_spec_json TEXT NOT NULL DEFAULT '',
    estimated_credits INTEGER NOT NULL DEFAULT 0,
    reserved_credits INTEGER NOT NULL DEFAULT 0,
    settled_credits INTEGER NOT NULL DEFAULT 0,
    released_credits INTEGER NOT NULL DEFAULT 0,
    generation_record_id BIGINT,
    work_id BIGINT,
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    batch_id BIGINT,
    generation_item_id BIGINT,
    subject_id BIGINT,
    subject_type VARCHAR(64) NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT '',
    pipeline TEXT NOT NULL DEFAULT '',
    recipe_key TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    idempotency_key VARCHAR(160) NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 0,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    lease_owner VARCHAR(160) NOT NULL DEFAULT '',
    lease_token VARCHAR(160) NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    cancel_requested_at TIMESTAMPTZ,
    payload_json TEXT NOT NULL DEFAULT '',
    result_json TEXT NOT NULL DEFAULT '',
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_commerce_jobs_generation_item
        FOREIGN KEY (generation_item_id) REFERENCES commerce_generation_items(id)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_commerce_jobs_generation_item'
          AND conrelid = 'commerce_jobs'::regclass
    ) THEN
        ALTER TABLE commerce_jobs
            ADD CONSTRAINT fk_commerce_jobs_generation_item
            FOREIGN KEY (generation_item_id) REFERENCES commerce_generation_items(id);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS commerce_credit_reservations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    batch_id BIGINT,
    scope_type VARCHAR(64) NOT NULL DEFAULT '',
    scope_key VARCHAR(64) NOT NULL DEFAULT '',
    idempotency_key VARCHAR(160) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT '',
    total_credits INTEGER NOT NULL DEFAULT 0,
    reserved_credits INTEGER NOT NULL DEFAULT 0,
    settled_credits INTEGER NOT NULL DEFAULT 0,
    released_credits INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS commerce_pricing_snapshots (
    id VARCHAR(160) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    request_digest VARCHAR(64) NOT NULL DEFAULT '',
    snapshot_json TEXT NOT NULL DEFAULT '',
    snapshot_hash VARCHAR(160) NOT NULL DEFAULT '',
    version VARCHAR(160) NOT NULL DEFAULT '',
    status VARCHAR(160) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_credit_settlements (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    batch_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    generation_item_id BIGINT NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL DEFAULT '',
    held_credits INTEGER NOT NULL DEFAULT 0,
    actual_credits INTEGER NOT NULL DEFAULT 0,
    settled_credits INTEGER NOT NULL DEFAULT 0,
    released_credits INTEGER NOT NULL DEFAULT 0,
    anomaly_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    batch_id BIGINT,
    job_id BIGINT,
    entity_type VARCHAR(64) NOT NULL DEFAULT '',
    entity_id BIGINT NOT NULL,
    pipeline TEXT NOT NULL DEFAULT '',
    recipe_key TEXT NOT NULL DEFAULT '',
    event_type VARCHAR(96) NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_object_cleanups (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    commerce_asset_id BIGINT,
    reference_asset_id BIGINT,
    generation_record_id BIGINT,
    work_id BIGINT,
    storage_scope VARCHAR(32) NOT NULL DEFAULT '',
    object_key VARCHAR(512) NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    delete_after TIMESTAMPTZ NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    object_deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS commerce_object_guards (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    storage_scope VARCHAR(32) NOT NULL DEFAULT '',
    object_key VARCHAR(512) NOT NULL DEFAULT '',
    state VARCHAR(32) NOT NULL DEFAULT 'active',
    delete_token VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_object_guards_identity
    ON commerce_object_guards(user_id, storage_scope, object_key);

DO $$
BEGIN
    CREATE TEMP TABLE IF NOT EXISTS commerce_object_guard_backfill_candidates (
        user_id BIGINT NOT NULL,
        object_key TEXT NOT NULL,
        is_active BOOLEAN NOT NULL
    ) ON COMMIT DROP;
    TRUNCATE commerce_object_guard_backfill_candidates;

    IF to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT user_id, asset_key, deleted_at IS NULL
        FROM reference_assets
        WHERE storage_scope = 'commerce_private' AND BTRIM(asset_key) <> '';
    END IF;
    IF to_regclass('works') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT user_id, asset_key, deleted_at IS NULL
        FROM works
        WHERE storage_scope = 'commerce_private' AND BTRIM(asset_key) <> '';
    END IF;
    IF to_regclass('generation_records') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT user_id, asset_key, TRUE
        FROM generation_records
        WHERE storage_scope = 'commerce_private' AND BTRIM(asset_key) <> '';
    END IF;
    IF to_regclass('generation_reference_assets') IS NOT NULL AND to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, TRUE
        FROM generation_reference_assets AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
    END IF;
    IF to_regclass('commerce_assets') IS NOT NULL AND to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, consumer.deleted_at IS NULL AND consumer.object_deleted_at IS NULL
        FROM commerce_assets AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
    END IF;
    IF to_regclass('user_video_style_templates') IS NOT NULL AND to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, consumer.deleted_at IS NULL
        FROM user_video_style_templates AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
    END IF;
    IF to_regclass('couple_albums') IS NOT NULL AND to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, consumer.deleted_at IS NULL
        FROM couple_albums AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.male_reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, consumer.deleted_at IS NULL
        FROM couple_albums AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.female_reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
    END IF;
    IF to_regclass('novel_video_shots') IS NOT NULL AND to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, TRUE
        FROM novel_video_shots AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
    END IF;
    IF to_regclass('commerce_brands') IS NOT NULL AND to_regclass('reference_assets') IS NOT NULL THEN
        INSERT INTO commerce_object_guard_backfill_candidates
        SELECT ra.user_id, ra.asset_key, consumer.deleted_at IS NULL
        FROM commerce_brands AS consumer
        JOIN reference_assets AS ra ON ra.id = consumer.logo_reference_asset_id
        WHERE ra.storage_scope = 'commerce_private' AND BTRIM(ra.asset_key) <> '';
    END IF;

    INSERT INTO commerce_object_guards(user_id, storage_scope, object_key, state, delete_token)
    SELECT user_id, 'commerce_private', object_key,
           CASE WHEN BOOL_OR(is_active) THEN 'active' ELSE 'deleted' END,
           ''
    FROM commerce_object_guard_backfill_candidates
    WHERE user_id <> 0 AND BTRIM(object_key) <> ''
    GROUP BY user_id, object_key
    ON CONFLICT (user_id, storage_scope, object_key) DO NOTHING;

    DROP TABLE commerce_object_guard_backfill_candidates;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_skus_product_code
    ON commerce_skus(product_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_batches_user_idempotency
    ON commerce_generation_batches(user_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_jobs_user_idempotency
    ON commerce_jobs(user_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_items_user_idempotency
    ON commerce_generation_items(user_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_jobs_generation_item
    ON commerce_jobs(generation_item_id) WHERE generation_item_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_reservations_user_idempotency
    ON commerce_credit_reservations(user_id, idempotency_key);
CREATE INDEX IF NOT EXISTS ix_commerce_reservations_scope
    ON commerce_credit_reservations(user_id, scope_type, scope_key);
CREATE INDEX IF NOT EXISTS ix_commerce_items_reservation
    ON commerce_generation_items(reservation_id);
CREATE INDEX IF NOT EXISTS ix_commerce_pricing_snapshots_lookup
    ON commerce_pricing_snapshots(user_id, project_id, request_digest, status);
CREATE INDEX IF NOT EXISTS ix_commerce_pricing_snapshots_expiry
    ON commerce_pricing_snapshots(expires_at);
CREATE INDEX IF NOT EXISTS ix_commerce_jobs_claim
    ON commerce_jobs(status, next_attempt_at, priority DESC, id);
CREATE INDEX IF NOT EXISTS ix_commerce_jobs_lease
    ON commerce_jobs(lease_expires_at);
CREATE INDEX IF NOT EXISTS ix_commerce_jobs_subject
    ON commerce_jobs(user_id, subject_type, subject_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_settlements_item
    ON commerce_credit_settlements(generation_item_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_generation_records_execution_key
    ON generation_records(execution_key) WHERE execution_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_commerce_events_metrics
    ON commerce_events(created_at, event_type, pipeline);

CREATE OR REPLACE FUNCTION commerce_assert_private_object_active(
    guarded_user_id BIGINT,
    guarded_storage_scope TEXT,
    guarded_object_key TEXT
) RETURNS VOID AS $$
DECLARE
    guard_state TEXT;
BEGIN
    IF COALESCE(BTRIM(guarded_storage_scope), '') <> 'commerce_private' THEN
        RETURN;
    END IF;
    SELECT state
    INTO guard_state
    FROM commerce_object_guards
    WHERE user_id = guarded_user_id
      AND storage_scope = 'commerce_private'
      AND object_key = guarded_object_key
    FOR SHARE;
    IF NOT FOUND OR guard_state <> 'active' THEN
        RAISE EXCEPTION 'commerce private object is not active'
            USING ERRCODE = 'check_violation';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION commerce_guard_direct_object_reference() RETURNS TRIGGER AS $$
BEGIN
    IF to_jsonb(NEW) ? 'deleted_at' AND NULLIF(to_jsonb(NEW) ->> 'deleted_at', '') IS NOT NULL THEN
        RETURN NEW;
    END IF;
    PERFORM commerce_assert_private_object_active(NEW.user_id, NEW.storage_scope, NEW.asset_key);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION commerce_guard_reference_asset_consumer() RETURNS TRIGGER AS $$
DECLARE
    reference_column TEXT;
    reference_id BIGINT;
    reference_row RECORD;
BEGIN
    IF to_jsonb(NEW) ? 'deleted_at' AND NULLIF(to_jsonb(NEW) ->> 'deleted_at', '') IS NOT NULL THEN
        RETURN NEW;
    END IF;
    IF to_jsonb(NEW) ? 'object_deleted_at' AND NULLIF(to_jsonb(NEW) ->> 'object_deleted_at', '') IS NOT NULL THEN
        RETURN NEW;
    END IF;
    FOREACH reference_column IN ARRAY TG_ARGV LOOP
        reference_id := NULLIF(to_jsonb(NEW) ->> reference_column, '')::BIGINT;
        IF reference_id IS NULL OR reference_id = 0 THEN
            CONTINUE;
        END IF;
        SELECT user_id, storage_scope, asset_key
        INTO reference_row
        FROM reference_assets
        WHERE id = reference_id;
        IF FOUND THEN
            PERFORM commerce_assert_private_object_active(reference_row.user_id, reference_row.storage_scope, reference_row.asset_key);
        END IF;
    END LOOP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    target RECORD;
    trigger_name TEXT;
BEGIN
    FOR target IN
        SELECT * FROM (VALUES
            ('reference_assets', 'user_id, storage_scope, asset_key, deleted_at'),
            ('works', 'user_id, storage_scope, asset_key, deleted_at'),
            ('generation_records', 'user_id, storage_scope, asset_key')
        ) AS direct_target(table_name, update_columns)
    LOOP
        IF to_regclass(target.table_name) IS NULL THEN
            CONTINUE;
        END IF;
        trigger_name := format('trg_%s_commerce_guard_insert', target.table_name);
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, target.table_name);
        EXECUTE format('CREATE TRIGGER %I BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION commerce_guard_direct_object_reference()', trigger_name, target.table_name);
        trigger_name := format('trg_%s_commerce_guard_update', target.table_name);
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, target.table_name);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OF %s ON %I FOR EACH ROW EXECUTE FUNCTION commerce_guard_direct_object_reference()', trigger_name, target.update_columns, target.table_name);
    END LOOP;

    FOR target IN
        SELECT * FROM (VALUES
            ('commerce_assets', 'reference_asset_id', 'reference_asset_id, deleted_at, object_deleted_at'),
            ('generation_reference_assets', 'reference_asset_id', 'reference_asset_id'),
            ('user_video_style_templates', 'reference_asset_id', 'reference_asset_id, deleted_at'),
            ('couple_albums', 'male_reference_asset_id', 'male_reference_asset_id, deleted_at'),
            ('couple_albums', 'female_reference_asset_id', 'female_reference_asset_id, deleted_at'),
            ('novel_video_shots', 'reference_asset_id', 'reference_asset_id'),
            ('commerce_brands', 'logo_reference_asset_id', 'logo_reference_asset_id, deleted_at')
        ) AS reference_target(table_name, reference_column, update_columns)
    LOOP
        IF to_regclass(target.table_name) IS NULL THEN
            CONTINUE;
        END IF;
        trigger_name := format('trg_%s_%s_commerce_guard_insert', target.table_name, target.reference_column);
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, target.table_name);
        EXECUTE format('CREATE TRIGGER %I BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION commerce_guard_reference_asset_consumer(%L)', trigger_name, target.table_name, target.reference_column);
        trigger_name := format('trg_%s_%s_commerce_guard_update', target.table_name, target.reference_column);
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, target.table_name);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OF %s ON %I FOR EACH ROW EXECUTE FUNCTION commerce_guard_reference_asset_consumer(%L)', trigger_name, target.update_columns, target.table_name, target.reference_column);
    END LOOP;
END $$;
