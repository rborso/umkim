# DCM control plane and service providers

> Status: v0.1.0 is an experimental proof of concept. Catalog, placement,
> tenancy, and failover behavior require independent validation.

DCM runs on the hub and provides the catalog, policy evaluation, placement, and
service-provider registration used to deploy workloads to managed clusters.

## FleetClusterJoin providers

For FleetClusterJoin clusters, the supported installable provider actions are
limited to:

| Provider | Workload type |
|---|---|
| `k8s` | Container workloads |
| `helm` | Helm releases |
| `kubevirt` | Virtual machines on clusters with discovered KVM capability |

These actions are available after the cluster reaches `Ready`. Do not describe
other providers as FCJ-installable.

## Catalog and placement

Catalog instances are placed by policy and provider availability. A successful
placement preview or provider registration is not proof that an application is
healthy, reachable, durable, or isolated. Validate the deployed workload,
storage, ingress, and recovery behavior separately.

The optional GPU inference catalog item is `llama-cpp-helm`, which deploys
llama.cpp; see [fleet-llm.md](fleet-llm.md).

## API access

Public DCM API access must use the authenticated `fleet-console-auth` proxy.
Use the console or an approved service-account client, and do not expose
internal controller or provider endpoints as public APIs.

## Related documentation

- [fleet-site.md](fleet-site.md) — FleetClusterJoin lifecycle
- [harbor.md](harbor.md) — fleet image and chart publishing
- [fleet-edge.md](fleet-edge.md) — workload routing
- [fleet-verify.md](../fleet-verify.md) — placement and deployment checks

Follow [install.md](../install.md) for the supported deployment path.
