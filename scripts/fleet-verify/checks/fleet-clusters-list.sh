#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

count="$(curl -sf "${FLEET_API}/clusters" | python3 -c "
import json, sys
d = json.load(sys.stdin)
clusters = d.get('clusters', d if isinstance(d, list) else [])
print(len(clusters))
")"

if [[ "${count}" -ge 0 ]]; then
  echo "OK fleet clusters list (count=${count})"
  exit 0
fi

echo "FAIL fleet clusters list invalid count ${count}" >&2
exit 1
