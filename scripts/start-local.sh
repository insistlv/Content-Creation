#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/.." && pwd)

PORT="${PORT:-8888}"
SKIP_WEB_BUILD="${SKIP_WEB_BUILD:-0}"

if [[ ! -f "${REPO_ROOT}/.env" ]]; then
	if [[ -f "${REPO_ROOT}/.env.example" ]]; then
		cp "${REPO_ROOT}/.env.example" "${REPO_ROOT}/.env"
		echo "Created .env from .env.example. Update OPENAI_API_KEY before generating images." >&2
	else
		echo ".env not found and .env.example is missing." >&2
		exit 1
	fi
fi

if ! command -v go >/dev/null 2>&1; then
	echo "Go is required but was not found in PATH." >&2
	exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
	echo "npm is required but was not found in PATH." >&2
	exit 1
fi

if [[ "${SKIP_WEB_BUILD}" != "1" ]]; then
	echo "Building web assets..." >&2
	(
		cd "${REPO_ROOT}/web"
		if [[ ! -d node_modules ]]; then
			npm install
		fi
		npm run build
	)
fi

export APP_BASE_URL="${APP_BASE_URL:-http://localhost:${PORT}}"
export FRONTEND_DIST_PATH="${FRONTEND_DIST_PATH:-web/dist}"
export LISTEN_ADDR="${LISTEN_ADDR:-:${PORT}}"

echo "Starting IMAGE AGENT at ${APP_BASE_URL}" >&2
echo "Press Ctrl+C to stop." >&2
(
	cd "${REPO_ROOT}"
	go run ./cmd/server
)
