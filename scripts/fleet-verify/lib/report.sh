#!/usr/bin/env bash
# Structured reporting for fleet-verify (JSONL + summary.json).
set -euo pipefail

FV_REPORT_DIR="${FV_REPORT_DIR:?set FV_REPORT_DIR before sourcing report.sh}"
FV_CHECKS_JSONL="${FV_REPORT_DIR}/checks.jsonl"
FV_PARITY_JSONL="${FV_REPORT_DIR}/parity.jsonl"
FV_SUMMARY_JSON="${FV_REPORT_DIR}/summary.json"
FV_LOG_DIR="${FV_REPORT_DIR}/logs"

FV_PASSED="${FV_PASSED:-0}"
FV_FAILED="${FV_FAILED:-0}"
FV_SKIPPED="${FV_SKIPPED:-0}"
FV_WARN="${FV_WARN:-0}"
FV_START_EPOCH="${FV_START_EPOCH:-$(date +%s)}"

if [[ "${FV_REPORT_INIT_DONE:-0}" != "1" ]]; then
  FV_BLOCKING_FAILURES=()
  fv_report_init() {
    mkdir -p "${FV_LOG_DIR}"
    : > "${FV_CHECKS_JSONL}"
    : > "${FV_PARITY_JSONL}"
  }
  FV_REPORT_INIT_DONE=1
  fv_report_init
fi

fv_report_check() {
  local id="$1" layer="$2" status="$3" message="${4:-}" duration_ms="${5:-0}" suite="${6:-}" module="${7:-}"
  local log_path="logs/$(echo "$id" | tr '/.' '__').log"
  case "${status}" in
    pass) FV_PASSED=$((FV_PASSED + 1)) ;;
    fail) FV_FAILED=$((FV_FAILED + 1)) ;;
    skip) FV_SKIPPED=$((FV_SKIPPED + 1)) ;;
    warn) FV_WARN=$((FV_WARN + 1)) ;;
  esac
  python3 -c "
import json, sys
print(json.dumps({
  'id': sys.argv[1], 'layer': int(sys.argv[2]), 'status': sys.argv[3],
  'message': sys.argv[4], 'duration_ms': int(sys.argv[5]),
  'log_path': sys.argv[6], 'suite': sys.argv[7], 'module': sys.argv[8],
}))" "$id" "$layer" "$status" "$message" "$duration_ms" "$log_path" "$suite" "$module" >> "${FV_CHECKS_JSONL}"
}

fv_report_blocking_fail() {
  FV_BLOCKING_FAILURES+=("$1")
}

fv_report_parity() {
  local operation="$1" api_status="$2" console_status="$3" parity="$4" diff="${5:-}"
  python3 -c "
import json, sys
print(json.dumps({
  'operation': sys.argv[1], 'api': sys.argv[2], 'console': sys.argv[3],
  'parity': sys.argv[4], 'diff': sys.argv[5],
}))" "$operation" "$api_status" "$console_status" "$parity" "$diff" >> "${FV_PARITY_JSONL}"
}

fv_report_finalize() {
  local layer="${1:-1}" profile="${2:-auto}" suite="${3:-all}"
  local end_epoch duration_s git_sha umkim_version
  end_epoch=$(date +%s)
  duration_s=$((end_epoch - FV_START_EPOCH))
  git_sha="$(git -C "${FV_ROOT:-.}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  umkim_version="$(grep -E '^umkim_version:' "${FV_ROOT:-.}/inventory/group_vars/all/common.yml" 2>/dev/null \
    | head -1 | sed -E 's/^[^"]*"([^"]+)".*/\1/' || echo unknown)"

  python3 -c "
import json, sys
blocking = sys.argv[6].split('\t') if sys.argv[6] else []
blocking = [b for b in blocking if b]
print(json.dumps({
  'layer': sys.argv[1], 'profile': sys.argv[2], 'suite': sys.argv[3],
  'git_sha': sys.argv[4], 'umkim_version': sys.argv[5],
  'passed': int(sys.argv[7]), 'failed': int(sys.argv[8]),
  'skipped': int(sys.argv[9]), 'warn': int(sys.argv[10]),
  'duration_s': int(sys.argv[11]), 'blocking_failures': blocking,
  'report_dir': sys.argv[12],
}, indent=2))" \
    "$layer" "$profile" "$suite" "$git_sha" "$umkim_version" \
    "$(printf '%s\n' "${FV_BLOCKING_FAILURES[@]:-}" | paste -sd$'\t' -)" \
    "$FV_PASSED" "$FV_FAILED" "$FV_SKIPPED" "$FV_WARN" "$duration_s" "$FV_REPORT_DIR" \
    > "${FV_SUMMARY_JSON}"

  if [[ "${FV_JSON_ONLY:-0}" != "1" ]]; then
    echo ""
    echo "Fleet verify L${layer} [${profile}] suite=${suite} — ${FV_PASSED} passed, ${FV_FAILED} failed, ${FV_SKIPPED} skipped, ${FV_WARN} warn (${duration_s}s)"
    if [[ ${#FV_BLOCKING_FAILURES[@]} -gt 0 ]]; then
      for bf in "${FV_BLOCKING_FAILURES[@]}"; do
        echo "  FAIL  ${bf}"
      done
    fi
    echo "Report: ${FV_REPORT_DIR}/"
  else
    cat "${FV_SUMMARY_JSON}"
  fi

  [[ ${#FV_BLOCKING_FAILURES[@]} -eq 0 ]]
}
