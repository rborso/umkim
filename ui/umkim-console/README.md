# UMKIM console

The UMKIM console is the React frontend in this directory. It serves the fleet UI
under `/fleet/` and uses the fleet-site and authentication proxy paths defined
by its source and deployment templates.

## Develop

```bash
cd ui/umkim-console
npm install
export FLEET_API_PROXY="http://127.0.0.1:30912"
npm run dev
```

`FLEET_API_PROXY` is the local Vite proxy target for `/fleet/api/` and
`/fleet/auth/`. The Vite dev server does not proxy `/llm/`; use a deployed hub
or extend `vite.config.ts` for local LLM proxy testing. Provide an authorized
environment value; this README intentionally supplies no production endpoint.

## Build and test

```bash
npm run build
npm test
npm run test:e2e
```

**Apps → Install** supports a llama-cpp-helm values profile picker (hub-merged YAML)
and **Edit** patches version/values on existing FleetApps. **Infrastructure → LLM**
can optionally register llama-cpp installs for hub LiteLLM (install modal checkbox)
and sync LiteLLM backends from Ready registered FleetApps.

Package scripts in [package.json](package.json) are the current contract.
End-to-end tests require deployment-specific access and credentials, so review
their configuration before running them.

## API and deployment boundaries

| Browser path | Contract |
|--------------|----------|
| `/fleet/api/` | Fleet-site API (sites, clusters, FleetApp `/apps` including PATCH, chart library `/charts`, llama profiles `/llama-profiles` and legacy `/charts/llama-cpp-helm/profiles`) |
| `/fleet/auth/` | Console authentication proxy |
| `/llm/` | Optional authenticated LLM proxy |

The client code, nginx template, and OpenAPI sources define endpoint details:

- `src/api/`
- [nginx-default.conf.template](nginx-default.conf.template)
- [Fleet-site OpenAPI](../../controllers/fleet-site/openapi.yaml)

Helm installs use the console **Apps** page (`FleetApp`). Publish charts on
**Apps → Library**. Supported `route.mode` values: `none`, `hub`, `site`.
See [fleet-apps.md](../../docs/components/fleet-apps.md) and
[examples/site-profiles/](../../examples/site-profiles/) for site exposure.

**Rocky Linux 10 VMs:** install library chart `rocky-10-vm` from **Apps** with
configurable vCPU, memory, disk, and guest SSH public keys (KubeVirt platform
required on the target cluster).

**Site connectivity:** registry sites expose jumphost, region, and failure domain
on **Sites → detail → Edge and DNS** (and on create). **Sites → detail → Hosts**
manages hub site host inventory, provision actions, and optional cluster automation
(enroll single-node or HA group, add worker, replace control plane) for registry sites.
**Sites → detail → Provisioner inventory** (YAIDAP sites) edits the native YAML tree;
use **Add VM guest** (name/placement, then CPU/memory/disk) to seed `guests.yml` + `host_vars/` templates, **Sync to jumphost**,
then provision on **Site hosts**. See [yaidap-integration.md](../../docs/components/yaidap-integration.md). Cluster
jumphost overrides are on **Clusters → detail → Edge and ingress**. CLI equivalents:
`umkim site patch`, `umkim site host`, and `umkim cluster patch`
([umkim-cli.md](../../docs/components/umkim-cli.md)).

**Virtual machines:** **Clusters → detail → Virtual machines** lists KubeVirt VMs
on a managed cluster, with serial console by default and on-demand VNC. Power
controls (start, soft/hard stop, soft/hard restart) match `umkim vm`
([fleet-kubevirt.md](../../docs/components/fleet-kubevirt.md)).

Deploy through `fleet-console-auth.yml` and `roles/fleet_console_helm/`; build and image
publication use the Harbor role (`Containerfile` in this directory). Example:

```bash
ansible-playbook harbor.yml -l hub --tags harbor_push_fleet -e harbor_fleet_push_force=true \
  -e '{"harbor_fleet_push_images":[{"repository":"umkim-console","tag":"0.2.0"}]}'
```

Resolve all hosts, registries, tags, and credentials from authorized inventory and secret configuration.

## Scope

This README documents the UI workflow only. It does not configure a backend,
ingress, authentication provider, or external endpoint.

## References

- [Console authentication guide](../../docs/fleet-console-auth.md)
- [Fleet-site component](../../docs/components/fleet-site.md)
- [YAIDAP integration](../../docs/components/yaidap-integration.md)
- [FleetApp Helm installs](../../docs/components/fleet-apps.md)
- [Installation guide](../../docs/install.md)
- [Design guide](../../docs/design.md)
