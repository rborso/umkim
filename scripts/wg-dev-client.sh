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
import re
import sys
from pathlib import Path

name, path_s = sys.argv[1], Path(sys.argv[2])

DEFAULT_POOL = 100
SUBNET_PREFIX = "10.10.0."


def parse_pool(text):
    m = re.search(r'^\s*wireguard_operator_client_pool_octet:\s*(\d+)\s*$', text, re.M)
    return int(m.group(1)) if m else DEFAULT_POOL


def parse_clients(text):
    """Return [(name, address_or_None)] in file order."""
    clients = []
    cur = None
    for line in text.splitlines():
        m = re.match(r'^\s*-\s*name:\s*(\S+)\s*$', line)
        if m:
            if cur is not None:
                clients.append(cur)
            cur = [m.group(1), None]
            continue
        m2 = re.match(r'^\s*address:\s*(\S+)\s*$', line)
        if m2 and cur is not None:
            cur[1] = m2.group(1)
    if cur is not None:
        clients.append(cur)
    return clients


text = path_s.read_text() if path_s.is_file() else ""
pool = parse_pool(text)
clients = parse_clients(text)

if any(c[0] == name for c in clients):
    print(f"[wg-dev-client] {name} already listed in {path_s}")
    sys.exit(0)

# Effective address per client: an explicit address, else pool+index (legacy
# position-based layout). Persisting every address keeps existing clients stable
# so adding a client never renumbers the others.
effective = []
used = set()
for i, (cname, addr) in enumerate(clients):
    if not addr:
        addr = f"{SUBNET_PREFIX}{pool + i}/32"
    effective.append((cname, addr))
    m = re.match(r'^' + re.escape(SUBNET_PREFIX) + r'(\d+)/32$', addr)
    if m:
        used.add(int(m.group(1)))

# First free octet at or after the pool.
octet = pool
while octet in used:
    octet += 1
new_addr = f"{SUBNET_PREFIX}{octet}/32"

out = [
    "---",
    "# Operator PCs — split-tunnel overlay access. Add names here or use scripts/wg-dev-client.sh.",
    "wireguard_operator_clients:",
]
for cname, addr in effective:
    out.append(f"  - name: {cname}")
    out.append(f"    address: {addr}")
out.append(f"  - name: {name}")
out.append(f"    address: {new_addr}")
out.append(f"wireguard_operator_client_pool_octet: {pool}")
path_s.write_text("\n".join(out) + "\n")
print(f"[wg-dev-client] added {name} ({new_addr}) to {path_s}")
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

Copy to your laptop (run on the laptop):
  scp linux@${HUB_PUBLIC_IP}:$CONF .

See docs/wireguard-operator-clients.md
EOF
