# BUG / gap: create_vm fails when guest host_vars absent from hub inventory store

**Date:** 2026-10-01  
**Phase:** B6  
**Severity:** high (blocks HA VM provision automation)

## Summary
`umkim site host provision homelab lab-ha-0-* --wait` (create_vm) failed for all four guests:

```
TASK [Require guest mgmt IP for VM create (connector store)] ***
fatal: ... test -n "$ip"  (host_vars/<guest>.yml missing or no ansible_host)
```

Hub inventory after B2/B3 restore only had hypervisor/BM host_vars + guests.yml listing —
**no** `host_vars/lab-ha-0-*.yml`, **no** `lab-ha-0.yml` / `lab-ha-0-w1.yml` cluster groups,
**no** `group_vars/lab-ha-0*.yml`. Guests still listed in `guests.yml` so `site host list` showed them Declared.

`cluster-apply` then failed: `guest is not a member of any inventory cluster group`.

## Workaround
PUT reference files from `demo_materials/yaidap-inventory-reference` (host_vars + lab-ha-0*.yml + group_vars), inventory sync, retry provision.

## Note
Pre-existing libvirt domains for lab-ha-0-* were already running and SSH-reachable (Rocky 10.2) before create_vm — leftover from prior lab; automation still requires host_vars for YAIDAP create_vm path.
