#!/usr/bin/env bash
# Restore hub wg0 peers after reboot. Operator peers come from a static file;
# cluster/site overlay peers are merged from FleetClusterJoin bootstrap secrets.
set -euo pipefail

WG_IF="${WG_IF:-wg0}"
OPERATOR_PEERS_FILE="${OPERATOR_PEERS_FILE:-/etc/wireguard/operator-peers.conf}"
NS="${FLEET_CONSOLE_NS:-fleet-console}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
MODE="${1:-all}"

require_root() {
  [[ "${EUID}" -eq 0 ]] || exec sudo -E bash "$0" "$@"
}

apply_operator_peers() {
  [[ -f "$OPERATOR_PEERS_FILE" ]] || {
    echo "[restore-hub-wg-peers] no operator peers file at $OPERATOR_PEERS_FILE (skip)" >&2
    return 0
  }
  if ! ip link show "$WG_IF" >/dev/null 2>&1; then
    echo "[restore-hub-wg-peers] $WG_IF is not up" >&2
    return 1
  fi
  local count=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    read -r pub ip <<<"$line"
    [[ -z "$pub" || -z "$ip" ]] && continue
    ip="${ip%/32}"
    wg set "$WG_IF" peer "$pub" allowed-ips "${ip}/32" persistent-keepalive 25
    ip route replace "${ip}/32" dev "$WG_IF" 2>/dev/null || true
    count=$((count + 1))
  done <"$OPERATOR_PEERS_FILE"
  echo "[restore-hub-wg-peers] applied $count operator peer(s)"
}

apply_cluster_peers() {
  if ! command -v kubectl >/dev/null; then
    echo "[restore-hub-wg-peers] kubectl not available (skip cluster peers)" >&2
    return 0
  fi
  if ! ip link show "$WG_IF" >/dev/null 2>&1; then
    echo "[restore-hub-wg-peers] $WG_IF is not up" >&2
    return 1
  fi
  local peers=0
  mapfile -t lines < <(
    kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get secrets -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null \
      | grep 'bootstrap$' || true
  )
  for sec in "${lines[@]}"; do
    [[ -z "$sec" ]] && continue
    data=$(kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get "secret/$sec" -o jsonpath='{.data.nodes-wg\.json}' 2>/dev/null || true)
    [[ -z "$data" ]] && continue
    while IFS='|' read -r _host pub ip; do
      [[ -z "$pub" || -z "$ip" ]] && continue
      ip="${ip%/32}"
      wg set "$WG_IF" peer "$pub" allowed-ips "${ip}/32" persistent-keepalive 25
      ip route replace "${ip}/32" dev "$WG_IF" 2>/dev/null || true
      peers=$((peers + 1))
    done < <(echo "$data" | base64 -d | jq -r '.[] | "\(.hostname)|\(.publicKey)|\(.wgAddress)"')
  done
  echo "[restore-hub-wg-peers] applied $peers cluster peer(s)"
}

require_root "$@"

case "$MODE" in
  --operators-only)
    apply_operator_peers
    ;;
  --clusters-only)
    apply_cluster_peers
    ;;
  all|--all)
    apply_operator_peers
    apply_cluster_peers
    ;;
  *)
    echo "usage: $0 [--operators-only|--clusters-only|all]" >&2
    exit 2
    ;;
esac

wg show "$WG_IF" 2>/dev/null || true
