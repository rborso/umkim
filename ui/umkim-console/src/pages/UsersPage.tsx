import { useEffect, useState } from 'react';
import { PageSection, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { listAuthUsers } from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { Navigate } from 'react-router-dom';

export function UsersPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  useEffect(() => {
    if (user?.dcAdmin) void listAuthUsers().then((u) => setRows(u as Record<string, unknown>[]));
  }, [user?.dcAdmin]);
  if (!user?.dcAdmin) return <Navigate to="/" replace />;
  return (
    <PageSection>
      <Title headingLevel="h1">Users</Title>
      <Table variant="compact">
        <Thead><Tr><Th>Name</Th><Th>Display</Th><Th>Auth</Th><Th>Groups</Th></Tr></Thead>
        <Tbody>
          {rows.map((u) => {
            const spec = (u.spec ?? {}) as Record<string, unknown>;
            const meta = (u.metadata ?? {}) as Record<string, string>;
            return (
              <Tr key={meta.name}>
                <Td dataLabel="Name">{meta.name}</Td>
                <Td dataLabel="Display">{String(spec.displayName ?? '')}</Td>
                <Td dataLabel="Auth">{String(spec.authMethod ?? '')}</Td>
                <Td dataLabel="Groups">{Array.isArray(spec.groups) ? spec.groups.join(', ') : ''}</Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>
    </PageSection>
  );
}
