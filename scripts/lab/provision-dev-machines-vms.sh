#!/usr/bin/env bash
# Legacy helper: provision libvirt VMs via archived lab playbooks.
# Those playbooks are NOT shipped in this repository (see docs/archive/README.md).
# Prefer FleetClusterJoin / playbooks/flows/ for greenfield clusters.
set -euo pipefail

cat >&2 <<'EOF'
provision-dev-machines-vms.sh depends on archived lab playbooks that are not
included in this repository (docs/archive/lab/vbm-lab.yml and related roles).

Use instead:
  - docs/install.md — greenfield hub + FleetClusterJoin
  - playbooks/flows/examples/ — site/cluster create flows
  - docs/e2e-full-lab-test.md — org-separation E2E with placeholder inventory

See docs/archive/README.md.
EOF
exit 1
