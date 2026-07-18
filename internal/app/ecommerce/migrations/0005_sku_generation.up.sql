ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS common_facts_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS sku_overrides_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE commerce_creative_specs ADD COLUMN IF NOT EXISTS sku_context_sha256 VARCHAR(64) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS ix_commerce_creative_specs_sku_context_sha256 ON commerce_creative_specs(sku_context_sha256);

ALTER TABLE commerce_generation_items ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'sku';
UPDATE commerce_generation_items SET scope = 'sku' WHERE scope = '' OR scope IS NULL;
