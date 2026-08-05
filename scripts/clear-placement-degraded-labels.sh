#!/usr/bin/env bash
# Clear fleet.health/hub_route=degraded from ManagedClusters (unblocks placement preview).
# Use when console shows "No deploy targets" / cluster excluded for hub_route degraded.
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-}"

if [[ -z "$KUBECONFIG" ]]; then
  echo "ERROR: set KUBECONFIG to hub kubeconfig"
  exit 1
fi

echo "=== clear-placement-degraded-labels ==="

CLUSTERS=$(kubectl get managedclusters -o json | jq -r '
  .items[]
  | select(.metadata.labels["fleet.health/hub_route"] == "degraded")
  | .metadata.name
')

if [[ -z "$CLUSTERS" ]]; then
  echo "No ManagedClusters with fleet.health/hub_route=degraded"
else
  for c in $CLUSTERS; do
    echo "Clearing label on $c"
    kubectl label managedcluster "$c" fleet.health/hub_route- --overwrite
  done
fi

echo "=== clear-placement-degraded-labels OK ==="
