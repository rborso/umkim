# Backup and restore

UMKIM ships hub backup and restore helpers for a **single-node management
cluster**. Test recovery in an isolated lab before relying on the path in
production.

See also [archive/plans/hub-backup-restore.md](archive/plans/hub-backup-restore.md)
for the full disaster-recovery design.

## What is backed up

`scripts/backup-hub.sh` writes a timestamped directory under
`/var/backups/umkim/<UTC-stamp>/`:

| Component | Default | Contents |
|---|---|---|
| K3s cluster state | always | `k3s-db/state.db` (SQLite) or `etcd/*.db` (embedded etcd) |
| WireGuard | always | `wireguard/wg.tgz` from `/etc/wireguard` |
| Hub ACME | on (`BACKUP_ACME=true`) | `host/traefik-acme.tgz` from `/var/lib/umkim/traefik-acme` |
| Harbor | off (`BACKUP_HARBOR=false`) | `harbor/pvc/*.tgz`, `harbor/registry.sql`, `harbor/harbor-pvcs.json` |
| Platform PVCs | on (`BACKUP_PLATFORM=true`) | `platform/pvc/*.tgz`, `platform/platform-pvcs.json` (LLDAP, Authelia) |

**Not included:** Ansible inventory, vault password, spoke clusters, or Velero
workload backups. Copy inventory and secrets off-host separately.

### Examples

```bash
# Standard hub backup (K3s datastore + WireGuard + ACME)
sudo ./scripts/backup-hub.sh

# Full backup including Harbor (brief Harbor downtime while scaled down)
sudo ./scripts/backup-hub.sh --full

# Etcd + WireGuard only
sudo BACKUP_ACME=false ./scripts/backup-hub.sh
```

Off-host copy (encrypt in transit and at rest):

```bash
sudo rsync -a /var/backups/umkim/<stamp>/ backup-host:umkim/
```

## Hub restore

`scripts/restore-hub.sh` is **destructive**. It stops K3s, restores the K3s
datastore (SQLite `state.db` or embedded etcd), and optionally restores
WireGuard, ACME, and Harbor PVC host paths.

```bash
# Review steps without changes
sudo ./scripts/restore-hub.sh --dry-run /var/backups/umkim/<stamp>

# Execute (prompts for confirmation)
sudo ./scripts/restore-hub.sh /var/backups/umkim/<stamp>

# Non-interactive (automation / runbooks)
sudo ./scripts/restore-hub.sh --yes /var/backups/umkim/<stamp>
```

Restore order: WireGuard → K3s datastore → ACME (+ gateway restart) → Harbor.

For seamless spoke reconnect after hub loss:

- Restore the **same** WireGuard hub keys (do not regenerate).
- Keep the **same** WAN/overlay plan as when the backup was taken.
- Take Harbor PVC backup in the **same run** as the K3s datastore backup when
  Harbor is enabled.

After restore, run verification (`verify-hub-install.sh`, `verify-fleet.sh`) and
check `ManagedClusters`. Re-accept CSRs if clusters show `Unknown`.

### K3s SQLite hubs (default single-node)

Many UMKIM hubs use K3s with an embedded **SQLite** datastore (`state.db`), not
embedded etcd. `backup-hub.sh` detects this automatically and writes
`k3s-db/state.db` + `k3s-db/token` instead of an etcd snapshot.

After restore on a host that ran a minimal fresh `k3s` install, also re-apply
host/Ansible state not in the backup bundle:

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
ansible-playbook $INV hub-firewall.yml -l hub
ansible-playbook $INV harbor.yml -l hub --tags harbor_registries
ansible-playbook $INV wireguard-clients.yml   # operator laptop/tablet peers (not in wg0.conf backup)
```

Hub `wg0` PostUp runs `umkim-restore-hub-wg-peers.sh --operators-only` to
re-apply operator peers from inventory. Cluster peers are restored by
`restore-hub-wg-peers.sh --clusters-only` (also available as a manual script).

See [archive/runbooks/hub-dr-test-20260902.md](archive/runbooks/hub-dr-test-20260902.md)
(`docs/archive/runbooks/`) for a full same-host DR test log.

## Workload recovery

For Helm workloads installed with FleetApp, prefer a chart-aware recovery plan:
restore hub/cluster state, then re-install or reconcile the `FleetApp`.

Velero instance backups (spoke workloads) are configured separately — see
`inventory/group_vars/all/fleet_backup.yml` (copy from
`examples/inventory/hub-only/group_vars/all/fleet_backup.yml` if bootstrap did
not create it).

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

## Scheduling (optional)

Example daily backup cron on the hub (adjust retention off-host):

```cron
15 2 * * * root /path/to/umkim/scripts/backup-hub.sh --full >> /var/log/umkim-backup.log 2>&1
```
