# UMKIM

UMKIM is an experimental reference implementation for operating independent
K3s clusters from an Open Cluster Management (OCM) hub. The hub combines DCM,
Harbor, FleetClusterJoin provisioning, and optional fleet-edge routing.

> [!WARNING]
> v0.1.0 is a proof of concept, not a production platform. Installation and
> teardown can be destructive. Backup/restore, upgrades, HA/failover, and
> tenancy guarantees require independent validation before any real use.

## Start here

1. Prepare a Rocky Linux 9 hub host and clone the required sibling repositories.
2. Create a fresh hub inventory from the public template:

   ```bash
   ./scripts/bootstrap-hub-inventory.sh
   ```

3. Set inventory values and encrypted Vault secrets, then install the hub:

   ```bash
   FLEET_PROFILE=production ./scripts/install-hub.sh
   ```

4. Verify the hub, create a non-management site and a FleetClusterJoin cluster,
   install its providers, then deploy a catalog item.

The complete safe path is [docs/install.md](docs/install.md). Do not use a
pre-filled `inventory/` as a public template.

## How it works

UMKIM runs a **management hub** and any number of **independent workload
clusters**. The hub is its own K3s cluster; it is not a parent K3s control
plane for the others.

**Hub** typically runs WireGuard (overlay endpoint), the fleet console (auth
proxy and UI), Harbor (images and charts), OCM (hub-side multi-cluster
control), DCM (catalog and placement), and optional fleet-edge /
ingress-auth services.

**Adding a cluster.** An operator creates a non-management `FleetSite`, then a
`FleetClusterJoin`. The hub issues a **bootstrap bundle** that includes
per-node WireGuard client config and install steps. On the first control
node, `bootstrap.sh` brings up WireGuard and stands up local K3s. After the
cluster reports **Validated**, **Provision** on the hub registers it with OCM
and applies hub-managed platform pieces (for example ManifestWorks, OLM, and
cert-manager prerequisites). When the cluster is **Ready**, the operator can
install workload **providers** (`k8s`, `Helm`, and optionally `KubeVirt` where
KVM is available).

**Workloads.** DCM schedules catalog item instances onto eligible managed
clusters using labels, policy, and capacity signals. Operators can also
**target a specific cluster** (placement / label selector). The hub itself is
never a placement target. Optional
[umkim-catalog](https://github.com/rborso/umkim-catalog) supplies wrapper
charts and catalog metadata for common FOSS applications (Gitea, Nextcloud,
and similar).

**Routing** (`FleetAppRoute` / catalog exposure):

| Mode | Behavior |
| --- | --- |
| `hub` | Public or edge entry is on the **hub gateway**; traffic is forwarded to the placed workload over the management path. |
| `site` | The **site’s own ingress path** exposes the app (needs a configured direct-ingress / site edge setup). |
| `none` | No fleet-managed route; the workload or chart owns ingress — for example catalog **`chart_ingress`**, which installs Traefik Ingress on the spoke. |

Optional Authelia forward-auth is per-route (`routing.require_auth`) and is
separate from console login.

**Untested (and likely broken) in v0.1.0.** These paths exist in the tree but
have not been validated end-to-end; treat them as probably broken until proven
otherwise:

- Hub and catalog **backup/restore**
- **Organization multi-tenancy** and full **RBAC / isolation**
- **LiteLLM** hub proxy / LLM routing
- **KubeVirt** VMs (needs more docs; may work with KVM prep, but unproven here)

See [docs/design.md](docs/design.md) for the architecture and
[docs/fleet-verify.md](docs/fleet-verify.md) for non-destructive verification.

## Documentation

| Topic | Document |
| --- | --- |
| Install a hub and first cluster | [docs/install.md](docs/install.md) |
| Architecture and lifecycle | [docs/design.md](docs/design.md) |
| Versions and release pins | [VERSIONS.md](VERSIONS.md) |
| Security and secrets | [SECURITY.md](SECURITY.md) · [docs/secrets.md](docs/secrets.md) |
| Components | [docs/components/](docs/components/) |
| Release limits | [RELEASE_NOTES.md](RELEASE_NOTES.md) |

## Related repositories

The hub image build expects sibling checkouts of
[control-plane](https://github.com/rborso/control-plane),
[fleet-olm-subscription-sp](https://github.com/rborso/fleet-olm-subscription-sp),
[fleet-helm-release-sp](https://github.com/rborso/fleet-helm-release-sp),
[k8s-container-service-provider](https://github.com/rborso/k8s-container-service-provider),
and
[kubevirt-service-provider](https://github.com/rborso/kubevirt-service-provider).
[umkim-catalog](https://github.com/rborso/umkim-catalog) is optional application
catalog content, not a hub-install prerequisite.

## Contributing and security

Read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting changes. Report
security issues through the repository's private GitHub Security Advisory;
see [SECURITY.md](SECURITY.md).

Apache-2.0: [LICENSE](LICENSE) · third-party attribution: [NOTICE](NOTICE) ·
conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
