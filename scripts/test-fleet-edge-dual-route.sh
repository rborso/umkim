#!/usr/bin/env bash
# E2E checks for catalog dual-route (run on hub with kubectl + curl).
# Usage:
#   ./scripts/test-fleet-edge-dual-route.sh
#   HUB_GATEWAY=http://127.0.0.1:30908 ./scripts/test-fleet-edge-dual-route.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

HUB_GATEWAY="${HUB_GATEWAY:-http://127.0.0.1:30908}"
EDGE_DOMAIN="${FLEET_EDGE_DOMAIN:-apps.example.com}"
NS="${FLEET_ROUTE_NAMESPACE:-fleet-system}"

echo "== CRD"
hub_kubectl get crd fleetapproutes.edge.umkim.com >/dev/null

echo "== Cluster registry"
hub_kubectl -n fleet-system get configmap fleet-edge-clusters -o yaml | grep -q direct_ingress

echo "== Controller pods"
hub_kubectl -n "$NS" rollout status deployment/fleet-edge-controller --timeout=60s
hub_kubectl -n "$NS" rollout status deployment/fleet-edge-route-sync --timeout=60s

echo "== Fleet routes ConfigMap"
hub_kubectl -n dcm get configmap dcm-traefik-fleet-routes -o yaml | grep -q fleet-routes.yml

echo "== Sample FleetAppRoute list"
hub_kubectl -n "$NS" get fleetapproutes -o wide 2>/dev/null || true

echo "== Hub gateway smoke"
curl -sf "${HUB_GATEWAY}/ping" >/dev/null
if hub_kubectl -n "$NS" get fleetapproutes -o json 2>/dev/null \
  | jq -e --arg host "whoami.${EDGE_DOMAIN}" '.items[] | select(.spec.hubHost == $host or .spec.clusterHost == $host)' >/dev/null; then
  curl -sf -H "Host: whoami.${EDGE_DOMAIN}" "${HUB_GATEWAY}/" -o /dev/null
  echo "OK: whoami.${EDGE_DOMAIN} route responds"
else
  echo "SKIP: no FleetAppRoute for whoami.${EDGE_DOMAIN} (gateway /ping OK)"
fi

echo "OK — dual-route stack present"
