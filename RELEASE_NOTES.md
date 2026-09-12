# UMKIM v0.2.0

Git tag: `umkim-0.2.0`

v0.2 is a hub-centric fleet manager for independent K3s clusters across sites:
Rocky Linux 10, WireGuard overlay, OCM, Harbor, FleetClusterJoin, FleetApp,
and fleet-edge with flexible per-site ingress.

Greenfield install: [docs/install.md](docs/install.md) and
[examples/inventory/hub-only/](examples/inventory/hub-only/).

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

- No support, certification, or complete upgrade path from v0.1
- Destructive install and teardown operations
- Operator-owned firewall, TLS, identity, secret, and per-site exposure reviews

## Known limitations (v0.2.0)

- OIDC/SSO is a stub; use local or LDAP auth until Keycloak integration lands.
- FleetApp supports create and delete only (no PATCH/upgrade API).
- Backup/restore is experimental; single-node hub only; spokes not included.
- No hub HA/failover controller; no organization-level tenancy isolation.

Hub images use `umkim_version` / `harbor_fleet_image_tag` (`0.2.0`). Read
[SECURITY.md](SECURITY.md) before deployment.

## Upgrading from v0.1

v0.2 removes DCM, catalog placement, and sibling service-provider deploy paths.
The supported path is a **greenfield hub reinstall** from
[docs/install.md](docs/install.md), not an in-place upgrade from v0.1.

If you upgraded an existing lab hub during v0.2 development, after deploying
`0.2.0` images:

```bash
kubectl -n fleet-console delete deploy dcm-fleet-dashboard --ignore-not-found
kubectl -n fleet-console get deploy umkim-console \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

Confirm the `umkim-console` Deployment exists and references tag `0.2.0`.

## v0.1.0 (previous)

v0.1.0 shipped DCM catalog placement with k8s, Helm, and KubeVirt provider
paths. That model is not part of v0.2. Historical notes:
[docs/archive/README.md](docs/archive/README.md).
