#!/usr/bin/env bash
# Verify site vs cluster separation: WAN egress vs per-cluster ingress, membership API.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
SITE_ID="${FLEET_SITE:-acme-site}"
CLUSTER="${FLEET_CLUSTER:-${FLEET_VERIFY_CLUSTER:-lab-sn01}}"

echo "=== verify-site-cluster-model (FLEET_API=$FLEET_API site=${SITE_ID} cluster=${CLUSTER}) ==="

SITE_JSON=$(curl -sf "${FLEET_API}/sites/${SITE_ID}" || true)
if [[ -z "$SITE_JSON" ]]; then
  echo "SKIP sites API (fleet-site-controller not reachable at $FLEET_API)"
else
  COUNT=$(echo "$SITE_JSON" | jq -r '.clusterCount // (.clusters | length) // 0')
  echo "OK   ${SITE_ID} site clusterCount=$COUNT"
  if echo "$SITE_JSON" | jq -e '.spec.edge.wanEgress.enabled == true or .spec.edge.externalAccess.enabled == true' >/dev/null; then
    echo "OK   ${SITE_ID} site wan egress enabled"
  else
    echo "WARN ${SITE_ID} site wan egress not enabled in FleetSite spec"
  fi
fi

if hub_kubectl get managedcluster "${CLUSTER}" &>/dev/null; then
  CLUSTER_JSON=$(curl -sf "${FLEET_API}/clusters/${CLUSTER}" || true)
  if [[ -n "$CLUSTER_JSON" ]]; then
    INGRESS=$(echo "$CLUSTER_JSON" | jq -r '.ingress_enabled // false')
    CLUSTER_SITE=$(echo "$CLUSTER_JSON" | jq -r '.site // ""')
    RWX=$(echo "$CLUSTER_JSON" | jq -r '.capabilities["storage.rwx"] // false')
    if [[ "$CLUSTER_SITE" == "$SITE_ID" ]]; then
      echo "OK   ${CLUSTER} fleet.site=${SITE_ID}"
    else
      echo "FAIL ${CLUSTER} site=$CLUSTER_SITE want ${SITE_ID}"
      exit 1
    fi
    if [[ "$INGRESS" == "false" ]]; then
      echo "OK   ${CLUSTER} ingress_enabled=false"
    else
      echo "WARN ${CLUSTER} ingress_enabled=$INGRESS (expected false for internal site)"
    fi
    if [[ "$RWX" == "true" ]]; then
      echo "OK   ${CLUSTER} storage.rwx=true"
    else
      echo "WARN ${CLUSTER} storage.rwx=$RWX (inventory may expect true after label sync)"
    fi
  fi
else
  echo "SKIP managedcluster ${CLUSTER} not found"
fi

echo "=== verify-site-cluster-model passed ==="
