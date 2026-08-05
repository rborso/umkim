#!/usr/bin/env bash
# Copy greenfield hub-only inventory template into inventory/.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${ROOT}/examples/inventory/hub-only"
DEST="${ROOT}/inventory"
FORCE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --force) FORCE=true; shift ;;
    -h|--help)
      echo "Usage: $0 [--force]"
      echo "Copies examples/inventory/hub-only into inventory/"
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

copy_file() {
  local rel="$1"
  mkdir -p "$(dirname "${DEST}/${rel}")"
  cp "${SRC}/${rel}" "${DEST}/${rel}"
}

copy_file hosts.yml
copy_file hosts-from-hub.yml
copy_file host_vars/umkim-hub.yml
copy_file group_vars/all/site.yml
copy_file group_vars/all/profile.yml
copy_file group_vars/all/common.yml
copy_file group_vars/all/fleet_git_pins.yml
copy_file group_vars/all/fleet_site.yml
copy_file group_vars/all/wg_operator_clients.yml
copy_file group_vars/all/fleet_edge_domains.yml
copy_file group_vars/hub/main.yml
copy_file group_vars/hub/fcj-workloads.yml
copy_file group_vars/fleet_edge.yml
copy_file group_vars/all/fleet_workloads.yml
mkdir -p "${DEST}/group_vars/all"
cp "${SRC}/fleet_sites.yml" "${DEST}/group_vars/all/fleet_sites.yml"

if grep -vE '^\s*#' "${DEST}/hosts.yml" | grep -q 'CHANGE_ME' && [[ "$FORCE" != true ]]; then
  echo "ERROR: inventory/hosts.yml still contains CHANGE_ME — edit ansible_host, or re-run with --force" >&2
  exit 1
fi

cat <<EOF
Copied hub-only inventory template to inventory/

Edit before install:
  inventory/hosts.yml                         — hub ansible_host (public IPv4)
  inventory/host_vars/umkim-hub.yml           — fleet_hub_public_ipv4 (same WAN IP; required for local Ansible)
  inventory/group_vars/all/site.yml         — fleet_edge_domain, fleet_dns, LDAP base DN
  inventory/group_vars/all/fleet_sites.yml  — management site (auto-copied)
  inventory/group_vars/hub/fcj-workloads.yml — copied by bootstrap; hostNetwork gateway for console FCJ clusters
  inventory/group_vars/all/fleet_workloads.yml — commented site/cluster template (install.md §5)

Then:
  export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
  export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
  ./scripts/install-hub.sh
EOF
