# BUG: capability discover returns stale Succeeded; job can stick without Job object

## Summary
After KubeVirt day-2 install, `umkim cluster discover lab-ha-0 --wait` returned the
previous `Succeeded` payload (`completed_at` still `2026-10-01T22:10:49Z`) without
running a new job. ManagedCluster still had `capability-discovery-phase=Succeeded`.
HTTP handler polls until Succeeded/Failed and does not require `completed_at` newer
than the `discover-capabilities` annotation timestamp.

Separately, FCJ gained `capability-discover-job=fleet-cluster-discover-lab-ha-0`
while the Job was missing (conflict/GC). Reconcile sat in Running until annotations
were cleared. CLI `--wait` through hub nginx hit **HTTP 504** (~60s) before discovery
finished.

## Workaround
Clear MC discovery annotations + FCJ `discover-capabilities` /
`capability-discover-job`, delete leftover Job/CM, re-POST discover, poll MC
annotations directly (avoid long nginx wait).

## Impact
`fleet.capability/kubevirt` and `network.multus_l2` stayed false; candidates empty
until rediscovery completed.
