#!/usr/bin/env bash
# Restore maintainer lab inventory from a local backup (gitignored).
# Default: inventory_backup/pre-oss-scrub-YYYYMMDD from the OSS scrub.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${1:-}"
if [[ -z "$SRC" ]]; then
  SRC="$(ls -d "${ROOT}"/inventory_backup/pre-oss-scrub-* 2>/dev/null | sort | tail -1 || true)"
fi
[[ -n "$SRC" && -d "$SRC" ]] || {
  echo "Usage: $0 [inventory_backup/pre-oss-scrub-YYYYMMDD]" >&2
  echo "No backup directory found." >&2
  exit 1
}

DEST="${ROOT}/inventory"
echo "Restoring lab inventory from ${SRC} -> ${DEST}"
# Preserve gitignored vault if present in dest but missing in older backup copy edge cases
VAULT_BAK=""
if [[ -f "${DEST}/group_vars/all/vault.yml" ]]; then
  VAULT_BAK="$(mktemp)"
  cp -a "${DEST}/group_vars/all/vault.yml" "$VAULT_BAK"
fi

rsync -a --delete \
  --exclude 'group_vars/all/vault.yml' \
  "${SRC}/" "${DEST}/"

if [[ -f "${SRC}/group_vars/all/vault.yml" ]]; then
  cp -a "${SRC}/group_vars/all/vault.yml" "${DEST}/group_vars/all/vault.yml"
elif [[ -n "$VAULT_BAK" ]]; then
  mkdir -p "${DEST}/group_vars/all"
  cp -a "$VAULT_BAK" "${DEST}/group_vars/all/vault.yml"
fi
[[ -n "$VAULT_BAK" ]] && rm -f "$VAULT_BAK"

echo "Done. Re-run ./scripts/bootstrap-hub-inventory.sh --force before any public commit."
