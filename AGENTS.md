# UMKIM agent guide

UMKIM is a hub-centric fleet manager for independent K3s clusters across sites,
with flexible per-site ingress (`FleetSite` `hubGateway` and/or `ingress[]`
methods `port_forward` / `reverse_proxy`; clusters bind via `ingressId`).
Read this file, the matching skill, and applicable rules before changing a
section. Treat `inventory/` as the sole source of deployment-specific values;
do not copy addresses, host names, domains, or secrets into documentation.

## Documentation boundaries

| Tier | Canonical location | Purpose |
|------|--------------------|---------|
| Design | [docs/design.md](docs/design.md) and [docs/components/](docs/components/) | Architecture, site exposure, FleetApp routing (`none`, `hub`, `site` only) |
| Install | [docs/install.md](docs/install.md) | Supported installation workflow |
| Site exposure | [examples/site-profiles/](examples/site-profiles/) | Per-site ingress inventory examples |
| Inventory | `inventory/` and [examples/inventory/hub-only/](examples/inventory/hub-only/) | Deployment inputs and templates |
| Reference | [VERSIONS.md](VERSIONS.md), [SECURITY.md](SECURITY.md), and [docs/secrets.md](docs/secrets.md) | Pins, security, and secret handling |

When documentation and runtime configuration differ, configuration and the
installation guide take precedence. Keep design documentation topology-neutral.

## Section picker

| Section | Skill | Scope |
|---------|-------|-------|
| S1 | `fleet-inventory` | Inventory, host variables, and Ansible configuration |
| S2 | `fleet-ocm-spokes` | WireGuard, K3s, OCM, and managed clusters |
| S3 | `fleet-harbor` | Harbor, OLM/platform ManifestWorks, and FleetApp charts |
| S4 | `fleet-edge-ingress` | Fleet edge and ingress |
| S5 | `fleet-controllers-go` | Fleet controllers and CRDs |
| S6 | `fleet-console-ui` | Console UI and its deployment |
| S7 | `fleet-ops-verify` | Deployment, cleanup, and verification |

## Change-to-document map

| Change area | Update with the change |
|-------------|------------------------|
| Inventory contract | `inventory/` and the applicable inventory template |
| Playbook workflow | [docs/design.md](docs/design.md) and [docs/install.md](docs/install.md) |
| Controller API or CRD | Relevant file under [docs/components/](docs/components/) and its skill |
| Helm apps (`FleetApp`) and chart library (`FleetChart`) | [docs/components/fleet-apps.md](docs/components/fleet-apps.md) |
| Site exposure / ingress | [docs/components/fleet-edge.md](docs/components/fleet-edge.md), [examples/site-profiles/](examples/site-profiles/) |
| Cluster lifecycle / detach | [docs/components/fleet-site.md](docs/components/fleet-site.md), [playbooks/flows/cluster-delete.yml](playbooks/flows/cluster-delete.yml) |
| Console route or user workflow | `ui/dcm-fleet-console/README.md` and [docs/fleet-console-auth.md](docs/fleet-console-auth.md) |
| Fleet CLI (`umkim`) | [docs/components/umkim-cli.md](docs/components/umkim-cli.md) and `cmd/umkim/` |
| Security or secret handling | [SECURITY.md](SECURITY.md) and [docs/secrets.md](docs/secrets.md) |
| Image or version pin | [VERSIONS.md](VERSIONS.md) and `inventory/group_vars/all/common.yml` |

## Working constraints

- Read the matching `.cursor/skills/*/SKILL.md` and scoped rules first.
- Prefer the narrowest applicable playbook tags for operational work.
- Shared fleet-wide variables belong in `inventory/group_vars/all/common.yml`;
  `group_vars/all/` takes precedence over a sibling `all.yml`.
- Keep diffs minimal and match local Ansible, Go, and UI conventions.
- Do not add secrets, commit changes, or edit `.cursor/plans/` unless explicitly
  requested.
- Documentation-only work must describe source limitations rather than imply a
  runtime behavior change.
- Do not document or add `route.mode: dual` — use `none`, `hub`, or `site` only.
- Follow active workspace policies for tools, model use, and reviews.
