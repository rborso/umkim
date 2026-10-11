import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  FormGroup,
  Label,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  Radio,
  Tab,
  Tabs,
  TabTitleText,
  TextArea,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { PlusIcon, SyncIcon, TrashIcon } from '@patternfly/react-icons';
import {
  createFleetApp,
  createFleetChart,
  deleteFleetApp,
  deleteFleetChart,
  fleetSiteEffectiveId,
  getLlamaCppProfileValues,
  listLlamaCppProfiles,
  patchFleetApp,
  uploadFleetChart,
} from '../api/client';
import { isLlamaCppChart } from '../apps/llamaCpp';
import { chartInstallProfile } from '../apps/chartProfile';
import { isRocky10VmChart, parseRockyDns, rockyVmValuesYaml } from '../apps/rockyVm';
import type { RockyVmNetwork, RockyVmNetworkMode } from '../apps/rockyVm';
import type {
  FleetApp,
  FleetAppCreate,
  FleetAppOIDCClient,
  FleetAppRoute,
  FleetAppRouteMode,
  FleetChart,
  FleetChartImagePolicy,
  FleetCluster,
  FleetSite,
} from '../api/types';

function parseOidcDefaults(json?: string): FleetAppOIDCClient | undefined {
  if (!json) return undefined;
  try {
    const d = JSON.parse(json);
    if (!d || typeof d !== 'object') return undefined;
    return d as FleetAppOIDCClient;
  } catch {
    return undefined;
  }
}
import { ErrorAlert } from '../components/ErrorAlert';
import { FleetModalSelect } from '../components/forms/FleetModalSelect';
import { LabeledField } from '../components/forms/LabeledField';
import { clusterIngressZone, isReverseProxyMethod } from '../edge/ingressMethod';
import { getFieldHelp } from '../help/fieldHelp';
import {
  useFleetAppsQuery,
  useFleetChartsQuery,
  useFleetClustersQuery,
  useLLMQuery,
  useFleetSitesQuery,
  useInvalidateFleet,
} from '../hooks/useFleetQueries';
import { useAuth } from '../auth/AuthContext';

const emptyForm = (clusterName = '', chart = 'whoami'): FleetAppCreate => ({
  clusterName,
  chart,
  version: '0.1.0',
  releaseName: chart.includes('/') ? chart.slice(chart.lastIndexOf('/') + 1) : chart,
  namespace: 'default',
  valuesYaml: '',
  route: { mode: 'none' },
});

type RockyForm = {
  cpu: string;
  memory: string;
  disk: string;
  sshKeys: string;
  netMode: RockyVmNetworkMode;
  multusNetwork: string;
  iface: string;
  staticIP: string;
  gateway: string;
  dns: string;
};

const newRockyForm = (): RockyForm => ({
  cpu: '2',
  memory: '2Gi',
  disk: '20Gi',
  sshKeys: '',
  netMode: 'pod',
  multusNetwork: 'default/mgmt-l2',
  iface: 'enp1s0',
  staticIP: '',
  gateway: '',
  dns: '',
});

function rockyNetworkFromForm(f: RockyForm): RockyVmNetwork | undefined {
  if (f.netMode !== 'lan') {
    return undefined;
  }
  return {
    mode: 'lan',
    multusNetwork: f.multusNetwork,
    interface: f.iface,
    staticIP: f.staticIP,
    gateway: f.gateway,
    dns: parseRockyDns(f.dns),
  };
}

function rockyValuesFromForm(f: RockyForm): string {
  return rockyVmValuesYaml(f.cpu, f.memory, f.disk, f.sshKeys, rockyNetworkFromForm(f));
}

/** Route target wiring for custom charts (carried through install and edit). */
function updateRoute(
  routes: FleetAppRoute[] | undefined,
  index: number,
  patch: Partial<FleetAppRoute>,
): FleetAppRoute[] {
  const list = [...(routes || [])];
  list[index] = { ...list[index], ...patch };
  return list;
}

function removeRouteAt(routes: FleetAppRoute[] | undefined, index: number): FleetAppRoute[] {
  return (routes || []).filter((_, i) => i !== index);
}

function routeWiringFromRoute(route?: FleetAppRoute): Partial<FleetAppRoute> {
  if (!route) return {};
  const wiring: Partial<FleetAppRoute> = {};
  if (route.serviceName?.trim()) wiring.serviceName = route.serviceName.trim();
  if (route.servicePort && route.servicePort > 0) wiring.servicePort = route.servicePort;
  if (route.serviceNamespace?.trim()) wiring.serviceNamespace = route.serviceNamespace.trim();
  if (route.healthPath?.trim()) wiring.healthPath = route.healthPath.trim();
  if (route.requireAuth) wiring.requireAuth = true;
  if (route.requireAuth && route.requireAuthGroups?.length) {
    wiring.requireAuthGroups = route.requireAuthGroups;
  }
  return wiring;
}

function parseAuthGroups(value: string): string[] | undefined {
  const groups = value
    .split(',')
    .map((g) => g.trim())
    .filter(Boolean);
  return groups.length ? groups : undefined;
}

function clusterSiteDomain(sites: FleetSite[], cluster?: FleetCluster): string {
  if (!cluster?.site || isReverseProxyMethod(cluster.method)) return '';
  const site = sites.find(
    (s) => fleetSiteEffectiveId(s) === cluster.site || s.metadata?.name === cluster.site,
  );
  if (!site) return '';
  return clusterIngressZone(site, cluster);
}

function suggestedRouteHost(release: string, domain: string): string {
  const rel = release.trim();
  const zone = domain.trim().replace(/^\./, '');
  if (!rel || !zone) return '';
  return `${rel}.${zone}`;
}

function appRouteHostname(app: FleetApp, hubEdgeDomain?: string): string {
  if (app.route?.mode === 'hub') {
    return app.route.hubHost || suggestedRouteHost(app.releaseName, hubEdgeDomain || '');
  }
  if (app.route?.mode === 'site') {
    return app.route.siteHost || '';
  }
  return '';
}

function phaseColor(phase?: string): 'green' | 'red' | 'blue' | 'grey' | 'orange' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Failed':
      return 'red';
    case 'Installing':
    case 'Publishing':
    case 'Fetching':
    case 'Deleting':
      return 'blue';
    case 'Pending':
      return 'orange';
    default:
      return 'grey';
  }
}

function imagePolicyLabel(p?: string): string {
  switch (p) {
    case 'copy':
      return 'images copied';
    case 'proxy':
      return 'pull-through';
    default:
      return 'external';
  }
}

export function AppsPage() {
  const { can } = useAuth();
  const canPublish = can('charts', 'create');
  const { invalidateApps, invalidateCharts, invalidateClusters, invalidateSites } = useInvalidateFleet();
  const appsQuery = useFleetAppsQuery(10_000);
  const chartsQuery = useFleetChartsQuery(10_000);
  const clustersQuery = useFleetClustersQuery(30_000);
  const sitesQuery = useFleetSitesQuery(30_000);
  const apps = appsQuery.data?.apps ?? [];
  const defaults = appsQuery.data?.defaults ?? { chartPrefix: '' };
  const charts = chartsQuery.data ?? [];
  const clusters = clustersQuery.data ?? [];
  const sites = sitesQuery.data ?? [];
  const loading =
    appsQuery.isLoading || chartsQuery.isLoading || clustersQuery.isLoading || sitesQuery.isLoading;
  const fetching =
    appsQuery.isFetching || chartsQuery.isFetching || clustersQuery.isFetching || sitesQuery.isFetching;
  const error = appsQuery.error || chartsQuery.error || clustersQuery.error || sitesQuery.error;
  const [tab, setTab] = useState<string | number>(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [addChartOpen, setAddChartOpen] = useState(false);
  const [form, setForm] = useState<FleetAppCreate>(emptyForm());
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [chartSource, setChartSource] = useState<'upload' | 'oci' | 'http'>('oci');
  const [chartOrigin, setChartOrigin] = useState('');
  const [chartVersion, setChartVersion] = useState('');
  const [chartPolicy, setChartPolicy] = useState<FleetChartImagePolicy>('external');
  const [chartFile, setChartFile] = useState<File | null>(null);
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [deleteTarget, setDeleteTarget] = useState<FleetApp | null>(null);
  const [deleteChartTarget, setDeleteChartTarget] = useState<FleetChart | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [rocky, setRocky] = useState<RockyForm>(newRockyForm());
  const [llamaProfiles, setLlamaProfiles] = useState<{ id: string; description?: string }[]>([]);
  const [llamaProfileId, setLlamaProfileId] = useState('');
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<FleetApp | null>(null);
  const [editForm, setEditForm] = useState({
    version: '',
    valuesYaml: '',
    routeMode: 'none',
    routeHost: '',
    llmBackend: false,
  });
  const [registerLiteLLM, setRegisterLiteLLM] = useState(false);
  const llmQuery = useLLMQuery();
  const hubLlmEnabled = !!llmQuery.data?.hubEnabled;

  const readyClusters = useMemo(
    () =>
      clusters.filter(
        (c) => c.provisioning_phase === 'Ready' && !c.management && c.name !== 'umkim-hub',
      ),
    [clusters],
  );

  const readyCharts = useMemo(() => charts.filter((c) => c.phase === 'Ready'), [charts]);
  const chartProfile = useMemo(() => chartInstallProfile(form.chart), [form.chart]);

  const selectedCluster = useMemo(
    () => readyClusters.find((c) => c.name === form.clusterName),
    [readyClusters, form.clusterName],
  );

  const routeHostPlaceholder = useMemo(() => {
    const release = form.releaseName || '';
    if (form.route?.mode === 'hub') {
      return suggestedRouteHost(release, defaults.hubEdgeDomain || '');
    }
    if (form.route?.mode === 'site') {
      const zone = clusterSiteDomain(sites, selectedCluster);
      return zone ? suggestedRouteHost(release, zone) : '';
    }
    return '';
  }, [form.releaseName, form.route?.mode, defaults.hubEdgeDomain, sites, selectedCluster]);

  const siteRouteNeedsManualHost =
    form.route?.mode === 'site' && isReverseProxyMethod(selectedCluster?.method);

  const routeHostValue =
    form.route?.mode === 'site' ? form.route?.siteHost || '' : form.route?.hubHost || '';

  const load = () => {
    void invalidateApps();
    void invalidateCharts();
    void invalidateClusters();
    void invalidateSites();
  };

  const loadLlamaProfiles = async () => {
    try {
      const list = await listLlamaCppProfiles();
      setLlamaProfiles(list);
      if (list.length && !llamaProfileId) {
        setLlamaProfileId(list[0].id);
      }
    } catch {
      setLlamaProfiles([]);
    }
  };

  const applyLlamaProfile = async (profileId: string) => {
    if (!profileId) return;
    const yaml = await getLlamaCppProfileValues(profileId);
    setForm((f) => ({ ...f, valuesYaml: yaml, namespace: f.namespace === 'default' ? 'llm' : f.namespace }));
  };

  const openInstall = () => {
    setActionError(null);
    void loadLlamaProfiles();
    const first = readyCharts[0]?.name || 'whoami';
    const firstProfile = chartInstallProfile(first);
    const rockyDefaults = newRockyForm();
    const firstOidc = parseOidcDefaults(readyCharts[0]?.oidcDefaults);
    setRocky(rockyDefaults);
    setRegisterLiteLLM(false);
    setOidcEnabled(!!firstOidc);
    setForm({
      ...emptyForm(readyClusters[0]?.name || '', first),
      version: readyCharts[0]?.version || '0.1.0',
      namespace: firstProfile.defaultNamespace || 'default',
      valuesYaml: firstProfile.kind === 'kubevirt' ? rockyValuesFromForm(rockyDefaults) : '',
      route: { mode: 'none' },
      oidcClient: firstOidc,
      oidcClientId: firstOidc?.clientId,
    });
    setModalOpen(true);
  };

  const openAddChart = () => {
    setActionError(null);
    setChartSource('oci');
    setChartOrigin('');
    setChartVersion('');
    setChartPolicy('external');
    setChartFile(null);
    setAddChartOpen(true);
  };

  const submit = async () => {
    setSaving(true);
    setActionError(null);
    try {
      const mode = form.route?.mode || 'none';
      const route = {
        mode,
        ...(mode === 'hub' && form.route?.hubHost?.trim()
          ? { hubHost: form.route.hubHost.trim() }
          : {}),
        ...(mode === 'site' && form.route?.siteHost?.trim()
          ? { siteHost: form.route.siteHost.trim() }
          : {}),
        ...(mode !== 'none' ? routeWiringFromRoute(form.route) : {}),
      };
      const llama = isLlamaCppChart(form.chart);
      const multiRoutes = form.routes && form.routes.length > 0 ? form.routes : undefined;
      const body: FleetAppCreate = {
        ...form,
        name: form.releaseName || form.name,
        clusterName: form.clusterName,
        chart: form.chart,
        version: form.version || undefined,
        releaseName: form.releaseName,
        namespace: form.namespace || 'default',
        valuesYaml: form.valuesYaml || undefined,
        route: multiRoutes ? undefined : route,
        ...(multiRoutes ? { routes: multiRoutes } : {}),
        ...(llama ? { llmBackend: registerLiteLLM } : {}),
        oidcClient: oidcEnabled ? form.oidcClient : undefined,
        oidcClientId: oidcEnabled ? form.oidcClient?.clientId : undefined,
      };
      await createFleetApp(body);
      setModalOpen(false);
      load();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const submitChart = async () => {
    setSaving(true);
    setActionError(null);
    try {
      if (chartSource === 'upload') {
        if (!chartFile) {
          throw new Error('Choose a .tgz chart package');
        }
        const fd = new FormData();
        fd.append('chart', chartFile);
        fd.append('imagePolicy', chartPolicy || 'external');
        if (chartVersion) fd.append('version', chartVersion);
        await uploadFleetChart(fd);
      } else {
        const origin =
          chartSource === 'http' || chartOrigin.startsWith('http')
            ? chartOrigin
            : chartOrigin.startsWith('oci://')
              ? chartOrigin
              : `oci://${chartOrigin.replace(/^oci:\/\//, '')}`;
        await createFleetChart({
          origin,
          version: chartVersion || undefined,
          imagePolicy: chartPolicy,
        });
      }
      setAddChartOpen(false);
      setTab(1);
      load();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setActionError(null);
    try {
      await deleteFleetApp(deleteTarget.name);
      setDeleteTarget(null);
      load();
    } catch (e) {
      setActionError(e);
    } finally {
      setDeleting(false);
    }
  };

  const confirmDeleteChart = async () => {
    if (!deleteChartTarget) return;
    setDeleting(true);
    setActionError(null);
    try {
      await deleteFleetChart(deleteChartTarget.name);
      setDeleteChartTarget(null);
      load();
    } catch (e) {
      setActionError(e);
    } finally {
      setDeleting(false);
    }
  };

  const onLibrarySelect = (name: string) => {
    const c = readyCharts.find((x) => x.name === name);
    const profile = chartInstallProfile(name);
    const isRocky = profile.kind === 'kubevirt';
    const llama = profile.kind === 'llm';
    const rockyDefaults = isRocky ? newRockyForm() : null;
    if (rockyDefaults) {
      setRocky(rockyDefaults);
    }
    const oidc = parseOidcDefaults(c?.oidcDefaults);
    setOidcEnabled(!!oidc);
    setForm((f) => ({
      ...f,
      chart: name,
      version: c?.version || f.version,
      releaseName: f.releaseName && f.releaseName !== f.chart ? f.releaseName : name,
      namespace: profile.defaultNamespace || f.namespace,
      valuesYaml: rockyDefaults ? rockyValuesFromForm(rockyDefaults) : f.valuesYaml,
      route: profile.supportsRoute ? f.route : { mode: 'none' },
      oidcClient: oidc,
      oidcClientId: oidc?.clientId,
    }));
    if (llama && llamaProfileId) {
      void applyLlamaProfile(llamaProfileId).catch((e) => setActionError(e));
    }
  };

  const openEdit = (app: FleetApp) => {
    setActionError(null);
    setEditTarget(app);
    setEditForm({
      version: app.version || '',
      valuesYaml: app.valuesYaml || '',
      routeMode: app.route?.mode || 'none',
      routeHost: app.route?.hubHost || app.route?.siteHost || '',
      llmBackend: app.llmBackend !== false,
    });
    setEditOpen(true);
  };

  const submitEdit = async () => {
    if (!editTarget) return;
    setSaving(true);
    setActionError(null);
    try {
      const routeMode: FleetAppRouteMode =
        editForm.routeMode === 'hub' ? 'hub' : editForm.routeMode === 'site' ? 'site' : 'none';
      const route = {
        mode: routeMode,
        ...(routeMode === 'hub' ? { hubHost: editForm.routeHost.trim() || undefined } : {}),
        ...(routeMode === 'site' ? { siteHost: editForm.routeHost.trim() || undefined } : {}),
        ...(routeMode !== 'none' ? routeWiringFromRoute(editTarget.route) : {}),
      };
      await patchFleetApp(editTarget.name, {
        version: editForm.version || undefined,
        valuesYaml: editForm.valuesYaml || undefined,
        route,
        ...(isLlamaCppChart(editTarget.chart) ? { llmBackend: editForm.llmBackend } : {}),
      });
      setEditOpen(false);
      setEditTarget(null);
      load();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const updateRocky = (patch: Partial<RockyForm>) => {
    const next = { ...rocky, ...patch };
    setRocky(next);
    setForm((f) => ({ ...f, valuesYaml: rockyValuesFromForm(next) }));
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Apps
        </Title>
        <p style={{ marginTop: '0.5rem', color: '#6a6e73' }}>
          Publish a chart to the hub library once, then install it onto a Ready cluster you select.
        </p>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="primary" icon={<PlusIcon />} onClick={openInstall} isDisabled={!readyClusters.length}>
                Install chart
              </Button>
            </ToolbarItem>
            {canPublish ? (
              <ToolbarItem>
                <Button variant="secondary" onClick={openAddChart}>
                  Add chart
                </Button>
              </ToolbarItem>
            ) : null}
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={() => void load()} isDisabled={fetching || loading}>
                Refresh
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Link to="/apps/llama-profiles">Llama profiles</Link>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
        <ErrorAlert error={error} />
        {!readyClusters.length && !loading ? (
          <p style={{ marginTop: '0.75rem', color: '#6a6e73' }}>
            Join a Rocky 10 cluster and wait until it is Ready, then install a chart here.
          </p>
        ) : null}
      </PageSection>
      <PageSection>
        <Tabs activeKey={tab} onSelect={(_e, k) => setTab(k)}>
          <Tab eventKey={0} title={<TabTitleText>Installed</TabTitleText>}>
            <Table aria-label="Fleet apps" variant="compact">
              <Thead>
                <Tr>
                  <Th>Name</Th>
                  <Th>Cluster</Th>
                  <Th>Chart</Th>
                  <Th>Release</Th>
                  <Th>Namespace</Th>
                  <Th>Route</Th>
                  <Th>Hostname</Th>
                  <Th>Phase</Th>
                  <Th>Actions</Th>
                </Tr>
              </Thead>
              <Tbody>
                {apps.map((app) => (
                  <Tr key={app.name}>
                    <Td>{app.name}</Td>
                    <Td>{app.clusterName}</Td>
                    <Td>
                      <code style={{ fontSize: '0.8rem' }}>{app.chart}</code>
                    </Td>
                    <Td>{app.releaseName}</Td>
                    <Td>{app.namespace}</Td>
                    <Td>{app.route?.mode || 'none'}</Td>
                    <Td>
                      {app.route?.mode && app.route.mode !== 'none' ? (
                        <code style={{ fontSize: '0.8rem' }}>
                          {appRouteHostname(app, defaults.hubEdgeDomain) || '—'}
                        </code>
                      ) : (
                        '—'
                      )}
                    </Td>
                    <Td>
                      <Label color={phaseColor(app.phase)}>{app.phase || 'Pending'}</Label>
                      {app.message ? (
                        <div style={{ fontSize: '0.75rem', color: '#6a6e73', marginTop: '0.25rem' }}>{app.message}</div>
                      ) : null}
                    </Td>
                    <Td>
                      <Button variant="link" onClick={() => openEdit(app)} isDisabled={app.phase === 'Installing' || app.phase === 'Deleting'}>
                        Edit
                      </Button>
                      <Button variant="link" icon={<TrashIcon />} onClick={() => setDeleteTarget(app)} isDanger>
                        Remove
                      </Button>
                    </Td>
                  </Tr>
                ))}
                {!apps.length && !loading ? (
                  <Tr>
                    <Td colSpan={9}>No apps installed.</Td>
                  </Tr>
                ) : null}
              </Tbody>
            </Table>
          </Tab>
          <Tab eventKey={1} title={<TabTitleText>Library</TabTitleText>}>
            <Table aria-label="Chart library" variant="compact">
              <Thead>
                <Tr>
                  <Th>Chart</Th>
                  <Th>Version</Th>
                  <Th>Source</Th>
                  <Th>Images</Th>
                  <Th>Phase</Th>
                  <Th>Actions</Th>
                </Tr>
              </Thead>
              <Tbody>
                {charts.map((c) => (
                  <Tr key={c.name}>
                    <Td>
                      <code>{c.name}</code>
                    </Td>
                    <Td>{c.version || (c.versions && c.versions[0]) || '—'}</Td>
                    <Td>{c.source || '—'}</Td>
                    <Td>
                      <Label color={c.imagePolicy === 'copy' ? 'blue' : 'grey'}>{imagePolicyLabel(c.imagePolicy)}</Label>
                    </Td>
                    <Td>
                      <Label color={phaseColor(c.phase)}>{c.phase || 'Pending'}</Label>
                      {c.message ? (
                        <div style={{ fontSize: '0.75rem', color: '#6a6e73', marginTop: '0.25rem' }}>{c.message}</div>
                      ) : null}
                    </Td>
                    <Td>
                      {canPublish ? (
                        <Button variant="link" icon={<TrashIcon />} onClick={() => setDeleteChartTarget(c)} isDanger>
                          Remove
                        </Button>
                      ) : null}
                    </Td>
                  </Tr>
                ))}
                {!charts.length && !loading ? (
                  <Tr>
                    <Td colSpan={6}>No library charts yet. Add a chart or wait for Harbor import.</Td>
                  </Tr>
                ) : null}
              </Tbody>
            </Table>
          </Tab>
        </Tabs>
      </PageSection>

      <Modal
        variant={ModalVariant.medium}
        title="Install Helm chart"
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        actions={[
          <Button key="install" variant="primary" onClick={() => void submit()} isDisabled={saving || !form.clusterName || !form.chart}>
            {saving ? 'Installing…' : 'Install'}
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setModalOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <ErrorAlert error={actionError} />
          <LabeledField label="Cluster" fieldId="app-cluster" isRequired help={getFieldHelp('apps.clusterName')}>
            <FleetModalSelect
              id="app-cluster"
              value={form.clusterName}
              placeholder="Select a Ready cluster"
              onChange={(v) => setForm((f) => ({ ...f, clusterName: v }))}
              options={[
                { value: '', label: 'Select a Ready cluster' },
                ...readyClusters.map((c) => ({ value: c.name, label: c.name })),
              ]}
            />
          </LabeledField>
          <LabeledField label="Chart" fieldId="app-chart" isRequired help={getFieldHelp('apps.libraryChart')}>
            <FleetModalSelect
              id="app-chart"
              value={form.chart}
              placeholder="Select a library chart"
              onChange={(v) => onLibrarySelect(v)}
              options={[
                { value: '', label: 'Select a library chart' },
                ...readyCharts.map((c) => ({
                  value: c.name,
                  label: `${c.name}${c.version ? ` (${c.version})` : ''}`,
                })),
              ]}
            />
          </LabeledField>
          <LabeledField label="Chart URL or short name" fieldId="app-chart-custom" help={getFieldHelp('apps.chart')}>
            <TextInput
              id="app-chart-custom"
              value={form.chart}
              onChange={(_e, v) => {
                const profile = chartInstallProfile(v);
                setForm((f) => ({
                  ...f,
                  chart: v,
                  valuesYaml: profile.kind === 'kubevirt' ? rockyValuesFromForm(rocky) : f.valuesYaml,
                  route: profile.supportsRoute ? f.route : { mode: 'none' },
                }));
              }}
            />
          </LabeledField>
          {isRocky10VmChart(form.chart) ? (
            <>
              <LabeledField label="vCPU cores" fieldId="app-rocky-cpu" help={getFieldHelp('apps.rockyCpu')}>
                <TextInput
                  id="app-rocky-cpu"
                  type="number"
                  min={1}
                  value={rocky.cpu}
                  onChange={(_e, v) => updateRocky({ cpu: v })}
                />
              </LabeledField>
              <LabeledField label="Memory" fieldId="app-rocky-memory" help={getFieldHelp('apps.rockyMemory')}>
                <TextInput
                  id="app-rocky-memory"
                  value={rocky.memory}
                  onChange={(_e, v) => updateRocky({ memory: v })}
                />
              </LabeledField>
              <LabeledField label="Disk size" fieldId="app-rocky-disk" help={getFieldHelp('apps.rockyDisk')}>
                <TextInput
                  id="app-rocky-disk"
                  value={rocky.disk}
                  onChange={(_e, v) => updateRocky({ disk: v })}
                />
              </LabeledField>
              <LabeledField
                label="SSH public key(s)"
                fieldId="app-rocky-ssh"
                help={getFieldHelp('apps.rockySshKeys')}
              >
                <TextArea
                  id="app-rocky-ssh"
                  value={rocky.sshKeys}
                  onChange={(_e, v) => updateRocky({ sshKeys: v })}
                  rows={3}
                  placeholder="ssh-ed25519 AAAA... user@host (one per line)"
                />
              </LabeledField>
              <LabeledField label="Network" fieldId="app-rocky-net-mode" help={getFieldHelp('apps.rockyNetMode')}>
                <FleetModalSelect
                  id="app-rocky-net-mode"
                  value={rocky.netMode}
                  onChange={(v) => updateRocky({ netMode: v as RockyVmNetworkMode })}
                  options={[
                    { value: 'pod', label: 'pod — NAT on the cluster network' },
                    { value: 'lan', label: 'lan — Multus L2 (DHCP or static IP)' },
                  ]}
                />
              </LabeledField>
              {rocky.netMode === 'lan' ? (
                <LabeledField
                  label="Multus network"
                  fieldId="app-rocky-multus"
                  help={getFieldHelp('apps.rockyMultusNetwork')}
                >
                  <TextInput
                    id="app-rocky-multus"
                    value={rocky.multusNetwork}
                    onChange={(_e, v) => updateRocky({ multusNetwork: v })}
                    placeholder="default/mgmt-l2"
                  />
                </LabeledField>
              ) : null}
              {rocky.netMode === 'lan' ? (
                <LabeledField
                  label="Static IPv4 (CIDR)"
                  fieldId="app-rocky-static-ip"
                  help={getFieldHelp('apps.rockyStaticIP')}
                >
                  <TextInput
                    id="app-rocky-static-ip"
                    value={rocky.staticIP}
                    onChange={(_e, v) => updateRocky({ staticIP: v })}
                    placeholder="172.16.10.150/24 (empty = DHCP)"
                  />
                </LabeledField>
              ) : null}
              {rocky.netMode === 'lan' && rocky.staticIP.trim() ? (
                <LabeledField label="Gateway" fieldId="app-rocky-gateway" help={getFieldHelp('apps.rockyGateway')}>
                  <TextInput
                    id="app-rocky-gateway"
                    value={rocky.gateway}
                    onChange={(_e, v) => updateRocky({ gateway: v })}
                    placeholder="172.16.10.1"
                  />
                </LabeledField>
              ) : null}
              {rocky.netMode === 'lan' && rocky.staticIP.trim() ? (
                <LabeledField label="DNS servers" fieldId="app-rocky-dns" help={getFieldHelp('apps.rockyDns')}>
                  <TextInput
                    id="app-rocky-dns"
                    value={rocky.dns}
                    onChange={(_e, v) => updateRocky({ dns: v })}
                    placeholder="172.16.10.1, 1.1.1.1"
                  />
                </LabeledField>
              ) : null}
              {rocky.netMode === 'lan' && rocky.staticIP.trim() ? (
                <LabeledField
                  label="Guest interface"
                  fieldId="app-rocky-iface"
                  help={getFieldHelp('apps.rockyInterface')}
                >
                  <TextInput
                    id="app-rocky-iface"
                    value={rocky.iface}
                    onChange={(_e, v) => updateRocky({ iface: v })}
                    placeholder="enp1s0"
                  />
                </LabeledField>
              ) : null}
            </>
          ) : null}
          {isLlamaCppChart(form.chart) ? (
            <LabeledField label="Llama values profile" fieldId="app-llama-profile" help={getFieldHelp('apps.llamaProfile')}>
              <FleetModalSelect
                id="app-llama-profile"
                value={llamaProfileId}
                placeholder="Select a profile"
                onChange={(v) => {
                  setLlamaProfileId(v);
                  void applyLlamaProfile(v).catch((e) => setActionError(e));
                }}
                options={[
                  { value: '', label: 'Select a profile' },
                  ...llamaProfiles.map((p) => ({
                    value: p.id,
                    label: p.description ? `${p.id} — ${p.description}` : p.id,
                  })),
                ]}
              />
            </LabeledField>
          ) : null}
          {isLlamaCppChart(form.chart) ? (
            <div
              style={{
                margin: '0.75rem 0',
                padding: '0.75rem 1rem',
                border: '1px solid var(--pf-v5-global--BorderColor--100)',
                borderRadius: 'var(--pf-v5-global--BorderRadius--sm)',
              }}
            >
              <Title headingLevel="h4" size="md" style={{ marginBottom: '0.5rem' }}>
                Hub LiteLLM proxy
              </Title>
              <Checkbox
                id="app-llm-backend"
                label="Add to LiteLLM proxy when install is Ready"
                isChecked={registerLiteLLM}
                onChange={(_e, checked) => setRegisterLiteLLM(checked)}
                description={
                  hubLlmEnabled
                    ? 'Optional. Registers this llama-cpp release as a hub LiteLLM backend after Helm reaches Ready.'
                    : 'Hub LiteLLM is disabled — enable it under Infrastructure → LLM platform first.'
                }
              />
              {registerLiteLLM && !hubLlmEnabled ? (
                <Alert variant="warning" isInline title="LiteLLM proxy is off" style={{ marginTop: '0.5rem' }}>
                  Enable the hub proxy on <Link to="/infrastructure">Infrastructure</Link> before clients can use this
                  backend.
                </Alert>
              ) : null}
            </div>
          ) : null}
          <LabeledField label="Version" fieldId="app-version" help={getFieldHelp('apps.version')}>
            <TextInput
              id="app-version"
              value={form.version || ''}
              onChange={(_e, v) => setForm((f) => ({ ...f, version: v }))}
            />
          </LabeledField>
          <LabeledField label="Release name" fieldId="app-release" isRequired help={getFieldHelp('apps.releaseName')}>
            <TextInput
              id="app-release"
              value={form.releaseName || ''}
              onChange={(_e, v) => setForm((f) => ({ ...f, releaseName: v }))}
            />
          </LabeledField>
          <LabeledField label="Namespace" fieldId="app-ns" help={getFieldHelp('apps.namespace')}>
            <TextInput
              id="app-ns"
              value={form.namespace || 'default'}
              onChange={(_e, v) => setForm((f) => ({ ...f, namespace: v }))}
            />
          </LabeledField>
          {chartProfile.supportsRoute ? (
            <>
              <LabeledField label="Route" fieldId="app-route" help={getFieldHelp('apps.routeMode')}>
                <FleetModalSelect
                  id="app-route"
                  value={form.route?.mode || 'none'}
                  onChange={(v) =>
                    setForm((f) => ({ ...f, route: { ...f.route, mode: v as FleetAppRouteMode } }))
                  }
                  options={[
                    { value: 'none', label: 'none — no fleet-managed ingress' },
                    { value: 'hub', label: 'hub — Traefik on the hub' },
                    { value: 'site', label: 'site — spoke ingress' },
                  ]}
                />
              </LabeledField>
              {form.route?.mode === 'hub' || form.route?.mode === 'site' ? (
                <LabeledField
                  label={form.route?.mode === 'site' ? 'Site hostname' : 'Hub hostname'}
                  fieldId="app-route-host"
                  help={getFieldHelp(form.route?.mode === 'site' ? 'apps.siteHost' : 'apps.hubHost')}
                >
                  <TextInput
                    id="app-route-host"
                    value={routeHostValue}
                    placeholder={routeHostPlaceholder}
                    onChange={(_e, v) =>
                      setForm((f) => ({
                        ...f,
                        route:
                          f.route?.mode === 'site'
                            ? { ...f.route, siteHost: v, hubHost: undefined }
                            : { ...f.route, hubHost: v, siteHost: undefined },
                      }))
                    }
                  />
                  {siteRouteNeedsManualHost ? (
                    <p style={{ fontSize: '0.85rem', marginTop: '0.35rem' }}>
                      Reverse-proxy clusters have no site DNS zone — enter the public hostname manually.
                    </p>
                  ) : null}
                </LabeledField>
              ) : null}
              {chartProfile.kind === 'custom' &&
              (form.route?.mode === 'hub' || form.route?.mode === 'site') ? (
                <div
                  style={{
                    margin: '0.75rem 0',
                    padding: '0.75rem 1rem',
                    border: '1px solid var(--pf-v5-global--BorderColor--100)',
                    borderRadius: 'var(--pf-v5-global--BorderRadius--sm)',
                  }}
                >
                  <Title headingLevel="h4" size="md" style={{ marginBottom: '0.25rem' }}>
                    Route wiring
                  </Title>
                  <p style={{ fontSize: '0.85rem', marginBottom: '0.5rem' }}>
                    The chart must create a Kubernetes Service. The route targets{' '}
                    <code>serviceName:servicePort</code> in <code>serviceNamespace</code> and health-checks{' '}
                    <code>healthPath</code>. Make sure your Values create a matching Service.
                  </p>
                  <LabeledField
                    label="Service name"
                    fieldId="app-route-svc"
                    help={getFieldHelp('apps.routeServiceName')}
                  >
                    <TextInput
                      id="app-route-svc"
                      value={form.route?.serviceName || ''}
                      onChange={(_e, v) => setForm((f) => ({ ...f, route: { ...f.route, serviceName: v } }))}
                      placeholder={form.releaseName || 'app'}
                    />
                  </LabeledField>
                  <LabeledField
                    label="Service port"
                    fieldId="app-route-port"
                    help={getFieldHelp('apps.routeServicePort')}
                  >
                    <TextInput
                      id="app-route-port"
                      type="number"
                      min={1}
                      value={form.route?.servicePort ?? ''}
                      onChange={(_e, v) => {
                        const n = Number.parseInt(v, 10);
                        setForm((f) => ({
                          ...f,
                          route: {
                            ...f.route,
                            servicePort: Number.isFinite(n) && n > 0 ? n : undefined,
                          },
                        }));
                      }}
                      placeholder="80"
                    />
                  </LabeledField>
                  <LabeledField
                    label="Service namespace"
                    fieldId="app-route-svcns"
                    help={getFieldHelp('apps.routeServiceNamespace')}
                  >
                    <TextInput
                      id="app-route-svcns"
                      value={form.route?.serviceNamespace || ''}
                      onChange={(_e, v) =>
                        setForm((f) => ({ ...f, route: { ...f.route, serviceNamespace: v } }))
                      }
                      placeholder={form.namespace || 'default'}
                    />
                  </LabeledField>
                  <LabeledField
                    label="Health path"
                    fieldId="app-route-health"
                    help={getFieldHelp('apps.routeHealthPath')}
                  >
                    <TextInput
                      id="app-route-health"
                      value={form.route?.healthPath || ''}
                      onChange={(_e, v) => setForm((f) => ({ ...f, route: { ...f.route, healthPath: v } }))}
                      placeholder="/"
                    />
                  </LabeledField>
                  <Checkbox
                    id="app-route-require-auth"
                    label="Require fleet ingress auth"
                    isChecked={!!form.route?.requireAuth}
                    onChange={(_e, checked) =>
                      setForm((f) => ({ ...f, route: { ...f.route, requireAuth: checked } }))
                    }
                  />
                  {form.route?.requireAuth ? (
                    <LabeledField
                      label="Tenant groups (comma-separated)"
                      fieldId="app-route-groups"
                    >
                      <TextInput
                        id="app-route-groups"
                        value={(form.route?.requireAuthGroups || []).join(', ')}
                        onChange={(_e, v) =>
                          setForm((f) => ({
                            ...f,
                            route: { ...f.route, requireAuthGroups: parseAuthGroups(v) },
                          }))
                        }
                        placeholder="tenant-a, tenant-b"
                      />
                    </LabeledField>
                  ) : null}
                </div>
              ) : null}
            </>
          ) : null}
          <FormGroup fieldId="app-multi-route">
            <Checkbox
              id="app-multi-route"
              label="Multiple routes (one per service; multi-service charts)"
              isChecked={form.routes !== undefined}
              onChange={(_e, c) =>
                setForm((f) => ({
                  ...f,
                  routes: c
                    ? f.routes ?? [{ key: 'app1', mode: 'site', siteHost: '', requireAuth: true }]
                    : undefined,
                }))
              }
            />
          </FormGroup>
          {form.routes ? (
            <>
              {form.routes.map((r, i) => (
                <div
                  key={i}
                  style={{
                    margin: '0.5rem 0',
                    padding: '0.5rem 0.75rem',
                    border: '1px solid var(--pf-v5-global--BorderColor--100)',
                    borderRadius: 'var(--pf-v5-global--BorderRadius--sm)',
                  }}
                >
                  <LabeledField label={`Route ${i + 1} key`} fieldId={`app-routes-${i}-key`}>
                    <TextInput
                      id={`app-routes-${i}-key`}
                      value={r.key || ''}
                      onChange={(_e, v) => setForm((f) => ({ ...f, routes: updateRoute(f.routes, i, { key: v }) }))}
                      placeholder="sonarr"
                    />
                  </LabeledField>
                  <LabeledField label="Mode" fieldId={`app-routes-${i}-mode`}>
                    <FleetModalSelect
                      id={`app-routes-${i}-mode`}
                      value={r.mode || 'site'}
                      onChange={(v) =>
                        setForm((f) => ({
                          ...f,
                          routes: updateRoute(f.routes, i, { mode: v as FleetAppRouteMode }),
                        }))
                      }
                      options={[
                        { value: 'site', label: 'site — spoke ingress' },
                        { value: 'hub', label: 'hub — Traefik on the hub' },
                        { value: 'none', label: 'none' },
                      ]}
                    />
                  </LabeledField>
                  <LabeledField
                    label={r.mode === 'hub' ? 'Hub hostname' : 'Site hostname'}
                    fieldId={`app-routes-${i}-host`}
                  >
                    <TextInput
                      id={`app-routes-${i}-host`}
                      value={(r.mode === 'hub' ? r.hubHost : r.siteHost) || ''}
                      onChange={(_e, v) =>
                        setForm((f) => ({
                          ...f,
                          routes: updateRoute(f.routes, i, r.mode === 'hub' ? { hubHost: v } : { siteHost: v }),
                        }))
                      }
                      placeholder="sonarr.devtest.otthol.hu"
                    />
                  </LabeledField>
                  <LabeledField label="Service name" fieldId={`app-routes-${i}-svc`}>
                    <TextInput
                      id={`app-routes-${i}-svc`}
                      value={r.serviceName || ''}
                      onChange={(_e, v) =>
                        setForm((f) => ({ ...f, routes: updateRoute(f.routes, i, { serviceName: v }) }))
                      }
                      placeholder="sonarr"
                    />
                  </LabeledField>
                  <LabeledField label="Service port" fieldId={`app-routes-${i}-port`}>
                    <TextInput
                      id={`app-routes-${i}-port`}
                      type="number"
                      min={1}
                      value={r.servicePort ?? ''}
                      onChange={(_e, v) => {
                        const n = Number.parseInt(v, 10);
                        setForm((f) => ({
                          ...f,
                          routes: updateRoute(f.routes, i, {
                            servicePort: Number.isFinite(n) && n > 0 ? n : undefined,
                          }),
                        }));
                      }}
                      placeholder="80"
                    />
                  </LabeledField>
                  <Checkbox
                    id={`app-routes-${i}-auth`}
                    label="Require fleet ingress auth"
                    isChecked={!!r.requireAuth}
                    onChange={(_e, c) =>
                      setForm((f) => ({ ...f, routes: updateRoute(f.routes, i, { requireAuth: c }) }))
                    }
                  />
                  {r.requireAuth ? (
                    <LabeledField
                      label="Tenant groups (comma-separated)"
                      fieldId={`app-routes-${i}-groups`}
                    >
                      <TextInput
                        id={`app-routes-${i}-groups`}
                        value={(r.requireAuthGroups || []).join(', ')}
                        onChange={(_e, v) =>
                          setForm((f) => ({
                            ...f,
                            routes: updateRoute(f.routes, i, { requireAuthGroups: parseAuthGroups(v) }),
                          }))
                        }
                        placeholder="tenant-a, tenant-b"
                      />
                    </LabeledField>
                  ) : null}
                  <Button
                    variant="plain"
                    icon={<TrashIcon />}
                    aria-label={`Remove route ${i + 1}`}
                    onClick={() => setForm((f) => ({ ...f, routes: removeRouteAt(f.routes, i) }))}
                  />
                </div>
              ))}
              <Button
                variant="secondary"
                icon={<PlusIcon />}
                onClick={() =>
                  setForm((f) => ({
                    ...f,
                    routes: [
                      ...(f.routes || []),
                      { key: `app${(f.routes?.length || 0) + 1}`, mode: 'site', siteHost: '', requireAuth: true },
                    ],
                  }))
                }
              >
                Add route
              </Button>
            </>
          ) : null}
          <FormGroup fieldId="app-oidc">
            <Checkbox
              id="app-oidc"
              label="Create an Authelia OIDC client for this app"
              isChecked={oidcEnabled}
              onChange={(_e, c) => {
                setOidcEnabled(c);
                if (c && !form.oidcClient) {
                  setForm((f) => ({
                    ...f,
                    oidcClient: { clientId: f.releaseName || f.name || '', redirectPaths: [] },
                  }));
                }
              }}
            />
          </FormGroup>
          {oidcEnabled ? (
            <>
              <LabeledField label="OIDC client id" fieldId="app-oidc-id">
                <TextInput
                  id="app-oidc-id"
                  value={form.oidcClient?.clientId || ''}
                  onChange={(_e, v) =>
                    setForm((f) => ({ ...f, oidcClient: { ...f.oidcClient, clientId: v } }))
                  }
                  placeholder={form.releaseName || form.name}
                />
              </LabeledField>
              <LabeledField
                label="Redirect paths (one per line, host-relative)"
                fieldId="app-oidc-paths"
              >
                <TextArea
                  id="app-oidc-paths"
                  rows={2}
                  value={(form.oidcClient?.redirectPaths || []).join('\n')}
                  onChange={(_e, v) =>
                    setForm((f) => ({
                      ...f,
                      oidcClient: {
                        ...f.oidcClient,
                        redirectPaths: v.split('\n').map((s) => s.trim()).filter(Boolean),
                      },
                    }))
                  }
                  placeholder="/auth/login"
                />
              </LabeledField>
              <LabeledField label="Extra redirect URIs (one per line)" fieldId="app-oidc-extra">
                <TextArea
                  id="app-oidc-extra"
                  rows={2}
                  value={(form.oidcClient?.extraRedirectUris || []).join('\n')}
                  onChange={(_e, v) =>
                    setForm((f) => ({
                      ...f,
                      oidcClient: {
                        ...f.oidcClient,
                        extraRedirectUris: v.split('\n').map((s) => s.trim()).filter(Boolean),
                      },
                    }))
                  }
                  placeholder="app.immich:///oauth-callback"
                />
              </LabeledField>
              <Checkbox
                id="app-oidc-pkce"
                label="Require PKCE"
                isChecked={form.oidcClient?.requirePkce !== false}
                onChange={(_e, c) =>
                  setForm((f) => ({ ...f, oidcClient: { ...f.oidcClient, requirePkce: c } }))
                }
              />
            </>
          ) : null}
          <LabeledField
            label="Values (YAML)"
            fieldId="app-values"
            help={
              isLlamaCppChart(form.chart)
                ? { ...getFieldHelp('llamaProfiles.profileYaml')!, popoverMaxWidth: '32rem' }
                : getFieldHelp('apps.valuesYaml')
            }
          >
            <TextArea
              id="app-values"
              value={form.valuesYaml || ''}
              onChange={(_e, v) => setForm((f) => ({ ...f, valuesYaml: v }))}
              rows={6}
            />
          </LabeledField>
        </Form>
      </Modal>

      <Modal
        variant={ModalVariant.medium}
        title={editTarget ? `Edit ${editTarget.name}` : 'Edit app'}
        isOpen={editOpen}
        onClose={() => setEditOpen(false)}
        actions={[
          <Button key="save" variant="primary" onClick={() => void submitEdit()} isLoading={saving} isDisabled={!editTarget}>
            Save changes
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setEditOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <ErrorAlert error={actionError} />
          <LabeledField label="Version" fieldId="edit-version" help={getFieldHelp('apps.version')}>
            <TextInput
              id="edit-version"
              value={editForm.version}
              onChange={(_e, v) => setEditForm((f) => ({ ...f, version: v }))}
            />
          </LabeledField>
          <LabeledField label="Values (YAML)" fieldId="edit-values" help={getFieldHelp('apps.editInstall')}>
            <TextArea
              id="edit-values"
              value={editForm.valuesYaml}
              onChange={(_e, v) => setEditForm((f) => ({ ...f, valuesYaml: v }))}
              rows={8}
            />
          </LabeledField>
          {editTarget && isLlamaCppChart(editTarget.chart) ? (
            <Checkbox
              id="edit-llm-backend"
              label="Register in hub LiteLLM proxy"
              isChecked={editForm.llmBackend}
              onChange={(_e, checked) => setEditForm((f) => ({ ...f, llmBackend: checked }))}
              description={getFieldHelp('apps.llmBackend')?.description}
            />
          ) : null}
        </Form>
      </Modal>

      <Modal
        variant={ModalVariant.medium}
        title="Add chart to library"
        isOpen={addChartOpen}
        onClose={() => setAddChartOpen(false)}
        actions={[
          <Button
            key="add"
            variant="primary"
            onClick={() => void submitChart()}
            isDisabled={saving || (chartSource === 'upload' ? !chartFile : !chartOrigin)}
          >
            {saving ? 'Publishing…' : 'Publish'}
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setAddChartOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <ErrorAlert error={actionError} />
          <Radio
            id="chart-src-oci"
            name="chart-src"
            label="OCI URL"
            isChecked={chartSource === 'oci'}
            onChange={() => setChartSource('oci')}
          />
          <Radio
            id="chart-src-http"
            name="chart-src"
            label="HTTP .tgz URL"
            isChecked={chartSource === 'http'}
            onChange={() => setChartSource('http')}
          />
          <Radio
            id="chart-src-upload"
            name="chart-src"
            label="Upload .tgz"
            isChecked={chartSource === 'upload'}
            onChange={() => setChartSource('upload')}
          />
          {chartSource === 'upload' ? (
            <LabeledField label="Chart package" fieldId="chart-file" isRequired help={getFieldHelp('charts.upload')}>
              <input
                id="chart-file"
                type="file"
                accept=".tgz,.tar.gz"
                onChange={(e) => setChartFile(e.target.files?.[0] || null)}
              />
            </LabeledField>
          ) : (
            <LabeledField label="Origin" fieldId="chart-origin" isRequired help={getFieldHelp('charts.origin')}>
              <TextInput
                id="chart-origin"
                value={chartOrigin}
                placeholder={chartSource === 'http' ? 'https://example.com/app-1.0.0.tgz' : 'oci://ghcr.io/org/chart'}
                onChange={(_e, v) => setChartOrigin(v)}
              />
            </LabeledField>
          )}
          <LabeledField label="Version" fieldId="chart-version" help={getFieldHelp('apps.version')}>
            <TextInput id="chart-version" value={chartVersion} onChange={(_e, v) => setChartVersion(v)} />
          </LabeledField>
          <LabeledField label="Images" fieldId="chart-images" help={getFieldHelp('charts.imagePolicy')}>
            <FleetModalSelect
              id="chart-images"
              value={chartPolicy}
              onChange={(v) => setChartPolicy(v as FleetChartImagePolicy)}
              options={[
                { value: 'external', label: 'external — pull from upstream' },
                { value: 'proxy', label: 'proxy — Harbor pull-through' },
                { value: 'copy', label: 'copy — mirror images into Harbor' },
              ]}
            />
          </LabeledField>
        </Form>
      </Modal>

      <Modal
        variant={ModalVariant.small}
        title="Remove app"
        isOpen={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        actions={[
          <Button key="del" variant="danger" onClick={() => void confirmDelete()} isDisabled={deleting}>
            {deleting ? 'Removing…' : 'Uninstall'}
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteTarget(null)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={actionError} />
        Uninstall Helm release {deleteTarget?.releaseName} from cluster {deleteTarget?.clusterName}?
      </Modal>

      <Modal
        variant={ModalVariant.small}
        title="Remove library chart"
        isOpen={!!deleteChartTarget}
        onClose={() => setDeleteChartTarget(null)}
        actions={[
          <Button key="del" variant="danger" onClick={() => void confirmDeleteChart()} isDisabled={deleting}>
            {deleting ? 'Removing…' : 'Remove'}
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteChartTarget(null)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={actionError} />
        Remove {deleteChartTarget?.name} from the library? Harbor artifacts are kept.
      </Modal>
    </>
  );
}
