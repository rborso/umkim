#!/usr/bin/env bash
# Wait until WireGuard peer Endpoint hostnames in /etc/wireguard/<iface>.conf resolve.
# Used as ExecStartPre for wg-quick@ so boot races (DNS after network-online) do not fail the unit permanently.
set -euo pipefail

IFACE="${1:-}"
if [[ -z "${IFACE}" ]]; then
  echo "usage: $0 <wireguard-iface>" >&2
  exit 2
fi

CONF="/etc/wireguard/${IFACE}.conf"
if [[ ! -f "${CONF}" ]]; then
  echo "umkim-wg-wait-dns: missing ${CONF}" >&2
  exit 1
fi

# Collect unique hostnames from Endpoint = host:port lines (ignore IPs).
mapfile -t HOSTS < <(
  awk -F= '
    tolower($1) ~ /^[[:space:]]*endpoint[[:space:]]*$/ {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
      split($2, a, ":")
      h = a[1]
      if (h ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) next
      if (h ~ /^\[/ ) next
      if (length(h) > 0) print h
    }
  ' "${CONF}" | sort -u
)

if [[ ${#HOSTS[@]} -eq 0 ]]; then
  exit 0
fi

TIMEOUT_SEC="${UMKIM_WG_DNS_WAIT_SEC:-120}"
deadline=$((SECONDS + TIMEOUT_SEC))

resolve_ok() {
  local h="$1"
  if getent hosts "${h}" >/dev/null 2>&1; then
    return 0
  fi
  if command -v dig >/dev/null 2>&1 && dig +short +time=2 +tries=1 "${h}" A >/dev/null 2>&1; then
    local ans
    ans="$(dig +short +time=2 +tries=1 "${h}" A 2>/dev/null | head -1)"
    [[ -n "${ans}" ]]
    return $?
  fi
  return 1
}

echo "umkim-wg-wait-dns: waiting up to ${TIMEOUT_SEC}s for: ${HOSTS[*]}"
while (( SECONDS < deadline )); do
  all_ok=1
  for h in "${HOSTS[@]}"; do
    if ! resolve_ok "${h}"; then
      all_ok=0
      break
    fi
  done
  if (( all_ok == 1 )); then
    echo "umkim-wg-wait-dns: DNS ready"
    exit 0
  fi
  sleep 2
done

echo "umkim-wg-wait-dns: timed out resolving ${HOSTS[*]}" >&2
exit 1
