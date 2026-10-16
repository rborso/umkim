# UMKIM v0.3.0

Git tag: `umkim-0.3.0`

v0.3.0 is a hub-centric fleet manager for independent K3s clusters across sites:
Rocky Linux 10, WireGuard overlay, OCM, Harbor, FleetClusterJoin, FleetApp,
and fleet-edge with flexible per-site ingress.

Greenfield install: [docs/install.md](docs/install.md) and
[examples/inventory/hub-only/](examples/inventory/hub-only/).

## Included

- Hub installation and independent workload-cluster enrollment (Rocky Linux 10)
- Controller-driven cluster bring-up: declare a cluster's node VMs and let
  `spec.autoProvision` provision it through the site jumphost
- FleetApp Helm installs onto a Ready cluster the operator selects
- Install-time Authelia OIDC client creation for FleetApps (chart `umkim.oidc`
  metadata → console/CLI prompt → `FleetApp.spec.oidcClient`)
- Directory tenants and onboarding: `umkim tenant`, `umkim invite`, per-cluster
  directories, a `default-tenant`, and route-declared groups
- Per-site exposure modes: `hub_gateway`, `port_forward`, `reverse_proxy`, `none`
- Per-app routing: `route.mode` `none`, `hub`, or `site`, plus TCP routes with a
  hub L4 port forward
- Console authentication/RBAC, an Apps workflow, and optional application ingress
- Cluster delete with optional app uninstall and VM destruction
- Spokes survive hub loss for non-hub-routed workloads
- Optional backup/restore and LLM integration paths

## Changes since v0.2.0

This release subsumes the 0.2.1 work, which was never released as its own cut.

- Cluster bring-up: `FleetClusterJoin.spec.autoProvision` runs provisioning from
  the controller (jumphost bootstrap, per-step history in the site provision
  history), and `umkim cluster create` can declare the cluster's node VMs in the
  same call. The console creates a cluster and provisions it in one flow.
- Cluster delete: optional app uninstall and VM destruction
  (`--cleanup-apps` / `--cleanup-vms`), VM declarations removed with the cluster,
  and node detach/reset actually runs on delete.
- Site hosts: add and remove hosts through the console or `umkim site host`, with
  read-only verification for hypervisors, additional VM data disks, and a split
  between infrastructure and workload hosts.
- Directory and onboarding: every LLDAP directory gets a `default-tenant`; the
  groups a route declares (`requireAuthGroups`) are reconciled into the cluster's
  own directory; an invitation can target a cluster's own directory and redeem
  against it; redeeming an invite no longer takes over an existing account
  (security fix).
- Apps: a chart's `umkim.tenant` metadata pre-fills the route's tenant groups;
  charts can be published with `--force`; Helm installs wait for slow
  post-install hooks.
- FleetApp: create an Authelia OIDC client at install time. A chart declares its
  recommended client under `umkim.oidc`; the publish job records it as
  `FleetChart.status.oidcDefaults`; the console and `umkim app create
  --oidc-client` prompt from it and set `FleetApp.spec.oidcClient`. The
  controller registers the client on the target cluster's `FleetClusterJoin`
  (owner-tagged) and removes it on app delete.
- CLI: `umkim tenant` (list/create/delete/add-member/remove-member/add-user) and
  `umkim invite` (list/create/delete/redeem), plus a hub tenant app deploy
  walkthrough in [docs/hub-tenant-app-deploy.md](docs/hub-tenant-app-deploy.md).
- Ingress: `FleetAppRoute` gains a TCP `protocol` with a hub L4 port forward; the
  site ingress wildcard is enforced and verified; the spoke ingress render waits
  for the edge registry, so the in-cluster name override is never dropped; a
  stalled spoke ACME episode retries once instead of wedging.
- fleet-edge: bound `kubectl` calls (timeout + `--wait=false`) so a stuck
  ManifestWork delete cannot hang reconcile; ACME queue starvation fix (drop
  route-less hosts, rotate stale hosts, probe before queue); duplicate route
  host rejected with **409** on app create/patch.
- Storage: one default StorageClass is chosen deterministically and the fleet
  ships its own local-path provisioner; NFS export paths are unique per cluster.
- WireGuard: pin operator client addresses so adding a client stops renumbering;
  fix the route task guard.
- ingress-auth: per-client `requirePkce` override.
- Console: mobile-friendly layout and tables, a choice of LLDAP web UI exposure,
  and deterministic ordering from the list API.

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

## Known limitations (v0.3.0)

- OIDC/SSO covers Authelia OIDC clients for apps; there is no Keycloak path.
- FleetApp supports create, patch, and delete; there is no placement engine.
- Backup/restore is experimental; single-node hub only; spokes not included.
- No hub HA/failover controller; no organization-level tenancy isolation.

Hub images use `umkim_version` / `harbor_fleet_image_tag` (`0.3.0`). Read
[SECURITY.md](SECURITY.md) before deployment.

## Upgrading from v0.1

v0.2 removes DCM, catalog placement, and sibling service-provider deploy paths.
The supported path is a **greenfield hub reinstall** from
[docs/install.md](docs/install.md), not an in-place upgrade from v0.1.

## v0.1.0 (previous)

v0.1.0 shipped DCM catalog placement with k8s, Helm, and KubeVirt provider
paths. That model is not part of v0.2. Historical notes:
[docs/archive/README.md](docs/archive/README.md).
