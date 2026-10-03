#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

count="$(curl -sf "${FLEET_API}/sites" | python3 -c "
import json, sys
d = json.load(sys.stdin)
sites = d.get('sites', d if isinstance(d, list) else [])
print(len(sites))
")"

if [[ "${count}" -ge 0 ]]; then
  echo "OK fleet sites list (count=${count})"
  exit 0
fi

echo "FAIL fleet sites list invalid count ${count}" >&2
exit 1
