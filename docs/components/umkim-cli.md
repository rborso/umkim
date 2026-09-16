# UMKIM CLI (`umkim`)

> Status: v0.2 experimental. Installs from the git repo; fleet operations use the
> hub API after platform install.

## Role in the platform

| Phase | Tool | Source of truth |
|-------|------|-----------------|
| Hub install | Ansible (`docs/install.md`, `fleet-deploy.yml`) | Git `inventory/` |
| Fleet operations | `umkim` CLI | Hub CRDs (`FleetSite`, `FleetClusterJoin`, …) |
| Derived runtime | Controllers | Per-cluster Ansible inventory ConfigMaps, secrets, jobs |

After greenfield hub install, operators use **`umkim`** for sites, clusters, node
operations, and bootstrap — not git inventory or flow playbooks. Git retains
install pins, vault, and **examples** only.

## Hub inventory model

Fleet intent lives on the hub:

- **`FleetSite`** — site catalog (exposure, jumphost, domains)
- **`FleetClusterJoin`** — cluster spec (nodes, storage, edge, jumphost override)
- **Status** — phases, jobs, `nodeOperationHistory` (never exported to git by default)

Controllers derive **Ansible inventory** (`fleet-cluster-{name}-inventory` ConfigMaps)
from FCJ spec. The CLI reads/writes CRDs via the authenticated fleet API; it does not
edit generated ConfigMaps or git `group_vars`.

Site seed from `inventory/group_vars/all/fleet_sites.yml` remains a **one-time /
install** bootstrap (`fleet_site_seed_registry`); ongoing site changes go through
`umkim site create` or the UMKIM console.

## Authentication

All fleet mutations go through **fleet-console-auth** (same as the UMKIM console):

| Method | Use |
|--------|-----|
| `umkim login` | Interactive; session cookie stored in `~/.config/umkim/` |
| `UMKIM_TOKEN` / `--token` | `Bearer fleet_sa_…` service account (automation) |
| Hub-local dev | Direct fleet-site `:18080` with `UMKIM_DEV=1` (identity headers only; lab) |

Public URL shape (via UMKIM console nginx):

- Fleet API: `{base}/fleet/api/v1alpha1`
- Auth API: `{base}/fleet/auth/v1alpha1`

Configure with `UMKIM_API_URL`, `UMKIM_AUTH_URL`, or `umkim config set`.

## Commands (v0.2)

```text
umkim login | logout | whoami
umkim config show

umkim site list | get NAME | create -f site.yaml
umkim site patch NAME [--jumphost user@host:port] [--region REGION] [--failure-domain FD] [-f patch.json]
umkim site ingress list SITE
umkim site ingress add SITE --id ID --method port_forward|reverse_proxy [--address A] [--wan-address-mode fixed|dns] [--zone Z] [--wildcard W]
umkim site ingress set SITE ID [--new-id ID] [--method …] [--address …] [--wan-address-mode fixed|dns] [--zone …] [--wildcard …]
umkim site ingress delete SITE ID
umkim site host list SITE | get SITE NAME | provision SITE NAME [--confirm] [--wait]
umkim site host cluster-apply SITE NAME

umkim cluster list | get NAME | create -f cluster.yaml
umkim cluster list --require gpu,kvm
umkim cluster candidates [--require gpu|kvm|ha_topology|storage.rwx|…] [--site SITE]
umkim cluster patch NAME [--jumphost user@host:port] [--ingress-id ID] [-f patch.json]
umkim cluster provision NAME | wait NAME [--phase Ready] [--timeout 30m]
umkim cluster bootstrap NAME [--ssh-user linux]
umkim cluster delete NAME [--cleanup-kubernetes]
umkim cluster storage enable-local NAME [--dir /path] [--cluster-path NAME] [--force] [--wait 10m]
umkim cluster storage status NAME
umkim cluster platform enable-kubevirt NAME [--lan-bridge] [--bridge NAME] [--force] [--wait 45m]
umkim cluster platform status NAME

umkim vm list --cluster NAME
umkim vm get NAME --cluster NAME [-n NS]
umkim vm start|stop|restart NAME --cluster NAME [--force]
umkim vm console NAME --cluster NAME [--type serial|vnc]

umkim chart list | get NAME
umkim chart add --origin oci://… [--version V] [--image-policy external|proxy|copy]
umkim chart add --chart package.tgz [--name N] [--version V] [--image-policy …]
umkim chart add -f chart.json
umkim chart delete NAME

umkim node list --cluster NAME
umkim node add --cluster NAME --address IP [--hostname HN]
umkim node remove --cluster NAME NODE [--force]
umkim node replace --cluster NAME --old-node NODE --address IP [--force]

umkim node-op history --cluster NAME
umkim node-op logs --cluster NAME --id ID
```

`chart add` mirrors console **Apps → Library** (`POST /charts`): OCI/HTTP origin
JSON or multipart `.tgz` upload. Example:
[examples/umkim/chart-add-oci.json](../../examples/umkim/chart-add-oci.json).

`cluster create` + `bootstrap` + `provision` + `wait` mirrors
[playbooks/flows/cluster-create.yml](../../playbooks/flows/cluster-create.yml).
For **HA** (`ha_3_node`), hub finalize Jobs allow up to **90 minutes**; use
`umkim cluster wait --timeout 90m` (default `--timeout` is 30m). Bootstrap on
three nodes can take ~20 minutes before hub provision starts.
`node add/remove/replace` mirrors [cluster-node-op.yml](../../playbooks/flows/cluster-node-op.yml).

## Output

List commands default to a **table** (kubectl-style). Use `-o` / `--output`:

| Format | Description |
|--------|-------------|
| `table` | Default — name, site, phase, ready (and similar per resource) |
| `wide` | Extra columns (topology, exposure, upstream, …) |
| `json` | Full API response |

```bash
umkim cluster list
umkim cluster list -o wide
umkim cluster list -o json
```

`get`, `create`, and mutation commands still emit JSON unless noted otherwise.

### Jumphost flags

`umkim cluster bootstrap` uses `--jumphost` when set; otherwise it reads
`GET /clusters/{name}` (`jumphost` field) and falls back to the parent site's
`spec.jumphost`.

Example patch files: [examples/umkim/site-patch-jumphost.json](../../examples/umkim/site-patch-jumphost.json),
[examples/umkim/cluster-patch-jumphost.json](../../examples/umkim/cluster-patch-jumphost.json).

### Site ingress entries

Each site can define named **ingress entries** (`port_forward` / `reverse_proxy`)
under `FleetSite.spec.edge.ingress[]`. Clusters bind with
`umkim cluster patch --ingress-id`. Hub gateway (`hubGateway`) is a separate
site toggle, not an ingress row.

```bash
umkim site ingress list SITE
umkim site ingress add SITE --id site-pf --method port_forward \
  --address 203.0.113.50 --zone site.example.com
umkim site ingress set SITE site-pf --description "Router WAN"
umkim site ingress delete SITE edge-rp
umkim cluster patch CLUSTER --ingress-id site-pf
```

### Site provisioner connector

Optional metal/VM provision before enroll / add-worker. Default provider is
`noop` (caller supplies the SSH-ready host fragment).

```bash
umkim site provision get SITE
umkim site provision set SITE --provider yaidap \
  --jumphost linux@jump.example:22 \
  --ssh-secret fleet-site-SITE-provision-ssh \
  --inventory /path/to/yaidap/inventory \
  --yaidap-root /path/to/yaidap \
  --yaidap-repo git@github.com:ORG/yaidap.git \
  --yaidap-ref main \
  --yaidap-git-secret fleet-site-SITE-yaidap-git
umkim site provision yaidap-sync SITE
umkim site provision run SITE --action create_vm --subject vm01 --wait
umkim site provision run SITE --action plan_vm --subject vm01 --wait
umkim site provision run SITE --action create_vm --subject vm01 \
  --ansible-host 203.0.113.10 --wait   # noop handoff
umkim site provision status SITE ACTION_ID
umkim site provision actions SITE
```

### Connector inventory (YAIDAP YAML)

```bash
umkim site inventory get SITE
umkim site inventory files list SITE
umkim site inventory files list SITE --tree
umkim site inventory files get SITE --path guests.yml
umkim site inventory files get SITE --path group_vars/all/all.yml --missing-ok
umkim site inventory files put SITE --path host_vars/guest01.yml --file profile.yml
umkim site inventory files put SITE --path cluster-a.yml --file group-membership.yml
umkim site inventory files put SITE --path group_vars/cluster-a.yml --file cluster-intent.yml
umkim site inventory files delete SITE --path host_vars/old-guest.yml
umkim site inventory migrate SITE
umkim site inventory revisions SITE [--limit 20] [--id REV]
umkim site inventory sync SITE [--wait]
umkim site inventory import preview SITE [--wait]
umkim site inventory import apply SITE --confirm [--paths guests.yml,host_vars/foo.yml]
```

Legacy JSON guest/catalog mutations were removed in v0.2; use `files put` instead.

### Site hosts (derived view)

List and provision guests defined in inventory YAML:

```bash
umkim site host list SITE
umkim site host get SITE guest01
umkim site host cluster-apply SITE guest01
umkim site host provision SITE guest01 --action plan_vm --wait
umkim site host provision SITE guest01 --wait
umkim site host provision SITE bm01 --confirm --wait
umkim site host destroy SITE guest01 --confirm --wait
```

Destroy tears down the libvirt VM while keeping the guest in inventory; phase returns to **Declared** when the job succeeds. Recreate with provision:

```bash
umkim site host destroy homelab lab-ha-0-cp1 --confirm --wait
umkim site host list homelab   # phase: Declared
umkim site host provision homelab lab-ha-0-cp1 --wait   # recreate VM
```

Add guests and cluster intent via `umkim site inventory files put` (`guests.yml`, `hypervisors.yml`, `group_vars/all/all.yml`, `host_vars/*.yml`, root `{cluster}.yml`, and `group_vars/{cluster}.yml`). Put `ansible_host` in both `guests.yml` and `host_vars/{guest}.yml` when the guest needs a static address. Use `vm_location: least-utilized` (or `auto`) only before first placement; after a successful `create_vm`, the hub rewrites inventory to the resolved hypervisor and auto-syncs to the jumphost — `umkim site host get` then shows the concrete name (for example `hv01`). Reset to `least-utilized` explicitly to re-run placement.

`install_bm` / `destroy` / **`discover_pxe`** require `--confirm`. **`install_bm`**
PXE-reinstalls the OS via TempXE; allowed on `bare_metal` (`bare_metals.yml`)
and `hypervisor` (`hypervisors.yml`) hosts. Example:

```bash
umkim site host provision SITE hv01 --action install_bm --confirm --wait
```

**Inventory roles** — VM guests: `guests.hosts`; bare-metal cluster nodes:
`bare_metals.yml`; hypervisors: `hypervisors.yml`. Console: *Add VM guest*,
*Add bare metal node*, *Add hypervisor*.

**Hardware discovery** — add a hypervisor in `hypervisors.yml` +
`host_vars/{name}.yml`, sync, discover, then import promoted drafts:

```bash
umkim site inventory sync SITE --wait
umkim site host provision SITE new-hv01 --action discover --wait
# or PXE (TempXE; requires tempxe_discovery.enabled + MAC in host_vars):
umkim site host provision SITE new-hv01 --action discover_pxe --confirm --wait
umkim site inventory import preview SITE --wait
umkim site inventory import apply SITE --confirm --paths host_vars/new-hv01.yml,hypervisors.yml
```

Import preview promotes jumphost `generated/host_vars/` and `generated/hosts/`
into canonical hub paths; `notes.json` stays on the jumphost for reference.

See [fleet-site.md](fleet-site.md) for the FleetSite API and
[yaidap-integration.md](yaidap-integration.md) for YAIDAP workflows, inventory
layout, and action mapping.

Mutating commands print `OK: …` on success (and re-list ingress after site changes).
Failures exit non-zero with an error (including when a patch is accepted but not
persisted — for example an outdated FleetSite CRD).

### Capability candidates (placement helper)

Filter Ready clusters by discovered capabilities (AND semantics):

```bash
umkim cluster candidates --require gpu
umkim cluster candidates --require kvm,ha_topology
umkim cluster candidates --require storage.rwx --site SITE
umkim cluster list --require gpu,kvm
```

Supported `--require` tokens: `gpu`, `kvm`, `kubevirt` (alias `kv`),
`network.multus_l2` (aliases `multus`, `lan_l2`), `ha_topology` (alias `ha`),
`storage.rwx` (alias `rwx`), `storage.rwo` (alias `rwo`). This only filters;
you still pick `FleetApp.spec.clusterName` explicitly.

### KubeVirt platform

KubeVirt is a **cluster operation** (Ansible job), mutual exclusion with node and
storage ops. History type `configureKubevirt`.

```bash
umkim cluster platform enable-kubevirt CLUSTER
umkim cluster platform enable-kubevirt CLUSTER --lan-bridge --wait 45m
# Day-2 LAN Multus when KubeVirt is already Ready (no --force required)
umkim cluster platform enable-kubevirt CLUSTER --lan-bridge
umkim cluster platform status CLUSTER
umkim cluster candidates --require kubevirt
```

### Virtual machines

Once KubeVirt is installed, manage guest VMs through the hub API:

```bash
umkim vm list --cluster CLUSTER
umkim vm stop smoke --cluster CLUSTER           # soft ACPI shutdown
umkim vm stop smoke --cluster CLUSTER --force   # hard poweroff
umkim vm start smoke --cluster CLUSTER
umkim vm restart smoke --cluster CLUSTER        # soft reboot
umkim vm restart smoke --cluster CLUSTER --force
umkim vm console smoke --cluster CLUSTER        # serial TTY
```

Console UI: **Clusters → Virtual machines**. Serial is default; VNC opens on
demand. See [fleet-kubevirt.md](fleet-kubevirt.md).

### Single-node local RWX storage

Local RWX (`nfs_local`) is a **cluster operation** (Ansible job on the hub), same
queue as node lifecycle — it cannot run while a node operation is in progress.
Completed runs appear in operation history (`umkim node-op history`).

```bash
umkim cluster storage enable-local CLUSTER
umkim cluster storage enable-local CLUSTER --dir /data/nfs --wait 10m
umkim cluster storage enable-local CLUSTER --wait 10m --force
umkim cluster storage status CLUSTER
umkim node-op history --cluster CLUSTER
```

Installs `nfs-server` on the node (`nfs_local`), exports `--dir` (default
`/srv/fleet-nfs`) with a per-cluster subdirectory, and deploys StorageClass
`nfs-client`. Single-node only; see
[fleet-cluster-storage.md](fleet-cluster-storage.md).

## Install

```bash
cd cmd/umkim && go build -o umkim .
# or: go install ./cmd/umkim@...
```

Set API URL (example):

```bash
export UMKIM_API_URL=https://<hub-host>/fleet/api/v1alpha1
export UMKIM_AUTH_URL=https://<hub-host>/fleet/auth/v1alpha1
umkim login
```

## Testing

| Layer | Location |
|-------|----------|
| Unit | `cmd/umkim/internal/.../*_test.go` — client, config, bootstrap SSH args |
| Integration | `go test ./...` with `httptest` mock auth + fleet API |
| Lab smoke | `scripts/verify-umkim-cli.sh` — login, list clusters, node-op history |
| Maintainer homelab rebuild | [`examples/homelab/rebuild-homelab-cli.md`](../../examples/homelab/rebuild-homelab-cli.md) — full CLI runbook with timing logs (site + BM + HA clusters) |

## Roadmap (post v0.2)

- `umkim cluster up` — create + bootstrap + provision + wait (single command)
- `umkim site export` / `umkim cluster export` — optional YAML backup (not git sync)
- Merge-safe site seed (install playbook does not clobber console edits)
- OCI-published `umkim` binary in Harbor or release artifacts

## Related

- [fleet-site.md](fleet-site.md) — API and FCJ lifecycle
- [fleet-console-auth.md](../fleet-console-auth.md) — roles and service accounts
- [install.md](../install.md) — hub install (Ansible)
- [examples/homelab/rebuild-homelab-cli.md](../../examples/homelab/rebuild-homelab-cli.md) — maintainer lab rebuild via CLI (inventory in `examples/homelab/`)
- [playbooks/flows/README.md](../../playbooks/flows/README.md) — legacy API flow playbooks
