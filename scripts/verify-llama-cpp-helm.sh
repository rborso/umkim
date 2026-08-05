#!/usr/bin/env bash
# Verify llama-cpp-helm catalog item and llm-gpu-single placement policy exist.
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"

echo "=== verify-llama-cpp-helm ==="

curl -sf "${API}/catalog-items/llama-cpp-helm" | jq '{id: .metadata.name, display_name}'

POLICY=$(curl -sf "${API}/policies/llm-gpu-single")
echo "$POLICY" | jq '{id: .metadata.name, enabled, strategy: .placement.strategy}'

echo "$POLICY" | jq -e '.cluster_requirements.matchLabels["fleet.capability/gpu"] == "true"' >/dev/null

PREVIEW=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d '{
    "mode": "deploy",
    "service_type": "helm",
    "catalog_item_id": "llama-cpp-helm",
    "label_selector": {
      "fleet.workload/profile": "llm",
      "fleet.placement/scope": "single"
    }
  }')
echo "$PREVIEW" | jq '{deployable, eligible_count}'

echo "=== verify-llama-cpp-helm OK ==="
