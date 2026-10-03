#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${FLEET_VIEWER_USER:-}" || -z "${FLEET_VIEWER_PASSWORD:-}" ]]; then
  echo "SKIP playwright admin/viewer.spec.ts (FLEET_VIEWER_USER unset)"
  exit 0
fi

fv_root="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/playwright.sh
source "${fv_root}/scripts/fleet-verify/lib/playwright.sh"

UI="${fv_root}/ui/umkim-console"
if fv_playwright_test "${UI}" e2e/admin/viewer.spec.ts --reporter=line; then
  echo "OK playwright admin/viewer.spec.ts"
else
  echo "FAIL playwright admin/viewer.spec.ts" >&2
  exit 1
fi
