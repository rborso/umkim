import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Checkbox,
  Label,
  Modal,
  TextArea,
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
import { vmLocationIsScheduleToken } from '../../utils/vmLocation';
import { ErrorAlert } from '../ErrorAlert';
import { SiteProvisionHistory } from './SiteProvisionHistory';

type Props = {
  siteName: string;
};

type DiscoverMode = 'discover' | 'discover_pxe';
type PrepareGpuMode = 'prepare_gpu_passthrough' | 'prepare_hypervisor_gpu';

function phaseColor(phase?: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Declared':
      return 'blue';
    case 'Provisioning':
    case 'Destroying':
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
  const navigate = useNavigate();
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
  const [destroyOpen, setDestroyOpen] = useState(false);
  const [destroyTarget, setDestroyTarget] = useState<SiteHostRecord | null>(null);
  const [destroyConfirm, setDestroyConfirm] = useState(false);
  const [destroying, setDestroying] = useState(false);
  const [installBmOpen, setInstallBmOpen] = useState(false);
  const [installBmTarget, setInstallBmTarget] = useState<SiteHostRecord | null>(null);
  const [installBmConfirm, setInstallBmConfirm] = useState(false);
  const [installingBm, setInstallingBm] = useState(false);
  const [preparingHv, setPreparingHv] = useState<string | null>(null);
  const [prepareGpuOpen, setPrepareGpuOpen] = useState(false);
  const [prepareGpuTarget, setPrepareGpuTarget] = useState<SiteHostRecord | null>(null);
  const [prepareGpuMode, setPrepareGpuMode] = useState<PrepareGpuMode>('prepare_gpu_passthrough');
  const [prepareGpuConfirm, setPrepareGpuConfirm] = useState(false);
  const [preparingGpu, setPreparingGpu] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [importBusy, setImportBusy] = useState(false);
  const [importItems, setImportItems] = useState<SiteInventoryImportChange[]>([]);
  const [importSelected, setImportSelected] = useState<Record<string, boolean>>({});
  const [importHostFilter, setImportHostFilter] = useState('');
  const [importEdits, setImportEdits] = useState<Record<string, string>>({});
  const [importEditPath, setImportEditPath] = useState<string | null>(null);
  const [importEditText, setImportEditText] = useState('');

  const load = useCallback(async (opts?: { background?: boolean; refreshHistory?: boolean }) => {
    const background = opts?.background ?? false;
    if (!background) {
      setLoading(true);
    }
    setError(null);
    try {
      const hostList = await listSiteHosts(siteName);
      setHosts(hostList);
      if (opts?.refreshHistory) {
        setHistoryRefreshKey(String(Date.now()));
      }
    } catch (e) {
      setError(e);
    } finally {
      if (!background) {
        setLoading(false);
      }
    }
  }, [siteName]);

  useEffect(() => {
    void load({ refreshHistory: true });
  }, [load]);

  useEffect(() => {
    const active = hosts.some(
      (h) =>
        h.status?.phase === 'Provisioning' ||
        h.status?.phase === 'Destroying' ||
        h.status?.cluster?.phase === 'Applying' ||
        h.status?.cluster?.phase === 'WaitingGroup' ||
        h.status?.cluster?.phase === 'Pending',
    );
    if (!active) return;
    const t = setInterval(() => void load({ background: true }), 5000);
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
      await load({ refreshHistory: true });
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
      await load({ refreshHistory: true });
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
          setImportEdits({});
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
      const content: Record<string, string> = {};
      for (const p of paths) {
        if (importEdits[p] !== undefined) content[p] = importEdits[p];
      }
      await applySiteInventoryImport(siteName, paths, true, Object.keys(content).length > 0 ? content : undefined);
      setImportOpen(false);
      await load({ refreshHistory: true });
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
      await load({ refreshHistory: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setApplyingCluster(null);
    }
  };

  const needsConfirm =
    provisionAction !== 'plan_vm' && provisionTarget?.status?.phase === 'Ready';

  const canEnrollNow = (host: SiteHostRecord) =>
    host.status?.phase === 'Ready' &&
    (host.ansible_host || host.status?.ansible_host) &&
    (!host.status?.cluster?.phase ||
      host.status.cluster.phase === 'Failed' ||
      host.status.cluster.phase === 'Pending');

  const activeClusterEnrollment = (host: SiteHostRecord) => {
    const phase = host.status?.cluster?.phase;
    return phase === 'Applying' || phase === 'WaitingGroup' || phase === 'Pending';
  };

  const canDestroyVM = (host: SiteHostRecord) =>
    host.kind === 'vm' &&
    (host.status?.phase === 'Ready' || host.status?.phase === 'Failed') &&
    !activeClusterEnrollment(host);

  const canPlanPlacement = (host: SiteHostRecord) =>
    host.kind === 'vm' && vmLocationIsScheduleToken(host.vm_location);

  const openDestroy = (host: SiteHostRecord) => {
    setDestroyTarget(host);
    setDestroyConfirm(false);
    setDestroyOpen(true);
  };

  const openInstallBm = (host: SiteHostRecord) => {
    setInstallBmTarget(host);
    setInstallBmConfirm(false);
    setInstallBmOpen(true);
  };

  const runPrepareHypervisor = async (host: SiteHostRecord) => {
    setPreparingHv(host.name);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, host.name, { action: 'prepare_hypervisor' });
      await load({ refreshHistory: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setPreparingHv(null);
    }
  };

  const openPrepareGpu = (host: SiteHostRecord, mode: PrepareGpuMode) => {
    setPrepareGpuTarget(host);
    setPrepareGpuMode(mode);
    setPrepareGpuConfirm(false);
    setPrepareGpuOpen(true);
  };

  const runPrepareGpu = async () => {
    if (!prepareGpuTarget) return;
    setPreparingGpu(true);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, prepareGpuTarget.name, {
        action: prepareGpuMode,
        confirm: true,
      });
      setPrepareGpuOpen(false);
      setPrepareGpuTarget(null);
      setPrepareGpuConfirm(false);
      await load({ refreshHistory: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setPreparingGpu(false);
    }
  };

  const runInstallBm = async () => {
    if (!installBmTarget) return;
    setInstallingBm(true);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, installBmTarget.name, {
        action: 'install_bm',
        confirm: true,
      });
      setInstallBmOpen(false);
      setInstallBmTarget(null);
      setInstallBmConfirm(false);
      await load({ refreshHistory: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setInstallingBm(false);
    }
  };

  const runDestroy = async () => {
    if (!destroyTarget) return;
    setDestroying(true);
    setActionError(null);
    try {
      await provisionSiteHost(siteName, destroyTarget.name, {
        action: 'destroy',
        confirm: true,
      });
      setDestroyOpen(false);
      setDestroyTarget(null);
      setDestroyConfirm(false);
      await load({ refreshHistory: true });
    } catch (e) {
      setActionError(e);
    } finally {
      setDestroying(false);
    }
  };

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
        Derived from <code>guests.yml</code> (VM guests), <code>bare_metals.yml</code>, <code>hypervisors.yml</code>,{' '}
        <code>host_vars/*.yml</code>, and root cluster groups. Use the Provisioner inventory tab to add hosts; sync
        before running actions.
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
                <Td>
                  {host.kind || '—'}
                  {host.kind === 'hypervisor' && (
                    <div style={{ fontSize: '0.75rem', color: 'var(--pf-v5-global--Color--200)' }}>
                      placement target
                    </div>
                  )}
                  {host.kind === 'bare_metal' && (
                    <div style={{ fontSize: '0.75rem', color: 'var(--pf-v5-global--Color--200)' }}>
                      cluster node
                    </div>
                  )}
                </Td>
                <Td>
                  <Label color={phaseColor(host.status?.phase)}>{host.status?.phase || '—'}</Label>
                  {host.kind === 'vm' && host.status?.phase === 'Declared' && (
                    <div style={{ fontSize: '0.8rem', marginTop: '0.25rem', color: 'var(--pf-v5-global--Color--200)' }}>
                      No VM running — use Provision to create
                    </div>
                  )}
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
                    {(host.kind === 'hypervisor' || host.kind === 'bare_metal') && (
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
                    )}
                    {host.kind === 'hypervisor' && (
                      <>
                        <Button
                          variant="link"
                          isInline
                          isLoading={preparingHv === host.name}
                          onClick={() => void runPrepareHypervisor(host)}
                        >
                          Prepare hypervisor
                        </Button>
                        <Button
                          variant="link"
                          isInline
                          onClick={() => openPrepareGpu(host, 'prepare_gpu_passthrough')}
                        >
                          Prepare GPU passthrough
                        </Button>
                        <Button
                          variant="link"
                          isInline
                          onClick={() => openPrepareGpu(host, 'prepare_hypervisor_gpu')}
                        >
                          Prepare libvirt + GPU
                        </Button>
                      </>
                    )}
                    {host.kind === 'bare_metal' && (
                      <Button variant="link" isInline onClick={() => openInstallBm(host)}>
                        Install bare metal
                      </Button>
                    )}
                    {host.kind === 'vm' && (
                      <>
                        <Button
                          variant="link"
                          isInline
                          onClick={() => openProvision(host, 'create_vm')}
                        >
                          Provision
                        </Button>
                        {canPlanPlacement(host) && (
                          <Button variant="link" isInline onClick={() => openProvision(host, 'plan_vm')}>
                            Plan placement
                          </Button>
                        )}
                        {canDestroyVM(host) && (
                          <Button variant="link" isInline isDanger onClick={() => openDestroy(host)}>
                            Destroy VM
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
                    {(host.status?.phase === 'Ready' || host.status?.phase === 'Declared') &&
                      host.kind !== 'hypervisor' &&
                      !(host.clusters?.length) &&
                      !host.clusterIntent &&
                      hostAnsibleAddr(host) && (
                        <Button
                          variant="link"
                          isInline
                          onClick={() =>
                            navigate(
                              `/clusters?site=${encodeURIComponent(siteName)}&create=1&node=${encodeURIComponent(host.name)}`,
                            )
                          }
                        >
                          Make new cluster
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
        title={
          prepareGpuMode === 'prepare_hypervisor_gpu'
            ? 'Prepare hypervisor and GPU passthrough'
            : 'Prepare GPU passthrough host'
        }
        isOpen={prepareGpuOpen}
        onClose={() => setPrepareGpuOpen(false)}
        actions={[
          <Button
            key="run"
            variant="danger"
            onClick={() => void runPrepareGpu()}
            isLoading={preparingGpu}
            isDisabled={!prepareGpuConfirm}
          >
            Run prepare
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setPrepareGpuOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Run <code>{prepareGpuMode}</code> for <code>{prepareGpuTarget?.name}</code>. This may reboot the
          host, remove the NVIDIA driver, and bind the GPU to VFIO. Ensure{' '}
          <code>gpu_passthrough_host</code> is set in <code>host_vars</code> and libvirt bridges exist
          {prepareGpuMode === 'prepare_gpu_passthrough' ? ' (run Prepare hypervisor first).' : '.'}
        </p>
        <Checkbox
          id="prepareGpuConfirm"
          isChecked={prepareGpuConfirm}
          onChange={(_e, c) => setPrepareGpuConfirm(c)}
          label="I confirm GPU passthrough host prepare (reboot and driver removal)"
        />
      </Modal>

      <Modal
        title="Install bare metal"
        isOpen={installBmOpen}
        onClose={() => setInstallBmOpen(false)}
        actions={[
          <Button
            key="run"
            variant="danger"
            onClick={() => void runInstallBm()}
            isLoading={installingBm}
            isDisabled={!installBmConfirm}
          >
            Install OS
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setInstallBmOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Run TempXE bare-metal install for <code>{installBmTarget?.name}</code>. This reinstalls the OS via PXE
          and is destructive.
        </p>
        <Checkbox
          id="installBmConfirm"
          isChecked={installBmConfirm}
          onChange={(_e, c) => setInstallBmConfirm(c)}
          label="I confirm bare-metal OS install (install_os / tempxe_confirm)"
        />
      </Modal>

      <Modal
        title="Destroy VM"
        isOpen={destroyOpen}
        onClose={() => setDestroyOpen(false)}
        actions={[
          <Button
            key="run"
            variant="danger"
            onClick={() => void runDestroy()}
            isLoading={destroying}
            isDisabled={!destroyConfirm}
          >
            Destroy VM
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDestroyOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Tear down the libvirt VM for <code>{destroyTarget?.name}</code> on the site jumphost. The guest
          remains in <code>guests.yml</code> and <code>host_vars</code> — use <strong>Provision</strong> later to
          recreate the VM.
        </p>
        <Checkbox
          id="destroyConfirm"
          isChecked={destroyConfirm}
          onChange={(_e, c) => setDestroyConfirm(c)}
          label="I confirm destroying this VM (inventory is kept)"
        />
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
              <Th>Edit</Th>
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
                <Td>
                  <Button
                    variant="link"
                    isInline
                    onClick={() => {
                      setImportEditPath(item.path);
                      setImportEditText(importEdits[item.path] ?? item.remote ?? item.local ?? '');
                    }}
                  >
                    {importEdits[item.path] !== undefined ? 'Edit ✓' : 'Edit'}
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      </Modal>

      <Modal
        title={`Edit ${importEditPath ?? ''}`}
        isOpen={importEditPath !== null}
        onClose={() => setImportEditPath(null)}
        variant="large"
        actions={[
          <Button
            key="save"
            variant="primary"
            onClick={() => {
              if (importEditPath !== null) {
                setImportEdits((prev) => ({ ...prev, [importEditPath]: importEditText }));
                setImportSelected((prev) => ({ ...prev, [importEditPath]: true }));
              }
              setImportEditPath(null);
            }}
          >
            Save edit
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setImportEditPath(null)}>
            Cancel
          </Button>,
        ]}
      >
        <p>Adjust the promoted content before applying — resolve CHANGE_ME placeholders here.</p>
        <TextArea
          id="import-edit"
          value={importEditText}
          onChange={(_e, v) => setImportEditText(v)}
          rows={18}
          style={{ fontFamily: 'monospace' }}
        />
      </Modal>
    </div>
  );
}
