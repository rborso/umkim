#!/usr/bin/env bash
# Verify catalog site routes with acme_cluster (port-forward profile).
# Run on hub with kubectl access to fleet-edge and spoke clusters.
#
# Usage:
#   ./scripts/verify-fleet-edge-site-acme.sh
#   SITE_HOST=whoami.homelab.otthol.hu CLUSTER=lab-single-0 NODEPORT_HTTP=30080 ./scripts/verify-fleet-edge-site-acme.sh
#   PREFLIGHT_ONLY=1 ./scripts/verify-fleet-edge-site-acme.sh   # DNS + port checks only (no LE order)

set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
SITE_HOST="${SITE_HOST:-demo.apps.umkim.com}"
CLUSTER="${CLUSTER:-virt02}"
NODEPORT_HTTP="${NODEPORT_HTTP:-30080}"
NODE_IP="${NODE_IP:-}"
PREFLIGHT_ONLY="${PREFLIGHT_ONLY:-0}"

if [[ -z "$NODE_IP" ]]; then
  NODE_IP="$(kubectl get nodes -o wide 2>/dev/null | awk -v c="$CLUSTER" '$1 ~ c {print $6; exit}')"
fi
if [[ -z "$NODE_IP" ]]; then
  echo "WARN: could not resolve NODE_IP for cluster $CLUSTER; set NODE_IP manually"
fi

echo "== FleetAppRoute for site host $SITE_HOST"
kubectl -n fleet-system get fleetapproutes -o wide 2>/dev/null | grep -F "$SITE_HOST" || true

echo "== FleetAppRoute TLS phase for $SITE_HOST"
route_json="$(kubectl -n fleet-system get fleetapproutes -o json 2>/dev/null \
  | jq -r --arg h "$SITE_HOST" '.items[] | select(.spec.siteHost==$h or .spec.hubHost==$h)')"
phase="$(echo "$route_json" | jq -r '.status.tlsPhase // empty' | head -1)"
tls_msg="$(echo "$route_json" | jq -r '.status.tlsMessage // empty' | head -1)"
echo "tlsPhase=${phase:-n/a}"
if [[ -n "$tls_msg" ]]; then
  echo "tlsMessage=${tls_msg}"
fi

echo "== DNS preflight for $SITE_HOST"
resolved="$(dig +short "$SITE_HOST" A 2>/dev/null | head -1 || true)"
if [[ -z "$resolved" ]]; then
  echo "FAIL: $SITE_HOST did not resolve"
  exit 1
fi
echo "OK: $SITE_HOST -> $resolved"

if [[ -n "$NODE_IP" && "$resolved" != "$NODE_IP" ]]; then
  echo "FAIL: $SITE_HOST resolves to $resolved but NODE_IP=$NODE_IP (site ACME needs DNS to hit the spoke WAN/VIP, not the hub)"
  exit 1
fi

echo "== TCP reachability preflight (public DNS address)"
for port in 80 443; do
  if timeout 5 bash -c "echo >/dev/tcp/${resolved}/${port}" 2>/dev/null; then
    echo "OK: ${resolved}:${port} reachable"
  else
    echo "FAIL: ${resolved}:${port} not reachable (port-forward or firewall)"
    exit 1
  fi
done

echo "== HTTP-01 path preflight (404 OK; HTTPS redirect on challenge path is not)"
challenge_headers="$(curl -sI --max-time 8 "http://${SITE_HOST}/.well-known/acme-challenge/preflight" || true)"
echo "$challenge_headers" | head -5
if echo "$challenge_headers" | grep -qiE '^location: https://.*acme-challenge'; then
  echo "FAIL: HTTP challenge path redirects to HTTPS (breaks ACME HTTP-01)"
  exit 1
fi
echo "OK: HTTP challenge path does not redirect to HTTPS"

if [[ "$PREFLIGHT_ONLY" == "1" ]]; then
  echo "OK — site ACME preflight checks complete (PREFLIGHT_ONLY)"
  exit 0
fi

case "${phase:-}" in
  FailedDNS|Failed|FailedRateLimit)
    echo "FAIL: FleetAppRoute TLS phase is ${phase} — fix preflight/ACME before production"
    exit 1
    ;;
  PendingPreflight|PendingStaging|Queued|Issuing|"")
    echo "WARN: TLS not Ready yet (phase=${phase:-empty}); staging/production issuance may still be in progress"
    ;;
  Ready)
    echo "OK: FleetAppRoute TLS phase Ready"
    ;;
esac

echo "== ManifestWork on $CLUSTER should include TLS ingress"
MW_YAML="$(kubectl -n "$CLUSTER" get manifestwork -o yaml 2>/dev/null | grep -E "certresolver|fleet-route-.*-tls" || true)"
if [[ -z "$MW_YAML" ]]; then
  echo "FAIL: no acme_cluster spoke ingress fragments found on $CLUSTER"
  exit 1
fi
echo "$MW_YAML"

echo "== Spoke phase6 Traefik should NOT use global HTTP→HTTPS redirect (port-forward HTTP-01)"
if kubectl -n "$CLUSTER" get manifestwork phase6-spoke-ingress -o yaml 2>/dev/null | grep -q 'entrypoints.web.http.redirections.entryPoint.to=:443'; then
  echo "WARN: phase6 global HTTP→HTTPS redirect present (breaks ACME HTTP-01 on port-forward spokes)"
else
  echo "OK: no global HTTP→HTTPS redirect on spoke Traefik web entrypoint"
fi

echo "== Catalog route ManifestWork should be HTTPS-only (no HTTP ingress with PathPrefix /)"
if kubectl -n "$CLUSTER" get manifestwork -o yaml 2>/dev/null | grep -E 'fleet-route-.*-tls|certresolver' | grep -q certresolver; then
  echo "OK: TLS ingress with certresolver present"
else
  echo "WARN: no certresolver on spoke route ManifestWork"
fi

if [[ -n "$NODE_IP" ]]; then
  echo "== HTTP smoke via NodePort (404 OK for site-only ACME; redirect WARN only)"
  HEADERS="$(curl -sI -H "Host: ${SITE_HOST}" "http://${NODE_IP}:${NODEPORT_HTTP}/" || true)"
  echo "$HEADERS" | head -5
  if echo "$HEADERS" | grep -qiE '^location: https://'; then
    echo "WARN: NodePort HTTP redirects to HTTPS (global redirect may still be enabled)"
  else
    echo "OK: NodePort HTTP does not redirect (ACME-safe)"
  fi
fi

echo "== HTTPS reachability (requires DNS + issued cert)"
if curl -sf --max-time 10 "https://${SITE_HOST}/" -o /dev/null 2>/dev/null; then
  echo "OK: https://${SITE_HOST}/ reachable with valid certificate"
elif curl -sfk --max-time 10 "https://${SITE_HOST}/" -o /dev/null 2>/dev/null; then
  echo "WARN: https://${SITE_HOST}/ reachable but certificate not yet valid (self-signed or staging)"
else
  echo "WARN: https://${SITE_HOST}/ not reachable yet"
fi

echo "OK — site ACME route checks complete"
