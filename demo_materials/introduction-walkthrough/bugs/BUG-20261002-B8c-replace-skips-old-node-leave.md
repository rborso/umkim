# BUG: replaceControlPlane Succeeded but old CP not removed (graceful leave no hosts matched)

**Date:** 2026-10-02  
**Phase:** B8c  
**Severity:** high (replace does not replace; leaves extra etcd voter)

## Summary
`umkim node replace --cluster lab-ha-0 --old-node lab-ha-0-cp3 --address 172.16.10.143 --hostname lab-ha-0-cp4` (no `--force`) completed with history **Succeeded**, joined `lab-ha-0-cp4` as control-plane/etcd, but **did not remove** `lab-ha-0-cp3`.

Ansible log:

```
PLAY [… Graceful leave on old node (online)] ****
skipping: no hosts matched

[WARNING]: Could not match supplied host pattern, ignoring: lab-ha-0-cp3
```

Force-path etcd remove tasks were correctly skipped (`force=false`). Verification only waited for the **new** node Ready, so the op was marked Succeeded with 4 Ready CP/etcd voters (cp1–cp4).

VIP `172.16.10.100:6443/healthz` still returns anonymous 401 (expected). Cluster phase Ready.

## Expected
Graceful replace inventory must include the old CP host so `cluster_leave` runs; old node should leave etcd and disappear from `kubectl get nodes` / `umkim node list`.

## Actual
Old host pattern missing from job inventory → leave skipped → add-only behavior → 4-node HA.

## Follow-up not taken
Did not run `--force` re-replace against already-live cp4 (risk of re-join / disruption). Left 4 Ready CPs; filed this bug.

## Evidence
- `assets/B8c-replace-logs.txt`
- `assets/B8c-cp-replace-history.txt`
- `assets/B8c-node-list-before-replace.txt` / `B8c-node-list-after.txt` / `B8-node-list-final.txt`
- `assets/B8c-cluster-get-before.json` / `B8c-cluster-get-after.json`
- `assets/B8c-vip-healthz-before.txt` / `B8c-vip-healthz-after.txt`
