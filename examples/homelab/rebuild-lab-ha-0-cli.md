# lab-ha-0 rebuild runbook (umkim CLI)

Full teardown and greenfield rebuild of cluster **`lab-ha-0`** on site **`homelab`**
using **`umkim` CLI** only (plus the fleet API for FleetApp installs — see
[Demo apps](#demo-apps-fleet-api)).

Canonical values: [`otthol-lab-reference.yml`](otthol-lab-reference.yml),
[`cluster-lab-ha-0.json`](cluster-lab-ha-0.json).

**Run log (2026-09-16):** [`../../logs/lab-ha-0-rebuild-20260916-171211.log`](../../logs/lab-ha-0-rebuild-20260916-171211.log)

## Prerequisites

```bash
export UMKIM_DEV=1   # hub-local fleet-site API on :18080
# or: umkim login + UMKIM_API_URL / UMKIM_AUTH_URL for console NodePort
```

- Hub and site **`homelab`** already provisioned (see
  [`rebuild-homelab-cli.md`](rebuild-homelab-cli.md) Phases 2–3).
- Jumphost: `linux@jump.otthol.hu:30200`
- Guests: `lab-ha-0-cp1`–`cp3`, `lab-ha-0-w1` in YAIDAP inventory
  (`guests.yml`, `host_vars/`).
- **`lab-single-0`** left running during this rebuild.

## Summary (actual run 2026-09-16)

| Step | Duration (approx.) | Result |
|------|-------------------|--------|
| Delete cluster + K8s cleanup | ~75 min (stuck MC; see notes) | FCJ removed |
| Destroy VMs + hypervisor wipe | ~15 min | QEMU + disks gone |
| Create cluster + 4 VMs | ~5 min | All guests `Ready` |
| Bootstrap | ~38 min (Galaxy 502 retries) | `Validated` |
| Hub provision | ~9 min | `Ready` |
| KubeVirt + LAN bridge | ~12 min | Platform `Ready` |
| Demo FleetApps | ~2 min | whoami-ha, petclinic-ha, rocky-lan-ha |

---

## Phase 0 — Disable autoApply (important)

`autoApply: true` in `group_vars/lab-ha-0.yml` recreates a `FleetClusterJoin`
as soon as the old one is deleted. Disable it before teardown:

```bash
umkim site inventory files put homelab \
  --path group_vars/lab-ha-0.yml \
  --file examples/homelab/inventory/group_vars/lab-ha-0.yml

# patch autoApply: false in the synced file (or edit locally first), then:
umkim site inventory sync homelab --wait
```

Re-enable `autoApply: true` at the end (restore from
`examples/homelab/inventory/group_vars/lab-ha-0.yml`).

## Phase 1 — Delete cluster

Delete FleetApps first if uninstall jobs will block `ManagedCluster` removal:

```bash
# Fleet API (no umkim app subcommand yet)
API=http://127.0.0.1:18080/api/v1alpha1
HDR=(-H 'Content-Type: application/json' \
     -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-flow' \
     -H 'X-Fleet-DC-Admin: true')
curl "${HDR[@]}" -X DELETE "$API/apps/petclinic-ha"
curl "${HDR[@]}" -X DELETE "$API/apps/rocky-lan-ha"
```

```bash
umkim cluster delete lab-ha-0 --cleanup-kubernetes
# poll until gone:
while umkim cluster get lab-ha-0 -o json >/dev/null 2>&1; do sleep 10; done
```

**What we hit:** delete hung ~1h while `ManagedCluster` stayed in `Terminating`
(stale FleetApps + missing spoke kubeconfig for uninstall jobs). Workaround used:

1. `cleanup_node_local.sh --mode reset` on all nodes (via jumphost SSH).
2. Remove `ManagedCluster` finalizers on the hub.
3. Second `umkim cluster delete lab-ha-0` (without `--cleanup-kubernetes`) after
   nodes were already reset.

## Phase 2 — Destroy VMs (QEMU + disks)

After the FCJ is gone:

```bash
for h in lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1; do
  umkim site host provision homelab "$h" --action destroy --confirm --wait
done
```

**Commands:**

```bash
umkim site host provision homelab <guest> --action destroy --confirm --wait
```

(`umkim site host destroy` exists in source but is not in the installed binary
here; use `provision --action destroy`.)

**What we hit:**

- `destroy` is blocked while a guest is a cluster member → delete FCJ first.
- After partial destroys, guest phase became `Declared` while disks still existed
  on virt01/virt02 → `destroy` rejected (`requires guest phase Ready or Failed`).

**Hypervisor cleanup (when umkim destroy cannot run):**

```bash
JUMP="linux@jump.otthol.hu:30200"
for hv in 172.16.10.2 172.16.10.3; do
  ssh -J "$JUMP" "linux@${hv}" 'sudo bash -s' <<'EOF'
set -euo pipefail
for d in lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1; do
  virsh destroy "$d" 2>/dev/null || true
  virsh undefine "$d" --remove-all-storage 2>/dev/null || true
done
rm -rf /opt/local/lab-ha-0-*
EOF
done
```

Verify: `virsh list --all` shows no `lab-ha-0-*`; `ls /opt/local/lab-ha-0*` → none.

## Phase 3 — Recreate VMs

Ensure inventory includes worker `lab-ha-0-w1`:

```bash
umkim site inventory files put homelab --path guests.yml \
  --file examples/homelab/inventory/guests.yml
umkim site inventory files put homelab --path host_vars/lab-ha-0-w1.yml \
  --file examples/homelab/inventory/host_vars/lab-ha-0-w1.yml
umkim site inventory sync homelab --wait
```

Create cluster intent, then VMs **one at a time** (YAIDAP site lock):

```bash
umkim cluster create -f examples/homelab/cluster-lab-ha-0.json
umkim cluster wait lab-ha-0 --phase ScriptIssued --timeout 10m

for h in lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1; do
  umkim site host provision homelab "$h" --wait
done
umkim site host list homelab   # all guests Ready
```

## Phase 4 — Bootstrap and hub provision

```bash
# wait for SSH on first CP (cloud-init)
umkim cluster bootstrap lab-ha-0 \
  --bootstrap-host 172.16.10.101 \
  --ssh-user linux

umkim cluster wait lab-ha-0 --phase Validated --timeout 45m
umkim cluster provision lab-ha-0
umkim cluster wait lab-ha-0 --phase Ready --timeout 90m
```

**Bootstrap peer seeding:** `umkim cluster bootstrap` seeds SSH keys to cp2/cp3/w1
and runs the updated bundle (`inventory_mgmt_peer_ips`, bridge on all peers).

**Galaxy 502:** If `ansible-galaxy collection install` fails during bootstrap,
pre-install on cp1 and retry:

```bash
ssh -J linux@jump.otthol.hu:30200 linux@172.16.10.101 \
  'sudo dnf install -y ansible-core && sudo ansible-galaxy collection install ansible.posix community.general ansible.utils'
umkim cluster bootstrap lab-ha-0 --bootstrap-host 172.16.10.101 --ssh-user linux
```

## Phase 5 — KubeVirt + LAN Multus

Day-2 after cluster `Ready`:

```bash
umkim cluster platform enable-kubevirt lab-ha-0 --lan-bridge --wait 45m
umkim cluster platform status lab-ha-0
```

Expect: `phase=Ready install=true lanBridge=true`, NAD `default/mgmt-l2`,
Calico `IP_AUTODETECTION_METHOD=interface=n0p0_mgmt_nw`.

## Demo apps (fleet API)

`umkim` has no `app install` subcommand yet. Use the same fleet API as the
console (`POST /api/v1alpha1/apps`). `lab-ha-0` uses **`route.mode: hub`**
(`*.demo.otthol.hu` via hub gateway); KubeVirt guest uses `route.mode: none`.

```bash
API=http://127.0.0.1:18080/api/v1alpha1
HDR=(-H 'Content-Type: application/json' \
     -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-flow' \
     -H 'X-Fleet-DC-Admin: true')

# whoami (hub route)
curl "${HDR[@]}" -X POST "$API/apps" -d '{
  "name":"whoami-ha","clusterName":"lab-ha-0","chart":"whoami","version":"0.1.0",
  "releaseName":"whoami-ha","namespace":"default",
  "route":{"mode":"hub","hubHost":"whoami-ha.demo.otthol.hu"}
}'

# petclinic (hub route)
curl "${HDR[@]}" -X POST "$API/apps" -d '{
  "name":"petclinic-ha","clusterName":"lab-ha-0","chart":"pet-clinic-helm","version":"0.1.1",
  "releaseName":"petclinic-ha","namespace":"default",
  "route":{"mode":"hub","hubHost":"petclinic-ha.demo.otthol.hu"}
}'

# Rocky 10 KubeVirt VM on mgmt-l2 (no route)
curl "${HDR[@]}" -X POST "$API/apps" -d "$(jq -n \
  --arg yaml "cpu: 2\nmemory: 2Gi\ndisk:\n  size: 20Gi\n  storageClass: longhorn\n  volumeMode: Filesystem\nnetwork:\n  mode: lan\n  multusNetwork: default/mgmt-l2" \
  '{name:"rocky-lan-ha",clusterName:"lab-ha-0",chart:"rocky-10-vm",version:"0.1.1",
    releaseName:"rocky-lan-ha",namespace:"default",valuesYaml:$yaml,route:{mode:"none"}}')"
```

Poll: `curl "${HDR[@]}" "$API/apps/whoami-ha" | jq .phase` → `Ready`.

## Phase 6 — Verify

```bash
umkim cluster list -o wide
umkim cluster get lab-ha-0
umkim cluster platform status lab-ha-0

# on cluster VIP via jumphost:
ssh -J linux@jump.otthol.hu:30200 linux@172.16.10.100 \
  'sudo kubectl get nodes; sudo kubectl get sc; sudo kubectl get net-attach-def -A'
```

**2026-09-16 results:**

- 4 nodes Ready (3 CP + 1 worker)
- Longhorn default SC
- Calico `interface=n0p0_mgmt_nw` on all nodes
- NAD `default/mgmt-l2`
- FleetApps: `whoami-ha`, `petclinic-ha`, `rocky-lan-ha` → `Ready`

Re-enable inventory autoApply:

```bash
umkim site inventory files put homelab \
  --path group_vars/lab-ha-0.yml \
  --file examples/homelab/inventory/group_vars/lab-ha-0.yml
umkim site inventory sync homelab --wait
```

## Related

- Full homelab rebuild: [`rebuild-homelab-cli.md`](rebuild-homelab-cli.md)
- KubeVirt / bridge behavior: [`../../docs/components/fleet-kubevirt.md`](../../docs/components/fleet-kubevirt.md)
- FleetApp routing: [`../../docs/components/fleet-apps.md`](../../docs/components/fleet-apps.md)
