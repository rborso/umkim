# GPU host preparation

> Status: v0.2 is an experimental proof of concept. GPU driver, runtime, and
> workload compatibility must be validated on the target hardware.

Fleet bootstrap does not install GPU drivers. Prepare every GPU node before
FleetClusterJoin bootstrap, then enable the container runtime and device plugin
after the cluster is ready.

## Required sequence

1. Install a vendor-supported NVIDIA driver compatible with the running kernel.
2. Reboot and verify that the host driver is functional.
3. Bootstrap the cluster.
4. Run the GPU-cluster enable flow to install the NVIDIA container toolkit,
   configure the K3s runtime, and deploy the device plugin.
5. Run capability discovery and verify that Kubernetes reports GPU capacity.

The repository provides `gpu-host-prep.yml` for host preparation and
`playbooks/flows/gpu-cluster-enable.yml` for post-ready runtime setup. Review
their inventory inputs and Secure Boot requirements before use.

## Inference terminology and runtime

The deployed GPU inference chart is **llama.cpp**, using the CUDA
`llama-server` image.

The default `llama-cpp-helm` values use an NVIDIA runtime class and
`hostNetwork: true`. They do not request `nvidia.com/gpu` unless
`useGpuResource` is enabled. Review those defaults, host networking, resource
requests, model cache storage, and node isolation before deploying inference.

Capability discovery uses the device plugin’s reported GPU capacity. A host
driver alone is not evidence that the scheduler can place GPU workloads.

Firewall and runtime isolation are deployment-specific; this release does not
claim to solve them automatically. See [fleet-llm.md](fleet-llm.md),
[install.md](../install.md), and [fleet-verify.md](../fleet-verify.md).
