#!/usr/bin/env bash
# Run ON a hypervisor (virt02 or virt03) to create inventory Rocky VMs locally.
# Usage on hypervisor:
#   sudo ./scripts/provision-dev-machines-vms-local.sh virt02
#   sudo ./scripts/provision-dev-machines-vms-local.sh virt03
#
# From jump host:
#   scp -J linux@jumphost.example.com scripts/provision-dev-machines-vms-local.sh linux@203.0.113.14:
#   ssh -J linux@jumphost.example.com linux@203.0.113.14 'sudo bash ~/provision-dev-machines-vms-local.sh virt02'
set -euo pipefail

HYPER="${1:-}"
BASE_IMAGE="/var/lib/libvirt/images/Rocky-9-GenericCloud-Base.qcow2"
IMAGE_DIR="/var/lib/libvirt/images"
CLOUD_DIR="/var/lib/libvirt/cloud-init"
OS_VARIANT="rocky9"
SSH_USER="linux"

usage() {
  echo "Usage: $0 virt02|virt03" >&2
  exit 1
}

[[ -n "${HYPER}" ]] || usage
[[ "${EUID}" -eq 0 ]] || { echo "run as root (sudo)" >&2; exit 1; }
[[ -r "${BASE_IMAGE}" ]] || { echo "missing ${BASE_IMAGE}" >&2; exit 1; }

AUTHORIZED_KEYS="$(cat "/home/${SSH_USER}/.ssh/authorized_keys" 2>/dev/null || true)"
if [[ -z "${AUTHORIZED_KEYS// }" ]]; then
  echo "no keys in /home/${SSH_USER}/.ssh/authorized_keys" >&2
  exit 1
fi

vm_exists() { virsh dominfo "$1" &>/dev/null; }
vm_running() { [[ "$(virsh domstate "$1" 2>/dev/null || echo missing)" == "running" ]]; }

create_os_disk() {
  local name="$1" size_gb="$2"
  local path="${IMAGE_DIR}/${name}.qcow2"
  if [[ ! -f "${path}" ]]; then
    qemu-img create -f qcow2 -F qcow2 -b "${BASE_IMAGE}" "${path}" "${size_gb}G"
  fi
}

create_data_disk() {
  local name="$1" suffix="$2" size_gb="$3"
  local path="${IMAGE_DIR}/${name}-${suffix}.qcow2"
  if [[ ! -f "${path}" ]]; then
    qemu-img create -f qcow2 "${path}" "${size_gb}G"
  fi
}

render_cloud_init() {
  local name="$1" ip="$2" gateway="$3" prefix="$4" dns="$5"
  local dir="${CLOUD_DIR}/${name}"
  mkdir -p "${dir}"
  cat > "${dir}/meta-data" <<EOF
instance-id: ${name}
local-hostname: ${name}
EOF
  cat > "${dir}/network-config" <<EOF
version: 2
ethernets:
  eth0:
    addresses:
      - ${ip}/${prefix}
    gateway4: ${gateway}
    nameservers:
      addresses:
        - ${dns}
EOF
  cat > "${dir}/user-data" <<EOF
#cloud-config
users:
  - name: rocky
    lock_passwd: true
  - name: ${SSH_USER}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
$(echo "${AUTHORIZED_KEYS}" | sed 's/^/      - /')
ssh_pwauth: false
runcmd:
  - nmcli con mod "Wired connection 1" ipv4.method manual ipv4.addresses ${ip}/${prefix} ipv4.gateway ${gateway} ipv4.dns "${dns}" || true
  - nmcli con up "Wired connection 1" || true
EOF
}

define_vm() {
  local name="$1" ip="$2" bridge="$3" memory_mib="$4" vcpus="$5" os_gb="$6"
  shift 6
  local extra_disks=("$@")

  create_os_disk "${name}" "${os_gb}"
  render_cloud_init "${name}" "${ip}" "192.168.1.1" "24" "192.168.1.1"

  local -a disk_args=(--disk "path=${IMAGE_DIR}/${name}.qcow2,format=qcow2")
  for spec in "${extra_disks[@]}"; do
    IFS=: read -r suffix size_gb <<< "${spec}"
    create_data_disk "${name}" "${suffix}" "${size_gb}"
    disk_args+=(--disk "path=${IMAGE_DIR}/${name}-${suffix}.qcow2,format=qcow2")
  done

  if vm_exists "${name}"; then
    echo "VM ${name} already defined"
  else
    virt-install \
      --name "${name}" \
      --memory "${memory_mib}" \
      --vcpus "${vcpus}" \
      --import \
      "${disk_args[@]}" \
      --network "bridge=${bridge},model=virtio" \
      --os-variant "${OS_VARIANT}" \
      --cloud-init "user-data=${CLOUD_DIR}/${name}/user-data,meta-data=${CLOUD_DIR}/${name}/meta-data,network-config=${CLOUD_DIR}/${name}/network-config" \
      --noautoconsole \
      --autostart
  fi

  if ! vm_running "${name}"; then
    virsh start "${name}"
  fi
  echo "OK ${name} ${ip} state=$(virsh domstate "${name}")"
}

case "${HYPER}" in
  virt02)
    define_vm virt02-ha-cp1 192.168.1.81 n0p0_mgmt_nw 8192 4 50 longhorn:50
    define_vm virt02-ha-cp2 192.168.1.82 n0p0_mgmt_nw 8192 4 50 longhorn:50
    define_vm virt02-ha-cp3 192.168.1.83 n0p0_mgmt_nw 8192 4 50 longhorn:50
    ;;
  virt03)
    define_vm virt03-gpu-01 192.168.1.91 n0p0_mgmt_nw 10240 6 50
    define_vm virt03-single-01 192.168.1.92 n0p0_mgmt_nw 8192 4 50
    define_vm virt03-single-02 192.168.1.93 n0p0_mgmt_nw 8192 4 50
    ;;
  virt01)
    echo "virt01 has no inventory VBM guests (bare-metal spoke only)" >&2
    exit 0
    ;;
  *)
    usage
    ;;
esac

echo "--- virsh list --all ---"
virsh list --all
