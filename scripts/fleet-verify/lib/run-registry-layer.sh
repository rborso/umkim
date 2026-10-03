#!/usr/bin/env bash
# Iterate registry checks and run via fv_run_check.
set -euo pipefail

_fv_run_registry_layer() {
  local layer_num="$1"

  checks_json="$(python3 "${FV_FRAMEWORK}/lib/registry.py" \
    --registry "${FV_REGISTRY}" \
    --layer "${layer_num}" \
    --suite "${FV_SUITE}" \
    $([[ "${FV_WITH_ANSIBLE:-0}" == "1" ]] && echo --with-ansible))"

  count="$(echo "${checks_json}" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")"
  echo "L${layer_num}: ${count} checks (profile=${FV_ACTIVE_PROFILE}, suite=${FV_SUITE})"

  while IFS=$'\t' read -r id module profiles path timeout_s blocking mode parity requires env_s; do
    [[ -n "${id}" ]] || continue
    if ! fv_profile_allows_check "${profiles}" "${FV_ACTIVE_PROFILE}"; then
      fv_report_check "${id}" "${layer_num}" "skip" "profile ${FV_ACTIVE_PROFILE} not in [${profiles}]" 0 "${FV_SUITE}" "${module}"
      continue
    fi
    if ! fv_profile_check_requires "${requires}"; then
      fv_report_check "${id}" "${layer_num}" "skip" "requires [${requires}] not met" 0 "${FV_SUITE}" "${module}"
      continue
    fi
    if [[ -n "${env_s}" ]]; then
      IFS='|' read -ra env_pairs <<< "${env_s}"
      for pair in "${env_pairs[@]}"; do
        [[ -n "${pair}" ]] && export "${pair?}"
      done
    fi
    fv_run_check "${id}" "${layer_num}" "${FV_SUITE}" "${module}" "${path}" "${timeout_s}" "${blocking}" "${mode}" "${parity}" || true
  done < <(echo "${checks_json}" | python3 -c "
import json, os, sys
skip = os.environ.get('FV_SKIP_CHECK_ID', '')
for c in json.load(sys.stdin):
    cid = c.get('id', '')
    if skip and cid == skip:
        continue
    if cid == 'cluster.reinstall.preset':
        continue
    path = c.get('handler') or c.get('script') or ''
    env = c.get('env') or {}
    env_s = '|'.join(f'{k}={v}' for k, v in env.items())
    print('\t'.join([
        cid,
        c.get('module', ''),
        ','.join(c.get('profiles') or []),
        path,
        str(c.get('timeout_s', 300)),
        str(c.get('blocking', True)).lower(),
        c.get('mode', 'readonly'),
        str(c.get('parity', False)).lower(),
        ','.join(c.get('requires') or []),
        env_s,
    ]))
") || true
}
