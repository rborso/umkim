# Introduction walkthrough

> **Run 1 archive (complete).** Keep this tree for comparison.
> Next run plan: [`../introduction-walkthrough-run2/PLAN.md`](../introduction-walkthrough-run2/PLAN.md).
> Hub-only evidence for run 2 (not in git): `/home/linux/umkim-demo-materials/introduction-run2/`.
> **Do not start run 2** until that plan’s remaining issues are fixed and go-ahead is given.


Greenfield run started from operational-introduction-strategy.md (full hub wipe + reinstall from A1).

Artifacts: `assets/`, bugs: `bugs/`.

## A1 progress
- Teardown completed (hub wipe). See `assets/A1-teardown-hub.txt`, `A1-teardown-verify.txt`.
- Bug: `bugs/BUG-20261001-A1-teardown-stuck-delete-jobs.md`.
- Next: install-hub → verify → wg-dev-client qa-tester.

## A1 complete (2026-10-01)
- teardown + install-hub + verify-hub-install OK
- Bugs: teardown stuck-delete-jobs; console InvalidImageName float tag (template quoted)
- WG client `qa-tester` → 10.10.0.100

## A2–A4 complete (2026-10-01)
- A2: hub ingress-auth enabled (`auth.demo.otthol.hu`)
- A3: site `intro-hub` Ready (zone `demo.otthol.hu`)
- A4: cluster `intro-single-0` on virt03 `172.16.10.4` → Ready (`upstream_host` 10.10.0.10)
- Bootstrap from QA WG client cannot reach jumphost:30200; run bootstrap from hub (see `A4-bootstrap-qa-jumphost-timeout.txt`)
- Bug: DKMS kernel headers during bootstrap (`bugs/BUG-20261001-A4-dkms-kernel-headers-missing.md`)
- Next: A5 chart library → A6 whoami hub route → A6b Authelia → A7 teardown

## A5–A6b complete (2026-10-01)
- A5: whoami in library (Harbor seed; `chart add` example OCI overwrote then Failed — restored from Harbor OCI). Bug: `bugs/BUG-20261001-A5-chart-add-example-oci-overwrites-harbor.md`
- A6: `whoami` hub route `whoami.demo.otthol.hu` Ready; HTTPS 200 whoami body (`A6-whoami-hub-curl.txt`). Prefer `curl -4` (IPv6 connect issues from hub).
- A6b: `whoami-auth` requireAuth; unauth 302→`auth.demo.otthol.hu`; auth session 200 (`A6b-curl-*.txt`).
- Bug: FleetAppRoute Degraded probing 127.0.0.1 (`bugs/BUG-20261001-A6-fleetapproute-probe-127.0.0.1.md`)
- CLI notes: use `--route-host` not `--hub-host`; no `umkim app wait`.
- Next: A7 teardown apps → cluster → site, then Phase B.
## A7 complete (2026-10-01)
- Apps deleted; cluster delete needed ManifestWork/finalizer cleanup + spoke k3s uninstall.
- Site intro-hub deleted (--confirm). virt03 k3s gone.
- Next: Phase B homelab + YAIDAP.

<!-- B3-STATUS -->
## Phase B status (live)

- **B1** Site `homelab` Ready; ingress `homelab-pf` → `172.16.10.100`; zone `homelab.otthol.hu`. Added `homelab-ha-pf` (same VIP) bound to lab-ha-0.
- **B2** YAIDAP provision bound; inventory restored from reference (guest host_vars/cluster groups completed in B6).
- **B3** virt03 discover Succeeded; install_bm Failed/skipped (no UEFI PXE). See bugs.
- **B4** virt01/virt02: discover_pxe **SKIPPED** (full host_vars + SSH/virsh). `prepare` **Ready** both (parallel CLI FleetSite conflict — bug; Ansible OK). Assets `B4-*`.
- **B5** lab-single-0 on virt03 — **Ready**.
- **B6** lab-ha-0 — **Ready** (`available=true`, ha_3_node, upstream_host `10.10.0.12`). Guests Ready on virt01/virt02 (pinned after least-utilized placement failure). Bootstrap from hub OK; Validated→provision→Ready (pipeline complete 2026-10-02 00:10 CEST). Ingress `homelab-ha-pf`. VIP :6443 anonymous healthz returns 401 (note); Traefik :30080 responds.
- **B7** `whoami-site` Ready on lab-ha-0; siteHost `whoami.homelab.otthol.hu`. HTTPS 200 whoami via VIP/`--resolve :30443:10.10.0.12` (`B7-whoami-site-curl.txt`). Public DNS :443 hits hub Traefik 404 (bug).
- **B7b** Spoke ingress-auth Ready; ACME enable then briefly disable/re-enable around queue unblock. `whoami-site-auth` requireAuth Ready. Unauth 302→`auth.homelab.otthol.hu`; auth session 200 whoami (`B7b-spoke-authelia-unauth.txt`, `B7b-spoke-authelia-auth.txt`). Spoke ACME staging queue blocked second route (bug). VIP :30443 still forwards.
- **B8** Cluster lifecycle on lab-ha-0 (2026-10-02 CEST):
  - **B8a** Added worker `lab-ha-0-w2` @ `172.16.10.142` (create_vm on virt02, pinned). Required manual inject of hub fleet SSH pubkey (bug). Ansible join OK; node Ready. node-op history/post-validation raced to Failed while node Ready (bug). Disabled `lab-ha-0-w1` autoApply storm first.
  - **B8b** Removed `lab-ha-0-w2` (drain/delete OK; cluster Ready; CP intact). History table showed Failed while live nodeOp Succeeded (same race/display bug).
  - **B8c** Provisioned temp CP `lab-ha-0-cp4` @ `172.16.10.143` on virt01; graceful replace of `lab-ha-0-cp3` reported Succeeded but **old CP leave skipped** (`no hosts matched`) — cluster now **4 Ready CP/etcd** (cp1–cp4). VIP :6443 anonymous healthz 401. Did not force-retry remove of cp3 (risk). See bug.
- Next after B11 was B12–B13 (done below).
- Bugs: B9a inventory/CNI/discover; B9b longhorn missing + 2cpu unschedulable; B4 concurrent prepare; B6 missing guest host_vars; B6 placement oversubscribed; B6 create_vm recreated guests SSH; B7 homelab DNS→hub Traefik; B7b spoke ACME queue stuck PendingStaging; B8 create_vm missing fleet SSH key; B8 node-op post-validation race; B8 w1 autoApply storm; B8c replace skips old-node leave.

## B9 — KubeVirt + Multus L2 (lab-ha-0) — 2026-10-02 CEST

**B9a** enable-kubevirt --lan-bridge --bridge n0p0_mgmt_nw → platform Ready (LAN Multus NAD).
Bridge: strategy name n0p0_mgmt_nw (on virt03; day-2 created on all CP VMs cp1–cp4).
Caps after rediscovery: kubevirt=true, network.multus_l2=true. Candidates lists lab-ha-0.
NAD default/mgmt-l2 → bridge n0p0_mgmt_nw.

**B9b** rocky-kv-pod Running on cp1 (pod IP 192.168.140.206; route.mode none).
**B9c** rocky-kv-lan Running on cp2; static 172.16.10.150/24 Multus L2 — ping OK from CP nodes
after guest NetworkManager settle (~3–4m). SSH password auth (chart default) not exercised
(no sshpass); serial console available via umkim vm console -n kubevirt.

**Deviations / bugs filed**
- Inventory omitted cp3 / included stale lab-ha-0-w1 (BUG-20261002-B9a-kubevirt-inventory-stale-w1-missing-cp3)
- bridge CNI install missing mkdir -p /opt/cni/bin (BUG-20261002-B9a-bridge-cni-missing-mkdir)
- discover stale Succeeded / stuck job annotation (BUG-20261002-B9a-discover-stale-succeeded-no-job)
- No Longhorn SC; only local-path (BUG-20261002-B9b-longhorn-sc-missing)
- Strategy cpu:2/mem:2Gi unschedulable on 2-vCPU nodes; used cpu:1 mem:1Gi disk:20Gi local-path
  (BUG-20261002-B9b-vm-unschedulable-2cpu)

**Assets:** B9a-*, B9b-*, B9c-*.

## B10 — virt04 GPU passthrough + VM lab-gpu-pt-0-cp1 — 2026-10-02 CEST

**Inventory:** put `host_vars/virt04.yml`, `host_vars/lab-gpu-pt-0-cp1.yml`, `lab-gpu-pt-0.yml`,
`group_vars/lab-gpu-pt-0.yml`; hypervisors already listed virt04; sync OK.
**Prepare:** `prepare --gpu-passthrough --confirm` started `prepare_hypervisor_gpu` → Succeeded
(VFIO bind 01:00.0/01:00.1 GTX 1650); CLI `--wait` returned HTTP 404 (bug). Follow-up
`prepare_hypervisor` also Succeeded; virt04 **Ready**.
**Leftovers:** uninstalled orphaned k3s+wg on virt04 (old lab-gpu-0); set
`bridge-nf-call-iptables=0` after jumphost→guest Packet filtered (bug).
**create_vm:** re-run Failed `Domain is already active` (bug); retry job reimaged guest
(fresh Rocky, UUID changed). GPU hostdev still attached. Disk grown 40→50G for validate.
**nvidia-smi:** worked on pre-reimage guest (driver 615.71.09, GTX 1650); after reimage
PCI visible, drivers not installed yet (expect cluster gpu host-prep / B12).
**Ingress:** `homelab-gpu-pf` address set to `172.16.10.141`.
**Assets:** B10-*; bugs BUG-20261002-B10-*.

## B11 — GPU clusters — 2026-10-02 CEST — COMPLETE

- **lab-single-0** still **Ready** (not wiped; GTX 1050 Ti on virt03).
- **lab-gpu-pt-0** **Ready** (single_node, ingress `homelab-gpu-pf`, upstream `10.10.0.14`,
  jumphost `linux@jump.otthol.hu:30200`). Guest `172.16.10.141` on virt04 GTX 1650 passthrough.
- Bootstrap: first fail rootFS <40GiB on size:40 disk (bug); grew qcow2/FS to 50G → Validated →
  provision → Ready (~8m). Hub fleet SSH key injected via jumphost YAIDAP key (B8 lesson).
- nvidia-smi on fresh guest: PCI OK, drivers not installed yet (defer gpu host-prep / B12).
- Pre-reimage guest had working nvidia-smi 615.71.09 (asset `B10-guest-nvidia-smi.txt`).
- B11 done. Assets B11-*; bug BUG-20261002-B11-rootfs-under-40gib.

## B12 — Hub LiteLLM — 2026-10-02 CEST — COMPLETE

- `umkim llm set --hub-enabled --public-host llm.api.demo.otthol.hu` (lab DNS; not llm.example.com).
- `umkim llm reconcile --wait` → phase **Ready**; endpoint `https://llm.api.demo.otthol.hu/v1`.
- Pods `fleet-llm/litellm` + `redis` Running 1/1; NodePort `:30918` liveliness OK.
- API key `intro-lab` created; token stored only in `~/.umkim-llm-intro.key` mode 600 (not in assets).
- `scripts/verify-fleet-llm.sh` fails unauthenticated curl to `/llm` (bug); manual verify OK.
- Assets: B12-*; bug BUG-20261002-B12-verify-fleet-llm-unauthenticated.

## B13 — GPU prepare + llama.cpp both clusters — 2026-10-02 CEST — COMPLETE

**GPU (both Ready, fleet.capability/gpu=true):**
- lab-single-0 (GTX 1050 Ti): host-prep Ready (~45s with existing driver); enable Ready; discover Succeeded.
- lab-gpu-pt-0 (GTX 1650 PT): host-prep Ready (~7m); enable reported fail while discover returned **stale** Succeeded (`completed_at` pre-host-prep). Spoke already had device plugin + `nvidia.com/gpu=1`. Cleared ManagedCluster discovery annotations → rediscover gpu=true. Bugs filed.

**Apps:**
- `llama-virt03` on lab-single-0 Ready (profile qwen35-2b-ud-4g, llmBackend true).
- `llama-gpu-pt` on lab-gpu-pt-0: initial Pending Insufficient memory (4Gi request vs ~3.7Gi allocatable) → updated to 1500Mi/2800Mi → Ready. Both backends `/health` ok.

**Hub inference:** backends sync-from-apps (Qwen3.5-2B-UD → 10.10.0.10 + 10.10.0.14). LiteLLM ConfigMap updated but pod kept stale `llama-cpp-demo` until rollout restart (bug). After restart, hub OpenAI chat returned **"Hello."** (model Qwen3.5-2B-UD). Direct spoke chat also OK.

**Strategy after B13:** no B14 / Phase C teardown in operational-introduction-strategy — ends at B13 + automation shortcut. **Not starting teardown.**

**Bugs:** BUG-20261002-B13-gpu-discover-stale-succeeded; BUG-20261002-B13-litellm-config-not-reloaded; BUG-20261002-B13-llama-profile-oom-small-guest; BUG-20261002-B12-verify-fleet-llm-unauthenticated.

**Assets:** B12-*, B13-*. Phase B GPU/LLM path done.
