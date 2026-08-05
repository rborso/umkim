#!/usr/bin/env bash
# Scan for leaked verify-synth / E2E resources after L2.
set -euo pipefail

FV_ROOT="${FV_ROOT:?}"
FV_REPORT_DIR="${FV_REPORT_DIR:?}"
RESIDUAL_JSON="${FV_REPORT_DIR}/residual.json"

fleet_api="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
dcm_api="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"

sites=()
clusters=()
instances=()

if curl -sf "${fleet_api}/sites" >/dev/null 2>&1; then
  while IFS= read -r s; do
    [[ -n "$s" ]] && sites+=("$s")
  done < <(curl -sf "${fleet_api}/sites" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for s in d.get('sites',[]):
  n=s.get('id') or s.get('siteName') or ''
  if 'verify-synth' in n or n.startswith('E2E '):
    print(n)
" 2>/dev/null || true)
fi

if curl -sf "${fleet_api}/clusters" >/dev/null 2>&1; then
  while IFS= read -r c; do
    [[ -n "$c" ]] && clusters+=("$c")
  done < <(curl -sf "${fleet_api}/clusters" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for c in d.get('clusters',[]):
  n=c.get('name') or ''
  if 'verify-synth' in n or n.startswith('verify-join'):
    print(n)
" 2>/dev/null || true)
fi

if curl -sf "${dcm_api}/catalog-item-instances" >/dev/null 2>&1; then
  while IFS= read -r i; do
    [[ -n "$i" ]] && instances+=("$i")
  done < <(curl -sf "${dcm_api}/catalog-item-instances" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for r in d.get('results',[]):
  n=r.get('display_name') or ''
  if 'verify-synth' in n or n.startswith('E2E '):
    print(n)
" 2>/dev/null || true)
fi

python3 -c "
import json, sys
print(json.dumps({
  'sites': sys.argv[1].split('\t') if sys.argv[1] else [],
  'clusters': sys.argv[2].split('\t') if sys.argv[2] else [],
  'instances': sys.argv[3].split('\t') if sys.argv[3] else [],
}, indent=2))" \
  "$(printf '%s\t' "${sites[@]:-}")" \
  "$(printf '%s\t' "${clusters[@]:-}")" \
  "$(printf '%s\t' "${instances[@]:-}")" \
  > "${RESIDUAL_JSON}"

if [[ ${#sites[@]} -gt 0 || ${#clusters[@]} -gt 0 || ${#instances[@]} -gt 0 ]]; then
  echo "WARN: residual verify resources — see ${RESIDUAL_JSON}"
  exit 1
fi
echo "OK: no residual verify-synth resources"
exit 0
