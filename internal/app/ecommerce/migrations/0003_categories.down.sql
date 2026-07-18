DO $$
BEGIN
    IF to_regclass('commerce_user_categories') IS NOT NULL AND EXISTS (SELECT 1 FROM commerce_user_categories LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back category migration: commerce_user_categories contains business data';
    END IF;
    IF EXISTS (SELECT 1 FROM commerce_products WHERE category_id IS NOT NULL OR category_source <> '' OR category_path <> '') THEN
        RAISE EXCEPTION 'refusing to roll back category migration: commerce_products contains category data';
    END IF;
END $$;

ALTER TABLE commerce_products DROP COLUMN IF EXISTS category_path;
ALTER TABLE commerce_products DROP COLUMN IF EXISTS category_source;
ALTER TABLE commerce_products DROP COLUMN IF EXISTS category_id;
DROP TABLE IF EXISTS commerce_user_categories;
DROP TABLE IF EXISTS commerce_system_categories;
