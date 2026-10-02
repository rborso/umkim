# Demo walkthrough bug triage (code review)

Review date: 2026-10-02. Source reports: `bugs/BUG-*.md` (36 files). Each item below maps to one report, verified against this repository (`v0.2` branch).

Legend for **Verdict**:

| Verdict | Meaning |
| --- | --- |
| **Confirmed** | Reproducible product/code gap; fix belongs in UMKIM (or documented contract is wrong). |
| **Partial** | Real pain in the walkthrough, but split across inventory, YAIDAP, lab networking, or operator procedure. |
| **Infra / ops** | Expected behavior given DNS, router, or inventory; not a controller bug. |
| **Not a bug** | Misread behavior or already documented. |
| **Fixed** | Issue was valid; current tree already contains the fix. |

**Fix status (2026-10-02 program):** Phases 1–4 and UMKIM connector/YAIDAP contract changes are implemented on `v0.2` (discovery staleness, teardown delete-job age gate, verify-fleet-llm auth, node-op post-validate retry, replace-CP inventory, chart `--force`, edge hub probes, ACME disable edge sync, LiteLLM rollout restart, inventory sync guard, HTTPS YAIDAP repo validation, provision conflict retry, Traefik overlay-first probe, k3s WG cleanup, KubeVirt CNI mkdir, docs/examples). YAIDAP runtime fixes remain in the sibling repo (see [docs/components/yaidap-integration.md](../../docs/components/yaidap-integration.md) and [VERSIONS.md](../../VERSIONS.md)). Homelab subset verification is recorded under `assets/verify-fix-20261002-*` when the lab is available.

---

## Phase A

### BUG-20261001-A1-console-invalid-image-tag-float

| Field | Value |
| --- | --- |
| **Verdict** | **Fixed** |
| **Code** | `roles/fleet_console_helm/templates/values.yaml.j2` quotes console and auth image tags: `tag: "{{ umkim_console_tag \| default(...) }}"` (lines 10, 44). |
| **Notes** | Original failure (YAML float `0.2` → InvalidImageName) matches unquoted tags; repo fix matches the bug’s “Update” section. |
| **Action** | None unless an older inventory/chart still renders unquoted tags from a fork. Re-run hub console Helm after upgrade. |

### BUG-20261001-A1-teardown-stuck-delete-jobs

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `scripts/teardown-hub.sh`: `wait_cluster_gone` calls `force_remove_stuck_cluster` after 60s. In `Deleting` phase, any delete Job with `succeeded != 1` and non-empty `startTime` sets `stuck=1`, then **deletes the Job** and re-issues `DELETE` (lines 156–188). No minimum age or progress check. |
| **Impact** | Legitimate long-running `fleet-cluster-delete-*` jobs (ManifestWork / klusterlet) are restarted in a loop. |
| **Fix** | Only treat jobs as stuck after a much longer threshold (e.g. 30–45m), or require Job `status.failed` / pod failure count; never delete a Running job younger than N minutes. Optionally surface Job log tail in the script before intervention. |

### BUG-20261001-A1-spoke-k3s-not-removed

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** |
| **Code** | Detach delete does not run spoke `k3s-uninstall` unless `cleanupKubernetes=true` (`playbooks/flows/cluster-delete.yml`, `cluster_delete.go` annotation). Hub teardown with stuck deletes / `--skip-cluster-delete` leaves spokes untouched. |
| **Impact** | virt03 kept K3s after hub wipe. |
| **Fix** | Document that hub teardown ≠ spoke reset. For greenfield reinstall, run cluster delete with `cleanupKubernetes=true` or spoke cleanup playbook before re-bootstrap. Optionally warn in `teardown-hub.sh` when FCJs remain or delete jobs fail. |

### BUG-20261001-A4-dkms-kernel-headers-missing

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** |
| **Code** | Bootstrap bundle can upgrade kernel during host prep; `roles/gpu_host/tasks/driver.yml` installs `kernel-devel-{{ ansible_kernel }}` **before** DKMS on GPU host-prep, but generic bootstrap/WG paths may build modules against a new kernel without matching devel. |
| **Impact** | Warning during bundle; cluster still reached Ready. |
| **Fix** | In bootstrap roles: install `kernel-devel`/`kernel-headers` for **running** kernel before any DKMS step; reboot if kernel was upgraded; fail validate if required modules missing. Mirror GPU host ordering in bundle static roles. |

### BUG-20261001-A5-chart-add-example-oci-overwrites-harbor

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `controllers/fleet-site/internal/httpapi/charts.go` `putChart`: if FleetChart exists, **unconditionally** `existing.Spec = spec` and Update (lines 254–262). No guard for Harbor-seeded charts or unreachable origins. `examples/umkim/chart-add-oci.json` can collide with seeded `whoami`. |
| **Fix** | Require `--force` to change `source`/`origin` on Ready charts; or reject OCI origins not on `ChartOCIPrefixes` allowlist when replacing Harbor. Rename example chart to `example-whoami`. CLI/docs: warn that `chart add` updates in place. |

### BUG-20261001-A6-fleetapproute-probe-127.0.0.1

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `controllers/fleet-edge/internal/config/config.go`: `HubGatewayURL` defaults to `http://127.0.0.1:80`. `route_health.go` probes via `health.ProbeHubPath` with that URL. In-cluster controller pod often has nothing listening on 127.0.0.1:80/443 → Degraded while public curl works. |
| **Fix** | Default `HUB_GATEWAY_URL` / `HUB_GATEWAY_HTTP_URL` to hub Traefik Service (e.g. `http://gateway.fleet-ingress.svc:80`) or set via fleet-edge deployment from inventory. Document required env for hub installs. Treat 401/302 as healthy already works when probe reaches gateway. |

### BUG-20261001-A7-spoke-leftover-wireguard

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** (same class as B3 tunl0) |
| **Code** | `roles/k3s/tasks/cleanup.yml` stops/uninstalls K3s and removes rancher dirs; **no** `wg-quick down` for `wg-umkim` or CNI iface cleanup. |
| **Fix** | Extend cluster delete cleanup play (when `fleet_cluster_cleanup_kubernetes=true`) with roles: stop/disable `wg-quick@wg-umkim`, remove `/etc/wireguard/wg-umkim.conf`, bring down `tunl0`/calico interfaces if present. Document manual spoke reset script. |

---

## Phase B (early)

### BUG-20261001-B2-inventory-sync-empty-hub-over-jumphost

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | Inventory sync projects hub connector store to jumphost (`siteinventory.EnqueueInventorySync`, `connectors/yaidap/files_ops.go`). No check that hub store is “complete” vs jumphost before overwriting. Empty/minimal hub tree can wipe jumphost YAIDAP content. |
| **Fix** | Refuse sync (or require `--confirm`) when host count / critical files drop sharply vs last sync snapshot; show diff summary. Block sync when `ListInventoryFiles` is empty and jumphost has non-empty guests. |

### BUG-20261001-B2-yaidap-sync-hangs-private-https

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `connectors/yaidap/repo_sync.go`: `YaidapGitSecretRef` required only for `git@` / `ssh://` (`repoNeedsDeployKey`). HTTPS private GitHub without token is accepted and hangs on jumphost `git fetch`. |
| **Fix** | Reject `https://` repos without `yaidapGitSecretRef` (or documented credential helper). Document SSH deploy key as supported path. Add job timeout + clear failure message in connector. |

### BUG-20261001-B3-bare_metals-missing-after-sync

| Field | Value |
| --- | --- |
| **Verdict** | **Not a bug** |
| **Code** | `docs/components/fleet-site.md`: `bare_metals.yml` is hub-canonical; jumphost gets `guests.yml` / `external.hosts`. |
| **Action** | Close as documentation/clarification only (report already self-corrects). |

### BUG-20261001-B3-install_bm-kickstart-poll-timeout

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (YAIDAP playbook + inventory) |
| **Code** | UMKIM triggers `install_bm` via connector; failure analysis in bug file points to YAIDAP `configure-tempxe` (`install_os` + `tempxe_discovery.enabled` → discovery mode, no PXE reinstall), missing UEFI network boot entry, kickstart poll on unchanged `boot_id`. Logic lives in YAIDAP roles, not this repo. |
| **Fix** | UMKIM: validate conflicting `install_os` + `tempxe_discovery` in inventory apply or preflight. YAIDAP: fail fast when no PXE boot entry; separate discovery vs reinstall arming. Walkthrough fix (disable discovery) is correct inventory hygiene. |

### BUG-20261001-B3-spoke-leftover-tunl0-after-k3s-uninstall

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | Same as A7: `roles/k3s/tasks/cleanup.yml` does not remove Calico `tunl0` or fleet WG. |
| **Fix** | Same spoke cleanup extension as A7. |

### BUG-20261001-B4-prepare-concurrent-fleetsite-conflict

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | Parallel `site host prepare` updates the same `FleetSite` status (provision action records / job metadata) without optimistic-lock retry on API path → HTTP 500 conflict. Connector job may still run (`site_provision.go` / reconciler). |
| **Fix** | Retry `Update` on conflict in provision commit path; or serialize per-site provision mutations. CLI already has `retryOnHTTP409` for POST in `site_host.go` but server still races on FleetSite status. |

### BUG-20261001-B5-provision-traefik-probe-ssh-timeout

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `roles/fleet_site/tasks/push_cluster_spoke_ingress.yml`: local Traefik probe `delegate_to: "{{ fleet_site_bootstrap_host }}"` runs **before** overlay probe; uses Ansible SSH to bootstrap host. Walkthrough logs show UNREACHABLE via port **65535** (broken ProxyCommand/jumphost inventory on hub runner). Second task probes overlay with curl from hub without delegate. |
| **Impact** | Job Failed while cluster later became Ready (reconcile continued). |
| **Fix** | Prefer overlay/WG probe first for enrolled clusters; make local probe optional or use `ansible_connection` local when delegate target is same as WG endpoint. Fix hub-runner inventory ProxyCommand port. Do not fail provision on probe if ManifestWork applied and overlay probe succeeds. |

### BUG-20261001-B6-create_vm-missing-guest-host-vars

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (inventory contract) |
| **Code** | YAIDAP `create_vm` requires `host_vars/<guest>.yml` with `ansible_host` (connector store). Hub inventory sync does not auto-generate per-guest host_vars from `guests.yml` alone. |
| **Fix** | Document required files in install/YAIDAP guide; optional hub helper to scaffold host_vars from guests template; fail `cluster-apply` with link to missing paths (already fails with clear message). |

### BUG-20261001-B6-create_vm-placement-oversubscribed

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (YAIDAP placement) |
| **Code** | Placement assertion `schedule_unplaced_guests | length == 0` is YAIDAP playbook behavior; UMKIM does not schedule VMs. Leftover domains + memory accounting caused failure until `vm_location` pin. |
| **Fix** | YAIDAP: adopt existing running domain into placement; better free-memory calculation. UMKIM: preflight host list vs libvirt before create_vm. |

### BUG-20261001-B6-create_vm-recreated-guests-ssh

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (expected cloud-init) |
| **Code** | `create_vm` reprovisions disk/cloud-init; hub operator SSH key is not the same as YAIDAP jumphost key unless both injected. |
| **Fix** | Document; align with B8 fleet-key injection. Not a defect if automation uses YAIDAP/bootstrap keys. |

---

## Phase B (B7–B13)

### BUG-20261002-B7-homelab-dns-hits-hub-traefik

| Field | Value |
| --- | --- |
| **Verdict** | **Infra / ops** |
| **Code** | Site whoami healthy on VIP/WG; public DNS `*.homelab.otthol.hu` → WAN IP that terminates on **hub** Traefik, not homelab port-forward VIP. |
| **Fix** | Router/DNS: WAN :443/:80 → `homelab-ha-pf` VIP NodePorts; or split-horizon DNS. Document in site profile examples. No fleet-edge code change fixes wrong PF. |

### BUG-20261002-B7b-spoke-acme-queue-stuck-pending-staging

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** (compounded by B7) |
| **Code** | `fleet-edge` spoke ACME queue (`advanceSpokeACMEQueue`) advances slowly when HTTP-01 cannot complete. `handleDisableClusterAcme` (`cluster_acme.go`) sets `fcj.Spec.Acme.Enabled = false` but **does not** clear `fleet-edge-clusters` ConfigMap `tls_policy: acme_cluster` — edge keeps SpokeACME gating (`clusterAcmeEligible` reads edge registry). |
| **Fix** | On ACME disable, patch edge cluster entry to non-ACME TLS policy or `direct_ingress` resolver. Treat staging PendingStaging timeout with fail-open or queue skip for lab. Couple to DNS/PF validation in preflight. |

### BUG-20261002-B8-create_vm-missing-fleet-ssh-key

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | Bootstrap bundle installs fleet SSH pubkey via `fleet_cluster_ssh` role (`bundle/static/roles/fleet_cluster_ssh`). YAIDAP `create_vm` cloud-init typically only YAIDAP/jumphost keys. Node-op jobs use cluster SSH secret from hub (`ensureClusterSSHSecret`) — **not** injected on create_vm guests. |
| **Fix** | Pass hub fleet public key into YAIDAP create_vm / cloud-init (inventory var from hub); or teach node-op to use jumphost key when fleet key missing; or run `fleet_cluster_ssh` after create_vm before node add. |

### BUG-20261002-B8-node-op-post-validation-race

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `cluster_node_ops.go` calls `postValidateNodeOperation` immediately after Ansible success (`nodeops.Validate` PhasePost). `node_joined` fails if node not Ready **yet** (no retry/backoff). `finishNodeOperation` can write Failed while node becomes Ready seconds later. History vs live `nodeOperation.phase` can diverge when updates race (`finishNodeOperation` lines 783–788). |
| **Fix** | Post-validate with retry loop (e.g. 5–10m, 15s interval) for Ready condition. Align history record phase with final authoritative phase. |

### BUG-20261002-B8-w1-autoapply-addworker-storm

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (configuration hazard) |
| **Code** | `group_vars/lab-ha-0-w1.yml` `autoApply: true` + `mode: add_worker` triggers repeated node ops when guest not cluster member / SSH fails. |
| **Fix** | Default `autoApply: false`; console warning when enabling autoApply without successful enroll; backoff after N failures. |

### BUG-20261002-B8c-replace-skips-old-node-leave

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `images/fleet-ansible-runner/cluster-replace-control.yml` play `hosts: "{{ fleet_node_replace_old }}"` requires old CP in Ansible inventory. Inventory CM built from `clusterNodeWG` / `nodes-wg.json` aligned to **spec** (`syncNodeWGForSpec` + `wg.AllocateClusterNodes`) — if old CP already removed from spec or missing from WG JSON, play skips (“no hosts matched”). Post-validate only warns `old_node_removed` as **Warn**, not Fail (`nodeops/validate.go` 374–377). |
| **Fix** | For replace, inject `fleet_node_replace_old` into inventory with mgmt address from spec history or K8s node list even if not in WG pool. Fail op if graceful leave play matches zero hosts. Do not mark Succeeded until old CP absent from etcd members. |

### BUG-20261002-B9a-bridge-cni-missing-mkdir

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `roles/kubevirt_host_bridge/tasks/main.yml`: `install -m 0755 ... "$BIN"` with `BIN=/opt/cni/bin/bridge` — **no** `mkdir -p /opt/cni/bin`. |
| **Fix** | Add `mkdir -p "{{ kubevirt_bridge_cni_bin_dir }}"` before install (same for `kubevirt_bridge_nad` role). |

### BUG-20261002-B9a-discover-stale-succeeded-no-job

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `httpapi/capability_discover.go` `handleDiscoverCapabilities`: sets FCJ annotation, then polls MC until `capdiscover.FromAnnotations` phase is Succeeded/Failed — **does not** compare `completed_at` to new request time or call `capdiscover.Begin` first. Stale `Succeeded` returns immediately. Reconcile can leave `capability-discover-job` annotation when Job GC’d (`cluster_discover.go` jobPollState). |
| **Fix** | On POST: `capdiscover.Begin` or clear MC discovery annotations; poll until `AnnAt` ≥ request timestamp or phase Running→terminal. Clear orphan job annotations when Job missing. |

### BUG-20261002-B9a-kubevirt-inventory-stale-w1-missing-cp3

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** (symptom of WG/spec drift) |
| **Code** | `ensureClusterInventoryCM` ← `clusterNodeWG` ← bootstrap secret `nodes-wg.json` / `fcj.Status.Nodes` after `syncNodeWGForSpec`. Stale worker VM in site inventory but not in cluster spec can remain in WG list; replaced/missing CP may be omitted if not in spec allocation. |
| **Fix** | Rebuild job inventory from **live** `kubectl get nodes` + FCJ spec addresses for platform jobs; prune WG entries not in spec and not in K8s. Refresh inventory CM after every node op terminal state. |

### BUG-20261002-B9b-longhorn-sc-missing

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (defaults vs capability labels) |
| **Code** | `charts/rocky-10-vm/values.yaml` default `disk.storageClass: longhorn`. Clusters without Longhorn install only have `local-path`. FCJ may still expose `ha_storage` / storage capability labels from spec without Longhorn deployed. |
| **Fix** | Chart default `local-path` or empty (use cluster default SC). Console/strategy: set SC from cluster storage profile. Do not advertise `ha_storage` until Longhorn ManifestWork Ready. |

### BUG-20261002-B9b-vm-unschedulable-2cpu

| Field | Value |
| --- | --- |
| **Verdict** | **Partial** (capacity / chart defaults) |
| **Code** | KubeVirt VM requests 2 CPU on 2-vCPU nodes with system pods — unschedulable. Chart/strategy defaults too large for lab HA nodes. |
| **Fix** | Document minimum node size for `rocky-10-vm` defaults; smoke profile cpu:1/mem:1Gi in examples. Optional scheduler precheck in console create VM. |

### BUG-20261002-B10-bridge-nf-filters-guest

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | K3s sets `bridge-nf-call-iptables=1`; `roles/k3s/tasks/cleanup.yml` does not restore bridge sysctl. Promoted hypervisor (ex-spoke) breaks bridged VM forwarding under firewalld. |
| **Fix** | Hypervisor prepare role: set `bridge-nf-call-*` to 0 for libvirt bridges; run on prepare_hypervisor after k3s uninstall detection. Document in homelab GPU path. |

### BUG-20261002-B10-create-vm-already-active

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** (YAIDAP playbook; UMKIM surfaces Failed) |
| **Code** | Re-run `create_vm` → `virsh start` on active domain fails; host phase Failed despite healthy VM. Idempotency belongs in YAIDAP virt role (`roles/virt01_hypervisor/tasks/vms.yml` uses `virsh start` without check). |
| **Fix** | YAIDAP: if domain active, skip start and succeed. UMKIM: map “already active” to Succeeded/no-op when guest Ready. |

### BUG-20261002-B10-prepare-wait-http-404

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `site_hosts.go`: POST `.../hosts/{name}/provision` returns **404** when `yaidap.LookupInventoryHost` fails (`host not found in inventory`). Asset `B10-prepare-gpu.txt` shows immediate 404 on prepare. Separate from `--wait` polling. |
| **Impact** | Operator sees 404 even if a later retry/sync adds host and job runs. |
| **Fix** | Return 400 with clear “host not in hub inventory store” message; ensure inventory sync completes before prepare in docs. Optional: auto-sync inventory on prepare. |

### BUG-20261002-B11-rootfs-under-40gib

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `fleet_site_validate` default `fleet_site_min_root_disk_gi_default: 40` compares **root FS** size (`roles/fleet_site_validate`). 40 GiB disk minus EFI/partitions ≈ 38.8 GiB root → validate fails. FCJ/bootstrap default `minRootDiskGi` 40 in `buildClusterInventoryINI`. |
| **Fix** | Document guest `disks[].size: 50` for Rocky 40 GiB nominal; or validate against block device size; or lower default to 35 with docs. create_vm could auto-grow root FS to min. |

### BUG-20261002-B12-verify-fleet-llm-unauthenticated

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `scripts/verify-fleet-llm.sh` line 21: `curl -sf "${API}/llm"` with no auth. Fleet API requires console auth (contrast `scripts/verify-llm-inference-e2e.sh` which documents 401 without key). |
| **Fix** | Use `UMKIM_TOKEN` / session cookie like other verify scripts, or call `umkim llm get` via CLI; document in `docs/fleet-verify.md`. |

### BUG-20261002-B13-gpu-discover-stale-succeeded

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `cluster_gpu.go` POST `discover` delegates to same `handleDiscoverCapabilities` as generic discover — same stale MC annotation bug (B9a). GPU enable waits on `fleet.capability/gpu` from discovery (`cluster_gpu_discover.go` deferral only during runtime jobs, not stale Succeeded). |
| **Fix** | Same as B9a; additionally tie GPU enable completion to fresh discovery or node capacity labels (`nvidia.com/gpu`). |

### BUG-20261002-B13-litellm-config-not-reloaded

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** |
| **Code** | `roles/fleet_llm/tasks/reconcile.yml`: `kubectl apply` ConfigMap then `kubectl apply` Deployment manifest — **no** `rollout restart` on `deploy/litellm` when only ConfigMap changes. LiteLLM mounts config at pod start; Ready phase misleading. |
| **Fix** | After ConfigMap apply, `kubectl rollout restart deployment/litellm -n fleet-llm` when config hash changes; or use Reloader sidecar; gate FleetLLM Ready on `/v1/models` containing expected backends. |

### BUG-20261002-B13-llama-profile-oom-small-guest

| Field | Value |
| --- | --- |
| **Verdict** | **Confirmed** (profile vs small nodes) |
| **Code** | `controllers/fleet-site/internal/llamaprofiles/data/values-fleet-runtime.yaml`: `resources.requests.memory: 4Gi`, limits `6Gi`. Profile `qwen35-2b-ud-4g` targets ~4 GB GPU but RAM request ignores ~3.7 GiB allocatable on 4 GiB VM. |
| **Fix** | Add profile `qwen35-2b-ud-small` (e.g. 1500Mi/2800Mi); console warning when node allocatable < requests; homelab GPU VM sizing guide (≥8 GiB RAM for default profile). |

---

## Summary

| Verdict | Count |
| --- | --- |
| Confirmed | 22 |
| Partial | 10 |
| Infra / ops | 1 |
| Not a bug | 1 |
| Fixed | 1 |
| **Total** | **36** |

### Suggested fix priority (product code)

1. **Discovery staleness** (B9a, B13-gpu) — `handleDiscoverCapabilities` / GPU discover path.
2. **Teardown stuck delete jobs** (A1) — `teardown-hub.sh` thresholds.
3. **Node replace inventory** (B8c) + **post-validation retry** (B8 race).
4. **LiteLLM rollout on config change** (B13-litellm).
5. **Fleet-edge hub probe defaults** (A6) and **ACME disable + edge registry** (B7b).
6. **Chart add overwrite guard** (A5), **verify-fleet-llm auth** (B12).
7. **KubeVirt CNI mkdir** (B9a-bridge), **spoke cleanup WG/tunl0/bridge-nf** (A7, B3, B10).
8. **create_vm fleet SSH key** (B8) — YAIDAP/hub integration.
9. **Inventory sync safety** (B2-empty), **HTTPS YAIDAP repo** (B2-hang).
10. **Provision Traefik probe ordering** (B5), **host prepare 404** (B10).

### Reports to close as non-product

- **B3-bare_metals-missing-after-sync** — by design.
- **B7-homelab-dns** — router/DNS; use `--resolve` / VIP checks as walkthrough did.

---

*Generated from static analysis of UMKIM `v0.2` sources; runtime behavior on the walkthrough lab was taken from the bug reports and assets, not re-executed in this review.*
