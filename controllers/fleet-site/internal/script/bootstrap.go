package script

import (
	"bytes"
	"fmt"
	"text/template"
)

type Params struct {
	SiteName               string
	ClusterName            string
	EnrollmentToken        string
	HubWireguardIP         string
	EnrollmentBaseURL      string
	WGConfigBlock          string
	WGPublicKey            string
	RockyMajor             string
	BundleMode             bool
	WireguardEndpointHost  string
	WireguardEndpointPort  string
	// KubevirtBridgeEnabled embeds host-bridge configure steps in bootstrap.sh.
	KubevirtBridgeEnabled bool
}

const bootstrapTmpl = `#!/usr/bin/env bash
# UMKIM fleet bootstrap — idempotent.{{ if .ClusterName }} Cluster: {{ .ClusterName }}{{ else }} Site: {{ .SiteName }}{{ end }}
set -euo pipefail

FLEET_SITE_NAME="{{ .SiteName }}"
FLEET_CLUSTER_NAME="{{ .ClusterName }}"
FLEET_SITE_ENROLL_TOKEN="{{ .EnrollmentToken }}"
FLEET_HUB_WG_IP="{{ .HubWireguardIP }}"
{{ if .ClusterName -}}
ENROLL_URL="{{ .EnrollmentBaseURL }}/cluster-enrollments/{{ .ClusterName }}"
{{ else -}}
ENROLL_URL="{{ .EnrollmentBaseURL }}/site-enrollments/${FLEET_SITE_NAME}"
{{ end -}}
WG_IFACE="wg-umkim"
WG_CONF="/etc/wireguard/${WG_IFACE}.conf"
WG_CONF_NEW="/etc/wireguard/${WG_IFACE}.conf.umkim-new"
WG_UNIT="wg-quick@${WG_IFACE}"

log() { echo "[fleet-site-bootstrap] $*"; }

require_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    return 0
  fi
  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    exec sudo -E bash "$0" "$@"
  fi
  log "Run as root or with passwordless sudo: sudo $0"; exit 1
}

assert_rocky9() {
  [[ -f /etc/os-release ]] || { log "ERROR: /etc/os-release missing"; exit 1; }
  # shellcheck source=/dev/null
  source /etc/os-release
  [[ "${ID}" == "rocky" && "${VERSION_ID%%.*}" == "{{ .RockyMajor }}" ]] || {
    log "ERROR: Rocky Linux {{ .RockyMajor }} required (found ${ID:-?} ${VERSION_ID:-?})"; exit 1; }
}

install_packages() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y wireguard-tools curl iputils openssh-clients
  else
    yum install -y wireguard-tools curl iputils openssh-clients
  fi
}

write_wg_config() {
  install -d -m 0700 /etc/wireguard
  cat >"${WG_CONF_NEW}" <<'WGEOF'
{{ .WGConfigBlock }}WGEOF
  if [[ -f "${WG_CONF}" ]] && cmp -s "${WG_CONF}" "${WG_CONF_NEW}"; then
    rm -f "${WG_CONF_NEW}"
    log "WireGuard config unchanged"
  else
    mv "${WG_CONF_NEW}" "${WG_CONF}"
    chmod 600 "${WG_CONF}"
    log "WireGuard config updated"
  fi
}

start_wg() {
  systemctl daemon-reload 2>/dev/null || true
  systemctl enable "${WG_UNIT}" 2>/dev/null || true
  if systemctl is-active --quiet "${WG_UNIT}" 2>/dev/null; then
    systemctl restart "${WG_UNIT}" || { wg-quick down "${WG_IFACE}" 2>/dev/null || true; wg-quick up "${WG_IFACE}"; systemctl enable "${WG_UNIT}"; }
  elif wg show "${WG_IFACE}" >/dev/null 2>&1; then
    wg-quick down "${WG_IFACE}" 2>/dev/null || true
    wg-quick up "${WG_IFACE}"
    systemctl enable "${WG_UNIT}"
  else
    systemctl start "${WG_UNIT}" 2>/dev/null || wg-quick up "${WG_IFACE}"
    systemctl enable "${WG_UNIT}"
  fi
}

wait_for_hub() {
  local i
  for i in $(seq 1 60); do
    ping -c1 -W2 "${FLEET_HUB_WG_IP}" >/dev/null 2>&1 && { log "Hub ${FLEET_HUB_WG_IP} reachable"; return 0; }
    sleep 2
  done
  log "ERROR: cannot ping hub ${FLEET_HUB_WG_IP}"; exit 1
}

post_enrollment() {
  local hostname rocky body code
  hostname="$(hostname -s 2>/dev/null || hostname)"
  rocky="$(. /etc/os-release && echo "${VERSION_ID}")"
  body=$(printf '{"hostname":"%s","rockyVersion":"%s","wgPublicKey":"%s","scriptVersion":"1"}' \
    "${hostname}" "${rocky}" "{{ .WGPublicKey }}")
  code=$(curl -s -o /tmp/fleet-site-enroll.json -w '%{http_code}' \
    -X POST "${ENROLL_URL}" \
    -H "Authorization: Bearer ${FLEET_SITE_ENROLL_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${body}")
  [[ "${code}" == "200" || "${code}" == "409" ]] || {
    log "ERROR: enrollment failed HTTP ${code}"; cat /tmp/fleet-site-enroll.json 2>/dev/null; exit 1; }
  log "Enrollment OK (HTTP ${code})"
}

main() {
  require_root
  assert_rocky9
  install_packages
  write_wg_config
  start_wg
  wait_for_hub
  post_enrollment
  log "Done. Use Fleet Console: Validate → Provision."
}

main "$@"
`

const bootstrapBundleTmpl = `#!/usr/bin/env bash
# UMKIM fleet bootstrap bundle — local validate + provision (ha_3_node). Cluster: {{ .ClusterName }}
set -euo pipefail

BUNDLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLEET_CLUSTER_NAME="{{ .ClusterName }}"
FLEET_SITE_ENROLL_TOKEN="$(tr -d '\n' < "${BUNDLE_DIR}/enrollment-token")"
FLEET_HUB_WG_IP="{{ .HubWireguardIP }}"
ENROLL_URL="{{ .EnrollmentBaseURL }}/cluster-enrollments/{{ .ClusterName }}"
PROGRESS_URL="{{ .EnrollmentBaseURL }}/cluster-enrollments/{{ .ClusterName }}/progress"

log() { echo "[fleet-cluster-bundle] $*"; }

require_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    return 0
  fi
  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    exec sudo -E bash "$0" "$@"
  fi
  log "Run as root or with passwordless sudo: sudo $0"; exit 1
}

assert_rocky9() {
  [[ -f /etc/os-release ]] || { log "ERROR: /etc/os-release missing"; exit 1; }
  # shellcheck source=/dev/null
  source /etc/os-release
  [[ "${ID}" == "rocky" && "${VERSION_ID%%.*}" == "{{ .RockyMajor }}" ]] || {
    log "ERROR: Rocky Linux {{ .RockyMajor }} required (found ${ID:-?} ${VERSION_ID:-?})"; exit 1; }
}

install_packages() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y wireguard-tools curl iputils openssh-server openssh-clients python3-pip rsync
  else
    yum install -y wireguard-tools curl iputils openssh-server openssh-clients python3-pip rsync
  fi
  systemctl enable --now sshd 2>/dev/null || true
}

bundle_ssh_pubkey() {
  local key_file="${BUNDLE_DIR}/ssh/id_rsa"
  if [[ -f "${BUNDLE_DIR}/ssh/id_rsa.pub" ]]; then
    tr -d '\n' < "${BUNDLE_DIR}/ssh/id_rsa.pub"
  elif [[ -f "${key_file}" ]]; then
    ssh-keygen -y -f "${key_file}" | tr -d '\n'
  fi
}

install_bundle_ssh_key_local() {
  local pubkey auth
  pubkey="$(bundle_ssh_pubkey || true)"
  [[ -n "${pubkey}" ]] || return 0
  install -d -m 0700 -o linux -g linux /home/linux/.ssh
  auth=/home/linux/.ssh/authorized_keys
  touch "${auth}"
  chown linux:linux "${auth}"
  chmod 600 "${auth}"
  if ! grep -qF "${pubkey}" "${auth}"; then
    echo "${pubkey}" >> "${auth}"
    log "Installed bundle SSH public key for linux"
  else
    log "Bundle SSH public key already present for linux"
  fi
}

is_local_mgmt_host() {
  local host="$1" ip
  [[ -n "${host}" ]] || return 1
  for ip in $(hostname -I 2>/dev/null); do
    [[ "${ip}" == "${host}" ]] && return 0
  done
  while IFS= read -r ip; do
    [[ "${ip}" == "${host}" ]] && return 0
  done < <(ip -4 -o addr show 2>/dev/null | awk '{print $4}' | cut -d/ -f1)
  return 1
}

# ansible_host values from [fleet_cluster_control] / [fleet_cluster_workers] only.
# Any other [section] must clear the active group — do not keep matching after
# [fleet_cluster_nodes:vars], [site_bootstrap], [hub], etc. (hub is 10.10.0.1).
inventory_mgmt_ips() {
  awk '
    /^\[fleet_cluster_control\]|^\[fleet_cluster_workers\]/ { sect=1; next }
    /^\[/ { sect=0; next }
    sect && /ansible_host=/ {
      for (i = 1; i <= NF; i++) if ($i ~ /^ansible_host=/) { sub(/^ansible_host=/, "", $i); print $i }
    }' "${BUNDLE_DIR}/inventory.ini"
}

seed_bundle_ssh_to_peers() {
  local pubkey host key_file="${BUNDLE_DIR}/ssh/id_rsa"
  local ssh_opts=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10 -o BatchMode=yes)
  pubkey="$(bundle_ssh_pubkey || true)"
  [[ -n "${pubkey}" ]] || return 0
  if [[ ! -f "${key_file}" ]]; then
    log "WARN: bundle SSH private key missing; skipping peer seed"
    return 0
  fi
  chmod 600 "${key_file}"
  while IFS= read -r host; do
    [[ -n "${host}" ]] || continue
    if is_local_mgmt_host "${host}"; then
      log "Skipping SSH peer seed for local host ${host} (authorized_keys already updated)"
      continue
    fi
    if ssh -i "${key_file}" "${ssh_opts[@]}" "linux@${host}" \
      "grep -qF '${pubkey%% *}' ~/.ssh/authorized_keys 2>/dev/null || { mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${pubkey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys; }"; then
      log "Bundle SSH public key present on ${host}"
    else
      log "WARN: could not seed bundle SSH key on ${host} (ansible may still reach it)"
    fi
  done < <(inventory_mgmt_ips)
}

install_ansible() {
  export PATH="/usr/local/bin:${PATH}"
  if ! command -v ansible-playbook >/dev/null 2>&1; then
    pip3 install --quiet ansible-core
  fi
  # ansible.utils ipaddr filter (k3s TLS SAN, API bind) requires netaddr on the node.
  if ! python3 -c 'import netaddr' 2>/dev/null; then
    pip3 install --quiet netaddr
  fi
  export PATH="/usr/local/bin:${PATH}"
}

{{ if .KubevirtBridgeEnabled -}}
local_mgmt_ip_from_inventory() {
  local host
  while IFS= read -r host; do
    [[ -n "${host}" ]] || continue
    if is_local_mgmt_host "${host}"; then
      echo "${host}"
      return 0
    fi
  done < <(inventory_mgmt_ips)
  return 1
}

run_kubevirt_host_bridge_local() {
  local script="${BUNDLE_DIR}/configure-kubevirt-bridge.sh"
  local mgmt_ip
  [[ -x "${script}" ]] || { log "ERROR: missing ${script}"; exit 1; }
  mgmt_ip="$(local_mgmt_ip_from_inventory || true)"
  if [[ -z "${mgmt_ip}" ]]; then
    log "ERROR: could not match a local address to inventory ansible_host for bridge configure"
    exit 1
  fi
  log "Configuring KubeVirt host bridge on local node (mgmt IP ${mgmt_ip})"
  # systemd-run survives SSH drop if mgmt IP moves onto the bridge.
  if command -v systemd-run >/dev/null 2>&1; then
    systemd-run --quiet --collect --wait --unit="fleet-kubevirt-bridge-local" \
      bash "${script}" "${mgmt_ip}" || bash "${script}" "${mgmt_ip}"
  else
    bash "${script}" "${mgmt_ip}"
  fi
}

run_kubevirt_host_bridge_peers() {
  local script="${BUNDLE_DIR}/configure-kubevirt-bridge.sh"
  local key_file="${BUNDLE_DIR}/ssh/id_rsa"
  local ssh_opts=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=15 -o BatchMode=yes -o ServerAliveInterval=2)
  local host remote
  [[ -f "${key_file}" ]] || { log "ERROR: no bundle SSH key; cannot configure peer bridges"; exit 1; }
  chmod 600 "${key_file}"
  while IFS= read -r host; do
    [[ -n "${host}" ]] || continue
    if is_local_mgmt_host "${host}"; then
      continue
    fi
    log "Configuring KubeVirt host bridge on peer ${host}"
    remote="/tmp/configure-kubevirt-bridge.sh"
    scp -i "${key_file}" "${ssh_opts[@]}" "${script}" "linux@${host}:${remote}" || {
      log "ERROR: scp bridge script to ${host} failed"; exit 1
    }
    # Pass peer ansible_host so the script discovers that node's NIC; fail bootstrap on error.
    if ssh -i "${key_file}" "${ssh_opts[@]}" "linux@${host}" \
      "sudo systemd-run --quiet --collect --wait --unit=fleet-kubevirt-bridge bash ${remote} ${host} || sudo bash ${remote} ${host}"; then
      log "Peer ${host} bridge configure OK"
    else
      log "ERROR: peer ${host} bridge configure failed"
      exit 1
    fi
  done < <(inventory_mgmt_ips)
}
{{ end -}}

wait_for_hub() {
  local i
  for i in $(seq 1 60); do
    ping -c1 -W2 "${FLEET_HUB_WG_IP}" >/dev/null 2>&1 && { log "Hub ${FLEET_HUB_WG_IP} reachable"; return 0; }
    sleep 2
  done
  log "ERROR: cannot ping hub ${FLEET_HUB_WG_IP}"; exit 1
}

post_enrollment() {
  local hostname rocky body code
  hostname="$(hostname -s 2>/dev/null || hostname)"
  rocky="$(. /etc/os-release && echo "${VERSION_ID}")"
  body=$(printf '{"hostname":"%s","rockyVersion":"%s","wgPublicKey":"%s","scriptVersion":"2"}' \
    "${hostname}" "${rocky}" "{{ .WGPublicKey }}")
  code=$(curl -s -o /tmp/fleet-cluster-enroll.json -w '%{http_code}' \
    -X POST "${ENROLL_URL}" \
    -H "Authorization: Bearer ${FLEET_SITE_ENROLL_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${body}")
  [[ "${code}" == "200" || "${code}" == "409" ]] || {
    log "ERROR: enrollment failed HTTP ${code}"; cat /tmp/fleet-cluster-enroll.json 2>/dev/null; exit 1; }
  log "Enrollment OK (HTTP ${code})"
}

post_progress() {
  local phase msg body code
  phase="${1:-Validated}"
  msg="${2:-Local provision complete; use Console Provision for hub finalize}"
  body=$(printf '{"phase":"%s","message":"%s","scriptVersion":"2"}' "${phase}" "${msg}")
  code=$(curl -s -o /tmp/fleet-cluster-progress.json -w '%{http_code}' \
    -X POST "${PROGRESS_URL}" \
    -H "Authorization: Bearer ${FLEET_SITE_ENROLL_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${body}")
  [[ "${code}" == "200" ]] || {
    log "ERROR: progress POST failed HTTP ${code}"; cat /tmp/fleet-cluster-progress.json 2>/dev/null; exit 1; }
  log "Progress OK (${phase})"
}

run_ansible() {
  export PATH="/usr/local/bin:${PATH}"
  export ANSIBLE_CONFIG="${BUNDLE_DIR}/ansible.cfg"
  export ANSIBLE_ROLES_PATH="${BUNDLE_DIR}/roles"
  cd "${BUNDLE_DIR}"
  ansible-galaxy collection install -r requirements.yml
  ansible-playbook "$1" -i inventory.ini \
    -e "fleet_cluster_name=${FLEET_CLUSTER_NAME}" \
    -e "fleet_site_name=${FLEET_CLUSTER_NAME}" \
    -e "fleet_site_id={{ .SiteName }}" \
    -e "fleet_cluster_bundle_dir=${BUNDLE_DIR}" \
    -e "wireguard_endpoint_host={{ .WireguardEndpointHost }}" \
    -e "wireguard_endpoint_port={{ .WireguardEndpointPort }}"
}

main() {
  require_root
  assert_rocky9
  install_packages
  install_bundle_ssh_key_local
  install_ansible
{{ if .KubevirtBridgeEnabled }}
  run_kubevirt_host_bridge_local
{{ end }}
  seed_bundle_ssh_to_peers
{{ if .KubevirtBridgeEnabled }}
  run_kubevirt_host_bridge_peers
{{ end }}
  run_ansible wireguard-install.yml
  wait_for_hub
  post_enrollment
  run_ansible cluster-validate.yml
  run_ansible cluster-provision-local.yml
  post_progress Validated "Local provision complete; use Fleet Console Provision for hub finalize"
  log "Done. Open Fleet Console and click Provision."
}

main "$@"
`

func WGConfigBlock(priv, pub, addr, hubPub, endpointHost, endpointPort, keepalive, subnet string) string {
	return fmt.Sprintf(`# UMKIM fleet site overlay
[Interface]
Address = %s
PrivateKey = %s

[Peer]
PublicKey = %s
Endpoint = %s:%s
AllowedIPs = %s
PersistentKeepalive = %s
`, addr, priv, hubPub, endpointHost, endpointPort, subnet, keepalive)
}

func Render(p Params) (string, error) {
	tmpl := bootstrapTmpl
	if p.BundleMode {
		tmpl = bootstrapBundleTmpl
	}
	t, err := template.New("bootstrap").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, p); err != nil {
		return "", err
	}
	return buf.String(), nil
}
