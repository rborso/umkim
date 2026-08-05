# Fleet verification

> Status: v0.1.0 is an experimental proof of concept. Verification improves
> confidence but does not certify production readiness.

`scripts/verify-fleet.sh` runs the repository’s layered checks from the
repository root.

## Layers

| Layer | Purpose | Safety |
|---|---|---|
| `1` | Read-only audit and health checks | Safe to run after deployment |
| `2` | Synthetic create, verify, and cleanup checks | Changes test resources |
| `3` | Rebuild workflow followed by checks | Destructive; requires an approved maintenance window |

## Common commands

```bash
# Read-only hub audit
./scripts/verify-fleet.sh --layer 1 --suite hub

# Core synthetic checks
./scripts/verify-fleet.sh --layer 2 --suite core

# Full destructive regression
./scripts/verify-fleet.sh --layer 3 --suite regression
```

The currently supported suite names are `hub`, `core`, `edge`, `backup`,
`tenancy`, `llm`, `nested`, `catalog`, `regression`, and `all`.

## Cluster reinstall presets

Cluster reinstall is an L2 operation. The only built-in preset names are:

- `lab-sn01`
- `lab-3n01`
- `lab-ha01`

For example:

```bash
./scripts/verify-fleet.sh --layer 2 \
  --cluster-preset lab-sn01 --cluster-preset-only
```

Use `--cluster-spec` only with a reviewed flow file. A reinstall can delete
cluster state, so confirm the target and recovery plan before running it.

Reports are written under `reports/verify-<layer>-<timestamp>/`. Protect
reports if they contain operational identifiers or failure output.

See [install.md](install.md) for the supported installation path and
[scripts/fleet-verify/README.md](../scripts/fleet-verify/README.md) for the
check registry and extension guidance.
