# Provisioner connector (implementation plan)

> Status: **planned** (not implemented). Optional path to make hosts SSH-ready
> before FleetClusterJoin / add-worker. Topology-neutral; no lab endpoints.

## Intent (product)

UMKIM **owns the site host inventory** (add host, list hosts, provision
host-x). YAIDAP / OpenTofu / Terraform are **execution backends only**.

A **connector** translates the same hub intent into backend-specific commands
and flows. Operators never edit YAIDAP inventory as the system of record when
using the hub path.

This is **not** reimplementing YAIDAP inside UMKIM: the hub keeps a **thin**
inventory (identity + intent). Connectors map that to YAIDAP playbooks/tags or
tofu apply — including generating any ephemeral inventory/vars the backend
needs for that run.

## Problem

UMKIM assumes **Rocky Linux 10 hosts are already installed and reachable over
SSH**. Metal install, hypervisor prepare, PXE/TempXE, and libvirt/Proxmox guest
create live outside the fleet control plane (today often in **YAIDAP**).

Operators want to:

1. Manage **which hosts a site has** on the hub (inventory of record).
2. Hit **Provision** on a hub host and have the right YAIDAP (or tofu) commands
   run.
3. Keep UMKIM as orchestrator; backends as interchangeable executors.
4. Use one hub → connector contract so a second backend does not change console
   or CLI shape.

## Goals

| Goal | Detail |
| --- | --- |
| Hub inventory | Site hosts live in UMKIM (API/CR); list / add / update / delete intent. |
| Same connector input | All providers receive the same provision command + host snapshot. |
| Translate, don’t reimplement | Connectors map flows to YAIDAP/tofu; no TempXE/HV engine in UMKIM. |
| Thin schema | Hub stores identity + provision intent + site defaults—not YAIDAP’s full `uplinks` / disk / ISO catalog. |
| Optional | Default remains no provisioner: hosts already SSH-ready (today). |
| YAIDAP first | First connector; tofu/terraform later with identical hub contract. |
| Handoff | Success → SSH-ready facts → existing enroll / add-worker. |
| Safety | Destructive PXE/reinstall requires confirm; job mutual exclusion as needed. |

## Non-goals

- Merging YAIDAP (or Terraform modules) into the UMKIM repository.
- Porting YAIDAP’s full inventory schema into Fleet CRDs.
- UMKIM becoming a general cloud scheduler or multi-hypervisor placer.
- Replacing FleetClusterJoin once the host is SSH-ready.
- Requiring operators to maintain a parallel YAIDAP inventory as source of truth
  for hub-managed sites (connector may **generate** ephemeral YAIDAP input).

## Layering

```text
┌──────────────────────────────────────────────────────────────┐
│ UMKIM hub                                                      │
│  FleetSite · site host inventory · provision API · console/CLI │
│  FleetClusterJoin / add-worker (after SSH-ready)               │
└────────────────────────────┬─────────────────────────────────┘
                             │ same Contract:
                             │   Host record + ProvisionCommand
                             ▼
┌──────────────────────────────────────────────────────────────┐
│ Connector (provider-specific translator)                       │
│  yaidap | tofu | terraform | noop                              │
└───────────────┬────────────────────────┬─────────────────────┘
                │                        │
                ▼                        ▼
     Generate ephemeral          Generate vars /
     YAIDAP inventory            tofu workspace inputs
     + ansible-playbook …        + tofu apply/destroy
                │                        │
                └────────────┬───────────┘
                             ▼
                    SSH-ready host
                             ▼
              Existing UMKIM enroll / add-worker
```

**Rule:** Hub speaks one language. Connectors translate. Backends execute.

## Concepts

### Site host inventory (hub-owned)

Per FleetSite, a collection of **host records** (API and/or CR). Thin fields
only—enough to identify intent and hand off after provision:

| Field (sketch) | Meaning |
| --- | --- |
| `name` | Stable id within the site (`host-x`) |
| `kind` | `bare_metal` \| `vm` \| … |
| `desiredState` | e.g. `present` \| `absent` (optional for v1) |
| `network` | Optional: `ansible_host` / addresses once known |
| `ssh` | Optional: user hint after ready |
| `os` | Family / image hint (e.g. rocky) |
| `labels` / `flavor` | Opaque keys mapped by connector + site defaults |
| `status.phase` | `Declared` \| `Provisioning` \| `Ready` \| `Failed` \| … |
| `status.lastProvision` | Last job id / message / observed `ansible_host` |

**Not** on the hub record: YAIDAP `uplinks`, full disk topology, TempXE ISO
catalogs, HV XML detail. Those stay in **site connector defaults** or backend
templates the connector applies when rendering.

### Site connector binding

Per FleetSite (optional):

| Field (sketch) | Meaning |
| --- | --- |
| `provisioner.provider` | `noop` \| `yaidap` \| `tofu` \| … |
| `provisioner.endpoint` | Where to run (e.g. jumphost for YAIDAP) |
| `provisioner.defaults` | Opaque map: playbook paths, tags, HV name, network profile, workspace, timeouts |
| `provisioner.credentialsRef` | Secret for jump / cloud / state backend |

### Provision command (hub → connector — stable)

Same shape for every provider:

| Field | Meaning |
| --- | --- |
| `siteId` | FleetSite |
| `action` | `provision` \| `deprovision` \| `discover` \| `status` \| `reconcile` |
| `host` | Full hub host record snapshot (or id + fetched record) |
| `siteDefaults` | Resolved `provisioner.defaults` |
| `confirm` | Required for destructive / PXE-arming class actions |
| `requestId` | Idempotency / audit |

`create_vm` vs `install_bm` is derived from `host.kind` + `action`, not from a
provider-specific API. Console always: select host → Provision.

### Provision result (connector → hub — stable)

| Field | Meaning |
| --- | --- |
| `phase` | `Pending` \| `Running` \| `Succeeded` \| `Failed` |
| `message` | Human-readable status |
| `observed` | On success: `ansible_host`, hostname, OS family, SSH user hint |
| `raw` | Optional provider diagnostics (log pointer, recap) |

Hub writes `observed` back onto the host inventory record.

## Connector contract

```text
input:  ProvisionCommand  (identical for all providers)
output: ProvisionResult   (identical for all providers)
```

| Provider | Translation responsibility |
| --- | --- |
| `noop` | Host already reachable; copy `ansible_host` from record or fail. |
| `yaidap` | Project connector inventory from hub CM to jumphost; run playbooks/tags/limit; parse outcome. |
| `tofu` / `terraform` | Render vars / select workspace; apply or destroy; map outputs → `observed`. |

Adding OpenTofu means a **new connector implementation**, not a new console
inventory model.

## YAIDAP connector

### Role

Translator + runner. YAIDAP remains the PXE/HV/VM engine. Hub does not call
random ansible tags from the UI; the connector owns the flow table.

### Mapping (illustrative)

| Hub intent | YAIDAP translation sketch |
| --- | --- |
| Add host + Provision (`kind=vm`) | Render guest inventory fragment → `provision-vms.yml --tags create --limit NAME` |
| Provision (`kind=bare_metal`) | Render host + TempXE flow → configure / confirm start / wait / disarm |
| Deprovision | destroy tags / playbooks with confirm |
| Discover | discovery playbooks; **results written into hub inventory**, not kept only in YAIDAP files |
| Status | status tags; map into host `status` |

Playbook/tag names live in connector code + `siteDefaults`, not in the core
hub schema.

### Connector-persistent inventory (hub YAML CM + sync to jumphost)

YAIDAP inventory for hub-managed sites is **connector-owned and hub-persisted** as editable YAML:

1. Store per-site files in hub ConfigMap `fleet-site-{site}-yaidap-inventory`
   (native Ansible tree: root `file.guests.yml`, `file.hypervisors.yml`,
   `file.{cluster}.yml`, `file.group_vars.*.yml`, `file.host_vars.*.yml`, plus
   `_meta/*` for sync phase, status, revisions).
2. **Sync** pushes synced paths to the jumphost inventory tree (scoped rsync; jumphost-only files preserved).
3. Run provision actions against the synced tree (`--limit <name>`).
4. **Import preview + apply** merges selected jumphost files back into the hub store
   (hub-only paths such as `_meta/*` are never overwritten).

| Stance | Owner |
|--------|--------|
| Guest index + YAIDAP profiles | `guests.yml` + `host_vars/*.yml` (synced) |
| Site base (HV list + group defaults) | `hypervisors.yml` + `group_vars/all/*.yml` (synced) |
| Cluster membership | `{cluster}.yml` at inventory root (synced) |
| Cluster Fleet intent | `group_vars/{cluster}.yml` (synced) |
| Provision/cluster runtime phase | `_meta/status.json` (hub-only) |
| Execution | YAIDAP on jumphost |

Out-of-band jumphost edits are supported via **import preview/apply**; hub-managed flows should
use console/CLI inventory file API + sync instead of hand-editing files on the jumphost only.

### Safety

- Mirror YAIDAP TempXE confirm (`tempxe_confirm` / arming).  
- Never arm PXE without hub `confirm=true`.  
- Prefer disarm/stop on failure and success where applicable.

## OpenTofu / Terraform connector

Same `ProvisionCommand` / `ProvisionResult`.

1. Map `host` + `siteDefaults` → tfvars / workspace.  
2. `apply` or `destroy`.  
3. Map outputs → `observed` on the hub host record.

No YAIDAP inventory involved.

## API sketch (hub)

Prefer extending fleet-site.

### Host inventory

- `GET/POST /sites/{site}/hosts`  
- `GET/PATCH/DELETE /sites/{site}/hosts/{name}`  

### Provision

- `POST /sites/{site}/hosts/{name}/provision` — body: `{ action, confirm }`  
- `GET /sites/{site}/provision/{id}` — job status  
- `GET /sites/{site}/provision` — recent jobs  

Optional later: `FleetSiteHost` CR and/or `FleetProvisionRequest` for GitOps.

### Auth

Same console-auth / RBAC patterns. Destructive actions need elevated permission
and `confirm=true`.

## Console / CLI UX

### Console

- **Sites → Hosts** — list hub inventory; add BM or VM (thin form + flavor).  
- **Provision** / **Deprovision** on a row (confirm modal when needed).  
- Job progress + logs (reuse node-op / platform patterns).  
- After Ready: **Enroll cluster** / **Add worker** using `observed.ansible_host`.

### CLI

```text
umkim site host list SITE
umkim site host add SITE --name host-x --kind vm [--flavor …]
umkim site host provision SITE host-x [--confirm]
umkim site host deprovision SITE host-x --confirm
umkim site provision status SITE [REQUEST_ID]
umkim cluster join …    # existing
umkim node add …        # existing
```

## Interaction with FleetClusterJoin

Unchanged after SSH-ready. Provision is optional and upstream of enroll /
add-worker. Optional audit annotation linking FCJ to provision request id.

## Implementation phases

### Phase 0 — Spec and spikes

- [ ] Freeze `Host` + `ProvisionCommand` / `ProvisionResult` JSON (this doc).  
- [ ] Spike: render minimal YAIDAP inventory from a fake hub host; run validate
      or create on jumphost.  
- [ ] Document secret / jumphost shape.  
- [ ] Agree RBAC for hosts + provision.

### Phase 1 — Hub inventory + `noop`

- [ ] Host CRUD API (and storage: CR or status-backed).  
- [ ] Provision endpoint → `noop` connector.  
- [ ] CLI + console list/add + provision stub.  
- [ ] Tests: validation, RBAC, mutual exclusion.

### Phase 2 — YAIDAP connector

- [ ] FleetSite `provisioner` binding (provider, endpoint, defaults).  
- [ ] Render ephemeral inventory; run create_vm / status flows.  
- [ ] Then install_bm + confirm + TempXE lifecycle.  
- [ ] Write `observed` back to hub host.  
- [ ] Lab: add host on hub → provision → SSH → FCJ / add-worker.  
- [ ] Docs: component page + thin host example (no lab IPs).

### Phase 3 — UX polish

- [ ] Discover → create/update hub hosts.  
- [ ] Provision → enroll/add-worker wizard.  
- [ ] Log streaming; jumphost unreachable messaging.

### Phase 4 — Tofu/Terraform connector

- [ ] Same command contract; tfvars from hub host.  
- [ ] Console/CLI unchanged aside from site provider setting.

### Phase 5 — Hardening

- [ ] Idempotent reconcile; quotas; audit for confirm-gated actions.  
- [ ] Optional one-shot import from existing YAIDAP inventory into hub.

## Inventory stance (clarified)

| Layer | Owns |
| --- | --- |
| UMKIM hub | Site host list, provision intent, observed SSH facts, job history |
| Connector | Translation, ephemeral backend inputs, command sequences |
| YAIDAP / tofu | Execution only (PXE, HV, VM create, apply) |

- Do **not** make operators maintain YAIDAP files as SoT for hub-managed sites.  
- Do **not** copy YAIDAP’s full schema onto the hub.  
- Do align handoff fields: name, `ansible_host`, SSH user, OS family.

## Security

- Secrets in Kubernetes Secrets / vault—not in examples or design docs.  
- Jumphost / cloud credentials scoped per site.  
- Confirm mandatory for reinstall/PXE arming.  
- Provision logs may contain host identifiers—treat as sensitive.  
- See [SECURITY.md](../SECURITY.md) and [secrets.md](secrets.md).

## Testing strategy

| Layer | Check |
| --- | --- |
| Unit | Host validation; connector mapping; confirm gating; render inventory/tfvars |
| Integration | Fake connector; API host + job lifecycle |
| Lab | Hub add host → YAIDAP connector → SSH → FCJ/add-worker |
| Regression | Sites without provisioner unchanged |

## Documentation map (when implementing)

| Change | Update |
| --- | --- |
| Connector + hub inventory | This file → graduate to [components/](components/) |
| FleetSite / host API | [fleet-site.md](components/fleet-site.md), CRD |
| CLI | [umkim-cli.md](components/umkim-cli.md) |
| Console | `ui/dcm-fleet-console/README.md` |
| Architecture | [design.md](design.md) optional services |
| Examples | [examples/site-profiles/](../examples/site-profiles/) pointer-only |

## Open decisions

1. Host storage: dedicated `FleetSiteHost` CR vs nested FleetSite status/API store.  
2. YAIDAP runs on jumphost only vs hub runner with generated inventory mounted.  
3. How much of `flavor` / profiles live in hub vs connector defaults.  
4. Discover: create draft hub hosts automatically vs operator confirm import.  
5. `FleetProvisionRequest` CR in v1 vs job history only.

## Success criteria

- Operator manages site hosts on the hub; Provision runs without editing YAIDAP
  inventory by hand.  
- YAIDAP connector translates the same command a future tofu connector will
  receive.  
- Hub host schema stays thin; YAIDAP is not reimplemented in UMKIM.  
- Sites with no provisioner behave as today.

## Related

- [design.md](design.md) — hub / site / cluster / app model  
- [fleet-site.md](components/fleet-site.md) — FCJ lifecycle  
- YAIDAP (external): execution engine for the `yaidap` connector  
