# BUG: create_vm fails when libvirt domain already active

**Phase:** B10  
**Date:** 2026-10-02 (CEST)

## Symptom
`umkim site host provision homelab lab-gpu-pt-0-cp1 --action create_vm` against an
already-running guest fails:

```
fatal: ... virsh start lab-gpu-pt-0-cp1
stderr: error: Domain is already active
```

Job `fleet-site-prov-0d133fa14b9f` → Failed; host phase Failed despite healthy VM.

## Impact
Re-running create_vm after a prior Succeeded create (or manual start) is not
idempotent; hub host status stuck Failed though guest SSH/GPU OK.

## Workaround
Skip re-create when `virsh list` shows domain running; treat prior Succeeded
create_vm as authoritative. Or destroy then create_vm.
