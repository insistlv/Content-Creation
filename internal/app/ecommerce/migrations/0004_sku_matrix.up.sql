ALTER TABLE commerce_products ADD COLUMN IF NOT EXISTS sku_version INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS commerce_sku_dimensions (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, product_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_dimensions_product REFERENCES commerce_products(id),
 name VARCHAR(80) NOT NULL, version INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0,
 status VARCHAR(32) NOT NULL DEFAULT 'active', created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_dimensions_product_name ON commerce_sku_dimensions(product_id,name);

CREATE TABLE IF NOT EXISTS commerce_sku_values (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, product_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_values_product REFERENCES commerce_products(id),
 dimension_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_values_dimension REFERENCES commerce_sku_dimensions(id), name VARCHAR(80) NOT NULL,
 sort_order INTEGER NOT NULL DEFAULT 0, status VARCHAR(32) NOT NULL DEFAULT 'active',
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_values_dimension_name ON commerce_sku_values(dimension_id,name);

CREATE TABLE IF NOT EXISTS commerce_sku_value_links (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, product_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_value_links_product REFERENCES commerce_products(id),
 sku_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_value_links_sku REFERENCES commerce_skus(id), value_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_value_links_value REFERENCES commerce_sku_values(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_value_links_sku_value ON commerce_sku_value_links(sku_id,value_id);

CREATE TABLE IF NOT EXISTS commerce_sku_matrix_requests (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, product_id BIGINT NOT NULL CONSTRAINT fk_commerce_sku_matrix_requests_product REFERENCES commerce_products(id),
 idempotency_key VARCHAR(160) NOT NULL, request_digest VARCHAR(64) NOT NULL, response_json TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_sku_matrix_requests_owner_key ON commerce_sku_matrix_requests(user_id,product_id,idempotency_key);

DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_dimensions_product') THEN ALTER TABLE commerce_sku_dimensions ADD CONSTRAINT fk_commerce_sku_dimensions_product FOREIGN KEY (product_id) REFERENCES commerce_products(id); END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_values_product') THEN ALTER TABLE commerce_sku_values ADD CONSTRAINT fk_commerce_sku_values_product FOREIGN KEY (product_id) REFERENCES commerce_products(id); END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_values_dimension') THEN ALTER TABLE commerce_sku_values ADD CONSTRAINT fk_commerce_sku_values_dimension FOREIGN KEY (dimension_id) REFERENCES commerce_sku_dimensions(id); END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_value_links_product') THEN ALTER TABLE commerce_sku_value_links ADD CONSTRAINT fk_commerce_sku_value_links_product FOREIGN KEY (product_id) REFERENCES commerce_products(id); END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_value_links_sku') THEN ALTER TABLE commerce_sku_value_links ADD CONSTRAINT fk_commerce_sku_value_links_sku FOREIGN KEY (sku_id) REFERENCES commerce_skus(id); END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_value_links_value') THEN ALTER TABLE commerce_sku_value_links ADD CONSTRAINT fk_commerce_sku_value_links_value FOREIGN KEY (value_id) REFERENCES commerce_sku_values(id); END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_commerce_sku_matrix_requests_product') THEN ALTER TABLE commerce_sku_matrix_requests ADD CONSTRAINT fk_commerce_sku_matrix_requests_product FOREIGN KEY (product_id) REFERENCES commerce_products(id); END IF;
END $$;
