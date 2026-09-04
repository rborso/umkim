# Fleet KubeVirt platform

> Status: v0.2 experimental. Optional spoke platform add-on (not a FleetApp).

Install Multus, CDI, and KubeVirt on a Ready workload cluster. Optionally attach
VMs to the node LAN with a Multus bridge NAD (`mgmt-l2`).

## Modes

| Mode | Behavior |
| --- | --- |
| Pod network (default) | Multus + CDI + KubeVirt; VMs use masquerade / pod network |
| LAN L2 | Also applies NAD `mgmt-l2` against a host Linux bridge (default `n0p0_mgmt_nw`). Prefer creating the bridge at bootstrap (`spec.providers.kubevirtBridge.enabled`) |

NAD IPAM is empty — LAN addresses come from site DHCP/static, not Kubernetes.

## Operator path

### Create cluster

Console **Create cluster**: enable **KubeVirt** (`providers.install.kubevirt`),
optionally **LAN Multus bridge** (host bridge at bootstrap + NAD after install).

After Ready, the controller auto-queues the same install job when
`providers.install.kubevirt` is set and the provider phase is still
`NotInstalled`.

### After Ready

Console **Operations → Enable KubeVirt**, or:

```bash
umkim cluster platform enable-kubevirt CLUSTER
umkim cluster platform enable-kubevirt CLUSTER --lan-bridge [--bridge n0p0_mgmt_nw]
umkim cluster platform enable-kubevirt CLUSTER --force --wait 45m
umkim cluster platform status CLUSTER
umkim cluster candidates --require kubevirt
```

API (via fleet-console-auth): `POST /clusters/{name}/platform/kubevirt`,
`GET …/platform/kubevirt`.

Jobs are mutually exclusive with node ops and storage configure. History type:
`configureKubevirt` (on `FleetClusterJoin.status.nodeOperationHistory`).

k8s/helm `providers.install` flags and former service-provider APIs are not used
(`410 Gone`); install Helm charts with FleetApp.

## Capabilities

Discovery sets:

| Label | Meaning |
| --- | --- |
| `fleet.capability/kvm` | `/dev/kvm` present |
| `fleet.capability/kubevirt` | KubeVirt CR Available |
| `fleet.capability/network.multus_l2` | NAD `default/mgmt-l2` present |

## Implementation notes

- Playbook: `cluster-configure-kubevirt.yml` →
  `roles/fleet_site/tasks/push_site_kubevirt_platform.yml`
- Roles: `kubevirt_stack`, `kubevirt_bridge_nad`
- Pins: `roles/kubevirt_stack/defaults/main.yml`
- Runner image must COPY `cluster-configure-kubevirt.yml` (see
  `images/fleet-ansible-runner/Dockerfile`); rebuild/push
  `fleet-ansible-runner` after playbook changes
- Site inventory `fleet_site_enable_kubevirt` can also install during
  `site-provision.yml`; day-2 path is the platform job above
- Legacy inventory playbooks (`fleet-platform-kubevirt.yml`,
  `kubevirt-bridge-ready.yml`, `phase4-kubevirt-parents.yml`) are superseded
  by the platform API/CLI for managed clusters

## Sample VM (pod network)

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: demo
spec:
  runStrategy: Always
  template:
    spec:
      domain:
        devices:
          disks:
          - name: disk0
            disk: { bus: virtio }
          interfaces:
          - name: default
            masquerade: {}
        resources:
          requests: { memory: 128Mi }
      networks:
      - name: default
        pod: {}
      volumes:
      - name: disk0
        containerDisk:
          image: quay.io/kubevirt/cirros-container-disk-demo:latest
```

## Sample VM (LAN Multus)

Requires `fleet.capability/network.multus_l2` (NAD `default/mgmt-l2`) and a
host bridge on the node:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: demo-lan
spec:
  runStrategy: Always
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: default/mgmt-l2
    spec:
      domain:
        devices:
          disks:
          - name: disk0
            disk: { bus: virtio }
          interfaces:
          - name: lan
            bridge: {}
        resources:
          requests: { memory: 128Mi }
      networks:
      - name: lan
        multus:
          networkName: default/mgmt-l2
      volumes:
      - name: disk0
        containerDisk:
          image: quay.io/kubevirt/cirros-container-disk-demo:latest
```

## Related

- [fleet-site.md](fleet-site.md)
- [umkim-cli.md](umkim-cli.md)
- [gpu-host-prep.md](gpu-host-prep.md) for nested/GPU hosts
