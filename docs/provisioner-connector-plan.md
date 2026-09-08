# Provisioner connector — implementation plan

> Status: **Phase 1–3 implemented**; **native Ansible/YAIDAP inventory tree** (root group files, `group_vars/`, `host_vars/`, scoped sync) landed in v0.2.
> OpenTofu/Terraform (phase 4) and hardening (phase 5) not done.
> Console Provision panel not shipped yet.
>
> Related: [design.md](design.md), [fleet-site.md](components/fleet-site.md),
> [design-provisioner-connector.md](design-provisioner-connector.md),
> YAIDAP at a sibling repo (`yaidap`) — metal / TempXE / HV / guest provisioner.

## Goal

Allow UMKIM to **optionally** provision bare-metal or hypervisor guests before
cluster enroll / add-worker, without merging YAIDAP (or Terraform/OpenTofu) into
the UMKIM codebase.

```text
UMKIM intent (site / host)
  → Provisioner connector (async hub job)
      → YAIDAP | OpenTofu | Terraform | noop
  → SSH-ready host (Rocky 10, IP, identity)
  → Existing path: FleetClusterJoin enroll / add-worker → Ready
```

Default remains **`noop`**: operators bring their own Rocky hosts (current v0.2).

## Non-goals (v1)

- One shared inventory schema between UMKIM and YAIDAP.
- UMKIM owning kickstart, PXE, bridges, disks, or HV prepare details.
- Blocking FleetClusterJoin reconcile on PXE arming internals.
- Merging the YAIDAP git tree into UMKIM.
- Multi-provider scheduling / placement engine.

## Principles

1. **Layers stay separate** — YAIDAP (or tofu) = L0 iron→OS; UMKIM = L1 OS→fleet.
2. **Small contract** — provider-agnostic request/result; adapters map to backends.
3. **Site-scoped provision config** — jumphost + inventory/workspace pointer on the
   site (or connector binding), not a second full hub inventory.
4. **Same job patterns as today** — hub Ansible/runner jobs, mutual exclusion with
   node/storage/KubeVirt ops where needed, status + log tail in history.
5. **Safety** — TempXE / destructive installs require explicit confirm; never
   auto-arm PXE from a casual UI click without the same guards YAIDAP already has.

## Compatibility with YAIDAP

| YAIDAP | UMKIM connector |
|--------|-----------------|
| `inventory/<site>/` (or one tree with clear site root) | Site stores `inventoryRef` / path on jumphost |
| Jumpserver / controller for that site | `jumphost` (SSH) in site provision config |
| `--limit <host>` + tags (`configure_tempxe`, `create`, …) | Action + subject → adapter argv |
| Discovery → `generated/host_vars` | Optional: import host fragment into enroll inputs |
| Out of scope: Kubernetes | UMKIM takes over only after SSH-ready |

Shared **handoff record** (minimal YAML/JSON both sides understand):

```yaml
hostname: virt02-vm01
ansible_host: 172.16.10.102
ansible_user: linux
osFamily: rocky   # rocky | proxmox | …
macs: []          # optional
siteId: homelab   # matches FleetSite / YAIDAP site id
```

Do **not** import YAIDAP `networks` / `uplinks` / `storages` / `disks` catalogs into
Fleet CRDs.

---

## Phase 0 — Spec freeze

**Deliverables**

- This plan accepted; OpenAPI / CRD field names locked for v1.
- Decision: where provision config lives:
  - **Recommended:** `FleetSite.spec.provision` (site-scoped jumphost + default provider).
  - Optional override on `FleetClusterJoin` / node-op body for one-shot actions.

**API sketch (hub, via fleet-site + console-auth)**

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/sites/{id}/provision` | Show binding (provider, jumphost, status) |
| `PUT`/`PATCH` | `/sites/{id}/provision` | Set provider + connection refs (no secrets in plain fields) |
| `POST` | `/sites/{id}/provision/actions` | Start action (`install_bm`, `create_vm`, `destroy`, `discover`, `status`) |
| `GET` | `/sites/{id}/provision/actions/{actionId}` | Poll phase / message / result host fragment |
| `GET` | `/sites/{id}/provision/actions` | Recent history |

**Action body (provider-agnostic)**

```json
{
  "action": "install_bm",
  "subject": "virt02",
  "confirm": true,
  "params": { "tags": ["configure_tempxe", "tempxe_start"] },
  "provider": "yaidap"
}
```

**Action phases:** `Pending` → `Running` → `Succeeded` | `Failed`  
**On Succeeded:** `result.host` = handoff record (enough for enroll / add-worker).

**CLI (sketch)**

```bash
umkim site provision get SITE
umkim site provision set SITE --provider yaidap --jumphost jump.example --inventory /path/on/jump
umkim site provision run SITE --action install_bm --subject virt02 --confirm --wait
umkim site provision status SITE ACTION_ID
```

**Console (sketch)**

- Site detail → **Provision** panel (hidden if provider `noop` / unset).
- Actions: discover / install BM / create VM (params minimal; advanced in YAML).
- After success: CTA **Enroll cluster** / **Add worker** with prefilled address.

---

## Phase 1 — Hub plumbing (noop + job skeleton)

**Status: done**

**Deliverables**

1. CRD / types: `FleetSite.spec.provision` (or `FleetProvisionBinding` CR if preferred).
2. Status: last action id, phase, message, optional result host.
3. Hub job runner path mirroring `configureKubevirt` / node-ops:
   - Annotation or status-driven request.
   - Job in `fleet-console` using `fleet-ansible-runner` **or** a thin “connector”
     container that only SSHes to the jumphost (decide in this phase).
4. Provider **`noop`**: action succeeds immediately with caller-supplied host
   fragment (or “already exists” check via SSH probe).
5. Auth: map `/sites/.../provision` to existing `sites` resource verbs.
6. Docs stub in [fleet-site.md](components/fleet-site.md) + this plan link.
7. Tests: API validation, noop happy path, mutual exclusion rules.

**Acceptance**

- Site with `provider: noop` can POST an action and reach `Succeeded` with a host
  fragment; no YAIDAP required.
- Console/CLI optional in this phase (API sufficient).

---

## Phase 2 — YAIDAP adapter (first real provider)

**Status: skeleton done** (job + playbook + mapping; lab handoff / host_vars parse TBD)

**Deliverables**

1. Provider id: `yaidap`.
2. Site provision config fields (example):

   | Field | Meaning |
   |-------|---------|
   | `provider` | `yaidap` |
   | `jumphost` | SSH target (or inventory host name resolved from hub) |
   | `sshSecretRef` | Hub secret: key to jumphost |
   | `inventoryPath` | Absolute path **on jumphost** to YAIDAP inventory root for this site |
   | `yaidapRoot` | Optional path to YAIDAP checkout on jumphost |
   | `defaultExtraVars` | Optional safe defaults (no passwords in CR) |

3. Action → YAIDAP playbook mapping (configurable table, defaults):

   | Action | Default YAIDAP invocation (illustrative) |
   |--------|------------------------------------------|
   | `discover` | `discover-hosts.yml --tags check_host_details --limit SUBJECT` |
   | `install_bm` | tempxe configure/start/stop sequence with `tempxe_confirm` when required |
   | `create_vm` | `provision-vms.yml --tags create --limit SUBJECT` |
   | `destroy` | destroy tags / playbooks with confirm |
   | `status` | `tempxe_status` / host ping |

4. Adapter runs **on/through jumphost** (not from hub with YAIDAP roles vendored).
5. Parse Succeeded → handoff record (from YAIDAP generated host_vars or ansible facts).
6. Safety: require `confirm: true` for armed TempXE / reinstall; surface YAIDAP
   arming report in action message/log tail.
7. Flow playbook: `playbooks/flows/site-provision-yaidap.yml` (identity headers).
8. Docs: operator runbook (jumphost prep, inventory layout, UMKIM site binding).
9. Lab test: one BM or nested fake-BM via existing YAIDAP lab inventory; then
   `umkim` enroll/add-worker using returned `ansible_host`.

**Acceptance**

- From UMKIM API: provision action on a site → YAIDAP runs on jumphost → host
  SSH-reachable → existing FCJ or add-worker completes without manual IP copy
  (beyond confirming the CTA).

---

## Phase 3 — Product surfaces

**Status: CLI done; console panel not started**

**Deliverables**

1. `umkim site provision …` CLI (list/get/set/run/wait).
2. Console Site → Provision panel + link into cluster create / add-worker.
3. Optional: cluster create wizard step “Provision nodes first” when site has
   non-noop provider.
4. History UX aligned with node-op / KubeVirt history (log tail).
5. Update [umkim-cli.md](components/umkim-cli.md), console README, [install.md](install.md)
   (“optional metal provision”).

**Acceptance**

- Operator can bind YAIDAP, manage connector inventory from console/CLI, sync to
  jumphost, run install/create, and enroll without manual jumphost file edits.

---

## Phase 4 — Second provider (OpenTofu / Terraform)

**Deliverables**

1. Provider `opentofu` (and/or `terraform`) behind the **same** action API.
2. Site config: workspace dir or git ref on jumphost/CI runner, backend config
   secret refs, `apply`/`destroy` mapping.
3. Result host from tofu **outputs** (`ansible_host`, etc.) — document required
   output names.
4. No YAIDAP-specific fields leaked into the generic API.

**Acceptance**

- Swap site `provider` to `opentofu`; same CLI/UI actions work for create/destroy
  with different params.

---

## Phase 5 — Hardening (as needed)

- Retry / cancel actions; idempotency keys.
- Stricter RBAC (provision vs sites update).
- Audit events for confirm-destructive actions.
- Capability discovery after provision (optional).
- Metrics: action duration, failure reasons.
- SELinux / TempXE confirm edge cases documented from lab.

---

## Suggested types (draft)

```go
// FleetSite.spec.provision (illustrative)
type SiteProvisionSpec struct {
    Provider string `json:"provider"` // noop | yaidap | opentofu | terraform
    // YaidaP / SSH jumphost backends
    Jumphost      string `json:"jumphost,omitempty"`
    SSHSecretRef  string `json:"sshSecretRef,omitempty"`
    InventoryPath string `json:"inventoryPath,omitempty"`
    YaidaPRoot    string `json:"yaidapRoot,omitempty"`
    // Tofu
    WorkspacePath string `json:"workspacePath,omitempty"`
}

type SiteProvisionStatus struct {
    Phase          string `json:"phase,omitempty"`
    Message        string `json:"message,omitempty"`
    LastActionID   string `json:"lastActionID,omitempty"`
    LastActionType string `json:"lastActionType,omitempty"`
}
```

Exact CRD placement can move to a dedicated `FleetProvisionAction` CR if actions
need richer lifecycle than site status history.

---

## Mutual exclusion / safety

- Provision actions that change metal/HV should **not** race FCJ provisioning or
  node replace on the same hostnames; document and enforce simple locks
  (per-site or per-subject).
- Reuse lessons from KubeVirt day-2: **do not treat an old Succeeded Job as
  completion of a new request** — delete/recreate or unique job names per action id.
- TempXE: never start armed PXE without confirm; always document `tempxe_stop`.

---

## Testing plan

| Phase | Test |
|-------|------|
| 1 | Unit/API: noop action; reject unknown provider |
| 2 | Lab: YAIDAP jumphost + demo/lab inventory; discover + create or BM install; SSH; enroll |
| 3 | Console/CLI smoke on lab site |
| 4 | Tofu workspace with fake outputs → handoff → noop enroll path |
| 5 | Confirm-required destructive action denied without flag |

Do not require full PXE bare-metal for every PR; YAIDAP nested fake-BM or VM create
is enough for CI-adjacent lab checks.

---

## Documentation / inventory boundaries

- Design: this file + short section in [design.md](design.md) (optional provisioner).
- Install: optional “bind YAIDAP jumphost” subsection — **no** lab IPs in design tier.
- YAIDAP remains documented in its own repo; UMKIM links out for TempXE details.
- Secrets: jumphost keys and tofu credentials only via Kubernetes secrets /
  [secrets.md](secrets.md) patterns.

---

## Open decisions (resolve in Phase 0)

1. **Job runtime:** extend `fleet-ansible-runner` vs dedicated `fleet-provision-connector` image.
2. **Config object:** embed on `FleetSite` vs separate CR.
3. **Where YAIDAP lives on jumphost:** git pull by connector vs operator-managed checkout.
4. **Whether `install_bm` is one UMKIM action or a guided multi-step** (configure → status → start → wait → stop) — recommend multi-step in UI, single composite action optional with strong confirm.
5. **Per-site YAIDAP inventory vs one inventory with site subgroups** — connector only needs a path + limit; either YAIDAP layout works.

---

## Implementation order (summary)

1. Phase 0 — approve contract and CR placement.  
2. Phase 1 — noop + hub job/API.  
3. Phase 2 — YAIDAP adapter + lab handoff to enroll.  
4. Phase 3 — CLI/console.  
5. Phase 4 — OpenTofu/Terraform.  
6. Phase 5 — harden.

## Success metric

An operator can, from UMKIM, optionally provision a site machine via YAIDAP (or later
tofu) and continue with the **unchanged** Ready-cluster / Apps / KubeVirt flows,
without embedding provisioner inventory schemas in Fleet CRDs.
