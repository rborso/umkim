import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Button,
  Checkbox,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Label,
  Modal,
  PageSection,
  PageSectionVariants,
  Tab,
  Tabs,
  TabTitleText,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { DownloadIcon } from '@patternfly/react-icons';
import {
  deleteFleetCluster,
  disableClusterIngressAuth,
  disableClusterAcme,
  downloadFleetClusterBootstrapBundle,
  downloadFleetClusterBootstrapScript,
  enableClusterIngressAuth,
  enableClusterAcme,
  fleetClusterIsConsoleProvisioned,
  fleetClusterIsDeleting,
  fleetClusterIsPending,
  fleetClusterPhase,
  getFleetCluster,
  listFleetClusterNodes,
  listFleetSites,
  patchFleetCluster,
  patchFleetClusterNode,
  provisionFleetCluster,
} from '../api/client';
import type {
  FleetCluster,
  FleetClusterPatch,
  FleetNode,
  FleetSite,
} from '../api/types';
import { ClusterCapabilitiesPanel } from '../components/clusters/ClusterCapabilitiesPanel';
import { ClusterDetailEdgeTab } from '../components/clusters/ClusterDetailEdgeTab';
import { ClusterNodesPanel } from '../components/clusters/ClusterNodesPanel';
import { ClusterOperationsPanel } from '../components/clusters/ClusterOperationsPanel';
import { ClusterStoragePanel } from '../components/clusters/ClusterStoragePanel';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { ingressAuthSupported, isPortForwardMethod } from '../edge/ingressMethod';
import { getFieldHelp } from '../help/fieldHelp';
import { useAuth } from '../auth/AuthContext';
import { canMutate } from '../auth/roles';

const CAPABILITY_LABEL_PREFIX = 'fleet.capability/';

function buildCustomLabelsPatch(labels: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(labels)) {
    if (k.startsWith(CAPABILITY_LABEL_PREFIX)) {
      continue;
    }
    if (
      k === 'fleet.site' ||
      k === 'fleet.topology/region' ||
      k === 'fleet.topology/nest_depth' ||
      k === 'fleet.topology/provisioner' ||
      k === 'fleet.topology/ha'
    ) {
      continue;
    }
    out[k] = v;
  }
  return out;
}

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Enrolled':
    case 'Validated':
    case 'ScriptIssued':
    case 'Pending':
      return 'blue';
    case 'Provisioning':
    case 'Deleting':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

export function ClusterDetailPage() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();
  const [cluster, setCluster] = useState<FleetCluster | null>(null);
  const [sites, setSites] = useState<FleetSite[]>([]);
  const [nodes, setNodes] = useState<FleetNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [saveError, setSaveError] = useState<unknown>(null);
  const [saving, setSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const [deleteCleanupKubernetes, setDeleteCleanupKubernetes] = useState(false);
  const [nodesFetchFailed, setNodesFetchFailed] = useState(false);
  const [tab, setTab] = useState<string | number>('overview');

  const [siteId, setSiteId] = useState('');
  const [region, setRegion] = useState('');
  const [nestDepth, setNestDepth] = useState('');
  const [provisioner, setProvisioner] = useState('');
  const [ingressId, setIngressId] = useState('');
  const [jumphost, setJumphost] = useState('');
  const [upstreamHost, setUpstreamHost] = useState('');
  const [traefikPort, setTraefikPort] = useState('30080');
  const [clusterLabels, setClusterLabels] = useState<Record<string, string>>({});
  const [enableAuthOpen, setEnableAuthOpen] = useState(false);
  const [disableAuthOpen, setDisableAuthOpen] = useState(false);
  const [authPassword, setAuthPassword] = useState('');
  const [exposeLldapWebUI, setExposeLldapWebUI] = useState(false);
  const [authActionError, setAuthActionError] = useState<unknown>(null);
  const [authActionBusy, setAuthActionBusy] = useState(false);

  const load = useCallback(async () => {
    if (!name) return;
    setLoading(true);
    setError(null);
    try {
      const c = await getFleetCluster(name);
      setCluster(c);
      try {
        setSites(await listFleetSites());
      } catch {
        setSites([]);
      }
      setSiteId(c.site || '');
      setClusterLabels(c.labels || {});
      setRegion(c.topology?.region || c.labels?.['fleet.topology/region'] || '');
      setNestDepth(c.topology?.nest_depth || c.labels?.['fleet.topology/nest_depth'] || '0');
      setProvisioner(c.topology?.provisioner || c.labels?.['fleet.topology/provisioner'] || 'k3s');
      setIngressId(c.ingress_id || '');
      setJumphost(c.jumphost || '');
      setUpstreamHost(c.upstream_host || '');
      setTraefikPort(String(c.traefik_http_port || 30080));
      const clusterReady = c.available || fleetClusterPhase(c) === 'Ready';
      if (clusterReady) {
        try {
          setNodes(await listFleetClusterNodes(name));
          setNodesFetchFailed(false);
        } catch {
          setNodes([]);
          setNodesFetchFailed(true);
        }
      } else {
        setNodes([]);
        setNodesFetchFailed(false);
      }
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    void load();
    const nodeOpBusy =
      cluster?.nodeOperation?.phase === 'Pending' || cluster?.nodeOperation?.phase === 'Running';
    const storageBusy =
      cluster?.storageStatus?.phase === 'Applying' || cluster?.storageStatus?.phase === 'Pending';
    const ms =
      cluster?.capability_discovery?.phase === 'Running' || nodeOpBusy || storageBusy ? 8000 : 15000;
    const t = setInterval(() => void load(), ms);
    return () => clearInterval(t);
  }, [load, cluster?.capability_discovery?.phase, cluster?.nodeOperation?.phase, cluster?.storageStatus?.phase]);

  const selectedSite = useMemo(
    () => sites.find((s) => (s.spec.siteId || s.metadata?.name) === (siteId || cluster?.site)),
    [sites, siteId, cluster?.site],
  );
  const ingressOptions = useMemo(
    () => selectedSite?.spec.edge?.ingress || [],
    [selectedSite],
  );

  const pending = cluster ? fleetClusterIsPending(cluster) : false;
  const deletingPhase = cluster ? fleetClusterIsDeleting(cluster) : false;
  const phase = cluster ? fleetClusterPhase(cluster) : '';
  const bundleMode = cluster?.provisioning_mode === 'bundle';
  const consoleProvisioned = cluster ? fleetClusterIsConsoleProvisioned(cluster) : false;
  const canDelete = cluster && !cluster.read_only && !cluster.management;

  const openDelete = () => {
    setDeleteCleanupKubernetes(false);
    setDeleteError(null);
    setDeleteOpen(true);
  };

  const confirmDelete = async () => {
    if (!name) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteFleetCluster(name, {
        cleanupKubernetes: consoleProvisioned && deleteCleanupKubernetes,
      });
      setDeleteOpen(false);
      navigate('/clusters');
    } catch (e) {
      setDeleteError(e);
    } finally {
      setDeleting(false);
    }
  };

  const downloadBootstrap = async () => {
    if (!name) return;
    setSaveError(null);
    try {
      const script = await downloadFleetClusterBootstrapScript(name);
      const blob = new Blob([script], { type: 'text/x-shellscript' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `bootstrap-${name}.sh`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      setSaveError(e);
    }
  };

  const downloadBootstrapBundle = async () => {
    if (!name) return;
    setSaveError(null);
    try {
      const blob = await downloadFleetClusterBootstrapBundle(name);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `bootstrap-${name}.tar.gz`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      setSaveError(e);
    }
  };

  const saveCluster = async () => {
    if (!name) return;
    setSaving(true);
    setSaveError(null);
    const site = clusterLabels['fleet.site'] ?? siteId;
    const topoRegion = clusterLabels['fleet.topology/region'] ?? region;
    const topoNest = clusterLabels['fleet.topology/nest_depth'] ?? nestDepth;
    const topoProvisioner = clusterLabels['fleet.topology/provisioner'] ?? provisioner;
    const patch: FleetClusterPatch = {
      siteId: site,
      jumphost: jumphost.trim(),
      topology: { region: topoRegion, nest_depth: topoNest, provisioner: topoProvisioner },
      customLabels: buildCustomLabelsPatch(clusterLabels),
      edge: {
        ingress_id: ingressId || undefined,
        upstream_host: upstreamHost,
        traefik_http_port: parseInt(traefikPort, 10) || 30080,
      },
    };
    try {
      setCluster(await patchFleetCluster(name, patch));
      await load();
    } catch (e) {
      setSaveError(e);
    } finally {
      setSaving(false);
    }
  };

  const authReady = cluster?.ingressAuthStatus?.phase === 'Ready';
  const authEnabled = !!cluster?.ingress_auth_enabled;
  const authPrereqReady = phase === 'Ready';
  const authPrereqIngress = !!cluster?.ingress_enabled;
  const authPrereqIngressBinding = !!cluster?.ingress_id;
  const authCanEnable =
    authPrereqReady && authPrereqIngress && authPrereqIngressBinding && !authEnabled && ingressAuthSupported(cluster?.method);
  const portalHost = cluster?.ingressAuthStatus?.portalHost;
  const portalUrl = portalHost ? `https://${portalHost}` : undefined;
  const lldapWebUIHost = cluster?.ingressAuthStatus?.lldapWebUIHost;
  const lldapWebUIUrl = lldapWebUIHost ? `https://${lldapWebUIHost}` : undefined;

  const confirmEnableAuth = async () => {
    if (!name || authPassword.length < 12) return;
    setAuthActionBusy(true);
    setAuthActionError(null);
    try {
      setCluster(await enableClusterIngressAuth(name, {
        lldapAdminPassword: authPassword,
        exposeLldapWebUI,
      }));
      setEnableAuthOpen(false);
      setAuthPassword('');
      setExposeLldapWebUI(false);
      await load();
    } catch (e) {
      setAuthActionError(e);
    } finally {
      setAuthActionBusy(false);
    }
  };

  const confirmDisableAuth = async () => {
    if (!name) return;
    setAuthActionBusy(true);
    setAuthActionError(null);
    try {
      setCluster(await disableClusterIngressAuth(name));
      setDisableAuthOpen(false);
      await load();
    } catch (e) {
      setAuthActionError(e);
    } finally {
      setAuthActionBusy(false);
    }
  };

  const showClusterAcme = isPortForwardMethod(cluster?.method);
  const acmeEnabled = !!cluster?.acme_enabled;
  const acmeCanEnable =
    showClusterAcme && phase === 'Ready' && !!cluster?.ingress_enabled && !acmeEnabled;

  const enableClusterAcmeAction = async () => {
    if (!name) return;
    setAuthActionBusy(true);
    setAuthActionError(null);
    try {
      setCluster(await enableClusterAcme(name));
      await load();
    } catch (e) {
      setAuthActionError(e);
    } finally {
      setAuthActionBusy(false);
    }
  };

  const disableClusterAcmeAction = async () => {
    if (!name) return;
    setAuthActionBusy(true);
    setAuthActionError(null);
    try {
      setCluster(await disableClusterAcme(name));
      await load();
    } catch (e) {
      setAuthActionError(e);
    } finally {
      setAuthActionBusy(false);
    }
  };

  const addNodeLabel = async (nodeName: string, key: string, value: string) => {
    if (!name || !key.trim()) return;
    setSaveError(null);
    try {
      await patchFleetClusterNode(name, nodeName, { labels: { [key]: value } });
      setNodes(await listFleetClusterNodes(name));
    } catch (e) {
      setSaveError(e);
    }
  };

  if (!name) return <p>Missing cluster name</p>;

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Link to="/clusters">← Back to clusters</Link>
        <Title headingLevel="h1">{name}</Title>
        {cluster && (
          <Label style={{ marginTop: '0.5rem' }} color={phaseColor(phase)}>
            {phase}
          </Label>
        )}
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        <ErrorAlert error={saveError} />
        {loading && <p>Loading…</p>}
        {cluster && deletingPhase && (
          <p>Cluster deletion in progress. WireGuard peers, OCM registration, and ManifestWorks are being removed.</p>
        )}
        {cluster && pending && (
          <>
            <Title headingLevel="h2" size="lg">
              Provisioning
            </Title>
            <p style={{ marginTop: '0.5rem' }}>
              Download the bootstrap bundle, extract on the first control node with{' '}
              <code>tar -xzf bootstrap-{name}.tar.gz</code>, then run <code>sudo ./bootstrap.sh</code>.
              Validation and local K3s provision run inside the bundle. When phase is Validated, click
              Provision for hub finalize (OCM join, cert-manager platform, spoke ingress). The cluster
              is usable for workloads when phase is Ready.
            </p>
            <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginTop: '0.75rem' }}>
              {(phase === 'ScriptIssued' ||
                phase === 'Enrolled' ||
                phase === 'Validated' ||
                phase === 'Failed' ||
                phase === 'Pending') &&
                (bundleMode ? (
                  <Button
                    variant="primary"
                    icon={<DownloadIcon />}
                    onClick={() => void downloadBootstrapBundle()}
                  >
                    Download bootstrap bundle (.tar.gz)
                  </Button>
                ) : (
                  <Button variant="secondary" icon={<DownloadIcon />} onClick={() => void downloadBootstrap()}>
                    Download bootstrap script (legacy)
                  </Button>
                ))}
              {bundleMode && (
                <Button variant="secondary" icon={<DownloadIcon />} onClick={() => void downloadBootstrap()}>
                  Download bootstrap script (debug)
                </Button>
              )}
              {(phase === 'Validated' || phase === 'Failed') && (
                <Button variant="primary" onClick={() => void provisionFleetCluster(name).then(load)}>
                  {phase === 'Failed' ? 'Retry provision' : 'Provision'}
                </Button>
              )}
            </div>
            <p style={{ marginTop: '1rem' }}>
              Site: <code>{cluster.site}</code>
            </p>
          </>
        )}
        {cluster && !pending && phase === 'Ready' && !cluster.available && (
          <p style={{ marginTop: '0.5rem', maxWidth: '42rem' }}>
            Hub provision is complete. Waiting for OCM klusterlet registration.
          </p>
        )}
        {cluster && !pending && (
          <Tabs activeKey={tab} onSelect={(_e, k) => setTab(k)} data-testid="cluster-detail-tabs">
            <Tab eventKey="overview" title={<TabTitleText>Overview</TabTitleText>} data-testid="cluster-tab-overview">
              <DescriptionList isHorizontal style={{ marginTop: '1rem', maxWidth: '48rem' }}>
                <DescriptionListGroup>
                  <DescriptionListTerm>Site</DescriptionListTerm>
                  <DescriptionListDescription>
                    {cluster.site ? (
                      <Link to={`/sites/${encodeURIComponent(cluster.site)}`}>
                        <code>{cluster.site}</code>
                      </Link>
                    ) : (
                      '—'
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Available</DescriptionListTerm>
                  <DescriptionListDescription>{cluster.available ? 'yes' : 'no'}</DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Provisioning</DescriptionListTerm>
                  <DescriptionListDescription>
                    <code>{cluster.provisioning_mode || '—'}</code>
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Topology</DescriptionListTerm>
                  <DescriptionListDescription>
                    {region || nestDepth || provisioner ? (
                      <>
                        region <code>{region || '—'}</code>, nest{' '}
                        <code>{nestDepth || '—'}</code>, provisioner{' '}
                        <code>{provisioner || '—'}</code>
                      </>
                    ) : (
                      '—'
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Ingress</DescriptionListTerm>
                  <DescriptionListDescription>
                    {cluster.ingress_id ? (
                      <>
                        <code>{cluster.ingress_id}</code>
                        {cluster.method ? (
                          <>
                            {' '}
                            · <code>{cluster.method}</code>
                          </>
                        ) : null}
                      </>
                    ) : (
                      'none'
                    )}
                    {cluster.ingress_enabled ? ' · enabled' : ' · disabled'}
                    {cluster.ingress_auth_enabled ? ' · forward-auth on' : ''}
                  </DescriptionListDescription>
                </DescriptionListGroup>
                {cluster.storageStatus?.phase && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Storage</DescriptionListTerm>
                    <DescriptionListDescription>
                      <code>{cluster.storageStatus.phase}</code>
                      {cluster.storage?.backend ? (
                        <>
                          {' '}
                          · <code>{cluster.storage.backend}</code>
                        </>
                      ) : null}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
                {cluster.nodeOperation?.phase && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Node operation</DescriptionListTerm>
                    <DescriptionListDescription>
                      <code>{cluster.nodeOperation.type || '—'}</code> —{' '}
                      <code>{cluster.nodeOperation.phase}</code>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
              </DescriptionList>
              <ClusterCapabilitiesPanel
                clusterName={name}
                cluster={cluster}
                clusterReady={phase === 'Ready' || !!cluster.available}
                onUpdated={load}
              />
            </Tab>
            {(consoleProvisioned || (canDelete && !deletingPhase)) && (
              <Tab eventKey="operations" title={<TabTitleText>Operations</TabTitleText>} data-testid="cluster-tab-operations">
                {consoleProvisioned ? (
                  <>
                    <p style={{ marginTop: '1rem', maxWidth: '42rem', fontSize: '0.9rem' }}>
                      Node lifecycle and cluster teardown. Changes run Ansible jobs against live nodes.
                    </p>
                    <ClusterOperationsPanel cluster={cluster} nodes={nodes} onUpdated={load} />
                  </>
                ) : null}
                {canDelete && !deletingPhase && (
                  <div style={{ marginTop: '2rem', paddingTop: '1rem', borderTop: '1px solid var(--pf-v6-global--BorderColor--100)' }}>
                    <Title headingLevel="h3" size="md">
                      Delete cluster
                    </Title>
                    <p style={{ marginTop: '0.5rem', maxWidth: '42rem', fontSize: '0.9rem' }}>
                      Remove hub registration, WireGuard peers, and ManifestWorks. Workloads on the
                      cluster are kept unless you opt in during delete.
                    </p>
                    <Button variant="danger" onClick={openDelete} data-testid="cluster-delete" style={{ marginTop: '0.75rem' }}>
                      Delete cluster
                    </Button>
                  </div>
                )}
              </Tab>
            )}
            <Tab eventKey="edge" title={<TabTitleText>Edge and ingress</TabTitleText>} data-testid="cluster-tab-edge">
              <ClusterDetailEdgeTab
                cluster={cluster}
                phase={phase}
                site={selectedSite}
                siteId={siteId}
                setSiteId={setSiteId}
                clusterLabels={clusterLabels}
                setClusterLabels={setClusterLabels}
                region={region}
                setRegion={setRegion}
                nestDepth={nestDepth}
                setNestDepth={setNestDepth}
                provisioner={provisioner}
                setProvisioner={setProvisioner}
                ingressId={ingressId}
                setIngressId={setIngressId}
                jumphost={jumphost}
                setJumphost={setJumphost}
                upstreamHost={upstreamHost}
                setUpstreamHost={setUpstreamHost}
                traefikPort={traefikPort}
                setTraefikPort={setTraefikPort}
                ingressOptions={ingressOptions}
                authPrereqReady={authPrereqReady}
                authPrereqIngress={authPrereqIngress}
                authPrereqIngressBinding={authPrereqIngressBinding}
                authCanEnable={authCanEnable}
                authEnabled={authEnabled}
                authActionBusy={authActionBusy}
                authActionError={authActionError}
                authReady={!!authReady}
                portalHost={portalHost}
                portalUrl={portalUrl}
                lldapWebUIUrl={lldapWebUIUrl}
                showClusterAcme={showClusterAcme}
                acmeEnabled={acmeEnabled}
                acmeCanEnable={acmeCanEnable}
                onEnableAuth={() => {
                  setAuthActionError(null);
                  setEnableAuthOpen(true);
                }}
                onDisableAuth={() => {
                  setAuthActionError(null);
                  setDisableAuthOpen(true);
                }}
                onEnableAcme={() => void enableClusterAcmeAction()}
                onDisableAcme={() => void disableClusterAcmeAction()}
                saving={saving}
                canSave={canMutate(user)}
                onSave={() => void saveCluster()}
              />
            </Tab>
            <Tab eventKey="storage" title={<TabTitleText>Storage</TabTitleText>} data-testid="cluster-tab-storage">
              <div style={{ marginTop: '1rem' }}>
                <ClusterStoragePanel cluster={cluster} onUpdated={load} />
              </div>
            </Tab>
            <Tab eventKey="nodes" title={<TabTitleText>Nodes</TabTitleText>} data-testid="cluster-tab-nodes">
              <div style={{ marginTop: '1rem' }}>
                <ClusterNodesPanel
                  nodes={nodes}
                  clusterPhase={phase}
                  clusterAvailable={!!cluster.available}
                  nodesFetchFailed={nodesFetchFailed}
                  showLabelEditor
                  onAddLabel={(nodeName, key, value) => void addNodeLabel(nodeName, key, value)}
                />
              </div>
            </Tab>
          </Tabs>
        )}
      </PageSection>
      <Modal
        title="Delete cluster"
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        actions={[
          <Button
            key="del"
            variant="danger"
            onClick={() => void confirmDelete()}
            isLoading={deleting}
            data-testid="cluster-delete-confirm"
          >
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={deleteError} />
        <p>
          Delete cluster <strong>{name}</strong>? Hub teardown unjoins OCM, removes ManifestWorks,
          removes hub WireGuard peers and the fleet overlay
          (wg-umkim) on nodes. User workloads on K3s are kept unless you opt in below.
        </p>
        {consoleProvisioned && (
          <LabeledField
            label="Uninstall Kubernetes (destroy cluster)"
            fieldId="deleteCleanupKubernetes"
            help={getFieldHelp('clusterDelete.cleanupKubernetes')}
          >
            <Checkbox
              id="deleteCleanupKubernetes"
              label="Uninstall Kubernetes (destroy cluster)"
              isChecked={deleteCleanupKubernetes}
              onChange={(_e, checked) => setDeleteCleanupKubernetes(checked)}
              data-testid="cluster-delete-cleanup-kubernetes"
            />
          </LabeledField>
        )}
        {!consoleProvisioned && cluster?.available && (
          <p>
            <strong>Warning:</strong> this is a fleet inventory spoke; deleting it removes OCM
            registration for the whole spoke.
          </p>
        )}
      </Modal>
      <Modal
        title="Enable ingress authentication"
        isOpen={enableAuthOpen}
        onClose={() => setEnableAuthOpen(false)}
        actions={[
          <Button
            key="enable"
            variant="primary"
            onClick={() => void confirmEnableAuth()}
            isLoading={authActionBusy}
            isDisabled={authPassword.length < 12}
          >
            Enable
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setEnableAuthOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Deploy Authelia + LLDAP on this cluster. LLDAP admin password is required at enable time
          (minimum 12 characters) and is stored in a per-cluster Kubernetes Secret — not returned by
          the API.
        </p>
        <LabeledField label="LLDAP admin password" fieldId="authPassword" isRequired help={getFieldHelp('clusterDetail.authPassword')}>
          <TextInput
            id="authPassword"
            type="password"
            value={authPassword}
            onChange={(_e, v) => setAuthPassword(v)}
            autoComplete="new-password"
          />
        </LabeledField>
        <LabeledField label="Expose LLDAP web UI" fieldId="exposeLldapWebUI" help={getFieldHelp('clusterDetail.exposeLldapWebUI')}>
          <Checkbox
            id="exposeLldapWebUI"
            label="Expose LLDAP web UI at lldap.<site-zone> (behind Authelia)"
            isChecked={exposeLldapWebUI}
            onChange={(_e, checked) => setExposeLldapWebUI(checked)}
          />
        </LabeledField>
      </Modal>
      <Modal
        title="Disable ingress authentication"
        isOpen={disableAuthOpen}
        onClose={() => setDisableAuthOpen(false)}
        actions={[
          <Button
            key="disable"
            variant="danger"
            onClick={() => void confirmDisableAuth()}
            isLoading={authActionBusy}
          >
            Disable
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDisableAuthOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Remove Authelia forward-auth from this cluster&apos;s spoke Traefik middleware. The
          per-cluster secret is retained; re-enable requires a new password.
        </p>
      </Modal>
    </>
  );
}
