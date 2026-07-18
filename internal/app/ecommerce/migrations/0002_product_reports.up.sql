ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS observed_facts_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS user_overrides_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS missing_fields_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS suggested_sections_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS analysis_error TEXT NOT NULL DEFAULT '';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS analysis_request_hash VARCHAR(64) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS ix_commerce_creative_specs_analysis_request_hash ON commerce_creative_specs(analysis_request_hash);

CREATE TABLE IF NOT EXISTS commerce_idempotency_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    scope VARCHAR(96) NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL,
    request_digest VARCHAR(64) NOT NULL,
    product_id BIGINT,
    sku_id BIGINT,
    project_id BIGINT,
    creative_spec_id BIGINT,
    job_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_idempotency_scope_key ON commerce_idempotency_records(user_id, scope, idempotency_key);

CREATE TABLE IF NOT EXISTS commerce_ai_invocations (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,
    purpose VARCHAR(96) NOT NULL,
    model_id BIGINT NOT NULL,
    channel_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    latency_ms BIGINT NOT NULL DEFAULT 0,
    provider_request_id VARCHAR(160) NOT NULL DEFAULT '',
    request_asset_ids_json TEXT NOT NULL DEFAULT '[]',
    response_schema_version INTEGER NOT NULL DEFAULT 1,
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_commerce_ai_invocations_job ON commerce_ai_invocations(job_id, created_at);
