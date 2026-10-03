#!/usr/bin/env bash
# Layer 3: greenfield rebuild then L1/L2 subsets.
set -euo pipefail

FV_LAYER=3
source "${FV_FRAMEWORK}/lib/report.sh"

L3_SUITE="${FV_SUITE:-hub}"
L3_ONLY="${FV_L3_ONLY:-0}"

echo "=== L3 rebuild suite=${L3_SUITE} ==="

if [[ "${FV_DRY_RUN:-0}" == "1" ]]; then
  echo "DRY-RUN: would run teardown-hub.sh --confirm && install-hub.sh && configure-fleet-llm.sh"
  fv_report_check "l3.teardown" 3 "skip" "dry-run" 0 "${L3_SUITE}" "S8"
  fv_report_check "l3.install" 3 "skip" "dry-run" 0 "${L3_SUITE}" "S8"
  fv_report_check "l3.configure-llm" 3 "skip" "dry-run" 0 "${L3_SUITE}" "S8"
else
  if [[ "${FV_SKIP_TEARDOWN:-0}" != "1" ]]; then
    echo "== teardown-hub =="
    start_ms=$(python3 -c "import time; print(int(time.time()*1000))")
    if bash "${FV_ROOT}/scripts/teardown-hub.sh" --confirm; then
      end_ms=$(python3 -c "import time; print(int(time.time()*1000))")
      fv_report_check "l3.teardown" 3 "pass" "ok" "$((end_ms - start_ms))" "${L3_SUITE}" "S8"
    else
      fv_report_check "l3.teardown" 3 "fail" "teardown failed" 0 "${L3_SUITE}" "S8"
      fv_report_blocking_fail "l3.teardown"
      return 1
    fi
  fi

  echo "== install-hub =="
  start_ms=$(python3 -c "import time; print(int(time.time()*1000))")
  if bash "${FV_ROOT}/scripts/install-hub.sh"; then
    end_ms=$(python3 -c "import time; print(int(time.time()*1000))")
    fv_report_check "l3.install" 3 "pass" "ok" "$((end_ms - start_ms))" "${L3_SUITE}" "S8"
  else
    fv_report_check "l3.install" 3 "fail" "install failed" 0 "${L3_SUITE}" "S8"
    fv_report_blocking_fail "l3.install"
    return 1
  fi

  echo "== configure-fleet-llm =="
  start_ms=$(python3 -c "import time; print(int(time.time()*1000))")
  if bash "${FV_FRAMEWORK}/checks/configure-fleet-llm.sh"; then
    end_ms=$(python3 -c "import time; print(int(time.time()*1000))")
    fv_report_check "l3.configure-llm" 3 "pass" "ok" "$((end_ms - start_ms))" "${L3_SUITE}" "S8"
  else
    end_ms=$(python3 -c "import time; print(int(time.time()*1000))")
    fv_report_check "l3.configure-llm" 3 "fail" "fleet-llm configure failed" "$((end_ms - start_ms))" "${L3_SUITE}" "S8"
    fv_report_blocking_fail "l3.configure-llm"
    if [[ "${FV_CONTINUE_ON_FAIL:-1}" != "1" ]]; then
      return 1
    fi
  fi
fi

if [[ "${L3_ONLY}" == "1" ]]; then
  return 0
fi

# Map L3 suite to L1/L2 suite subsets
case "${L3_SUITE}" in
  hub)
    L1_SUITE="hub"
    L2_SUITE="llm"
    ;;
  workload)
    L1_SUITE="hub"
    L2_SUITE="core"
    ;;
  homelab)
    L1_SUITE="all"
    L2_SUITE="core"
    export FV_CLUSTER_PRESET_ONLY=0
    if [[ -n "${FV_CLUSTER_SPEC:-}" ]]; then
      for preset in lab-sn01 lab-3n01 lab-ha01; do
        export FV_CLUSTER_PRESET="${preset}"
        FV_SUITE="core" source "${FV_FRAMEWORK}/layers/l2-synthetic.sh" || true
      done
    else
      echo "NOTE: homelab flow fixtures are not in this public tree. Set FLEET_VERIFY_CLUSTER_SPEC to a local extra-vars file to reinstall. Hub inventory starts at examples/inventory/hub-only (replace CHANGE_ME)."
      export FV_CLUSTER_PRESET=""
    fi
    # Hub LLM inference after clusters (LiteLLM + console /llm/)
    export FV_SUITE="llm"
    source "${FV_FRAMEWORK}/layers/l2-synthetic.sh" || true
    ;;
  regression)
    L1_SUITE="all"
    L2_SUITE="regression"
    ;;
  *)
    L1_SUITE="hub"
    L2_SUITE="core"
    ;;
esac

export FV_SUITE="${L1_SUITE}"
source "${FV_FRAMEWORK}/layers/l1-audit.sh" || true

if [[ -n "${L2_SUITE}" ]]; then
  export FV_SUITE="${L2_SUITE}"
  source "${FV_FRAMEWORK}/layers/l2-synthetic.sh" || true
fi

if [[ "${L3_SUITE}" == "workload" ]]; then
  export FV_SUITE="llm"
  source "${FV_FRAMEWORK}/layers/l2-synthetic.sh" || true
fi

if [[ "${L3_SUITE}" == "regression" ]]; then
  export FV_SUITE="regression"
  source "${FV_FRAMEWORK}/layers/l2-synthetic.sh" || true
fi