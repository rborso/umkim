#!/usr/bin/env bash
# Create homelab-int/ext sites and clusters, bootstrap, provision, providers.
#
# Maintainer lab automation (fixed node IPs, SSH jump). Greenfield operators follow
# docs/install.md §5 in the console instead. Requires vault at ~/.umkim-vault-pass.
#
# Optional env: SSH_JUMP, FLEET_AUTH_PASSWORD, COOKIE_JAR
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="${FLEET_API:-http://127.0.0.1:30912/fleet/api/v1alpha1}"
AUTH="${FLEET_AUTH_API:-http://127.0.0.1:30912/fleet/auth/v1alpha1}"
DCM="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
COOKIE_JAR="${COOKIE_JAR:-/tmp/fleet-homelab-cookies.txt}"
SSH_JUMP="${SSH_JUMP:-linux@wg.umkim.com:30200}"
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes)
# ProxyJump fails host-key check on inner hop; ProxyCommand passes options correctly.
SSH_OPTS+=(-o "ProxyCommand=ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -W %h:%p -p 30200 linux@wg.umkim.com")
SSH_USER="${SSH_USER:-linux}"

ADMIN_PASS="${FLEET_AUTH_PASSWORD:-$(ansible-vault view "${ROOT}/inventory/group_vars/all/vault.yml" --vault-password-file "${HOME}/.umkim-vault-pass" 2>/dev/null | awk -F': ' '/fleet_console_auth_admin_password_vault/{gsub(/^"|"$/, "", $2); print $2}')}"

log() { echo "=== $* ==="; }
api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sf -b "$COOKIE_JAR" -X "$method" "${API}${path}" -H 'Content-Type: application/json' -d "$body"
  else
    curl -sf -b "$COOKIE_JAR" -X "$method" "${API}${path}"
  fi
}

login() {
  rm -f "$COOKIE_JAR"
  curl -sf -c "$COOKIE_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"${ADMIN_PASS}\"}" >/dev/null
}

wait_phase() {
  local cluster="$1" want="$2" max="${3:-600}"
  for ((i = 0; i < max; i += 5)); do
    local phase avail
    phase=$(api GET "/clusters/${cluster}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('provisioning_phase',''))" 2>/dev/null || echo "")
    avail=$(api GET "/clusters/${cluster}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('available', False))" 2>/dev/null || echo "False")
    if [[ "$want" == "Ready" && ("$phase" == "Ready" || "$avail" == "True") ]]; then return 0; fi
    [[ "$phase" == "$want" ]] && return 0
    [[ "$phase" == "Failed" ]] && return 1
    sleep 5
  done
  return 1
}

poll_bundle() {
  local cluster="$1" i http
  for i in $(seq 1 60); do
    http=$(curl -s -b "$COOKIE_JAR" -o "/tmp/bundle-${cluster}.tar.gz" -w '%{http_code}' "${API}/clusters/${cluster}/bootstrap-bundle")
    if [[ "$http" == "200" ]] && tar -tzf "/tmp/bundle-${cluster}.tar.gz" bootstrap.sh >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

run_bootstrap() {
  local cluster="$1" node="$2"
  scp "${SSH_OPTS[@]}" "/tmp/bundle-${cluster}.tar.gz" "${SSH_USER}@${node}:/tmp/bootstrap-${cluster}.tar.gz"
  ssh "${SSH_OPTS[@]}" "${SSH_USER}@${node}" bash -s <<EOF
set -euo pipefail
rm -rf "/tmp/bootstrap-${cluster}"
mkdir -p "/tmp/bootstrap-${cluster}"
tar -xzf "/tmp/bootstrap-${cluster}.tar.gz" -C "/tmp/bootstrap-${cluster}"
chmod +x "/tmp/bootstrap-${cluster}/bootstrap.sh"
cd "/tmp/bootstrap-${cluster}"
sudo bash ./bootstrap.sh
EOF
}

install_provider() {
  local cluster="$1" prov="$2"
  local code
  code=$(curl -s -b "$COOKIE_JAR" -o /tmp/prov.json -w '%{http_code}' -X POST "${API}/clusters/${cluster}/providers/${prov}/install")
  [[ "$code" == "202" || "$code" == "200" || "$code" == "409" ]] || { cat /tmp/prov.json >&2; return 1; }
}

wait_provider() {
  local name="$1" max="${2:-600}"
  for ((i = 0; i < max; i += 10)); do
    curl -sf "${DCM}/providers" | python3 -c "
import json,sys
for p in json.load(sys.stdin).get('providers',[]):
    if p['name']=='${name}' and (p.get('health') or p.get('health_status',''))=='ready':
        sys.exit(0)
sys.exit(1)
" && return 0
    sleep 10
  done
  return 1
}

create_site() {
  local name="$1" body="$2"
  local code
  code=$(curl -s -b "$COOKIE_JAR" -o /tmp/site.json -w '%{http_code}' -X POST "${API}/sites" -H 'Content-Type: application/json' -d "$body")
  [[ "$code" == "201" || "$code" == "409" ]] || { echo "create site ${name} HTTP ${code}: $(cat /tmp/site.json)" >&2; return 1; }
}

create_cluster() {
  local body="$1"
  local code
  code=$(curl -s -b "$COOKIE_JAR" -o /tmp/cluster.json -w '%{http_code}' -X POST "${API}/clusters" -H 'Content-Type: application/json' -d "$body")
  [[ "$code" == "201" || "$code" == "409" ]] || { echo "create cluster HTTP ${code}: $(cat /tmp/cluster.json)" >&2; return 1; }
}

login

log "Create sites"
create_site homelab-int "$(cat <<'JSON'
{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"homelab-int"},"spec":{"siteName":"homelab-int","siteId":"homelab-int","provisioningMode":"registry","displayName":"Homelab Internal","region":"homelab","edge":{"exposureMode":"none","wanEgress":{"enabled":false}}}}
JSON
)"

create_site homelab-ext "$(cat <<'JSON'
{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"homelab-ext"},"spec":{"siteName":"homelab-ext","siteId":"homelab-ext","provisioningMode":"registry","displayName":"Homelab External","region":"homelab","edge":{"edgeDomain":"apps.umkim.com","wildcard":"*.apps.umkim.com","exposureMode":"port_forward","defaultExposureMode":"port_forward","externalEndpoints":[{"id":"router-wan","address":"203.0.113.12","role":"primary","description":"Homelab WAN — port-forward to cluster NodePorts"}],"domains":[{"id":"homelab-zone","zone":"apps.umkim.com","wildcard":"*.apps.umkim.com","tlsPolicy":"acme_cluster","ingressEndpointId":"router-wan"}],"wanEgress":{"enabled":true,"expectedWanIp":"203.0.113.12","dnsTargetNote":"A/AAAA *.apps.umkim.com → 203.0.113.12 → node :30080/:30443"}}}}
JSON
)"

for cluster in homelab-int homelab-ext; do
  log "Cleanup prior ${cluster} if any"
  curl -s -b "$COOKIE_JAR" -X DELETE "${API}/clusters/${cluster}?cleanupKubernetes=1" >/dev/null 2>&1 || true
  sudo kubectl delete fleetclusterjoin "${cluster}" -n fleet-console --ignore-not-found --wait=true --timeout=180s 2>/dev/null || true
  sudo kubectl delete managedcluster "${cluster}" --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sleep 3
done

log "Create homelab-int cluster"
create_cluster "$(cat <<'JSON'
{"clusterName":"homelab-int","siteId":"homelab-int","bootstrapHost":"198.51.100.225","topology":"single_node","networkMode":"wireguard","minRootDiskGi":40,"clusterRegion":"homelab","edge":{"exposureMode":"none"},"nodes":[{"address":"198.51.100.225","hostname":"homelab-int","role":"control"}]}
JSON
)"

log "Create homelab-ext cluster"
create_cluster "$(cat <<'JSON'
{"clusterName":"homelab-ext","siteId":"homelab-ext","bootstrapHost":"203.0.113.14","topology":"single_node","networkMode":"wireguard","minRootDiskGi":40,"clusterRegion":"homelab","domainBindings":["homelab-zone"],"edge":{"exposureMode":"port_forward","tlsPolicy":"acme_cluster","ingressEndpointId":"router-wan"},"nodes":[{"address":"203.0.113.14","hostname":"homelab-ext","role":"control"}]}
JSON
)"

for cluster in homelab-int homelab-ext; do
  node="198.51.100.225"; [[ "$cluster" == "homelab-ext" ]] && node="203.0.113.14"
  log "Bundle ${cluster} on ${node}"
  poll_bundle "$cluster"
  run_bootstrap "$cluster" "$node"
  log "Wait Validated ${cluster}"
  wait_phase "$cluster" Validated 480
  log "Provision ${cluster}"
  curl -sf -b "$COOKIE_JAR" -X POST "${API}/clusters/${cluster}/provision" >/dev/null
  log "Wait Ready ${cluster}"
  wait_phase "$cluster" Ready 900
  if [[ "$cluster" == "homelab-ext" ]]; then
    curl -sf -b "$COOKIE_JAR" -X PATCH "${API}/clusters/${cluster}" \
      -H 'Content-Type: application/json' \
      -d '{"edge":{"direct_ingress":true,"traefik_http_port":30080}}' >/dev/null || true
  fi
  for prov in k8s helm; do
    log "Install ${prov} on ${cluster}"
    install_provider "$cluster" "$prov"
    wait_provider "${prov}-${cluster}" 900
  done
done

log "Enable ACME on homelab-ext"
curl -sf -b "$COOKIE_JAR" -X POST "${API}/clusters/homelab-ext/acme/enable" >/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10 11 12; do
  phase=$(api GET "/clusters/homelab-ext" | python3 -c "import json,sys; print(json.load(sys.stdin).get('acmeStatus',{}).get('phase',''))" 2>/dev/null || echo "")
  [[ "$phase" == "Ready" ]] && break
  sleep 10
done

log "Enable hub ingress auth"
curl -sf -b "$COOKIE_JAR" -X PATCH "${API}/ingress-auth" \
  -H 'Content-Type: application/json' -d '{"hubEnabled":true}' >/dev/null
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18; do
  phase=$(sudo kubectl get fleetingressauth fleet-ingress-auth -n fleet-console -o jsonpath='{.status.hub.phase}' 2>/dev/null || echo "")
  [[ "$phase" == "Ready" ]] && break
  sleep 10
done

log "Enable cluster ingress auth on homelab-ext"
LLDAP_PW="$(ansible-vault view "${ROOT}/inventory/group_vars/all/vault.yml" --vault-password-file "${HOME}/.umkim-vault-pass" 2>/dev/null | awk -F': ' '/fleet_lldap_admin_password_vault/{gsub(/^"|"$/, "", $2); print $2}')"
curl -sf -b "$COOKIE_JAR" -X POST "${API}/clusters/homelab-ext/ingress-auth/enable" \
  -H 'Content-Type: application/json' \
  -d "{\"lldapAdminPassword\":\"${LLDAP_PW}\"}" >/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18; do
  phase=$(sudo kubectl get fleetclusterjoin homelab-ext -n fleet-console -o jsonpath='{.status.ingressAuth.phase}' 2>/dev/null || echo "")
  [[ "$phase" == "Ready" ]] && break
  sleep 10
done

log "Setup complete"
api GET "/clusters" | python3 -c "import json,sys; [print(c['name'], c.get('provisioning_phase'), c.get('available')) for c in json.load(sys.stdin).get('clusters',[])]"
curl -sf "${DCM}/providers" | python3 -c "import json,sys; [print(p['name'], p.get('health') or p.get('health_status')) for p in json.load(sys.stdin).get('providers',[])]"
