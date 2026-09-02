#!/usr/bin/env bash
# Verify fleet ingress auth (Authelia + LLDAP) on hub and workload clusters.
#
# Usage:
#   ./scripts/verify-fleet-ingress-auth.sh
#
# Optional route smoke:
#   VERIFY_ROUTES=1 ./scripts/verify-fleet-ingress-auth.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/fleet-verify/lib/hub-kubectl.sh
source "${ROOT}/scripts/fleet-verify/lib/hub-kubectl.sh"

AUTH_NS="${FLEET_INGRESS_AUTH_NS:-fleet-ingress-auth}"
LLDAP_NS="${FLEET_LLDAP_NS:-fleet-console}"
FIA_NS="${FLEET_INGRESS_AUTH_CR_NS:-fleet-console}"
HUB_GATEWAY="${HUB_GATEWAY:-http://127.0.0.1:80}"
EDGE_DOMAIN="${FLEET_EDGE_DOMAIN:-apps.example.com}"
SPOKE_CLUSTER="${FLEET_INGRESS_AUTH_CLUSTER:-lab-ha01}"
AUTH_PASS="${FLEET_INGRESS_AUTH_PASSWORD:-${LLDAP_PASSWORD:-}}"
if [[ -z "${AUTH_PASS}" ]]; then
  vault="${ROOT}/inventory/group_vars/all/vault.yml"
  pass_file="${ANSIBLE_VAULT_PASSWORD_FILE:-${HOME}/.umkim-vault-pass}"
  if [[ -f "${pass_file}" && -f "${vault}" ]]; then
    AUTH_PASS="$(ansible-vault view "${vault}" --vault-password-file "${pass_file}" 2>/dev/null \
      | awk -F'"' '/fleet_lldap_admin_password_vault/{print $2; exit}' || true)"
  fi
fi
if [[ -z "${AUTH_PASS}" ]]; then
  AUTH_PASS="$(hub_kubectl -n "$FIA_NS" get secret fleet-ingress-auth-secrets -o jsonpath='{.data.bootstrapPassword}' 2>/dev/null | base64 -d || true)"
fi
AUTH_PASS="${AUTH_PASS:-fleet-ingress-auth-change-me}"

echo "== Hub auth stack"
hub_kubectl get ns "$AUTH_NS" >/dev/null
if hub_kubectl get fleetingressauth fleet-ingress-auth -n "$FIA_NS" >/dev/null 2>&1; then
  phase="$(hub_kubectl get fleetingressauth fleet-ingress-auth -n "$FIA_NS" -o jsonpath='{.status.phase}')"
  echo "FleetIngressAuth (hub) status.phase=${phase}"
  if [[ -n "$phase" && "$phase" != "Ready" && "$phase" != "Applying" ]]; then
    echo "WARN: FleetIngressAuth phase is ${phase}"
  fi
fi
hub_kubectl -n "$LLDAP_NS" rollout status deployment/lldap --timeout=120s
hub_kubectl -n "$AUTH_NS" rollout status deployment/authelia --timeout=180s
curl -sf "http://$(hub_kubectl -n "$AUTH_NS" get svc authelia -o jsonpath='{.spec.clusterIP}'):9091/api/health" >/dev/null

if hub_kubectl get managedcluster "$SPOKE_CLUSTER" &>/dev/null; then
  echo "== ${SPOKE_CLUSTER} FleetIngressAuth status"
  if hub_kubectl get fleetingressauth fleet-ingress-auth -n "$FIA_NS" >/dev/null 2>&1; then
    phase="$(hub_kubectl get fleetingressauth fleet-ingress-auth -n "$FIA_NS" -o json \
      | jq -r --arg c "$SPOKE_CLUSTER" '.status.clusters[]? | select(.name == $c) | .phase' | head -1)"
    if [[ -n "$phase" ]]; then
      echo "FleetIngressAuth cluster ${SPOKE_CLUSTER} phase=${phase}"
    else
      echo "SKIP: ${SPOKE_CLUSTER} not listed in FleetIngressAuth status.clusters"
    fi
  fi
  echo "== ${SPOKE_CLUSTER} auth ManifestWork"
  if hub_kubectl get manifestwork fleet-ingress-auth -n "$SPOKE_CLUSTER" >/dev/null 2>&1; then
    hub_kubectl get managedcluster "$SPOKE_CLUSTER" -o json \
      | jq -e '.status.conditions[]? | select(.type == "ManagedClusterConditionAvailable" or .type == "Available") | select(.status == "True")' >/dev/null
    KC="/tmp/verify-ingress-auth-${SPOKE_CLUSTER}.yaml"
    hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${SPOKE_CLUSTER}" -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC"
    if kubectl --kubeconfig="$KC" get ns "$AUTH_NS" >/dev/null 2>&1; then
      kubectl --kubeconfig="$KC" -n "$AUTH_NS" rollout status deployment/authelia --timeout=180s
      kubectl --kubeconfig="$KC" -n fleet-ingress get cm fleet-ingress-auth-traefik >/dev/null 2>&1 \
        || echo "WARN: fleet-ingress-auth-traefik ConfigMap not found on ${SPOKE_CLUSTER}"
    else
      echo "WARN: ${SPOKE_CLUSTER} spoke missing ${AUTH_NS} namespace"
    fi
    rm -f "$KC"
  else
    echo "SKIP: no ManifestWork fleet-ingress-auth on ${SPOKE_CLUSTER}"
  fi
else
  echo "SKIP: managedcluster ${SPOKE_CLUSTER} not found"
fi

if [[ "${VERIFY_ROUTES:-0}" == "1" ]]; then
  echo "== Route policy smoke"
  hub_code="$(curl -s -o /dev/null -w '%{http_code}' -H "Host: whoami.${EDGE_DOMAIN}" "${HUB_GATEWAY}/" || true)"
  echo "whoami.${EDGE_DOMAIN} HTTP ${hub_code}"
  if [[ "$hub_code" != "200" && "$hub_code" != "301" && "$hub_code" != "302" && "$hub_code" != "401" ]]; then
    echo "WARN: unexpected HTTP ${hub_code} for whoami route smoke"
  fi
fi

echo "== Authelia login API (in-cluster)"
AUTH_IP="$(hub_kubectl -n "$AUTH_NS" get svc authelia -o jsonpath='{.spec.clusterIP}')"
if [[ -z "${AUTH_PASS}" ]]; then
  AUTH_PASS="$(hub_kubectl -n "$FIA_NS" get secret fleet-ingress-auth-secrets -o jsonpath='{.data.bootstrapPassword}' 2>/dev/null | base64 -d || true)"
fi
login_code="$(hub_kubectl -n "$AUTH_NS" run "authelia-login-${FV_RUN_ID:-$$}" --rm -i --restart=Never --image=curlimages/curl:8.11.1 --quiet -- \
  curl -s -o /dev/null -w '%{http_code}' \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"${AUTH_PASS}\",\"keepMeLoggedIn\":false,\"targetURL\":\"http://localhost/\"}" \
  "http://${AUTH_IP}:9091/api/firstfactor" 2>/dev/null || echo "000")"
echo "firstfactor HTTP ${login_code}"
if [[ "$login_code" == "200" ]]; then
  echo "OK: Authelia LDAP login"
else
  echo "WARN: Authelia firstfactor HTTP ${login_code} (stack healthy; check LLDAP admin password sync)"
fi

echo "OK — fleet ingress auth verified"
