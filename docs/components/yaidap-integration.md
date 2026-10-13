# YAIDAP provisioner integration

> Status: v0.2 experimental. YAIDAP is a **sibling** metal/VM provisioner (TempXE,
> libvirt guests, hypervisor prepare). UMKIM does not vendor YAIDAP; it binds a
> site jumphost and runs YAIDAP through hub connector jobs. For FleetSite API
> surface and cluster lifecycle after SSH-ready, see [fleet-site.md](fleet-site.md).

## Layering

```text
┌─────────────────────────────────────────────────────────────────────────┐
│ UMKIM hub (L1: OS → fleet)                                               │
│  FleetSite.spec.provision · hub YAML inventory · provision API · console │
│  FleetClusterJoin enroll / add-worker / Ready                            │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │ async Kubernetes Job (fleet-ansible-runner)
                                │ SSH → site jumphost
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Site jumphost                                                            │
│  YAIDAP checkout (ansible-playbook) · inventory path on disk             │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │ ansible-playbook --limit SUBJECT
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ YAIDAP (L0: iron → OS)                                                   │
│  TempXE / PXE · libvirt · hypervisor discovery · guest host_vars         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Contract:** UMKIM owns **intent and inventory** on the hub. YAIDAP owns **execution
semantics** (kickstart, bridges, disks, HV XML). The connector translates hub
actions into `ansible-playbook` invocations on the jumphost and parses a minimal
**handoff record** (`ansible_host`, hostname, OS family) for enroll / add-worker.

Default provider is **`noop`**: operators supply SSH-ready hosts and skip YAIDAP
entirely.

## Site binding

Configure on `FleetSite.spec.provision` (API: `GET`/`PUT`/`PATCH /sites/{name}/provision`).

| Field | Required (yaidap) | Meaning |
|-------|-------------------|---------|
| `provider` | yes | `yaidap` |
| `jumphost` | yes | SSH target for connector jobs (`user@host` or `user@host:port`) |
| `sshSecretRef` | yes | Hub Secret in `fleet-console` namespace; private key for jumphost |
| `inventoryPath` | yes | Absolute path **on the jumphost** to this site's YAIDAP inventory root |
| `yaidapRoot` | no | YAIDAP git checkout on jumphost (default `~/yaidap`) |
| `yaidapRepo` | no* | Git remote for hub-managed YAIDAP sync (`https://…` or `git@…`) |
| `yaidapRef` | no | Branch, tag, or commit to checkout (default `main`) |
| `yaidapGitSecretRef` | no** | Hub Secret with deploy key (`id_ed25519`, `deploy_key`, or `id_rsa`) |
| `defaultExtraVars` | no | Safe non-secret `-e` pairs passed to every connector run |

\*Required for `POST …/provision/yaidap/sync`.  
\*\*Required when `yaidapRepo` uses SSH (`git@…`) or private HTTPS (`https://…`
without anonymous read). Public HTTPS remotes still need a deploy-key Secret when
the hub cannot clone without credentials.

### Fleet Ansible SSH key on new guests

`create_vm` connector runs pass **`fleet_ansible_ssh_pubkey`** to YAIDAP (from hub
Secret `fleet-site-ansible-ssh`, public key material). YAIDAP must install that
key in guest `authorized_keys` alongside the jumphost provision key so hub
cluster/node Ansible can SSH without manual steps. Minimum YAIDAP release for
this contract is pinned in [VERSIONS.md](../../VERSIONS.md).

### YAIDAP-side fixes (sibling repository)

UMKIM does not vendor YAIDAP. The following behaviors are required in the YAIDAP
checkout referenced by `yaidapRef` / the VERSIONS pin:

| Area | Expected behavior |
|------|-------------------|
| `create_vm` | Idempotent when the libvirt domain is already running (success without reimage) |
| Placement | Adopt running domains; improve free-memory accounting for plans |
| `install_bm` / TempXE | Fail fast without UEFI netboot; separate `install_os` vs `tempxe_discovery` arming |
| Cloud-init | Accept `fleet_ansible_ssh_pubkey` for hub fleet SSH |
| Disk | Optional growpart when guest disk is below cluster `minRootDiskGi` |

**Secret:** create before binding, e.g. `fleet-site-{site}-provision-ssh` with
`id_ed25519` / `id_ed25519.pub` (or `id_rsa` / `id_rsa.pub`). The ansible-runner
Job mounts this under `/root/.ssh`. Jumphost must accept that key.

**CLI binding:**

```bash
umkim site provision set SITE --provider yaidap \
  --jumphost USER@JUMPHOST \
  --ssh-secret fleet-site-SITE-provision-ssh \
  --inventory /path/on/jumphost/to/inventory \
  --yaidap-root /path/on/jumphost/to/yaidap
```

**Jumphost prep (operator):**

1. Bind `yaidapRepo`, `yaidapRoot`, and optional `yaidapGitSecretRef`, then run hub YAIDAP sync
   (`POST /sites/{site}/provision/yaidap/sync` or `umkim site provision yaidap-sync SITE`) to
   clone or update the checkout on the jumphost. For private repos, create a deploy-key Secret
   in `fleet-console` (read-only repo access) and reference it in `yaidapGitSecretRef`; the key is
   mounted only on the connector Job pod and staged on the jumphost for the duration of the sync.
2. Maintain a per-site inventory tree at `inventoryPath` (see layout below).
3. Ensure SSH from jumphost to hypervisors, guests, and BM targets on the site
   management LAN.
4. For libvirt VM create: base OS images and `group_vars/all/all.yml` site
   catalogs (`networks`, `storages`, …) must match what YAIDAP expects.

Lab-specific values and a full rebuild runbook:
[examples/homelab/rebuild-homelab-cli.md](../../examples/homelab/rebuild-homelab-cli.md).

## Hub inventory store

YAIDAP sites keep **native Ansible YAML** on the hub in ConfigMap
`fleet-site-{site}-yaidap-inventory` (`file.*` keys). Hub-only metadata lives
under `_meta/` and is **never** pushed to the jumphost.

| Path | Synced to jumphost | Role |
|------|-------------------|------|
| `guests.yml` | yes (hub: VM index only) | `guests.hosts`; jumphost copy also has `external.hosts` composed from hub `bare_metals.yml` on sync |
| `bare_metals.yml` | **hub only** | Bare-metal cluster nodes (`kind: bare_metal` in host APIs) |
| `hypervisors.yml` | yes | Hypervisor group (`kind: hypervisor` in host APIs) |
| `{cluster}.yml` | yes | Ansible group membership (`hosts:` list) |
| `group_vars/all/all.yml` (+ optional `group_vars/all/*.yml`) | yes | Site networks, storages, defaults |
| `group_vars/{group}.yml` | yes | Group vars; Fleet cluster intent for cluster groups |
| `host_vars/{guest}.yml` | yes | Per-guest YAIDAP profile (disks, uplinks, TempXE discovery) |
| `_meta/meta.json` | no | Sync phase, revision head, last sync/import timestamps |
| `_meta/status.json` | no | Per-guest provision + cluster-apply phase |
| `_meta/revisions.json` | no | Timestamped edit log |
| `_meta/import_preview.json` | no | Staging for jumphost import diff |

**Jumphost-only** (preserved on sync; not stored on hub): `controllers.yml`,
`routers.yml`, `generated/` (discovery drafts, PXE tarballs).

**Sync phases** (`_meta/meta.json` → `syncPhase`):

| Phase | Meaning |
|-------|---------|
| `pending` | Never synced or freshly migrated |
| `dirty` | Hub edits not yet on jumphost |
| `syncing` | Push job running |
| `synced` | Jumphost tree matches last successful push |

`POST /sites/{name}/provision/inventory/sync` (or console **Sync to jumphost**)
builds a bundle from synced paths only, base64-encodes it, and runs
`site-provision-connector.yml` with `fleet_provision_action=sync_inventory`.
Success is detected via log marker `FLEET_INVENTORY_SYNCED=1`.

Provision actions that mutate YAIDAP input **pre-sync** the same bundle
(`fleet_provision_sync_from_store=true`) before running the YAIDAP playbook.

Run `POST …/provision/inventory/migrate` once to seed the native skeleton from
legacy JSON (`catalog.json` / `guests.json`) or deprecated `clusters.yml`.

## Inventory editing

Three surfaces share one store:

| Surface | Commands / UI |
|---------|----------------|
| File API | `GET/PUT/DELETE …/provision/inventory/files/{path}` |
| CLI | `umkim site inventory files {list,get,put,delete}` |
| Console | **Sites → detail → Provisioner inventory** |

**VM guest bootstrap (console):** **Add VM guest** writes `guests.yml` +
`host_vars/{name}.yml` from templates (`inventoryVmTemplates.ts`). Defaults:
`vm_location: least-utilized`, libvirt profile, lab-style uplink placeholders.

**Hypervisor (console):** **Add hypervisor** writes `hypervisors.yml` +
`host_vars/{name}.yml` with optional `tempxe_discovery.enabled` and uplink MAC.

**Bare-metal node (console):** **Add bare metal node** writes `bare_metals.yml` +
`host_vars/{name}.yml` (optional `tempxe_discovery.enabled` and uplink MAC for PXE discover).

**Guided add (console + CLI):** **Add host** is the recommended path — pick the
host kind, give a name plus an address (IP for SSH, or MAC for PXE), and choose
network/storage/zone from the site catalog. It calls
`POST /sites/{name}/provision/inventory/hosts` (kind-aware intent), which composes
`guests.yml` / `bare_metals.yml` / `hypervisors.yml` and `host_vars/{name}.yml`
server-side. VM guests take `cores`, `memory`, and `diskSizeGi` for the boot disk,
plus an optional `disks` list (`{name, sizeGi, storage, format}`) of additional
data disks — rendered after the boot disk, which is significant because YAIDAP
derives each target device from the list position (`disks[0]` is boot). Discovery
(SSH first, PXE fallback) is offered as the next step; the
raw YAML editor stays available for advanced edits. The same intent is exposed on
the CLI as `umkim site host add` (`--disk` for the boot disk, repeatable
`--extra-disk` for data disks).

**Host kinds** on `GET /sites/{name}/hosts`: `vm` (`guests.hosts`), `bare_metal`
(`bare_metals.yml`), `hypervisor` (`hypervisors.yml`). On jumphost sync, hub
`bare_metals.yml` is merged into YAIDAP `guests.yml` → `external.hosts` (not
stored as a separate file on the jumphost).

**Cluster automation** uses native Ansible layout:

- Root `{cluster}.yml` — group with `hosts:` listing member guests.
- `group_vars/{cluster}.yml` — `topology`, `autoApply`, `controlPlaneVip`,
  `haGroup`, `mode` (`enroll` | `add_worker` | `replace_control`), etc.

Members are defined only in the root group file, not duplicated in `group_vars`.

**Placeholder safety:** discovery drafts and stubs often contain `CHANGE_ME`.
Import preview and the console editor surface **warnings** per path. Resolve
placeholders before sync or provision; YAIDAP may fail or produce wrong networks.

## Provision actions

`POST /sites/{name}/provision/actions` body:

```json
{
  "action": "create_vm",
  "subject": "guest01",
  "confirm": false,
  "params": { "tags": ["create"], "playbook": "playbooks/provision-vms.yml" }
}
```

`params.tags` and `params.playbook` override defaults. Destructive actions
require `confirm: true`.

### Action → YAIDAP mapping

Implemented in `controllers/fleet-site/internal/provision/yaidap.go` and
executed by [site-provision-connector.yml](../../site-provision-connector.yml)
on the jumphost (`cd $yaidapRoot && ansible-playbook …`).

| Action | Confirm | Pre-sync inventory | Default YAIDAP invocation |
|--------|---------|-------------------|---------------------------|
| `discover` | no | yes | `playbooks/discover-hosts.yml` · tags `check_host_details` · `-e yaidap_emit_result=true` |
| `discover_pxe` | **yes** | yes | `playbooks/discover-pxe.yml` · tag `discover_pxe` · `--connection local` · `-e tempxe_confirm=true` |
| `install_bm` | **yes** | yes | `playbooks/install-bare-metal.yml` · tag `install_bm` · SSH to target for reboot/wait; TempXE arm uses per-play `connection: local` · `-e tempxe_confirm=true` · `-e install_os=true` |
| `create_vm` | no* | yes | `playbooks/provision-vms.yml` · tags `create`, `never` · `-e yaidap_emit_result=true` · `-e yaidap_recreate=true` when guest has concrete `vm_location` |
| `plan_vm` | no | yes | `playbooks/plan-vm-placement.yml` · tag `plan` (dry-run; no VM, no inventory rewrite) |
| `destroy` | **yes** | yes | `playbooks/provision-vms.yml` · tag `destroy` · `-e confirm_destroy=true` |
| `prepare_hypervisor` | no | yes | `playbooks/configure-hypervisors.yml` · tag `prepare` · `-e configure_firewalld=false` |
| `prepare_gpu_passthrough` | **yes** | yes | `playbooks/configure-hypervisors.yml` · tag `prepare_gpu_passthrough` · `-e configure_firewalld=false` |
| `prepare_hypervisor_gpu` | **yes** | yes | `configure-hypervisors.yml` · tags `prepare`, `prepare_gpu_passthrough` · `-e configure_firewalld=false` |
| `status` | no | no | `playbooks/configure-tempxe.yml` · tag `tempxe_status` |

\*Re-provision on an already-**Ready** VM guest with a concrete `vm_location`
sets `yaidap_recreate=true`. The console requires a confirm checkbox for this
case; the API does not enforce `confirm` on `create_vm` today.

**`install_os` in site inventory:** commit **`install_os: false`** (or omit; role
default is false). Do not set `install_os: true` in `host_vars` for normal hosts.
The hub connector adds **`-e install_os=true`** only for **`install_bm`**
([site-provision-connector.yml](../../site-provision-connector.yml)). **`discover_pxe`**
forces `install_os` off in YAIDAP and arms the discovery menu via
`tempxe_discovery.enabled` (TempXE templates treat discovery separately from
full kickstart install).

**Validation highlights:**

- `discover`: subject in inventory; reachable `ansible_host`.
- `discover_pxe`: subject in `hypervisors.yml` or `bare_metals.yml`; `host_vars` with
  `tempxe_discovery.enabled: true` and uplink `mac_address`.
- `plan_vm`: existing guest in `guests.yml` (not external-only).
- `create_vm` / `plan_vm`: `ansible_host` required in `host_vars` or sync extra
  vars (static management IP for cloud-init).
- `destroy`: VM guest in `guests.yml`; guest phase **Ready** or **Failed**;
  `confirm=true`;
  blocked when the guest is a cluster member and a live `FleetClusterJoin` exists
  (unless the join is deleting). On success the hub sets guest phase **Declared**
  (not Ready), clears cluster sub-status, and keeps inventory for reprovision.

### YAIDAP integration contract (`YAIDAP_RESULT`)

YAIDAP v1 integration playbooks emit a single JSON line when the hub passes
`-e yaidap_emit_result=true` (set automatically for provision actions). The
fleet-site controller parses the last `YAIDAP_RESULT=` line from connector job
logs (`schema: yaidap.integration.v1`).

```text
YAIDAP_RESULT={"schema":"yaidap.integration.v1","action":"create_vm","subject":"vm01","phase":"ready","ok":true,"message":"VM created","vm_location":"virt02","handoff":{"hostname":"vm01","ansible_host":"172.16.10.21","ansible_user":"linux","os_family":"rocky-10"},...}
```

| Field | Hub use |
|-------|---------|
| `message` | Provision action history message |
| `handoff` | `ProvisionHandoffHost` for enroll / add-worker |
| `vm_location` | Persist concrete hypervisor after `create_vm` |
| `placement` | `plan_vm` assignment summary |
| `generated_paths` | Discovery import hints (optional) |

Upstream contract reference: YAIDAP `docs/integrations/umkim.md` (sibling repo).

### Connector workflow (`site-provision-connector.yml`)

The hub connector syncs inventory to the jumphost, then runs one YAIDAP playbook
per action. Composite TempXE / PXE flows live in YAIDAP (`install-bare-metal.yml`,
`discover-pxe.yml`); the connector no longer re-implements them.

**`create_vm`**

1. Pre-sync hub bundle to jumphost.
2. When the guest already has a concrete `vm_location`, pass
   `-e yaidap_recreate=true` (YAIDAP destroys the stale libvirt domain before create).
3. Ensure libvirt guest profile in `host_vars` when using legacy per-field sync.
4. Run YAIDAP create with `-e yaidap_emit_result=true`.

After successful `create_vm`, the fleet-site controller:

1. Parses `vm_location` from `YAIDAP_RESULT` (falls back to legacy
   `FLEET_YAIDAP_VM_LOCATION=` log marker when absent).
2. Rewrites `guests.yml` and `host_vars/{guest}.yml` from schedule tokens
   (`least-utilized`, `auto`, empty) to the concrete hypervisor.
3. Marks inventory `dirty` and enqueues **inventory sync** so the jumphost matches.

Schedule tokens are rejected for persistence (`PersistResolvedVmLocation`).

### Action phases and handoff

| Phase | Meaning |
|-------|---------|
| `Pending` | Accepted; Job not yet observed |
| `Running` | Connector Job active |
| `Succeeded` | YAIDAP finished; `host` fragment populated when known |
| `Failed` | Job failed or missing; `logTail` on history entries |

**Handoff record** (`ProvisionHandoffHost`):

```yaml
hostname: guest01
ansible_host: 203.0.113.10
ansible_user: linux
osFamily: rocky
siteId: SITE
```

Used by enroll / add-worker flows. VM guests created through the connector use
**`minRootDiskGi: 38`** on `FleetClusterJoin` (40G vdisk root LV < 40 GiB after
EFI/boot partitions).

Guest provision status is mirrored in `_meta/status.json` per guest:
`Declared` → `Provisioning` → `Ready` | `Failed`.

## End-to-end flows

### A. Libvirt VM → cluster enroll

```text
1. Bind site (yaidap + jumphost + inventoryPath + secret)
2. Edit inventory (guests.yml, host_vars, group_vars/all, cluster files)
      OR console: Add VM guest → edit YAML → Save
3. POST …/inventory/sync  (or auto before provision)
4. POST …/hosts/{guest}/provision  action=create_vm
5. Hub rewrites vm_location + syncs inventory
6. Cluster-apply reconciler (or POST …/hosts/{guest}/cluster) when guests Ready
7. cluster create → bootstrap → provision → Ready
```

Console: **Provisioner inventory** → **Site hosts** → **Provision** → **Apply cluster**.

### B. Bare metal hypervisor → single-node cluster

```text
1. bare_metals.yml + host_vars stub (discovery stub or hand-authored)
2. sync
3. discover OR discover_pxe (confirm for PXE)
4. import preview → import apply (promote generated/ → host_vars, bare_metals.yml)
5. install_bm (confirm) — full OS reinstall via TempXE
6. cluster create on BM host (may be external to guests.yml)
7. bootstrap / provision as usual
```

Bare-metal cluster nodes live under hub `bare_metals.yml` (`kind: bare_metal`).
Hypervisors live under `hypervisors.yml` (`kind: hypervisor`). Libvirt guests
live under `guests.hosts` (`kind: vm`). Do not put `external.hosts` in hub
`guests.yml`; the controller composes that section for the jumphost on sync.

### C. Import from jumphost

```text
POST …/inventory/import/preview  → Job reads jumphost tree + generated/
GET  …/inventory/import/preview  → per-path diff, warnings
POST …/inventory/import/apply    → merge selected paths into hub store
```

Discovery promotion maps `generated/host_vars/` → `host_vars/`,
`generated/hosts/` → merged `hypervisors.yml`. `*.notes.*` files stay on jumphost.

### D. Cluster intent automation

When `group_vars/{cluster}.yml` has `autoApply: true` and all member guests reach
`Ready`, the registry-site reconciler creates `FleetClusterJoin` from root +
`group_vars` cluster files. Manual retry: `POST …/hosts/{guest}/cluster`.

Status fields under each guest's `cluster` key in `_meta/status.json` mirror apply
progress (`Pending`, `WaitingGroup`, `Applying`, `Ready`, `Failed`).

## Job execution

```text
fleet-site HTTP API
  → startYaidapProvisionJob()
  → Batch Job fleet-site-prov-{actionId}
  → Pod: fleet-ansible-runner image
  → ansible-playbook site-provision-connector.yml -l jumphost
  → SSH session on jumphost runs YAIDAP ansible-playbook
```

- Jumphost ansible inventory: ConfigMap `fleet-site-{site}-provision-inventory`
  (`[jumphost]` group).
- One running provision action per site (`409` if busy).
- History: last 20 actions on `FleetSite.status.provision`; log tail up to 64 KiB.
- Flow playbook for operators/automation:
  [playbooks/flows/site-provision-yaidap.yml](../../playbooks/flows/site-provision-yaidap.yml).

Inventory-only jobs use prefix `fleet-site-inv-` with the same connector playbook
and `fleet_provision_action=sync_inventory` or `import_inventory_preview`.

## API quick reference

| Method | Path | Purpose |
|--------|------|---------|
| `GET`/`PUT`/`PATCH` | `/sites/{name}/provision` | Connector binding |
| `POST` | `/sites/{name}/provision/yaidap/sync` | Clone/update YAIDAP checkout on jumphost |
| `POST` | `/sites/{name}/provision/actions` | Start action |
| `GET` | `/sites/{name}/provision/actions[/{id}]` | Poll / history |
| `GET` | `/sites/{name}/provision/inventory` | Meta + file index |
| `POST` | `/sites/{name}/provision/inventory/hosts` | Guided host intent (compose inventory from minimal fields) |
| `GET`/`PUT`/`DELETE` | `/sites/{name}/provision/inventory/files/{path}` | YAML CRUD |
| `POST` | `/sites/{name}/provision/inventory/sync` | Push to jumphost |
| `POST` | `/sites/{name}/provision/inventory/migrate` | Legacy → native YAML |
| `POST`/`GET` | `/sites/{name}/provision/inventory/import/preview` | Jumphost pull + diff |
| `POST` | `/sites/{name}/provision/inventory/import/apply` | Merge selected paths |
| `GET` | `/sites/{name}/provision/inventory/hosts` | Derived host rows |
| `GET` | `/sites/{name}/provision/inventory/clusters` | Cluster intent aggregate |
| `POST` | `/sites/{name}/hosts/{host}/provision` | Provision by guest name |
| `GET`/`POST` | `/sites/{name}/hosts/{host}/cluster` | Cluster apply status / manual trigger |

Deprecated: `POST/PATCH/DELETE /sites/{name}/hosts/{host}` — use file API.

OpenAPI: [controllers/fleet-site/openapi.yaml](../../controllers/fleet-site/openapi.yaml).

## CLI quick reference

See [umkim-cli.md](umkim-cli.md) for full syntax.

```bash
# Binding
umkim site provision get|set SITE …

# Inventory
umkim site inventory files list|get|put|delete SITE …
umkim site inventory migrate|sync|revisions SITE …
umkim site inventory import preview|apply SITE …

# Actions
umkim site provision run SITE --action ACTION --subject HOST [--confirm] [--wait]
umkim site host list|get|provision|cluster-apply SITE HOST …
```

## Console

| Location | Function |
|----------|----------|
| **Sites → detail → Provisioner inventory** | YAML tree editor, sync, import preview, Add VM guest, Add discovery stub, revisions |
| **Sites → detail → Site hosts** | Derived host table, Provision / Plan / Discover, Apply cluster, provision history |
| **Sites → detail → Edge and DNS** | Site jumphost (separate from provision binding SSH target when both set) |

Provisioner panels are relevant when `spec.provision.provider=yaidap`. Site
**jumphost** for cluster bootstrap SSH may match the YAIDAP jumphost but is
configured independently (`FleetSite.spec.jumphost`).

## Safety and exclusions

| Rule | Detail |
|------|--------|
| Confirm gate | `install_bm`, `discover_pxe`, `destroy`, `prepare_gpu_passthrough`, `prepare_hypervisor_gpu` require `confirm=true` |
| TempXE | Never arm PXE without confirm; connector always runs `tempxe_stop` after BM install / PXE discovery |
| Mutual exclusion | One site provision action at a time; inventory sync blocked while sync job runs |
| Hub schema boundary | Do **not** import YAIDAP `networks` / `uplinks` / `storages` / disk catalogs into Fleet CRDs — they stay in `group_vars` / `host_vars` |
| YAIDAP scope | Kubernetes, OCM, Harbor, and FleetApp are **out of scope** for YAIDAP; UMKIM takes over at SSH-ready |
| Secrets | Jumphost keys only via `sshSecretRef`; no passwords in `defaultExtraVars` |

## Troubleshooting

| Symptom | Likely cause | Check |
|---------|--------------|-------|
| `guests.yml missing` | Empty hub store | Migrate; add `guests.yml` via file API |
| `confirm=true is required` | Destructive action | Pass `--confirm` / UI checkbox |
| `ansible_host is required for create_vm` | Missing static IP | Set in `guests.yml` + `host_vars` before provision |
| `discover_pxe requires tempxe_discovery.enabled` | Stub incomplete | Edit `host_vars`; enable PXE discovery + MAC |
| Import warnings `CHANGE_ME` | Unresolved discovery draft | Edit YAML; re-sync |
| `site provision action already running` | Overlapping jobs | Wait or inspect Job `fleet-site-prov-*` |
| Placement still `least-utilized` after create | Job log missing result | Inspect YAIDAP logs; verify `YAIDAP_RESULT` includes concrete `vm_location` |
| Sync stuck `syncing` | Failed push job | Job logs; jumphost disk permissions on `inventoryPath` |
| BM install timeout | PXE / kickstart | TempXE status on jumphost; target power; firewall |
| Cluster **Failed** but spoke joined | Hub provision Job or finalize error | `kubectl get fleetclusterjoin`; Job `fleet-cluster-provision-*`; retry `umkim cluster provision` |

## Implementation map

| Component | Path |
|-----------|------|
| CRD types | `controllers/fleet-site/api/v1alpha1/fleetsite_types.go` |
| Action mapping | `controllers/fleet-site/internal/provision/yaidap.go` |
| Hub YAML store | `controllers/fleet-site/internal/connectors/yaidap/` |
| HTTP API | `controllers/fleet-site/internal/httpapi/site_provision.go`, `site_inventory*.go` |
| Job completion / vm_location | `controllers/fleet-site/internal/siteprovision/sync.go` |
| Connector playbook | [site-provision-connector.yml](../../site-provision-connector.yml) |
| Operator flow | [playbooks/flows/site-provision-yaidap.yml](../../playbooks/flows/site-provision-yaidap.yml) |
| CLI | `cmd/umkim/internal/cmd/site_provision.go`, `site_inventory*.go` |
| Console UI | `ui/umkim-console/src/components/sites/SiteProvisionerInventoryPanel.tsx`, `SiteHostsPanel.tsx` |

Historical design notes (superseded by this doc and [fleet-site.md](fleet-site.md)):
[archive/provisioner-connector-plan.md](../archive/provisioner-connector-plan.md),
[archive/design-provisioner-connector.md](../archive/design-provisioner-connector.md).

## Related

- [fleet-site.md](fleet-site.md) — FleetSite lifecycle, ingress, post-Ready ops
- [umkim-cli.md](umkim-cli.md) — CLI examples
- [ui/umkim-console/README.md](../../ui/umkim-console/README.md) — console workflows
- [examples/homelab/](../../examples/homelab/) — maintainer lab inventory and rebuild script
- [docs/secrets.md](../secrets.md) — secret handling patterns
