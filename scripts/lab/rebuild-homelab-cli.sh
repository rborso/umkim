#!/usr/bin/env bash
# Rebuild otthol homelab via umkim CLI (homelab site, lab-single-0 BM, lab-ha-0 VMs).
# Reference: examples/homelab/otthol-lab-reference.yml
# Runbook: examples/homelab/rebuild-homelab-cli.md
#
# Usage:
#   ./scripts/lab/rebuild-homelab-cli.sh
#   DRY_RUN=1 ./scripts/lab/rebuild-homelab-cli.sh
#   SKIP_PHASE=virsh,bm ./scripts/lab/rebuild-homelab-cli.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SITE="${FLEET_SITE:-homelab}"
SINGLE="${FLEET_SINGLE_CLUSTER:-lab-single-0}"
SINGLE_IP="${FLEET_SINGLE_IP:-172.16.10.4}"
BM_HOST="${FLEET_BM_HOST:-virt03}"
HA_CLUSTER="${FLEET_HA_CLUSTER:-lab-ha-0}"
HA_VIP="${FLEET_HA_VIP:-172.16.10.100}"
HA_CP1="${FLEET_HA_CP1:-lab-ha-0-cp1}"
HA_CP2="${FLEET_HA_CP2:-lab-ha-0-cp2}"
HA_CP3="${FLEET_HA_CP3:-lab-ha-0-cp3}"
HA_W1="${FLEET_HA_W1:-lab-ha-0-w1}"
HA_IP1="${FLEET_HA_IP1:-172.16.10.101}"
HA_IP2="${FLEET_HA_IP2:-172.16.10.102}"
HA_IP3="${FLEET_HA_IP3:-172.16.10.103}"
HA_IPW="${FLEET_HA_IPW:-172.16.10.140}"
JUMPHOST="${FLEET_JUMPHOST:-linux@jump.otthol.hu:30200}"
YAIDAP_ROOT="${YAIDAP_ROOT:-/home/linux/yaidap}"
YAIDAP_INVENTORY="${YAIDAP_INVENTORY:-/home/linux/yaidap/inventory/homelab}"
PROVISION_SECRET="${FLEET_PROVISION_SSH_SECRET:-fleet-site-homelab-provision-ssh}"
UMKIM="${UMKIM_BIN:-umkim}"
API="${UMKIM_API_URL:-http://127.0.0.1:30912/fleet/api/v1alpha1}"
AUTH="${UMKIM_AUTH_URL:-http://127.0.0.1:30912/fleet/auth/v1alpha1}"
DRY_RUN="${DRY_RUN:-0}"
SKIP_PHASE="${SKIP_PHASE:-}"
VIRSH_TARGETS="${VIRSH_TARGETS:-172.16.10.2 172.16.10.3 172.16.10.6}"

export UMKIM_API_URL="$API" UMKIM_AUTH_URL="$AUTH"

RUN_ID="$(date +%Y%m%d-%H%M%S)"
mkdir -p "${ROOT}/logs"
LOG="${ROOT}/logs/homelab-rebuild-${RUN_ID}.log"
SUMMARY="${ROOT}/logs/homelab-rebuild-${RUN_ID}.summary.tsv"
echo -e "step\tstart\tend\tseconds" >"$SUMMARY"

JUMPHOST_USER="${JUMPHOST%%@*}"
JUMPHOST_HOSTPORT="${JUMPHOST#*@}"
JUMPHOST_HOST="${JUMPHOST_HOSTPORT%%:*}"
JUMPHOST_PORT="${JUMPHOST_HOSTPORT##*:}"
[ "$JUMPHOST_HOST" = "$JUMPHOST_HOSTPORT" ] && JUMPHOST_PORT=22

log() {
  echo "[$(date -Is)] $*" | tee -a "$LOG"
}

skip_phase() {
  local phase="$1"
  local normalized="${SKIP_PHASE//|/,}"
  [[ ",${normalized}," == *",${phase},"* ]]
}

step() {
  local label="$1"
  shift
  local start end elapsed
  start="$(date -Is)"
  log "START: ${label}"
  if [ "$DRY_RUN" = "1" ]; then
    log "DRY_RUN: $*"
  else
    "$@"
  fi
  end="$(date -Is)"
  elapsed=$(( $(date -d "$end" +%s) - $(date -d "$start" +%s) ))
  log "END: ${label} elapsed=${elapsed}s"
  printf '%s\t%s\t%s\t%s\n' "$label" "$start" "$end" "$elapsed" >>"$SUMMARY"
}

step_optional() {
  local label="$1"
  shift
  local start end elapsed
  start="$(date -Is)"
  log "START: ${label} (optional)"
  if [ "$DRY_RUN" = "1" ]; then
    log "DRY_RUN: $*"
  else
    "$@" || log "WARN: ${label} failed (continuing)"
  fi
  end="$(date -Is)"
  elapsed=$(( $(date -d "$end" +%s) - $(date -d "$start" +%s) ))
  log "END: ${label} elapsed=${elapsed}s"
  printf '%s\t%s\t%s\t%s\n' "$label" "$start" "$end" "$elapsed" >>"$SUMMARY"
}

wait_cluster_gone() {
  local name="$1" max_wait="${2:-600}"
  [ "$DRY_RUN" = "1" ] && return 0
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

bootstrap_via_jumphost() {
  local cluster="$1" host_ip="$2"
  local bundle="/tmp/${cluster}-bootstrap.tar.gz"
  local cookie
  cookie="$(python3 -c "import json; print(json.load(open('${HOME}/.config/umkim/cookies.json'))[0]['Value'])")"
  ssh -p "${JUMPHOST_PORT}" "${JUMPHOST_USER}@${JUMPHOST_HOST}" ssh-keygen -R "${host_ip}" 2>/dev/null || true
  curl -sf -H "Cookie: fleet_session=${cookie}" -o "${bundle}" "${API}/clusters/${cluster}/bootstrap-bundle"
  scp -P "${JUMPHOST_PORT}" "${bundle}" "${JUMPHOST_USER}@${JUMPHOST_HOST}:/tmp/${cluster}.tar.gz"
  ssh -p "${JUMPHOST_PORT}" "${JUMPHOST_USER}@${JUMPHOST_HOST}" bash -s -- "${cluster}" "${host_ip}" <<'REMOTE'
set -euo pipefail
cluster=$1
host=$2
bundle=/tmp/${cluster}.tar.gz
remote_dir=/tmp/bootstrap-${cluster}
ssh-keygen -R "${host}" 2>/dev/null || true
scp -o StrictHostKeyChecking=accept-new "${bundle}" "linux@${host}:${bundle}"
ssh -o StrictHostKeyChecking=accept-new "linux@${host}" "set -euo pipefail
rm -rf '${remote_dir}'
mkdir -p '${remote_dir}'
tar -xzf '${bundle}' -C '${remote_dir}'
chmod +x '${remote_dir}/bootstrap.sh'
cd '${remote_dir}' && sudo ./bootstrap.sh"
REMOTE
}

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
  curl -sf -H "Cookie: fleet_session=${cookie}" -o "${bundle}" "${API}/clusters/${cluster}/bootstrap-bundle"
  for ip in "$@"; do
    ssh -p "${JUMPHOST_PORT}" "${JUMPHOST_USER}@${JUMPHOST_HOST}" ssh-keygen -R "${ip}" 2>/dev/null || true
  done
  python3 -c "
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
"
}

put_inventory_file() {
  local rel="$1"
  $UMKIM site inventory files put "$SITE" --path "$rel" --file "${ROOT}/examples/homelab/inventory/${rel}"
}

virsh_cleanup_hypervisor() {
  local hv_ip="$1"
  ssh -p "${JUMPHOST_PORT}" "${JUMPHOST_USER}@${JUMPHOST_HOST}" \
    ssh -o StrictHostKeyChecking=accept-new "linux@${hv_ip}" bash -s <<'EOF'
set -euo pipefail
virsh list --all --name | while read -r d; do
  [[ -z "$d" ]] && continue
  virsh destroy "$d" 2>/dev/null || true
  virsh undefine "$d" --remove-all-storage 2>/dev/null || true
done
echo "--- virsh list --all on $(hostname) ---"
virsh list --all
EOF
}

log "homelab rebuild run_id=${RUN_ID} log=${LOG}"

# Phase 0 — preflight
step "preflight umkim whoami" $UMKIM whoami
if [ "$DRY_RUN" != "1" ]; then
  if ! sudo kubectl -n fleet-console get secret "$PROVISION_SECRET" >/dev/null 2>&1; then
    log "ERROR: missing secret ${PROVISION_SECRET} in fleet-console (see examples/homelab/rebuild-homelab-cli.md)"
    exit 1
  fi
fi
step_optional "delete stale lab-single-0" $UMKIM cluster delete "$SINGLE" --cleanup-kubernetes
step_optional "wait lab-single-0 gone" wait_cluster_gone "$SINGLE" 600
step_optional "delete stale lab-ha-0" $UMKIM cluster delete "$HA_CLUSTER" --cleanup-kubernetes
step_optional "wait lab-ha-0 gone" wait_cluster_gone "$HA_CLUSTER" 600

# Phase 1 — virsh cleanup on virt01/virt02
if ! skip_phase virsh; then
  for hv in $VIRSH_TARGETS; do
    step "virsh cleanup ${hv}" virsh_cleanup_hypervisor "$hv"
  done
else
  log "SKIP: virsh phase"
fi

# Phase 2 — site + YAIDAP binding + inventory
if ! skip_phase site; then
  step_optional "site create homelab" $UMKIM site create -f "${ROOT}/examples/homelab/site-homelab.json"
  step "site provision set yaidap" $UMKIM site provision set "$SITE" --provider yaidap \
    --jumphost "$JUMPHOST" \
    --ssh-secret "$PROVISION_SECRET" \
    --inventory "$YAIDAP_INVENTORY" \
    --yaidap-root "$YAIDAP_ROOT"
  step "site inventory migrate" $UMKIM site inventory migrate "$SITE"
  step "inventory put hypervisors.yml" put_inventory_file hypervisors.yml
  step "inventory put guests.yml" put_inventory_file guests.yml
  step "inventory put group_vars/all/all.yml" put_inventory_file group_vars/all/all.yml
  step "inventory put lab-ha-0.yml" put_inventory_file lab-ha-0.yml
  step "inventory put group_vars/lab-ha-0.yml" put_inventory_file group_vars/lab-ha-0.yml
  # Every hypervisor that can receive a guest needs host_vars (os, uplinks):
  # YAIDAP validates guest disks[].storage against the target hypervisor os, and
  # a missing hypervisor os fails create_vm for that guest.
  for g in "$HA_CP1" "$HA_CP2" "$HA_CP3" "$HA_W1" virt01 virt02 virt04 "$BM_HOST"; do
    step "inventory put host_vars/${g}.yml" put_inventory_file "host_vars/${g}.yml"
  done
  step "site inventory sync" $UMKIM site inventory sync "$SITE" --wait
else
  log "SKIP: site phase"
fi

# Phase 3 — BM virt03: discover → import → install_bm
if ! skip_phase bm; then
  step "inventory put bare_metals + hypervisors for BM" \
    $UMKIM site inventory files put "$SITE" --path bare_metals.yml --file "${ROOT}/examples/homelab/inventory/bare_metals.yml"
  step "inventory put hypervisors (virt01/02)" \
    $UMKIM site inventory files put "$SITE" --path hypervisors.yml --file "${ROOT}/examples/homelab/inventory/hypervisors.yml"
  step "site inventory sync before BM" $UMKIM site inventory sync "$SITE" --wait
  step "virt03 discover" $UMKIM site host provision "$SITE" "$BM_HOST" --action discover --wait
  step "import preview" $UMKIM site inventory import preview "$SITE" --wait
  step "import apply virt03" $UMKIM site inventory import apply "$SITE" --confirm \
    --paths "host_vars/${BM_HOST}.yml,bare_metals.yml"
  step "virt03 install_bm" $UMKIM site host provision "$SITE" "$BM_HOST" --action install_bm --confirm --wait
else
  log "SKIP: bm phase"
fi

# Phase 4 — lab-single-0 cluster
if ! skip_phase single; then
  step_optional "cluster create lab-single-0" $UMKIM cluster create -f "${ROOT}/examples/homelab/cluster-lab-single-0.json"
  step "wait lab-single-0 ScriptIssued" $UMKIM cluster wait "$SINGLE" --phase ScriptIssued --timeout 10m
  step "bootstrap lab-single-0" bootstrap_via_jumphost "$SINGLE" "$SINGLE_IP"
  step "wait lab-single-0 Validated" $UMKIM cluster wait "$SINGLE" --phase Validated --timeout 45m
  step "provision lab-single-0" $UMKIM cluster provision "$SINGLE"
  step "wait lab-single-0 Ready" $UMKIM cluster wait "$SINGLE" --phase Ready --timeout 45m
else
  log "SKIP: single phase"
fi

# Phase 5 — lab-ha-0 VMs + cluster
if ! skip_phase ha; then
  for h in "$HA_CP1" "$HA_CP2" "$HA_CP3" "$HA_W1"; do
    step "create_vm ${h}" $UMKIM site host provision "$SITE" "$h" --wait
    step_optional "cluster-apply ${h}" $UMKIM site host cluster-apply "$SITE" "$h"
  done
  step_optional "cluster create lab-ha-0" $UMKIM cluster create -f "${ROOT}/examples/homelab/cluster-lab-ha-0.json"
  step "wait lab-ha-0 ScriptIssued" $UMKIM cluster wait "$HA_CLUSTER" --phase ScriptIssued --timeout 10m
  step "seed HA bootstrap SSH keys" seed_ha_bootstrap_ssh_from_jumphost "$HA_CLUSTER" "$HA_IP1" "$HA_IP2" "$HA_IP3"
  step "bootstrap lab-ha-0" bootstrap_via_jumphost "$HA_CLUSTER" "$HA_IP1"
  step "wait lab-ha-0 Validated" $UMKIM cluster wait "$HA_CLUSTER" --phase Validated --timeout 45m
  step "provision lab-ha-0" $UMKIM cluster provision "$HA_CLUSTER"
  step "wait lab-ha-0 Ready" $UMKIM cluster wait "$HA_CLUSTER" --phase Ready --timeout 90m
  step_optional "patch lab-ha-0 ingress-id" $UMKIM cluster patch "$HA_CLUSTER" --ingress-id edge-rp
else
  log "SKIP: ha phase"
fi

# Phase 6 — verify
step "verify site list" $UMKIM site list
step "verify cluster list wide" $UMKIM cluster list -o wide
step "verify lab-single-0" $UMKIM cluster get "$SINGLE"
step "verify lab-ha-0" $UMKIM cluster get "$HA_CLUSTER"

log "OK: homelab rebuild complete run_id=${RUN_ID}"
log "Timing summary: ${SUMMARY}"
column -t "$SUMMARY" 2>/dev/null || cat "$SUMMARY"
