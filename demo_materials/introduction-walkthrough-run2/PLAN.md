# Introduction walkthrough — Run 2 plan

**Status:** AUTHORIZED — user confirmed 2026-10-02: YAIDAP push recheck, FULL hub wipe+reinstall (run 2), console OK via browser, WAN PF stays, phase gates OK.
Execution in progress (Phase 0 done → A1).

**Written:** 2026-10-02 (Europe/Budapest)  
**Hub:** `linux@wg.otthol.hu` — repo `/home/linux/umkim`

---

## Goals (vs run 1)

| Goal | Run 1 (done) | Run 2 (planned) |
|------|----------------|-----------------|
| Outcome | Full A→B13 executed; many bugs filed; some fixed later | **Introducible demo** — stages pass cleanly with proof |
| Bugs | Document + often workaround; buglist grew | **Document → fix → rebuild/deploy → re-test** before advancing |
| Evidence | Mostly under git-tracked `introduction-walkthrough/` (~3.5M assets already) | Screenshots + full demo evidence on **hub-only** path (not GitHub-sized) |
| Console | CLI-heavy; browser screenshots agreed but incomplete | **Browser on agent computer** + screenshots for Console steps |

Run 1 remains the comparison archive. Do **not** delete or wipe it for run 2.

---

## Paths

### Git-tracked (repo `/home/linux/umkim`)

| Path | Role |
|------|------|
| `demo_materials/introduction-walkthrough/` | **Run 1 archive** — keep for comparison |
| `demo_materials/introduction-walkthrough-run2/` | Run 2 **plan** + later git-sized narrative README / bug index |
| `demo_materials/operational-introduction-strategy.md` | Strategy copy (bundle); master also under `docs/` |
| `docs/operational-introduction-strategy.md` | Strategy master |

### Hub-only evidence (OUTSIDE git repo)

**Chosen root:** `/home/linux/umkim-demo-materials/introduction-run2/`

Rationale: sibling to `/home/linux/umkim` (not under the clone), clear name,
safe for large PNGs/videos/logs that must not bloat GitHub. Stub layout:

```text
/home/linux/umkim-demo-materials/introduction-run2/
  README.md                 # hub-only notice
  assets/                   # {phase}-{slug}.{png,txt,…}
  console-screenshots/      # browser Console UI captures
  cli-logs/                 # long install/provision transcripts
  bugs/                     # optional large attachments
```

**Do not** put this tree under `demo_materials/introduction-walkthrough*` or any
path that is (or will be) pushed to GitHub. Link from the git README to absolute
hub paths or a short relative note (“on hub only”).

---

## Bug workflow (mandatory for run 2)

When Verify fails or behavior diverges from docs:

1. **Document** — File `BUG-{YYYYMMDD}-{stage}-{slug}.md` (same fields as strategy:
   Stage, Environment, Expected, Actual, Reproduce, Evidence, Workaround, Follow-up).
   Put the note in git under `introduction-walkthrough-run2/bugs/` (or index there)
   and store bulky logs/screens under the hub-only evidence root.
2. **Fix** — Implement the real fix in UMKIM / YAIDAP / inventory / DNS as appropriate.
   Prefer permanent fix over walkthrough-only workarounds.
3. **Rebuild / deploy** — Push images / redeploy controllers / rebuild CLI / sync
   YAIDAP as needed so the **live hub** actually runs the fix (source-only commits
   do not count).
4. **Re-test** — Re-run the failing stage Verify until it passes; capture **pass**
   evidence. Only then advance to the next stage.

**Goal:** A proper introducible demo at the end — **not** a long buglist with
“worked around” stages. Temporary workarounds are allowed only to unblock a fix
investigation; they must not be the final story for that chapter.

This tightens the strategy’s existing “When something fails” section for run 2
execution agents.

---

## Console / screenshots

- **Console parts:** drive the UMKIM console in a **browser on the agent computer**;
  capture screenshots for every Console-heavy Verify (wizard done, Ready, app URL,
  delete confirm, Authelia flows, etc.).
- Store screenshots under  
  `/home/linux/umkim-demo-materials/introduction-run2/console-screenshots/`  
  (and/or `assets/` with the usual `{phase}-{slug}.png` names).
- CLI transcripts go under `cli-logs/` or `assets/` as `.txt` / redacted `.json`.
- Redact secrets (keys, cookies, vault, WG configs) as in the strategy.

---

## Before greenfield run — fix remaining known issues first

Do **not** start Phase A (including hub wipe) or Phase B until these are addressed
or explicitly deferred with a demo-safe story. Priority from run-1 / post-fix:

| # | Issue | Notes / refs |
|---|--------|----------------|
| 1 | **ACME / public redirect for homelab** | `*.homelab.otthol.hu` → hub WAN Traefik 404; spoke ACME HTTP-01 / PendingStaging queue stuck. Need WAN PF or split DNS so public names reach site VIP `:30080`/`:30443`. Bugs: `BUG-20261002-B7-homelab-dns-hits-hub-traefik`, `BUG-20261002-B7b-spoke-acme-queue-stuck-pending-staging`. |
| 2 | **YAIDAP fleet pubkey push** | Jumphost has local `63bd7c1` (cloud-init/kickstart accept fleet pubkey); **GitHub push pending credentials**; hub `yaidap-sync` deferred until remote has the commit. Without this, create_vm guests miss fleet SSH key (B8). |
| 3 | Other high-impact leftovers (fix or soak before/during run as needed) | install_bm UEFI/TempXE fail-fast; B8c CP replace leave; create_vm idempotency soak; Longhorn SC / GPU discover stale Succeeded / LiteLLM reload — prefer fixed in tree **and** redeployed before relying on them in the demo narrative. |

Also see: `introduction-walkthrough/FIX-DEPLOY-RESULTS.md` § Remaining blockers,
`POST-FIX-VERIFICATION.md`, `BUG-CODE-REVIEW-20261002.md`.

**Prep order (suggested, still not “start run”):**

1. Finish YAIDAP pubkey commit push (when creds available) → pin `VERSIONS.md` → controlled yaidap-sync (do **not** sync empty hub inventory over jumphost).
2. Fix ACME/DNS/redirect for `homelab.otthol.hu` (or document a deliberate split-DNS demo path that still proves public HTTPS + Authelia without LE failure).
3. Redeploy any remaining unfixed walkthrough bugs that would block a clean A→B13 story.
4. Only after explicit go-ahead: archive note on run 1, then greenfield per strategy (Phase A wipe is part of A1 — **not authorized yet**).

---

## Run 2 execution outline (when authorized)

1. **Archive run 1** — Leave `demo_materials/introduction-walkthrough/` intact; optional
   `README` banner “Run 1 complete — see run2 PLAN”.
2. **Evidence** — All new screenshots/large logs →  
   `/home/linux/umkim-demo-materials/introduction-run2/`.
3. **Follow** `operational-introduction-strategy.md` phases A1→B13.
4. **On every bug:** document → fix → rebuild/deploy → re-test (section above).
5. **Assemble** git-sized walkthrough README under this folder linking hub evidence paths.
6. **Do not** start teardown Phase C / wipe unless the strategy and user ask for it.

### Explicit non-goals until go-ahead

- Do **not** wipe clusters or hub.
- Do **not** start Phase A or Phase B.
- Do **not** run `rebuild-homelab-cli.sh` / `install-hub.sh` teardown as part of “planning”.

---

## Pointers

- Strategy (bundle): [`../operational-introduction-strategy.md`](../operational-introduction-strategy.md)
- Strategy (docs master): [`../../docs/operational-introduction-strategy.md`](../../docs/operational-introduction-strategy.md)
- Run 1 walkthrough: [`../introduction-walkthrough/README.md`](../introduction-walkthrough/README.md)
- Post-fix / remaining: [`../introduction-walkthrough/FIX-DEPLOY-RESULTS.md`](../introduction-walkthrough/FIX-DEPLOY-RESULTS.md)
