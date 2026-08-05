#!/usr/bin/env bash
# DCM policy engine deployment name (control-plane monolith).
set -euo pipefail

if [[ -n "${DCM_POLICY_DEPLOY:-}" ]]; then
  echo "$DCM_POLICY_DEPLOY"
  exit 0
fi
echo control-plane
