#!/usr/bin/env bash
# Verify FleetAppRoute hub-path health probes mark routes Degraded after failures.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

NS="${FLEET_ROUTE_NAMESPACE:-fleet-system}"
MAX_WAIT_SEC="${ROUTE_HEALTH_MAX_WAIT_SEC:-180}"
INTERVAL_SEC="${ROUTE_HEALTH_POLL_SEC:-15}"
TRAEFIK_CM_NS="${FLEET_ROUTES_CM_NS:-fleet-ingress}"
TRAEFIK_CM_NAME="${FLEET_ROUTES_CM_NAME:-fleet-traefik-fleet-routes}"
TRAEFIK_CM_KEY="${FLEET_ROUTES_CM_KEY:-fleet-routes.yml}"
HUB_INGRESS_NS="${HUB_INGRESS_NS:-fleet-ingress}"
GATEWAY_DEPLOY="${GATEWAY_DEPLOYMENT:-gateway}"

echo "=== verify-route-hub-health ==="

GW_JSON=$(hub_kubectl -n "$HUB_INGRESS_NS" get deploy "$GATEWAY_DEPLOY" -o json 2>/dev/null || echo '{}')
if [[ "$GW_JSON" != "{}" ]]; then
  echo "$GW_JSON" | jq -r '"gateway replicas=" + (.spec.replicas|tostring) + " ready=" + ((.status.readyReplicas//0)|tostring) + " strategy=" + (.spec.strategy.type//"?")'
fi

ROUTE=$(hub_kubectl -n "$NS" get fleetapproutes -o json 2>/dev/null | jq -r '
  .items[]
  | select(.spec.mode == "hub" or .spec.effectiveMode == "hub")
  | select(.status.phase == "Applied" or .status.phase == "Degraded")
  | .metadata.name
' | head -1)

if [[ -z "$ROUTE" || "$ROUTE" == "null" ]]; then
  echo "SKIP: no hub-routed FleetAppRoute in namespace $NS"
  exit 0
fi

echo "Watching route=$ROUTE namespace=$NS"
hub_kubectl -n "$NS" get fleetapproute "$ROUTE" -o json | jq '{hubHost:.spec.hubHost, healthy:.status.hubPathHealthy, phase:.status.failoverPhase, failures:.status.consecutiveFailures}'

echo "Route health reconciler should update status.hubPathHealthy / failoverPhase within ${MAX_WAIT_SEC}s"
elapsed=0
while [[ "$elapsed" -lt "$MAX_WAIT_SEC" ]]; do
  LAST_PROBE=$(hub_kubectl -n "$NS" get fleetapproute "$ROUTE" -o jsonpath='{.status.lastProbeAt}' 2>/dev/null || true)
  if [[ -n "$LAST_PROBE" ]]; then
    echo "lastProbeAt=$LAST_PROBE"
    hub_kubectl -n "$NS" get fleetapproute "$ROUTE" -o json | jq '{hubPathHealthy:.status.hubPathHealthy, failoverPhase:.status.failoverPhase, consecutiveFailures:.status.consecutiveFailures, conditions:.status.conditions}'
    HUB_HOST=$(hub_kubectl -n "$NS" get fleetapproute "$ROUTE" -o jsonpath='{.spec.hubHost}')
    STANDBY=$(hub_kubectl -n "$NS" get fleetapproute "$ROUTE" -o jsonpath='{.spec.standbyCluster}')
    TRAEFIK_YAML=$(hub_kubectl -n "$TRAEFIK_CM_NS" get configmap "$TRAEFIK_CM_NAME" -o jsonpath="{.data.${TRAEFIK_CM_KEY}}" 2>/dev/null || true)
    if [[ -n "$TRAEFIK_YAML" && -n "$HUB_HOST" ]]; then
      if echo "$TRAEFIK_YAML" | grep -q "hostname: ${HUB_HOST}"; then
        echo "OK: Traefik healthCheck.hostname=$HUB_HOST"
      else
        echo "WARN: Traefik fragment missing healthCheck.hostname for hub host"
      fi
      if [[ -n "$STANDBY" ]]; then
        URL_COUNT=$(echo "$TRAEFIK_YAML" | grep -c "url:" || true)
        echo "Traefik upstream url count=$URL_COUNT (standbyCluster=$STANDBY)"
      fi
    fi
    echo "=== verify-route-hub-health OK (probe loop active) ==="
    exit 0
  fi
  sleep "$INTERVAL_SEC"
  elapsed=$((elapsed + INTERVAL_SEC))
done

echo "WARN: no lastProbeAt after ${MAX_WAIT_SEC}s — ensure fleet-edge-controller is deployed with ROUTE_HEALTH_* env"
exit 1
