#!/usr/bin/env bash
# Disable retired virt spoke policies and remove stale site-* policies for deleted clusters.
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"

LEGACY=(
  lab-route-virt02 lab-route-virt01
  lab-spread-storage-rwx lab-spread-ha-storage
  homelab-operator-virt02 homelab-operator-virt02-t2
  homelab-helm-virt01 homelab-helm-virt02
  lab-helm-hubonly-virt02 lab-helm-dual
)

disable() {
  curl -sf -X PATCH "${API}/policies/${1}" \
    -H 'Content-Type: application/merge-patch+json' \
    -d '{"enabled":false}' >/dev/null 2>&1 || true
  echo "disabled ${1}"
}

echo "=== disable legacy lab policies ==="
for id in "${LEGACY[@]}"; do
  disable "${id}"
done

if [ "${FLEET_CLEANUP_STALE_SITE_POLICIES:-0}" = "1" ]; then
  echo "=== remove disabled site-* policies (no matching provider) ==="
  curl -sf "${API}/providers" | python3 -c "
import json, sys, subprocess, os
api = os.environ['API']
providers = {p['name'] for p in json.load(sys.stdin).get('providers', [])}
pols = json.loads(subprocess.check_output(['curl', '-sf', f'{api}/policies']))
for p in pols.get('policies', []):
    pid = p.get('id', '')
    if not (pid.startswith('site-container-') or pid.startswith('site-helm-')):
        continue
    cluster = pid.split('-', 2)[-1] if pid.startswith('site-container-') else pid.replace('site-helm-', '', 1)
    want = 'k8s-' + cluster if pid.startswith('site-container-') else 'helm-' + cluster
    if want not in providers:
        print(pid)
" | while read -r id; do
    [ -z "${id}" ] && continue
    curl -sf -X DELETE "${API}/policies/${id}" >/dev/null 2>&1 || true
    echo "deleted ${id}"
  done
fi

echo "=== cleanup-legacy-dcm-policies OK ==="
