# Fleet KubeVirt platform

> Status: v0.2 experimental. Optional spoke platform add-on (not a FleetApp).

Install Multus, CDI, and KubeVirt on a Ready workload cluster. Optionally attach
VMs to the node LAN with a Multus bridge NAD (`mgmt-l2`).

## Modes

| Mode | Behavior |
| --- | --- |
| Pod network (default) | Multus + CDI + KubeVirt; VMs use masquerade / pod network |
| LAN L2 | Host Linux bridge on every node + NAD `mgmt-l2` (default bridge `n0p0_mgmt_nw`). Prefer creating the bridge at bootstrap (`spec.providers.kubevirtBridge.enabled`); day-2 enable is supported |

NAD IPAM is empty — LAN addresses come from site DHCP/static, not Kubernetes.

## Operator path

### Create cluster

Console **Create cluster**: enable **KubeVirt** (`providers.install.kubevirt`),
optionally **LAN Multus bridge** (host bridge at bootstrap + NAD after install).

After Ready, the controller auto-queues the same install job when
`providers.install.kubevirt` is set and the provider phase is still
`NotInstalled`.

### After Ready

Console **Operations → Enable KubeVirt** / **Enable LAN Multus**, or:

```bash
# KubeVirt only (pod-network VMs)
umkim cluster platform enable-kubevirt CLUSTER

# Install KubeVirt with LAN Multus, or add LAN Multus when KubeVirt is already Ready
umkim cluster platform enable-kubevirt CLUSTER --lan-bridge [--bridge n0p0_mgmt_nw]
umkim cluster platform enable-kubevirt CLUSTER --lan-bridge --wait 45m

# Ansible flow (same API)
ansible-playbook $INV playbooks/flows/cluster-enable-kubevirt-lan.yml -l hub \
  -e fleet_flow_cluster_name=CLUSTER -e fleet_flow_lan_bridge=true

umkim cluster platform status CLUSTER
umkim cluster candidates --require network.multus_l2
```

Day-2 `--lan-bridge` on an already-Ready KubeVirt cluster configures the host
bridge on **all nodes**, installs the bridge CNI plugin, applies NAD
`default/mgmt-l2`, and re-runs capability discovery (`network.multus_l2`).
`--force` is only needed to re-run when LAN Multus is already enabled.

Jobs are mutually exclusive with node ops and storage configure. History type:
`configureKubevirt` (on `FleetClusterJoin.status.nodeOperationHistory`).

API: `POST /clusters/{name}/platform/kubevirt`, `GET …/platform/kubevirt`.

k8s/helm `providers.install` flags and former service-provider APIs are not used
(`410 Gone`); install Helm charts with FleetApp.

## Virtual machine management

After KubeVirt is Ready on a cluster, operators manage guest VMs from the hub
(no direct spoke kubeconfig required):

| Surface | Path |
| --- | --- |
| Console | **Clusters → cluster → Virtual machines** (`/clusters/{name}/vms`) |
| CLI | `umkim vm …` |
| API | `/clusters/{name}/vms…` |

Default console is **serial**. **VNC** opens on demand (websocket only while the
modal/session is open). Power actions: soft/hard power off, soft/hard reboot,
and power on.

```bash
umkim vm list --cluster CLUSTER
umkim vm get NAME --cluster CLUSTER [-n NAMESPACE]
umkim vm start|stop|restart NAME --cluster CLUSTER [--force]
umkim vm console NAME --cluster CLUSTER            # serial (TTY)
umkim vm console NAME --cluster CLUSTER --type vnc # binary VNC stream
```

API:

| Method | Path |
| --- | --- |
| `GET` | `/clusters/{name}/vms` |
| `GET` | `/clusters/{name}/vms/{namespace}/{vm}` |
| `POST` | `/clusters/{name}/vms/{namespace}/{vm}/power` (`action=start\|stop\|restart`, optional `force`) |
| `GET` (WebSocket) | `/clusters/{name}/vms/{namespace}/{vm}/console` (serial) |
| `GET` (WebSocket) | `/clusters/{name}/vms/{namespace}/{vm}/vnc` |

## Capabilities

Discovery sets:

| Label | Meaning |
| --- | --- |
| `fleet.capability/kvm` | `/dev/kvm` present |
| `fleet.capability/kubevirt` | KubeVirt CR Available |
| `fleet.capability/network.multus_l2` | NAD `default/mgmt-l2` present |

## Implementation notes

- Playbook: `cluster-configure-kubevirt.yml` imports `kubevirt_host_bridge` on
  all nodes (when LAN bridge is requested), then
  `roles/fleet_site/tasks/push_site_kubevirt_platform.yml`
- Roles: `kubevirt_host_bridge`, `kubevirt_stack`, `kubevirt_bridge_nad`
- Pins: `roles/kubevirt_stack/defaults/main.yml`
- Runner image must COPY `cluster-configure-kubevirt.yml` (see
  `images/fleet-ansible-runner/Dockerfile`); rebuild/push
  `fleet-ansible-runner` after playbook changes
- Site inventory `fleet_site_enable_kubevirt` can also install during
  `site-provision.yml`; day-2 path is the platform job above
- KubeVirt on managed clusters is installed via platform API/CLI (`umkim cluster platform enable-kubevirt`)

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

## Rocky Linux 10 via Apps (`rocky-10-vm`)

Prefer the seeded Helm chart **`rocky-10-vm`** from **Apps → Install chart**
(or `FleetApp` with short chart name `rocky-10-vm`). Configure `cpu`, `memory`,
and `disk.size` in the install dialog / values. Requires KubeVirt + CDI and a
storage class suitable for CDI imports (default `longhorn`). Guest management
after Ready uses the cluster **Virtual machines** page / `umkim vm`.
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
