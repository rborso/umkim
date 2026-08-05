#!/bin/sh
set -eu
CLUSTER="${1:?cluster name}"
HUB_KC="${HUB_KUBECONFIG:-/etc/ocm/hub.kubeconfig}"
SECRET_NS="${2:-fleet-system}"

export KUBECONFIG="$HUB_KC"
clusteradm get token -o json > /tmp/ocm-tok.json
TOK_ONELINE=$(tr -d '\n' </tmp/ocm-tok.json)
HUBTOKEN=$(echo "$TOK_ONELINE" | sed -n 's/.*"hub-token":"\([^"]*\)".*/\1/p')
APISERVER=$(echo "$TOK_ONELINE" | sed -n 's/.*"hub-apiserver":"\([^"]*\)".*/\1/p')
[ -n "$APISERVER" ] || APISERVER="${OCM_HUB_APISERVER:-https://10.10.0.1:6443}"

kubectl -n "$SECRET_NS" get secret "fleet-kubeconfig-${CLUSTER}" -o jsonpath='{.data.kubeconfig}' | base64 -d > /tmp/nested.kubeconfig
export KUBECONFIG=/tmp/nested.kubeconfig

echo "Joining ${CLUSTER} to ${APISERVER} ..."
clusteradm join \
  --hub-token "$HUBTOKEN" \
  --hub-apiserver "$APISERVER" \
  --cluster-name "$CLUSTER" \
  --force-internal-endpoint-lookup
