# Hub gateway profile

`exposure_mode: hub_gateway` is an inventory example for traffic routed through
the fleet gateway.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Set the site identity, domain, endpoint, and certificate policy in your
   local inventory using approved placeholders and deployment values.
3. Confirm that the external DNS, network, and access-control prerequisites are
   operated outside this repository.
4. Validate inventory before running the relevant edge playbook.

The snippet records desired configuration; it does not create external DNS,
firewall, certificate, or routing resources. Refer to the component guide for
the current controller contract.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
