import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  FormGroup,
  PageSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Link, Navigate } from 'react-router-dom';
import { createInvite, deleteInvite, listInvites, listTenants } from '../api/client';
import type { Invite } from '../api/types';
import { useAuth } from '../auth/AuthContext';

export function InvitesPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Invite[]>([]);
  const [tenants, setTenants] = useState<string[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [ttlHours, setTtlHours] = useState('72');
  const [note, setNote] = useState('');
  const [links, setLinks] = useState<{ cluster: string; url: string }[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => {
    void listInvites()
      .then(setRows)
      .catch((e) => setError(String(e)));
    void listTenants()
      .then((d) => setTenants((d.groups ?? []).filter((g) => !g.system).map((g) => g.name)))
      .catch((e) => setError(String(e)));
  };

  useEffect(() => {
    if (user?.dcAdmin) load();
  }, [user?.dcAdmin]);

  if (!user?.dcAdmin) return <Navigate to="/" replace />;

  const toggle = (name: string, checked: boolean) => {
    setSelected((s) => (checked ? [...s, name] : s.filter((x) => x !== name)));
  };

  const submit = async () => {
    setError(null);
    setLinks([]);
    if (selected.length === 0) {
      setError('Select at least one tenant group.');
      return;
    }
    setBusy(true);
    try {
      const res = await createInvite({
        groups: selected,
        ttlHours: Number.parseInt(ttlHours, 10) || undefined,
        note: note || undefined,
      });
      setLinks(
        res.links && res.links.length
          ? res.links
          : [{ cluster: 'hub', url: window.location.origin + res.path }],
      );
      setSelected([]);
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
        adds the selected tenant groups, and sets the initial password. Links expire after 72 hours by default.
      </Alert>
      {error ? (
        <Alert variant="danger" isInline title={error} style={{ marginBottom: '1rem' }} />
      ) : null}
      <Form onSubmit={(e) => { e.preventDefault(); void submit(); }} style={{ marginBottom: '1.5rem' }}>
        <FormGroup label="Tenant groups" isRequired fieldId="invite-groups">
          {tenants.length === 0 ? (
            <Alert variant="warning" isInline title="No tenant groups yet">
              Create one on the <Link to="/admin/tenants">Tenants</Link> page first.
            </Alert>
          ) : (
            <div>
              {tenants.map((g) => (
                <Checkbox
                  key={g}
                  id={`invite-group-${g}`}
                  label={g}
                  isChecked={selected.includes(g)}
                  onChange={(_e, checked) => toggle(g, checked)}
                />
              ))}
            </div>
          )}
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
        <Button
          type="submit"
          variant="primary"
          isLoading={busy}
          isDisabled={busy || selected.length === 0}
        >
          Create invite
        </Button>
      </Form>

      {links.length ? (
        <Alert variant="success" isInline title="Invite link (shown once)" style={{ marginBottom: '1.5rem' }}>
          {links.map((l) => (
            <div key={l.cluster} style={{ marginBottom: '0.5rem' }}>
              <div style={{ fontSize: '0.85rem' }}>{l.cluster}</div>
              <TextInput
                value={l.url}
                readOnlyVariant="default"
                aria-label={`invite link ${l.cluster}`}
              />
            </div>
          ))}
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
              <Td dataLabel="Created">{inv.createdAt}</Td>
              <Td dataLabel="Groups">{(inv.groups ?? []).join(', ')}</Td>
              <Td dataLabel="Expires">{inv.expiresAt}</Td>
              <Td dataLabel="Status">
                {inv.used ? `used by ${inv.redeemedBy ?? '?'}` : inv.expired ? 'expired' : 'active'}
              </Td>
              <Td dataLabel="By">{inv.createdBy ?? ''}</Td>
              <Td dataLabel="Actions">
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
