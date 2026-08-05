#!/usr/bin/env bash
# Pre-monolith VPS storage cleanup: Harbor fleet DCM manager artifacts, GC, hub podman cache.
# Run on the Ansible controller; hub podman step runs via SSH when HUB_SSH is set.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HUB_SSH="${HUB_SSH:-linux@10.10.0.1}"
HARBOR_URL="${HARBOR_URL:-https://10.10.0.1:30443}"
HARBOR_USER="${HARBOR_USER:-admin}"
HARBOR_PASS="${HARBOR_PASS:-FleetHarbor-ChangeMe}"
SKIP_FLEET_CLEANUP="${SKIP_FLEET_CLEANUP:-false}"
SKIP_HARBOR="${SKIP_HARBOR:-false}"
SKIP_PODMAN="${SKIP_PODMAN:-false}"

log() { echo "[dcm-pre-monolith] $*"; }

log "=== inventory snapshot ==="
if command -v kubectl >/dev/null 2>&1 && kubectl -n dcm get deploy >/dev/null 2>&1; then
  kubectl -n dcm get deploy -o custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image 2>/dev/null || true
else
  log "kubectl dcm namespace not available locally; skip deploy inventory"
fi

curl -sk -u "$HARBOR_USER:$HARBOR_PASS" \
  "$HARBOR_URL/api/v2.0/projects/fleet/repositories" 2>/dev/null \
  | python3 -c "import json,sys; [print(r['name']) for r in json.load(sys.stdin)]" 2>/dev/null \
  || log "Harbor fleet repo list unavailable"

if [[ "$SKIP_FLEET_CLEANUP" != "true" ]]; then
  log "=== optional: stop DCM workloads (set SKIP_FLEET_CLEANUP=true to skip) ==="
  log "To tear down workloads first: ansible-playbook fleet-cleanup.yml -l hub"
fi

if [[ "$SKIP_HARBOR" != "true" ]]; then
  log "=== Harbor: delete untagged fleet DCM manager artifacts ==="
  DCM_REPOS="catalog-manager policy-manager placement-manager \
    fleet-site-controller dcm-fleet-console fleet-console-auth fleet-ansible-runner"
  "$ROOT/scripts/cleanup-harbor-fleet-artifacts.sh" $DCM_REPOS

  log "=== Harbor: garbage collection ==="
  HARBOR_URL="$HARBOR_URL" HARBOR_USER="$HARBOR_USER" HARBOR_PASS="$HARBOR_PASS" \
    "$ROOT/scripts/cleanup-harbor-gc.sh"
else
  log "SKIP_HARBOR=true — skipping Harbor cleanup"
fi

if [[ "$SKIP_PODMAN" != "true" ]]; then
  log "=== hub podman + fleet-src cache (via ${HUB_SSH}) ==="
  ssh -o ConnectTimeout=10 "$HUB_SSH" bash -s <<'REMOTE'
set -euo pipefail
if [[ -x ./scripts/cleanup-hub-podman-images.sh ]]; then
  sudo ./scripts/cleanup-hub-podman-images.sh
elif command -v podman >/dev/null 2>&1; then
  podman image prune -af || true
fi
sudo rm -rf /opt/fleet-src/catalog-manager /opt/fleet-src/policy-manager /opt/fleet-src/placement-manager 2>/dev/null || true
if command -v k3s >/dev/null 2>&1; then
  echo "--- k3s images (dcm-related) ---"
  sudo k3s crictl images 2>/dev/null | grep -E 'catalog-manager|policy-manager|placement-manager|service-provider-manager|control-plane' || true
fi
REMOTE
else
  log "SKIP_PODMAN=true — skipping hub podman cleanup"
fi

log "=== done ==="
log "Next: harbor_push_fleet control-plane, phase5_hub"
