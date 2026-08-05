#!/usr/bin/env bash
set -euo pipefail
FV_LAYER=1
source "${FV_FRAMEWORK}/lib/report.sh"
source "${FV_FRAMEWORK}/lib/profile.sh"
source "${FV_FRAMEWORK}/lib/run-check.sh"
source "${FV_FRAMEWORK}/lib/run-registry-layer.sh"
export FV_ACTIVE_PROFILE="$(fv_profile_detect)"
_fv_run_registry_layer 1
