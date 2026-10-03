# Fleet verification framework

[`../verify-fleet.sh`](../verify-fleet.sh) selects checks from
`check-registry.yaml`, prepares the configured environment, and writes a local
report. The registry and runner are the current execution contract.

## Layout

- `check-registry.yaml` defines check metadata and handlers.
- `checks/` contains focused handlers.
- `layers/` selects workflow behavior.
- `lib/` provides environment, authentication, reporting, and runner helpers.
- `parity/` contains API and UI parity support.

## Add or change a check

1. Implement the handler in the appropriate script or `checks/` path.
2. Add a registry entry that declares its execution mode, requirements,
   profile, handler, and failure behavior.
3. Run the verifier in dry-run mode with an authorized local configuration.
4. Update [docs/fleet-verify.md](../../docs/fleet-verify.md) when the
   operator-facing contract changes.

Some layers and handlers can mutate a target. Review the selected registry
entries and runner arguments before execution; this README does not guarantee
that a check is safe for every environment.

Prefer [docs/fleet-verify.md](../../docs/fleet-verify.md) and
`scripts/verify-fleet-app.sh` for Helm-app verification.
