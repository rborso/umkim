import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const CLUSTERS_FIELD_HELP: Record<string, FieldHelpContent> = {
  'clusterJoin.siteId': {
    description: 'Fleet site the cluster belongs to. Sets fleet.site label used by placement policies.',
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
    description: 'IP address of a worker node joined after the control plane is ready (ha_3_node only).',
    example: '172.16.10.82',
  },
  'clusterJoin.bootstrapHost': {
    description: 'Primary node IP used for initial K3s bootstrap and cluster join token exchange.',
    example: '203.0.113.12',
  },
  'clusterJoin.kubevirtBridgeName': {
    description:
      'Linux bridge device name on each node for Multus L2 KubeVirt VMs. Created during bootstrap when Configure host bridge is enabled.',
    example: 'n0p0_mgmt_nw',
  },
  'clusterJoin.kubevirtBridgeSlaveNic': {
    description:
      'Optional NIC override to enslave under the bridge. Leave empty to discover the interface that holds the node management IP (inventory address). Bootstrap fails if that IP is already on a bridge.',
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
    description: 'DCM service providers auto-installed after the cluster reaches Ready (k8s, helm, kubevirt).',
    example: 'k8s, helm',
  },
  'clusterJoin.minRootDiskGi': {
    description: 'Minimum root disk size (GiB) required on nodes for cluster validation. Default is 40.',
    example: '40',
  },
  'clusterJoin.region': {
    description: 'Region label (fleet.topology/region) stored on the cluster for placement and display.',
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
  'clusterJoin.nfsEnabled': {
    description:
      'Install nfs-subdir-external-provisioner for ReadWriteMany volumes on a shared NFS export (nfs_external). Server and export must exist before enable. For single-node RWX without an external server, use configure-storage with nfs_local after Ready (not in this create form).',
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
    description: 'Domain zone IDs from the site registry this cluster serves. Select one or more when the site has multiple zones.',
    example: 'virt02',
  },
  'clusterJoin.exposureMode': {
    description:
      'Override site edge exposure, or inherit. Working pairs: none/hub_gateway→acme_hub, port_forward→acme_cluster, reverse_proxy→external. Site catalog routing needs port_forward or reverse_proxy.',
    example: 'inherit',
    examples: ['inherit', 'none', 'port_forward', 'reverse_proxy', 'hub_gateway'],
    learnMoreUrl: DOCS.createClusterEdge,
  },
  'clusterDetail.siteId': {
    description: 'fleet.site label on the ManagedCluster (site membership for placement).',
    example: 'homelab',
  },
  'clusterDetail.org': {
    description:
      'dc-admin only. Assigns fleet.org on the ManagedCluster for dedicated org placement. Empty clears the label.',
    example: 'acme',
  },
  'clusterDetail.pool': {
    description: 'dc-admin only. shared = platform pool; dedicated = org-owned or unassigned dedicated capacity.',
    example: 'shared',
  },
  'clusterDetail.ingressEnabled': {
    description:
      'Read-only status: whether spoke Traefik ingress is installed for this cluster (driven by exposure mode and provision, not a direct toggle).',
    example: 'true',
  },
  'clusterDetail.ingressAuth': {
    description:
      'Enable Authelia + LLDAP forward-auth on cluster Traefik ingress routes. Requires Ready, ingress (port_forward or reverse_proxy), and site domain bindings.',
    example: 'true',
    learnMoreUrl: DOCS.ingressAuth,
  },
  'clusterDetail.clusterAcme': {
    description:
      'Install Let\'s Encrypt on this cluster\'s Traefik for site hostnames. Requires tls_policy acme_cluster, direct ingress, cluster Ready, and hub fleet-edge ACME enabled.',
    example: 'true',
    learnMoreUrl: DOCS.fleetEdge,
  },
  'clusterDetail.region': {
    description: 'Cluster region label (fleet.topology/region) for policies and display.',
    example: 'homelab',
  },
  'clusterDetail.clusterLabels': {
    description:
      'OCM ManagedCluster fleet.* labels used by placement policies. Capability labels (fleet.capability/*) are discovered automatically and read-only here.',
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
      'Cluster edge exposure. Pair with TLS: none/hub_gateway→acme_hub, port_forward→acme_cluster, reverse_proxy→external. Other mixes break WAN TLS or ACME.',
    example: 'port_forward',
    examples: ['inherit', 'none', 'port_forward', 'reverse_proxy', 'hub_gateway'],
    learnMoreUrl: DOCS.exposureTlsPairs,
  },
  'clusterDetail.tlsPolicy': {
    description:
      'Where certificates live. Working pairs: acme_hub (none/hub_gateway), acme_cluster (port_forward), external (reverse_proxy). inherit follows exposure defaults. passthrough is offered but not a tested profile.',
    example: 'acme_cluster',
    examples: ['inherit', 'acme_hub', 'acme_cluster', 'external', 'passthrough'],
    learnMoreUrl: DOCS.exposureTlsPairs,
  },
  'clusterDetail.ingressEndpointId': {
    description: 'Site external endpoint ID used for WAN port forwards (from site edge config).',
    example: 'openwrt-wan',
  },
  'clusterDetail.domainBindings': {
    description: 'Domain zone IDs from the site registry this cluster publishes routes for.',
    example: 'virt02',
  },
  'clusterDetail.directIngress': {
    description:
      'Legacy flag: route hub edge directly to the cluster NodePort without a site proxy. Also set automatically when exposure is port_forward or reverse_proxy.',
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
};
