#!/usr/bin/env bash
# Enable and configure hub FleetLLM (LiteLLM proxy) after install-hub.
# Idempotent: safe to re-run when fleet_llm_enabled is already true.
set -euo pipefail

ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"
# shellcheck source=scripts/fleet-verify/lib/hub-endpoint.sh
source "${FRAMEWORK}/lib/hub-endpoint.sh"
fv_hub_endpoint_export_urls
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${FRAMEWORK}/lib/hub-kubectl.sh"

INV="${INV:--i ${ROOT}/inventory/hosts.yml}"
if [[ -f "${ROOT}/inventory/hosts-from-hub.yml" ]]; then
  INV="-i ${ROOT}/inventory/hosts.yml -i ${ROOT}/inventory/hosts-from-hub.yml"
fi
VAULT_ARGS=()
[[ -f "${HOME}/.umkim-vault-pass" ]] && VAULT_ARGS=(--vault-password-file "${HOME}/.umkim-vault-pass")

API="${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}"
NS="${FLEET_SITE_NS:-fleet-console}"
LITELLM_URL="${FLEET_LITELLM_URL:-http://${FLEET_HUB_IP:-127.0.0.1}:30918}"
CR="${FLEET_LLM_CR_NAME:-fleet-llm}"

log() { echo "[configure-fleet-llm] $*"; }
fail() { echo "[configure-fleet-llm] FAIL: $*" >&2; exit 1; }

enabled_in_inventory() {
  grep -E '^fleet_llm_enabled:' "${ROOT}/inventory/group_vars/all/common.yml" 2>/dev/null \
    | grep -qE 'true' && return 0
  grep -E '^fleet_llm_enabled:' "${ROOT}/inventory/group_vars/hub/"*.yml 2>/dev/null \
    | grep -qE 'true' && return 0
  [[ "${FLEET_LLM_ENABLED:-}" == "true" || "${FLEET_LLM_ENABLED:-}" == "1" ]]
}

if ! enabled_in_inventory && [[ "${FLEET_LLM_FORCE:-0}" != "1" ]]; then
  log "SKIP: fleet_llm_enabled is false (set FLEET_LLM_FORCE=1 to enable anyway)"
  exit 0
fi

log "ensure FleetLLM CR hubEnabled=true"
curl -sf "${API}/llm" >/dev/null || fail "Fleet API /llm unreachable at ${API}"
hub_kubectl -n "${NS}" get fleetllm "${CR}" >/dev/null \
  || fail "FleetLLM CR ${CR} missing (run fleet-site-crd.yml)"

hub_kubectl -n "${NS}" patch fleetllm "${CR}" --type merge \
  -p '{"spec":{"hubEnabled":true}}' >/dev/null || true

log "run fleet-llm.yml (LiteLLM + Traefik routes)"
ansible-playbook ${INV} "${ROOT}/fleet-llm.yml" -l hub \
  -e fleet_llm_enabled=true \
  "${VAULT_ARGS[@]}"

log "trigger controller reconcile"
hub_kubectl -n "${NS}" annotate fleetllm "${CR}" \
  fleet.umkim.com/llm-reconcile="$(date +%s)" --overwrite >/dev/null || true

log "wait for FleetLLM Ready"
phase=""
enabled=""
for _ in $(seq 1 60); do
  read -r phase enabled < <(curl -sf "${API}/llm" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('phase',''), str(d.get('hubEnabled',False)).lower())" 2>/dev/null || echo " unknown")
  if [[ "${phase}" == "Ready" && "${enabled}" == "true" ]]; then
    break
  fi
  sleep 5
done
[[ "${phase}" == "Ready" ]] || fail "FleetLLM phase=${phase:-unknown} (want Ready)"
[[ "${enabled}" == "true" ]] || fail "hubEnabled=${enabled} (want true)"

log "wait for LiteLLM NodePort ${LITELLM_URL}/health"
MASTER_KEY="$(hub_kubectl -n dcm get secret fleet-llm-master-key -o jsonpath='{.data.master_key}' 2>/dev/null | base64 -d || true)"
for _ in $(seq 1 36); do
  if [[ -n "${MASTER_KEY}" ]] && curl -sf --max-time 5 -H "Authorization: Bearer ${MASTER_KEY}" "${LITELLM_URL}/health" >/dev/null 2>&1; then
    log "OK FleetLLM Ready; LiteLLM responding at ${LITELLM_URL}"
    exit 0
  fi
  if curl -sf --max-time 5 "${LITELLM_URL}/health/liveliness" >/dev/null 2>&1; then
    log "OK FleetLLM Ready; LiteLLM responding at ${LITELLM_URL}"
    exit 0
  fi
  sleep 5
done
fail "LiteLLM health not reachable at ${LITELLM_URL}/health"
