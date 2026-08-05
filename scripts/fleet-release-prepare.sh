#!/usr/bin/env bash
# Bump umkim_version and record sibling-repo SHAs for a release.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PARENT="$(dirname "$ROOT")"
VERSION=""
CHECKOUT=false
EXAMPLES_ONLY=false

usage() {
  cat <<'EOF'
Usage: fleet-release-prepare.sh <version> [--checkout-siblings] [--examples-only]

  <version>              e.g. 0.2.0 (without umkim- prefix)
  --checkout-siblings    git checkout siblings to pins before recording new HEADs
  --examples-only        Update examples/inventory/hub-only only (not inventory/)

Then: git tag, stage-embed-assets.sh, harbor_push_fleet
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --checkout-siblings) CHECKOUT=true; shift ;;
    --examples-only) EXAMPLES_ONLY=true; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "Unknown option: $1" >&2; exit 1 ;;
    *)
      if [[ -z "$VERSION" ]]; then VERSION="$1"; shift; else echo "Unexpected arg: $1" >&2; exit 1; fi
      ;;
  esac
done

[[ -n "$VERSION" ]] || { usage; exit 1; }

if $CHECKOUT; then
  "${ROOT}/scripts/fleet-checkout-pins.sh"
fi

read_head() {
  local path="${PARENT}/$1"
  if [[ -d "${path}/.git" ]]; then
    git -C "$path" rev-parse HEAD
  fi
}

read_existing_pin() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 0
  grep -E "^${key}:" "$file" | head -1 | sed -E 's/^[^"]*"([^"]+)".*/\1/'
}

write_pins() {
  local pins="$1"
  local cp k8s kv olm helm
  cp="$(read_head control-plane)"
  k8s="$(read_head k8s-container-service-provider)"
  kv="$(read_head kubevirt-service-provider)"
  olm="$(read_head fleet-olm-subscription-sp)"
  helm="$(read_head fleet-helm-release-sp)"
  cp="${cp:-$(read_existing_pin "$pins" harbor_fleet_control_plane_git_ref)}"
  k8s="${k8s:-$(read_existing_pin "$pins" harbor_fleet_k8s_sp_git_ref)}"
  kv="${kv:-$(read_existing_pin "$pins" harbor_fleet_kubevirt_sp_git_ref)}"
  olm="${olm:-$(read_existing_pin "$pins" harbor_fleet_olm_sp_git_ref)}"
  helm="${helm:-$(read_existing_pin "$pins" harbor_fleet_helm_sp_git_ref)}"
  cat >"$pins" <<EOF
---
# Sibling-repo commits for harbor_push_fleet (local-src builds on the Ansible controller).
# Updated by scripts/fleet-release-prepare.sh at each release.
harbor_fleet_control_plane_git_ref: "${cp}"
harbor_fleet_k8s_sp_git_ref: "${k8s}"
harbor_fleet_kubevirt_sp_git_ref: "${kv}"
harbor_fleet_olm_sp_git_ref: "${olm}"
harbor_fleet_helm_sp_git_ref: "${helm}"

fleet_enforce_git_pins: false
EOF
}

update_common() {
  local common="$1"
  sed -i "s/^umkim_version:.*/umkim_version: \"${VERSION}\"/" "$common"
}

update_versions_md() {
  local vf="${ROOT}/VERSIONS.md"
  [[ -f "$vf" ]] || return 0
  sed -i "s/^\*\*Release:\*\*.*/**Release:** \`${VERSION}\` (\`umkim_version\` in [inventory\/group_vars\/all\/common.yml](inventory\/group_vars\/all\/common.yml))/" "$vf"
  sed -i "s/^| \*\*UMKIM\*\* | [^|]* |/| **UMKIM** | ${VERSION} |/" "$vf"
  sed -i "s/^| \*\*Fleet images\*\* (\`fleet\/\*\`) | [^|]* |/| **Fleet images** (\`fleet\/\*\`) | ${VERSION} |/" "$vf"
}

TARGETS=("${ROOT}/examples/inventory/hub-only")
if ! $EXAMPLES_ONLY && [[ -d "${ROOT}/inventory/group_vars" ]]; then
  TARGETS+=("${ROOT}/inventory")
fi

for base in "${TARGETS[@]}"; do
  update_common "${base}/group_vars/all/common.yml"
  write_pins "${base}/group_vars/all/fleet_git_pins.yml"
  echo "Updated ${base}/group_vars/all/{common,fleet_git_pins}.yml -> umkim_version=${VERSION}"
done

update_versions_md

echo ""
echo "Next:"
echo "  ./scripts/fleet-verify-pins.sh"
echo "  git tag -a umkim-${VERSION} -m \"UMKIM ${VERSION}\""
echo "  ./scripts/stage-embed-assets.sh"
echo "  ansible-playbook harbor.yml -l hub -t harbor_push_fleet --vault-password-file ~/.umkim-vault-pass"
