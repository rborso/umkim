#!/usr/bin/env bash
# Re-create a DCM catalog-item-instance after Velero restore (patched user_values).
#
# Usage:
#   ./scripts/rebind-dcm-instance.sh --manifest dcm-instance.json --cluster lab-int01 \
#     [--mode supersede|clone] [--source-instance-id <uuid>] \
#     [--patch routing.cluster=lab-ext02] [--patch values.persistence.storageClass=local-path]
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
MANIFEST=""
CLUSTER=""
MODE="supersede"
SOURCE_INSTANCE_ID=""
declare -a PATCHES=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --manifest) MANIFEST="$2"; shift 2 ;;
    --cluster) CLUSTER="$2"; shift 2 ;;
    --mode) MODE="$2"; shift 2 ;;
    --source-instance-id) SOURCE_INSTANCE_ID="$2"; shift 2 ;;
    --patch) PATCHES+=("$2"); shift 2 ;;
    -h|--help)
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$MANIFEST" && -f "$MANIFEST" ]] || { echo "ERROR: --manifest required" >&2; exit 1; }
[[ -n "$CLUSTER" ]] || { echo "ERROR: --cluster required" >&2; exit 1; }
[[ "$MODE" == "supersede" || "$MODE" == "clone" ]] || { echo "ERROR: --mode must be supersede or clone" >&2; exit 1; }

apply_patch() {
  local path="$1" value="$2"
  local tmp
  tmp=$(mktemp)
  jq --arg p "$path" --arg v "$value" '
    .spec.user_values = (
      (.spec.user_values // [])
      | map(select(.path != $p))
      + [{path: $p, value: $v}]
    )
  ' "$MANIFEST" >"$tmp"
  mv "$tmp" "$MANIFEST"
}

apply_patch "routing.cluster" "$CLUSTER"

for patch in "${PATCHES[@]}"; do
  path="${patch%%=*}"
  value="${patch#*=}"
  apply_patch "$path" "$value"
done

DISPLAY_NAME=$(jq -r '.display_name // "restored-instance"' "$MANIFEST")
CATALOG_ITEM_ID=$(jq -r '.spec.catalog_item_id // empty' "$MANIFEST")
LABEL_SELECTOR=$(jq -c '.spec.label_selector // {"fleet.workload/profile":"demo","fleet.placement/scope":"single"}' "$MANIFEST")
USER_VALUES=$(jq -c '.spec.user_values // []' "$MANIFEST")

[[ -n "$CATALOG_ITEM_ID" ]] || { echo "ERROR: manifest missing spec.catalog_item_id" >&2; exit 1; }

if [[ "$MODE" == "supersede" ]]; then
  if [[ -z "$SOURCE_INSTANCE_ID" ]]; then
    SOURCE_INSTANCE_ID=$(jq -r '.id // .uid // empty' "$MANIFEST")
  fi
  if [[ -n "$SOURCE_INSTANCE_ID" ]]; then
    curl -sf -X DELETE "${API}/catalog-item-instances/${SOURCE_INSTANCE_ID}" || true
  fi
  REBIND_DISPLAY_NAME="$DISPLAY_NAME"
else
  REBIND_DISPLAY_NAME="${DISPLAY_NAME}-restored"
fi

HTTP=$(curl -s -o /tmp/rebind-instance.json -w '%{http_code}' -X POST "${API}/catalog-item-instances" \
  -H 'Content-Type: application/json' \
  -d "{
    \"api_version\": \"v1alpha1\",
    \"display_name\": \"${REBIND_DISPLAY_NAME}\",
    \"spec\": {
      \"catalog_item_id\": \"${CATALOG_ITEM_ID}\",
      \"label_selector\": ${LABEL_SELECTOR},
      \"user_values\": ${USER_VALUES}
    }
  }")

if [[ "$HTTP" != "201" ]]; then
  echo "ERROR: rebind HTTP ${HTTP}" >&2
  jq . /tmp/rebind-instance.json 2>/dev/null || cat /tmp/rebind-instance.json
  exit 1
fi

jq -r '.id // .uid' /tmp/rebind-instance.json
rm -f /tmp/rebind-instance.json
