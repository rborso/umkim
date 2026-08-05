#!/usr/bin/env bash
# Generate a split-tunnel WireGuard client for developer access to the hub overlay.
# Run on the hub after wireguard_server is up (see docs/install.md).
#
# Safe after FleetClusterJoin clusters exist: peers are added with wg set only (no wg0.conf rewrite).
#
# Usage:
#   ./scripts/wg-dev-client.sh <client-name>
#   ./scripts/wg-dev-client.sh laptop-alice
#
# Output: output/wireguard-clients/<client-name>/wg-umkim.conf
# Import on your PC (wireguard-tools): sudo wg-quick up ./wg-umkim.conf
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLIENT_NAME="${1:-}"
WG_VARS="${WG_VARS:-$REPO_ROOT/inventory/group_vars/all/wg_operator_clients.yml}"
INV="${INV:--i $REPO_ROOT/inventory/hosts.yml -i $REPO_ROOT/inventory/hosts-from-hub.yml}"

usage() {
  cat <<EOF
Usage: $(basename "$0") <client-name>

Creates (or reuses) a WireGuard client config for overlay access to the fleet console.
Client name: short label for your laptop (letters, numbers, hyphen).

Example:
  $(basename "$0") dev-laptop

Files:
  $REPO_ROOT/output/wireguard-clients/<client-name>/wg-umkim.conf
EOF
}

if [[ -z "$CLIENT_NAME" ]] || [[ "$CLIENT_NAME" == -h ]] || [[ "$CLIENT_NAME" == --help ]]; then
  usage
  exit 0
fi

if ! [[ "$CLIENT_NAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]]; then
  echo "error: client name must match [a-zA-Z0-9][a-zA-Z0-9_-]*" >&2
  exit 1
fi

mkdir -p "$(dirname "$WG_VARS")"

python3 - "$CLIENT_NAME" "$WG_VARS" <<'PY'
import sys
from pathlib import Path

name, path_s = sys.argv[1], Path(sys.argv[2])
marker = f"- name: {name}"

if path_s.is_file():
    text = path_s.read_text()
    if marker in text:
        print(f"[wg-dev-client] {name} already listed in {path_s}")
        sys.exit(0)
    lines = text.splitlines()
    out = []
    inserted = False
    i = 0
    while i < len(lines):
        line = lines[i]
        if not inserted and line.strip().startswith("wireguard_operator_clients:"):
            rest = line.split(":", 1)[1].strip()
            if rest == "[]":
                out.append("wireguard_operator_clients:")
                out.append(f"  {marker}")
            elif rest:
                out.append(line)
                out.append(f"  {marker}")
            else:
                out.append(line)
                out.append(f"  {marker}")
            inserted = True
            i += 1
            continue
        out.append(line)
        i += 1
    if not inserted:
        raise SystemExit(f"error: could not find wireguard_operator_clients: in {path_s}")
    path_s.write_text("\n".join(out).rstrip() + "\n")
else:
    path_s.write_text(
        "---\n"
        "# Operator PCs — split-tunnel overlay access (managed by scripts/wg-dev-client.sh)\n"
        "wireguard_operator_clients:\n"
        f"  {marker}\n"
        "wireguard_operator_client_pool_octet: 100\n"
    )
print(f"[wg-dev-client] added {name} to {path_s}")
PY

cd "$REPO_ROOT"
# shellcheck disable=SC2086
ansible-playbook $INV wireguard-clients.yml

OUT_DIR="$REPO_ROOT/output/wireguard-clients/$CLIENT_NAME"
CONF="$OUT_DIR/wg-umkim.conf"

if [[ ! -f "$CONF" ]]; then
  echo "error: expected config at $CONF" >&2
  exit 1
fi

HUB_PUBLIC_IP="$(grep -E 'ansible_host:' "$REPO_ROOT/inventory/hosts.yml" | head -1 | awk '{print $2}' | tr -d "'\"")"
[[ -z "$HUB_PUBLIC_IP" || "$HUB_PUBLIC_IP" == 127.0.0.1 ]] && HUB_PUBLIC_IP='<hub-public-ip>'

cat <<EOF

WireGuard client ready.

  Config:  $CONF
  On your PC:  sudo wg-quick up ./wg-umkim.conf
  Test:        ping -c1 10.10.0.1
  Console:     http://10.10.0.1:30912/fleet/   (dashboard NodePort)
               http://10.10.0.1:30908/fleet/   (DCM gateway HTTP)

Copy to your laptop (run on the laptop):
  scp linux@${HUB_PUBLIC_IP}:$CONF .

See docs/wireguard-operator-clients.md
EOF
