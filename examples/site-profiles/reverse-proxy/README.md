# Reverse-proxy profile

`exposure_mode: reverse_proxy` is an inventory example for a proxy that is
operated outside the fleet and manages the external TLS boundary.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Replace all site, endpoint, domain, and certificate-policy placeholders in
   local inventory.
3. Configure the external proxy, DNS, and network controls through their own
   approved procedures.
4. Validate the resulting inventory and ingress behavior before deployment.

[`nginx-vhost.example`](nginx-vhost.example) is a commented reference only.
Neither it nor this profile creates proxy configuration, certificates, DNS
records, or network access.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
