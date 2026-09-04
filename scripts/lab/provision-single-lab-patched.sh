#!/usr/bin/env bash
# Create single-lab with overlay IP .14 (avoid HA .11 collision) until controller fix is deployed.
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER=single-lab
NODE=192.168.1.92
HOST=virt03-single-01
WG_IP=10.10.0.14
SSH_JUMP="${FLEET_SSH_JUMP:-linux@10.10.0.10}"
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o "ProxyJump=${SSH_JUMP}")

wait_phase() {
  local want="$1" max="${2:-120}" i phase avail
  for ((i = 0; i < max; i += 5)); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('provisioning_phase',''))")
    avail=$(curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('available',False))")
    [ "${phase}" = "${want}" ] && return 0
    [ "${want}" = "Ready" ] && [ "${phase}" = "Ready" ] && [ "${avail}" = "True" ] && return 0
    [ "${phase}" = "Failed" ] && return 1
    sleep 5
  done
  return 1
}

echo "=== create ${CLUSTER} ==="
curl -sf -X POST "${API}/clusters" -H 'Content-Type: application/json' -d "{
  \"clusterName\":\"${CLUSTER}\",\"siteId\":\"homelab\",\"bootstrapHost\":\"${NODE}\",
  \"topology\":\"single_node\",\"networkMode\":\"wireguard\",\"clusterRegion\":\"homelab\",
  \"nodes\":[{\"address\":\"${NODE}\",\"hostname\":\"${HOST}\",\"role\":\"control\"}]
}" >/dev/null || true
sleep 5

echo "=== patch bootstrap secret to ${WG_IP} ==="
sudo kubectl -n fleet-console get secret "fleet-cluster-${CLUSTER}-bootstrap" -o json > "/tmp/${CLUSTER}-bootstrap-raw.json"
python3 <<PY
import json
s=json.load(open("/tmp/${CLUSTER}-bootstrap-raw.json"))
nodes=json.loads(s["data"]["nodes-wg.json"])
nodes[0]["wgAddress"]="${WG_IP}/32"
s["data"]["nodes-wg.json"]=json.dumps(nodes).encode()
conf=s["data"]["bootstrap.sh"].decode()
conf=conf.replace("10.10.0.11","${WG_IP}").replace("10.10.0.10","${WG_IP}")
s["data"]["bootstrap.sh"]=conf.encode()
json.dump(s, open("/tmp/${CLUSTER}-bootstrap.json","w"))
PY
sudo kubectl apply -f "/tmp/${CLUSTER}-bootstrap.json"
sudo kubectl -n fleet-console patch fleetclusterjoin "${CLUSTER}" --subresource=status --type=merge \
  -p "{\"status\":{\"wireguardAddress\":\"${WG_IP}/32\"}}"

echo "=== download bundle ==="
WORKDIR=/tmp/${CLUSTER}-bundle
rm -rf "$WORKDIR" && mkdir -p "$WORKDIR"
for i in $(seq 1 60); do
  http=$(curl -s -o "${WORKDIR}/bundle.tar.gz" -w '%{http_code}' "${API}/clusters/${CLUSTER}/bootstrap-bundle")
  [ "$http" = "200" ] && [ -s "${WORKDIR}/bundle.tar.gz" ] && break
  sleep 2
done
tar -xzf "${WORKDIR}/bundle.tar.gz" -C "$WORKDIR" inventory.ini "wireguard/${HOST}/wg-umkim.conf" 2>/dev/null || true
sed -i "s|10.10.0.11|${WG_IP}|g; s|10.10.0.10|${WG_IP}|g" "${WORKDIR}/inventory.ini" "${WORKDIR}/wireguard/${HOST}/wg-umkim.conf" 2>/dev/null || true
(cd "$WORKDIR" && tar -czf bundle-patched.tar.gz .)

echo "=== bootstrap on ${NODE} ==="
scp "${SSH_OPTS[@]}" "${WORKDIR}/bundle-patched.tar.gz" "linux@${NODE}:/tmp/bootstrap-${CLUSTER}.tar.gz"
ssh "${SSH_OPTS[@]}" "linux@${NODE}" bash -s <<'EOF'
set -euo pipefail
rm -rf "/tmp/bootstrap-single-lab"
mkdir -p "/tmp/bootstrap-single-lab"
tar -xzf "/tmp/bootstrap-single-lab.tar.gz" -C "/tmp/bootstrap-single-lab"
chmod +x "/tmp/bootstrap-single-lab/bootstrap.sh"
cd "/tmp/bootstrap-single-lab"
sudo bash ./bootstrap.sh
EOF

echo "=== wait Validated ==="
wait_phase Validated 360

echo "=== patch inventory for ocm delegate host ==="
sudo kubectl -n fleet-console get cm "fleet-cluster-${CLUSTER}-inventory" -o json > "/tmp/${CLUSTER}-inv-raw.json"
python3 <<PY
import json
cm=json.load(open("/tmp/${CLUSTER}-inv-raw.json"))
inv=cm["data"]["inventory.ini"]
if "[single-lab]" not in inv:
  inv += f"\n[single-lab]\nsingle-lab ansible_host=${WG_IP} ansible_user=linux\n\n[single-lab:vars]\nk3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\nocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\nwireguard_address=${WG_IP}/32\n"
inv=inv.replace("ansible_host=10.10.0.11","ansible_host=${WG_IP}")
cm["data"]["inventory.ini"]=inv
json.dump(cm, open("/tmp/${CLUSTER}-inv.json","w"))
PY
sudo kubectl apply -f "/tmp/${CLUSTER}-inv.json"

echo "=== provision ==="
sudo kubectl -n fleet-console delete job "fleet-cluster-provision-${CLUSTER}" --ignore-not-found
curl -sf -X POST "${API}/clusters/${CLUSTER}/provision"
wait_phase Ready 600

echo "=== providers ==="
for kind in k8s helm; do
  sudo kubectl -n fleet-console patch fleetclusterjoin "${CLUSTER}" --subresource=status --type=merge \
    -p "{\"status\":{\"providers\":{\"${kind}\":{\"phase\":\"NotInstalled\",\"jobName\":\"\",\"message\":\"\"}}}}" 2>/dev/null || true
  curl -sf -X POST "${API}/clusters/${CLUSTER}/providers/${kind}/install"
  for j in $(seq 1 24); do
    phase=$(curl -sf "${API}/clusters/${CLUSTER}/providers" | python3 -c "import json,sys
for p in json.load(sys.stdin).get('providers',[]):
  if p.get('kind')=='$kind': print(p.get('phase',''))")
    echo "  $kind [$j] $phase"; [ "$phase" = "Ready" ] && break; sleep 15
  done
done
curl -sf -X POST "${API}/clusters/${CLUSTER}/discover-capabilities" >/dev/null
curl -sf "${API}/clusters/${CLUSTER}" | python3 -c "import json,sys; d=json.load(sys.stdin); print('OK', d['provisioning_phase'], d['available'], d.get('ingress_id'), d.get('method'))"
