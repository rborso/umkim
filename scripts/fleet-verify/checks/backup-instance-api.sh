#!/usr/bin/env bash
# Deploy whoami-helm and smoke-test instance backup API.
set -euo pipefail

ROOT="${FV_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
FRAMEWORK="${FV_FRAMEWORK:-${ROOT}/scripts/fleet-verify}"

bash "${FRAMEWORK}/checks/whoami-deploy.sh"

DCM="${DCM_API:-${DCM_API_URL:-http://127.0.0.1:30908/api/v1alpha1}}"
RELEASE="verify-synth-whoami-${FV_RUN_ID:-test}"
INSTANCE_ID=""
for _ in $(seq 1 30); do
  INSTANCE_ID="$(curl -sf "${DCM}/catalog-item-instances" | python3 -c "
import json,sys,os
n=os.environ.get('RELEASE','')
for r in json.load(sys.stdin).get('results',[]):
  if r.get('display_name')==n:
    print(r.get('id') or (r.get('path','').split('/')[-1] if r.get('path') else '') or r.get('uid') or ''); break
" RELEASE="${RELEASE}" 2>/dev/null || true)"
  [[ -n "${INSTANCE_ID}" ]] && break
  sleep 5
done
[[ -n "${INSTANCE_ID}" ]] || { echo "FAIL: whoami instance not found for ${RELEASE}"; exit 1; }

bash "${ROOT}/scripts/verify-instance-backup-api.sh" --instance-id "${INSTANCE_ID}"
echo "OK backup-instance-api ${INSTANCE_ID}"
