# BUG / env gap: rocky-10-vm default storageClass longhorn missing on lab-ha-0

## Summary
B9b/B9c FleetApps created with `disk.storageClass: longhorn` (chart/strategy default).
Spoke has only `local-path` (default). Events:
`ProvisioningFailed: storageclass.storage.k8s.io "longhorn" not found`.
No `longhorn-system` namespace. Cluster still advertises `ha_storage: true` /
`storage.rwo: true` (labels), which is misleading for chart placement.

## Impact
DataVolumes/PVCs Pending; VMs stuck Provisioning.

## Workaround
Reinstall apps with `disk.storageClass: local-path` (node-local; not HA), or
install Longhorn before rocky-10-vm.

## Assets
`B9b-pvc-pending-diag.txt`, `B9b-storage-survey.txt`
