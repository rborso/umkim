#!/usr/bin/env bash
# Resolve hub endpoints for fleet-verify tests (WireGuard overlay).
# Inventory ansible_host stays the hub WAN IP; verification uses the WG interface when reachable.
set -euo pipefail

_fv_hub_wireguard_ip() {
  local common="${FV_ROOT:-.}/inventory/group_vars/all/common.yml"
  if [[ -f "${common}" ]]; then
    grep -E '^fleet_hub_wireguard_ip:' "${common}" 2>/dev/null \
      | head -1 | sed -E 's/^[^"]*"([^"]+)".*/\1/' || true
  fi
}

_fv_site_var() {
  local key="$1"
  local site="${FV_ROOT:-.}/inventory/group_vars/all/site.yml"
  if [[ -f "${site}" ]]; then
    grep -E "^${key}:" "${site}" 2>/dev/null | head -1 \
      | sed -E "s/^${key}:[[:space:]]*//" | sed -E 's/^"([^"]*)".*$/\1/' | tr -d '"' || true
  fi
}

fv_hub_endpoint_init() {
  if [[ -n "${FLEET_HUB_IP:-}" ]]; then
    return 0
  fi

  local wg_ip ansible_ip
  wg_ip="$(_fv_hub_wireguard_ip)"
  wg_ip="${wg_ip:-10.10.0.1}"

  if ping -c1 -W1 "${wg_ip}" >/dev/null 2>&1; then
    export FLEET_HUB_IP="${wg_ip}"
    return 0
  fi

  if command -v ansible-inventory >/dev/null 2>&1 \
    && [[ -f "${FV_ROOT:-.}/inventory/hosts.yml" ]]; then
    ansible_ip="$(ansible-inventory -i "${FV_ROOT}/inventory/hosts.yml" --host umkim-hub 2>/dev/null \
      | python3 -c "import json,sys; print(json.load(sys.stdin).get('ansible_host',''))" 2>/dev/null || true)"
    if [[ -n "${ansible_ip}" && "${ansible_ip}" != "127.0.0.1" ]]; then
      export FLEET_HUB_IP="${ansible_ip}"
      return 0
    fi
  fi

  export FLEET_HUB_IP="127.0.0.1"
}

fv_hub_endpoint_export_urls() {
  fv_hub_endpoint_init
  export FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://${FLEET_HUB_IP}:30912}"
  export FLEET_API="${FLEET_API:-http://${FLEET_HUB_IP}:18080/api/v1alpha1}"
  export DCM_API="${DCM_API:-http://${FLEET_HUB_IP}:30908/api/v1alpha1}"
  export HUB_HOST="${HUB_HOST:-${FLEET_HUB_IP}}"
  export HUB_SSH="${HUB_SSH:-linux@${FLEET_HUB_IP}}"
  export FLEET_EDGE_DOMAIN="${FLEET_EDGE_DOMAIN:-$(_fv_site_var fleet_edge_domain)}"
  export FLEET_MANAGEMENT_SITE_ID="${FLEET_MANAGEMENT_SITE_ID:-$(_fv_site_var fleet_management_site_id)}"
}
