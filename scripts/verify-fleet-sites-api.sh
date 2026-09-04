#!/usr/bin/env bash
# Verify FleetSite registry create (ingress[]), jumphost PATCH, and domains CM sync.
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
NS="${FLEET_EDGE_NS:-fleet-system}"
CM="${FLEET_EDGE_DOMAINS_CM:-fleet-edge-domains}"
TEST_SITE="${FLEET_TEST_SITE_NAME:-verify-site-$$}"
WORKDIR="${TMPDIR:-/tmp}/fleet-sites-api-$$"
mkdir -p "${WORKDIR}"
trap 'rm -rf "${WORKDIR}"' EXIT

echo "=== verify-fleet-sites-api ==="
curl -sf "${API}/sites" >/dev/null || { echo "Fleet API unreachable at ${API}"; exit 1; }

EDGE_DOMAIN="verify.${FLEET_HUB_EDGE_DOMAIN:-mgmt.umkim.com}"
CREATE_BODY=$(cat <<EOF
{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"${TEST_SITE}"},"spec":{"siteName":"${TEST_SITE}","siteId":"${TEST_SITE}","provisioningMode":"registry","displayName":"Verify site","jumphost":"verify@127.0.0.1:22","edge":{"ingress":[{"id":"verify-pf","method":"port_forward","address":"203.0.113.50","zone":"${EDGE_DOMAIN}","wildcard":"*.${EDGE_DOMAIN}"}],"wanEgress":{"enabled":true,"dnsTargetNote":"verify script"}}}}
EOF
)
HTTP=$(curl -s -o /tmp/fs-create.json -w '%{http_code}' -X POST "${API}/sites" \
  -H 'Content-Type: application/json' -d "${CREATE_BODY}")
[ "${HTTP}" = "201" ] || { echo "POST /sites expected 201 got ${HTTP}: $(cat /tmp/fs-create.json)"; exit 1; }
JH=$(python3 -c "import json; print(json.load(open('/tmp/fs-create.json')).get('spec',{}).get('jumphost',''))")
[ "${JH}" = "verify@127.0.0.1:22" ] || { echo "create jumphost=${JH}"; exit 1; }
curl -sf "${API}/sites/${TEST_SITE}/bootstrap-script" && { echo "registry site should not expose bootstrap"; exit 1; } || true

echo "== DELETE registry site (API) =="
HTTP_DEL=$(curl -s -o /tmp/fs-delete.json -w '%{http_code}' -X DELETE "${API}/sites/${TEST_SITE}")
[ "${HTTP_DEL}" = "204" ] || { echo "DELETE /sites/${TEST_SITE} expected 204 got ${HTTP_DEL}: $(cat /tmp/fs-delete.json)"; exit 1; }

echo "== POST site with latched siteId (h + homelab) normalizes =="
LATCH_SITE="homelab-verify-$$"
LATCH_BODY=$(cat <<EOF
{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"${LATCH_SITE}"},"spec":{"siteName":"${LATCH_SITE}","siteId":"h","provisioningMode":"registry"}}
EOF
)
HTTP_LATCH=$(curl -s -o /tmp/fs-latch.json -w '%{http_code}' -X POST "${API}/sites" \
  -H 'Content-Type: application/json' -d "${LATCH_BODY}")
[ "${HTTP_LATCH}" = "201" ] || { echo "POST latch site expected 201 got ${HTTP_LATCH}: $(cat /tmp/fs-latch.json)"; exit 1; }
SID=$(curl -sf "${API}/sites/${LATCH_SITE}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('spec',{}).get('siteId',''))")
[ "${SID}" = "${LATCH_SITE}" ] || { echo "latched siteId=${SID} want ${LATCH_SITE}"; exit 1; }
curl -sf -X DELETE "${API}/sites/${LATCH_SITE}" >/dev/null

echo "== DELETE site with attached cluster returns 409 =="
CONFLICT_SITE="verify-conflict-$$"
CONFLICT_CLUSTER="verify-mc-$$"
CREATE_CONFLICT=$(cat <<EOF
{"apiVersion":"fleet.umkim.com/v1alpha1","kind":"FleetSite","metadata":{"name":"${CONFLICT_SITE}"},"spec":{"siteName":"${CONFLICT_SITE}","siteId":"${CONFLICT_SITE}","provisioningMode":"registry"}}
EOF
)
curl -sf -X POST "${API}/sites" -H 'Content-Type: application/json' -d "${CREATE_CONFLICT}" >/dev/null
MC_FILE="${WORKDIR}/managedcluster-${CONFLICT_CLUSTER}.yaml"
cat > "${MC_FILE}" <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: ${CONFLICT_CLUSTER}
  labels:
    fleet.site: ${CONFLICT_SITE}
spec:
  hubAcceptsClient: true
EOF
kubectl apply -f "${MC_FILE}" >/dev/null
HTTP_CONFLICT=$(curl -s -o /tmp/fs-conflict.json -w '%{http_code}' -X DELETE "${API}/sites/${CONFLICT_SITE}")
[ "${HTTP_CONFLICT}" = "409" ] || { echo "DELETE with clusters expected 409 got ${HTTP_CONFLICT}: $(cat /tmp/fs-conflict.json)"; exit 1; }
kubectl delete managedcluster "${CONFLICT_CLUSTER}" --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
for _ in $(seq 1 30); do
  kubectl get managedcluster "${CONFLICT_CLUSTER}" >/dev/null 2>&1 || break
  sleep 2
done
HTTP_CLEAN=""
for _ in $(seq 1 30); do
  HTTP_CLEAN=$(curl -s -o /tmp/fs-clean.json -w '%{http_code}' -X DELETE "${API}/sites/${CONFLICT_SITE}")
  [[ "${HTTP_CLEAN}" = "204" || "${HTTP_CLEAN}" = "404" ]] && break
  sleep 2
done
[[ "${HTTP_CLEAN}" = "204" || "${HTTP_CLEAN}" = "404" ]] || { echo "cleanup DELETE ${CONFLICT_SITE} expected 204/404 got ${HTTP_CLEAN}: $(cat /tmp/fs-clean.json)"; exit 1; }

kubectl delete fleetsite "${TEST_SITE}" -n fleet-console --ignore-not-found --wait=false 2>/dev/null || true

TEST_ZONE="test-console.${FLEET_HUB_EDGE_DOMAIN:-mgmt.umkim.com}"
TEST_JH="verify-cli@127.0.0.1:22"
PATCH_BODY=$(cat <<EOF
{"jumphost":"${TEST_JH}","ingress":[{"id":"homelab-pf","method":"port_forward","address":"203.0.113.50","zone":"${TEST_ZONE}","wildcard":"*.${TEST_ZONE}"}],"wanEgress":{"enabled":true,"dnsTargetNote":"console verify"}}
EOF
)

PATCH_SITE="${FLEET_SITE_PATCH_TARGET:-${FLEET_SITE:-homelab}}"
# Prefer a registry site that exists (workload sites only — avoid management).
if ! curl -sf "${API}/sites/${PATCH_SITE}" >/dev/null 2>&1; then
  PATCH_SITE="$(curl -sf "${API}/sites" | python3 -c "
import json,sys
ss=json.load(sys.stdin).get('sites',[])
for s in ss:
  ann=(s.get('metadata') or {}).get('annotations') or {}
  if ann.get('fleet.umkim.com/management-site')=='true':
    continue
  name=(s.get('metadata') or {}).get('name') or s.get('name') or ''
  if name:
    print(name); break
" 2>/dev/null || true)"
fi
[[ -n "${PATCH_SITE}" ]] || { echo "SKIP: no fleet site for PATCH/domains test"; exit 0; }

# Preserve prior jumphost for restore best-effort
BEFORE_JH=$(curl -sf "${API}/sites/${PATCH_SITE}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('spec',{}).get('jumphost',''))" 2>/dev/null || true)

curl -sf -X PATCH "${API}/sites/${PATCH_SITE}" \
  -H 'Content-Type: application/json' \
  -d "${PATCH_BODY}" | head -c 200
echo ""

AFTER_JH=$(curl -sf "${API}/sites/${PATCH_SITE}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('spec',{}).get('jumphost',''))")
[ "${AFTER_JH}" = "${TEST_JH}" ] || { echo "PATCH jumphost got ${AFTER_JH} want ${TEST_JH}"; exit 1; }

RAW=""
for _ in $(seq 1 20); do
  RAW="$(kubectl -n "${NS}" get configmap "${CM}" -o json 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('domains.json',''))" 2>/dev/null || true)"
  echo "${RAW}" | grep -q "${TEST_ZONE}" && break
  sleep 2
done
echo "${RAW}" | grep -q "${TEST_ZONE}" || { echo "domains.json missing ${TEST_ZONE}"; exit 1; }
ANN="$(kubectl -n "${NS}" get configmap "${CM}" -o json 2>/dev/null \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('metadata',{}).get('annotations',{}).get('fleet.umkim.com/console-managed',''))" 2>/dev/null || true)"
[ "${ANN}" = "true" ] || { echo "expected console-managed annotation got '${ANN}'"; exit 1; }

# Restore jumphost if we had one
if [[ -n "${BEFORE_JH}" ]]; then
  curl -sf -X PATCH "${API}/sites/${PATCH_SITE}" -H 'Content-Type: application/json' \
    -d "{\"jumphost\":\"${BEFORE_JH}\"}" >/dev/null || true
fi

echo "=== verify-fleet-sites-api OK ==="
