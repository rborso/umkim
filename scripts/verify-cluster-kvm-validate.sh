#!/usr/bin/env bash
# Negative test: cluster create with kubevirt provider on host without /dev/kvm should fail at Validated.
# Requires a non-KVM bootstrap target (set FLEET_NODE1 to such a host).
#
# Required env:
#   FLEET_SITE=<workload-site-id>
#   FLEET_NODE1=<bootstrap host without /dev/kvm>
set -euo pipefail

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_CLUSTER:-kvm-validate-fail-$$}"
SITE="${FLEET_SITE:?set FLEET_SITE}"
NODE1="${FLEET_NODE1:?set FLEET_NODE1 to a host without /dev/kvm}"
SSH_USER="${FLEET_SSH_USER:-linux}"
SSH_JUMP="${FLEET_SSH_JUMP:-linux@10.10.0.2}"
SKIP_CLEANUP="${FLEET_SKIP_CLEANUP:-0}"

WORKDIR="${TMPDIR:-/tmp}/cluster-kvm-validate-$$"
mkdir -p "${WORKDIR}"
trap 'rm -rf "${WORKDIR}"' EXIT

SSH_OPTS=(-o StrictHostKeyChecking=no)
[ -n "${SSH_JUMP}" ] && SSH_OPTS+=(-o "ProxyJump=${SSH_JUMP}")

run_on_node() { ssh "${SSH_OPTS[@]}" "${SSH_USER}@${NODE1}" "$@"; }
log() { echo "=== $* ==="; }

if [ "${SKIP_CLEANUP}" != "1" ]; then
  curl -sf -X DELETE "${API}/clusters/${CLUSTER}" >/dev/null 2>&1 || true
fi

log "create cluster with kubevirt provider on ${NODE1}"
curl -sf -X POST "${API}/clusters" -H 'Content-Type: application/json' -d "{
  \"clusterName\": \"${CLUSTER}\",
  \"siteId\": \"${SITE}\",
  \"bootstrapHost\": \"${NODE1}\",
  \"topology\": \"single_node\",
  \"networkMode\": \"wireguard\",
  \"nodes\": [{\"address\": \"${NODE1}\", \"role\": \"control\"}],
  \"providers\": { \"install\": { \"kubevirt\": true } }
}" >/dev/null

log "download bootstrap bundle"
for _ in $(seq 1 60); do
  http=$(curl -s -o "${WORKDIR}/bundle.tar.gz" -w '%{http_code}' "${API}/clusters/${CLUSTER}/bootstrap-bundle")
  [ "${http}" = "200" ] && [ -s "${WORKDIR}/bundle.tar.gz" ] && break
  sleep 2
done
[ -s "${WORKDIR}/bundle.tar.gz" ] || { echo "bootstrap bundle not ready"; exit 1; }

log "run bootstrap on node"
run_on_node "sudo wg-quick down wg0 2>/dev/null || true; sudo /usr/local/bin/k3s-uninstall.sh 2>/dev/null || true; sudo /usr/local/bin/k3s-agent-uninstall.sh 2>/dev/null || true"
scp "${SSH_OPTS[@]}" "${WORKDIR}/bundle.tar.gz" "${SSH_USER}@${NODE1}:/tmp/bootstrap-${CLUSTER}.tar.gz"
run_on_node "rm -rf /tmp/bootstrap-${CLUSTER} && mkdir -p /tmp/bootstrap-${CLUSTER} && tar -xzf /tmp/bootstrap-${CLUSTER}.tar.gz -C /tmp/bootstrap-${CLUSTER} && cd /tmp/bootstrap-${CLUSTER} && chmod +x bootstrap.sh && sudo bash ./bootstrap.sh"

log "wait for Failed or validation errors"
for _ in $(seq 1 120); do
  phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))" 2>/dev/null || echo "")
  if [ "${phase}" = "Failed" ]; then
    echo "=== verify-cluster-kvm-validate OK (Failed as expected) ==="
    exit 0
  fi
  sleep 5
done

echo "timeout: expected Failed phase for missing /dev/kvm"
exit 1
