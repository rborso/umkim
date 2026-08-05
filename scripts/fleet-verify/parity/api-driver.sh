#!/usr/bin/env bash
# API driver for parity operations (console-proxied :30912).
set -euo pipefail

OP="${1:-}"
CHECK_ID="${2:-${OP}}"
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"
CONSOLE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
FLEET_API="${CONSOLE}/fleet/api/v1alpha1"
DCM_API="${CONSOLE}/api/v1alpha1"
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
  parity.policy.crud|policy.crud)
    POLICY_NAME="E2E Policy ${RUN_ID}"
    POLICY_PRIORITY="$((200 + RANDOM % 800))"
    export DCM_API_URL="${DCM_API}"
    export FLEET_AUTH_COOKIE_JAR="${FV_AUTH_ADMIN_JAR}"
    BODY=$(POLICY_NAME="${POLICY_NAME}" POLICY_PRIORITY="${POLICY_PRIORITY}" python3 - <<'PY'
import json, os, random
print(json.dumps({
  "display_name": os.environ["POLICY_NAME"],
  "description": "parity",
  "policy_type": "GLOBAL",
  "enabled": True,
  "priority": int(os.environ.get("POLICY_PRIORITY", str(9000 + random.randint(0, 999)))),
  "rego_code": 'package dcm.fleet.demo.container\n\nmain := {\n  "rejected": false\n} if input.spec.service_type == "container"\n',
}))
PY
)
    code=$(curl -s -b "${FV_AUTH_ADMIN_JAR}" -o /tmp/fv-policy.json -w '%{http_code}' -X POST "${DCM_API}/policies" \
      -H 'Content-Type: application/json' -d "${BODY}")
    echo "API policy.create name=${POLICY_NAME} http=${code}"
    [[ "${code}" == "201" || "${code}" == "200" || "${code}" == "409" ]] || exit 1
    bash "${ROOT}/ui/dcm-fleet-console/scripts/verify-policy-api.sh" assert-exists --display-name "${POLICY_NAME}"
    pid=$(curl -sf -b "${FV_AUTH_ADMIN_JAR}" "${DCM_API}/policies" \
      | POLICY_NAME="${POLICY_NAME}" python3 -c "import json,sys,os; n=os.environ['POLICY_NAME']; print(next((p['id'] for p in json.load(sys.stdin).get('policies',[]) if p.get('display_name')==n), ''))")
    [[ -n "${pid}" ]] || exit 1
    bash "${ROOT}/ui/dcm-fleet-console/scripts/verify-policy-api.sh" delete --id "${pid}"
    ;;
  parity.deploy.submit|deploy.submit)
    FV_RUN_ID="${RUN_ID}" bash "${FRAMEWORK}/checks/whoami-deploy.sh"
    ;;
  *)
    echo "unknown parity operation: ${OP}" >&2
    exit 1
    ;;
esac
