#!/usr/bin/env bash
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
NAME="verify-synth-${FV_RUN_ID:-test}-site"
DISPLAY="Verify synth site ${FV_RUN_ID:-test}"

code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API}/sites" \
  -H 'Content-Type: application/json' \
  -d "{\"apiVersion\":\"fleet.umkim.com/v1alpha1\",\"kind\":\"FleetSite\",\"metadata\":{\"name\":\"${NAME}\"},\"spec\":{\"siteName\":\"${NAME}\",\"provisioningMode\":\"registry\",\"displayName\":\"${DISPLAY}\"}}")
[[ "${code}" == "201" || "${code}" == "409" ]] || { echo "FAIL POST site HTTP ${code}"; exit 1; }

curl -sf -X PATCH "${API}/sites/${NAME}" \
  -H 'Content-Type: application/json' \
  -d "{\"displayName\":\"${DISPLAY} patched\"}" >/dev/null

curl -sf "${API}/sites/${NAME}" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'patched' in (d.get('displayName') or d.get('spec',{}).get('displayName',''))" || {
  echo "FAIL PATCH site"; exit 1;
}

curl -sf -X DELETE "${API}/sites/${NAME}" >/dev/null 2>&1 || true
echo "OK fleet-sites-patch"
