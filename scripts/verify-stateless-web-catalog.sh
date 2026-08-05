#!/usr/bin/env bash
# Deploy and verify stateless web helm catalog items (json-pretty, ebook-converter).
#
# Required env:
#   FLEET_SITE=<workload-site-id>
#   FLEET_HELM_CLUSTER=<cluster>   e.g. lab-sn01 (required for in-cluster HTTP probe)
# Optional:
#   DCM_API=http://127.0.0.1:30908/api/v1alpha1
#   FLEET_KUBECONFIG_SECRET=fleet-kubeconfig-<cluster>  (fleet-system on hub)
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
SITE="${FLEET_SITE:?set FLEET_SITE}"
HELM_CLUSTER="${FLEET_HELM_CLUSTER:?set FLEET_HELM_CLUSTER}"
RUN_ID="${RUN_ID:-$$}"
KC_SECRET="${FLEET_KUBECONFIG_SECRET:-fleet-kubeconfig-${HELM_CLUSTER}}"
KC_FILE="${KC_FILE:-/tmp/fleet-kubeconfig-${HELM_CLUSTER}-${RUN_ID}}"

declare -a INST_IDS=()

cleanup() {
  for id in "${INST_IDS[@]}"; do
    curl -sf -X DELETE "${API}/catalog-item-instances/${id}" >/dev/null 2>&1 || true
  done
  rm -f "$KC_FILE"
}
trap cleanup EXIT

load_kubeconfig() {
  kubectl -n fleet-system get secret "$KC_SECRET" -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC_FILE"
}

deploy_instance() {
  local catalog_id="$1"
  local name="$2"
  local ns="$3"
  local http
  http=$(curl -s -o "/tmp/stateless-${name}.json" -w '%{http_code}' -X POST "${API}/catalog-item-instances" \
    -H 'Content-Type: application/json' \
    -d "{
      \"api_version\": \"v1alpha1\",
      \"display_name\": \"${name}\",
      \"spec\": {
        \"catalog_item_id\": \"${catalog_id}\",
        \"label_selector\": {\"fleet.workload/profile\": \"demo\", \"fleet.placement/scope\": \"single\"},
        \"user_values\": [
          { \"path\": \"metadata.name\", \"value\": \"${name}\" },
          { \"path\": \"release.namespace\", \"value\": \"${ns}\" },
          { \"path\": \"routing.mode\", \"value\": \"hub\" },
          { \"path\": \"routing.cluster\", \"value\": \"${HELM_CLUSTER}\" }
        ]
      }
    }")
  if [[ "$http" != "201" ]]; then
    echo "FAIL deploy ${catalog_id} HTTP ${http}"
    jq . "/tmp/stateless-${name}.json" 2>/dev/null || cat "/tmp/stateless-${name}.json"
    return 1
  fi
  local id
  id=$(jq -r '.id // .uid' "/tmp/stateless-${name}.json")
  INST_IDS+=("$id")
  echo "instance ${name} id=${id}"
}

probe_service() {
  local ns="$1"
  local name="$2"
  local expect="$3"
  echo "probe ${ns}/${name} for '${expect}'"
  kubectl --kubeconfig="$KC_FILE" -n "$ns" wait --for=condition=available deployment/"$name" --timeout=180s >/dev/null
  local body
  body=$(kubectl --kubeconfig="$KC_FILE" -n "$ns" exec "deploy/${name}" -- \
    wget -qO- 127.0.0.1/ 2>/dev/null || true)
  if ! grep -qi "$expect" <<<"$body"; then
    echo "FAIL: HTTP body missing expected text (len=${#body})"
    return 1
  fi
}

echo "=== verify-stateless-web-catalog (site=${SITE} cluster=${HELM_CLUSTER}) ==="

for item in json-pretty-helm ebook-converter-helm; do
  echo "Catalog item ${item}..."
  if ! curl -sf "${API}/catalog-items/${item}" | jq -e '.spec.service_type == "helm"' >/dev/null 2>&1; then
    echo "SKIP: catalog item ${item} not seeded (optional stateless-web catalog)"
    exit 0
  fi
done

JSON_NAME="json-pretty-verify-${RUN_ID}"
EBOOK_NAME="ebook-converter-verify-${RUN_ID}"

deploy_instance json-pretty-helm "$JSON_NAME" "$JSON_NAME"
deploy_instance ebook-converter-helm "$EBOOK_NAME" "$EBOOK_NAME"

echo "Waiting for helm placement..."
sleep 35

for id in "${INST_IDS[@]}"; do
  echo "Helm history ${id}..."
  curl -sf "${API}/catalog-item-instances/${id}/helm/history" | jq -e '.releases | length >= 1' >/dev/null
done

load_kubeconfig
probe_service "$JSON_NAME" "$JSON_NAME" "JSON Pretty Formatter"
probe_service "$EBOOK_NAME" "$EBOOK_NAME" "Ebook Converter"

echo "=== verify-stateless-web-catalog OK ==="
