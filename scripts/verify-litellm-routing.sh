#!/usr/bin/env bash
# Verify hub LiteLLM load-balancing and sticky routing across llama-cpp spokes.
# Reads x-litellm-model-api-base from the proxy (requires hub NodePort or FLEET_LITELLM_URL).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh" 2>/dev/null || true

LITELLM_URL="${FLEET_LITELLM_URL:-http://${FLEET_HUB_IP:-127.0.0.1}:30918}"
LLM_NS="${FLEET_LLM_NAMESPACE:-fleet-llm}"
MASTER_KEY="${FLEET_LLM_MASTER_KEY:-}"

if [[ -z "${MASTER_KEY}" ]]; then
  MASTER_KEY="$(hub_kubectl -n "$LLM_NS" get secret fleet-llm-master-key -o jsonpath='{.data.master_key}' 2>/dev/null | base64 -d || true)"
fi
[[ -n "${MASTER_KEY}" ]] || { echo "FAIL: set FLEET_LLM_MASTER_KEY or run on hub with kubectl"; exit 1; }

LONG="You are verifying LiteLLM routing. Summarize in 2 sentences. Context: $(printf '%s ' {1..200})"
RUN_ID="lb-$(date +%s)"

chat() {
  local user="$1"
  local session="${2:-}"
  local extra=()
  [[ -n "$session" ]] && extra+=(-H "x-litellm-session-id: $session")
  curl -sS -m 180 "${extra[@]}" \
    -H "Authorization: Bearer ${MASTER_KEY}" \
    -H "Content-Type: application/json" \
    -D /tmp/litellm-hdr.$$ \
    -o /tmp/litellm-body.$$ \
    "${LITELLM_URL}/v1/chat/completions" \
    -d "$(jq -nc --arg u "$user" --arg c "$LONG" --arg s "$session" \
      '{model:"Qwen3.5-2B-UD",user:$u,messages:[{role:"user",content:$c}],max_tokens:48}
       + (if $s != "" then {metadata:{session_id:$s}} else {} end)')" \
    >/dev/null
  grep -i '^x-litellm-model-api-base:' /tmp/litellm-hdr.$$ | awk '{print $2}' | tr -d '\r'
}

echo "=== Load balance (12 users, long prompt) ==="
declare -A LB
for i in $(seq 0 11); do
  b="$(chat "${RUN_ID}-user-$i")"
  echo "  ${RUN_ID}-user-$i: $b"
  LB["$b"]=$(( ${LB["$b"]:-0} + 1 ))
done
echo "Counts: ${LB[*]}"
lb_ok=0
[[ ${#LB[@]} -ge 2 ]] && lb_ok=1
echo "LB_OK=$lb_ok"

echo "=== Session sticky (8 calls; session_affinity + x-litellm-session-id) ==="
sess="sess-verify-${RUN_ID}"
declare -A SE
for i in $(seq 1 8); do
  b="$(chat "session-user-${RUN_ID}" "$sess")"
  echo "  req $i: $b"
  SE["$b"]=$(( ${SE["$b"]:-0} + 1 ))
done
session_ok=0
[[ ${#SE[@]} -eq 1 ]] && session_ok=1
echo "SESSION_STICKY_OK=$session_ok"

if [[ "$lb_ok" == 1 && "$session_ok" == 1 ]]; then
  echo "=== verify-litellm-routing OK ==="
  exit 0
fi
echo "=== verify-litellm-routing FAILED (lb=$lb_ok session=$session_ok) ==="
exit 1
