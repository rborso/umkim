#!/usr/bin/env bash
# Deploy and verify homelab-int/ext routing matrix (with per-app timing + burst deploy).
#
# Lab maintainer script — not part of greenfield install.md. Requires homelab sites/clusters
# (see scripts/setup-homelab-sites.sh). From the hub, site WAN IP (172.16.10.x) often fails
# hairpin NAT; set FLEET_EXT_CONNECT_IP to the homelab-ext spoke WireGuard IP (default 10.10.0.12).
#
# Optional env:
#   FLEET_EXT_CONNECT_IP  — spoke WG IP for :30080/:30443 checks (default 10.10.0.12)
#   FLEET_EXT_IP          — documented WAN IP for summary only (default 203.0.113.12)
#   KEEP_INSTANCES=1      — skip cleanup on exit
set -uo pipefail

DCM="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
HUB_GW_HTTP="${HUB_GATEWAY_HTTP:-http://127.0.0.1:30908}"
HUB_GW_HTTPS="${HUB_GATEWAY_HTTPS:-https://203.0.113.1}"
HUB_TLS_CONNECT="${HUB_TLS_CONNECT:-203.0.113.1}"
HUB_DOMAIN="${FLEET_EDGE_DOMAIN:-mgmt.umkim.com}"
SITE_DOMAIN="${FLEET_SITE_DOMAIN:-apps.umkim.com}"
EXT_IP="${FLEET_EXT_IP:-203.0.113.12}"
# From hub, WAN hairpin to EXT_IP often fails; reach homelab-ext NodePorts via WG overlay.
EXT_CONNECT="${FLEET_EXT_CONNECT_IP:-10.10.0.12}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"
KUBECTL="${KUBECTL:-kubectl}"
if ! ${KUBECTL} get nodes >/dev/null 2>&1; then
  if [[ -n "${FLEET_HUB_IP:-}" ]]; then
    KUBECTL() { hub_kubectl "$@"; }
  else
    KUBECTL="sudo kubectl"
  fi
fi

FAIL=0
INSTANCES=()
declare -A TIMING_START TIMING_END TIMING_LABEL
RUN_START=$(date +%s)

log() { echo "=== $* ==="; }
fail() { echo "FAIL: $*" >&2; FAIL=1; }
ok() { echo "OK: $*"; }

timer_start() { TIMING_START[$1]=$(date +%s); log "START $1"; }
timer_end() {
  TIMING_END[$1]=$(date +%s)
  local s=$((TIMING_END[$1] - TIMING_START[$1]))
  TIMING_LABEL[$1]="${s}s"
  printf 'TIME %s %ds (%dm %ds)\n' "$1" "$s" "$((s / 60))" "$((s % 60))"
}

whoami_ok() { grep -qi 'Hostname:' <<<"$1" && grep -qi 'RemoteAddr:' <<<"$1"; }

cleanup() {
  if [[ "${KEEP_INSTANCES:-0}" != "1" ]]; then
    for id in "${INSTANCES[@]}"; do
      curl -sf -X DELETE "${DCM}/catalog-item-instances/${id}" >/dev/null 2>&1 || true
    done
  fi
}
trap cleanup EXIT

wait_instance() {
  local id="$1" name="${2:-}" max="${3:-300}"
  for ((i = 0; i < max; i += 5)); do
    local phase
    phase=$(curl -sf "${DCM}/catalog-item-instances/${id}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',{}).get('phase',''))" 2>/dev/null || echo "")
    if [[ "${phase}" == "Running" || "${phase}" == "Succeeded" ]]; then return 0; fi
    if [[ -n "${id}" ]]; then
      local rp tls healthy
      rp=$(${KUBECTL} -n fleet-system get fleetapproute -l "edge.umkim.com/instance-id=${id}" -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "")
      tls=$(${KUBECTL} -n fleet-system get fleetapproute -l "edge.umkim.com/instance-id=${id}" -o jsonpath='{.items[0].status.tlsPhase}' 2>/dev/null || echo "")
      healthy=$(${KUBECTL} -n fleet-system get fleetapproute -l "edge.umkim.com/instance-id=${id}" -o jsonpath='{.items[0].status.hubPathHealthy}' 2>/dev/null || echo "")
      [[ "${rp}" == "Applied" ]] && return 0
      [[ "${tls}" == "Ready" || "${tls}" == "Queued" ]] && return 0
      [[ "${healthy}" == "true" ]] && return 0
    fi
    [[ "${phase}" == "Failed" ]] && return 1
    sleep 5
  done
  return 1
}

deploy_container() {
  local name="$1" labels="$2" values="$3"
  local http id tmp="/tmp/deploy-${name}-$$.json"
  for _ in 1 2 3 4 5; do
    http=$(curl -s -o "${tmp}" -w '%{http_code}' -X POST "${DCM}/catalog-item-instances" \
      -H 'Content-Type: application/json' \
      -d "{\"api_version\":\"v1alpha1\",\"display_name\":\"${name}\",\"spec\":{\"catalog_item_id\":\"demo-webapp\",\"label_selector\":${labels},\"user_values\":${values}}}")
    [[ "${http}" == "200" || "${http}" == "201" ]] && break
    [[ "${http}" == "000" ]] && sleep 2 && continue
    break
  done
  [[ "${http}" == "200" || "${http}" == "201" ]] || { fail "deploy ${name} HTTP ${http}"; cat "${tmp}" >&2; rm -f "${tmp}"; return 1; }
  id=$(python3 -c "import json; d=json.load(open('${tmp}')); print(d.get('uid') or d.get('id') or d.get('resource_id',''))")
  rm -f "${tmp}"
  INSTANCES+=("${id}")
  echo "${id}"
}

deploy_helm() {
  local name="$1" labels="$2" values="$3"
  local http id tmp="/tmp/deploy-${name}-$$.json"
  for _ in 1 2 3 4 5; do
    http=$(curl -s -o "${tmp}" -w '%{http_code}' -X POST "${DCM}/catalog-item-instances" \
      -H 'Content-Type: application/json' \
      -d "{\"api_version\":\"v1alpha1\",\"display_name\":\"${name}\",\"spec\":{\"catalog_item_id\":\"whoami-helm\",\"label_selector\":${labels},\"user_values\":${values}}}")
    [[ "${http}" == "200" || "${http}" == "201" ]] && break
    [[ "${http}" == "000" ]] && sleep 2 && continue
    break
  done
  [[ "${http}" == "200" || "${http}" == "201" ]] || { fail "helm ${name} HTTP ${http}"; cat "${tmp}" >&2; rm -f "${tmp}"; return 1; }
  id=$(python3 -c "import json; d=json.load(open('${tmp}')); print(d.get('uid') or d.get('id') or d.get('resource_id',''))")
  rm -f "${tmp}"
  INSTANCES+=("${id}")
  echo "${id}"
}

wait_tls() {
  local host="$1" max_secs="${2:-300}" connect="${3:-${HUB_TLS_CONNECT}}" port="${4:-443}"
  local max_iters=$(( max_secs / 10 ))
  [[ "${max_iters}" -lt 1 ]] && max_iters=1
  for ((i = 0; i < max_iters; i++)); do
    local si
    si=$(timeout 8 bash -c "echo | openssl s_client -connect '${connect}:${port}' -servername '${host}' 2>/dev/null | openssl x509 -noout -issuer 2>/dev/null" || true)
    if echo "${si}" | grep -qiE "Let's Encrypt|R11|R12|R13|YR2|E7|E8"; then
      ok "LE cert ready for ${host}"
      return 0
    fi
    sleep 10
  done
  fail "LE cert not ready for ${host} within ${max_secs}s"
  return 1
}

wait_http_ok() {
  local label="$1" url="$2" host="$3" auth="${4:-false}" max_secs="${5:-600}"
  local max_iters=$(( max_secs / 15 ))
  [[ "${max_iters}" -lt 1 ]] && max_iters=1
  for ((i = 0; i < max_iters; i++)); do
    local code body
    code=$(timeout 12 curl -sk -o /tmp/b.txt -w '%{http_code}' -H "Host: ${host}" "${url}/" 2>/dev/null || echo 000)
    body=$(cat /tmp/b.txt 2>/dev/null || true)
    if [[ "${auth}" == "true" ]]; then
      if [[ "${code}" == "401" || "${code}" == "302" || "${code}" == "307" ]]; then
        ok "${label}: auth gate (${code})"
        return 0
      fi
    elif [[ "${code}" == "200" ]] && whoami_ok "${body}"; then
      ok "${label}: HTTP 200 whoami"
      return 0
    fi
    sleep 15
  done
  fail "${label}: HTTP not ready within ${max_secs}s (last ${code:-000})"
  echo "${body:-}" | head -3 >&2
  return 1
}


check_http() {
  local label="$1" url="$2" host="$3" auth="${4:-false}"
  local code body
  code=$(curl -sk -o /tmp/b.txt -w '%{http_code}' -H "Host: ${host}" "${url}/" 2>/dev/null || echo 000)
  body=$(cat /tmp/b.txt)
  if [[ "${auth}" == "true" ]]; then
    [[ "${code}" == "401" || "${code}" == "302" || "${code}" == "307" ]] && { ok "${label}: auth gate (${code})"; return 0; }
    fail "${label}: expected auth gate, got ${code}"; return 1
  fi
  [[ "${code}" == "200" ]] || { fail "${label}: HTTP ${code}"; echo "${body}" | head -3 >&2; return 1; }
  whoami_ok "${body}" || { fail "${label}: not whoami"; return 1; }
  ok "${label}: HTTP 200 whoami"
}

check_tls() {
  local label="$1" host="$2" connect="${3:-}" port="${4:-443}"
  [[ -n "${connect}" ]] || connect="${host}"
  local si
  si=$(timeout 8 bash -c "echo | openssl s_client -connect '${connect}:${port}' -servername '${host}' 2>/dev/null | openssl x509 -noout -subject -issuer 2>/dev/null" || true)
  [[ -n "${si}" ]] || { fail "${label}: no cert for ${host}"; return 1; }
  echo "${si}" | grep -qiE "${host}|Let's Encrypt|R11|R12|R13|YR2|E7|E8" || { fail "${label}: bad cert ${si}"; return 1; }
  ok "${label}: LE cert for ${host}"
}

sync_routes() {
  # fleet-edge-controller rolls the gateway when fleet-routes CM changes; avoid
  # redundant patches here — they interrupt ACME TLS-ALPN on :443.
  return 0
}

wait_gateway() {
  local max="${1:-120}"
  for ((i = 0; i < max; i += 3)); do
    local ready
    ready=$(${KUBECTL} -n dcm get deploy gateway -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
    [[ "${ready}" == "1" ]] && return 0
    sleep 3
  done
  fail "gateway not ready within ${max}s"
  return 1
}

# Wait for providers
for _ in 1 2 3 4 5 6 7 8 9 10; do
  curl -sf "${DCM}/providers" | python3 -c "
import json,sys
n={p['name'] for p in json.load(sys.stdin).get('providers',[])}
for c in ('k8s-homelab-int','k8s-homelab-ext','helm-homelab-int','helm-homelab-ext'):
    assert c in n, c
print('providers ok')
" && break
  sleep 5
done

LABELS_SINGLE='{"fleet.workload/profile":"demo","fleet.placement/scope":"single"}'
LABELS_HA='{"fleet.workload/profile":"demo","fleet.placement/scope":"ha-standby"}'

# Clean prior test instances unless keeping
if [[ "${KEEP_INSTANCES:-0}" != "1" ]]; then
  log "cleanup prior instances"
  curl -sf "${DCM}/catalog-item-instances" | python3 -c "
import json,sys,subprocess
for i in json.load(sys.stdin).get('results',[]):
    subprocess.run(['curl','-sf','-X','DELETE',f'${DCM}/catalog-item-instances/{i[\"uid\"]}'],check=False)
" || true
  ${KUBECTL} -n fleet-system delete fleetapproute --all 2>/dev/null || true
  sleep 5
fi
INSTANCES=()

verify_hub_app() {
  local key="$1" name="$2" host="$3" auth="${4:-false}"
  timer_start "${key}"
  if ! wait_instance "${INSTANCES_BY_NAME[$name]}" "${name}" 600; then
    fail "${key}: instance/route not ready"; timer_end "${key}"; return 1
  fi
  wait_gateway 120 || true
  wait_http_ok "${key}" "${HUB_GW_HTTPS}" "${host}" "${auth}" 600 || true
  wait_tls "${host}" 600 || true
  check_tls "${key} TLS" "${host}" "${HUB_TLS_CONNECT}" || true
  timer_end "${key}"
}

verify_site_app() {
  local key="$1" name="$2" host="$3" auth="${4:-false}"
  timer_start "${key}"
  if ! wait_instance "${INSTANCES_BY_NAME[$name]}" "${name}" 600; then
    fail "${key}: instance/route not ready"; timer_end "${key}"; return 1
  fi
  sleep 10
  if [[ "${auth}" != "true" ]]; then
    wait_http_ok "${key}-http" "http://${EXT_CONNECT}:30080" "${host}" false 300 || true
    wait_tls "${host}" 600 "${EXT_CONNECT}" "30443" || true
    check_tls "${key} TLS" "${host}" "${EXT_CONNECT}" "30443" || true
    wait_http_ok "${key}-https" "https://${EXT_CONNECT}:30443" "${host}" false 300 || true
  else
    wait_http_ok "${key}" "https://${EXT_CONNECT}:30443" "${host}" true 300 || true
  fi
  timer_end "${key}"
}

declare -A INSTANCES_BY_NAME

# --- 1. Sequential baseline: hub single ---
timer_start "demo-hub-single"
ID=$(deploy_container demo-hub-single "${LABELS_SINGLE}" '[
  {"path":"metadata.name","value":"demo-hub-single"},
  {"path":"routing.mode","value":"hub"},
  {"path":"routing.require_auth","value":false},
  {"path":"routing.cluster","value":"homelab-int"}
]')
INSTANCES_BY_NAME[demo-hub-single]="${ID}"
if ! wait_instance "${ID}" demo-hub-single 600; then fail "demo-hub-single not ready"; fi
wait_gateway 120 || true
wait_http_ok "hub-single" "${HUB_GW_HTTPS}" "demo-hub-single.${HUB_DOMAIN}" false 600 || true
wait_tls "demo-hub-single.${HUB_DOMAIN}" 600 || true
check_tls "hub-single TLS" "demo-hub-single.${HUB_DOMAIN}" "${HUB_TLS_CONNECT}" || true
timer_end "demo-hub-single"

# --- 2. BURST: three hub apps deployed within seconds (ACME queue stress) ---
log "BURST deploy (simultaneous): demo-hub-ha, demo-hub-ha-auth, demo-helm-hub"
BURST_START=$(date +%s)
(
  deploy_container demo-hub-ha "${LABELS_HA}" '[
    {"path":"metadata.name","value":"demo-hub-ha"},
    {"path":"routing.mode","value":"hub"},
    {"path":"routing.require_auth","value":false}
  ]' > /tmp/burst-id-demo-hub-ha.txt
) &
(
  deploy_container demo-hub-ha-auth "${LABELS_HA}" '[
    {"path":"metadata.name","value":"demo-hub-ha-auth"},
    {"path":"routing.mode","value":"hub"},
    {"path":"routing.require_auth","value":true}
  ]' > /tmp/burst-id-demo-hub-ha-auth.txt
) &
(
  deploy_helm demo-helm-hub "${LABELS_SINGLE}" '[
    {"path":"metadata.name","value":"demo-helm-hub"},
    {"path":"release.namespace","value":"default"},
    {"path":"routing.mode","value":"hub"},
    {"path":"routing.cluster","value":"homelab-int"}
  ]' > /tmp/burst-id-demo-helm-hub.txt
) &
wait
BURST_DEPLOY_SEC=$(( $(date +%s) - BURST_START ))
ok "BURST deploy finished in ${BURST_DEPLOY_SEC}s"
INSTANCES_BY_NAME[demo-hub-ha]=$(cat /tmp/burst-id-demo-hub-ha.txt)
INSTANCES_BY_NAME[demo-hub-ha-auth]=$(cat /tmp/burst-id-demo-hub-ha-auth.txt)
INSTANCES_BY_NAME[demo-helm-hub]=$(cat /tmp/burst-id-demo-helm-hub.txt)
INSTANCES+=("${INSTANCES_BY_NAME[demo-hub-ha]}" "${INSTANCES_BY_NAME[demo-hub-ha-auth]}" "${INSTANCES_BY_NAME[demo-helm-hub]}")

verify_hub_app "demo-hub-ha" demo-hub-ha "demo-hub-ha.${HUB_DOMAIN}" false
verify_hub_app "demo-hub-ha-auth" demo-hub-ha-auth "demo-hub-ha-auth.${HUB_DOMAIN}" true
verify_hub_app "demo-helm-hub" demo-helm-hub "demo-helm-hub.${HUB_DOMAIN}" false

# --- 3. BURST: two site apps on homelab-ext ---
log "BURST deploy (simultaneous): demo-site-open, demo-site-auth"
BURST2_START=$(date +%s)
(
  deploy_container demo-site-open "${LABELS_SINGLE}" '[
    {"path":"metadata.name","value":"demo-site-open"},
    {"path":"routing.mode","value":"site"},
    {"path":"routing.require_auth","value":false},
    {"path":"routing.site_host","value":"demo-site-open.'"${SITE_DOMAIN}"'"},
    {"path":"routing.cluster","value":"homelab-ext"}
  ]' > /tmp/burst-id-demo-site-open.txt
) &
(
  deploy_container demo-site-auth "${LABELS_SINGLE}" '[
    {"path":"metadata.name","value":"demo-site-auth"},
    {"path":"routing.mode","value":"site"},
    {"path":"routing.require_auth","value":true},
    {"path":"routing.site_host","value":"demo-site-auth.'"${SITE_DOMAIN}"'"},
    {"path":"routing.cluster","value":"homelab-ext"}
  ]' > /tmp/burst-id-demo-site-auth.txt
) &
wait
BURST2_DEPLOY_SEC=$(( $(date +%s) - BURST2_START ))
ok "BURST site deploy finished in ${BURST2_DEPLOY_SEC}s"
INSTANCES_BY_NAME[demo-site-open]=$(cat /tmp/burst-id-demo-site-open.txt)
INSTANCES_BY_NAME[demo-site-auth]=$(cat /tmp/burst-id-demo-site-auth.txt)
INSTANCES+=("${INSTANCES_BY_NAME[demo-site-open]}" "${INSTANCES_BY_NAME[demo-site-auth]}")

verify_site_app "demo-site-open" demo-site-open "demo-site-open.${SITE_DOMAIN}" false
verify_site_app "demo-site-auth" demo-site-auth "demo-site-auth.${SITE_DOMAIN}" true

# HA failover check — count placement targets on two clusters
log "HA placement targets"
HA_ID="${INSTANCES_BY_NAME[demo-hub-ha-auth]:-${INSTANCES_BY_NAME[demo-hub-ha]:-}}"
PT=$(curl -sf "${DCM}/catalog-item-instances/${HA_ID}/placement-targets" 2>/dev/null || echo '{}')
echo "${PT}" | python3 -c "
import json,sys
d=json.load(sys.stdin)
targets=d.get('targets',d.get('placement_targets',[]))
clusters={t.get('cluster') or t.get('cluster_name') or (t.get('provider','').replace('k8s-','') if t.get('provider') else None) for t in targets if isinstance(t,dict)}
clusters={c for c in clusters if c}
print('clusters', clusters)
assert len(clusters)>=2, f'HA needs 2 clusters, got {clusters}'
print('HA standby: 2 clusters')
" && ok "HA standby spans 2 clusters" || fail "HA standby placement check"

# Final cert sweep (all hostnames)
log "final LE cert sweep"
for host in \
  "demo-hub-single.${HUB_DOMAIN}" \
  "demo-hub-ha.${HUB_DOMAIN}" \
  "demo-hub-ha-auth.${HUB_DOMAIN}" \
  "demo-helm-hub.${HUB_DOMAIN}" \
  "demo-site-open.${SITE_DOMAIN}" \
  "demo-site-auth.${SITE_DOMAIN}"; do
  si=$(timeout 8 bash -c "echo | openssl s_client -connect \"${HUB_TLS_CONNECT}:443\" -servername \"${host}\" 2>/dev/null | openssl x509 -noout -issuer 2>/dev/null" || \
       timeout 8 bash -c "echo | openssl s_client -connect \"${EXT_CONNECT}:30443\" -servername \"${host}\" 2>/dev/null | openssl x509 -noout -issuer 2>/dev/null" || true)
  if echo "${si}" | grep -qiE "Let's Encrypt|R11|R12|R13|YR2|E7|E8"; then
    ok "final LE: ${host}"
  else
    fail "final LE missing: ${host} (${si:-no cert})"
  fi
done

RUN_END=$(date +%s)
TOTAL=$((RUN_END - RUN_START))
echo ""
echo "=== TIMING SUMMARY ==="
printf 'Site connect IP:      %s (WAN %s)\n' "${EXT_CONNECT}" "${EXT_IP}"
printf 'BURST hub deploy:     %ds\n' "${BURST_DEPLOY_SEC:-0}"
printf 'BURST site deploy:    %ds\n' "${BURST2_DEPLOY_SEC:-0}"
for key in demo-hub-single demo-hub-ha demo-hub-ha-auth demo-helm-hub demo-site-open demo-site-auth; do
  [[ -n "${TIMING_LABEL[$key]:-}" ]] && printf '  %-22s %s\n' "${key}" "${TIMING_LABEL[$key]}"
done
printf 'TOTAL wall time:      %ds (%dm %ds)\n' "${TOTAL}" "$((TOTAL / 60))" "$((TOTAL % 60))"
echo ""

[[ "${FAIL}" -eq 0 ]] && echo "=== verify-homelab-matrix OK ===" || { echo "=== FAILED ==="; exit 1; }
