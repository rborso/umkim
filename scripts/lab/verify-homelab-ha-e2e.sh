#!/usr/bin/env bash
# E2E: homelab-ha (virt02-ha-cp1..3) console bootstrap + hub Provision.
# Defaults from inventory/. Run from hub or control host with kubectl + SSH jump.
set -euo pipefail

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_CLUSTER:-homelab-ha}"
SITE="${FLEET_SITE:-homelab}"
NODE1="${FLEET_NODE1:-192.168.1.81}"
NODE2="${FLEET_NODE2:-192.168.1.82}"
NODE3="${FLEET_NODE3:-192.168.1.83}"
HOST1="${FLEET_HOST1:-virt02-ha-cp1}"
HOST2="${FLEET_HOST2:-virt02-ha-cp2}"
HOST3="${FLEET_HOST3:-virt02-ha-cp3}"
VIP="${FLEET_CONTROL_PLANE_VIP:-198.51.100.84}"
LONGHORN_DEV="${FLEET_LONGHORN_DEVICE:-/dev/vdb}"
SSH_USER="${FLEET_SSH_USER:-linux}"
SKIP_CLEANUP="${FLEET_SKIP_CLEANUP:-0}"
SKIP_BUNDLE_RUN="${FLEET_SKIP_BUNDLE_RUN:-0}"
AUTO_PROVISION="${FLEET_AUTO_PROVISION:-1}"
TEST_DELETE="${FLEET_TEST_DELETE:-0}"

WORKDIR="${TMPDIR:-/tmp}/homelab-ha-e2e-$$"
mkdir -p "${WORKDIR}"
trap 'rm -rf "${WORKDIR}"' EXIT

tar_list() { tar -tzf "$1" 2>/dev/null; }
tar_has() { tar -tzf "$1" "$2" >/dev/null 2>&1; }

SSH_JUMP="${FLEET_SSH_JUMP:-linux@jumphost.example.com}"
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null)
if [ -n "${SSH_JUMP}" ]; then
  SSH_OPTS+=(-o "ProxyJump=${SSH_JUMP}")
fi

run_on_node1() {
  ssh "${SSH_OPTS[@]}" "${SSH_USER}@${NODE1}" "$@"
}

copy_to_node1() {
  scp "${SSH_OPTS[@]}" "$1" "${SSH_USER}@${NODE1}:$2"
}

log() { echo "=== $* ==="; }

wait_phase() {
  local want="$1" max="${2:-120}" i phase avail
  for ((i = 0; i < max; i += 5)); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}" | sed -n 's/.*"provisioning_phase":"\([^"]*\)".*/\1/p' | head -1)
    if [ "${want}" = "Ready" ] && [ -z "${phase}" ]; then
      avail=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('available', False))" 2>/dev/null || echo false)
      if [ "${avail}" = "True" ]; then
        echo "phase=Ready"
        return 0
      fi
    fi
    if [ "${phase}" = "${want}" ]; then
      echo "phase=${phase}"
      return 0
    fi
    sleep 5
  done
  echo "timeout waiting for phase ${want} (last=${phase:-unknown})"
  return 1
}

poll_bundle() {
  local i http host
  for i in $(seq 1 60); do
    http=$(curl -s -o "${WORKDIR}/bundle.tar.gz" -w '%{http_code}' "${API}/clusters/${CLUSTER}/bootstrap-bundle")
    if [ "${http}" = "200" ] && [ -s "${WORKDIR}/bundle.tar.gz" ]; then
      tar_has "${WORKDIR}/bundle.tar.gz" bootstrap.sh || continue
      tar_has "${WORKDIR}/bundle.tar.gz" wireguard-install.yml || continue
      tar_has "${WORKDIR}/bundle.tar.gz" cluster-provision-local.yml || continue
      tar_has "${WORKDIR}/bundle.tar.gz" ssh/id_rsa || continue
      for host in "${HOST1}" "${HOST2}" "${HOST3}"; do
        tar_has "${WORKDIR}/bundle.tar.gz" "wireguard/${host}/wg-umkim.conf" || continue 2
      done
      return 0
    fi
    sleep 2
  done
  echo "bootstrap-bundle not ready (HTTP ${http:-unknown})"
  return 1
}

log "Prereq: SSH to control nodes"
for ip in "${NODE1}" "${NODE2}" "${NODE3}"; do
  ssh "${SSH_OPTS[@]}" "${SSH_USER}@${ip}" 'hostname' >/dev/null
done

log "Prereq: clear stale control-plane VIP if left on a node"
for ip in "${NODE1}" "${NODE2}" "${NODE3}"; do
  ssh "${SSH_OPTS[@]}" "${SSH_USER}@${ip}" "sudo ip addr del ${VIP}/32 dev eth0 2>/dev/null || true"
done

if [ "${SKIP_CLEANUP}" != "1" ]; then
  log "Step 0: cleanup prior cluster"
  curl -s -o "${WORKDIR}/del.json" -w '%{http_code}' -X DELETE \
    "${API}/clusters/${CLUSTER}?cleanupKubernetes=1" >/dev/null || true
  sudo kubectl delete fleetclusterjoin "${CLUSTER}" -n fleet-console --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sudo kubectl delete managedcluster "${CLUSTER}" --ignore-not-found --wait=true --timeout=120s 2>/dev/null || true
  sleep 5
fi

log "Step 1: create ${CLUSTER} ha_3_node with VIP and Longhorn"
CREATE_BODY=$(cat <<EOF
{"clusterName":"${CLUSTER}","siteId":"${SITE}","bootstrapHost":"${NODE1}","controlPlaneVip":"${VIP}","topology":"ha_3_node","networkMode":"wireguard","minRootDiskGi":35,"clusterRegion":"homelab","clusterCapabilities":{"ha_topology":true},"storage":{"longhorn":{"enabled":true,"device":"${LONGHORN_DEV}"},"localPath":{"enabled":false}},"nodes":[{"address":"${NODE1}","hostname":"${HOST1}","role":"control"},{"address":"${NODE2}","hostname":"${HOST2}","role":"control"},{"address":"${NODE3}","hostname":"${HOST3}","role":"control"}]}
EOF
)
HTTP=$(curl -s -o "${WORKDIR}/create.json" -w '%{http_code}' -X POST "${API}/clusters" \
  -H 'Content-Type: application/json' -d "${CREATE_BODY}")
if [ "${HTTP}" != "201" ] && [ "${HTTP}" != "409" ]; then
  echo "POST /clusters expected 201 or 409 got ${HTTP}: $(cat "${WORKDIR}/create.json")"
  exit 1
fi
sleep 5

log "Step 2: download bootstrap bundle"
poll_bundle
for host in "${HOST1}" "${HOST2}" "${HOST3}"; do
  tar_has "${WORKDIR}/bundle.tar.gz" "wireguard/${host}/wg-umkim.conf" || {
    echo "bundle missing wireguard/${host}/wg-umkim.conf"
    tar_list "${WORKDIR}/bundle.tar.gz" | grep wireguard || true
    exit 1
  }
done
mkdir -p "${WORKDIR}/bundle"
tar -xzf "${WORKDIR}/bundle.tar.gz" -C "${WORKDIR}/bundle" inventory.ini
grep -q "k3s_control_plane_vip=${VIP}" "${WORKDIR}/bundle/inventory.ini" || {
  echo "inventory missing k3s_control_plane_vip=${VIP}"
  exit 1
}

log "Step 3: run bundle on ${NODE1}"
if [ "${SKIP_BUNDLE_RUN}" != "1" ]; then
  copy_to_node1 "${WORKDIR}/bundle.tar.gz" "/tmp/bootstrap-${CLUSTER}.tar.gz"
  run_on_node1 bash -s <<EOF
set -euo pipefail
rm -rf "/tmp/bootstrap-${CLUSTER}"
mkdir -p "/tmp/bootstrap-${CLUSTER}"
tar -xzf "/tmp/bootstrap-${CLUSTER}.tar.gz" -C "/tmp/bootstrap-${CLUSTER}"
chmod +x "/tmp/bootstrap-${CLUSTER}/bootstrap.sh"
cd "/tmp/bootstrap-${CLUSTER}"
sudo bash ./bootstrap.sh
EOF
fi

log "Step 4: expect Validated"
wait_phase Validated 360

log "Step 5: verify K3s nodes and API VIP on ${NODE1}"
run_on_node1 'sudo /usr/local/bin/k3s kubectl get nodes -o wide'
ready=$(run_on_node1 'sudo /usr/local/bin/k3s kubectl get nodes --no-headers 2>/dev/null | grep -c " Ready" || true')
[ "${ready}" = "3" ] || { echo "expected 3 Ready nodes, got ${ready}"; exit 1; }
run_on_node1 "curl -sk --connect-timeout 5 https://${VIP}:6443/version | grep -qE 'major|Unauthorized'"

log "Step 6: verify Longhorn"
run_on_node1 'sudo /usr/local/bin/k3s kubectl -n longhorn-system get pods'
run_on_node1 'sudo /usr/local/bin/k3s kubectl get storageclass longhorn' >/dev/null

if [ "${AUTO_PROVISION}" = "1" ]; then
  log "Step 7: hub Provision"
  HTTP=$(curl -s -o "${WORKDIR}/provision.json" -w '%{http_code}' -X POST "${API}/clusters/${CLUSTER}/provision")
  [ "${HTTP}" = "202" ] || { echo "provision expected 202 got ${HTTP}: $(cat "${WORKDIR}/provision.json")"; exit 1; }
  log "Step 8: wait Ready"
  wait_phase Ready 600
fi

log "Step 9: hub checks"
sudo kubectl get managedcluster "${CLUSTER}" -o wide
curl -sf -X POST "${API}/clusters/${CLUSTER}/discover-capabilities" >/dev/null || true

if [ "${TEST_DELETE}" = "1" ]; then
  log "Step 10: delete cluster"
  DHTTP=$(curl -s -o "${WORKDIR}/delete.json" -w '%{http_code}' -X DELETE \
    "${API}/clusters/${CLUSTER}?cleanupKubernetes=1")
  [ "${DHTTP}" = "202" ] || [ "${DHTTP}" = "204" ] || {
    echo "DELETE expected 202/204 got ${DHTTP}: $(cat "${WORKDIR}/delete.json")"
    exit 1
  }
fi

echo "=== verify-homelab-ha-e2e OK ==="
