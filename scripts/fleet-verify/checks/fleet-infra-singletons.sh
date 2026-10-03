#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

for path in ingress-auth acme llm; do
  if ! curl -sf "${FLEET_API}/${path}" >/dev/null; then
    echo "FAIL fleet GET /${path}" >&2
    exit 1
  fi
done

echo "OK fleet singletons (ingress-auth, acme, llm)"
exit 0
