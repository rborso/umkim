# Post-fix verification (UMKIM / YAIDAP)

**When:** 2026-10-02 ~07:35 Europe/Budapest (UTC+2)  
**Operator:** Robert Borso (read-only checks on hub `wg.otthol.hu`; YAIDAP via hub→`jump.otthol.hu:30200`)  
**Scope:** Verify Cursor walkthrough bug-fix claims after intro walkthrough. No cluster teardown. No secrets pasted.

---

## Executive summary

| Item | Result |
| --- | --- |
| UMKIM git vs claimed `3e2e8d0` | **Match** — hub `v0.2` HEAD **is** `3e2e8d0` |
| YAIDAP git vs claimed `52bd22f` | **Not found** — SHA absent locally and on GitHub API (404). Jumphost `main` HEAD = `0429c71` |
| Fixes in UMKIM **source** | Many Phase 1–4 items **present in tree** at `3e2e8d0` |
| Fixes on **live hub** | **Not redeployed** — fleet-site / fleet-edge / ansible-runner still image tag `0.2` from pods created **before** `3e2e8d0` (2026-10-01 / early 2026-10-02) |
| YAIDAP pin in `VERSIONS.md` | Still **TBD** |
| Clusters | Still **Ready** (`lab-single-0`, `lab-ha-0`, `lab-gpu-pt-0`); sites `homelab` / `management` Ready |

**Bottom line:** UMKIM code claims for `3e2e8d0` largely check out in git. Runtime has **not** picked them up yet. Cursor’s YAIDAP pin `52bd22f` is unverifiable / wrong for this repo; jumphost is ahead on some items (placement, PXE discovery stay-live) but still missing fleet pubkey install for create_vm.

---

## 1. UMKIM on hub

```
branch:  v0.2  (tracks origin/v0.2)
HEAD:    3e2e8d095a64ad5787426e3544bca8d2bf4212ac
message: Fix demo walkthrough gaps across fleet-site, edge, and ops scripts.
         (2026-10-02 07:23:21 +0200)
```

- `git fetch` done; `3e2e8d0` is HEAD and ancestor of itself (reachable on `v0.2` / `origin/v0.2`).
- Recent relevant history includes discovery/LiteLLM/ACME/YAIDAP/GPU work (`e99b1e1`, `11ef9c5`, `293fc77`, `51e30d2`, `da0c865`, …) culminating in the walkthrough gap fix.

### `VERSIONS.md` YAIDAP pin

Still:

> YAIDAP | `main` @ **TBD** (set after YAIDAP PR merges)  
> Notes: `fleet_ansible_ssh_pubkey`, create_vm idempotency, install_bm/TempXE guards

---

## 2. YAIDAP on jumphost

```
path:    /home/linux/yaidap
branch:  main → origin/main
HEAD:    0429c71  Add opt-in UMKIM hub integration contract (YAIDAP_RESULT).
52bd22f: NOT a local object; GitHub commits API → 404
fetch:   https://github.com/rborso/yaidap.git failed (no interactive creds)
```

Notable commits already on jumphost (≠ Cursor’s missing SHA):

| Commit | Topic |
| --- | --- |
| `0429c71` | UMKIM hub integration contract (`YAIDAP_RESULT`) |
| `dab6858` | PXE discovery stays in live env until manual reboot |
| `b566864` | Memory-based VM **placement** (plan-vm-placement + schedule_*) |
| `a44ca18` | destroy vs running create tasks |

**Working tree dirty** (local inventory/homelab edits, TempXE template mods). Treat as lab WIP — not a clean pin candidate until committed/pushed.

### YAIDAP gap scan (brief)

| Claimed gap | On jumphost `0429c71`? |
| --- | --- |
| `fleet_ansible_ssh_pubkey` install into guest cloud-init | **Absent** — zero matches for that var; cloud-init uses inventory `ssh_pubkey` / `ssh_pubkeys` only |
| Placement | **Present** (`b566864`, `plan-vm-placement.yml`, `schedule_*` tasks) — Cursor “still missing placement” is **outdated** for this tree |
| create_vm idempotency | **Partial** — `create_vm.yml` skips `virsh start` when domain already `running`; redefine path only when `dominfo` fails. Full “re-run create_vm on healthy guest → Succeeded” still needs soak / may fail on other steps |
| install_bm / TempXE guards | **Partial** — `install_bm_validate.yml` requires confirm + `install_os`; discover path forces `install_os: false`. Walkthrough conflict (discovery + install_os) still operator hygiene; stronger fail-fast for missing UEFI netboot not fully verified here |
| yaidap-sync needed? | **Yes after** YAIDAP pubkey/idempotency land **and** hub controllers redeployed; current jumphost already has placement/discovery commits that hub docs may expect. Do **not** sync empty hub inventory over jumphost (B2). |

**Contract mismatch:** UMKIM `3e2e8d0` passes Ansible extra `-e fleet_provision_fleet_ansible_ssh_pubkey=…` on `create_vm`, while docs say YAIDAP should accept `fleet_ansible_ssh_pubkey`. YAIDAP accepts neither yet.

---

## 3. Walkthrough bugs vs Cursor claims

Legend: **code** = present in UMKIM `3e2e8d0` tree · **live** = running hub images/config · **open** = still missing · **redeploy** = needs rebuild/rollout to verify

### UMKIM-side (claimed shipped in `3e2e8d0`)

| Bug / theme | Code at `3e2e8d0` | Live hub | Notes |
| --- | --- | --- | --- |
| A1 teardown stuck delete jobs | **Confirmed shipped** — `TEARDOWN_DELETE_JOB_MIN_AGE_SEC` default **1800** | Script on disk only | Redeploy N/A (ops script) |
| A5 chart add overwrite / `--force` | **Confirmed** — `charts.go` + CLI `--force` | **Redeploy** needed for API; CLI binary dated **2026-10-01 07:24** (pre-fix) | Rebuild `umkim` CLI + fleet-site |
| A6 edge hub probe 127.0.0.1 | **Confirmed in role defaults** → `gateway.fleet-ingress.svc…` | **Still open live** — deploy env `HUB_GATEWAY_URL=https://127.0.0.1` | Re-run fleet-edge Ansible/Helm from new defaults |
| A7 / B3 WG + tunl0 cleanup | **Confirmed** in `roles/k3s/tasks/cleanup.yml` (+ bundle copy) | Needs cluster-delete with cleanup / new runner image | Don’t verify with teardown now |
| B2 empty inventory sync guard | **Confirmed** — refuse unless `confirm=true` | **Redeploy** fleet-site | |
| B2 HTTPS YAIDAP repo without secret | **Confirmed** — `repo_sync.go` rejects bare https | **Redeploy** | |
| B7b ACME disable clears edge TLS policy | **Confirmed** — `clearClusterEdgeACMEPolicy` | **Redeploy** | |
| B8 fleet SSH on create_vm | **Hub half shipped** (`hub_ssh_pubkey.go` + provision extra var) | **Redeploy** + YAIDAP still open | End-to-end open |
| B8 node-op post-validate race | **Confirmed** — retry up to 12m / 15s on “not Ready” | **Redeploy** | |
| B8c replace CP inventory / old node WG | **Confirmed** — `nodeWGWithReplaceControlOld` | **Redeploy** | Manual B8c not re-run |
| B9a / B13 discovery staleness | **Confirmed** — rediscover ignores stale MC Succeeded | **Redeploy** | Manual rediscover not re-run |
| B9a bridge CNI mkdir | In commit/doc program; not re-grepped in depth | Needs runner/bundle image | |
| B12 verify-fleet-llm auth | **Confirmed** — `scripts/verify-fleet-llm.sh` uses bearer | Script-only | Re-run verify after login |
| B13 LiteLLM config reload | **Confirmed** — `rollout restart` after ConfigMap | Role change — re-run `fleet-llm` play / reconcile | Live litellm pod age ~5h (pre-commit) |
| A1 console image tag float | Earlier fix (review: Fixed) | Live console up | |

### YAIDAP-side (Cursor: only TempXE/PXE @ `52bd22f`; rest missing)

| Theme | Status on jumphost |
| --- | --- |
| TempXE / PXE discovery stay-live | **Present** (`dab6858`) — not via `52bd22f` |
| Placement | **Present** (`b566864`) — Cursor “missing” claim **wrong** for this checkout |
| `fleet_ansible_ssh_pubkey` | **Still open** |
| create_vm full idempotency | **Partial / open** for soak |
| install_bm guards | **Partial** |

### Cannot verify without redeploy / re-test

- B8 / B8c / B9 / B12 / B13 **manual** paths (explicitly not re-run after push).
- Homelab layer-1: last recorded run was on **`d26cedc`** (parent of `3e2e8d0`): **28 pass / 2 fail**.
  - `ops.umkim-management-site`: expected PATCH ingress **403**, got **401** (auth session / gate — not clearly fixed by `3e2e8d0` file list).
  - `console.playwright.cluster-detail`: fail (also in checks.jsonl).
- End-to-end create_vm + node-op with fleet key.
- Edge route health after gateway URL fix.

---

## 4. Live hub state (read-only)

| Resource | State |
| --- | --- |
| ManagedClusters | `lab-single-0`, `lab-ha-0`, `lab-gpu-pt-0` — Joined/Available **True** |
| FleetClusterJoin | all three **Ready** |
| FleetSite | `homelab`, `management` — **Ready** |
| Controllers | `fleet-site-controller` (ns `fleet-console`), `fleet-edge-controller` (ns `fleet-system`) — Ready; images `…/fleet/*:0.2` |
| Pod ages | fleet-site created **2026-10-01T19:10Z**; fleet-edge **2026-10-02T00:22Z** — both **before** commit `3e2e8d0` (07:23 +0200) |
| CLI | `~/.local/bin/umkim` mtime **2026-10-01 07:24** — pre-fix (`chart --force` not in this binary) |
| umkim API | `umkim whoami` / `cluster list` → **401** without fresh login (expected for this check) |

**Conclusion:** Lab is healthy enough for continued work, but **new controller behavior is not live** until Harbor rebuild + rollout (and CLI rebuild).

---

## 5. Practical next steps

1. **Rebuild & rollout UMKIM 0.2 from `3e2e8d0`** on hub (fleet-site, fleet-edge, ansible-runner; reinstall/rebuild `umkim` CLI). Prefer existing hub install/push path — do **not** teardown clusters.
2. **Re-apply fleet-edge** so `HUB_GATEWAY_*` pick up `gateway.fleet-ingress.svc.cluster.local` (live still 127.0.0.1).
3. **YAIDAP:** discard phantom pin `52bd22f`. Either:
   - pin jumphost/reality (`0429c71` or later after pubkey PR), or
   - land remaining YAIDAP work (accept `fleet_ansible_ssh_pubkey` **or** alias `fleet_provision_fleet_ansible_ssh_pubkey` into cloud-init `ssh_pubkeys`; harden create_vm no-op; install_bm UEFI/netboot fail-fast), push, then set `VERSIONS.md`.
4. **yaidap-sync** only after YAIDAP pin + hub connector redeploy; use inventory sync **confirm** guards; avoid wiping jumphost.
5. **Re-test priority:**  
   - Layer-1 hub suite (expect re-check `ops.umkim-management-site` + playwright).  
   - Manual **B9a/B13** rediscover, **B12** verify-fleet-llm (authenticated), **B8/B8c** node-op / replace, **B13** LiteLLM after config change, **A5** chart `--force`, **A6** edge health after gateway fix.  
   - create_vm + SSH with fleet key once YAIDAP consumes pubkey.
6. Leave clusters up; scrub any future logs of keys/tokens before committing.

---

## 6. Artifact paths

- This note: `demo_materials/introduction-walkthrough/POST-FIX-VERIFICATION.md`
- Prior triage: `demo_materials/introduction-walkthrough/BUG-CODE-REVIEW-20261002.md` (body still lists many **Confirmed** verdicts; header claims Phases 1–4 implemented — treat header + `3e2e8d0` as the fix program, body as pre-fix analysis)
- Layer-1 snapshot (pre-fix SHA `d26cedc`): `demo_materials/introduction-walkthrough/assets/verify-fix-20261002-homelab/verify-1-20261002T065451/`
