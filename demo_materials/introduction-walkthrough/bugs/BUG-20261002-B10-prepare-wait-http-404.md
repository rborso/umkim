# BUG: `umkim site host prepare/provision --wait` returns HTTP 404 while job succeeds

**Phase:** B10  
**Date:** 2026-10-02 (CEST)

## Symptom
`umkim site host prepare homelab virt04 --gpu-passthrough --confirm --wait` and
`umkim site host provision ... --wait` print `Error: HTTP 404: not found` almost
immediately, exit 0/1 inconsistently, while the FleetSite provision job still
starts and can Succeed (e.g. `prepare_hypervisor_gpu` job `1d5fe485eaf5`).

## Impact
Operators cannot rely on `--wait`; must poll `umkim site get` /
`status.provision.current|history` or kubectl job logs.

## Workaround
Omit `--wait` or ignore the 404 and poll FleetSite provision status until
Succeeded/Failed.
