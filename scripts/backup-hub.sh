#!/usr/bin/env bash
# Hub backup — K3s etcd snapshot and WireGuard keys.
# Run on hub as root or linux with sudo.
# Usage: sudo ./scripts/backup-hub.sh
set -euo pipefail

BACKUP_ROOT="${BACKUP_ROOT:-/var/backups/umkim}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DEST="${BACKUP_ROOT}/${STAMP}"

mkdir -p "${DEST}/etcd" "${DEST}/wireguard"

echo "[backup-hub] writing to ${DEST}"

if command -v k3s >/dev/null 2>&1; then
  echo "[backup-hub] k3s etcd snapshot"
  k3s etcd-snapshot save --name "umkim-${STAMP}"
  SNAP_DIR="/var/lib/rancher/k3s/server/db/snapshots"
  if [[ -d "$SNAP_DIR" ]]; then
    cp -a "${SNAP_DIR}/"* "${DEST}/etcd/" 2>/dev/null || true
  fi
else
  echo "[backup-hub] WARN k3s not found — skip etcd"
fi

if [[ -d /etc/wireguard ]]; then
  tar -czf "${DEST}/wireguard/wg.tgz" -C /etc/wireguard .
fi

cat > "${DEST}/manifest.json" <<EOF
{
  "umkim_version": "0.2.0-dev",
  "timestamp": "${STAMP}",
  "hostname": "$(hostname -f 2>/dev/null || hostname)",
  "components": ["etcd", "wireguard"]
}
EOF

echo "[backup-hub] Harbor: backup registry PVC and DB separately (see docs/backup-restore.md)"
echo "[backup-hub] Off-host copy: rsync -a ${DEST}/ backup-host:umkim/"
echo "[backup-hub] Remember Ansible inventory + ansible-vault password file"
echo "[backup-hub] done ${DEST}"
