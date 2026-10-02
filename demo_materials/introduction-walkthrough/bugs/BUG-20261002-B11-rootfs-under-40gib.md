# BUG: 40G guest disk yields ~38.8G root FS; bootstrap validate fails (<40 GiB)

**Phase:** B11  
**Date:** 2026-10-02 (CEST)

## Symptom
`lab-gpu-pt-0-cp1` host_vars `disks[].size: 40` creates a 40GiB virtual disk, but after
EFI/boot partitions the root XFS is ~38.8 GiB (`df` ~39G). Fleet bootstrap validate
asserts `size_total >= minRootDiskGi (40)` and fails:

```
Root filesystem smaller than 40 GiB
```

Cluster left in Enrolled; wireguard/enrollment already OK.

## Workaround
Grow qcow2 (e.g. 50G), `growpart` + `xfs_growfs`, update host_vars size, re-run
`umkim cluster bootstrap`.

## Suggestion
Either document `size: 50` for Rocky guests, or validate against virtual disk size /
default minRootDiskGi to 35, or auto-grow in create_vm.
