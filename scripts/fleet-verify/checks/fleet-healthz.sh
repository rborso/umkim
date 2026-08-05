#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
BASE="${FLEET_API%/api/v1alpha1}"

if curl -sf "${BASE}/healthz" >/dev/null; then
  echo "OK fleet-site /healthz"
  exit 0
fi

echo "FAIL fleet-site /healthz unreachable at ${BASE}/healthz" >&2
exit 1
