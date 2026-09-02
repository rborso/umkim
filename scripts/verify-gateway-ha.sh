#!/usr/bin/env bash
# Verify hub Traefik gateway: replicas ready, RollingUpdate when replicas>1, ping.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

NS="${HUB_INGRESS_NS:-fleet-ingress}"
DEPLOY="${GATEWAY_DEPLOYMENT:-gateway}"
PING_URL="${GATEWAY_PING_URL:-http://127.0.0.1:80/ping}"
TEST_ROLLOUT="${TEST_GATEWAY_ROLLOUT:-0}"
GW_POD_LABEL="${GATEWAY_POD_LABEL:-app=hub-gateway}"

echo "=== verify-gateway-ha ==="

DEPLOY_JSON=$(hub_kubectl -n "$NS" get deploy "$DEPLOY" -o json)
REPLICAS=$(echo "$DEPLOY_JSON" | jq -r '.spec.replicas')
READY=$(echo "$DEPLOY_JSON" | jq -r '.status.readyReplicas // 0')
STRATEGY=$(echo "$DEPLOY_JSON" | jq -r '.spec.strategy.type')
MAX_UNAVAIL=$(echo "$DEPLOY_JSON" | jq -r '.spec.strategy.rollingUpdate.maxUnavailable // empty')
EXPECTED_REPLICAS="${GATEWAY_REPLICAS:-$REPLICAS}"

echo "replicas=$REPLICAS ready=$READY strategy=$STRATEGY maxUnavailable=${MAX_UNAVAIL:-n/a}"

if [[ "$REPLICAS" != "$EXPECTED_REPLICAS" ]]; then
  echo "ERROR: expected spec.replicas=$EXPECTED_REPLICAS got $REPLICAS"
  exit 1
fi

if [[ "$READY" -lt "$REPLICAS" ]]; then
  echo "ERROR: readyReplicas=$READY (expected >= $REPLICAS)"
  hub_kubectl -n "$NS" get pods -l "$GW_POD_LABEL" -o wide
  exit 1
fi

if [[ "$REPLICAS" -gt 1 ]]; then
  HOST_NET=$(echo "$DEPLOY_JSON" | jq -r '.spec.template.spec.hostNetwork // false')
  if [[ "$HOST_NET" == "true" ]]; then
    echo "ERROR: hostNetwork=true with replicas=$REPLICAS — port conflict on single-node hub"
    echo "       Patch hostNetwork=false or scale the fleet-ingress gateway"
    exit 1
  fi
  echo "OK: hostNetwork is not true (pod-network HA; omitted or false)"
  if [[ "$STRATEGY" != "RollingUpdate" ]]; then
    echo "ERROR: expected RollingUpdate strategy, got $STRATEGY"
    exit 1
  fi
  if [[ -n "$MAX_UNAVAIL" && "$MAX_UNAVAIL" != "0" && "$MAX_UNAVAIL" != "0%" ]]; then
    echo "WARN: maxUnavailable=$MAX_UNAVAIL (expected 0)"
  fi
else
  echo "OK: single-replica gateway (lab/single-node hub)"
fi

NOT_READY=$(hub_kubectl -n "$NS" get pods -l "$GW_POD_LABEL" --field-selector=status.phase!=Running -o name 2>/dev/null | wc -l)
if [[ "$NOT_READY" -gt 0 ]]; then
  echo "ERROR: non-running gateway pods"
  hub_kubectl -n "$NS" get pods -l "$GW_POD_LABEL"
  exit 1
fi

echo "OK: $READY gateway pod(s) ready"

if [[ "$REPLICAS" -gt 1 ]]; then
  SOLVER_SPEC=$(hub_kubectl -n "$NS" get deploy gateway-acme -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "missing")
  if [[ "$SOLVER_SPEC" == "missing" ]]; then
    echo "WARN: gateway-acme Deployment not found (HA ACME solver is not installed)"
  elif [[ "$SOLVER_SPEC" != "0" ]]; then
    echo "WARN: gateway-acme spec.replicas=$SOLVER_SPEC (expected 0 in steady state)"
  else
    echo "OK: gateway-acme idle (replicas=0)"
  fi
fi

if [[ "$REPLICAS" -gt 1 ]]; then
  EXT_IPS=$(hub_kubectl -n "$NS" get svc gateway -o jsonpath='{.spec.externalIPs}' 2>/dev/null || true)
  if [[ -z "$EXT_IPS" || "$EXT_IPS" == "[]" ]]; then
    echo "ERROR: gateway Service missing externalIPs (required for WAN :80/:443 when replicas > 1)"
    exit 1
  fi
  echo "OK: gateway externalIPs=$EXT_IPS"
fi

if curl -sf "$PING_URL" >/dev/null 2>&1; then
  echo "OK: gateway ping $PING_URL"
else
  echo "WARN: ping failed at $PING_URL (gateway may still be starting)"
fi

if [[ "$TEST_ROLLOUT" == "1" && "$REPLICAS" -gt 1 ]]; then
  echo "TEST_GATEWAY_ROLLOUT=1: rolling restart with ping loop..."
  hub_kubectl -n "$NS" rollout restart "deployment/${DEPLOY}"
  FAILURES=0
  for _ in $(seq 1 60); do
    if ! curl -sf "$PING_URL" >/dev/null 2>&1; then
      FAILURES=$((FAILURES + 1))
    fi
    sleep 1
  done
  hub_kubectl -n "$NS" rollout status "deployment/${DEPLOY}" --timeout=180s
  echo "ping failures during rollout: $FAILURES"
  if [[ "$FAILURES" -gt 5 ]]; then
    echo "WARN: elevated ping failures during rollout ($FAILURES)"
  else
    echo "OK: rolling restart maintained gateway availability"
  fi
fi

echo "=== verify-gateway-ha OK ==="
