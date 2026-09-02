#!/usr/bin/env bash
# Hub-only readiness checks (v0.2 spine, no DCM).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FAIL=0

fail() { echo "FAIL: $*" >&2; FAIL=1; }
ok() { echo "OK: $*"; }

echo "=== verify-hub-install (v0.2 spine) ==="

kctl() {
  if kubectl "$@" 2>/dev/null; then return 0; fi
  sudo kubectl "$@" 2>/dev/null
}

if ! command -v kubectl >/dev/null 2>&1; then
  fail "kubectl not found"
else
  kctl get nodes >/dev/null 2>&1 && ok "k3s API" || fail "k3s API unreachable"

  kctl get crd managedclusters.cluster.open-cluster-management.io >/dev/null 2>&1 && ok "OCM ManagedCluster CRD" || fail "OCM CRD missing"
  kctl get crd fleetclusterjoins.fleet.umkim.com >/dev/null 2>&1 && ok "FleetClusterJoin CRD" || fail "FleetClusterJoin CRD missing"
  kctl get crd fleetapps.fleet.umkim.com >/dev/null 2>&1 && ok "FleetApp CRD" || fail "FleetApp CRD missing"
  kctl get crd fleetcharts.fleet.umkim.com >/dev/null 2>&1 && ok "FleetChart CRD" || fail "FleetChart CRD missing"
  kctl get crd fleetapproutes.edge.umkim.com >/dev/null 2>&1 && ok "FleetAppRoute CRD" || fail "FleetAppRoute CRD missing"

  kctl -n harbor get deploy harbor-core >/dev/null 2>&1 && ok "Harbor deployment" || fail "Harbor missing"
  kctl -n fleet-console get deploy fleet-site-controller >/dev/null 2>&1 && ok "fleet-site-controller" || fail "fleet-site-controller missing"
  kctl -n fleet-system get deploy fleet-edge-controller >/dev/null 2>&1 && ok "fleet-edge-controller" || fail "fleet-edge-controller missing"
  kctl -n fleet-console get deploy fleet-console-auth >/dev/null 2>&1 && ok "fleet-console-auth" || fail "fleet-console-auth missing"
  kctl -n fleet-console get deploy dcm-fleet-dashboard >/dev/null 2>&1 && ok "fleet console dashboard" || fail "console dashboard missing"

  GW_NS="${HUB_INGRESS_NS:-fleet-ingress}"
  HN="$(kctl -n "${GW_NS}" get deploy gateway -o jsonpath='{.spec.template.spec.hostNetwork}' 2>/dev/null || true)"
  if [[ "${HN}" == "true" ]]; then
    ok "hub gateway hostNetwork=true (${GW_NS}/gateway)"
  else
    fail "hub gateway missing hostNetwork (${GW_NS}/gateway)"
  fi

  PROBE="$(kctl -n fleet-system get deploy fleet-edge-controller -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="FLEET_GATEWAY_PROBE_HOST")].value}' 2>/dev/null || true)"
  WAN_ENV="$(kctl -n fleet-system get deploy fleet-edge-controller -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="FLEET_GATEWAY_WAN_IP")].value}' 2>/dev/null || true)"
  if [[ -n "$PROBE" && "$PROBE" != "127.0.0.1" && "$PROBE" != "localhost" ]]; then
    ok "fleet-edge FLEET_GATEWAY_PROBE_HOST=${PROBE}"
  elif [[ -n "$WAN_ENV" && "$WAN_ENV" != "127.0.0.1" && "$WAN_ENV" != "localhost" ]]; then
    ok "fleet-edge FLEET_GATEWAY_WAN_IP=${WAN_ENV}"
  else
    fail "fleet-edge ACME probe host invalid (set fleet_hub_public_ipv4)"
  fi
fi

if command -v curl >/dev/null 2>&1; then
  if curl -sf --max-time 5 http://127.0.0.1/ping >/dev/null 2>&1; then
    ok "Traefik /ping on :80"
  else
    fail "Traefik /ping not reachable on :80"
  fi
fi

if command -v nft >/dev/null 2>&1; then
  if sudo nft list table inet fleet_hub >/dev/null 2>&1; then
    ok "nft table fleet_hub"
  else
    fail "nft table fleet_hub missing"
  fi
fi

if ip link show wg0 >/dev/null 2>&1; then
  ok "WireGuard wg0"
else
  fail "WireGuard wg0 missing"
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "=== verify-hub-install FAILED ===" >&2
  exit 1
fi
echo "=== verify-hub-install OK ==="
