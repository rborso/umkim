#!/usr/bin/env bash
# Assert catalog-item-instance API state for operator E2E (no browser).
set -euo pipefail

DCM_API="${DCM_API:-${DCM_API_URL:-http://127.0.0.1:30908/api/v1alpha1}}"
CMD="${1:-}"

CURL_AUTH=()
if [[ -n "${FLEET_AUTH_COOKIE_JAR:-}" && -f "${FLEET_AUTH_COOKIE_JAR}" ]]; then
  CURL_AUTH=(-b "${FLEET_AUTH_COOKIE_JAR}" -c "${FLEET_AUTH_COOKIE_JAR}")
fi

usage() {
  echo "Usage: $0 assert-exists|assert-deleted|delete --id <instance-id>" >&2
  exit 1
}

[[ -n "${CMD}" ]] || usage

INSTANCE_ID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --id) INSTANCE_ID="$2"; shift 2 ;;
    assert-exists|assert-deleted|delete) shift ;;
    *) shift ;;
  esac
done

[[ -n "${INSTANCE_ID}" ]] || usage

case "${CMD}" in
  assert-exists)
    curl "${CURL_AUTH[@]}" -sf "${DCM_API}/catalog-item-instances/${INSTANCE_ID}" >/dev/null
    ;;
  assert-deleted)
    code="$(curl "${CURL_AUTH[@]}" -s -o /dev/null -w '%{http_code}' "${DCM_API}/catalog-item-instances/${INSTANCE_ID}")"
    [[ "${code}" == "404" ]]
    ;;
  delete)
    curl "${CURL_AUTH[@]}" -sf -X DELETE "${DCM_API}/catalog-item-instances/${INSTANCE_ID}" >/dev/null || true
    ;;
  *)
    usage
    ;;
esac

echo "OK ${CMD} ${INSTANCE_ID}"
