#!/usr/bin/env bash
# Verify main gateway HA stays up while ACME issuance runs (solver path).
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
NS="${HUB_INGRESS_NS:-fleet-ingress}"
BASELINE_HOST="${ACME_VERIFY_BASELINE_HOST:-}"
DOMAIN="${FLEET_EDGE_DOMAIN:-mgmt.umkim.com}"
POLL_SEC="${POLL_SEC:-120}"

fail() { echo "[verify-acme-ha-stability] FAIL $*" >&2; exit 1; }
ok()   { echo "[verify-acme-ha-stability] OK   $*"; }

echo "=== verify-acme-ha-stability ==="

REPLICAS=$(kubectl -n "$NS" get deploy gateway -o jsonpath='{.spec.replicas}')
READY=$(kubectl -n "$NS" get deploy gateway -o jsonpath='{.status.readyReplicas}')
HOST_NET=$(kubectl -n "$NS" get deploy gateway -o jsonpath='{.spec.template.spec.hostNetwork}')
EXPECTED_REPLICAS="${GATEWAY_REPLICAS:-$REPLICAS}"

if [[ "$HOST_NET" == "true" ]]; then
  echo "SKIP: hub gateway uses hostNetwork (single-node); HA solver path does not apply"
  exit 0
fi

[[ "$REPLICAS" == "$EXPECTED_REPLICAS" ]] || fail "gateway replicas=$REPLICAS want $EXPECTED_REPLICAS"
[[ "$READY" == "$EXPECTED_REPLICAS" ]] || fail "gateway ready=$READY want $EXPECTED_REPLICAS"

SOLVER_REPLICAS=$(kubectl -n "$NS" get deploy gateway-acme -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "0")
[[ "$SOLVER_REPLICAS" == "0" ]] || ok "gateway-acme replicas=$SOLVER_REPLICAS (issuance may be active)"

if [[ -n "$BASELINE_HOST" ]]; then
  issuer="$(echo | openssl s_client -connect "${BASELINE_HOST}:443" -servername "$BASELINE_HOST" 2>/dev/null \
    | openssl x509 -noout -issuer 2>/dev/null || true)"
  echo "baseline issuer: ${issuer:-unknown}"
  if echo "$issuer" | grep -qiE 'TRAEFIK DEFAULT CERT'; then
    fail "baseline host serves default cert before test"
  fi
fi

# Optional: watch during burst (caller deploys instances separately)
end=$((SECONDS + POLL_SEC))
while (( SECONDS < end )); do
  READY_NOW=$(kubectl -n "$NS" get deploy gateway -o jsonpath='{.status.readyReplicas}')
  [[ "$READY_NOW" == "$EXPECTED_REPLICAS" ]] || fail "gateway ready dropped to $READY_NOW during poll"
  if [[ -n "$BASELINE_HOST" ]]; then
    issuer="$(echo | openssl s_client -connect "${BASELINE_HOST}:443" -servername "$BASELINE_HOST" 2>/dev/null \
      | openssl x509 -noout -issuer 2>/dev/null || true)"
    if echo "$issuer" | grep -qiE 'TRAEFIK DEFAULT CERT'; then
      fail "baseline host lost LE cert during poll"
    fi
  fi
  sleep 5
done

ok "gateway HA stable for ${POLL_SEC}s"
echo "=== verify-acme-ha-stability OK ==="
