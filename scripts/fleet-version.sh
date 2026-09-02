#!/usr/bin/env bash
# Print UMKIM release and key version pins.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "=== UMKIM fleet-version ==="
if [[ -f VERSIONS.md ]]; then
  head -25 VERSIONS.md
else
  echo "VERSIONS.md missing" >&2
fi

PINS="${REPO_ROOT}/inventory/group_vars/all/fleet_git_pins.yml"
if [[ -f "$PINS" ]]; then
  echo ""
  echo "=== leftover sibling git pins (unused in v0.2) ==="
  grep -E '^harbor_fleet_.*_git_ref:' "$PINS" || true
fi

if command -v ansible-inventory >/dev/null 2>&1; then
  VAULT_ARGS=()
  [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" ]] && VAULT_ARGS=(--vault-password-file "$ANSIBLE_VAULT_PASSWORD_FILE")
  echo ""
  echo "=== inventory (hub) ==="
  ansible-inventory -i inventory/hosts.yml "${VAULT_ARGS[@]}" --host hub 2>/dev/null \
    | python3 -c "
import json,sys
d=json.load(sys.stdin)
for k in ('umkim_version','fleet_profile','harbor_fleet_image_tag','ocm_bundle_version','k3s_channel'):
    if k in d: print(f'{k}: {d[k]}')
" 2>/dev/null || echo "(ansible-inventory unavailable or vault locked)"
fi
