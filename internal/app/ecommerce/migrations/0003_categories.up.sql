CREATE TABLE IF NOT EXISTS commerce_system_categories (
    id BIGSERIAL PRIMARY KEY,
    parent_id BIGINT REFERENCES commerce_system_categories(id),
    level INTEGER NOT NULL,
    name VARCHAR(80) NOT NULL,
	seed_key VARCHAR(64) NOT NULL DEFAULT '',
    search_aliases_json TEXT NOT NULL DEFAULT '[]',
    sort_order INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    catalog_version VARCHAR(64) NOT NULL DEFAULT 'cn-commerce-v1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_system_categories_parent_name ON commerce_system_categories(COALESCE(parent_id, 0), name);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_system_categories_seed_key ON commerce_system_categories(seed_key) WHERE seed_key <> '';

CREATE TABLE IF NOT EXISTS commerce_user_categories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    parent_id BIGINT NOT NULL REFERENCES commerce_system_categories(id),
    name VARCHAR(80) NOT NULL,
    search_aliases_json TEXT NOT NULL DEFAULT '[]',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_commerce_user_categories_owner_parent_name ON commerce_user_categories(user_id, parent_id, name);

ALTER TABLE commerce_products ADD COLUMN IF NOT EXISTS category_id BIGINT;
ALTER TABLE commerce_products ADD COLUMN IF NOT EXISTS category_source VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE commerce_products ADD COLUMN IF NOT EXISTS category_path VARCHAR(255) NOT NULL DEFAULT '';
