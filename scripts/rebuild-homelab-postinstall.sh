#!/usr/bin/env bash
# Post hub-install: sites, clusters, FleetApp whoami, ACME verification.
# Run on the hub after ./scripts/install-hub.sh and ./scripts/verify-hub-install.sh succeed.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export INV="${INV:--i inventory/hosts.yml -i inventory/hosts-from-hub.yml}"
export ANSIBLE_VAULT_PASSWORD_FILE="${ANSIBLE_VAULT_PASSWORD_FILE:-${HOME}/.umkim-vault-pass}"
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
DOMAIN="${FLEET_EDGE_DOMAIN:-apps.example.com}"
DEMO_HOST="whoami.${DOMAIN}"
CLUSTER="${FLEET_DEMO_CLUSTER:-beta-lab02}"
SITE_ACME="acme-site"
SITE_BETA="beta-site"
SITE_SHARED="homelab-shared"

# NFS for demo cluster (203.0.113.40 mergerfs export)
NFS_SERVER="${FLEET_NFS_SERVER:-203.0.113.40}"
NFS_EXPORT_BASE="${FLEET_NFS_EXPORT_BASE:-/mnt/data/mergerfs/share}"
NFS_CLUSTER_PATH="${FLEET_NFS_CLUSTER_PATH:-${CLUSTER}}"
NFS_MOUNT_PATH="${FLEET_NFS_MOUNT_PATH:-${CLUSTER}}"
# beta-lab02 has port_forward + ACME; skip only when demo cluster is internal
SKIP_CLUSTER_ACME="${POSTINSTALL_SKIP_CLUSTER_ACME:-0}"
SKIP_CLUSTER_INGRESS_AUTH="${POSTINSTALL_SKIP_CLUSTER_INGRESS_AUTH:-0}"

VAULT_ARGS=()
[[ -f "${ANSIBLE_VAULT_PASSWORD_FILE}" ]] && VAULT_ARGS=(--vault-password-file "${ANSIBLE_VAULT_PASSWORD_FILE}")

log() { echo "[postinstall] $*"; }
fail() { echo "[postinstall] FAIL: $*" >&2; exit 1; }

wait_cluster_ready() {
  local name="$1" retries="${2:-120}" delay="${3:-20}"
  local i phase
  for ((i = 0; i < retries; i++)); do
    phase=$(curl -sf "${API}/clusters/${name}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('provisioning_phase',''))" 2>/dev/null || echo "")
    avail=$(curl -sf "${API}/clusters/${name}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('available',False))" 2>/dev/null || echo "False")
    if [[ "${phase}" == "Ready" && "${avail}" == "True" ]]; then
      log "cluster ${name} Ready"
      return 0
    fi
    sleep "${delay}"
  done
  fail "timeout waiting for cluster ${name} Ready (last phase=${phase})"
}

log "Bootstrap FleetAcme CR (fresh install)"
curl -sf "${API}/acme" >/dev/null || fail "fleet-site API not reachable"

log "Verify hub ACME configuration"
FLEET_EDGE_DOMAIN="${DOMAIN}" ACME_VERIFY_HOST="${DOMAIN}" ./scripts/verify-acme-production.sh || true

log "Create sites (org-separation layout)"
if [[ "${POSTINSTALL_SKIP_SITES:-0}" != "1" ]]; then
  ansible-playbook ${INV} playbooks/flows/site-create.yml -e @playbooks/flows/examples/acme-site.yml "${VAULT_ARGS[@]}"
  ansible-playbook ${INV} playbooks/flows/site-create.yml -e @playbooks/flows/examples/beta-site.yml "${VAULT_ARGS[@]}"
  ansible-playbook ${INV} playbooks/flows/site-create.yml -e @playbooks/flows/examples/homelab-shared-site.yml "${VAULT_ARGS[@]}"
else
  log "Skipping site create (POSTINSTALL_SKIP_SITES=1)"
fi

provision_cluster_if_needed() {
  local example="$1"
  shift
  local name
  name=$(python3 -c "import yaml; print(yaml.safe_load(open('${example}'))['fleet_flow_cluster_name'])")
  local phase
  phase=$(curl -sf "${API}/clusters/${name}" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))" 2>/dev/null || echo "")
  if [[ "${phase}" == "Ready" ]]; then
    log "Cluster ${name} already Ready"
    return 0
  fi
  provision_cluster "${example}" "$@"
}

provision_cluster() {
  local example="$1"
  shift
  local name
  name=$(python3 -c "import yaml; print(yaml.safe_load(open('${example}'))['fleet_flow_cluster_name'])")
  log "Provision cluster ${name}"
  ansible-playbook ${INV} playbooks/flows/cluster-create.yml -e @"${example}" "$@" "${VAULT_ARGS[@]}"
  wait_cluster_ready "${name}"
}

provision_cluster_if_needed playbooks/flows/examples/acme-lab01.yml
provision_cluster_if_needed playbooks/flows/examples/beta-lab02.yml

GPU_EXAMPLE=playbooks/flows/examples/lab-gpu03.yml
if ! curl -sf "${API}/clusters/lab-gpu03" | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get('provisioning_phase')=='Ready' or d.get('available') else 1)" 2>/dev/null; then
  log "GPU host prep for lab-gpu03"
  ansible-playbook ${INV} playbooks/flows/gpu-host-prep.yml -e @"${GPU_EXAMPLE}" "${VAULT_ARGS[@]}"
  provision_cluster "${GPU_EXAMPLE}"
  log "GPU cluster enable for lab-gpu03"
  ansible-playbook ${INV} playbooks/flows/gpu-cluster-enable.yml -e @"${GPU_EXAMPLE}" "${VAULT_ARGS[@]}"
else
  log "lab-gpu03 already Ready — skip provision"
fi

if [[ "${SKIP_CLUSTER_ACME}" != "1" ]]; then
  log "Enable cluster ACME on ${CLUSTER}"
  curl -sf -X POST "${API}/clusters/${CLUSTER}/acme/enable" >/dev/null || true

  log "Wait for cluster ACME Ready on ${CLUSTER}"
  for _ in $(seq 1 45); do
    acme_phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('acmeStatus',{}).get('phase',''))" 2>/dev/null || echo "")
    [[ "${acme_phase}" == "Ready" ]] && break
    sleep 20
  done
  [[ "${acme_phase:-}" == "Ready" ]] || fail "cluster ACME not Ready (last=${acme_phase:-none})"
else
  log "Skipping cluster ACME (POSTINSTALL_SKIP_CLUSTER_ACME=1)"
fi

if [[ "${SKIP_CLUSTER_INGRESS_AUTH}" != "1" ]]; then
  log "Verify cluster ingress auth on ${CLUSTER}"
  for _ in $(seq 1 30); do
    auth_phase=$(sudo kubectl get fleetclusterjoin "${CLUSTER}" -n fleet-console -o jsonpath='{.status.ingressAuth.phase}' 2>/dev/null || echo "")
    [[ "${auth_phase}" == "Ready" ]] && break
    sleep 10
  done
  [[ "${auth_phase:-}" == "Ready" ]] || fail "ingress auth not Ready on ${CLUSTER} (last=${auth_phase:-none})"
else
  log "Skipping cluster ingress-auth (POSTINSTALL_SKIP_CLUSTER_INGRESS_AUTH=1)"
fi

log "Configure NFS storage on ${CLUSTER}"
curl -sf -X POST "${API}/clusters/${CLUSTER}/configure-storage" \
  -H 'Content-Type: application/json' \
  -d "{
    \"backend\": \"nfs_external\",
    \"rwo\": true,
    \"rwx\": true,
    \"nfs\": {
      \"enabled\": true,
      \"server\": \"${NFS_SERVER}\",
      \"exportBase\": \"${NFS_EXPORT_BASE}\",
      \"clusterPath\": \"${NFS_CLUSTER_PATH}\",
      \"mountPath\": \"${NFS_MOUNT_PATH}\",
      \"storageClass\": \"nfs-client\",
      \"delegateHost\": \"${NFS_SERVER}\"
    }
  }" >/dev/null

log "Wait for NFS storage Applied on ${CLUSTER}"
for _ in $(seq 1 60); do
  storage_phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('storageStatus',{}).get('phase',''))" 2>/dev/null || echo "")
  [[ "${storage_phase}" == "Applied" ]] && break
  sleep 10
done
[[ "${storage_phase:-}" == "Applied" ]] || fail "NFS storage not Applied (last=${storage_phase:-none})"

log "Install whoami FleetApp (hub routing) on ${CLUSTER}"
FLEET_SITE_API="${API}" FLEET_APP_CLUSTER="${CLUSTER}" FLEET_APP_CHART=whoami \
  FLEET_APP_NAME=whoami FLEET_APP_RELEASE=whoami FLEET_APP_ROUTE_MODE=hub \
  ./scripts/verify-fleet-app.sh

log "Wait for FleetAppRoute TLS (up to 15m)"
for _ in $(seq 1 45); do
  phase=$(sudo kubectl -n fleet-system get fleetapproutes -o json 2>/dev/null \
    | python3 -c "import json,sys; h='${DEMO_HOST}'; print(next((i.get('status',{}).get('tlsPhase','') for i in json.load(sys.stdin).get('items',[]) if i.get('spec',{}).get('hubHost')==h), ''))" 2>/dev/null || echo "")
  [[ "${phase}" == "Ready" ]] && break
  sleep 20
done
[[ "${phase:-}" == "Ready" ]] || fail "FleetAppRoute tlsPhase not Ready for ${DEMO_HOST} (last=${phase:-none})"

log "Verify Let's Encrypt certificate (hub route ${DEMO_HOST})"
issuer=$(echo | openssl s_client -connect "${DEMO_HOST}:443" -servername "${DEMO_HOST}" 2>/dev/null \
  | openssl x509 -noout -issuer 2>/dev/null || true)
echo "${issuer}" | grep -qi "Let's Encrypt" || fail "expected Let's Encrypt cert on ${DEMO_HOST}, got: ${issuer:-none}"

log "Verify whoami page content"
body=$(curl -sf "https://${DEMO_HOST}/")
echo "${body}" | grep -qi 'Hostname' || fail "unexpected whoami body (expected Hostname)"
echo "${body}" | head -5

log "=== rebuild-homelab-postinstall OK ==="
log "whoami: https://${DEMO_HOST}/"
