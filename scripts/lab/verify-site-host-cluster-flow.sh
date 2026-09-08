#!/usr/bin/env bash
# E2E: site-host inventory → YAIDAP VM → FleetClusterJoin → bootstrap → hub provision.
# Validates the intended operator flow (not ad-hoc ansible on nodes).
#
# Prerequisites:
#   - Updated fleet-site-controller deployed on hub (local v0.2 changes)
#   - umkim CLI on PATH (or set UMKIM_BIN)
#   - Site provision binding: yaidap + inventory on jumphost
#   - SSH from jumphost to lab nodes (bootstrap runs there)
#
# Usage:
#   FLEET_SITE=homelab ./scripts/lab/verify-site-host-cluster-flow.sh
#   FLEET_SKIP_CLEANUP=1  — leave hosts/clusters after run
#   FLEET_DRY_RUN=1       — print steps only
set -euo pipefail

SITE="${FLEET_SITE:-homelab}"
SINGLE="${FLEET_SINGLE_CLUSTER:-lab-single-1}"
SINGLE_HOST="${FLEET_SINGLE_HOST:-lab-single-1-0}"
SINGLE_IP="${FLEET_SINGLE_IP:-172.16.10.161}"
HA_CLUSTER="${FLEET_HA_CLUSTER:-lab-ha-1}"
HA_VIP="${FLEET_HA_VIP:-172.16.10.160}"
HA_CP0="${FLEET_HA_CP0:-lab-ha-1-cp0}"
HA_CP1="${FLEET_HA_CP1:-lab-ha-1-cp1}"
HA_CP2="${FLEET_HA_CP2:-lab-ha-1-cp2}"
HA_IP0="${FLEET_HA_IP0:-172.16.10.162}"
HA_IP1="${FLEET_HA_IP1:-172.16.10.163}"
HA_IP2="${FLEET_HA_IP2:-172.16.10.164}"
JUMPHOST="${FLEET_JUMPHOST:-linux@jump.otthol.hu:30200}"
UMKIM="${UMKIM_BIN:-umkim}"
API="${UMKIM_API_URL:-http://127.0.0.1:30912/fleet/api/v1alpha1}"
AUTH="${UMKIM_AUTH_URL:-http://127.0.0.1:30912/fleet/auth/v1alpha1}"
SKIP_CLEANUP="${FLEET_SKIP_CLEANUP:-0}"
DRY_RUN="${FLEET_DRY_RUN:-0}"

export UMKIM_API_URL="$API" UMKIM_AUTH_URL="$AUTH"

log() { echo "=== $* ==="; }
run() {
  log "$*"
  if [ "$DRY_RUN" = "1" ]; then return 0; fi
  eval "$@" || return 1
}

run_optional() {
  log "$*"
  if [ "$DRY_RUN" = "1" ]; then return 0; fi
  eval "$@" || true
}

wait_cluster_gone() {
  local name="$1"
  local max_wait="${2:-600}"
  log "wait for cluster $name to be removed (up to ${max_wait}s)"
  if [ "$DRY_RUN" = "1" ]; then return 0; fi
  local i=0
  while [ "$i" -lt "$max_wait" ]; do
    if ! $UMKIM cluster get "$name" -o json >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
    i=$((i + 5))
  done
  return 1
}

cleanup() {
  [ "$SKIP_CLEANUP" = "1" ] && return 0
  log "cleanup"
  run_optional "$UMKIM cluster delete $SINGLE --cleanup-kubernetes"
  run_optional "$UMKIM cluster delete $HA_CLUSTER --cleanup-kubernetes"
  wait_cluster_gone "$SINGLE" 600 || true
  wait_cluster_gone "$HA_CLUSTER" 600 || true
  for h in "$SINGLE_HOST" "$HA_CP0" "$HA_CP1" "$HA_CP2"; do
    run_optional "$UMKIM site host provision $SITE $h --action destroy --confirm --wait"
  done
}

JUMPHOST_USER="${JUMPHOST%%@*}"
JUMPHOST_HOSTPORT="${JUMPHOST#*@}"
JUMPHOST_HOST="${JUMPHOST_HOSTPORT%%:*}"
JUMPHOST_PORT="${JUMPHOST_HOSTPORT##*:}"
[ "$JUMPHOST_HOST" = "$JUMPHOST_HOSTPORT" ] && JUMPHOST_PORT=22

patch_inventory_guest() {
  log "patch_inventory_guest is deprecated; inventory YAML is seeded via seed_inventory_yaml"
}

# shellcheck source=scripts/lab/seed-inventory-yaml.sh
source "$(dirname "$0")/seed-inventory-yaml.sh"

seed_ha_bootstrap_ssh_from_jumphost() {
  local cluster="$1"
  shift
  local bundle="/tmp/${cluster}-bootstrap.tar.gz"
  local cookie ip_list=""
  local ip
  cookie="$(python3 -c "import json; print(json.load(open('${HOME}/.config/umkim/cookies.json'))[0]['Value'])")"
  for ip in "$@"; do
    ip_list="${ip_list}${ip} "
  done
  run "curl -sf -H 'Cookie: fleet_session=${cookie}' -o ${bundle} ${API}/clusters/${cluster}/bootstrap-bundle"
  for ip in "$@"; do
    run_optional "ssh -p ${JUMPHOST_PORT} ${JUMPHOST_USER}@${JUMPHOST_HOST} ssh-keygen -R ${ip} 2>/dev/null || true"
  done
  run "python3 -c \"
import subprocess, tarfile
from pathlib import Path
bundle = Path('${bundle}')
ips = '''${ip_list}'''.split()
with tarfile.open(bundle) as tf:
    pub = tf.extractfile('ssh/id_rsa.pub').read().decode().strip()
for ip in ips:
    if not ip:
        continue
    subprocess.run([
        'ssh', '-p', '${JUMPHOST_PORT}', '${JUMPHOST_USER}@${JUMPHOST_HOST}',
        'ssh', '-o', 'StrictHostKeyChecking=accept-new', f'linux@{ip}',
        'grep -qF \\'' + pub.split()[1] + '\\' ~/.ssh/authorized_keys 2>/dev/null || '
        '{ mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo \\'' + pub + '\\' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys; }',
    ], check=True)
    print('seeded', ip)
\""
}

bootstrap_via_jumphost() {
  local cluster="$1" host_ip="$2"
  local bundle="/tmp/${cluster}-bootstrap.tar.gz"
  local cookie
  cookie="$(python3 -c "import json; print(json.load(open('${HOME}/.config/umkim/cookies.json'))[0]['Value'])")"
  run "ssh -p ${JUMPHOST_PORT} ${JUMPHOST_USER}@${JUMPHOST_HOST} ssh-keygen -R ${host_ip} 2>/dev/null || true"
  run "curl -sf -H 'Cookie: fleet_session=${cookie}' -o ${bundle} ${API}/clusters/${cluster}/bootstrap-bundle"
  run "scp -P ${JUMPHOST_PORT} ${bundle} ${JUMPHOST_USER}@${JUMPHOST_HOST}:/tmp/${cluster}.tar.gz"
  run "ssh -p ${JUMPHOST_PORT} ${JUMPHOST_USER}@${JUMPHOST_HOST} bash -s -- '${cluster}' '${host_ip}' <<'REMOTE'
set -euo pipefail
cluster=\$1
host=\$2
bundle=/tmp/\${cluster}.tar.gz
remote_dir=/tmp/bootstrap-\${cluster}
ssh-keygen -R \"\${host}\" 2>/dev/null || true
scp -o StrictHostKeyChecking=accept-new \"\${bundle}\" \"linux@\${host}:\${bundle}\"
ssh -o StrictHostKeyChecking=accept-new \"linux@\${host}\" \"set -euo pipefail
rm -rf '\${remote_dir}'
mkdir -p '\${remote_dir}'
tar -xzf '\${bundle}' -C '\${remote_dir}'
chmod +x '\${remote_dir}/bootstrap.sh'
cd '\${remote_dir}' && sudo ./bootstrap.sh\"
REMOTE"
}

wait_cluster() {
  local name="$1"
  run "$UMKIM cluster wait $name --phase Ready --timeout 45m --interval 20s"
}

wait_cluster_validated() {
  local name="$1"
  run "$UMKIM cluster wait $name --phase Validated --timeout 45m --interval 20s"
}

trap cleanup EXIT
cleanup

log "seed hub YAIDAP inventory YAML"
seed_inventory_yaml

log "sync connector inventory to jumphost"
run "$UMKIM site inventory sync $SITE --wait"

log "provision VMs (YAIDAP create_vm)"
for h in "$SINGLE_HOST" "$HA_CP0" "$HA_CP1" "$HA_CP2"; do
  run "$UMKIM site host provision $SITE $h --wait"
done

log "cluster-apply (FCJ creation; no-op when autoApply already created FCJ)"
for h in "$SINGLE_HOST" "$HA_CP0" "$HA_CP1" "$HA_CP2"; do
  run_optional "$UMKIM site host cluster-apply $SITE $h"
done

log "wait for bootstrap bundles"
run "$UMKIM cluster wait $SINGLE --phase ScriptIssued --timeout 10m"
run "$UMKIM cluster wait $HA_CLUSTER --phase ScriptIssued --timeout 10m"

log "bootstrap clusters from jumphost"
bootstrap_via_jumphost "$SINGLE" "$SINGLE_IP"
seed_ha_bootstrap_ssh_from_jumphost "$HA_CLUSTER" "$HA_IP0" "$HA_IP1" "$HA_IP2"
bootstrap_via_jumphost "$HA_CLUSTER" "$HA_IP0"

log "wait for bootstrap validation before hub provision"
wait_cluster_validated "$SINGLE"
wait_cluster_validated "$HA_CLUSTER"

log "hub provision + wait Ready"
run "$UMKIM cluster provision $SINGLE"
run "$UMKIM cluster provision $HA_CLUSTER"
wait_cluster "$SINGLE"
wait_cluster "$HA_CLUSTER"

log "OK: site-host cluster flow complete"
