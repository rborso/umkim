# Fleet verification

> Status: v0.2 is an experimental proof of concept. Verification improves
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

The currently registered suite names include `hub`, `core`, `edge`, `backup`,
`tenancy` (console auth and RBAC checks), `llm`, `apps`, `regression`, and
`all`. The `verify-fleet.sh --help` text may list legacy names (`nested`,
`catalog`); use `apps` for chart-library checks.

For hub LiteLLM status, use `scripts/verify-fleet-llm.sh` with fleet identity
headers (built in) or `UMKIM_TOKEN` when the Fleet API requires auth; it falls
back to `umkim llm get` when curl fails.

For a Helm app smoke test on a Ready cluster, use
`scripts/verify-fleet-app.sh` (set `FLEET_APP_CLUSTER`) or the console **Apps**
page. `scripts/verify-fleet-chart.sh` checks that the hub library lists seeded
charts. For llama-cpp profile install, PATCH upgrade, and LLM backend sync, use
`scripts/verify-fleet-llama-app-lifecycle.sh` (`FLEET_LLM_CLUSTER` on a GPU
Ready cluster). Profile CRUD, quick/extended verify, and hub-proxied chat:
`scripts/verify-fleet-llama-profile.sh` (check id `llm.profile-verify`). See
[components/fleet-apps.md](components/fleet-apps.md) and
[components/fleet-llama-profiles.md](components/fleet-llama-profiles.md).

## Cluster reinstall presets

Cluster reinstall is an L2 operation. The built-in preset names are homelab-profile
labels (`profiles: [homelab]` in the check registry). This public tree does not
ship the extra-vars files those names used to load. Maintainer lab inventory is
not published. Start hub inventory from
[examples/inventory/hub-only](../examples/inventory/hub-only/) and replace
`CHANGE_ME`.

Preset names:

- `lab-sn01`
- `lab-3n01`
- `lab-ha01`

Pass a reviewed local extra-vars file with `--cluster-spec`:

```bash
./scripts/verify-fleet.sh --layer 2 \
  --cluster-preset lab-sn01 \
  --cluster-spec /path/to/local-flow.yml \
  --cluster-preset-only
```

A reinstall can delete cluster state, so confirm the target and recovery plan
before running it.

For **HA** presets (`lab-ha01`, `lab-3n01`), hub finalize Jobs allow up to
**90 minutes**. Default flow polling in [cluster-create.yml](../playbooks/flows/cluster-create.yml)
is 90 retries × 20s (30 minutes). Override `fleet_flow_poll_retries` (for example
`270` for 90 minutes) when driving HA reinstall flows.

Reports are written under `reports/verify-<layer>-<timestamp>/`. Protect
reports if they contain operational identifiers or failure output.

See [install.md](install.md) for the supported installation path and
[scripts/fleet-verify/README.md](../scripts/fleet-verify/README.md) for the
check registry and extension guidance.
