#!/usr/bin/env bash
# Greenfield hub install wrapper (defaults to production profile + vault).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export INV="${INV:--i inventory/hosts.yml -i inventory/hosts-from-hub.yml}"
FLEET_PROFILE="${FLEET_PROFILE:-production}"
export FLEET_PROFILE

PROFILE_ARGS=(-e "fleet_profile=${FLEET_PROFILE}")

VAULT_ARGS=()
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
if [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${ANSIBLE_VAULT_PASSWORD_FILE}" ]]; then
  VAULT_ARGS=(--vault-password-file "$ANSIBLE_VAULT_PASSWORD_FILE")
elif [[ "$FLEET_PROFILE" == "production" ]]; then
  echo "ERROR: FLEET_PROFILE=production requires encrypted inventory/group_vars/all/vault.yml" >&2
  echo "       and ANSIBLE_VAULT_PASSWORD_FILE (default: ~/.umkim-vault-pass). See docs/install.md §3." >&2
  exit 1
fi

echo "=== install-hub ==="
echo "INV=${INV} FLEET_PROFILE=${FLEET_PROFILE}"
ansible-playbook ${INV} hub-install.yml -l hub "${PROFILE_ARGS[@]}" "${VAULT_ARGS[@]}" "$@"

echo ""
echo "=== Next: verify ==="
echo "./scripts/verify-hub-install.sh"
echo ""
echo "=== Then (console): create workload site + FleetClusterJoin cluster ==="
echo "Deploy demo-webapp after k8s provider is Ready on a joined cluster."
