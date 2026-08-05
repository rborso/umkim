#!/usr/bin/env bash
# Prune stale podman build images on the hub (post harbor_push_fleet).
set -euo pipefail

log() { echo "[podman-cleanup] $*"; }

if ! command -v podman >/dev/null 2>&1; then
  log "podman not installed; skip"
  exit 0
fi

before=$(podman images -q 2>/dev/null | wc -l)
log "images before: ${before}"

log "remove non-main localhost fleet/dcm build tags"
while read -r img; do
  [[ -z "${img}" ]] && continue
  log "rmi ${img}"
  podman rmi "${img}" 2>/dev/null || true
done < <(podman images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null \
  | grep -E '^localhost/(fleet-|dcm-fleet)' \
  | grep -v ':main$' || true)

log "prune dangling layers"
podman image prune -f

log "prune all unused images (not used by running podman containers)"
podman image prune -a -f

after=$(podman images -q 2>/dev/null | wc -l)
log "images after: ${after} (removed $((before - after)))"
podman system df 2>/dev/null || true
log "done"
