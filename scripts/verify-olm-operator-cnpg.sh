#!/usr/bin/env bash
# Acceptance checks for CloudNativePG operator via DCM + fleet-olm-subscription-sp.
set -euo pipefail

DCM_API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
OCM_CLUSTER="${OCM_CLUSTER:-virt02}"
SP_PROVIDER="${SP_PROVIDER:-olm-virt02}"
CATALOG_ITEM="${CATALOG_ITEM:-cloudnative-pg-operator}"
SUB_NAME="${SUB_NAME:-cloudnative-pg}"
TARGET_NS="${TARGET_NS:-operators}"
KUBECONFIG_SPOKE="${KUBECONFIG_SPOKE:-}"

echo "== 1. Provider ${SP_PROVIDER} ready =="
PROVIDERS="$(curl -sf "${DCM_API}/providers")"
echo "${PROVIDERS}" | grep -q "\"name\":\"${SP_PROVIDER}\"" || {
  echo "provider ${SP_PROVIDER} not found"
  exit 1
}
echo "${PROVIDERS}" | grep -qE '"health_status":"ready"|"status":"ready"' || echo "WARN: provider may not be ready yet"

echo "== 2. Create catalog-item-instance =="
INSTANCE_JSON="$(cat <<EOF
{
  "api_version": "v1alpha1",
  "display_name": "cnpg-operator-${OCM_CLUSTER}",
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
RESP="$(curl -sf -X POST "${DCM_API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "${INSTANCE_JSON}")"
echo "${RESP}" | head -c 500
echo
RESOURCE_ID="$(echo "${RESP}" | sed -n 's/.*"resource_id":"\([^"]*\)".*/\1/p' | head -1)"
if [[ -z "${RESOURCE_ID}" ]]; then
  RESOURCE_ID="$(echo "${RESP}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)"
fi
echo "resource_id=${RESOURCE_ID:-unknown}"

echo "== 3. Wait for placement (optional) =="
sleep 10

if [[ -n "${KUBECONFIG_SPOKE}" ]]; then
  export KUBECONFIG="${KUBECONFIG_SPOKE}"
  echo "== 4. OLM Subscription + CSV on spoke (${OCM_CLUSTER}) =="
  kubectl -n "${TARGET_NS}" get subscription,csv 2>/dev/null || kubectl -n "${TARGET_NS}" get subscription,csv
  CSV_PHASE="$(kubectl -n "${TARGET_NS}" get csv -o jsonpath='{.items[?(@.spec.displayName)].status.phase}' 2>/dev/null || true)"
  if [[ -n "${CSV_PHASE}" ]] && [[ "${CSV_PHASE}" != *Succeeded* ]]; then
    echo "WARN: CSV phase not Succeeded yet: ${CSV_PHASE}"
  fi
else
  echo "== 4. Skipped spoke kubectl (set KUBECONFIG_SPOKE) =="
fi

echo "== 5. Platform OLM not duplicated =="
if [[ -n "${KUBECONFIG_SPOKE}" ]]; then
  MW_COUNT="$(kubectl get manifestwork -A 2>/dev/null | grep -c phase4-platform-bootstrap || true)"
  echo "phase4-platform-bootstrap manifestworks visible: ${MW_COUNT}"
fi

if [[ -n "${VERIFY_DELETE:-}" && -n "${RESOURCE_ID:-}" ]]; then
  echo "== 6. DELETE catalog-item-instance and check subscription =="
  curl -sf -X DELETE "${DCM_API}/catalog-item-instances/${RESOURCE_ID}"
  sleep 15
  if [[ -n "${KUBECONFIG_SPOKE}" ]]; then
    if kubectl -n "${TARGET_NS}" get subscription "${SUB_NAME}" 2>/dev/null; then
      echo "WARN: Subscription ${SUB_NAME} still present after DELETE — patch fleet-olm-subscription-sp uninstall"
      exit 1
    fi
    echo "Subscription removed after DELETE"
  else
    code="$(curl -s -o /dev/null -w '%{http_code}' "${DCM_API}/catalog-item-instances/${RESOURCE_ID}")"
    [[ "${code}" == "404" ]] || { echo "instance still exists: HTTP ${code}"; exit 1; }
  fi
fi

echo "OK"
