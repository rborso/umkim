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
  FleetIngressMethod,
  FleetSite,
  FleetSiteIngress,
} from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import {
  defaultPortForwardIngressRow,
  effectiveIngressWildcard,
  fleetSitePortForwardZone,
  ingressMethodLabel,
} from '../edge/ingressMethod';
import { getFieldHelp } from '../help/fieldHelp';

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
  const [region, setRegion] = useState('');
  const [failureDomain, setFailureDomain] = useState('');
  const [jumphost, setJumphost] = useState('');
  const [hubGateway, setHubGateway] = useState(false);
  const [hubZone, setHubZone] = useState('');
  const [hubWildcard, setHubWildcard] = useState('');
  const [ingress, setIngress] = useState<FleetSiteIngress[]>([]);
  const [extEnabled, setExtEnabled] = useState(true);
  const [dnsNote, setDnsNote] = useState('');
  const [expectedWanIp, setExpectedWanIp] = useState('');
  const [dnsHost, setDnsHost] = useState('');
  const [dnsResult, setDnsResult] = useState<Record<string, unknown> | null>(null);
  const [edgeFormDirty, setEdgeFormDirty] = useState(false);
  const edgeFormDirtyRef = useRef(false);
  edgeFormDirtyRef.current = edgeFormDirty;

  const dnsZone = useMemo(() => {
    if (site) return fleetSitePortForwardZone(site) || hubZone;
    const pf = ingress.find((row) => row.method === 'port_forward' && row.zone);
    return pf?.zone || hubZone;
  }, [site, ingress, hubZone]);

  const applySiteToEdgeForm = useCallback((s: FleetSite) => {
    setSiteIdEdit(fleetSiteEffectiveId(s));
    setDisplayName(s.spec.displayName || '');
    setRegion(s.spec.region || '');
    setFailureDomain(s.spec.failureDomain || '');
    setJumphost(s.spec.jumphost || '');
    setHubGateway(!!s.spec.edge?.hubGateway);
    setHubZone(s.spec.edge?.hubZone || '');
    setHubWildcard(s.spec.edge?.hubWildcard || '');
    setIngress(
      s.spec.edge?.ingress?.length ? s.spec.edge.ingress : [defaultPortForwardIngressRow()],
    );
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
      const normalizedIngress: FleetSiteIngress[] = ingress.map((row) => {
        const { assignedClusters: _ignored, ...rest } = row;
        const zone = rest.zone?.trim();
        const wildcard =
          rest.wildcard?.trim() ||
          (rest.method === 'port_forward' && zone ? `*.${zone}` : undefined);
        return {
          ...rest,
          id: rest.id.trim(),
          zone,
          address: rest.address?.trim(),
          wildcard,
        };
      });
      await patchFleetSite(
        name,
        isManagement
          ? {
              displayName,
              wanEgress: {
                enabled: extEnabled,
                dnsTargetNote: dnsNote,
                expectedWanIp: expectedWanIp || undefined,
              },
            }
          : {
              displayName,
              region: region.trim() || undefined,
              failureDomain: failureDomain.trim() || undefined,
              jumphost: jumphost.trim(),
              hubGateway,
              hubZone: hubZone.trim() || undefined,
              hubWildcard: hubWildcard.trim() || undefined,
              ingress: normalizedIngress,
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
                  {site.spec.failureDomain && (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Failure domain</DescriptionListTerm>
                      <DescriptionListDescription>
                        <code>{site.spec.failureDomain}</code>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  )}
                  {site.spec.jumphost && (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Jumphost</DescriptionListTerm>
                      <DescriptionListDescription>
                        <code>{site.spec.jumphost}</code>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  )}
                  <DescriptionListGroup>
                    <DescriptionListTerm>DNS zone</DescriptionListTerm>
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
                  {!isManagement && (
                    <>
                      <Title headingLevel="h4" style={{ marginTop: '1rem' }}>
                        Connectivity
                      </Title>
                      <LabeledField label="Jumphost" fieldId="siteJumphost" help={getFieldHelp('site.jumphost')}>
                        <TextInput
                          id="siteJumphost"
                          value={jumphost}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            setJumphost(v);
                          }}
                          placeholder="user@bastion.example.com:22"
                        />
                      </LabeledField>
                      <LabeledField label="Region" fieldId="siteRegion" help={getFieldHelp('site.region')}>
                        <TextInput
                          id="siteRegion"
                          value={region}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            setRegion(v);
                          }}
                          placeholder="homelab"
                        />
                      </LabeledField>
                      <LabeledField
                        label="Failure domain"
                        fieldId="siteFailureDomain"
                        help={getFieldHelp('site.failureDomain')}
                      >
                        <TextInput
                          id="siteFailureDomain"
                          value={failureDomain}
                          onChange={(_e, v) => {
                            markEdgeDirty();
                            setFailureDomain(v);
                          }}
                          placeholder="rack-a"
                        />
                      </LabeledField>
                    </>
                  )}
                  {isManagement ? (
                    <>
                      <LabeledField label="Hub gateway" fieldId="hubGateway" help={getFieldHelp('site.exposureMode')}>
                        <TextInput id="hubGateway" value="enabled" readOnly aria-label="Hub gateway" />
                        <p style={{ fontSize: '0.85rem', marginTop: '0.25rem' }}>
                          Hub gateway — WAN terminates on VPS Traefik; spokes reached over WireGuard.
                        </p>
                      </LabeledField>
                      <LabeledField label="Hub zone" fieldId="hubZone" help={getFieldHelp('site.edgeDomain')}>
                        <TextInput id="hubZone" value={hubZone} readOnly />
                      </LabeledField>
                      <LabeledField label="Hub wildcard" fieldId="hubWildcard" help={getFieldHelp('site.wildcard')}>
                        <TextInput id="hubWildcard" value={hubWildcard} readOnly />
                      </LabeledField>
                    </>
                  ) : (
                    <>
                      <LabeledField label="Hub gateway catalog" fieldId="hubGateway" help={getFieldHelp('site.exposureMode')}>
                        <Checkbox
                          id="hubGateway"
                          isChecked={hubGateway}
                          onChange={(_e, c) => {
                            markEdgeDirty();
                            setHubGateway(c);
                          }}
                          label="Expose hub catalog routes on hub zone"
                        />
                      </LabeledField>
                      {hubGateway && (
                        <>
                          <LabeledField label="Hub zone" fieldId="hubZone" help={getFieldHelp('site.edgeDomain')}>
                            <TextInput
                              id="hubZone"
                              value={hubZone}
                              onChange={(_e, v) => {
                                markEdgeDirty();
                                setHubZone(v);
                              }}
                              placeholder="hub.example.com"
                            />
                          </LabeledField>
                          <LabeledField label="Hub wildcard" fieldId="hubWildcard" help={getFieldHelp('site.wildcard')}>
                            <TextInput
                              id="hubWildcard"
                              value={hubWildcard}
                              onChange={(_e, v) => {
                                markEdgeDirty();
                                setHubWildcard(v);
                              }}
                              placeholder={hubZone ? `*.${hubZone}` : '*.hub.example.com'}
                            />
                          </LabeledField>
                        </>
                      )}
                    </>
                  )}
                  <Title headingLevel="h4" style={{ marginTop: '1rem' }}>
                    Ingress paths
                  </Title>
                  {isManagement ? (
                    <p style={{ fontSize: '0.85rem' }}>
                      Ingress rows are managed outside the console for the management site.
                    </p>
                  ) : (
                    <>
                      {ingress.map((row, i) => (
                        <div key={`ingress-row-${i}`} style={{ display: 'grid', gap: '0.5rem', marginBottom: '0.75rem', paddingBottom: '0.75rem', borderBottom: '1px solid var(--pf-v5-global--BorderColor--100)' }}>
                          <LabeledField label="Ingress ID" fieldId={`ingress-id-${i}`} help={getFieldHelp('site.domainZoneId')}>
                            <TextInput
                              aria-label="Ingress id"
                              value={row.id}
                              onChange={(_e, v) => {
                                markEdgeDirty();
                                const next = [...ingress];
                                next[i] = { ...row, id: v };
                                setIngress(next);
                              }}
                            />
                          </LabeledField>
                          <LabeledField label="Method" fieldId={`ingress-method-${i}`} help={getFieldHelp('site.exposureMode')}>
                            <FormSelect
                              aria-label="Ingress method"
                              value={row.method}
                              onChange={(_e, v) => {
                                markEdgeDirty();
                                const next = [...ingress];
                                next[i] = { ...row, method: v as FleetIngressMethod };
                                setIngress(next);
                              }}
                            >
                              <FormSelectOption value="port_forward" label="Port forward (WAN → node/VIP)" />
                              <FormSelectOption value="reverse_proxy" label="External reverse proxy (TLS at edge)" />
                            </FormSelect>
                          </LabeledField>
                          {row.method === 'port_forward' ? (
                            <>
                              <LabeledField label="WAN address" fieldId={`ingress-address-${i}`} help={getFieldHelp('site.endpointAddress')}>
                                <TextInput
                                  aria-label="Ingress address"
                                  placeholder="WAN IP or VIP"
                                  value={row.address || ''}
                                  onChange={(_e, v) => {
                                    markEdgeDirty();
                                    const next = [...ingress];
                                    next[i] = { ...row, address: v };
                                    setIngress(next);
                                  }}
                                />
                              </LabeledField>
                              <LabeledField label="DNS zone" fieldId={`ingress-zone-${i}`} help={getFieldHelp('site.domainZone')}>
                                <TextInput
                                  aria-label="Ingress zone"
                                  placeholder="workload.example.com"
                                  value={row.zone || ''}
                                  onChange={(_e, v) => {
                                    markEdgeDirty();
                                    const next = [...ingress];
                                    next[i] = {
                                      ...row,
                                      zone: v,
                                      wildcard: row.wildcard || (v ? `*.${v}` : ''),
                                    };
                                    setIngress(next);
                                  }}
                                />
                              </LabeledField>
                              <LabeledField label="Wildcard" fieldId={`ingress-wildcard-${i}`} help={getFieldHelp('site.wildcard')}>
                                <TextInput
                                  aria-label="Ingress wildcard"
                                  value={row.wildcard || effectiveIngressWildcard(row)}
                                  onChange={(_e, v) => {
                                    markEdgeDirty();
                                    const next = [...ingress];
                                    next[i] = { ...row, wildcard: v };
                                    setIngress(next);
                                  }}
                                />
                              </LabeledField>
                            </>
                          ) : (
                            <>
                              <LabeledField label="Description" fieldId={`ingress-desc-${i}`} help={getFieldHelp('site.domainZone')}>
                                <TextInput
                                  aria-label="Ingress description"
                                  value={row.description || ''}
                                  onChange={(_e, v) => {
                                    markEdgeDirty();
                                    const next = [...ingress];
                                    next[i] = { ...row, description: v };
                                    setIngress(next);
                                  }}
                                />
                              </LabeledField>
                              <LabeledField label="Operator notes" fieldId={`ingress-notes-${i}`}>
                                <TextArea
                                  aria-label="Ingress operator notes"
                                  value={row.operatorNotes || ''}
                                  onChange={(_e, v) => {
                                    markEdgeDirty();
                                    const next = [...ingress];
                                    next[i] = { ...row, operatorNotes: v };
                                    setIngress(next);
                                  }}
                                  rows={2}
                                />
                              </LabeledField>
                            </>
                          )}
                          <LabeledField label="Assigned clusters" fieldId={`ingress-assigned-${i}`}>
                            <TextInput
                              aria-label="Assigned clusters"
                              value={(row.assignedClusters || []).join(', ') || '—'}
                              readOnly
                            />
                          </LabeledField>
                          <p style={{ fontSize: '0.85rem', margin: 0 }}>
                            {ingressMethodLabel(row.method)} — assigned from cluster ingress binding.
                          </p>
                          <Button
                            variant="link"
                            isDisabled={ingress.length <= 1}
                            onClick={() => {
                              markEdgeDirty();
                              setIngress(ingress.filter((_, idx) => idx !== i));
                            }}
                          >
                            Remove ingress row
                          </Button>
                        </div>
                      ))}
                      <Button
                        variant="link"
                        onClick={() => {
                          markEdgeDirty();
                          setIngress([
                            ...ingress,
                            {
                              id: '',
                              method: 'reverse_proxy',
                            },
                          ]);
                        }}
                      >
                        + Add ingress row
                      </Button>
                    </>
                  )}
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
                      placeholder={'app.' + (dnsZone || 'workload.example.com')}
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
