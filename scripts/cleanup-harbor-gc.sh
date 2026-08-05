#!/usr/bin/env bash
# Trigger Harbor garbage collection after untagged artifact deletion.
set -euo pipefail

HARBOR="${HARBOR_URL:-https://10.10.0.1:30443}"
USER="${HARBOR_USER:-admin}"
PASS="${HARBOR_PASS:-FleetHarbor-ChangeMe}"

log() { echo "[harbor-gc] $*"; }

# Harbor 2.x manual GC
code=$(curl -sk -o /tmp/harbor-gc.json -w '%{http_code}' -u "$USER:$PASS" \
  -X POST "$HARBOR/api/v2.0/system/gc" \
  -H 'Content-Type: application/json' \
  -d '{"schedule":{"type":"Manual"}}' || true)

if [[ "$code" != "201" && "$code" != "200" && "$code" != "202" ]]; then
  log "POST /system/gc returned ${code}; trying schedule endpoint"
  curl -sk -u "$USER:$PASS" -X PUT "$HARBOR/api/v2.0/system/gc/schedule" \
    -H 'Content-Type: application/json' \
    -d '{"schedule":{"type":"Manual","cron":"0 0 * * *"}}' >/dev/null || true
  curl -sk -u "$USER:$PASS" -X POST "$HARBOR/api/v2.0/system/gc" \
    -H 'Content-Type: application/json' \
    -d '{"schedule":{"type":"Manual"}}' >/dev/null || true
fi

log "waiting for GC (up to 120s)"
for _ in $(seq 1 24); do
  status=$(curl -sk -u "$USER:$PASS" "$HARBOR/api/v2.0/system/gc" \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('status',''))" 2>/dev/null || echo "")
  log "status=${status:-unknown}"
  [[ "$status" == "Success" || "$status" == "succeed" ]] && break
  [[ "$status" == "Error" || "$status" == "failed" ]] && break
  sleep 5
done

curl -sk -u "$USER:$PASS" "$HARBOR/api/v2.0/system/gc" | python3 -m json.tool 2>/dev/null || true
log "done"
