#!/usr/bin/env bash
# Verify site ingress[] and cluster ingress_id binding against edge ConfigMaps.
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
SITE="${VERIFY_SITE:-homelab}"
CLUSTER="${VERIFY_CLUSTER:-virt01}"
NS="${FLEET_EDGE_NS:-fleet-system}"
DOMAINS_CM="${FLEET_EDGE_DOMAINS_CM:-fleet-edge-domains}"
CLUSTERS_CM="${FLEET_EDGE_CLUSTERS_CM:-fleet-edge-clusters}"
INGRESS_ID="${VERIFY_INGRESS_ID:-edge-rp}"
INGRESS_ZONE="${VERIFY_INGRESS_ZONE:-homelab.mgmt.umkim.com}"
RESTORE_INGRESS_ID="${VERIFY_RESTORE_INGRESS_ID:-homelab-pf}"

echo "=== verify-fleet-external-access ==="

# Ensure site has a reverse_proxy ingress row, bind cluster to it, then restore.
curl -sf -X PATCH "${API}/sites/${SITE}" \
  -H 'Content-Type: application/json' \
  -d "{\"ingress\":[
    {\"id\":\"${RESTORE_INGRESS_ID}\",\"method\":\"port_forward\",\"address\":\"203.0.113.50\",\"zone\":\"${INGRESS_ZONE}\",\"wildcard\":\"*.${INGRESS_ZONE}\"},
    {\"id\":\"${INGRESS_ID}\",\"method\":\"reverse_proxy\",\"operatorNotes\":\"verify script\"}
  ]}" \
  | grep -q reverse_proxy

curl -sf -X PATCH "${API}/clusters/${CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d "{\"edge\":{\"ingress_id\":\"${INGRESS_ID}\"}}" \
  | grep -q "\"ingress_id\":\"${INGRESS_ID}\""

sleep 2
RAW=$(kubectl -n "${NS}" get configmap "${DOMAINS_CM}" -o json \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('domains.json',''))")
echo "${RAW}" | grep -q "\"method\": \"reverse_proxy\""
echo "${RAW}" | grep -q "\"id\": \"${INGRESS_ID}\""

CYAML=$(kubectl -n "${NS}" get configmap "${CLUSTERS_CM}" -o jsonpath='{.data.clusters\.yaml}')
echo "${CYAML}" | grep -q "ingress_id: ${INGRESS_ID}"
echo "${CYAML}" | grep -q 'method: reverse_proxy'
echo "${CYAML}" | grep -q 'tls_policy: external'

# Restore default port_forward binding for lab
curl -sf -X PATCH "${API}/clusters/${CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d "{\"edge\":{\"ingress_id\":\"${RESTORE_INGRESS_ID}\"}}" >/dev/null

echo "=== verify-fleet-external-access OK ==="
