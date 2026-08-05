#!/usr/bin/env bash
# Push umkim-catalog charts to Harbor and seed DCM catalog items + placement policies.
#
# Prerequisites: sibling checkout at ../umkim-catalog, shared/config.env configured.
# Usage: ./scripts/seed-umkim-catalog.sh [--push-charts-only | --seed-only]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CATALOG_ROOT="${UMKIM_CATALOG_ROOT:-${ROOT}/../umkim-catalog}"
MODE="${1:-all}"

[[ -d "${CATALOG_ROOT}/shared/scripts" ]] || {
  echo "ERROR: umkim-catalog not found at ${CATALOG_ROOT}" >&2
  echo "umkim-catalog is optional and typically private — not required for greenfield hub install." >&2
  echo "If you have access: git clone https://github.com/rborso/umkim-catalog.git ${CATALOG_ROOT}" >&2
  exit 1
}

push_charts() {
  for item in gitea paperless uptime-kuma nextcloud vaultwarden immich; do
    "${CATALOG_ROOT}/shared/scripts/push-chart.sh" "${item}"
  done
}

seed_catalog() {
  "${CATALOG_ROOT}/shared/scripts/seed-catalog.sh"
}

case "${MODE}" in
  --push-charts-only) push_charts ;;
  --seed-only) seed_catalog ;;
  all|"") push_charts; seed_catalog ;;
  -h|--help)
    sed -n '1,8p' "$0"
    exit 0
    ;;
  *) echo "unknown arg: ${MODE}" >&2; exit 1 ;;
esac

echo "umkim-catalog seed complete"
