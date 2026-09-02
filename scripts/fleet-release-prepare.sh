#!/usr/bin/env bash
# Bump umkim_version for a release.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION=""
EXAMPLES_ONLY=false

usage() {
  cat <<'EOF'
Usage: fleet-release-prepare.sh <version> [--examples-only]

  <version>         e.g. 0.2.0 (without umkim- prefix)
  --examples-only   Update examples/inventory/hub-only only (not inventory/)

Then: git tag, stage-embed-assets.sh, harbor_push_fleet
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --checkout-siblings)
      echo "WARN: --checkout-siblings is ignored; v0.2 has no sibling image pins" >&2
      shift
      ;;
    --examples-only) EXAMPLES_ONLY=true; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "Unknown option: $1" >&2; exit 1 ;;
    *)
      if [[ -z "$VERSION" ]]; then VERSION="$1"; shift; else echo "Unexpected arg: $1" >&2; exit 1; fi
      ;;
  esac
done

[[ -n "$VERSION" ]] || { usage; exit 1; }

update_common() {
  local common="$1"
  [[ -f "$common" ]] || return 0
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
  echo "Updated ${base}/group_vars/all/common.yml -> umkim_version=${VERSION}"
done

update_versions_md

echo ""
echo "Next:"
echo "  git tag -a umkim-${VERSION} -m \"UMKIM ${VERSION}\""
echo "  ./scripts/stage-embed-assets.sh"
echo "  ansible-playbook harbor.yml -l hub -t harbor_push_fleet --vault-password-file ~/.umkim-vault-pass"
