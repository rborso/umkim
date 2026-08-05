import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Button,
  Checkbox,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Form,
  FormSelect,
  FormSelectOption,
  Modal,
  PageSection,
  PageSectionVariants,
  Tab,
  Tabs,
  TabTitleText,
  TextArea,
  TextInput,
  Title,
  Label,
  List,
  ListItem,
} from '@patternfly/react-core';
import { DownloadIcon } from '@patternfly/react-icons';
import {
  deleteFleetSite,
  downloadFleetSiteBootstrapScript,
  fleetSiteCanDelete,
  fleetSiteClusterCount,
  fleetSiteDisplayName,
  fleetSiteEffectiveDomain,
  fleetSiteEffectiveId,
  fleetSiteIsManagement,
  fleetSiteIsRegistry,
  fleetSitePhase,
  getFleetSite,
  patchFleetSite,
  provisionFleetSite,
  validateEdgeDNS,
  validateFleetSite,
} from '../api/client';
import type {
  FleetExposureMode,
  FleetSite,
  FleetSiteDomain,
  FleetSiteExternalEndpoint,
  FleetTlsPolicy,
} from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { ExposureTlsPairWarning } from '../components/ExposureTlsPairWarning';
import { LabeledField } from '../components/forms/LabeledField';
import { getExposureTlsPairIssue, recommendedTlsForExposure } from '../edge/exposureTlsPairs';
import { getFieldHelp } from '../help/fieldHelp';
import { SiteOperatorsPanel } from '../components/sites/SiteOperatorsPanel';

export function SiteDetailPage() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const [site, setSite] = useState<FleetSite | null>(null);
  const [tab, setTab] = useState<string | number>(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [saving, setSaving] = useState(false);
  const [siteIdEdit, setSiteIdEdit] = useState('');
  const [siteIdSaving, setSiteIdSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);

  const [displayName, setDisplayName] = useState('');
  const [exposureMode, setExposureMode] = useState<FleetExposureMode>('port_forward');
  const [endpoints, setEndpoints] = useState<FleetSiteExternalEndpoint[]>([]);
  const [domains, setDomains] = useState<FleetSiteDomain[]>([]);
  const [edgeDomain, setEdgeDomain] = useState('');
  const [wildcard, setWildcard] = useState('');
  const [extEnabled, setExtEnabled] = useState(true);
  const [dnsNote, setDnsNote] = useState('');
  const [expectedWanIp, setExpectedWanIp] = useState('');
  const [dnsHost, setDnsHost] = useState('');
  const [dnsResult, setDnsResult] = useState<Record<string, unknown> | null>(null);
  const [edgeFormDirty, setEdgeFormDirty] = useState(false);
  const edgeFormDirtyRef = useRef(false);
  edgeFormDirtyRef.current = edgeFormDirty;

  const domainTlsIssues = useMemo(
    () =>
      domains.map((dom, i) => ({
        key: dom.id || `domain-${i}`,
        label: dom.id || dom.zone || `domain ${i + 1}`,
        issue: getExposureTlsPairIssue(exposureMode, dom.tlsPolicy || 'inherit', exposureMode),
      })),
    [domains, exposureMode],
  );

  const applySiteToEdgeForm = useCallback((s: FleetSite) => {
    setSiteIdEdit(fleetSiteEffectiveId(s));
    setDisplayName(s.spec.displayName || '');
    setExposureMode(s.spec.edge?.exposureMode || 'none');
    setEndpoints(s.spec.edge?.externalEndpoints || []);
    setDomains(
      s.spec.edge?.domains?.length
        ? s.spec.edge.domains
        : s.spec.edge?.edgeDomain || s.spec.edgeDomain
          ? [{
              id: 'primary',
              zone: s.spec.edge?.edgeDomain || s.spec.edgeDomain || '',
              wildcard: s.spec.edge?.wildcard,
            }]
          : [],
    );
    setEdgeDomain(s.spec.edge?.edgeDomain || s.spec.edgeDomain || '');
    setWildcard(s.spec.edge?.wildcard || '');
    const wan =
      s.spec.edge?.wanEgress ||
      (s.spec.edge?.externalAccess
        ? {
            enabled: s.spec.edge.externalAccess.enabled,
            dnsTargetNote: s.spec.edge.externalAccess.dnsTargetNote,
            expectedWanIp: s.spec.edge.externalAccess.expectedWanIp,
          }
        : undefined);
    setExtEnabled(wan?.enabled ?? false);
    setDnsNote(wan?.dnsTargetNote || '');
    setExpectedWanIp(wan?.expectedWanIp || '');
  }, []);

  const load = useCallback(
    async (opts?: { silent?: boolean; refreshForm?: boolean }) => {
      if (!name) return;
      const silent = opts?.silent ?? false;
      const refreshForm = opts?.refreshForm ?? !edgeFormDirtyRef.current;
      if (!silent) {
        setLoading(true);
        setError(null);
      }
      try {
        const s = await getFleetSite(name);
        setSite(s);
        if (refreshForm) {
          applySiteToEdgeForm(s);
          setEdgeFormDirty(false);
        }
      } catch (e) {
        if (!silent) {
          setError(e);
        }
      } finally {
        if (!silent) {
          setLoading(false);
        }
      }
    },
    [name, applySiteToEdgeForm],
  );

  useEffect(() => {
    setEdgeFormDirty(false);
    void load({ refreshForm: true });
    const t = setInterval(() => void load({ silent: true, refreshForm: false }), 15000);
    return () => clearInterval(t);
  }, [load]);

  if (!name) return <p>Missing site name</p>;

  const phase = site ? fleetSitePhase(site) : '';
  const isRegistry = site ? fleetSiteIsRegistry(site) : false;
  const isManagement = site ? fleetSiteIsManagement(site) : false;
  const clusterCount = site ? fleetSiteClusterCount(site) : 0;
  const canDelete = site ? fleetSiteCanDelete(site) : false;
  const canEditSiteId = site && !isManagement && clusterCount === 0;

  const saveSiteId = async () => {
    if (!name || !siteIdEdit.trim()) return;
    setSiteIdSaving(true);
    setActionError(null);
    try {
      await patchFleetSite(name, { siteId: siteIdEdit.trim() });
      await load({ refreshForm: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setSiteIdSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!name || deleteConfirm !== name) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteFleetSite(name);
      navigate('/sites');
    } catch (e) {
      setDeleteError(e);
    } finally {
      setDeleting(false);
    }
  };

  const download = async () => {
    setActionError(null);
    try {
      const script = await downloadFleetSiteBootstrapScript(name);
      const blob = new Blob([script], { type: 'text/x-shellscript' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `bootstrap-${name}.sh`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      setActionError(e);
    }
  };

  const saveEdge = async () => {
    setSaving(true);
    setActionError(null);
    try {
      const normalizedDomains: FleetSiteDomain[] = domains.map((d) => {
        const { assignedClusters: _ignored, ...rest } = d;
        return {
          ...rest,
          wildcard: d.wildcard || (d.zone ? `*.${d.zone}` : undefined),
        };
      });
      await patchFleetSite(
        name,
        isManagement
          ? {
              displayName,
              externalEndpoints: endpoints,
              domains: normalizedDomains,
              wanEgress: {
                enabled: extEnabled,
                dnsTargetNote: dnsNote,
                expectedWanIp: expectedWanIp || undefined,
              },
            }
          : {
              displayName,
              exposureMode,
              externalEndpoints: endpoints,
              domains: normalizedDomains,
              edgeDomain: edgeDomain || normalizedDomains[0]?.zone,
              wildcard: wildcard || (edgeDomain ? `*.${edgeDomain}` : normalizedDomains[0]?.wildcard),
              wanEgress: {
                enabled: extEnabled,
                dnsTargetNote: dnsNote,
                expectedWanIp: expectedWanIp || undefined,
              },
            },
      );
      setEdgeFormDirty(false);
      await load({ refreshForm: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const markEdgeDirty = () => setEdgeFormDirty(true);

  const runDnsValidate = async () => {
    if (!dnsHost.trim()) return;
    setActionError(null);
    try {
      setDnsResult(await validateEdgeDNS(dnsHost.trim(), expectedWanIp.trim() || undefined));
    } catch (e) {
      setActionError(e);
    }
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Link to="/sites">← Back to sites</Link>
        <Title headingLevel="h1">{site ? fleetSiteDisplayName(site) : name}</Title>
        {site && (
          <Label style={{ marginTop: '0.5rem' }} color="blue">
            {phase}
            {isManagement ? ' · management' : isRegistry ? ' · registry' : ' · greenfield'}
          </Label>
        )}
        {canDelete && (
          <div style={{ marginTop: '0.75rem' }}>
            <Button
              variant="danger"
              onClick={() => {
                setDeleteConfirm('');
                setDeleteError(null);
                setDeleteOpen(true);
              }}
              data-testid="site-detail-delete"
            >
              Delete site
            </Button>
          </div>
        )}
        {isRegistry && !isManagement && clusterCount > 0 && (
          <p style={{ marginTop: '0.75rem', fontSize: '0.9rem' }}>
            Delete all clusters in this site before changing the site ID or deleting the site.
          </p>
        )}
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        <ErrorAlert error={actionError} />
        {loading && <p>Loading…</p>}
        {site && (
          <>
            <Tabs activeKey={tab} onSelect={(_e, k) => setTab(k)}>
              <Tab eventKey={0} title={<TabTitleText>Overview</TabTitleText>}>
                <DescriptionList isHorizontal style={{ marginTop: '1rem' }}>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Site ID</DescriptionListTerm>
                    <DescriptionListDescription>
                      {canEditSiteId ? (
                        <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', flexWrap: 'wrap' }}>
                          <TextInput
                            id="siteIdEdit"
                            value={siteIdEdit}
                            onChange={(_e, v) => setSiteIdEdit(v)}
                            style={{ maxWidth: '16rem' }}
                          />
                          <Button
                            variant="secondary"
                            size="sm"
                            isLoading={siteIdSaving}
                            isDisabled={siteIdEdit.trim() === fleetSiteEffectiveId(site)}
                            onClick={() => void saveSiteId()}
                          >
                            Save site ID
                          </Button>
                        </div>
                      ) : (
                        <code>{fleetSiteEffectiveId(site)}</code>
                      )}
                      {canEditSiteId && (
                        <p style={{ fontSize: '0.85rem', marginTop: '0.35rem' }}>
                          OCM <code>fleet.site</code> label for clusters created under this site.
                        </p>
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  {site.spec.region && (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Region</DescriptionListTerm>
                      <DescriptionListDescription>
                        <code>{site.spec.region}</code>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  )}
                  <DescriptionListGroup>
                    <DescriptionListTerm>Edge domain</DescriptionListTerm>
                    <DescriptionListDescription>
                      <code>{fleetSiteEffectiveDomain(site) || '—'}</code>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>{isManagement ? 'Hub host' : 'Clusters'}</DescriptionListTerm>
                    <DescriptionListDescription>
                      {isManagement ? (
                        <Link to="/infrastructure">Infrastructure</Link>
                      ) : (site.clusters || []).length === 0 ? (
                        <Link to={`/clusters?site=${encodeURIComponent(fleetSiteEffectiveId(site))}`}>
                          Create cluster
                        </Link>
                      ) : (
                        site.clusters!.map((c) => (
                          <span key={c}>
                            <Link to={`/clusters/${c}`}>{c}</Link>{' '}
                          </span>
                        ))
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  {!isRegistry && (
                    <>
                      <DescriptionListGroup>
                        <DescriptionListTerm>OCM name</DescriptionListTerm>
                        <DescriptionListDescription>{site.spec.siteName}</DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>WireGuard</DescriptionListTerm>
                        <DescriptionListDescription>
                          {site.status?.wireguardAddress || '—'}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </>
                  )}
                </DescriptionList>
                {!isRegistry && !isManagement && (
                  <div style={{ marginTop: '1rem', display: 'flex', gap: '0.5rem' }}>
                    <Button variant="secondary" icon={<DownloadIcon />} onClick={() => void download()}>
                      Download bootstrap script
                    </Button>
                    {phase === 'Enrolled' && (
                      <Button variant="primary" onClick={() => void validateFleetSite(name).then(() => load())}>
                        Validate
                      </Button>
                    )}
                    {phase === 'Validated' && (
                      <Button variant="primary" onClick={() => void provisionFleetSite(name).then(() => load())}>
                        Provision
                      </Button>
                    )}
                  </div>
                )}
                {!isRegistry && !isManagement && (
                  <SiteOperatorsPanel siteName={name} sitePhase={phase} />
                )}
              </Tab>
              <Tab eventKey={1} title={<TabTitleText>Edge and DNS</TabTitleText>}>
                <Form style={{ maxWidth: '40rem', marginTop: '1rem' }}>
                  <LabeledField label="Display name" fieldId="displayName" help={getFieldHelp('site.displayName')}>
                    <TextInput
                      id="displayName"
                      value={displayName}
                      onChange={(_e, v) => {
                        markEdgeDirty();
                        setDisplayName(v);
                      }}
                    />
                  </LabeledField>
                  {isManagement ? (
                    <LabeledField label="Exposure mode" fieldId="exposureMode" help={getFieldHelp('site.exposureMode')}>
                      <TextInput
                        id="exposureMode"
                        value={exposureMode}
                        readOnly
                        aria-label="Exposure mode"
                      />
                      <p style={{ fontSize: '0.85rem', marginTop: '0.25rem' }}>
                        Hub gateway — WAN terminates on VPS Traefik; spokes reached over WireGuard.
                      </p>
                    </LabeledField>
                  ) : (
                    <>
                      <LabeledField label="Exposure mode" fieldId="exposureMode" help={getFieldHelp('site.exposureMode')}>
                        <FormSelect
                          id="exposureMode"
                          value={exposureMode}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            const mode = v as FleetExposureMode;
                            setExposureMode(mode);
                            const recommended = recommendedTlsForExposure(mode);
                            setDomains((current) =>
                              current.map((d) => {
                                const tls = d.tlsPolicy || 'inherit';
                                if (tls === 'inherit' || tls === 'acme_hub' || tls === recommended) {
                                  return { ...d, tlsPolicy: recommended as FleetTlsPolicy };
                                }
                                return d;
                              }),
                            );
                          }}
                        >
                          <FormSelectOption value="none" label="None (hub/WG only)" />
                          <FormSelectOption value="port_forward" label="Port forward (WAN → node/VIP)" />
                          <FormSelectOption value="reverse_proxy" label="External reverse proxy (TLS at edge)" />
                        </FormSelect>
                      </LabeledField>
                      <LabeledField label="Primary edge domain (zone)" fieldId="edgeDomain" isRequired help={getFieldHelp('site.edgeDomain')}>
                        <TextInput
                          id="edgeDomain"
                          value={edgeDomain}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            setEdgeDomain(v);
                          }}
                          placeholder="workload.example.com"
                        />
                      </LabeledField>
                      <LabeledField label="Primary wildcard" fieldId="wildcard" help={getFieldHelp('site.wildcard')}>
                        <TextInput
                          id="wildcard"
                          value={wildcard}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            setWildcard(v);
                          }}
                          placeholder={'*.' + (edgeDomain || 'workload.example.com')}
                        />
                      </LabeledField>
                    </>
                  )}
                  <Title headingLevel="h4" style={{ marginTop: '1rem' }}>
                    External endpoints (WAN IPs / VIPs)
                  </Title>
                  {endpoints.map((ep, i) => (
                    <div key={`endpoint-row-${i}`} style={{ display: 'grid', gap: '0.5rem', marginBottom: '0.75rem' }}>
                      <LabeledField label="Endpoint ID" fieldId={`endpoint-id-${i}`} help={getFieldHelp('site.endpointId')}>
                        <TextInput
                          aria-label="Endpoint id"
                          placeholder="id (e.g. openwrt-wan)"
                          value={ep.id}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            const next = [...endpoints];
                            next[i] = { ...ep, id: v };
                            setEndpoints(next);
                          }}
                        />
                      </LabeledField>
                      <LabeledField label="Endpoint address" fieldId={`endpoint-addr-${i}`} help={getFieldHelp('site.endpointAddress')}>
                        <TextInput
                          aria-label="Endpoint address"
                          placeholder="WAN IP or VIP"
                          value={ep.address}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            const next = [...endpoints];
                            next[i] = { ...ep, address: v };
                            setEndpoints(next);
                          }}
                        />
                      </LabeledField>
                    </div>
                  ))}
                  <Button
                    variant="link"
                    onClick={() => {
                      markEdgeDirty();
                      setEndpoints([...endpoints, { id: '', address: '' }]);
                    }}
                  >
                    + Add endpoint
                  </Button>
                  <Title headingLevel="h4" style={{ marginTop: '1rem' }}>
                    Domain zones
                  </Title>
                  {domains.map((dom, i) => (
                    <div key={`domain-row-${i}`} style={{ display: 'grid', gap: '0.5rem', marginBottom: '0.75rem' }}>
                      <LabeledField label="Domain zone ID" fieldId={`domain-id-${i}`} help={getFieldHelp('site.domainZoneId')}>
                        <TextInput
                          aria-label="Domain id"
                          placeholder="id (e.g. site-zone)"
                          value={dom.id}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            const next = [...domains];
                            next[i] = { ...dom, id: v };
                            setDomains(next);
                          }}
                        />
                      </LabeledField>
                      <LabeledField label="Zone FQDN" fieldId={`domain-zone-${i}`} help={getFieldHelp('site.domainZone')}>
                        <TextInput
                          aria-label="Zone"
                          placeholder="workload.example.com"
                          value={dom.zone}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            const next = [...domains];
                            next[i] = { ...dom, zone: v };
                            setDomains(next);
                          }}
                        />
                      </LabeledField>
                      <LabeledField label="TLS policy" fieldId={`domain-tls-${i}`} help={getFieldHelp('site.domainTlsPolicy')}>
                        <FormSelect
                          aria-label="TLS policy"
                          value={dom.tlsPolicy || 'inherit'}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            const next = [...domains];
                            next[i] = { ...dom, tlsPolicy: v as FleetTlsPolicy };
                            setDomains(next);
                          }}
                        >
                          <FormSelectOption value="inherit" label="Inherit" />
                          <FormSelectOption value="external" label="External (no cluster LE)" />
                          <FormSelectOption value="acme_cluster" label="ACME on cluster" />
                          <FormSelectOption value="acme_hub" label="ACME on hub" />
                          <FormSelectOption value="passthrough" label="TLS passthrough" />
                        </FormSelect>
                      </LabeledField>
                      {domainTlsIssues[i]?.issue ? (
                        <ExposureTlsPairWarning
                          issue={domainTlsIssues[i].issue}
                          domainLabel={domainTlsIssues[i].label}
                        />
                      ) : null}
                      <TextInput
                        aria-label="Assigned clusters"
                        value={(dom.assignedClusters || []).join(', ') || '—'}
                        readOnly
                      />
                      <p style={{ fontSize: '0.85rem', margin: 0 }}>
                        Assigned from cluster domain bindings (edit on Cluster detail).
                      </p>
                    </div>
                  ))}
                  <Button
                    variant="link"
                    onClick={() => {
                      markEdgeDirty();
                      setDomains([...domains, { id: '', zone: '' }]);
                    }}
                  >
                    + Add domain zone
                  </Button>
                  <LabeledField label="WAN egress (site has internet path)" fieldId="extEnabled" help={getFieldHelp('site.wanEgress')}>
                    <Checkbox
                      id="extEnabled"
                      isChecked={extEnabled}
                      onChange={(_e, c) => {
                        markEdgeDirty();
                        setExtEnabled(c);
                      }}
                    />
                  </LabeledField>
                  <LabeledField label="DNS target note" fieldId="dnsNote" help={getFieldHelp('site.dnsNote')}>
                    <TextArea
                      id="dnsNote"
                      value={dnsNote}
                      onChange={(_e, v) => {
                        markEdgeDirty();
                        setDnsNote(v);
                      }}
                      rows={3}
                    />
                  </LabeledField>
                  <LabeledField label="Expected WAN IP (optional)" fieldId="expectedWanIp" help={getFieldHelp('site.expectedWanIp')}>
                    <TextInput
                      id="expectedWanIp"
                      value={expectedWanIp}
                      onChange={(_e, v) => {
                        markEdgeDirty();
                        setExpectedWanIp(v);
                      }}
                    />
                  </LabeledField>
                  <Button variant="primary" isLoading={saving} onClick={() => void saveEdge()}>
                    Save site edge settings
                  </Button>
                </Form>
                <Title headingLevel="h3" style={{ marginTop: '2rem' }}>
                  DNS validate
                </Title>
                <Form style={{ maxWidth: '32rem', marginTop: '0.5rem' }}>
                  <LabeledField label="Hostname" fieldId="dnsHost" help={getFieldHelp('site.dnsValidateHost')}>
                    <TextInput
                      id="dnsHost"
                      value={dnsHost}
                      onChange={(_e, v) => setDnsHost(v)}
                      placeholder={'app.' + (edgeDomain || 'workload.example.com')}
                    />
                  </LabeledField>
                  <Button variant="secondary" onClick={() => void runDnsValidate()}>
                    Validate DNS
                  </Button>
                </Form>
                {dnsResult && (
                  <pre style={{ marginTop: '0.75rem', fontSize: '0.8rem' }}>
                    {JSON.stringify(dnsResult, null, 2)}
                  </pre>
                )}
              </Tab>
              {!isRegistry && site.status?.validationErrors && site.status.validationErrors.length > 0 && (
                <Tab eventKey={2} title={<TabTitleText>Validation</TabTitleText>}>
                  <List style={{ marginTop: '1rem' }}>
                    {site.status.validationErrors.map((e) => (
                      <ListItem key={e}>{e}</ListItem>
                    ))}
                  </List>
                </Tab>
              )}
            </Tabs>
          </>
        )}
      </PageSection>

      <Modal
        title="Delete site"
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        actions={[
          <Button
            key="del"
            variant="danger"
            onClick={() => void confirmDelete()}
            isLoading={deleting}
            isDisabled={!name || deleteConfirm !== name}
            data-testid="site-detail-delete-confirm"
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
          Delete registry site <strong>{name}</strong>? This cannot be undone.
        </p>
        <LabeledField label="Type site name to confirm" fieldId="deleteConfirmDetail" isRequired>
          <TextInput
            id="deleteConfirmDetail"
            value={deleteConfirm}
            onChange={(_e, v) => setDeleteConfirm(v)}
            placeholder={name}
          />
        </LabeledField>
      </Modal>
    </>
  );
}
