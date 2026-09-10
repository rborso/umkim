import { useEffect, useState } from 'react';
import { PageSection, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { listAuditEvents } from '../api/authClient';
import { useAuth } from '../auth/AuthContext';
import { Navigate } from 'react-router-dom';

export function AuditPage() {
  const { user } = useAuth();
  const [rows, setRows] = useState<Record<string, unknown>[]>([]);
  useEffect(() => {
    if (user?.dcAdmin) void listAuditEvents().then((e) => setRows(e as Record<string, unknown>[]));
  }, [user?.dcAdmin]);
  if (!user?.dcAdmin) return <Navigate to="/" replace />;
  return (
    <PageSection>
      <Title headingLevel="h1">Audit log</Title>
      <Table variant="compact">
        <Thead><Tr><Th>Time</Th><Th>Actor</Th><Th>Action</Th><Th>Result</Th><Th>Path</Th></Tr></Thead>
        <Tbody>
          {rows.slice(0, 200).map((ev) => {
            const spec = (ev.spec ?? {}) as Record<string, unknown>;
            const meta = (ev.metadata ?? {}) as Record<string, string>;
            const actor = (spec.actor ?? {}) as Record<string, string>;
            const http = (spec.http ?? {}) as Record<string, string>;
            return (
              <Tr key={meta.name}>
                <Td>{String(spec.timestamp ?? '')}</Td>
                <Td>{actor.kind}:{actor.name}</Td>
                <Td>{String(spec.action ?? '')}</Td>
                <Td>{String(spec.result ?? '')}</Td>
                <Td>{http.path ?? ''}</Td>
              </Tr>
            );
          })}
        </Tbody>
      </Table>
    </PageSection>
  );
}
