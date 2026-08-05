#!/usr/bin/env bash
# Hub-only readiness checks (no workload clusters required).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
FAIL=0

fail() { echo "FAIL: $*" >&2; FAIL=1; }
ok() { echo "OK: $*"; }

echo "=== verify-hub-install ==="

if ! curl -sf "${API}/catalog-items/demo-webapp" >/dev/null; then
  fail "demo-webapp catalog missing — run fleet-dcm.yml --tags dcm_seed_catalog"
else
  ok "demo-webapp catalog"
fi

if ! curl -sf "${API}/catalog-items/whoami-helm" >/dev/null; then
  fail "whoami-helm catalog missing"
else
  ok "whoami-helm catalog"
fi

if ! curl -sf "${API}/catalog-items/vm-test" >/dev/null; then
  fail "vm-test catalog missing — run fleet-dcm.yml --tags dcm_seed_vm"
else
  ok "vm-test catalog"
fi

for id in demo-container-single demo-container-ha-standby demo-helm-single demo-helm-ha-standby; do
  if ! curl -sf "${API}/policies/${id}" >/dev/null; then
    fail "policy ${id} missing"
  else
    ok "policy ${id}"
  fi
done

PREVIEW=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d '{"mode":"deploy","service_type":"container","catalog_item_id":"demo-webapp","label_selector":{"fleet.workload/profile":"demo","fleet.placement/scope":"single"}}' \
  2>/dev/null || echo '{}')
DEPLOYABLE=$(echo "$PREVIEW" | python3 -c "import sys,json; print(json.load(sys.stdin).get('deployable', False))" 2>/dev/null || echo False)
COUNT=$(echo "$PREVIEW" | python3 -c "import sys,json; print(json.load(sys.stdin).get('eligible_count', 0))" 2>/dev/null || echo 0)
if [[ "$COUNT" -ge 1 ]]; then
  ok "placement-preview eligible_count=${COUNT}"
elif [[ "$DEPLOYABLE" == "True" ]]; then
  ok "placement-preview deployable (no clusters yet is OK on hub-only)"
else
  echo "WARN: placement-preview not deployable (expected until a cluster joins)"
fi

if command -v kubectl >/dev/null 2>&1; then
  kctl() {
    if kubectl "$@" 2>/dev/null; then return 0; fi
    sudo kubectl "$@" 2>/dev/null
  }
  kctl get crd fleetapproutes.edge.umkim.com >/dev/null 2>&1 && ok "FleetAppRoute CRD" || fail "FleetAppRoute CRD missing"
  kctl -n fleet-console get deploy fleet-site-controller >/dev/null 2>&1 && ok "fleet-site-controller" || fail "fleet-site-controller missing"
  kctl -n fleet-system get deploy fleet-edge-controller >/dev/null 2>&1 && ok "fleet-edge-controller" || fail "fleet-edge-controller missing"
  PROBE="$(kctl -n fleet-system get deploy fleet-edge-controller -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="FLEET_GATEWAY_PROBE_HOST")].value}' 2>/dev/null || true)"
  WAN_ENV="$(kctl -n fleet-system get deploy fleet-edge-controller -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="FLEET_GATEWAY_WAN_IP")].value}' 2>/dev/null || true)"
  if [[ -n "$PROBE" && "$PROBE" != "127.0.0.1" && "$PROBE" != "localhost" ]]; then
    ok "fleet-edge-controller FLEET_GATEWAY_PROBE_HOST=${PROBE}"
  elif [[ -n "$WAN_ENV" && "$WAN_ENV" != "127.0.0.1" && "$WAN_ENV" != "localhost" ]]; then
    ok "fleet-edge-controller FLEET_GATEWAY_WAN_IP=${WAN_ENV}"
  else
    fail "fleet-edge-controller ACME probe host invalid (set fleet_hub_public_ipv4; re-run fleet-edge-controller.yml)"
  fi
  kctl -n dcm get deploy control-plane >/dev/null 2>&1 && ok "control-plane deployment" || fail "control-plane missing"
  GW_EXT=$(kctl -n dcm get svc gateway -o jsonpath='{.spec.externalIPs}' 2>/dev/null || true)
  if [[ -n "$GW_EXT" && "$GW_EXT" != "[]" ]]; then
    ok "gateway externalIPs=${GW_EXT}"
  else
    echo "WARN: gateway Service has no externalIPs (required for WAN :80/:443 when dcm_gateway_replicas > 1)"
  fi
else
  echo "SKIP kubectl checks (kubectl not available)"
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "=== verify-hub-install FAILED ==="
  exit 1
fi
echo "=== verify-hub-install OK ==="
