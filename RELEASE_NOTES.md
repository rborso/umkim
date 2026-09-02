# UMKIM v0.2

v0.2 is a hub-centric fleet manager for independent K3s clusters across sites:
Rocky Linux 10, WireGuard overlay, OCM, Harbor, FleetClusterJoin, FleetApp,
and fleet-edge with flexible per-site ingress.

## Included

- Hub installation and independent workload-cluster enrollment (Rocky Linux 10)
- FleetApp Helm installs onto a Ready cluster the operator selects
- Per-site exposure modes: `hub_gateway`, `port_forward`, `reverse_proxy`, `none`
- Per-app routing: `route.mode` `none`, `hub`, or `site`
- Console authentication/RBAC, an Apps workflow, and optional application ingress
- Cluster delete defaults to **detach** (keeps spoke K3s and workloads)
- Spokes survive hub loss for non-hub-routed workloads
- Optional backup/restore and LLM integration paths

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

- No support, certification, or complete upgrade path
- Incomplete backup/restore, HA/failover, tenancy, and application lifecycle
  coverage
- Destructive install and teardown operations
- Operator-owned firewall, TLS, identity, secret, and per-site exposure reviews

Hub images use `umkim_version` / `harbor_fleet_image_tag` (currently
`0.2.0-dev`). Read [SECURITY.md](SECURITY.md) before deployment.

## v0.1.0 (previous)

v0.1.0 shipped DCM catalog placement with k8s, Helm, and KubeVirt provider
paths. That model is not part of v0.2. Historical notes:
[docs/archive/README.md](docs/archive/README.md).
