# BUG / capacity: rocky-10-vm cpu:2 unschedulable on 2-vCPU lab-ha-0 nodes

## Summary
lab-ha-0 nodes have allocatable cpu=2. System pods already request ~0.4–0.7 cores.
virt-launcher for cpu:2 / memory:2Gi requests cpu:2 + memory:2328Mi →
VM status ErrorUnschedulable; WaitForFirstConsumer PVC never binds.

## Workaround for B9 smoke
Use cpu:1, memory:1Gi, disk.size:20Gi, storageClass: local-path.
