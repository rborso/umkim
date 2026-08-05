#!/usr/bin/env bash
# Run Playwright E2E against deployed fleet console on hub NodePort (:30912).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="${ROOT}/ui/dcm-fleet-console"

export FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://10.10.0.1:30912}"
export DCM_API_URL="${DCM_API_URL:-${FLEET_CONSOLE_URL}/api/v1alpha1}"
export FLEET_API_URL="${FLEET_API_URL:-${FLEET_CONSOLE_URL}/fleet/api/v1alpha1}"
export FLEET_AUTH_USER="${FLEET_AUTH_USER:-localadmin}"
export FLEET_AUTH_PASSWORD="${FLEET_AUTH_PASSWORD:-fleet-admin-change-me}"

PW_GREP="${PW_GREP:-}"
PW_SPECS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --grep) PW_GREP="$2"; shift 2 ;;
    --parity) PW_GREP="@parity"; shift ;;
    --spec) PW_SPECS+=("$2"); shift 2 ;;
    *) shift ;;
  esac
done

echo "=== Fleet console Playwright E2E ==="
echo "FLEET_CONSOLE_URL=${FLEET_CONSOLE_URL}"

echo "== preflight: fleet-console-auth (readonly) =="
VERIFY_CREATE_ORG=0 FLEET_VERIFY_MUTATING=0 \
  FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL}" \
  FLEET_AUTH_USER="${FLEET_AUTH_USER}" \
  FLEET_AUTH_PASSWORD="${FLEET_AUTH_PASSWORD}" \
  "${ROOT}/scripts/verify-fleet-console-auth.sh"

cd "${UI_DIR}"
echo "== npm ci =="
npm ci

echo "== playwright chromium =="
npx playwright install chromium

if ! npx playwright test -c e2e/playwright.config.ts --list >/dev/null 2>&1; then
  echo "Hint: if Chromium fails to launch (missing libatk etc.), run:" >&2
  echo "  cd ${UI_DIR} && npx playwright install-deps chromium" >&2
fi

echo "== optional catalog preflight (warn only) =="
COOKIE_JAR="${TMPDIR:-/tmp}/fleet-e2e-preflight-cookies.txt"
rm -f "${COOKIE_JAR}"
curl -s -c "${COOKIE_JAR}" -X POST "${FLEET_CONSOLE_URL}/fleet/auth/v1alpha1/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${FLEET_AUTH_USER}\",\"password\":\"${FLEET_AUTH_PASSWORD}\"}" >/dev/null
for item in demo-webapp whoami-helm "${E2E_OPERATOR_CATALOG:-cloudnative-pg-operator}"; do
  code="$(curl -s -o /dev/null -w '%{http_code}' -b "${COOKIE_JAR}" "${DCM_API_URL}/catalog-items/${item}" || true)"
  if [[ "${code}" != "200" ]]; then
    echo "WARN: catalog item ${item} not reachable (HTTP ${code}); related tests may skip" >&2
  fi
done
rm -f "${COOKIE_JAR}"

echo "== playwright test =="
PW_ARGS=(-c e2e/playwright.config.ts)
[[ -n "${PW_GREP}" ]] && PW_ARGS+=(--grep "${PW_GREP}")
if [[ ${#PW_SPECS[@]} -gt 0 ]]; then
  PW_ARGS+=("${PW_SPECS[@]}")
fi
npx playwright test "${PW_ARGS[@]}"

echo "=== Fleet console Playwright E2E OK ==="
