# Operational introduction strategy

> **Purpose:** End-to-end learning path from greenfield hub install through YAIDAP
> metal/VM provisioning, multi-cluster operations, GPU inference, and teardown.
> This is the **operational** companion to the TLDR
> [`umkim_yaidap_introduction.md`](umkim_yaidap_introduction.md).
>
> **Deliverable:** When executed, this strategy becomes an **introduction-style
> walkthrough** (narrative + commands + proof). Each stage below must be **documented**
> as you go—not only “what to run,” but **evidence that it worked** (see
> [Documenting each stage](#documenting-each-stage-procedure--evidence)).
>
> **Audience:** Operators with a lab like this repo’s homelab; inventory reference:
> [`yaidap-inventory-reference/`](yaidap-inventory-reference/)
> (jumphost, `172.16.10.0/24`, virt01–04, virt03 BM, and so on). Replace hostnames,
> IPs, DNS zones, and jumphost with your deployment values.
>
> **Conventions**
>
> | Marker | Meaning |
> |--------|---------|
> | **Console** | UMKIM console UI path (after `umkim login` or browser session) |
> | **CLI** | Equivalent `umkim` command |
> | **Verify** | Pass/fail checkpoint before continuing |
> | **Document** | Required artifact for this stage (screenshot and/or terminal output); see naming rules below |
>
> Canonical references: [install.md](../docs/install.md), [umkim-cli.md](../docs/components/umkim-cli.md),
> [yaidap-integration.md](../docs/components/yaidap-integration.md),
> [rebuild-homelab-cli.md](../examples/homelab/rebuild-homelab-cli.md).


> **Run 2 plan (next introducible demo):**  
> [`introduction-walkthrough-run2/PLAN.md`](introduction-walkthrough-run2/PLAN.md)  
> — Archive run 1; hub-only evidence at `/home/linux/umkim-demo-materials/introduction-run2/`;  
> bug workflow document→fix→rebuild/deploy→re-test; **do not start** Phase A/B until remaining
> issues (ACME/redirect, YAIDAP pubkey push, …) are fixed and go-ahead is given.

---

## Overview (story arc)

| Phase | Focus | Primary UI |
|-------|--------|------------|
| A1 | Hub install | Ansible + scripts |
| A2 | Hub LLDAP + Authelia (ingress auth) | Console or CLI |
| A3–A4 | First site + cluster on **existing Rocky 10** | **Console** |
| A5–A6 | **Chart library** → deploy app (hub-gateway); **A6b** hub **Authelia** middleware | Console + CLI |
| A7 | Teardown (app, cluster, site) | **Console** |
| B1+ | New site, port-forward, YAIDAP on jumphost | **CLI** (unless noted) |
| B2–B4 | virt03 reinstall; virt01/02 discovery; single-node via YAIDAP | CLI + **Console** (one cluster) |
| B5–B6 | HA cluster (`172.16.10.100` VIP) | CLI |
| B7 | HA site-routed app; **B7b** spoke **Authelia** on `port_forward` | CLI + **Console** |
| B8 | **Cluster ops** on HA (add/remove worker, replace control plane) | CLI (+ console where noted) |
| B9 | **KubeVirt + Multus L2** (platform, LAN `rocky-10-vm`, VM ops) | CLI + **Console** |
| B10–B11 | virt04 GPU passthrough VM; two GPU single-node clusters | CLI |
| B12–B13 | Hub LiteLLM + llama.cpp on both GPU clusters | CLI + optional console |

Estimated wall time (homelab): **multiple days** (HA Ready up to ~90m, BM install, GPU prep).

---

## Documenting each stage (procedure + evidence)

This file is a **strategy**; the published introduction is the **procedure you write
while following it**. Treat every subsection (A1, A2, … B13) as a mini-chapter:

1. **Procedure** — Short prose: goal, prerequisites, Console path and/or CLI commands
   (copy from this strategy, adjusted for your DNS/IPs). Number steps inside the chapter.
2. **Evidence** — At least one of the following per stage (prefer **both** for Console-heavy steps):
   - **Screenshot** — UI state that proves success (wizard finished, phase **Ready**, app URL, delete confirmation).
   - **Console output** — Redacted terminal log showing the command and the **Verify** checkpoint
     (exit code 0, expected phase, `curl` response snippet).

Do not advance to the next stage until **Verify** passes **and** the **Document** artifact exists.

### Logical naming

Store artifacts under a dedicated tree (recommended):

```text
demo_materials/introduction-walkthrough/
  assets/
    A1-verify-hub-install.txt
    ...
  bugs/
    BUG-20261001-B8c-cp-replace-timeout.md
  README.md                    # assembled introduction (links/embeds assets)
```

**File name pattern:** `{phase}{step}-{short-slug}.{ext}`

| Part | Rule | Examples |
|------|------|----------|
| Phase + step | Same id as this doc (`A1`, `B6`, …) | `A4`, `B13` |
| Slug | kebab-case, what the image/log proves | `cluster-ready`, `llm-chat-completion` |
| Extension | `.png` / `.webp` screenshots; `.txt` plain CLI; `.json` for `umkim … -o json` | `A5-whoami-curl.txt` |

For Console-only actions without a useful CLI transcript, **screenshot is mandatory**.
For long-running CLI (`install-hub.sh`, `install_bm`, `cluster wait`), capture **start + end**
(two files or one log with clear `=== Verify ===` section).

### Redaction

Scrub secrets from every artifact: bootstrap bundle paths, vault passwords, LLM API keys,
WireGuard configs, Authelia session cookies, and raw LDAP passwords. Replace with
`REDACTED` in text; blur or crop in screenshots.

### When something fails (bug log)

If **Verify** fails, behavior diverges from docs, or the UI/CLI errors unexpectedly,
**do not silently work around it** without recording a bug note. File one report per
issue under:

```text
demo_materials/introduction-walkthrough/bugs/
  BUG-20261001-A6-whoami-route-not-ready.md
```

**Naming:** `BUG-{YYYYMMDD}-{stage-id}-{short-slug}.md`

Each report must include:

| Field | Content |
|-------|---------|
| **Stage** | Strategy id (e.g. `A6`, `B7`) and walkthrough chapter title |
| **Environment** | Hub/spoke versions, `umkim` build, relevant cluster/site names |
| **Expected** | What should have happened (from docs or this strategy) |
| **Actual** | What happened (symptoms, phases stuck, HTTP codes, job failures) |
| **Reproduce** | Numbered steps; exact Console path or CLI commands; starting state |
| **Evidence** | Links to redacted logs/screenshots in `assets/` (`A6-failure-job-log.txt`) |
| **Workaround** | Optional; mark as temporary |
| **Follow-up** | GitHub issue URL or “fixed in commit …” when resolved |


**Run 2 requirement:** Do not leave the stage on a workaround. After filing the bug, **fix it, rebuild/deploy the fix, and re-test** until Verify passes. Goal is an introducible demo, not a terminal buglist. See [`introduction-walkthrough-run2/PLAN.md`](introduction-walkthrough-run2/PLAN.md).

Re-run the stage after a fix and add a **pass** artifact (`A6-whoami-curl.txt`) so the
introduction shows both failure (optional) and success.

See [fleet-site.md](../docs/components/fleet-site.md) (node operations) and
[fleet-apps.md](../docs/components/fleet-apps.md) (charts/apps) when filing upstream bugs.

### Assembling the introduction

The walkthrough `README.md` (or future `docs.umkim.com` page) should:

- Follow the same **A → B** order as this strategy.
- Embed or link each asset next to the step it proves.
- Keep the TLDR [`umkim_yaidap_introduction.md`](umkim_yaidap_introduction.md) as the
  front door; link to the full walkthrough for operators who want proof and click-paths.

**Document** checkpoints in phase sections below refer to this section; add a one-line
“Artifacts: `A3-site-create.png`, …” in each chapter as you execute the run.

---

## Phase A — Hub and first workload (console-first)

### A1 — Install the hub (step-by-step)

Follow [install.md](../docs/install.md). Summary:

1. **Controller packages** — Rocky 10, `ansible-core`, clone `umkim`, `ansible-galaxy collection install`.
2. **Inventory** — `./scripts/bootstrap-hub-inventory.sh`; fill `inventory/hosts.yml`, `host_vars/umkim-hub.yml`, `group_vars/all/site.yml`, vault; `./scripts/preflight-hub-inventory.sh`.
3. **Secrets** — `./scripts/generate-vault-secrets.sh --encrypt` or manual `ansible-vault encrypt`; `./scripts/preflight-v1.sh`.
4. **Install** — `./scripts/install-hub.sh` then `./scripts/verify-hub-install.sh`.
5. **WireGuard client** — `./scripts/wg-dev-client.sh <name>`; connect operator laptop to hub overlay.

**Verify:** `./scripts/verify-hub-install.sh` passes; console reachable on management path; `umkim login` (or browser sign-in) succeeds.

**CLI:** `umkim whoami` after login.

---

### A2 — Enable hub LLDAP + Authelia (ingress authentication)

Ingress auth (Authelia + LLDAP) is **separate** from console login. See
[fleet-ingress-auth.md](../docs/fleet-ingress-auth.md).

**Option 1 — at install (inventory):** set `fleet_ingress_auth_enabled: true` in
`inventory/group_vars/hub/main.yml` before `install-hub.sh` (greenfield template defaults off).

**Option 2 — after install:**

| Step | Console | CLI |
|------|---------|-----|
| Inspect | **Infrastructure** → ingress auth status | `umkim ingress-auth get` |
| Enable hub stack | **Infrastructure** → enable hub forward-auth (+ optional hub LLDAP Web UI) | `umkim ingress-auth set --hub-enabled --hub-lldap-webui` |
| Hub ACME (if needed for public hub routes) | **Infrastructure** → ACME | `umkim acme set --hub-enabled --email you@example.com` |

Directory bootstrap: [lldap-bootstrap.md](../docs/lldap-bootstrap.md).

**Verify:** `umkim ingress-auth get` shows hub enabled; hub Authelia/LLDAP pods healthy on hub (`kubectl get pods -A | grep -E 'authelia|lldap'`).

Hub forward-auth is required before **A6b** (hub-routed apps with `route.requireAuth: true`).
Spoke forward-auth on **`port_forward`** ingress is configured in **B7b** ([fleet-ingress-auth.md](../docs/fleet-ingress-auth.md) — not available on `reverse_proxy`).

**Document:** `A2-ingress-auth-get.json`, optional Authelia portal URL screenshot (redacted).

---

### A3 — Create a site with hub gateway (console)

Create a **registry** workload site whose apps can use `route.mode: hub`.

**Console:** **Sites** → **Create** → choose a profile with **hub gateway** (wizard:
`hub_gateway` / hub-routed DNS). Set site name (example: `intro-hub`), region, and hub zone
(wildcard DNS you control, e.g. `*.apps.example.com` → hub WAN).

**CLI equivalent (after Phase B you may prefer JSON files):**

```bash
# Example shape — adjust siteName, hubZone, hubWildcard to your DNS
cat > /tmp/site-intro-hub.json <<'EOF'
{
  "apiVersion": "fleet.umkim.com/v1alpha1",
  "kind": "FleetSite",
  "metadata": { "name": "intro-hub" },
  "spec": {
    "siteName": "intro-hub",
    "siteId": "intro-hub",
    "provisioningMode": "registry",
    "edge": {
      "hubGateway": true,
      "hubZone": "apps.example.com",
      "hubWildcard": "*.apps.example.com"
    }
  }
}
EOF
umkim site create -f /tmp/site-intro-hub.json
```

Reference: [examples/site-profiles/hub-gateway/](../examples/site-profiles/hub-gateway/).

**Verify:** `umkim site get intro-hub` shows `hubGateway: true`; DNS for hub zone points at hub WAN.

---

### A4 — Create a cluster on pre-existing Rocky Linux 10 (console)

Target: one node already installed (SSH reachable), **no YAIDAP** (`provisioner: noop`).

**Console:**

1. **Clusters** → **Create** → site `intro-hub`, topology **single node**.
2. Set **bootstrap host** = management IP of the Rocky 10 node.
3. Create → **download bootstrap bundle** (treat as secret).
4. On the node: extract bundle, `sudo ./bootstrap.sh`, wait until phase **Validated**.
5. **Provision** (console) or wait for hub finalize.

**CLI equivalent:**

```bash
# Minimal cluster spec — set bootstrapHost to your node IP
umkim cluster create -f - <<EOF
{
  "clusterName": "intro-single-0",
  "siteId": "intro-hub",
  "bootstrapHost": "REPLACE_MGMT_IP",
  "topology": "single_node",
  "networkMode": "wireguard"
}
EOF
umkim cluster wait intro-single-0 --phase ScriptIssued --timeout 15m
umkim cluster bootstrap intro-single-0 --bootstrap-host REPLACE_MGMT_IP --ssh-user linux
umkim cluster wait intro-single-0 --phase Validated --timeout 45m
umkim cluster provision intro-single-0
umkim cluster wait intro-single-0 --phase Ready --timeout 45m
```

**Verify:** `umkim cluster get intro-single-0` phase **Ready**; `kubectl get managedcluster intro-single-0` Available.

Optional spoke ingress auth (for `route.mode: site` later, not required for hub route):

```bash
umkim cluster ingress-auth enable intro-single-0 --lldap-password-file /path/to/lldap-user-secret
umkim cluster acme enable intro-single-0
```

---

### A5 — Add a basic Helm chart to the library

Validate **Apps → Library** / chart publishing before installing workloads. The hub
install seeds **`whoami`** in Harbor; this stage still exercises **registering** a chart
(OCI origin or `.tgz`) so the introduction covers add → deploy separately.

**Console:** **Apps** → **Library** → **Add chart** — OCI URL or upload `.tgz`, set name/version/image policy.

**CLI:**

```bash
umkim chart list
# Publish from maintainer machine (optional): ./scripts/fleet-chart.sh publish …
# Register in hub library (example — adjust origin to your registry):
umkim chart add -f examples/umkim/chart-add-oci.json
# Or upload a minimal custom chart:
umkim chart add --chart path/to/my-chart-0.1.0.tgz --name my-chart --version 0.1.0
umkim chart get whoami
```

**Verify:** `umkim chart list` shows the chart; console library lists the same name/version.

**Document:** `A5-chart-list.png` and/or `A5-umkim-chart-list.txt`.

If chart add fails, file a bug report ([When something fails](#when-something-fails-bug-log)).

See [fleet-apps.md](../docs/components/fleet-apps.md) and [umkim-cli.md](../docs/components/umkim-cli.md) (`chart add`).

---

### A6 — Deploy the chart to a cluster (hub-gateway routing, public)

Install **whoami** (or the chart added in A5) onto `intro-single-0` with hub routing **without**
forward-auth first (baseline reachability).

**Console:** **Apps** → **Install chart** → cluster `intro-single-0`, chart `whoami`, route mode **hub**,
set **hub host** if needed; leave **Require authentication** unchecked.

**CLI:**

```bash
umkim app create --cluster intro-single-0 --chart whoami --version 0.1.0 \
  --release whoami --route-mode hub --hub-host whoami.apps.example.com
```

**Verify:**

- `umkim app get whoami` → phase ready.
- `curl -sS https://whoami.apps.example.com/` returns whoami body **without** login (public route).

See [fleet-apps.md](../docs/components/fleet-apps.md) (`route.mode: hub` requires site `hubGateway`).

**Document:** `A6-app-get.json`, `A6-whoami-hub-curl.txt`.

---

### A6b — Test hub Authelia middleware (forward-auth)

Requires **A2** (hub ingress-auth enabled). Fleet-edge attaches Authelia **forward-auth**
middleware to hub Traefik when `route.requireAuth: true` and hub Authelia is ready.

**Console:** **Apps** → **Install chart** → same cluster, release e.g. `whoami-auth`, route **hub**,
enable **Require authentication** (maps to `route.requireAuth`).

**CLI** (JSON — CLI has no `--require-auth` flag today):

```bash
umkim app create -f - <<'EOF'
{
  "name": "whoami-auth",
  "clusterName": "intro-single-0",
  "chart": "whoami",
  "version": "0.1.0",
  "releaseName": "whoami-auth",
  "namespace": "default",
  "route": {
    "mode": "hub",
    "hubHost": "whoami-auth.apps.example.com",
    "requireAuth": true
  }
}
EOF
```

**Verify middleware behavior:**

| Check | Expected | Actual (record in walkthrough) |
|-------|----------|--------------------------------|
| Unauthenticated `curl -sS -o /dev/null -w '%{http_code}' https://whoami-auth.apps.example.com/` | Redirect to Authelia portal and/or **401/302** (not 200 with body) | … |
| Browser, private/incognito | Hitting app URL opens **Authelia** login; after LLDAP user login, whoami response visible | … |
| Authenticated session | Repeat `curl` with session cookie **or** browser already logged in → **200** + whoami body | … |
| Hub Traefik | Route exists with forward-auth middleware (redacted inspect or console route status) | … |

Use an ingress-auth LLDAP test user created per [lldap-bootstrap.md](../docs/lldap-bootstrap.md) (never paste passwords into docs).

**Document:** `A6b-curl-unauthenticated.txt`, `A6b-authelia-login.png`, `A6b-curl-authenticated.txt` (cookie redacted).

If the app stays public (200 without login), file a **bug report** (expected forward-auth, actual open route).

Delete `whoami-auth` in **A7** with `whoami`.

---

### A7 — Delete workload, cluster, and site (console)

Order: **apps → clusters → site**.

| Resource | Console | CLI |
|----------|---------|-----|
| App | **Apps** → select `whoami` / `whoami-auth` → **Delete** | `umkim app delete whoami whoami-auth` |
| Cluster | **Clusters** → `intro-single-0` → **Delete** (default detach; opt-in K3s cleanup in UI if offered) | `umkim cluster delete intro-single-0` or `--cleanup-kubernetes --wait` |
| Site | **Sites** → `intro-hub` → **Delete** (requires no remaining clusters) | `umkim site delete intro-hub --confirm` |

**Verify:** `umkim app list`, `umkim cluster list`, `umkim site list` no longer show deleted names.

**Document:** delete confirmations (`A7-delete-cluster.png`, …).

---

## Phase B — Homelab site with YAIDAP (CLI-first)

From here, default to **CLI** unless a step says **Console**.

Lab names below match the homelab site; inventory snapshot:
**[`yaidap-inventory-reference/`](yaidap-inventory-reference/)** site **`homelab`**, jumphost on **bck**,
BM **virt03** (`172.16.10.4`), hypervisors **virt01/02/04**, HA VIP **`172.16.10.100`**.

---

### B1 — Create site via CLI: port-forward ingress + jumphost

```bash
umkim site create -f examples/homelab/site-homelab.json
# Patch jumphost if needed:
umkim site patch homelab --jumphost linux@JUMPHOST:PORT
```

Add or adjust **port-forward** ingress targeting HA VIP (before HA exists, plan address `172.16.10.100`):

```bash
umkim site ingress list homelab
umkim site ingress add homelab --id homelab-ha-pf --method port_forward \
  --address 172.16.10.100 --zone homelab.example.com \
  --wildcard '*.homelab.example.com'
# Or patch existing row:
umkim site ingress set homelab homelab-pf --address 172.16.10.100
```

Configure WAN router: TCP 80/443 → `172.16.10.100:30080` / `:30443` (Traefik NodePorts on spoke).

**Verify:** `umkim site get homelab`; ingress row shows `172.16.10.100`.

---

### B2 — Configure YAIDAP integration on the jumphost

**Prerequisites on hub:** Secret `fleet-site-homelab-provision-ssh` in `fleet-console` (jumphost SSH key).

**CLI:**

```bash
umkim site provision set homelab --provider yaidap \
  --jumphost linux@JUMPHOST:PORT \
  --ssh-secret fleet-site-homelab-provision-ssh \
  --inventory /home/linux/yaidap/inventory/homelab \
  --yaidap-root /home/linux/yaidap \
  --yaidap-repo https://github.com/rborso/yaidap.git \
  --yaidap-ref main

umkim site provision yaidap-sync homelab --wait
```

**Configure inventory variables (site catalogs + host profiles):**

```bash
umkim site inventory migrate homelab
umkim site inventory files put homelab --path group_vars/all/all.yml \
  --file examples/homelab/inventory/group_vars/all/all.yml
# Put hypervisors.yml, bare_metals.yml, guests.yml, host_vars/*, cluster group files — see rebuild runbook
umkim site inventory sync homelab --wait
```

**Verify:** `umkim site provision get homelab`; sync phase **synced** in inventory `_meta/meta.json` (via API or console **Provisioner inventory**).

**Console (optional):** **Sites** → **homelab** → **Provisioner inventory** / **Sync to jumphost** — same store as CLI.

Full contract: [yaidap-integration.md](../docs/components/yaidap-integration.md).

---

### B3 — virt03: check, reinstall bare metal, verify

**Pre-check (SSH from jumphost or hub job):**

```bash
umkim site host get homelab virt03
ssh -J linux@JUMPHOST linux@172.16.10.4 'cat /etc/redhat-release; hostname'
```

**Reinstall flow:**

```bash
umkim site host provision homelab virt03 --action discover --wait
# PXE path if no SSH: discover_pxe --confirm (tempxe_discovery + MAC in host_vars)

umkim site inventory import preview homelab --wait
umkim site inventory import apply homelab --confirm \
  --paths host_vars/virt03.yml,bare_metals.yml

umkim site host provision homelab virt03 --action install_bm --confirm --wait
```

**Verify reinstall:**

```bash
ssh -J linux@JUMPHOST linux@172.16.10.4 'sudo cat /etc/redhat-release; lsblk'
umkim site host get homelab virt03   # provision phase succeeded
```

---

### B4 — virt01 and virt02: MAC-only stubs → discovery → hypervisors

1. **Console** or CLI: add hypervisor stubs with MAC only (`tempxe_discovery`, uplink MAC) — templates in console **Add hypervisor** or `inventoryVmTemplates` patterns in [yaidap-integration.md](../docs/components/yaidap-integration.md).
2. Sync inventory, then discover each host:

```bash
umkim site inventory sync homelab --wait
umkim site host provision homelab virt01 --action discover_pxe --confirm --wait
umkim site host provision homelab virt02 --action discover_pxe --confirm --wait
umkim site inventory import preview homelab --wait
umkim site inventory import apply homelab --confirm
umkim site host prepare homelab virt01 --wait
umkim site host prepare homelab virt02 --wait
```

**Verify:** `virt01`/`virt02` in `hypervisors.yml`; SSH from jumphost; `virsh list` shows no errors.

---

### B5 — Single-node cluster via console + YAIDAP provision

**Console-driven path (recommended for this one cluster):**

1. Ensure **virt03** in hub inventory / `bare_metals.yml` and cluster group intent if using automation.
2. **Sites** → **homelab** → **Hosts** → **virt03** → provision actions through **Ready** / cluster automation (**Enroll** single-node) if enabled.
3. Alternatively: create cluster **lab-single-0** in **Clusters** → **Create**, bootstrap host `172.16.10.4`, download bundle only if not using full automation.

**CLI mirror (same cluster):**

```bash
umkim cluster create -f examples/homelab/cluster-lab-single-0.json
umkim cluster bootstrap lab-single-0 --bootstrap-host 172.16.10.4 --ssh-user linux \
  --jumphost linux@JUMPHOST:PORT
umkim cluster wait lab-single-0 --phase Validated --timeout 45m
umkim cluster provision lab-single-0
umkim cluster wait lab-single-0 --phase Ready --timeout 45m
```

**Verify:** `umkim cluster get lab-single-0` **Ready**; ingress `homelab-pf` if bound.

---

### B6 — HA cluster via CLI (`least-utilized`, VIP 172.16.10.100)

Ensure guest definitions use `vm_location: least-utilized` in `guests.yml` (see
[`yaidap-inventory-reference/guests.yml`](yaidap-inventory-reference/guests.yml)).

**Create VMs and apply cluster inventory:**

```bash
for h in lab-ha-0-cp1 lab-ha-0-cp2 lab-ha-0-cp3 lab-ha-0-w1; do
  umkim site host provision homelab "$h" --wait
  umkim site host cluster-apply homelab "$h" || true
done
```

**Create cluster** (VIP must match port-forward target):

```bash
umkim cluster create -f examples/homelab/cluster-lab-ha-0.json
# controlPlaneVip: 172.16.10.100 in JSON; patch ingress:
umkim cluster patch lab-ha-0 --ingress-id homelab-ha-pf
umkim cluster bootstrap lab-ha-0 --bootstrap-host 172.16.10.101 --ssh-user linux \
  --jumphost linux@JUMPHOST:PORT
umkim cluster wait lab-ha-0 --phase Validated --timeout 45m
umkim cluster provision lab-ha-0
umkim cluster wait lab-ha-0 --phase Ready --timeout 90m
```

**Verify:** `curl -k https://172.16.10.100:6443/healthz` from jumphost (or API reachability); Traefik NodePorts on VIP.

---

### B7 — HA workload and site routing check (public)

Cluster must use **`port_forward`** ingress for spoke Authelia in **B7b** (`homelab-ha-pf` →
`172.16.10.100`, not `edge-rp` / `reverse_proxy`).

```bash
umkim cluster patch lab-ha-0 --ingress-id homelab-ha-pf
```

**CLI:**

```bash
umkim app create --cluster lab-ha-0 --chart whoami --version 0.1.0 \
  --release whoami-site --route-mode site --site-host whoami.homelab.example.com
```

**Verify:** URL via port-forward / DNS to `172.16.10.100`; pod running; **public** whoami body without login.

**Document:** `B7-whoami-site-curl.txt`.

---

### B7b — Test spoke Authelia middleware (`port_forward` + `route.mode: site`)

Spoke forward-auth runs on the cluster Traefik for **`port_forward`** site ingress only
([fleet-ingress-auth.md](../docs/fleet-ingress-auth.md)).

**Enable spoke ingress-auth stack:**

**Console:** **Clusters** → **lab-ha-0** → **Ingress authentication** → enable (LLDAP password from secret).

**CLI:**

```bash
umkim cluster ingress-auth enable lab-ha-0 --lldap-password-file /path/to/lldap-user-secret
umkim cluster acme enable lab-ha-0
umkim cluster get lab-ha-0   # wait until ingress-auth / ACME phases ready
```

**Deploy protected app:**

**Console:** **Apps** → **Install chart** → `whoami-site-auth`, route **site**, **Require authentication**.

**CLI:**

```bash
umkim app create -f - <<'EOF'
{
  "name": "whoami-site-auth",
  "clusterName": "lab-ha-0",
  "chart": "whoami",
  "version": "0.1.0",
  "releaseName": "whoami-site-auth",
  "route": {
    "mode": "site",
    "siteHost": "whoami-auth.homelab.example.com",
    "requireAuth": true
  }
}
EOF
```

**Verify middleware (same matrix as A6b):**

- Unauthenticated access to `https://whoami-auth.homelab.example.com/` → Authelia challenge (not anonymous whoami).
- After LLDAP login → whoami response.
- Confirm **`homelab-ha-pf`** / VIP `172.16.10.100` still forwards 80/443 to spoke Traefik NodePorts.

**Document:** `B7b-spoke-authelia-unauth.txt`, `B7b-spoke-authelia-auth.png`.

Optional: compare **`reverse_proxy`** ingress (`edge-rp`) — document that **requireAuth** on site routes
is **not** supported there (expectation for introduction readers).

---

### B8 — Cluster lifecycle operations (`lab-ha-0`)

Exercise post-**Ready** node APIs on the 3-node HA cluster (after B6). Prepare replacement
capacity via YAIDAP (new guest + `host_vars`, sync, `create_vm`) before add/replace when
nodes are libvirt guests.

Reference: [fleet-site.md](../docs/components/fleet-site.md) (node operations),
[cluster-node-op.yml](../playbooks/flows/cluster-node-op.yml).

#### B8a — Add a worker

**Console:** **Clusters** → **lab-ha-0** → **Operations** → add worker (address/hostname).

**CLI:**

```bash
umkim node list --cluster lab-ha-0
# After guest OS is SSH-ready on the lab LAN:
umkim node add --cluster lab-ha-0 --address 172.16.10.142 --hostname lab-ha-0-w2
umkim node-op history --cluster lab-ha-0
umkim cluster wait lab-ha-0 --phase Ready --timeout 90m
```

**Verify:** New node Ready in `umkim node list`; workload pods can schedule on worker.

**Document:** `B8a-node-list.txt`, job log if queued.

#### B8b — Remove a worker

Remove a non-critical worker (e.g. `lab-ha-0-w2` from B8a, or `lab-ha-0-w1` if reprovisioning).

**Console:** **Clusters** → **lab-ha-0** → remove worker (confirm drain behavior).

**CLI:**

```bash
umkim node remove --cluster lab-ha-0 --node lab-ha-0-w2
# If node unreachable:
umkim node remove --cluster lab-ha-0 --node lab-ha-0-w2 --force
umkim node-op history --cluster lab-ha-0
```

**Verify:** Node gone from `umkim node list`; cluster phase **Ready**; HA control plane intact.

**Document:** `B8b-node-remove-history.txt`.

#### B8c — Replace a control-plane member

Replace one control-plane node (example: swap `lab-ha-0-cp3` for a newly provisioned guest).

**Console:** **Clusters** → **lab-ha-0** → replace control plane (old node + new address).

**CLI:**

```bash
umkim node replace --cluster lab-ha-0 --old-node lab-ha-0-cp3 \
  --address 172.16.10.103 --force
umkim node-op history --cluster lab-ha-0
umkim cluster wait lab-ha-0 --phase Ready --timeout 90m
```

**Verify:** etcd/quorum healthy; VIP `172.16.10.100` still reachable; `kubectl get nodes` on spoke.

**Document:** `B8c-cp-replace-history.txt`, before/after `umkim cluster get lab-ha-0`.

If any step fails (stuck job, wrong inventory, drain timeout), file a **bug report**
with expected vs actual and reproduction ([When something fails](#when-something-fails-bug-log)).

---

### B9 — KubeVirt platform and Multus L2 VM testing (`lab-ha-0`)

Validate the optional spoke **platform** (Multus + CDI + KubeVirt) and a **LAN L2**
guest on the same bridge/NAD as node management (`default/mgmt-l2`, host bridge
default `n0p0_mgmt_nw`). Use cluster **`lab-ha-0`** (Longhorn from B6) unless noted.

Reference: [fleet-kubevirt.md](../docs/components/fleet-kubevirt.md),
[fleet-apps.md](../docs/components/fleet-apps.md) (`rocky-10-vm`),
[charts/rocky-10-vm/README.md](../charts/rocky-10-vm/README.md).

#### B9a — Enable KubeVirt with LAN Multus bridge

Prefer **day-0** bridge when creating HA clusters (optional patch before bootstrap):

```json
"providers": {
  "install": {
    "kubevirt": true,
    "k8s": false,
    "helm": false
  },
  "kubevirtBridge": {
    "enabled": true,
    "bridgeName": "n0p0_mgmt_nw"
  }
}
```

**Day-2** (cluster already **Ready** from B6):

**Console:** **Clusters** → **lab-ha-0** → **Operations** → **Enable KubeVirt** with
**LAN Multus bridge** (or enable bridge after KubeVirt is installed).

**CLI:**

```bash
umkim cluster platform enable-kubevirt lab-ha-0 --lan-bridge --bridge n0p0_mgmt_nw --wait 45m
umkim cluster platform status lab-ha-0
umkim cluster candidates --require kubevirt,network.multus_l2 --site homelab
```

**Verify:**

- `umkim cluster get lab-ha-0` — KubeVirt provider phase installed.
- Labels/capabilities: `fleet.capability/kubevirt`, `fleet.capability/network.multus_l2`.
- On a spoke node: NAD `default/mgmt-l2` exists; bridge `n0p0_mgmt_nw` carries management L2.

**Document:** `B9a-platform-status.json`, `B9a-multus-nad.txt` (redacted `kubectl get net-attach-def`).

Day-2 LAN bridge **rewires** management onto the bridge; plan maintenance window. Failures
(Calico autodetection, DHCP `/32` flush) → bug report with node symptoms.

#### B9b — Pod-network smoke VM (optional baseline)

**Console:** **Apps** → **Install chart** → `rocky-10-vm`, `route.mode: none`, default
`network.mode: pod`.

**CLI:**

```bash
umkim app create --cluster lab-ha-0 --chart rocky-10-vm --version 0.1.0 \
  --release rocky-kv-pod --namespace kubevirt \
  --route-mode none \
  --values-file /tmp/rocky-kv-pod-values.yaml
# values: cpu, memory, disk.storageClass: longhorn — see chart README
```

**Verify:** `umkim vm list --cluster lab-ha-0`; guest Running; serial console login.

**Document:** `B9b-vm-list.txt`, console screenshot.

#### B9c — Multus L2 VM on lab LAN

Install **`rocky-10-vm`** with **`network.mode: lan`** and static IP on `172.16.10.0/24`
(DHCP/static outside Kubernetes — empty NAD IPAM).

Example values file:

```yaml
cpu: 2
memory: 4Gi
disk:
  size: 40Gi
  storageClass: longhorn
  volumeMode: Filesystem
network:
  mode: lan
  multusNetwork: default/mgmt-l2
  staticIP: 172.16.10.150/24
  gateway: 172.16.10.1
  dns:
    - 172.16.10.1
```

**Console:** **Apps** → **Install chart** → `rocky-10-vm` → set LAN / static IP fields.

**CLI:**

```bash
umkim app create --cluster lab-ha-0 --chart rocky-10-vm --version 0.1.0 \
  --release rocky-kv-lan --namespace kubevirt \
  --route-mode none \
  --values-file /tmp/rocky-kv-lan-multus.yaml
```

**Verify:**

- `umkim vm list --cluster lab-ha-0` — VM Running.
- From jumphost: `ping 172.16.10.150` (or SSH after cloud-init).
- **Console:** **Clusters** → **lab-ha-0** → **Virtual machines** → serial console.
- **CLI:** `umkim vm console rocky-kv-lan --cluster lab-ha-0`

**Document:** `B9c-ping-lan-vm.txt`, `B9c-vm-console.png`.

#### B9d — VM power operations (KubeVirt API path)

**Console:** **Virtual machines** → stop / start / restart (soft or hard).

**CLI:**

```bash
umkim vm stop rocky-kv-lan --cluster lab-ha-0
umkim vm start rocky-kv-lan --cluster lab-ha-0
umkim vm restart rocky-kv-lan --cluster lab-ha-0 --force
```

**Verify:** VM phase transitions; L2 IP still reachable after start when using static IP.

**Document:** `B9d-vm-power-history.txt`.

Clean up test FleetApps when done (`umkim app delete rocky-kv-lan rocky-kv-pod`) unless
the introduction keeps them as proof.

---

### B10 — virt04 as hypervisor + GPU passthrough VM `lab-gpu-pt-0`

Follow [`examples/homelab/lab-gpu-passthrough-cli.md`](../examples/homelab/lab-gpu-passthrough-cli.md).

Summary:

```bash
# Promote virt04 (172.16.10.6), VFIO prepare, create guest lab-gpu-pt-0-cp1
umkim site inventory files put homelab ...   # hypervisors, host_vars, guests, lab-gpu-pt-0.yml
umkim site inventory sync homelab --wait
umkim site host prepare homelab virt04 --gpu-passthrough --confirm --wait
umkim site host provision homelab lab-gpu-pt-0-cp1 --action create_vm --wait
```

**Verify:** GPU visible inside guest (`nvidia-smi` after OS install); cluster name **`lab-gpu-pt-0`**.

---

### B11 — Provision two single-node GPU clusters (virt03 + lab-gpu-pt-0)

| Cluster | Node | Example JSON |
|---------|------|----------------|
| **lab-single-0** (virt03, GTX 1050 Ti) | `172.16.10.4` | `cluster-lab-single-0.json` |
| **lab-gpu-pt-0** (passthrough VM) | guest IP e.g. `172.16.10.141` | `cluster-lab-gpu-pt-0.json` |

```bash
# virt03 cluster — if not already Ready from B5, (re)provision
umkim cluster create -f examples/homelab/cluster-lab-single-0.json
# ... bootstrap / provision / wait Ready

umkim cluster create -f examples/homelab/cluster-lab-gpu-pt-0.json
umkim cluster bootstrap lab-gpu-pt-0 --bootstrap-host 172.16.10.141 \
  --jumphost linux@JUMPHOST:PORT --ssh-user linux
umkim cluster provision lab-gpu-pt-0
umkim cluster wait lab-gpu-pt-0 --phase Ready --timeout 120m
```

**Verify:** both clusters **Ready**; `umkim cluster list -o wide`.

---

### B12 — Hub LiteLLM proxy

**CLI:**

```bash
umkim llm set --hub-enabled --public-host llm.example.com
umkim llm reconcile --wait --timeout 600
```

**Console:** **Infrastructure** → **LLM platform** → enable hub LiteLLM.

Ansible alternative: [fleet-llm.md](../docs/components/fleet-llm.md) (`fleet-llm.yml`).

**Verify:** `umkim llm get`; LiteLLM pod on hub; optional `scripts/verify-fleet-llm.sh`.

Create API key for tests:

```bash
umkim auth llm-api-keys create --name intro-lab --display-name "Intro lab"
```

---

### B13 — GPU prepare + llama.cpp on both GPU clusters; test inference

On **each** GPU cluster (`lab-single-0`, `lab-gpu-pt-0`):

```bash
umkim cluster gpu host-prep CLUSTER --wait 45m
umkim cluster gpu enable CLUSTER --wait 30m
umkim cluster gpu discover CLUSTER
umkim cluster gpu status CLUSTER   # expect fleet.capability/gpu=true
```

Install llama.cpp FleetApp (register backend for LiteLLM):

```bash
umkim app create --cluster lab-single-0 --chart llama-cpp-helm --version 0.1.4 \
  --llama-profile qwen35-2b-ud-4g --namespace llm --release llama-cpp --name llama-virt03

umkim app create --cluster lab-gpu-pt-0 --chart llama-cpp-helm --version 0.1.4 \
  --llama-profile qwen35-2b-ud-4g --namespace llm --release llama-cpp --name llama-gpu-pt
```

**Console:** **Apps** → Install → llama profile picker; enable **Register in hub LiteLLM proxy**.

Sync backends:

```bash
umkim llm backends sync-from-apps
umkim llm reconcile --wait
```

**Verify inference:**

```bash
# Hub OpenAI-compatible API (use key from B12)
curl -sS "https://llm.example.com/v1/chat/completions" \
  -H "Authorization: Bearer $UMKIM_LLM_KEY" \
  -H "Content-Type: application/json" \
  -H "x-litellm-session-id: intro-test-1" \
  -d '{"model":"Qwen3.5-2B-UD","messages":[{"role":"user","content":"Say hello in one word."}]}'
```

Optional scripts: `scripts/verify-llm-inference-e2e.sh`, [fleet-llm.md](../docs/components/fleet-llm.md).

---

## Automation shortcut

Maintainers can run the homelab rebuild script (overlaps B2–B6):

```bash
./scripts/lab/rebuild-homelab-cli.sh
```

See [`examples/homelab/rebuild-homelab-cli.md`](../examples/homelab/rebuild-homelab-cli.md) for phases, env vars, and logs.

---

## Related documentation map

| Topic | Document |
|-------|----------|
| TLDR intro | [`umkim_yaidap_introduction.md`](umkim_yaidap_introduction.md) |
| Hub install | [install.md](../docs/install.md) |
| CLI reference | [components/umkim-cli.md](../docs/components/umkim-cli.md) |
| YAIDAP integration | [components/yaidap-integration.md](../docs/components/yaidap-integration.md) |
| Console UI | [`ui/umkim-console/README.md`](../ui/umkim-console/README.md) |
| Apps / routing | [components/fleet-apps.md](../docs/components/fleet-apps.md) |
| Node operations | [components/fleet-site.md](../docs/components/fleet-site.md) (add/remove worker, replace CP) |
| GPU | [components/gpu-host-prep.md](../docs/components/gpu-host-prep.md) |
| LLM | [components/fleet-llm.md](../docs/components/fleet-llm.md) |
| Verification | [fleet-verify.md](../docs/fleet-verify.md) |
| Ingress auth / Authelia | [fleet-ingress-auth.md](../docs/fleet-ingress-auth.md) |
| KubeVirt / Multus L2 | [components/fleet-kubevirt.md](../docs/components/fleet-kubevirt.md) |

---

## Gaps to fill as you execute this strategy

Track these while turning the strategy into the published **introduction walkthrough**:

1. **Per-stage artifacts** — Every A/B step has a named screenshot and/or log per [Documenting each stage](#documenting-each-stage-procedure--evidence); none skipped.
2. **Bug reports** — Failures during the walkthrough documented under `introduction-walkthrough/bugs/` (expected vs actual, reproduce steps).
3. **Exact hub zone / DNS** values for Phase A vs homelab zones in Phase B (record in procedure prose).
4. **Network diagram** (draw.io) — asset e.g. `assets/network-topology.png`, linked from TLDR intro when ready.
5. **Timing table** per step (from `logs/homelab-rebuild-*.summary.tsv` or your own `A1-duration.txt` notes).
6. **Failure rollback** (detach vs `--cleanup-kubernetes`) documented in delete chapters (A7) with evidence screenshots.
7. **Console click-path** — Where Phase A is UI-first, screenshots must show navigation (Sites → Create, Apps → Library → Add chart, Apps → Install, deletes).
8. **Cluster ops evidence** — B8 add/remove/replace: capture `node-op history` and cluster phase after each operation.
9. **KubeVirt / Multus L2** — B9: platform status, `network.multus_l2` capability, LAN VM ping/console proof; document bridge/NAD names used on your nodes.
10. **Authelia middleware** — A6b (hub `route.mode: hub` + `requireAuth`) and B7b (spoke `port_forward` + `requireAuth`): unauthenticated vs authenticated evidence; bug log if route stays public.
