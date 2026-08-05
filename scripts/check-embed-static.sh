#!/usr/bin/env bash
# CI/local: regenerate bootstrap embed and fail if controllers/fleet-site/internal/bundle/static drifts.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"
"${ROOT}/controllers/fleet-site/sync-bundle-static.sh"
if ! git diff --exit-code -- controllers/fleet-site/internal/bundle/static; then
  echo "ERROR: bundle static tree is out of date. Run: ./scripts/stage-embed-assets.sh" >&2
  exit 1
fi
echo "bundle static tree is up to date."
