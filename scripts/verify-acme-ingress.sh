#!/usr/bin/env bash
# Verify public HTTP/S and Let's Encrypt paths on VPS + homelab.
set -euo pipefail

VPS_IP="${VPS_IP:-203.0.113.1}"
FLEET_HOST="${FLEET_HOST:-whoami.mgmt.umkim.com}"
HOMELAB_HOST="${HOMELAB_HOST:-whoami.homelab.mgmt.umkim.com}"
OPENWRT_WAN="${OPENWRT_WAN:-}"

echo "=== VPS fleet path (${FLEET_HOST} → ${VPS_IP}) ==="
echo -n "TCP 80 open: "
timeout 3 bash -c "echo >/dev/tcp/${VPS_IP}/80" 2>/dev/null && echo OK || echo FAIL

echo -n "TCP 443 open: "
timeout 3 bash -c "echo >/dev/tcp/${VPS_IP}/443" 2>/dev/null && echo OK || echo FAIL

echo -n "HTTP :80 challenge path (via Host): "
code=$(curl -sf -o /dev/null -w "%{http_code}" --connect-timeout 5 -H "Host: ${FLEET_HOST}" "http://${VPS_IP}/" || echo "000")
echo "${code}"

echo -n "HTTPS :443 (cert check): "
if curl -sf --connect-timeout 10 "https://${FLEET_HOST}/" >/dev/null 2>&1; then
  echo OK
  echo | openssl s_client -connect "${FLEET_HOST}:443" -servername "${FLEET_HOST}" 2>/dev/null \
    | openssl x509 -noout -issuer -dates 2>/dev/null || true
else
  echo "not ready (curl failed — cert may still be issuing)"
fi

if [[ -n "${OPENWRT_WAN}" ]]; then
  echo ""
  echo "=== Homelab path (${HOMELAB_HOST} → OpenWrt ${OPENWRT_WAN}) ==="
  echo -n "TCP 80 open: "
  timeout 3 bash -c "echo >/dev/tcp/${OPENWRT_WAN}/80" 2>/dev/null && echo OK || echo FAIL
  echo -n "TCP 443 open: "
  timeout 3 bash -c "echo >/dev/tcp/${OPENWRT_WAN}/443" 2>/dev/null && echo OK || echo FAIL
  echo -n "HTTP :80 (Host ${HOMELAB_HOST}): "
  code=$(curl -sf -o /dev/null -w "%{http_code}" --connect-timeout 5 -H "Host: ${HOMELAB_HOST}" "http://${OPENWRT_WAN}/" || echo "000")
  echo "${code}"
  echo -n "HTTPS (cert check): "
  if curl -sf --connect-timeout 10 "https://${HOMELAB_HOST}/" >/dev/null 2>&1; then
    echo OK
    echo | openssl s_client -connect "${HOMELAB_HOST}:443" -servername "${HOMELAB_HOST}" 2>/dev/null \
      | openssl x509 -noout -issuer -dates 2>/dev/null || true
  else
    echo "not ready"
  fi
else
  echo ""
  echo "Skip homelab checks: set OPENWRT_WAN to your OpenWrt public IP."
fi

echo ""
echo "Done."
