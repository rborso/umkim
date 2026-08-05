#!/usr/bin/env bash
# Console-style routing policy matrix: hub/site routes with optional Authelia + ACME checks.
# Usage: ./scripts/verify-routing-policy-matrix.sh
set -euo pipefail

DCM="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
FLEET="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
HUB_GW_HTTP="${HUB_GATEWAY_HTTP:-http://127.0.0.1:30908}"
HUB_GW_HTTPS="${HUB_GATEWAY_HTTPS:-https://203.0.113.1}"
HUB_TLS_CONNECT="${HUB_TLS_CONNECT:-203.0.113.1}"
HUB_DOMAIN="${FLEET_EDGE_DOMAIN:-mgmt.umkim.com}"
SITE_DOMAIN="${FLEET_SITE_DOMAIN:-apps.umkim.com}"
AUTH_USER="${FLEET_AUTH_USER:-}"
AUTH_PASS="${FLEET_AUTH_PASS:-}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG

FAIL=0
INSTANCES=()

log() { echo "=== $* ==="; }
fail() { echo "FAIL: $*" >&2; FAIL=1; }
ok() { echo "OK: $*"; }

sync_gateway_routes() {
  local checksum
  checksum=$(kubectl -n dcm get configmap dcm-traefik-fleet-routes -o jsonpath='{.data.fleet-routes\.yml}' 2>/dev/null | sha256sum | awk '{print $1}')
  [[ -n "${checksum}" ]] || return 0
  kubectl -n dcm patch deployment gateway --type=strategic -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"checksum/dcm-traefik-fleet-routes\":\"${checksum}\"}}}}}" >/dev/null 2>&1 || true
  kubectl -n dcm rollout status deployment/gateway --timeout=120s >/dev/null 2>&1 || true
}

cleanup() {
  for id in "${INSTANCES[@]}"; do
    curl -sf -X DELETE "${DCM}/catalog-item-instances/${id}" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

wait_instance() {
  local id="$1" name="${2:-}" max="${3:-240}" i
  for ((i = 0; i < max; i += 5)); do
    local phase route_phase container_ok
    phase=$(curl -sf "${DCM}/catalog-item-instances/${id}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',{}).get('phase',''))" 2>/dev/null || echo "")
    if [[ "${phase}" == "Running" || "${phase}" == "Succeeded" ]]; then
      return 0
    fi
    if [[ -n "${name}" ]]; then
      route_phase=$(kubectl -n fleet-system get fleetapproute -o json 2>/dev/null | python3 -c "
import json,sys
name=sys.argv[1]
for item in json.load(sys.stdin).get('items',[]):
    spec=item.get('spec',{})
    hub=spec.get('hubHost','')
    site=spec.get('siteHost','')
    if hub.startswith(name+'.') or site.startswith(name+'.') or spec.get('instanceId','')==sys.argv[2]:
        print(item.get('status',{}).get('phase',''))
        sys.exit(0)
print('')
" "${name}" "${id}" 2>/dev/null || echo "")
      if [[ "${route_phase}" == "Applied" ]]; then
        return 0
      fi
      # Nudge fleet-edge controller if route exists but never reconciled (e.g. after leader election).
      if [[ -n "${route_phase}" && "${route_phase}" != "Applied" ]]; then
        kubectl -n fleet-system annotate fleetapproute -l "edge.umkim.com/instance-id=${id}" \
          fleet.umkim.com/reconcile="$(date +%s)" --overwrite >/dev/null 2>&1 || true
      fi
    fi
    if [[ "${phase}" == "Failed" ]]; then
      curl -sf "${DCM}/catalog-item-instances/${id}" | python3 -m json.tool >&2 || true
      return 1
    fi
    sleep 5
  done
  echo "timeout waiting for instance ${id} (route/app not ready)" >&2
  kubectl -n fleet-system get fleetapproute -l "edge.umkim.com/instance-id=${id}" -o wide 2>/dev/null || true
  return 1
}

deploy_instance() {
  local name="$1" labels_json="$2" json_values="$3"
  local resp http id
  http=$(curl -s -o /tmp/deploy-$$.json -w '%{http_code}' -X POST "${DCM}/catalog-item-instances" \
    -H 'Content-Type: application/json' \
    -d "{
      \"api_version\": \"v1alpha1\",
      \"display_name\": \"${name}\",
      \"spec\": {
        \"catalog_item_id\": \"demo-webapp\",
        \"label_selector\": ${labels_json},
        \"user_values\": ${json_values}
      }
    }")
  if [[ "${http}" != "200" && "${http}" != "201" ]]; then
    echo "deploy ${name} HTTP ${http}" >&2
    cat /tmp/deploy-$$.json >&2
    return 1
  fi
  id=$(python3 -c "import json; d=json.load(open('/tmp/deploy-$$.json')); print(d.get('id') or d.get('uid') or '')")
  [[ -n "${id}" ]] || { echo "no instance id for ${name}" >&2; return 1; }
  INSTANCES+=("${id}")
  echo "${id}"
}

whoami_body_ok() {
  local body="$1"
  echo "${body}" | grep -qi 'Hostname:' && echo "${body}" | grep -qi 'RemoteAddr:'
}

check_http_content() {
  local label="$1" url="$2" host="$3" expect_auth="${4:-false}"
  local code body
  if [[ -n "${host}" ]]; then
    code=$(curl -sk -o /tmp/curl-body-$$.txt -w '%{http_code}' -H "Host: ${host}" "${url}/" 2>/dev/null || echo 000)
  else
    code=$(curl -sk -o /tmp/curl-body-$$.txt -w '%{http_code}' "${url}/" 2>/dev/null || echo 000)
  fi
  body=$(cat /tmp/curl-body-$$.txt)
  if [[ "${expect_auth}" == "true" ]]; then
    if [[ "${code}" == "401" || "${code}" == "302" || "${code}" == "307" ]]; then
      ok "${label}: auth gate (${code})"
      return 0
    fi
    fail "${label}: expected auth gate, got HTTP ${code}"
    return 1
  fi
  if [[ "${code}" == "301" || "${code}" == "308" ]]; then
    if [[ -n "${host}" ]]; then
      code=$(curl -skL -o /tmp/curl-body-$$.txt -w '%{http_code}' -H "Host: ${host}" "${url}/" 2>/dev/null || echo 000)
      body=$(cat /tmp/curl-body-$$.txt)
    fi
  fi
  if [[ "${code}" == "404" ]]; then
    fail "${label}: HTTP 404"
    return 1
  fi
  if [[ "${code}" != "200" ]]; then
    fail "${label}: HTTP ${code}"
    echo "${body}" | head -5 >&2
    return 1
  fi
  if ! whoami_body_ok "${body}"; then
    fail "${label}: body is not whoami (got: $(echo "${body}" | tr '\n' ' ' | head -c 120))"
    return 1
  fi
  ok "${label}: HTTP 200 whoami content"
}

check_tls_cert() {
  local label="$1" host="$2" connect_host="${3:-}" connect_port="${4:-443}"
  local connect subject_issuer
  if [[ -n "${connect_host}" ]]; then
    connect="${connect_host}:${connect_port}"
  else
    connect="${host}:${connect_port}"
  fi
  subject_issuer=$(echo | openssl s_client -connect "${connect}" -servername "${host}" 2>/dev/null | openssl x509 -noout -subject -issuer 2>/dev/null || true)
  if [[ -z "${subject_issuer}" ]]; then
    fail "${label}: TLS cert fetch failed for ${host} via ${connect}"
    return 1
  fi
  if ! echo "${subject_issuer}" | grep -qiE "${host}|Let's Encrypt|R11|R12|R13|YR2|E7|E8"; then
    fail "${label}: TLS cert does not look valid for ${host}"
    echo "${subject_issuer}" || true
    return 1
  fi
  ok "${label}: TLS cert OK for ${host}"
}

log "preflight providers"
for _ in 1 2 3 4 5 6; do
  if curl -sf "${DCM}/providers" | python3 -c "
import json,sys
names={p['name'] for p in json.load(sys.stdin).get('providers',[])}
for c in ('k8s-lab-internal','k8s-lab-external'):
    assert c in names, f'missing provider {c}'
print('providers ok')
"; then
    break
  fi
  sleep 5
done

DEMO_LABELS_SINGLE='{"fleet.workload/profile":"demo","fleet.placement/scope":"single"}'
DEMO_LABELS_HA='{"fleet.workload/profile":"demo","fleet.placement/scope":"ha-standby"}'

# --- Hub single (lab-internal) ---
log "hub single route"
ID=$(deploy_instance "test-hub-single" "${DEMO_LABELS_SINGLE}" '[
  {"path":"metadata.name","value":"test-hub-single"},
  {"path":"routing.mode","value":"hub"},
  {"path":"routing.require_auth","value":false},
  {"path":"routing.cluster","value":"lab-internal"},
  {"path":"resources.cpu.min","value":"200m"},
  {"path":"resources.cpu.max","value":"500m"}
]')
wait_instance "${ID}" "test-hub-single" 120
sync_gateway_routes
sleep 10
kubectl -n fleet-system get fleetapproute test-hub-single -o wide 2>/dev/null || true
check_http_content "hub-single HTTP" "${HUB_GW_HTTP}" "test-hub-single.${HUB_DOMAIN}" false
check_http_content "hub-single HTTPS" "${HUB_GW_HTTPS}" "test-hub-single.${HUB_DOMAIN}" false
check_tls_cert "hub-single TLS" "test-hub-single.${HUB_DOMAIN}" "${HUB_TLS_CONNECT}"

# --- Hub HA (no auth) ---
log "hub HA route"
ID=$(deploy_instance "test-hub-ha" "${DEMO_LABELS_HA}" '[
  {"path":"metadata.name","value":"test-hub-ha"},
  {"path":"routing.mode","value":"hub"},
  {"path":"routing.require_auth","value":false}
]')
wait_instance "${ID}" "test-hub-ha" 180
sync_gateway_routes
sleep 15
check_http_content "hub-ha HTTP" "${HUB_GW_HTTP}" "test-hub-ha.${HUB_DOMAIN}" false
check_http_content "hub-ha HTTPS" "${HUB_GW_HTTPS}" "test-hub-ha.${HUB_DOMAIN}" false

# --- Hub HA + Authelia ---
log "hub HA + authelia"
ID=$(deploy_instance "test-hub-ha-auth" "${DEMO_LABELS_HA}" '[
  {"path":"metadata.name","value":"test-hub-ha-auth"},
  {"path":"routing.mode","value":"hub"},
  {"path":"routing.require_auth","value":true}
]')
wait_instance "${ID}" "test-hub-ha-auth" 180
sync_gateway_routes
sleep 10
check_http_content "hub-ha-auth" "${HUB_GW_HTTPS}" "test-hub-ha-auth.${HUB_DOMAIN}" true

# --- Site without authelia (lab-external / homelab zone) ---
log "site route without authelia (lab-external ACME)"
ID=$(deploy_instance "test-site-open" "${DEMO_LABELS_SINGLE}" '[
  {"path":"metadata.name","value":"test-site-open"},
  {"path":"routing.mode","value":"site"},
  {"path":"routing.require_auth","value":false},
  {"path":"routing.site_host","value":"test-site-open.'"${SITE_DOMAIN}"'"},
  {"path":"routing.cluster","value":"lab-external"}
]')
wait_instance "${ID}" "test-site-open" 180
sync_gateway_routes
sleep 20
# Site WAN via cluster NodePort (port_forward)
EXT_IP=$(curl -sf "${FLEET}/clusters/lab-external" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('upstream_host') or d.get('bootstrap_host',''))" 2>/dev/null || echo "")
if [[ -z "${EXT_IP}" ]]; then
  EXT_IP=$(sudo kubectl get managedcluster lab-external -o jsonpath='{.metadata.labels.fleet\.umkim\.com/bootstrap-host}' 2>/dev/null || true)
fi
if [[ -z "${EXT_IP}" ]]; then
  EXT_IP=$(sudo kubectl get managedcluster lab-external -o jsonpath='{.metadata.labels.fleet\.site}' 2>/dev/null || true)
fi
[[ -n "${EXT_IP}" ]] || EXT_IP="10.10.0.12"
check_http_content "site-open HTTP" "http://${EXT_IP}:30080" "test-site-open.${SITE_DOMAIN}" false
check_tls_cert "site-open TLS" "test-site-open.${SITE_DOMAIN}" "${EXT_IP}" "30443"
check_http_content "site-open HTTPS" "https://${EXT_IP}:30443" "test-site-open.${SITE_DOMAIN}" false

# --- Site behind authelia (lab-external) ---
log "site route with authelia (lab-external)"
ID=$(deploy_instance "test-site-auth" "${DEMO_LABELS_SINGLE}" '[
  {"path":"metadata.name","value":"test-site-auth"},
  {"path":"routing.mode","value":"site"},
  {"path":"routing.require_auth","value":true},
  {"path":"routing.site_host","value":"test-site-auth.'"${SITE_DOMAIN}"'"},
  {"path":"routing.cluster","value":"lab-external"}
]')
wait_instance "${ID}" "test-site-auth" 180
sync_gateway_routes
sleep 15
check_http_content "site-auth HTTPS" "https://${EXT_IP}:30443" "test-site-auth.${SITE_DOMAIN}" true

# Millicore: verify via k8s SP on lab-internal (resource id from hub-single instance)
log "verify millicore CPU on container"
cpu_rid=$(curl -sf "${DCM}/catalog-item-instances" | python3 -c "
import json,sys
for it in json.load(sys.stdin).get('results',[]):
    if it.get('display_name')=='test-hub-single' and it.get('resource_id'):
        print(it['resource_id']); sys.exit(0)
" 2>/dev/null || echo "")
if [[ -z "${cpu_rid}" ]]; then
  cpu_rid=$(curl -sf 'http://10.10.0.10:30190/api/v1alpha1/containers' | python3 -c "
import json,sys
for c in json.load(sys.stdin).get('containers',[]):
    spec=c.get('spec',{})
    meta=spec.get('metadata',{})
    if 'test-hub-single' in (meta.get('name','') or c.get('id','')):
        print(c.get('id','')); sys.exit(0)
" 2>/dev/null || echo "")
fi
cpu_req=$(curl -sf "http://10.10.0.10:30190/api/v1alpha1/containers/${cpu_rid}" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('spec',{}).get('resources',{}).get('cpu',{}).get('min',''))" 2>/dev/null || echo "")
if [[ "${cpu_req}" == "200m" ]]; then
  ok "container spec CPU min is 200m"
else
  fail "container spec CPU min is ${cpu_req:-unset}, expected 200m"
fi

if [[ "${FAIL}" -ne 0 ]]; then
  echo "=== verify-routing-policy-matrix FAILED ===" >&2
  exit 1
fi
echo "=== verify-routing-policy-matrix OK ==="
