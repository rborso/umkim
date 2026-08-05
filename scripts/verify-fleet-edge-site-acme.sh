#!/usr/bin/env bash
# Verify catalog site routes with acme_cluster (port-forward profile).
# Run on hub with kubectl access to fleet-edge and spoke clusters.
#
# Usage:
#   ./scripts/verify-fleet-edge-site-acme.sh
#   SITE_HOST=demo.apps.umkim.com CLUSTER=virt02 NODEPORT_HTTP=30080 ./scripts/verify-fleet-edge-site-acme.sh

set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
SITE_HOST="${SITE_HOST:-demo.apps.umkim.com}"
CLUSTER="${CLUSTER:-virt02}"
NODEPORT_HTTP="${NODEPORT_HTTP:-30080}"
NODE_IP="${NODE_IP:-}"

if [[ -z "$NODE_IP" ]]; then
  NODE_IP="$(kubectl get nodes -o wide 2>/dev/null | awk -v c="$CLUSTER" '$1 ~ c {print $6; exit}')"
fi
if [[ -z "$NODE_IP" ]]; then
  echo "WARN: could not resolve NODE_IP for cluster $CLUSTER; set NODE_IP manually"
fi

echo "== FleetAppRoute for site host $SITE_HOST"
kubectl -n fleet-system get fleetapproutes -o wide 2>/dev/null | grep -F "$SITE_HOST" || true

echo "== FleetAppRoute TLS phase for $SITE_HOST"
phase="$(kubectl -n fleet-system get fleetapproutes -o json 2>/dev/null \
  | jq -r --arg h "$SITE_HOST" '.items[] | select(.spec.siteHost==$h or .spec.hubHost==$h) | .status.tlsPhase' | head -1)"
echo "tlsPhase=${phase:-n/a}"

echo "== ManifestWork on $CLUSTER should include TLS ingress"
MW_YAML="$(kubectl -n "$CLUSTER" get manifestwork -o yaml 2>/dev/null | grep -E "certresolver|fleet-route-.*-tls" || true)"
if [[ -z "$MW_YAML" ]]; then
  echo "FAIL: no acme_cluster spoke ingress fragments found on $CLUSTER"
  exit 1
fi
echo "$MW_YAML"

echo "== Spoke phase6 Traefik should redirect HTTP at entrypoint (ACME-safe)"
if kubectl -n "$CLUSTER" get manifestwork phase6-spoke-ingress -o yaml 2>/dev/null | grep -q 'entrypoints.web.http.redirections.entryPoint.to=:443'; then
  echo "OK: phase6 web entrypoint HTTP→HTTPS redirect"
else
  echo "WARN: phase6 missing entrypoint redirect (re-apply phase6-spoke-ingress on spoke)"
fi

echo "== Catalog route ManifestWork should be HTTPS-only (no HTTP ingress with PathPrefix /)"
if kubectl -n "$CLUSTER" get manifestwork -o yaml 2>/dev/null | grep -E 'fleet-route-.*-tls|certresolver' | grep -q certresolver; then
  echo "OK: TLS ingress with certresolver present"
else
  echo "WARN: no certresolver on spoke route ManifestWork"
fi
if kubectl -n "$CLUSTER" get manifestwork -o yaml 2>/dev/null | grep -q 'fleet-redirect-https@file'; then
  echo "WARN: legacy per-route HTTP redirect middleware still present (breaks ACME HTTP-01)"
fi

if [[ -n "$NODE_IP" ]]; then
  echo "== HTTP redirect smoke via NodePort"
  HEADERS="$(curl -sI -H "Host: ${SITE_HOST}" "http://${NODE_IP}:${NODEPORT_HTTP}/" || true)"
  echo "$HEADERS" | head -5
  if ! echo "$HEADERS" | grep -qiE '^location: https://'; then
    echo "WARN: expected HTTP redirect to https (ACME site route or middleware not applied yet)"
  else
    echo "OK: HTTP redirects to HTTPS"
  fi
fi

echo "== HTTPS reachability (requires DNS + issued cert)"
if curl -sfk --max-time 10 "https://${SITE_HOST}/" -o /dev/null 2>/dev/null; then
  echo "OK: https://${SITE_HOST}/ reachable"
elif curl -sf --max-time 10 "https://${SITE_HOST}/" -o /dev/null 2>/dev/null; then
  echo "OK: https://${SITE_HOST}/ reachable (valid cert)"
else
  echo "WARN: https://${SITE_HOST}/ not reachable yet (DNS, ACME issuance, or OpenWrt :443 forward)"
fi

echo "OK — site ACME route checks complete"
