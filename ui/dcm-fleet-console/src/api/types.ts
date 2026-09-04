export type RoutingMode = 'hub' | 'site' | 'none';

export type FleetSitePhase =
  | 'Draft'
  | 'ScriptIssued'
  | 'Enrolled'
  | 'Validated'
  | 'Provisioning'
  | 'Ready'
  | 'Failed';

export type FleetIngressMethod = 'port_forward' | 'reverse_proxy';
export type FleetTlsPolicy = 'inherit' | 'external' | 'acme_cluster' | 'acme_hub' | 'passthrough';

export interface FleetSiteExternalAccess {
  enabled?: boolean;
  dnsTargetNote?: string;
  expectedWanIp?: string;
}

export interface FleetSiteWanEgress {
  enabled?: boolean;
  dnsTargetNote?: string;
  expectedWanIp?: string;
}

export interface FleetSiteIngress {
  id: string;
  method: FleetIngressMethod;
  address?: string;
  zone?: string;
  wildcard?: string;
  description?: string;
  operatorNotes?: string;
  assignedClusters?: string[];
}

export interface FleetSiteEdge {
  hubGateway?: boolean;
  hubZone?: string;
  hubWildcard?: string;
  ingress?: FleetSiteIngress[];
  wanEgress?: FleetSiteWanEgress;
  externalAccess?: FleetSiteExternalAccess;
}

export interface FleetSiteSpec {
  siteName: string;
  siteId?: string;
  provisioningMode?: 'greenfield' | 'registry';
  displayName?: string;
  region?: string;
  failureDomain?: string;
  jumphost?: string;
  edgeDomain?: string;
  edge?: FleetSiteEdge;
  bootstrapHost?: string;
  mgmtLanIp?: string;
  rockyVersion?: string;
  clusterRegion?: string;
  minRootDiskGi?: number;
  clusterCapabilities?: Record<string, boolean>;
  gpu?: boolean;
  storage?: {
    backend?: 'local_path' | 'nfs_external';
    minDataDiskGi?: number;
    rwo?: boolean;
    rwx?: boolean;
    nfs?: {
      server?: string;
      exportBase?: string;
      clusterPath?: string;
      storageClass?: string;
      delegateHost?: string;
      mountPath?: string;
    };
  };
}

export interface FleetSiteStatus {
  phase?: FleetSitePhase;
  message?: string;
  wireguardAddress?: string;
  wireguardPublicKey?: string;
  enrolledAt?: string;
  enrolledHostname?: string;
  validationErrors?: string[];
  validateJobName?: string;
  provisionJobName?: string;
  managedClusterAvailable?: boolean;
}

export interface FleetSite {
  metadata?: {
    name?: string;
    namespace?: string;
    creationTimestamp?: string;
    annotations?: Record<string, string>;
  };
  spec: FleetSiteSpec;
  status?: FleetSiteStatus;
  clusters?: string[];
  clusterCount?: number;
}

export interface FleetSitePatch {
  displayName?: string;
  siteId?: string;
  region?: string;
  failureDomain?: string;
  jumphost?: string;
  hubGateway?: boolean;
  hubZone?: string;
  hubWildcard?: string;
  ingress?: FleetSiteIngress[];
  wanEgress?: FleetSiteWanEgress;
  externalAccess?: FleetSiteExternalAccess;
  provisioningMode?: 'greenfield' | 'registry';
}

export type FleetClusterStorageBackend = 'local_path' | 'nfs_external' | 'nfs_local' | 'longhorn';

export interface FleetClusterNFSSpec {
  enabled?: boolean;
  server?: string;
  exportBase?: string;
  clusterPath?: string;
  storageClass?: string;
  delegateHost?: string;
  mountPath?: string;
}

export interface FleetClusterStorageSpec {
  localPath?: { enabled?: boolean };
  backend?: FleetClusterStorageBackend;
  minDataDiskGi?: number;
  rwo?: boolean;
  rwx?: boolean;
  nfs?: FleetClusterNFSSpec;
  longhorn?: FleetClusterLonghornSpec;
}

export interface FleetClusterLonghornSpec {
  enabled?: boolean;
  device?: string;
  storageClass?: string;
}

export interface FleetClusterStorageStatus {
  phase?: 'Pending' | 'Applying' | 'Applied' | 'Failed';
  jobName?: string;
  message?: string;
}

export interface FleetClusterProviderStatus {
  phase?: string;
  jobName?: string;
  message?: string;
  providerName?: string;
}

export interface FleetClusterConfigureStorage extends FleetClusterStorageSpec {
  force?: boolean;
}

export interface FleetClusterStorageValidateCheck {
  name: string;
  ok: boolean;
  message: string;
}

export interface FleetClusterStorageValidateResult {
  ok: boolean;
  checks: FleetClusterStorageValidateCheck[];
  nfsPath?: string;
  mountPath?: string;
  node?: string;
}

export type FleetClusterPhase =
  | 'Pending'
  | 'ScriptIssued'
  | 'Enrolled'
  | 'Validated'
  | 'Provisioning'
  | 'Ready'
  | 'Failed'
  | 'Deleting';

export type FleetAppPhase = 'Pending' | 'Installing' | 'Ready' | 'Failed' | 'Deleting' | '';

export type FleetAppRouteMode = 'none' | 'hub' | 'site';

export interface FleetAppRoute {
  mode?: FleetAppRouteMode;
  hubHost?: string;
  siteHost?: string;
  requireAuth?: boolean;
  serviceName?: string;
  servicePort?: number;
  serviceNamespace?: string;
  healthPath?: string;
}

export interface FleetApp {
  name: string;
  clusterName: string;
  chart: string;
  version?: string;
  releaseName: string;
  namespace: string;
  valuesYaml?: string;
  route?: FleetAppRoute;
  phase?: FleetAppPhase;
  message?: string;
  routeName?: string;
}

export interface FleetAppCreate {
  name?: string;
  clusterName: string;
  chart: string;
  version?: string;
  releaseName?: string;
  namespace?: string;
  values?: Record<string, unknown>;
  valuesYaml?: string;
  route?: FleetAppRoute;
}

export interface FleetAppDefaults {
  chartPrefix: string;
  hubEdgeDomain?: string;
}

export type FleetChartPhase = 'Pending' | 'Fetching' | 'Publishing' | 'Ready' | 'Failed' | '';

export type FleetChartSource = 'oci' | 'http' | 'upload' | 'harbor' | '';

export type FleetChartImagePolicy = 'external' | 'proxy' | 'copy' | '';

export interface FleetChartCopiedImage {
  from: string;
  to: string;
}

export interface FleetChart {
  name: string;
  source: FleetChartSource | string;
  origin?: string;
  version?: string;
  imagePolicy?: FleetChartImagePolicy | string;
  chart?: string;
  digest?: string;
  versions?: string[];
  phase?: FleetChartPhase;
  message?: string;
  imagesCopied?: FleetChartCopiedImage[];
}

export interface FleetChartCreate {
  name?: string;
  origin?: string;
  version?: string;
  imagePolicy?: FleetChartImagePolicy | string;
  source?: FleetChartSource | string;
  valuesYaml?: string;
}

export type ClusterNodeOperationType = 'addWorker' | 'removeWorker' | 'replaceControlPlane';
export type ClusterNodeOperationPhase = 'Pending' | 'Running' | 'Succeeded' | 'Failed';

export interface ClusterNodeOperation {
  id?: string;
  type?: ClusterNodeOperationType;
  node?: string;
  phase?: ClusterNodeOperationPhase;
  jobName?: string;
  message?: string;
  force?: boolean;
  actorKind?: string;
  actorName?: string;
}

export interface ClusterNodeOperationHistoryItem {
  id: string;
  type?: ClusterNodeOperationType;
  node?: string;
  phase?: ClusterNodeOperationPhase;
  jobName?: string;
  message?: string;
  force?: boolean;
  actorKind?: string;
  actorName?: string;
  startedAt?: string;
  finishedAt?: string;
  hasLogs?: boolean;
}

export interface ClusterNodeOperationLogs {
  id: string;
  jobName?: string;
  logs: string;
  source?: string;
}

export type NodeOperationValidatePhase = 'pre' | 'post';
export type NodeOperationCheckStatus = 'pass' | 'fail' | 'warn';

export interface NodeOperationValidateCheck {
  id: string;
  status: NodeOperationCheckStatus;
  message?: string;
}

export interface NodeOperationValidateResult {
  ok: boolean;
  phase: NodeOperationValidatePhase;
  type?: ClusterNodeOperationType;
  checks: NodeOperationValidateCheck[];
}

export interface FleetClusterAddWorkerBody {
  address: string;
  hostname?: string;
  role?: 'worker';
  labels?: Record<string, string>;
}

export interface FleetClusterReplaceNewNode {
  address: string;
  hostname?: string;
  labels?: Record<string, string>;
}

export interface FleetClusterReplaceControlPlaneBody {
  oldNode: string;
  newNode: FleetClusterReplaceNewNode;
  force?: boolean;
}

export interface FleetCluster {
  name: string;
  site?: string;
  available?: boolean;
  management?: boolean;
  read_only?: boolean;
  provisioning_phase?: FleetClusterPhase;
  capabilities?: Record<string, boolean>;
  topology?: Record<string, string>;
  ingress_enabled?: boolean;
  direct_ingress?: boolean;
  ingress_id?: string;
  method?: FleetIngressMethod;
  tls_policy?: FleetTlsPolicy;
  dns_zone?: string;
  edge_wan_ip?: string;
  upstream_host?: string;
  traefik_http_port?: number;
  jumphost?: string;
  console_managed?: boolean;
  provisioning_mode?: 'bundle' | 'hub_job';
  managed_cluster_available?: boolean;
  capability_discovery?: CapabilityDiscovery;
  storage?: FleetClusterStorageSpec;
  storageStatus?: FleetClusterStorageStatus;
  providers?: Record<string, FleetClusterProviderStatus>;
  ingress_auth_enabled?: boolean;
  ingressAuthStatus?: FleetClusterIngressAuthStatus;
  acme_enabled?: boolean;
  acmeStatus?: FleetClusterAcmeStatus;
  nodeOperation?: ClusterNodeOperation;
  labels?: Record<string, string>;
}

export interface FleetClusterAcmeStatus {
  phase?: string;
  jobName?: string;
  message?: string;
}

export interface FleetClusterIngressAuthStatus {
  phase?: string;
  jobName?: string;
  message?: string;
  secretsRef?: string;
  portalHost?: string;
  lldapWebUIHost?: string;
  accessDomains?: string[];
}

export interface CapabilityDiscoveryNode {
  name: string;
  gpu_count?: number;
  capabilities?: Record<string, boolean>;
}

export interface CapabilityDiscovery {
  phase?: 'Idle' | 'Running' | 'Succeeded' | 'Failed';
  message?: string;
  completed_at?: string;
  nodes?: CapabilityDiscoveryNode[];
  cluster_capabilities?: Record<string, boolean>;
  cluster_resources?: Record<string, number>;
}

export interface FleetClusterPatch {
  siteId?: string;
  jumphost?: string;
  capabilities?: Record<string, boolean>;
  topology?: Record<string, string>;
  edge?: {
    ingress_id?: string;
    upstream_host?: string;
    traefik_http_port?: number;
  };
  customLabels?: Record<string, string>;
  ingressAuth?: {
    enabled?: boolean;
  };
}

export interface FleetIngressAuth {
  hubEnabled: boolean;
  hubExposeLldapWebUI?: boolean;
  consoleLldapDeployed?: boolean;
  portalHost?: string;
  cookieDomain?: string;
  accessDomains?: string[];
  phase?: string;
  message?: string;
  hub?: {
    phase?: string;
    message?: string;
    autheliaServiceURL?: string;
    lldapWebUIEnabled?: boolean;
    lldapWebUIURL?: string;
    lldapWebUIPhase?: string;
    lldapWebUIMessage?: string;
  };
  clusters?: { name: string; phase?: string; message?: string }[];
  lastJobName?: string;
}

export interface FleetIngressAuthPatch {
  hubEnabled?: boolean;
  hubExposeLldapWebUI?: boolean;
  portalHost?: string;
  cookieDomain?: string;
  accessDomains?: string[];
}

export interface FleetAcme {
  hubEnabled: boolean;
  staging?: boolean;
  email?: string;
  phase?: string;
  message?: string;
  hub?: { phase?: string; message?: string };
  clusters?: { name: string; phase?: string; message?: string }[];
  lastJobName?: string;
}

export interface FleetAcmePatch {
  hubEnabled?: boolean;
  staging?: boolean;
  email?: string;
}

export interface FleetLLMBackend {
  model?: string;
  apiBase?: string;
}

export interface FleetLLM {
  hubEnabled: boolean;
  publicHost?: string;
  phase?: string;
  message?: string;
  hub?: { phase?: string; message?: string; endpoint?: string };
  lastJobName?: string;
  backends?: FleetLLMBackend[];
}

export interface FleetLLMPatch {
  hubEnabled?: boolean;
  publicHost?: string;
  backends?: FleetLLMBackend[];
}

export interface HubInfrastructureService {
  name: string;
  description?: string;
}

export interface HubInfrastructure {
  host_name: string;
  management_site_id: string;
  wireguard_ip?: string;
  public_endpoint?: string;
  hub_gateway?: boolean;
  hub_zone?: string;
  dns_target_note?: string;
  services?: HubInfrastructureService[];
  nodes?: FleetNode[];
  ingress_auth?: {
    enabled?: boolean;
    phase?: string;
    portalHost?: string;
    hubEnabled?: boolean;
  };
}

export interface FleetNodeAddress {
  type: string;
  address: string;
}

export interface FleetNodeResources {
  cpu?: string;
  memory?: string;
  ephemeral_storage?: string;
  pods?: string;
}

export interface FleetNodeCondition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
}

export interface FleetNode {
  name: string;
  roles?: string[];
  ready?: boolean;
  labels?: Record<string, string>;
  addresses?: FleetNodeAddress[];
  capacity?: FleetNodeResources;
  allocatable?: FleetNodeResources;
  usage?: FleetNodeResources;
  conditions?: FleetNodeCondition[];
  kubelet_version?: string;
  os_image?: string;
}

export interface FleetNodePatch {
  labels?: Record<string, string>;
  labelsToRemove?: string[];
}

export interface FleetSiteCreate {
  siteName: string;
  siteId?: string;
  displayName?: string;
  region?: string;
  failureDomain?: string;
  jumphost?: string;
  hubGateway?: boolean;
  hubZone?: string;
  hubWildcard?: string;
  wanEgress?: FleetSiteWanEgress;
  externalAccess?: FleetSiteExternalAccess;
  edge?: FleetSiteEdge;
}

export type FleetClusterTopology = 'single_node' | 'ha_3_node';
export type FleetClusterNetworkMode = 'wireguard' | 'lan';

export type FleetClusterNodeRole = 'control' | 'worker';

export interface FleetClusterNodeInput {
  hostname?: string;
  address: string;
  role?: FleetClusterNodeRole;
  gpu?: boolean;
  labels?: Record<string, string>;
}

export interface FleetClusterCreate {
  clusterName: string;
  siteId: string;
  bootstrapHost: string;
  jumphost?: string;
  minRootDiskGi?: number;
  clusterRegion?: string;
  clusterLabels?: Record<string, string>;
  gpu?: boolean;
  kvm?: boolean;
  clusterCapabilities?: Record<string, boolean>;
  storage?: FleetClusterStorageSpec;
  topology?: FleetClusterTopology;
  networkMode?: FleetClusterNetworkMode;
  hubLanApi?: string;
  controlPlaneVip?: string;
  nodes?: FleetClusterNodeInput[];
  edge?: {
    ingressId?: string;
  };
  /** Optional KubeVirt platform add-on (not a chart install path). */
  providers?: {
    kubevirt?: boolean;
    kubevirtBridge?: {
      enabled?: boolean;
      bridgeName?: string;
      slaveNic?: string;
      keepIp?: boolean;
    };
  };
}

