# UMKIM design

This document describes the inventory-independent architecture. For an
operator runbook, use [install.md](install.md). Inventory values belong in
`inventory/` or [examples/inventory/hub-only/](../examples/inventory/hub-only/).

> [!WARNING]
> UMKIM v0.2 is an experimental reference stack. It is not a supported or
> production-certified platform.

## Product model

UMKIM is a **hub-centric management plane for independent K3s clusters across
sites**. Outbound-only reachability via the hub WireGuard overlay is one
supported pattern, not the only topology. Sites may use hub gateway,
port-forward, reverse proxy, or private-only exposure.

| Layer | Responsibility |
| --- | --- |
| Hub | K3s, OCM, Harbor, console, WireGuard, and optional edge services |
| Site | A `FleetSite` logical location with `ingress[]` and optional `hubGateway` |
| Cluster | An independent K3s data plane represented by `FleetClusterJoin` and OCM `ManagedCluster` |
| App | A `FleetApp` Helm release on a cluster the operator selects |

The hub is infrastructure, not a workload `ManagedCluster`. Each workload
cluster runs its own K3s server and is managed by OCM after it joins.

## Network and control planes

The management LAN carries cluster-local Kubernetes traffic. A WireGuard
overlay provides hub-to-cluster **management** reachability. Workload
execution does not depend on the hub; the overlay is for enrollment, OCM,
Helm install jobs, and optional hub-gateway forwarding.

Do not expose management APIs, registry endpoints, or Kubernetes NodePorts to
the public network by default; see [../SECURITY.md](../SECURITY.md).

OCM registers workload clusters and delivers platform manifests through
`ManifestWork`. Application charts are installed with `FleetApp` onto a
named Ready cluster. There is no placement engine and no DCM catalog.

## Site ingress

Each `FleetSite` declares `spec.edge.ingress[]` entries (see
[fleet-edge.md](components/fleet-edge.md)). Clusters select an entry with
`FleetClusterJoin.spec.edge.ingressId`. Optional `hubGateway` replaces the
former site-wide `hub_gateway` exposure mode.

| Ingress `method` | Meaning |
| --- | --- |
| `port_forward` | WAN port-forwards to spoke Traefik NodePorts; one cluster per entry |
| `reverse_proxy` | External reverse proxy terminates TLS; many clusters may share an entry |

Sites with no ingress rows and no `hubGateway` have no fleet-managed WAN path.
Inventory examples live under [../examples/site-profiles/](../examples/site-profiles/).

## Cluster lifecycle

1. An operator creates a registry `FleetSite` with ingress entries (and optional `hubGateway`).
2. The operator creates a `FleetClusterJoin` for a non-management site.
3. The controller issues a bootstrap bundle containing enrollment material.
4. The operator runs the bundle on the cluster control node.
5. After local K3s validation, the hub provisions OCM registration and platform
   manifests (OLM, cert-manager, spoke Traefik ingress).
6. The cluster becomes Ready and can receive Helm charts via `FleetApp`.
   Optional KubeVirt is a separate spoke platform operation (not a FleetApp);
   see [components/fleet-kubevirt.md](components/fleet-kubevirt.md).

The public lifecycle phases are `Pending`, `ScriptIssued`, `Enrolled`,
`Validated`, `Provisioning`, and `Ready`. **`Failed`** is reported on
`FleetClusterJoin` with a status message; operators retry provision or inspect
hub Jobs after partial success (see [components/fleet-site.md](components/fleet-site.md)).
Cluster deletion defaults to **detach**:
it removes hub-managed resources while preserving the cluster and workloads
unless an operator explicitly requests Kubernetes cleanup
(`cleanupKubernetes=true` on delete). On a YAIDAP site the delete can also
destroy the cluster's guest VMs (`cleanupVms=true`) and drop their inventory
declarations (`cleanupVmDeclarations=true`).

## Hub independence

- Spokes are independent K3s control planes; the hub does not run their
  workloads.
- **Hub down:** console and hub-routed (`route.mode: hub`) URLs stop; pods and
  Helm releases on spokes keep running. Apps with `route.mode: site` or
  `none` can remain reachable when site ingress is configured and healthy.
- **Detach:** default cluster delete keeps K3s and user workloads on the spoke.

## Helm apps

After a cluster is Ready, install an OCI chart with `FleetApp`:

- `spec.clusterName` is required. The hub is not a valid target.
- `spec.chart` is an `oci://` URL or a short name under Harbor `charts`.
- `spec.route.mode` is `none`, `hub`, or `site`.
- Charts may be published to the hub library (`FleetChart`) before install.

Match `route.mode` to site edge: `hub` with `hubGateway`; `site` with
`port_forward` or `reverse_proxy`; `none` for chart-owned or internal ingress.

See [components/fleet-apps.md](components/fleet-apps.md).

## Edge routing

`FleetAppRoute` represents an application route created for a `FleetApp` when
the route mode is not `none`. The edge controller reconciles hub Traefik
configuration and, when required, spoke ingress manifests.

| Routing mode | Intended use | Hub required for public URL? |
| --- | --- | --- |
| `hub` | Hub gateway forwards to the selected workload cluster | Yes |
| `site` | Site ingress (port-forward, reverse proxy, etc.) exposes the app | No |
| `none` | No fleet-managed public route | No |

Exposure and TLS settings must be compatible with the selected network path.
See [components/fleet-edge.md](components/fleet-edge.md).

## Optional services

- **Harbor** hosts fleet-built images and OCI charts.
- **KubeVirt** is an optional spoke platform add-on (Multus + CDI + KubeVirt),
  installed after Ready via console/CLI/API — not a FleetApp.
- **Provisioner connector** (optional, implemented for YAIDAP): hub owns native
  YAML inventory on the hub; YAIDAP executes via connector jobs — see
  [components/yaidap-integration.md](components/yaidap-integration.md) and
  [components/fleet-site.md](components/fleet-site.md).
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
| Helm apps | [components/fleet-apps.md](components/fleet-apps.md) |
| KubeVirt platform | [components/fleet-kubevirt.md](components/fleet-kubevirt.md) |
| Provisioner connector / YAIDAP | [components/yaidap-integration.md](components/yaidap-integration.md) |
| Edge routes | [components/fleet-edge.md](components/fleet-edge.md) |
| Site exposure examples | [../examples/site-profiles/](../examples/site-profiles/) |
| Versions | [../VERSIONS.md](../VERSIONS.md) |
| Security | [../SECURITY.md](../SECURITY.md) |
