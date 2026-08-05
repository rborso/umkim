#!/usr/bin/env bash
# Verify wrong DNS sets FailedDNS and does not run gateway-acme issuance.
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
NS="${FLEET_ROUTE_NAMESPACE:-fleet-system}"
BAD_HOST="${BAD_DNS_HOST:-bad-dns-check.mgmt.umkim.com}"
ROUTE_NAME="${BAD_DNS_ROUTE_NAME:-}"

fail() { echo "[verify-acme-dns-guard] FAIL $*" >&2; exit 1; }
ok()   { echo "[verify-acme-dns-guard] OK   $*"; }

echo "=== verify-acme-dns-guard host=${BAD_HOST} ==="

if [[ -z "$ROUTE_NAME" ]]; then
  ROUTE_NAME="$(kubectl -n "$NS" get fleetapproutes -o json \
    | jq -r --arg h "$BAD_HOST" '.items[] | select(.spec.hubHost==$h) | .metadata.name' | head -1)"
fi
[[ -n "$ROUTE_NAME" ]] || fail "no FleetAppRoute for $BAD_HOST (deploy a hub route with wrong DNS first)"

phase="$(kubectl -n "$NS" get fleetapproute "$ROUTE_NAME" -o jsonpath='{.status.tlsPhase}')"
[[ "$phase" == "FailedDNS" ]] || fail "expected tlsPhase=FailedDNS got ${phase:-empty}"

solver_ready=$(kubectl -n dcm get deploy gateway-acme -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
[[ "${solver_ready:-0}" == "0" ]] || fail "gateway-acme should not be running for FailedDNS host (ready=$solver_ready)"

gw_ext="$(kubectl -n dcm get svc gateway -o jsonpath='{.spec.externalIPs}' 2>/dev/null || true)"
[[ -n "$gw_ext" && "$gw_ext" != "[]" ]] || fail "gateway Service lost externalIPs"

ok "FailedDNS without solver disruption"
echo "=== verify-acme-dns-guard OK ==="
