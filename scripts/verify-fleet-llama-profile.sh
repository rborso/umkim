#!/usr/bin/env bash
# FleetLlamaProfile CRUD, quick verify, extended chat, cleanup.
#
#   FLEET_LLM_CLUSTER=lab-gpu-0 ./scripts/verify-fleet-llama-profile.sh
set -euo pipefail

API="${FLEET_SITE_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_LLM_CLUSTER:-lab-gpu-0}"
PROFILE="${FLEET_LLM_PROFILE:-qwen35-2b-ud-4g}"
RUN_ID="${FV_RUN_ID:-$(date +%s)}"
TEMP_PROFILE="llama-prof-verify-${RUN_ID}"

HDR=(-H 'Content-Type: application/json' -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-verify' -H 'X-Fleet-DC-Admin: true')

log() { echo "[llama-profile] $*"; }

wait_test_phase() {
  local id="$1" want="$2"
  local phase=""
  for _ in $(seq 1 180); do
    curl -sf "${HDR[@]}" "${API}/llama-profiles/tests/${id}" -o /tmp/llama-prof-test.json
    phase=$(jq -r '.phase // empty' /tmp/llama-prof-test.json)
    log "test ${id} phase=${phase}"
    if [[ "$phase" == "$want" || "$phase" == "Failed" ]]; then
      break
    fi
    sleep 10
  done
  [[ "$phase" == "$want" ]]
}

log "=== verify-fleet-llama-profile cluster=${CLUSTER} ==="

curl -sf "${HDR[@]}" "${API}/llama-profiles" | jq -e --arg p "$PROFILE" '.profiles[]|select(.name==$p)' >/dev/null

http=$(curl -sS -o /tmp/llama-prof-create.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/llama-profiles" \
  -d "$(jq -n --arg id "$TEMP_PROFILE" --arg desc 'verify temp' '{id:$id,description:$desc,profileYaml:"model: verify-temp\n"}')")
if [[ "$http" != "201" ]]; then
  log "FAIL create temp profile HTTP ${http}"
  cat /tmp/llama-prof-create.json
  exit 1
fi

http=$(curl -sS -o /tmp/llama-prof-quick.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/llama-profiles/${PROFILE}/verify" \
  -d "$(jq -n --arg c "$CLUSTER" '{clusterName:$c,mode:"quick",wait:true}')")
if [[ "$http" != "200" && "$http" != "202" ]]; then
  log "FAIL quick verify HTTP ${http}"
  cat /tmp/llama-prof-quick.json
  exit 1
fi
quick_id=$(jq -r '.id // empty' /tmp/llama-prof-quick.json)
if [[ -n "$quick_id" ]]; then
  wait_test_phase "$quick_id" "Succeeded" || { log "FAIL quick verify"; cat /tmp/llama-prof-test.json; exit 1; }
fi

http=$(curl -sS -o /tmp/llama-prof-ext.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/llama-profiles/${PROFILE}/verify" \
  -d "$(jq -n --arg c "$CLUSTER" '{clusterName:$c,mode:"extended",wait:false}')")
if [[ "$http" != "200" && "$http" != "202" ]]; then
  log "FAIL extended verify HTTP ${http}"
  cat /tmp/llama-prof-ext.json
  exit 1
fi
ext_id=$(jq -r '.id // empty' /tmp/llama-prof-ext.json)
wait_test_phase "$ext_id" "Ready" || { log "FAIL extended not Ready"; cat /tmp/llama-prof-test.json; exit 1; }

http=$(curl -sS -o /tmp/llama-prof-chat.json -w '%{http_code}' "${HDR[@]}" -X POST "${API}/llama-profiles/tests/${ext_id}/chat/completions" \
  -d '{"messages":[{"role":"user","content":"Say hi in one word."}],"max_tokens":8}')
if [[ "$http" != "200" ]]; then
  log "FAIL chat HTTP ${http}"
  cat /tmp/llama-prof-chat.json
  exit 1
fi

http=$(curl -sS -o /dev/null -w '%{http_code}' "${HDR[@]}" -X DELETE "${API}/llama-profiles/tests/${ext_id}")
if [[ "$http" != "204" && "$http" != "200" ]]; then
  log "FAIL end test HTTP ${http}"
  exit 1
fi

http=$(curl -sS -o /dev/null -w '%{http_code}' "${HDR[@]}" -X DELETE "${API}/llama-profiles/${TEMP_PROFILE}")
if [[ "$http" != "204" && "$http" != "200" ]]; then
  log "FAIL delete temp profile HTTP ${http}"
  exit 1
fi

log "OK"
