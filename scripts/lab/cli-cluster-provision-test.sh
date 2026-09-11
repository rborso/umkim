#!/usr/bin/env bash
# Homelab CLI smoke: VM provision + single-node + HA multinode inventory + virt03 BM cluster intent.
# Resource limits: VMs on virt01/virt02 only; virt03 is BM/hypervisor single-node cluster member.
#
# Usage:
#   export UMKIM_API_URL=http://127.0.0.1:18080/api/v1alpha1 UMKIM_DEV=1
#   ./scripts/lab/cli-cluster-provision-test.sh
#   DRY_RUN=1 ./scripts/lab/cli-cluster-provision-test.sh
#
# Logs: logs/cli-cluster-provision-test-${RUN_ID}.log
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SITE="${SITE:-homelab}"
if [[ -z "${UMKIM:-}" ]]; then
  if [[ -x /tmp/umkim-dev ]]; then
    UMKIM=/tmp/umkim-dev
  elif [[ -x "${ROOT}/cmd/umkim/umkim" ]]; then
    UMKIM="${ROOT}/cmd/umkim/umkim"
  else
    UMKIM=umkim
  fi
fi
RUN_ID="$(date +%Y%m%d-%H%M%S)"
LOG="${ROOT}/logs/cli-cluster-provision-test-${RUN_ID}.log"
FIXTURES="${ROOT}/scripts/lab/cli-test-fixtures"
mkdir -p logs "$FIXTURES"
export SITE

export UMKIM_API_URL="${UMKIM_API_URL:-http://127.0.0.1:18080/api/v1alpha1}"
export UMKIM_DEV="${UMKIM_DEV:-1}"

log() { echo "[$(date -Iseconds)] $*" | tee -a "$LOG"; }
run() {
  if [[ "${DRY_RUN:-}" == "1" ]]; then
    log "DRY_RUN: $*"
    return 0
  fi
  log "CMD: $*"
  "$@" >>"$LOG" 2>&1
}

wait_for_idle() {
  for i in $(seq 1 90); do
    local n
    n="$("$UMKIM" site host list "$SITE" -o json 2>/dev/null | python3 -c "
import json,sys
try:
  d=json.load(sys.stdin)
  items=d if isinstance(d,list) else d.get('items',[])
  print(sum(1 for h in items if h.get('status',{}).get('phase') in ('Provisioning','Destroying')))
except Exception:
  print(0)
" 2>/dev/null || echo 0)"
    if [[ "${n:-0}" == "0" ]]; then
      return 0
    fi
    [[ "$i" == "1" ]] && log "waiting for host Provisioning/Destroying to finish..."
    sleep 10
  done
  log "WARN: timed out waiting for idle hosts"
  return 1
}

sync_inventory() {
  wait_for_idle || true
  if ! run "$UMKIM" site inventory sync "$SITE" --wait; then
    wait_for_idle || true
    run "$UMKIM" site inventory sync "$SITE" --wait || true
  fi
}

# --- inventory helpers (Python + PyYAML via ruamel or pyyaml) ---
merge_guest() {
  local name="$1" ip="$2" location="$3"
  log "merge_guest $name ip=$ip loc=$location"
  python3 - "$name" "$ip" "$location" <<'PY' >>"$LOG" 2>&1
import sys, urllib.request, json, yaml
name, ip, loc = sys.argv[1], sys.argv[2], sys.argv[3]
api = __import__("os").environ.get("UMKIM_API_URL", "http://127.0.0.1:18080/api/v1alpha1")
site = __import__("os").environ.get("SITE", "homelab")
url = f"{api}/sites/{site}/provision/inventory/files/guests.yml"
with urllib.request.urlopen(url) as r:
    doc = yaml.safe_load(r.read().decode()) or {}
doc.setdefault("guests", {}).setdefault("hosts", {})
entry = doc["guests"]["hosts"].get(name, {}) or {}
if ip:
    entry["ansible_host"] = ip
if loc:
    entry["vm_location"] = loc
doc["guests"]["hosts"][name] = entry
body = yaml.dump(doc, default_flow_style=False, sort_keys=False)
headers = {"Content-Type": "text/yaml; charset=utf-8"}
if __import__("os").environ.get("UMKIM_DEV") == "1":
    headers["X-Fleet-Subject-Kind"] = "User"
    headers["X-Fleet-Subject-Name"] = "umkim-cli"
req = urllib.request.Request(url, data=body.encode(), method="PUT", headers=headers)
with urllib.request.urlopen(req) as r:
    print(r.read().decode())
PY
}

remove_guests() {
  local -a names=("$@")
  log "remove_guests ${names[*]}"
  python3 - "${names[@]}" <<'PY' >>"$LOG" 2>&1
import sys, urllib.request, yaml
names = set(sys.argv[1:])
api = __import__("os").environ.get("UMKIM_API_URL")
site = __import__("os").environ.get("SITE", "homelab")
url = f"{api}/sites/{site}/provision/inventory/files/guests.yml"
with urllib.request.urlopen(url) as r:
    doc = yaml.safe_load(r.read().decode()) or {}
hosts = doc.get("guests", {}).get("hosts", {})
for n in names:
    hosts.pop(n, None)
doc["guests"]["hosts"] = hosts
body = yaml.dump(doc, default_flow_style=False, sort_keys=False)
headers = {"Content-Type": "text/yaml; charset=utf-8"}
if __import__("os").environ.get("UMKIM_DEV") == "1":
    headers["X-Fleet-Subject-Kind"] = "User"
    headers["X-Fleet-Subject-Name"] = "umkim-cli"
req = urllib.request.Request(url, data=body.encode(), method="PUT", headers=headers)
urllib.request.urlopen(req)
print("removed:", ",".join(sorted(names)))
PY
}

write_host_vars() {
  local name="$1" ip="$2" mac="$3" location="${4:-least-utilized}"
  local file="$FIXTURES/host_vars_${name}.yml"
  cat >"$file" <<EOF
---
ansible_host: ${ip}
configure_firewalld: false
cores: 2
disks:
  - bootdisk: true
    device: 0
    disk: vda
    format: qcow2
    name: bootdisk
    size: 40
    storage: local
memory: 4096
os: rocky-10
provisioning_provider: libvirt
uplinks:
  - mac_address: "${mac}"
    name: n0p0
    networks:
      - bridge_name: n0p0_lab_nw
        cloudinit: true
        default_route: true
        hypervisor_uplink: n0p0
        ip: ${ip}
        model: virtio
        network_name: lab_nw
        pxe: false
        services_open: []
        tcp_open: [22]
        udp_open: []
        vm_network: false
        zone: lab
    physical_network: lab_access_l1
vm_location: ${location}
EOF
  run "$UMKIM" site inventory files put "$SITE" --path "host_vars/${name}.yml" --file "$file"
}

fix_host_vm_location() {
  local host="$1" loc="$2"
  local f
  f="$(mktemp)"
  if ! "$UMKIM" site inventory files get "$SITE" --path "host_vars/${host}.yml" >"$f" 2>/dev/null; then
    rm -f "$f"
    return 0
  fi
  if [[ -s "$f" ]] && grep -q 'vm_location:' "$f"; then
    sed -i "s/^vm_location:.*/vm_location: ${loc}/" "$f"
    run "$UMKIM" site inventory files put "$SITE" --path "host_vars/${host}.yml" --file "$f"
  fi
  rm -f "$f"
}

destroy_if_ready() {
  local host="$1"
  local phase loc
  phase="$("$UMKIM" site host get "$SITE" "$host" -o json 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('status',{}).get('phase',''))" 2>/dev/null || true)"
  loc="$("$UMKIM" site host get "$SITE" "$host" -o json 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('vm_location',''))" 2>/dev/null || true)"
  if [[ -n "$loc" && "$loc" != "least-utilized" && "$loc" != "auto" ]]; then
    fix_host_vm_location "$host" "$loc"
  fi
  if [[ "$phase" == "Ready" || "$phase" == "Failed" ]]; then
    wait_for_idle || true
    log "destroy $host (phase=$phase)"
    run "$UMKIM" site host provision "$SITE" "$host" --action destroy --confirm || true
    wait_for_host_phase "$host" Declared || true
    wait_for_idle || true
  fi
}

host_phase() {
  local host="$1"
  "$UMKIM" site host get "$SITE" "$host" -o json 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('status',{}).get('phase',''))" 2>/dev/null || true
}

wait_for_host() {
  local host="$1"
  for _ in $(seq 1 30); do
    if "$UMKIM" site host get "$SITE" "$host" -o json >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  log "WARN: host $host not visible in API after sync"
  return 1
}

wait_for_host_phase() {
  local host="$1" want="$2"
  local phase
  for _ in $(seq 1 120); do
    phase="$(host_phase "$host")"
    if [[ "$phase" == "$want" ]]; then
      log "$host reached phase=$want"
      return 0
    fi
    if [[ "$phase" == "Failed" ]]; then
      log "FAIL: $host phase=Failed"
      return 1
    fi
    sleep 10
  done
  log "WARN: timed out waiting for $host phase=$want (last=$phase)"
  return 1
}

provision_vm() {
  local host="$1"
  local phase attempt
  wait_for_host "$host" || true
  for attempt in 1 2 3; do
    phase="$(host_phase "$host")"
    if [[ "$phase" == "Ready" ]]; then
      log "skip provision $host (already Ready)"
      return 0
    fi
    if [[ "$phase" == "Failed" ]]; then
      log "recover $host from Failed before retry $attempt"
      destroy_if_ready "$host" || true
      sync_inventory || true
      wait_for_host "$host" || true
    fi
    log "provision $host (phase=${phase:-unknown}, attempt=$attempt)"
    wait_for_idle || true
    run "$UMKIM" site host provision "$SITE" "$host" || true
    if wait_for_host_phase "$host" Ready; then
      return 0
    fi
    log "WARN: $host not Ready after attempt $attempt"
    sleep 15
  done
  return 1
}

FREE_VMS=(lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1 cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3)

delete_inventory_file() {
  local path="$1"
  if "$UMKIM" site inventory files get "$SITE" --path "$path" >/dev/null 2>&1; then
    log "delete inventory file $path"
    run "$UMKIM" site inventory files delete "$SITE" --path "$path" || true
  fi
}

cleanup_prior_run() {
  log "=== Pre-cleanup: prior lab-cli clusters, inventory, and orphaned cli-test VMs ==="
  local c
  for c in lab-cli-sn0 lab-cli-ha0 lab-cli-bm0; do
    if "$UMKIM" cluster get "$c" -o json >/dev/null 2>&1; then
      log "delete cluster $c"
      run "$UMKIM" cluster delete "$c" --force || true
    fi
  done
  sleep 5
  for c in lab-cli-sn0.yml lab-cli-ha0.yml lab-cli-bm0.yml \
    group_vars/lab-cli-sn0.yml group_vars/lab-cli-ha0.yml group_vars/lab-cli-bm0.yml \
    host_vars/cli-test-vm1.yml host_vars/cli-test-sn1.yml host_vars/cli-test-vm-a.yml \
    host_vars/cli-test-vm-b.yml host_vars/cli-test-ha-cp1.yml host_vars/cli-test-ha-cp2.yml host_vars/cli-test-ha-cp3.yml; do
    delete_inventory_file "$c"
  done
  sync_inventory
  # Re-stage guests that may still exist on hypervisors after a prior partial run.
  merge_guest cli-test-vm1 172.16.10.201 virt01 || true
  merge_guest cli-test-sn1 172.16.10.202 virt02 || true
  merge_guest cli-test-vm-a 172.16.10.203 virt01 || true
  merge_guest cli-test-vm-b 172.16.10.204 virt02 || true
  merge_guest cli-test-ha-cp1 172.16.10.211 virt01 || true
  merge_guest cli-test-ha-cp2 172.16.10.212 virt02 || true
  merge_guest cli-test-ha-cp3 172.16.10.213 virt01 || true
  for h in cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3; do
    if [[ -f "$FIXTURES/host_vars_${h}.yml" ]]; then
      run "$UMKIM" site inventory files put "$SITE" --path "host_vars/${h}.yml" --file "$FIXTURES/host_vars_${h}.yml" || true
    fi
  done
  sync_inventory
  for h in cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3; do
    destroy_if_ready "$h" || true
  done
  remove_guests cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3 2>/dev/null || true
  sync_inventory
  log "=== Pre-cleanup done ==="
}

log "=== cli-cluster-provision-test run_id=$RUN_ID site=$SITE api=$UMKIM_API_URL ==="

if [[ "${SKIP_CLEANUP:-}" != "1" ]]; then
  cleanup_prior_run
fi

# Phase 0 — free virt01/virt02: destroy running VMs (keep lab-ha inventory rows)
for h in "${FREE_VMS[@]}"; do
  destroy_if_ready "$h" || true
done
sync_inventory
if [[ "${DRY_RUN:-}" != "1" ]]; then
  remove_guests cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3 2>/dev/null || true
  sync_inventory
fi

# Phase 1 — single VM (no cluster)
log "=== Phase 1: create VM cli-test-vm1 ==="
merge_guest cli-test-vm1 172.16.10.201 virt01
write_host_vars cli-test-vm1 172.16.10.201 "52:54:00:0a:c1:01" virt01
sync_inventory
provision_vm cli-test-vm1
run "$UMKIM" site host get "$SITE" cli-test-vm1
destroy_if_ready cli-test-vm1

# Phase 2 — single-node cluster (one VM)
log "=== Phase 2: single-node cluster cli-test-sn1 / lab-cli-sn0 ==="
cat >"$FIXTURES/lab-cli-sn0.yml" <<'EOF'
lab-cli-sn0:
  hosts:
    cli-test-sn1:
EOF
cat >"$FIXTURES/group_vars_lab-cli-sn0.yml" <<'EOF'
topology: single_node
autoApply: true
mode: enroll
EOF
cat >"$FIXTURES/cluster-lab-cli-sn0.json" <<'EOF'
{
  "clusterName": "lab-cli-sn0",
  "siteId": "homelab",
  "bootstrapHost": "172.16.10.202",
  "topology": "single_node",
  "networkMode": "wireguard",
  "clusterRegion": "homelab",
  "jumphost": "linux@jump.otthol.hu:30200",
  "nodes": [
    {"address": "172.16.10.202", "hostname": "cli-test-sn1", "role": "control"}
  ]
}
EOF
merge_guest cli-test-sn1 172.16.10.202 virt02
write_host_vars cli-test-sn1 172.16.10.202 "52:54:00:0a:c1:02" virt02
run "$UMKIM" site inventory files put "$SITE" --path lab-cli-sn0.yml --file "$FIXTURES/lab-cli-sn0.yml"
run "$UMKIM" site inventory files put "$SITE" --path group_vars/lab-cli-sn0.yml --file "$FIXTURES/group_vars_lab-cli-sn0.yml"
sync_inventory
provision_vm cli-test-sn1
run "$UMKIM" site host cluster-apply "$SITE" cli-test-sn1 || true
run "$UMKIM" cluster create -f "$FIXTURES/cluster-lab-cli-sn0.json" || true
log "Phase 2: cluster lab-cli-sn0 created (bootstrap: umkim cluster bootstrap lab-cli-sn0 --ssh-user linux)"
log "Phase 2: keeping cli-test-sn1 (cluster member; destroy blocked until cluster delete)"

# Phase 3 — multiple VMs (no cluster)
log "=== Phase 3: multiple VMs cli-test-vm-a / cli-test-vm-b ==="
merge_guest cli-test-vm-a 172.16.10.203 virt01
merge_guest cli-test-vm-b 172.16.10.204 virt02
write_host_vars cli-test-vm-a 172.16.10.203 "52:54:00:0a:c1:03" virt01
write_host_vars cli-test-vm-b 172.16.10.204 "52:54:00:0a:c1:04" virt02
sync_inventory
provision_vm cli-test-vm-a
provision_vm cli-test-vm-b
destroy_if_ready cli-test-vm-a
destroy_if_ready cli-test-vm-b

# Phase 4 — HA multinode (3 control-plane VMs on virt01+virt02)
log "=== Phase 4: HA multinode lab-cli-ha0 (3 VMs) ==="
cat >"$FIXTURES/lab-cli-ha0.yml" <<'EOF'
lab-cli-ha0:
  hosts:
    cli-test-ha-cp1:
    cli-test-ha-cp2:
    cli-test-ha-cp3:
EOF
cat >"$FIXTURES/group_vars_lab-cli-ha0.yml" <<'EOF'
topology: ha_3_node
autoApply: true
haGroup: lab-cli-ha0
controlPlaneVip: 172.16.10.210
mode: enroll
EOF
cat >"$FIXTURES/cluster-lab-cli-ha0.json" <<'EOF'
{
  "clusterName": "lab-cli-ha0",
  "siteId": "homelab",
  "bootstrapHost": "172.16.10.211",
  "controlPlaneVip": "172.16.10.210",
  "topology": "ha_3_node",
  "networkMode": "wireguard",
  "minRootDiskGi": 38,
  "clusterRegion": "homelab",
  "jumphost": "linux@jump.otthol.hu:30200",
  "nodes": [
    {"address": "172.16.10.211", "hostname": "cli-test-ha-cp1", "role": "control"},
    {"address": "172.16.10.212", "hostname": "cli-test-ha-cp2", "role": "control"},
    {"address": "172.16.10.213", "hostname": "cli-test-ha-cp3", "role": "control"}
  ]
}
EOF
merge_guest cli-test-ha-cp1 172.16.10.211 virt01
merge_guest cli-test-ha-cp2 172.16.10.212 virt02
merge_guest cli-test-ha-cp3 172.16.10.213 virt01
write_host_vars cli-test-ha-cp1 172.16.10.211 "52:54:00:0a:c2:01" virt01
write_host_vars cli-test-ha-cp2 172.16.10.212 "52:54:00:0a:c2:02" virt02
write_host_vars cli-test-ha-cp3 172.16.10.213 "52:54:00:0a:c2:03" virt01
run "$UMKIM" site inventory files put "$SITE" --path lab-cli-ha0.yml --file "$FIXTURES/lab-cli-ha0.yml"
run "$UMKIM" site inventory files put "$SITE" --path group_vars/lab-cli-ha0.yml --file "$FIXTURES/group_vars_lab-cli-ha0.yml"
sync_inventory
for h in cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3; do
  provision_vm "$h"
  run "$UMKIM" site host cluster-apply "$SITE" "$h" || true
done
run "$UMKIM" cluster create -f "$FIXTURES/cluster-lab-cli-ha0.json" || true
log "Phase 4: HA cluster lab-cli-ha0 created (bootstrap + umkim cluster wait --timeout 90m)"
log "Phase 4: keeping cli-test-ha-cp* (cluster members; destroy blocked until cluster delete)"

# Phase 5 — virt03 single-node cluster (hypervisor / BM member, no VM)
log "=== Phase 5: virt03 single-node cluster lab-cli-bm0 ==="
cat >"$FIXTURES/lab-cli-bm0.yml" <<'EOF'
lab-cli-bm0:
  hosts:
    virt03:
EOF
cat >"$FIXTURES/group_vars_lab-cli-bm0.yml" <<'EOF'
topology: single_node
autoApply: true
mode: enroll
EOF
cat >"$FIXTURES/cluster-lab-cli-bm0.json" <<'EOF'
{
  "clusterName": "lab-cli-bm0",
  "siteId": "homelab",
  "bootstrapHost": "172.16.10.4",
  "topology": "single_node",
  "networkMode": "wireguard",
  "clusterRegion": "homelab",
  "jumphost": "linux@jump.otthol.hu:30200",
  "edge": { "ingressId": "homelab-pf" },
  "nodes": [
    {"address": "172.16.10.4", "hostname": "virt03", "role": "control"}
  ]
}
EOF
run "$UMKIM" site inventory files put "$SITE" --path lab-cli-bm0.yml --file "$FIXTURES/lab-cli-bm0.yml"
run "$UMKIM" site inventory files put "$SITE" --path group_vars/lab-cli-bm0.yml --file "$FIXTURES/group_vars_lab-cli-bm0.yml"
sync_inventory
run "$UMKIM" site host cluster-apply "$SITE" virt03 || true
run "$UMKIM" cluster create -f "$FIXTURES/cluster-lab-cli-bm0.json" || true
log "Phase 5: BM cluster lab-cli-bm0 on virt03 (bootstrap when OS ready)"

# Cleanup inventory entries for test VMs
remove_guests cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3 || true
sync_inventory

log "=== verification ==="
VERIFY_FAIL=0
for c in lab-cli-sn0 lab-cli-ha0 lab-cli-bm0; do
  if "$UMKIM" cluster get "$c" -o json >>"$LOG" 2>&1; then
    log "OK: cluster $c exists"
  else
    log "FAIL: cluster $c missing"
    VERIFY_FAIL=1
  fi
done
for h in cli-test-vm1 cli-test-sn1 cli-test-vm-a cli-test-vm-b cli-test-ha-cp1 cli-test-ha-cp2 cli-test-ha-cp3; do
  if "$UMKIM" site host get "$SITE" "$h" -o json >/dev/null 2>&1; then
    phase="$("$UMKIM" site host get "$SITE" "$h" -o json | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',{}).get('phase',''))")"
    if [[ "$phase" != "Declared" && "$phase" != "Ready" ]]; then
      log "WARN: $h still phase=$phase (expected Declared or Ready for cluster members)"
    elif [[ "$phase" == "Ready" ]]; then
      log "OK: $h Ready (cluster member or virt03 BM)"
    else
      log "OK: $h cleaned (Declared)"
    fi
  else
    log "OK: $h removed from host API"
  fi
done
if [[ "$VERIFY_FAIL" -ne 0 ]]; then
  log "VERIFICATION FAILED"
  exit 1
fi
log "=== done log=$LOG ==="
run "$UMKIM" site host list "$SITE"
run "$UMKIM" cluster list -o wide || true
