# Fleet site controller

> Status: v0.2 is an experimental proof of concept. FleetClusterJoin and
> its optional integrations require independent operational validation.

The fleet-site controller manages `FleetSite`, `FleetClusterJoin`,
`FleetApp`, and `FleetChart` resources and coordinates greenfield workload-cluster bootstrap
from the hub.

## Lifecycle

An FCJ normally progresses through:

```text
Pending → ScriptIssued → Enrolled → Validated → Provisioning → Ready
```

Failures are reported in resource status. Review the resource, controller Job,
and cluster state before retrying; do not assume a partial provisioning run is
safe to repeat without inspection.

Cluster deletion **defaults to detach**: hub-managed resources are removed
while K3s and workloads on the spoke are preserved. Full Kubernetes cleanup
is opt-in (`cleanupKubernetes=true` on delete, or
`fleet_flow_cleanup_kubernetes=true` in
[playbooks/flows/cluster-delete.yml](../../playbooks/flows/cluster-delete.yml)).

## Site ingress

Each `FleetSite` carries `spec.edge.ingress[]` (port-forward and/or
reverse-proxy entries) and optional `hubGateway` for hub-routed DNS. Clusters
bind with `FleetClusterJoin.spec.edge.ingressId`. When a site has exactly one
`port_forward` ingress, an empty `ingressId` auto-binds at reconcile time.
Inventory examples: [../../examples/site-profiles/](../../examples/site-profiles/).

Match per-app `FleetApp` `route.mode` to cluster ingress:

- `hub` with site `hubGateway` (and hub zone DNS)
- `site` with `port_forward` or `reverse_proxy` ingress binding
- `none` for chart-owned or internal ingress

See [fleet-edge.md](fleet-edge.md).

## Hub independence

Spokes are independent K3s control planes. The hub registers and manages them
via OCM; it does not run their workloads.

- **Hub down:** console and hub-routed URLs stop; workloads on spokes keep
  running.
- **Detach:** default delete keeps the spoke cluster and Helm releases.

## Helm charts (FleetApp)

After the cluster is `Ready`, install OCI charts with `FleetApp`. You must
set `spec.clusterName`; the controller does not choose a cluster. Publish
charts to the hub library (`FleetChart`) first when you want reuse. See
[fleet-apps.md](fleet-apps.md).

KubeVirt remains an optional spoke platform add-on (KVM capability), not a
chart-install provider.

## Post-Ready node operations

When a cluster is `Ready`, operators can add or remove workers and replace
control-plane nodes through the console **Operations** panel or the fleet-site
API:

| Operation | API |
|-----------|-----|
| Add worker | `POST /clusters/{name}/nodes` |
| Remove worker | `DELETE /clusters/{name}/nodes/{node}?force=true` |
| Replace control plane | `POST /clusters/{name}/control-plane/replace` |
| Validate node operation (pre/post) | `POST /clusters/{name}/node-operations/validate?phase=pre\|post` |
| List operation history | `GET /clusters/{name}/node-operations` |
| Operation Ansible logs | `GET /clusters/{name}/node-operations/{id}/logs` |

Hub Ansible jobs (provision, storage, node operations) reach cluster nodes through
`FleetClusterJoin.spec.jumphost`, or `FleetSite.spec.jumphost` for the cluster's
site when the cluster field is empty. Use `user@host:port` (port optional). When
set, jobs SSH to management LAN addresses via `ProxyJump` instead of the WireGuard
overlay — required for sites where only a bastion can reach lab nodes.

Edit jumphost (and site region / failure domain) in the console **Sites** detail
(Edge and DNS → Connectivity) or via `umkim site patch`. Edit **ingress entries**
(`port_forward` / `reverse_proxy`) in Sites detail or
`umkim site ingress list|add|set|delete`. Per-cluster ingress binding:
console **Clusters** detail (Edge and ingress) or `umkim cluster patch --ingress-id`.
`umkim cluster bootstrap` resolves jumphost as cluster override, then site default.

When a cluster reaches **Ready**, the controller ensures a hub secret
`fleet-system/fleet-kubeconfig-<cluster>` exists (SSH to the spoke API over the
overlay). Provision playbooks sync it immediately after CSR approval; if a
provision job times out or fails later, a dedicated kubeconfig-sync Ansible job
is retried automatically so node list APIs keep working.

Workers join during initial provision when listed in the FCJ `spec.nodes` at
create time. Post-Ready add uses the same Ansible agent-join path without
re-running full cluster provision.

Offline removal or replacement (`force=true`) skips cordon/drain on unreachable
hosts and may leave orphaned disk state; run `cleanup_node_local.sh` on revived
machines before reuse.

Each operation follows **validate → execute → validate**:

1. **Pre-validation** — `POST .../node-operations/validate` (or the same checks
   run automatically when submitting add/remove/replace). Fails fast on cluster
   state, duplicate addresses, or missing nodes.
2. **Execute** — existing add/remove/replace APIs queue an Ansible job.
3. **Post-validation** — controller and `?phase=post` confirm inventory and
   spoke node state after the job completes.

Completed operations are appended to `FleetClusterJoin.status.nodeOperationHistory`
(newest first, capped at 20). Each record stores actor identity, timestamps, outcome,
and a tail of the Ansible job log so operators can review failures after the Job TTL
expires.

CLI equivalent: [playbooks/flows/cluster-node-op.yml](../../playbooks/flows/cluster-node-op.yml).

## Management API

Public fleet-site management API access requires authenticated use through
`fleet-console-auth`. Use the console or an approved service-account client;
do not publish direct controller ports or include credentials in scripts and
examples. Enrollment is a separate, bearer-controlled bootstrap operation.

## Related features

- [fleet-apps.md](fleet-apps.md) — Helm install onto a selected cluster
- [fleet-cluster-storage.md](../fleet-cluster-storage.md) — storage intent and
  NFS safety
- [fleet-edge.md](fleet-edge.md) — site exposure and route configuration
- [fleet-ingress-auth.md](../fleet-ingress-auth.md) — optional forward-auth
- [fleet-llm.md](fleet-llm.md) — optional LLM integration
- [../../examples/site-profiles/](../../examples/site-profiles/) — exposure inventory examples

Network, firewall, certificate, and workload behavior depend on the
live environment and must be verified there.
