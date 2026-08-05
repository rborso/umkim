# Fleet site controller

> Status: v0.1.0 is an experimental proof of concept. FleetClusterJoin and
> its optional integrations require independent operational validation.

The fleet-site controller manages `FleetSite` and `FleetClusterJoin` resources
and coordinates greenfield workload-cluster bootstrap from the hub.

## Lifecycle

An FCJ normally progresses through:

```text
Pending → ScriptIssued → Enrolled → Validated → Provisioning → Ready
```

Failures are reported in resource status. Review the resource, controller Job,
and cluster state before retrying; do not assume a partial provisioning run is
safe to repeat without inspection.

## Installable providers

For FleetClusterJoin clusters, the only installable provider kinds are:

| Kind | Purpose |
|---|---|
| `k8s` | Container workloads |
| `helm` | Helm releases |
| `kubevirt` | Virtual machines |

Provider actions are available only after the cluster is `Ready`. KubeVirt
installation additionally requires discovered KVM capability. Other provider
paths must not be documented as FCJ-installable.

## Management API

Public fleet-site management API access requires authenticated use through
`fleet-console-auth`. Use the console or an approved service-account client;
do not publish direct controller ports or include credentials in scripts and
examples. Enrollment is a separate, bearer-controlled bootstrap operation.

## Related features

- [fleet-cluster-storage.md](../fleet-cluster-storage.md) — storage intent and
  NFS safety
- [fleet-edge.md](fleet-edge.md) — route exposure
- [fleet-ingress-auth.md](../fleet-ingress-auth.md) — optional forward-auth
- [fleet-llm.md](fleet-llm.md) — optional LLM integration

Network, firewall, certificate, and provider-runtime behavior depend on the
actual deployment. Verify each path using [fleet-verify.md](../fleet-verify.md)
and follow [install.md](../install.md) for the supported installation workflow.
