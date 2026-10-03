#!/usr/bin/env bash
# Merge FleetClusterJoin overlay peers into live hub wg0 (runtime + /etc/wireguard/wg0.conf).
# Run on umkim-hub after regenerating spoke hub configs or wg-quick restart.
set -euo pipefail

WG_IF="${WG_IF:-wg0}"
WG_CONF="${WG_CONF:-/etc/wireguard/wg0.conf}"
NS="${FLEET_CONSOLE_NS:-fleet-console}"

require_root() {
  [[ "${EUID}" -eq 0 ]] || exec sudo -E bash "$0" "$@"
}

require_root "$@"

if ! command -v kubectl >/dev/null; then
  echo "kubectl required" >&2
  exit 1
fi

if ip link show "$WG_IF" >/dev/null 2>&1; then
  while read -r peer; do
    [[ -z "$peer" ]] && continue
    wg set "$WG_IF" peer "$peer" remove
    echo "[sync-hub-wg-cluster-peers] pruned stale peer ${peer}"
  done < <(wg show "$WG_IF" dump | awk -F'\t' '$4=="(none)"{print $1}')
fi

mapfile -t lines < <(
  kubectl -n "$NS" get secrets -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
    | grep 'bootstrap$' || true
)

peers=0
for sec in "${lines[@]}"; do
  [[ -z "$sec" ]] && continue
  data=$(kubectl -n "$NS" get "secret/$sec" -o jsonpath='{.data.nodes-wg\.json}' 2>/dev/null || true)
  [[ -z "$data" ]] && continue
  while IFS='|' read -r host pub ip; do
    [[ -z "$pub" || -z "$ip" ]] && continue
    ip="${ip%/32}"
    echo "[fleet-node-cleanup] peer cluster/$host $pub -> ${ip}/32"
    wg set "$WG_IF" peer "$pub" allowed-ips "${ip}/32" persistent-keepalive 25
    peers=$((peers + 1))
  done < <(echo "$data" | base64 -d | jq -r '.[] | "\(.hostname)|\(.publicKey)|\(.wgAddress)"')
done

if [[ -f "$WG_CONF" ]] && [[ "$peers" -gt 0 ]]; then
  tmp=$(mktemp)
  awk -v peers=1 '
    /^\[Peer\]/ { inpeer=1; block=$0 ORS; next }
    inpeer && /^\[/ { inpeer=0; block="" }
    inpeer { block=block $0 ORS; next }
    !inpeer { print }
    END { }
  ' "$WG_CONF" >"$tmp" || cp "$WG_CONF" "$tmp"
  for sec in "${lines[@]}"; do
    [[ -z "$sec" ]] && continue
    data=$(kubectl -n "$NS" get "secret/$sec" -o jsonpath='{.data.nodes-wg\.json}' 2>/dev/null || true)
    [[ -z "$data" ]] && continue
    while IFS='|' read -r host pub ip; do
      [[ -z "$pub" || -z "$ip" ]] && continue
      ip="${ip%/32}"
      if ! grep -q "$pub" "$tmp"; then
        {
          echo "[Peer]  # cluster $host"
          echo "PublicKey = $pub"
          echo "AllowedIPs = ${ip}/32"
          echo ""
        } >>"$tmp"
      fi
    done < <(echo "$data" | base64 -d | jq -r '.[] | "\(.hostname)|\(.publicKey)|\(.wgAddress)"')
  done
  install -m 600 "$tmp" "$WG_CONF"
  rm -f "$tmp"
fi

wg show "$WG_IF"
echo "[sync-hub-wg-cluster-peers] merged $peers cluster peer(s)"
