# Internal profile

`exposure_mode: none` is an inventory example for a site without public
ingress. Access and name resolution remain private deployment concerns.

## Apply

1. Start from [`fleet_sites.yml.snippet`](fleet_sites.yml.snippet).
2. Replace the site identity and domain placeholders in local inventory.
3. Confirm that private connectivity, authorization, and any private name
   resolution are managed by the deployment owner.
4. Validate inventory before applying a scoped playbook.

This profile expresses a no-public-exposure intent. It does not create private
network connectivity, access policies, or internal DNS records.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
- [Installation guide](../../../docs/install.md)
