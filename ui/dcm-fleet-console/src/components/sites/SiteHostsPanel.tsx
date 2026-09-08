import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  Checkbox,
  Label,
  Modal,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  applySiteHostCluster,
  listSiteHosts,
  provisionSiteHost,
} from '../../api/client';
import type { SiteHostRecord } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { SiteProvisionHistory } from './SiteProvisionHistory';

type Props = {
  siteName: string;
};

function phaseColor(phase?: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Declared':
      return 'blue';
    case 'Provisioning':
    case 'Applying':
    case 'WaitingGroup':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

export function SiteHostsPanel({ siteName }: Props) {
  const [hosts, setHosts] = useState<SiteHostRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [provisionOpen, setProvisionOpen] = useState(false);
  const [provisionTarget, setProvisionTarget] = useState<SiteHostRecord | null>(null);
  const [provisionAction, setProvisionAction] = useState<'create_vm' | 'plan_vm'>('create_vm');
  const [provisionConfirm, setProvisionConfirm] = useState(false);
  const [provisioning, setProvisioning] = useState(false);
  const [applyingCluster, setApplyingCluster] = useState<string | null>(null);
  const [historyRefreshKey, setHistoryRefreshKey] = useState('0');

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const hostList = await listSiteHosts(siteName);
      setHosts(hostList);
      setHistoryRefreshKey(String(Date.now()));
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [siteName]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const active = hosts.some(
      (h) =>
        h.status?.phase === 'Provisioning' ||
        h.status?.cluster?.phase === 'Applying' ||
        h.status?.cluster?.phase === 'WaitingGroup' ||
        h.status?.cluster?.phase === 'Pending',
    );
    if (!active) return;
    const t = setInterval(() => void load(), 5000);
    return () => clearInterval(t);
  }, [hosts, load]);

  const runProvision = async () => {
    if (!provisionTarget) return;
    setProvisioning(true);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, provisionTarget.name, {
        action: provisionAction,
        confirm: provisionConfirm,
      });
      setProvisionOpen(false);
      setProvisionTarget(null);
      setProvisionConfirm(false);
      setProvisionAction('create_vm');
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setProvisioning(false);
    }
  };

  const openProvision = (host: SiteHostRecord, action: 'create_vm' | 'plan_vm' = 'create_vm') => {
    setProvisionTarget(host);
    setProvisionAction(action);
    setProvisionConfirm(false);
    setProvisionOpen(true);
  };

  const runClusterApply = async (host: SiteHostRecord) => {
    setApplyingCluster(host.name);
    setActionError(null);
    try {
      await applySiteHostCluster(siteName, host.name);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setApplyingCluster(null);
    }
  };

  const needsConfirm =
    provisionAction !== 'plan_vm' &&
    (provisionTarget?.kind === 'bare_metal' || provisionTarget?.status?.phase === 'Ready');

  const canEnrollNow = (host: SiteHostRecord) =>
    host.status?.phase === 'Ready' &&
    (host.ansible_host || host.status?.ansible_host) &&
    (!host.status?.cluster?.phase ||
      host.status.cluster.phase === 'Failed' ||
      host.status.cluster.phase === 'Pending');

  return (
    <div style={{ marginTop: '1rem' }}>
      <ErrorAlert error={error} />
      <ErrorAlert error={actionError} />
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.75rem' }}>
        <Title headingLevel="h3" style={{ margin: 0 }}>
          Site hosts (from inventory)
        </Title>
      </div>
      <p style={{ fontSize: '0.85rem', marginBottom: '0.75rem' }}>
        Derived from <code>guests.yml</code>, <code>host_vars/*.yml</code>, and{' '}
        root cluster groups (<code>lab-ha01.yml</code>) and matching{' '}
        <code>group_vars/lab-ha01.yml</code>. Add or edit guests in the Provisioner inventory tab; run
        provision and cluster apply actions here.
      </p>
      {loading ? (
        <p>Loading hosts…</p>
      ) : hosts.length === 0 ? (
        <p>No hosts in inventory yet. Add guests under Provisioner inventory.</p>
      ) : (
        <Table aria-label="Site hosts" variant="compact">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Kind</Th>
              <Th>Phase</Th>
              <Th>Cluster</Th>
              <Th>ansible_host</Th>
              <Th>Location</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {hosts.map((host) => (
              <Tr key={host.name}>
                <Td>
                  <code>{host.name}</code>
                </Td>
                <Td>{host.kind || '—'}</Td>
                <Td>
                  <Label color={phaseColor(host.status?.phase)}>{host.status?.phase || '—'}</Label>
                </Td>
                <Td>
                  {host.clusters?.length ? (
                    <code>{host.clusters.join(', ')}</code>
                  ) : host.status?.cluster?.clusterName ? (
                    <div>
                      <code>{host.status.cluster.clusterName}</code>{' '}
                      <Label color={phaseColor(host.status.cluster.phase)}>
                        {host.status.cluster.phase || '—'}
                      </Label>
                      {host.status?.cluster?.phase === 'Ready' && host.status.cluster.clusterName && (
                        <>
                          {' '}
                          <Link to={`/clusters/${encodeURIComponent(host.status.cluster.clusterName)}`}>View</Link>
                        </>
                      )}
                    </div>
                  ) : (
                    '—'
                  )}
                </Td>
                <Td>
                  <code>{host.ansible_host || host.status?.ansible_host || '—'}</code>
                </Td>
                <Td>{host.vm_location || '—'}</Td>
                <Td>
                  <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                    <Button
                      variant="link"
                      isInline
                      onClick={() => openProvision(host, 'create_vm')}
                    >
                      Provision
                    </Button>
                    {host.kind === 'vm' && (
                      <Button variant="link" isInline onClick={() => openProvision(host, 'plan_vm')}>
                        Plan placement
                      </Button>
                    )}
                    {canEnrollNow(host) && (host.clusters?.length || host.clusterIntent) && (
                      <Button
                        variant="link"
                        isInline
                        isLoading={applyingCluster === host.name}
                        onClick={() => void runClusterApply(host)}
                      >
                        Apply cluster
                      </Button>
                    )}
                  </div>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <SiteProvisionHistory siteName={siteName} refreshKey={historyRefreshKey} />

      <Modal
        title={provisionAction === 'plan_vm' ? 'Plan VM placement' : 'Provision host'}
        isOpen={provisionOpen}
        onClose={() => setProvisionOpen(false)}
        actions={[
          <Button
            key="run"
            variant="primary"
            onClick={() => void runProvision()}
            isLoading={provisioning}
            isDisabled={needsConfirm && !provisionConfirm}
          >
            {provisionAction === 'plan_vm' ? 'Run placement plan' : 'Run provision'}
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setProvisionOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Start <code>{provisionAction}</code> for <code>{provisionTarget?.name}</code>.
          {provisionTarget?.kind === 'vm' && provisionAction === 'create_vm' && (
            <>
              {' '}
              VM location <code>{provisionTarget.vm_location || 'least-utilized'}</code> is synced to YAIDAP
              inventory before create.
            </>
          )}
          {provisionAction === 'plan_vm' && (
            <>
              {' '}
              Dry-run only: shows memory-based least-utilized placement without creating a VM or
              persisting inventory.
            </>
          )}
        </p>
        {needsConfirm && (
          <Checkbox
            id="provisionConfirm"
            isChecked={provisionConfirm}
            onChange={(_e, c) => setProvisionConfirm(c)}
            label="I confirm this destructive or bare-metal install action"
          />
        )}
      </Modal>
    </div>
  );
}
