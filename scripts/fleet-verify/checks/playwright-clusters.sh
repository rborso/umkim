#!/usr/bin/env bash
set -euo pipefail

FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
ROOT="${FV_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
UI_DIR="${ROOT}/ui/dcm-fleet-console"
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/playwright.sh
source "${SCRIPT_DIR}/lib/playwright.sh"

if ! curl -sf --connect-timeout 5 "${FLEET_CONSOLE_URL%/}/" >/dev/null 2>&1; then
  echo "WARN FLEET_CONSOLE_URL unreachable (${FLEET_CONSOLE_URL}), skipping playwright clusters"
  exit 0
fi

export FLEET_CONSOLE_URL FLEET_API_URL="${FLEET_CONSOLE_URL}/fleet/api/v1alpha1"

if fv_playwright_test "${UI_DIR}" e2e/clusters.spec.ts; then
  echo "OK playwright clusters.spec.ts"
  exit 0
fi

echo "FAIL playwright clusters.spec.ts" >&2
exit 1
