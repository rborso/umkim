#!/usr/bin/env bash
# Canonical bootstrap bundle embed tree (go:embed). Run before fleet-site-controller build.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DST="$(dirname "$0")/internal/bundle/static"
BUNDLE_CFG="${ROOT}/images/fleet-bootstrap-bundle"

rm -rf "${DST}"
mkdir -p "${DST}/roles"

for f in wireguard-install.yml cluster-provision-local.yml cluster-validate.yml; do
  cp -f "${ROOT}/${f}" "${DST}/${f}"
done
cp -f "${BUNDLE_CFG}/ansible.cfg" "${DST}/ansible.cfg"
cp -f "${BUNDLE_CFG}/requirements.yml" "${DST}/requirements.yml"

for role in k3s calico harbor fleet_cluster_ssh fleet_cluster_wireguard fleet_cluster_storage fleet_cluster_longhorn fleet_site_validate kube_vip host_prepare; do
  cp -a "${ROOT}/roles/${role}" "${DST}/roles/"
done

# Node-local detach/reset helper for operators (also used by fleet-ansible-runner delete Jobs).
cp -f "${ROOT}/cleanup_node_local.sh" "${DST}/cleanup_node_local.sh"
chmod 0755 "${DST}/cleanup_node_local.sh"

echo "Synced bundle static at ${DST}"
