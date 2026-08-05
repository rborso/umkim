#!/usr/bin/env bash
# Verify cluster PATCH updates ManagedCluster labels and fleet-edge-clusters CM.
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${VERIFY_CLUSTER:-${FLEET_VERIFY_CLUSTER:-lab-sn01}}"
NS="${FLEET_EDGE_NS:-fleet-system}"
CM="${FLEET_EDGE_CLUSTERS_CM:-fleet-edge-clusters}"

echo "=== verify-fleet-clusters-api ==="
curl -sf "${API}/clusters" >/dev/null || { echo "Fleet API unreachable"; exit 1; }

curl -sf -X PATCH "${API}/clusters/${CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d '{"edge":{"exposure_mode":"port_forward","direct_ingress":true}}' \
  | head -c 300
echo ""

curl -sf "${API}/clusters/${CLUSTER}" | jq -e '.ingress_enabled == true' >/dev/null
kubectl -n "${NS}" get configmap "${CM}" -o yaml | grep -q "direct_ingress: true"
echo "=== verify-fleet-clusters-api OK ==="
