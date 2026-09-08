import { useEffect, useState } from 'react';
import { Button, Label, Modal, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { fetchSiteProvisionActionLogs, listSiteProvisionActions } from '../../api/client';
import type { SiteProvisionHistoryItem } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

const ACTION_LABELS: Record<string, string> = {
  create_vm: 'Create VM',
  plan_vm: 'Plan VM placement',
  destroy: 'Destroy VM',
  install_bm: 'Install bare metal',
  discover: 'Discover',
  status: 'Status',
};

function phaseColor(phase?: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Succeeded':
      return 'green';
    case 'Running':
    case 'Pending':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

function formatWhen(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

type Props = {
  siteName: string;
  refreshKey: string;
};

export function SiteProvisionHistory({ siteName, refreshKey }: Props) {
  const [items, setItems] = useState<SiteProvisionHistoryItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [historyError, setHistoryError] = useState<unknown>(null);
  const [logOpen, setLogOpen] = useState(false);
  const [logLoading, setLogLoading] = useState(false);
  const [logError, setLogError] = useState<unknown>(null);
  const [logText, setLogText] = useState('');
  const [logTitle, setLogTitle] = useState('');

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setHistoryError(null);
      try {
        const list = await listSiteProvisionActions(siteName);
        if (!cancelled) setItems(list);
      } catch (e) {
        if (!cancelled) setHistoryError(e);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [siteName, refreshKey]);

  const openLogs = async (item: SiteProvisionHistoryItem) => {
    setLogOpen(true);
    setLogLoading(true);
    setLogError(null);
    setLogText('');
    setLogTitle(
      `${ACTION_LABELS[item.action || ''] || item.action || 'Provision'} — ${item.subject || item.id}`,
    );
    try {
      const resp = await fetchSiteProvisionActionLogs(siteName, item.id);
      setLogText(resp.logs || '(no log output captured)');
    } catch (e) {
      setLogError(e);
    } finally {
      setLogLoading(false);
    }
  };

  return (
    <div style={{ marginTop: '2rem' }}>
      <Title headingLevel="h3" size="md">
        Provision operation history
      </Title>
      <p style={{ marginTop: '0.35rem', fontSize: '0.875rem', maxWidth: '42rem' }}>
        Recent site host provision connector runs (create_vm, destroy, plan_vm, etc.). Ansible log
        tails are retained on the hub for troubleshooting after jobs expire.
      </p>
      <ErrorAlert error={historyError} />
      {loading ? (
        <p style={{ fontSize: '0.875rem', marginTop: '0.75rem' }}>Loading history…</p>
      ) : items.length === 0 ? (
        <p style={{ fontSize: '0.875rem', marginTop: '0.75rem', color: 'var(--pf-v6-global--Color--200)' }}>
          No provision operations yet.
        </p>
      ) : (
        <Table variant="compact" aria-label="Site provision history" style={{ marginTop: '0.75rem' }}>
          <Thead>
            <Tr>
              <Th>Finished</Th>
              <Th>Action</Th>
              <Th>Host</Th>
              <Th>Phase</Th>
              <Th>Message</Th>
              <Th>Logs</Th>
            </Tr>
          </Thead>
          <Tbody>
            {items.map((item) => (
              <Tr key={item.id}>
                <Td dataLabel="Finished">{formatWhen(item.endedAt || item.startedAt)}</Td>
                <Td dataLabel="Action">{ACTION_LABELS[item.action || ''] || item.action || '—'}</Td>
                <Td dataLabel="Host">
                  <code>{item.subject || '—'}</code>
                </Td>
                <Td dataLabel="Phase">
                  <Label color={phaseColor(item.phase || '')} isCompact>
                    {item.phase || '—'}
                  </Label>
                </Td>
                <Td dataLabel="Message">{item.message || '—'}</Td>
                <Td dataLabel="Logs">
                  {item.hasLogs || item.jobName ? (
                    <Button variant="link" isInline onClick={() => void openLogs(item)}>
                      View
                    </Button>
                  ) : (
                    '—'
                  )}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <Modal
        title={logTitle || 'Provision logs'}
        isOpen={logOpen}
        onClose={() => setLogOpen(false)}
        actions={[
          <Button key="close" variant="primary" onClick={() => setLogOpen(false)}>
            Close
          </Button>,
        ]}
        style={{ ['--pf-v6-c-modal-box--MaxWidth' as string]: '60rem' }}
      >
        <ErrorAlert error={logError} />
        {logLoading ? (
          <p>Loading logs…</p>
        ) : (
          <pre
            style={{
              maxHeight: '28rem',
              overflow: 'auto',
              fontSize: '0.75rem',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {logText}
          </pre>
        )}
      </Modal>
    </div>
  );
}
