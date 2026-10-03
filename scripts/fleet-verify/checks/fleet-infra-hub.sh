#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

body="$(curl -sf "${FLEET_API}/infrastructure/hub")"
if echo "${body}" | python3 -c "
import json, sys
d = json.load(sys.stdin)
ok = bool(d.get('host_name') or d.get('management_site_id') or d.get('managementSiteId'))
sys.exit(0 if ok else 1)
"; then
  echo "OK fleet infrastructure/hub"
  exit 0
fi

echo "FAIL fleet infrastructure/hub missing host or management site" >&2
exit 1
