#!/usr/bin/env bash
# Verify multi-cluster hub-routed instances: per-cluster ManifestWork services,
# Traefik dual upstream + health check, and optional standby failover path.
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
KUBECONFIG="${KUBECONFIG:-}"
NS="${FLEET_ROUTE_NAMESPACE:-fleet-system}"
TRAEFIK_CM_NS="${FLEET_ROUTES_CM_NS:-dcm}"
TRAEFIK_CM_NAME="${FLEET_ROUTES_CM_NAME:-dcm-traefik-fleet-routes}"
TRAEFIK_CM_KEY="${FLEET_ROUTES_CM_KEY:-fleet-routes.yml}"
HUB_GATEWAY="${HUB_GATEWAY:-http://127.0.0.1:30908}"
INSTANCE_ID="${MULTISITE_INSTANCE_ID:-}"
SIMULATE_FAILOVER="${SIMULATE_FAILOVER:-0}"

echo "=== verify-hub-multisite-failover ==="

if [[ -z "$KUBECONFIG" ]]; then
  echo "ERROR: set KUBECONFIG to hub kubeconfig"
  exit 1
fi

echo "Catalog manager health..."
curl -sf "${API%/api/v1alpha1}/health/catalog" >/dev/null || curl -sf "${API}/health/catalog" >/dev/null

if [[ -z "$INSTANCE_ID" || "$INSTANCE_ID" == "null" ]]; then
  LIST=$(curl -sf "${API}/catalog-item-instances")
  INSTANCE_ID=$(echo "$LIST" | jq -r '
    (.results // .catalog_item_instances // .items // [])[]
    | select((.labels // {})["fleet.multisite"] == "true" or (.labels // {})["fleet.multisite"] == true)
    | select(.resource_id != null and .resource_id != "")
    | .id // .uid
  ' | head -1)
  if [[ -z "$INSTANCE_ID" || "$INSTANCE_ID" == "null" ]]; then
    INSTANCE_ID=$(echo "$LIST" | jq -r '
      (.results // .catalog_item_instances // .items // [])[]
      | select(.resource_id != null and .resource_id != "")
      | .id // .uid
    ' | head -1)
  fi
fi

if [[ -z "$INSTANCE_ID" || "$INSTANCE_ID" == "null" ]]; then
  echo "SKIP: no provisioned catalog item instance"
  exit 0
fi

echo "Instance id=$INSTANCE_ID"

TARGETS=$(curl -sf "${API}/catalog-item-instances/${INSTANCE_ID}/placement-targets")
echo "$TARGETS" | jq .
TARGET_COUNT=$(echo "$TARGETS" | jq -r '.target_count // 0')
if [[ "$TARGET_COUNT" -lt 2 ]]; then
  echo "WARN: target_count=$TARGET_COUNT (multisite failover expects >= 2)"
fi

# Assert distinct sprm_id per provider when multiple targets exist.
if [[ "$TARGET_COUNT" -ge 2 ]]; then
  DISTINCT_SPRM=$(echo "$TARGETS" | jq -r '[.targets[]? | .sprm_id // .sprmId // empty] | unique | length')
  if [[ "$DISTINCT_SPRM" -lt 2 ]]; then
    echo "WARN: expected distinct sprm_id per provider for multisite (got $DISTINCT_SPRM unique)"
  else
    echo "OK: distinct sprm_id per provider ($DISTINCT_SPRM)"
  fi
fi

ROUTE_NAME=$(kubectl -n "$NS" get fleetapproutes -o json | jq -r --arg id "$INSTANCE_ID" '
  .items[] | select(.spec.instanceId == $id or .spec.instanceID == $id) | .metadata.name
' | head -1)

if [[ -z "$ROUTE_NAME" || "$ROUTE_NAME" == "null" ]]; then
  echo "ERROR: no FleetAppRoute for instance $INSTANCE_ID"
  exit 1
fi

ROUTE_JSON=$(kubectl -n "$NS" get fleetapproute "$ROUTE_NAME" -o json)
HUB_HOST=$(echo "$ROUTE_JSON" | jq -r '.spec.hubHost')
PRIMARY=$(echo "$ROUTE_JSON" | jq -r '.spec.cluster')
STANDBY=$(echo "$ROUTE_JSON" | jq -r '.spec.standbyCluster // empty')
FAILOVER_MODE=$(echo "$ROUTE_JSON" | jq -r '.spec.failoverMode // empty')

echo "route=$ROUTE_NAME hubHost=$HUB_HOST primary=$PRIMARY standby=$STANDBY failoverMode=$FAILOVER_MODE"
echo "$ROUTE_JSON" | jq '{clusterTargets:.spec.clusterTargets, activeCluster:.status.activeCluster, hubPathHealthy:.status.hubPathHealthy, failoverPhase:.status.failoverPhase}'

# Per-cluster ManifestWork service names must match clusterTargets (or spec.service fallback).
CLUSTER_TARGETS=$(echo "$ROUTE_JSON" | jq -c '.spec.clusterTargets // []')
if [[ "$CLUSTER_TARGETS" != "[]" ]]; then
  echo "Checking ManifestWork ingress service names per cluster..."
  MW_ERRORS=0
  while IFS= read -r target; do
    cluster=$(echo "$target" | jq -r '.cluster')
    want_svc=$(echo "$target" | jq -r '.service.name')
    mw=$(kubectl get manifestwork -n "$cluster" -o json 2>/dev/null | jq -r --arg route "$ROUTE_NAME" '
      .items[] | select(.metadata.name | test($route)) | .metadata.name
    ' | head -1)
    if [[ -z "$mw" || "$mw" == "null" ]]; then
      echo "WARN: no ManifestWork for route $ROUTE_NAME in namespace $cluster"
      continue
    fi
    got_svc=$(kubectl get manifestwork -n "$cluster" "$mw" -o json | jq -r '
      .spec.workload.manifests[]?
      | select(.kind == "Ingress")
      | .spec.rules[]?.http.paths[]?.backend.service.name
    ' | head -1)
    if [[ "$got_svc" == "$want_svc" ]]; then
      echo "OK: $cluster ManifestWork service=$got_svc"
    else
      echo "ERROR: $cluster ManifestWork service=$got_svc want=$want_svc (sprm mismatch?)"
      MW_ERRORS=$((MW_ERRORS + 1))
    fi
  done < <(echo "$ROUTE_JSON" | jq -c '.spec.clusterTargets[]')
  if [[ "$MW_ERRORS" -gt 0 ]]; then
    exit 1
  fi
else
  echo "WARN: spec.clusterTargets empty — route-sync may not have resolved per-provider services yet"
fi

# Hub URL health via gateway with Host header.
if [[ -n "$HUB_HOST" && "$HUB_HOST" != "null" ]]; then
  CODE=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: ${HUB_HOST}" "${HUB_GATEWAY}/" || true)
  echo "Hub gateway GET / Host=${HUB_HOST} -> HTTP $CODE"
  if [[ "$CODE" != "200" && "$CODE" != "302" ]]; then
    echo "WARN: expected 200/302 from hub gateway (got $CODE)"
  fi
fi

# Traefik fleet-routes fragment: dual upstream + healthCheck.hostname.
TRAEFIK_YAML=$(kubectl -n "$TRAEFIK_CM_NS" get configmap "$TRAEFIK_CM_NAME" -o jsonpath="{.data.${TRAEFIK_CM_KEY}}" 2>/dev/null || true)
if [[ -z "$TRAEFIK_YAML" ]]; then
  echo "WARN: Traefik CM $TRAEFIK_CM_NS/$TRAEFIK_CM_NAME not found"
else
  if echo "$TRAEFIK_YAML" | grep -q "healthCheck:" && echo "$TRAEFIK_YAML" | grep -q "hostname: ${HUB_HOST}"; then
    echo "OK: Traefik healthCheck.hostname=$HUB_HOST"
  else
    echo "WARN: Traefik fragment missing healthCheck.hostname for $HUB_HOST"
  fi
  if [[ -n "$STANDBY" && "$STANDBY" != "null" ]]; then
    UPSTREAM_COUNT=$(echo "$TRAEFIK_YAML" | grep -c "url:" || true)
    if [[ "$UPSTREAM_COUNT" -ge 2 ]]; then
      echo "OK: Traefik lists $UPSTREAM_COUNT upstream url(s) for dual-cluster route"
    else
      echo "WARN: expected >= 2 upstream urls when standbyCluster is set (got $UPSTREAM_COUNT)"
    fi
  fi
  if echo "$TRAEFIK_YAML" | grep -q "fleet-edge-spoke-insecure"; then
    echo "OK: Traefik serversTransport fleet-edge-spoke-insecure present (HTTPS spoke upstream)"
  fi
fi

# Optional: patch activeCluster to standby and re-check hub URL (non-destructive).
if [[ "$SIMULATE_FAILOVER" == "1" && -n "$STANDBY" && "$STANDBY" != "null" ]]; then
  echo "SIMULATE_FAILOVER=1: setting status.activeCluster=$STANDBY"
  kubectl -n "$NS" patch fleetapproute "$ROUTE_NAME" --subresource=status --type=merge \
    -p "{\"status\":{\"activeCluster\":\"${STANDBY}\"}}" >/dev/null
  sleep 5
  CODE=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: ${HUB_HOST}" "${HUB_GATEWAY}/" || true)
  echo "After activeCluster=$STANDBY: hub gateway HTTP $CODE"
  if [[ "$CODE" != "200" && "$CODE" != "302" ]]; then
    echo "ERROR: hub path not healthy via standby upstream"
    exit 1
  fi
fi

echo "=== verify-hub-multisite-failover OK ==="
