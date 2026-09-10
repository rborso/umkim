#!/usr/bin/env bash
# Security scans for UMKIM v0.1.0 release gate.
# Usage: ./scripts/security-scan.sh [--skip-images]
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
SKIP_IMAGES=0
[[ "${1:-}" == "--skip-images" ]] && SKIP_IMAGES=1

FAIL=0
note() { echo "[security-scan] $*"; }
fail() { note "FAIL $*"; FAIL=1; }

note "OSS tip scrub (maintainer PII)"
if ! ./scripts/preflight-oss-scrub.sh; then
  fail "preflight-oss-scrub"
fi

note "placeholder grep in inventory/roles (excluding examples/docs)"
if rg -i 'change-me|ChangeMe|REPLACE_ME' inventory/group_vars/hub.yml roles/harbor/defaults/main.yml 2>/dev/null \
   | rg -v 'default\(' | rg -v 'vault' >/dev/null; then
  note "WARN plaintext placeholder defaults remain (use vault in production)"
fi

note "Go controllers: go vet"
for d in controllers/fleet-edge controllers/fleet-site controllers/fleet-console-auth; do
  if [[ -d "$d" ]]; then
    (cd "$d" && go vet ./...) || fail "go vet $d"
  fi
done

if command -v govulncheck >/dev/null 2>&1; then
  note "Go controllers: govulncheck"
  for d in controllers/fleet-edge controllers/fleet-site controllers/fleet-console-auth; do
    [[ -d "$d" ]] && (cd "$d" && govulncheck ./...) || fail "govulncheck $d"
  done
else
  note "WARN govulncheck not installed"
fi

if command -v ansible-lint >/dev/null 2>&1; then
  note "ansible-lint"
  ansible-lint fleet-deploy.yml hub-firewall.yml phase5.yml fleet-console-auth.yml playbooks/preflight.yml || fail "ansible-lint"
else
  note "WARN ansible-lint not installed"
fi

if [[ "$SKIP_IMAGES" == "0" ]] && command -v trivy >/dev/null 2>&1; then
  TAG="${HARBOR_FLEET_TAG:-main}"
  REG="${HARBOR_REGISTRY:-10.10.0.1:30443/fleet}"
  for img in fleet-console-auth umkim-console fleet-site-controller; do
    note "trivy $REG/$img:$TAG"
    trivy image --severity CRITICAL --exit-code 1 "$REG/$img:$TAG" || fail "trivy $img"
  done
elif [[ "$SKIP_IMAGES" == "0" ]]; then
  note "WARN trivy not installed — skip image scan (use --skip-images)"
fi

if [[ "$FAIL" != "0" ]]; then
  exit 1
fi
note "OK"
exit 0
