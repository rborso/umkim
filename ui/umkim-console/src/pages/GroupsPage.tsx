import { useEffect, useState } from 'react';
import { PageSection, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { listAuthGroups } from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { Navigate } from 'react-router-dom';

export function GroupsPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  useEffect(() => {
    if (user?.dcAdmin) void listAuthGroups().then((g) => setRows(g as Record<string, unknown>[]));
  }, [user?.dcAdmin]);
  if (!user?.dcAdmin) return <Navigate to="/" replace />;
  return (
    <PageSection>
      <Title headingLevel="h1">Groups</Title>
      <Table variant="compact">
        <Thead><Tr><Th>Name</Th><Th>LDAP CN</Th><Th>Display</Th></Tr></Thead>
        <Tbody>
          {rows.map((g) => {
            const spec = (g.spec ?? {}) as Record<string, unknown>;
            const meta = (g.metadata ?? {}) as Record<string, string>;
            return (
              <Tr key={meta.name}>
                <Td>{meta.name}</Td>
                <Td>{String(spec.ldapGroupCN ?? '—')}</Td>
                <Td>{String(spec.displayName ?? '')}</Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>
    </PageSection>
  );
}
