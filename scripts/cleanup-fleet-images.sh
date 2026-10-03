#!/usr/bin/env bash
# Prune stale fleet build images (podman + k3s containerd). Safe on hub or dev host.
set -euo pipefail

log() { echo "[cleanup-fleet-images] $*"; }

if command -v podman >/dev/null 2>&1; then
  log "podman: removing dangling images"
  podman image prune -f >/dev/null 2>&1 || true
  for pat in 'localhost/fleet-site-controller:test*' 'localhost/fleet-site-controller:main' \
    'harbor.local:*' 'localhost/fleet-site-controller' 'localhost/fleet-ansible-runner' \
    'localhost/umkim-console'; do
    podman images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep -E "^${pat//\*/.*}$" || true
  done | sort -u | while read -r img; do
    [[ -z "${img}" || "${img}" == "<none>:<none>" ]] && continue
    log "podman rmi ${img}"
    podman rmi "${img}" 2>/dev/null || true
  done
fi

K3S_CTR=""
for c in /usr/local/bin/k3s ctr ctr; do
  if [[ -x /usr/local/bin/k3s ]]; then
    K3S_CTR="/usr/local/bin/k3s ctr"
    break
  fi
done

if [[ -n "${K3S_CTR}" ]]; then
  log "k3s ctr: prune unused fleet-site-controller tags"
  for ns in k8s.io default; do
    while read -r img; do
      [[ -z "${img}" ]] && continue
      case "${img}" in
        *fleet-site-controller:v2-provision*|*fleet-site-controller@test*|*harbor.local*)
          log "k3s ctr -n ${ns} images rm ${img}"
          sudo ${K3S_CTR} -n "${ns}" images rm "${img}" 2>/dev/null || true
          ;;
      esac
    done < <(sudo ${K3S_CTR} -n "${ns}" images ls -q 2>/dev/null | grep fleet-site || true)
  done
fi

log "done"
