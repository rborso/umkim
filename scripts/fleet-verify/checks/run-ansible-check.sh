#!/usr/bin/env bash
# Run ansible-playbook for registry handlers (ansible:PLAYBOOK[:TAGS] or *.yml).
set -euo pipefail

id="${1:?check id}"
handler="${2:?handler}"

ROOT="${FV_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
INV="${FV_ANSIBLE_INVENTORY:-inventory}"

playbook=""
tags=""

if [[ "${handler}" == ansible:* ]]; then
  rest="${handler#ansible:}"
  if [[ "${rest}" == *:* ]]; then
    playbook="${rest%%:*}"
    tags="${rest#*:}"
  else
    playbook="${rest}"
  fi
elif [[ "${handler}" == *.yml ]] || [[ "${handler}" == *.yaml ]]; then
  playbook="${handler}"
else
  echo "FAIL run-ansible-check: unsupported handler ${handler} for ${id}" >&2
  exit 1
fi

[[ -n "${playbook}" ]] || { echo "FAIL run-ansible-check: empty playbook for ${id}" >&2; exit 1; }

export FV_ANSIBLE_PLAYBOOK="${playbook}"
export FV_ANSIBLE_TAGS="${tags}"
export FV_ROOT="${ROOT}"
export FV_ANSIBLE_INVENTORY="${INV}"

exec "$(dirname "$0")/ansible-verify.sh"
