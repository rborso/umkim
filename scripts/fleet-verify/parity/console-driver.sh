#!/usr/bin/env bash
# Console (Playwright) driver for parity operations.
set -euo pipefail

OP="${1:-}"
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
UI="${ROOT}/ui/dcm-fleet-console"
CONSOLE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=scripts/fleet-verify/lib/playwright.sh
source "${SCRIPT_DIR}/lib/playwright.sh"

export FLEET_CONSOLE_URL="${CONSOLE}"
export DCM_API_URL="${CONSOLE}/api/v1alpha1"
export FLEET_API_URL="${CONSOLE}/fleet/api/v1alpha1"
export FV_RUN_ID="${FV_RUN_ID:-$(date +%Y%m%d%H%M%S)}"

spec=""
case "${OP}" in
  parity.site.create|site.create) spec="e2e/parity/site-create.spec.ts" ;;
  parity.policy.crud|policy.crud) spec="e2e/parity/policy-crud.spec.ts" ;;
  parity.deploy.submit|deploy.submit) spec="e2e/parity/deploy-submit.spec.ts" ;;
  *) echo "unknown console parity op: ${OP}" >&2; exit 1 ;;
esac

if ! curl -sf "${CONSOLE}/fleet/login" >/dev/null 2>&1; then
  echo "SKIP console unreachable at ${CONSOLE}"
  exit 0
fi

if ! fv_playwright_bin "${UI}" >/dev/null 2>&1; then
  echo "SKIP playwright unavailable (install ui deps or npx)"
  exit 0
fi

if fv_playwright_test "${UI}" "${spec}" --reporter=line; then
  exit 0
fi
rc=$?
if [[ "$rc" -eq 0 ]]; then
  exit 0
fi
echo "FAIL playwright ${spec}" >&2
exit "$rc"
