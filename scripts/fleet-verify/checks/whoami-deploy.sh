#!/usr/bin/env bash
set -euo pipefail
ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"
# shellcheck source=scripts/fleet-verify/lib/ensure-providers.sh
source "${FRAMEWORK}/lib/ensure-providers.sh"
fv_ensure_cluster_providers
INV="-i ${ROOT}/inventory/hosts.yml"
[[ -f "${ROOT}/inventory/hosts-from-hub.yml" ]] && INV="-i ${ROOT}/inventory/hosts.yml -i ${ROOT}/inventory/hosts-from-hub.yml"
EXTRA=()
[[ -n "${FLEET_HUB_IP:-}" ]] && EXTRA=(-e "ansible_host=${FLEET_HUB_IP}")
VAULT_ARGS=()
[[ -f "${HOME}/.umkim-vault-pass" ]] && VAULT_ARGS=(--vault-password-file "${HOME}/.umkim-vault-pass")

RELEASE="verify-synth-whoami-${FV_RUN_ID:-test}"
export WHOAMI_RELEASE="${RELEASE}"

ansible-playbook ${INV} "${ROOT}/playbooks/flows/whoami-helm-deploy.yml" \
  -l hub \
  "${EXTRA[@]}" \
  -e fleet_flow_cluster="${FLEET_VERIFY_CLUSTER:-lab-sn01}" \
  -e fleet_flow_cluster_name="${FLEET_VERIFY_CLUSTER:-lab-sn01}" \
  -e "fleet_flow_release_name=${RELEASE}" \
  -e fleet_flow_namespace=default \
  "${VAULT_ARGS[@]}"

DCM="${DCM_API:-${DCM_API_URL:-http://127.0.0.1:30908/api/v1alpha1}}"
id=$(curl -sf "${DCM}/catalog-item-instances" | python3 -c "
import json,sys,os
n=os.environ.get('RELEASE','')
for r in json.load(sys.stdin).get('results',[]):
  if r.get('display_name')==n:
    print(r.get('id') or r.get('uid') or ''); break
" RELEASE="${RELEASE}" 2>/dev/null || true)
if [[ -n "${id}" ]]; then
  curl -sf -X DELETE "${DCM}/catalog-item-instances/${id}" >/dev/null || true
fi
echo "OK whoami-deploy ${RELEASE}"
