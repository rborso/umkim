import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Form,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
} from '@patternfly/react-table';
import { PlusIcon, SyncIcon } from '@patternfly/react-icons';
import {
  createProvider,
  listFleetClusters,
  listFleetSites,
  listProviders,
  providerCluster,
  providerDisplayName,
  providerHealth,
  providerSite,
  updateProvider,
} from '../api/client';
import type { FleetCluster, FleetSite, Provider, ProviderRegistration } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';
import { Label } from '@patternfly/react-core';

const emptyForm = (): ProviderRegistration => ({
  name: '',
  displayName: '',
  serviceType: 'container',
  schemaVersion: 'v1alpha1',
  operations: ['CREATE', 'DELETE', 'READ'],
  endpoint: '',
  region: 'fleet',
  zone: '',
  metadata: {},
});

function providerToForm(p: Provider): ProviderRegistration {
  return {
    name: p.name || '',
    displayName: p.display_name || p.displayName || '',
    serviceType: p.service_type || p.serviceType || 'container',
    schemaVersion: p.schema_version || p.schemaVersion || 'v1alpha1',
    operations: p.operations || ['CREATE', 'DELETE', 'READ'],
    endpoint: p.endpoint || '',
    region: p.metadata?.region_code || p.region || 'fleet',
    zone: p.metadata?.zone || p.zone || '',
    metadata: p.metadata || {},
  };
}

export function ProvidersPage() {
  const [rows, setRows] = useState<Provider[]>([]);
  const [clusters, setClusters] = useState<FleetCluster[]>([]);
  const [sites, setSites] = useState<FleetSite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<ProviderRegistration>(emptyForm());
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [providers, clusterList, siteList] = await Promise.all([
        listProviders(),
        listFleetClusters().catch(() => [] as FleetCluster[]),
        listFleetSites().catch(() => [] as FleetSite[]),
      ]);
      setRows(providers);
      setClusters(clusterList);
      setSites(siteList);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditId(null);
    setForm(emptyForm());
    setSaveError(null);
    setModalOpen(true);
  };

  const openEdit = (p: Provider) => {
    setEditId(p.id || null);
    setForm(providerToForm(p));
    setSaveError(null);
    setModalOpen(true);
  };

  const save = async () => {
    setSaving(true);
    setSaveError(null);
    try {
      const ops = form.operations;
      const body: ProviderRegistration = {
        ...form,
        operations: Array.isArray(ops) ? ops : String(ops).split(',').map((s) => s.trim()),
      };
      if (editId) {
        await updateProvider(editId, body);
      } else {
        await createProvider(body);
      }
      setModalOpen(false);
      await load();
    } catch (e) {
      setSaveError(e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Service providers
        </Title>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="primary" icon={<PlusIcon />} onClick={openCreate}>
                Register provider
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={load}>
                Refresh
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        <Table aria-label="Providers" variant="compact">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Site</Th>
              <Th>Cluster</Th>
              <Th>Health</Th>
              <Th>Endpoint</Th>
              <Th>ID</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {rows.length === 0 && !loading ? (
              <Tr>
                <Td colSpan={7}>No providers registered</Td>
              </Tr>
            ) : (
              rows.map((p) => (
                <Tr key={p.id || p.name}>
                  <Td>{providerDisplayName(p)}</Td>
                  <Td>{providerSite(p, clusters, sites)}</Td>
                  <Td>{providerCluster(p, clusters)}</Td>
                  <Td>
                    <Label color={providerHealth(p) === 'ready' ? 'green' : 'orange'}>
                      {providerHealth(p)}
                    </Label>
                  </Td>
                  <Td>
                    <code style={{ fontSize: '0.8rem' }}>{p.endpoint || '—'}</code>
                  </Td>
                  <Td>
                    <code style={{ fontSize: '0.75rem' }}>{p.id || '—'}</code>
                  </Td>
                  <Td>
                    <Button variant="link" isInline onClick={() => openEdit(p)}>
                      Edit
                    </Button>
                  </Td>
                </Tr>
              ))
            )}
          </Tbody>
        </Table>
      </PageSection>

      <Modal
        variant={ModalVariant.medium}
        title={editId ? 'Edit provider' : 'Register provider'}
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        actions={[
          <Button key="save" variant="primary" onClick={save} isLoading={saving}>
            Save
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setModalOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={saveError} />
        <Form>
          <LabeledField label="Name" isRequired fieldId="p-name" help={getFieldHelp('provider.name')}>
            <TextInput
              id="p-name"
              value={form.name}
              onChange={(_e, v) => setForm({ ...form, name: v })}
              isDisabled={!!editId}
            />
          </LabeledField>
          <LabeledField label="Display name" fieldId="p-display" help={getFieldHelp('provider.displayName')}>
            <TextInput
              id="p-display"
              value={form.displayName || ''}
              onChange={(_e, v) => setForm({ ...form, displayName: v })}
            />
          </LabeledField>
          <LabeledField label="Service type" isRequired fieldId="p-stype" help={getFieldHelp('provider.serviceType')}>
            <TextInput
              id="p-stype"
              value={form.serviceType}
              onChange={(_e, v) => setForm({ ...form, serviceType: v })}
            />
          </LabeledField>
          <LabeledField label="Endpoint" isRequired fieldId="p-endpoint" help={getFieldHelp('provider.endpoint')}>
            <TextInput
              id="p-endpoint"
              value={form.endpoint}
              onChange={(_e, v) => setForm({ ...form, endpoint: v })}
            />
          </LabeledField>
          <LabeledField label="Zone" fieldId="p-zone" help={getFieldHelp('provider.zone')}>
            <TextInput
              id="p-zone"
              value={form.zone || ''}
              onChange={(_e, v) => setForm({ ...form, zone: v })}
            />
          </LabeledField>
          <LabeledField label="Operations (comma-separated)" fieldId="p-ops" help={getFieldHelp('provider.operations')}>
            <TextInput
              id="p-ops"
              value={form.operations.join(', ')}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  operations: v.split(',').map((s) => s.trim()).filter(Boolean),
                })
              }
            />
          </LabeledField>
        </Form>
      </Modal>
    </>
  );
}
