# Fleet site controller

> Status: v0.2 is an experimental proof of concept. FleetClusterJoin and
> its optional integrations require independent operational validation.

The fleet-site controller manages `FleetSite`, `FleetClusterJoin`,
`FleetApp`, and `FleetChart` resources and coordinates greenfield workload-cluster bootstrap
from the hub.

## Lifecycle

An FCJ normally progresses through:

```text
Pending → ScriptIssued → Enrolled → Validated → Provisioning → Ready
```

Failures are reported in resource status. Review the resource, controller Job,
and cluster state before retrying; do not assume a partial provisioning run is
safe to repeat without inspection.

Cluster deletion **defaults to detach**: hub-managed resources are removed
while K3s and workloads on the spoke are preserved. Full Kubernetes cleanup
is opt-in (`cleanupKubernetes=true` on delete, or
`fleet_flow_cleanup_kubernetes=true` in
[playbooks/flows/cluster-delete.yml](../../playbooks/flows/cluster-delete.yml)).

## Site ingress

Each `FleetSite` carries `spec.edge.ingress[]` (port-forward and/or
reverse-proxy entries) and optional `hubGateway` for hub-routed DNS. Clusters
bind with `FleetClusterJoin.spec.edge.ingressId`. When a site has exactly one
`port_forward` ingress, an empty `ingressId` auto-binds at reconcile time.
Inventory examples: [../../examples/site-profiles/](../../examples/site-profiles/).

Port-forward rows support optional **`wanAddressMode`** (`fixed` or `dns`) for
WAN/DNS validation during spoke ACME. Pair `dns` mode with
`spec.edge.wanEgress` when the public address is tracked by DDNS rather than
stored on the ingress row. Patching site ingress via the console, CLI, or
`PATCH /sites/{name}` refreshes bound clusters in the `fleet-edge-clusters`
ConfigMap (`edge_wan_ip`, `wan_address_mode`, and related fields) without
re-provisioning the cluster.

Match per-app `FleetApp` `route.mode` to cluster ingress:

- `hub` with site `hubGateway` (and hub zone DNS)
- `site` with `port_forward` or `reverse_proxy` ingress binding
- `none` for chart-owned or internal ingress

See [fleet-edge.md](fleet-edge.md).

## Optional provisioner connector

`FleetSite.spec.provision` binds an optional metal/VM provisioner
(`noop` default, `yaidap`). The hub starts
async connector actions; YAIDAP runs on the site jumphost using that site's
inventory path. On success the action returns a handoff host fragment
(`ansible_host`, …) for enroll / add-worker.

| API | Purpose |
|-----|---------|
| `GET` / `PUT` / `PATCH /sites/{name}/provision` | Connector binding |
| `GET /sites/{name}/provision/inventory` | Meta, file index, legacy read-only guest/catalog projection |
| `GET/PUT/DELETE /sites/{name}/provision/inventory/files/{path}` | Native Ansible/YAIDAP YAML tree (see layout below) |
| `GET /sites/{name}/provision/inventory/hosts` | Derived host rows (`guests.yml`, `hypervisors.yml`, `_meta/status.json`) |
| `GET /sites/{name}/provision/inventory/clusters` | Aggregated cluster intent from root `{cluster}.yml` + `group_vars/{cluster}.yml` |
| `POST /sites/{name}/provision/inventory/migrate` | One-shot legacy JSON / site-hosts → YAML migration |
| `POST/GET /sites/{name}/provision/inventory/import/preview` | Pull jumphost tree; per-file diff (no write) |
| `POST /sites/{name}/provision/inventory/import/apply` | Merge selected preview paths into hub store |
| `GET /sites/{name}/provision/inventory/revisions[/{id}]` | Timestamped revision log + diff |
| `POST /sites/{name}/provision/inventory/sync` | Push synced YAML files to jumphost |
| `POST /sites/{name}/provision/yaidap/sync` | Clone or update YAIDAP git checkout on jumphost (`yaidapRepo`, `yaidapRoot`, optional deploy-key Secret) |
| `POST /sites/{name}/provision/actions` | `install_bm`, `create_vm`, `plan_vm`, `destroy`, `discover`, `discover_pxe`, `status` |
| `GET /sites/{name}/provision/actions[/{id}]` | History / poll |
| `GET /sites/{name}/hosts` | Derived host list (same source as `/provision/inventory/hosts`) |
| `POST/PATCH/DELETE /sites/{name}/hosts/{host}` | **Removed in v0.2** — use inventory file API (`PUT/PATCH …/provision/inventory/files/…`) |
| `POST /sites/{name}/hosts/{host}/provision` | Provision by guest name |
| `GET/POST /sites/{name}/hosts/{host}/cluster` | Cluster status / manual apply from cluster group files |

Hub inventory is stored in ConfigMap `fleet-site-{name}-yaidap-inventory`, where
`{name}` is the FleetSite `metadata.name`, as `file.*` YAML keys plus hub-only
`_meta/*` (`meta.json`, `status.json`, `revisions.json`). `spec.siteId` is a
lookup alias for that site and the id stamped on cluster joins. It does not
select a different inventory ConfigMap.

**Native Ansible/YAIDAP tree**

| Path | Synced to jumphost |
|------|-------------------|
| `guests.yml` | Yes — `guests.hosts` only (VM index: `ansible_host`, `vm_location`); synced to jumphost with `external.hosts` composed from `bare_metals.yml` |
| `bare_metals.yml` | **Hub only** — bare-metal cluster nodes (`bare_metals.hosts`); composed into jumphost `guests.yml` → `external.hosts` on sync |
| `hypervisors.yml` | Yes — YAIDAP hypervisor group (VM placement targets) |
| `{cluster}.yml` | Yes — Ansible group membership (hosts under the group key) |
| `group_vars/all/all.yml` (+ optional `group_vars/all/*.yml`) | Yes — site storages, networks, defaults |
| `group_vars/{group}.yml` | Yes — group-level vars; Fleet cluster intent for cluster groups |
| `host_vars/{guest}.yml` | Yes — per-guest YAIDAP profile |
| `_meta/*` | **No** — runtime sync phase, status, revisions |

Sync pushes only synced paths; jumphost-only files (`controllers.yml`, `routers.yml`, `generated/`) are preserved. Run `POST …/provision/inventory/migrate` to seed the native skeleton, split legacy `clusters.yml` / `clusters/*.yml` into root + `group_vars/`, and convert deprecated `catalog.yml` into `group_vars/all/all.yml`.

Provision status and cluster-apply progress are recorded in `_meta/status.json` keyed by guest name.

**Site host kinds** (derived from inventory layout; exposed on `GET /sites/{name}/hosts`):

| Kind | Inventory source | Role |
|------|------------------|------|
| `vm` | `guests.yml` → `guests.hosts` | Libvirt workloads (`create_vm`, `destroy`, `plan_vm`) |
| `bare_metal` | `bare_metals.yml` | Cluster members that are not libvirt guests (`install_bm`, `discover`, `discover_pxe`) |
| `hypervisor` | `hypervisors.yml` | Placement / discovery / TempXE targets (`discover`, `discover_pxe`; optional `install_bm`) |

**Site host phases** (per-guest `status.phase` in `_meta/status.json`):

| Phase | Meaning |
|-------|---------|
| `Declared` | Guest in inventory; no libvirt VM running (initial state or after successful `destroy`) |
| `Provisioning` | Connector action in progress (`create_vm`, `destroy`, …) |
| `Ready` | VM provisioned and reachable (`ansible_host` populated) |
| `Failed` | Last provision action failed |

VM lifecycle: `Declared` → `create_vm` → `Provisioning` → `Ready` → `destroy` (confirm) → `Provisioning` → `Declared`. Destroy tears down the libvirt VM only; `guests.yml` and `host_vars` are unchanged so **Provision** can recreate the VM. Destroy requires guest phase **Ready** or **Failed**, VM kind, and no active `FleetClusterJoin` for a cluster the guest belongs to (delete or unjoin the fleet cluster first). The console **Destroy VM** action on the site hosts panel wraps the same API as `POST …/hosts/{guest}/provision` with `action: destroy`.

Optional **cluster membership and intent** use the native Ansible layout:

- **`{cluster}.yml`** (inventory root) — group with `hosts:` listing member guest names.
- **`group_vars/{cluster}.yml`** — Fleet `ClusterSpec` fields (`topology`, `autoApply`, `controlPlaneVip`, `haGroup`, `mode`, …). Members are taken from the root group file, not duplicated here.

| Field (in `group_vars/{cluster}.yml`) | Purpose |
|-------|---------|
| `topology` | `single_node` or `ha_3_node` |
| `autoApply` | Reconciler creates FCJ when guests are Ready |
| `controlPlaneVip` / `haGroup` | HA enroll |

Status is mirrored in `_meta/status.json` under each guest's `cluster` field.
The registry-site reconciler polls running provision jobs and applies cluster
intent from root + `group_vars/` cluster files; use `POST …/cluster` to retry manually.

**Inventory → cluster flow (bundle mode):**

1. Edit `guests.yml`, `hypervisors.yml`, `group_vars/all/all.yml`, `host_vars/{guest}.yml`, `{cluster}.yml`, and `group_vars/{cluster}.yml` (console, CLI, or API).
2. `POST …/provision/inventory/sync` — push synced YAML to jumphost.
3. `POST …/hosts/{guest}/provision` — YAIDAP `create_vm`; hub marks guest status in `_meta/status.json`.
   After successful placement, the controller rewrites `guests.yml` and
   `host_vars/{guest}.yml` from schedule tokens (`least-utilized`, `auto`, or
   empty) to the concrete hypervisor, marks inventory `dirty`, and enqueues an
   inventory sync so the jumphost matches the hub.
4. Reconciler **cluster-apply** — creates `FleetClusterJoin` from cluster group files when members are Ready.
5. Download bootstrap bundle; run `bootstrap.sh` on first control (via jumphost).
6. `POST /clusters/{name}/provision` — hub finalize (OCM, edge, providers).

VM hosts created through the connector use **`minRootDiskGi: 38`** on the FCJ
(40G vdisk root LV is smaller than 40 GiB after EFI/boot). Bare-metal enroll
defaults remain 40 GiB.

In bundle mode, hub provision accepts **`Enrolled`** phase (bootstrap enrolls
before the progress POST marks `Validated`) so `cluster provision` is not
blocked when local validate/provision-local already ran on the node.

Destructive actions require `confirm=true`. **`install_bm`** runs a composite
TempXE flow on the jumphost: arm kickstart with hub-injected **`-e install_os=true`**
(inventory should keep **`install_os: false`**), start PXE, UEFI network boot on
the target host, wait for kickstart completion (non-empty `/root/ks-post.log` +
new `boot_id`), then `tempxe_stop`. Allowed on `bare_metal` and `hypervisor`
kinds. **`discover_pxe`** never sets `install_os`; PXE discovery is armed via
`tempxe_discovery.enabled` only. Greenfield
`POST /sites/{name}/provision` (site bootstrap) is unchanged and separate from
the connector binding GET/PUT paths.

**Hardware discovery (`discover`, `discover_pxe`)** collects NIC/disk drafts via
YAIDAP on the jumphost. Both actions auto-sync hub inventory before running.

| Action | Requires | YAIDAP sketch |
|--------|----------|---------------|
| `discover` | `hypervisor` or `bare_metal` kind; reachable `ansible_host` | `discover-hosts.yml --tags check_host_details` |
| `discover_pxe` | `confirm=true`; subject in `hypervisors.yml` or `bare_metals.yml`; `host_vars` with `tempxe_discovery.enabled: true` and uplink `mac_address` | TempXE arm → optional PXE reboot → wait for discovery tarball → `collect_discovery` → `tempxe_stop` |

Discovery output lands under jumphost `generated/` (`host_vars/`, `hosts/`,
`pxe-discovery/`). **Import preview** reads those drafts and promotes them into
canonical hub paths (`host_vars/{name}.yml`, merged `hypervisors.yml`). Discovery
drafts often include `CHANGE_ME` placeholders; **import preview** surfaces
`warnings` on affected paths. Resolve them before sync or the next provision
action. When `ansible_host` is unknown, PXE discovery waits for a manual reboot.

Flow: [playbooks/flows/site-provision-yaidap.yml](../../playbooks/flows/site-provision-yaidap.yml).
Full YAIDAP integration reference: [yaidap-integration.md](yaidap-integration.md).
Historical design notes: [archive/design-provisioner-connector.md](../archive/design-provisioner-connector.md).

## Hub independence

Spokes are independent K3s control planes. The hub registers and manages them
via OCM; it does not run their workloads.

- **Hub down:** console and hub-routed URLs stop; workloads on spokes keep
  running.
- **Detach:** default delete keeps the spoke cluster and Helm releases.

## Helm charts (FleetApp)

After the cluster is `Ready`, install OCI charts with `FleetApp`. You must
set `spec.clusterName`; the controller does not choose a cluster. Publish
charts to the hub library (`FleetChart`) first when you want reuse. See
[fleet-apps.md](fleet-apps.md).

KubeVirt is an optional spoke **platform** add-on (Multus + CDI + KubeVirt),
not a chart-install path. See [fleet-kubevirt.md](fleet-kubevirt.md).

## Post-Ready cluster operations

When a cluster is `Ready`, operators use the console **Operations** panel,
`umkim`, or the fleet-site API:

| Operation | API |
|-----------|-----|
| Add worker | `POST /clusters/{name}/nodes` |
| Remove worker | `DELETE /clusters/{name}/nodes/{node}?force=true` |
| Replace control plane | `POST /clusters/{name}/control-plane/replace` |
| Validate node operation (pre/post) | `POST /clusters/{name}/node-operations/validate?phase=pre\|post` |
| List operation history | `GET /clusters/{name}/node-operations` |
| Operation Ansible logs | `GET /clusters/{name}/node-operations/{id}/logs` |
| Enable local RWX (`nfs_local`) | `POST /clusters/{name}/configure-storage` |
| Enable KubeVirt platform (optional `lanBridge`) | `POST /clusters/{name}/platform/kubevirt` |
| KubeVirt platform status | `GET /clusters/{name}/platform/kubevirt` |
| Day-2 LAN Multus flow | [playbooks/flows/cluster-enable-kubevirt-lan.yml](../../playbooks/flows/cluster-enable-kubevirt-lan.yml) |
| Site provisioner binding | `GET`/`PUT`/`PATCH /sites/{name}/provision` |
| Site provision actions | `POST`/`GET /sites/{name}/provision/actions` |
| List VirtualMachines | `GET /clusters/{name}/vms` |
| VM power (start/stop/restart) | `POST /clusters/{name}/vms/{ns}/{vm}/power` |
| VM serial / VNC console | WebSocket `…/vms/{ns}/{vm}/console` or `…/vnc` |

Node lifecycle, storage configure, and KubeVirt install share one job queue
(mutual exclusion). Completed storage and KubeVirt runs are recorded in
`nodeOperationHistory` as `configureStorage` / `configureKubevirt`.

Hub Ansible jobs (provision, storage, KubeVirt, node operations) reach cluster
nodes through `FleetClusterJoin.spec.jumphost`, or `FleetSite.spec.jumphost` for
the cluster's site when the cluster field is empty. Use `user@host:port` (port
optional). When set, jobs SSH to management LAN addresses via `ProxyJump`
instead of the WireGuard overlay — required for sites where only a bastion can
reach lab nodes.

Edit jumphost (and site region / failure domain) in the console **Sites** detail
(Edge and DNS → Connectivity) or via `umkim site patch`. Edit **ingress entries**
(`port_forward` / `reverse_proxy`) in Sites detail or
`umkim site ingress list|add|set|delete`. Per-cluster ingress binding:
console **Clusters** detail (Edge and ingress) or `umkim cluster patch --ingress-id`.
`umkim cluster bootstrap` resolves jumphost as cluster override, then site default.

When a cluster reaches **Ready**, the controller ensures a hub secret
`fleet-system/fleet-kubeconfig-<cluster>` exists (SSH to the spoke API over the
overlay). Provision playbooks sync it immediately after CSR approval; if a
provision job times out or fails later, a dedicated kubeconfig-sync Ansible job
is retried automatically so node list APIs keep working. HA bundle hub provision
jobs use a 90-minute deadline (single-node bundle jobs remain 30 minutes).

Hub finalize inventory for HA sets **`fleet_site_sp_endpoint`** (WireGuard IP of
the bootstrap control plane) in **`[site_bootstrap:vars]`**. Spoke ingress
verification curls that overlay address for Traefik `/ping`, not the management
LAN `ansible_host` used for jumphost SSH. Custom inventory must preserve this
when `site_bootstrap` nodes use ProxyJump to lab addresses. Generated inventory
keeps Ansible host names and group names distinct: a shared name makes
`delegate_to` drop `ansible_host`, so the cluster alias lives in `[fleet_cluster]`
rather than a group named after the cluster.

The HA bootstrap bundle parallelizes peer kernel-module setup, skips redundant
local kernel package installs when modules are already loaded, and uses shorter
SSH wait loops before K3s install.

Workers join during initial provision when listed in the FCJ `spec.nodes` at
create time. Post-Ready add uses the same Ansible agent-join path without
re-running full cluster provision.

Offline removal or replacement (`force=true`) skips cordon/drain on unreachable
hosts and may leave orphaned disk state; run `cleanup_node_local.sh` on revived
machines before reuse.

Each operation follows **validate → execute → validate**:

1. **Pre-validation** — `POST .../node-operations/validate` (or the same checks
   run automatically when submitting add/remove/replace). Fails fast on cluster
   state, duplicate addresses, or missing nodes.
2. **Execute** — existing add/remove/replace APIs queue an Ansible job.
3. **Post-validation** — controller and `?phase=post` confirm inventory and
   spoke node state after the job completes.

Completed operations are appended to `FleetClusterJoin.status.nodeOperationHistory`
(newest first, capped at 20). Each record stores actor identity, timestamps, outcome,
and a tail of the Ansible job log so operators can review failures after the Job TTL
expires.

CLI equivalent: [playbooks/flows/cluster-node-op.yml](../../playbooks/flows/cluster-node-op.yml).

## Management API

Public fleet-site management API access requires authenticated use through
`fleet-console-auth`. Use the console or an approved service-account client;
do not publish direct controller ports or include credentials in scripts and
examples. Enrollment is a separate, bearer-controlled bootstrap operation.

## Related features

- [fleet-apps.md](fleet-apps.md) — Helm install onto a selected cluster
- [fleet-kubevirt.md](fleet-kubevirt.md) — KubeVirt / Multus platform install path
- [fleet-cluster-storage.md](../fleet-cluster-storage.md) — storage intent and
  NFS safety
- [fleet-edge.md](fleet-edge.md) — site exposure and route configuration
- [fleet-ingress-auth.md](../fleet-ingress-auth.md) — optional forward-auth
- [fleet-llm.md](fleet-llm.md) — optional LLM integration
- [../../examples/site-profiles/](../../examples/site-profiles/) — exposure inventory examples

Network, firewall, certificate, and workload behavior depend on the
live environment and must be verified there.
