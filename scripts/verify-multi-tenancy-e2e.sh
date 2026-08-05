#!/usr/bin/env bash
# E2E: org RBAC, shared/dedicated pools, customer isolation.
# Requires fleet-console-auth + fleet-site with multi-tenancy build deployed.
set -euo pipefail

BASE="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
AUTH="${BASE}/fleet/auth/v1alpha1"
API="${BASE}/api/v1alpha1"
DCM_DIRECT="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
FLEET_API="${BASE}/fleet/api/v1alpha1"
ADMIN_USER="${FLEET_AUTH_USER:-localadmin}"
ADMIN_PASS="${FLEET_AUTH_PASSWORD:?set FLEET_AUTH_PASSWORD}"
TEST_ORG="${FLEET_TEST_ORG:-acme}"
SHARED_CLUSTER="${FLEET_SHARED_CLUSTER:-lab-gpu03}"
CUSTOMER_USER="${FLEET_CUSTOMER_USER:-${TEST_ORG}-user}"
CUSTOMER_PASS="${FLEET_CUSTOMER_PASSWORD:-${TEST_ORG}-change-me}"
ADMIN_JAR="${TMPDIR:-/tmp}/fleet-mt-admin.txt"
CUSTOMER_JAR="${TMPDIR:-/tmp}/fleet-mt-customer.txt"
BETA_ORG="${FLEET_ORG_BETA:-beta}"
BETA_USER="${FLEET_BETA_USER:-beta-user}"
BETA_PASS="${FLEET_BETA_PASSWORD:-beta-change-me}"
BETA_JAR="${TMPDIR:-/tmp}/fleet-mt-beta.txt"
VERIFY_PLACEMENT="${VERIFY_PLACEMENT:-1}"
DEDICATED_CLUSTER="${FLEET_DEDICATED_CLUSTER:-acme-lab01}"
BETA_DEDICATED_CLUSTER="${FLEET_BETA_DEDICATED_CLUSTER:-beta-lab02}"

log() { echo "[mt-e2e] $*"; }
fail() { echo "[mt-e2e] FAIL: $*" >&2; exit 1; }

rm -f "$ADMIN_JAR" "$CUSTOMER_JAR" "$BETA_JAR"

admin_login() {
  curl -sf -c "$ADMIN_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" >/dev/null
}

customer_login() {
  curl -sf -c "$CUSTOMER_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${CUSTOMER_USER}\",\"password\":\"${CUSTOMER_PASS}\"}" >/dev/null
}

beta_login() {
  curl -sf -c "$BETA_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${BETA_USER}\",\"password\":\"${BETA_PASS}\"}" >/dev/null
}

placement_preview() {
  local jar="$1"
  local org="$2"
  local pool="$3"
  curl -sf -b "$jar" -X POST "${API}/placement-preview" \
    -H 'Content-Type: application/json' \
    -d "{\"mode\":\"deploy\",\"service_type\":\"container\",\"catalog_item_id\":\"demo-webapp\",\"label_selector\":{\"fleet.org\":\"${org}\",\"fleet.placement/pool\":\"${pool}\"}}"
}

placement_names() {
  jq -r '[.eligible_clusters[]?.name] | join(",")'
}

http_code() {
  curl -s -o /dev/null -w '%{http_code}' "$@"
}

log "admin login"
admin_login

me=$(curl -sf -b "$ADMIN_JAR" "${AUTH}/me")
echo "$me" | jq -e '.dcAdmin == true' >/dev/null || fail "admin /me not dcAdmin"
if ! echo "$me" | jq -e '.roles' >/dev/null 2>&1; then
  fail "multi-tenancy build required: /me missing roles (redeploy fleet-console-auth)"
fi

log "create org ${TEST_ORG}"
code=$(http_code -b "$ADMIN_JAR" -X POST "${AUTH}/orgs" \
  -H 'Content-Type: application/json' \
  -d "{\"metadata\":{\"name\":\"${TEST_ORG}\"},\"spec\":{\"slug\":\"${TEST_ORG}\",\"displayName\":\"E2E Org\",\"sharedQuota\":{\"cpuMilli\":8000,\"memoryBytes\":17179869184}}}")
[[ "$code" == "201" || "$code" == "409" ]] || fail "create org HTTP $code"

log "create org ${BETA_ORG}"
code=$(http_code -b "$ADMIN_JAR" -X POST "${AUTH}/orgs" \
  -H 'Content-Type: application/json' \
  -d "{\"metadata\":{\"name\":\"${BETA_ORG}\"},\"spec\":{\"slug\":\"${BETA_ORG}\",\"displayName\":\"E2E Beta\",\"sharedQuota\":{\"cpuMilli\":8000,\"memoryBytes\":17179869184}}}")
[[ "$code" == "201" || "$code" == "409" ]] || fail "create beta org HTTP $code"

log "mark shared pool on ${SHARED_CLUSTER}"
code=$(http_code -b "$ADMIN_JAR" -X PATCH "${FLEET_API}/clusters/${SHARED_CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d '{"pool":"shared"}')
[[ "$code" == "200" || "$code" == "404" || "$code" == "409" ]] || fail "patch shared pool HTTP $code"
pool=$(curl -sf -b "$ADMIN_JAR" "${FLEET_API}/clusters/${SHARED_CLUSTER}" | jq -r '.pool // .labels["fleet.pool"] // empty')
[[ "$pool" == "shared" ]] || log "WARN: ${SHARED_CLUSTER} pool=${pool:-unset} (expected shared when Ready)"

log "assignOrg ${DEDICATED_CLUSTER} → ${TEST_ORG} (dc-admin; idempotent if FCJ already labeled)"
code=$(http_code -b "$ADMIN_JAR" -X PATCH "${FLEET_API}/clusters/${DEDICATED_CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d "{\"pool\":\"dedicated\",\"org\":\"${TEST_ORG}\"}")
[[ "$code" == "200" || "$code" == "404" || "$code" == "409" ]] || fail "assignOrg HTTP $code"
if [[ "$code" == "200" ]]; then
  org_lbl=$(curl -sf -b "$ADMIN_JAR" "${FLEET_API}/clusters/${DEDICATED_CLUSTER}" | jq -r '.labels["fleet.org"] // empty')
  [[ "$org_lbl" == "$TEST_ORG" ]] || fail "assignOrg label got ${org_lbl:-empty}"
fi

log "assignOrg ${BETA_DEDICATED_CLUSTER} → ${BETA_ORG}"
code=$(http_code -b "$ADMIN_JAR" -X PATCH "${FLEET_API}/clusters/${BETA_DEDICATED_CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d "{\"pool\":\"dedicated\",\"org\":\"${BETA_ORG}\"}")
[[ "$code" == "200" || "$code" == "404" || "$code" == "409" ]] || fail "beta assignOrg HTTP $code"

log "bind customer user ${CUSTOMER_USER} (optional)"
rb_name="${TEST_ORG}-${CUSTOMER_USER}-customer"
code=$(http_code -b "$ADMIN_JAR" -X POST "${AUTH}/role-bindings" \
  -H 'Content-Type: application/json' \
  -d "{
    \"apiVersion\": \"console.umkim.com/v1alpha1\",
    \"kind\": \"FleetRoleBinding\",
    \"metadata\": {\"name\": \"${rb_name}\"},
    \"spec\": {
      \"roleRef\": {\"name\": \"customer\"},
      \"subjects\": [{\"kind\": \"User\", \"name\": \"${CUSTOMER_USER}\"}],
      \"scopes\": [{\"kind\": \"Org\", \"name\": \"${TEST_ORG}\"}]
    }
  }")
[[ "$code" == "201" || "$code" == "409" ]] || fail "customer binding HTTP $code"

log "bind org-admin for ${CUSTOMER_USER}"
oa_rb="${TEST_ORG}-${CUSTOMER_USER}-org-admin"
code=$(http_code -b "$ADMIN_JAR" -X POST "${AUTH}/role-bindings" \
  -H 'Content-Type: application/json' \
  -d "{
    \"apiVersion\": \"console.umkim.com/v1alpha1\",
    \"kind\": \"FleetRoleBinding\",
    \"metadata\": {\"name\": \"${oa_rb}\"},
    \"spec\": {
      \"roleRef\": {\"name\": \"org-admin\"},
      \"subjects\": [{\"kind\": \"User\", \"name\": \"${CUSTOMER_USER}\"}],
      \"scopes\": [{\"kind\": \"Org\", \"name\": \"${TEST_ORG}\"}]
    }
  }")
[[ "$code" == "201" || "$code" == "409" ]] || fail "org-admin binding HTTP $code"

log "bind beta customer ${BETA_USER}"
beta_rb="${BETA_ORG}-${BETA_USER}-customer"
code=$(http_code -b "$ADMIN_JAR" -X POST "${AUTH}/role-bindings" \
  -H 'Content-Type: application/json' \
  -d "{
    \"apiVersion\": \"console.umkim.com/v1alpha1\",
    \"kind\": \"FleetRoleBinding\",
    \"metadata\": {\"name\": \"${beta_rb}\"},
    \"spec\": {
      \"roleRef\": {\"name\": \"customer\"},
      \"subjects\": [{\"kind\": \"User\", \"name\": \"${BETA_USER}\"}],
      \"scopes\": [{\"kind\": \"Org\", \"name\": \"${BETA_ORG}\"}]
    }
  }")
[[ "$code" == "201" || "$code" == "409" ]] || fail "beta binding HTTP $code"

CUSTOMER_OK=false
if customer_login 2>/dev/null; then
  CUSTOMER_OK=true
else
  log "WARN: customer login skipped (set FleetUser/LLDAP user ${CUSTOMER_USER})"
fi

if [[ "$CUSTOMER_OK" == "true" ]]; then
  cust_me=$(curl -sf -b "$CUSTOMER_JAR" "${AUTH}/me")
  echo "$cust_me" | jq -e --arg o "$TEST_ORG" '.orgs[]? == $o' >/dev/null || fail "customer missing org scope"

  log "customer cannot create catalog-items"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${API}/catalog-items?id=deny-test" \
    -H 'Content-Type: application/json' -d '{}')
  [[ "$code" == "403" ]] || fail "expected 403 catalog create, got $code"

  log "customer policies list empty or forbidden"
  code=$(http_code -b "$CUSTOMER_JAR" "${API}/policies")
  # Customer-only binding would empty-list; with org-admin binding they see org policies only.
  if [[ "$code" == "403" ]]; then
    log "customer policies denied (403) — OK"
  else
    body=$(curl -sf -b "$CUSTOMER_JAR" "${API}/policies")
    echo "$body" | jq -e '
      (.results // []) | all(
        (.id // "" | startswith("'"${TEST_ORG}"'-"))
        or ((.label_selector["fleet.org"] // "") == "'"${TEST_ORG}"'")
        or ((.cluster_requirements.matchLabels["fleet.org"] // "") == "'"${TEST_ORG}"'")
      )
    ' >/dev/null || fail "policies not filtered to org"
  fi

  log "org-admin dedicated policy create"
  POL_ID="${TEST_ORG}-e2e-dedicated-$$"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${API}/policies?id=${POL_ID}" \
    -H 'Content-Type: application/json' \
    -d "{
      \"display_name\": \"E2E dedicated\",
      \"policy_type\": \"GLOBAL\",
      \"enabled\": true,
      \"priority\": 80,
      \"rego_code\": \"package e2e\\nmain := {\\\"rejected\\\": false}\",
      \"label_selector\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.placement/pool\": \"dedicated\"},
      \"cluster_requirements\": {
        \"matchLabels\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.pool\": \"dedicated\"},
        \"matchExpressions\": []
      },
      \"placement\": {\"cluster_count\": 1, \"strategy\": \"single\"}
    }")
  [[ "$code" == "200" || "$code" == "201" || "$code" == "409" ]] || fail "org-admin policy create HTTP $code"

  log "org-admin shared-pool policy rejected"
  BAD_POL="${TEST_ORG}-e2e-shared-$$"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${API}/policies?id=${BAD_POL}" \
    -H 'Content-Type: application/json' \
    -d "{
      \"display_name\": \"E2E shared deny\",
      \"policy_type\": \"GLOBAL\",
      \"enabled\": true,
      \"priority\": 80,
      \"rego_code\": \"package e2e\\nmain := {\\\"rejected\\\": false}\",
      \"label_selector\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.placement/pool\": \"shared\"},
      \"cluster_requirements\": {
        \"matchLabels\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.pool\": \"dedicated\"},
        \"matchExpressions\": []
      }
    }")
  [[ "$code" == "403" ]] || fail "expected 403 shared org policy, got $code"

  log "shared quota enforcement"
  code=$(http_code -b "$ADMIN_JAR" -X PATCH "${AUTH}/orgs/${TEST_ORG}" \
    -H 'Content-Type: application/json' \
    -d '{"spec":{"sharedQuota":{"cpuMilli":1,"memoryBytes":1}}}')
  [[ "$code" == "200" ]] || fail "patch quota HTTP $code"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${API}/catalog-item-instances" \
    -H 'Content-Type: application/json' \
    -d "{
      \"catalog_item_id\": \"demo-webapp\",
      \"display_name\": \"quota-deny-$$\",
      \"label_selector\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.placement/pool\": \"shared\"},
      \"user_values\": [
        {\"path\": \"resources.cpu.min\", \"value\": \"500m\"},
        {\"path\": \"resources.memory.min\", \"value\": \"256Mi\"}
      ]
    }")
  [[ "$code" == "403" ]] || fail "expected 403 shared quota exceeded, got $code"

  log "shared quota blocks tiny request + huge limit bypass"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${API}/catalog-item-instances" \
    -H 'Content-Type: application/json' \
    -d "{
      \"api_version\": \"v1alpha1\",
      \"display_name\": \"quota-limit-bypass-$$\",
      \"spec\": {
        \"catalog_item_id\": \"demo-webapp\",
        \"label_selector\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.placement/pool\": \"shared\"},
        \"user_values\": [
          {\"path\": \"resources.cpu.min\", \"value\": \"1m\"},
          {\"path\": \"resources.cpu.max\", \"value\": \"4000m\"},
          {\"path\": \"resources.memory.min\", \"value\": \"1MB\"},
          {\"path\": \"resources.memory.max\", \"value\": \"4Gi\"}
        ]
      }
    }")
  [[ "$code" == "403" ]] || fail "expected 403 limit bypass blocked, got $code"

  log "dedicated pool instance create skips shared quota"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${API}/catalog-item-instances" \
    -H 'Content-Type: application/json' \
    -d "{
      \"catalog_item_id\": \"demo-webapp\",
      \"display_name\": \"quota-dedicated-$$\",
      \"label_selector\": {\"fleet.org\": \"${TEST_ORG}\", \"fleet.placement/pool\": \"dedicated\"},
      \"user_values\": [
        {\"path\": \"resources.cpu.min\", \"value\": \"500m\"},
        {\"path\": \"resources.memory.min\", \"value\": \"256Mi\"}
      ]
    }")
  # May 201/200 (created) or 4xx from DCM placement — must not be quota 403.
  [[ "$code" != "403" ]] || fail "dedicated create should not hit shared quota 403"

  log "restore shared quota"
  code=$(http_code -b "$ADMIN_JAR" -X PATCH "${AUTH}/orgs/${TEST_ORG}" \
    -H 'Content-Type: application/json' \
    -d '{"spec":{"sharedQuota":{"cpuMilli":8000,"memoryBytes":17179869184}}}')
  [[ "$code" == "200" ]] || fail "restore quota HTTP $code"
fi

SITE_NAME="${TEST_ORG}-site-e2e"
CLUSTER_NAME="${TEST_ORG}-cluster-e2e"

log "create org site ${SITE_NAME} (admin)"
code=$(http_code -b "$ADMIN_JAR" -X POST "${FLEET_API}/sites" \
  -H 'Content-Type: application/json' \
  -d "{
    \"apiVersion\": \"fleet.umkim.com/v1alpha1\",
    \"kind\": \"FleetSite\",
    \"metadata\": {\"name\": \"${SITE_NAME}\"},
    \"spec\": {
      \"siteName\": \"${SITE_NAME}\",
      \"orgId\": \"${TEST_ORG}\",
      \"provisioningMode\": \"registry\",
      \"displayName\": \"E2E site\"
    }
  }")
[[ "$code" == "201" || "$code" == "409" ]] || fail "create site HTTP $code"

if [[ "$CUSTOMER_OK" == "true" ]]; then
  log "customer cannot create site without org prefix"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${FLEET_API}/sites" \
    -H 'Content-Type: application/json' \
    -d '{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"bad-site"},"spec":{"siteName":"bad-site","orgId":"'"${TEST_ORG}"'","provisioningMode":"registry"}}')
  [[ "$code" == "400" || "$code" == "403" ]] || fail "expected 400/403 bad site name, got $code"
fi

if [[ "$SHARED_CLUSTER" != "lab-gpu03" ]]; then
  log "patch lab-gpu03 shared pool"
  code=$(http_code -b "$ADMIN_JAR" -X PATCH "${FLEET_API}/clusters/lab-gpu03" \
    -H 'Content-Type: application/json' -d '{"pool":"shared"}')
  [[ "$code" == "200" || "$code" == "404" || "$code" == "409" ]] || fail "patch lab-gpu03 pool HTTP $code"
fi

if [[ "$CUSTOMER_OK" == "true" ]]; then
  log "customer list sites scoped"
  sites=$(curl -sf -b "$CUSTOMER_JAR" "${FLEET_API}/sites" | jq -r '.sites[]? | .spec.orgId // .orgId // empty')
  while IFS= read -r org; do
    [[ -z "$org" ]] && continue
    [[ "$org" == "$TEST_ORG" ]] || fail "customer saw foreign org site: $org"
  done <<< "$sites"

  log "customer cannot set shared pool on cluster create"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${FLEET_API}/clusters" \
    -H 'Content-Type: application/json' \
    -d "{
      \"clusterName\": \"${CLUSTER_NAME}\",
      \"siteId\": \"${SITE_NAME}\",
      \"bootstrapHost\": \"198.51.100.99\",
      \"clusterLabels\": {\"fleet.pool\": \"shared\"},
      \"topology\": \"single_node\",
      \"nodes\": [{\"hostname\": \"${CLUSTER_NAME}\", \"address\": \"198.51.100.99\", \"role\": \"control\"}]
    }")
  [[ "$code" == "403" ]] || fail "expected 403 shared pool cluster, got $code"

  log "customer create dedicated cluster ${CLUSTER_NAME}"
  code=$(http_code -b "$CUSTOMER_JAR" -X POST "${FLEET_API}/clusters" \
    -H 'Content-Type: application/json' \
    -d "{
      \"clusterName\": \"${CLUSTER_NAME}\",
      \"siteId\": \"${SITE_NAME}\",
      \"bootstrapHost\": \"198.51.100.99\",
      \"topology\": \"single_node\",
      \"nodes\": [{\"hostname\": \"${CLUSTER_NAME}\", \"address\": \"198.51.100.99\", \"role\": \"control\"}]
    }")
  [[ "$code" == "201" || "$code" == "409" ]] || fail "create cluster HTTP $code"

  log "customer list clusters scoped"
  clusters=$(curl -sf -b "$CUSTOMER_JAR" "${FLEET_API}/clusters" | jq -r '.clusters[]? | .labels["fleet.org"] // empty')
  while IFS= read -r org; do
    [[ -z "$org" ]] && continue
    [[ "$org" == "$TEST_ORG" ]] || fail "customer saw foreign org cluster: $org"
  done <<< "$clusters"
fi

log "seed shared/dedicated placement policies"
INV="${INV:--i inventory/hosts.yml -i inventory/hosts-from-hub.yml}"
EXTRA=()
[[ -n "${FLEET_HUB_IP:-}" ]] && EXTRA=(-e "ansible_host=${FLEET_HUB_IP}")
VAULT_ARGS=()
[[ -f "${HOME}/.umkim-vault-pass" ]] && VAULT_ARGS=(--vault-password-file "${HOME}/.umkim-vault-pass")
ansible-playbook ${INV} fleet-dcm.yml -l hub "${EXTRA[@]}" --tags dcm_seed_placement_policies "${VAULT_ARGS[@]}" >/dev/null

for pid in shared-pool-default-container shared-pool-default-helm dedicated-pool-default-container dedicated-pool-default-helm; do
  curl -sf "${DCM_DIRECT}/policies/${pid}" | jq -e '.id == "'"$pid"'"' >/dev/null || fail "missing policy $pid"
done

if [[ "$VERIFY_PLACEMENT" == "1" && "$CUSTOMER_OK" == "true" ]]; then
  log "placement: acme shared pool → ${SHARED_CLUSTER}"
  names=$(placement_preview "$CUSTOMER_JAR" "$TEST_ORG" shared | placement_names)
  [[ "$names" == *"${SHARED_CLUSTER}"* ]] || fail "acme shared missing ${SHARED_CLUSTER}: $names"
  [[ "$names" != *"${DEDICATED_CLUSTER}"* ]] || fail "acme shared must not include ${DEDICATED_CLUSTER}"
  [[ "$names" != *"${BETA_DEDICATED_CLUSTER}"* ]] || fail "acme shared must not include ${BETA_DEDICATED_CLUSTER}"

  log "placement: acme dedicated pool → ${DEDICATED_CLUSTER}"
  names=$(placement_preview "$CUSTOMER_JAR" "$TEST_ORG" dedicated | placement_names)
  [[ "$names" == *"${DEDICATED_CLUSTER}"* ]] || fail "acme dedicated expected ${DEDICATED_CLUSTER}, got: $names"
  [[ "$names" != *"${SHARED_CLUSTER}"* ]] || fail "acme dedicated must not include shared ${SHARED_CLUSTER}"

  if beta_login 2>/dev/null; then
    log "placement: beta dedicated pool → ${BETA_DEDICATED_CLUSTER} (must exclude ${DEDICATED_CLUSTER})"
    resp=$(placement_preview "$BETA_JAR" "$BETA_ORG" dedicated)
    names=$(echo "$resp" | placement_names)
    [[ "$names" == *"${BETA_DEDICATED_CLUSTER}"* ]] || fail "beta dedicated expected ${BETA_DEDICATED_CLUSTER}, got: $names"
    [[ "$names" != *"${DEDICATED_CLUSTER}"* ]] || fail "beta dedicated must not include ${DEDICATED_CLUSTER}: $names"

    log "placement: beta shared pool → ${SHARED_CLUSTER}"
    names=$(placement_preview "$BETA_JAR" "$BETA_ORG" shared | placement_names)
    [[ "$names" == *"${SHARED_CLUSTER}"* ]] || fail "beta shared missing ${SHARED_CLUSTER}: $names"
    [[ "$names" != *"${DEDICATED_CLUSTER}"* ]] || fail "beta shared must not include ${DEDICATED_CLUSTER}"

    log "beta cluster list must not include ${DEDICATED_CLUSTER}"
    beta_clusters=$(curl -sf -b "$BETA_JAR" "${FLEET_API}/clusters" | jq -r '.clusters[]?.name // empty')
    while IFS= read -r c; do
      [[ -z "$c" ]] && continue
      [[ "$c" != "${DEDICATED_CLUSTER}" ]] || fail "beta customer saw ${DEDICATED_CLUSTER}"
    done <<< "$beta_clusters"

    log "beta cannot pin routing.cluster to ${DEDICATED_CLUSTER}"
    code=$(http_code -b "$BETA_JAR" -X POST "${API}/catalog-item-instances" \
      -H 'Content-Type: application/json' \
      -d "{
        \"api_version\": \"v1alpha1\",
        \"display_name\": \"pin-foreign-$$\",
        \"spec\": {
          \"catalog_item_id\": \"demo-webapp\",
          \"label_selector\": {\"fleet.org\": \"${BETA_ORG}\", \"fleet.placement/pool\": \"dedicated\"},
          \"user_values\": [
            {\"path\": \"metadata.name\", \"value\": \"pin-foreign-$$\"},
            {\"path\": \"routing.mode\", \"value\": \"hub\"},
            {\"path\": \"routing.cluster\", \"value\": \"${DEDICATED_CLUSTER}\"}
          ]
        }
      }")
    [[ "$code" != "200" && "$code" != "201" ]] || fail "expected reject for foreign routing.cluster pin, got $code"
    log "foreign routing.cluster pin rejected (HTTP $code)"
  else
    log "WARN: beta placement checks skipped (beta-user login failed)"
  fi
fi

log "OK multi-tenancy e2e"
