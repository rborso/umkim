# Fleet Llama profiles

Hub-stored **FleetLlamaProfile** custom resources (namespace: fleet console) describe
`llama-cpp-helm` value overlays. The fleet-site API merges each profile with the
fleet runtime values and Harbor image overlay, same as embedded files under
`deploy/llama-cpp-helm/`.

## CRDs

| Kind | Purpose |
|------|---------|
| `FleetLlamaProfile` | `spec.profileYaml`, chart pin, optional `spec.system` for seeded built-ins |
| `FleetLlamaProfileTest` | One verify session (quick or extended) |

System profiles (for example `qwen35-2b-ud-4g`, `qwen35-2b-ud-small` for guests
with about 4–6 GiB RAM) are created on controller start when missing.

## HTTP API (`/llama-profiles`)

| Method | Path | Notes |
|--------|------|-------|
| GET/POST | `/llama-profiles` | List / create |
| GET/PATCH/DELETE | `/llama-profiles/{name}` | System delete needs `?force=true` |
| GET | `/llama-profiles/{name}/values` | Merged `valuesYaml` |
| POST | `/llama-profiles/{name}/verify` | Body: `clusterName`, `mode` (`quick` \| `extended`), optional `wait` |
| GET | `/llama-profiles/tests` | Optional `extended=true`, `active=true` |
| GET/DELETE | `/llama-profiles/tests/{id}` | Status / end extended test |
| POST | `/llama-profiles/tests/{id}/chat/completions` | Hub proxy to spoke `llama-server` (extended, Ready only) |

Legacy `GET /charts/llama-cpp-helm/profiles` lists the same profile ids for the
Apps install picker.

## Verify modes

- **Quick** — install FleetApp on the chosen Ready cluster, smoke `GET /v1/models`
  over WireGuard, delete the app, mark test `Succeeded`.
- **Extended** — keep the FleetApp, set test `Ready`, use the hub proxy for chat
  (no browser access to spoke IPs). End with `DELETE` on the test.

FleetApps created for tests carry annotations
`fleet.umkim.com/llama-profile` and `fleet.umkim.com/llama-profile-test`.

## Security

Mutating routes require console identity headers. Extended chat is allowed only
when the test CR is `Ready` and `mode=extended`. Streaming is not supported in v1.

## CLI and console

- `umkim llm profile …` — CRUD, `values`, `verify`, `test chat`, `test end`
- Console **Apps → Llama profiles** — same workflows with verify modal and chat panel

## Spoke DNS and `hostNetwork`

`llama-cpp-helm` uses `hostNetwork` so the hub can reach `llama-server` on the
node WireGuard IP. With `hostNetwork`, Kubernetes defaults to
`dnsPolicy: ClusterFirstWithHostNet`, which still queries cluster CoreDNS. On some
GPU spokes the pod CNI cannot reach the node's upstream resolver (CoreDNS logs
show `SERVFAIL` / timeouts forwarding to the LAN nameserver), while the node
itself resolves public names fine.

Chart **0.1.4+** sets `dnsPolicy: Default` when `hostNetwork` is enabled so
Hugging Face model download uses the node's `resolv.conf`. Clusters where pod
network DNS works for the internet do not need this change.

## Verification

`scripts/verify-fleet-llama-profile.sh` (GPU homelab profile in check registry).
