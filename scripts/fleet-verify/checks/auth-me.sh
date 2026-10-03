#!/usr/bin/env bash
set -euo pipefail

FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
AUTH="${FLEET_CONSOLE_URL%/}/fleet/auth/v1alpha1"
FV_AUTH_ADMIN_JAR="${FV_AUTH_ADMIN_JAR:-${TMPDIR:-/tmp}/fleet-verify-admin.jar}"
USER="${FLEET_AUTH_USER:-localadmin}"
PASS="${FLEET_AUTH_PASSWORD:-}"

fv_root="$(cd "$(dirname "$0")/../../.." && pwd)"
if [[ -z "${PASS}" && -f "${fv_root}/scripts/fleet-verify/lib/auth.sh" ]]; then
  # shellcheck source=/dev/null
  source "${fv_root}/scripts/fleet-verify/lib/auth.sh"
  fv_auth_load_password
  PASS="${FLEET_AUTH_PASSWORD}"
fi
PASS="${PASS:-CHANGE_ME}"

rm -f "${FV_AUTH_ADMIN_JAR}"
curl -sf -c "${FV_AUTH_ADMIN_JAR}" -X POST "${AUTH}/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USER}\",\"password\":\"${PASS}\"}" >/dev/null

me="$(curl -sf -b "${FV_AUTH_ADMIN_JAR}" "${AUTH}/me")"
if echo "${me}" | python3 -c "
import json, sys
d = json.load(sys.stdin)
roles = d.get('roles')
sys.exit(0 if roles is not None else 1)
"; then
  echo "OK auth /me (roles present)"
  exit 0
fi

echo "FAIL auth /me missing roles" >&2
exit 1
