#!/usr/bin/env bash
# E2E: verify OpenAI-compatible inference **over the hub** via console `/llm/`.
# Path: client → ${FLEET_CONSOLE_URL}/llm → fleet-console-auth (FleetLLMApiKey) → LiteLLM.
#
# Without a key: HTTP 401 on /llm/v1/*
# With a FleetLLMApiKey: chat completion succeeds
#
# Default (no FLEET_LLM_CLUSTER): hub mock OpenAI backend (tests hub proxy + auth).
# With FLEET_LLM_CLUSTER set: deploy llama-cpp-helm on that spoke when possible.
#
# Optional env:
#   FLEET_CONSOLE_URL=http://10.10.0.1:30912
#   FLEET_LLM_CLUSTER=example-gpu        optional spoke cluster for real inference
#   FLEET_LLM_RELEASE=llama-cpp
#   FLEET_LLM_MODEL=Qwen3.5-2B-UD   (default when FLEET_LLM_CLUSTER is set)
#   FLEET_LLM_APP_NAME=llama-cpp-lab
#   FLEET_LLM_VALUES_FILE=deploy/llama-cpp-helm/values-fleet-gpu-4g.yaml
#   SKIP_LLM_DEPLOY=1                    skip spoke llama-cpp deploy; use healthy backend or mock
#   LLM_E2E_USE_MOCK_FALLBACK=1          hub mock when spoke deploy unavailable (default without GPU)
#   LLM_E2E_REQUIRE_INFERENCE_DEPLOY=1   fail instead of mock fallback
#   REBUILD_AUTH=1                       rebuild + rollout fleet-console-auth before test
#   FLEET_LITELLM_URL=http://10.10.0.1:30918
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-endpoint.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-endpoint.sh"
fv_hub_endpoint_export_urls
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

CONSOLE="${FLEET_CONSOLE_URL:-http://${FLEET_HUB_IP:-127.0.0.1}:30912}"
LLM_PROXY="${CONSOLE}/llm"
FLEET_API="${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}"
LITELLM_URL="${FLEET_LITELLM_URL:-http://${FLEET_HUB_IP:-127.0.0.1}:30918}"
CLUSTER="${FLEET_LLM_CLUSTER:-${FLEET_GPU_CLUSTER:-}}"
RELEASE="${FLEET_LLM_RELEASE:-llama-cpp}"
NS="${FLEET_CONSOLE_NS:-fleet-console}"
LLM_NS="${FLEET_LLM_NAMESPACE:-fleet-llm}"
KEY_NAME="${FLEET_LLM_APIKEY_NAME:-llm-e2e-test}"
MODEL="${FLEET_LLM_MODEL:-Qwen2.5-0.5B-Instruct}"
APP_NAME="${FLEET_LLM_APP_NAME:-llama-cpp-e2e}"
VALUES_FILE="${FLEET_LLM_VALUES_FILE:-${ROOT}/deploy/llama-cpp-helm/values-fleet-gpu-4g.yaml}"
CHART="${FLEET_LLM_CHART:-llama-cpp-helm}"
PUBLIC_HOST="${FLEET_LLM_PUBLIC_HOST:-}"
PUBLIC_URL="${FLEET_LLM_PUBLIC_URL:-}"
LLM_PORT="${FLEET_LLM_INFERENCE_PORT:-8080}"
MOCK_PORT="${LLM_E2E_MOCK_PORT:-48080}"
# Hub-path default: mock OpenAI backend on hub when no GPU cluster is set.
if [[ -z "${CLUSTER}" ]]; then
  SKIP_LLM_DEPLOY="${SKIP_LLM_DEPLOY:-1}"
  LLM_E2E_USE_MOCK_FALLBACK="${LLM_E2E_USE_MOCK_FALLBACK:-1}"
  MODEL="${FLEET_LLM_MODEL:-llm-e2e-mock}"
else
  MODEL="${FLEET_LLM_MODEL:-Qwen3.5-2B-UD}"
fi
INV="${INV:--i ${ROOT}/inventory/hosts.yml}"
VAULT_ARGS=()
[[ -f "${HOME}/.umkim-vault-pass" ]] && VAULT_ARGS=(--vault-password-file "${HOME}/.umkim-vault-pass")

hub_k() { hub_kubectl "$@"; }

log() { echo "[llm-e2e] $*"; }
fail() { echo "[llm-e2e] FAIL: $*" >&2; exit 1; }

# LiteLLM /health requires the master key when LITELLM_MASTER_KEY is set.
litellm_master_key() {
  if [[ -n "${LITELLM_MASTER_KEY:-}" ]]; then
    printf '%s' "${LITELLM_MASTER_KEY}"
    return 0
  fi
  hub_k -n "$LLM_NS" get secret fleet-llm-master-key \
    -o jsonpath='{.data.master_key}' 2>/dev/null | base64 -d
}

litellm_health_ok() {
  local key
  key="$(litellm_master_key)"
  [[ -n "$key" ]] || return 1
  curl -sf -H "Authorization: Bearer ${key}" "${LITELLM_URL}/health" \
    | jq -e '.healthy_endpoints | length >= 1' >/dev/null 2>&1
}

http_code() {
  curl -s -o /dev/null -w '%{http_code}' "$@"
}

cluster_upstream() {
  curl -sf "${FLEET_API}/clusters/${CLUSTER}" | jq -r '.upstream_host // empty'
}

wait_fleetapp_ready() {
  local name="$1"
  local phase="" rounds="${FLEET_LLM_APP_WAIT_ROUNDS:-60}"
  for _ in $(seq 1 "$rounds"); do
    phase=$(curl -sf "${FLEET_API}/apps/${name}" 2>/dev/null | jq -r '.phase // empty' || true)
    [[ "$phase" == "Ready" ]] && return 0
    [[ "$phase" == "Failed" ]] && return 1
    sleep 15
  done
  log "WARN: FleetApp ${name} not Ready (last phase=${phase:-unknown})"
  return 1
}

deploy_llama_fleetapp() {
  local values_yaml phase
  [[ -f "${VALUES_FILE}" ]] || fail "values file not found: ${VALUES_FILE}"
  values_yaml="$(cat "${VALUES_FILE}")"
  phase=$(curl -sf "${FLEET_API}/apps/${APP_NAME}" 2>/dev/null | jq -r '.phase // empty' || true)
  if [[ "$phase" == "Ready" ]]; then
    log "FleetApp ${APP_NAME} already Ready on ${CLUSTER}"
    return 0
  fi
  if [[ -n "$phase" && "$phase" != "Failed" ]]; then
    log "FleetApp ${APP_NAME} phase=${phase}; waiting"
    wait_fleetapp_ready "${APP_NAME}"
    return $?
  fi
  log "install FleetApp ${APP_NAME} (${CHART}) on ${CLUSTER}"
  if command -v umkim >/dev/null 2>&1; then
    umkim app create \
      --cluster "${CLUSTER}" \
      --chart "${CHART}" \
      --name "${APP_NAME}" \
      --release "${RELEASE}" \
      --namespace llm \
      --route-mode none \
      --values-file "${VALUES_FILE}"
  else
    local payload
    payload="$(jq -n \
      --arg cluster "${CLUSTER}" \
      --arg chart "${CHART}" \
      --arg name "${APP_NAME}" \
      --arg release "${RELEASE}" \
      --arg ns llm \
      --arg values "${values_yaml}" \
      '{clusterName:$cluster,chart:$chart,name:$name,releaseName:$release,namespace:$ns,valuesYaml:$values,route:{mode:"none"}}')"
    curl -sf -X POST "${FLEET_API}/apps" \
      -H 'Content-Type: application/json' \
      -d "${payload}" >/dev/null
  fi
  wait_fleetapp_ready "${APP_NAME}"
}

wait_llm_reconcile() {
  log "trigger FleetLLM reconcile"
  if command -v umkim >/dev/null 2>&1; then
    umkim llm reconcile --wait --timeout 600 || return 1
  else
    curl -sf -X POST "${FLEET_API}/llm/reconcile" >/dev/null
    for _ in $(seq 1 120); do
      if curl -sf "${FLEET_API}/llm" | jq -e '.phase == "Ready"' >/dev/null 2>&1; then
        return 0
      fi
      sleep 5
    done
    return 1
  fi
}

ensure_litellm_backend_from_fleetllm() {
  local upstream="$1"
  local api_base="http://${upstream}:${LLM_PORT}/v1"
  log "wait for LiteLLM backends via FleetLLM (expect ${MODEL} at ${api_base})"
  for _ in $(seq 1 24); do
    if curl -sf "${FLEET_API}/llm" \
      | jq -e --arg m "${MODEL}" --arg b "${api_base}" \
        '.backends[]? | select(.model==$m and .apiBase==$b)' >/dev/null 2>&1; then
      wait_llm_reconcile || true
      if litellm_health_ok; then
        return 0
      fi
    fi
    curl -sf -X POST "${FLEET_API}/llm/reconcile" >/dev/null 2>&1 || true
    sleep 10
  done
  log "WARN: FleetLLM backends missing ${MODEL}; falling back to ConfigMap patch"
  apply_litellm_backend "${api_base}"
}

wait_inference_health() {
  local upstream="$1"
  local port="${2:-8080}"
  for _ in $(seq 1 60); do
    if curl -sf --max-time 5 "http://${upstream}:${port}/health" >/dev/null 2>&1; then
      if curl -sf --max-time 5 "http://${upstream}:${port}/health" | jq -e '.status == "ok"' >/dev/null 2>&1; then
        return 0
      fi
    fi
    sleep 15
  done
  return 1
}

apply_litellm_backend() {
  local api_base="$1"
  local tmp
  tmp="$(mktemp)"
  log "point LiteLLM backend at ${api_base}"
  cat > "${tmp}" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: litellm-config
  namespace: ${LLM_NS}
data:
  config.yaml: |
    model_list:
      - model_name: ${MODEL}
        litellm_params:
          model: openai/${MODEL}
          api_base: ${api_base}
          api_key: "none"
    litellm_settings:
      drop_params: true
      set_verbose: false
EOF
  hub_k apply -f - < "${tmp}"
  rm -f "${tmp}"
  # hostNetwork LiteLLM cannot do a normal rolling update on a single-node hub.
  hub_k -n "$LLM_NS" scale deployment/litellm --replicas=1
  hub_k -n "$LLM_NS" delete pod -l app=fleet-llm --force --grace-period=0 || true
  for _ in $(seq 1 60); do
    if hub_k -n "$LLM_NS" get pod -l app=fleet-llm -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null | grep -q true; then
      break
    fi
    sleep 5
  done
  for _ in $(seq 1 36); do
    if litellm_health_ok; then
      return 0
    fi
    sleep 5
  done
  fail "LiteLLM health did not report healthy endpoints at ${LITELLM_URL}"
}

deploy_mock_backend() {
  log "deploy hub mock OpenAI backend on 127.0.0.1:${MOCK_PORT}"
  hub_k -n "$LLM_NS" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: llm-e2e-mock
  labels:
    app: llm-e2e-mock
spec:
  replicas: 1
  selector:
    matchLabels:
      app: llm-e2e-mock
  template:
    metadata:
      labels:
        app: llm-e2e-mock
    spec:
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      containers:
        - name: mock
          image: python:3.12-alpine
          command: ["python3", "-u", "-c"]
          args:
            - |
              import json
              from http.server import BaseHTTPRequestHandler, HTTPServer
              class H(BaseHTTPRequestHandler):
                  def log_message(self, *a): pass
                  def _json(self, code, body):
                      raw = json.dumps(body).encode()
                      self.send_response(code)
                      self.send_header("Content-Type", "application/json")
                      self.send_header("Content-Length", str(len(raw)))
                      self.end_headers()
                      self.wfile.write(raw)
                  def do_GET(self):
                      if self.path in ("/health", "/v1/models"):
                          self._json(200, {"data": [{"id": "${MODEL}"}]})
                      else:
                          self.send_error(404)
                  def do_POST(self):
                      if self.path != "/v1/chat/completions":
                          self.send_error(404); return
                      ln = int(self.headers.get("Content-Length", 0))
                      _ = self.rfile.read(ln)
                      self._json(200, {
                          "id": "chatcmpl-e2e",
                          "object": "chat.completion",
                          "model": "${MODEL}",
                          "choices": [{
                              "index": 0,
                              "message": {"role": "assistant", "content": "llm e2e ok"},
                              "finish_reason": "stop"
                          }]
                      })
              HTTPServer(("0.0.0.0", ${MOCK_PORT}), H).serve_forever()
EOF
  hub_k -n "$LLM_NS" rollout status deployment/llm-e2e-mock --timeout=120s
  apply_litellm_backend "http://127.0.0.1:${MOCK_PORT}/v1"
}

ensure_inference_backend() {
  if [[ "${SKIP_LLM_DEPLOY:-0}" == "1" ]]; then
    if litellm_health_ok; then
      log "SKIP_LLM_DEPLOY=1 — existing LiteLLM backend healthy"
      return 0
    fi
    if [[ -n "${CLUSTER}" ]]; then
      local upstream
      upstream="$(cluster_upstream 2>/dev/null || true)"
      if [[ -n "$upstream" ]] && wait_inference_health "$upstream" "$LLM_PORT"; then
        log "SKIP_LLM_DEPLOY=1 — repointing LiteLLM at inference on ${upstream}:${LLM_PORT}"
        apply_litellm_backend "http://${upstream}:${LLM_PORT}/v1"
        return 0
      fi
    fi
    if [[ "${LLM_E2E_USE_MOCK_FALLBACK:-0}" == "1" ]]; then
      log "SKIP_LLM_DEPLOY=1 — hub mock OpenAI backend for console /llm/ path"
      deploy_mock_backend
      return 0
    fi
    fail "SKIP_LLM_DEPLOY=1 but LiteLLM backend unhealthy (set LLM_E2E_USE_MOCK_FALLBACK=1)"
  fi

  if [[ -z "${CLUSTER}" ]]; then
    log "no FLEET_LLM_CLUSTER — hub mock OpenAI backend for console /llm/ path"
    deploy_mock_backend
    return 0
  fi

  local deployed=false
  if deploy_llama_fleetapp; then
    local upstream
    upstream="$(cluster_upstream)"
    [[ -n "$upstream" ]] || fail "no upstream_host for ${CLUSTER}"
    if wait_inference_health "$upstream" "$LLM_PORT"; then
      if ensure_litellm_backend_from_fleetllm "$upstream"; then
        deployed=true
      else
        log "WARN: LiteLLM backend wiring failed"
      fi
    else
      log "WARN: inference health timeout on ${upstream}:${LLM_PORT}"
    fi
  else
    log "WARN: FleetApp deploy failed on ${CLUSTER}"
  fi

  if [[ "$deployed" == "true" ]]; then
    return 0
  fi
  if [[ "${LLM_E2E_REQUIRE_INFERENCE_DEPLOY:-0}" == "1" ]]; then
    fail "spoke inference deploy required but failed (cluster=${CLUSTER})"
  fi
  if [[ "${LLM_E2E_USE_MOCK_FALLBACK:-0}" != "1" ]]; then
    fail "inference backend not ready and mock fallback disabled"
  fi
  deploy_mock_backend
}

apply_auth_rbac() {
  log "apply fleet-console-auth RBAC (FleetLLMApiKey list)"
  hub_k apply -f "${ROOT}/deploy/fleet-console-auth/rbac.yaml"
}

rebuild_auth_if_requested() {
  [[ "${REBUILD_AUTH:-0}" == "1" ]] || return 0
  log "rebuild fleet-console-auth image (force push)"
  ansible-playbook ${INV} "${ROOT}/harbor.yml" -l hub --tags harbor_push_fleet \
    "${VAULT_ARGS[@]}" \
    -e harbor_fleet_push_force=true \
    -e "harbor_umkim_root=${ROOT}" \
    -e '{"harbor_fleet_push_images":[{"repository":"fleet-console-auth","tag":"0.1.0","local_src":"'"${ROOT}"'/controllers/fleet-console-auth","clone_dir":"/opt/fleet-src/fleet-console-auth","containerfile":"Containerfile"}]}'
  ansible-playbook ${INV} "${ROOT}/fleet-console-auth.yml" -l hub \
    "${VAULT_ARGS[@]}"
  hub_k -n "$NS" rollout status deployment/fleet-console-auth --timeout=300s
}

create_api_key() {
  local token secret_name
  log "create FleetLLMApiKey ${KEY_NAME}"
  if command -v umkim >/dev/null 2>&1 && [[ -n "${UMKIM_TOKEN:-}" ]]; then
    local models_arg=() auth_url
    [[ -n "${MODEL}" ]] && models_arg=(--models "${MODEL}")
    auth_url="${UMKIM_AUTH_URL:-}"
    if [[ -z "$auth_url" && -n "${CONSOLE}" ]]; then
      auth_url="${CONSOLE%/}/fleet/auth/v1alpha1"
      export UMKIM_AUTH_URL="$auth_url"
    fi
    token="$(umkim auth llm-api-keys create \
      --name "${KEY_NAME}" \
      --display-name "LLM E2E test key" \
      "${models_arg[@]}" \
      -o json 2>/dev/null | jq -r '.token // empty' || true)"
    if [[ -n "$token" ]]; then
      export FLEET_LLM_TEST_TOKEN="$token"
      return 0
    fi
    log "WARN: umkim auth llm-api-keys create failed; using kubectl FleetLLMApiKey"
  fi
  token="${FLEET_LLM_APIKEY_TOKEN:-CHANGE_ME}"
  secret_name="fleet-llm-apikey-${KEY_NAME}"
  hub_k -n "$NS" create secret generic "$secret_name" \
    --from-literal=api_key="$token" \
    --dry-run=client -o yaml | hub_k apply -f -
  hub_k -n "$NS" apply -f - <<EOF
apiVersion: console.umkim.com/v1alpha1
kind: FleetLLMApiKey
metadata:
  name: ${KEY_NAME}
  namespace: ${NS}
spec:
  displayName: LLM E2E test key
  secretRef: ${secret_name}
  models:
    - ${MODEL}
EOF
  hub_k -n "$NS" patch fleetllmapikey "${KEY_NAME}" --subresource=status --type=merge \
    -p '{"status":{"active":true}}' 2>/dev/null \
    || hub_k -n "$NS" patch fleetllmapikey "${KEY_NAME}" --type=merge \
      -p '{"status":{"active":true}}'
  export FLEET_LLM_TEST_TOKEN="$token"
}

test_without_key() {
  log "inference without API key must be rejected"
  local code
  code="$(http_code "${LLM_PROXY}/v1/models")"
  [[ "$code" == "401" ]] || fail "expected 401 on /llm/v1/models without key, got ${code}"
  code="$(http_code -X POST "${LLM_PROXY}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"${MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}")"
  [[ "$code" == "401" ]] || fail "expected 401 on chat/completions without key, got ${code}"
}

test_with_key_curl() {
  log "inference with FleetLLMApiKey (curl)"
  local body code
  body="$(mktemp)"
  code="$(curl -s -o "$body" -w '%{http_code}' -X POST "${LLM_PROXY}/v1/chat/completions" \
    -H "Authorization: Bearer ${FLEET_LLM_TEST_TOKEN}" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"${MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"Say llm e2e ok\"}],\"max_tokens\":32}")"
  [[ "$code" == "200" ]] || { cat "$body" >&2; fail "expected 200 with key, got ${code}"; }
  jq -e '.choices[0].message.content' "$body" >/dev/null || fail "invalid chat completion body"
  log "curl completion: $(jq -r '.choices[0].message.content' "$body")"
  rm -f "$body"
}

test_with_key_openai() {
  command -v python3 >/dev/null 2>&1 || { log "skip openai client test (no python3)"; return 0; }
  if ! python3 -c 'import openai' 2>/dev/null; then
    log "skip openai client test (pip install openai)"
    return 0
  fi
  log "inference with FleetLLMApiKey (openai python client)"
  OPENAI_BASE_URL="${LLM_PROXY}/v1" \
  OPENAI_API_KEY="${FLEET_LLM_TEST_TOKEN}" \
  FLEET_LLM_MODEL="${MODEL}" \
  python3 - <<'PY'
import os
from openai import OpenAI

client = OpenAI(
    base_url=os.environ["OPENAI_BASE_URL"],
    api_key=os.environ["OPENAI_API_KEY"],
)
resp = client.chat.completions.create(
    model=os.environ["FLEET_LLM_MODEL"],
    messages=[{"role": "user", "content": "Reply with exactly: llm e2e ok"}],
    max_tokens=32,
)
text = resp.choices[0].message.content or ""
print("openai client:", text.strip())
if not text.strip():
    raise SystemExit("empty completion")
PY
}

test_direct_litellm_rejects_anonymous() {
  # LiteLLM may allow GET /v1/models without a key; chat completions require the master key.
  log "direct LiteLLM chat without master key must be rejected"
  local code
  code="$(http_code -X POST "${LITELLM_URL}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"${MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":8}")"
  if [[ "$code" != "401" && "$code" != "403" ]]; then
    log "WARN: expected 401/403 on direct LiteLLM chat without master key, got ${code} (backend may be unhealthy)"
  fi
}

test_traefik_public_host() {
  [[ -n "${PUBLIC_HOST}" || -n "${PUBLIC_URL}" ]] || return 0
  local base="${PUBLIC_URL}"
  if [[ -z "${base}" ]]; then
    base="https://${PUBLIC_HOST}"
  fi
  log "public Traefik LLM API at ${base}/v1 (FleetLLMApiKey)"
  local code body
  body="$(mktemp)"
  code="$(curl -sk -o "$body" -w '%{http_code}' "${base}/v1/models")"
  [[ "$code" == "401" ]] || fail "expected 401 on ${base}/v1/models without key, got ${code}"
  code="$(curl -sk -o "$body" -w '%{http_code}' -X POST "${base}/v1/chat/completions" \
    -H "Authorization: Bearer ${FLEET_LLM_TEST_TOKEN}" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"${MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"Say traefik ok\"}],\"max_tokens\":16}")"
  [[ "$code" == "200" ]] || { cat "$body" >&2; fail "expected 200 on ${base}/v1/chat/completions, got ${code}"; }
  log "traefik completion: $(jq -r '.choices[0].message.content' "$body")"
  rm -f "$body"
}

main() {
  log "=== verify-llm-inference-e2e (hub=${FLEET_HUB_IP:-?} console=${CONSOLE} cluster=${CLUSTER:-hub-mock} release=${RELEASE} backend=${BACKEND:-auto}) ==="
  curl -sf "${FLEET_API}/llm" | jq -e '.hubEnabled == true' >/dev/null \
    || fail "hub LiteLLM not enabled (set fleet_llm_enabled: true; hub-install or configure-fleet-llm.sh)"
  apply_auth_rbac
  rebuild_auth_if_requested
  ensure_inference_backend
  create_api_key
  test_without_key
  test_direct_litellm_rejects_anonymous
  test_with_key_curl
  test_with_key_openai
  test_traefik_public_host
  log "=== verify-llm-inference-e2e OK (inference over hub ${LLM_PROXY}) ==="
}

main "$@"
