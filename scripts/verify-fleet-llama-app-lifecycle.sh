#!/usr/bin/env bash
# FleetApp llama-cpp profile install, PATCH upgrade, LLM backend sync, cleanup.
#
#   FLEET_LLM_CLUSTER=lab-gpu-0 ./scripts/verify-fleet-llama-app-lifecycle.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="${FLEET_SITE_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_LLM_CLUSTER:-lab-gpu-0}"
CHART="${FLEET_LLM_CHART:-llama-cpp-helm}"
VERSION="${FLEET_LLM_CHART_VERSION:-0.1.4}"
PROFILE="${FLEET_LLM_PROFILE:-qwen35-2b-ud-4g}"
RUN_ID="${FV_RUN_ID:-$(date +%s)}"
NAME="${FLEET_LLM_VERIFY_APP:-llama-cpp-cli-verify-${RUN_ID}}"
RELEASE="${FLEET_LLM_RELEASE:-llama-cpp-verify}"
NS="${FLEET_LLM_NAMESPACE:-llm}"

HDR=(-H 'Content-Type: application/json' -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-verify' -H 'X-Fleet-DC-Admin: true')

log() { echo "[llama-lifecycle] $*"; }

wait_app_phase() {
  local phase=""
  for _ in $(seq 1 120); do
    curl -sf "${HDR[@]}" "${API}/apps/${NAME}" -o /tmp/llama-verify-app.json
    phase=$(jq -r '.phase // empty' /tmp/llama-verify-app.json)
    log "app phase=${phase}"
    if [[ "$phase" == "Ready" || "$phase" == "Failed" ]]; then
      break
    fi
    sleep 5
  done
  [[ "$phase" == "Ready" ]]
}

log "=== verify-fleet-llama-app-lifecycle cluster=${CLUSTER} app=${NAME} ==="

profiles=$(curl -sf "${HDR[@]}" "${API}/charts/llama-cpp-helm/profiles")
echo "$profiles" | jq -e --arg p "$PROFILE" '.profiles[]|select(.id==$p)' >/dev/null

values_yaml=$(curl -sf "${HDR[@]}" "${API}/charts/llama-cpp-helm/profiles/${PROFILE}" | jq -r '.valuesYaml')
if [[ -z "$values_yaml" ]]; then
  log "FAIL empty profile valuesYaml"
  exit 1
fi

http=$(curl -sS -o /tmp/llama-verify-create.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/apps" \
  -d "$(jq -n --arg name "$NAME" --arg cluster "$CLUSTER" --arg chart "$CHART" --arg version "$VERSION" \
    --arg release "$RELEASE" --arg ns "$NS" --arg values "$values_yaml" \
    '{name:$name,clusterName:$cluster,chart:$chart,version:$version,releaseName:$release,namespace:$ns,route:{mode:"none"},valuesYaml:$values}')")
if [[ "$http" != "201" && "$http" != "409" ]]; then
  log "FAIL POST /apps HTTP ${http}"
  cat /tmp/llama-verify-create.json
  exit 1
fi

wait_app_phase || { log "FAIL install not Ready"; exit 1; }

patched_yaml=$(echo "$values_yaml" | sed 's/ctxSize: 8192/ctxSize: 4096/')
http=$(curl -sS -o /tmp/llama-verify-patch.json -w '%{http_code}' "${HDR[@]}" -X PATCH "${API}/apps/${NAME}" \
  -d "$(jq -n --arg values "$patched_yaml" '{valuesYaml:$values}')")
if [[ "$http" != "200" ]]; then
  log "FAIL PATCH /apps HTTP ${http}"
  cat /tmp/llama-verify-patch.json
  exit 1
fi

wait_app_phase || { log "FAIL patch rollout not Ready"; exit 1; }

http=$(curl -sS -o /tmp/llama-verify-sync.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/llm/backends/sync-from-apps" -d '{}')
if [[ "$http" != "200" ]]; then
  log "FAIL POST sync-from-apps HTTP ${http}"
  cat /tmp/llama-verify-sync.json
  exit 1
fi
jq -e --arg model "$(echo "$values_yaml" | awk -F': ' '/^model:/{print $2}')" \
  '.backends[]|select(.model==$model)' /tmp/llama-verify-sync.json >/dev/null || {
  log "FAIL expected model in backends after sync"
  cat /tmp/llama-verify-sync.json
  exit 1
}

curl -sf "${HDR[@]}" -X DELETE "${API}/apps/${NAME}" >/dev/null || true
log "=== verify-fleet-llama-app-lifecycle OK ==="
