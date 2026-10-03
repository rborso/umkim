# Hub gateway profile

Inventory example for a site with `hub_gateway: true` (public HTTP/S on the hub;
spokes reached over WireGuard). Typical for the management site. Workload sites
may also enable hub gateway for `FleetApp` `route.mode: hub`.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Set the site identity, `hub_zone` / `hub_wildcard`, and WAN notes in your
   local inventory using approved placeholders and deployment values.
3. Confirm that the external DNS, network, and access-control prerequisites are
   operated outside this repository.
4. Validate inventory before running the relevant edge playbook.

The snippet records desired configuration; it does not create external DNS,
firewall, certificate, or routing resources. Use `FleetApp` `route.mode: hub`
for apps published through the hub gateway. Hub-routed URLs require a healthy
hub.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
