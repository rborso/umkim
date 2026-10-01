# Demo materials

Self-contained bundle for the UMKIM + YAIDAP **introduction walkthrough** (narrative,
operational plan, and reference inventory).

| File / directory | Role |
|------------------|------|
| [umkim_yaidap_introduction.md](umkim_yaidap_introduction.md) | TLDR overview (start here) |
| [operational-introduction-strategy.md](operational-introduction-strategy.md) | Step-by-step plan (Console/CLI, evidence, bugs) |
| [yaidap-inventory-reference/](yaidap-inventory-reference/) | Snapshot of homelab YAIDAP Ansible inventory (reference only) |

**Canonical repo paths** (keep in sync when inventory or strategy changes):

- Strategy master copy: [docs/operational-introduction-strategy.md](../docs/operational-introduction-strategy.md)
- Live homelab inventory: [examples/homelab/inventory/](../examples/homelab/inventory/)

When executing the plan, add walkthrough artifacts under `introduction-walkthrough/`
(`assets/`, `bugs/`, assembled `README.md`) as described in the strategy doc.

**Note:** `yaidap-inventory-reference/group_vars/all/all.yml` contains lab SSH public keys
and example passwords for YAIDAP cloud-init—treat as lab-only; redact before publishing outside your environment.
