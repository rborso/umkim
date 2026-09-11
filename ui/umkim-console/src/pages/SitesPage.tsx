import { useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  Form,
  FormSelect,
  FormSelectOption,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Label,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { PlusIcon, SyncIcon, DownloadIcon } from '@patternfly/react-icons';
import {
  createFleetSite,
  deleteFleetSite,
  downloadFleetSiteBootstrapScript,
  fleetSiteCanDelete,
  fleetSiteClusterCount,
  fleetSiteDisplayName,
  fleetSiteEffectiveId,
  fleetSiteIsManagement,
  fleetSiteIsRegistry,
  fleetSitePhase,
  provisionFleetSite,
  validateFleetSite,
} from '../api/client';
import type { FleetIngressMethod, FleetSite, FleetSiteCreate, FleetSiteIngress } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { defaultPortForwardIngressRow, fleetSiteIngressSummary } from '../edge/ingressMethod';
import { getFieldHelp } from '../help/fieldHelp';
import { useFleetSitesQuery, useInvalidateFleet } from '../hooks/useFleetQueries';
import { useAuth } from '../auth/AuthContext';
import { canMutate } from '../auth/roles';

const emptyForm = (): FleetSiteCreate => ({
  siteName: '',
  displayName: '',
  edge: {
    ingress: [defaultPortForwardIngressRow()],
  },
  wanEgress: { enabled: true, dnsTargetNote: '' },
});

type SiteCreateProfile = 'port_forward' | 'hub_gateway' | 'custom';

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Enrolled':
    case 'Validated':
    case 'ScriptIssued':
      return 'blue';
    case 'Provisioning':
    case 'Destroying':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

export function SitesPage() {
  const { user } = useAuth();
  const { invalidateSites } = useInvalidateFleet();
  const sitesQuery = useFleetSitesQuery();
  const rows = sitesQuery.data ?? [];
  const loading = sitesQuery.isLoading;
  const fetching = sitesQuery.isFetching;
  const error = sitesQuery.error;
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<FleetSiteCreate>(emptyForm());
  const [profile, setProfile] = useState<SiteCreateProfile>('port_forward');
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<FleetSite | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);

  const load = () => void invalidateSites();

  const siteName = (s: FleetSite) => s.metadata?.name || s.spec.siteName;

  const handleDownload = async (name: string) => {
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

  const handleValidate = async (name: string) => {
    setActionError(null);
    try {
      await validateFleetSite(name);
      await load();
    } catch (e) {
      setActionError(e);
    }
  };

  const handleProvision = async (name: string) => {
    setActionError(null);
    try {
      await provisionFleetSite(name);
      await load();
    } catch (e) {
      setActionError(e);
    }
  };

  const handleCreate = async () => {
    setSaving(true);
    setActionError(null);
    try {
      const payload: FleetSiteCreate = {
        ...form,
        hubGateway: profile === 'hub_gateway',
        edge:
          profile === 'hub_gateway'
            ? {
                hubGateway: true,
                hubZone: form.hubZone,
                hubWildcard: form.hubWildcard,
                ingress: [],
                wanEgress: form.wanEgress,
              }
            : form.edge,
      };
      await createFleetSite(payload);
      setModalOpen(false);
      setForm(emptyForm());
      setProfile('port_forward');
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const openDelete = (s: FleetSite) => {
    setDeleteTarget(s);
    setDeleteConfirm('');
    setDeleteError(null);
    setDeleteOpen(true);
  };

  const ingressRows = form.edge?.ingress?.length ? form.edge.ingress : [defaultPortForwardIngressRow()];

  const updateIngressAt = (index: number, patch: Partial<FleetSiteIngress>) => {
    const rows = [...ingressRows];
    const next = { ...rows[index], ...patch };
    if (patch.zone !== undefined && !rows[index]?.wildcard) {
      next.wildcard = patch.zone ? `*.${patch.zone}` : '';
    }
    rows[index] = next;
    setForm({
      ...form,
      edge: { ...form.edge, ingress: rows },
    });
  };

  const addIngressRow = () => {
    setForm({
      ...form,
      edge: { ...form.edge, ingress: [...ingressRows, defaultPortForwardIngressRow()] },
    });
  };

  const confirmDelete = async () => {
    const name = deleteTarget ? siteName(deleteTarget) : '';
    if (!name || deleteConfirm !== name) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteFleetSite(name);
      setDeleteOpen(false);
      setDeleteTarget(null);
      await load();
    } catch (e) {
      setDeleteError(e);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1">Sites</Title>
        <p style={{ marginTop: '0.5rem', maxWidth: '48rem' }}>
          Logical datacenter sites: edge DNS, exposure, and WAN endpoints. Provision clusters under
          a site from the Clusters page.
        </p>
        <Toolbar style={{ marginTop: '0.75rem' }}>
          <ToolbarContent>
            <ToolbarItem>
              <Button
                variant="primary"
                icon={<PlusIcon />}
                isDisabled={!canMutate(user)}
                onClick={() => {
                  setForm(emptyForm());
                  setProfile('port_forward');
                  setModalOpen(true);
                }}
              >
                Create site
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={() => void load()} isDisabled={fetching}>
                Refresh
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        <ErrorAlert error={actionError} />
        <Table aria-label="Fleet sites">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Type</Th>
              <Th>Phase</Th>
              <Th>Ingress</Th>
              <Th>Clusters</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {rows.map((s) => {
              const name = siteName(s);
              const phase = fleetSitePhase(s);
              const registry = fleetSiteIsRegistry(s);
              const management = fleetSiteIsManagement(s);
              const clusterCount = fleetSiteClusterCount(s);
              const canDelete = fleetSiteCanDelete(s);
              return (
                <Tr key={name}>
                  <Td>
                    <Link to={`/sites/${name}`}>{fleetSiteDisplayName(s)}</Link>
                    <br />
                    <code>{fleetSiteEffectiveId(s)}</code>
                  </Td>
                  <Td>
                    {management ? 'Management' : registry ? 'Registry' : 'Greenfield'}
                  </Td>
                  <Td>
                    <Label color={phaseColor(phase)}>{phase}</Label>
                  </Td>
                  <Td style={{ maxWidth: '22rem', whiteSpace: 'normal' }}>
                    {management ? (
                      <span>Hub gateway (management)</span>
                    ) : (
                      <span style={{ fontSize: '0.9rem' }}>{fleetSiteIngressSummary(s)}</span>
                    )}
                  </Td>
                  <Td>
                    {management ? (
                      <Link to="/infrastructure">Hub host</Link>
                    ) : clusterCount === 0 ? (
                      '—'
                    ) : (
                      (s.clusters || []).map((c, i) => (
                        <span key={c}>
                          {i > 0 ? ', ' : ''}
                          <Link to={`/clusters/${c}`}>{c}</Link>
                        </span>
                      ))
                    )}
                  </Td>
                  <Td>
                    {!registry && !management && (
                      <>
                        {(phase === 'ScriptIssued' ||
                          phase === 'Enrolled' ||
                          phase === 'Validated' ||
                          phase === 'Failed') && (
                          <Button
                            variant="link"
                            icon={<DownloadIcon />}
                            onClick={() => void handleDownload(name)}
                          >
                            Script
                          </Button>
                        )}
                        {phase === 'Enrolled' && (
                          <Button variant="link" onClick={() => void handleValidate(name)}>
                            Validate
                          </Button>
                        )}
                        {phase === 'Validated' && (
                          <Button variant="link" onClick={() => void handleProvision(name)}>
                            Provision
                          </Button>
                        )}
                      </>
                    )}
                    {registry && !management && clusterCount > 0 && (
                      <span style={{ fontSize: '0.85rem', color: 'var(--pf-v5-global--Color--200)' }}>
                        Delete clusters first
                      </span>
                    )}
                    {canDelete && (
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => openDelete(s)}
                        data-testid={`site-delete-${name}`}
                      >
                        Delete
                      </Button>
                    )}
                  </Td>
                </Tr>
              );
            })}
          </Tbody>
        </Table>
        {!loading && rows.length === 0 && <p>No sites yet. Run fleet-site seed or create one.</p>}
      </PageSection>

      <Modal
        variant={ModalVariant.medium}
        title="Create site"
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        actions={[
          <Button key="create" variant="primary" isLoading={saving} onClick={() => void handleCreate()}>
            Create
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setModalOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <LabeledField label="Site name" isRequired fieldId="siteName" help={getFieldHelp('site.siteId')}>
            <TextInput
              id="siteName"
              value={form.siteName}
              onChange={(_e, v) => setForm({ ...form, siteName: v })}
              placeholder="homelab"
            />
          </LabeledField>
          <LabeledField label="Display name" fieldId="displayName" help={getFieldHelp('site.displayName')}>
            <TextInput
              id="displayName"
              value={form.displayName || ''}
              onChange={(_e, v) => setForm({ ...form, displayName: v })}
            />
          </LabeledField>
          <LabeledField label="Region" fieldId="siteRegion" help={getFieldHelp('site.region')}>
            <TextInput
              id="siteRegion"
              value={form.region || ''}
              onChange={(_e, v) => setForm({ ...form, region: v })}
              placeholder="homelab"
            />
          </LabeledField>
          <LabeledField label="Jumphost" fieldId="siteJumphost" help={getFieldHelp('site.jumphost')}>
            <TextInput
              id="siteJumphost"
              value={form.jumphost || ''}
              onChange={(_e, v) => setForm({ ...form, jumphost: v })}
              placeholder="user@bastion.example.com:22"
            />
          </LabeledField>
          <LabeledField label="Site profile" fieldId="siteProfile" help={getFieldHelp('site.exposureMode')}>
            <FormSelect
              id="siteProfile"
              value={profile}
              onChange={(_e, v) => setProfile(v as SiteCreateProfile)}
            >
              <FormSelectOption value="port_forward" label="Port forward (default)" />
              <FormSelectOption value="hub_gateway" label="Hub gateway catalog" />
              <FormSelectOption value="custom" label="Custom (multi-ingress)" />
            </FormSelect>
          </LabeledField>
          {profile === 'hub_gateway' ? (
            <>
              <LabeledField label="Hub zone" fieldId="hubZone" isRequired help={getFieldHelp('site.edgeDomain')}>
                <TextInput
                  id="hubZone"
                  value={form.hubZone || ''}
                  onChange={(_e, v) => setForm({ ...form, hubZone: v })}
                  placeholder="hub.example.com"
                />
              </LabeledField>
              <LabeledField label="Hub wildcard" fieldId="hubWildcard" help={getFieldHelp('site.wildcard')}>
                <TextInput
                  id="hubWildcard"
                  value={form.hubWildcard || ''}
                  onChange={(_e, v) => setForm({ ...form, hubWildcard: v })}
                  placeholder={form.hubZone ? `*.${form.hubZone}` : '*.hub.example.com'}
                />
              </LabeledField>
            </>
          ) : (
            <>
              {(profile === 'custom' ? ingressRows : [ingressRows[0]]).map((row, index) => (
                <div key={`ingress-${index}`} style={{ marginTop: index > 0 ? '1rem' : 0 }}>
                  {profile === 'custom' && (
                    <Title headingLevel="h4" size="md">
                      Ingress row {index + 1}
                    </Title>
                  )}
                  {profile === 'custom' && (
                    <LabeledField label="Method" fieldId={`ingressMethod-${index}`}>
                      <FormSelect
                        id={`ingressMethod-${index}`}
                        value={row.method}
                        onChange={(_e, v) => updateIngressAt(index, { method: v as FleetIngressMethod })}
                      >
                        <FormSelectOption value="port_forward" label="port_forward" />
                        <FormSelectOption value="reverse_proxy" label="reverse_proxy" />
                      </FormSelect>
                    </LabeledField>
                  )}
                  <LabeledField
                    label="Ingress ID"
                    fieldId={index === 0 ? 'ingressId' : `ingressId-${index}`}
                    isRequired
                    help={getFieldHelp('site.domainZoneId')}
                  >
                    <TextInput
                      id={index === 0 ? 'ingressId' : `ingressId-${index}`}
                      value={row.id}
                      onChange={(_e, v) => updateIngressAt(index, { id: v })}
                      placeholder="default"
                    />
                  </LabeledField>
                  {row.method !== 'reverse_proxy' && (
                    <LabeledField
                      label="WAN address"
                      fieldId={index === 0 ? 'ingressAddress' : `ingressAddress-${index}`}
                      isRequired={row.method === 'port_forward'}
                      help={getFieldHelp('site.endpointAddress')}
                    >
                      <TextInput
                        id={index === 0 ? 'ingressAddress' : `ingressAddress-${index}`}
                        value={row.address || ''}
                        onChange={(_e, v) => updateIngressAt(index, { address: v })}
                        placeholder="203.0.113.1"
                      />
                    </LabeledField>
                  )}
                  <LabeledField
                    label="DNS zone"
                    fieldId={index === 0 ? 'ingressZone' : `ingressZone-${index}`}
                    isRequired
                    help={getFieldHelp('site.domainZone')}
                  >
                    <TextInput
                      id={index === 0 ? 'ingressZone' : `ingressZone-${index}`}
                      value={row.zone || ''}
                      onChange={(_e, v) => updateIngressAt(index, { zone: v })}
                      placeholder="lab.example.com"
                    />
                  </LabeledField>
                  <LabeledField
                    label="Wildcard"
                    fieldId={index === 0 ? 'ingressWildcard' : `ingressWildcard-${index}`}
                    help={getFieldHelp('site.wildcard')}
                  >
                    <TextInput
                      id={index === 0 ? 'ingressWildcard' : `ingressWildcard-${index}`}
                      value={row.wildcard || ''}
                      onChange={(_e, v) => updateIngressAt(index, { wildcard: v })}
                      placeholder={row.zone ? `*.${row.zone}` : '*.lab.example.com'}
                    />
                  </LabeledField>
                </div>
              ))}
              {profile === 'custom' && (
                <Button variant="secondary" onClick={addIngressRow} style={{ marginTop: '0.5rem' }}>
                  Add ingress row
                </Button>
              )}
              {profile === 'port_forward' && (
                <p style={{ fontSize: '0.85rem', marginTop: '-0.25rem' }}>
                  Creates one <code>port_forward</code> ingress row. Add reverse-proxy rows on the site
                  detail page.
                </p>
              )}
            </>
          )}
          <LabeledField label="DNS target note" fieldId="dnsNote" help={getFieldHelp('site.dnsNote')}>
            <TextInput
              id="dnsNote"
              value={form.wanEgress?.dnsTargetNote || ''}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  wanEgress: { ...form.wanEgress, enabled: true, dnsTargetNote: v },
                })
              }
            />
          </LabeledField>
        </Form>
      </Modal>

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
            isDisabled={!deleteTarget || deleteConfirm !== siteName(deleteTarget)}
            data-testid="site-delete-confirm"
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
          Delete registry site <strong>{deleteTarget ? siteName(deleteTarget) : ''}</strong>? Edge
          domain registry entries for this site are removed. This cannot be undone.
        </p>
        <LabeledField label="Type site name to confirm" fieldId="deleteConfirm" isRequired>
          <TextInput
            id="deleteConfirm"
            value={deleteConfirm}
            onChange={(_e, v) => setDeleteConfirm(v)}
            placeholder={deleteTarget ? siteName(deleteTarget) : ''}
            data-testid="site-delete-confirm-name"
          />
        </LabeledField>
      </Modal>
    </>
  );
}
