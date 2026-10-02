# BUG-20261002-B6: create_vm placement total_mib=0 / cannot place

**Stage:** B6 HA create_vm
**Severity:** High
**Observed:** 2026-10-02 ~11:46 Europe/Budapest
**Root cause found:** 2026-10-02 ~12:11 (resume)

## Expected
least-utilized placement uses real hypervisor free memory (~32 GiB on virt01/02).

## Actual
Schedule report: `hypervisor=virt01 total_mib=0 reserve_mib=4096 used_mib=4096 free_mib=0` (all HVs).
Fail: `VM placement failed for: lab-ha-0-cp1 (4096 MiB): virt01: free=-4096 MiB ...`

## Root cause
`site-provision-connector.yml` builds ansible `--limit` as `${subject},${vm_location}`.
When `vm_location` is a schedule token (`least-utilized` / `auto` / empty), the limit becomes
`lab-ha-0-cp1,least-utilized`. That is **not** an inventory group, so the first play
(`hosts: hypervisors`, `gather_facts: true`) is skipped. Without gathered
`ansible_memtotal_mb` and without `*.notes.json` / `hypervisor_memory_mib` fallbacks,
`schedule_load_inventory_memory` leaves totals at 0 → placement fails.

Related to run1 BUG-20261001-B6-create_vm-placement-oversubscribed (symptoms overlap).

## Fix
Map schedule tokens to the `hypervisors` group in the LIMIT builder so gather runs:
`images/fleet-ansible-runner/site-provision-connector.yml` (and repo-root copy).

## Workaround (used for run2 B6)
Pin `vm_location` to virt01/virt02 in host_vars + guests.yml; retry create_vm.

## Evidence
cli-logs/B6-create-vm-cp1-fail.txt
