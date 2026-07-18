#!/usr/bin/env sh
set -eu

forbidden='(root@|DEPLOY_SSH_PASSWORD|BEGIN [A-Z ]*PRIVATE KEY)'
legacy_ip="39.97.$(printf '%s' '232.148')"
legacy_deploy="127.0.0.1:$(printf '%s%s' '180' '88')"
if git grep -I -n -E "$forbidden" -- ':!scripts/check-public-snapshot.sh' \
  || git grep -I -n -F "$legacy_ip" -- ':!scripts/check-public-snapshot.sh' \
  || git grep -I -n -F "$legacy_deploy" -- ':!scripts/check-public-snapshot.sh'; then
  echo "public snapshot contains private operations markers" >&2
  exit 1
fi

tracked_env="$(git ls-files | grep -E '(^|/)[.]env([.]|$)' | grep -v -E '(^|/)[.]env[.]example$' || true)"
if [ -n "$tracked_env" ]; then
  echo "tracked environment files are forbidden:" >&2
  echo "$tracked_env" >&2
  exit 1
fi

if git ls-files | grep -E '([.]pem|[.]key|[.]p12|[.]pfx|[.]db|[.]sqlite3?)$'; then
  echo "tracked key, certificate, or database artifact detected" >&2
  exit 1
fi
