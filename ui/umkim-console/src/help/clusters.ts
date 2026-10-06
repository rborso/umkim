import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const CLUSTERS_FIELD_HELP: Record<string, FieldHelpContent> = {
  'clusterJoin.siteId': {
    description: 'Sets fleet.site label on the cluster.',
    example: 'homelab',
  },
  'clusterJoin.clusterName': {
    description: 'OCM ManagedCluster name (DNS label). Used across edge routing, kubeconfig paths, and policies.',
    example: 'virt02',
  },
  'clusterJoin.topology': {
    description:
      'single_node: one K3s server. ha_3_node: three control-plane nodes with cluster-init (optional workers). Storage backends differ by topology (Longhorn is HA-only; nfs_local is single-node only).',
    example: 'single_node',
    examples: ['single_node', 'ha_3_node'],
    learnMoreUrl: DOCS.createCluster,
  },
  'clusterJoin.controlNodeIp': {
    description: 'IP address of a control-plane node (management LAN). Must be reachable for bootstrap SSH.',
    example: '203.0.113.12',
  },
  'clusterJoin.workerNodeIp': {
    description:
      'Optional worker IP at cluster create time (ha_3_node only). Provisioned during initial bootstrap with the control plane. To add workers after Ready, use Cluster detail → Node operations.',
    example: '172.16.10.82',
  },
  'clusterJoin.bootstrapHost': {
    description: 'Primary node IP used for initial K3s bootstrap and cluster join token exchange.',
    example: '203.0.113.12',
  },
  'clusterJoin.kubevirt': {
    description:
      'Optional KubeVirt platform add-on after the cluster is Ready (Multus + CDI + KubeVirt). Helm charts still install with FleetApp.',
    example: 'enabled',
  },
  'clusterJoin.kubevirtBridgeName': {
    description:
      'Linux bridge device name on each node for Multus L2 KubeVirt VMs. Default mgmt-l2-br. If the node management IP already sits on a bridge, that bridge is adopted instead of creating a new one.',
    example: 'mgmt-l2-br',
  },
  'clusterJoin.kubevirtBridgeSlaveNic': {
    description:
      'Optional NIC override to enslave under the bridge. Leave empty to discover the interface that holds the node management IP (inventory address). If that IP already sits on a bridge, the bridge is adopted.',
    example: 'enp0s31f6',
  },
  'clusterJoin.kubevirtBridgeKeepIp': {
    description:
      'When enabled, moves the node management IP onto the bridge so SSH/mgmt stay reachable after enslaving the NIC.',
    example: 'true',
  },
  'clusterJoin.controlPlaneVip': {
    description:
      'Management LAN IPv4 VIP floated by kube-vip for the K3s API. Required for ha_3_node. Must not match any node IP.',
    example: '198.51.100.84',
  },
  'clusterJoin.longhornEnabled': {
    description:
      'Install Longhorn during bootstrap on control-plane nodes (ha_3_node only). Provides replicated RWO StorageClass longhorn; disables local-path as default when enabled. Requires a dedicated data disk per control node.',
    example: 'true',
    learnMoreUrl: DOCS.clusterStorage,
  },
  'clusterJoin.longhornDevice': {
    description:
      'Dedicated unformatted block device for Longhorn data on each control node. Virtio lab VMs often use /dev/vdb; bare metal may use /dev/sdb.',
    example: '/dev/vdb',
    examples: ['/dev/vdb', '/dev/sdb'],
  },
  'clusterJoin.providers': {
    description:
      'Optional KubeVirt platform add-on after the cluster is Ready. Helm charts install with FleetApp, not service providers.',
    example: 'kubevirt',
  },
  'clusterJoin.minRootDiskGi': {
    description: 'Minimum root disk size (GiB) required on nodes for cluster validation. Default is 40.',
    example: '40',
  },
  'clusterJoin.region': {
    description: 'Region label (fleet.topology/region) stored on the cluster for display.',
    example: 'homelab',
  },
  'clusterJoin.clusterLabels': {
    description: 'Custom fleet.* or fleet.custom/* labels applied to the ManagedCluster at Ready.',
    example: 'fleet.label/zone=homelab',
  },
  'clusterJoin.nodeLabels': {
    description: 'Kubernetes labels applied to this node during provision (kubectl label node).',
    example: 'fleet.label/zone=virt01-flat',
  },
  'clusterJoin.localPath': {
    description:
      'K3s built-in local-path StorageClass for ReadWriteOnce volumes. Default on single_node. Turned off as default when Longhorn is enabled on HA.',
    example: 'true',
    learnMoreUrl: DOCS.clusterStorage,
  },
  'clusterJoin.nfsLocal': {
    description:
      'Single-node only: install nfs-server on the cluster node, export /srv/fleet-nfs, and deploy StorageClass nfs-client for ReadWriteMany. Evaluation convenience — not a managed shared-storage service.',
    example: 'false',
    learnMoreUrl: DOCS.clusterStorage,
  },
  'clusterJoin.nfsEnabled': {
    description:
      'Install nfs-subdir-external-provisioner for ReadWriteMany on a shared external NFS export (nfs_external). Server and export must exist before enable. Mutually exclusive with Local RWX (nfs_local) on single_node.',
    example: 'false',
    learnMoreUrl: DOCS.clusterStorage,
  },
  'clusterJoin.nfsServer': {
    description: 'NFS server IP or hostname reachable from all cluster nodes.',
    example: '10.10.0.3',
    examples: ['198.51.100.40', '10.10.0.3'],
  },
  'clusterJoin.nfsExportBase': {
    description: 'Filesystem path exported on the NFS server (not the per-cluster subdirectory).',
    example: '/srv/fleet-nfs',
  },
  'clusterJoin.nfsMountPath': {
    description:
      'Path cluster nodes mount via NFS for the provisioner. Usually export base plus cluster subdirectory; may be an NFSv4 pseudo-path when the server uses fsid=0.',
    example: '/srv/fleet-nfs/virt02',
  },
  'clusterJoin.nfsClusterPath': {
    description: 'Subdirectory name under the export base for this cluster (created on the NFS server). Defaults to the cluster name.',
    example: 'virt02',
  },
  'clusterJoin.nfsDelegateHost': {
    description:
      'SSH-reachable host (IP or DNS) used to mkdir the cluster subdirectory on the NFS server when it is not a cluster node. Do not include a user@ prefix; SSH uses the ansible_user (linux).',
    example: '10.10.0.3',
  },
  'clusterJoin.domainBindings': {
    description: 'Site ingress row id (FleetSite.spec.edge.ingress[].id) this cluster binds to. Required for public site routes.',
    example: 'homelab-pf',
  },
  'clusterJoin.exposureMode': {
    description:
      'Legacy name — clusters no longer set exposure mode. Bind an ingressId; method and TLS come from the site ingress row.',
    example: 'port_forward',
    examples: ['port_forward', 'reverse_proxy'],
    learnMoreUrl: DOCS.createClusterEdge,
  },
  'clusterDetail.siteId': {
    description: 'fleet.site label on the ManagedCluster (site membership).',
    example: 'homelab',
  },
  'clusterDetail.ingressEnabled': {
    description:
      'Read-only status: whether spoke Traefik ingress is installed for this cluster (driven by ingress binding and provision, not a direct toggle).',
    example: 'true',
  },
  'clusterDetail.ingressAuth': {
    description:
      'Enable Authelia + LLDAP forward-auth on cluster Traefik ingress routes. Requires Ready, a port_forward ingress binding, and site zone.',
    example: 'true',
    learnMoreUrl: DOCS.ingressAuth,
  },
  'clusterDetail.clusterAcme': {
    description:
      'Install Let\'s Encrypt on this cluster\'s Traefik for site hostnames. Requires port_forward ingress (acme_cluster), cluster Ready, and hub fleet-edge ACME enabled.',
    example: 'true',
    learnMoreUrl: DOCS.fleetEdge,
  },
  'clusterDetail.region': {
    description: 'Cluster region label (fleet.topology/region) for display.',
    example: 'homelab',
  },
  'clusterDetail.jumphost': {
    description:
      'Optional SSH bastion override for this cluster (user@host:port). When empty, the parent site jumphost is used for hub Ansible jobs.',
    example: 'jump@bastion.example.com:22',
  },
  'clusterJoin.jumphost': {
    description: 'Optional cluster jumphost at create time. Leave empty to inherit the site default.',
    example: 'jump@bastion.example.com:22',
  },
  'clusterDetail.clusterLabels': {
    description:
      'OCM ManagedCluster fleet.* labels. Capability labels (fleet.capability/*) are discovered automatically and read-only here.',
    example: 'fleet.site=homelab',
    examples: ['fleet.custom/zone=lab-a', 'fleet.topology/region=homelab'],
  },
  'clusterDetail.nestDepth': {
    description: 'Topology nesting depth label (0 for top-level spokes).',
    example: '0',
  },
  'clusterDetail.provisioner': {
    description: 'Provisioner label for the cluster (for example k3s).',
    example: 'k3s',
  },
  'clusterDetail.exposureMode': {
    description:
      'Resolved ingress method for this cluster (from the bound site ingress row): port_forward or reverse_proxy.',
    example: 'port_forward',
    examples: ['port_forward', 'reverse_proxy'],
    learnMoreUrl: DOCS.fleetEdge,
  },
  'clusterDetail.tlsPolicy': {
    description:
      'TLS policy derived from the bound ingress method: acme_cluster for port_forward, external for reverse_proxy.',
    example: 'acme_cluster',
    examples: ['acme_cluster', 'external', 'acme_hub'],
    learnMoreUrl: DOCS.fleetEdge,
  },
  'clusterDetail.ingressEndpointId': {
    description: 'Legacy name — use site ingress id (ingress_id) instead.',
    example: 'homelab-pf',
  },
  'clusterDetail.domainBindings': {
    description: 'Site ingress row id this cluster is bound to (spec.edge.ingressId / summary ingress_id).',
    example: 'homelab-pf',
  },
  'clusterDetail.directIngress': {
    description:
      'True when the cluster is bound to a port_forward or reverse_proxy ingress (WAN can reach spoke Traefik).',
    example: 'true',
  },
  'clusterDetail.upstreamHost': {
    description: 'WireGuard or management LAN IP hub Traefik uses as the upstream for this cluster.',
    example: '10.10.0.10',
  },
  'clusterDetail.traefikPort': {
    description: 'HTTP NodePort on the cluster Traefik service (typically 30080).',
    example: '30080',
  },
  'clusterDetail.authPassword': {
    description: 'LLDAP admin password used when enabling ingress forward-auth on the cluster.',
    example: '(set a strong password)',
  },
  'clusterStorage.nfsServer': {
    description: 'NFS server reachable from cluster nodes. See NFS setup help for export prerequisites.',
    example: '10.10.0.3',
    learnMoreUrl: DOCS.clusterStorage,
  },
  'clusterStorage.nfsExportBase': {
    description: 'Server path listed in /etc/exports (parent of per-cluster subdirectories).',
    example: '/srv/fleet-nfs',
  },
  'clusterStorage.nfsMountPath': {
    description: 'Mount path on nodes used by nfs-subdir-external-provisioner.',
    example: '/srv/fleet-nfs/virt02',
  },
  'clusterStorage.nfsDelegateHost': {
    description:
      'SSH target (IP or DNS) to create the cluster subdirectory on the NFS server. No user@ prefix.',
    example: '198.51.100.210',
  },
  'clusterStorage.nfsClusterPath': {
    description: 'Subdirectory under export base dedicated to this cluster (read-only; defaults to cluster name).',
    example: 'virt02',
  },
  'clusterNodes.labelKey': {
    description: 'Kubernetes node label key (fleet.label/* or standard labels).',
    example: 'fleet.label/zone',
  },
  'clusterNodes.labelValue': {
    description: 'Label value applied to the selected node.',
    example: 'virt01-flat',
  },
  'clusterDetail.exposeLldapWebUI': {
    description: 'When enabled, exposes LLDAP web UI at lldap.<site-zone> behind Authelia forward-auth.',
    example: 'false',
  },
  'clusterDelete.cleanupKubernetes': {
    description:
      'Uninstall K3s and wipe local cluster state on all nodes. Leave unchecked to detach from the fleet while keeping Kubernetes and your application pods running (standalone K3s).',
    example: 'false',
  },
  'clusterOperations.addWorkerAddress': {
    description:
      'SSH-reachable IP or hostname of the new Rocky node on the management LAN. The hub Ansible job installs K3s agent and joins the cluster.',
    example: '198.51.100.60',
  },
  'clusterOperations.addWorkerHostname': {
    description: 'Kubernetes node name. Defaults to a generated hostname when empty.',
    example: 'virt02-w1',
  },
  'clusterOperations.removeWorker': {
    description: 'Worker node to drain and remove from K3s and fleet inventory.',
    example: 'virt02-w1',
  },
  'clusterOperations.removeWorkerForce': {
    description:
      'Skip drain and remove the node from inventory when it is unreachable. Use only when the host is permanently lost.',
    example: 'false',
  },
  'clusterOperations.replaceOldNode': {
    description: 'Failed or retiring control-plane node to replace (HA clusters).',
    example: 'virt02-cp2',
  },
  'clusterOperations.replaceNewAddress': {
    description: 'SSH-reachable IP of the replacement Rocky control-plane node.',
    example: '198.51.100.61',
  },
  'clusterOperations.replaceNewHostname': {
    description: 'Kubernetes node name for the replacement host. Optional when it matches inventory.',
    example: 'virt02-cp2',
  },
  'clusterOperations.replaceForce': {
    description:
      'Replace when the old control-plane node is unreachable (skips graceful leave).',
    example: 'false',
  },
};
