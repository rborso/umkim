#!/usr/bin/env bash
# UMKIM v0.1.0 preflight orchestrator.
# Usage:
#   ./scripts/preflight-v1.sh
#   FLEET_PROFILE=production ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass ./scripts/preflight-v1.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FLEET_PROFILE="${FLEET_PROFILE:-lab}"
export FLEET_PROFILE
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
STRICT=0
[[ "$FLEET_PROFILE" == "production" ]] && STRICT=1
[[ "${PREFLIGHT_STRICT:-$STRICT}" == "1" ]] && STRICT=1

fail() { echo "[preflight-v1] FAIL $*" >&2; FAILED=1; }
warn() { echo "[preflight-v1] WARN $*" >&2; }
ok()   { echo "[preflight-v1] OK   $*"; }

FAILED=0
echo "[preflight-v1] profile=$FLEET_PROFILE"

# 1. Secrets
if ! "$REPO_ROOT/scripts/preflight-secrets.sh"; then
  [[ "$STRICT" == "1" ]] && FAILED=1 || warn "preflight-secrets"
fi

# 2. VERSIONS.md
if [[ -f "$REPO_ROOT/VERSIONS.md" ]]; then
  ok "VERSIONS.md"
else
  [[ "$STRICT" == "1" ]] && fail "missing VERSIONS.md" || warn "missing VERSIONS.md"
fi

# 3. Ansible collections
if command -v ansible-galaxy >/dev/null 2>&1 && [[ -f "$REPO_ROOT/requirements.yml" ]]; then
  if ansible-galaxy collection list 2>/dev/null | grep -qE 'community\.general|ansible\.posix'; then
    ok "ansible collections"
  else
    [[ "$STRICT" == "1" ]] && fail "run: ansible-galaxy collection install -r requirements.yml" || warn "collections may be missing"
  fi
else
  warn "ansible-galaxy or requirements.yml not found"
fi

# 4. Hub SSH (optional)
HUB_HOST="${HUB_HOST:-${FLEET_HUB_IP:-}}"
if [[ -z "$HUB_HOST" ]] && command -v ansible-inventory >/dev/null 2>&1; then
  HUB_HOST="$(ansible-inventory -i "$REPO_ROOT/inventory/hosts.yml" --host umkim-hub 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('ansible_host',''))" 2>/dev/null || true)"
fi
if [[ -n "$HUB_HOST" ]]; then
  if ssh -n -o BatchMode=yes -o ConnectTimeout=5 "linux@${HUB_HOST}" 'echo ok' >/dev/null 2>&1; then
    ok "hub SSH linux@${HUB_HOST}"
  else
    warn "hub SSH unreachable (linux@${HUB_HOST})"
  fi
fi

# 5. Ansible production preflight play
if [[ "$STRICT" == "1" ]]; then
  VAULT_ARGS=()
  [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" ]] && VAULT_ARGS=(--vault-password-file "$ANSIBLE_VAULT_PASSWORD_FILE")
  if ansible-playbook "$REPO_ROOT/playbooks/preflight.yml" \
      -e "fleet_profile=production" "${VAULT_ARGS[@]}"; then
    ok "ansible playbooks/preflight.yml"
  else
    fail "ansible playbooks/preflight.yml (production checks)"
  fi
fi

if [[ "$FAILED" != "0" ]]; then
  echo "[preflight-v1] aborted — fix failures above (see docs/install.md)"
  exit 1
fi
echo "[preflight-v1] passed"
exit 0
