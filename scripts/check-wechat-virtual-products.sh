#!/usr/bin/env bash

set -euo pipefail

env_file="${1:-.env}"

if [[ ! -f "${env_file}" ]]; then
	echo "Local .env not found at ${env_file}" >&2
	exit 1
fi

env_value_from_file() {
	local file="$1"
	local key="$2"

	awk -v key="${key}" '
	index($0, key "=") == 1 {
		print substr($0, length(key) + 2)
		exit
	}
	' "${file}"
}

trim() {
	local value="$1"
	value="${value#"${value%%[![:space:]]*}"}"
	value="${value%"${value##*[![:space:]]}"}"
	printf '%s' "${value}"
}

database_url=$(trim "$(env_value_from_file "${env_file}" "DATABASE_URL")")
database_url="${database_url%\"}"
database_url="${database_url#\"}"
database_url="${database_url%\'}"
database_url="${database_url#\'}"

if [[ -z "${database_url}" ]]; then
	echo "DATABASE_URL is required in ${env_file}." >&2
	exit 1
fi

psql "${database_url}" -v ON_ERROR_STOP=1 <<'SQL'
WITH active_packages AS (
	SELECT
		id,
		name,
		price_cents,
		credits,
		COALESCE(wechat_virtual_product_id, '') AS wechat_virtual_product_id
	FROM packages
	WHERE deleted_at IS NULL AND is_active = TRUE
	ORDER BY sort_order ASC, id ASC
)
SELECT
	id,
	name,
	price_cents,
	credits,
	wechat_virtual_product_id,
	CASE
		WHEN TRIM(wechat_virtual_product_id) = '' THEN 'missing'
		ELSE 'configured'
	END AS virtual_product_status
FROM active_packages;

DO $$
DECLARE
	missing_count integer;
BEGIN
	SELECT COUNT(*)
	INTO missing_count
	FROM packages
	WHERE deleted_at IS NULL
		AND is_active = TRUE
		AND TRIM(COALESCE(wechat_virtual_product_id, '')) = '';

	IF missing_count > 0 THEN
		RAISE EXCEPTION 'active packages missing wechat_virtual_product_id: %', missing_count;
	END IF;
END $$;
SQL
