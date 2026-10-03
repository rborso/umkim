---
name: fleet-edge-ingress
description: >-
  Configures fleet edge routing, spoke ingress ManifestWorks, and ingress
  automation. Use for fleet-edge.yml, fleet-spoke-ingress.yml, spoke_ingress,
  or fleet edge inventory.
---

# Fleet edge & ingress (S4)

## Scope

Use this skill for the edge route and ingress path. The primary playbooks are
`fleet-edge.yml`, `fleet-spoke-ingress.yml`, and `fleet-edge-cleanup.yml`;
their roles and inventory inputs define the active behavior.

## Working guidance

- Site ingress: optional `hubGateway`, plus `ingress[]` methods
  `port_forward` / `reverse_proxy` (clusters bind with `ingressId`).
  Inventory examples: [examples/site-profiles/](../../../examples/site-profiles/).
- `FleetApp` routing: `none`, `hub`, `site` only. Do not document or add
  `route.mode: dual` (deprecated).
- Resolve upstreams, exposure configuration, and access controls from the
  active inventory rather than examples or documentation.
- Keep controller-driven `FleetAppRoute` resources and Ansible-managed ingress
  behavior consistent with their source contracts.
- Use the fleet-edge component guide for exposure and TLS decisions instead of
  duplicating an environment-specific matrix.
- Run verification only against an authorized target supplied by inventory or
  runtime configuration.

## References

- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [FleetApp Helm installs](../../../docs/components/fleet-apps.md)
- [Design guide](../../../docs/design.md)
- [Installation guide](../../../docs/install.md)
