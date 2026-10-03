# GPU host preparation

> Status: v0.2 is an experimental proof of concept. GPU driver, runtime, and
> workload compatibility must be validated on the target hardware.

Fleet bootstrap does not install GPU drivers automatically during cluster
provision. Prepare every GPU node before FleetClusterJoin bootstrap when
possible (`umkim cluster gpu host-prep` while the cluster is still
`ScriptIssued`), then enable the container runtime and device plugin after the cluster
is ready. Post-Ready repair is supported with `force` (CLI or console).

## Hardware catalog

PCI device IDs, recommended NVIDIA driver module branches, and CUDA repository
URLs per OS major are defined in [`data/gpu/catalog.yaml`](../../data/gpu/catalog.yaml).
The hub Ansible runner (`gpu_host` role) loads this catalog during detection and
selects the driver branch (default **535** for Fleet LLM / `llama.cpp`
`server-cuda`).

Rocky Linux 10 / RHEL 10 nodes use the `el10` CUDA repo entry in the catalog
and install `nvidia-driver` packages directly (no DNF `nvidia-driver` module
stream). RHEL 9 uses `el9` and the `nvidia-driver:535-dkms` module when DKMS
is enabled.

Some GPUs are not supported by the default EL10 **open** kernel module
(`kmod-nvidia-open-dkms`). Per-PCI catalog fields `el10_kmod_flavor: proprietary`
and `el10_driver_version` pin the proprietary `kmod-nvidia-latest-dkms` stack
(for example Pascal mobile parts that require **580.xx** legacy drivers on EL10).

## Required sequence

1. Install a vendor-supported NVIDIA host driver (hub job).
2. Reboot and verify `nvidia-smi` on the node.
3. Bootstrap the cluster (or use **force** host prep on an already Ready cluster).
4. Enable GPU runtime (container toolkit + device plugin) on a Ready cluster.
5. Confirm `fleet.capability/gpu=true` (automatic after `umkim cluster gpu enable --wait`,
   which re-runs capability discovery when runtime enable succeeds).

## Operator interfaces (CLI and console)

Use the fleet API only — do not run `playbooks/flows/gpu-host-prep.yml` or
`gpu-cluster-enable.yml` by hand for normal operations.

| Step | UMKIM CLI | Console |
|------|-----------|---------|
| Status (PCI, driver branch, K8s GPU) | `umkim cluster gpu status NAME` | Cluster → Capabilities + Operations |
| Host driver | `umkim cluster gpu host-prep NAME [--force] [--wait 45m]` | **Prepare GPU host** |
| Runtime + device plugin | `umkim cluster gpu enable NAME [--wait 30m]` | **Enable GPU runtime** |
| Re-discover | `umkim cluster gpu discover NAME` or `umkim cluster discover NAME` | **Discover capabilities** (also runs at end of `gpu enable --wait`) |

On Ready clusters, host prep requires `--force` (the console sends `force: true`
automatically).

Example:

```bash
umkim cluster gpu status lab-gpu-0
umkim cluster gpu host-prep lab-gpu-0 --force --wait 45m
umkim cluster gpu enable lab-gpu-0 --wait 30m
umkim cluster list --require gpu
```

Hub-side implementation: annotations on `FleetClusterJoin` trigger Ansible runner
jobs (`cluster-gpu-host-prep.yml`, `cluster-gpu-enable.yml`) with inventory from
the cluster ConfigMap — same pattern as storage and KubeVirt platform install.

`cluster-gpu-enable.yml` also applies `fleet_spoke_firewall` when
`configure_firewalld: true` on cluster nodes: trust `wg-umkim`, allow the hub
overlay (`wireguard_subnet`) to reach K3s API ports and hostNetwork LLM
inference (`:8080`). Standalone:
`playbooks/flows/gpu-cluster-firewall.yml` with the cluster flow example vars.

## Inference terminology and runtime

The deployed GPU inference chart is **llama.cpp**, using the CUDA
`llama-server` image.

Capability discovery uses the device plugin’s reported GPU capacity. A host
driver alone is not evidence that the scheduler can place GPU workloads.

See [fleet-llm.md](fleet-llm.md), [install.md](../install.md), and
[fleet-verify.md](../fleet-verify.md).
