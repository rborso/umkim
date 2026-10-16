#!/usr/bin/env bash
# Delete and recreate a cluster from inventory preset or --cluster-spec flow YAML.
set -euo pipefail

ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"
PRESET="${FLEET_VERIFY_CLUSTER_PRESET:-${FV_CLUSTER_PRESET:-}}"
SPEC="${FLEET_VERIFY_CLUSTER_SPEC:-${FV_CLUSTER_SPEC:-}}"
INV="-i ${ROOT}/inventory/hosts.yml"
ANSIBLE_EXTRA=()
if [[ -n "${FLEET_HUB_IP:-}" ]]; then
  ANSIBLE_EXTRA=(-e "ansible_host=${FLEET_HUB_IP}")
elif [[ -f "${ROOT}/inventory/hosts-from-hub.yml" ]]; then
  INV="-i ${ROOT}/inventory/hosts.yml -i ${ROOT}/inventory/hosts-from-hub.yml"
fi

VAULT_ARGS=()
[[ -f "${HOME}/.umkim-vault-pass" ]] && VAULT_ARGS=(--vault-password-file "${HOME}/.umkim-vault-pass")

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

log() { echo "[cluster-reinstall] $*"; }
fail() { echo "[cluster-reinstall] FAIL: $*" >&2; exit 1; }

[[ -n "${PRESET}" || -n "${SPEC}" ]] || fail "set FLEET_VERIFY_CLUSTER_PRESET or FLEET_VERIFY_CLUSTER_SPEC"

resolve_flow() {
  if [[ -n "${SPEC}" ]]; then
    echo "${ROOT}/${SPEC#${ROOT}/}"
    return
  fi
  python3 -c "
import yaml, sys
from pathlib import Path
p = Path(sys.argv[1])
presets = yaml.safe_load(p.read_text())
name = sys.argv[2]
if name not in presets:
    raise SystemExit(f'unknown preset {name}')
flow = (presets.get(name) or {}).get('flow') or ''
if not flow:
    raise SystemExit(
        'preset %s has no flow file in this public tree; '
        'set FLEET_VERIFY_CLUSTER_SPEC to a local extra-vars file '
        '(hub inventory: examples/inventory/hub-only, replace CHANGE_ME)' % name)
print(sys.argv[3] + '/' + flow)
" "${FRAMEWORK}/presets/clusters.yaml" "${PRESET}" "${ROOT}"
}

resolve_site_flow() {
  python3 -c "
import yaml, sys
from pathlib import Path
p = Path(sys.argv[1])
presets = yaml.safe_load(p.read_text())
name = sys.argv[2]
sf = presets.get(name, {}).get('site_flow')
print(sys.argv[3] + '/' + sf if sf else '')
" "${FRAMEWORK}/presets/clusters.yaml" "${PRESET}" "${ROOT}" 2>/dev/null || true
}

FLOW_YML="$(resolve_flow)"
[[ -f "${FLOW_YML}" ]] || fail "flow file not found: ${FLOW_YML}${PRESET:+ (preset '${PRESET}' references it)} — set FLEET_VERIFY_CLUSTER_SPEC to a local flow/extra-vars file"

CLUSTER="$(grep -E '^fleet_flow_cluster_name:' "${FLOW_YML}" | head -1 | sed -E 's/^[^:]*:\s*//' | tr -d ' \"')"
[[ -n "${CLUSTER}" ]] || fail "fleet_flow_cluster_name missing in ${FLOW_YML}"

SITE_FLOW="$(resolve_site_flow)"
if [[ -n "${SITE_FLOW}" && -f "${SITE_FLOW}" ]]; then
  log "ensure site via ${SITE_FLOW}"
  ansible-playbook ${INV} "${ANSIBLE_EXTRA[@]}" "${ROOT}/playbooks/flows/site-create.yml" \
    -e "@${SITE_FLOW}" "${VAULT_ARGS[@]}" || true
fi

if [[ -n "${PRESET}" ]] && python3 -c "import yaml; d=yaml.safe_load(open('${FRAMEWORK}/presets/clusters.yaml')); exit(0 if d.get('${PRESET}',{}).get('gpu_host_prep') else 1)" 2>/dev/null; then
  if [[ "${FV_SKIP_GPU_HOST_PREP:-0}" != "1" ]]; then
    log "gpu host prep via ${FLOW_YML} (fleet-verify preset; uses flow playbook before FCJ exists)"
    ansible-playbook ${INV} "${ANSIBLE_EXTRA[@]}" "${ROOT}/playbooks/flows/gpu-host-prep.yml" \
      -e "@${FLOW_YML}" "${VAULT_ARGS[@]}"
  else
    log "SKIP gpu host prep (FV_SKIP_GPU_HOST_PREP=1)"
  fi
fi

log "delete cluster ${CLUSTER} (cleanupKubernetes=true)"
ansible-playbook ${INV} "${ANSIBLE_EXTRA[@]}" "${ROOT}/playbooks/flows/cluster-delete.yml" \
  -e "@${FLOW_YML}" \
  -e fleet_flow_cleanup_kubernetes=true \
  "${VAULT_ARGS[@]}"

for _ in $(seq 1 30); do
  code=$(curl -s -o /dev/null -w '%{http_code}' "${API}/clusters/${CLUSTER}" || echo 000)
  [[ "${code}" == "404" ]] && break
  sleep 10
done
code=$(curl -s -o /dev/null -w '%{http_code}' "${API}/clusters/${CLUSTER}" || echo 000)
[[ "${code}" == "404" ]] || fail "cluster still listed after delete (HTTP ${code})"

log "create cluster ${CLUSTER}"
ansible-playbook ${INV} "${ANSIBLE_EXTRA[@]}" "${ROOT}/playbooks/flows/cluster-create.yml" \
  -e "@${FLOW_YML}" \
  -e fleet_flow_poll_retries="${FLEET_FLOW_POLL_RETRIES:-90}" \
  "${VAULT_ARGS[@]}"

wait_ready() {
  local retries="${1:-90}" delay="${2:-20}"
  for ((i = 0; i < retries; i++)); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))" 2>/dev/null || echo "")
    avail=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('available',False))" 2>/dev/null || echo "False")
    if [[ "${phase}" == "Ready" && "${avail}" == "True" ]]; then
      log "cluster ${CLUSTER} Ready"
      return 0
    fi
    sleep "${delay}"
  done
  fail "timeout waiting for Ready (last phase=${phase})"
}

PRESET_RETRIES="$(python3 -c "import yaml; d=yaml.safe_load(open('${FRAMEWORK}/presets/clusters.yaml')); print(d.get('${PRESET}',{}).get('poll_retries',90))" 2>/dev/null || echo 90)"
PRESET_INTERVAL="$(python3 -c "import yaml; d=yaml.safe_load(open('${FRAMEWORK}/presets/clusters.yaml')); print(d.get('${PRESET}',{}).get('poll_interval',20))" 2>/dev/null || echo 20)"
wait_ready "${PRESET_RETRIES}" "${PRESET_INTERVAL}"

install_providers() {
  for kind in k8s helm; do
    log "POST providers/${kind}/install"
    code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API}/clusters/${CLUSTER}/providers/${kind}/install" || echo 000)
    [[ "${code}" == "200" || "${code}" == "202" || "${code}" == "409" ]] || fail "provider ${kind} install HTTP ${code}"
  done
  for _ in $(seq 1 60); do
    k8s=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; d=json.load(sys.stdin); print((d.get('providers') or {}).get('k8s',{}).get('phase',''))" 2>/dev/null || echo "")
    helm=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; d=json.load(sys.stdin); print((d.get('providers') or {}).get('helm',{}).get('phase',''))" 2>/dev/null || echo "")
    if [[ "${k8s}" == "Ready" && "${helm}" == "Ready" ]]; then
      log "providers Ready"
      return 0
    fi
    sleep 10
  done
  fail "providers not Ready"
}

install_providers

# Optional post_ready from preset
if [[ -n "${PRESET}" ]]; then
  if python3 -c "import yaml; d=yaml.safe_load(open('${FRAMEWORK}/presets/clusters.yaml')); exit(0 if d.get('${PRESET}',{}).get('post_ready',{}).get('acme_enable') else 1)" 2>/dev/null; then
    curl -sf -X POST "${API}/clusters/${CLUSTER}/acme/enable" >/dev/null || log "WARN acme enable"
  fi
  if python3 -c "import yaml; d=yaml.safe_load(open('${FRAMEWORK}/presets/clusters.yaml')); exit(0 if d.get('${PRESET}',{}).get('post_ready',{}).get('ingress_auth_enable') else 1)" 2>/dev/null; then
    pass="${FLEET_INGRESS_AUTH_PASSWORD:-}"
    if [[ -z "${pass}" && -f "${HOME}/.umkim-vault-pass" ]]; then
      pass=$(ansible-vault view "${ROOT}/inventory/group_vars/all/vault.yml" \
        --vault-password-file "${HOME}/.umkim-vault-pass" 2>/dev/null \
        | awk -F'"' '/fleet_lldap_admin_password_vault/{print $2; exit}')
    fi
    curl -sf -X POST "${API}/clusters/${CLUSTER}/ingress-auth/enable" \
      -H 'Content-Type: application/json' \
      -d "{\"lldapAdminPassword\":\"${pass:-CHANGE_ME}\"}" >/dev/null || log "WARN ingress-auth enable"
  fi
  if python3 -c "import yaml; d=yaml.safe_load(open('${FRAMEWORK}/presets/clusters.yaml')); exit(0 if d.get('${PRESET}',{}).get('post_ready',{}).get('gpu_cluster_enable') else 1)" 2>/dev/null; then
    log "gpu runtime enable via fleet API"
    code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API}/clusters/${CLUSTER}/gpu/enable" \
      -H 'Content-Type: application/json' -d '{}' || echo 000)
    [[ "${code}" == "200" || "${code}" == "202" || "${code}" == "409" ]] || fail "gpu enable HTTP ${code}"
  fi
fi

log "OK cluster-reinstall ${CLUSTER}"
