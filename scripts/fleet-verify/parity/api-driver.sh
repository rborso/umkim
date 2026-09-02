#!/usr/bin/env bash
# API driver for parity operations (console-proxied :30912).
set -euo pipefail

OP="${1:-}"
CHECK_ID="${2:-${OP}}"
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"
CONSOLE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
FLEET_API="${CONSOLE}/fleet/api/v1alpha1"
RUN_ID="${FV_RUN_ID:-$(date +%Y%m%d%H%M%S)}"

source "${FRAMEWORK}/lib/auth.sh"
fv_auth_admin_login

case "${OP}" in
  parity.site.create|site.create)
    NAME="verify-synth-${RUN_ID}-parity-site"
    code=$(curl -s -b "${FV_AUTH_ADMIN_JAR}" -o /dev/null -w '%{http_code}' -X POST "${FLEET_API}/sites" \
      -H 'Content-Type: application/json' \
      -d "{\"apiVersion\":\"fleet.umkim.com/v1alpha1\",\"kind\":\"FleetSite\",\"metadata\":{\"name\":\"${NAME}\"},\"spec\":{\"siteName\":\"${NAME}\",\"provisioningMode\":\"registry\",\"displayName\":\"Parity ${RUN_ID}\"}}")
    phase=$(curl -sf -b "${FV_AUTH_ADMIN_JAR}" "${FLEET_API}/sites/${NAME}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('phase') or d.get('status',{}).get('phase',''))" 2>/dev/null || echo "")
    echo "API site.create name=${NAME} http=${code} phase=${phase}"
    export FV_PARITY_SITE_NAME="${NAME}"
    [[ "${code}" == "201" || "${code}" == "409" ]] || exit 1
    ;;
  parity.deploy.submit|deploy.submit)
    FV_RUN_ID="${RUN_ID}" bash "${ROOT}/scripts/verify-fleet-app.sh"
    ;;
  *)
    echo "unknown parity operation: ${OP}" >&2
    exit 1
    ;;
esac
