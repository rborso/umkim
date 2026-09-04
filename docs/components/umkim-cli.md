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
`umkim site create` or the console.

## Authentication

All fleet mutations go through **fleet-console-auth** (same as the console):

| Method | Use |
|--------|-----|
| `umkim login` | Interactive; session cookie stored in `~/.config/umkim/` |
| `UMKIM_TOKEN` / `--token` | `Bearer fleet_sa_…` service account (automation) |
| Hub-local dev | Direct fleet-site `:18080` with `UMKIM_DEV=1` (identity headers only; lab) |

Public URL shape (via console nginx):

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
umkim site ingress add SITE --id ID --method port_forward|reverse_proxy [--address A] [--zone Z] [--wildcard W]
umkim site ingress set SITE ID [--new-id ID] [--method …] [--address …] [--zone …] [--wildcard …]
umkim site ingress delete SITE ID

umkim cluster list | get NAME | create -f cluster.yaml
umkim cluster list --require gpu,kvm
umkim cluster candidates [--require gpu|kvm|ha_topology|storage.rwx|…] [--site SITE]
umkim cluster patch NAME [--jumphost user@host:port] [--ingress-id ID] [-f patch.json]
umkim cluster provision NAME | wait NAME [--phase Ready]
umkim cluster bootstrap NAME [--ssh-user linux]
umkim cluster delete NAME [--cleanup-kubernetes]
umkim cluster storage enable-local NAME [--dir /path] [--cluster-path NAME] [--force] [--wait 10m]
umkim cluster storage status NAME
umkim cluster platform enable-kubevirt NAME [--lan-bridge] [--bridge NAME] [--force] [--wait 45m]
umkim cluster platform status NAME

umkim chart list | get NAME
umkim chart add --origin oci://… [--version V] [--image-policy external|proxy|copy]
umkim chart add --chart package.tgz [--name N] [--version V] [--image-policy …]
umkim chart add -f chart.json
umkim chart delete NAME

umkim node list --cluster NAME
umkim node add --cluster NAME --address IP [--hostname HN]
umkim node remove --cluster NAME NODE [--force]

umkim node-op history --cluster NAME
umkim node-op logs --cluster NAME --id ID
```

`chart add` mirrors console **Apps → Library** (`POST /charts`): OCI/HTTP origin
JSON or multipart `.tgz` upload. Example:
[examples/umkim/chart-add-oci.json](../../examples/umkim/chart-add-oci.json).

`cluster create` + `bootstrap` + `provision` + `wait` mirrors
[playbooks/flows/cluster-create.yml](../../playbooks/flows/cluster-create.yml).
`node add/remove` mirrors [cluster-node-op.yml](../../playbooks/flows/cluster-node-op.yml).

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
umkim site ingress list homelab
umkim site ingress add homelab --id site-pf --method port_forward \
  --address 203.0.113.50 --zone site.example.com
umkim site ingress set homelab site-pf --description "Router WAN"
umkim site ingress delete homelab edge-rp
umkim cluster patch lab-single-0 --ingress-id site-pf
```

Mutating commands print `OK: …` on success (and re-list ingress after site changes).
Failures exit non-zero with an error (including when a patch is accepted but not
persisted — for example an outdated FleetSite CRD).

### Capability candidates (placement helper)

Filter Ready clusters by discovered capabilities (AND semantics):

```bash
umkim cluster candidates --require gpu
umkim cluster candidates --require kvm,ha_topology
umkim cluster candidates --require storage.rwx --site homelab
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
umkim cluster platform enable-kubevirt lab-ha-0
umkim cluster platform enable-kubevirt lab-ha-0 --lan-bridge --wait 45m
umkim cluster platform status lab-ha-0
umkim cluster candidates --require kubevirt
```

See [fleet-kubevirt.md](fleet-kubevirt.md).

### Single-node local RWX storage

Local RWX (`nfs_local`) is a **cluster operation** (Ansible job on the hub), same
queue as node lifecycle — it cannot run while a node operation is in progress.
Completed runs appear in operation history (`umkim node-op history`).

```bash
umkim cluster storage enable-local lab-single-0
umkim cluster storage enable-local lab-single-0 --dir /data/nfs --wait 10m
umkim cluster storage enable-local lab-single-0 --wait 10m --force
umkim cluster storage status lab-single-0
umkim node-op history --cluster lab-single-0
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

## Roadmap (post v0.2)

- `umkim cluster up` — create + bootstrap + provision + wait (single command)
- `umkim site export` / `umkim cluster export` — optional YAML backup (not git sync)
- Merge-safe site seed (install playbook does not clobber console edits)
- OCI-published `umkim` binary in Harbor or release artifacts

## Related

- [fleet-site.md](fleet-site.md) — API and FCJ lifecycle
- [fleet-console-auth.md](../fleet-console-auth.md) — roles and service accounts
- [install.md](../install.md) — hub install (Ansible)
- [playbooks/flows/README.md](../../playbooks/flows/README.md) — legacy API flow playbooks
