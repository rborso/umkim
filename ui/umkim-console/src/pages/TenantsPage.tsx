import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  Label,
  PageSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Navigate } from 'react-router-dom';
import {
  addTenantMember,
  createTenant,
  deleteTenant,
  listTenants,
  removeTenantMember,
} from '../api/client';
import type { DirectoryUser, TenantGroup } from '../api/types';
import { useAuth } from '../auth/AuthContext';

export function TenantsPage() {
  const { user } = useAuth();
  const [groups, setGroups] = useState<TenantGroup[]>([]);
  const [users, setUsers] = useState<DirectoryUser[]>([]);
  const [newName, setNewName] = useState('');
  const [memberInput, setMemberInput] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => {
    void listTenants()
      .then((d) => {
        setGroups(d.groups ?? []);
        setUsers(d.users ?? []);
      })
      .catch((e) => setError(String(e)));
  };

  useEffect(() => {
    if (user?.dcAdmin) load();
  }, [user?.dcAdmin]);

  if (!user?.dcAdmin) return <Navigate to="/" replace />;

  const run = async (fn: () => Promise<void>) => {
    setError(null);
    setBusy(true);
    try {
      await fn();
      load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageSection>
      <Title headingLevel="h1">Tenants</Title>
      <Alert variant="info" isInline title="Directory groups" style={{ marginBottom: '1rem' }}>
        Tenants are directory groups. Route access is restricted to a tenant with the app route
        field <code>requireAuthGroups</code>. LLDAP internal groups are read-only here.
      </Alert>
      {error ? <Alert variant="danger" isInline title={error} style={{ marginBottom: '1rem' }} /> : null}
      <Form
        onSubmit={(e) => {
          e.preventDefault();
          void run(() => createTenant(newName).then(() => setNewName('')));
        }}
        style={{ marginBottom: '1.5rem' }}
      >
        <FormGroup label="New tenant group" fieldId="tenant-new">
          <TextInput id="tenant-new" value={newName} onChange={(_e, v) => setNewName(v)} placeholder="tenant-a" />
        </FormGroup>
        <Button type="submit" variant="primary" isDisabled={busy || !newName.trim()}>
          Create tenant
        </Button>
      </Form>

      <Table variant="compact">
        <Thead>
          <Tr>
            <Th>Tenant</Th>
            <Th>Members</Th>
            <Th>Add member</Th>
            <Th />
          </Tr>
        </Thead>
        <Tbody>
          {groups.map((g) => (
            <Tr key={g.name}>
              <Td>
                {g.name} {g.system ? <Label isCompact color="grey">system</Label> : null}
              </Td>
              <Td>
                {(g.members ?? []).map((m) => (
                  <Button
                    key={m.id}
                    variant="plain"
                    isDanger
                    onClick={() => void run(() => removeTenantMember(g.name, m.id))}
                    title="Remove from tenant"
                  >
                    {m.id} ✕
                  </Button>
                ))}
              </Td>
              <Td>
                <TextInput
                  value={memberInput[g.name] ?? ''}
                  onChange={(_e, v) => setMemberInput((s) => ({ ...s, [g.name]: v }))}
                  placeholder="username"
                  list={`tenant-users-${g.name}`}
                  aria-label={`add member to ${g.name}`}
                />
                <datalist id={`tenant-users-${g.name}`}>
                  {users.map((u) => (
                    <option key={u.id} value={u.id} />
                  ))}
                </datalist>
                <Button
                  variant="secondary"
                  isDisabled={!(memberInput[g.name] ?? '').trim()}
                  onClick={() =>
                    void run(() =>
                      addTenantMember(g.name, (memberInput[g.name] ?? '').trim()).then(() =>
                        setMemberInput((s) => ({ ...s, [g.name]: '' })),
                      ),
                    )
                  }
                >
                  Add
                </Button>
              </Td>
              <Td>
                {!g.system ? (
                  <Button variant="link" isDanger onClick={() => void run(() => deleteTenant(g.name))}>
                    Delete
                  </Button>
                ) : null}
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </PageSection>
  );
}
