#!/usr/bin/env bash
# Run kubectl on the hub when the local control node has no readable k3s kubeconfig.
set -euo pipefail

hub_kubectl() {
  local kc="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
  if [[ -r "$kc" ]]; then
    kubectl --kubeconfig="$kc" "$@"
  elif [[ -n "${FLEET_HUB_IP:-}" ]]; then
    local ssh_opts=(-o BatchMode=yes -o ConnectTimeout=10)
    local remote="" a
    for a in "$@"; do
      remote+=" $(printf '%q' "$a")"
    done
    local args=" $* "
    if [[ "${args}" == *" -f -"* || "${args}" == *" -f-"* ]]; then
      # shellcheck disable=SC2029
      ssh "${ssh_opts[@]}" "${HUB_SSH:-linux@${FLEET_HUB_IP}}" "sudo kubectl${remote}"
    else
      # shellcheck disable=SC2029
      ssh -n "${ssh_opts[@]}" "${HUB_SSH:-linux@${FLEET_HUB_IP}}" "sudo kubectl${remote}"
    fi
  elif [[ -f "$kc" ]]; then
    sudo kubectl --kubeconfig="$kc" "$@"
  else
    echo "hub_kubectl: no local kubeconfig and FLEET_HUB_IP unset" >&2
    return 127
  fi
}

hub_kubectl_remote() {
  hub_kubectl "$@"
}
