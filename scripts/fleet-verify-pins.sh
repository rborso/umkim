#!/usr/bin/env bash
# Verify local sibling-repo HEADs match inventory/group_vars/all/fleet_git_pins.yml.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INV_ROOT="${ROOT}/inventory"
PINS_FILE="${PINS_FILE:-${INV_ROOT}/group_vars/all/fleet_git_pins.yml}"
STRICT=false
INVENTORY_DIR=""

usage() {
  cat <<'EOF'
Usage: fleet-verify-pins.sh [-i inventory-dir] [--strict]

Compares sibling-repo HEAD SHAs under the UMKIM parent directory to
harbor_fleet_*_git_ref values in fleet_git_pins.yml.

  -i DIR   Inventory root (default: repo inventory/)
  --strict Fail when a sibling repo directory is missing
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -i) INV_ROOT="$2"; PINS_FILE="${INV_ROOT}/group_vars/all/fleet_git_pins.yml"; shift 2 ;;
    --strict) STRICT=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ ! -f "$PINS_FILE" ]]; then
  echo "ERROR: pins file not found: $PINS_FILE" >&2
  exit 1
fi

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

FAIL=0
echo "=== fleet-verify-pins (${PINS_FILE}) ==="
for key in "${!REPOS[@]}"; do
  name="${REPOS[$key]}"
  expected="$(read_pin "$key")"
  path="${PARENT}/${name}"
  if [[ ! -d "${path}/.git" ]]; then
    if $STRICT; then
      echo "FAIL ${name}: missing ${path}"
      FAIL=1
    else
      echo "SKIP ${name}: not cloned at ${path}"
    fi
    continue
  fi
  actual="$(git -C "$path" rev-parse HEAD)"
  if [[ "$actual" == "$expected" ]]; then
    echo "OK   ${name}: ${actual:0:12}"
  else
    echo "FAIL ${name}: HEAD ${actual} != pin ${expected}"
    FAIL=1
  fi
done

if [[ $FAIL -ne 0 ]]; then
  echo "Run ./scripts/fleet-checkout-pins.sh to align siblings, or ./scripts/fleet-release-prepare.sh to refresh pins." >&2
  exit 1
fi
echo "All present sibling repos match pins."
