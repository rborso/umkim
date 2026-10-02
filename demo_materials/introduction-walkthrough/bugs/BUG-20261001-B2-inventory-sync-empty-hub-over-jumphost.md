# BUG / hazard: empty hub inventory sync overwrites jumphost YAIDAP tree

## Context
Phase B2: after migrate, hub inventory was mostly empty stubs. `umkim site inventory sync` projected that empty tree onto jumphost `/home/linux/yaidap/inventory/homelab/`, wiping useful hypervisor/guest indexes that previously lived only on the jumphost.

## Impact
Hosts disappear from hub view; restore required from `demo_materials/yaidap-inventory-reference` or backup.

## Suggested fix
Refuse sync when hub inventory is empty/incomplete relative to jumphost, or require `--confirm` with a diff summary before projecting deletes.
