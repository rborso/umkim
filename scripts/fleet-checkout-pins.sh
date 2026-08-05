#!/usr/bin/env bash
# Checkout sibling repos to SHAs from fleet_git_pins.yml.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INV_ROOT="${ROOT}/inventory"
PINS_FILE="${PINS_FILE:-${INV_ROOT}/group_vars/all/fleet_git_pins.yml}"

usage() {
  echo "Usage: fleet-checkout-pins.sh [-i inventory-dir]"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -i) INV_ROOT="$2"; PINS_FILE="${INV_ROOT}/group_vars/all/fleet_git_pins.yml"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

read_pin() {
  local key="$1"
  grep -E "^${key}:" "$PINS_FILE" | head -1 | sed -E 's/^[^"]*"([^"]+)".*/\1/'
}

PARENT="$(dirname "$ROOT")"
declare -A REPOS=(
  [harbor_fleet_control_plane_git_ref]="control-plane"
  [harbor_fleet_k8s_sp_git_ref]="k8s-container-service-provider"
  [harbor_fleet_kubevirt_sp_git_ref]="kubevirt-service-provider"
  [harbor_fleet_olm_sp_git_ref]="fleet-olm-subscription-sp"
  [harbor_fleet_helm_sp_git_ref]="fleet-helm-release-sp"
)

echo "=== fleet-checkout-pins (${PINS_FILE}) ==="
for key in "${!REPOS[@]}"; do
  name="${REPOS[$key]}"
  ref="$(read_pin "$key")"
  path="${PARENT}/${name}"
  if [[ ! -d "${path}/.git" ]]; then
    echo "SKIP ${name}: not cloned at ${path}"
    continue
  fi
  echo "CHECKOUT ${name} -> ${ref:0:12}"
  git -C "$path" fetch --all --tags --quiet 2>/dev/null || true
  git -C "$path" checkout --quiet "$ref"
done
echo "Done."
