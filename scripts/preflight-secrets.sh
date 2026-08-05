#!/usr/bin/env bash
# Validate Ansible Vault secrets before deploy.
# Usage:
#   FLEET_PROFILE=production ./scripts/preflight-secrets.sh
#   FLEET_PROFILE=production ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass ./scripts/preflight-secrets.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FLEET_PROFILE="${FLEET_PROFILE:-lab}"
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
STRICT=0
[[ "$FLEET_PROFILE" == "production" ]] && STRICT=1
[[ "${PREFLIGHT_STRICT:-$STRICT}" == "1" ]] && STRICT=1

VAULT_FILE="${VAULT_FILE:-$REPO_ROOT/inventory/group_vars/all/vault.yml}"
PLACEHOLDER_RE='change-me|ChangeMe|REPLACE_ME|fleet-admin-change-me|FleetHarbor-ChangeMe|FleetDCM-ChangeMe'

fail() { echo "[preflight-secrets] FAIL $*" >&2; exit 1; }
warn() { echo "[preflight-secrets] WARN $*" >&2; }
ok()   { echo "[preflight-secrets] OK   $*"; }

echo "[preflight-secrets] profile=$FLEET_PROFILE strict=$STRICT"

if [[ "$FLEET_PROFILE" == "production" ]]; then
  if [[ ! -f "$VAULT_FILE" ]]; then
    fail "missing $VAULT_FILE — copy vault.example.yml, fill, encrypt (docs/secrets.md)"
  fi
  if ! head -1 "$VAULT_FILE" | grep -q '^\$ANSIBLE_VAULT'; then
    fail "vault.yml must be ansible-vault encrypted for production"
  fi
  ok "vault file present and encrypted"
else
  if [[ -f "$VAULT_FILE" ]] && head -1 "$VAULT_FILE" | grep -q '^\$ANSIBLE_VAULT'; then
    ok "encrypted vault.yml found (optional for lab)"
  else
    warn "no encrypted vault.yml (acceptable for lab profile)"
  fi
fi

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
    if [[ "$required" == "1" && "$STRICT" == "1" ]]; then
      fail "$name unset (vault or inventory)"
    elif [[ "$required" == "1" ]]; then
      warn "$name unset"
    fi
    return 0
  fi
  if echo "$val" | grep -qiE "$PLACEHOLDER_RE"; then
    if [[ "$STRICT" == "1" ]]; then
      fail "$name contains placeholder value"
    else
      warn "$name contains placeholder value"
    fi
  else
    ok "$name"
  fi
}

if [[ "$STRICT" == "1" ]]; then
  check_var dcm_postgres_password 1
  check_var harbor_admin_password 1
  check_var fleet_console_auth_session_secret 1
fi

echo "[preflight-secrets] done"
exit 0
