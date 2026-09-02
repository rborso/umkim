# Scripts

This directory contains installation, verification, maintenance, and developer
helpers. Each script is the source of truth for its arguments, prerequisites,
and side effects.

## Use safely

1. Read the script and run `--help` before execution.
2. Supply inventory, credentials, and endpoints through the approved local
   configuration.
3. Use dry-run or read-only modes where available.
4. Treat cleanup, restore, rebuild, and credential-generation scripts as
   potentially mutating.

The supported installation workflow is documented in
[docs/install.md](../docs/install.md). Fleet verification is documented in
[docs/fleet-verify.md](../docs/fleet-verify.md).

## Layout

- `bootstrap-hub-inventory.sh` and `install-hub.sh` support the greenfield
  inventory and installation workflow.
- `verify-*.sh` and `fleet-verify/` provide verification entry points.
  `verify-fleet-app.sh` is the v0.2 Helm smoke test (requires a Ready cluster).
  `verify-fleet-chart.sh` checks the hub chart library.
- Backup, restore, cleanup, and asset-staging helpers have operational side
  effects that must be reviewed before use.
- [lab/](lab/) contains maintainer-oriented helpers outside the supported
  greenfield path.

Scripts do not make environment configuration public-safe by themselves; never
record live endpoints, domains, or secrets in commands, reports, or new files.
