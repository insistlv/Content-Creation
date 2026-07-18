DO $$
DECLARE
 table_name TEXT;
 has_business_data BOOLEAN;
BEGIN
 FOREACH table_name IN ARRAY ARRAY[
  'commerce_sku_matrix_requests',
  'commerce_sku_value_links',
  'commerce_sku_values',
  'commerce_sku_dimensions'
 ] LOOP
  IF to_regclass(table_name) IS NOT NULL THEN
   EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I LIMIT 1)', table_name) INTO has_business_data;
   IF has_business_data THEN
    RAISE EXCEPTION 'refusing to roll back SKU matrix migration: table % contains business data', table_name;
   END IF;
  END IF;
 END LOOP;
END $$;
DROP TABLE IF EXISTS commerce_sku_matrix_requests;
DROP TABLE IF EXISTS commerce_sku_value_links;
DROP TABLE IF EXISTS commerce_sku_values;
DROP TABLE IF EXISTS commerce_sku_dimensions;
ALTER TABLE commerce_products DROP COLUMN IF EXISTS sku_version;
