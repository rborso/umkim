#!/usr/bin/env bash
# Remove hub wg0 runtime peers with no AllowedIPs (abandoned fleet enrollments).
set -euo pipefail

WG_IF="${WG_IF:-wg0}"

require_root() {
  [[ "${EUID}" -eq 0 ]] || exec sudo -E bash "$0" "$@"
}

require_root "$@"

if ! ip link show "$WG_IF" >/dev/null 2>&1; then
  echo "[prune-stale-hub-wg-peers] interface ${WG_IF} not found; nothing to do"
  exit 0
fi

removed=0
while read -r peer; do
  [[ -z "$peer" ]] && continue
  wg set "$WG_IF" peer "$peer" remove
  echo "[prune-stale-hub-wg-peers] removed ${peer}"
  removed=$((removed + 1))
done < <(wg show "$WG_IF" dump | awk -F'\t' '$4=="(none)"{print $1}')

echo "[prune-stale-hub-wg-peers] removed ${removed} stale peer(s)"
wg show "$WG_IF"
