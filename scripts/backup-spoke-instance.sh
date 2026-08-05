#!/usr/bin/env bash
# Velero backup for a catalog instance workload on a spoke cluster.
#
# Usage:
#   ./scripts/backup-spoke-instance.sh \
#     --cluster lab-int01 --instance-id <uid> --release gitea-br-test --namespace gitea [--wait]
set -euo pipefail

CLUSTER=""
INSTANCE_ID=""
RELEASE=""
NAMESPACE="default"
WAIT=false
ROOT="${FLEET_INSTANCE_BACKUP_ROOT:-/var/backups/umkim/instances}"
VELERO="${VELERO_CLI:-/usr/local/bin/velero}"
HUB_KC="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster) CLUSTER="$2"; shift 2 ;;
    --instance-id) INSTANCE_ID="$2"; shift 2 ;;
    --release) RELEASE="$2"; shift 2 ;;
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --wait) WAIT=true; shift ;;
    --root) ROOT="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$CLUSTER" && -n "$INSTANCE_ID" && -n "$RELEASE" ]] || {
  echo "ERROR: --cluster, --instance-id, --release required" >&2
  exit 1
}

STAMP="$(date -u +%Y%m%d%H%M%S)"
DEST="${ROOT}/${STAMP}"
mkdir -p "$DEST"

hub_kubectl() {
  if [[ -r "$HUB_KC" ]]; then
    kubectl --kubeconfig="$HUB_KC" "$@"
  else
    sudo kubectl --kubeconfig="$HUB_KC" "$@"
  fi
}

KC_FILE="${DEST}/kubeconfig-${CLUSTER}.yaml"
hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${CLUSTER}" \
  -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC_FILE"

export KUBECONFIG="$KC_FILE"
BACKUP_NAME="instance-${RELEASE}-${STAMP}"

"$VELERO" backup create "$BACKUP_NAME" \
  --include-namespaces "$NAMESPACE" \
  --selector "app.kubernetes.io/instance=${RELEASE}" \
  --labels "fleet.umkim.com/instance-id=${INSTANCE_ID},fleet.umkim.com/backup=enabled" \
  --wait="$WAIT" >&2

./scripts/export-dcm-instance.sh --id "$INSTANCE_ID" --out "${DEST}/dcm-instance.json" >/dev/null

DOMAIN="${FLEET_EDGE_DOMAIN:-apps.example.com}"
cat >"${DEST}/metadata.json" <<EOF
{
  "timestamp": "${STAMP}",
  "cluster": "${CLUSTER}",
  "release": "${RELEASE}",
  "namespace": "${NAMESPACE}",
  "instance_id": "${INSTANCE_ID}",
  "velero_backup": "${BACKUP_NAME}",
  "hub_host": "${RELEASE}.${DOMAIN}"
}
EOF
echo "$BACKUP_NAME" >"${DEST}/velero-backup-name.txt"
echo "$DEST"
