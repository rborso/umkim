# Demo materials

Self-contained bundle for the UMKIM + YAIDAP **introduction walkthrough** (narrative,
operational plan, and reference inventory).

| File / directory | Role |
|------------------|------|
| [umkim_yaidap_introduction.md](umkim_yaidap_introduction.md) | TLDR overview (start here) |
| [operational-introduction-strategy.md](operational-introduction-strategy.md) | Step-by-step plan (Console/CLI, evidence, bugs) |
| [yaidap-inventory-reference/](yaidap-inventory-reference/) | Snapshot of homelab YAIDAP Ansible inventory (reference only) |
| [introduction-walkthrough/](introduction-walkthrough/) | **Run 1** archive (completed walkthrough + bugs/assets) |
| [introduction-walkthrough-run2/](introduction-walkthrough-run2/) | **Run 2 plan only** — see [PLAN.md](introduction-walkthrough-run2/PLAN.md); do **not** start Phase A/B yet |

**Canonical repo paths** (keep in sync when inventory or strategy changes):

- Strategy master copy: [docs/operational-introduction-strategy.md](../docs/operational-introduction-strategy.md)
- Live homelab inventory: [examples/homelab/inventory/](../examples/homelab/inventory/)

When executing the plan, add walkthrough artifacts under `introduction-walkthrough/`
(`assets/`, `bugs/`, assembled `README.md`) as described in the strategy doc.

**Note:** `yaidap-inventory-reference/group_vars/all/all.yml` contains lab SSH public keys
and example passwords for YAIDAP cloud-init—treat as lab-only; redact before publishing outside your environment.

## Run 2 (planned)

- Plan: [introduction-walkthrough-run2/PLAN.md](introduction-walkthrough-run2/PLAN.md)
- Hub-only evidence (screenshots / large logs, **outside git**): `/home/linux/umkim-demo-materials/introduction-run2/` on this hub
- Bug workflow for run 2: document → fix → rebuild/deploy → re-test (demo-ready, not a buglist)
- **Do not start** the greenfield run until remaining issues (ACME/redirect, YAIDAP pubkey push, …) are fixed and go-ahead is given
