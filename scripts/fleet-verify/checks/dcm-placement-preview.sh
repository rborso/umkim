#!/usr/bin/env bash
set -euo pipefail

DCM_API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"

body='{"mode":"deploy","service_type":"container","catalog_item_id":"demo-webapp"}'
resp="$(curl -sf -X POST "${DCM_API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d "${body}")"

if echo "${resp}" | python3 -c "
import json, sys
d = json.load(sys.stdin)
deployable = d.get('deployable')
count = d.get('eligible_count', 0)
if deployable is None and 'eligible_count' not in d:
    raise SystemExit(1)
if not isinstance(count, (int, float)) or count < 0:
    raise SystemExit(1)
"; then
  deployable="$(echo "${resp}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('deployable', False))")"
  count="$(echo "${resp}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('eligible_count', 0))")"
  echo "OK DCM placement-preview (deployable=${deployable}, eligible_count=${count})"
  exit 0
fi

echo "FAIL DCM placement-preview invalid response" >&2
exit 1
