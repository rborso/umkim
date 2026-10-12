import { useEffect, useState } from 'react';
import { PageSection, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { listRoleBindings } from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { Navigate } from 'react-router-dom';

export function RoleBindingsPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  useEffect(() => {
    if (user?.dcAdmin) void listRoleBindings().then((r) => setRows(r as Record<string, unknown>[]));
  }, [user?.dcAdmin]);
  if (!user?.dcAdmin) return <Navigate to="/" replace />;
  return (
    <PageSection>
      <Title headingLevel="h1">Role bindings</Title>
      <Table variant="compact">
        <Thead><Tr><Th>Name</Th><Th>Role</Th><Th>Subjects</Th><Th>Scopes</Th></Tr></Thead>
        <Tbody>
          {rows.map((rb) => {
            const spec = (rb.spec ?? {}) as Record<string, unknown>;
            const meta = (rb.metadata ?? {}) as Record<string, string>;
            const roleRef = (spec.roleRef ?? {}) as Record<string, string>;
            const subjects = (spec.subjects as { kind: string; name: string }[] | undefined) ?? [];
            const scopes = (spec.scopes as { kind: string; name: string }[] | undefined) ?? [];
            return (
              <Tr key={meta.name}>
                <Td dataLabel="Name">{meta.name}</Td>
                <Td dataLabel="Role">{roleRef.name}</Td>
                <Td dataLabel="Subjects">{subjects.map((s) => `${s.kind}:${s.name}`).join(', ')}</Td>
                <Td dataLabel="Scopes">{scopes.map((s) => `${s.kind}/${s.name}`).join(', ') || '*'}</Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>
    </PageSection>
  );
}
