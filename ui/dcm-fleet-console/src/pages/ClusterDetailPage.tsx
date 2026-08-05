import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Button,
  Checkbox,
  Form,
  FormSelect,
  FormSelectOption,
  Label,
  Modal,
  PageSection,
  PageSectionVariants,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { DownloadIcon, ExternalLinkAltIcon } from '@patternfly/react-icons';
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
  FleetExposureMode,
  FleetNode,
  FleetSite,
  FleetTlsPolicy,
} from '../api/types';
import { ClusterCapabilitiesPanel } from '../components/clusters/ClusterCapabilitiesPanel';
import { ClusterNodesPanel } from '../components/clusters/ClusterNodesPanel';
import { ClusterProvidersPanel } from '../components/clusters/ClusterProvidersPanel';
import { ClusterStoragePanel } from '../components/clusters/ClusterStoragePanel';
import { ErrorAlert } from '../components/ErrorAlert';
import { ExposureTlsPairWarning } from '../components/ExposureTlsPairWarning';
import { LabeledField } from '../components/forms/LabeledField';
import { LabelKeyValueEditor } from '../components/forms/LabelKeyValueEditor';
import { getExposureTlsPairIssue } from '../edge/exposureTlsPairs';
import { getFieldHelp } from '../help/fieldHelp';
import { useAuth } from '../auth/AuthContext';

const CAPABILITY_LABEL_PREFIX = 'fleet.capability/';

function editableClusterLabels(labels: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(labels).filter(([k]) => !k.startsWith(CAPABILITY_LABEL_PREFIX)),
  );
}

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

  const [siteId, setSiteId] = useState('');
  const [region, setRegion] = useState('');
  const [nestDepth, setNestDepth] = useState('');
  const [provisioner, setProvisioner] = useState('');
  const [directIngress, setDirectIngress] = useState(false);
  const [exposureMode, setExposureMode] = useState<FleetExposureMode | 'inherit'>('inherit');
  const [tlsPolicy, setTlsPolicy] = useState<FleetTlsPolicy>('inherit');
  const [ingressEndpointId, setIngressEndpointId] = useState('');
  const [domainBindings, setDomainBindings] = useState<string[]>([]);
  const [upstreamHost, setUpstreamHost] = useState('');
  const [traefikPort, setTraefikPort] = useState('30080');
  const [clusterLabels, setClusterLabels] = useState<Record<string, string>>({});
  const [pool, setPool] = useState<'shared' | 'dedicated'>('dedicated');
  const [assignOrg, setAssignOrg] = useState('');
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
      setPool((c.pool as 'shared' | 'dedicated') || (c.labels?.['fleet.pool'] as 'shared' | 'dedicated') || 'dedicated');
      setAssignOrg(c.labels?.['fleet.org'] || '');
      setRegion(c.topology?.region || c.labels?.['fleet.topology/region'] || '');
      setNestDepth(c.topology?.nest_depth || c.labels?.['fleet.topology/nest_depth'] || '0');
      setProvisioner(c.topology?.provisioner || c.labels?.['fleet.topology/provisioner'] || 'k3s');
      setDirectIngress(!!c.direct_ingress);
      setExposureMode(
        (c.exposure_mode as FleetExposureMode | 'inherit') ||
          (c.direct_ingress ? 'port_forward' : 'none'),
      );
      setTlsPolicy(c.tls_policy || 'inherit');
      setIngressEndpointId(c.ingress_endpoint_id || '');
      setDomainBindings(c.domain_bindings || []);
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
    const ms = cluster?.capability_discovery?.phase === 'Running' ? 3000 : 15000;
    const t = setInterval(() => void load(), ms);
    return () => clearInterval(t);
  }, [load, cluster?.capability_discovery?.phase]);

  const selectedSite = useMemo(
    () => sites.find((s) => (s.spec.siteId || s.metadata?.name) === (siteId || cluster?.site)),
    [sites, siteId, cluster?.site],
  );
  const domainOptions = useMemo(() => {
    const domains = selectedSite?.spec.edge?.domains || [];
    if (domains.length > 0) return domains.map((d) => d.id);
    const zone = selectedSite?.spec.edge?.edgeDomain || selectedSite?.spec.edgeDomain;
    return zone ? ['primary'] : [];
  }, [selectedSite]);
  const siteExposureMode = selectedSite?.spec.edge?.exposureMode || 'none';
  const exposureTlsIssue = useMemo(
    () => getExposureTlsPairIssue(exposureMode, tlsPolicy, siteExposureMode),
    [exposureMode, tlsPolicy, siteExposureMode],
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
      pool: user?.dcAdmin ? pool : undefined,
      org: user?.dcAdmin ? assignOrg : undefined,
      topology: { region: topoRegion, nest_depth: topoNest, provisioner: topoProvisioner },
      customLabels: buildCustomLabelsPatch(clusterLabels),
      edge: {
        exposure_mode: exposureMode,
        tls_policy: tlsPolicy,
        ingress_endpoint_id: ingressEndpointId || undefined,
        domain_bindings: domainBindings,
        direct_ingress: directIngress,
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
  const authPrereqDomains = (cluster?.domain_bindings?.length ?? 0) > 0;
  const authCanEnable = authPrereqReady && authPrereqIngress && authPrereqDomains && !authEnabled;
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

  const effectiveTlsPolicy =
    tlsPolicy !== 'inherit' ? tlsPolicy : (cluster?.tls_policy ?? 'inherit');
  const showClusterAcme = effectiveTlsPolicy === 'acme_cluster';
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
        {canDelete && !deletingPhase && (
          <div style={{ marginTop: '0.75rem' }}>
            <Button variant="danger" onClick={openDelete} data-testid="cluster-delete">
              Delete cluster
            </Button>
          </div>
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
              Provision for hub finalize (OCM join, cert-manager platform, edge). Install k8s/helm
              providers from the Providers section after Ready.
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
            Hub provision is complete. Waiting for OCM klusterlet registration; you can install DCM
            providers below while node list catches up.
          </p>
        )}
        {cluster && !pending && (
          <>
            <ClusterCapabilitiesPanel
              clusterName={name}
              cluster={cluster}
              clusterReady={phase === 'Ready' || !!cluster.available}
              onUpdated={load}
            />
            <ClusterStoragePanel cluster={cluster} onUpdated={load} />
            <ClusterProvidersPanel
              clusterName={name}
              clusterReady={phase === 'Ready' || !!cluster.available}
              kvmCapability={!!cluster.capabilities?.kvm}
            />
            <Title headingLevel="h2" size="lg">
              Labels and edge
            </Title>
            <Form style={{ maxWidth: '36rem', marginTop: '1rem' }}>
              {user?.dcAdmin ? (
                <LabeledField label="Pool" fieldId="clusterPool" help={getFieldHelp('clusterDetail.pool')}>
                  <FormSelect
                    id="clusterPool"
                    value={pool}
                    onChange={(_e, v) => setPool((v || 'dedicated') as 'shared' | 'dedicated')}
                  >
                    <FormSelectOption value="dedicated" label="Dedicated" />
                    <FormSelectOption value="shared" label="Shared" />
                  </FormSelect>
                </LabeledField>
              ) : cluster.pool ? (
                <LabeledField label="Pool" fieldId="clusterPoolReadonly">
                  <Label color={cluster.pool === 'shared' ? 'blue' : 'green'}>{cluster.pool}</Label>
                </LabeledField>
              ) : null}
              {user?.dcAdmin ? (
                <LabeledField
                  label="Org (fleet.org)"
                  fieldId="clusterOrg"
                  help={getFieldHelp('clusterDetail.org')}
                >
                  <TextInput
                    id="clusterOrg"
                    value={assignOrg}
                    placeholder="Leave empty to clear"
                    onChange={(_e, v) => setAssignOrg(v)}
                  />
                </LabeledField>
              ) : cluster.labels?.['fleet.org'] ? (
                <LabeledField label="Org (fleet.org)" fieldId="clusterOrgReadonly">
                  <Label>{cluster.labels['fleet.org']}</Label>
                </LabeledField>
              ) : null}
              <LabeledField label="Site ID (fleet.site)" fieldId="siteId" help={getFieldHelp('clusterDetail.siteId')}>
                <TextInput
                  id="siteId"
                  value={clusterLabels['fleet.site'] ?? siteId}
                  onChange={(_e, v) => {
                    setSiteId(v);
                    setClusterLabels((current) => ({ ...current, 'fleet.site': v }));
                  }}
                />
              </LabeledField>
              <LabeledField label="Ingress enabled" fieldId="ingressEnabled" help={getFieldHelp('clusterDetail.ingressEnabled')}>
                <TextInput
                  id="ingressEnabled"
                  value={cluster.ingress_enabled ? 'yes' : 'no'}
                  readOnly
                />
              </LabeledField>
              <LabeledField label="Ingress forward-auth (Authelia + LLDAP)" fieldId="ingressAuth" help={getFieldHelp('clusterDetail.ingressAuth')}>
                <div style={{ fontSize: '0.875rem', marginBottom: '0.5rem' }}>
                  <p style={{ margin: '0 0 0.35rem' }}>
                    <strong>Prerequisites</strong>
                  </p>
                  <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
                    <li style={{ color: authPrereqReady ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
                      Cluster phase Ready {authPrereqReady ? '✓' : `(current: ${phase || '—'})`}
                    </li>
                    <li style={{ color: authPrereqIngress ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
                      Ingress enabled (port forward or reverse proxy) {authPrereqIngress ? '✓' : '✗'}
                    </li>
                    <li style={{ color: authPrereqDomains ? 'inherit' : 'var(--pf-v6-global--danger-color--100)' }}>
                      Site domain bindings configured {authPrereqDomains ? '✓' : '✗'}
                    </li>
                    <li>
                      DNS: point <code>auth.&lt;site-zone&gt;</code> and app wildcards at this cluster edge
                      {portalHost ? (
                        <>
                          {' '}
                          (portal: <code>{portalHost}</code>)
                        </>
                      ) : null}
                    </li>
                  </ul>
                </div>
                {cluster.ingressAuthStatus?.phase && (
                  <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
                    Status: {cluster.ingressAuthStatus.phase}
                    {cluster.ingressAuthStatus.message ? ` — ${cluster.ingressAuthStatus.message}` : ''}
                  </p>
                )}
                {authReady && portalUrl && (
                  <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
                    <a href={portalUrl} target="_blank" rel="noreferrer">
                      Auth portal <ExternalLinkAltIcon />
                    </a>
                    {lldapWebUIUrl ? (
                      <>
                        {' '}
                        ·{' '}
                        <a href={lldapWebUIUrl} target="_blank" rel="noreferrer">
                          LLDAP UI <ExternalLinkAltIcon />
                        </a>
                      </>
                    ) : null}
                    {cluster.ingressAuthStatus?.accessDomains?.length ? (
                      <>
                        {' '}
                        — access domains: {cluster.ingressAuthStatus.accessDomains.join(', ')}
                      </>
                    ) : null}
                  </p>
                )}
                <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
                  <Button
                    variant="primary"
                    onClick={() => {
                      setAuthActionError(null);
                      setEnableAuthOpen(true);
                    }}
                    isDisabled={!authCanEnable || authActionBusy}
                  >
                    Enable ingress auth
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => {
                      setAuthActionError(null);
                      setDisableAuthOpen(true);
                    }}
                    isDisabled={!authEnabled || authActionBusy}
                  >
                    Disable
                  </Button>
                </div>
                {authActionError ? <ErrorAlert error={authActionError} /> : null}
              </LabeledField>
              {showClusterAcme && (
                <LabeledField
                  label="ACME on cluster"
                  fieldId="clusterAcme"
                  help={getFieldHelp('clusterDetail.clusterAcme')}
                >
                  {cluster.acmeStatus?.phase && (
                    <p style={{ marginTop: 0, fontSize: '0.875rem' }}>
                      Status: {cluster.acmeStatus.phase}
                      {cluster.acmeStatus.message ? ` — ${cluster.acmeStatus.message}` : ''}
                    </p>
                  )}
                  <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
                    <Button
                      variant="primary"
                      onClick={() => void enableClusterAcmeAction()}
                      isDisabled={!acmeCanEnable || authActionBusy}
                    >
                      Enable Let&apos;s Encrypt on cluster
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => void disableClusterAcmeAction()}
                      isDisabled={!acmeEnabled || authActionBusy}
                    >
                      Disable
                    </Button>
                  </div>
                </LabeledField>
              )}
              <LabeledField label="Region" fieldId="region" help={getFieldHelp('clusterDetail.region')}>
                <TextInput
                  id="region"
                  value={clusterLabels['fleet.topology/region'] ?? region}
                  onChange={(_e, v) => {
                    setRegion(v);
                    setClusterLabels((current) => ({ ...current, 'fleet.topology/region': v }));
                  }}
                />
              </LabeledField>
              <LabeledField label="Nest depth" fieldId="nestDepth" help={getFieldHelp('clusterDetail.nestDepth')}>
                <TextInput
                  id="nestDepth"
                  value={clusterLabels['fleet.topology/nest_depth'] ?? nestDepth}
                  onChange={(_e, v) => {
                    setNestDepth(v);
                    setClusterLabels((current) => ({ ...current, 'fleet.topology/nest_depth': v }));
                  }}
                />
              </LabeledField>
              <LabeledField label="Provisioner" fieldId="provisioner" help={getFieldHelp('clusterDetail.provisioner')}>
                <TextInput
                  id="provisioner"
                  value={clusterLabels['fleet.topology/provisioner'] ?? provisioner}
                  onChange={(_e, v) => {
                    setProvisioner(v);
                    setClusterLabels((current) => ({ ...current, 'fleet.topology/provisioner': v }));
                  }}
                />
              </LabeledField>
              <LabeledField
                label="Cluster labels (OCM)"
                fieldId="clusterLabels"
                help={getFieldHelp('clusterDetail.clusterLabels')}
              >
                <LabelKeyValueEditor
                  labels={editableClusterLabels(clusterLabels)}
                  onChange={(next) =>
                    setClusterLabels((current) => ({
                      ...Object.fromEntries(
                        Object.entries(current).filter(([k]) => k.startsWith(CAPABILITY_LABEL_PREFIX)),
                      ),
                      ...next,
                    }))
                  }
                  idPrefix="cluster-detail-label"
                />
                {Object.entries(clusterLabels).some(([k]) => k.startsWith(CAPABILITY_LABEL_PREFIX)) ? (
                  <div style={{ marginTop: '0.5rem', fontSize: '0.85rem' }}>
                    <strong>Discovered capabilities (read-only):</strong>
                    <pre style={{ fontSize: '0.75rem', margin: '0.35rem 0 0' }}>
                      {JSON.stringify(
                        Object.fromEntries(
                          Object.entries(clusterLabels).filter(([k]) =>
                            k.startsWith(CAPABILITY_LABEL_PREFIX),
                          ),
                        ),
                        null,
                        2,
                      )}
                    </pre>
                  </div>
                ) : null}
              </LabeledField>
              <LabeledField label="Exposure mode" fieldId="exposureMode" help={getFieldHelp('clusterDetail.exposureMode')}>
                <FormSelect
                  id="exposureMode"
                  value={exposureMode}
                  onChange={(_e, v) => {
                    const mode = v as FleetExposureMode | 'inherit';
                    setExposureMode(mode);
                    if (mode === 'port_forward' || mode === 'reverse_proxy') setDirectIngress(true);
                    if (mode === 'none') setDirectIngress(false);
                    // Match registry.EffectiveClusterTLSPolicy defaults for operator clarity.
                    if (mode === 'reverse_proxy' && (tlsPolicy === 'inherit' || tlsPolicy === 'acme_hub')) {
                      setTlsPolicy('external');
                    }
                    if (mode === 'port_forward' && (tlsPolicy === 'inherit' || tlsPolicy === 'acme_hub')) {
                      setTlsPolicy('acme_cluster');
                    }
                  }}
                >
                  <FormSelectOption
                    value="inherit"
                    label={`Inherit from site (${siteExposureMode})`}
                  />
                  <FormSelectOption value="none" label="None" />
                  <FormSelectOption value="hub_gateway" label="Hub gateway" />
                  <FormSelectOption value="port_forward" label="Port forward" />
                  <FormSelectOption value="reverse_proxy" label="External reverse proxy" />
                </FormSelect>
                {exposureMode === 'inherit' && (
                  <p style={{ fontSize: '0.85rem', marginTop: '0.35rem' }}>
                    Clusters without an override use the site default exposure mode ({siteExposureMode}).
                  </p>
                )}
              </LabeledField>
              <LabeledField label="TLS policy" fieldId="tlsPolicy" help={getFieldHelp('clusterDetail.tlsPolicy')}>
                <FormSelect
                  id="tlsPolicy"
                  value={tlsPolicy}
                  onChange={(_e, v) => setTlsPolicy(v as FleetTlsPolicy)}
                >
                  <FormSelectOption value="inherit" label="Inherit" />
                  <FormSelectOption value="external" label="External (no cluster LE)" />
                  <FormSelectOption value="acme_cluster" label="ACME on cluster" />
                  <FormSelectOption value="acme_hub" label="ACME on hub" />
                  <FormSelectOption value="passthrough" label="TLS passthrough" />
                </FormSelect>
              </LabeledField>
              <ExposureTlsPairWarning issue={exposureTlsIssue} />
              <LabeledField label="Ingress endpoint ID" fieldId="ingressEndpointId" help={getFieldHelp('clusterDetail.ingressEndpointId')}>
                <TextInput
                  id="ingressEndpointId"
                  value={ingressEndpointId}
                  onChange={(_e, v) => setIngressEndpointId(v)}
                  placeholder="openwrt-wan"
                />
              </LabeledField>
              {domainOptions.length > 1 ? (
                <LabeledField label="Domain bindings" fieldId="domainBindings" help={getFieldHelp('clusterDetail.domainBindings')}>
                  {domainOptions.map((id) => (
                    <Checkbox
                      key={id}
                      id={`detail-domain-${id}`}
                      label={id}
                      isChecked={domainBindings.includes(id)}
                      onChange={(_e, checked) =>
                        setDomainBindings((current) =>
                          checked ? [...current, id] : current.filter((d) => d !== id),
                        )
                      }
                    />
                  ))}
                </LabeledField>
              ) : domainOptions.length === 1 ? (
                <LabeledField label="Domain binding" fieldId="domainBindings" help={getFieldHelp('clusterDetail.domainBindings')}>
                  <FormSelect
                    id="domainBindings"
                    value={domainBindings[0] || ''}
                    onChange={(_e, v) => setDomainBindings(v ? [v] : [])}
                  >
                    <FormSelectOption value="" label="None" />
                    <FormSelectOption value={domainOptions[0]} label={domainOptions[0]} />
                  </FormSelect>
                </LabeledField>
              ) : (
                <LabeledField label="Domain bindings (ids)" fieldId="domainBindings" help={getFieldHelp('clusterDetail.domainBindings')}>
                  <TextInput
                    id="domainBindings"
                    value={domainBindings.join(', ')}
                    onChange={(_e, v) =>
                      setDomainBindings(
                        v
                          .split(',')
                          .map((s) => s.trim())
                          .filter(Boolean),
                      )
                    }
                    placeholder="site-zone, cluster-a"
                  />
                </LabeledField>
              )}
              <LabeledField label="Direct ingress (legacy)" fieldId="directIngress" help={getFieldHelp('clusterDetail.directIngress')}>
                <Checkbox
                  id="directIngress"
                  isChecked={directIngress}
                  onChange={(_e, checked) => setDirectIngress(checked)}
                />
              </LabeledField>
              <LabeledField label="Upstream host (WG or LAN IP)" fieldId="upstreamHost" help={getFieldHelp('clusterDetail.upstreamHost')}>
                <TextInput
                  id="upstreamHost"
                  value={upstreamHost}
                  onChange={(_e, v) => setUpstreamHost(v)}
                />
              </LabeledField>
              <LabeledField label="Traefik HTTP NodePort" fieldId="traefikPort" help={getFieldHelp('clusterDetail.traefikPort')}>
                <TextInput
                  id="traefikPort"
                  value={traefikPort}
                  onChange={(_e, v) => setTraefikPort(v)}
                />
              </LabeledField>
              <Button variant="primary" isLoading={saving} onClick={() => void saveCluster()}>
                Save cluster
              </Button>
            </Form>

            <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem' }}>
              Nodes
            </Title>
            <div style={{ marginTop: '0.75rem' }}>
              <ClusterNodesPanel
                nodes={nodes}
                clusterPhase={phase}
                clusterAvailable={!!cluster.available}
                nodesFetchFailed={nodesFetchFailed}
                showLabelEditor
                onAddLabel={(nodeName, key, value) => void addNodeLabel(nodeName, key, value)}
              />
            </div>
          </>
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
          deregisters DCM providers, revokes hub WireGuard peers, and removes the fleet overlay
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
            registration and DCM workers for the whole spoke.
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
