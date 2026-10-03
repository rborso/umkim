#!/usr/bin/env bash
# Remove stale FleetClusterJoin CRs and orphaned ManagedClusters that show unavailable
# in the fleet API (verify-* test clusters, abandoned enrollments, etc.).
# Keeps production spokes: virt01, virt02.
set -euo pipefail

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
NS="${FLEET_CONSOLE_NS:-fleet-console}"
KEEP_REGEX="${FLEET_CLEANUP_KEEP:-^(virt01|virt02)$}"
DRY_RUN="${FLEET_CLEANUP_DRY_RUN:-0}"

log() { echo "=== $* ==="; }

kubectl_cmd() {
  if command -v kubectl >/dev/null 2>&1 && kubectl cluster-info >/dev/null 2>&1; then
    kubectl "$@"
  else
    sudo kubectl "$@"
  fi
}

delete_fcj() {
  local name="$1"
  if [ "${DRY_RUN}" = "1" ]; then
    echo "[dry-run] delete fleetclusterjoin/${name}"
    return 0
  fi
  kubectl_cmd delete fleetclusterjoin "${name}" -n "${NS}" --ignore-not-found --wait=false
  kubectl_cmd delete secret -n "${NS}" -l "fleet.umkim.com/cluster=${name}" --ignore-not-found 2>/dev/null || true
  kubectl_cmd delete job -n "${NS}" -l "fleet.umkim.com/cluster=${name}" --ignore-not-found 2>/dev/null || true
}

delete_mc() {
  local name="$1"
  if [ "${DRY_RUN}" = "1" ]; then
    echo "[dry-run] delete managedcluster/${name}"
    return 0
  fi
  kubectl_cmd delete managedcluster "${name}" --ignore-not-found --wait=false
}

log "List unavailable clusters from ${API}"
mapfile -t UNAVAILABLE < <(
  curl -sf "${API}/clusters" | python3 -c "
import json, re, sys
keep = re.compile(sys.argv[1])
for c in json.load(sys.stdin).get('clusters', []):
    if c.get('available') is False and not keep.match(c['name']):
        print(c['name'])
" "${KEEP_REGEX}"
)

if [ "${#UNAVAILABLE[@]}" -eq 0 ]; then
  log "No unavailable clusters to clean (keep=${KEEP_REGEX})"
else
  log "Cleaning ${#UNAVAILABLE[@]} unavailable cluster(s): ${UNAVAILABLE[*]}"
  for name in "${UNAVAILABLE[@]}"; do
    delete_fcj "${name}"
    delete_mc "${name}"
  done
fi

log "Remove verify-* FleetClusterJoin CRs (test artifacts)"
mapfile -t VERIFY_FCJ < <(kubectl_cmd get fleetclusterjoin -n "${NS}" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep '^verify-' || true)
for name in "${VERIFY_FCJ[@]}"; do
  [ -n "${name}" ] || continue
  delete_fcj "${name}"
done

log "Remove verify-* completed Jobs"
if [ "${DRY_RUN}" != "1" ]; then
  kubectl_cmd get jobs -n "${NS}" -o name 2>/dev/null | grep 'verify-' | xargs -r kubectl_cmd delete --ignore-not-found 2>/dev/null || true
fi

sleep 3
log "Remaining FleetClusterJoin"
kubectl_cmd get fleetclusterjoin -n "${NS}" 2>/dev/null || true
log "Remaining ManagedClusters"
kubectl_cmd get managedcluster 2>/dev/null || true
echo "=== cleanup-unavailable-fleet-clusters OK ==="
