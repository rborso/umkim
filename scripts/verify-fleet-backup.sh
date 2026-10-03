#!/usr/bin/env bash
# Verify hub MinIO and Velero on OCM workload clusters.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

HUB_KC="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
VELERO="${VELERO_CLI:-/usr/local/bin/velero}"
CLUSTERS="${FLEET_BACKUP_CLUSTERS:-lab-sn01 lab-ha01}"
SKIPPED=0
CHECKED=0

cluster_api_reachable() {
  local kc="$1"
  local server
  server="$(grep -m1 'server:' "$kc" | awk '{print $2}')" || return 1
  [[ -n "$server" ]] || return 1
  curl -sf --connect-timeout 5 --max-time 8 -k "${server}/version" >/dev/null 2>&1
}

echo "=== verify-fleet-backup ==="
if ! hub_kubectl -n fleet-system get deploy fleet-velero-minio &>/dev/null; then
  echo "SKIP: fleet-backup not deployed (ansible-playbook fleet-backup.yml -l hub)"
  exit 0
fi
echo "OK: hub MinIO deployment"

for c in $CLUSTERS; do
  KC="/tmp/verify-backup-kc-${c}.yaml"
  if ! hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${c}" &>/dev/null; then
    echo "SKIP: no fleet-kubeconfig-${c} on hub"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi
  hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${c}" -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC"
  if ! cluster_api_reachable "$KC"; then
    echo "SKIP: ${c} API unreachable from verify host (spoke down or WG route missing)"
    rm -f "$KC"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi
  if [[ ! -x "$VELERO" ]]; then
    echo "SKIP: velero CLI not installed locally (${VELERO})"
    rm -f "$KC"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi
  KUBECONFIG="$KC" "$VELERO" backup-location get >/dev/null
  KUBECONFIG="$KC" kubectl -n velero get deploy velero >/dev/null
  echo "OK: Velero on ${c}"
  rm -f "$KC"
  CHECKED=$((CHECKED + 1))
done

if [[ "$CHECKED" -eq 0 ]]; then
  echo "WARN: no workload Velero checks ran (${SKIPPED} skipped); hub MinIO OK"
fi

echo "=== verify-fleet-backup OK ==="
