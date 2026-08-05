#!/usr/bin/env bash
set -euo pipefail

export VERIFY_CREATE_ORG=0
export FLEET_VERIFY_MUTATING=0

SCRIPTS="$(cd "$(dirname "$0")/../.." && pwd)"
REPO="$(cd "${SCRIPTS}/.." && pwd)"
export FV_ROOT="${FV_ROOT:-${REPO}}"
# shellcheck source=scripts/fleet-verify/lib/auth.sh
source "${SCRIPTS}/fleet-verify/lib/auth.sh"
fv_auth_load_password
export FLEET_AUTH_PASSWORD

exec "${SCRIPTS}/verify-fleet-console-auth.sh"
