#!/usr/bin/env bash
# Velero restore for a catalog instance workload on a spoke cluster.
#
# Usage:
#   ./scripts/restore-spoke-instance.sh \
#     --cluster lab-int01 --backup <name> --namespace gitea --release gitea-br-test \
#     [--source-cluster lab-int01] [--storage-class-map local-path:longhorn]
set -euo pipefail

CLUSTER=""
BACKUP=""
NAMESPACE="default"
RELEASE=""
SOURCE_CLUSTER=""
STORAGE_CLASS_MAP=""
VELERO="${VELERO_CLI:-/usr/local/bin/velero}"
HUB_KC="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster) CLUSTER="$2"; shift 2 ;;
    --backup) BACKUP="$2"; shift 2 ;;
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --release) RELEASE="$2"; shift 2 ;;
    --source-cluster) SOURCE_CLUSTER="$2"; shift 2 ;;
    --storage-class-map) STORAGE_CLASS_MAP="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$CLUSTER" && -n "$BACKUP" && -n "$RELEASE" ]] || {
  echo "ERROR: --cluster, --backup, --release required" >&2
  exit 1
}

hub_kubectl() {
  if [[ -r "$HUB_KC" ]]; then
    kubectl --kubeconfig="$HUB_KC" "$@"
  else
    sudo kubectl --kubeconfig="$HUB_KC" "$@"
  fi
}

KC_FILE="/tmp/fleet-restore-kc-${CLUSTER}-$$.yaml"
hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${CLUSTER}" \
  -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC_FILE"
export KUBECONFIG="$KC_FILE"

RESTORE_NAME="restore-${RELEASE}-$(date -u +%H%M%S)"
RESTORE_ARGS=(
  restore create "$RESTORE_NAME"
  --from-backup "$BACKUP"
  --include-namespaces "$NAMESPACE"
  --selector "app.kubernetes.io/instance=${RELEASE}"
  --wait
)

if [[ -n "$STORAGE_CLASS_MAP" ]]; then
  # Velero v1.15 restore CLI has no --storage-class-mappings; node-agent restore uses
  # target cluster default StorageClass when the source class is unavailable.
  echo "NOTE: storage-class-map ${STORAGE_CLASS_MAP} (ensure target cluster default SC is correct)" >&2
fi

"$VELERO" "${RESTORE_ARGS[@]}" >&2

if [[ -n "$SOURCE_CLUSTER" && "$SOURCE_CLUSTER" != "$CLUSTER" ]]; then
  echo "restored backup ${BACKUP} from ${SOURCE_CLUSTER} onto ${CLUSTER}"
fi

rm -f "$KC_FILE"
echo "$RESTORE_NAME"
