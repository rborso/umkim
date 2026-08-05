package script

import (
	"bytes"
	"fmt"
	"text/template"
)

// HostBridgeParams drives configure-kubevirt-bridge.sh in the bootstrap bundle.
type HostBridgeParams struct {
	Enabled    bool
	BridgeName string
	SlaveNic   string // optional override; empty = discover from MGMT_IP
	KeepIP     bool
}

const hostBridgeTmpl = `#!/usr/bin/env bash
# UMKIM — configure host Linux bridge for KubeVirt Multus L2 VMs.
# Idempotent. Designed to survive brief SSH/network blips (re-run safe).
#
# Usage: configure-kubevirt-bridge.sh <mgmt_ipv4>
#   mgmt_ipv4 — node management IP (inventory ansible_host). The NIC that
#   currently holds this address is discovered and enslaved under BRIDGE_NAME.
#   Fails if that address is already on a bridge (or a bridge-slave), unless it
#   is already our target bridge in the expected final state.
set -euo pipefail

BRIDGE_NAME="{{ .BridgeName }}"
SLAVE_NIC_OVERRIDE="{{ .SlaveNic }}"
KEEP_IP="{{ if .KeepIP }}true{{ else }}false{{ end }}"
MGMT_IP="${1:-}"

log() { echo "[fleet-kubevirt-bridge] $*"; }

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    log "ERROR: run as root"; exit 1
  fi
}

require_mgmt_ip() {
  if [[ -z "${MGMT_IP}" ]]; then
    log "ERROR: usage: $0 <mgmt_ipv4>  (inventory ansible_host / node management IP)"
    exit 1
  fi
  if ! [[ "${MGMT_IP}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log "ERROR: mgmt IP must be IPv4 dotted-quad, got: ${MGMT_IP}"
    exit 1
  fi
}

# Device that currently owns MGMT_IP (e.g. eth0 or an existing bridge).
# Prints iface name; returns 1 if not found (does not exit).
iface_for_ip() {
  local ip="$1" iface
  iface="$(ip -4 -o addr show 2>/dev/null | awk -v ip="${ip}" '
    $4 ~ ("^" ip "/") { print $2; exit }
  ')"
  if [[ -z "${iface}" ]]; then
    return 1
  fi
  # Strip @suffix from vlan-style names in -o output (eth0.10@eth0 → eth0.10)
  echo "${iface%%@*}"
  return 0
}

require_iface_for_ip() {
  local ip="$1" iface
  if ! iface="$(iface_for_ip "${ip}")"; then
    log "ERROR: management IP ${ip} not found on any local interface"
    exit 1
  fi
  echo "${iface}"
}

iface_is_bridge() {
  local iface="$1" typ
  typ="$(ip -d link show "${iface}" 2>/dev/null | awk '/^[0-9]+:/ {next} /bridge/ {print "bridge"; exit}')"
  [[ "${typ}" == "bridge" ]] && return 0
  # Fallback: /sys
  [[ -d "/sys/class/net/${iface}/bridge" ]] && return 0
  return 1
}

iface_has_master() {
  ip -o link show "$1" 2>/dev/null | grep -q " master "
}

iface_master_name() {
  ip -o link show "$1" 2>/dev/null | sed -n 's/.* master \([^ ]*\).*/\1/p' | head -1
}

current_ipv4_cidr() {
  ip -4 -o addr show dev "$1" 2>/dev/null | awk '{print $4; exit}'
}

current_gateway() {
  ip -4 route show default 2>/dev/null | awk '{print $3; exit}'
}

# Final desired state: MGMT_IP on BRIDGE_NAME, optional slave under that bridge.
already_ok() {
  local slave="$1"
  ip link show "${BRIDGE_NAME}" >/dev/null 2>&1 || return 1
  iface_is_bridge "${BRIDGE_NAME}" || return 1
  local on_br
  on_br="$(iface_for_ip "${MGMT_IP}" || true)"
  [[ "${on_br}" == "${BRIDGE_NAME}" ]] || return 1
  if [[ -n "${slave}" ]] && ip link show "${slave}" >/dev/null 2>&1; then
    local master
    master="$(iface_master_name "${slave}" || true)"
    [[ "${master}" == "${BRIDGE_NAME}" ]] || return 1
  fi
  return 0
}

# Discover NIC to enslave: override, else the device that holds MGMT_IP (must be a plain NIC).
detect_slave() {
  local ip_iface="$1"

  if [[ -n "${SLAVE_NIC_OVERRIDE}" ]]; then
    if ! ip link show "${SLAVE_NIC_OVERRIDE}" >/dev/null 2>&1; then
      log "ERROR: slaveNic override ${SLAVE_NIC_OVERRIDE} does not exist"
      exit 1
    fi
    echo "${SLAVE_NIC_OVERRIDE}"
    return 0
  fi

  # If IP is already on our target bridge, try to find an enslaved port for already_ok / logging.
  if [[ "${ip_iface}" == "${BRIDGE_NAME}" ]]; then
    local port
    port="$(bridge link show 2>/dev/null | awk -v br="${BRIDGE_NAME}" '
      $0 ~ ("master " br) {
        split($2, a, "@"); gsub(/:$/, "", a[1]); print a[1]; exit
      }
    ')"
    echo "${port}"
    return 0
  fi

  echo "${ip_iface}"
}

# Fail when create-bridge was requested but mgmt IP already lives on a bridge topology
# (unless that topology is already our completed target — handled by already_ok).
assert_not_already_bridged() {
  local ip_iface="$1" slave="$2"
  local master

  if iface_is_bridge "${ip_iface}"; then
    if [[ "${ip_iface}" == "${BRIDGE_NAME}" ]]; then
      return 0
    fi
    log "ERROR: management IP ${MGMT_IP} is on bridge interface ${ip_iface}."
    log "ERROR: refusing to create bridge ${BRIDGE_NAME} — uncheck Configure host bridge,"
    log "ERROR: or bootstrap a node whose management IP is on a plain NIC (not a bridge)."
    exit 1
  fi

  if iface_has_master "${ip_iface}"; then
    master="$(iface_master_name "${ip_iface}")"
    if [[ "${master}" == "${BRIDGE_NAME}" ]]; then
      return 0
    fi
    log "ERROR: management IP ${MGMT_IP} is on ${ip_iface}, which is enslaved to bridge ${master}."
    log "ERROR: refusing to create bridge ${BRIDGE_NAME} — uncheck Configure host bridge,"
    log "ERROR: or use a node whose management IP is not already behind a bridge."
    exit 1
  fi

  if [[ -n "${slave}" ]] && iface_has_master "${slave}"; then
    master="$(iface_master_name "${slave}")"
    if [[ "${master}" != "${BRIDGE_NAME}" ]]; then
      log "ERROR: discovered NIC ${slave} is already enslaved to bridge ${master}."
      log "ERROR: refusing to create bridge ${BRIDGE_NAME}."
      exit 1
    fi
  fi
}

nm_available() {
  command -v nmcli >/dev/null 2>&1 && systemctl is-active --quiet NetworkManager 2>/dev/null
}

configure_with_nmcli() {
  local slave="$1" cidr="$2" gw="$3"
  local br_con slave_con

  br_con="$(nmcli -t -f NAME,DEVICE connection show 2>/dev/null | awk -F: -v d="${BRIDGE_NAME}" '$2==d {print $1; exit}')"
  if [[ -z "${br_con}" ]]; then
    br_con="${BRIDGE_NAME}"
    if [[ "${KEEP_IP}" == "true" && -n "${cidr}" ]]; then
      if [[ -n "${gw}" ]]; then
        nmcli connection add type bridge ifname "${BRIDGE_NAME}" con-name "${br_con}" \
          ipv4.method manual ipv4.addresses "${cidr}" ipv4.gateway "${gw}" \
          ipv4.dns "1.1.1.1,8.8.8.8" connection.autoconnect yes bridge.stp no >/dev/null
      else
        nmcli connection add type bridge ifname "${BRIDGE_NAME}" con-name "${br_con}" \
          ipv4.method manual ipv4.addresses "${cidr}" \
          ipv4.dns "1.1.1.1,8.8.8.8" connection.autoconnect yes bridge.stp no >/dev/null
      fi
    else
      nmcli connection add type bridge ifname "${BRIDGE_NAME}" con-name "${br_con}" \
        ipv4.method disabled ipv6.method ignore \
        connection.autoconnect yes bridge.stp no >/dev/null
    fi
    log "Created NetworkManager bridge connection ${br_con}"
  else
    if [[ "${KEEP_IP}" == "true" && -n "${cidr}" ]]; then
      if [[ -n "${gw}" ]]; then
        nmcli connection modify "${br_con}" ipv4.method manual ipv4.addresses "${cidr}" \
          ipv4.gateway "${gw}" connection.autoconnect yes bridge.stp no
      else
        nmcli connection modify "${br_con}" ipv4.method manual ipv4.addresses "${cidr}" \
          connection.autoconnect yes bridge.stp no
      fi
    fi
    log "Updated NetworkManager bridge connection ${br_con}"
  fi

  if [[ -n "${slave}" ]]; then
    slave_con="$(nmcli -t -f NAME,DEVICE connection show 2>/dev/null | awk -F: -v d="${slave}" '$2==d {print $1; exit}')"
    if [[ -n "${slave_con}" ]]; then
      nmcli connection modify "${slave_con}" ipv4.method disabled ipv6.method ignore 2>/dev/null || true
      nmcli connection down "${slave_con}" 2>/dev/null || true
      nmcli connection delete "${slave_con}" 2>/dev/null || true
      log "Removed prior connection ${slave_con} from ${slave}"
    fi
    slave_con="${BRIDGE_NAME}-port-${slave}"
    if ! nmcli -t -f NAME connection show 2>/dev/null | grep -qx "${slave_con}"; then
      nmcli connection add type bridge-slave ifname "${slave}" master "${BRIDGE_NAME}" \
        con-name "${slave_con}" connection.autoconnect yes >/dev/null
      log "Created bridge-slave connection ${slave_con}"
    fi
  fi

  nmcli connection up "${br_con}" || true
  if [[ -n "${slave_con:-}" ]]; then
    nmcli connection up "${slave_con}" || true
  fi
}

configure_with_ip() {
  local slave="$1" cidr="$2" gw="$3"
  ip link add name "${BRIDGE_NAME}" type bridge 2>/dev/null || true
  ip link set "${BRIDGE_NAME}" up
  if [[ -n "${slave}" ]]; then
    ip link set "${slave}" down || true
    ip addr flush dev "${slave}" || true
    ip link set "${slave}" master "${BRIDGE_NAME}"
    ip link set "${slave}" up
  fi
  if [[ "${KEEP_IP}" == "true" && -n "${cidr}" ]]; then
    ip addr add "${cidr}" dev "${BRIDGE_NAME}" 2>/dev/null || true
    if [[ -n "${gw}" ]]; then
      ip route replace default via "${gw}" dev "${BRIDGE_NAME}" || true
    fi
  fi
  log "Configured bridge ${BRIDGE_NAME} with iproute2 (not persistent across reboot)"
  log "WARN: install NetworkManager for persistent bridge config"
}

main() {
  require_root
  require_mgmt_ip

  local ip_iface slave cidr gw
  ip_iface="$(require_iface_for_ip "${MGMT_IP}")"
  slave="$(detect_slave "${ip_iface}")"
  log "mgmtIp=${MGMT_IP} ipIface=${ip_iface} bridge=${BRIDGE_NAME} slave=${slave:-none} keepIp=${KEEP_IP}"

  if already_ok "${slave}"; then
    log "Bridge ${BRIDGE_NAME} already configured for ${MGMT_IP}; nothing to do"
    exit 0
  fi

  assert_not_already_bridged "${ip_iface}" "${slave}"

  if [[ -z "${slave}" ]]; then
    log "ERROR: could not discover NIC to enslave for management IP ${MGMT_IP}"
    exit 1
  fi
  if iface_is_bridge "${slave}"; then
    log "ERROR: discovered device ${slave} is itself a bridge; refuse to enslave a bridge"
    exit 1
  fi

  cidr=""
  gw=""
  if [[ "${KEEP_IP}" == "true" ]]; then
    cidr="$(current_ipv4_cidr "${ip_iface}" || true)"
    if [[ -z "${cidr}" && -n "${slave}" ]]; then
      cidr="$(current_ipv4_cidr "${slave}" || true)"
    fi
    gw="$(current_gateway || true)"
    if [[ -z "${cidr}" ]]; then
      log "ERROR: keepIp=true but no IPv4 CIDR found for ${MGMT_IP} on ${ip_iface}"
      exit 1
    fi
    log "Will place ${cidr} (gw ${gw:-none}) on ${BRIDGE_NAME}"
  fi

  if nm_available; then
    configure_with_nmcli "${slave}" "${cidr}" "${gw}"
  else
    configure_with_ip "${slave}" "${cidr}" "${gw}"
  fi

  sleep 2
  if ! ip link show "${BRIDGE_NAME}" >/dev/null 2>&1; then
    log "ERROR: bridge ${BRIDGE_NAME} missing after configure"
    exit 1
  fi
  if [[ "${KEEP_IP}" == "true" ]]; then
    local br_ip on
    br_ip="$(current_ipv4_cidr "${BRIDGE_NAME}" || true)"
    [[ -n "${br_ip}" ]] || { log "ERROR: no IPv4 on ${BRIDGE_NAME} after configure"; exit 1; }
    on="$(require_iface_for_ip "${MGMT_IP}")"
    [[ "${on}" == "${BRIDGE_NAME}" ]] || {
      log "ERROR: management IP ${MGMT_IP} is on ${on}, expected ${BRIDGE_NAME}"
      exit 1
    }
    log "Bridge ready: ${BRIDGE_NAME} ${br_ip} (NIC ${slave})"
  else
    log "Bridge ready: ${BRIDGE_NAME} (NIC ${slave})"
  fi
}

main "$@"
`

// RenderHostBridge returns configure-kubevirt-bridge.sh contents, or empty if disabled.
func RenderHostBridge(p HostBridgeParams) (string, error) {
	if !p.Enabled {
		return "", nil
	}
	if p.BridgeName == "" {
		p.BridgeName = "n0p0_mgmt_nw"
	}
	tmpl, err := template.New("host-bridge").Parse(hostBridgeTmpl)
	if err != nil {
		return "", fmt.Errorf("parse host bridge template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("render host bridge template: %w", err)
	}
	return buf.String(), nil
}
