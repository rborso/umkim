#!/usr/bin/env bash
set -euo pipefail

DCM_API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
FLEET_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
CONSOLE_API="${FLEET_CONSOLE_URL%/}/api/v1alpha1"

dcm_health() {
  local component="$1"
  if curl -sf --connect-timeout 5 "${CONSOLE_API}/health/${component}" >/dev/null 2>&1; then
    return 0
  fi
  if curl -sf --connect-timeout 5 "${DCM_API}/health/${component}" >/dev/null 2>&1; then
    return 0
  fi
  local base="${DCM_API%/api/v1alpha1}"
  curl -sf --connect-timeout 5 "${base}/api/v1alpha1/health/${component}" >/dev/null 2>&1
}

failed=0
for component in providers catalog policies placement; do
  if dcm_health "${component}"; then
    echo "OK DCM health/${component}"
  else
    echo "FAIL DCM health/${component}" >&2
    failed=1
  fi
done

exit "${failed}"
