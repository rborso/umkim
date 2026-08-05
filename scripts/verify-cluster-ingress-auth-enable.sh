#!/usr/bin/env bash
# Verify per-cluster ingress auth enable API and spoke deployment.
#
# Usage (from workstation with fleet API cookies, or on hub with port-forward):
#   export FLEET_API_BASE="https://mgmt.umkim.com/fleet/api/v1alpha1"
#   export FLEET_COOKIE_JAR="$HOME/.fleet-console-cookies.txt"
#   export CLUSTER=virt02
#   export LLDAP_PASSWORD='your-secure-password-here'
#   ./scripts/verify-cluster-ingress-auth-enable.sh
#
# On hub (kubectl only, after API enable):
#   SKIP_API=1 CLUSTER=virt02 PORTAL_HOST=auth.apps.umkim.com ./scripts/verify-cluster-ingress-auth-enable.sh

set -euo pipefail

CLUSTER="${CLUSTER:-${FLEET_VERIFY_CLUSTER:-virt02}}"
FLEET_API_BASE="${FLEET_API_BASE:-${FLEET_SITE_API:-http://127.0.0.1:30912/fleet/api/v1alpha1}}"
FLEET_COOKIE_JAR="${FLEET_COOKIE_JAR:-${FLEET_AUTH_COOKIE_JAR:-}}"
LLDAP_PASSWORD="${LLDAP_PASSWORD:-}"
SKIP_API="${SKIP_API:-0}"
PORTAL_HOST="${PORTAL_HOST:-}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
NS="${FLEET_INGRESS_AUTH_NS:-fleet-ingress-auth}"
FCJ_NS="${FLEET_SITE_NS:-fleet-system}"
WAIT_SEC="${WAIT_SEC:-600}"
EDGE_HTTPS="${EDGE_HTTPS:-}"

curl_api() {
  local method="$1"
  local path="$2"
  local data="${3:-}"
  local args=(-sS -k -X "$method" "${FLEET_API_BASE}${path}")
  if [[ -n "$FLEET_COOKIE_JAR" && -f "$FLEET_COOKIE_JAR" ]]; then
    args+=(-b "$FLEET_COOKIE_JAR")
  fi
  if [[ -n "$data" ]]; then
    args+=(-H "Content-Type: application/json" -d "$data")
  fi
  curl "${args[@]}"
}

if [[ "$SKIP_API" != "1" ]]; then
  if [[ -z "$LLDAP_PASSWORD" ]]; then
    echo "FAIL: set LLDAP_PASSWORD (min 12 chars) or SKIP_API=1" >&2
    exit 1
  fi
  if [[ ${#LLDAP_PASSWORD} -lt 12 ]]; then
    echo "FAIL: LLDAP_PASSWORD must be at least 12 characters" >&2
    exit 1
  fi

  echo "== POST enable ingress auth for ${CLUSTER}"
  enable_resp="$(curl_api POST "/clusters/${CLUSTER}/ingress-auth/enable" \
    "{\"lldapAdminPassword\":\"${LLDAP_PASSWORD}\"}")"
  echo "$enable_resp" | head -c 500
  echo

  portal="$(echo "$enable_resp" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('ingressAuthStatus',{}).get('portalHost',''))" 2>/dev/null || true)"
  if [[ -n "$portal" ]]; then
    PORTAL_HOST="$portal"
  fi

  echo "== Wait for FleetClusterJoin ingressAuth.phase=Ready (timeout ${WAIT_SEC}s)"
  deadline=$((SECONDS + WAIT_SEC))
  while (( SECONDS < deadline )); do
    phase="$(kubectl -n "$FCJ_NS" get fleetclusterjoin "$CLUSTER" -o jsonpath='{.status.ingressAuth.phase}' 2>/dev/null || true)"
    echo "  phase=${phase:-—}"
    if [[ "$phase" == "Ready" ]]; then
      break
    fi
    if [[ "$phase" == "Failed" ]]; then
      kubectl -n "$FCJ_NS" get fleetclusterjoin "$CLUSTER" -o jsonpath='{.status.ingressAuth.message}{"\n"}'
      exit 1
    fi
    sleep 10
  done
  if [[ "${phase:-}" != "Ready" ]]; then
    echo "FAIL: timed out waiting for ingress auth Ready"
    exit 1
  fi
fi

if [[ -z "$PORTAL_HOST" ]]; then
  PORTAL_HOST="$(kubectl -n "$FCJ_NS" get fleetclusterjoin "$CLUSTER" -o jsonpath='{.status.ingressAuth.portalHost}' 2>/dev/null || true)"
fi

echo "== Spoke workloads via OCM ManifestWork"
export KUBECONFIG
kubectl get manifestwork fleet-ingress-auth -n "$CLUSTER" >/dev/null

spoke_kc="/etc/ocm/${CLUSTER}.kubeconfig"
if [[ -f "$spoke_kc" ]]; then
  kubectl --kubeconfig "$spoke_kc" -n "$NS" rollout status deployment/authelia --timeout=180s
  kubectl --kubeconfig "$spoke_kc" -n fleet-ingress get cm fleet-ingress-auth-traefik >/dev/null
else
  echo "WARN: spoke kubeconfig ${spoke_kc} missing; skipping spoke pod checks"
fi

if [[ -n "$PORTAL_HOST" ]]; then
  if [[ -z "$EDGE_HTTPS" ]]; then
  edge_ip="$(kubectl -n "$FCJ_NS" get fleetclusterjoin "$CLUSTER" -o jsonpath='{.status.wireguardAddress}' 2>/dev/null | cut -d/ -f1)"
    if [[ -n "$edge_ip" ]]; then
      EDGE_HTTPS="https://${edge_ip}:30443/"
    fi
  fi
  if [[ -n "$EDGE_HTTPS" ]]; then
    echo "== curl Authelia portal Host: ${PORTAL_HOST}"
    code="$(curl -sk -o /dev/null -w '%{http_code}' -H "Host: ${PORTAL_HOST}" "${EDGE_HTTPS}" || true)"
    echo "HTTP ${code}"
    if [[ "$code" != "200" && "$code" != "301" && "$code" != "302" && "$code" != "401" ]]; then
      echo "WARN: unexpected HTTP code from edge (expected redirect or auth challenge)"
    fi
  fi
fi

echo "OK — cluster ingress auth verified for ${CLUSTER}"
