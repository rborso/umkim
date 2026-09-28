#!/usr/bin/env bash
# Run from repo root before harbor push: stages playbooks into fleet-ansible-runner build dir.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DST="$(dirname "$0")"
for f in site-validate.yml site-provision.yml site-provision-connector.yml cluster-validate.yml cluster-provision.yml \
  cluster-provision-local.yml cluster-provision-hub.yml cluster-discover-capabilities.yml \
  cluster-delete-hub.yml cluster-configure-storage.yml cluster-configure-kubevirt.yml \
  cluster-gpu-host-prep.yml cluster-gpu-enable.yml cluster-gpu-firewall.yml cluster-validate-storage.yml \
  cluster-overlay-recover.yml \
  ingress-auth-reconcile.yml acme-reconcile.yml llm-reconcile.yml wireguard-install.yml \
  ansible.cfg; do
  cp -f "${ROOT}/${f}" "${DST}/${f}"
done
for f in cluster-add-worker.yml cluster-remove-worker.yml cluster-replace-control.yml cluster-kubeconfig-sync.yml; do
  if [[ -f "${ROOT}/${f}" ]]; then
    cp -f "${ROOT}/${f}" "${DST}/${f}"
  fi
done
cp -f "${ROOT}/cleanup_node_local.sh" "${DST}/cleanup_node_local.sh"
cp -f "${ROOT}/requirements.yml" "${DST}/requirements.yml"
cp -f "${ROOT}/LICENSE" "${ROOT}/NOTICE" "${DST}/"
rm -rf "${DST}/roles" "${DST}/inventory" "${DST}/charts" "${DST}/deploy"
mkdir -p "${DST}/roles" "${DST}/charts" "${DST}/deploy/fleet-edge"
cp -a "${ROOT}/roles/fleet_site_validate" "${ROOT}/roles/fleet_site" "${ROOT}/roles/fleet_cluster_wireguard" \
  "${ROOT}/roles/fleet_cluster_storage" "${ROOT}/roles/fleet_cluster_longhorn" "${ROOT}/roles/kube_vip" \
  "${ROOT}/roles/k3s" "${ROOT}/roles/calico" "${ROOT}/roles/ocm" "${ROOT}/roles/harbor" \
  "${ROOT}/roles/host_prepare" "${ROOT}/roles/hub_ingress" \
  "${ROOT}/roles/olm" "${ROOT}/roles/kubevirt_stack" \
  "${ROOT}/roles/kubevirt_bridge_nad" "${ROOT}/roles/kubevirt_host_bridge" \
  "${ROOT}/roles/spoke_ingress" "${ROOT}/roles/fleet_ingress_auth" "${ROOT}/roles/fleet_acme" "${ROOT}/roles/fleet_llm" "${ROOT}/roles/fleet_hub_firewall" "${ROOT}/roles/fleet_edge" \
  "${ROOT}/roles/fleet_edge_controller" "${ROOT}/roles/gpu_host" "${ROOT}/roles/fleet_spoke_firewall" "${DST}/roles/"
mkdir -p "${DST}/data/gpu"
cp -f "${ROOT}/data/gpu/catalog.yaml" "${DST}/data/gpu/"
cp -f "${ROOT}/cluster-gpu-host-prep.yml" "${ROOT}/cluster-gpu-enable.yml" "${ROOT}/cluster-gpu-firewall.yml" "${DST}/"
cp -a "${ROOT}/charts/fleet-ingress-auth" "${DST}/charts/"
cp -f "${ROOT}/deploy/fleet-edge/rbac.yaml" "${DST}/deploy/fleet-edge/"
mkdir -p "${DST}/inventory/group_vars/all"
for inv in common.yml site.yml fleet_site.yml fleet_backup.yml; do
  if [[ -f "${ROOT}/inventory/group_vars/all/${inv}" ]]; then
    cp -f "${ROOT}/inventory/group_vars/all/${inv}" "${DST}/inventory/group_vars/all/"
  fi
done
