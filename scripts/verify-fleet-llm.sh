#!/usr/bin/env bash
# Verify FleetLLM CR exists, hub LiteLLM is enabled, and NodePort health responds.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-endpoint.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-endpoint.sh" 2>/dev/null || true
if declare -F fv_hub_endpoint_export_urls >/dev/null 2>&1; then
  fv_hub_endpoint_export_urls
fi
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

API="${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}"
NS="${FLEET_SITE_NS:-fleet-console}"
LITELLM_URL="${FLEET_LITELLM_URL:-http://${FLEET_HUB_IP:-127.0.0.1}:30918}"
EXPECT_ENABLED="${FLEET_LLM_EXPECT_ENABLED:-true}"

echo "=== verify-fleet-llm ==="

RESP=$(curl -sf "${API}/llm") || { echo "FAIL: Fleet API /llm unreachable at ${API}"; exit 1; }
echo "$RESP" | jq '{hubEnabled, phase, message, endpoint: .hub.endpoint}'

PHASE=$(echo "$RESP" | jq -r '.phase // empty')
ENABLED=$(echo "$RESP" | jq -r '.hubEnabled // false')

if [[ "${EXPECT_ENABLED}" == "true" ]]; then
  [[ "$ENABLED" == "true" ]] || { echo "FAIL: expected hubEnabled=true"; exit 1; }
  [[ "$PHASE" == "Ready" ]] || { echo "FAIL: expected phase=Ready got ${PHASE:-empty}"; exit 1; }
else
  [[ "$ENABLED" == "false" ]] || echo "WARN: hubEnabled=true (set FLEET_LLM_EXPECT_ENABLED=true to require)"
fi

hub_kubectl -n "$NS" get fleetllm fleet-llm >/dev/null

ok_health=0
LLM_NS="${FLEET_LLM_NAMESPACE:-fleet-llm}"
MASTER_KEY="$(hub_kubectl -n "$LLM_NS" get secret fleet-llm-master-key -o jsonpath='{.data.master_key}' 2>/dev/null | base64 -d || true)"
for _ in $(seq 1 12); do
  if [[ -n "${MASTER_KEY}" ]] && curl -sf --max-time 5 -H "Authorization: Bearer ${MASTER_KEY}" "${LITELLM_URL}/health" >/dev/null; then
    ok_health=1
    break
  fi
  # Unauthenticated liveliness (same probe path as the Deployment)
  if curl -sf --max-time 5 "${LITELLM_URL}/health/liveliness" >/dev/null; then
    ok_health=1
    break
  fi
  sleep 5
done
if [[ "${ok_health}" != "1" ]]; then
  echo "FAIL: LiteLLM health unreachable at ${LITELLM_URL}/health"
  exit 1
fi
echo "LiteLLM health OK at ${LITELLM_URL}"

echo "=== verify-fleet-llm OK (phase=${PHASE:-unknown}) ==="
