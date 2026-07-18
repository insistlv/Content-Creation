DO $$
DECLARE
    table_name TEXT;
    has_rows BOOLEAN;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'commerce_brands',
        'commerce_products',
        'commerce_skus',
        'commerce_projects',
        'commerce_assets',
        'commerce_creative_specs',
        'commerce_generation_batches',
        'commerce_generation_items',
        'commerce_jobs',
        'commerce_credit_reservations',
        'commerce_pricing_snapshots',
        'commerce_credit_settlements',
        'commerce_events',
        'commerce_object_cleanups',
        'commerce_object_guards'
    ]
    LOOP
        IF to_regclass(table_name) IS NOT NULL THEN
            EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I LIMIT 1)', table_name) INTO has_rows;
            IF has_rows THEN
                RAISE EXCEPTION 'refusing to roll back foundation migration: table % contains business data', table_name;
            END IF;
        END IF;
    END LOOP;
END $$;

DO $$
DECLARE
    target RECORD;
BEGIN
    FOR target IN
        SELECT * FROM (VALUES
            ('reference_assets', 'trg_reference_assets_commerce_guard_insert'),
            ('reference_assets', 'trg_reference_assets_commerce_guard_update'),
            ('works', 'trg_works_commerce_guard_insert'),
            ('works', 'trg_works_commerce_guard_update'),
            ('generation_records', 'trg_generation_records_commerce_guard_insert'),
            ('generation_records', 'trg_generation_records_commerce_guard_update'),
            ('commerce_assets', 'trg_commerce_assets_reference_asset_id_commerce_guard_insert'),
            ('commerce_assets', 'trg_commerce_assets_reference_asset_id_commerce_guard_update'),
            ('generation_reference_assets', 'trg_generation_reference_assets_reference_asset_id_commerce_guard_insert'),
            ('generation_reference_assets', 'trg_generation_reference_assets_reference_asset_id_commerce_guard_update'),
            ('user_video_style_templates', 'trg_user_video_style_templates_reference_asset_id_commerce_guard_insert'),
            ('user_video_style_templates', 'trg_user_video_style_templates_reference_asset_id_commerce_guard_update'),
            ('couple_albums', 'trg_couple_albums_male_reference_asset_id_commerce_guard_insert'),
            ('couple_albums', 'trg_couple_albums_male_reference_asset_id_commerce_guard_update'),
            ('couple_albums', 'trg_couple_albums_female_reference_asset_id_commerce_guard_insert'),
            ('couple_albums', 'trg_couple_albums_female_reference_asset_id_commerce_guard_update'),
            ('novel_video_shots', 'trg_novel_video_shots_reference_asset_id_commerce_guard_insert'),
            ('novel_video_shots', 'trg_novel_video_shots_reference_asset_id_commerce_guard_update'),
            ('commerce_brands', 'trg_commerce_brands_logo_reference_asset_id_commerce_guard_insert'),
            ('commerce_brands', 'trg_commerce_brands_logo_reference_asset_id_commerce_guard_update')
        ) AS trigger_target(table_name, trigger_name)
    LOOP
        IF to_regclass(target.table_name) IS NOT NULL THEN
            EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', target.trigger_name, target.table_name);
        END IF;
    END LOOP;
END $$;

DROP FUNCTION IF EXISTS commerce_guard_reference_asset_consumer();
DROP FUNCTION IF EXISTS commerce_guard_direct_object_reference();
DROP FUNCTION IF EXISTS commerce_assert_private_object_active(BIGINT, TEXT, TEXT);

DROP INDEX IF EXISTS ux_reference_assets_storage_object;
DROP TABLE IF EXISTS commerce_jobs;
DROP TABLE IF EXISTS commerce_generation_items;
DROP TABLE IF EXISTS commerce_credit_settlements;
DROP TABLE IF EXISTS commerce_credit_reservations;
DROP TABLE IF EXISTS commerce_generation_batches;
DROP TABLE IF EXISTS commerce_pricing_snapshots;
DROP TABLE IF EXISTS commerce_events;
DROP TABLE IF EXISTS commerce_object_cleanups;
DROP TABLE IF EXISTS commerce_object_guards;
DROP TABLE IF EXISTS commerce_creative_specs;
DROP TABLE IF EXISTS commerce_assets;
DROP TABLE IF EXISTS commerce_projects;
DROP TABLE IF EXISTS commerce_skus;
DROP TABLE IF EXISTS commerce_products;
DROP TABLE IF EXISTS commerce_brands;

-- Parent-table compatibility columns are intentionally preserved on rollback:
-- generation_records.provider_request_started and
-- generation_records.provider_idempotency_supported may already be consumed by
-- application binaries that predate or outlive the Commerce foundation tables.
