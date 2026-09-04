# Port-forward profile

Inventory example for a site with one or more `ingress[]` rows using
`method: port_forward` (WAN DNAT to cluster Traefik NodePorts). Bind each
cluster with `ingressId` set to that row's `id`.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Replace the site, ingress address/zone, and TLS placeholders in local inventory.
3. Configure the router and public DNS outside this repository, using the
   approved destination and access policy for the deployment.
4. Validate the ingress configuration before applying the relevant playbook.

[`openwrt-uci.example`](openwrt-uci.example) is a commented router example, not
an executable deployment contract. Use `FleetApp` `route.mode: site` for apps
published through spoke ingress. Site-routed apps can remain reachable when
the hub is down if ingress is healthy.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
