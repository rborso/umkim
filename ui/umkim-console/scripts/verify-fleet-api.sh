#!/usr/bin/env bash
# Verify fleet-site API state for Playwright E2E (curl + jq).
set -euo pipefail

FLEET_API_URL="${FLEET_API_URL:-http://127.0.0.1:30912/fleet/api/v1alpha1}"
E2E_PREFIX="${E2E_SITE_PREFIX:-E2E Site}"

CURL_AUTH=()
if [[ -n "${FLEET_AUTH_COOKIE_JAR:-}" && -f "${FLEET_AUTH_COOKIE_JAR}" ]]; then
  CURL_AUTH=(-b "${FLEET_AUTH_COOKIE_JAR}" -c "${FLEET_AUTH_COOKIE_JAR}")
fi

usage() {
  echo "Usage: $0 assert-exists --name NAME | --display-name NAME"
  echo "       $0 assert-deleted --name NAME"
  echo "       $0 delete --name NAME   (only if display_name starts with '$E2E_PREFIX')"
  exit 1
}

fetch_site() {
  local name="$1"
  curl "${CURL_AUTH[@]}" -sf "${FLEET_API_URL}/sites/${name}"
}

cmd="${1:-}"
shift || usage

case "$cmd" in
  assert-exists)
    name=""
    display_name=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --name) name="$2"; shift 2 ;;
        --display-name) display_name="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    if [[ -n "$name" ]]; then
      fetch_site "$name" | jq -e '.metadata.name' >/dev/null
      echo "OK site exists name=$name"
    elif [[ -n "$display_name" ]]; then
      curl "${CURL_AUTH[@]}" -sf "${FLEET_API_URL}/sites" | jq -e --arg n "$display_name" \
        '(.sites // .)[] | select(.spec.displayName == $n or .spec.display_name == $n)' >/dev/null
      echo "OK site exists display_name=$display_name"
    else
      usage
    fi
    ;;
  assert-deleted)
    name=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --name) name="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    [[ -n "$name" ]] || usage
    code=$(curl "${CURL_AUTH[@]}" -s -o /dev/null -w "%{http_code}" "${FLEET_API_URL}/sites/${name}")
    if [[ "$code" != "404" ]]; then
      echo "Expected 404 for deleted site $name, got $code" >&2
      exit 1
    fi
    echo "OK site deleted name=$name"
    ;;
  delete)
    name=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --name) name="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    [[ -n "$name" ]] || usage
    display=$(fetch_site "$name" | jq -r '.spec.displayName // .spec.display_name // ""')
    if [[ "$display" != "$E2E_PREFIX"* ]]; then
      echo "Refusing to delete non-E2E site: $display" >&2
      exit 1
    fi
    curl "${CURL_AUTH[@]}" -sf -X DELETE "${FLEET_API_URL}/sites/${name}"
    echo "OK deleted name=$name"
    ;;
  *)
    usage
    ;;
esac
