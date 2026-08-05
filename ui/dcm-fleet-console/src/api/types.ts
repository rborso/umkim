export interface Provider {
  id?: string;
  name?: string;
  display_name?: string;
  displayName?: string;
  service_type?: string;
  serviceType?: string;
  health_status?: string;
  endpoint?: string;
  operations?: string[];
  schema_version?: string;
  schemaVersion?: string;
  metadata?: Record<string, string>;
  region?: string;
  zone?: string;
  create_time?: string;
  update_time?: string;
}

export interface ProviderRegistration {
  name: string;
  displayName?: string;
  serviceType: string;
  schemaVersion: string;
  operations: string[];
  endpoint: string;
  region?: string;
  zone?: string;
  metadata?: Record<string, string>;
}

export type CatalogFieldUiTier = 'required' | 'common' | 'advanced' | 'hidden';

export type CatalogFieldUiWidget =
  | 'text'
  | 'select'
  | 'password'
  | 'boolean'
  | 'number'
  | 'cpu-quantity'
  | 'domain'
  | 'site-domain'
  | 'hostname'
  | 'dns-label'
  | 'yaml';

export interface CatalogResourceSpec {
  name: string;
  service_type: string;
  fields?: CatalogFieldDef[];
  requires_resources?: string[];
}

export interface CatalogFieldDef {
  path: string;
  display_name?: string;
  description?: string;
  example?: string;
  examples?: string[];
  editable?: boolean;
  default?: unknown;
  required?: boolean;
  ui_tier?: CatalogFieldUiTier;
  ui_widget?: CatalogFieldUiWidget;
  validation_schema?: {
    type?: string;
    enum?: unknown[];
    minimum?: number;
    maximum?: number;
    minLength?: number;
    maxLength?: number;
    pattern?: string;
    format?: string;
    /** Catalog-manager persists UI hints under validation_schema (top-level ui_* is stripped). */
    'x-ui-tier'?: CatalogFieldUiTier;
    'x-ui-widget'?: CatalogFieldUiWidget;
    'x-required'?: boolean;
    'x-ui-depends-on'?: {
      path: string;
      equals?: unknown;
      notEquals?: unknown;
      in?: unknown[];
    };
    'x-value-preset'?: Record<string, Record<string, unknown>>;
    'x-sensitive'?: boolean;
    'x-secret-bind'?: {
      secretName: string;
      secretKey: string;
      secretDataFrom?: Record<string, string>;
      valuesSet?: Record<string, string>;
      valuesClear?: string[];
    };
    [key: string]: unknown;
  };
  /** Control-plane conditional allowed-values constraint (server validation). */
  depends_on?: {
    path: string;
    allowed_values: Record<string, unknown[]>;
  };
}

export interface CatalogItem {
  id?: string;
  uid?: string;
  name?: string;
  display_name?: string;
  service_type?: string;
  serviceType?: string;
  status?: string;
  state?: string;
  description?: string;
  spec?: Record<string, unknown> & {
    service_type?: string;
    fields?: CatalogFieldDef[];
    resources?: CatalogResourceSpec[];
    workload?: CatalogWorkloadSpec;
  };
  schema_version?: string;
  api_version?: string;
  path?: string;
  create_time?: string;
  update_time?: string;
}

export interface PlacementTargetStatus {
  provider?: string;
  sprm_id?: string;
  role?: string;
  status?: string;
  status_message?: string;
  healthy?: boolean;
}

export interface PlacementTargetsResponse {
  resource_id?: string;
  approval_status?: string;
  targets?: PlacementTargetStatus[];
  healthy_count?: number;
  target_count?: number;
}

export interface InstanceHubRouteResponse {
  instance_id?: string;
  effective_mode?: string;
  hub_route_applicable?: boolean;
  hub_host?: string;
  active_cluster?: string;
  standby_cluster?: string;
  hub_route_healthy?: boolean;
  failover_phase?: string;
  failover_mode?: string;
  last_failover_at?: string;
  consecutive_failures?: number;
}

export interface BackupInfrastructureStatus {
  hub_minio_ready?: boolean;
  bucket?: string;
  clusters?: Array<{
    name: string;
    ready: boolean;
    velero_ready: boolean;
    message?: string;
  }>;
}

export interface InstanceWorkloadResponse {
  instance_id: string;
  display_name?: string;
  service_type?: string;
  cluster: string;
  release: string;
  namespace?: string;
  hub_host?: string;
  active_cluster?: string;
  deployed?: boolean;
}

export interface CatalogInstanceSecretsRequest {
  cluster: string;
  namespace: string;
  secretName: string;
  data: Record<string, string>;
}

export interface InstanceBackupResponse {
  name: string;
  instance_id: string;
  cluster: string;
  release: string;
  namespace?: string;
  phase: string;
  message?: string;
  velero_backup_name?: string;
  artifact_secret?: string;
  started_at?: string;
  completed_at?: string;
}

export interface InstanceRestoreResponse {
  name: string;
  backup_ref: string;
  target_cluster: string;
  source_cluster?: string;
  phase: string;
  message?: string;
  velero_restore_name?: string;
  new_instance_id?: string;
  started_at?: string;
  completed_at?: string;
}

export interface CatalogItemInstance {
  id?: string;
  uid?: string;
  display_name?: string;
  resource_id?: string;
  spec?: {
    catalog_item_id?: string;
    user_values?: Array<{ path: string; value: unknown }>;
  };
  create_time?: string;
  update_time?: string;
  path?: string;
}

export interface CatalogItemCreate {
  /** Optional stable id for POST /catalog-items?id=… (import flow). */
  id?: string;
  name?: string;
  display_name?: string;
  serviceType?: string;
  service_type?: string;
  schemaVersion?: string;
  description?: string;
  spec?: Record<string, unknown> & {
    service_type?: string;
    fields?: CatalogFieldDef[];
    resources?: CatalogResourceSpec[];
    workload?: CatalogWorkloadSpec;
  };
  api_version?: string;
}

/** Console offers hub | site | none. `dual` remains valid on the API / FleetAppRoute for advanced use. */
export type RoutingMode = 'hub' | 'site' | 'dual' | 'none';

export type FleetSitePhase =
  | 'Draft'
  | 'ScriptIssued'
  | 'Enrolled'
  | 'Validated'
  | 'Provisioning'
  | 'Ready'
  | 'Failed';

export type FleetExposureMode = 'none' | 'port_forward' | 'reverse_proxy' | 'hub_gateway';
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

export interface FleetSiteExternalEndpoint {
  id: string;
  address: string;
  role?: string;
  description?: string;
}

export interface FleetSiteDomain {
  id: string;
  zone: string;
  wildcard?: string;
  tlsPolicy?: FleetTlsPolicy;
  assignedClusters?: string[];
  ingressEndpointId?: string;
}

export interface FleetSiteEdge {
  edgeDomain?: string;
  wildcard?: string;
  exposureMode?: FleetExposureMode;
  defaultExposureMode?: FleetExposureMode;
  externalEndpoints?: FleetSiteExternalEndpoint[];
  domains?: FleetSiteDomain[];
  wanEgress?: FleetSiteWanEgress;
  externalAccess?: FleetSiteExternalAccess;
}

export interface FleetSiteSpec {
  siteName: string;
  siteId?: string;
  orgId?: string;
  provisioningMode?: 'greenfield' | 'registry';
  displayName?: string;
  region?: string;
  failureDomain?: string;
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
  edgeDomain?: string;
  wildcard?: string;
  exposureMode?: FleetExposureMode;
  defaultExposureMode?: FleetExposureMode;
  externalEndpoints?: FleetSiteExternalEndpoint[];
  domains?: FleetSiteDomain[];
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

export interface FleetCluster {
  name: string;
  site?: string;
  pool?: 'shared' | 'dedicated';
  available?: boolean;
  management?: boolean;
  read_only?: boolean;
  provisioning_phase?: FleetClusterPhase;
  capabilities?: Record<string, boolean>;
  topology?: Record<string, string>;
  ingress_enabled?: boolean;
  direct_ingress?: boolean;
  exposure_mode?: FleetExposureMode | 'inherit';
  tls_policy?: FleetTlsPolicy;
  ingress_endpoint_id?: string;
  domain_bindings?: string[];
  upstream_host?: string;
  traefik_http_port?: number;
  console_managed?: boolean;
  provisioning_mode?: 'bundle' | 'hub_job';
  managed_cluster_available?: boolean;
  capability_discovery?: CapabilityDiscovery;
  storage?: FleetClusterStorageSpec;
  storageStatus?: FleetClusterStorageStatus;
  ingress_auth_enabled?: boolean;
  ingressAuthStatus?: FleetClusterIngressAuthStatus;
  acme_enabled?: boolean;
  acmeStatus?: FleetClusterAcmeStatus;
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
  pool?: 'shared' | 'dedicated';
  /** dc-admin only: set or clear ManagedCluster fleet.org */
  org?: string;
  orgId?: string;
  capabilities?: Record<string, boolean>;
  topology?: Record<string, string>;
  edge?: {
    direct_ingress?: boolean;
    exposure_mode?: FleetExposureMode | 'inherit';
    tls_policy?: FleetTlsPolicy;
    ingress_endpoint_id?: string;
    domain_bindings?: string[];
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
  exposure_mode?: string;
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
  orgId?: string;
  displayName?: string;
  region?: string;
  exposureMode?: FleetExposureMode;
  defaultExposureMode?: FleetExposureMode;
  edgeDomain?: string;
  wildcard?: string;
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
  minRootDiskGi?: number;
  clusterRegion?: string;
  clusterLabels?: Record<string, string>;
  gpu?: boolean;
  kvm?: boolean;
  clusterCapabilities?: Record<string, boolean>;
  storage?: FleetClusterStorageSpec;
  domainBindings?: string[];
  topology?: FleetClusterTopology;
  networkMode?: FleetClusterNetworkMode;
  hubLanApi?: string;
  controlPlaneVip?: string;
  nodes?: FleetClusterNodeInput[];
  edge?: {
    exposureMode?: FleetExposureMode | 'inherit';
    tlsPolicy?: FleetTlsPolicy;
    ingressEndpointId?: string;
  };
  providers?: {
    k8s?: boolean;
    helm?: boolean;
    kubevirt?: boolean;
    /** Day-0 host Linux bridge for Multus L2 KubeVirt VMs */
    kubevirtBridge?: {
      enabled?: boolean;
      bridgeName?: string;
      slaveNic?: string;
      keepIp?: boolean;
    };
  };
}

export type ClusterProviderKind = 'k8s' | 'helm' | 'kubevirt';

export type ClusterProviderPhase =
  | 'NotInstalled'
  | 'Installing'
  | 'Ready'
  | 'Deleting'
  | 'Failed';

export interface ClusterProviderRow {
  kind: ClusterProviderKind;
  displayName: string;
  providerName: string;
  nodePort?: number;
  phase: ClusterProviderPhase;
  message?: string;
}

export interface UserValue {
  path: string;
  value: unknown;
}

export interface CatalogItemInstanceCreate {
  api_version?: string;
  display_name: string;
  spec: {
    catalog_item_id: string;
    user_values: UserValue[];
    label_selector?: Record<string, string>;
    placement_override?: { cluster_count?: number };
  };
}

export type PolicyType = 'GLOBAL' | 'USER';

export interface ClusterRequirements {
  matchLabels?: Record<string, string>;
  matchExpressions?: unknown[];
}

export type PlacementStrategy = 'single' | 'spread' | 'all_matching' | 'least_utilized';

export interface PlacementSpec {
  cluster_count?: number;
  strategy?: PlacementStrategy;
  site_diversity?: boolean;
}

export type FailoverMode = 'manual' | 'relocatable' | 'standby';

export interface FailoverSpec {
  mode?: FailoverMode;
  hub_health_path?: string;
  failure_threshold?: number;
  success_threshold?: number;
  cooldown_seconds?: number;
  auto_revert?: boolean;
}

export interface Policy {
  id?: string;
  display_name?: string;
  description?: string;
  policy_type?: PolicyType;
  enabled?: boolean;
  priority?: number;
  rego_code?: string;
  label_selector?: Record<string, string>;
  cluster_requirements?: ClusterRequirements;
  placement?: PlacementSpec;
  failover?: FailoverSpec;
  create_time?: string;
  update_time?: string;
  path?: string;
}

export interface PolicyCreate {
  display_name: string;
  policy_type: PolicyType;
  rego_code: string;
  description?: string;
  enabled?: boolean;
  priority?: number;
  label_selector?: Record<string, string>;
  cluster_requirements?: ClusterRequirements;
  placement?: PlacementSpec;
  failover?: FailoverSpec;
}

export interface CatalogWorkloadSpec {
  replicas?: number;
}

export type PolicyPatch = Partial<PolicyCreate>;

export interface ServiceType {
  id?: string;
  name?: string;
  service_type?: string;
  description?: string;
}

export interface HealthStatus {
  status?: string;
  message?: string;
  [key: string]: unknown;
}

export interface PlacementPreviewCluster {
  name?: string;
  provider?: string;
  provider_registered?: boolean;
  provider_healthy?: boolean;
  matches_cluster_requirements?: boolean;
  fleet_site_phase?: string | null;
  cpu_utilization?: number | null;
  memory_utilization?: number | null;
  metrics_ok?: boolean;
  workload_fit?: string;
  fits_node?: boolean | null;
  fits_cluster?: boolean | null;
  limits_over_capacity?: boolean;
}

export interface PlacementPreviewClusterDiagnostic {
  name?: string;
  eligible?: boolean;
  provider?: string;
  matches_cluster_requirements?: boolean;
  provider_registered?: boolean;
  provider_healthy?: boolean;
  excluded?: boolean;
  reasons?: string[];
  cpu_utilization?: number | null;
  memory_utilization?: number | null;
  metrics_ok?: boolean;
  workload_fit?: string;
  fits_node?: boolean | null;
  fits_cluster?: boolean | null;
  limits_over_capacity?: boolean;
}

export interface PlacementPreviewPolicyTraceEntry {
  id?: string;
  display_name?: string;
  policy_type?: string;
  priority?: number;
  enabled?: boolean;
  instance_label_match?: boolean;
  evaluated?: boolean;
  rego_defined?: boolean;
  rego_rejected?: boolean;
  rejection_reason?: string;
  sets_cluster_requirements?: boolean;
  selected_provider?: string;
  won?: boolean;
  outcome?: string;
  detail?: string;
}

export interface PlacementPreviewResponse {
  deployable?: boolean;
  service_type?: string;
  winning_policy?: { id?: string; display_name?: string };
  eligible_clusters?: PlacementPreviewCluster[];
  eligible_count?: number;
  effective_placement_count?: number;
  warnings?: string[];
  blocking_reasons?: string[];
  cluster_diagnostics?: PlacementPreviewClusterDiagnostic[];
  policy_trace?: PlacementPreviewPolicyTraceEntry[];
}

export type PlacementPreviewMode = 'deploy' | 'policy';

export interface PlacementPreviewUserValue {
  path: string;
  value: unknown;
}

export interface PlacementPreviewRequest {
  mode: PlacementPreviewMode;
  verbose?: boolean;
  catalog_item_id?: string;
  service_type?: string;
  label_selector?: Record<string, string>;
  user_values?: PlacementPreviewUserValue[];
  placement_override?: { cluster_count?: number };
  policy?: PolicyCreate & { id?: string };
  probe_label_selector?: Record<string, string>;
}
