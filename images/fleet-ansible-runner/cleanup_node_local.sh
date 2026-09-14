#!/usr/bin/env bash
# UMKIM fleet node local cleanup — detach from fleet (default) or full K3s reset.
# Does not touch hub-side FleetClusterJoin, ManagedCluster, or WireGuard hub peers.
# Never removes wg0 or other non-fleet WireGuard interfaces.
#
# Typical use (on the bootstrap / cluster node):
#   sudo ./cleanup_node_local.sh --cluster my-cluster --mode detach
#   sudo ./cleanup_node_local.sh --cluster my-cluster --mode reset
#
# Legacy: --cleanup-wireguard implies detach mode (WG always removed in detach/reset).
set -euo pipefail

CLUSTER=""
VIP=""
MODE="detach"
REMOVE_ALL_BOOTSTRAP_DIRS=0
DRY_RUN=0
FLEET_OVERLAY_IFACE="wg-umkim"
FLEET_OVERLAY_CONF="/etc/wireguard/wg-umkim.conf"

log() { echo "[fleet-node-cleanup] $*"; }

usage() {
  cat <<'EOF'
Usage: cleanup_node_local.sh [options]

Detach from UMKIM fleet (default) or fully reset local K3s.

Options:
  --mode detach|reset    detach (default): remove OCM/DCM fleet glue, keep K3s + apps
                         reset: detach then uninstall K3s and wipe CNI/kubelet state
  --cluster NAME         Also remove /tmp/bootstrap-NAME and /tmp/bootstrap-NAME.tar.gz
  --vip ADDRESS          Remove HA control plane VIP (e.g. 192.168.1.80) from local NICs
  --all-bootstrap-dirs   Remove /tmp/bootstrap-* directories and archives
  --cleanup-wireguard    Alias for --mode detach (deprecated)
  --dry-run              Print actions without changing the system
  -h, --help             Show this help

detach mode always:
  - OCM klusterlet namespaces (when K3s API is reachable)
  - dcm-worker namespace (fleet service providers)
  - Bootstrap temp files
  - fleet overlay wg-umkim last (never wg0; hub SSH uses overlay until script exits)

reset mode additionally:
  - K3s server/agent uninstall and state directories
  - Calico/CNI leftovers

Does not remove SSH keys, Ansible packages, wg0, or hub/console cluster records.
EOF
}

require_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    return 0
  fi
  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    exec sudo -E bash "$0" "$@"
  fi
  log "Run as root or with passwordless sudo: sudo $0"; exit 1
}

run() {
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    log "[dry-run] $*"
  else
    log "$*"
    eval "$@"
  fi
}

run_or_ignore() {
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    log "[dry-run] $* (ignored on failure)"
  else
    log "$*"
    eval "$@" || true
  fi
}

run_timeout() {
  local secs="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    run_or_ignore "timeout ${secs} $*"
  else
    run_or_ignore "$*"
  fi
}

# Calico leaves bind mounts under /run/calico and /var/run/calico; rm -rf can hang until unmounted.
unmount_calico_paths() {
  local mp
  if command -v findmnt >/dev/null 2>&1; then
    while IFS= read -r mp; do
      [[ -n "${mp}" ]] && run_or_ignore "umount -l '${mp}'"
    done < <(findmnt -rn -o TARGET /run/calico /var/run/calico 2>/dev/null | sort -r)
  fi
  if command -v mount >/dev/null 2>&1; then
    while IFS= read -r mp; do
      [[ -n "${mp}" ]] && run_or_ignore "umount -l '${mp}'"
    done < <(mount | awk '/calico/ {print $3}' | sort -r)
  fi
}

remove_path() {
  local path="$1"
  [[ -e "${path}" ]] || return 0
  case "${path}" in
    /run/calico|/var/run/calico) unmount_calico_paths ;;
  esac
  if command -v timeout >/dev/null 2>&1; then
    run_or_ignore "timeout 120 rm -rf '${path}'"
  else
    run_or_ignore "rm -rf '${path}'"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster) CLUSTER="${2:-}"; shift 2 ;;
    --vip) VIP="${2:-}"; shift 2 ;;
    --mode) MODE="${2:-}"; shift 2 ;;
    --all-bootstrap-dirs) REMOVE_ALL_BOOTSTRAP_DIRS=1; shift ;;
    --cleanup-wireguard) MODE="detach"; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) log "Unknown option: $1"; usage; exit 1 ;;
  esac
done

case "${MODE}" in
  detach|reset) ;;
  *)
    log "Invalid --mode ${MODE} (use detach or reset)"
    exit 1
    ;;
esac

require_root "$@"

KUBECTL=""
if [[ -x /usr/local/bin/k3s ]]; then
  KUBECTL="/usr/local/bin/k3s kubectl"
elif command -v kubectl >/dev/null 2>&1; then
  KUBECTL="kubectl"
fi

cleanup_kubernetes_glue() {
  # reset mode: klusterlet namespaces already removed by cluster-delete OCM play; skip API calls.
  if [[ "${MODE}" == "reset" ]]; then
    log "Skipping in-cluster namespace cleanup (reset mode; OCM play already ran)"
    return 0
  fi
  if [[ -n "${KUBECTL}" && -f /etc/rancher/k3s/k3s.yaml ]]; then
    log "Skipping OCM klusterlet removal in detach mode (preserving spoke Traefik/platform)"
  fi
}

cleanup_bootstrap_files() {
  log "Removing bootstrap enrollment/progress temp files"
  for path in /tmp/fleet-site-enroll.json /tmp/fleet-cluster-enroll.json /tmp/fleet-cluster-progress.json; do
    [[ -f "${path}" ]] && run_or_ignore "rm -f '${path}'"
  done

  if [[ -n "${CLUSTER}" ]]; then
    log "Removing bootstrap artifacts for cluster ${CLUSTER}"
    for path in "/tmp/bootstrap-${CLUSTER}" "/tmp/bootstrap-${CLUSTER}.tar.gz" "/tmp/bootstrap-${CLUSTER}.zip"; do
      [[ -e "${path}" ]] && run_or_ignore "rm -rf '${path}'"
    done
  fi

  if [[ "${REMOVE_ALL_BOOTSTRAP_DIRS}" -eq 1 ]]; then
    log "Removing all /tmp/bootstrap-* paths"
    shopt -s nullglob
    for path in /tmp/bootstrap-*; do
      run_or_ignore "rm -rf '${path}'"
    done
    shopt -u nullglob
  fi
}

teardown_wireguard() {
  # Must run last: hub Ansible reaches nodes over fleet overlay WG (wg-umkim).
  log "Tearing down fleet overlay WireGuard (${FLEET_OVERLAY_IFACE} only)"
  run_timeout 30 "wg-quick down ${FLEET_OVERLAY_IFACE}"
  run_timeout 30 "systemctl disable wg-quick@${FLEET_OVERLAY_IFACE} 2>/dev/null"
  run_timeout 30 "systemctl stop wg-quick@${FLEET_OVERLAY_IFACE} 2>/dev/null"
  [[ -f "${FLEET_OVERLAY_CONF}" ]] && run_or_ignore "rm -f '${FLEET_OVERLAY_CONF}'"
}

remove_control_plane_vip() {
  [[ -n "${VIP}" ]] || return 0
  log "Removing control plane VIP ${VIP} from local interfaces"
  local dev
  while IFS= read -r dev; do
    [[ -z "${dev}" ]] && continue
    run_or_ignore "ip addr del '${VIP}/32' dev '${dev}'"
    run_or_ignore "ip addr del '${VIP}/24' dev '${dev}'"
  done < <(ip -o addr show 2>/dev/null | awk -v v="${VIP}" '$0 ~ v {print $2}' | sort -u)
}

reset_kubernetes() {
  remove_control_plane_vip
  log "Stopping K3s services"
  run_timeout 90 "systemctl stop k3s k3s-agent 2>/dev/null"
  run_timeout 30 "systemctl disable k3s k3s-agent 2>/dev/null"

  if [[ -x /usr/local/bin/k3s-uninstall.sh ]]; then
    log "Uninstalling K3s server"
    run_timeout 300 "/usr/local/bin/k3s-uninstall.sh"
  fi
  if [[ -x /usr/local/bin/k3s-agent-uninstall.sh ]]; then
    log "Uninstalling K3s agent"
    run_timeout 300 "/usr/local/bin/k3s-agent-uninstall.sh"
  fi

  log "Removing K3s state directories"
  for path in /etc/rancher/k3s /var/lib/rancher/k3s /etc/rancher/node /etc/rancher; do
    remove_path "${path}"
  done

  log "Removing CNI / kubelet leftovers"
  unmount_calico_paths
  for path in /etc/cni/net.d /var/lib/cni /var/lib/kubelet /run/calico /var/run/calico; do
    remove_path "${path}"
  done
  remove_control_plane_vip
}

log "Mode: ${MODE}"
cleanup_kubernetes_glue
if [[ "${MODE}" == "reset" ]]; then
  reset_kubernetes
fi
cleanup_bootstrap_files
teardown_wireguard

if [[ "${MODE}" == "reset" ]]; then
  log "Done. Node is ready for a fresh bootstrap bundle run."
else
  log "Done. K3s left running; node detached from fleet overlay and OCM."
fi
