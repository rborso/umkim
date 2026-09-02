#!/usr/bin/env bash
# Hub backup — K3s etcd, WireGuard, optional ACME and Harbor.
# Run on the hub as root.
#
# Usage:
#   sudo ./scripts/backup-hub.sh
#   sudo BACKUP_HARBOR=true ./scripts/backup-hub.sh --full
#   sudo ./scripts/backup-hub.sh --without-acme
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/hub-backup-common.sh
source "${ROOT}/scripts/lib/hub-backup-common.sh"

BACKUP_ACME="${BACKUP_ACME:-true}"
BACKUP_HARBOR="${BACKUP_HARBOR:-false}"

usage() {
  cat <<EOF
Usage: $0 [options]

Options:
  --full            etcd + wireguard + ACME + Harbor (same as BACKUP_HARBOR=true)
  --with-harbor     include Harbor PVC archives and DB dump
  --without-harbor  skip Harbor (default)
  --with-acme       include Traefik ACME hostPath (default)
  --without-acme    skip ACME hostPath
  -h, --help        show this help

Environment:
  BACKUP_ROOT / HUB_BACKUP_ROOT   backup parent dir (default: /var/backups/umkim)
  HUB_ACME_HOST_PATH              ACME hostPath (default: /var/lib/umkim/traefik-acme)
  HARBOR_NAMESPACE                Harbor namespace (default: harbor)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --full)
      BACKUP_ACME=true
      BACKUP_HARBOR=true
      ;;
    --with-harbor) BACKUP_HARBOR=true ;;
    --without-harbor) BACKUP_HARBOR=false ;;
    --with-acme) BACKUP_ACME=true ;;
    --without-acme) BACKUP_ACME=false ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

hub_require_root

STAMP="$(hub_backup_stamp)"
DEST="${HUB_BACKUP_ROOT}/${STAMP}"
mkdir -p "${DEST}"

hub_log "writing to ${DEST}"
components=()

if hub_backup_k3s_datastore "${DEST}" "${STAMP}"; then
  case "$(hub_k3s_datastore_type)" in
    sqlite) components+=("k3s-sqlite") ;;
    etcd) components+=("etcd") ;;
  esac
fi

if hub_backup_wireguard "${DEST}"; then
  components+=("wireguard")
fi

if [[ "${BACKUP_ACME}" == "true" ]]; then
  if hub_backup_acme "${DEST}"; then
    components+=("acme")
  fi
fi

if [[ "${BACKUP_HARBOR}" == "true" ]]; then
  if hub_backup_harbor "${DEST}"; then
    components+=("harbor")
  fi
fi

hub_write_manifest "${DEST}" "${components[@]}"
hub_backup_inventory_hint
hub_log "done ${DEST} (components: ${components[*]:-none})"
