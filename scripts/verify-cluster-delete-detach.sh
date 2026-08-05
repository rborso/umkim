#!/usr/bin/env bash
# E2E: FCJ detach delete — keeps K3s + user pods; removes fleet glue.
# Prerequisites: hub phase5, dcm_seed_demo, harbor_push_fleet, enrolled cluster node.
#
# Required env:
#   FLEET_SITE=<workload-site-id>
#   FLEET_NODE1=<node IP reachable via SSH>
#
# Optional:
#   FLEET_CLUSTER, FLEET_SSH_USER, FLEET_SSH_JUMP, FLEET_SKIP_CLEANUP, FLEET_SKIP_BUNDLE_RUN
#   FLEET_MANUAL_CLEANUP=1  — leave cluster on node for manual inspection (default 0)
set -euo pipefail

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
DCM="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
CLUSTER="${FLEET_CLUSTER:-detach-e2e-$$}"
SITE="${FLEET_SITE:?set FLEET_SITE}"
NODE1="${FLEET_NODE1:?set FLEET_NODE1}"
SSH_USER="${FLEET_SSH_USER:-linux}"
SSH_JUMP="${FLEET_SSH_JUMP:-}"
SKIP_CLEANUP="${FLEET_SKIP_CLEANUP:-0}"
SKIP_BUNDLE_RUN="${FLEET_SKIP_BUNDLE_RUN:-0}"
MANUAL_CLEANUP="${FLEET_MANUAL_CLEANUP:-0}"

WORKDIR="${TMPDIR:-/tmp}/cluster-delete-detach-$$"
mkdir -p "${WORKDIR}"
trap 'rm -rf "${WORKDIR}"' EXIT

SSH_OPTS=(-o StrictHostKeyChecking=no)
[ -n "${SSH_JUMP}" ] && SSH_OPTS+=(-o "ProxyJump=${SSH_JUMP}")

run_on_node() { ssh "${SSH_OPTS[@]}" "${SSH_USER}@${NODE1}" "$@"; }
log() { echo "=== $* ==="; }

wait_phase() {
  local want="$1" max="${2:-300}" i phase
  for ((i = 0; i < max; i += 5)); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))" 2>/dev/null || echo "")
    if [ "${phase}" = "${want}" ]; then
      return 0
    fi
    sleep 5
  done
  echo "timeout waiting for phase ${want} (last=${phase:-unknown})"
  return 1
}

wait_provider() {
  local kind="$1" want="${2:-Ready}" max="${3:-300}" i phase
  for ((i = 0; i < max; i += 5)); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}/providers" | python3 -c "
import json,sys
for p in json.load(sys.stdin).get('providers',[]):
  if p.get('kind')=='${kind}':
    print(p.get('phase',''))
    sys.exit(0)
print('')
")
    if [ "${phase}" = "${want}" ]; then
      return 0
    fi
    sleep 5
  done
  echo "timeout waiting for provider ${kind} phase ${want}"
  return 1
}

provider_in_spm() {
  curl -sf "${DCM}/providers" | python3 -c "
import json,sys
for p in json.load(sys.stdin).get('providers',[]):
  if p.get('name')=='${1}':
    sys.exit(0)
sys.exit(1)
"
}

wait_delete_done() {
  for _ in $(seq 1 90); do
    if ! curl -sf "${API}/clusters" | grep -q "\"name\":\"${CLUSTER}\""; then
      return 0
    fi
    sleep 2
  done
  echo "cluster still listed after DELETE"
  return 1
}

if [ "${SKIP_CLEANUP}" != "1" ]; then
  log "Step 0: cleanup stale cluster (full reset)"
  curl -sf -X DELETE "${API}/clusters/${CLUSTER}?cleanupKubernetes=1" >/dev/null 2>&1 || true
  sudo kubectl delete fleetclusterjoin "${CLUSTER}" -n fleet-console --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sudo kubectl delete managedcluster "${CLUSTER}" --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sleep 5
fi

log "Step 1: create cluster ${CLUSTER}"
CREATE_JSON="{
  \"clusterName\": \"${CLUSTER}\",
  \"siteId\": \"${SITE}\",
  \"bootstrapHost\": \"${NODE1}\",
  \"topology\": \"single_node\",
  \"networkMode\": \"wireguard\",
  \"nodes\": [{\"address\": \"${NODE1}\", \"role\": \"control\"}]
}"
curl -sf -X POST "${API}/clusters" -H 'Content-Type: application/json' -d "${CREATE_JSON}"

log "Step 2: bootstrap bundle"
for _ in $(seq 1 60); do
  http=$(curl -s -o "${WORKDIR}/bundle.tar.gz" -w '%{http_code}' "${API}/clusters/${CLUSTER}/bootstrap-bundle")
  [ "${http}" = "200" ] && [ -s "${WORKDIR}/bundle.tar.gz" ] && break
  sleep 2
done
[ -s "${WORKDIR}/bundle.tar.gz" ] || { echo "bootstrap bundle not ready"; exit 1; }

if [ "${SKIP_BUNDLE_RUN}" != "1" ]; then
  log "Step 3: run bootstrap on node"
  scp "${SSH_OPTS[@]}" "${WORKDIR}/bundle.tar.gz" "${SSH_USER}@${NODE1}:/tmp/bootstrap-${CLUSTER}.tar.gz"
  run_on_node "rm -rf /tmp/bootstrap-${CLUSTER} && mkdir -p /tmp/bootstrap-${CLUSTER} && tar -xzf /tmp/bootstrap-${CLUSTER}.tar.gz -C /tmp/bootstrap-${CLUSTER} && cd /tmp/bootstrap-${CLUSTER} && chmod +x bootstrap.sh && sudo bash ./bootstrap.sh"
  wait_phase Validated 240
fi

log "Step 4: hub provision"
curl -sf -X POST "${API}/clusters/${CLUSTER}/provision"
wait_phase Ready 300

log "Step 5: install k8s provider"
curl -sf -X POST "${API}/clusters/${CLUSTER}/providers/k8s/install"
wait_provider k8s Ready 300
provider_in_spm "k8s-${CLUSTER}"

log "Step 6: deploy demo-webapp"
INST="demo-webapp-${CLUSTER}-detach"
RESP=$(curl -sf -X POST "${DCM}/catalog-item-instances" -H 'Content-Type: application/json' -d "{
  \"api_version\": \"v1alpha1\",
  \"display_name\": \"${INST}\",
  \"spec\": {
    \"label_selector\": {\"fleet.site\": \"${SITE}\"},
    \"catalog_item_id\": \"demo-webapp\",
    \"user_values\": [
      {\"path\": \"metadata.name\", \"value\": \"${INST}\"},
      {\"path\": \"routing.mode\", \"value\": \"hub\"}
    ]
  }
}")
INST_ID=$(echo "${RESP}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('id') or d.get('uid') or '')")
[ -n "${INST_ID}" ] || { echo "catalog instance create failed: ${RESP}"; exit 1; }

for _ in $(seq 1 60); do
  phase=$(curl -sf "${DCM}/catalog-item-instances/${INST_ID}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',{}).get('phase',''))" 2>/dev/null || echo "")
  [ "${phase}" = "Ready" ] && break
  sleep 5
done
[ "${phase}" = "Ready" ] || { echo "catalog instance not Ready (last=${phase:-unknown})"; exit 1; }

run_on_node 'sudo /usr/local/bin/k3s kubectl get pods -A' | tee "${WORKDIR}/pods-before.txt"
grep -q "${INST}" "${WORKDIR}/pods-before.txt" || { echo "demo-webapp pod not found before delete"; exit 1; }

log "Step 7: DELETE without cleanupKubernetes (detach)"
DHTTP=$(curl -s -o "${WORKDIR}/delete.json" -w '%{http_code}' -X DELETE "${API}/clusters/${CLUSTER}")
[ "${DHTTP}" = "202" ] || [ "${DHTTP}" = "204" ] || {
  echo "DELETE expected 202/204 got ${DHTTP}: $(cat "${WORKDIR}/delete.json")"
  exit 1
}
wait_delete_done

log "Step 8: assert hub teardown"
sudo kubectl get managedcluster "${CLUSTER}" 2>/dev/null && {
  echo "ManagedCluster still exists"
  exit 1
} || true
sudo kubectl get manifestwork -A 2>/dev/null | grep -q "${CLUSTER}" && {
  echo "ManifestWork still references cluster"
  exit 1
} || true
kubectl get secret "fleet-cluster-${CLUSTER}-bootstrap" -n fleet-console 2>/dev/null && {
  echo "bootstrap secret still exists"
  exit 1
} || true
! provider_in_spm "k8s-${CLUSTER}" || {
  echo "k8s provider still registered in SPM"
  exit 1
}

log "Step 9: assert node detach — K3s up, wg-umkim down, app pod still running"
run_on_node 'sudo /usr/local/bin/k3s kubectl get nodes --no-headers' | grep -q Ready || {
  echo "K3s nodes not Ready after detach"
  exit 1
}
run_on_node 'ip link show wg-umkim' 2>/dev/null && {
  echo "wg-umkim still present after detach"
  exit 1
} || true
run_on_node 'sudo /usr/local/bin/k3s kubectl get pods -A' | tee "${WORKDIR}/pods-after.txt"
grep -q "${INST}" "${WORKDIR}/pods-after.txt" || {
  echo "demo-webapp pod missing after detach (expected still running)"
  exit 1
}

if [ "${MANUAL_CLEANUP}" = "1" ]; then
  log "Manual cleanup: on node run: sudo cleanup_node_local.sh --cluster ${CLUSTER} --mode reset"
else
  log "Step 10: full node reset after test"
  run_on_node "command -v k3s-uninstall.sh >/dev/null && sudo k3s-uninstall.sh || true; sudo ip link del wg-umkim 2>/dev/null || true"
fi

echo "=== verify-cluster-delete-detach OK ==="
