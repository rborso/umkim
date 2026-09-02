# Fleet LLM

> Status: v0.2 is an experimental proof of concept. The LLM, GPU,
> proxy, and billing-related paths are not production guarantees.

Fleet LLM is an optional hub LiteLLM proxy. GPU inference is the
`llama-cpp-helm` chart installed with FleetApp onto a selected Ready
cluster; see [fleet-apps.md](fleet-apps.md).

## Components

| Component | Role |
|---|---|
| `FleetLLM` | Optional configuration and reconciliation of the hub proxy |
| LiteLLM | Hub-side OpenAI-compatible proxy |
| `llama-cpp-helm` | GPU inference workload discovered as a LiteLLM backend |

The workload is **llama.cpp** using a CUDA `llama-server` image and a GGUF
model source. Its model alias, context size, parallelism, GPU-layer setting,
resource requests, persistence, and runtime class are chart values that must
be reviewed for each deployment.

## Access control

The public LLM API requires authenticated access. The proxy accepts a
platform-administrator session or a scoped Fleet LLM API key; use approved
client configuration and secret management rather than copying credentials into
commands or examples.

Do not expose the LiteLLM service directly as an anonymous endpoint. The
configured proxy and inference workloads may use host networking, so firewall,
network policy, TLS, and ingress validation are required before any public
exposure.

## GPU prerequisites

A usable inference target requires a prepared GPU node, NVIDIA runtime support,
device-plugin-reported capacity, and successful capability discovery. See
[gpu-host-prep.md](gpu-host-prep.md).

## Verify

Run the LLM and GPU checks through [fleet-verify.md](../fleet-verify.md) after
an authenticated deployment. Follow [install.md](../install.md) for the
supported enablement path.
