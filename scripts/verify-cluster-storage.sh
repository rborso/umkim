#!/usr/bin/env bash
# Verify nfs-client StorageClass and RWX PVC bind on a spoke cluster.
# Usage: ./scripts/verify-cluster-storage.sh [cluster] [storage_class]
set -euo pipefail

CLUSTER="${1:-virt02}"
SC="${2:-nfs-client}"
NS="fleet-storage-verify"
PVC="nfs-rwx-test"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
KC_FILE="${KC_FILE:-/tmp/fleet-kc-${CLUSTER}-storage-verify.yaml}"

run_kubectl() {
  if [[ "${CLUSTER}" == "virt02" ]]; then
    ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -J linux@10.10.0.2 linux@198.51.100.210 "sudo env -u KUBECONFIG kubectl $*"
  elif [[ "${CLUSTER}" == "virt01" ]]; then
    ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -J linux@10.10.0.2 linux@198.51.100.225 "sudo env -u KUBECONFIG kubectl $*"
  elif [[ -n "${FLEET_HUB_IP:-}" ]] && [[ -f "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh" ]]; then
    # shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
    source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"
    if [[ ! -s "${KC_FILE}" ]]; then
      hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${CLUSTER}" \
        -o jsonpath='{.data.kubeconfig}' | base64 -d >"${KC_FILE}"
      api="${FLEET_SITE_API_DIRECT:-${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}}"
      wg_host="$(curl -sf "${api}/clusters/${CLUSTER}" 2>/dev/null \
        | python3 -c "import json,sys; u=json.load(sys.stdin).get('upstream_host',''); print(u.split(':')[0] if u else '')" 2>/dev/null || true)"
      if [[ -n "${wg_host}" ]]; then
        sed -i -E "s|server: https://[^[:space:]]+:6443|server: https://${wg_host}:6443|" "${KC_FILE}"
      fi
    fi
    kubectl --kubeconfig="${KC_FILE}" --insecure-skip-tls-verify "$@" || {
      rm -f "${KC_FILE}"
      return 1
    }
  else
    env -u KUBECONFIG kubectl "$@"
  fi
}

echo "== StorageClass ${SC} =="
run_kubectl get storageclass "${SC}" -o wide

echo "== Test RWX PVC =="
run_kubectl create namespace "${NS}" --dry-run=client -o yaml | run_kubectl apply -f -
run_kubectl -n "${NS}" delete pvc "${PVC}" --ignore-not-found
run_kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${PVC}
  namespace: ${NS}
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: ${SC}
  resources:
    requests:
      storage: 64Mi
EOF

for _ in $(seq 1 60); do
  phase="$(run_kubectl -n "${NS}" get pvc "${PVC}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  if [[ "${phase}" == "Bound" ]]; then
    run_kubectl -n "${NS}" get pvc "${PVC}" -o wide
    echo "OK   ${CLUSTER} RWX PVC bound via ${SC}"
    exit 0
  fi
  sleep 2
done

run_kubectl -n "${NS}" get pvc "${PVC}" -o wide || true
echo "FAIL ${CLUSTER} PVC not Bound within timeout"
exit 1
