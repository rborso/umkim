#!/usr/bin/env bash
# Run API + console parity for a registered check id.
set -euo pipefail

CHECK_ID="${1:?check id}"
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"

source "${FRAMEWORK}/lib/report.sh" 2>/dev/null || true

OP="${CHECK_ID#parity.}"
API_RC=0
CON_RC=0

set +e
bash "${FRAMEWORK}/parity/api-driver.sh" "${CHECK_ID}" >> /dev/stderr 2>&1
API_RC=$?
bash "${FRAMEWORK}/parity/console-driver.sh" "${CHECK_ID}" >> /dev/stderr 2>&1
CON_RC=$?
set -e

parity="pass"
if [[ "${API_RC}" -ne 0 || "${CON_RC}" -ne 0 ]]; then
  parity="fail"
fi

if declare -F fv_report_parity >/dev/null 2>&1; then
  fv_report_parity "${OP}" "$([[ ${API_RC} -eq 0 ]] && echo pass || echo fail)" \
    "$([[ ${CON_RC} -eq 0 ]] && echo pass || echo fail)" "${parity}" "api_rc=${API_RC} console_rc=${CON_RC}"
fi

echo "parity ${CHECK_ID}: api=${API_RC} console=${CON_RC} => ${parity}"
[[ "${parity}" == "pass" ]]
