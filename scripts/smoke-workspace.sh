#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${SMOKE_BASE_URL:-https://example.com}"
BASE_URL="${BASE_URL%/}"
COOKIE_JAR="$(mktemp)"
trap 'rm -f "${COOKIE_JAR}"' EXIT

curl_json() {
  curl -fsS "$@"
}

echo "Checking ${BASE_URL}/workspace"
workspace_html="$(curl_json "${BASE_URL}/workspace")"
if ! grep -qi '<html' <<<"${workspace_html}"; then
  echo "workspace page did not return HTML" >&2
  exit 1
fi

echo "Checking ${BASE_URL}/api/workspace/discovery"
discovery_json="$(curl_json "${BASE_URL}/api/workspace/discovery")"
DISCOVERY_JSON="${discovery_json}" node <<'NODE'
const payload = JSON.parse(process.env.DISCOVERY_JSON || '{}')
for (const key of ['tools', 'models', 'hot', 'inspiration']) {
  if (!Array.isArray(payload[key])) {
    throw new Error(`discovery.${key} is not an array`)
  }
}
console.log(`discovery counts: tools=${payload.tools.length}, models=${payload.models.length}, hot=${payload.hot.length}, inspiration=${payload.inspiration.length}`)
NODE

if [[ -n "${SMOKE_USER_USERNAME:-}" && -n "${SMOKE_USER_PASSWORD:-}" ]]; then
  echo "Checking logged-in estimate only; no generation will be submitted"
  curl_json \
    -c "${COOKIE_JAR}" \
    -H 'Content-Type: application/json' \
    -H 'X-Image-Agent-Client: mp-weixin' \
    -d "{\"username\":\"${SMOKE_USER_USERNAME}\",\"password\":\"${SMOKE_USER_PASSWORD}\"}" \
    "${BASE_URL}/api/auth/login" >/dev/null

  estimate_json="$(curl_json \
    -b "${COOKIE_JAR}" \
    -H 'Content-Type: application/json' \
    -H 'X-Image-Agent-Client: mp-weixin' \
    -d '{"prompt":"只做 smoke 点数估算，不提交生成","aspect_ratio":"1:1","tool_mode":"generate"}' \
    "${BASE_URL}/api/images/generations/estimate")"
  ESTIMATE_JSON="${estimate_json}" node <<'NODE'
const payload = JSON.parse(process.env.ESTIMATE_JSON || '{}')
if (!Number.isFinite(Number(payload.required_credits)) || Number(payload.required_credits) <= 0) {
  throw new Error('estimate.required_credits is missing or invalid')
}
console.log(`estimate required_credits=${payload.required_credits}, available_credits=${payload.available_credits}`)
NODE
else
  echo "Skipping logged-in estimate; set SMOKE_USER_USERNAME and SMOKE_USER_PASSWORD to enable it"
fi

echo "workspace smoke passed"
