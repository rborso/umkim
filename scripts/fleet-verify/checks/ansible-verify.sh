#!/usr/bin/env bash
set -euo pipefail

ROOT="${FV_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
PLAYBOOK="${FV_ANSIBLE_PLAYBOOK:?FV_ANSIBLE_PLAYBOOK required}"
TAGS="${FV_ANSIBLE_TAGS:-}"
INV_ARGS=()
if [[ -f "${ROOT}/inventory/hosts-from-hub.yml" ]]; then
  INV_ARGS=(-i "${ROOT}/inventory/hosts.yml" -i "${ROOT}/inventory/hosts-from-hub.yml")
else
  INV_ARGS=(-i "${ROOT}/inventory/hosts.yml")
fi

cd "${ROOT}"

args=("${INV_ARGS[@]}" "${PLAYBOOK}" -l hub)
if [[ -n "${TAGS}" ]]; then
  args+=(--tags "${TAGS}")
fi

if ansible-playbook "${args[@]}"; then
  echo "OK ansible-playbook ${PLAYBOOK}${TAGS:+ (tags=${TAGS})}"
  exit 0
fi

echo "FAIL ansible-playbook ${PLAYBOOK}${TAGS:+ (tags=${TAGS})}" >&2
exit 1
