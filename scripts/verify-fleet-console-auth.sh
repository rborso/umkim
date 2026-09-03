#!/usr/bin/env bash
# Verify fleet-console-auth: login, unauthorized API, optional SA token, viewer smoke.
set -euo pipefail

BASE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
AUTH="${BASE}/fleet/auth/v1alpha1"
FLEET_API="${BASE}/fleet/api/v1alpha1"
USER="${FLEET_AUTH_USER:-localadmin}"
PASS="${FLEET_AUTH_PASSWORD:-fleet-admin-change-me}"
COOKIE_JAR="${TMPDIR:-/tmp}/fleet-auth-cookies.txt"
VIEWER_USER="${FLEET_VIEWER_USER:-}"
VIEWER_PASS="${FLEET_VIEWER_PASSWORD:-}"
VIEWER_JAR="${TMPDIR:-/tmp}/fleet-viewer-cookies.txt"

rm -f "$COOKIE_JAR" "$VIEWER_JAR"

echo "== unauthenticated fleet API should fail =="
code=$(curl -s -o /dev/null -w '%{http_code}' "${FLEET_API}/clusters" || true)
if [[ "$code" != "401" && "$code" != "403" ]]; then
  echo "expected 401/403, got $code" >&2
  exit 1
fi

echo "== login =="
curl -s -c "$COOKIE_JAR" -X POST "${AUTH}/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USER}\",\"password\":\"${PASS}\"}" | head -c 200
echo

echo "== authenticated me =="
me=$(curl -s -b "$COOKIE_JAR" "${AUTH}/me")
echo "$me" | head -c 300
echo
if ! echo "$me" | grep -q '"roles"'; then
  echo "expected roles in /me response" >&2
  exit 1
fi

echo "== authenticated cluster list =="
code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' "${FLEET_API}/clusters")
if [[ "$code" != "200" ]]; then
  echo "expected 200, got $code" >&2
  exit 1
fi

echo "== global LLM API keys list (dc-admin) =="
code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' "${AUTH}/llm-api-keys")
if [[ "$code" != "200" ]]; then
  echo "expected 200 for llm-api-keys list, got $code" >&2
  exit 1
fi

if [[ -n "${FLEET_SA_TOKEN:-}" ]]; then
  echo "== service account token =="
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${FLEET_SA_TOKEN}" "${AUTH}/me")
  echo "auth/me HTTP $code"
fi

if [[ -n "$VIEWER_USER" && -n "$VIEWER_PASS" ]]; then
  echo "== viewer login =="
  curl -s -c "$VIEWER_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${VIEWER_USER}\",\"password\":\"${VIEWER_PASS}\"}" | head -c 200
  echo

  echo "== viewer cluster list =="
  code=$(curl -s -b "$VIEWER_JAR" -o /dev/null -w '%{http_code}' "${FLEET_API}/clusters")
  if [[ "$code" != "200" ]]; then
    echo "expected 200 for viewer clusters, got $code" >&2
    exit 1
  fi

  echo "== viewer mutate denied =="
  code=$(curl -s -b "$VIEWER_JAR" -o /dev/null -w '%{http_code}' -X POST "${FLEET_API}/sites" \
    -H 'Content-Type: application/json' \
    -d '{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"viewer-deny-test"},"spec":{"siteName":"viewer-deny-test","provisioningMode":"registry"}}')
  if [[ "$code" != "403" ]]; then
    echo "expected 403 for viewer site create, got $code" >&2
    exit 1
  fi
fi

echo "OK fleet-console-auth verify"
