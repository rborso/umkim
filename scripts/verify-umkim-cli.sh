#!/usr/bin/env bash
# Smoke-test umkim CLI against hub fleet API (dev headers or login).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLI="${UMKIM_BIN:-$ROOT/cmd/umkim/umkim}"
BASE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
API_URL="${UMKIM_API_URL:-${BASE}/fleet/api/v1alpha1}"
AUTH_URL="${UMKIM_AUTH_URL:-${BASE}/fleet/auth/v1alpha1}"
CLUSTER="${UMKIM_VERIFY_CLUSTER:-lab-ha-0}"
USER="${FLEET_AUTH_USER:-localadmin}"
PASS="${FLEET_AUTH_PASSWORD:-}"
if [[ -z "$PASS" && -f "${HOME}/.umkim-vault-pass" ]]; then
  PASS="$(ansible-vault view "${ROOT}/inventory/group_vars/all/vault.yml" --vault-password-file "${HOME}/.umkim-vault-pass" 2>/dev/null \
    | awk -F': ' '/fleet_console_auth_admin_password_vault/{gsub(/^"|"$/, "", $2); print $2}')"
fi
PASS="${PASS:-fleet-admin-change-me}"

if [[ ! -x "$CLI" ]]; then
  echo "building umkim..."
  (cd "$ROOT/cmd/umkim" && go build -o umkim .)
fi

export UMKIM_API_URL="$API_URL"
export UMKIM_AUTH_URL="$AUTH_URL"

if [[ "${UMKIM_DEV:-}" == "1" ]]; then
  echo "== umkim config show (dev) =="
  UMKIM_DEV=1 "$CLI" config show | head -c 200
  echo
  echo "== umkim cluster list (dev) =="
  UMKIM_DEV=1 "$CLI" cluster list | head -c 400
  echo
  echo "== umkim site list -o wide (dev) =="
  UMKIM_DEV=1 "$CLI" site list -o wide | head -c 400
  echo
  echo "== umkim node-op history (dev) =="
  UMKIM_DEV=1 "$CLI" node-op history --cluster "$CLUSTER" | head -c 400
  echo
  echo "== umkim app list (dev) =="
  UMKIM_DEV=1 "$CLI" app list | head -c 400
  echo
  echo "== umkim infra get hub (dev) =="
  UMKIM_DEV=1 "$CLI" infra get hub -o table | head -c 400
  echo
  echo "umkim CLI verify OK (dev mode)"
  exit 0
fi

echo "== umkim login =="
UMKIM_PASSWORD="$PASS" "$CLI" login -u "$USER"

echo "== umkim whoami =="
"$CLI" whoami | head -c 300
echo

echo "== umkim cluster list =="
"$CLI" cluster list | head -c 400
echo

echo "== umkim cluster get $CLUSTER =="
"$CLI" cluster get "$CLUSTER" | head -c 400
echo

SITE_FOR_PATCH="${UMKIM_VERIFY_SITE:-homelab-pf}"
if [[ -n "$SITE_FOR_PATCH" ]]; then
  echo "== umkim site patch jumphost round-trip ($SITE_FOR_PATCH) =="
  PATCH_FILE="$(mktemp)"
  RESTORE_FILE="$(mktemp)"
  trap 'rm -f "$PATCH_FILE" "$RESTORE_FILE"' EXIT
  BEFORE="$("$CLI" site get "$SITE_FOR_PATCH")"
  echo "$BEFORE" >"$RESTORE_FILE"
  CURRENT_JH="$(echo "$BEFORE" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("spec",{}).get("jumphost",""))' 2>/dev/null || true)"
  TEST_JH="verify-cli@127.0.0.1:22"
  printf '{"jumphost":"%s"}' "$TEST_JH" >"$PATCH_FILE"
  "$CLI" site patch "$SITE_FOR_PATCH" --file "$PATCH_FILE" >/dev/null
  AFTER_JH="$("$CLI" site get "$SITE_FOR_PATCH" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("spec",{}).get("jumphost",""))')"
  if [[ "$AFTER_JH" != "$TEST_JH" ]]; then
    echo "site patch jumphost failed: got $AFTER_JH want $TEST_JH" >&2
    exit 1
  fi
  if [[ -n "$CURRENT_JH" ]]; then
    printf '{"jumphost":"%s"}' "$CURRENT_JH" >"$RESTORE_FILE"
    "$CLI" site patch "$SITE_FOR_PATCH" --file "$RESTORE_FILE" >/dev/null
  else
    printf '{"jumphost":""}' >"$RESTORE_FILE"
    "$CLI" site patch "$SITE_FOR_PATCH" --file "$RESTORE_FILE" >/dev/null
  fi
fi

echo "== umkim node-op history =="
"$CLI" node-op history --cluster "$CLUSTER" | head -c 400
echo

echo "== umkim app list =="
"$CLI" app list | head -c 400
echo

echo "== umkim infra get hub =="
"$CLI" infra get hub -o table | head -c 400
echo

echo "== umkim auth users list =="
if "$CLI" auth users list 2>/dev/null | head -c 200; then
  echo
else
  echo "(skipped: requires dcAdmin or auth unavailable)"
fi

echo "umkim CLI verify OK"
