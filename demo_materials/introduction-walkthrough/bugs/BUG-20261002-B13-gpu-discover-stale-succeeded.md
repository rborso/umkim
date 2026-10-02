# BUG: cluster gpu discover returns stale Succeeded after GPU enable (lab-gpu-pt-0)

**Phase:** B13  
**Date:** 2026-10-02 (CEST)  
**Cluster:** lab-gpu-pt-0

## Symptom
After `umkim cluster gpu host-prep --force --wait` and `umkim cluster gpu enable --wait`, enable failed with:

```
fleet.capability/gpu is still false after discovery (device plugin may not be advertising nvidia.com/gpu)
```

Meanwhile on the spoke:
- `nvidia-device-plugin-daemonset` Running
- node capacity/allocatable `nvidia.com/gpu: 1`
- `nvidia-smi` OK (GTX 1650, driver 615.71.09)

`umkim cluster gpu discover` kept returning the **same** payload with
`completed_at: 2026-10-02T00:22:50Z` (before host-prep) and
`cluster_capabilities.gpu: false`, `gpu.total: 0`.

ManagedCluster annotations were stuck:
- `fleet.umkim.com/capability-discovery-phase: Succeeded`
- `fleet.umkim.com/capability-discovery-at: 2026-10-02T00:22:50Z`
- labels `fleet.capability/gpu=false` while metrics already had `fleet.resource/gpu.total=1`

Similar class to BUG-20261002-B9a-discover-stale-succeeded-no-job.

## Workaround
Clear discovery annotations on ManagedCluster, then re-run discover:

```bash
kubectl annotate managedcluster lab-gpu-pt-0 \
  fleet.umkim.com/capability-discovery-at- \
  fleet.umkim.com/capability-discovery-phase- \
  fleet.umkim.com/capability-discovery-message- \
  fleet.umkim.com/capability-discovery-cluster- \
  fleet.umkim.com/capability-discovery-nodes- \
  fleet.umkim.com/capability-discovery-resources-
umkim cluster gpu discover lab-gpu-pt-0
```

After clear: `gpu=true`, `gpu.total=1`, `clusterGpuCapability=true`.

## Expected
`umkim cluster gpu discover` / `gpu enable --wait` must start a **new** discovery job when runtime/device plugin state changes, not return a cached Succeeded result.
