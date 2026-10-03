#!/usr/bin/env bash
# Layer 2: synthetic transactions + parity from registry.
set -euo pipefail

FV_LAYER=2
source "${FV_FRAMEWORK}/lib/report.sh"
source "${FV_FRAMEWORK}/lib/profile.sh"
source "${FV_FRAMEWORK}/lib/auth.sh"
source "${FV_FRAMEWORK}/lib/run-check.sh"
source "${FV_FRAMEWORK}/lib/run-registry-layer.sh"

export FV_ACTIVE_PROFILE="$(fv_profile_detect)"
export FV_RUN_ID="verify-$(date +%Y%m%d%H%M%S)-$$"

if [[ -n "${FV_CLUSTER_PRESET:-}" ]]; then
  export FLEET_VERIFY_CLUSTER_PRESET="${FV_CLUSTER_PRESET}"
  export FLEET_VERIFY_CLUSTER_SPEC="${FV_CLUSTER_SPEC:-}"
  fv_auth_admin_login 2>/dev/null || true
  fv_run_check "cluster.reinstall.preset" 2 "${FV_SUITE}" "S6" \
    "checks/cluster-reinstall.sh" 7200 true mutating false || true
  if [[ "${FV_CLUSTER_PRESET_ONLY:-0}" == "1" ]]; then
    bash "${FV_FRAMEWORK}/lib/residual-scan.sh" || true
    return 0
  fi
fi

fv_auth_admin_login 2>/dev/null || true
export FLEET_SITE_COOKIE_JAR="${FLEET_AUTH_COOKIE_JAR:-${FV_AUTH_ADMIN_JAR:-}}"
export FV_SKIP_CHECK_ID="cluster.reinstall.preset"
_fv_run_registry_layer 2
bash "${FV_FRAMEWORK}/lib/residual-scan.sh" || true
