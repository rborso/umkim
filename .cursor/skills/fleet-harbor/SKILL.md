---
name: fleet-harbor
description: >-
  Deploys Harbor, platform ManifestWorks (OLM, cert-manager), and FleetApp
  chart publishing. Use for harbor.yml, fleet-platform.yml, roles/harbor,
  roles/olm, or OCI chart pushes.
---

# Harbor & platform (S3)

## Scope

Use `harbor.yml` for registry and chart-publish workflows and
`fleet-platform.yml` for platform ManifestWorks. Helm installs onto a selected
Ready cluster use `FleetApp`. There is no catalog or service-provider path.
Publish reusable charts with `FleetChart` (`scripts/fleet-chart.sh` or Apps Library).

## Working guidance

- Use `harbor_push_fleet` through the Harbor role; registry host, project,
  image list, and source locations are inventory-controlled.
- Treat `roles/harbor/tasks/push_fleet_image.yml` as the build-and-push
  contract. Do not publish copied registry endpoints, tags, or external source
  locations in documentation.
- Platform work is derived from templates under `roles/olm/` and related
  roles; update component documentation when those contracts change.
- Verify against the authorized deployment configuration, not an assumed local
  endpoint.

## References

- [Harbor component](../../../docs/components/harbor.md)
- [FleetApp Helm installs](../../../docs/components/fleet-apps.md)
- [Installation guide](../../../docs/install.md)
- [Design guide](../../../docs/design.md)
