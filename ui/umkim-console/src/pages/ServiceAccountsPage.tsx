import { useEffect, useState } from 'react';
import { Alert, PageSection, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { listServiceAccounts } from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { Navigate } from 'react-router-dom';

export function ServiceAccountsPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  useEffect(() => {
    if (user?.dcAdmin) void listServiceAccounts().then((s) => setRows(s as Record<string, unknown>[]));
  }, [user?.dcAdmin]);
  if (!user?.dcAdmin) return <Navigate to="/" replace />;
  return (
    <PageSection>
      <Title headingLevel="h1">Service accounts</Title>
      <Alert variant="info" isInline title="Automation tokens" style={{ marginBottom: '1rem' }}>
        Create service accounts via API or Ansible; use Bearer token for scripts.
      </Alert>
      <Table variant="compact">
        <Thead><Tr><Th>Name</Th><Th>Display</Th></Tr></Thead>
        <Tbody>
          {rows.map((sa) => {
            const spec = (sa.spec ?? {}) as Record<string, unknown>;
            const meta = (sa.metadata ?? {}) as Record<string, string>;
            return (
              <Tr key={meta.name}>
                <Td>{meta.name}</Td>
                <Td>{String(spec.displayName ?? '')}</Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>
    </PageSection>
  );
}
