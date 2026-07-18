#!/usr/bin/env sh
set -eu

destination="${1:-}"
if [ -z "$destination" ]; then
  echo "usage: scripts/export-public-snapshot.sh <new-empty-directory>" >&2
  exit 2
fi
if [ -e "$destination" ]; then
  echo "destination must not already exist" >&2
  exit 2
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "working tree must be clean before export" >&2
  exit 2
fi

mkdir -p "$destination"
archive="$(mktemp "${TMPDIR:-/tmp}/image-agent-public.XXXXXX.tar")"
trap 'rm -f "$archive"' EXIT

git archive --format=tar --output="$archive" HEAD -- \
  .env.example .gitattributes .github .gitignore \
  ASSET_LICENSES.md CODE_OF_CONDUCT.md CONTRIBUTING.md DCO Dockerfile LICENSE README.md SECURITY.md THIRD_PARTY_NOTICES.md compose.yaml \
  cmd docs go.mod go.sum internal mobile-h5 scripts web
tar -xf "$archive" -C "$destination"

(
  cd "$destination"
  git init -b main
  git add -A
  git commit -s -m "chore: 导入 image-agent 开源快照"
)

echo "clean public snapshot created at $destination"
