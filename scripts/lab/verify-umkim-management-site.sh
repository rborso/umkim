#!/usr/bin/env bash
# Verify management site, hub infrastructure API, and domain sync model.
set -euo pipefail

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
MGMT_SITE="${FLEET_MANAGEMENT_SITE_ID:-management}"
MGMT_CLUSTER="${FLEET_MANAGEMENT_CLUSTER_NAME:-umkim-hub}"
TEST_CLUSTER="${FLEET_DOMAIN_SYNC_CLUSTER:-lab-ha01}"
TEST_SITE="${FLEET_DOMAIN_SYNC_SITE:-beta-site}"
TEST_DOMAIN_BINDING="${FLEET_TEST_DOMAIN_BINDING:-homelab-zone}"

echo "=== verify-umkim-management-site ==="
echo "API=${API}"

curl -sf "${API}/sites" | jq -e --arg s "${MGMT_SITE}" '[.sites[]? | (.metadata.name // .spec.siteId)] | index($s)' >/dev/null \
  || { echo "Site ${MGMT_SITE} not found in GET /sites"; exit 1; }
echo "OK: site ${MGMT_SITE} listed"

hub_json="$(curl -sf "${API}/infrastructure/hub")"
echo "${hub_json}" | grep -q "\"host_name\"" \
  || { echo "GET /infrastructure/hub missing host_name"; exit 1; }
echo "${hub_json}" | grep -q "\"management_site_id\":\"${MGMT_SITE}\"" \
  || { echo "GET /infrastructure/hub missing management_site_id=${MGMT_SITE}"; exit 1; }
echo "OK: GET /infrastructure/hub returns host + site id"

clusters_json="$(curl -sf "${API}/clusters")"
if echo "${clusters_json}" | grep -q "\"name\":\"${MGMT_CLUSTER}\""; then
  echo "FAIL: cluster ${MGMT_CLUSTER} must not appear in GET /clusters"
  exit 1
fi
echo "OK: GET /clusters does not contain ${MGMT_CLUSTER}"

code="$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "${API}/sites/${MGMT_SITE}" \
  -H 'Content-Type: application/json' \
  -d '{"exposureMode":"port_forward"}')"
[[ "${code}" == "403" ]] || { echo "Expected PATCH site exposureMode → 403, got ${code}"; exit 1; }
echo "OK: PATCH site exposureMode → 403"

code="$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "${API}/sites/${MGMT_SITE}" \
  -H 'Content-Type: application/json' \
  -d '{"displayName":"Management DC"}')"
[[ "${code}" == "200" ]] || { echo "Expected PATCH site displayName → 200, got ${code}"; exit 1; }
echo "OK: PATCH site displayName → 200"

domains_json="$(curl -sf "${API}/edge/domains")"
echo "${domains_json}" | grep -q 'hub_zone' \
  || { echo "edge/domains missing hub_zone"; exit 1; }
echo "OK: edge/domains exposes hub_zone"

if curl -sf "${API}/clusters/${TEST_CLUSTER}" >/dev/null 2>&1; then
  site_json="$(curl -sf "${API}/sites/${TEST_SITE}")"
  before="$(echo "${site_json}" | jq -r '.clusters // [] | join(",")')"
  curl -sf -X PATCH "${API}/clusters/${TEST_CLUSTER}" \
    -H 'Content-Type: application/json' \
    -d "{\"edge\":{\"domain_bindings\":[\"${TEST_DOMAIN_BINDING}\"]}}" >/dev/null
  sleep 1
  site_after="$(curl -sf "${API}/sites/${TEST_SITE}")"
  echo "${site_after}" | jq -e --arg c "${TEST_CLUSTER}" '.clusters | index($c)' >/dev/null \
    || { echo "Site ${TEST_SITE} clusters not updated after cluster PATCH (before: ${before})"; exit 1; }
  echo "OK: cluster PATCH domain_bindings updates site assignedClusters"
else
  echo "SKIP: cluster ${TEST_CLUSTER} not found — domain sync test skipped"
fi

echo "=== verify-umkim-management-site OK ==="
