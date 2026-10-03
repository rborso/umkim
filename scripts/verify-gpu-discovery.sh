#!/usr/bin/env bash
# Verify ManagedCluster GPU resource labels after capability discovery.
set -euo pipefail

CLUSTER="${FLEET_GPU_CLUSTER:?set FLEET_GPU_CLUSTER to a cluster name}"

echo "=== verify-gpu-discovery (cluster=${CLUSTER}) ==="

LABELS=$(kubectl get managedcluster "$CLUSTER" -o json | jq -r '.metadata.labels')
echo "$LABELS" | jq '{
  gpu: ."fleet.capability/gpu",
  total: ."fleet.resource/gpu.total",
  available: ."fleet.resource/gpu.available"
}'

GPU=$(echo "$LABELS" | jq -r '."fleet.capability/gpu" // "missing"')
if [[ "$GPU" != "true" && "${FLEET_GPU_EXPECT:-false}" == "true" ]]; then
  echo "FAIL: fleet.capability/gpu is not true"
  exit 1
fi

echo "=== verify-gpu-discovery OK ==="
