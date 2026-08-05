---
name: fleet-controllers-go
description: >-
  Develops and deploys Go controllers for fleet edge routes and FleetSite or
  FleetClusterJoin provisioning. Use for controllers/fleet-edge,
  controllers/fleet-site, their CRDs, or controller playbooks.
---

# Go controllers (S5)

## Components

| Path | Responsibility |
|------|----------------|
| `controllers/fleet-edge/` | Fleet route reconciliation |
| `controllers/fleet-site/` | Fleet site and cluster-join reconciliation |
| `deploy/fleet-site/` | Fleet site and cluster-join CRDs |

## Development contract

- The Go API types, generated manifests, and reconciler tests are the source of
  truth for controller behavior.
- Keep CRD schema, controller logic, OpenAPI where applicable, and component
  documentation aligned.
- Use the controller-specific playbooks and CRD playbooks for deployment; image
  build and push orchestration is defined under `roles/harbor/`.
- Review inventory-driven options before describing or relying on a deployment
  mode. Documentation does not replace those runtime inputs.

## References

- [Fleet site component](../../../docs/components/fleet-site.md)
- [Fleet edge component](../../../docs/components/fleet-edge.md)
- [Design guide](../../../docs/design.md)
- [Agent routing](../../../AGENTS.md)
