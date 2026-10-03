#!/usr/bin/env bash
# Execute a single registered check with timeout and logging.
set -euo pipefail

FV_RUN_ID="${FV_RUN_ID:-$(date +%Y%m%d%H%M%S)}"
export FV_RUN_ID

fv_run_check() {
  local id="$1" layer="$2" suite="$3" module="$4"
  local handler="$5" timeout_s="${6:-300}" blocking="${7:-true}"
  local mode="${8:-readonly}" parity="${9:-false}"

  local log_file="${FV_LOG_DIR}/$(echo "$id" | tr '/.' '__').log"
  local start_ms end_ms duration_ms status message rc=0

  start_ms=$(python3 -c "import time; print(int(time.time()*1000))")

  if [[ "${FV_DRY_RUN:-0}" == "1" ]]; then
    echo "DRY-RUN  ${id}  ${handler}"
    fv_report_check "$id" "$layer" "skip" "dry-run" 0 "$suite" "$module"
    return 0
  fi

  echo "== CHECK ${id} ==" | tee "${log_file}"
  set +e
  if [[ "${parity}" == "true" ]]; then
    "${FV_FRAMEWORK}/parity/run-parity.sh" "$id" >> "${log_file}" 2>&1 </dev/null
    rc=$?
  elif [[ "${handler}" == *.yml ]] || [[ "${handler}" == ansible:* ]]; then
    bash "${FV_FRAMEWORK}/checks/run-ansible-check.sh" "$id" "$handler" >> "${log_file}" 2>&1 </dev/null
    rc=$?
  elif [[ "${handler}" == scripts/* ]] && [[ -f "${FV_ROOT}/${handler}" ]]; then
    timeout "${timeout_s}" bash "${FV_ROOT}/${handler}" >> "${log_file}" 2>&1 </dev/null
    rc=$?
  elif [[ -x "${FV_ROOT}/${handler}" ]] || [[ -f "${FV_ROOT}/${handler}" ]]; then
    timeout "${timeout_s}" bash "${FV_ROOT}/${handler}" >> "${log_file}" 2>&1 </dev/null
    rc=$?
  elif [[ -x "${FV_FRAMEWORK}/${handler}" ]] || [[ -f "${FV_FRAMEWORK}/${handler}" ]]; then
    timeout "${timeout_s}" bash "${FV_FRAMEWORK}/${handler}" >> "${log_file}" 2>&1 </dev/null
    rc=$?
  else
    echo "handler not found: ${handler}" >> "${log_file}"
    rc=127
  fi
  set -e

  end_ms=$(python3 -c "import time; print(int(time.time()*1000))")
  duration_ms=$((end_ms - start_ms))

  if [[ "$rc" -eq 0 ]]; then
    status="pass"
    message="ok"
  elif [[ "$rc" -eq 124 ]]; then
    status="fail"
    message="timeout after ${timeout_s}s"
  else
    status="fail"
    message="exit ${rc}"
  fi

  fv_report_check "$id" "$layer" "$status" "$message" "$duration_ms" "$suite" "$module"
  if [[ "$status" == "fail" && "$blocking" == "true" ]]; then
    fv_report_blocking_fail "${id}: ${message}"
    if [[ "${FV_CONTINUE_ON_FAIL:-1}" != "1" ]]; then
      return "$rc"
    fi
  fi
  return 0
}
