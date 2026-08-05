#!/usr/bin/env bash
# Fail if tracked tip contains maintainer PII unsuitable for a public OSS tree.
# Usage: ./scripts/preflight-oss-scrub.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FAIL=0

fail() {
  echo "FAIL: $*" >&2
  FAIL=1
}

# Build patterns without embedding denylisted literals in this file's searchable body.
# shellcheck disable=SC2028
DOM='otthol'"."'hu'
WAN='159'"."'195'"."'137'"."'119'

hits="$(git grep -nE "${DOM}" -- . ':(exclude)reports' ':(exclude)docs/archive' ':(exclude)scripts/preflight-oss-scrub.sh' 2>/dev/null || true)"
if [[ -n "$hits" ]]; then
  echo "$hits"
  fail "tracked files contain personal DNS zone (${DOM})"
fi

hits="$(git grep -nE "${WAN}" -- . ':(exclude)reports' ':(exclude)scripts/preflight-oss-scrub.sh' 2>/dev/null || true)"
if [[ -n "$hits" ]]; then
  echo "$hits"
  fail "tracked files contain maintainer WAN IP"
fi

hits="$(git grep -nE '/home/linux/' -- inventory examples 2>/dev/null || true)"
if [[ -n "$hits" ]]; then
  echo "$hits"
  fail "tracked inventory/examples contain /home/linux/ paths"
fi

if git ls-files 'inventory/extra_vars/*' | grep -q .; then
  fail "inventory/extra_vars/* is tracked — remove from git"
fi
if git ls-files --error-unmatch inventory/group_vars/hub/lab-webui-extras.yml >/dev/null 2>&1; then
  fail "inventory/group_vars/hub/lab-webui-extras.yml is tracked — use examples only / gitignore"
fi

hits="$(git grep -nE 'ansible_host:[[:space:]]*[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' -- inventory/hosts.yml inventory/host_vars 2>/dev/null || true)"
if echo "$hits" | grep -vE '127\.0\.0\.1|0\.0\.0\.0|192\.0\.2\.|198\.51\.100\.|203\.0\.113\.' | grep -q .; then
  echo "$hits"
  fail "inventory hosts use a non-documentation public IP — use CHANGE_ME or RFC5737"
fi

hits="$(git grep -nE 'fleet_hub_public_ipv4:[[:space:]]*[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' -- inventory/host_vars 2>/dev/null || true)"
if echo "$hits" | grep -vE '127\.0\.0\.1|192\.0\.2\.|198\.51\.100\.|203\.0\.113\.' | grep -q .; then
  echo "$hits"
  fail "fleet_hub_public_ipv4 uses a non-documentation public IP — use CHANGE_ME"
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "[preflight-oss-scrub] FAILED" >&2
  exit 1
fi

echo "[preflight-oss-scrub] OK"
