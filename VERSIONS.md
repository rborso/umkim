# Versions and release pins

This tree is UMKIM v0.3.0. Fleet-built images use `harbor_fleet_image_tag`,
which normally follows `umkim_version`. The previous public tag was `v0.2.0`.

## Greenfield platform pins (0.3.0)

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

Hub install builds fleet images from this checkout.

## YAIDAP provisioner (optional)

When `FleetSite.spec.provision.provider` is `yaidap`, clone the sibling YAIDAP
repository on the site jumphost at or above:

| Component | Minimum pin | Notes |
| --- | --- | --- |
| YAIDAP | `main` @ **b8d7851** (pushed 2026-10-02; rebases 63bd7c1 onto 52bd22f TempXE) | `fleet_ansible_ssh_pubkey` (+ `fleet_provision_fleet_ansible_ssh_pubkey`) in cloud-init/kickstart; placement+PXE already on jumphost — see [docs/components/yaidap-integration.md](docs/components/yaidap-integration.md) |

Record the resolved git SHA in site `provision` status after
`umkim site provision yaidap-sync`.

## Image policy

- Fleet-owned images are built into Harbor's `fleet` project.
- OCI charts used by FleetApp are published into Harbor's `charts` project.
- Base images and downloaded tools must use the pinned values in their build
  definitions.
- Mutable `main` and `latest` tags are not release pins.

For supported scope and known limitations, see
[RELEASE_NOTES.md](RELEASE_NOTES.md).
