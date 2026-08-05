#!/usr/bin/env bash
# Reclaim disk on the hub: stale /tmp warm artifacts, container images, Harbor registry GC.
#
# Usage: sudo ./scripts/cleanup-hub-disk.sh [--tmp-only | --images-only | --harbor-gc-only]
set -euo pipefail

MODE="${1:-all}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

log() { echo "[cleanup] $*"; }

cleanup_tmp() {
  log "removing stale /tmp warm and extract dirs"
  rm -rf /tmp/harbor-warm /tmp/phase4-warm-* /tmp/velero-* /tmp/linux-amd64 /tmp/ocm-lab-dashboard 2>/dev/null || true
  find /tmp -maxdepth 1 -name '*.tgz' -mtime +1 -delete 2>/dev/null || true
  find /tmp -maxdepth 1 -name 'gitea-*' -mtime +1 -delete 2>/dev/null || true
  find /tmp -maxdepth 1 -name 'whoami-*' -mtime +1 -delete 2>/dev/null || true
  find /tmp -maxdepth 1 -name 'pet-clinic-*' -mtime +1 -delete 2>/dev/null || true
  find "${ROOT}/.cache/harbor-push" -type f -mtime +7 -delete 2>/dev/null || true
}

cleanup_images() {
  if command -v crictl >/dev/null 2>&1; then
    log "pruning unused container images (crictl)"
    crictl rmi --prune 2>/dev/null || true
  fi
  if command -v k3s >/dev/null 2>&1; then
    log "pruning unused k3s images"
    k3s crictl rmi --prune 2>/dev/null || true
  fi
}

harbor_gc() {
  local ns="${HARBOR_NAMESPACE:-harbor}"
  local pass="${HARBOR_ADMIN_PASSWORD:-}"
  if [[ -z "${pass}" && -f "${ROOT}/inventory/group_vars/all/vault.yml" ]]; then
    pass="$(ansible-vault view "${ROOT}/inventory/group_vars/all/vault.yml" 2>/dev/null \
      | awk -F': ' '/harbor_admin_password_vault/{print $2}' | tr -d '"' || true)"
  fi
  if [[ -z "${pass}" ]]; then
    log "WARN: HARBOR_ADMIN_PASSWORD not set — skip Harbor GC"
    return 0
  fi
  if ! kubectl -n "${ns}" get deploy harbor-core >/dev/null 2>&1; then
    log "WARN: harbor-core not found — skip Harbor GC"
    return 0
  fi
  log "triggering Harbor garbage collection"
  kubectl -n "${ns}" exec deploy/harbor-core -- curl -sf -u "admin:${pass}" \
    -X POST -H 'Content-Type: application/json' \
    'http://127.0.0.1:8080/api/v2.0/system/gc/schedule' \
    -d '{"schedule":{"type":"Manual"}}' >/dev/null \
    || kubectl -n "${ns}" exec deploy/harbor-core -- curl -sf -u "admin:${pass}" \
    -X POST -H 'Content-Type: application/json' \
    'http://127.0.0.1:8080/api/v2.0/system/gc' \
    -d '{"schedule":{"type":"Manual"}}' >/dev/null \
    || log "WARN: Harbor GC API call failed (may already be running)"
}

harbor_prune_charts() {
  local pass="${HARBOR_ADMIN_PASSWORD:-}"
  [[ -n "${pass}" ]] || return 0
  local api="https://10.10.0.1:30443/api/v2.0"
  for repo in gitea json-pretty ebook-converter; do
    log "delete orphaned chart artifacts: charts/${repo}"
    mapfile -t digests < <(curl -sk -u "admin:${pass}" \
      "${api}/projects/charts/repositories/${repo}/artifacts" \
      | python3 -c "import sys,json; arts=json.load(sys.stdin); print('\n'.join(a['digest'] for a in arts if not a.get('tags')))" 2>/dev/null || true)
    for d in "${digests[@]}"; do
      [[ -n "${d}" ]] || continue
      curl -sk -u "admin:${pass}" -X DELETE \
        "${api}/projects/charts/repositories/${repo}/artifacts/${d}" >/dev/null || true
    done
  done
}

case "${MODE}" in
  --tmp-only) cleanup_tmp ;;
  --images-only) cleanup_images ;;
  --harbor-gc-only) harbor_gc ;;
  all|"") cleanup_tmp; cleanup_images; harbor_prune_charts; harbor_gc ;;
  -h|--help)
    sed -n '1,6p' "$0"
    exit 0
    ;;
  *) echo "unknown mode: ${MODE}" >&2; exit 1 ;;
esac

log "done — disk: $(df -h / | awk 'NR==2{print $3" used, "$4" free ("$5")"}')"
