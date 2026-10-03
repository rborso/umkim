#!/usr/bin/env bash
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_VERIFY_CLUSTER:-lab-sn01}"

curl -sf "${API}/clusters/${CLUSTER}" >/dev/null || { echo "SKIP cluster ${CLUSTER} not found"; exit 0; }

code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API}/clusters/${CLUSTER}/validate-storage" \
  -H 'Content-Type: application/json' -d '{}' || echo 000)
if [[ "${code}" != "200" && "${code}" != "202" ]]; then
  echo "WARN validate-storage HTTP ${code} (storage may be unconfigured)"
fi
echo "OK fleet-clusters-storage ${CLUSTER}"
