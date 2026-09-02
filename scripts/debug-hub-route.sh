#!/usr/bin/env bash
set -euo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
INGRESS_NS="${HUB_INGRESS_NS:-fleet-ingress}"
ROUTES_CM="${FLEET_ROUTES_CM_NAME:-fleet-traefik-fleet-routes}"
kubectl -n fleet-system apply -f - <<'YAML'
apiVersion: edge.umkim.com/v1alpha1
kind: FleetAppRoute
metadata:
  name: debug-hub-route
spec:
  instanceId: debug-hub-route
  cluster: virt01
  mode: hub
  hubHost: whoami-hub-auth-test.mgmt.umkim.com
  service:
    name: whoami
    namespace: fleet-ingress
    port: 80
YAML
for i in 1 2 3 4 5 6; do
  sleep 5
  echo "--- attempt $i"
  kubectl -n fleet-system get fleetapproute debug-hub-route -o jsonpath='phase={.status.phase} mode={.spec.effectiveMode} msg={.status.message}{"\n"}' || true
  kubectl -n "$INGRESS_NS" get cm "$ROUTES_CM" -o jsonpath='{.data.fleet-routes\.yml}' | grep -c whoami-hub || echo 0
  kubectl get manifestwork -n virt01 fleet-route-debug-hub-route 2>/dev/null && echo MW exists || echo MW missing
done
echo "CM snippet:"
kubectl -n "$INGRESS_NS" get cm "$ROUTES_CM" -o jsonpath='{.data.fleet-routes\.yml}' | head -40
curl -s -o /dev/null -w 'gateway=%{http_code}\n' -H 'Host: whoami-hub-auth-test.mgmt.umkim.com' http://127.0.0.1:80/
curl -s -o /dev/null -w 'spoke=%{http_code}\n' -H 'Host: whoami-hub-auth-test.mgmt.umkim.com' http://10.10.0.11:30080/
