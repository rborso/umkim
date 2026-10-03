#!/usr/bin/env bash
# E2E: Ready FleetClusterJoin with local_path → configure-storage → nfs-client SC + RWX PVC.
#
# Prerequisites:
#   - Cluster exists, phase Ready, reachable via hub fleet-site SSH inventory
#   - External NFS server configured (see docs/fleet-cluster-storage.md)
#   - FLEET_API (default http://127.0.0.1:18080/api/v1alpha1)
#   - FLEET_BEARER_TOKEN or console session cookie via fleet-site nginx
#
# Env:
#   CLUSTER          cluster name (required)
#   NFS_SERVER       NFS server IP (required for configure)
#   NFS_DELEGATE     delegate host for export mkdir (default: NFS_SERVER)
#   NFS_EXPORT_BASE  default /srv/fleet-nfs
#   FLEET_API        fleet-site API base
#   SKIP_PVC         set 1 to skip PVC bind test (API/job only)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLUSTER="${CLUSTER:-}"
NFS_SERVER="${NFS_SERVER:-}"
NFS_DELEGATE="${NFS_DELEGATE:-${NFS_SERVER}}"
NFS_EXPORT_BASE="${NFS_EXPORT_BASE:-/srv/fleet-nfs}"
FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
SKIP_PVC="${SKIP_PVC:-0}"

log() { echo "[verify-fleet-cluster-storage] $*"; }
die() { log "ERROR: $*"; exit 1; }

[[ -n "${CLUSTER}" ]] || die "CLUSTER is required"
[[ -n "${NFS_SERVER}" ]] || die "NFS_SERVER is required"

AUTH_HEADER=()
if [[ -n "${FLEET_BEARER_TOKEN:-}" ]]; then
  AUTH_HEADER=(-H "Authorization: Bearer ${FLEET_BEARER_TOKEN}")
fi

api() {
  local method="$1" path="$2"
  shift 2
  curl -sf "${AUTH_HEADER[@]}" -X "${method}" "${FLEET_API}${path}" "$@"
}

log "GET cluster ${CLUSTER}"
phase="$(api GET "/clusters/${CLUSTER}" | jq -r '.provisioning_phase // empty')"
[[ "${phase}" == "Ready" ]] || die "cluster phase is ${phase:-unknown}, want Ready"

payload="$(jq -n \
  --arg srv "${NFS_SERVER}" \
  --arg base "${NFS_EXPORT_BASE}" \
  --arg del "${NFS_DELEGATE}" \
  --arg path "${CLUSTER}" \
  '{
    backend: "nfs_external",
    rwo: true,
    rwx: true,
    nfs: {
      server: $srv,
      exportBase: $base,
      clusterPath: $path,
      storageClass: "nfs-client",
      delegateHost: $del
    }
  }')"

log "POST configure-storage"
api POST "/clusters/${CLUSTER}/configure-storage" \
  -H "Content-Type: application/json" \
  -d "${payload}" >/dev/null

deadline=$((SECONDS + 600))
storage_phase=""
while (( SECONDS < deadline )); do
  storage_phase="$(api GET "/clusters/${CLUSTER}" | jq -r '.storageStatus.phase // empty')"
  case "${storage_phase}" in
    Applied)
      log "storage phase Applied"
      break
      ;;
    Failed)
      msg="$(api GET "/clusters/${CLUSTER}" | jq -r '.storageStatus.message // empty')"
      die "configure-storage failed: ${msg}"
      ;;
    Applying|Pending|"")
      sleep 10
      ;;
    *)
      sleep 10
      ;;
  esac
done
[[ "${storage_phase}" == "Applied" ]] || die "timeout waiting for storage Applied (last: ${storage_phase:-none})"

if [[ "${SKIP_PVC}" == "1" ]]; then
  log "SKIP_PVC=1 — done"
  exit 0
fi

log "discover-capabilities"
api POST "/clusters/${CLUSTER}/discover-capabilities" >/dev/null || true
sleep 15

if [[ -x "${ROOT}/scripts/verify-cluster-storage.sh" ]]; then
  # Uses cluster-specific kubectl when not virt02; extend verify-cluster-storage for WG SSH as needed.
  log "PVC bind test via verify-cluster-storage.sh (may require cluster kubeconfig/SSH setup)"
  "${ROOT}/scripts/verify-cluster-storage.sh" "${CLUSTER}" nfs-client || die "PVC verify failed"
fi

log "OK ${CLUSTER} NFS configure-storage E2E"
