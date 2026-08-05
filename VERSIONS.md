# Versions and release pins

UMKIM v0.1.0 is released as Git tag `v0.1.0`. Fleet-built images use
`harbor_fleet_image_tag`, which normally follows `umkim_version`.

## Authoritative files

| Need | Source of truth |
| --- | --- |
| UMKIM, K3s, OCM, platform, and component versions | `inventory/group_vars/all/common.yml` and role defaults |
| Fleet image tag | `harbor_fleet_image_tag` |
| Sibling repository commits | `inventory/group_vars/all/fleet_git_pins.yml` |
| Public bootstrap defaults | `examples/inventory/hub-only/` |

Do not copy sibling commit hashes into another document. The executable pin file
is the release record and is copied by the inventory bootstrap process.

## Verify a release checkout

```bash
./scripts/fleet-checkout-pins.sh
./scripts/fleet-verify-pins.sh --strict
```

The Harbor image build uses local sibling clones. Pin enforcement is opt-in, so
use the strict check before building a release image.

## Image policy

- Fleet-owned images are built into Harbor's `fleet` project.
- Base images and downloaded tools must use the pinned values in their build
  definitions.
- Mutable `main` and `latest` tags are not release pins.

For supported scope and known limitations, see
[RELEASE_NOTES.md](RELEASE_NOTES.md).
