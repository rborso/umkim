#!/usr/bin/env bash
# Install whoami via FleetApp onto a selected Ready cluster and probe the pod.
#
# Required:
#   FLEET_APP_CLUSTER  e.g. lab-single-0
# Optional:
#   FLEET_SITE_API     default http://127.0.0.1:18080/api/v1alpha1
set -euo pipefail

API="${FLEET_SITE_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER="${FLEET_APP_CLUSTER:?set FLEET_APP_CLUSTER to a Ready cluster}"
CHART="${FLEET_APP_CHART:-whoami}"
VERSION="${FLEET_APP_VERSION:-0.1.0}"
RELEASE="${FLEET_APP_RELEASE:-whoami}"
NS="${FLEET_APP_NAMESPACE:-default}"
NAME="${FLEET_APP_NAME:-whoami}"
ROUTE_MODE="${FLEET_APP_ROUTE_MODE:-none}"

HDR=(-H 'Content-Type: application/json' -H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-flow' -H 'X-Fleet-DC-Admin: true')

echo "=== FleetApp ${NAME} chart=${CHART} cluster=${CLUSTER} ==="
http=$(curl -sS -o /tmp/fleet-app-create.json -w '%{http_code}' "${HDR[@]}" \
  -X POST "${API}/apps" \
  -d "$(jq -n --arg name "$NAME" --arg cluster "$CLUSTER" --arg chart "$CHART" \
    --arg version "$VERSION" --arg release "$RELEASE" --arg ns "$NS" --arg mode "$ROUTE_MODE" \
    '{name:$name,clusterName:$cluster,chart:$chart,version:$version,releaseName:$release,namespace:$ns,route:{mode:$mode}}')")
if [[ "$http" != "201" && "$http" != "409" ]]; then
  echo "FAIL POST /apps HTTP ${http}"
  cat /tmp/fleet-app-create.json
  exit 1
fi

phase=""
for _ in $(seq 1 36); do
  curl -sf "${HDR[@]}" "${API}/apps/${NAME}" -o /tmp/fleet-app.json
  phase=$(jq -r '.phase // empty' /tmp/fleet-app.json)
  echo "phase=${phase} message=$(jq -r '.message // empty' /tmp/fleet-app.json)"
  if [[ "$phase" == "Ready" || "$phase" == "Failed" ]]; then
    break
  fi
  sleep 5
done
if [[ "$phase" != "Ready" ]]; then
  echo "FAIL FleetApp phase=${phase}"
  kubectl -n fleet-console get fleetapp "$NAME" -o yaml || true
  kubectl -n fleet-console get jobs -l app=fleet-app-helm -o wide || true
  exit 1
fi

KC=/tmp/fleet-kubeconfig-${CLUSTER}
kubectl -n fleet-system get secret "fleet-kubeconfig-${CLUSTER}" -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC"
kubectl --kubeconfig="$KC" --insecure-skip-tls-verify -n "$NS" \
  wait --for=condition=available "deployment/${RELEASE}" --timeout=180s
kubectl --kubeconfig="$KC" --insecure-skip-tls-verify -n "$NS" get deploy,svc,pods -l "app.kubernetes.io/instance=${RELEASE}"
# whoami image has no wget; probe the Service via API port-forward on the hub.
pf=18099
kubectl --kubeconfig="$KC" --insecure-skip-tls-verify -n "$NS" port-forward "svc/${RELEASE}" "${pf}:80" >/tmp/fleet-app-pf.log 2>&1 &
pfpid=$!
trap 'kill "$pfpid" 2>/dev/null || true' EXIT
for _ in $(seq 1 20); do
  if curl -sf "http://127.0.0.1:${pf}/" | tee /tmp/fleet-app-body.txt | grep -qi 'Hostname:'; then
    echo "OK ${RELEASE} on ${CLUSTER} ($(wc -c < /tmp/fleet-app-body.txt) bytes)"
    exit 0
  fi
  sleep 0.5
done
echo "FAIL HTTP probe of ${NS}/${RELEASE} (see /tmp/fleet-app-pf.log)"
cat /tmp/fleet-app-pf.log /tmp/fleet-app-body.txt 2>/dev/null || true
exit 1
