import { useEffect, useState } from 'react';
import {
  Button,
  Form,
  Modal,
  ModalVariant,
  PageSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Navigate } from 'react-router-dom';
import {
  createOrg,
  deleteOrg,
  listOrgs,
  type FleetOrg,
  type FleetOrgCreate,
} from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';

const emptyForm = (): FleetOrgCreate => ({
  slug: '',
  displayName: '',
  sharedQuota: { cpuMilli: 8000, memoryBytes: 16 * 1024 * 1024 * 1024 },
});

export function OrgsPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<FleetOrg[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<FleetOrgCreate>(emptyForm());
  const [saving, setSaving] = useState(false);

  const load = async () => {
    setError(null);
    try {
      setRows(await listOrgs());
    } catch (e) {
      setError(e);
    }
  };

  useEffect(() => {
    if (user?.dcAdmin) void load();
  }, [user?.dcAdmin]);

  if (!user?.dcAdmin) return <Navigate to="/" replace />;

  const handleCreate = async () => {
    setSaving(true);
    setError(null);
    try {
      await createOrg(form);
      setModalOpen(false);
      setForm(emptyForm());
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageSection>
      <Title headingLevel="h1">Organizations</Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '48rem' }}>
        Platform admins create org slugs. Customer site and cluster names must use the{' '}
        <code>{'{slug}-*'}</code> prefix.
      </p>
      <ErrorAlert error={error} />
      <Button variant="primary" onClick={() => setModalOpen(true)} style={{ marginBottom: '1rem' }}>
        Create org
      </Button>
      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Slug</Th>
            <Th>Display name</Th>
            <Th>Shared CPU (m)</Th>
            <Th>Shared memory</Th>
            <Th>Usage CPU</Th>
            <Th>Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {rows.map((o) => {
            const slug = o.spec.slug || o.metadata?.name || '—';
            return (
              <Tr key={slug}>
                <Td>{slug}</Td>
                <Td>{o.spec.displayName || '—'}</Td>
                <Td>{o.spec.sharedQuota?.cpuMilli ?? '—'}</Td>
                <Td>{o.spec.sharedQuota?.memoryBytes ?? '—'}</Td>
                <Td>{o.status?.sharedUsage?.cpuMilli ?? '—'}</Td>
                <Td>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => void deleteOrg(slug).then(load)}
                  >
                    Delete
                  </Button>
                </Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>

      <Modal
        variant={ModalVariant.medium}
        title="Create organization"
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
          <LabeledField label="Slug" fieldId="org-slug" isRequired help={getFieldHelp('org.slug')}>
            <TextInput
              id="org-slug"
              value={form.slug}
              onChange={(_e, v) => setForm((f) => ({ ...f, slug: v }))}
              placeholder="acme"
            />
          </LabeledField>
          <LabeledField label="Display name" fieldId="org-display" help={getFieldHelp('org.displayName')}>
            <TextInput
              id="org-display"
              value={form.displayName || ''}
              onChange={(_e, v) => setForm((f) => ({ ...f, displayName: v }))}
            />
          </LabeledField>
          <LabeledField
            label="Shared CPU quota (millicores)"
            fieldId="org-cpu"
            help={getFieldHelp('org.sharedCpuQuota')}
          >            <TextInput
              id="org-cpu"
              type="number"
              value={String(form.sharedQuota?.cpuMilli ?? '')}
              onChange={(_e, v) =>
                setForm((f) => ({
                  ...f,
                  sharedQuota: { ...f.sharedQuota, cpuMilli: Number(v) || 0 },
                }))
              }
            />
          </LabeledField>
        </Form>
      </Modal>
    </PageSection>
  );
}
