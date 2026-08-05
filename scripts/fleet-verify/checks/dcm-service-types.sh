#!/usr/bin/env bash
set -euo pipefail

DCM_API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"

count="$(curl -sf "${DCM_API}/service-types" | python3 -c "
import json, sys
d = json.load(sys.stdin)
items = d if isinstance(d, list) else d.get('service_types', d.get('service-types', d.get('items', [])))
print(len(items))
")"

if [[ "${count}" -ge 0 ]]; then
  echo "OK DCM service-types list (count=${count})"
  exit 0
fi

echo "FAIL DCM service-types list" >&2
exit 1
