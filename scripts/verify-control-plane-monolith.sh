#!/usr/bin/env bash
# Run DCM control-plane monolith verification suite (hub).
# Hub-only: use verify-hub-install.sh instead.
# Set FLEET_VERIFY_CLUSTERS=1 to include deploy/alignment scripts that need joined clusters.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "=== verify-control-plane-monolith ==="

scripts=(
  verify-cluster-labels.sh
  validate-policy-manager-phase2-fork.sh
  verify-placement-phase2.sh
  verify-placement-preview.sh
  verify-placement-preview-verbose.sh
)

if [[ "${FLEET_VERIFY_CLUSTERS:-}" == "1" ]]; then
  scripts+=(
    verify-placement-policy-alignment.sh
    verify-instance-placement-targets.sh
    verify-helm-catalog.sh
  )
else
  echo "Skipping cluster deploy scripts (set FLEET_VERIFY_CLUSTERS=1 after FCJ + k8s provider)"
fi

for s in "${scripts[@]}"; do
  echo ""
  echo ">>> ./scripts/${s}"
  "./scripts/${s}"
done

echo ""
echo "=== verify-control-plane-monolith OK ==="
