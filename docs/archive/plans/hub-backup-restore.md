> **Historical** — superseded by [backup-restore.md](../../backup-restore.md).
> Do not use for installation or operations.

# Hub backup and restore plan (single-node management cluster)

> Status: implemented in `scripts/backup-hub.sh`, `scripts/restore-hub.sh`,
> and `scripts/lib/hub-backup-common.sh`. Operator inventory/vault remain
> off-host.

## Goal

After hub disaster (disk loss, reinstall, or corrupt etcd), restore the
management cluster to the **last backed-up working state**:

- OCM registration and Fleet CRs (FleetSite, FCJ, FleetApp, …)
- WireGuard overlay (same hub keys and peer layout)
- Hub TLS (ACME `acme.json` on hostPath)
- Harbor images/charts (optional PVC + DB backup)
- Spokes reconnect when overlay addressing and etcd state are unchanged

Spoke clusters are **not** included; they keep running independently when detached.

## Layers

| Layer | Location | Default backup |
|---|---|---|
| Operator config | `inventory/`, vault password (off-host) | Manual |
| WireGuard | `/etc/wireguard` | Always |
| K3s etcd | `k3s etcd-snapshot` | Always |
| Hub ACME | `/var/lib/umkim/traefik-acme` | On (`BACKUP_ACME=true`) |
| Harbor data | PVC host paths + Postgres dump | Off (`BACKUP_HARBOR=false`) |

## Backup bundle layout

```text
/var/backups/umkim/<UTC-stamp>/
├── manifest.json
├── etcd/*.db
├── wireguard/wg.tgz
├── host/traefik-acme.tgz          # when ACME enabled
└── harbor/
    ├── harbor-pvcs.json           # PVC → host path map
    ├── registry.sql               # pg_dump (best effort)
    └── pvc/<claim-name>.tgz       # per-PVC tar
```

## Backup order

1. K3s etcd snapshot (API object state)
2. WireGuard host config
3. Traefik ACME hostPath (if present)
4. Harbor (optional): scale down → pg_dump → tar PVC dirs → scale up

## Restore order

1. Confirm maintenance window; spokes may flap until hub is healthy
2. Restore WireGuard (`wg0` keys/peers)
3. Stop k3s → `cluster-reset --cluster-reset-restore-path` → start k3s
4. Restore ACME hostPath; restart hub gateway if needed
5. Restore Harbor PVC dirs + optional SQL (scale Harbor down first)
6. Run `verify-hub-install.sh` and spot-check ManagedClusters / Harbor

## Prerequisites for seamless spoke reconnect

- Same hub **WAN** and **overlay** plan as backup time (inventory)
- Restored **WireGuard hub keys** (not regenerated)
- Etcd snapshot from the same era as Harbor PVC backup (take together)

## Out of scope (v0.2)

- Automated off-host upload (rsync/restic documented, not scripted)
- Spoke etcd / workload backup
- Zero-downtime hub failover (single-node product floor)

## Verification

- Dry-run: `sudo ./scripts/restore-hub.sh --dry-run <backup-dir>`
- Lab DR test: restore to an isolated VM quarterly; run `verify-hub-install.sh`
