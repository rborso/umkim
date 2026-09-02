#!/usr/bin/env bash
# Inventory profile detection: hub-only vs homelab.
set -euo pipefail

FV_PROFILE="${FV_PROFILE:-auto}"
FV_ROOT="${FV_ROOT:?set FV_ROOT}"

fv_profile_detect() {
  if [[ "${FV_PROFILE}" != "auto" ]]; then
    echo "${FV_PROFILE}"
    return 0
  fi

  local workloads="${FV_ROOT}/inventory/group_vars/all/fleet_workloads.yml"
  local hosts="${FV_ROOT}/inventory/hosts.yml"

  if [[ -f "${workloads}" ]]; then
    if grep -vE '^\s*#' "${workloads}" | grep -qE 'fleet_flow_cluster_name:'; then
      echo "homelab"
      return 0
    fi
  fi

  if [[ -f "${hosts}" ]]; then
    if grep -q 'examples/inventory/hub-only' "${hosts}" 2>/dev/null; then
      echo "hub-only"
      return 0
    fi
  fi

  # Flow examples present + preset clusters in inventory site.yml homelab hints
  if [[ -f "${FV_ROOT}/playbooks/flows/examples/lab-sn01.yml" ]]; then
    if curl -sf --connect-timeout 3 "${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}/clusters" 2>/dev/null \
      | python3 -c "import json,sys; d=json.load(sys.stdin); cs=[c for c in d.get('clusters',[]) if c.get('name') not in ('umkim-hub',)]; sys.exit(0 if cs else 1)" 2>/dev/null; then
      echo "homelab"
      return 0
    fi
  fi

  echo "hub-only"
}

fv_profile_allows_check() {
  local check_profiles_csv="$1"
  local active_profile="$2"
  IFS=',' read -ra profs <<< "${check_profiles_csv}"
  for p in "${profs[@]}"; do
    p="${p// /}"
    [[ -z "$p" ]] && continue
    if [[ "$p" == "$active_profile" ]]; then
      return 0
    fi
  done
  return 1
}

fv_profile_check_requires() {
  local requires_csv="$1"
  local req
  IFS=',' read -ra reqs <<< "${requires_csv}"
  for req in "${reqs[@]}"; do
    req="${req// /}"
    case "${req}" in
      hub) ;;
      fleet_api)
        curl -sf --connect-timeout 3 "${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}/clusters" >/dev/null 2>&1 || return 1
        ;;
      console)
        curl -sf --connect-timeout 3 "${FLEET_CONSOLE_URL:-http://${FLEET_HUB_IP:-127.0.0.1}:30912}/fleet/login" >/dev/null 2>&1 || return 1
        ;;
      kubectl)
        command -v kubectl >/dev/null 2>&1 || return 1
        ;;
      jumphost)
        [[ -n "${FLEET_FLOW_JUMPHOST:-}" ]] || grep -q 'fleet_flow_jumphost' "${FV_ROOT}/playbooks/flows/examples/lab-sn01.yml" 2>/dev/null || return 1
        ;;
      gpu)
        [[ "${FLEET_GPU_CLUSTER:-}" != "" ]] || return 1
        ;;
      vault)
        [[ -f "${HOME}/.umkim-vault-pass" ]] || [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" ]] || return 1
        ;;
      dns)
        [[ -n "${FLEET_EDGE_DOMAIN:-}" ]] || return 1
        ;;
      nfs)
        [[ -n "${FLEET_NFS_SERVER:-203.0.113.40}" ]] || return 1
        ;;
      *) ;;
    esac
  done
  return 0
}
