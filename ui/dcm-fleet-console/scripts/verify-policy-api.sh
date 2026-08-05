#!/usr/bin/env bash
# Verify DCM policy-manager state via gateway API (curl + jq).
set -euo pipefail

DCM_API_URL="${DCM_API_URL:-http://127.0.0.1:30908/api/v1alpha1}"
E2E_PREFIX="${E2E_POLICY_PREFIX:-E2E Policy}"

CURL_AUTH=()
if [[ -n "${FLEET_AUTH_COOKIE_JAR:-}" && -f "${FLEET_AUTH_COOKIE_JAR}" ]]; then
  CURL_AUTH=(-b "${FLEET_AUTH_COOKIE_JAR}" -c "${FLEET_AUTH_COOKIE_JAR}")
fi

usage() {
  echo "Usage: $0 assert-exists --id ID | --display-name NAME"
  echo "       $0 assert-field --id ID --jq EXPR"
  echo "       $0 assert-deleted --id ID"
  echo "       $0 delete --id ID   (only if display_name starts with '$E2E_PREFIX')"
  exit 1
}

fetch_policy() {
  local id="$1"
  curl "${CURL_AUTH[@]}" -sf "${DCM_API_URL}/policies/${id}"
}

cmd="${1:-}"
shift || usage

case "$cmd" in
  assert-exists)
    id=""
    display_name=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --id) id="$2"; shift 2 ;;
        --display-name) display_name="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    if [[ -n "$id" ]]; then
      fetch_policy "$id" | jq -e '.id' >/dev/null
      echo "OK policy exists id=$id"
    elif [[ -n "$display_name" ]]; then
      curl "${CURL_AUTH[@]}" -sf "${DCM_API_URL}/policies" | jq -e --arg n "$display_name" \
        '.policies[] | select(.display_name == $n)' >/dev/null
      echo "OK policy exists display_name=$display_name"
    else
      usage
    fi
    ;;
  assert-field)
    id=""
    expr=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --id) id="$2"; shift 2 ;;
        --jq) expr="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    [[ -n "$id" && -n "$expr" ]] || usage
    fetch_policy "$id" | jq -e "$expr" >/dev/null
    echo "OK policy $id satisfies: $expr"
    ;;
  assert-deleted)
    id=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --id) id="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    [[ -n "$id" ]] || usage
    code=$(curl "${CURL_AUTH[@]}" -s -o /dev/null -w "%{http_code}" "${DCM_API_URL}/policies/${id}")
    if [[ "$code" != "404" ]]; then
      echo "Expected 404 for deleted policy $id, got $code" >&2
      exit 1
    fi
    echo "OK policy deleted id=$id"
    ;;
  delete)
    id=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --id) id="$2"; shift 2 ;;
        *) usage ;;
      esac
    done
    [[ -n "$id" ]] || usage
    name=$(fetch_policy "$id" | jq -r '.display_name // ""')
    if [[ "$name" != "$E2E_PREFIX"* ]]; then
      echo "Refusing to delete non-E2E policy: $name" >&2
      exit 1
    fi
    curl "${CURL_AUTH[@]}" -sf -X DELETE "${DCM_API_URL}/policies/${id}"
    echo "OK deleted id=$id"
    ;;
  *)
    usage
    ;;
esac
