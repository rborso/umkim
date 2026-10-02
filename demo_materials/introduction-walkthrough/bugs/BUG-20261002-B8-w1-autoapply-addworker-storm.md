# BUG / ops: lab-ha-0-w1 group_vars autoApply=true caused addWorker retry storm

**Date:** 2026-10-02  
**Phase:** B8 (precondition from B6 cluster-apply)  
**Severity:** medium (job queue noise; masks real ops)

## Summary
`group_vars/lab-ha-0-w1.yml` had `mode: add_worker`, `autoApply: true`. Guest Ready but unreachable with fleet SSH key → continuous Failed `addWorker` ops (~30s interval) filling `nodeOperationHistory` and contending for `fleet-cluster-nodeop-lab-ha-0`.

## Workaround
Set `autoApply: false` via inventory files put + sync before B8a (`assets/B8-disable-w1-autoapply.txt`).

## Evidence
`assets/B8-w1-add-fail-logs.txt`, early `B8a-node-op-history*.txt`
