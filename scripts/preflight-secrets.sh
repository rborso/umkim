#!/usr/bin/env bash
# Validate Ansible Vault secrets before deploy.
# Usage:
#   ./scripts/preflight-secrets.sh
#   ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass ./scripts/preflight-secrets.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
STRICT=1

VAULT_FILE="${VAULT_FILE:-$REPO_ROOT/inventory/group_vars/all/vault.yml}"
PLACEHOLDER_RE='change-me|ChangeMe|REPLACE_ME|fleet-admin-change-me|FleetHarbor-ChangeMe|FleetDCM-ChangeMe'

fail() { echo "[preflight-secrets] FAIL $*" >&2; exit 1; }
warn() { echo "[preflight-secrets] WARN $*" >&2; }
ok()   { echo "[preflight-secrets] OK   $*"; }

echo "[preflight-secrets] strict=$STRICT"

if [[ ! -f "$VAULT_FILE" ]]; then
  fail "missing $VAULT_FILE — copy vault.example.yml, fill, encrypt (docs/secrets.md)"
fi
if ! head -1 "$VAULT_FILE" | grep -q '^\$ANSIBLE_VAULT'; then
  fail "vault.yml must be ansible-vault encrypted"
fi
ok "vault file present and encrypted"

# Resolve vars via ansible debug (full Jinja templating) when vault password available
INV_ARGS=(-i "$REPO_ROOT/inventory/hosts.yml")
VAULT_ARGS=()
if [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${ANSIBLE_VAULT_PASSWORD_FILE}" ]]; then
  VAULT_ARGS=(--vault-password-file "$ANSIBLE_VAULT_PASSWORD_FILE")
elif [[ -n "${ANSIBLE_VAULT_PASSWORD:-}" ]]; then
  VAULT_ARGS=(--vault-password-file <(printf '%s' "$ANSIBLE_VAULT_PASSWORD"))
fi

check_var() {
  local name="$1" required="$2"
  local val=""
  if command -v ansible >/dev/null 2>&1; then
    val="$(ansible hub "${INV_ARGS[@]}" "${VAULT_ARGS[@]}" -m debug -a "var=$name" -o 2>/dev/null \
      | python3 -c 'import json,sys
name = sys.argv[1]
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
if "=> " in raw:
    raw = raw.split("=> ", 1)[1].strip()
try:
    d = json.loads(raw)
except json.JSONDecodeError:
    sys.exit(0)
if name in d and d[name] is not None:
    print(d[name])
' "$name" 2>/dev/null || true)"
  fi
  if [[ -z "$val" ]]; then
    if [[ "$required" == "1" ]]; then
      fail "$name unset (vault or inventory)"
    fi
    return 0
  fi
  if echo "$val" | grep -qiE "$PLACEHOLDER_RE"; then
    fail "$name contains placeholder value"
  else
    ok "$name"
  fi
}

check_var harbor_admin_password 1
check_var fleet_console_auth_session_secret 1

echo "[preflight-secrets] done"
exit 0
