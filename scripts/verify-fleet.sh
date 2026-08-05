#!/usr/bin/env bash
# Fleet multi-layer verification orchestrator.
#
# Usage:
#   ./scripts/verify-fleet.sh --layer 1
#   ./scripts/verify-fleet.sh --layer 2 --suite core
#   ./scripts/verify-fleet.sh --layer 2 --cluster-preset lab-sn01
#   ./scripts/verify-fleet.sh --layer 3 --suite hub
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRAMEWORK="${ROOT}/scripts/fleet-verify"
REGISTRY="${FRAMEWORK}/check-registry.yaml"

FV_ROOT="${ROOT}"
FV_FRAMEWORK="${FRAMEWORK}"
FV_REGISTRY="${REGISTRY}"

LAYER=""
FV_SUITE="all"
FV_PROFILE="auto"
FV_REPORT_DIR=""
FV_JSON_ONLY=0
FV_DRY_RUN=0
FV_CONTINUE_ON_FAIL=1
FV_WITH_ANSIBLE=0
FV_CLUSTER_PRESET=""
FV_CLUSTER_SPEC=""
FV_CLUSTER_PRESET_ONLY=0
FV_L3_ONLY=0
FV_SKIP_TEARDOWN=0

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  echo ""
  echo "Options:"
  echo "  --layer 1|2|3          Verification layer (required)"
  echo "  --suite NAME           hub|core|edge|backup|tenancy|llm|nested|catalog|regression|all"
  echo "  --profile auto|hub-only|homelab"
  echo "  --report-dir DIR       Output directory (default reports/verify-<ts>)"
  echo "  --cluster-preset NAME  L2: reinstall cluster (lab-sn01, lab-3n01, lab-ha01)"
  echo "  --cluster-spec PATH    Override flow YAML for cluster reinstall"
  echo "  --cluster-preset-only  Run only cluster reinstall (L2)"
  echo "  --with-ansible         Include ansible tag verify checks (L1)"
  echo "  --continue-on-fail     Do not stop on first blocking failure (default L1/L2)"
  echo "  --no-continue-on-fail  Stop on first blocking failure"
  echo "  --layer-3-only         L3: teardown+install only, skip L1/L2"
  echo "  --skip-teardown        L3: skip teardown-hub (reinstall on current hub)"
  echo "  --dry-run              List checks without executing"
  echo "  --json                 Print summary.json only at end"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --layer) LAYER="$2"; shift 2 ;;
    --suite) FV_SUITE="$2"; shift 2 ;;
    --profile) FV_PROFILE="$2"; shift 2 ;;
    --report-dir) FV_REPORT_DIR="$2"; shift 2 ;;
    --cluster-preset) FV_CLUSTER_PRESET="$2"; shift 2 ;;
    --cluster-spec) FV_CLUSTER_SPEC="$2"; shift 2 ;;
    --cluster-preset-only) FV_CLUSTER_PRESET_ONLY=1; shift ;;
    --with-ansible) FV_WITH_ANSIBLE=1; shift ;;
    --continue-on-fail) FV_CONTINUE_ON_FAIL=1; shift ;;
    --no-continue-on-fail) FV_CONTINUE_ON_FAIL=0; shift ;;
    --layer-3-only) FV_L3_ONLY=1; shift ;;
    --skip-teardown) FV_SKIP_TEARDOWN=1; shift ;;
    --dry-run) FV_DRY_RUN=1; shift ;;
    --json) FV_JSON_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

[[ -n "${LAYER}" ]] || { echo "ERROR: --layer required" >&2; usage; exit 1; }
[[ -f "${REGISTRY}" ]] || { echo "ERROR: registry not found: ${REGISTRY}" >&2; exit 1; }

TS="$(date +%Y%m%dT%H%M%S)"
FV_REPORT_DIR="${FV_REPORT_DIR:-${ROOT}/reports/verify-${LAYER}-${TS}}"
export FV_ROOT FV_FRAMEWORK FV_REGISTRY FV_SUITE FV_PROFILE FV_REPORT_DIR
export FV_JSON_ONLY FV_DRY_RUN FV_CONTINUE_ON_FAIL FV_WITH_ANSIBLE
export FV_CLUSTER_PRESET FV_CLUSTER_SPEC FV_CLUSTER_PRESET_ONLY FV_L3_ONLY FV_SKIP_TEARDOWN
export FV_START_EPOCH="$(date +%s)"

if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi

source "${FRAMEWORK}/lib/hub-endpoint.sh"
fv_hub_endpoint_export_urls
export FLEET_HUB_IP HUB_HOST HUB_SSH FLEET_CONSOLE_URL FLEET_API DCM_API

FV_KUBECTL_BIN="${TMPDIR:-/tmp}/fleet-verify-kubectl-bin-${$}"
mkdir -p "${FV_KUBECTL_BIN}"
ln -sf "${FRAMEWORK}/lib/kubectl-hub" "${FV_KUBECTL_BIN}/kubectl"
export PATH="${FV_KUBECTL_BIN}:${PATH}"

source "${FRAMEWORK}/lib/cluster-target.sh"
fv_cluster_target_export

source "${FRAMEWORK}/lib/auth.sh"
fv_auth_load_password
export FLEET_AUTH_USER="${FLEET_AUTH_USER:-localadmin}"
export FLEET_AUTH_PASSWORD
export DCM_API_URL="${DCM_API_URL:-${FLEET_CONSOLE_URL}/api/v1alpha1}"
export FLEET_API_URL="${FLEET_API_URL:-${FLEET_CONSOLE_URL}/fleet/api/v1alpha1}"

source "${FRAMEWORK}/lib/report.sh"
fv_report_init

ACTIVE_PROFILE="$(bash -c "source ${FRAMEWORK}/lib/profile.sh; FV_PROFILE=${FV_PROFILE}; fv_profile_detect")"

case "${LAYER}" in
  1)
    # shellcheck source=scripts/fleet-verify/layers/l1-audit.sh
    source "${FRAMEWORK}/layers/l1-audit.sh"
    ;;
  2)
    # shellcheck source=scripts/fleet-verify/layers/l2-synthetic.sh
    source "${FRAMEWORK}/layers/l2-synthetic.sh"
    ;;
  3)
    # shellcheck source=scripts/fleet-verify/layers/l3-rebuild.sh
    source "${FRAMEWORK}/layers/l3-rebuild.sh"
    ;;
  *)
    echo "ERROR: invalid layer ${LAYER}" >&2
    exit 1
    ;;
esac

fv_report_finalize "${LAYER}" "${ACTIVE_PROFILE}" "${FV_SUITE}" || exit 1
