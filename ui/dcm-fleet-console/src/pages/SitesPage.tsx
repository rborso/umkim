import { useCallback, useEffect, useState } from 'react';
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
  fleetSiteEffectiveDomain,
  fleetSiteEffectiveId,
  fleetSiteIsManagement,
  fleetSiteIsRegistry,
  fleetSitePhase,
  listFleetSites,
  provisionFleetSite,
  validateFleetSite,
} from '../api/client';
import type { FleetExposureMode, FleetSite, FleetSiteCreate } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { EXPOSURE_TLS_DOCS_URL, recommendedTlsForExposure } from '../edge/exposureTlsPairs';
import { getFieldHelp } from '../help/fieldHelp';
import { useAuth } from '../auth/AuthContext';
import { isCustomerUser, primaryOrg } from '../auth/orgUtils';

const emptyForm = (): FleetSiteCreate => ({
  siteName: '',
  displayName: '',
  exposureMode: 'port_forward',
  edgeDomain: '',
  wildcard: '',
  wanEgress: { enabled: true, dnsTargetNote: '' },
});

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Enrolled':
    case 'Validated':
    case 'ScriptIssued':
      return 'blue';
    case 'Provisioning':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

export function SitesPage() {
  const { user } = useAuth();
  const orgSlug = primaryOrg(user);
  const [rows, setRows] = useState<FleetSite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<FleetSiteCreate>(emptyForm());
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<FleetSite | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRows(await listFleetSites());
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

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
      const payload = {
        ...form,
        orgId: user?.dcAdmin ? form.orgId : orgSlug,
      };
      await createFleetSite(payload);
      setModalOpen(false);
      setForm(emptyForm());
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
                onClick={() => {
                  setForm({ ...emptyForm(), orgId: user?.dcAdmin ? undefined : orgSlug });
                  setModalOpen(true);
                }}
              >
                Create site
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={() => void load()}>
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
              <Th>Edge domain</Th>
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
                  <Td>
                    <code>{fleetSiteEffectiveDomain(s) || '—'}</code>
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
              placeholder={orgSlug ? `${orgSlug}-site-a` : 'homelab'}
            />
            {isCustomerUser(user) && orgSlug ? (
              <small>
                Names must start with <code>{orgSlug}-</code>
              </small>
            ) : null}
          </LabeledField>
          <LabeledField label="Display name" fieldId="displayName" help={getFieldHelp('site.displayName')}>
            <TextInput
              id="displayName"
              value={form.displayName || ''}
              onChange={(_e, v) => setForm({ ...form, displayName: v })}
            />
          </LabeledField>
          <LabeledField label="Exposure mode" fieldId="exposureMode" help={getFieldHelp('site.exposureMode')}>
            <FormSelect
              id="exposureMode"
              value={form.exposureMode || 'none'}
              onChange={(_e, v) => setForm({ ...form, exposureMode: v as FleetExposureMode })}
            >
              <FormSelectOption value="none" label="None" />
              <FormSelectOption value="port_forward" label="Port forward" />
              <FormSelectOption value="reverse_proxy" label="Reverse proxy" />
            </FormSelect>
          </LabeledField>
          <p style={{ fontSize: '0.85rem', marginTop: '-0.25rem' }}>
            Default TLS for this exposure: <code>{recommendedTlsForExposure(form.exposureMode || 'none')}</code>
            {' · '}
            Override per domain on the site page if needed — mismatched pairs show a warning.{' '}
            <a href={EXPOSURE_TLS_DOCS_URL} target="_blank" rel="noreferrer">
              Working pairs
            </a>
          </p>
          <LabeledField label="Primary edge domain" fieldId="edgeDomain" help={getFieldHelp('site.edgeDomain')}>
            <TextInput
              id="edgeDomain"
              value={form.edgeDomain || ''}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  edgeDomain: v,
                  wildcard: form.wildcard || (v ? `*.${v}` : ''),
                })
              }
              placeholder="lab.example.com"
            />
          </LabeledField>
          <LabeledField label="Wildcard" fieldId="wildcard" help={getFieldHelp('site.wildcard')}>
            <TextInput
              id="wildcard"
              value={form.wildcard || ''}
              onChange={(_e, v) => setForm({ ...form, wildcard: v })}
              placeholder="*.lab.example.com"
            />
          </LabeledField>
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
