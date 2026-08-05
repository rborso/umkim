#!/usr/bin/env bash
set -euo pipefail

FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
API="${FLEET_CONSOLE_URL%/}/api/v1alpha1"

code="$(curl -s -o /dev/null -w '%{http_code}' "${API}/catalog-items" || true)"
if [[ "${code}" == "401" || "${code}" == "403" ]]; then
  echo "OK auth gate: unauthenticated catalog-items returned ${code}"
  exit 0
fi

echo "FAIL auth gate: expected 401/403 for unauthenticated catalog-items, got ${code}" >&2
exit 1
