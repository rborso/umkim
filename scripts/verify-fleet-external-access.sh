#!/usr/bin/env bash
# Verify site exposure mode, endpoints, domains, and cluster bindings.
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:30908/fleet/api/v1alpha1}"
SITE="${VERIFY_SITE:-homelab}"
CLUSTER="${VERIFY_CLUSTER:-virt01}"
NS="${FLEET_EDGE_NS:-fleet-system}"
DOMAINS_CM="${FLEET_EDGE_DOMAINS_CM:-fleet-edge-domains}"
CLUSTERS_CM="${FLEET_EDGE_CLUSTERS_CM:-fleet-edge-clusters}"

echo "=== verify-fleet-external-access ==="

# Toggle reverse_proxy then restore port_forward
curl -sf -X PATCH "${API}/sites/${SITE}" \
  -H 'Content-Type: application/json' \
  -d '{"exposureMode":"reverse_proxy","domains":[{"id":"site-zone","zone":"homelab.mgmt.umkim.com","wildcard":"*.homelab.mgmt.umkim.com","tlsPolicy":"external","assignedClusters":["virt01"]}]}' \
  | grep -q reverse_proxy

curl -sf -X PATCH "${API}/clusters/${CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d '{"edge":{"exposure_mode":"reverse_proxy","tls_policy":"external","domain_bindings":["site-zone"]}}' \
  | grep -q '"tls_policy":"external"'

sleep 2
RAW=$(kubectl -n "${NS}" get configmap "${DOMAINS_CM}" -o json \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('domains.json',''))")
echo "${RAW}" | grep -q '"exposure_mode": "reverse_proxy"'
echo "${RAW}" | grep -q '"tls_policy": "external"'

CYAML=$(kubectl -n "${NS}" get configmap "${CLUSTERS_CM}" -o jsonpath='{.data.clusters\.yaml}')
echo "${CYAML}" | grep -q 'exposure_mode: reverse_proxy'
echo "${CYAML}" | grep -q 'tls_policy: external'

# Restore homelab port_forward for lab default
curl -sf -X PATCH "${API}/sites/${SITE}" \
  -H 'Content-Type: application/json' \
  -d '{"exposureMode":"port_forward"}' >/dev/null

curl -sf -X PATCH "${API}/clusters/${CLUSTER}" \
  -H 'Content-Type: application/json' \
  -d '{"edge":{"exposure_mode":"inherit","tls_policy":"inherit"}}' >/dev/null

echo "=== verify-fleet-external-access OK ==="
