# Versions and release pins

This tree is UMKIM v0.2. Fleet-built images use `harbor_fleet_image_tag`,
which normally follows `umkim_version` (currently `0.2.1`). The previous
public tag was `v0.1.0`.

## Greenfield platform pins (0.2.1)

| Component | Pin | Source |
| --- | --- | --- |
| K3s channel | `v1.36` → `1.36.4+k3s1` | `roles/k3s/defaults/main.yml` |
| Calico | `v3.31.3` | `roles/calico/defaults/main.yml` |
| cert-manager | `1.20.4` | `inventory/group_vars/all/common.yml` |
| OLM | `0.45.0` | `inventory/group_vars/all/common.yml` |
| Traefik (hub + spoke) | `v3.6.25` @ digest | `roles/hub_ingress`, `roles/spoke_ingress`, `common.yml` |
| Harbor Helm chart | `1.18.4` | `roles/harbor/defaults/main.yml` |
| LiteLLM | `v1.102.2` | `roles/fleet_llm/defaults/main.yml` |
| NVIDIA device plugin | `v0.19.3` | `roles/gpu_host/defaults/main.yml` |
| ansible-runner kubectl | `v1.36.4` | `images/fleet-ansible-runner/Dockerfile` |
| ansible-runner Helm | `v3.17.4` | `images/fleet-ansible-runner/Dockerfile` |
| ansible-runner Ansible Core | `2.18.8` | `images/fleet-ansible-runner/Dockerfile` |
| Fleet controllers client-go | `v0.36.4` / controller-runtime `v0.24.1` | `controllers/fleet-site/go.mod` |

## Authoritative files

| Need | Source of truth |
| --- | --- |
| UMKIM release tag and fleet image tag | `inventory/group_vars/all/common.yml` (`umkim_version`, `harbor_fleet_image_tag`) |
| Phase 4 platform pins (OLM, cert-manager, Traefik, controller images) | `inventory/group_vars/all/common.yml` |
| K3s channel | `roles/k3s/defaults/main.yml` (`k3s_channel`) |
| OCM bundle and clusteradm | `inventory/group_vars/hub/main.yml` |
| Harbor chart version | `roles/harbor/defaults/main.yml` |
| Public bootstrap defaults | `examples/inventory/hub-only/` |
| NVIDIA driver branch for GPU catalog / Fleet LLM | `data/gpu/catalog.yaml` (`driver_branches.535`) |

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
