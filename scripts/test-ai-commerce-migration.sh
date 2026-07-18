#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/.." && pwd)

if [[ -z "${TEST_DATABASE_URL:-}" ]]; then
	if [[ "${CI:-}" == "true" ]]; then
		echo "TEST_DATABASE_URL is required in CI" >&2
		exit 1
	fi
	echo "SKIP: TEST_DATABASE_URL is not set"
	exit 0
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/image-agent-commerce-migration.XXXXXX")
trap 'rm -rf -- "${tmp_dir}"' EXIT
binary="${tmp_dir}/database-migrate"
(cd "${REPO_ROOT}" && go build -o "${binary}" ./cmd/database-migrate)

log_file="${tmp_dir}/migration.log"
DATABASE_URL="${TEST_DATABASE_URL}" MIGRATION_BINARY="${binary}" bash "${SCRIPT_DIR}/migrate-ai-commerce.sh" >"${log_file}" 2>&1
if grep -Fq -- "${TEST_DATABASE_URL}" "${log_file}"; then
	echo "migration output leaked TEST_DATABASE_URL" >&2
	exit 1
fi
grep -Fq "migration verify succeeded" "${log_file}"
echo "ai commerce migration command ok"
