# BUG: leftover `tunl0` (and often WireGuard) after spoke k3s uninstall

## Context
After Phase A7 cluster teardown / manual `k3s-uninstall.sh` on virt03, B3 discover still saw interface `tunl0` UP with a CNI-style address.

## Impact
Informational inventory noise; possible network confusion if leftover tunnels stay UP.

## Evidence
B3 discover host summary listed `tunl0` alongside `enp2s0` / bridge.

## Suggested fix
Uninstall / cleanup should remove CNI tunnels and site WireGuard iface/config when leaving the cluster.
