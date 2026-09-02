# Backup and restore

> Status: v0.2 is an experimental proof of concept. Backup and restore are
> incomplete and must not be treated as a production recovery solution.

UMKIM includes hub backup helpers. Test a recovery in an isolated
environment before relying on the path.

## Hub backup

`scripts/backup-hub.sh` writes a timestamped backup containing:

- a K3s etcd snapshot, when K3s is installed; and
- an archive of the hub WireGuard configuration.

Run the helper only from a trusted administrative environment. Encrypt backups
at rest, store a copy off-host, and protect the backup destination at least as
carefully as the Ansible Vault and WireGuard private keys. Harbor data and the
inventory are separate recovery concerns.

## Restore

`scripts/restore-hub.sh` is destructive. A full restore prints the
manual K3s etcd recovery steps. Schedule a maintenance window, review the
script for the installed version, and validate the recovered control plane and
managed clusters before allowing changes.

The restore path can require cluster re-enrollment or redeployment of
ManifestWorks. It does not provide a guaranteed zero-data-loss or
zero-downtime recovery objective.

## Workload recovery

For Helm workloads installed with FleetApp, prefer a chart-aware recovery
plan: restore hub/cluster state, then re-install the `FleetApp`.

## Hub detach and spoke independence

Cluster delete **defaults to detach**: hub-managed resources are removed while
K3s and workloads on the spoke are preserved. Full K3s reset is opt-in
(`cleanupKubernetes=true`). See
[playbooks/flows/cluster-delete.yml](../playbooks/flows/cluster-delete.yml).

Hub backup does **not** include spoke etcd or workload data. Detached spokes
keep running but are no longer hub-managed until re-enrolled.

After hub-only disaster recovery, re-enroll or recreate FleetClusterJoin
records. FleetApps on surviving detached spokes are not automatically
re-imported. Hub-routed apps (`route.mode: hub`) are unreachable while the
hub is down; `site` and `none` apps may remain reachable on their own paths.

## Verify

Use the read-only and synthetic checks in
[fleet-verify.md](fleet-verify.md) after a tested recovery. For installation
and disaster-preparation prerequisites, see [install.md](install.md) and
[secrets.md](secrets.md).
