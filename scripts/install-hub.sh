#!/usr/bin/env bash
# Greenfield hub install wrapper. Requires encrypted vault.yml.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export INV="${INV:--i inventory/hosts.yml -i inventory/hosts-from-hub.yml}"

VAULT_ARGS=()
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
if [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${ANSIBLE_VAULT_PASSWORD_FILE}" ]]; then
  VAULT_ARGS=(--vault-password-file "$ANSIBLE_VAULT_PASSWORD_FILE")
else
  echo "ERROR: hub install requires encrypted inventory/group_vars/all/vault.yml" >&2
  echo "       and ANSIBLE_VAULT_PASSWORD_FILE (default: ~/.umkim-vault-pass). See docs/install.md." >&2
  exit 1
fi

echo "=== install-hub ==="
echo "INV=${INV}"
ansible-playbook ${INV} hub-install.yml -l hub "${VAULT_ARGS[@]}" "$@"

echo ""
echo "=== Next: verify ==="
echo "./scripts/verify-hub-install.sh"
echo ""
echo "=== Then (console): create workload site + FleetClusterJoin cluster ==="
echo "Deploy apps via FleetApp (v0.2) after a cluster joins and is Ready."
echo ""
echo "To wipe and reinstall from zero: ./scripts/teardown-hub.sh --confirm"
