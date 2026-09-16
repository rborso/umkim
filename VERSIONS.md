# Versions and release pins

This tree is UMKIM v0.2. Fleet-built images use `harbor_fleet_image_tag`,
which normally follows `umkim_version` (currently `0.2.0`). The previous
public tag was `v0.1.0`.

## Authoritative files

| Need | Source of truth |
| --- | --- |
| UMKIM release tag and fleet image tag | `inventory/group_vars/all/common.yml` (`umkim_version`, `harbor_fleet_image_tag`) |
| Phase 4 platform pins (OLM, cert-manager, Traefik, controller images) | `inventory/group_vars/all/common.yml` |
| K3s channel | `roles/k3s/defaults/main.yml` (`k3s_channel`) |
| OCM bundle and clusteradm | `inventory/group_vars/hub/main.yml` |
| Harbor chart version | `roles/harbor/defaults/main.yml` |
| Public bootstrap defaults | `examples/inventory/hub-only/` |

Run `./scripts/fleet-version.sh` to print resolved pins from the active
inventory.

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
