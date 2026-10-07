import type {
  FleetSite,
  FleetSiteCreate,
  FleetSitePatch,
  SiteHostRecord,
  SiteHostIntent,
  SiteHostIntentResult,
  SiteInventoryCatalog,
  SiteProvisionHistoryItem,
  SiteProvisionLogs,
  SiteProvisionBinding,
  SiteInventoryEnvelope,
  SiteInventoryFileInfo,
  SiteInventoryImportChange,
  SiteInventoryRevision,
  FleetCluster,
  FleetClusterAddWorkerBody,
  FleetClusterCreate,
  FleetClusterPatch,
  FleetClusterReplaceControlPlaneBody,
  ClusterNodeOperation,
  ClusterNodeOperationHistoryItem,
  ClusterNodeOperationLogs,
  CapabilityDiscovery,
  FleetNode,
  FleetNodePatch,
  HubInfrastructure,
  FleetVM,
  FleetVMPowerAction,
} from './types';
import { fleetSitePortForwardZone } from '../edge/ingressMethod';

const FLEET_SITE_API_BASE = '/fleet/api/v1alpha1';

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public body?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function listKey<T>(obj: unknown, keys: string[]): T[] {
  if (!obj || typeof obj !== 'object') return [];
  const record = obj as Record<string, unknown>;
  for (const k of keys) {
    const v = record[k];
    if (Array.isArray(v)) return v as T[];
  }
  if (Array.isArray(obj)) return obj as T[];
  return [];
}

function onUnauthorized(status: number) {
  if (status === 401 && !window.location.pathname.endsWith('/login')) {
    window.location.href = '/fleet/login';
  }
}

async function fleetSiteRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(FLEET_SITE_API_BASE + path, {
    credentials: 'include',
    headers: { Accept: 'application/json', ...init?.headers },
    ...init,
  });
  const text = await r.text();
  if (!r.ok) {
    onUnauthorized(r.status);
    throw new ApiError(
      path + ' → HTTP ' + r.status + (text ? ': ' + text.slice(0, 200) : ''),
      r.status,
      text,
    );
  }
  if (!text) return {} as T;
  if ((r.headers.get('content-type') || '').includes('application/json')) {
    return JSON.parse(text) as T;
  }
  return { _raw: text } as T;
}

export async function listFleetSites(): Promise<FleetSite[]> {
  const data = await fleetSiteRequest<{ sites?: FleetSite[] }>('/sites');
  return data.sites ?? [];
}

export async function getFleetSite(name: string): Promise<FleetSite> {
  return fleetSiteRequest<FleetSite>('/sites/' + encodeURIComponent(name));
}

export async function patchFleetSite(name: string, patch: FleetSitePatch): Promise<FleetSite> {
  return fleetSiteRequest<FleetSite>('/sites/' + encodeURIComponent(name), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export function fleetSiteEffectiveDomain(s: FleetSite): string {
  const pfZone = fleetSitePortForwardZone(s);
  if (pfZone) return pfZone;
  return s.spec?.edge?.hubZone || s.spec?.edgeDomain || '';
}

export function fleetSiteEffectiveId(s: FleetSite): string {
  return s.spec?.siteId || s.spec?.siteName || s.metadata?.name || '';
}

/** Resolve a site from an id that may be siteId, siteName, or metadata.name. */
export function fleetSiteById(sites: FleetSite[], id: string | undefined): FleetSite | undefined {
  if (!id) return undefined;
  return sites.find(
    (s) =>
      fleetSiteEffectiveId(s) === id ||
      s.metadata?.name === id ||
      s.spec?.siteId === id ||
      s.spec?.siteName === id,
  );
}

/** Human-readable site label for a fleet.site / siteId value. */
export function fleetSiteLabelForId(sites: FleetSite[], siteId: string | undefined): string {
  if (!siteId) return '—';
  const match = sites.find(
    (s) =>
      s.spec?.siteId === siteId ||
      s.metadata?.name === siteId ||
      s.spec?.siteName === siteId,
  );
  if (match) return fleetSiteDisplayName(match);
  return siteId;
}

export function fleetSiteClusterCount(s: FleetSite): number {
  return s.clusterCount ?? s.clusters?.length ?? 0;
}

export function fleetSiteCanDelete(s: FleetSite): boolean {
  return (
    fleetSiteIsRegistry(s) &&
    !fleetSiteIsManagement(s) &&
    fleetSiteClusterCount(s) === 0
  );
}

export function fleetSiteIsRegistry(s: FleetSite): boolean {
  return s.spec?.provisioningMode === 'registry';
}

export function fleetSiteIsManagement(s: FleetSite): boolean {
  return (
    s.metadata?.annotations?.['fleet.umkim.com/management-site'] === 'true' ||
    fleetSiteEffectiveId(s) === 'umkim'
  );
}

/** First registry site that is not the management site (workload default). */
export function fleetDefaultWorkloadSiteId(sites: FleetSite[]): string {
  const workload = sites.find(
    (s) => fleetSiteIsRegistry(s) && !fleetSiteIsManagement(s),
  );
  return workload ? fleetSiteEffectiveId(workload) : '';
}

export async function getHubInfrastructure(): Promise<HubInfrastructure> {
  return fleetSiteRequest<HubInfrastructure>('/infrastructure/hub');
}

export async function getIngressAuth(): Promise<import('./types').FleetIngressAuth> {
  return fleetSiteRequest<import('./types').FleetIngressAuth>('/ingress-auth');
}

export async function patchIngressAuth(
  patch: import('./types').FleetIngressAuthPatch,
): Promise<import('./types').FleetIngressAuth> {
  return fleetSiteRequest<import('./types').FleetIngressAuth>('/ingress-auth', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export async function getAcme(): Promise<import('./types').FleetAcme> {
  return fleetSiteRequest<import('./types').FleetAcme>('/acme');
}

export async function patchAcme(
  patch: import('./types').FleetAcmePatch,
): Promise<import('./types').FleetAcme> {
  return fleetSiteRequest<import('./types').FleetAcme>('/acme', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export async function getLLM(): Promise<import('./types').FleetLLM> {
  return fleetSiteRequest<import('./types').FleetLLM>('/llm');
}

export async function patchLLM(
  patch: import('./types').FleetLLMPatch,
): Promise<import('./types').FleetLLM> {
  return fleetSiteRequest<import('./types').FleetLLM>('/llm', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export async function reconcileLLM(): Promise<import('./types').FleetLLM> {
  return fleetSiteRequest<import('./types').FleetLLM>('/llm/reconcile', {
    method: 'POST',
  });
}

export async function listFleetClusters(): Promise<FleetCluster[]> {
  const data = await fleetSiteRequest<{ clusters?: FleetCluster[] }>('/clusters');
  return data.clusters ?? [];
}

export async function listFleetApps(): Promise<{
  apps: import('./types').FleetApp[];
  defaults: import('./types').FleetAppDefaults;
}> {
  const data = await fleetSiteRequest<{
    apps?: import('./types').FleetApp[];
    defaults?: import('./types').FleetAppDefaults;
  }>('/apps');
  return {
    apps: data.apps ?? [],
    defaults: data.defaults ?? { chartPrefix: '' },
  };
}

export async function createFleetApp(
  body: import('./types').FleetAppCreate,
): Promise<import('./types').FleetApp> {
  return fleetSiteRequest<import('./types').FleetApp>('/apps', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function deleteFleetApp(name: string): Promise<void> {
  await fleetSiteRequest('/apps/' + encodeURIComponent(name), { method: 'DELETE' });
}

export async function patchFleetApp(
  name: string,
  body: import('./types').FleetAppPatch,
): Promise<import('./types').FleetApp> {
  return fleetSiteRequest<import('./types').FleetApp>('/apps/' + encodeURIComponent(name), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function listLlamaCppProfiles(): Promise<import('./types').LlamaCppProfileMeta[]> {
  const data = await fleetSiteRequest<{ profiles?: import('./types').LlamaCppProfileMeta[] }>(
    '/charts/llama-cpp-helm/profiles',
  );
  return data.profiles ?? [];
}

export async function getLlamaCppProfileValues(profileId: string): Promise<string> {
  const data = await fleetSiteRequest<{ valuesYaml?: string }>(
    '/llama-profiles/' + encodeURIComponent(profileId) + '/values',
  );
  return data.valuesYaml ?? '';
}

export async function listFleetLlamaProfiles(): Promise<import('./types').FleetLlamaProfile[]> {
  const data = await fleetSiteRequest<{ profiles?: import('./types').FleetLlamaProfile[] }>(
    '/llama-profiles',
  );
  return data.profiles ?? [];
}

export async function getFleetLlamaProfile(name: string): Promise<import('./types').FleetLlamaProfile> {
  return fleetSiteRequest('/llama-profiles/' + encodeURIComponent(name));
}

export async function createFleetLlamaProfile(
  body: import('./types').FleetLlamaProfileCreate,
): Promise<import('./types').FleetLlamaProfile> {
  return fleetSiteRequest('/llama-profiles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function patchFleetLlamaProfile(
  name: string,
  body: import('./types').FleetLlamaProfilePatch,
): Promise<import('./types').FleetLlamaProfile> {
  return fleetSiteRequest('/llama-profiles/' + encodeURIComponent(name), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function deleteFleetLlamaProfile(name: string, force = false): Promise<void> {
  const q = force ? '?force=true' : '';
  await fleetSiteRequest('/llama-profiles/' + encodeURIComponent(name) + q, { method: 'DELETE' });
}

export async function previewFleetLlamaProfileValues(name: string): Promise<string> {
  const data = await fleetSiteRequest<{ valuesYaml?: string }>(
    '/llama-profiles/' + encodeURIComponent(name) + '/values',
  );
  return data.valuesYaml ?? '';
}

export async function verifyFleetLlamaProfile(
  name: string,
  body: { clusterName: string; mode: 'quick' | 'extended'; wait?: boolean; chartVersion?: string },
): Promise<import('./types').FleetLlamaProfileTest> {
  return fleetSiteRequest('/llama-profiles/' + encodeURIComponent(name) + '/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function getFleetLlamaProfileTest(id: string): Promise<import('./types').FleetLlamaProfileTest> {
  return fleetSiteRequest('/llama-profiles/tests/' + encodeURIComponent(id));
}

export async function endFleetLlamaProfileTest(id: string): Promise<void> {
  await fleetSiteRequest('/llama-profiles/tests/' + encodeURIComponent(id), { method: 'DELETE' });
}

export async function chatFleetLlamaProfileTest(
  id: string,
  body: { messages: { role: string; content: string }[]; max_tokens?: number },
): Promise<unknown> {
  return fleetSiteRequest('/llama-profiles/tests/' + encodeURIComponent(id) + '/chat/completions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function syncLLMBackendsFromApps(): Promise<import('./types').FleetLLM> {
  return fleetSiteRequest<import('./types').FleetLLM>('/llm/backends/sync-from-apps', {
    method: 'POST',
  });
}

export async function discoverFleetClusterGpu(name: string): Promise<unknown> {
  return fleetSiteRequest('/clusters/' + encodeURIComponent(name) + '/gpu/discover', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  });
}

export async function listFleetCharts(): Promise<import('./types').FleetChart[]> {
  const data = await fleetSiteRequest<{ charts?: import('./types').FleetChart[] }>('/charts');
  return data.charts ?? [];
}

export async function createFleetChart(
  body: import('./types').FleetChartCreate,
): Promise<import('./types').FleetChart> {
  return fleetSiteRequest<import('./types').FleetChart>('/charts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function uploadFleetChart(form: FormData): Promise<import('./types').FleetChart> {
  return fleetSiteRequest<import('./types').FleetChart>('/charts', {
    method: 'POST',
    body: form,
  });
}

export async function deleteFleetChart(name: string): Promise<void> {
  await fleetSiteRequest('/charts/' + encodeURIComponent(name), { method: 'DELETE' });
}

export async function getFleetCluster(name: string): Promise<FleetCluster> {
  return fleetSiteRequest<FleetCluster>('/clusters/' + encodeURIComponent(name));
}

export async function patchFleetCluster(name: string, patch: FleetClusterPatch): Promise<FleetCluster> {
  return fleetSiteRequest<FleetCluster>('/clusters/' + encodeURIComponent(name), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export async function enableClusterIngressAuth(
  name: string,
  body: { lldapAdminPassword: string; exposeLldapWebUI?: boolean },
): Promise<FleetCluster> {
  return fleetSiteRequest<FleetCluster>(
    '/clusters/' + encodeURIComponent(name) + '/ingress-auth/enable',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
}

export async function disableClusterIngressAuth(name: string): Promise<FleetCluster> {
  return fleetSiteRequest<FleetCluster>(
    '/clusters/' + encodeURIComponent(name) + '/ingress-auth/disable',
    { method: 'POST' },
  );
}

export async function enableClusterAcme(name: string): Promise<FleetCluster> {
  return fleetSiteRequest<FleetCluster>(
    '/clusters/' + encodeURIComponent(name) + '/acme/enable',
    { method: 'POST' },
  );
}

export async function disableClusterAcme(name: string): Promise<FleetCluster> {
  return fleetSiteRequest<FleetCluster>(
    '/clusters/' + encodeURIComponent(name) + '/acme/disable',
    { method: 'POST' },
  );
}

export async function discoverFleetClusterCapabilities(name: string): Promise<CapabilityDiscovery> {
  return fleetSiteRequest<CapabilityDiscovery>(
    '/clusters/' + encodeURIComponent(name) + '/discover-capabilities',
    { method: 'POST' },
  );
}

export async function listFleetClusterNodes(clusterName: string): Promise<FleetNode[]> {
  const data = await fleetSiteRequest<{ nodes?: FleetNode[] }>(
    '/clusters/' + encodeURIComponent(clusterName) + '/nodes',
  );
  return data.nodes ?? [];
}

export async function patchFleetClusterNode(
  clusterName: string,
  nodeName: string,
  patch: FleetNodePatch,
): Promise<FleetNode> {
  return fleetSiteRequest<FleetNode>(
    '/clusters/' + encodeURIComponent(clusterName) + '/nodes/' + encodeURIComponent(nodeName),
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    },
  );
}

export async function addFleetClusterWorker(
  clusterName: string,
  body: FleetClusterAddWorkerBody,
): Promise<ClusterNodeOperation> {
  return fleetSiteRequest<ClusterNodeOperation>(
    '/clusters/' + encodeURIComponent(clusterName) + '/nodes',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
}

export async function removeFleetClusterNode(
  clusterName: string,
  nodeName: string,
  force?: boolean,
): Promise<ClusterNodeOperation> {
  const q = force ? '?force=1' : '';
  return fleetSiteRequest<ClusterNodeOperation>(
    '/clusters/' + encodeURIComponent(clusterName) + '/nodes/' + encodeURIComponent(nodeName) + q,
    { method: 'DELETE' },
  );
}

export async function replaceFleetClusterControlPlane(
  clusterName: string,
  body: FleetClusterReplaceControlPlaneBody,
): Promise<ClusterNodeOperation> {
  return fleetSiteRequest<ClusterNodeOperation>(
    '/clusters/' + encodeURIComponent(clusterName) + '/control-plane/replace',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
}

export async function validateFleetClusterNodeOperation(
  clusterName: string,
  body: Record<string, unknown>,
  phase: 'pre' | 'post' = 'pre',
): Promise<import('./types').NodeOperationValidateResult> {
  const q = phase === 'post' ? '?phase=post' : '';
  const r = await fetch(
    FLEET_SITE_API_BASE +
      '/clusters/' +
      encodeURIComponent(clusterName) +
      '/node-operations/validate' +
      q,
    {
      method: 'POST',
      credentials: 'include',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
  const text = await r.text();
  let data: import('./types').NodeOperationValidateResult;
  try {
    data = JSON.parse(text) as import('./types').NodeOperationValidateResult;
  } catch {
    onUnauthorized(r.status);
    throw new ApiError('node-operation validate → HTTP ' + r.status, r.status, text);
  }
  if (!r.ok && r.status !== 422) {
    onUnauthorized(r.status);
    throw new ApiError('node-operation validate → HTTP ' + r.status, r.status, text);
  }
  return data;
}

export async function listFleetClusterNodeOperations(
  clusterName: string,
): Promise<ClusterNodeOperationHistoryItem[]> {
  const data = await fleetSiteRequest<{ items: ClusterNodeOperationHistoryItem[] }>(
    '/clusters/' + encodeURIComponent(clusterName) + '/node-operations',
  );
  return data.items || [];
}

export async function fetchFleetClusterNodeOperationLogs(
  clusterName: string,
  operationId: string,
): Promise<ClusterNodeOperationLogs> {
  return fleetSiteRequest<ClusterNodeOperationLogs>(
    '/clusters/' +
      encodeURIComponent(clusterName) +
      '/node-operations/' +
      encodeURIComponent(operationId) +
      '/logs',
  );
}

export async function createFleetSite(body: FleetSiteCreate): Promise<FleetSite> {
  const ingress = body.edge?.ingress?.length
    ? body.edge.ingress
    : [
        {
          id: 'default',
          method: 'port_forward' as const,
          address: '',
          zone: '',
          wildcard: '',
        },
      ];
  const payload = {
    apiVersion: 'fleet.umkim.com/v1alpha1',
    kind: 'FleetSite',
    metadata: { name: body.siteName },
    spec: {
      siteName: body.siteName,
      displayName: body.displayName,
      provisioningMode: 'registry',
      region: body.region,
      failureDomain: body.failureDomain,
      jumphost: body.jumphost,
      edge: {
        hubGateway: body.hubGateway ?? body.edge?.hubGateway,
        hubZone: body.hubZone ?? body.edge?.hubZone,
        hubWildcard: body.hubWildcard ?? body.edge?.hubWildcard,
        ingress,
        wanEgress:
          body.wanEgress ||
          body.edge?.wanEgress ||
          (body.externalAccess || body.edge?.externalAccess
            ? {
                enabled: (body.externalAccess || body.edge?.externalAccess)?.enabled,
                dnsTargetNote: (body.externalAccess || body.edge?.externalAccess)?.dnsTargetNote,
                expectedWanIp: (body.externalAccess || body.edge?.externalAccess)?.expectedWanIp,
              }
            : undefined),
      },
    },
  };
  return fleetSiteRequest<FleetSite>('/sites', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export async function createFleetCluster(body: FleetClusterCreate): Promise<FleetCluster> {
  const caps = { ...(body.clusterCapabilities || {}) };
  const prov = body.providers;
  if (prov?.kubevirt || body.kvm) caps.kvm = true;
  if (body.topology === 'ha_3_node') caps.ha_topology = true;
  const install = prov?.kubevirt
    ? {
        kubevirt: true,
      }
    : undefined;
  const kubevirtBridge =
    prov?.kubevirt && prov.kubevirtBridge?.enabled
      ? {
          enabled: true,
          bridgeName: prov.kubevirtBridge.bridgeName || 'mgmt-l2-br',
          slaveNic: prov.kubevirtBridge.slaveNic || undefined,
          keepIp: prov.kubevirtBridge.keepIp !== false,
        }
      : undefined;
  return fleetSiteRequest<FleetCluster>('/clusters', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      clusterName: body.clusterName,
      siteId: body.siteId,
      bootstrapHost: body.bootstrapHost,
      jumphost: body.jumphost,
      minRootDiskGi: body.minRootDiskGi ?? 40,
      clusterRegion: body.clusterRegion || 'fleet',
      clusterLabels: body.clusterLabels,
      clusterCapabilities: caps,
      storage: body.storage,
      edge: body.edge,
      topology: body.topology,
      networkMode: body.networkMode,
      nodes: body.nodes,
      hubLanApi: body.hubLanApi,
      controlPlaneVip: body.controlPlaneVip,
      providers: install
        ? {
            install,
            ...(kubevirtBridge ? { kubevirtBridge } : {}),
          }
        : undefined,
    }),
  });
}

export async function downloadFleetClusterBootstrapScript(name: string): Promise<string> {
  const r = await fetch(
    FLEET_SITE_API_BASE + '/clusters/' + encodeURIComponent(name) + '/bootstrap-script',
    { credentials: 'include' },
  );
  const text = await r.text();
  if (!r.ok) {
    throw new ApiError('bootstrap-script → HTTP ' + r.status, r.status, text);
  }
  return text;
}

export async function downloadFleetClusterBootstrapBundle(name: string): Promise<Blob> {
  const r = await fetch(
    FLEET_SITE_API_BASE + '/clusters/' + encodeURIComponent(name) + '/bootstrap-bundle',
    { credentials: 'include' },
  );
  if (!r.ok) {
    const text = await r.text();
    throw new ApiError('bootstrap-bundle → HTTP ' + r.status, r.status, text);
  }
  return r.blob();
}

export async function validateFleetCluster(name: string): Promise<void> {
  await fleetSiteRequest('/clusters/' + encodeURIComponent(name) + '/validate', {
    method: 'POST',
  });
}

export async function provisionFleetCluster(name: string): Promise<void> {
  await fleetSiteRequest('/clusters/' + encodeURIComponent(name) + '/provision', {
    method: 'POST',
  });
}

export async function configureFleetClusterStorage(
  name: string,
  body: import('./types').FleetClusterConfigureStorage,
): Promise<{ phase?: string; message?: string }> {
  return fleetSiteRequest<{ phase?: string; message?: string }>(
    '/clusters/' + encodeURIComponent(name) + '/configure-storage',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
}

export async function enableFleetClusterKubevirt(
  name: string,
  body: {
    lanBridge?: boolean;
    bridgeName?: string;
    slaveNic?: string;
    keepIp?: boolean;
    force?: boolean;
  } = {},
): Promise<{ phase?: string; message?: string; lanBridge?: boolean }> {
  return fleetSiteRequest<{ phase?: string; message?: string; lanBridge?: boolean }>(
    '/clusters/' + encodeURIComponent(name) + '/platform/kubevirt',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
}

export async function getFleetClusterKubevirtStatus(
  name: string,
): Promise<{
  phase?: string;
  jobName?: string;
  message?: string;
  install?: boolean;
  lanBridge?: boolean;
  bridgeName?: string;
}> {
  return fleetSiteRequest('/clusters/' + encodeURIComponent(name) + '/platform/kubevirt');
}

export type FleetClusterGpuStatus = {
  hostPhase?: string;
  hostJobName?: string;
  hostMessage?: string;
  runtimePhase?: string;
  runtimeJobName?: string;
  runtimeMessage?: string;
  pciId?: string;
  marketingName?: string;
  driverBranch?: string;
  driverLoaded?: boolean;
  clusterGpuCapability?: boolean;
  gpuTotal?: number;
  gpuAvailable?: number;
  recommendedDriverBranch?: string;
};

export async function getFleetClusterGpuStatus(name: string): Promise<FleetClusterGpuStatus> {
  return fleetSiteRequest<FleetClusterGpuStatus>(
    '/clusters/' + encodeURIComponent(name) + '/gpu',
  );
}

export async function prepFleetClusterGpuHost(
  name: string,
  body: { force?: boolean } = {},
): Promise<{ phase?: string; message?: string }> {
  return fleetSiteRequest<{ phase?: string; message?: string }>(
    '/clusters/' + encodeURIComponent(name) + '/gpu/host-prep',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
}

export async function enableFleetClusterGpu(
  name: string,
): Promise<{ phase?: string; message?: string }> {
  return fleetSiteRequest<{ phase?: string; message?: string }>(
    '/clusters/' + encodeURIComponent(name) + '/gpu/enable',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    },
  );
}

export async function validateFleetClusterStorage(
  name: string,
  body: import('./types').FleetClusterConfigureStorage,
): Promise<import('./types').FleetClusterStorageValidateResult> {
  const r = await fetch(
    FLEET_SITE_API_BASE + '/clusters/' + encodeURIComponent(name) + '/validate-storage',
    {
      method: 'POST',
      credentials: 'include',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  );
  const text = await r.text();
  let parsed: import('./types').FleetClusterStorageValidateResult | null = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as import('./types').FleetClusterStorageValidateResult;
    } catch {
      parsed = null;
    }
  }
  if ((r.status === 200 || r.status === 422) && parsed?.checks) {
    return parsed;
  }
  if (!r.ok) {
    onUnauthorized(r.status);
    throw new ApiError(
      'validate-storage → HTTP ' + r.status + (text ? ': ' + text.slice(0, 200) : ''),
      r.status,
      text,
    );
  }
  throw new ApiError('validate-storage → unexpected response', r.status, text);
}

export async function deleteFleetSite(name: string): Promise<void> {
  try {
    await fleetSiteRequest('/sites/' + encodeURIComponent(name), { method: 'DELETE' });
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return;
    throw e;
  }
}

export async function deleteFleetCluster(
  name: string,
  opts?: { cleanupKubernetes?: boolean },
): Promise<void> {
  const q = new URLSearchParams();
  if (opts?.cleanupKubernetes) q.set('cleanupKubernetes', '1');
  const qs = q.toString();
  const path = '/clusters/' + encodeURIComponent(name) + (qs ? '?' + qs : '');
  try {
    await fleetSiteRequest(path, { method: 'DELETE' });
  } catch (e) {
    // Idempotent: cluster already removed server-side (stale console row).
    if (e instanceof ApiError && e.status === 404) return;
    throw e;
  }
}

/** Console-provisioned cluster (FleetClusterJoin), not inventory-only OCM registration. */
export function fleetClusterIsConsoleProvisioned(c: FleetCluster): boolean {
  return (
    c.provisioning_mode === 'bundle' ||
    c.provisioning_mode === 'hub_job' ||
    (!!c.provisioning_phase && c.provisioning_phase !== 'Ready')
  );
}

export function fleetClusterIsDeleting(c: FleetCluster): boolean {
  return c.provisioning_phase === 'Deleting';
}

export function fleetClusterIsPending(c: FleetCluster): boolean {
  return (
    !c.available &&
    !!c.provisioning_phase &&
    c.provisioning_phase !== 'Ready' &&
    c.provisioning_phase !== 'Deleting'
  );
}

export function fleetClusterPhase(c: FleetCluster): string {
  if (c.provisioning_phase) return c.provisioning_phase;
  return c.available ? 'Ready' : 'Unknown';
}

export async function downloadFleetSiteBootstrapScript(name: string): Promise<string> {
  const r = await fetch(
    FLEET_SITE_API_BASE + '/sites/' + encodeURIComponent(name) + '/bootstrap-script',
    { credentials: 'include' },
  );
  const text = await r.text();
  if (!r.ok) {
    onUnauthorized(r.status);
    throw new ApiError('bootstrap-script → HTTP ' + r.status, r.status, text);
  }
  return text;
}

export async function validateFleetSite(name: string): Promise<void> {
  await fleetSiteRequest('/sites/' + encodeURIComponent(name) + '/validate', {
    method: 'POST',
  });
}

export async function provisionFleetSite(name: string): Promise<void> {
  await fleetSiteRequest('/sites/' + encodeURIComponent(name) + '/provision', {
    method: 'POST',
  });
}

export async function listSiteHosts(site: string): Promise<SiteHostRecord[]> {
  const data = await fleetSiteRequest<{ items?: SiteHostRecord[] }>(
    '/sites/' + encodeURIComponent(site) + '/hosts',
  );
  return data.items ?? [];
}

export async function provisionSiteHost(
  site: string,
  name: string,
  body?: { action?: string; confirm?: boolean },
): Promise<Record<string, unknown>> {
  return fleetSiteRequest<Record<string, unknown>>(
    '/sites/' + encodeURIComponent(site) + '/hosts/' + encodeURIComponent(name) + '/provision',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body ?? {}),
    },
  );
}

export async function getSiteInventoryCatalog(site: string): Promise<SiteInventoryCatalog> {
  return fleetSiteRequest<SiteInventoryCatalog>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/catalog',
  );
}

export async function createSiteHost(
  site: string,
  intent: SiteHostIntent,
): Promise<SiteHostIntentResult> {
  return fleetSiteRequest<SiteHostIntentResult>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/hosts',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(intent),
    },
  );
}

export async function applySiteHostCluster(site: string, name: string): Promise<SiteHostRecord> {
  return fleetSiteRequest<SiteHostRecord>(
    '/sites/' + encodeURIComponent(site) + '/hosts/' + encodeURIComponent(name) + '/cluster',
    { method: 'POST' },
  );
}

export async function listSiteProvisionActions(site: string): Promise<SiteProvisionHistoryItem[]> {
  const data = await fleetSiteRequest<{ items?: SiteProvisionHistoryItem[] }>(
    '/sites/' + encodeURIComponent(site) + '/provision/actions',
  );
  return data.items ?? [];
}

export async function fetchSiteProvisionActionLogs(
  site: string,
  actionId: string,
): Promise<SiteProvisionLogs> {
  return fleetSiteRequest<SiteProvisionLogs>(
    '/sites/' +
      encodeURIComponent(site) +
      '/provision/actions/' +
      encodeURIComponent(actionId) +
      '/logs',
  );
}

export async function getSiteProvisionBinding(site: string): Promise<SiteProvisionBinding> {
  return fleetSiteRequest<SiteProvisionBinding>('/sites/' + encodeURIComponent(site) + '/provision');
}

export async function getSiteInventory(site: string): Promise<SiteInventoryEnvelope> {
  return fleetSiteRequest<SiteInventoryEnvelope>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory',
  );
}

function inventoryFileURL(site: string, path: string): string {
  const segments = path.split('/').map((p) => encodeURIComponent(p)).join('/');
  return '/sites/' + encodeURIComponent(site) + '/provision/inventory/files/' + segments;
}

async function fleetSiteRequestText(path: string, init?: RequestInit): Promise<string> {
  const r = await fetch(FLEET_SITE_API_BASE + path, {
    credentials: 'include',
    ...init,
  });
  const text = await r.text();
  if (!r.ok) {
    onUnauthorized(r.status);
    throw new ApiError(
      path + ' → HTTP ' + r.status + (text ? ': ' + text.slice(0, 200) : ''),
      r.status,
      text,
    );
  }
  return text;
}

export async function listSiteInventoryFiles(site: string): Promise<SiteInventoryFileInfo[]> {
  const data = await fleetSiteRequest<{ items?: SiteInventoryFileInfo[] }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/files',
  );
  return data.items ?? [];
}

export async function getSiteInventoryFile(site: string, path: string): Promise<string> {
  return fleetSiteRequestText(inventoryFileURL(site, path));
}

export async function getSiteInventoryFileOptional(
  site: string,
  path: string,
): Promise<{ content: string; missing: boolean }> {
  try {
    const content = await getSiteInventoryFile(site, path);
    return { content, missing: false };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      return { content: '', missing: true };
    }
    throw e;
  }
}

export async function putSiteInventoryFile(site: string, path: string, content: string): Promise<{ path?: string; syncPhase?: string }> {
  return fleetSiteRequest<{ path?: string; syncPhase?: string }>(inventoryFileURL(site, path), {
    method: 'PUT',
    headers: { 'Content-Type': 'text/yaml; charset=utf-8' },
    body: content,
  });
}

export async function deleteSiteInventoryFile(site: string, path: string): Promise<void> {
  await fleetSiteRequestText(inventoryFileURL(site, path), { method: 'DELETE' });
}

export async function previewSiteInventoryImport(site: string): Promise<{ jobName?: string; phase?: string }> {
  return fleetSiteRequest<{ jobName?: string; phase?: string }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/import/preview',
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' },
  );
}

export async function getSiteInventoryImportPreview(site: string): Promise<SiteInventoryImportChange[]> {
  const data = await fleetSiteRequest<{ items?: SiteInventoryImportChange[] }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/import/preview',
  );
  return data.items ?? [];
}

export async function applySiteInventoryImport(
  site: string,
  paths: string[],
  confirm = true,
  content?: Record<string, string>,
): Promise<{ applied?: number; status?: string }> {
  return fleetSiteRequest<{ applied?: number; status?: string }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/import/apply',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(content ? { confirm, paths, content } : { confirm, paths }),
    },
  );
}

export async function listSiteInventoryRevisions(
  site: string,
  limit = 20,
): Promise<SiteInventoryRevision[]> {
  const data = await fleetSiteRequest<{ items?: SiteInventoryRevision[] }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/revisions?limit=' + limit,
  );
  return data.items ?? [];
}

export async function getSiteInventoryRevision(
  site: string,
  id: string,
): Promise<SiteInventoryRevision> {
  return fleetSiteRequest<SiteInventoryRevision>(
    '/sites/' +
      encodeURIComponent(site) +
      '/provision/inventory/revisions/' +
      encodeURIComponent(id),
  );
}

export async function syncSiteInventory(site: string): Promise<{ jobName?: string; phase?: string }> {
  return fleetSiteRequest<{ jobName?: string; phase?: string }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/sync',
    { method: 'POST' },
  );
}

export async function importSiteInventory(site: string): Promise<{ jobName?: string; phase?: string }> {
  return fleetSiteRequest<{ jobName?: string; phase?: string }>(
    '/sites/' + encodeURIComponent(site) + '/provision/inventory/import',
    { method: 'POST' },
  );
}

export function fleetSiteDisplayName(s: FleetSite): string {
  return s.spec?.displayName || s.spec?.siteName || s.metadata?.name || '—';
}

export function fleetSitePhase(s: FleetSite): string {
  return s.status?.phase || 'Draft';
}

export async function getEdgeDomains(): Promise<{ registry?: unknown }> {
  return fleetSiteRequest<{ registry?: unknown }>('/edge/domains');
}

export async function checkEdgeHostname(
  host: string,
): Promise<{ host: string; available: boolean; used_by?: string }> {
  const q = new URLSearchParams({ host });
  return fleetSiteRequest(`/edge/hostnames/check?${q.toString()}`);
}

export async function validateEdgeDNS(
  host: string,
  expectedIp?: string,
): Promise<Record<string, unknown>> {
  const q = new URLSearchParams({ host });
  if (expectedIp) q.set('expected_ip', expectedIp);
  return fleetSiteRequest<Record<string, unknown>>(`/edge/dns/validate?${q.toString()}`);
}

export async function listFleetClusterVMs(clusterName: string): Promise<FleetVM[]> {
  const data = await fleetSiteRequest<{ vms?: FleetVM[] }>(
    '/clusters/' + encodeURIComponent(clusterName) + '/vms',
  );
  return listKey<FleetVM>(data, ['vms']);
}

export async function getFleetClusterVM(
  clusterName: string,
  namespace: string,
  name: string,
): Promise<FleetVM> {
  return fleetSiteRequest<FleetVM>(
    '/clusters/' +
      encodeURIComponent(clusterName) +
      '/vms/' +
      encodeURIComponent(namespace) +
      '/' +
      encodeURIComponent(name),
  );
}

export async function powerFleetClusterVM(
  clusterName: string,
  namespace: string,
  name: string,
  action: FleetVMPowerAction,
  force = false,
): Promise<{ status?: string; action?: string }> {
  return fleetSiteRequest(
    '/clusters/' +
      encodeURIComponent(clusterName) +
      '/vms/' +
      encodeURIComponent(namespace) +
      '/' +
      encodeURIComponent(name) +
      '/power',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, force }),
    },
  );
}

/** Browser websocket URL for serial (console) or VNC. */
export function fleetClusterVMConsoleWSURL(
  clusterName: string,
  namespace: string,
  name: string,
  kind: 'serial' | 'vnc' = 'serial',
): string {
  const sub = kind === 'vnc' ? 'vnc' : 'console';
  const path =
    FLEET_SITE_API_BASE +
    '/clusters/' +
    encodeURIComponent(clusterName) +
    '/vms/' +
    encodeURIComponent(namespace) +
    '/' +
    encodeURIComponent(name) +
    '/' +
    sub;
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return proto + '//' + window.location.host + path;
}
