# Port-forward profile

`exposure_mode: port_forward` is an inventory example for direct forwarding
from an operator-managed router to cluster ingress.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Replace the site, endpoint, domain, and TLS placeholders in local inventory.
3. Configure the router and public DNS outside this repository, using the
   approved destination and access policy for the deployment.
4. Validate the ingress configuration before applying the relevant playbook.

[`openwrt-uci.example`](openwrt-uci.example) is a commented router example, not
an executable deployment contract. The snippet declares intent only; it does
not alter router, DNS, certificate, or controller state.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
