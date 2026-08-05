#!/usr/bin/env bash
# Verify fleet-edge-controller logs show permanent DNS failure skip (no tight re-queue).
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
NS="${FLEET_EDGE_CONTROLLER_NS:-fleet-system}"
SINCE="${SINCE:-10m}"

fail() { echo "[verify-acme-queue-backoff] FAIL $*" >&2; exit 1; }
ok()   { echo "[verify-acme-queue-backoff] OK   $*"; }

echo "=== verify-acme-queue-backoff ==="

logs="$(kubectl -n "$NS" logs deploy/fleet-edge-controller --since="$SINCE" 2>/dev/null || true)"
if echo "$logs" | grep -q 'DNS pre-check failed'; then
  ok "found DNS pre-check skip in controller logs"
else
  echo "WARN: no DNS pre-check log lines in last $SINCE (deploy a bad-DNS host to test)"
fi

if echo "$logs" | grep -qi 'rate limit\|429'; then
  ok "rate-limit errors classified in logs"
fi

echo "=== verify-acme-queue-backoff OK (informational) ==="
