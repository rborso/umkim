#!/usr/bin/env bash
# Ensure k8s + helm SPM providers are Ready on FLEET_VERIFY_CLUSTER.
set -euo pipefail

CLUSTER="${FLEET_VERIFY_CLUSTER:-lab-sn01}"
HUB="${FLEET_HUB_IP:-127.0.0.1}"
API="http://${HUB}:18080/api/v1alpha1"
DCM_API="http://${HUB}:30908/api/v1alpha1"

provider_registered() {
  local kind="$1"
  curl -sf "${DCM_API}/providers" 2>/dev/null \
    | python3 -c "import json,sys; c=sys.argv[1]; print('yes' if any(p.get('name')==c for p in json.load(sys.stdin).get('providers',[])) else 'no')" \
      "${kind}-${CLUSTER}" 2>/dev/null | grep -q yes
}

fv_ensure_cluster_providers() {
  local kind code
  for kind in k8s helm; do
    if provider_registered "${kind}"; then
      continue
    fi
    code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API}/clusters/${CLUSTER}/providers/${kind}/install" || echo 000)
    [[ "${code}" == "200" || "${code}" == "202" || "${code}" == "409" ]] || {
      echo "FAIL: provider ${kind} install HTTP ${code} on ${CLUSTER} (API ${API})" >&2
      return 1
    }
  done
  for _ in $(seq 1 60); do
    if provider_registered k8s && provider_registered helm; then
      return 0
    fi
    sleep 5
  done
  echo "FAIL: providers not registered for ${CLUSTER}" >&2
  return 1
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  fv_ensure_cluster_providers
fi
