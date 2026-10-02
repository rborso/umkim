# BUG: enable-kubevirt Ansible inventory stale (includes removed worker, omits CP)

## Summary
Day-2 `umkim cluster platform enable-kubevirt lab-ha-0 --lan-bridge` used
ConfigMap `fleet-console/fleet-cluster-lab-ha-0-inventory` with:
- `lab-ha-0-w1` still in `[fleet_cluster_workers]` (VM Ready in site host list but
  **not** a Kubernetes node after B8 worker add/remove storms)
- `lab-ha-0-cp3` **missing** from `[fleet_cluster_control]` despite being Ready in
  the cluster (cp1, cp2, cp4 only)

## Impact
1. Host-bridge play failed on `lab-ha-0-w1` (`install … /opt/cni/bin/bridge`: No such
   file or directory — role does not `mkdir -p /opt/cni/bin`). Ansible exit 2 → Job
   backoff limit → platform phase **Failed**, even though Multus/CDI/KubeVirt and NAD
   `default/mgmt-l2` were successfully applied on cp1.
2. Calico day-2 `IP_AUTODETECTION_METHOD=interface=n0p0_mgmt_nw` applied cluster-wide
   while cp3 had **no** bridge → `calico-node` CrashLoopBackOff on cp3 until bridge
   was configured manually.

## Expected
Inventory for platform jobs should match **current Ready Kubernetes nodes** (or at
least all control-plane members), not leftover guest VMs. Stale workers must be
dropped after failed addWorker / removeWorker. replaceControlPlane must keep both
old and new CPs in inventory when both remain Ready.

## Workaround
- Manual bridge + CNI on cp3 via ProxyJump; mkdir/install bridge CNI on w1
- Patch inventory CM to add cp3 / remove w1; re-run `enable-kubevirt --force`

## Assets
`B9a-inventory-cm-before.yaml`, `B9a-job-pod1-tail.txt`, `B9a-cp3-bridge-manual-fix.txt`
