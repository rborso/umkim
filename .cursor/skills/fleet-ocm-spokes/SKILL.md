---
name: fleet-ocm-spokes
description: >-
  Operates the fleet core: WireGuard, K3s, OCM initialization, cluster joins, CSR
  approval, and managed-cluster registration. Use for wireguard.yml, ocm.yml,
  spokes.yml, clusteradm, ManagedCluster, or klusterlet work.
---

# Fleet core — OCM & spokes (S2)

## Scope

Use this skill for the fleet connectivity and OCM join path. The relevant
playbooks are `host-prepare.yml`, `wireguard.yml`, `ocm.yml`, `spokes.yml`, and
`ocm-cluster-labels.yml`.

## Join guidance

- Read the active inventory before choosing a limit, tag, join host, or
  kubeconfig.
- Treat `roles/ocm/tasks/init_hub.yml` and
  `roles/ocm/tasks/approve_cluster_csr.yml` as the implementation source for
  initialization and CSR handling.
- Confirm the managed-cluster and agent state from the authorized management
  context after a join or repair. After the cluster is Ready, Helm installs
  use FleetApp on a named cluster; see
  [fleet-apps.md](../../../docs/components/fleet-apps.md).
- Cluster delete defaults to **detach** (workloads remain). Full K3s reset is
  opt-in. Spokes are independent K3s; hub loss does not stop workloads.
- Do not reuse host names, addresses, or kubeconfig locations from examples;
  those values are deployment inputs.

For changes to the join contract, update the component or design documentation
identified by [AGENTS.md](../../../AGENTS.md).

## References

- [Design guide](../../../docs/design.md)
- [Multi-tenancy guide](../../../docs/multi-tenancy.md)
- [Installation guide](../../../docs/install.md)
- [FleetApp Helm installs](../../../docs/components/fleet-apps.md)
