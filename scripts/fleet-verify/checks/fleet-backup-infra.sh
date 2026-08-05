#!/usr/bin/env bash
set -euo pipefail

FLEET_API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

code="$(curl -s -o "${tmp}" -w '%{http_code}' "${FLEET_API}/backup-infrastructure" || true)"

if [[ "${code}" == "404" ]]; then
  echo "WARN backup-infrastructure not configured (404), skipping hub-only"
  exit 0
fi

if [[ "${code}" != "200" ]]; then
  echo "FAIL backup-infrastructure HTTP ${code}" >&2
  exit 1
fi

if python3 -c "import json; json.load(open('${tmp}'))" 2>/dev/null; then
  echo "OK fleet backup-infrastructure"
  exit 0
fi

echo "FAIL backup-infrastructure invalid JSON" >&2
exit 1
