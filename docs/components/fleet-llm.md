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

Prerequisites on at least one spoke cluster:

1. GPU host driver and runtime via the fleet API ([gpu-host-prep.md](gpu-host-prep.md)) —
   `umkim cluster gpu host-prep` / `umkim cluster gpu enable` (or console), then
   `umkim cluster gpu discover` until `fleet.capability/gpu=true`.
2. Hub LiteLLM enabled — `umkim llm set --hub-enabled`, then
   `umkim llm reconcile --wait` (or console **Infrastructure → LLM platform**).
3. Install `llama-cpp-helm` with FleetApp on the GPU cluster (for example
   `deploy/llama-cpp-helm/values-fleet-gpu-4g.yaml` on ~4GB GPUs). Ready
   llama-cpp apps auto-populate `FleetLLM` backends; override with
   `umkim llm set --backends-file …` or console JSON override if needed, then
   `umkim llm reconcile`.
4. Create a scoped API key: `umkim auth llm-api-keys create` (or console
   **Access control → LLM API keys**). Call `POST …/llm/v1/chat/completions`
   with `Authorization: Bearer <token>`.
5. Run `scripts/verify-fleet-llm.sh` and
   `FLEET_LLM_CLUSTER=<cluster> FLEET_LLM_MODEL=Qwen3.5-2B-UD scripts/verify-llm-inference-e2e.sh`.

See [fleet-verify.md](../fleet-verify.md) for the full check registry.

Enable the hub proxy with `fleet-llm.yml` (set `fleet_llm_enabled: true` in
`inventory/group_vars/hub/main.yml` or pass `-e fleet_llm_enabled=true`):

```bash
ansible-playbook fleet-llm.yml -l hub --tags fleet_llm_hub -e fleet_llm_enabled=true
```
