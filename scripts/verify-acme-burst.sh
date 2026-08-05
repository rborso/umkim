#!/usr/bin/env bash
# Verify multiple hub-routed hostnames reach tlsPhase=Ready (requires pre-deployed instances).
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
NS="${FLEET_ROUTE_NAMESPACE:-fleet-system}"
N="${N:-5}"
TIMEOUT_SEC="${TIMEOUT_SEC:-1800}"
HOST_PREFIX="${BURST_HOST_PREFIX:-burst-acme}"
DOMAIN="${FLEET_EDGE_DOMAIN:-mgmt.umkim.com}"

fail() { echo "[verify-acme-burst] FAIL $*" >&2; exit 1; }
ok()   { echo "[verify-acme-burst] OK   $*"; }

echo "=== verify-acme-burst N=${N} timeout=${TIMEOUT_SEC}s ==="

hosts=()
for i in $(seq 1 "$N"); do
  hosts+=("${HOST_PREFIX}-${i}.${DOMAIN}")
done

deadline=$((SECONDS + TIMEOUT_SEC))
ready=0
while (( SECONDS < deadline )); do
  ready=0
  for h in "${hosts[@]}"; do
    phase="$(kubectl -n "$NS" get fleetapproutes -o json 2>/dev/null \
      | jq -r --arg h "$h" '.items[] | select(.spec.hubHost==$h) | .status.tlsPhase' | head -1)"
    if [[ "$phase" == "Ready" ]]; then
      ready=$((ready + 1))
    fi
  done
  echo "ready=${ready}/${N}"
  if [[ "$ready" -ge "$N" ]]; then
    break
  fi
  sleep 15
done

[[ "$ready" -ge "$N" ]] || fail "only ${ready}/${N} routes reached tlsPhase=Ready"

solver_pods=$(kubectl -n dcm get pods -l app=dcm-gateway-acme --field-selector=status.phase=Running -o name 2>/dev/null | wc -l)
[[ "$solver_pods" -le 1 ]] || fail "more than one gateway-acme pod running: $solver_pods"

ok "all ${N} hostnames Ready"
echo "=== verify-acme-burst OK ==="
