import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  PageSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Navigate } from 'react-router-dom';
import { createInvite, deleteInvite, listInvites } from '../api/client';
import type { Invite } from '../api/types';
import { useAuth } from '../auth/AuthContext';

function parseGroups(value: string): string[] {
  return value
    .split(',')
    .map((g) => g.trim())
    .filter(Boolean);
}

export function InvitesPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Invite[]>([]);
  const [groups, setGroups] = useState('');
  const [ttlHours, setTtlHours] = useState('72');
  const [note, setNote] = useState('');
  const [link, setLink] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => {
    void listInvites()
      .then(setRows)
      .catch((e) => setError(String(e)));
  };

  useEffect(() => {
    if (user?.dcAdmin) load();
  }, [user?.dcAdmin]);

  if (!user?.dcAdmin) return <Navigate to="/" replace />;

  const submit = async () => {
    setError(null);
    setLink('');
    const list = parseGroups(groups);
    if (list.length === 0) {
      setError('Enter at least one tenant group.');
      return;
    }
    setBusy(true);
    try {
      const res = await createInvite({
        groups: list,
        ttlHours: Number.parseInt(ttlHours, 10) || undefined,
        note: note || undefined,
      });
      setLink(window.location.origin + res.path);
      setGroups('');
      setNote('');
      load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (id: string) => {
    setError(null);
    try {
      await deleteInvite(id);
      load();
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <PageSection>
      <Title headingLevel="h1">Invitations</Title>
      <Alert variant="info" isInline title="One-time directory invitations" style={{ marginBottom: '1rem' }}>
        Create a single-use link for a new user. Redeeming it creates the user in the shared directory,
        adds the tenant groups, and sets the initial password. Links expire after 72 hours by default.
      </Alert>
      {error ? (
        <Alert variant="danger" isInline title={error} style={{ marginBottom: '1rem' }} />
      ) : null}
      <Form onSubmit={(e) => { e.preventDefault(); void submit(); }} style={{ marginBottom: '1.5rem' }}>
        <FormGroup label="Tenant groups (comma-separated)" isRequired fieldId="invite-groups">
          <TextInput
            id="invite-groups"
            value={groups}
            onChange={(_e, v) => setGroups(v)}
            placeholder="tenant-a, tenant-b"
          />
        </FormGroup>
        <FormGroup label="Expires in (hours)" fieldId="invite-ttl">
          <TextInput
            id="invite-ttl"
            type="number"
            min={1}
            value={ttlHours}
            onChange={(_e, v) => setTtlHours(v)}
          />
        </FormGroup>
        <FormGroup label="Note" fieldId="invite-note">
          <TextInput id="invite-note" value={note} onChange={(_e, v) => setNote(v)} placeholder="optional" />
        </FormGroup>
        <Button type="submit" variant="primary" isLoading={busy} isDisabled={busy}>
          Create invite
        </Button>
      </Form>

      {link ? (
        <Alert variant="success" isInline title="Invite link (shown once)" style={{ marginBottom: '1.5rem' }}>
          <TextInput value={link} readOnlyVariant="default" aria-label="invite link" />
        </Alert>
      ) : null}

      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Created</Th>
            <Th>Groups</Th>
            <Th>Expires</Th>
            <Th>Status</Th>
            <Th>By</Th>
            <Th />
          </Tr>
        </Thead>
        <Tbody>
          {rows.map((inv) => (
            <Tr key={inv.id}>
              <Td>{inv.createdAt}</Td>
              <Td>{(inv.groups ?? []).join(', ')}</Td>
              <Td>{inv.expiresAt}</Td>
              <Td>
                {inv.used ? `used by ${inv.redeemedBy ?? '?'}` : inv.expired ? 'expired' : 'active'}
              </Td>
              <Td>{inv.createdBy ?? ''}</Td>
              <Td>
                <Button variant="link" isDanger onClick={() => void revoke(inv.id)}>
                  Revoke
                </Button>
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </PageSection>
  );
}
