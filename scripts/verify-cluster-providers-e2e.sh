#!/usr/bin/env bash
# E2E: console bootstrap cluster + install k8s/helm providers + catalog deploy.
# Prerequisites: hub phase5, dcm_seed_demo, dcm_seed_helm, harbor_push_fleet, harbor_push_charts.
# Provide nodes via env (FLEET_NODE1, optional FLEET_SSH_JUMP). See docs/e2e-full-lab-test.md.
#
# Required env:
#   FLEET_SITE=<workload-site-id>  # e.g. from fleet_sites inventory key
set -euo pipefail

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
DCM="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
CLUSTER="${FLEET_CLUSTER:-vbm-providers-e2e-$$}"
SITE="${FLEET_SITE:?set FLEET_SITE}"
NODE1="${FLEET_NODE1:-198.51.100.71}"
FLEET_TEST_KVM_PROVIDERS="${FLEET_TEST_KVM_PROVIDERS:-0}"
SSH_USER="${FLEET_SSH_USER:-linux}"
SSH_JUMP="${FLEET_SSH_JUMP:-linux@10.10.0.2}"
SKIP_CLEANUP="${FLEET_SKIP_CLEANUP:-0}"
SKIP_BUNDLE_RUN="${FLEET_SKIP_BUNDLE_RUN:-0}"
FLEET_TEST_DELETE="${FLEET_TEST_DELETE:-1}"

WORKDIR="${TMPDIR:-/tmp}/cluster-providers-e2e-$$"
mkdir -p "${WORKDIR}"
trap 'rm -rf "${WORKDIR}"' EXIT

SSH_OPTS=(-o StrictHostKeyChecking=no)
[ -n "${SSH_JUMP}" ] && SSH_OPTS+=(-o "ProxyJump=${SSH_JUMP}")

run_on_node() { ssh "${SSH_OPTS[@]}" "${SSH_USER}@${NODE1}" "$@"; }
log() { echo "=== $* ==="; }

wait_phase() {
  local want="$1" max="${2:-180}" i phase
  for ((i = 0; i < max; i += 5)); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))" 2>/dev/null || echo "")
    if [ "${phase}" = "${want}" ]; then
      if [ "${want}" = "Ready" ]; then
        curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get('available') and d.get('provisioning_phase')=='Ready' else 1)" 2>/dev/null && return 0
      else
        return 0
      fi
    fi
    sleep 5
  done
  echo "timeout waiting for phase ${want} (last=${phase:-unknown})"
  return 1
}

wait_provider() {
  local kind="$1" want="${2:-Ready}" max="${3:-120}" i phase
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
  echo "timeout waiting for provider ${kind} phase ${want} (last=${phase:-unknown})"
  return 1
}

provider_in_spm() {
  local name="$1"
  curl -sf "${DCM}/providers" | python3 -c "
import json,sys
for p in json.load(sys.stdin).get('providers',[]):
  if p.get('name')=='${name}' and p.get('health_status')=='ready':
    sys.exit(0)
sys.exit(1)
"
}

if [ "${SKIP_CLEANUP}" != "1" ]; then
  log "Step 0: cleanup stale cluster"
  curl -sf -X DELETE "${API}/clusters/${CLUSTER}" >/dev/null 2>&1 || true
  sudo kubectl delete fleetclusterjoin "${CLUSTER}" -n fleet-console --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sudo kubectl delete managedcluster "${CLUSTER}" --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sleep 5
fi

log "Step 1: create cluster ${CLUSTER} on ${NODE1}"
CREATE_JSON="{
  \"clusterName\": \"${CLUSTER}\",
  \"siteId\": \"${SITE}\",
  \"bootstrapHost\": \"${NODE1}\",
  \"topology\": \"single_node\",
  \"networkMode\": \"wireguard\",
  \"nodes\": [{\"address\": \"${NODE1}\", \"role\": \"control\"}]"
if [ "${FLEET_TEST_KVM_PROVIDERS}" = "1" ]; then
  CREATE_JSON="${CREATE_JSON},
  \"providers\": { \"install\": { \"kubevirt\": true, \"k8s\": true } }"
fi
CREATE_JSON="${CREATE_JSON}
}"
curl -sf -X POST "${API}/clusters" -H 'Content-Type: application/json' -d "${CREATE_JSON}" | tee "${WORKDIR}/create.json"

log "Step 2: download bootstrap bundle"
for _ in $(seq 1 60); do
  http=$(curl -s -o "${WORKDIR}/bundle.tar.gz" -w '%{http_code}' "${API}/clusters/${CLUSTER}/bootstrap-bundle")
  [ "${http}" = "200" ] && [ -s "${WORKDIR}/bundle.tar.gz" ] && break
  sleep 2
done
[ -s "${WORKDIR}/bundle.tar.gz" ] || { echo "bootstrap bundle not ready"; exit 1; }

if [ "${SKIP_BUNDLE_RUN}" != "1" ]; then
  log "Step 3: run bootstrap on VBM node"
  run_on_node "sudo wg-quick down wg0 2>/dev/null || true; sudo /usr/local/bin/k3s-uninstall.sh 2>/dev/null || true; sudo /usr/local/bin/k3s-agent-uninstall.sh 2>/dev/null || true"
  scp "${SSH_OPTS[@]}" "${WORKDIR}/bundle.tar.gz" "${SSH_USER}@${NODE1}:/tmp/bootstrap-${CLUSTER}.tar.gz"
  run_on_node "rm -rf /tmp/bootstrap-${CLUSTER} && mkdir -p /tmp/bootstrap-${CLUSTER} && tar -xzf /tmp/bootstrap-${CLUSTER}.tar.gz -C /tmp/bootstrap-${CLUSTER} && cd /tmp/bootstrap-${CLUSTER} && chmod +x bootstrap.sh && sudo bash ./bootstrap.sh"
  wait_phase Validated 240
fi

log "Step 4: hub provision"
curl -sf -X POST "${API}/clusters/${CLUSTER}/provision"
wait_phase Ready 300

log "Step 5: assert no k8s/helm providers in SPM yet"
! curl -sf "${DCM}/providers" | python3 -c "import json,sys; names=[p['name'] for p in json.load(sys.stdin).get('providers',[])]; sys.exit(0 if any(n in ('k8s-${CLUSTER}','helm-${CLUSTER}') for n in names) else 1)" \
  || { echo "unexpected k8s/helm provider before install"; exit 1; }

if [ "${FLEET_TEST_KVM_PROVIDERS}" = "1" ]; then
  log "Step 5b: wait auto kubevirt provider Ready"
  wait_provider kubevirt Ready 1200
  provider_in_spm "kubevirt-${CLUSTER}"
  log "Step 5c: wait auto k8s provider Ready"
  wait_provider k8s Ready 600
  provider_in_spm "k8s-${CLUSTER}"
  log "Step 6: skip manual k8s install (auto-installed)"
  SKIP_MANUAL_K8S=1
else
  SKIP_MANUAL_K8S=0
fi

log "Step 6: install k8s provider"
if [ "${SKIP_MANUAL_K8S:-0}" = "1" ]; then
  log "Step 6: skipped (auto-install path)"
else
curl -sf -X POST "${API}/clusters/${CLUSTER}/providers/k8s/install"
wait_provider k8s Ready 300
provider_in_spm "k8s-${CLUSTER}"
fi

log "Step 7: deploy demo-webapp container instance"
INST_K8S="demo-webapp-${CLUSTER}-$$"
PLACEMENT_MODE="${PLACEMENT_MODE:-policy}"
K8S_USER_VALUES="[
      { \"path\": \"metadata.name\", \"value\": \"${INST_K8S}\" },
      { \"path\": \"routing.mode\", \"value\": \"hub\" }"
if [ "${PLACEMENT_MODE}" = "pin" ]; then
  K8S_USER_VALUES="${K8S_USER_VALUES},
      { \"path\": \"routing.cluster\", \"value\": \"${CLUSTER}\" }"
fi
K8S_USER_VALUES="${K8S_USER_VALUES}
    ]"
K8S_LABEL_SELECTOR=""
if [ "${PLACEMENT_MODE}" != "pin" ]; then
  K8S_LABEL_SELECTOR="\"label_selector\": {\"fleet.site\": \"${SITE}\"},"
fi
RESP=$(curl -sf -X POST "${DCM}/catalog-item-instances" -H 'Content-Type: application/json' -d "{
  \"api_version\": \"v1alpha1\",
  \"display_name\": \"${INST_K8S}\",
  \"spec\": {
    ${K8S_LABEL_SELECTOR}
    \"catalog_item_id\": \"demo-webapp\",
    \"user_values\": ${K8S_USER_VALUES}
  }
}")
INST_K8S_ID=$(echo "${RESP}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('id') or d.get('uid') or '')")
[ -n "${INST_K8S_ID}" ] || { echo "no k8s instance id"; exit 1; }
sleep 20

log "Step 8: install helm provider"
curl -sf -X POST "${API}/clusters/${CLUSTER}/providers/helm/install"
wait_provider helm Ready 300
provider_in_spm "helm-${CLUSTER}"

log "Step 9: deploy whoami-helm instance"
INST_HELM="whoami-helm-${CLUSTER}-$$"
HELM_USER_VALUES="[
      { \"path\": \"metadata.name\", \"value\": \"${INST_HELM}\" },
      { \"path\": \"release.namespace\", \"value\": \"default\" }"
if [ "${PLACEMENT_MODE}" = "pin" ]; then
  HELM_USER_VALUES="${HELM_USER_VALUES},
      { \"path\": \"routing.cluster\", \"value\": \"${CLUSTER}\" }"
fi
HELM_USER_VALUES="${HELM_USER_VALUES}
    ]"
HELM_LABEL_SELECTOR=""
if [ "${PLACEMENT_MODE}" != "pin" ]; then
  HELM_LABEL_SELECTOR="\"label_selector\": {\"fleet.site\": \"${SITE}\"},"
fi
RESP=$(curl -sf -X POST "${DCM}/catalog-item-instances" -H 'Content-Type: application/json' -d "{
  \"api_version\": \"v1alpha1\",
  \"display_name\": \"${INST_HELM}\",
  \"spec\": {
    ${HELM_LABEL_SELECTOR}
    \"catalog_item_id\": \"whoami-helm\",
    \"user_values\": ${HELM_USER_VALUES}
  }
}")
INST_HELM_ID=$(echo "${RESP}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('id') or d.get('uid') or '')")
[ -n "${INST_HELM_ID}" ] || { echo "no helm instance id"; exit 1; }
sleep 15
curl -sf "${DCM}/catalog-item-instances/${INST_HELM_ID}/helm/history" | python3 -c "import json,sys; json.load(sys.stdin)"

log "Step 10: delete instances"
curl -sf -X DELETE "${DCM}/catalog-item-instances/${INST_K8S_ID}" || true
curl -sf -X DELETE "${DCM}/catalog-item-instances/${INST_HELM_ID}" || true

log "Step 11: delete providers"
curl -sf -X POST "${API}/clusters/${CLUSTER}/providers/helm/delete"
wait_provider helm NotInstalled 180
curl -sf -X POST "${API}/clusters/${CLUSTER}/providers/k8s/delete"
wait_provider k8s NotInstalled 180

if [ "${FLEET_TEST_DELETE}" = "1" ]; then
  log "Step 12: delete cluster"
  DHTTP=$(curl -s -o "${WORKDIR}/delete.json" -w '%{http_code}' -X DELETE "${API}/clusters/${CLUSTER}")
  [ "${DHTTP}" = "202" ] || [ "${DHTTP}" = "204" ] || { echo "DELETE failed ${DHTTP}"; exit 1; }
fi

echo "=== verify-cluster-providers-e2e OK ==="
