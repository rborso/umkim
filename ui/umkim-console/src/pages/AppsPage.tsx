import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Alert,
  Button,
  Checkbox,
  Form,
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
import type {
  FleetApp,
  FleetAppCreate,
  FleetAppRouteMode,
  FleetChart,
  FleetChartImagePolicy,
  FleetCluster,
  FleetSite,
} from '../api/types';
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

function chartBaseName(chart: string): string {
  const trimmed = chart.trim().toLowerCase();
  const noScheme = trimmed.replace(/^oci:\/\//, '');
  const last = noScheme.includes('/') ? noScheme.slice(noScheme.lastIndexOf('/') + 1) : noScheme;
  return last.replace(/:.*/, '');
}

function isRocky10VmChart(chart: string): boolean {
  return chartBaseName(chart) === 'rocky-10-vm';
}

function rockyVmValuesYaml(cpu: string, memory: string, diskSize: string): string {
  const cores = Number.parseInt(cpu, 10);
  return [
    `cpu: ${Number.isFinite(cores) && cores > 0 ? cores : 2}`,
    `memory: ${memory.trim() || '2Gi'}`,
    'disk:',
    `  size: ${diskSize.trim() || '20Gi'}`,
  ].join('\n');
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
  const [rockyCpu, setRockyCpu] = useState('2');
  const [rockyMemory, setRockyMemory] = useState('2Gi');
  const [rockyDisk, setRockyDisk] = useState('20Gi');
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
    const rocky = isRocky10VmChart(first);
    setRockyCpu('2');
    setRockyMemory('2Gi');
    setRockyDisk('20Gi');
    setRegisterLiteLLM(false);
    setForm({
      ...emptyForm(readyClusters[0]?.name || '', first),
      version: readyCharts[0]?.version || '0.1.0',
      valuesYaml: rocky ? rockyVmValuesYaml('2', '2Gi', '20Gi') : '',
      route: { mode: 'none' },
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
      };
      const llama = isLlamaCppChart(form.chart);
      const body: FleetAppCreate = {
        ...form,
        name: form.releaseName || form.name,
        clusterName: form.clusterName,
        chart: form.chart,
        version: form.version || undefined,
        releaseName: form.releaseName,
        namespace: form.namespace || 'default',
        valuesYaml: form.valuesYaml || undefined,
        route,
        ...(llama ? { llmBackend: registerLiteLLM } : {}),
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
    const rocky = isRocky10VmChart(name);
    const llama = isLlamaCppChart(name);
    if (rocky) {
      setRockyCpu('2');
      setRockyMemory('2Gi');
      setRockyDisk('20Gi');
    }
    setForm((f) => ({
      ...f,
      chart: name,
      version: c?.version || f.version,
      releaseName: f.releaseName && f.releaseName !== f.chart ? f.releaseName : name,
      namespace: llama ? 'llm' : f.namespace,
      valuesYaml: rocky ? rockyVmValuesYaml('2', '2Gi', '20Gi') : llama ? f.valuesYaml : f.valuesYaml,
      route: rocky ? { mode: 'none' } : f.route,
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
      const route =
        editForm.routeMode === 'hub'
          ? { mode: 'hub' as FleetAppRouteMode, hubHost: editForm.routeHost.trim() || undefined }
          : editForm.routeMode === 'site'
            ? { mode: 'site' as FleetAppRouteMode, siteHost: editForm.routeHost.trim() || undefined }
            : { mode: 'none' as FleetAppRouteMode };
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

  const applyRockySizing = (cpu: string, memory: string, disk: string) => {
    setRockyCpu(cpu);
    setRockyMemory(memory);
    setRockyDisk(disk);
    setForm((f) => ({ ...f, valuesYaml: rockyVmValuesYaml(cpu, memory, disk) }));
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
                const rocky = isRocky10VmChart(v);
                setForm((f) => ({
                  ...f,
                  chart: v,
                  valuesYaml: rocky ? rockyVmValuesYaml(rockyCpu, rockyMemory, rockyDisk) : f.valuesYaml,
                  route: rocky ? { mode: 'none' } : f.route,
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
                  value={rockyCpu}
                  onChange={(_e, v) => applyRockySizing(v, rockyMemory, rockyDisk)}
                />
              </LabeledField>
              <LabeledField label="Memory" fieldId="app-rocky-memory" help={getFieldHelp('apps.rockyMemory')}>
                <TextInput
                  id="app-rocky-memory"
                  value={rockyMemory}
                  onChange={(_e, v) => applyRockySizing(rockyCpu, v, rockyDisk)}
                />
              </LabeledField>
              <LabeledField label="Disk size" fieldId="app-rocky-disk" help={getFieldHelp('apps.rockyDisk')}>
                <TextInput
                  id="app-rocky-disk"
                  value={rockyDisk}
                  onChange={(_e, v) => applyRockySizing(rockyCpu, rockyMemory, v)}
                />
              </LabeledField>
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
          {isLlamaCppChart(form.chart) && (form.route?.mode || 'none') !== 'none' ? (
            <Alert variant="warning" isInline title="Route vs hub LiteLLM" style={{ marginBottom: '0.75rem' }}>
              {getFieldHelp('apps.llamaRoute')?.description}{' '}
              <Link to="/infrastructure">LLM platform</Link> reaches the GPU spoke via WireGuard upstream, not this
              hostname.
            </Alert>
          ) : null}
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
          {isLlamaCppChart(form.chart) && (form.route?.mode || 'none') === 'none' ? (
            <p style={{ fontSize: '0.85rem', marginTop: '0.35rem', maxWidth: '36rem' }}>
              Recommended for llama-cpp: leave route <code>none</code> unless you expose the Service yourself. Hub
              LiteLLM does not use FleetApp route hostnames.
            </p>
          ) : null}
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
