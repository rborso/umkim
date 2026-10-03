#!/usr/bin/env bash
# Verify FleetClusterJoin API: create, list phase, bootstrap download, cleanup.
set -euo pipefail
API="${FLEET_SITE_API_DIRECT:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}"
TEST_CLUSTER="${FLEET_TEST_CLUSTER:-verify-join-$$}"
TEST_SITE="${FLEET_TEST_SITE:-homelab}"
FAKE_HOST="${FLEET_TEST_BOOTSTRAP_HOST:-192.0.2.50}"

WORKDIR="${TMPDIR:-/tmp}/fcj-verify-$$"
mkdir -p "${WORKDIR}"
trap 'rm -rf "${WORKDIR}"' EXIT

tar_list() { tar -tzf "$1" 2>/dev/null; }
tar_has() {
  local listing
  listing="$(tar -tzf "$1" 2>/dev/null || true)"
  grep -Fxq "$2" <<<"${listing}"
}

wait_script_issued() {
  local phase=""
  for _ in $(seq 1 60); do
    phase="$(curl -sf "${API}/clusters/${TEST_CLUSTER}" 2>/dev/null \
      | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))" 2>/dev/null || true)"
    [[ "${phase}" == "ScriptIssued" || "${phase}" == "Enrolled" ]] && return 0
    sleep 2
  done
  echo "WARN: cluster ${TEST_CLUSTER} phase=${phase:-unknown} (continuing bundle poll)"
}

bundle_ready() {
  [[ -s "${WORKDIR}/bundle.tar.gz" ]] \
    && gzip -t "${WORKDIR}/bundle.tar.gz" 2>/dev/null \
    && tar_has "${WORKDIR}/bundle.tar.gz" bootstrap.sh \
    && tar_has "${WORKDIR}/bundle.tar.gz" wireguard-install.yml \
    && tar_has "${WORKDIR}/bundle.tar.gz" cluster-provision-local.yml
}

echo "=== verify-fleet-cluster-join (cluster=${TEST_CLUSTER}) ==="
curl -sf "${API}/clusters" >/dev/null || { echo "Fleet API unreachable at ${API}"; exit 1; }

CREATE_BODY=$(cat <<EOF
{"clusterName":"${TEST_CLUSTER}","siteId":"${TEST_SITE}","bootstrapHost":"${FAKE_HOST}","topology":"single_node","networkMode":"wireguard","minRootDiskGi":40,"clusterRegion":"fleet","nodes":[{"address":"${FAKE_HOST}","hostname":"${TEST_CLUSTER}","role":"control"}]}
EOF
)

HTTP=$(curl -s -o "${WORKDIR}/create.json" -w '%{http_code}' -X POST "${API}/clusters" \
  -H 'Content-Type: application/json' -d "${CREATE_BODY}")
[ "${HTTP}" = "201" ] || { echo "POST /clusters expected 201 got ${HTTP}: $(cat "${WORKDIR}/create.json")"; exit 1; }

for _ in $(seq 1 15); do
  LIST=$(curl -sf "${API}/clusters")
  if echo "${LIST}" | grep -q "\"name\":\"${TEST_CLUSTER}\""; then
    break
  fi
  sleep 1
done
echo "${LIST}" | grep -q "\"name\":\"${TEST_CLUSTER}\"" || { echo "cluster missing from list"; exit 1; }
echo "${LIST}" | grep -q "provisioning_phase" || { echo "expected provisioning_phase in list"; exit 1; }

wait_script_issued

for _ in $(seq 1 90); do
  BZ_HTTP=$(curl -s -o "${WORKDIR}/bundle.tar.gz" -w '%{http_code}' "${API}/clusters/${TEST_CLUSTER}/bootstrap-bundle")
  if [ "${BZ_HTTP}" = "200" ] && bundle_ready; then
    break
  fi
  sleep 3
done
[ "${BZ_HTTP}" = "200" ] || { echo "bootstrap-bundle expected 200 got ${BZ_HTTP}"; exit 1; }
bundle_ready || { echo "bundle missing cluster-provision-local.yml (size=$(wc -c < "${WORKDIR}/bundle.tar.gz" 2>/dev/null || echo 0))"; exit 1; }

if [ "${FLEET_TEST_BUNDLE:-0}" = "1" ]; then
  BUNDLE_CLUSTER="${FLEET_BUNDLE_TEST_CLUSTER:-verify-bundle-$$}"
  BUNDLE_VIP="${FLEET_TEST_CONTROL_PLANE_VIP:-192.0.2.54}"
  BUNDLE_BODY=$(cat <<EOF
{"clusterName":"${BUNDLE_CLUSTER}","siteId":"${TEST_SITE}","bootstrapHost":"192.0.2.51","controlPlaneVip":"${BUNDLE_VIP}","topology":"ha_3_node","networkMode":"wireguard","minRootDiskGi":40,"clusterRegion":"fleet","storage":{"longhorn":{"enabled":true,"device":"/dev/sdb"}},"nodes":[{"address":"192.0.2.51","hostname":"${BUNDLE_CLUSTER}-1","role":"control"},{"address":"192.0.2.52","hostname":"${BUNDLE_CLUSTER}-2","role":"control"},{"address":"192.0.2.53","hostname":"${BUNDLE_CLUSTER}-3","role":"control"}]}
EOF
)
  BHTTP=$(curl -s -o "${WORKDIR}/bundle-create.json" -w '%{http_code}' -X POST "${API}/clusters" \
    -H 'Content-Type: application/json' -d "${BUNDLE_BODY}")
  [ "${BHTTP}" = "201" ] || { echo "bundle test POST expected 201 got ${BHTTP}: $(cat "${WORKDIR}/bundle-create.json")"; exit 1; }
  for _ in $(seq 1 30); do
    BZ_HTTP=$(curl -s -o "${WORKDIR}/bundle-ha.tar.gz" -w '%{http_code}' "${API}/clusters/${BUNDLE_CLUSTER}/bootstrap-bundle")
    if [ "${BZ_HTTP}" = "200" ] && tar_has "${WORKDIR}/bundle-ha.tar.gz" bootstrap.sh && tar_has "${WORKDIR}/bundle-ha.tar.gz" ssh/id_rsa; then
      break
    fi
    sleep 2
  done
  [ "${BZ_HTTP}" = "200" ] || { echo "ha bootstrap-bundle expected 200 got ${BZ_HTTP}"; exit 1; }
  for n in 1 2 3; do
    tar_has "${WORKDIR}/bundle-ha.tar.gz" "wireguard/${BUNDLE_CLUSTER}-${n}/wg-umkim.conf" || {
      echo "bundle missing wireguard/${BUNDLE_CLUSTER}-${n}/wg-umkim.conf"
      tar_list "${WORKDIR}/bundle-ha.tar.gz" | grep wireguard || true
      exit 1
    }
  done
  mkdir -p "${WORKDIR}/bundle-ha"
  tar -xzf "${WORKDIR}/bundle-ha.tar.gz" -C "${WORKDIR}/bundle-ha" inventory.ini
  grep -q "k3s_control_plane_vip=${BUNDLE_VIP}" "${WORKDIR}/bundle-ha/inventory.ini" || {
    echo "bundle inventory missing k3s_control_plane_vip=${BUNDLE_VIP}"
    exit 1
  }
  grep -q "fleet_cluster_longhorn_enabled=true" "${WORKDIR}/bundle-ha/inventory.ini" || {
    echo "bundle inventory missing fleet_cluster_longhorn_enabled=true"
    exit 1
  }
  kubectl delete fleetclusterjoin "${BUNDLE_CLUSTER}" -n fleet-console --ignore-not-found --wait=false 2>/dev/null || true
fi

if [ "${FLEET_TEST_SINGLE_BUNDLE:-0}" = "1" ]; then
  SINGLE_CLUSTER="${FLEET_SINGLE_TEST_CLUSTER:-verify-single-$$}"
  SINGLE_BODY=$(cat <<EOF
{"clusterName":"${SINGLE_CLUSTER}","siteId":"${TEST_SITE}","bootstrapHost":"192.0.2.60","topology":"single_node","networkMode":"wireguard","minRootDiskGi":40,"clusterRegion":"fleet","nodes":[{"address":"192.0.2.60","hostname":"${SINGLE_CLUSTER}","role":"control"}]}
EOF
)
  SHTTP=$(curl -s -o "${WORKDIR}/single-create.json" -w '%{http_code}' -X POST "${API}/clusters" \
    -H 'Content-Type: application/json' -d "${SINGLE_BODY}")
  [ "${SHTTP}" = "201" ] || { echo "single bundle POST expected 201 got ${SHTTP}"; exit 1; }
  for _ in $(seq 1 30); do
    SZ_HTTP=$(curl -s -o "${WORKDIR}/bundle-single.tar.gz" -w '%{http_code}' "${API}/clusters/${SINGLE_CLUSTER}/bootstrap-bundle")
    if [ "${SZ_HTTP}" = "200" ] && tar_has "${WORKDIR}/bundle-single.tar.gz" wireguard-install.yml; then
      break
    fi
    sleep 2
  done
  [ "${SZ_HTTP}" = "200" ] || { echo "single bootstrap-bundle expected 200 got ${SZ_HTTP}"; exit 1; }
  kubectl delete fleetclusterjoin "${SINGLE_CLUSTER}" -n fleet-console --ignore-not-found --wait=false 2>/dev/null || true
fi

GET=$(curl -sf "${API}/clusters/${TEST_CLUSTER}")
echo "${GET}" | grep -q "\"provisioning_phase\"" || { echo "GET missing provisioning_phase"; exit 1; }

echo "DELETE /clusters/${TEST_CLUSTER}"
DHTTP=$(curl -s -o "${WORKDIR}/delete.json" -w '%{http_code}' -X DELETE "${API}/clusters/${TEST_CLUSTER}")
[ "${DHTTP}" = "202" ] || [ "${DHTTP}" = "204" ] || {
  echo "DELETE expected 202 or 204 got ${DHTTP}: $(cat "${WORKDIR}/delete.json")"
  exit 1
}

for _ in $(seq 1 60); do
  LIST2=$(curl -sf "${API}/clusters")
  if ! echo "${LIST2}" | grep -q "\"name\":\"${TEST_CLUSTER}\""; then
    break
  fi
  sleep 2
done
if echo "${LIST2}" | grep -q "\"name\":\"${TEST_CLUSTER}\""; then
  echo "cluster still listed after DELETE timeout"
  kubectl get fleetclusterjoin "${TEST_CLUSTER}" -n fleet-console 2>/dev/null || true
  exit 1
fi
kubectl get fleetclusterjoin "${TEST_CLUSTER}" -n fleet-console 2>/dev/null && {
  echo "FleetClusterJoin still exists after DELETE"
  exit 1
} || true

kubectl get secret "fleet-cluster-${TEST_CLUSTER}-bootstrap" -n fleet-console 2>/dev/null && {
  echo "bootstrap secret still exists after DELETE"
  exit 1
} || true

MGMT="${FLEET_MANAGEMENT_CLUSTER_NAME:-umkim-hub}"
MCODE=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${API}/clusters/${MGMT}")
[ "${MCODE}" = "403" ] || echo "WARN: DELETE management cluster expected 403 got ${MCODE}"

echo "=== verify-fleet-cluster-join OK ==="
