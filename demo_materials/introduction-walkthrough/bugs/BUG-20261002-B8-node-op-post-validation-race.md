# BUG: node-op history/phase Failed after successful Ansible; post-validation race

**Date:** 2026-10-02  
**Phase:** B8a/B8b  
**Severity:** medium (operator confusion; history untrustworthy)

## Summary
After `umkim node add` for `lab-ha-0-w2`, Ansible PLAY RECAP showed success (`ok=30 changed=15`) and the node later appeared Ready in `umkim node list`, but `nodeOperation.phase` / history recorded **Failed** with:

`Post-validation failed: node lab-ha-0-w2 is not Ready`

Similarly, successful `removeWorker` showed `nodeOperation.phase=Succeeded` / message "Node operation completed" while history table listed the same id as **Failed**.

## Expected
Post-validation should wait long enough for kubelet Ready (or retry), and history phase should match the authoritative `status.nodeOperation.phase`.

## Actual
Transient NotReady at validation time marks the op Failed even when the node becomes Ready shortly after; history can disagree with live status.

## Evidence
- `assets/B8a-w2-add-logs-final.txt` (Ansible success)
- `assets/B8a-node-list-after-add.txt` / `B8a-node-list.txt` (w2 Ready)
- `assets/B8a-cluster-get-after-add.json` (Failed post-validation)
- `assets/B8b-node-remove-history.txt` vs `B8b-cluster-get-after.json`
