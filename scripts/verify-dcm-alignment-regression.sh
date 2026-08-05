#!/usr/bin/env bash
# Full DCM alignment regression (run on hub with lab KVM path available).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

echo "=== verify-dcm-alignment-regression ==="

run() {
  echo ""
  echo ">>> $*"
  "$@"
}

run ./scripts/verify-control-plane-monolith.sh
run ./scripts/verify-cluster-providers-e2e.sh

run ./scripts/verify-helm-catalog.sh

echo ""
echo "=== verify-dcm-alignment-regression OK ==="
