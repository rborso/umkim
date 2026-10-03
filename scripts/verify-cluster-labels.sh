#!/usr/bin/env bash
# Verify OCM ManagedCluster fleet.* labels match Ansible inventory expectations.
# Run on hub (umkim-hub) or anywhere with KUBECONFIG pointing at hub.
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG

check_mc() {
  local name="$1"
  local expect_gpu="${2:-false}"
  local expect_storage_rwo="${3:-true}"
  local expect_storage_rwx="${4:-false}"
  local expect_nest_depth="${5:-0}"
  local expect_provisioner="${6:-k3s}"

  if ! kubectl get managedcluster "$name" &>/dev/null; then
    echo "SKIP $name (ManagedCluster not found)"
    return 0
  fi

  local labels
  labels="$(kubectl get managedcluster "$name" -o json | jq -r '.metadata.labels // {}')"

  local gpu storage_rwo storage_rwx ha_storage nest_depth provisioner
  gpu="$(echo "$labels" | jq -r '."fleet.capability/gpu" // "missing"')"
  storage_rwo="$(echo "$labels" | jq -r '."fleet.capability/storage.rwo" // "missing"')"
  storage_rwx="$(echo "$labels" | jq -r '."fleet.capability/storage.rwx" // "missing"')"
  ha_storage="$(echo "$labels" | jq -r '."fleet.capability/ha_storage" // "missing"')"
  nest_depth="$(echo "$labels" | jq -r '."fleet.topology/nest_depth" // "missing"')"
  provisioner="$(echo "$labels" | jq -r '."fleet.topology/provisioner" // "missing"')"

  local ok=true
  if [[ "$gpu" != "$expect_gpu" ]]; then
    echo "FAIL $name fleet.capability/gpu: got $gpu want $expect_gpu"
    ok=false
  fi
  if [[ "$storage_rwo" != "$expect_storage_rwo" ]]; then
    echo "FAIL $name fleet.capability/storage.rwo: got $storage_rwo want $expect_storage_rwo"
    ok=false
  fi
  if [[ "$storage_rwx" != "$expect_storage_rwx" ]]; then
    echo "FAIL $name fleet.capability/storage.rwx: got $storage_rwx want $expect_storage_rwx"
    ok=false
  fi
  local expect_ha_alias="false"
  if [[ "$expect_storage_rwo" == "true" || "$expect_storage_rwx" == "true" ]]; then
    expect_ha_alias="true"
  fi
  if [[ "$ha_storage" != "$expect_ha_alias" ]]; then
    echo "FAIL $name fleet.capability/ha_storage alias: got $ha_storage want $expect_ha_alias"
    ok=false
  fi
  if [[ "$nest_depth" != "$expect_nest_depth" ]]; then
    echo "FAIL $name fleet.topology/nest_depth: got $nest_depth want $expect_nest_depth"
    ok=false
  fi
  if [[ "$provisioner" != "$expect_provisioner" ]]; then
    echo "FAIL $name fleet.topology/provisioner: got $provisioner want $expect_provisioner"
    ok=false
  fi
  if [[ "$ok" == true ]]; then
    echo "OK   $name labels (gpu=$gpu storage.rwo=$storage_rwo storage.rwx=$storage_rwx nest_depth=$nest_depth provisioner=$provisioner)"
  else
    return 1
  fi
}

echo "=== verify-cluster-labels (hub KUBECONFIG=$KUBECONFIG) ==="
check_mc virt01 false true false 0 k3s
check_mc virt02 false true true 0 k3s
check_mc virt01-k3s false true false 0 k3s
# virt02-t2 (legacy nested CAPK MC) removed — nested clusters TBD via KubeVirt + K3s
echo "=== all checks passed ==="
