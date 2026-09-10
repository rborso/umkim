#!/usr/bin/env bash
set -euo pipefail
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
UI="${ROOT}/ui/umkim-console"
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/playwright.sh
source "${SCRIPT_DIR}/lib/playwright.sh"
CONSOLE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
export FLEET_CONSOLE_URL="${CONSOLE}" FLEET_API_URL="${CONSOLE}/fleet/api/v1alpha1"
curl -sf "${CONSOLE}/fleet/login" >/dev/null || { echo "SKIP console unreachable"; exit 0; }
[[ -d "${UI}/node_modules/@playwright" ]] || (cd "${UI}" && npm ci --silent 2>/dev/null || true)
fv_playwright_test "${UI}" e2e/cluster-detail.spec.ts --reporter=line
