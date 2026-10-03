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

Use the same chart `model` value (OpenAI alias) on multiple GPU clusters when
you want one logical model id on the hub. Ready `llama-cpp-helm` FleetApps are
synced as **multiple LiteLLM backends** with the same `model_name`; LiteLLM
load-balances across them (`router_settings.routing_strategy: simple-shuffle` in
the hub config). **`deployment_affinity` is not enabled** on the hub proxy: LiteLLM
pins that mode by **API key hash**, which would send all traffic for one Fleet LLM
key to a single spoke. For sticky multi-turn chats, use **`session_affinity`**
(Redis-backed): send header `x-litellm-session-id` or `metadata.session_id` in the
JSON body on each `chat/completions` call in a conversation (TTL
`deployment_affinity_ttl_seconds` in LiteLLM config, default 3600). A small
**Redis** deployment in `fleet-llm` backs the router cache so pins survive LiteLLM
restarts. Hub reconcile clears legacy `deployment_affinity:*` Redis keys when
`fleet_llm_deployment_affinity` is false so load balancing is not pinned to one
spoke by API key.

**Auto backends** (`backendsSource: auto`) are rebuilt only from Ready registered
llama-cpp FleetApps. Removing the last app on a spoke drops that `apiBase` from
`FleetLLM` on the next reconcile; use `umkim llm backends sync-from-apps` to force
a refresh. Manual JSON overrides are not auto-pruned.

For **Qwen3.x** GGUFs, set chart values `jinja: true` and
`chatTemplateKwargs: '{"enable_thinking":false}'` so replies land in
`message.content` instead of filling `reasoning_content`. Shared pieces live
under `deploy/llama-cpp-helm/`:

| Layer | File |
|---|---|
| Fleet GPU runtime | `values-fleet-runtime.yaml` (hostNetwork, NVIDIA runtime, jinja kwargs) |
| Model profile | `profiles/qwen35-2b-ud-4g.yaml` (HF repo, quant, OpenAI `model` alias) |
| Registry overlay | `examples/llama-cpp-helm/overlay-harbor-ghcr-proxy.yaml.example` |

Merge layers with `scripts/render-llama-cpp-values.sh` (later files override), or use the
pre-merged `values-fleet-gpu-4g.yaml`. Ansible flow
`playbooks/flows/llama-cpp-helm-deploy.yml` merges runtime + profile +
`fleet_llama_cpp_image_repository` from inventory (`common.yml`).
Install with `umkim app create --values-file` on the rendered YAML.

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
3. Install `llama-cpp-helm` via console **Apps** (llama profile picker),
   `umkim app create --llama-profile qwen35-2b-ud-4g`, or merged values from
   `deploy/llama-cpp-helm/`. Upgrade with **Edit** / `umkim app update`.
   Ready llama-cpp apps with LiteLLM registration enabled auto-populate
   `FleetLLM` backends (console **Register in hub LiteLLM proxy**, or omit the
   `fleet.umkim.com/llm-backend: "false"` annotation on the FleetApp). Use
   `umkim llm backends sync-from-apps` or console **Sync backends from FleetApps**
   to rebuild from FleetApps. Manual JSON override sets `backendsSource: manual`.
   FleetApp `route` (hub/site ingress) is separate: hub LiteLLM reaches spokes via
   WireGuard `upstream_host`, not route hostnames.
4. `umkim llm reconcile --wait` after backend changes.
5. Create a scoped API key: `umkim auth llm-api-keys create` (or console
   **Access control → LLM API keys**). Call `POST …/llm/v1/chat/completions`
   with `Authorization: Bearer <token>`.
6. Run `scripts/verify-fleet-llm.sh`,
   `scripts/verify-fleet-llama-app-lifecycle.sh`,
   `scripts/verify-fleet-llama-profile.sh`, and
   `FLEET_LLM_CLUSTER=<cluster> FLEET_LLM_MODEL=Qwen3.5-2B-UD SKIP_LLM_DEPLOY=1 scripts/verify-llm-inference-e2e.sh`.

Profile CRUD and verify/chat: [fleet-llama-profiles.md](fleet-llama-profiles.md).

See [fleet-verify.md](../fleet-verify.md) for the full check registry.

Enable the hub proxy with `fleet-llm.yml` (set `fleet_llm_enabled: true` in
`inventory/group_vars/hub/main.yml` or pass `-e fleet_llm_enabled=true`):

```bash
ansible-playbook fleet-llm.yml -l hub --tags fleet_llm_hub -e fleet_llm_enabled=true
```
