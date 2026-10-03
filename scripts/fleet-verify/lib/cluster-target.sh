#!/usr/bin/env bash
# Export cluster/site env for L2 checks (FLEET_VERIFY_CLUSTER or --cluster-preset).
set -euo pipefail

fv_cluster_target_export() {
  local cluster preset flow
  cluster="${FLEET_VERIFY_CLUSTER:-}"
  preset="${FV_CLUSTER_PRESET:-${FLEET_VERIFY_CLUSTER_PRESET:-}}"

  if [[ -z "${cluster}" && -n "${preset}" ]]; then
    cluster="${preset}"
  fi
  cluster="${cluster:-lab-sn01}"
  export FLEET_VERIFY_CLUSTER="${cluster}"
  export VERIFY_CLUSTER="${cluster}"
  export FLEET_HELM_CLUSTER="${cluster}"
  export CLUSTER="${cluster}"
  export OCM_CLUSTER="${cluster}"
  export SP_PROVIDER="olm-${cluster}"

  flow=""
  if [[ -n "${preset}" && -f "${FV_FRAMEWORK}/presets/clusters.yaml" ]]; then
    flow="$(FV_CLUSTER_PRESET="${preset}" FV_PRESETS="${FV_FRAMEWORK}/presets/clusters.yaml" python3 - <<'PY'
import yaml, os
preset = os.environ.get("FV_CLUSTER_PRESET", "")
with open(os.environ["FV_PRESETS"]) as f:
    data = yaml.safe_load(f) or {}
print((data.get(preset) or {}).get("flow", ""))
PY
)"
  fi
  if [[ -n "${flow}" && "${flow}" != /* ]]; then
    flow="${FV_ROOT}/${flow}"
  fi

  if [[ -f "${flow}" ]]; then
    eval "$(python3 - <<PY
import re
flow = open("${flow}").read()
site = ""
for line in flow.splitlines():
    m = re.match(r"^fleet_flow_site_id:\\s*(\\S+)", line)
    if m:
        site = m.group(1).strip('"')
        break
if site:
    print(f'export FLEET_SITE="{site}"')
    print(f'export FLEET_TEST_SITE="{site}"')
PY
)"
  fi

  export FLEET_SITE="${FLEET_SITE:-homelab}"
  export FLEET_TEST_SITE="${FLEET_TEST_SITE:-${FLEET_SITE}}"
  export FLEET_CLUSTER="${FLEET_CLUSTER:-${FLEET_VERIFY_CLUSTER}}"
  export FLEET_HELM_CLUSTER="${FLEET_HELM_CLUSTER:-${FLEET_VERIFY_CLUSTER}}"
  export FLEET_API="${FLEET_API:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}"
  export FLEET_SITE_API="${FLEET_SITE_API:-${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}/fleet/api/v1alpha1}"
  export FLEET_SITE_API_DIRECT="${FLEET_SITE_API_DIRECT:-http://${FLEET_HUB_IP:-127.0.0.1}:18080/api/v1alpha1}"
  export FLEET_SITE_COOKIE_JAR="${FLEET_SITE_COOKIE_JAR:-${FLEET_AUTH_COOKIE_JAR:-${FV_AUTH_ADMIN_JAR:-}}}"
  export FLEET_NFS_SERVER="${FLEET_NFS_SERVER:-203.0.113.40}"
  export NFS_SERVER="${NFS_SERVER:-${FLEET_NFS_SERVER}}"

  local vault="${FV_ROOT}/inventory/group_vars/all/vault.yml"
  local pass_file="${ANSIBLE_VAULT_PASSWORD_FILE:-${HOME}/.umkim-vault-pass}"
  if [[ -z "${LLDAP_PASSWORD:-}" && -f "${pass_file}" && -f "${vault}" ]]; then
    local vault_key
    vault_key="$(echo "${cluster}" | tr '-' '_')_lldap_password_vault"
    LLDAP_PASSWORD="$(ansible-vault view "${vault}" --vault-password-file "${pass_file}" 2>/dev/null \
      | awk -v k="${vault_key}" '$1 == k":" { sub(/^[^:]*:[[:space:]]*/, ""); gsub(/^"|"$/, ""); print; exit }' || true)"
    export LLDAP_PASSWORD
  fi
  export FLEET_API_BASE="${FLEET_API_BASE:-${FLEET_CONSOLE_URL}/fleet/api/v1alpha1}"
  export FLEET_COOKIE_JAR="${FLEET_COOKIE_JAR:-${FLEET_AUTH_COOKIE_JAR:-${FV_AUTH_ADMIN_JAR:-}}}"
}
