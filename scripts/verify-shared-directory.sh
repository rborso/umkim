#!/usr/bin/env bash
# Verify the shared (hub) LLDAP directory wiring: the hub services, the
# directory service accounts, and the WireGuard-only NodePorts.
#
# Read-only. Run on the hub (umkim-hub) or anywhere with KUBECONFIG pointing at
# the hub. Spoke-side checks (no spoke LLDAP, Authelia binds the hub directory,
# tenant group enforcement, invite redemption) are described in
# docs/lldap-bootstrap.md and must be run against each cluster.
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG

NS="${FLEET_CONSOLE_NS:-fleet-console}"
LDAP_NODEPORT="${FLEET_LLDAP_LDAP_NODEPORT:-30389}"
AUTH_NODEPORT="${FLEET_AUTH_WG_NODEPORT:-30919}"

fail=0
pass() { echo "PASS $*"; }
bad() { echo "FAIL $*"; fail=1; }

# 1. Hub directory services.
lldap_type="$(kubectl -n "$NS" get svc lldap -o jsonpath='{.spec.type}' 2>/dev/null || true)"
if [[ "$lldap_type" == "ClusterIP" ]]; then
  pass "hub lldap Service is ClusterIP"
else
  bad "hub lldap Service type is '${lldap_type:-missing}' (want ClusterIP)"
fi

lldap_np="$(kubectl -n "$NS" get svc lldap-ldap -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null || true)"
if [[ "$lldap_np" == "$LDAP_NODEPORT" ]]; then
  pass "hub lldap-ldap NodePort is ${LDAP_NODEPORT}"
else
  bad "hub lldap-ldap NodePort is '${lldap_np:-missing}' (want ${LDAP_NODEPORT})"
fi

auth_np="$(kubectl -n "$NS" get svc fleet-console-auth-wg -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null || true)"
if [[ "$auth_np" == "$AUTH_NODEPORT" ]]; then
  pass "hub fleet-console-auth-wg NodePort is ${AUTH_NODEPORT}"
else
  bad "hub fleet-console-auth-wg NodePort is '${auth_np:-missing}' (want ${AUTH_NODEPORT})"
fi

# 2. Directory service accounts.
for key in admin-username admin-password bind-username bind-password; do
  if kubectl -n "$NS" get secret lldap-service-accounts -o "jsonpath={.data.$key}" 2>/dev/null | grep -q .; then
    pass "lldap-service-accounts has ${key}"
  else
    bad "lldap-service-accounts missing ${key}"
  fi
done

# 3. Firewall drops the WireGuard-only NodePorts on the public NIC.
if command -v nft >/dev/null 2>&1; then
  rules="$(sudo -n nft list ruleset 2>/dev/null || nft list ruleset 2>/dev/null || true)"
  for port in "$LDAP_NODEPORT" "$AUTH_NODEPORT"; do
    if grep -qE "dport .*${port}" <<<"$rules"; then
      pass "nft drops WAN traffic to :${port}"
    else
      bad "nft has no WAN drop rule for :${port}"
    fi
  done
else
  echo "SKIP nft firewall check (nft not available)"
fi

# 4. Report clusters using the shared hub directory.
mapfile -t hub_clusters < <(
  kubectl -n "$NS" get fleetclusterjoins -o json 2>/dev/null \
    | jq -r '.items[] | select(.spec.ingressAuth.enabled == true and (.spec.ingressAuth.directory // "") == "hub") | .metadata.name'
)
if ((${#hub_clusters[@]})); then
  echo "INFO clusters using the shared hub directory: ${hub_clusters[*]}"
  echo "INFO verify per cluster: no LLDAP Deployment in fleet-ingress-auth; Authelia binds ldap://<hub-wg-ip>:${LDAP_NODEPORT} as the read-only account"
else
  echo "INFO no cluster currently uses the shared hub directory"
fi

if ((fail)); then
  echo "RESULT shared-directory verification FAILED"
  exit 1
fi
echo "RESULT shared-directory verification passed"
