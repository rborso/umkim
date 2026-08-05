import { useEffect, useState } from 'react';
import { Button, Form, Modal, ModalVariant, PageSection, TextInput, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Navigate } from 'react-router-dom';
import {
  createOrgRoleBinding,
  deleteOrgRoleBinding,
  listOrgRoleBindings,
  listOrgUsers,
} from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { isOrgAdmin, primaryOrg } from '../auth/orgUtils';
import { ErrorAlert } from '../components/ErrorAlert';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';

export function MembersPage() {
  const { user } = useAuth();
  const org = primaryOrg(user);
  const [users, setUsers] = useState<Record<string, unknown>[]>([]);
  const [bindings, setBindings] = useState<Record<string, unknown>[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [userName, setUserName] = useState('');
  const [roleName, setRoleName] = useState('customer');
  const [saving, setSaving] = useState(false);

  const allowed = user?.dcAdmin || isOrgAdmin(user);

  const load = async () => {
    if (!org) return;
    setError(null);
    try {
      const [u, rb] = await Promise.all([listOrgUsers(org), listOrgRoleBindings(org)]);
      setUsers(u as Record<string, unknown>[]);
      setBindings(rb as Record<string, unknown>[]);
    } catch (e) {
      setError(e);
    }
  };

  useEffect(() => {
    if (allowed && org) void load();
  }, [allowed, org]);

  if (!allowed) return <Navigate to="/" replace />;
  if (!org && !user?.dcAdmin) {
    return (
      <PageSection>
        <Title headingLevel="h1">Members</Title>
        <p>No organization scope on your account.</p>
      </PageSection>
    );
  }

  const handleBind = async () => {
    if (!org || !userName.trim()) return;
    setSaving(true);
    setError(null);
    const name = `${org}-${userName.trim()}-${roleName}`;
    try {
      await createOrgRoleBinding(org, {
        apiVersion: 'console.umkim.com/v1alpha1',
        kind: 'FleetRoleBinding',
        metadata: { name },
        spec: {
          roleRef: { name: roleName },
          subjects: [{ kind: 'User', name: userName.trim() }],
          scopes: [{ kind: 'Org', name: org }],
        },
      });
      setModalOpen(false);
      setUserName('');
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageSection>
      <Title headingLevel="h1">Members · {org}</Title>
      <p style={{ marginTop: '0.5rem' }}>
        Manage users and role bindings within your organization. Cannot grant <code>dc-admin</code>.
      </p>
      <ErrorAlert error={error} />
      <Button variant="primary" onClick={() => setModalOpen(true)} style={{ marginBottom: '1rem' }}>
        Add role binding
      </Button>
      <Title headingLevel="h2" size="lg">
        Users
      </Title>
      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Display</Th>
          </Tr>
        </Thead>
        <Tbody>
          {users.map((u) => {
            const spec = (u.spec ?? {}) as Record<string, unknown>;
            const meta = (u.metadata ?? {}) as Record<string, string>;
            return (
              <Tr key={meta.name}>
                <Td>{meta.name}</Td>
                <Td>{String(spec.displayName ?? '')}</Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>
      <Title headingLevel="h2" size="lg" style={{ marginTop: '1.5rem' }}>
        Role bindings
      </Title>
      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Role</Th>
            <Th>Subject</Th>
            <Th>Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {bindings.map((rb) => {
            const spec = (rb.spec ?? {}) as Record<string, unknown>;
            const meta = (rb.metadata ?? {}) as Record<string, string>;
            const roleRef = (spec.roleRef ?? {}) as Record<string, string>;
            const subjects = (spec.subjects ?? []) as { kind?: string; name?: string }[];
            const sub = subjects[0];
            return (
              <Tr key={meta.name}>
                <Td>{meta.name}</Td>
                <Td>{roleRef.name}</Td>
                <Td>
                  {sub ? `${sub.kind}/${sub.name}` : '—'}
                </Td>
                <Td>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => org && void deleteOrgRoleBinding(org, meta.name).then(load)}
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
        variant={ModalVariant.small}
        title="Add role binding"
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        actions={[
          <Button key="save" variant="primary" isLoading={saving} onClick={() => void handleBind()}>
            Create
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setModalOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <LabeledField label="Username" fieldId="member-user" isRequired help={getFieldHelp('member.username')}>
            <TextInput id="member-user" value={userName} onChange={(_e, v) => setUserName(v)} />
          </LabeledField>
          <LabeledField label="Role" fieldId="member-role" isRequired help={getFieldHelp('member.role')}>
            <TextInput
              id="member-role"
              value={roleName}
              onChange={(_e, v) => setRoleName(v)}
              placeholder="customer or org-admin"
            />
          </LabeledField>
        </Form>
      </Modal>
    </PageSection>
  );
}
