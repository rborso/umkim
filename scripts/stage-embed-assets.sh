#!/usr/bin/env bash
# Stage generated Ansible trees for fleet-site-controller embed and fleet-ansible-runner image.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
"${ROOT}/controllers/fleet-site/sync-bundle-static.sh"
"${ROOT}/images/fleet-ansible-runner/build-context.sh"
echo "Staged embed static and fleet-ansible-runner build context."
