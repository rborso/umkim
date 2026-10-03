#!/usr/bin/env bash
# Stage bootstrap bundle for go:embed (delegates to canonical sync script).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
exec "${ROOT}/controllers/fleet-site/sync-bundle-static.sh"
