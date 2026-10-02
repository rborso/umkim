# BUG-20261001-A7-spoke-leftover-wireguard

## Stage
A7 / B3 precheck on virt03

## Summary
After cluster delete + `k3s-uninstall.sh`, virt03 still had `wg-umkim` (10.10.0.10/32) and `tunl0` (Calico) interfaces up.

## Evidence
`assets/B3-virt03-ssh-precheck.txt`

## Suggested fix
Cluster cleanup should remove overlay WireGuard and CNI interfaces (or document a spoke reset script).
