# Site profiles

These are configuration examples for `FleetSite.spec.edge` ingress and optional
hub gateway settings (inventory under `fleet_sites:`). They illustrate inventory
inputs only; they do not configure DNS, routers, certificates, or controller
behavior.

A multi-site fleet can use a **different profile per site**. Outbound-only
reachability via the hub is one supported pattern (`hub_gateway: true`), not the
only topology.

| Profile | Inventory shape |
|---------|-----------------|
| [Hub gateway](hub-gateway/) | `hub_gateway: true` (+ `hub_zone` / `hub_wildcard`) |
| [Port forward](port-forward/) | `ingress[]` with `method: port_forward` |
| [Reverse proxy](reverse-proxy/) | `ingress[]` with `method: reverse_proxy` (may combine with PF) |
| [Internal](internal/) | No `ingress` rows; no hub gateway |

Clusters bind to an ingress row with `FleetClusterJoin.spec.edge.ingressId`
(inventory/API: `ingressId` / `ingress_id`).

## Use an example safely

1. Read the chosen profile and its `fleet_sites.yml.snippet`.
2. Copy only the needed fields into your local
   `inventory/group_vars/all/fleet_sites.yml`.
3. Replace every placeholder with values approved for that deployment.
4. Validate the resulting inventory and use the applicable scoped playbook.

The controller API and supported edge behavior are defined by the source and
the [fleet edge component guide](../../docs/components/fleet-edge.md). These
examples deliberately do not duplicate an exposure or lifecycle matrix.

## References

- [Design guide](../../docs/design.md)
- [Installation guide](../../docs/install.md)
- [Fleet site component](../../docs/components/fleet-site.md)
