DO $$
BEGIN
    IF to_regclass('commerce_idempotency_records') IS NOT NULL AND EXISTS (SELECT 1 FROM commerce_idempotency_records LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back product report migration: commerce_idempotency_records contains business data';
    END IF;
	IF to_regclass('commerce_ai_invocations') IS NOT NULL AND EXISTS (SELECT 1 FROM commerce_ai_invocations LIMIT 1) THEN
		RAISE EXCEPTION 'refusing to roll back product report migration: commerce_ai_invocations contains business data';
	END IF;
    IF EXISTS (SELECT 1 FROM commerce_creative_specs WHERE analysis_request_hash <> '' OR observed_facts_json <> '[]' OR user_overrides_json <> '{}') THEN
        RAISE EXCEPTION 'refusing to roll back product report migration: commerce_creative_specs contains report data';
    END IF;
END $$;

DROP TABLE IF EXISTS commerce_idempotency_records;
DROP TABLE IF EXISTS commerce_ai_invocations;
DROP INDEX IF EXISTS ix_commerce_creative_specs_analysis_request_hash;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS analysis_request_hash;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS analysis_error;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS suggested_sections_json;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS missing_fields_json;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS user_overrides_json;
ALTER TABLE commerce_creative_specs DROP COLUMN IF EXISTS observed_facts_json;
