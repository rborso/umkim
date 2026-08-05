#!/usr/bin/env bash
# E2E route-policy smoke for fleet ingress auth.
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
NS="${FLEET_ROUTE_NAMESPACE:-fleet-system}"
HUB_GATEWAY="${HUB_GATEWAY:-http://127.0.0.1:30908}"
HUB_GATEWAY_HTTPS="${HUB_GATEWAY_HTTPS:-https://127.0.0.1:30909}"
VIRT01_TRAEFIK="${VIRT01_TRAEFIK:-http://10.10.0.11:30080}"
VIRT01_TRAEFIK_HTTPS="${VIRT01_TRAEFIK_HTTPS:-https://10.10.0.11:30443}"
DEMO_NS="${FLEET_AUTH_TEST_NS:-fleet-ingress}"

echo "== Hub auth stack"
kubectl -n fleet-ingress-auth rollout status deployment/authelia --timeout=60s
kubectl -n fleet-ingress-auth rollout status deployment/lldap --timeout=60s

echo "== Fleet-edge authelia middleware present"
kubectl -n dcm get cm dcm-traefik-fleet-routes -o yaml | grep -q fleet-edge-authelia || \
  kubectl -n dcm get cm dcm-traefik-dynamic -o yaml | grep -q fleet-edge-authelia

cleanup() { kubectl -n "$NS" delete fleetapproute --all --ignore-not-found; }
trap cleanup EXIT

apply_route() {
  local name="$1" mode="$2" hub_host="$3" site_host="${4:-}"
  if [[ -n "$site_host" ]]; then
    kubectl -n "$NS" apply -f - <<EOF
apiVersion: edge.umkim.com/v1alpha1
kind: FleetAppRoute
metadata:
  name: ${name}
spec:
  instanceId: ${name}
  cluster: virt01
  mode: ${mode}
  hubHost: ${hub_host}
  siteHost: ${site_host}
  service:
    name: whoami
    namespace: ${DEMO_NS}
    port: 80
EOF
  else
    kubectl -n "$NS" apply -f - <<EOF
apiVersion: edge.umkim.com/v1alpha1
kind: FleetAppRoute
metadata:
  name: ${name}
spec:
  instanceId: ${name}
  cluster: virt01
  mode: ${mode}
  hubHost: ${hub_host}
  service:
    name: whoami
    namespace: ${DEMO_NS}
    port: 80
EOF
  fi
  sleep 10
}

echo "== hub-only route"
apply_route auth-test-hub hub whoami-hub-auth-test.mgmt.umkim.com
code=$(curl -sk -o /dev/null -w '%{http_code}' -H 'Host: whoami-hub-auth-test.mgmt.umkim.com' "${HUB_GATEWAY_HTTPS}/" || true)
echo "hub-only HTTP ${code}"
[[ "$code" == "401" || "$code" == "302" || "$code" == "303" ]] || { echo "FAIL hub-only got ${code}"; exit 1; }

echo "== dual route"
apply_route auth-test-dual dual whoami-dual-auth-test.mgmt.umkim.com whoami-dual-auth-test.virt01.mgmt.umkim.com
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: whoami-dual-auth-test.mgmt.umkim.com' "${HUB_GATEWAY}/" || true)
echo "dual HTTP ${code}"
[[ "$code" == "200" ]] || echo "WARN dual got ${code}"

echo "== site-only route"
apply_route auth-test-site site whoami-site-auth-test.mgmt.umkim.com whoami-site-auth-test.virt01.mgmt.umkim.com
code=$(curl -sk -o /dev/null -w '%{http_code}' -H 'Host: whoami-site-auth-test.virt01.mgmt.umkim.com' "${VIRT01_TRAEFIK_HTTPS}/" || true)
echo "site-only HTTP ${code}"
[[ "$code" == "401" || "$code" == "302" || "$code" == "303" ]] || { echo "FAIL site-only got ${code}"; exit 1; }

echo "OK — route auth policy smoke passed"
