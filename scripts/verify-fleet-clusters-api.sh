#!/usr/bin/env bash
# Verify cluster PATCH updates ManagedCluster labels and fleet-edge-clusters CM.
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${VERIFY_CLUSTER:-${FLEET_VERIFY_CLUSTER:-lab-sn01}}"
NS="${FLEET_EDGE_NS:-fleet-system}"
CM="${FLEET_EDGE_CLUSTERS_CM:-fleet-edge-clusters}"
INGRESS_ID="${VERIFY_INGRESS_ID:-}"

echo "=== verify-fleet-clusters-api ==="
curl -sf "${API}/clusters" >/dev/null || { echo "Fleet API unreachable"; exit 1; }

# Resolve a site ingress id when not provided
if [[ -z "${INGRESS_ID}" ]]; then
  SITE=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('site',''))")
  [[ -n "${SITE}" ]] || { echo "cluster ${CLUSTER} has no site"; exit 1; }
  INGRESS_ID=$(curl -sf "${API}/sites/${SITE}" | python3 -c "
import json,sys
edge=(json.load(sys.stdin).get('spec') or {}).get('edge') or {}
rows=edge.get('ingress') or []
for r in rows:
  if r.get('method')=='port_forward' and r.get('id'):
    print(r['id']); break
else:
  print(rows[0]['id'] if rows else '')
")
fi
[[ -n "${INGRESS_ID}" ]] || { echo "no ingress id available for ${CLUSTER}"; exit 1; }

curl -sf -X PATCH "${API}/clusters/${CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d "{\"edge\":{\"ingress_id\":\"${INGRESS_ID}\",\"upstream_host\":\"10.10.0.11\",\"traefik_http_port\":30080}}" \
  | head -c 300
echo ""

curl -sf "${API}/clusters/${CLUSTER}" | jq -e --arg id "${INGRESS_ID}" '.ingress_id == $id' >/dev/null
kubectl -n "${NS}" get configmap "${CM}" -o yaml | grep -q "ingress_id: ${INGRESS_ID}"
echo "=== verify-fleet-clusters-api OK ==="
