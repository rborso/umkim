# UMKIM

UMKIM is a hub-centric fleet manager for **independent K3s clusters across
sites**. One Rocky Linux 10 hub provides the operator entrypoint (console,
Harbor, OCM, WireGuard overlay, optional hub gateway). Each site runs its own
K3s data plane with a **per-site ingress model** — hub gateway, port-forward,
reverse proxy, or private-only.

> [!WARNING]
> v0.2 is a proof of concept, not a production platform. Installation and
> teardown can be destructive. Backup/restore, upgrades, HA/failover, and
> tenancy guarantees require independent validation before any real use.

## Start here

1. Prepare a Rocky Linux 10 hub host and clone this repository.
2. Create a fresh hub inventory from the public template:

   ```bash
   ./scripts/bootstrap-hub-inventory.sh
   ```

3. Set inventory values and encrypted Vault secrets, then install the hub:

   ```bash
   ./scripts/install-hub.sh
   ```

4. Verify the hub, create a non-management site and a FleetClusterJoin cluster,
   wait until it is **Ready**, then install a Helm chart with **Apps** (you
   must select the cluster).

The complete safe path is [docs/install.md](docs/install.md). Do not use a
pre-filled `inventory/` as a public template.

## How it works

UMKIM runs a **management hub** and any number of **independent workload
clusters**. The hub is its own K3s cluster; it is not a parent K3s control
plane for the others. Each `FleetSite` can use a different network and
exposure model.

**Hub** typically runs WireGuard (overlay endpoint), the fleet console (auth
proxy and UI), Harbor (images and charts), OCM (hub-side multi-cluster
control), and fleet-edge / ingress-auth services.

**Adding a cluster.** An operator creates a non-management `FleetSite`, then a
`FleetClusterJoin`. The hub issues a **bootstrap bundle** that includes
per-node WireGuard client config and install steps. On the first control
node, `bootstrap.sh` brings up WireGuard and stands up local K3s. After the
cluster reports **Validated**, **Provision** on the hub registers it with OCM
and applies hub-managed platform pieces (for example ManifestWorks, OLM, and
cert-manager prerequisites). When the cluster is **Ready**, install Helm
charts with `FleetApp`. Optional KubeVirt is a spoke platform add-on where
KVM is available.

**Workloads.** A `FleetApp` installs an OCI chart onto the **cluster you
name**. There is no placement engine and no DCM catalog. The hub itself is
never a workload target. Charts are published into Harbor (whoami is the
smoke-test chart).

**Site ingress** (`FleetSite.spec.edge` — each site may differ):

| Pattern | Behavior |
| --- | --- |
| `hubGateway` | Public HTTP/S on the hub; hub forwards to spokes over WireGuard |
| `ingress[]` `port_forward` | WAN port-forwards to spoke Traefik NodePorts (one cluster per ingress id) |
| `ingress[]` `reverse_proxy` | External reverse proxy reaches cluster ingress (TLS off-cluster) |
| Empty ingress (no hub gateway) | No fleet-managed WAN exposure at the site |

Clusters bind with `FleetClusterJoin.spec.edge.ingressId`. See
[examples/site-profiles/](examples/site-profiles/) for inventory snippets.

**App routing** (`FleetApp` `route.mode` → `FleetAppRoute`):

| Mode | Behavior | Hub required for public URL? |
| --- | --- | --- |
| `hub` | Hub gateway publishes `hubHost` and forwards to the cluster | Yes |
| `site` | Spoke ingress exposes `siteHost` (port-forward, reverse proxy, etc.) | No |
| `none` | No fleet-managed route; chart or workload owns ingress | No |

Optional Authelia forward-auth is per-route (`route.requireAuth`) and is
separate from console login.

**Hub independence.** Spokes are real independent K3s clusters. If the hub is
down, the console and **hub-routed** URLs stop; workloads on spokes keep
running. Apps with `route.mode: site` or `none` (and working site ingress)
can remain reachable. Cluster delete **defaults to detach** — it removes
hub-managed resources while keeping K3s and workloads unless you opt into
full Kubernetes cleanup.

See [docs/design.md](docs/design.md) for the architecture and
[docs/fleet-verify.md](docs/fleet-verify.md) for non-destructive verification.

## Documentation

| Topic | Document |
| --- | --- |
| Install a hub and first cluster | [docs/install.md](docs/install.md) |
| Architecture and lifecycle | [docs/design.md](docs/design.md) |
| Helm apps | [docs/components/fleet-apps.md](docs/components/fleet-apps.md) |
| Site exposure profiles | [examples/site-profiles/](examples/site-profiles/) |
| Versions and release pins | [VERSIONS.md](VERSIONS.md) |
| Security and secrets | [SECURITY.md](SECURITY.md) · [docs/secrets.md](docs/secrets.md) |
| Components | [docs/components/](docs/components/) |
| Release limits | [RELEASE_NOTES.md](RELEASE_NOTES.md) |

## Contributing and security

Read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting changes. Report
security issues through the repository's private GitHub Security Advisory;
see [SECURITY.md](SECURITY.md).

Apache-2.0: [LICENSE](LICENSE) · third-party attribution: [NOTICE](NOTICE) ·
conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
