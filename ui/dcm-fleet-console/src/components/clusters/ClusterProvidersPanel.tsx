import { useCallback, useEffect, useState } from 'react';
import { Button, Label, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  deleteClusterProvider,
  installClusterProvider,
  listClusterProviders,
} from '../../api/client';
import type { ClusterProviderKind, ClusterProviderRow } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

type Props = {
  clusterName: string;
  clusterReady: boolean;
  kvmCapability?: boolean;
};

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Installing':
    case 'Deleting':
      return 'orange';
    case 'Failed':
      return 'red';
    case 'NotInstalled':
      return 'grey';
    default:
      return 'blue';
  }
}

function isBusy(row: ClusterProviderRow): boolean {
  return row.phase === 'Installing' || row.phase === 'Deleting';
}

function canDelete(row: ClusterProviderRow): boolean {
  return row.phase === 'Ready' || row.phase === 'Failed';
}

function canInstallRow(
  row: ClusterProviderRow,
  kvmCapability?: boolean,
): { ok: boolean; reason?: string } {
  if (row.phase !== 'NotInstalled' && row.phase !== 'Failed') {
    return { ok: false };
  }
  if (row.kind === 'kubevirt' && kvmCapability === false) {
    return { ok: false, reason: 'Cluster lacks KVM capability' };
  }
  return { ok: true };
}

export function ClusterProvidersPanel({ clusterName, clusterReady, kvmCapability }: Props) {
  const [rows, setRows] = useState<ClusterProviderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [acting, setActing] = useState<ClusterProviderKind | null>(null);

  const load = useCallback(async () => {
    if (!clusterReady) {
      setRows([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      setRows(await listClusterProviders(clusterName));
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [clusterName, clusterReady]);

  useEffect(() => {
    void load();
    const busy = rows.some(isBusy);
    const interval = setInterval(() => void load(), busy ? 3000 : 10000);
    return () => clearInterval(interval);
  }, [load, rows]);

  const runAction = async (kind: ClusterProviderKind, action: 'install' | 'delete') => {
    setActing(kind);
    setActionError(null);
    try {
      if (action === 'install') {
        await installClusterProvider(clusterName, kind);
      } else {
        await deleteClusterProvider(clusterName, kind);
      }
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(null);
    }
  };

  if (!clusterReady) {
    return null;
  }

  return (
    <>
      <Title headingLevel="h2" size="lg" style={{ marginTop: '1.5rem' }}>
        DCM providers
      </Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '42rem' }}>
        Install providers after the cluster is Ready. Container and helm catalog deployments require the
        matching provider.
      </p>
      <ErrorAlert error={error} />
      <ErrorAlert error={actionError} />
      {loading && rows.length === 0 && <p>Loading providers…</p>}
      <Table variant="compact" style={{ marginTop: '0.75rem', maxWidth: '56rem' }}>
        <Thead>
          <Tr>
            <Th>Type</Th>
            <Th>Provider</Th>
            <Th>NodePort</Th>
            <Th>Status</Th>
            <Th>Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {rows.length === 0 && !loading ? (
            <Tr>
              <Td colSpan={5}>No provider types configured</Td>
            </Tr>
          ) : (
            rows.map((row) => {
              const installGate = canInstallRow(row, kvmCapability);
              return (
                <Tr key={row.kind}>
                  <Td>{row.displayName}</Td>
                  <Td>
                    <code>{row.providerName}</code>
                  </Td>
                  <Td>{row.nodePort ?? '—'}</Td>
                  <Td>
                    <Label color={phaseColor(row.phase)}>{row.phase}</Label>
                    {row.message ? (
                      <span style={{ marginLeft: '0.5rem', fontSize: '0.85rem' }}>{row.message}</span>
                    ) : null}
                    {!installGate.ok && installGate.reason ? (
                      <span style={{ marginLeft: '0.5rem', fontSize: '0.85rem', color: 'var(--pf-v6-global--disabled-color--100)' }}>
                        {installGate.reason}
                      </span>
                    ) : null}
                  </Td>
                  <Td>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      <Button
                        variant="primary"
                        size="sm"
                        isDisabled={!installGate.ok || isBusy(row) || acting !== null}
                        isLoading={acting === row.kind && (row.phase === 'Installing' || acting !== null)}
                        onClick={() => void runAction(row.kind, 'install')}
                      >
                        Install
                      </Button>
                      <Button
                        variant="danger"
                        size="sm"
                        isDisabled={!canDelete(row) || isBusy(row) || acting !== null}
                        onClick={() => void runAction(row.kind, 'delete')}
                      >
                        Delete
                      </Button>
                    </div>
                  </Td>
                </Tr>
              );
            })
          )}
        </Tbody>
      </Table>
    </>
  );
}
