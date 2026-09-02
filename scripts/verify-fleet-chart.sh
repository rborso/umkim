#!/usr/bin/env bash
# Confirm the hub chart library lists seeded Harbor charts.
set -euo pipefail

API="${FLEET_SITE_API:-http://127.0.0.1:18080/api/v1alpha1}"
HDR=(-H 'Accept: application/json' -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-flow' -H 'X-Fleet-DC-Admin: true')

echo "=== verify-fleet-chart ==="
if ! curl -sfS "${HDR[@]}" "${API}/charts" -o /tmp/fleet-charts.json; then
  echo "FAIL GET /charts" >&2
  exit 1
fi
python3 - <<'PY'
import json, sys
data = json.load(open("/tmp/fleet-charts.json"))
charts = data.get("charts") or []
names = {c.get("name") for c in charts}
print("library charts:", ", ".join(sorted(n for n in names if n)) or "(none)")
if "whoami" not in names:
    print("FAIL: whoami is not in the chart library", file=sys.stderr)
    sys.exit(1)
print("OK whoami is in the chart library")
PY

if [[ -n "${FLEET_CHART_SMOKE_PUBLISH:-}" ]]; then
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  echo "=== optional publish ${FLEET_CHART_SMOKE_PUBLISH} ==="
  "${ROOT}/scripts/fleet-chart.sh" publish "${FLEET_CHART_SMOKE_PUBLISH}"
  curl -sfS "${HDR[@]}" "${API}/charts" -o /tmp/fleet-charts.json
  python3 - <<'PY'
import json, os
data = json.load(open("/tmp/fleet-charts.json"))
print("library after publish:", ", ".join(sorted(c.get("name") or "" for c in (data.get("charts") or []))))
PY
fi
