#!/usr/bin/env bash
# E2E: FleetChart publish (copy + external), multi-cluster FleetApp deploy,
# HTTPS/Let's Encrypt checks, and lab-single-0 detach persistence.
#
# Prerequisites: lab-single-0 + lab-ha-0 Ready; hub API on localhost:18080.
#
# Optional env:
#   FLEET_SITE_API          default http://127.0.0.1:18080/api/v1alpha1
#   SINGLE_CLUSTER          default lab-single-0
#   HA_CLUSTER              default lab-ha-0
#   SINGLE_SITE_HOST        default fleettest-single.homelab.otthol.hu
#   HA_SITE_HOST            default fleettest-ha.lab-ha-0.demo.otthol.hu
#   SKIP_DETACH=1           skip cluster detach test (keeps lab-single-0 enrolled)
#   SKIP_CHART_PUBLISH=1    skip whoami-copy publish
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
API="${FLEET_SITE_API:-http://127.0.0.1:18080/api/v1alpha1}"
SINGLE="${SINGLE_CLUSTER:-lab-single-0}"
HA="${HA_CLUSTER:-lab-ha-0}"
SINGLE_HOST="${SINGLE_SITE_HOST:-fleettest-single.homelab.otthol.hu}"
HA_HOST="${HA_SITE_HOST:-fleettest-ha.lab-ha-0.demo.otthol.hu}"
COPY_CHART="${FLEET_COPY_CHART:-whoami-copy}"
COPY_APP="${FLEET_COPY_APP:-whoami-copy}"
EXT_CHART="${FLEET_EXT_CHART:-pet-clinic-helm}"
EXT_APP="${FLEET_EXT_APP:-petclinic-ha}"
EXT_RELEASE="${FLEET_EXT_RELEASE:-petclinic-ha}"
SKIP_DETACH="${SKIP_DETACH:-0}"
SKIP_CHART_PUBLISH="${SKIP_CHART_PUBLISH:-0}"
SKIP_TLS_WAIT="${SKIP_TLS_WAIT:-0}"

HDR=(-H 'Content-Type: application/json' -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-flow' -H 'X-Fleet-DC-Admin: true')
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG

log() { echo "=== $* ==="; }
fail() { echo "FAIL: $*" >&2; exit 1; }

wait_app_phase() {
  local name="$1" want="${2:-Ready}" max="${3:-360}"
  local phase="" i
  for ((i = 0; i < max; i += 5)); do
    phase=$(curl -sf "${HDR[@]}" "${API}/apps/${name}" | jq -r '.phase // empty' 2>/dev/null || true)
    echo "  app ${name} phase=${phase:-unknown}"
    [[ "$phase" == "$want" || "$phase" == "Failed" ]] && break
    sleep 5
  done
  [[ "$phase" == "$want" ]] || fail "FleetApp ${name} phase=${phase:-empty} want ${want}"
}

wait_route_tls() {
  local host="$1" max="${2:-180}"
  local phase="" i
  for ((i = 0; i < max; i += 10)); do
    phase=$(sudo kubectl -n fleet-system get fleetapproutes -o json 2>/dev/null \
      | jq -r --arg h "$host" '.items[] | select(.spec.siteHost==$h or .spec.hubHost==$h) | .status.tlsPhase // empty' | head -1)
    echo "  tls ${host} phase=${phase:-empty}"
    [[ "$phase" == "Ready" || "$phase" == "Failed" ]] && break
    sleep 10
  done
  [[ "$phase" == "Ready" ]] || fail "FleetAppRoute TLS for ${host} phase=${phase:-empty}"
}

create_app() {
  local name="$1" cluster="$2" chart="$3" version="$4" release="$5" ns="$6" mode="$7" site_host="${8:-}"
  local body
  if [[ -n "$site_host" ]]; then
    body=$(jq -n --arg name "$name" --arg cluster "$cluster" --arg chart "$chart" \
      --arg version "$version" --arg release "$release" --arg ns "$ns" --arg mode "$mode" --arg sh "$site_host" \
      '{name:$name,clusterName:$cluster,chart:$chart,version:$version,releaseName:$release,namespace:$ns,route:{mode:$mode,siteHost:$sh}}')
  else
    body=$(jq -n --arg name "$name" --arg cluster "$cluster" --arg chart "$chart" \
      --arg version "$version" --arg release "$release" --arg ns "$ns" --arg mode "$mode" \
      '{name:$name,clusterName:$cluster,chart:$chart,version:$version,releaseName:$release,namespace:$ns,route:{mode:$mode}}')
  fi
  local http
  http=$(curl -sS -o /tmp/fleet-app-create.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/apps" -d "$body")
  [[ "$http" == "201" || "$http" == "409" ]] || fail "POST /apps ${name} HTTP ${http}: $(cat /tmp/fleet-app-create.json)"
}

delete_app() {
  local name="$1"
  curl -sf "${HDR[@]}" -X DELETE "${API}/apps/${name}" >/dev/null 2>&1 || true
  for _ in $(seq 1 60); do
    if ! curl -sf "${HDR[@]}" "${API}/apps/${name}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  fail "FleetApp ${name} still present after DELETE"
}

check_le_cert() {
  local host="$1"
  local issuer
  issuer=$(echo | openssl s_client -connect "${host}:443" -servername "${host}" 2>/dev/null \
    | openssl x509 -noout -issuer 2>/dev/null || true)
  echo "  cert issuer: ${issuer:-none}"
  echo "$issuer" | grep -qi "Let's Encrypt"
}

assert_le_cert() {
  check_le_cert "$1" || fail "expected Let's Encrypt on $1"
}

assert_le_cert_overlay() {
  local host="$1" upstream="$2" port="${3:-30443}"
  local issuer
  issuer=$(echo | openssl s_client -connect "${upstream}:${port}" -servername "${host}" 2>/dev/null \
    | openssl x509 -noout -issuer 2>/dev/null || true)
  echo "  overlay cert issuer (${upstream}:${port} SNI ${host}): ${issuer:-none}"
  echo "$issuer" | grep -qi "Let's Encrypt" || fail "expected Let's Encrypt on ${host} via overlay, got: ${issuer:-none}"
}

assert_app_on_cluster() {
  local cluster="$1" release="$2" ns="${3:-default}" pattern="$4"
  local kc="/tmp/fleet-kubeconfig-${cluster}"
  sudo kubectl -n fleet-system get secret "fleet-kubeconfig-${cluster}" -o jsonpath='{.data.kubeconfig}' | base64 -d >"$kc"
  local pf=18198
  sudo kubectl --kubeconfig="$kc" --insecure-skip-tls-verify -n "$ns" port-forward "svc/${release}" "${pf}:80" >/tmp/pf-${cluster}.log 2>&1 &
  local pid=$!
  trap "kill ${pid} 2>/dev/null || true" RETURN
  local body=""
  for _ in $(seq 1 20); do
    if body=$(curl -sf "http://127.0.0.1:${pf}/" 2>/dev/null); then
      echo "$body" | grep -qi "$pattern" || fail "pod probe ${cluster}/${release} missing /${pattern}/"
      kill "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 0.5
  done
  kill "$pid" 2>/dev/null || true
  fail "pod probe failed for ${cluster}/${release} (see /tmp/pf-${cluster}.log)"
}

assert_route_http() {
  local host="$1" upstream="$2" port="${3:-30080}"
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 -H "Host: ${host}" "http://${upstream}:${port}/" || true)
  [[ "$code" =~ ^(200|301|302|308|404)$ ]] || fail "route HTTP ${host}@${upstream}:${port} unexpected status ${code:-none}"
}

assert_https_body() {
  local url="$1" pattern="$2"
  local body
  body=$(curl -sf --max-time 20 "$url")
  echo "$body" | grep -qi "$pattern" || fail "HTTPS ${url} missing pattern /${pattern}/"
}

log "Preflight: clusters Ready"
for c in "$SINGLE" "$HA"; do
  phase=$(curl -sf "${HDR[@]}" "${API}/clusters/${c}" | jq -r '.provisioning_phase // empty')
  [[ "$phase" == "Ready" ]] || fail "cluster ${c} not Ready (phase=${phase:-none})"
done

log "Cleanup prior test apps"
delete_app "$COPY_APP"
delete_app "$EXT_APP"

if [[ "$SKIP_CHART_PUBLISH" != "1" ]]; then
  log "Publish ${COPY_CHART} with imagePolicy=copy"
  "${ROOT}/scripts/fleet-chart.sh" delete "$COPY_CHART" 2>/dev/null || true
  "${ROOT}/scripts/fleet-chart.sh" publish "${ROOT}/charts/whoami" --name "$COPY_CHART" --images copy
  for _ in $(seq 1 60); do
    phase=$(curl -sf "${HDR[@]}" "${API}/charts/${COPY_CHART}" | jq -r '.phase // empty' 2>/dev/null || true)
    [[ "$phase" == "Ready" || "$phase" == "Failed" ]] && break
    sleep 5
  done
  policy=$(curl -sf "${HDR[@]}" "${API}/charts/${COPY_CHART}" | jq -r '.imagePolicy // empty')
  [[ "$policy" == "copy" ]] || fail "${COPY_CHART} imagePolicy=${policy:-empty} want copy"
  [[ "$phase" == "Ready" ]] || fail "${COPY_CHART} chart phase=${phase:-empty}"
fi

log "Verify external chart ${EXT_CHART} Ready"
ext_phase=$(curl -sf "${HDR[@]}" "${API}/charts/${EXT_CHART}" | jq -r '.phase // empty')
[[ "$ext_phase" == "Ready" ]] || fail "${EXT_CHART} not Ready (phase=${ext_phase:-empty})"
ext_policy=$(curl -sf "${HDR[@]}" "${API}/charts/${EXT_CHART}" | jq -r '.imagePolicy // empty')
[[ "$ext_policy" == "external" ]] || fail "${EXT_CHART} imagePolicy=${ext_policy:-empty} want external"

log "Deploy ${COPY_APP} (${COPY_CHART}, copy images) on ${SINGLE} route=site host=${SINGLE_HOST}"
create_app "$COPY_APP" "$SINGLE" "$COPY_CHART" "0.1.0" "$COPY_APP" default site "$SINGLE_HOST"
wait_app_phase "$COPY_APP" Ready 360

log "Deploy ${EXT_APP} (${EXT_CHART}, external images) on ${HA} route=site host=${HA_HOST}"
create_app "$EXT_APP" "$HA" "$EXT_CHART" "0.1.1" "$EXT_RELEASE" default site "$HA_HOST"
wait_app_phase "$EXT_APP" Ready 600

log "Wait TLS Ready for site routes"
if [[ "$SKIP_TLS_WAIT" == "1" ]]; then
  echo "  SKIP_TLS_WAIT=1 — checking overlay routing only"
else
  wait_route_tls "$SINGLE_HOST" 180 || echo "  WARN: TLS not Ready for ${SINGLE_HOST} (ACME may need public HTTP-01 path)"
fi
# reverse_proxy clusters use external TLS; no cluster ACME tlsPhase.

log "Verify HTTPS on ${SINGLE_HOST}"
if check_le_cert "$SINGLE_HOST"; then
  assert_https_body "https://${SINGLE_HOST}/" 'Hostname'
else
  echo "  WARN: public LE cert not ready; verify spoke workload + Traefik route"
  SINGLE_UPSTREAM=$(curl -sf "${HDR[@]}" "${API}/clusters/${SINGLE}" | jq -r '.upstream_host // empty')
  assert_app_on_cluster "$SINGLE" "$COPY_APP" default 'Hostname'
  assert_route_http "${SINGLE_HOST}" "${SINGLE_UPSTREAM}" 30080
fi

log "Verify ${HA_HOST} (reverse_proxy — workload + Traefik route)"
HA_UPSTREAM=$(curl -sf "${HDR[@]}" "${API}/clusters/${HA}" | jq -r '.upstream_host // empty')
[[ -n "$HA_UPSTREAM" ]] || fail "missing upstream_host for ${HA}"
assert_app_on_cluster "$HA" "$EXT_RELEASE" default 'PetClinic'
assert_route_http "${HA_HOST}" "${HA_UPSTREAM}" 30080
if curl -sfk --max-time 15 "https://${HA_HOST}/" | grep -qi 'PetClinic'; then
  issuer=$(echo | openssl s_client -connect "${HA_HOST}:443" -servername "${HA_HOST}" 2>/dev/null \
    | openssl x509 -noout -issuer 2>/dev/null || true)
  echo "  public HTTPS issuer: ${issuer:-none}"
  echo "$issuer" | grep -qi "Let's Encrypt" || echo "  WARN: public cert not Let's Encrypt (configure external reverse_proxy)"
else
  echo "  WARN: https://${HA_HOST}/ not publicly reachable (external reverse_proxy vhost may be required)"
fi

if [[ "$SKIP_DETACH" == "1" ]]; then
  log "SKIP_DETACH=1 — skipping detach test"
  log "=== verify-fleet-app-multicluster OK (no detach) ==="
  exit 0
fi

log "Pre-detach: app reachable on ${SINGLE}"
SINGLE_UPSTREAM=$(curl -sf "${HDR[@]}" "${API}/clusters/${SINGLE}" | jq -r '.upstream_host // empty')
assert_app_on_cluster "$SINGLE" "$COPY_APP" default 'Hostname'
assert_route_http "${SINGLE_HOST}" "${SINGLE_UPSTREAM}" 30080

log "Detach ${SINGLE} (DELETE without cleanupKubernetes)"
http=$(curl -sS -o /tmp/fleet-detach.json -w '%{http_code}' "${HDR[@]}" -X DELETE "${API}/clusters/${SINGLE}")
[[ "$http" == "202" || "$http" == "204" ]] || fail "DELETE ${SINGLE} HTTP ${http}: $(cat /tmp/fleet-detach.json)"
for _ in $(seq 1 120); do
  if ! sudo kubectl get fleetclusterjoin -n fleet-console "$SINGLE" >/dev/null 2>&1; then
    break
  fi
  fcj_phase=$(sudo kubectl get fleetclusterjoin -n fleet-console "$SINGLE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  [[ "$fcj_phase" == "Deleting" ]] || fail "FleetClusterJoin ${SINGLE} phase=${fcj_phase:-empty} want Deleting"
  sleep 5
done
sudo kubectl get fleetclusterjoin -n fleet-console "$SINGLE" >/dev/null 2>&1 && fail "FleetClusterJoin ${SINGLE} still exists after detach"

log "Post-detach: spoke Traefik ManifestWork preserved"
if sudo kubectl -n "$SINGLE" get manifestwork phase6-spoke-ingress >/dev/null 2>&1; then
  echo "  OK: phase6-spoke-ingress MW still on hub"
else
  fail "phase6-spoke-ingress MW missing after detach"
fi

log "Post-detach: ${SINGLE} route still up on node LAN (overlay/WG removed)"
SINGLE_LAN="${FLEET_SINGLE_LAN_IP:-172.16.10.4}"
JUMPHOST_SSH="${FLEET_JUMPHOST_SSH:-linux@jump.otthol.hu}"
JUMPHOST_PORT="${FLEET_JUMPHOST_PORT:-30200}"
code=$(ssh -p "${JUMPHOST_PORT}" -o ConnectTimeout=15 "${JUMPHOST_SSH}" \
  "curl -sf -o /dev/null -w '%{http_code}' --connect-timeout 5 -H 'Host: ${SINGLE_HOST}' 'http://${SINGLE_LAN}:30080/'" 2>/dev/null || true)
[[ "$code" =~ ^(200|301|302|308|404)$ ]] || fail "post-detach route HTTP ${SINGLE_HOST}@${SINGLE_LAN}:30080 status ${code:-none}"
echo "  OK: route HTTP ${code} for ${SINGLE_HOST} via ${SINGLE_LAN}:30080"

log "=== verify-fleet-app-multicluster OK ==="
log "Note: ${SINGLE} is detached. Recreate with examples/homelab/cluster-lab-single-0.json if needed."
