#!/usr/bin/env bash
# Verify enriched nodes API (addresses, capacity). Uses cluster nodes when proxy works,
# otherwise falls back to infrastructure/hub nodes (same enrichment helper).
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_NODES_CLUSTER:-}"

echo "=== verify-fleet-cluster-nodes ==="
curl -sf "${API}/clusters" >/dev/null || { echo "Fleet API unreachable at ${API}"; exit 1; }

if [ -z "${CLUSTER}" ]; then
  CLUSTER=$(curl -sf "${API}/clusters" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for c in d.get('clusters',[]):
  if c.get('available'):
    print(c['name']); break
" 2>/dev/null || true)
fi

RAW=""
SOURCE=""
if [ -n "${CLUSTER}" ]; then
  HTTP=$(curl -s -o /tmp/fcj-nodes.json -w '%{http_code}' "${API}/clusters/${CLUSTER}/nodes")
  if [ "${HTTP}" = "200" ]; then
    RAW=$(cat /tmp/fcj-nodes.json)
    SOURCE="cluster ${CLUSTER}"
  fi
fi

if [ -z "${RAW}" ]; then
  RAW=$(curl -sf "${API}/infrastructure/hub")
  SOURCE="infrastructure/hub"
fi

echo "using ${SOURCE}"
echo "${RAW}" | grep -q '"nodes"' || { echo "expected nodes in response"; exit 1; }

if echo "${RAW}" | grep -q '"capacity"'; then
  echo "capacity fields present"
elif echo "${RAW}" | grep -qE '"nodes":\[\]|"nodes":null'; then
  echo "WARN: empty nodes array; enrichment shape OK but no data"
else
  echo "nodes returned but no capacity — API shape may have changed"
  exit 1
fi

if echo "${RAW}" | grep -q '"addresses"'; then
  echo "addresses fields present"
fi

echo "=== verify-fleet-cluster-nodes OK ==="
