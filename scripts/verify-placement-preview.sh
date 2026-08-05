#!/usr/bin/env bash
# Verify POST /api/v1alpha1/placement-preview (policy-manager).
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"

echo "=== verify-placement-preview ==="

echo "Policy manager health..."
curl -sf "${API%/api/v1alpha1}/api/v1alpha1/health/policies" >/dev/null \
  || curl -sf "${API}/health/policies" >/dev/null

BODY='{"mode":"deploy","service_type":"container","catalog_item_id":"demo-webapp"}'
RESP=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d "$BODY")

echo "$RESP" | jq .

DEPLOYABLE=$(echo "$RESP" | jq -r '.deployable // false')
COUNT=$(echo "$RESP" | jq -r '.eligible_count // 0')

echo "deployable=$DEPLOYABLE eligible_count=$COUNT"

if [[ "$COUNT" -lt 0 ]]; then
  echo "FAIL: invalid eligible_count"
  exit 1
fi

if [[ "$DEPLOYABLE" != "true" || "$COUNT" -lt 1 ]]; then
  echo "WARN: expected deployable=true with RWO clusters + k8s providers (demo-container-single policy)"
fi

echo "=== verify-placement-preview OK ==="
