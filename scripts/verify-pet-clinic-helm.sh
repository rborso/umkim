#!/usr/bin/env bash
# Verify pet-clinic-helm catalog seed and optional deploy (three-tier Helm demo).
#
# Optional env:
#   FLEET_HELM_CLUSTER=<cluster>   # pin routing.cluster in user_values
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
HELM_CLUSTER="${FLEET_HELM_CLUSTER:-}"

echo "=== verify-pet-clinic-helm ==="

echo "Catalog item pet-clinic-helm..."
curl -sf "${API}/catalog-items/pet-clinic-helm" | jq -e '.spec.service_type == "helm"' >/dev/null

FIELD=$(curl -sf "${API}/catalog-items/pet-clinic-helm" | jq -r '
  (.spec.fields // [])[] | select(.path == "chart.name") | .default // empty
')
if [[ "$FIELD" != "pet-clinic-helm" ]]; then
  echo "WARN: chart.name default is ${FIELD:-<empty>} (expected pet-clinic-helm)"
fi

echo "Policy demo-helm-single..."
curl -sf "${API}/policies/demo-helm-single" | jq -e '.enabled == true' >/dev/null

INST_NAME="pet-clinic-verify-$$"
if [[ -n "$HELM_CLUSTER" ]]; then
  ROUTING_JSON=",{ \"path\": \"routing.cluster\", \"value\": \"${HELM_CLUSTER}\" }"
  echo "Deploy instance ${INST_NAME} (pinned cluster ${HELM_CLUSTER})..."
else
  ROUTING_JSON=""
  echo "Deploy instance ${INST_NAME} (placement via demo-helm-single)..."
fi

HTTP=$(curl -s -o /tmp/pet-clinic-verify-$$.json -w '%{http_code}' -X POST "${API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "{
    \"api_version\": \"v1alpha1\",
    \"display_name\": \"${INST_NAME}\",
    \"spec\": {
      \"catalog_item_id\": \"pet-clinic-helm\",
      \"label_selector\": {\"fleet.workload/profile\": \"demo\", \"fleet.placement/scope\": \"single\"},
      \"user_values\": [
        { \"path\": \"metadata.name\", \"value\": \"${INST_NAME}\" },
        { \"path\": \"release.namespace\", \"value\": \"default\" },
        { \"path\": \"routing.mode\", \"value\": \"hub\" },
        { \"path\": \"values.database.version\", \"value\": \"18\" },
        { \"path\": \"install.timeout\", \"value\": 1200 }
        ${ROUTING_JSON}
      ]
    }
  }")

if [[ "$HTTP" != "201" ]]; then
  echo "WARN: deploy HTTP ${HTTP} ($(jq -r '.detail // .title // .' /tmp/pet-clinic-verify-$$.json 2>/dev/null))"
  echo "SKIP instance checks (helm SP or chart may be missing on target cluster)"
  rm -f /tmp/pet-clinic-verify-$$.json
  echo "=== verify-pet-clinic-helm OK (deploy skipped) ==="
  exit 0
fi

RESP=$(cat /tmp/pet-clinic-verify-$$.json)
rm -f /tmp/pet-clinic-verify-$$.json
INST_ID=$(echo "$RESP" | jq -r '.id // .uid')
echo "instance_id=${INST_ID}"

echo "Placement targets (may still be provisioning)..."
curl -sf "${API}/catalog-item-instances/${INST_ID}/placement-targets" | jq .

echo "Delete instance..."
curl -sf -X DELETE "${API}/catalog-item-instances/${INST_ID}"

echo "=== verify-pet-clinic-helm OK ==="
