#!/usr/bin/env bash
# Seed native Ansible/YAIDAP inventory YAML on the hub.
seed_inventory_yaml() {
  local tmpdir
  tmpdir="$(mktemp -d)"
  mkdir -p "${tmpdir}/host_vars" "${tmpdir}/group_vars/all"

  cat >"${tmpdir}/guests.yml" <<EOF
guests:
  hosts:
    ${SINGLE_HOST}:
      ansible_host: ${SINGLE_IP}
      vm_location: least-utilized
    ${HA_CP0}:
      ansible_host: ${HA_IP0}
      vm_location: least-utilized
    ${HA_CP1}:
      ansible_host: ${HA_IP1}
      vm_location: least-utilized
    ${HA_CP2}:
      ansible_host: ${HA_IP2}
      vm_location: least-utilized
external:
  hosts: {}
EOF

  cat >"${tmpdir}/hypervisors.yml" <<EOF
hypervisors:
  hosts:
    virt01:
      ansible_host: 172.16.10.2
    virt02:
      ansible_host: 172.16.10.3
EOF

  cat >"${tmpdir}/bare_metals.yml" <<EOF
bare_metals:
  hosts:
    virt03:
      ansible_host: 172.16.10.4
EOF

  cat >"${tmpdir}/group_vars/all/all.yml" <<EOF
---
memory: 4096
cores: 2
libvirt_dir: /opt/local
storages:
  - name: local
    type: local
    shared: false
    path: /opt/local
    os: [rocky, fedora]
networks:
  - network_name: lab_nw
    zone: lab
EOF

  for guest in "$SINGLE_HOST" "$HA_CP0" "$HA_CP1" "$HA_CP2"; do
    cat >"${tmpdir}/host_vars/${guest}.yml" <<EOF
memory: 4096
cores: 2
os: rocky
uplinks:
  - name: lan
    network: lan
    ip: dhcp
EOF
  done

  cat >"${tmpdir}/${SINGLE}.yml" <<EOF
${SINGLE}:
  hosts:
    ${SINGLE_HOST}:
EOF

  cat >"${tmpdir}/group_vars/${SINGLE}.yml" <<EOF
topology: single_node
autoApply: true
EOF

  cat >"${tmpdir}/${HA_CLUSTER}.yml" <<EOF
${HA_CLUSTER}:
  hosts:
    ${HA_CP0}:
    ${HA_CP1}:
    ${HA_CP2}:
EOF

  cat >"${tmpdir}/group_vars/${HA_CLUSTER}.yml" <<EOF
topology: ha_3_node
autoApply: true
haGroup: ${HA_CLUSTER}
controlPlaneVip: ${HA_VIP}
EOF

  run "$UMKIM site inventory migrate $SITE"
  run "$UMKIM site inventory files put $SITE --path guests.yml --file ${tmpdir}/guests.yml"
  run "$UMKIM site inventory files put $SITE --path hypervisors.yml --file ${tmpdir}/hypervisors.yml"
  run "$UMKIM site inventory files put $SITE --path bare_metals.yml --file ${tmpdir}/bare_metals.yml"
  run "$UMKIM site inventory files put $SITE --path group_vars/all/all.yml --file ${tmpdir}/group_vars/all/all.yml"
  for guest in "$SINGLE_HOST" "$HA_CP0" "$HA_CP1" "$HA_CP2"; do
    run "$UMKIM site inventory files put $SITE --path host_vars/${guest}.yml --file ${tmpdir}/host_vars/${guest}.yml"
  done
  run "$UMKIM site inventory files put $SITE --path ${SINGLE}.yml --file ${tmpdir}/${SINGLE}.yml"
  run "$UMKIM site inventory files put $SITE --path group_vars/${SINGLE}.yml --file ${tmpdir}/group_vars/${SINGLE}.yml"
  run "$UMKIM site inventory files put $SITE --path ${HA_CLUSTER}.yml --file ${tmpdir}/${HA_CLUSTER}.yml"
  run "$UMKIM site inventory files put $SITE --path group_vars/${HA_CLUSTER}.yml --file ${tmpdir}/group_vars/${HA_CLUSTER}.yml"
}
