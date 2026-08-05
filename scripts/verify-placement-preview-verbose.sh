#!/usr/bin/env bash
# Verify POST /api/v1alpha1/placement-preview with verbose=true (cluster_diagnostics + policy_trace).
#
# Optional env:
#   FLEET_SITE=<workload-site-id>  # label only; API body is fleet-wide
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
SITE="${FLEET_SITE:-management}"

echo "=== verify-placement-preview-verbose (site=${SITE}) ==="

BODY=$(jq -nc \
  '{mode:"deploy",verbose:true,service_type:"container",label_selector:{"fleet.workload/profile":"demo","fleet.placement/scope":"single"},catalog_item_id:"demo-webapp"}')
RESP=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d "$BODY")

echo "$RESP" | jq '{deployable, eligible_count, diagnostics: (.cluster_diagnostics | length), trace: (.policy_trace | length)}'

echo "$RESP" | jq -e '(.cluster_diagnostics | type) == "array"' >/dev/null
DIAG_LEN=$(echo "$RESP" | jq -r '.cluster_diagnostics | length')
if [[ "$DIAG_LEN" -lt 1 ]]; then
  echo "FAIL: expected cluster_diagnostics length >= 1 (got $DIAG_LEN)"
  exit 1
fi

TRACE_LEN=$(echo "$RESP" | jq -r '.policy_trace | length')
if [[ "$TRACE_LEN" -lt 1 ]]; then
  echo "FAIL: expected policy_trace length >= 1 (got $TRACE_LEN)"
  exit 1
fi

DEPLOYABLE=$(echo "$RESP" | jq -r '.deployable // false')
if [[ "$DEPLOYABLE" == "true" ]]; then
  echo "$RESP" | jq -e '[.policy_trace[]? | select(.won == true)] | length >= 1' >/dev/null
fi

HAS_REASONS=$(echo "$RESP" | jq -r '[.cluster_diagnostics[]? | select((.reasons // []) | length > 0)] | length')
ELIGIBLE=$(echo "$RESP" | jq -r '.eligible_count // 0')
if [[ "$HAS_REASONS" -lt 1 && "$ELIGIBLE" -lt 1 ]]; then
  echo "WARN: no cluster reasons and eligible_count=0 (cluster lister or policies may be empty)"
fi

# Non-verbose must not include verbose fields
PLAIN_BODY=$(jq -nc \
  '{mode:"deploy",service_type:"container",label_selector:{"fleet.workload/profile":"demo","fleet.placement/scope":"single"},catalog_item_id:"demo-webapp"}')
PLAIN=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d "$PLAIN_BODY")
if echo "$PLAIN" | jq -e '.cluster_diagnostics != null' >/dev/null 2>&1; then
  echo "FAIL: non-verbose response must omit cluster_diagnostics"
  exit 1
fi
if echo "$PLAIN" | jq -e '.policy_trace != null' >/dev/null 2>&1; then
  echo "FAIL: non-verbose response must omit policy_trace"
  exit 1
fi

echo "=== verify-placement-preview-verbose OK ==="
