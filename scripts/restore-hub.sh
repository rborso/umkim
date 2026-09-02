#!/usr/bin/env bash
# Hub restore helpers — DESTRUCTIVE. Read docs/backup-restore.md first.
# Usage: sudo ./scripts/restore-hub.sh /var/backups/umkim/20260101T120000Z
set -euo pipefail

SRC="${1:-}"
[[ -z "$SRC" || ! -d "$SRC" ]] && { echo "usage: $0 <backup-dir>" >&2; exit 1; }

echo "[restore-hub] source=$SRC"
echo "[restore-hub] WARNING: confirm hub maintenance window; spokes may need OCM re-accept"

snap="$(find "${SRC}/etcd" -name '*.db' 2>/dev/null | head -1)"
[[ -n "$snap" ]] || { echo "no etcd snapshot in ${SRC}/etcd" >&2; exit 1; }
echo "[restore-hub] etcd restore requires k3s server stop — manual steps:"
echo "  systemctl stop k3s"
echo "  k3s server --cluster-reset --cluster-reset-restore-path=$snap"
echo "  systemctl start k3s"
echo "[restore-hub] re-run OCM accept/CSR if ManagedClusters show unknown"
