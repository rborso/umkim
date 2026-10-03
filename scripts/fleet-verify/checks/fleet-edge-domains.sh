#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

body="$(curl -sf "${FLEET_API}/edge/domains")"
if echo "${body}" | python3 -c "
import json, sys
json.load(sys.stdin)
"; then
  echo "OK fleet edge/domains"
  exit 0
fi

echo "FAIL fleet edge/domains invalid JSON" >&2
exit 1
