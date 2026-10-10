# UMKIM v0.2.1

Git tag: `umkim-0.2.1`

v0.2.1 is a hub-centric fleet manager for independent K3s clusters across sites:
Rocky Linux 10, WireGuard overlay, OCM, Harbor, FleetClusterJoin, FleetApp,
and fleet-edge with flexible per-site ingress.

Greenfield install: [docs/install.md](docs/install.md) and
[examples/inventory/hub-only/](examples/inventory/hub-only/).

## Included

- Hub installation and independent workload-cluster enrollment (Rocky Linux 10)
- FleetApp Helm installs onto a Ready cluster the operator selects
- Install-time Authelia OIDC client creation for FleetApps (chart `umkim.oidc`
  metadata → console/CLI prompt → `FleetApp.spec.oidcClient`)
- `umkim tenant` and `umkim invite` CLI for directory tenants and onboarding
- Per-site exposure modes: `hub_gateway`, `port_forward`, `reverse_proxy`, `none`
- Per-app routing: `route.mode` `none`, `hub`, or `site`
- Console authentication/RBAC, an Apps workflow, and optional application ingress
- Cluster delete defaults to **detach** (keeps spoke K3s and workloads)
- Spokes survive hub loss for non-hub-routed workloads
- Optional backup/restore and LLM integration paths

## Changes since v0.2.0

- FleetApp: create an Authelia OIDC client at install time. A chart declares its
  recommended client under `umkim.oidc`; the publish job records it as
  `FleetChart.status.oidcDefaults`; the console and `umkim app create
  --oidc-client` prompt from it and set `FleetApp.spec.oidcClient`. The
  controller registers the client on the target cluster's `FleetClusterJoin`
  (owner-tagged) and removes it on app delete.
- CLI: `umkim tenant` (list/create/delete/add-member/remove-member/add-user) and
  `umkim invite` (list/create/delete/redeem), plus a hub tenant app deploy
  walkthrough in [docs/hub-tenant-app-deploy.md](docs/hub-tenant-app-deploy.md).
- fleet-edge: bound `kubectl` calls (timeout + `--wait=false`) so a stuck
  ManifestWork delete cannot hang reconcile; ACME queue starvation fix (drop
  route-less hosts, rotate stale hosts, probe before queue); duplicate route
  host rejected with **409** on app create/patch.
- WireGuard: pin operator client addresses so adding a client stops renumbering;
  fix the route task guard.
- ingress-auth: per-client `requirePkce` override.

## Removed relative to v0.1

- DCM control plane
- Catalog placement and k8s/helm/KubeVirt service-provider deploy paths
- Sibling DCM service-provider clones as a hub-install requirement
- `route.mode: dual` (deprecated; use `hub` or `site` separately)

There is no placement engine. After a cluster is Ready, name it on a
`FleetApp` and install an OCI chart. See
[docs/components/fleet-apps.md](docs/components/fleet-apps.md).

Site exposure examples: [examples/site-profiles/](examples/site-profiles/).
Architecture: [docs/design.md](docs/design.md).

## Not a production guarantee

- No support, certification, or complete upgrade path from v0.1
- Destructive install and teardown operations
- Operator-owned firewall, TLS, identity, secret, and per-site exposure reviews

## Known limitations (v0.2.1)

- OIDC/SSO covers Authelia OIDC clients for apps; there is no Keycloak path.
- FleetApp supports create, patch, and delete; there is no placement engine.
- Backup/restore is experimental; single-node hub only; spokes not included.
- No hub HA/failover controller; no organization-level tenancy isolation.

Hub images use `umkim_version` / `harbor_fleet_image_tag` (`0.2`). Read
[SECURITY.md](SECURITY.md) before deployment.

## Upgrading from v0.1

v0.2 removes DCM, catalog placement, and sibling service-provider deploy paths.
The supported path is a **greenfield hub reinstall** from
[docs/install.md](docs/install.md), not an in-place upgrade from v0.1.

## v0.1.0 (previous)

v0.1.0 shipped DCM catalog placement with k8s, Helm, and KubeVirt provider
paths. That model is not part of v0.2. Historical notes:
[docs/archive/README.md](docs/archive/README.md).
