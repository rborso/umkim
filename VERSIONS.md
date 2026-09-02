# Versions and release pins

This tree is UMKIM v0.2. Fleet-built images use `harbor_fleet_image_tag`,
which normally follows `umkim_version` (currently `0.2.0-dev`). The previous
public tag was `v0.1.0`.

## Authoritative files

| Need | Source of truth |
| --- | --- |
| UMKIM, K3s, OCM, platform, and component versions | `inventory/group_vars/all/common.yml` and role defaults |
| Fleet image tag | `harbor_fleet_image_tag` |
| Public bootstrap defaults | `examples/inventory/hub-only/` |

Hub install builds fleet images from this checkout. Sibling service-provider
repositories are not required.

## Image policy

- Fleet-owned images are built into Harbor's `fleet` project.
- OCI charts used by FleetApp are published into Harbor's `charts` project.
- Base images and downloaded tools must use the pinned values in their build
  definitions.
- Mutable `main` and `latest` tags are not release pins.

For supported scope and known limitations, see
[RELEASE_NOTES.md](RELEASE_NOTES.md).
