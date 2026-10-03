#!/usr/bin/env bash
# Refresh a sibling umkim-public tree from this repo's committed tip (scrubbed inventory).
# Does not push. Lab-only working-tree edits are not exported.
#
# Usage:
#   ./scripts/sync-umkim-public.sh              # default: ../umkim-public
#   ./scripts/sync-umkim-public.sh /path/to/dir
#
# After sync: cd ../umkim-public && ./scripts/preflight-oss-scrub.sh && git status
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="${1:-${ROOT}/../umkim-public}"

if [[ ! -d "$DEST/.git" ]]; then
  echo "ERROR: destination is not a git repo: $DEST" >&2
  exit 1
fi

cd "$ROOT"
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "NOTE: working tree has local changes; export uses committed HEAD only ($(git rev-parse --short HEAD))."
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

git archive HEAD | tar -x -C "$TMP"

# Preserve destination .git; replace tracked content from the archive.
# Delete files that vanished on HEAD (except .git and local-only paths).
rsync -a --delete \
  --exclude '.git/' \
  --exclude 'reports/' \
  --exclude '.cursor/' \
  --exclude 'secrets/' \
  --exclude 'output/' \
  --exclude 'inventory_backup/' \
  --exclude '.harbor-staging/' \
  --exclude 'ui/umkim-console/node_modules/' \
  --exclude 'ui/umkim-console/e2e/.auth/' \
  "$TMP"/ "$DEST"/

# Drop stale paths that must not ship in a public tip (removed on umkim main).
rm -rf "$DEST/.github" \
  "$DEST/ui/umkim-console/e2e/.auth"

echo "Synced $DEST from umkim@$(git rev-parse --short HEAD)"
echo "Next: cd $DEST && ./scripts/preflight-oss-scrub.sh && git status"
