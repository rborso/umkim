#!/usr/bin/env bash
# Hub restore — WireGuard, K3s etcd, optional ACME and Harbor.
# DESTRUCTIVE. Read docs/backup-restore.md and docs/plans/hub-backup-restore.md first.
#
# Usage:
#   sudo ./scripts/restore-hub.sh --dry-run /var/backups/umkim/20260101T120000Z
#   sudo ./scripts/restore-hub.sh --yes /var/backups/umkim/20260101T120000Z
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/hub-backup-common.sh
source "${ROOT}/scripts/lib/hub-backup-common.sh"

DRY_RUN=false
CONFIRM=false
SKIP_ETCD=false
SKIP_WIREGUARD=false
SKIP_ACME=false
SKIP_HARBOR=false
SRC=""

usage() {
  cat <<EOF
Usage: $0 [options] <backup-dir>

Options:
  --dry-run           print planned steps only
  --yes               skip interactive confirmation
  --skip-etcd         restore host components only (wireguard, acme, harbor)
  --skip-wireguard    skip WireGuard restore
  --skip-acme         skip ACME hostPath restore
  --skip-harbor       skip Harbor PVC restore
  -h, --help          show this help

Environment:
  RESTORE_HUB_CONFIRM=yes   same as --yes
  HUB_ACME_HOST_PATH        ACME hostPath (default: /var/lib/umkim/traefik-acme)
  HARBOR_NAMESPACE          Harbor namespace (default: harbor)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true ;;
    --yes) CONFIRM=true ;;
    --skip-etcd) SKIP_ETCD=true ;;
    --skip-wireguard) SKIP_WIREGUARD=true ;;
    --skip-acme) SKIP_ACME=true ;;
    --skip-harbor) SKIP_HARBOR=true ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      [[ -z "${SRC}" ]] || { echo "unexpected argument: $1" >&2; exit 1; }
      SRC="$1"
      ;;
  esac
  shift
done

[[ "${RESTORE_HUB_CONFIRM:-}" == "yes" ]] && CONFIRM=true
[[ -n "${SRC}" && -d "${SRC}" ]] || { usage >&2; exit 1; }

hub_require_root

snap=""
datastore_type="none"
if [[ "${SKIP_ETCD}" != "true" ]]; then
  snap="$(hub_find_k3s_datastore_backup "${SRC}")"
  datastore_type="$(hub_k3s_datastore_backup_type "${SRC}")"
  [[ -n "${snap}" ]] || hub_die "no k3s datastore backup in ${SRC} (use --skip-etcd for host-only restore)"
fi

hub_log "source=${SRC}"
if [[ -f "${SRC}/manifest.json" ]]; then
  hub_log "manifest:"
  sed 's/^/  /' "${SRC}/manifest.json"
fi

plan() {
  hub_log "restore plan:"
  [[ "${SKIP_WIREGUARD}" != "true" && -f "${SRC}/wireguard/wg.tgz" ]] && hub_log "  - WireGuard -> /etc/wireguard"
  [[ "${SKIP_ETCD}" != "true" && -n "${snap}" ]] && hub_log "  - k3s ${datastore_type} <- ${snap}"
  [[ "${SKIP_ACME}" != "true" && -f "${SRC}/host/traefik-acme.tgz" ]] && hub_log "  - ACME -> ${HUB_ACME_HOST_PATH}"
  [[ "${SKIP_HARBOR}" != "true" && -f "${SRC}/harbor/harbor-pvcs.json" ]] && hub_log "  - Harbor PVCs (${HARBOR_NAMESPACE})"
}

plan

if [[ "${DRY_RUN}" == "true" ]]; then
  hub_log "dry-run only — no changes made"
  exit 0
fi

if [[ "${CONFIRM}" != "true" ]]; then
  echo
  echo "WARNING: This will stop k3s and replace hub state. Spokes may flap until the hub is healthy."
  read -r -p "Type 'yes' to continue: " answer
  [[ "${answer}" == "yes" ]] || hub_die "aborted"
fi

if [[ "${SKIP_WIREGUARD}" != "true" && -f "${SRC}/wireguard/wg.tgz" ]]; then
  hub_restore_wireguard "${SRC}"
fi

if [[ "${SKIP_ETCD}" != "true" ]]; then
  hub_restore_k3s_datastore "${SRC}"
fi

if [[ "${SKIP_ACME}" != "true" ]]; then
  hub_restore_acme "${SRC}" || true
  hub_restart_hub_gateway || true
fi

if [[ "${SKIP_HARBOR}" != "true" ]]; then
  hub_restore_harbor "${SRC}" || true
fi

hub_log "restore complete"
hub_log "next: re-apply operator WG peers: ansible-playbook wireguard-clients.yml -l hub (see docs/wireguard-operator-clients.md)"
hub_log "next: ./scripts/verify-hub-install.sh (if available) and check ManagedClusters / Harbor"
hub_log "re-run OCM accept/CSR if ManagedClusters show unknown after etcd age mismatch"
