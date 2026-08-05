---
name: fleet-dcm-harbor
description: >-
  Deploys and configures the DCM control plane, platform ManifestWorks, and
  Harbor registry. Use for fleet-platform.yml, fleet-dcm.yml, harbor.yml,
  roles/dcm, roles/olm, provider workflows, or fleet image pushes.
---

# DCM & Harbor (S3)

## Scope

Use `harbor.yml` for registry workflows, `fleet-platform.yml` for platform
components, and `fleet-dcm.yml` for the DCM stack. Provider-specific playbooks
and the roles they call define their own inputs and lifecycle.

## Working guidance

- Use `harbor_push_fleet` through the Harbor role; registry host, project,
  image list, and source locations are inventory-controlled.
- Treat `roles/harbor/tasks/push_fleet_image.yml` as the build-and-push
  contract. Do not publish copied registry endpoints, tags, or external source
  locations in documentation.
- Rendered platform and provider work is derived from templates under
  `roles/olm/`, `roles/dcm/`, and related roles; update component documentation
  when those contracts change.
- Verify against the authorized deployment configuration, not an assumed local
  endpoint.

## References

- [DCM component](../../../docs/components/dcm.md)
- [Harbor component](../../../docs/components/harbor.md)
- [Installation guide](../../../docs/install.md)
- [Design guide](../../../docs/design.md)
