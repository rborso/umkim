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
| `disk.storageClass` | `longhorn` | CDI storage class |
| `disk.volumeMode` | `Filesystem` | Prefer Filesystem with Longhorn (Block importer fails without privileged access) |
| `image.url` | Rocky 10 GenericCloud Base | HTTP source for CDI import |
| `network.mode` | `pod` | `pod` (masquerade) or `lan` (Multus) |
| `cloudInit.user` / `password` | `rocky` / `rocky` | Local console login |

Example values (pod network):

```yaml
cpu: 2
memory: 4Gi
disk:
  size: 40Gi
  storageClass: longhorn
```

LAN (same L2 as the node — requires `network.multus_l2`):

```yaml
network:
  mode: lan
  multusNetwork: default/mgmt-l2
```

Route mode should stay `none` (no Service/ingress for the guest).
