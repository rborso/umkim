#!/usr/bin/env bash
# Verify fleet-failover degraded CM and placement-manager relocate endpoint (dry checks).
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
FLEET_API="${FLEET_SITE_API:-http://127.0.0.1:30908/fleet/api/v1alpha1}"
KUBECONFIG="${KUBECONFIG:-}"
CM_NS="${FLEET_FAILOVER_CM_NS:-fleet-system}"

echo "=== verify-failover-relocate ==="

if [[ -n "$KUBECONFIG" ]]; then
  echo "Degraded clusters ConfigMap..."
  kubectl -n "$CM_NS" get configmap fleet-failover-degraded-clusters -o jsonpath='{.data.degraded\.json}' 2>/dev/null | jq . || echo "WARN: CM not found (fleet-failover-controller may not be deployed)"
else
  echo "SKIP: KUBECONFIG unset — skipping CM check"
fi

echo "List instances for hub-route status..."
LIST=$(curl -sf "${API}/catalog-item-instances")
INSTANCE_ID=$(echo "$LIST" | jq -r '
  (.results // [])[]
  | select(.resource_id != null and .resource_id != "")
  | .id // .uid
' | head -1)

if [[ -n "$INSTANCE_ID" && "$INSTANCE_ID" != "null" ]]; then
  echo "instance=$INSTANCE_ID"
  curl -sf "${FLEET_API}/instance-routes/${INSTANCE_ID}" | jq . || echo "WARN: no FleetAppRoute for instance"
else
  echo "SKIP: no provisioned instance"
fi

echo "Placement-manager health..."
curl -sf "${API%/api/v1alpha1}/health/placement" >/dev/null || curl -sf "${API}/health/placement" >/dev/null

RESOURCE_ID=$(echo "$LIST" | jq -r '(.results // [])[] | select(.resource_id != null) | .resource_id' | head -1)
if [[ -n "$RESOURCE_ID" && "$RESOURCE_ID" != "null" ]]; then
  echo "Relocate endpoint smoke (expect 4xx/5xx without degraded trigger): resource_id=$RESOURCE_ID"
  CODE=$(curl -s -o /tmp/relocate-body.json -w '%{http_code}' \
    -X POST "${API}/placements/${RESOURCE_ID}/relocate" \
    -H 'Content-Type: application/json' \
    -d '{"exclude_clusters":["virt01"],"reason":"verify_script"}' || true)
  echo "HTTP $CODE"
  head -c 400 /tmp/relocate-body.json 2>/dev/null || true
  echo
fi

echo "=== verify-failover-relocate OK ==="
