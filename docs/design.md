# UMKIM design

This document describes the inventory-independent architecture. For an
operator runbook, use [install.md](install.md). Inventory values belong in
`inventory/` or [examples/inventory/hub-only/](../examples/inventory/hub-only/).

> [!WARNING]
> UMKIM v0.1.0 is an experimental reference stack. It is not a supported or
> production-certified platform.

## Model

UMKIM operates a multi-cluster fleet from one hub.

| Layer | Responsibility |
| --- | --- |
| Hub | K3s, OCM, DCM, Harbor, console, and optional edge services |
| Site | A `FleetSite` logical location with placement and exposure metadata |
| Cluster | An independent K3s data plane represented by `FleetClusterJoin` and OCM `ManagedCluster` |
| Provider | A service provider that materializes a placed workload |

The hub is infrastructure, not a workload `ManagedCluster`. Each workload
cluster runs its own K3s server and is managed by OCM after it joins.

## Network and control planes

The management LAN carries cluster-local Kubernetes traffic. A WireGuard
overlay provides hub-to-cluster management reachability. Do not expose
management APIs, registry endpoints, or Kubernetes NodePorts to the public
network by default; see [../SECURITY.md](../SECURITY.md).

OCM registers workload clusters and delivers platform manifests through
`ManifestWork`. DCM evaluates catalog and placement intent, then invokes a
provider on an eligible cluster.

## Cluster lifecycle

1. An operator creates a registry `FleetSite`.
2. The operator creates a `FleetClusterJoin` for a non-management site.
3. The controller issues a bootstrap bundle containing enrollment material.
4. The operator runs the bundle on the cluster control node.
5. After local K3s validation, the hub provisions OCM registration and platform
   manifests.
6. The cluster becomes Ready and can receive optional provider installation.

The public lifecycle phases are `Pending`, `ScriptIssued`, `Enrolled`,
`Validated`, `Provisioning`, and `Ready`. Cluster deletion defaults to detach:
it removes hub-managed resources while preserving the cluster unless an
operator explicitly requests Kubernetes cleanup.

## Providers and workloads

FleetClusterJoin supports post-Ready installation of these workload providers:

- **k8s:** deploys container workloads.
- **Helm:** installs allowed OCI charts.
- **KubeVirt:** provisions VM workloads where the cluster is prepared for it.

OLM and cert-manager are platform prerequisites. The OLM Subscription provider
is deployed separately and is not a FleetClusterJoin provider choice.

DCM uses labels, policy, and capacity signals to choose a cluster. Placement
count is the number of selected clusters; workload replicas are pods per
selected cluster. The hub itself is not a placement target.

## Edge routing

`FleetAppRoute` represents an application route. The edge controller reconciles
hub Traefik configuration and, when required, spoke ingress manifests.

| Routing mode | Intended use |
| --- | --- |
| `hub` | Hub gateway forwards to a selected workload cluster |
| `site` | A site exposes its own ingress path |
| `none` | No fleet-managed public route |

Exposure and TLS settings must be compatible with the selected network path.
See [components/fleet-edge.md](components/fleet-edge.md) for the supported
model. Active–backup behavior is experimental; active–active routing is not a
v0.1.0 feature.

## Optional services

- **Harbor** hosts fleet-built images and may proxy approved upstream sources.
- **Ingress authentication** protects selected public application routes; it is
  separate from console login.
- **Backup/restore** is experimental and must be tested for every application.
- **Fleet LLM** is optional and requires an explicit security and exposure
  review.

## Source map

| Topic | Reference |
| --- | --- |
| Hub install | [install.md](install.md) |
| FleetSite and FleetClusterJoin | [components/fleet-site.md](components/fleet-site.md) |
| Placement and catalog | [components/dcm.md](components/dcm.md) |
| Edge routes | [components/fleet-edge.md](components/fleet-edge.md) |
| Versions | [../VERSIONS.md](../VERSIONS.md) |
| Security | [../SECURITY.md](../SECURITY.md) |
