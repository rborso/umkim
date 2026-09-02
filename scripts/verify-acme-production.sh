#!/usr/bin/env bash
# Verify hub app hostnames serve production Let's Encrypt certs (not Traefik default / staging).
set -euo pipefail

DOMAIN="${FLEET_EDGE_DOMAIN:-mgmt.umkim.com}"
HOST="${ACME_VERIFY_HOST:-demo-hub.${DOMAIN}}"
KUBECTL="${KUBECTL:-}"
if [[ -z "$KUBECTL" ]]; then
  if command -v kubectl >/dev/null 2>&1 && kubectl get nodes >/dev/null 2>&1; then
    KUBECTL=kubectl
  elif [[ -x /usr/local/bin/k3s ]]; then
    KUBECTL="sudo /usr/local/bin/k3s kubectl"
  else
    KUBECTL="sudo k3s kubectl"
  fi
fi

fail() { echo "[verify-acme-production] FAIL $*" >&2; exit 1; }
ok()   { echo "[verify-acme-production] OK   $*"; }

echo "=== verify-acme-production host=${HOST} ==="

phase="$(kubectl -n fleet-system get fleetapproutes -o json 2>/dev/null \
  | jq -r --arg h "$HOST" '.items[] | select(.spec.hubHost==$h) | .status.tlsPhase' | head -1 || true)"
if [[ -n "$phase" && "$phase" != "Ready" ]]; then
  fail "FleetAppRoute tlsPhase=$phase (expected Ready)"
fi
if [[ -n "$phase" ]]; then
  ok "FleetAppRoute tlsPhase=Ready"
fi

INGRESS_NS="${HUB_INGRESS_NS:-fleet-ingress}"
STATIC_CM="${HUB_INGRESS_STATIC_CM:-fleet-traefik-static}"
static="$($KUBECTL -n "$INGRESS_NS" get cm "$STATIC_CM" -o jsonpath='{.data.traefik\.yml}' 2>/dev/null || true)"
[[ -n "$static" ]] || fail "${STATIC_CM} missing in ${INGRESS_NS}"
echo "$static" | grep -q 'letsencrypt-preflight' \
  && ok "traefik static has preflight resolver" \
  || fail "missing letsencrypt-preflight resolver"
echo "$static" | grep -q 'storage: /var/traefik/acme.json' \
  && ok "production resolver uses acme.json" \
  || fail "production resolver missing acme.json"

issuer="$(echo | openssl s_client -connect "${HOST}:443" -servername "$HOST" 2>/dev/null \
  | openssl x509 -noout -issuer 2>/dev/null || true)"
[[ -n "$issuer" ]] || fail "no TLS cert from ${HOST}:443"
if echo "$issuer" | grep -qiE 'TRAEFIK DEFAULT CERT'; then
  fail "Traefik default cert — ACME preflight/production issuance did not complete"
fi
if echo "$issuer" | grep -qiE 'STAGING|Fake LE'; then
  fail "staging cert still served — expected production LE: ${issuer}"
fi
if echo "$issuer" | grep -qi "Let's Encrypt"; then
  ok "production Let's Encrypt issuer: ${issuer}"
else
  fail "unexpected issuer: ${issuer}"
fi

echo "=== verify-acme-production OK ==="
