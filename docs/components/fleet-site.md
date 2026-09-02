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

## Site exposure

Each `FleetSite` carries edge metadata, including `exposureMode` (`hub_gateway`,
`port_forward`, `reverse_proxy`, `none`). Clusters can inherit or override.
Inventory examples: [../../examples/site-profiles/](../../examples/site-profiles/).

Match per-app `FleetApp` `route.mode` to site exposure:

- `hub` with `hub_gateway`
- `site` with `port_forward` or `reverse_proxy`
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
