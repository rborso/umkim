# Changelog

All notable changes to this project are documented here.

## [Unreleased]

## [0.2.1] — 2026-10-11 — lab / reference

- FleetApp: create an Authelia OIDC client at install time. Chart `umkim.oidc`
  metadata is published as `FleetChart.status.oidcDefaults`; the console and
  `umkim app create --oidc-client` prompt from it and set
  `FleetApp.spec.oidcClient`. The controller registers the client on the target
  cluster's `FleetClusterJoin` (owner-tagged) and removes it on app delete.
- CLI: `umkim tenant` (list/create/delete/add-member/remove-member/add-user) and
  `umkim invite` (list/create/delete/redeem); hub tenant app deploy walkthrough.
- fleet-edge: bound `kubectl` calls so a stuck ManifestWork delete cannot hang
  reconcile; ACME queue starvation fix (drop route-less hosts, rotate stale
  hosts, probe before queue); duplicate route host rejected with **409**.
- WireGuard: pin operator client addresses so adding a client stops renumbering;
  fix the route task guard.
- ingress-auth: per-client `requirePkce` override.

## [0.2.0] — 2026-09-12 — lab / reference

- Rocky Linux 10 only; one management hub. Hub images are tagged from this
  repository (`umkim_version` / `harbor_fleet_image_tag`).
- Removed DCM, catalog placement, and k8s/helm service-provider deploy paths.
- Added `FleetApp`: install an OCI Helm chart onto a Ready cluster the
  operator names. There is no placement engine. Console **Apps** page and
  `POST /api/v1alpha1/apps` (`clusterName` required).
- Per-site ingress exposure modes documented (`hub_gateway`, `port_forward`,
  `reverse_proxy`, `none`). See [docs/design.md](docs/design.md) and
  [examples/site-profiles/](examples/site-profiles/).
- Cluster delete defaults to **detach** (preserves spoke K3s and workloads).
- Removed `route.mode: dual` from supported routing (API, CRDs, fleet-edge controller,
  console UI, and verify scripts).
- Documentation refresh: hub-centric fleet positioning, hub independence, and
  exposure/routing alignment across README and component guides.
- Hub backup/restore: `backup-hub.sh` and `restore-hub.sh` with shared
  `lib/hub-backup-common.sh` (etcd, WireGuard, ACME, optional Harbor PVC/DB).
  See [docs/backup-restore.md](docs/backup-restore.md) and
  [docs/archive/plans/hub-backup-restore.md](docs/archive/plans/hub-backup-restore.md).
- Removed deprecated provisioner surfaces: site host CRUD API, JSON inventory
  guest/catalog mutations, `umkim site host add|patch|delete`, `umkim site inventory set|patch|import pull`, and OpenTofu/Terraform provider stubs.
- Archived historical provisioner design docs under `docs/archive/`.
- Renamed console product from legacy DCM naming: UI directory `ui/umkim-console`,
  Harbor image `umkim-console`, K8s Deployment `umkim-console`. Ansible vars
  `umkim_console_*` and `fleet_console_namespace` replace `dcm_fleet_*` and
  `fleet_webui_namespace` (removed in 0.2.0). Delete stale `dcm-fleet-dashboard`
  Deployment after upgrade on existing hubs.
- Harbor `charts` project publishes the whoami smoke-test chart.
- Greenfield path: [docs/install.md](docs/install.md) +
  [examples/inventory/hub-only/](examples/inventory/hub-only/).

## [0.1.0] — lab / reference

- Initial public-facing release of the UMKIM fleet stack (OCM hub, DCM, Harbor,
  fleet-site / fleet-edge, console).
- Sibling images: control-plane, k8s/kubevirt service providers,
  fleet-olm-subscription-sp, fleet-helm-release-sp.
