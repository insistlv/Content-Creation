ALTER TABLE commerce_generation_items DROP COLUMN IF EXISTS scope;
DROP INDEX IF EXISTS ix_commerce_creative_specs_sku_context_sha256;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS sku_context_sha256;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS sku_overrides_json;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS common_facts_json;
