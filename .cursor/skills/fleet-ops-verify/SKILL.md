---
name: fleet-ops-verify
description: >-
  Guides fleet deployment, cleanup, and verification. Use for fleet-deploy.yml,
  fleet-cleanup.yml, verification scripts, firewall workflows, or health checks.
  Update linked documentation when behavior changes.
---

# Ops & verification (S7)

## Scope

Use this skill for operational playbooks and scripts. Read the target's source,
inventory inputs, and `--help` output before running it; operational scripts can
be destructive or environment-dependent.

## Workflow

1. Start with the supported path in [docs/install.md](../../../docs/install.md).
2. Prefer a scoped tag or verification suite when the task is narrow.
3. Use `./scripts/verify-fleet.sh --help` to select the verification layer,
   profile, and suite. The registry at
   `scripts/fleet-verify/check-registry.yaml` is the current check contract.
4. Run a dry run before mutating verification, and obtain explicit approval for
   cleanup, teardown, or rebuild actions.

Do not infer deployment state from documentation. Resolve endpoints, credentials,
and topology from the authorized inventory and runtime configuration.

## References

- [Installation guide](../../../docs/install.md)
- [Design guide](../../../docs/design.md)
- [Verification guide](../../../docs/fleet-verify.md)
- [Agent routing](../../../AGENTS.md)
