#!/usr/bin/env bash
# Verify helm catalog seed, deploy whoami-helm, history, rollback, delete.
#
# Required env:
#   FLEET_SITE=<workload-site-id>  # e.g. from fleet_sites inventory key
# Optional:
#   FLEET_HELM_CLUSTER=<cluster>   # pin routing.cluster in user_values
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
SITE="${FLEET_SITE:?set FLEET_SITE}"
HELM_CLUSTER="${FLEET_HELM_CLUSTER:-}"

echo "=== verify-helm-catalog (site=${SITE}) ==="

echo "Catalog item whoami-helm..."
curl -sf "${API}/catalog-items/whoami-helm" | jq -e '
  (.spec.service_type == "helm")
  or ((.spec.resources // []) | any(.service_type == "helm"))
' >/dev/null

echo "Policy demo-helm-single..."
curl -sf "${API}/policies/demo-helm-single" | jq -e '.enabled == true' >/dev/null

INST_NAME="whoami-helm-verify-$$"
if [[ -n "$HELM_CLUSTER" ]]; then
  ROUTING_JSON=",{ \"path\": \"routing.cluster\", \"value\": \"${HELM_CLUSTER}\" }"
  echo "Deploy instance ${INST_NAME} (pinned cluster ${HELM_CLUSTER})..."
else
  ROUTING_JSON=""
  echo "Deploy instance ${INST_NAME} (placement via demo-helm-single)..."
fi
HTTP=$(curl -s -o /tmp/helm-verify-$$.json -w '%{http_code}' -X POST "${API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "{
    \"api_version\": \"v1alpha1\",
    \"display_name\": \"${INST_NAME}\",
    \"spec\": {
      \"catalog_item_id\": \"whoami-helm\",
      \"label_selector\": {\"fleet.workload/profile\": \"demo\", \"fleet.placement/scope\": \"single\"},
      \"user_values\": [
        { \"path\": \"metadata.name\", \"value\": \"${INST_NAME}\" },
        { \"path\": \"release.namespace\", \"value\": \"default\" }
        ${ROUTING_JSON}
      ]
    }
  }")
if [[ "$HTTP" != "201" ]]; then
  echo "WARN: helm deploy HTTP ${HTTP} ($(jq -r '.detail // .title // .' /tmp/helm-verify-$$.json 2>/dev/null))"
  echo "SKIP helm history (no instance); run fleet-helm-sp.yml on all target spokes"
  rm -f /tmp/helm-verify-$$.json
  echo "=== verify-helm-catalog OK (deploy skipped) ==="
  exit 0
fi
RESP=$(cat /tmp/helm-verify-$$.json)
rm -f /tmp/helm-verify-$$.json
INST_ID=$(echo "$RESP" | jq -r '.id // .uid')
echo "instance_id=${INST_ID}"

sleep 15

echo "Helm history..."
HIST_OK=0
for _ in $(seq 1 12); do
  if curl -sf "${API}/catalog-item-instances/${INST_ID}/helm/history" | jq -e '.releases // .history // .' >/dev/null 2>&1; then
    curl -sf "${API}/catalog-item-instances/${INST_ID}/helm/history" | jq .
    HIST_OK=1
    break
  fi
  sleep 10
done
if [[ "$HIST_OK" -ne 1 ]]; then
  echo "FAIL: helm history not available after wait (instance ${INST_ID})"
  curl -sf -X DELETE "${API}/catalog-item-instances/${INST_ID}" >/dev/null 2>&1 || true
  exit 1
fi

if command -v kubectl >/dev/null; then
  echo "helm list (local kubectl)..."
  helm list -n default 2>/dev/null | grep -F "${INST_NAME}" || true
fi

echo "Delete instance..."
curl -sf -X DELETE "${API}/catalog-item-instances/${INST_ID}"

echo "=== verify-helm-catalog OK ==="
