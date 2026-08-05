#!/usr/bin/env bash
# Verify fleet-console-auth: login, unauthorized API, optional SA token, multi-tenancy.
set -euo pipefail

BASE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
AUTH="${BASE}/fleet/auth/v1alpha1"
API="${BASE}/api/v1alpha1"
FLEET_API="${BASE}/fleet/api/v1alpha1"
USER="${FLEET_AUTH_USER:-localadmin}"
PASS="${FLEET_AUTH_PASSWORD:-fleet-admin-change-me}"
COOKIE_JAR="${TMPDIR:-/tmp}/fleet-auth-cookies.txt"
CUSTOMER_USER="${FLEET_CUSTOMER_USER:-}"
CUSTOMER_PASS="${FLEET_CUSTOMER_PASSWORD:-}"
CUSTOMER_JAR="${TMPDIR:-/tmp}/fleet-customer-cookies.txt"
TEST_ORG="${FLEET_TEST_ORG:-acme}"

rm -f "$COOKIE_JAR" "$CUSTOMER_JAR"

echo "== unauthenticated catalog should fail =="
code=$(curl -s -o /dev/null -w '%{http_code}' "${API}/catalog-items" || true)
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

echo "== authenticated catalog list =="
code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' "${API}/catalog-items")
if [[ "$code" != "200" ]]; then
  echo "expected 200, got $code" >&2
  exit 1
fi

echo "== catalog-access requires dc-admin =="
code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' "${AUTH}/catalog-access")
if [[ "$code" != "200" ]]; then
  echo "expected 200 for dc-admin catalog-access, got $code" >&2
  exit 1
fi

echo "== org API list =="
code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' "${AUTH}/orgs")
if [[ "$code" != "200" ]]; then
  echo "expected 200 for org list, got $code" >&2
  exit 1
fi

if [[ "${VERIFY_CREATE_ORG:-1}" != "0" ]]; then
echo "== ensure test org ${TEST_ORG} =="
code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' -X POST "${AUTH}/orgs" \
  -H 'Content-Type: application/json' \
  -d "{\"metadata\":{\"name\":\"${TEST_ORG}\"},\"spec\":{\"slug\":\"${TEST_ORG}\",\"displayName\":\"Test Org\",\"sharedQuota\":{\"cpuMilli\":8000,\"memoryBytes\":17179869184}}}")
if [[ "$code" != "201" && "$code" != "409" ]]; then
  echo "expected 201/409 creating org, got $code" >&2
  exit 1
fi
fi

if [[ -n "${FLEET_SA_TOKEN:-}" ]]; then
  echo "== service account token =="
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${FLEET_SA_TOKEN}" "${API}/health/catalog")
  echo "health/catalog HTTP $code"
fi

if [[ -n "$CUSTOMER_USER" && -n "$CUSTOMER_PASS" && "${FLEET_VERIFY_MUTATING:-1}" != "0" ]]; then
  echo "== customer login =="
  curl -s -c "$CUSTOMER_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${CUSTOMER_USER}\",\"password\":\"${CUSTOMER_PASS}\"}" | head -c 200
  echo

  echo "== customer policies list empty =="
  body=$(curl -s -b "$CUSTOMER_JAR" "${API}/policies")
  if echo "$body" | grep -q '"results":\[\]'; then
    echo "policies list filtered"
  else
    echo "expected empty policies results for customer" >&2
    exit 1
  fi

  echo "== customer cannot POST catalog-items =="
  code=$(curl -s -b "$CUSTOMER_JAR" -o /dev/null -w '%{http_code}' -X POST "${API}/catalog-items?id=test-deny" \
    -H 'Content-Type: application/json' -d '{}')
  if [[ "$code" != "403" ]]; then
    echo "expected 403 for customer catalog create, got $code" >&2
    exit 1
  fi

  echo "== customer cluster list scoped =="
  code=$(curl -s -b "$CUSTOMER_JAR" -o /dev/null -w '%{http_code}' "${FLEET_API}/clusters")
  if [[ "$code" != "200" ]]; then
    echo "expected 200 for customer clusters, got $code" >&2
    exit 1
  fi
fi

echo "OK fleet-console-auth verify"
