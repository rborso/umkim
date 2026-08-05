#!/usr/bin/env bash
# Verify resource-aware DCM placement: MC labels, placement-preview, and policy behavior.
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
HUB_KUBECONFIG="${HUB_KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
CLUSTERS="${FLEET_VERIFY_CLUSTERS:-lab-sn01,lab-ha01}"

kubectl_cmd() {
  if [[ -r "$HUB_KUBECONFIG" ]]; then
    kubectl --kubeconfig "$HUB_KUBECONFIG" "$@"
  elif [[ -f "$HUB_KUBECONFIG" ]]; then
    sudo env -u KUBECONFIG kubectl --kubeconfig "$HUB_KUBECONFIG" "$@"
  else
    sudo kubectl "$@"
  fi
}

echo "=== verify-resource-aware-placement ==="

echo "ManagedCluster resource labels..."
for c in $(echo "$CLUSTERS" | tr ',' ' '); do
  cpu=$(kubectl_cmd get managedcluster "$c" \
    -o jsonpath='{.metadata.labels.fleet\.resource/cpu\.utilization}' 2>/dev/null || true)
  mem=$(kubectl_cmd get managedcluster "$c" \
    -o jsonpath='{.metadata.labels.fleet\.resource/memory\.utilization}' 2>/dev/null || true)
  ok=$(kubectl_cmd get managedcluster "$c" \
    -o jsonpath='{.metadata.labels.fleet\.resource/metrics_ok}' 2>/dev/null || true)
  at=$(kubectl_cmd get managedcluster "$c" \
    -o jsonpath='{.metadata.labels.fleet\.resource/metrics_at}' 2>/dev/null || true)
  cap=$(kubectl_cmd get managedcluster "$c" \
    -o jsonpath='{.metadata.labels.fleet\.resource/capacity_ok}' 2>/dev/null || true)
  maxmem=$(kubectl_cmd get managedcluster "$c" \
    -o jsonpath='{.metadata.labels.fleet\.resource/memory\.max_node_allocatable_bytes}' 2>/dev/null || true)
  echo "  $c: cpu=${cpu:-—}% mem=${mem:-—}% metrics_ok=${ok:-—} capacity_ok=${cap:-—} max_node_mem=${maxmem:-—} metrics_at=${at:-—}"
  if [[ -z "$at" ]]; then
    echo "FAIL: $c missing fleet.resource/metrics_at (fleet-site resource-metrics sync not running?)"
    exit 1
  fi
  if [[ "$ok" != "true" ]]; then
    echo "WARN: $c metrics_ok!=true (metrics-server or spoke connectivity required for utilization placement)"
  fi
done

echo "Policies demo-*-least-util..."
curl -sf "${API}/policies/demo-helm-least-util" | jq -e \
  '.enabled == true and .placement.strategy == "least_utilized" and .label_selector["fleet.placement/picker"] == "least-utilized"' >/dev/null
curl -sf "${API}/policies/demo-container-least-util" | jq -e \
  '.enabled == true and .placement.strategy == "least_utilized"' >/dev/null

echo "Placement preview (least_utilized)..."
PREVIEW_BODY='{
  "mode":"deploy",
  "verbose":true,
  "service_type":"helm",
  "catalog_item_id":"whoami-helm",
  "label_selector":{
    "fleet.workload/profile":"demo",
    "fleet.placement/scope":"single",
    "fleet.placement/picker":"least-utilized"
  }
}'
PREVIEW=$(curl -sf -X POST "${API}/placement-preview" -H 'Content-Type: application/json' -d "$PREVIEW_BODY")
echo "$PREVIEW" | jq '{deployable, winning_policy, eligible_clusters: [.eligible_clusters[]? | {name, cpu_utilization, memory_utilization, metrics_ok}]}'

echo "$PREVIEW" | jq -e '.winning_policy.id == "demo-helm-least-util"' >/dev/null
if [[ "$(echo "$PREVIEW" | jq -r '.deployable // false')" != "true" ]]; then
  echo "WARN: placement preview deployable=false (spoke providers or connectivity may be degraded)"
else
  echo "$PREVIEW" | jq -e '.eligible_count >= 1' >/dev/null
fi

echo "Regression: verify-placement-preview..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/verify-placement-preview.sh"

echo "=== verify-resource-aware-placement OK ==="
