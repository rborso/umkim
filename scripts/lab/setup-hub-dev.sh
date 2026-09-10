#!/usr/bin/env bash
# Bootstrap ~/umkim on umkim-hub for Ansible + Harbor + Go + UI builds.
# Run on the hub as user linux (after repo clone):
#   cd ~/umkim && ./scripts/setup-hub-dev.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

echo "=== setup-hub-dev (repo=${REPO_ROOT}) ==="

if ! command -v ansible-playbook >/dev/null 2>&1; then
  echo "Installing ansible-core, git, golang, python3-pip, podman…"
  sudo dnf install -y ansible-core git golang python3-pip podman containernetworking-plugins
fi

if ! node --version 2>/dev/null | grep -qE 'v(18|20|22)\.'; then
  echo "Installing Node.js 20 (Vite requires >=18)…"
  curl -fsSL https://rpm.nodesource.com/setup_20.x | sudo bash -
  sudo dnf remove -y nodejs npm 2>/dev/null || true
  sudo dnf install -y nodejs --allowerasing
fi

mkdir -p ~/.ssh
chmod 700 ~/.ssh
if [ ! -f ~/.ssh/config ]; then
  cat > ~/.ssh/config <<'EOF'
Host github.com
  HostName github.com
  User git
  IdentityFile ~/.ssh/id_ed25519
  IdentitiesOnly yes

Host 10.10.0.*
  User linux
  StrictHostKeyChecking accept-new
EOF
  chmod 600 ~/.ssh/config
fi

if [ ! -f ~/.ssh/id_ed25519 ]; then
  echo "WARN: ~/.ssh/id_ed25519 missing — copy your GitHub/spoke key before git push/pull."
fi

git config --global --get user.email >/dev/null 2>&1 || git config --global user.email "linux@umkim-hub"
git config --global --get user.name >/dev/null 2>&1 || git config --global user.name "linux"

echo "=== Go controllers ==="
for d in controllers/fleet-site controllers/fleet-console-auth controllers/fleet-edge; do
  if [ -d "${d}" ]; then
    echo "  go build ./... in ${d}"
    (cd "${d}" && go build ./...)
  fi
done

echo "=== Console UI ==="
if [ -f ui/umkim-console/package.json ]; then
  (cd ui/umkim-console && npm ci && npm run build)
fi

cat <<EOF

=== Hub dev workspace ready ===
Repo:     ${REPO_ROOT}
Hub plays: ansible-playbook -i inventory/hosts.yml -i inventory/hosts-from-hub.yml <playbook> -l hub
Spokes:    same -i … -l virt01,virt02  (needs WG SSH key on hub)
Harbor:    ansible-playbook harbor.yml -l hub -t harbor_push_fleet -e @output/harbor-push-site-separation.yml
Verify:    FLEET_API=http://127.0.0.1:18080/api/v1alpha1 ./scripts/verify-fleet-cluster-join.sh

Open in Cursor: Remote-SSH → linux@203.0.113.1 → folder ${REPO_ROOT}
EOF
