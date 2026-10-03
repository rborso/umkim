# Reverse-proxy profile

Inventory example for a site with an `ingress[]` row using
`method: reverse_proxy` (TLS terminated on an external proxy). A site may also
include `port_forward` rows; bind each cluster with `ingressId`.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Replace all site and ingress placeholders in local inventory.
3. Configure the external proxy, DNS, and network controls through their own
   approved procedures.
4. Validate the resulting inventory and ingress behavior before deployment.

[`nginx-vhost.example`](nginx-vhost.example) is a commented reference only.
Use `FleetApp` `route.mode: site` for apps published through spoke ingress.
Site-routed apps can remain reachable when the hub is down if ingress is healthy.
Ingress auth and spoke ACME apply to `port_forward` only, not reverse proxy.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
