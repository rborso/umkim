import type {
  FleetSite,
  FleetSiteCreate,
  FleetSitePatch,
  FleetCluster,
  FleetClusterCreate,
  FleetClusterPatch,
  CapabilityDiscovery,
  ClusterProviderKind,
  ClusterProviderRow,
  FleetNode,
  FleetNodePatch,
  HubInfrastructure,
  CatalogItem,
  CatalogItemCreate,
  CatalogItemInstance,
  CatalogItemInstanceCreate,
  HealthStatus,
  Policy,
  PolicyCreate,
  PolicyPatch,
  PlacementPreviewRequest,
  PlacementPreviewResponse,
  InstanceHubRouteResponse,
  PlacementTargetsResponse,
  Provider,
  ProviderRegistration,
  ServiceType,
  UserValue,
} from './types';

const API_BASE = '/api/v1alpha1';
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

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const r = await fetch(API_BASE + path, {
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
  const ct = r.headers.get('content-type') || '';
  if (ct.includes('application/json')) {
    return JSON.parse(text) as T;
  }
  return { _raw: text } as T;
}

const healthListFallback: Record<
  'providers' | 'catalog' | 'policies' | 'placement',
  string
> = {
  providers: '/providers',
  catalog: '/catalog-items',
  policies: '/policies',
  placement: '/catalog-item-instances',
};

export async function getHealth(
  component: 'providers' | 'catalog' | 'policies' | 'placement',
): Promise<HealthStatus> {
  try {
    return await request<HealthStatus>('/health/' + component);
  } catch (e) {
    // control-plane monolith may not expose per-component /health/* routes yet
    if (!(e instanceof ApiError && e.status === 404)) {
      throw e;
    }
    await request<unknown>(healthListFallback[component]);
    return { status: 'reachable', component };
  }
}

export async function listProviders(): Promise<Provider[]> {
  const data = await request<unknown>('/providers');
  return listKey<Provider>(data, ['providers', 'items']);
}

export async function getProvider(id: string): Promise<Provider> {
  return request<Provider>('/providers/' + encodeURIComponent(id));
}

export async function createProvider(body: ProviderRegistration): Promise<Provider> {
  return request<Provider>('/providers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function updateProvider(
  id: string,
  body: ProviderRegistration,
): Promise<Provider> {
  return request<Provider>('/providers/' + encodeURIComponent(id), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

function normalizeCatalogItem(item: CatalogItem): CatalogItem {
  return {
    ...item,
    id: item.id || item.uid,
    name: item.name || item.display_name,
    service_type: item.service_type || item.serviceType,
  };
}

function normalizeCatalogInstance(item: CatalogItemInstance): CatalogItemInstance {
  return {
    ...item,
    id: item.id || item.uid,
  };
}

export function catalogItemDisplayName(c: CatalogItem): string {
  return c.display_name || c.name || '—';
}

export function catalogItemId(c: CatalogItem): string {
  return c.id || c.uid || '—';
}

export async function getCatalogItem(id: string): Promise<CatalogItem> {
  const item = await request<CatalogItem>(
    '/catalog-items/' + encodeURIComponent(id),
  );
  return normalizeCatalogItem(item);
}

export async function deleteCatalogItem(id: string): Promise<void> {
  await request<void>('/catalog-items/' + encodeURIComponent(id), {
    method: 'DELETE',
  });
}

export async function getInstancePlacementTargets(
  instanceId: string,
): Promise<PlacementTargetsResponse> {
  return request<PlacementTargetsResponse>(
    '/catalog-item-instances/' + encodeURIComponent(instanceId) + '/placement-targets',
  );
}

export async function getInstanceHubRoute(
  instanceId: string,
): Promise<InstanceHubRouteResponse | null> {
  try {
    return await fleetSiteRequest<InstanceHubRouteResponse>(
      '/instance-routes/' + encodeURIComponent(instanceId),
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

export async function getBackupInfrastructure(): Promise<import('./types').BackupInfrastructureStatus> {
  return fleetSiteRequest<import('./types').BackupInfrastructureStatus>('/backup-infrastructure');
}

export async function getInstanceWorkload(
  instanceId: string,
): Promise<import('./types').InstanceWorkloadResponse> {
  return fleetSiteRequest<import('./types').InstanceWorkloadResponse>(
    '/instance-workloads/' + encodeURIComponent(instanceId),
  );
}

export async function putCatalogInstanceSecrets(
  body: import('./types').CatalogInstanceSecretsRequest,
): Promise<void> {
  await fleetSiteRequest('/catalog-instance-secrets', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function deleteCatalogInstanceSecrets(
  body: import('./types').CatalogInstanceSecretsRequest,
): Promise<void> {
  await fleetSiteRequest('/catalog-instance-secrets', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function listInstanceBackups(params?: {
  instance_id?: string;
  cluster?: string;
  phase?: string;
}): Promise<import('./types').InstanceBackupResponse[]> {
  const q = new URLSearchParams();
  if (params?.instance_id) q.set('instance_id', params.instance_id);
  if (params?.cluster) q.set('cluster', params.cluster);
  if (params?.phase) q.set('phase', params.phase);
  const suffix = q.toString() ? '?' + q.toString() : '';
  const data = await fleetSiteRequest<{ backups?: import('./types').InstanceBackupResponse[] }>(
    '/instance-backups' + suffix,
  );
  return data.backups ?? [];
}

export async function getInstanceBackup(
  name: string,
): Promise<import('./types').InstanceBackupResponse> {
  return fleetSiteRequest<import('./types').InstanceBackupResponse>(
    '/instance-backups/' + encodeURIComponent(name),
  );
}

export async function createInstanceBackup(body: {
  instance_id: string;
  cluster?: string;
  release?: string;
  namespace?: string;
  wait?: boolean;
}): Promise<{ name: string; phase: string }> {
  return fleetSiteRequest<{ name: string; phase: string }>('/instance-backups', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function listInstanceRestores(params?: {
  instance_id?: string;
  backup_name?: string;
}): Promise<import('./types').InstanceRestoreResponse[]> {
  const q = new URLSearchParams();
  if (params?.instance_id) q.set('instance_id', params.instance_id);
  if (params?.backup_name) q.set('backup_name', params.backup_name);
  const suffix = q.toString() ? '?' + q.toString() : '';
  const data = await fleetSiteRequest<{ restores?: import('./types').InstanceRestoreResponse[] }>(
    '/instance-restores' + suffix,
  );
  return data.restores ?? [];
}

export async function getInstanceRestore(
  name: string,
): Promise<import('./types').InstanceRestoreResponse> {
  return fleetSiteRequest<import('./types').InstanceRestoreResponse>(
    '/instance-restores/' + encodeURIComponent(name),
  );
}

export async function createInstanceRestore(body: {
  backup_name: string;
  target_cluster: string;
  source_cluster?: string;
  rebind?: {
    enabled: boolean;
    mode?: 'supersede' | 'clone';
    display_name_suffix?: string;
    user_value_patches?: Array<{ path: string; value: string }>;
  };
  storage_class_hints?: Record<string, string>;
}): Promise<{ name: string; phase: string }> {
  return fleetSiteRequest<{ name: string; phase: string }>('/instance-restores', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function getCatalogItemInstance(
  id: string,
): Promise<CatalogItemInstance> {
  const item = await request<CatalogItemInstance>(
    '/catalog-item-instances/' + encodeURIComponent(id),
  );
  return normalizeCatalogInstance(item);
}

export async function deleteCatalogItemInstance(
  id: string,
  opts?: { deletionMode?: 'strict' | 'deferred' | 'abandon' },
): Promise<void> {
  const mode = opts?.deletionMode ?? 'strict';
  const q = mode === 'strict' ? '' : `?deletion_mode=${encodeURIComponent(mode)}`;
  await request<void>('/catalog-item-instances/' + encodeURIComponent(id) + q, {
    method: 'DELETE',
  });
}

export type HelmHistoryEntry = {
  revision?: number;
  status?: string;
  chart?: string;
  app_version?: string;
  description?: string;
};

export async function getHelmInstanceHistory(
  instanceId: string,
): Promise<{ releases: HelmHistoryEntry[] }> {
  return request<{ releases: HelmHistoryEntry[] }>(
    '/catalog-item-instances/' + encodeURIComponent(instanceId) + '/helm/history',
  );
}

export async function rollbackHelmInstance(
  instanceId: string,
  revision: number,
): Promise<unknown> {
  return request<unknown>(
    '/catalog-item-instances/' + encodeURIComponent(instanceId) + '/helm/rollback',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ revision }),
    },
  );
}

export async function upgradeCatalogItemInstance(
  instanceId: string,
  userValues: UserValue[],
): Promise<CatalogItemInstance> {
  return request<CatalogItemInstance>(
    '/catalog-item-instances/' + encodeURIComponent(instanceId),
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ spec: { user_values: userValues } }),
    },
  );
}

export async function listCatalogItems(): Promise<CatalogItem[]> {
  const data = await request<unknown>('/catalog-items');
  return listKey<CatalogItem>(data, [
    'results',
    'catalog_items',
    'catalog-items',
    'items',
  ]).map(normalizeCatalogItem);
}

export async function listCatalogItemInstances(): Promise<CatalogItemInstance[]> {
  const data = await request<unknown>('/catalog-item-instances');
  return listKey<CatalogItemInstance>(data, [
    'results',
    'catalog_item_instances',
    'catalog-item-instances',
    'instances',
    'items',
  ]).map(normalizeCatalogInstance);
}

function toCatalogItemBody(
  body: CatalogItemCreate,
): Record<string, unknown> {
  const serviceType =
    body.service_type || body.serviceType || body.spec?.service_type || 'container';
  const displayName =
    body.display_name || body.name || '';
  const specIn = { ...(body.spec || {}) };
  let spec: Record<string, unknown>;
  if (Array.isArray(specIn.resources) && specIn.resources.length > 0) {
    spec = specIn;
  } else {
    const { service_type: _st, fields, ...rest } = specIn;
    spec = {
      ...rest,
      resources: [
        {
          name: 'app',
          service_type: serviceType,
          fields: fields || [],
        },
      ],
    };
  }
  return {
    api_version: body.api_version || 'v1alpha1',
    display_name: displayName,
    description: body.description,
    spec,
  };
}

export async function createCatalogItem(body: CatalogItemCreate): Promise<CatalogItem> {
  const id = (body.id || body.name || '').trim();
  const path = id
    ? `/catalog-items?id=${encodeURIComponent(id)}`
    : '/catalog-items';
  return request<CatalogItem>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(toCatalogItemBody(body)),
  });
}

export async function createCatalogItemInstance(
  body: CatalogItemInstanceCreate,
): Promise<CatalogItemInstance> {
  return request<CatalogItemInstance>('/catalog-item-instances', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      api_version: body.api_version || 'v1alpha1',
      display_name: body.display_name,
      spec: body.spec,
    }),
  });
}

export async function updateCatalogItem(
  id: string,
  body: Partial<CatalogItemCreate>,
): Promise<CatalogItem> {
  return request<CatalogItem>('/catalog-items/' + encodeURIComponent(id), {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/merge-patch+json',
    },
    body: JSON.stringify(toCatalogItemBody(body as CatalogItemCreate)),
  });
}

export function catalogItemServiceType(c: CatalogItem): string {
  const spec = c.spec as Record<string, unknown> | undefined;
  const resources = spec?.resources;
  if (Array.isArray(resources) && resources.length > 0) {
    const st = (resources[0] as { service_type?: string }).service_type;
    if (st) return st;
  }
  return (
    c.service_type ||
    c.serviceType ||
    (spec?.service_type as string) ||
    '—'
  );
}

export function isOperatorCatalogItem(c: CatalogItem): boolean {
  return catalogItemServiceType(c) === 'operator';
}

export async function listOperatorCatalogItems(): Promise<CatalogItem[]> {
  const items = await listCatalogItems();
  return items.filter(isOperatorCatalogItem);
}

export function isOperatorInstance(
  inst: CatalogItemInstance,
  operatorCatalogIds: Set<string>,
): boolean {
  const cid = inst.spec?.catalog_item_id || '';
  return operatorCatalogIds.has(cid);
}

export async function listOperatorInstances(): Promise<{
  instances: CatalogItemInstance[];
  catalogItems: CatalogItem[];
}> {
  const [instances, catalogItems] = await Promise.all([
    listCatalogItemInstances(),
    listCatalogItems(),
  ]);
  const operatorIds = new Set(
    catalogItems.filter(isOperatorCatalogItem).map((c) => catalogItemId(c)).filter((id) => id !== '—'),
  );
  return {
    instances: instances.filter((i) => isOperatorInstance(i, operatorIds)),
    catalogItems: catalogItems.filter(isOperatorCatalogItem),
  };
}

/** Clusters with an olm-* DCM provider registered on the hub. */
export function listOlmProviderClusters(providers: Provider[]): string[] {
  const clusters: string[] = [];
  for (const p of providers) {
    const name = p.name || '';
    if (name.startsWith('olm-')) {
      clusters.push(name.slice(4));
    } else if (
      p.service_type === 'operator' ||
      p.serviceType === 'operator'
    ) {
      const n = p.name || p.display_name || '';
      if (n) clusters.push(n);
    }
  }
  return [...new Set(clusters)].sort();
}

export function instanceMatchesCluster(
  replication: PlacementTargetsResponse | undefined,
  cluster: string,
): boolean {
  if (!cluster || !replication?.targets?.length) return false;
  return replication.targets.some(
    (t) => t.provider === `olm-${cluster}` || t.provider?.endsWith(cluster),
  );
}

export async function listPolicies(): Promise<Policy[]> {
  const data = await request<unknown>('/policies');
  return listKey<Policy>(data, ['policies', 'items']);
}

/** Policy Manager expects snake_case request bodies (unlike SPM/catalog). */
function toPolicyBody(body: PolicyCreate | PolicyPatch): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (body.display_name !== undefined) out.display_name = body.display_name;
  if (body.policy_type !== undefined) out.policy_type = body.policy_type;
  if (body.rego_code !== undefined) out.rego_code = body.rego_code;
  if (body.description !== undefined) out.description = body.description;
  if (body.enabled !== undefined) out.enabled = body.enabled;
  if (body.priority !== undefined) out.priority = body.priority;
  if (body.label_selector !== undefined) out.label_selector = body.label_selector;
  if (body.cluster_requirements !== undefined) {
    out.cluster_requirements = body.cluster_requirements;
  }
  if (body.placement !== undefined) out.placement = body.placement;
  if (body.failover !== undefined) out.failover = body.failover;
  return out;
}

export async function previewPlacement(
  body: PlacementPreviewRequest,
): Promise<PlacementPreviewResponse> {
  return request<PlacementPreviewResponse>('/placement-preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function getPolicy(id: string): Promise<Policy> {
  return request<Policy>('/policies/' + encodeURIComponent(id));
}

export async function createPolicy(body: PolicyCreate, opts?: { id?: string }): Promise<Policy> {
  const qs = opts?.id ? `?id=${encodeURIComponent(opts.id)}` : '';
  return request<Policy>('/policies' + qs, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(toPolicyBody(body)),
  });
}

export async function updatePolicy(id: string, body: PolicyPatch): Promise<Policy> {
  return request<Policy>('/policies/' + encodeURIComponent(id), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/merge-patch+json' },
    body: JSON.stringify(toPolicyBody(body)),
  });
}

export async function deletePolicy(id: string): Promise<void> {
  await request<void>('/policies/' + encodeURIComponent(id), { method: 'DELETE' });
}

export function policyDisplayName(p: Policy): string {
  return p.display_name || '—';
}

export async function listServiceTypes(): Promise<ServiceType[]> {
  const data = await request<unknown>('/service-types');
  return listKey<ServiceType>(data, [
    'results',
    'service_types',
    'service-types',
    'items',
  ]).map((st) => ({
    ...st,
    name: st.service_type || st.name || st.id,
  }));
}

export function providerDisplayName(p: Provider): string {
  return p.display_name || p.displayName || p.name || '—';
}

export function providerHealth(p: Provider): string {
  return p.health_status || (p as { status?: string }).status || '—';
}

/** Cluster suffix from fleet provider names (e.g. k8s-lab-ext02 → lab-ext02). */
export function providerClusterFromName(name: string | undefined): string | undefined {
  if (!name) return undefined;
  const match = name.match(/^(?:k8s|helm|olm|kubevirt)-(.+)$/);
  return match?.[1];
}

/** OCM cluster name for a registered service provider. */
export function providerCluster(p: Provider, clusters: FleetCluster[] = []): string {
  const zone = p.metadata?.zone || p.zone;
  if (zone) return zone;
  const fromName = providerClusterFromName(p.name);
  if (fromName) return fromName;
  void clusters;
  return '—';
}

/** Fleet site label for a provider (metadata.site, or cluster fleet.site lookup). */
export function providerSite(
  p: Provider,
  clusters: FleetCluster[] = [],
  sites: FleetSite[] = [],
): string {
  const fromMeta = p.metadata?.site || p.metadata?.site_id;
  if (fromMeta) return fleetSiteLabelForId(sites, fromMeta);

  const clusterName = providerCluster(p, clusters);
  if (clusterName !== '—') {
    const cluster = clusters.find((c) => c.name === clusterName);
    if (cluster?.site) return fleetSiteLabelForId(sites, cluster.site);
  }
  return '—';
}

/** @deprecated Use providerCluster — column header is "Cluster" but DCM field is zone. */
export function providerZone(p: Provider, clusters: FleetCluster[] = []): string {
  return providerCluster(p, clusters);
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
  return s.spec?.edge?.edgeDomain || s.spec?.edgeDomain || '';
}

export function fleetSiteEffectiveId(s: FleetSite): string {
  return s.spec?.siteId || s.spec?.siteName || s.metadata?.name || '';
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

export async function listFleetClusters(): Promise<FleetCluster[]> {
  const data = await fleetSiteRequest<{ clusters?: FleetCluster[] }>('/clusters');
  return data.clusters ?? [];
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

export async function createFleetSite(body: FleetSiteCreate): Promise<FleetSite> {
  const edgeDomain = body.edgeDomain || body.edge?.edgeDomain;
  const wildcard = body.wildcard || body.edge?.wildcard;
  const exposureMode = body.exposureMode || body.edge?.exposureMode || 'none';
  const payload = {
    apiVersion: 'fleet.umkim.com/v1alpha1',
    kind: 'FleetSite',
    metadata: { name: body.siteName },
    spec: {
      siteName: body.siteName,
      orgId: body.orgId,
      displayName: body.displayName,
      provisioningMode: 'registry',
      region: body.region,
      edge: {
        edgeDomain,
        wildcard: wildcard || (edgeDomain ? `*.${edgeDomain}` : undefined),
        exposureMode,
        defaultExposureMode: body.defaultExposureMode || body.edge?.defaultExposureMode,
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
  const install = prov
    ? {
        k8s: !!prov.k8s,
        helm: !!prov.helm,
        kubevirt: !!prov.kubevirt,
      }
    : undefined;
  const kubevirtBridge =
    prov?.kubevirt && prov.kubevirtBridge?.enabled
      ? {
          enabled: true,
          bridgeName: prov.kubevirtBridge.bridgeName || 'n0p0_mgmt_nw',
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
      minRootDiskGi: body.minRootDiskGi ?? 40,
      clusterRegion: body.clusterRegion || 'fleet',
      clusterLabels: body.clusterLabels,
      clusterCapabilities: caps,
      storage: body.storage,
      domainBindings: body.domainBindings,
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

export async function listClusterProviders(name: string): Promise<ClusterProviderRow[]> {
  const data = await fleetSiteRequest<{ providers?: ClusterProviderRow[] }>(
    '/clusters/' + encodeURIComponent(name) + '/providers',
  );
  return data.providers ?? [];
}

export async function installClusterProvider(
  name: string,
  kind: ClusterProviderKind,
): Promise<void> {
  await fleetSiteRequest(
    '/clusters/' + encodeURIComponent(name) + '/providers/' + kind + '/install',
    { method: 'POST' },
  );
}

export async function deleteClusterProvider(
  name: string,
  kind: ClusterProviderKind,
): Promise<void> {
  await fleetSiteRequest(
    '/clusters/' + encodeURIComponent(name) + '/providers/' + kind + '/delete',
    { method: 'POST' },
  );
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
  );
  const text = await r.text();
  if (!r.ok) {
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
