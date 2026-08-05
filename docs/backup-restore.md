# Backup and restore

> Status: v0.1.0 is an experimental proof of concept. Backup and restore are
> incomplete and must not be treated as a production recovery solution.

UMKIM includes hub backup helpers and an experimental catalog-instance
backup/restore path. Test a recovery in an isolated environment before relying
on either path.

## Hub backup

`scripts/backup-hub.sh` writes a timestamped backup containing:

- a DCM Postgres dump, when the hub API is available;
- a K3s etcd snapshot, when K3s is installed; and
- an archive of the hub WireGuard configuration.

Run it only from a trusted administrative environment. Encrypt backups at rest,
store a copy off-host, and protect the backup destination at least as carefully
as the Ansible Vault and WireGuard private keys. Harbor data and the inventory
are separate recovery concerns.

## Restore

`scripts/restore-hub.sh` is destructive. It supports a Postgres-only restore
and prints the manual K3s etcd recovery steps for a full restore. Schedule a
maintenance window, review the script for the installed version, and validate
the recovered control plane and managed clusters before allowing changes.

The restore path can require cluster re-enrollment or redeployment of
ManifestWorks. It does not provide a guaranteed zero-data-loss or
zero-downtime recovery objective.

## Catalog instance backup and restore

The fleet-site API can create asynchronous instance backup and restore
operations backed by Velero and catalog metadata. This feature is experimental:

- confirm the target cluster and storage compatibility first;
- keep the original workload until the restored workload is verified;
- treat cross-cluster recovery as a migration exercise, not an automatic
  failover mechanism; and
- use the authenticated console or approved API client. Do not expose
  controller ports or put credentials in shell history.

The API contract is in
[controllers/fleet-site/openapi.yaml](../controllers/fleet-site/openapi.yaml).

## Verify

Use the read-only and synthetic checks in
[fleet-verify.md](fleet-verify.md) after a tested recovery. For installation
and disaster-preparation prerequisites, see [install.md](install.md) and
[secrets.md](secrets.md).
