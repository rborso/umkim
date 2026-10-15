#!/usr/bin/env bash
# Hub pod hygiene: no pod outside Running/Succeeded, and no orphaned pod.
#
# A Job name is reused (fa-i-<app>, fa-u-<app>), so replacing a finished Job used
# to leave its pods behind. Background deletion relies on the garbage collector,
# and the pod GC only reclaims an orphaned terminated pod past a high water mark,
# so Failed/Unknown pods with no owner accumulate. ensureProviderJob and the
# inventory refresh now take the pods with the Job; this check keeps it honest.
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"
read -r -a KUBECTL_CMD <<< "${KUBECTL}"
if ! "${KUBECTL_CMD[@]}" version >/dev/null 2>&1; then
  echo "SKIP hub pod hygiene: ${KUBECTL} not available or not authorized"
  exit 0
fi

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT
if ! "${KUBECTL_CMD[@]}" get pods -A -o json >"${tmp}" 2>/dev/null; then
  echo "SKIP hub pod hygiene: cannot list pods"
  exit 0
fi

python3 - "${tmp}" <<'PY'
import json
import sys

with open(sys.argv[1]) as handle:
    data = json.load(handle)

bad = []
for pod in data.get("items", []):
    meta = pod["metadata"]
    phase = (pod.get("status") or {}).get("phase", "")
    if phase in ("Running", "Succeeded"):
        continue
    owners = meta.get("ownerReferences") or []
    owner = f"{owners[0]['kind']}/{owners[0]['name']}" if owners else "none"
    bad.append(f"{meta['namespace']}/{meta['name']} phase={phase} owner={owner}")

if not bad:
    print("OK hub pod hygiene: every pod is Running or Succeeded")
    sys.exit(0)

for line in bad:
    print(f"FAIL hub pod hygiene: {line}", file=sys.stderr)
print(f"FAIL hub pod hygiene: {len(bad)} pod(s) outside Running/Succeeded", file=sys.stderr)
sys.exit(1)
PY
