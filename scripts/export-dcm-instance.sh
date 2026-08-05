#!/usr/bin/env bash
# Export a DCM catalog-item-instance JSON manifest.
#
# Usage:
#   ./scripts/export-dcm-instance.sh --id <instance-id> [--out /path/to.json]
set -euo pipefail

API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
INSTANCE_ID=""
OUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --id) INSTANCE_ID="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,8p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$INSTANCE_ID" ]] || { echo "ERROR: --id required" >&2; exit 1; }

if [[ -z "$OUT" ]]; then
  STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
  ROOT="${FLEET_INSTANCE_BACKUP_ROOT:-/var/backups/umkim/instances}"
  OUT="${ROOT}/${STAMP}/dcm-instance.json"
  mkdir -p "$(dirname "$OUT")"
fi

curl -sf "${API}/catalog-item-instances/${INSTANCE_ID}" | jq . >"$OUT"
echo "$OUT"
