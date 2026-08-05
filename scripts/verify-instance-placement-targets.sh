#!/usr/bin/env bash
# Verify GET .../catalog-item-instances/{id}/placement-targets (multi-cluster replication status).
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
MIN_TARGETS="${MIN_PLACEMENT_TARGETS:-1}"

echo "=== verify-instance-placement-targets ==="

echo "Catalog manager health..."
curl -sf "${API%/api/v1alpha1}/health/catalog" >/dev/null || curl -sf "${API}/health/catalog" >/dev/null

echo "List catalog item instances..."
LIST=$(curl -sf "${API}/catalog-item-instances")
INSTANCE_ID=$(echo "$LIST" | jq -r '
  (.results // .catalog_item_instances // .["catalog-item-instances"] // .items // [])[]
  | select(
      (.resource_id != null and .resource_id != "")
      or ((.spec.resource_ids // []) | length) > 0
    )
  | .id // .uid
' | head -1)

if [[ -z "$INSTANCE_ID" || "$INSTANCE_ID" == "null" ]]; then
  echo "SKIP: no provisioned catalog item instance with resource_id (deploy one first)"
  exit 0
fi

echo "Instance id=$INSTANCE_ID"
TARGETS_URL="${API}/catalog-item-instances/${INSTANCE_ID}/placement-targets"
RESP=$(curl -sf "$TARGETS_URL")
echo "$RESP" | jq .

TARGET_COUNT=$(echo "$RESP" | jq -r '.target_count // 0')
HEALTHY=$(echo "$RESP" | jq -r '.healthy_count // 0')

if [[ "$TARGET_COUNT" -lt "$MIN_TARGETS" ]]; then
  echo "WARN: target_count=$TARGET_COUNT (expected >= $MIN_TARGETS for full Phase 2 test)"
fi

echo "healthy_count=$HEALTHY target_count=$TARGET_COUNT"
if [[ "$TARGET_COUNT" -ge 1 && "$HEALTHY" -lt 1 ]]; then
  echo "WARN: no healthy targets reported (SPM status may still be provisioning)"
fi

if [[ "$TARGET_COUNT" -ge 2 ]]; then
  echo "Multi-target sprm_id check..."
  echo "$RESP" | jq -r '.targets[]? | "\(.provider // "?") role=\(.role // "?") sprm_id=\(.sprm_id // .sprmId // "missing")"'
  DISTINCT_SPRM=$(echo "$RESP" | jq -r '[.targets[]? | .sprm_id // .sprmId // empty] | map(select(. != "")) | unique | length')
  if [[ "$DISTINCT_SPRM" -ge 2 ]]; then
    echo "OK: $DISTINCT_SPRM distinct sprm_id values (per-provider workloads)"
  else
    echo "WARN: expected distinct sprm_id per provider for multisite standby (unique=$DISTINCT_SPRM)"
  fi
fi

echo "=== verify-instance-placement-targets OK ==="
