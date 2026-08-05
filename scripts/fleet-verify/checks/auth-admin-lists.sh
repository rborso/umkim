#!/usr/bin/env bash
set -euo pipefail

FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
AUTH="${FLEET_CONSOLE_URL%/}/fleet/auth/v1alpha1"
FV_AUTH_ADMIN_JAR="${FV_AUTH_ADMIN_JAR:-${TMPDIR:-/tmp}/fleet-verify-admin.jar}"

if [[ ! -s "${FV_AUTH_ADMIN_JAR}" ]]; then
  fv_root="$(cd "$(dirname "$0")/../../.." && pwd)"
  if [[ -f "${fv_root}/scripts/fleet-verify/lib/auth.sh" ]]; then
    # shellcheck source=/dev/null
    source "${fv_root}/scripts/fleet-verify/lib/auth.sh"
    fv_auth_admin_login
  else
    echo "FAIL auth-admin-lists: FV_AUTH_ADMIN_JAR missing and auth helper unavailable" >&2
    exit 1
  fi
fi

failed=0
for path in users groups role-bindings service-accounts audit-events orgs; do
  code="$(curl -s -b "${FV_AUTH_ADMIN_JAR}" -o /dev/null -w '%{http_code}' "${AUTH}/${path}" || true)"
  if [[ "${code}" == "200" ]]; then
    echo "OK auth GET /${path}"
  else
    echo "FAIL auth GET /${path} HTTP ${code}" >&2
    failed=1
  fi
done

exit "${failed}"
