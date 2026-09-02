# Site profiles

These are configuration examples for `FleetSite.spec.edge.exposureMode`
(inventory: `exposure_mode`). They illustrate inventory inputs only; they do
not configure DNS, routers, certificates, or controller behavior.

A multi-site fleet can use a **different profile per site**. Outbound-only
reachability via the hub is one supported pattern (`hub_gateway`), not the
only topology.

| Profile | `exposure_mode` / `exposureMode` |
|---------|----------------------------------|
| [Hub gateway](hub-gateway/) | `hub_gateway` |
| [Port forward](port-forward/) | `port_forward` |
| [Reverse proxy](reverse-proxy/) | `reverse_proxy` |
| [Internal](internal/) | `none` |

Inventory snippets use `exposure_mode` (Ansible → API). The CRD field is
`exposureMode`. Values are the same.

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
