# Homelab CLI rebuild runbook (otthol.hu maintainer lab)

Full fresh install of site **`homelab`**, cluster **`lab-single-0`** (BM on virt03), and
**`lab-ha-0`** (HA VMs on virt01/virt02) using **`umkim` CLI only**. Canonical values live in
[`otthol-lab-reference.yml`](otthol-lab-reference.yml).

**Out of scope:** `lab-jumphost` site, `lab-single-1` cluster.

## Prerequisites

- Hub installed and verified: `./scripts/verify-hub-install.sh` (management site only)
- `umkim` on PATH; session via `umkim login` or `UMKIM_TOKEN`
- YAIDAP checkout on jumphost at `/home/linux/yaidap` (override with `YAIDAP_ROOT`)
- Hub secret `fleet-site-homelab-provision-ssh` in namespace `fleet-console` (jumphost SSH key)
- SSH from jumphost to lab LAN (`172.16.10.0/24`)
- **virt03** (`172.16.10.4`) powered and reachable from jumphost before BM discover/install_bm
- Base image **`/opt/local/rocky-10.qcow2`** on virt01/virt02 (YAIDAP `os: rocky-10` on guests)
- Inventory **`group_vars/all/all.yml`** includes `alphabet`, `local_ssd` storage, and `networks` (see `examples/homelab/inventory/`)
- Hypervisor **`host_vars/virt01.yml`** and **`virt02.yml`** with `os: rocky` (virt03 uses `vm_scheduling_pool: false` until BM phase adds `hypervisors-full.yml`)

Optional API URLs (defaults target console NodePort on hub):

```bash
export UMKIM_API_URL=http://127.0.0.1:30912/fleet/api/v1alpha1
export UMKIM_AUTH_URL=http://127.0.0.1:30912/fleet/auth/v1alpha1
```

Create provision SSH secret if missing (on hub; use keys that can reach the jumphost):

```bash
kubectl -n fleet-console create secret generic fleet-site-homelab-provision-ssh \
  --from-file=id_ed25519="${HOME}/.ssh/id_ed25519" \
  --from-file=id_ed25519.pub="${HOME}/.ssh/id_ed25519.pub" \
  --dry-run=client -o yaml | kubectl apply -f -
```

(RSA keys named `id_rsa` / `id_rsa.pub` also work when mounted under `/root/.ssh` in ansible-runner jobs.)

## Automated run (recommended)

```bash
cd ~/umkim
./scripts/lab/rebuild-homelab-cli.sh
```

Environment:

| Variable | Default | Purpose |
|----------|---------|---------|
| `DRY_RUN=1` | off | Print steps only |
| `SKIP_PHASE` | — | Comma list: `virsh`, `site`, `bm`, `single`, `ha` |
| `FLEET_JUMPHOST` | `linux@jump.otthol.hu:30200` | Site / bootstrap jump |
| `YAIDAP_ROOT` | `/home/linux/yaidap` | YAIDAP tree on jumphost |
| `YAIDAP_INVENTORY` | `/home/linux/yaidap/inventory/homelab` | Sync target on jumphost |

Logs:

- Full log: `logs/homelab-rebuild-<timestamp>.log`
- Summary TSV: `logs/homelab-rebuild-<timestamp>.summary.tsv`

```bash
grep -E 'START|END' logs/homelab-rebuild-*.log
column -t logs/homelab-rebuild-*.summary.tsv
```

Expected long steps: `virt03 install_bm`, HA `create_vm` ×4, HA `wait Ready` (up to 90m).

---

## Manual steps (same order as script)

### Phase 0 — Preflight

```bash
umkim whoami
umkim cluster delete lab-single-0 --cleanup-kubernetes || true
umkim cluster delete lab-ha-0 --cleanup-kubernetes || true
```

### Phase 1 — Ad-hoc virsh cleanup (virt01 + virt02 only)

**Do not** run on virt03 (`172.16.10.4`); it is reinstalled via TempXE later.

```bash
JUMP="linux@jump.otthol.hu:30200"
for hv in 172.16.10.2 172.16.10.3; do
  ssh -J "$JUMP" "linux@${hv}" 'bash -s' <<'EOF'
set -euo pipefail
virsh list --all --name | while read -r d; do
  [[ -z "$d" ]] && continue
  virsh destroy "$d" 2>/dev/null || true
  virsh undefine "$d" --remove-all-storage 2>/dev/null || true
done
virsh list --all
EOF
done
```

### Phase 2 — Site + YAIDAP inventory

```bash
umkim site create -f examples/homelab/site-homelab.json
umkim site provision set homelab --provider yaidap \
  --jumphost linux@jump.otthol.hu:30200 \
  --ssh-secret fleet-site-homelab-provision-ssh \
  --inventory /home/linux/yaidap/inventory/homelab \
  --yaidap-root /home/linux/yaidap

umkim site inventory migrate homelab
umkim site inventory files put homelab --path hypervisors.yml --file examples/homelab/inventory/hypervisors.yml
umkim site inventory files put homelab --path guests.yml --file examples/homelab/inventory/guests.yml
umkim site inventory files put homelab --path group_vars/all/all.yml --file examples/homelab/inventory/group_vars/all/all.yml
umkim site inventory files put homelab --path lab-ha-0.yml --file examples/homelab/inventory/lab-ha-0.yml
umkim site inventory files put homelab --path group_vars/lab-ha-0.yml --file examples/homelab/inventory/group_vars/lab-ha-0.yml
for f in lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1 virt01 virt02 virt03; do
  umkim site inventory files put homelab --path "host_vars/${f}.yml" \
    --file "examples/homelab/inventory/host_vars/${f}.yml"
done
umkim site inventory sync homelab --wait
```

### Phase 3 — BM virt03: discover → reinstall → add

| Step | Command |
|------|---------|
| Discover | `umkim site host provision homelab virt03 --action discover --wait` |
| Promote | `umkim site inventory import preview homelab --wait` |
| Apply | `umkim site inventory import apply homelab --confirm --paths host_vars/virt03.yml,hypervisors.yml` |
| Reinstall | `umkim site host provision homelab virt03 --action install_bm --confirm --wait` |

If virt03 is unreachable over SSH, enable `tempxe_discovery` + MAC in
`examples/homelab/inventory/host_vars/virt03.yml` and use
`--action discover_pxe --confirm` instead of `discover`.

**Add cluster** (virt03 is not a YAIDAP guest):

```bash
umkim cluster create -f examples/homelab/cluster-lab-single-0.json
umkim cluster wait lab-single-0 --phase ScriptIssued --timeout 10m
umkim cluster bootstrap lab-single-0 --ssh-user linux
umkim cluster wait lab-single-0 --phase Validated --timeout 45m
umkim cluster provision lab-single-0
umkim cluster wait lab-single-0 --phase Ready --timeout 45m
```

### Phase 4 — lab-ha-0 (VM guests)

```bash
for h in lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1; do
  umkim site host provision homelab "$h" --wait
  umkim site host cluster-apply homelab "$h" || true
done
umkim cluster create -f examples/homelab/cluster-lab-ha-0.json
umkim cluster wait lab-ha-0 --phase ScriptIssued --timeout 10m
umkim cluster bootstrap lab-ha-0 --ssh-user linux
umkim cluster wait lab-ha-0 --phase Validated --timeout 45m
umkim cluster provision lab-ha-0
umkim cluster wait lab-ha-0 --phase Ready --timeout 90m
umkim cluster patch lab-ha-0 --ingress-id edge-rp
```

HA bootstrap may require seeding SSH keys to all control-plane nodes before
`bootstrap` (see `scripts/lab/rebuild-homelab-cli.sh`).

### Phase 5 — Verify

```bash
umkim site list
umkim cluster list -o wide
umkim cluster get lab-single-0
umkim cluster get lab-ha-0
```

On hub: `kubectl get managedclusters`, confirm `lab-single-0` and `lab-ha-0` Accepted/Available.

## Edge bindings

| Cluster | Ingress ID | Method | TLS |
|---------|------------|--------|-----|
| lab-single-0 | homelab-pf | port_forward | acme_cluster (auto from ingress) |
| lab-ha-0 | edge-rp | reverse_proxy | external (auto from ingress) |

Set at cluster create via `edge.ingressId` in JSON under `examples/homelab/`.

## Related files

- Reference: [`otthol-lab-reference.yml`](otthol-lab-reference.yml)
- Site JSON: [`site-homelab.json`](site-homelab.json)
- Cluster JSON: [`cluster-lab-single-0.json`](cluster-lab-single-0.json), [`cluster-lab-ha-0.json`](cluster-lab-ha-0.json)
- Inventory: [`inventory/`](inventory/)
- Script: [`../../scripts/lab/rebuild-homelab-cli.sh`](../../scripts/lab/rebuild-homelab-cli.sh)
