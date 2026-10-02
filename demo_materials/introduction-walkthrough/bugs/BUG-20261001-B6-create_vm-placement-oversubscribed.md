# BUG: create_vm fails placement when hypervisors oversubscribed / existing domain not adopted

**Date:** 2026-10-01  
**Phase:** B6  
**Severity:** high

## Summary
`create_vm` for `lab-ha-0-cp1` (least-utilized) failed:

```
assertion: schedule_unplaced_guests | length == 0
msg: VM placement failed for: lab-ha-0-cp1 (4096 MiB):
  virt01: free=-4096 MiB need=4096 MiB;
  virt02: free=-8192 MiB need=4096 MiB; ...
```

Playbook ran probe/set-placement-from-existing tasks, yet guest remained unplaced despite running `lab-ha-0-*` domains. Hypervisors also hosted leftover `cli-test-*` / `lab-single-1-0`.

## Workaround
Pin `vm_location` to current hypervisor (virt01/virt02) matching existing domains; retry create_vm (Succeeded).
