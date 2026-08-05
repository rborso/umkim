#!/usr/bin/env bash
# Verify policy-manager returns selected_providers (Phase 2 fleet images).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
NS="${DCM_NAMESPACE:-dcm}"
EVAL_BODY='{
  "service_instance": {
    "spec": {
      "service_type": "container",
      "metadata": {
        "name": "phase2-verify",
        "labels": {
          "fleet.workload/profile": "demo",
          "fleet.placement/scope": "single"
        }
      },
      "user_values": []
    }
  }
}'

echo "=== verify-placement-phase2 ==="

echo "Policy manager health (gateway)..."
curl -sf "${API%/api/v1alpha1}/health/policies" >/dev/null || curl -sf "${API}/health/policies" >/dev/null

echo "Check RWO placement policies..."
for id in demo-container-single demo-container-ha-standby demo-helm-single demo-helm-ha-standby; do
  POL=$(curl -sf "${API}/policies/${id}" 2>/dev/null || true)
  if [[ -n "$POL" ]]; then
    echo "${id}: enabled=$(echo "$POL" | jq -r '.enabled')"
  else
    echo "WARN: ${id} not seeded"
  fi
done

echo "Evaluate demo container spec (policy engine in-cluster)..."
if ! hub_kubectl get ns "$NS" &>/dev/null; then
  echo "SKIP: kubectl cannot reach cluster; set POLICY_ENGINE to a reachable engine URL"
  exit 0
fi

POLICY_DEPLOY="$(SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"; "$SCRIPT_DIR/dcm-policy-engine-deploy.sh")"
ENGINE_HOST="http://${POLICY_DEPLOY}:8081"

POD="pm-eval-${FV_RUN_ID:-$$}"
RESP=$(hub_kubectl -n "$NS" run "${POD}" --rm -i --restart=Never --image=curlimages/curl:8.11.1 --quiet -- \
  curl -sf -X POST "${ENGINE_HOST}/api/v1alpha1/policies:evaluateRequest" \
  -H 'Content-Type: application/json' \
  -d "$EVAL_BODY")

echo "$RESP" | jq '{selected_provider, selected_providers, rejected}'
PROVIDERS=$(echo "$RESP" | jq -r '.selected_providers // [] | length')
SP=$(echo "$RESP" | jq -r '.selected_provider // ""')
echo "selected_provider=${SP}"
echo "selected_providers count=${PROVIDERS}"

if [[ "$PROVIDERS" -ge 1 || "$SP" == k8s-* || "$SP" == helm-* ]]; then
  echo "provider resolution OK"
else
  echo "FAIL: no selected_providers or selected_provider from Phase 2 evaluate"
  exit 1
fi

if hub_kubectl get deploy -n "$NS" "$POLICY_DEPLOY" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="ENABLE_CLUSTER_PLACEMENT")].value}' 2>/dev/null | grep -q true; then
  echo "${POLICY_DEPLOY} ENABLE_CLUSTER_PLACEMENT=true"
else
  echo "WARN: ENABLE_CLUSTER_PLACEMENT not set on ${POLICY_DEPLOY} (Phase 2 may be inactive)"
fi

IMG=$(hub_kubectl get deploy -n "$NS" "$POLICY_DEPLOY" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
echo "${POLICY_DEPLOY} image=${IMG:-unknown}"

echo "=== verify-placement-phase2 OK ==="
