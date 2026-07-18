#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/.." && pwd)
MIGRATION_BINARY="${MIGRATION_BINARY:-${REPO_ROOT}/database-migrate}"

if [[ -z "${DATABASE_URL:-}" ]]; then
	echo "DATABASE_URL is required" >&2
	exit 1
fi
if [[ ! -x "${MIGRATION_BINARY}" ]]; then
	echo "database migration binary is missing or not executable" >&2
	exit 1
fi

"${MIGRATION_BINARY}" -scope ai-commerce -action up
"${MIGRATION_BINARY}" -scope ai-commerce -action verify
