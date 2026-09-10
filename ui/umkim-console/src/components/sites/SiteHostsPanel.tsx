import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Alert,
  Button,
  Checkbox,
  Label,
  Modal,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  applySiteHostCluster,
  applySiteInventoryImport,
  getSiteInventoryImportPreview,
  listSiteHosts,
  previewSiteInventoryImport,
  provisionSiteHost,
} from '../../api/client';
import type { SiteHostRecord, SiteInventoryImportChange } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { SiteProvisionHistory } from './SiteProvisionHistory';

type Props = {
  siteName: string;
};

type DiscoverMode = 'discover' | 'discover_pxe';

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
  const [discoverOpen, setDiscoverOpen] = useState(false);
  const [discoverTarget, setDiscoverTarget] = useState<SiteHostRecord | null>(null);
  const [discoverMode, setDiscoverMode] = useState<DiscoverMode>('discover');
  const [discoverConfirm, setDiscoverConfirm] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [importBusy, setImportBusy] = useState(false);
  const [importItems, setImportItems] = useState<SiteInventoryImportChange[]>([]);
  const [importSelected, setImportSelected] = useState<Record<string, boolean>>({});
  const [importHostFilter, setImportHostFilter] = useState('');

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

  const openDiscover = (host: SiteHostRecord, mode: DiscoverMode) => {
    setDiscoverTarget(host);
    setDiscoverMode(mode);
    setDiscoverConfirm(false);
    setDiscoverOpen(true);
  };

  const runDiscover = async () => {
    if (!discoverTarget) return;
    setDiscovering(true);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, discoverTarget.name, {
        action: discoverMode,
        confirm: discoverMode === 'discover_pxe' ? discoverConfirm : false,
      });
      setDiscoverOpen(false);
      setDiscoverTarget(null);
      setDiscoverConfirm(false);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setDiscovering(false);
    }
  };

  const runImportPreviewForHost = async (hostName: string) => {
    setImportBusy(true);
    setActionError(null);
    setImportHostFilter(hostName);
    try {
      await previewSiteInventoryImport(siteName);
      for (let i = 0; i < 30; i++) {
        await new Promise((r) => setTimeout(r, 2000));
        const items = await getSiteInventoryImportPreview(siteName);
        if (items.length > 0) {
          const filtered = items.filter(
            (item) =>
              item.path === 'hypervisors.yml' || item.path === `host_vars/${hostName}.yml`,
          );
          const useItems = filtered.length > 0 ? filtered : items;
          setImportItems(useItems);
          const sel: Record<string, boolean> = {};
          for (const item of useItems) {
            if (item.status !== 'unchanged') {
              sel[item.path] = true;
            }
          }
          setImportSelected(sel);
          setImportOpen(true);
          return;
        }
      }
      throw new Error('Timed out waiting for import preview');
    } catch (e) {
      setActionError(e);
    } finally {
      setImportBusy(false);
    }
  };

  const applyImport = async () => {
    setImportBusy(true);
    setActionError(null);
    try {
      const paths = Object.entries(importSelected)
        .filter(([, v]) => v)
        .map(([p]) => p);
      await applySiteInventoryImport(siteName, paths, true);
      setImportOpen(false);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setImportBusy(false);
    }
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

  const hostAnsibleAddr = (host: SiteHostRecord) =>
    (host.ansible_host || host.status?.ansible_host || '').trim();

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
        Derived from <code>guests.yml</code>, <code>hypervisors.yml</code>, <code>host_vars/*.yml</code>, and{' '}
        root cluster groups. Add discovery stubs in the Provisioner inventory tab; run Discover and import preview
        to promote hardware drafts.
      </p>
      {loading ? (
        <p>Loading hosts…</p>
      ) : hosts.length === 0 ? (
        <p>No hosts in inventory yet. Add guests or discovery stubs under Provisioner inventory.</p>
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
                  <code>{hostAnsibleAddr(host) || '—'}</code>
                </Td>
                <Td>{host.vm_location || '—'}</Td>
                <Td>
                  <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                    {host.kind === 'bare_metal' ? (
                      <>
                        <Button
                          variant="link"
                          isInline
                          isDisabled={!hostAnsibleAddr(host)}
                          onClick={() => openDiscover(host, 'discover')}
                        >
                          Discover (SSH)
                        </Button>
                        <Button variant="link" isInline onClick={() => openDiscover(host, 'discover_pxe')}>
                          Discover (PXE)
                        </Button>
                      </>
                    ) : (
                      <>
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
                      </>
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

      <SiteProvisionHistory
        siteName={siteName}
        refreshKey={historyRefreshKey}
        onReviewDiscovery={(hostName) => void runImportPreviewForHost(hostName)}
      />

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

      <Modal
        title={discoverMode === 'discover_pxe' ? 'Discover host details (PXE)' : 'Discover host details (SSH)'}
        isOpen={discoverOpen}
        onClose={() => setDiscoverOpen(false)}
        actions={[
          <Button
            key="run"
            variant="primary"
            onClick={() => void runDiscover()}
            isLoading={discovering}
            isDisabled={discoverMode === 'discover_pxe' && !discoverConfirm}
          >
            Run discovery
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDiscoverOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Run <code>{discoverMode}</code> for <code>{discoverTarget?.name}</code>. Inventory is synced to the
          jumphost before YAIDAP runs. After success, use Review discovery in history to import promoted drafts.
        </p>
        {discoverMode === 'discover' && discoverTarget && (
          <p style={{ fontSize: '0.85rem' }}>
            Requires SSH reachability at <code>{hostAnsibleAddr(discoverTarget) || 'ansible_host'}</code>.
          </p>
        )}
        {discoverMode === 'discover_pxe' && (
          <Checkbox
            id="discoverConfirm"
            isChecked={discoverConfirm}
            onChange={(_e, c) => setDiscoverConfirm(c)}
            label="I confirm arming TempXE for PXE discovery (tempxe_confirm)"
          />
        )}
      </Modal>

      <Modal
        title={`Import preview${importHostFilter ? ` — ${importHostFilter}` : ''}`}
        isOpen={importOpen}
        onClose={() => setImportOpen(false)}
        variant="large"
        actions={[
          <Button
            key="apply"
            variant="primary"
            isLoading={importBusy}
            onClick={() => void applyImport()}
          >
            Apply selected
          </Button>,
          <Button key="close" variant="link" onClick={() => setImportOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>Promoted discovery drafts merge into canonical hub inventory paths.</p>
        {importItems.some((item) => (item.warnings?.length ?? 0) > 0) ? (
          <Alert variant="warning" title="CHANGE_ME placeholders detected" isInline style={{ marginBottom: '1rem' }}>
            Resolve placeholder fields before sync or the next YAIDAP/ansible run.
          </Alert>
        ) : null}
        <Table variant="compact">
          <Thead>
            <Tr>
              <Th>Apply</Th>
              <Th>Path</Th>
              <Th>Status</Th>
              <Th>Warnings</Th>
            </Tr>
          </Thead>
          <Tbody>
            {importItems.map((item) => (
              <Tr key={item.path}>
                <Td>
                  <Checkbox
                    id={`import-${item.path}`}
                    isChecked={importSelected[item.path] ?? false}
                    isDisabled={item.status === 'unchanged'}
                    onChange={(_e, c) =>
                      setImportSelected((prev) => ({ ...prev, [item.path]: c }))
                    }
                    aria-label={`Apply ${item.path}`}
                  />
                </Td>
                <Td>
                  <code>{item.path}</code>
                </Td>
                <Td>{item.status}</Td>
                <Td>
                  {(item.warnings ?? []).length > 0 ? (
                    <ul style={{ margin: 0, paddingLeft: '1.1rem', fontSize: '0.85rem' }}>
                      {(item.warnings ?? []).map((w) => (
                        <li key={w}>{w}</li>
                      ))}
                    </ul>
                  ) : (
                    '—'
                  )}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      </Modal>
    </div>
  );
}
