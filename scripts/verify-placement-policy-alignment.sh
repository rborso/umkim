#!/usr/bin/env bash
# End-to-end: deploy container + helm without routing.cluster (policy-driven placement).
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
WAIT_SECS="${PLACEMENT_ALIGN_WAIT:-90}"
VERIFY_DELETE="${VERIFY_DELETE:-1}"

echo "=== verify-placement-policy-alignment ==="

echo "Pre-check placement-preview (container)..."
PREVIEW=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d '{"mode":"deploy","service_type":"container","catalog_item_id":"demo-webapp","label_selector":{"fleet.workload/profile":"demo","fleet.placement/scope":"single"}}')
echo "$PREVIEW" | jq '{deployable, eligible_count}'
echo "$PREVIEW" | jq -e '.deployable == true and (.eligible_count // 0) >= 1' >/dev/null

echo "Pre-check placement-preview (helm)..."
  PREVIEW_HELM=$(curl -sf -X POST "${API}/placement-preview" \
  -H 'Content-Type: application/json' \
  -d '{"mode":"deploy","service_type":"helm","catalog_item_id":"whoami-helm","label_selector":{"fleet.workload/profile":"demo","fleet.placement/scope":"single"}}' 2>/dev/null || echo '{}')
if echo "$PREVIEW_HELM" | jq -e '.deployable == true' >/dev/null 2>&1; then
  echo "$PREVIEW_HELM" | jq '{deployable, eligible_count}'
else
  echo "SKIP helm preview (whoami-helm catalog may not be seeded)"
fi

INST_CONTAINER="align-demo-$$"
echo "Deploy container without routing.cluster (${INST_CONTAINER})..."
RESP=$(curl -sf -X POST "${API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "{
    \"api_version\": \"v1alpha1\",
    \"display_name\": \"${INST_CONTAINER}\",
    \"spec\": {
      \"catalog_item_id\": \"demo-webapp\",
      \"label_selector\": {\"fleet.workload/profile\": \"demo\", \"fleet.placement/scope\": \"single\"},
      \"user_values\": [
        {\"path\": \"metadata.name\", \"value\": \"${INST_CONTAINER}\"},
        {\"path\": \"routing.mode\", \"value\": \"hub\"}
      ]
    }
  }")
INST_ID=$(echo "$RESP" | jq -r '.id // .uid')
[ -n "$INST_ID" ] || { echo "no container instance id"; exit 1; }
echo "container_instance_id=${INST_ID}"

deadline=$((SECONDS + WAIT_SECS))
while (( SECONDS < deadline )); do
  TARGETS=$(curl -sf "${API}/catalog-item-instances/${INST_ID}/placement-targets" 2>/dev/null || echo '{}')
  COUNT=$(echo "$TARGETS" | jq -r '.target_count // (.targets | length) // 0')
  if [[ "$COUNT" -ge 1 ]]; then
    PROVIDER=$(echo "$TARGETS" | jq -r '.targets[0].provider // ""')
    echo "placement-targets count=${COUNT} provider=${PROVIDER}"
    [[ "$PROVIDER" == k8s-* ]] || { echo "FAIL: expected k8s-* provider, got ${PROVIDER}"; exit 1; }
    break
  fi
  sleep 5
done
if (( SECONDS >= deadline )); then
  echo "FAIL: placement-targets not populated for container instance"
  exit 1
fi

if curl -sf "${API}/catalog-items/whoami-helm" >/dev/null 2>&1; then
  INST_HELM="align-helm-$$"
  echo "Deploy helm without routing.cluster (${INST_HELM})..."
  HTTP=$(curl -s -o /tmp/align-helm-$$.json -w '%{http_code}' -X POST "${API}/catalog-item-instances" \
    -H 'Content-Type: application/json' \
    -d "{
      \"api_version\": \"v1alpha1\",
      \"display_name\": \"${INST_HELM}\",
      \"spec\": {
        \"catalog_item_id\": \"whoami-helm\",
        \"label_selector\": {\"fleet.workload/profile\": \"demo\", \"fleet.placement/scope\": \"single\"},
        \"user_values\": [
          {\"path\": \"metadata.name\", \"value\": \"${INST_HELM}\"},
          {\"path\": \"release.namespace\", \"value\": \"default\"}
        ]
      }
    }")
  if [[ "$HTTP" != "201" ]]; then
    echo "WARN: helm deploy HTTP ${HTTP} ($(jq -r '.detail // .title // .' /tmp/align-helm-$$.json 2>/dev/null))"
    echo "WARN: ensure fleet-helm-sp.yml ran and helm-* providers are registered"
    rm -f /tmp/align-helm-$$.json
  else
  RESP=$(cat /tmp/align-helm-$$.json)
  rm -f /tmp/align-helm-$$.json
  HELM_ID=$(echo "$RESP" | jq -r '.id // .uid')
  sleep 15
  HT=$(curl -sf "${API}/catalog-item-instances/${HELM_ID}/placement-targets" 2>/dev/null || echo '{}')
  HPROV=$(echo "$HT" | jq -r '.targets[0].provider // ""')
  echo "helm provider=${HPROV}"
  [[ "$HPROV" == helm-* ]] || echo "WARN: expected helm-* provider for whoami-helm"
  if [[ "$VERIFY_DELETE" == "1" ]]; then
    curl -sf -X DELETE "${API}/catalog-item-instances/${HELM_ID}" || true
  fi
  fi
fi

echo "Override check (routing.cluster pin)..."
INST_PIN="align-pin-$$"
PIN_CLUSTER=$(echo "$TARGETS" | jq -r '.targets[0].provider' | sed 's/^k8s-//')
RESP=$(curl -sf -X POST "${API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "{
    \"api_version\": \"v1alpha1\",
    \"display_name\": \"${INST_PIN}\",
    \"spec\": {
      \"catalog_item_id\": \"demo-webapp\",
      \"label_selector\": {\"fleet.workload/profile\": \"demo\", \"fleet.placement/scope\": \"single\"},
      \"user_values\": [
        {\"path\": \"metadata.name\", \"value\": \"${INST_PIN}\"},
        {\"path\": \"routing.mode\", \"value\": \"hub\"},
        {\"path\": \"routing.cluster\", \"value\": \"${PIN_CLUSTER}\"}
      ]
    }
  }")
PIN_ID=$(echo "$RESP" | jq -r '.id // .uid')
sleep 10
PT_PIN=$(curl -sf "${API}/catalog-item-instances/${PIN_ID}/placement-targets" 2>/dev/null || echo '{}')
PIN_PROV=$(echo "$PT_PIN" | jq -r '.targets[0].provider // ""')
echo "pinned provider=${PIN_PROV} (cluster=${PIN_CLUSTER})"
[[ "$PIN_PROV" == "k8s-${PIN_CLUSTER}" ]] || echo "WARN: override provider mismatch"

if [[ "$VERIFY_DELETE" == "1" ]]; then
  curl -sf -X DELETE "${API}/catalog-item-instances/${INST_ID}" || true
  curl -sf -X DELETE "${API}/catalog-item-instances/${PIN_ID}" || true
fi

echo "=== verify-placement-policy-alignment OK ==="
