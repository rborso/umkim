#!/usr/bin/env bash
# Validate hub-only inventory before install.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# Default: full inventory (hub-local plays use hosts-from-hub.yml).
INV="${INV:--i ${ROOT}/inventory/hosts.yml -i ${ROOT}/inventory/hosts-from-hub.yml}"
# fleet-verify sets FLEET_HUB_IP to the WG overlay; reachability tests use it without changing inventory.
ANSIBLE_EXTRA=()
if [[ -n "${FLEET_HUB_IP:-}" ]]; then
  INV="-i ${ROOT}/inventory/hosts.yml"
  ANSIBLE_EXTRA=(-e "ansible_host=${FLEET_HUB_IP}")
fi
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
FAIL=0

fail() { echo "FAIL: $*" >&2; FAIL=1; }

echo "=== preflight-hub-inventory ==="
[[ -n "${FLEET_HUB_IP:-}" ]] && echo "verify hub reachability via WG: ${FLEET_HUB_IP}"

if grep -vE '^\s*#' "${ROOT}/inventory/hosts.yml" 2>/dev/null | grep -q 'CHANGE_ME'; then
  fail "inventory/hosts.yml contains CHANGE_ME"
fi

if ! grep -q '^fleet_sites:' "${ROOT}/inventory/group_vars/all/fleet_sites.yml" 2>/dev/null; then
  fail "inventory/group_vars/all/fleet_sites.yml missing (re-run scripts/bootstrap-hub-inventory.sh)"
fi

ansible ${INV} "${ANSIBLE_EXTRA[@]}" hub -m debug -a "var=groups['hub']" >/dev/null 2>&1 \
  || fail "hub group unreachable or missing"

WAN="$(ansible ${INV} "${ANSIBLE_EXTRA[@]}" hub -m debug -a "var=fleet_hub_public_ipv4" 2>/dev/null \
  | grep fleet_hub_public_ipv4 | sed -E 's/.*: "(.*)".*/\1/' | head -1 || true)"
if [[ -z "$WAN" || "$WAN" == "CHANGE_ME" || "$WAN" == "127.0.0.1" || "$WAN" == "localhost" ]]; then
  fail "fleet_hub_public_ipv4 missing or invalid in host_vars/<hub>.yml (set hub WAN IPv4)"
fi

ansible ${INV} "${ANSIBLE_EXTRA[@]}" hub -m debug -a "var=fleet_edge_clusters" 2>/dev/null \
  | grep -q "null" && fail "fleet_edge_clusters is null (use {})"

if [[ "$FAIL" -ne 0 ]]; then
  exit 1
fi
echo "=== preflight-hub-inventory OK ==="
