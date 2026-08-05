#!/usr/bin/env bash
set -euo pipefail
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
UI="${ROOT}/ui/dcm-fleet-console"
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/playwright.sh
source "${SCRIPT_DIR}/lib/playwright.sh"
CONSOLE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
export FLEET_CONSOLE_URL="${CONSOLE}" DCM_API_URL="${CONSOLE}/api/v1alpha1" FLEET_API_URL="${CONSOLE}/fleet/api/v1alpha1"
curl -sf "${CONSOLE}/fleet/login" >/dev/null || { echo "SKIP console unreachable"; exit 0; }
[[ -d "${UI}/node_modules/@playwright" ]] || (cd "${UI}" && npm ci --silent 2>/dev/null || true)
if fv_playwright_test "${UI}" e2e/admin/orgs.spec.ts --reporter=line; then
  echo "OK playwright admin/orgs.spec.ts"
else
  echo "FAIL playwright admin/orgs.spec.ts" >&2
  exit 1
fi
