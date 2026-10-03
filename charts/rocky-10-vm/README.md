# rocky-10-vm

Install a **Rocky Linux 10** guest on a Ready cluster that already has the
KubeVirt platform (Multus + CDI + KubeVirt). Use **Apps → Install chart** or
`FleetApp` with short chart name `rocky-10-vm`.

## Values

| Key | Default | Meaning |
|-----|---------|---------|
| `cpu` | `2` | vCPU cores |
| `memory` | `2Gi` | Guest memory |
| `disk.size` | `20Gi` | Root PVC size (must cover the cloud image) |
| `disk.storageClass` | `local-path` | CDI storage class |
| `disk.volumeMode` | `Filesystem` | Filesystem mode (avoids CDI importer issues with block volumes) |
| `image.url` | Rocky 10 GenericCloud Base | HTTP source for CDI import |
| `network.mode` | `pod` | `pod` (masquerade) or `lan` (Multus) |
| `network.staticIP` | `""` | Static IPv4 CIDR on LAN (e.g. `192.0.2.150/24`) |
| `network.gateway` | `""` | Default gateway when `staticIP` is set |
| `network.dns` | `[]` | DNS servers when `staticIP` is set |
| `network.interface` | `enp1s0` | Guest NIC name (virtio bridge on q35) |
| `cloudInit.user` / `password` | `rocky` / `CHANGE_ME` | Local console login |

Example values (pod network):

```yaml
cpu: 2
memory: 4Gi
disk:
  size: 40Gi
  storageClass: local-path
```

LAN (same L2 as the node — requires `network.multus_l2`):

```yaml
network:
  mode: lan
  multusNetwork: default/mgmt-l2
```

Static IPv4 on LAN (Rocky GenericCloud disables cloud-init `network:` v2; the chart
writes a NetworkManager keyfile instead):

```yaml
network:
  mode: lan
  multusNetwork: default/mgmt-l2
  staticIP: 192.0.2.150/24
  gateway: 192.0.2.1
  dns:
    - 192.0.2.1
```

Route mode should stay `none` (no Service/ingress for the guest).
