#!/usr/bin/env bash
# Smoke test: whoami-operator catalog item via DCM + fleet-olm-subscription-sp (async create).
set -euo pipefail

DCM_API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
OCM_CLUSTER="${OCM_CLUSTER:-${FLEET_VERIFY_CLUSTER:-virt02}}"
SP_PROVIDER="${SP_PROVIDER:-olm-${OCM_CLUSTER}}"
CATALOG_ITEM="${CATALOG_ITEM:-whoami-operator}"
SUB_NAME="${SUB_NAME:-whoami-operator}"
TARGET_NS="${TARGET_NS:-operators}"
KUBECONFIG_SPOKE="${KUBECONFIG_SPOKE:-}"

echo "== 1. Provider ${SP_PROVIDER} ready =="
PROVIDERS="$(curl -sf "${DCM_API}/providers")"
echo "${PROVIDERS}" | grep -q "\"name\":\"${SP_PROVIDER}\"" || {
  echo "SKIP: provider ${SP_PROVIDER} not installed (install olm provider on ${OCM_CLUSTER} first)"
  exit 0
}

echo "== 2. Create whoami-operator instance =="
INSTANCE_JSON="$(cat <<EOF
{
  "api_version": "v1alpha1",
  "display_name": "whoami-operator-${OCM_CLUSTER}",
  "spec": {
    "catalog_item_id": "${CATALOG_ITEM}",
    "user_values": [
      { "path": "metadata.name", "value": "${SUB_NAME}" },
      { "path": "operatorGroup.targetNamespace", "value": "${TARGET_NS}" }
    ]
  }
}
EOF
)"
RESP="$(curl -sf -m 120 -X POST "${DCM_API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "${INSTANCE_JSON}")"
echo "${RESP}" | head -c 600
echo
RESOURCE_ID="$(echo "${RESP}" | sed -n 's/.*"resource_id":"\([^"]*\)".*/\1/p' | head -1)"
INSTANCE_UID="$(echo "${RESP}" | sed -n 's/.*"uid":"\([^"]*\)".*/\1/p' | head -1)"
if [[ -z "${RESOURCE_ID}" ]]; then
  RESOURCE_ID="$(echo "${RESP}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)"
fi
API_REF="${INSTANCE_UID:-${RESOURCE_ID}}"
echo "resource_id=${RESOURCE_ID:-unknown} uid=${INSTANCE_UID:-—} api_ref=${API_REF}"
[[ -n "${API_REF}" ]] || exit 1

echo "== 3. Instance visible in API =="
curl -sf "${DCM_API}/catalog-item-instances/${API_REF}" >/dev/null

if [[ -n "${KUBECONFIG_SPOKE}" ]]; then
  export KUBECONFIG="${KUBECONFIG_SPOKE}"
  echo "== 4. Subscription on spoke =="
  for _ in $(seq 1 30); do
    if kubectl -n "${TARGET_NS}" get subscription "${SUB_NAME}" 2>/dev/null; then
      break
    fi
    sleep 5
  done
  kubectl -n "${TARGET_NS}" get subscription "${SUB_NAME}" -o wide
fi

if [[ -n "${VERIFY_DELETE:-}" && -n "${API_REF}" ]]; then
  echo "== 5. DELETE instance =="
  curl -sf -X DELETE "${DCM_API}/catalog-item-instances/${API_REF}"
  sleep 10
  code="$(curl -s -o /dev/null -w '%{http_code}' "${DCM_API}/catalog-item-instances/${API_REF}")"
  [[ "${code}" == "404" ]] || { echo "instance still exists: HTTP ${code}"; exit 1; }
fi

echo "OK"
