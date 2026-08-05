#!/usr/bin/env bash
# Validate policy-manager fleet-phase2 fork resolves cluster → provider without Rego selected_provider.
# Run on hub (or with DCM_API + kubectl to dcm namespace).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
NS="${DCM_NAMESPACE:-dcm}"

echo "=== validate-policy-manager-phase2-fork ==="

echo "Check placement policies seeded..."
for id in demo-container-single demo-container-ha-standby demo-helm-single demo-helm-ha-standby; do
  POL=$(curl -sf "${API}/policies/${id}" 2>/dev/null || true)
  if [[ -z "$POL" ]]; then
    echo "WARN: policy ${id} not found — run dcm_seed_placement_policies on hub"
    continue
  fi
  ENABLED=$(echo "$POL" | jq -r '.enabled')
  RWO=$(echo "$POL" | jq -r '.cluster_requirements.matchLabels["fleet.capability/storage.rwo"] // ""')
  echo "${id}: enabled=${ENABLED} storage.rwo=${RWO}"
done

echo "Placement preview (container, RWO clusters)..."
PREVIEW=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d '{"mode":"deploy","service_type":"container","catalog_item_id":"demo-webapp","label_selector":{"fleet.workload/profile":"demo","fleet.placement/scope":"single"}}' \
  2>/dev/null || echo '{}')
echo "$PREVIEW" | jq '{deployable, eligible_count, blocking_reasons}' 2>/dev/null || echo "$PREVIEW"
DEPLOYABLE=$(echo "$PREVIEW" | jq -r '.deployable // false')
COUNT=$(echo "$PREVIEW" | jq -r '.eligible_count // 0')
if [[ "$DEPLOYABLE" != "true" || "$COUNT" -lt 1 ]]; then
  echo "WARN: placement-preview not deployable (RWO clusters + k8s providers may be missing)"
fi

if hub_kubectl get ns "$NS" &>/dev/null; then
  echo "In-cluster evaluate (accept-only rego, cluster_requirements)..."
  POLICY_DEPLOY="$(SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"; "$SCRIPT_DIR/dcm-policy-engine-deploy.sh")"
  ENGINE_HOST="http://${POLICY_DEPLOY}:8081"
  EVAL_BODY='{
    "service_instance": {
      "spec": {
        "service_type": "container",
        "metadata": {
          "name": "fork-validate",
          "labels": {
            "fleet.workload/profile": "demo",
            "fleet.placement/scope": "single"
          }
        },
        "user_values": []
      }
    }
  }'
  POD="pm-fork-${FV_RUN_ID:-$$}"
  RESP='{}'
  for attempt in 1 2 3; do
    RESP=$(hub_kubectl -n "$NS" run "${POD}" --rm -i --restart=Never --image=curlimages/curl:8.11.1 --quiet -- \
      curl -sf -X POST "${ENGINE_HOST}/api/v1alpha1/policies:evaluateRequest" \
      -H 'Content-Type: application/json' \
      -d "$EVAL_BODY" 2>/dev/null || echo '{}')
    PROVIDERS=$(echo "$RESP" | jq -r '.selected_providers // [] | length' 2>/dev/null || echo 0)
    SP=$(echo "$RESP" | jq -r '.selected_provider // ""' 2>/dev/null || echo "")
    if [[ "$PROVIDERS" -ge 1 || "$SP" == k8s-* ]]; then
      break
    fi
    POD="pm-fork-${FV_RUN_ID:-$$}-${attempt}"
    sleep 2
  done
  echo "$RESP" | jq '{selected_provider, selected_providers, rejected}' 2>/dev/null || echo "$RESP"
  PROVIDERS=$(echo "$RESP" | jq -r '.selected_providers // [] | length')
  SP=$(echo "$RESP" | jq -r '.selected_provider // ""')
  if [[ "$PROVIDERS" -ge 1 || "$SP" == k8s-* ]]; then
    echo "Phase 2 provider resolution OK (selected_providers or k8s-* selected_provider)"
  else
    echo "FAIL: policy engine did not resolve providers from cluster_requirements"
    exit 1
  fi
  IMG=$(hub_kubectl get deploy -n "$NS" "$POLICY_DEPLOY" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
  echo "${POLICY_DEPLOY} image=${IMG:-unknown}"
else
  echo "SKIP in-cluster evaluate (kubectl unavailable)"
fi

echo "=== validate-policy-manager-phase2-fork OK ==="
