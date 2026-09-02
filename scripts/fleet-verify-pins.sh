#!/usr/bin/env bash
# v0.2 does not pin sibling DCM/service-provider repos. This command is a no-op.
set -euo pipefail
echo "OK: sibling git pins are not used in v0.2 (images are built from this checkout)."
exit 0
