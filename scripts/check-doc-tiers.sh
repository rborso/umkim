#!/usr/bin/env bash
# Fail if design-tier docs contain lab IP literals (not in allowlist).
# Runbook tiers (install, e2e lab) may reference lab IPs per docs tier policy.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ALLOWLIST="${ROOT}/docs/.lab-ip-allowlist"
PATTERN='192\.168\.1\.|10\.10\.0\.'
FAIL=0

# Install + E2E runbooks are inventory-adjacent; not generic design docs.
RUNBOOK_EXCLUDE=(
  docs/install.md
  docs/e2e-full-lab-test.md
  docs/fleet-verify.md
)

is_runbook_excluded() {
  local rel="$1"
  local entry
  for entry in "${RUNBOOK_EXCLUDE[@]}"; do
    [[ "$rel" == "$entry" ]] && return 0
  done
  return 1
}

check_file() {
  local f="$1"
  local rel="${f#"$ROOT"/}"
  is_runbook_excluded "$rel" && return 0
  if grep -qE "$PATTERN" "$f" 2>/dev/null; then
    while IFS= read -r line; do
      if [[ -n "$line" ]] && ! grep -qxF "$rel" "$ALLOWLIST" 2>/dev/null; then
        echo "LAB-IP in generic doc: $rel:$line"
        FAIL=1
      fi
    done < <(grep -nE "$PATTERN" "$f" | cut -d: -f1 || true)
  fi
}

while IFS= read -r -d '' f; do
  check_file "$f"
done < <(find "$ROOT/docs" -name '*.md' ! -path '*/archive/*' -print0)

if [[ "$FAIL" -ne 0 ]]; then
  echo "check-doc-tiers: move lab IPs to inventory/ or add path to docs/.lab-ip-allowlist"
  exit 1
fi
echo "check-doc-tiers: OK"
