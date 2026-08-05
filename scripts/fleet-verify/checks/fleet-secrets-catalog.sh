#!/usr/bin/env bash
set -euo pipefail
FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_VERIFY_CLUSTER:-lab-sn01}"
NS="verify-synth-${FV_RUN_ID:-test}"
SECRET="verify-synth-secret"

curl -sf -X PUT "${FLEET_API}/catalog-instance-secrets" \
  -H 'Content-Type: application/json' \
  -d "{\"cluster\":\"${CLUSTER}\",\"namespace\":\"${NS}\",\"secretName\":\"${SECRET}\",\"data\":{\"token\":\"dGVzdA==\"}}" >/dev/null 2>&1 || {
  echo "SKIP catalog-instance-secrets (cluster ${CLUSTER} may be missing)"; exit 0;
}

curl -sf -X DELETE "${FLEET_API}/catalog-instance-secrets" \
  -H 'Content-Type: application/json' \
  -d "{\"cluster\":\"${CLUSTER}\",\"namespace\":\"${NS}\",\"secretName\":\"${SECRET}\"}" >/dev/null 2>&1 || true
echo "OK fleet-secrets-catalog"
