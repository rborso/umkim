#!/usr/bin/env bash
# One-shot: install DNS-ready drop-in for wg-quick@ on an existing node (e.g. after boot race).
# Usage (on the node as root):
#   curl -fsSL … | bash
# Or from umkim checkout:
#   sudo ./scripts/fix-wg-umkim-dns-ready.sh [iface]
set -euo pipefail

IFACE="${1:-wg-umkim}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HELPER_SRC="${ROOT}/roles/fleet_cluster_wireguard/files/umkim-wg-wait-dns.sh"
DROPIN_SRC="${ROOT}/roles/fleet_cluster_wireguard/files/wg-quick-umkim-dns-ready.conf"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root" >&2
  exit 1
fi

if [[ ! -f "${HELPER_SRC}" || ! -f "${DROPIN_SRC}" ]]; then
  echo "Missing helper files under ${ROOT}/roles/fleet_cluster_wireguard/files" >&2
  exit 1
fi

install -d -m 0755 /usr/local/libexec /etc/systemd/system/wg-quick@.service.d
install -m 0755 "${HELPER_SRC}" /usr/local/libexec/umkim-wg-wait-dns.sh
install -m 0644 "${DROPIN_SRC}" /etc/systemd/system/wg-quick@.service.d/umkim-dns-ready.conf
systemctl daemon-reload
systemctl enable "wg-quick@${IFACE}"
systemctl restart "wg-quick@${IFACE}"
systemctl --no-pager --full status "wg-quick@${IFACE}" || true
echo "Fixed: wg-quick@${IFACE} now waits for Endpoint DNS and retries on failure."
