#!/usr/bin/env bash
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_VERIFY_CLUSTER:-lab-sn01}"

curl -sf "${API}/clusters/${CLUSTER}" >/dev/null || { echo "SKIP cluster ${CLUSTER} not found"; exit 0; }

curl -sf -X POST "${API}/clusters/${CLUSTER}/discover-capabilities" >/dev/null || {
  echo "FAIL discover-capabilities"; exit 1;
}
echo "OK fleet-clusters-discover ${CLUSTER}"
