#!/usr/bin/env bash
# Hub restore helpers — DESTRUCTIVE. Read docs/backup-restore.md first.
# Usage: sudo ./scripts/restore-hub.sh /var/backups/umkim/20260101T120000Z [--postgres-only]
set -euo pipefail

SRC="${1:-}"
MODE="${2:-}"
[[ -z "$SRC" || ! -d "$SRC" ]] && { echo "usage: $0 <backup-dir> [--postgres-only]" >&2; exit 1; }

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
DCM_NS="${DCM_NS:-dcm}"
DCM_DB="${DCM_DB:-control-plane}"
DCM_USER="${DCM_USER:-admin}"

echo "[restore-hub] source=$SRC mode=${MODE:-full}"
echo "[restore-hub] WARNING: confirm hub maintenance window; spokes may need OCM re-accept"

restore_postgres() {
  local dump="${SRC}/postgres/${DCM_DB}.dump"
  [[ -f "$dump" ]] || { echo "missing $dump" >&2; exit 1; }
  echo "[restore-hub] scaling postgres down"
  kubectl -n "$DCM_NS" scale deploy/postgres --replicas=0
  kubectl -n "$DCM_NS" wait --for=delete pod -l app=dcm-postgres --timeout=120s 2>/dev/null || true
  echo "[restore-hub] restoring postgres (drop/create handled by pg_restore --clean if DB empty)"
  kubectl -n "$DCM_NS" scale deploy/postgres --replicas=1
  kubectl -n "$DCM_NS" wait --for=condition=Ready pod -l app=dcm-postgres --timeout=300s
  cat "$dump" | kubectl -n "$DCM_NS" exec -i deploy/postgres -- \
    pg_restore -U "$DCM_USER" -d "$DCM_DB" --clean --if-exists || \
  cat "$dump" | kubectl -n "$DCM_NS" exec -i deploy/postgres -- \
    pg_restore -U "$DCM_USER" -d postgres --create --clean
  echo "[restore-hub] postgres restore submitted"
}

restore_etcd() {
  local snap
  snap="$(find "${SRC}/etcd" -name '*.db' 2>/dev/null | head -1)"
  [[ -n "$snap" ]] || { echo "no etcd snapshot in ${SRC}/etcd" >&2; exit 1; }
  echo "[restore-hub] etcd restore requires k3s server stop — manual steps:"
  echo "  systemctl stop k3s"
  echo "  k3s server --cluster-reset --cluster-reset-restore-path=$snap"
  echo "  systemctl start k3s"
}

if [[ "$MODE" == "--postgres-only" ]]; then
  restore_postgres
  exit 0
fi

restore_postgres
restore_etcd
echo "[restore-hub] re-run spokes.yml accept/CSR if ManagedClusters show unknown"
echo "[restore-hub] ansible-playbook fleet-deploy.yml --tags phase4,phase5 as needed"
