import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  FormSelect,
  FormSelectOption,
  Label,
  Modal,
  TextInput,
  Title,
} from '@patternfly/react-core';
import {
  addFleetClusterWorker,
  configureFleetClusterStorage,
  enableFleetClusterGpu,
  enableFleetClusterKubevirt,
  getFleetClusterGpuStatus,
  prepFleetClusterGpuHost,
  fetchFleetClusterNodeOperationLogs,
  listFleetClusterNodeOperations,
  removeFleetClusterNode,
  replaceFleetClusterControlPlane,
  validateFleetClusterNodeOperation,
} from '../../api/client';
import type {
  ClusterNodeOperation,
  ClusterNodeOperationHistoryItem,
  FleetCluster,
  FleetNode,
  NodeOperationValidateResult,
} from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';
import { getFieldHelp } from '../../help/fieldHelp';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';

type Props = {
  cluster: FleetCluster;
  nodes: FleetNode[];
  onUpdated: () => void | Promise<void>;
};

const OP_LABELS: Record<string, string> = {
  addWorker: 'Add worker',
  removeWorker: 'Remove worker',
  replaceControlPlane: 'Replace control plane',
  configureStorage: 'Configure storage',
  configureKubevirt: 'Configure KubeVirt',
  configureGpuHost: 'Prepare GPU host',
  configureGpuEnable: 'Enable GPU runtime',
};

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Succeeded':
    case 'Applied':
      return 'green';
    case 'Pending':
      return 'blue';
    case 'Running':
    case 'Applying':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

function checkColor(status: string): 'green' | 'orange' | 'red' | 'grey' {
  switch (status) {
    case 'pass':
      return 'green';
    case 'warn':
      return 'orange';
    case 'fail':
      return 'red';
    default:
      return 'grey';
  }
}

function isControlNode(n: FleetNode): boolean {
  const roles = n.roles || [];
  return roles.some((r) => r === 'control-plane' || r === 'master');
}

function isWorkerNode(n: FleetNode): boolean {
  return (n.roles || []).includes('worker');
}

function formatWhen(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function actorLabel(kind?: string, name?: string): string {
  if (!name) return '—';
  if (kind) return `${kind}/${name}`;
  return name;
}

function nodeOpBusy(op?: ClusterNodeOperation): boolean {
  return op?.phase === 'Pending' || op?.phase === 'Running';
}

function storageOpBusy(phase?: string): boolean {
  return phase === 'Pending' || phase === 'Applying';
}

function isSingleNodeCluster(cluster: FleetCluster): boolean {
  const topo = cluster.labels?.['fleet.topology/topology'];
  if (topo === 'single_node') return true;
  if (topo === 'ha_3_node') return false;
  if (cluster.labels?.['fleet.topology/ha'] === 'true') return false;
  if (cluster.capabilities?.ha_topology) return false;
  return true;
}

function NodeOperationHistory({
  clusterName,
  refreshKey,
}: {
  clusterName: string;
  refreshKey: string;
}) {
  const [items, setItems] = useState<ClusterNodeOperationHistoryItem[]>([]);
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
        const list = await listFleetClusterNodeOperations(clusterName);
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
  }, [clusterName, refreshKey]);

  const openLogs = async (item: ClusterNodeOperationHistoryItem) => {
    setLogOpen(true);
    setLogLoading(true);
    setLogError(null);
    setLogText('');
    setLogTitle(`${OP_LABELS[item.type || ''] || item.type || 'Operation'} — ${item.node || item.id}`);
    try {
      const resp = await fetchFleetClusterNodeOperationLogs(clusterName, item.id);
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
        Operation history
      </Title>
      <p style={{ marginTop: '0.35rem', fontSize: '0.875rem', maxWidth: '42rem' }}>
        Recent completed cluster operations (newest first), including node lifecycle and storage
        configure. Ansible log tails are retained on the hub for troubleshooting after jobs expire.
      </p>
      <ErrorAlert error={historyError} />
      {loading ? (
        <p style={{ fontSize: '0.875rem', marginTop: '0.75rem' }}>Loading history…</p>
      ) : items.length === 0 ? (
        <p style={{ fontSize: '0.875rem', marginTop: '0.75rem', color: 'var(--pf-v6-global--Color--200)' }}>
          No completed operations yet.
        </p>
      ) : (
        <Table variant="compact" aria-label="Node operation history" style={{ marginTop: '0.75rem' }}>
          <Thead>
            <Tr>
              <Th>Finished</Th>
              <Th>Operation</Th>
              <Th>Node</Th>
              <Th>Phase</Th>
              <Th>Actor</Th>
              <Th>Message</Th>
              <Th>Logs</Th>
            </Tr>
          </Thead>
          <Tbody>
            {items.map((item) => (
              <Tr key={item.id}>
                <Td dataLabel="Finished">{formatWhen(item.finishedAt || item.startedAt)}</Td>
                <Td dataLabel="Operation">{OP_LABELS[item.type || ''] || item.type || '—'}</Td>
                <Td dataLabel="Node">
                  <code>{item.node || '—'}</code>
                </Td>
                <Td dataLabel="Phase">
                  <Label color={phaseColor(item.phase || '')} isCompact>
                    {item.phase || '—'}
                  </Label>
                </Td>
                <Td dataLabel="Actor">{actorLabel(item.actorKind, item.actorName)}</Td>
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
        title={logTitle || 'Operation logs'}
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
              margin: 0,
              maxHeight: '28rem',
              overflow: 'auto',
              padding: '0.75rem',
              background: 'var(--pf-v6-global--BackgroundColor--200)',
              fontSize: '0.75rem',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {logText || '(empty)'}
          </pre>
        )}
      </Modal>
    </div>
  );
}

function ValidationChecks({
  title,
  result,
}: {
  title: string;
  result: NodeOperationValidateResult | null;
}) {
  if (!result) return null;
  return (
    <Alert
      variant={result.ok ? 'success' : 'warning'}
      title={title}
      isInline
      style={{ marginTop: '0.75rem', maxWidth: '42rem' }}
    >
      <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
        {result.checks.map((c) => (
          <li key={c.id} style={{ marginBottom: '0.25rem' }}>
            <Label color={checkColor(c.status)} isCompact>
              {c.status}
            </Label>{' '}
            <code>{c.id}</code>
            {c.message ? <span> — {c.message}</span> : null}
          </li>
        ))}
      </ul>
    </Alert>
  );
}

export function ClusterOperationsPanel({ cluster, nodes, onUpdated }: Props) {
  const clusterName = cluster.name;
  const phase = cluster.provisioning_phase;
  const clusterReady = phase === 'Ready' || !!cluster.available;
  const nodeOp = cluster.nodeOperation;
  const storagePhase = cluster.storageStatus?.phase || '';
  const storageBusy = storageOpBusy(storagePhase);
  const kubevirtPhaseEarly = cluster.providers?.kubevirt?.phase || '';
  const kubevirtBusy = kubevirtPhaseEarly === 'Installing' || kubevirtPhaseEarly === 'Pending';
  const [gpuHostPhase, setGpuHostPhase] = useState('');
  const [gpuRuntimePhase, setGpuRuntimePhase] = useState('');
  const gpuBusy =
    gpuHostPhase === 'Applying' || gpuRuntimePhase === 'Applying';
  const busy = nodeOpBusy(nodeOp) || storageBusy || kubevirtBusy || gpuBusy;
  const singleNode = isSingleNodeCluster(cluster);
  const nfsLocalApplied = cluster.storage?.backend === 'nfs_local' && storagePhase === 'Applied';

  const workerNodes = useMemo(() => nodes.filter(isWorkerNode), [nodes]);
  const controlNodes = useMemo(() => nodes.filter(isControlNode), [nodes]);

  const [addAddress, setAddAddress] = useState('');
  const [addHostname, setAddHostname] = useState('');
  const [removeNode, setRemoveNode] = useState('');
  const [removeForce, setRemoveForce] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);
  const [replaceOldNode, setReplaceOldNode] = useState('');
  const [replaceAddress, setReplaceAddress] = useState('');
  const [replaceHostname, setReplaceHostname] = useState('');
  const [replaceForce, setReplaceForce] = useState(false);
  const [nfsDir, setNfsDir] = useState('/srv/fleet-nfs');
  const [acting, setActing] = useState(false);
  const [validating, setValidating] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [preValidate, setPreValidate] = useState<NodeOperationValidateResult | null>(null);
  const [postValidate, setPostValidate] = useState<NodeOperationValidateResult | null>(null);
  const lastPostValidated = useRef('');
  const [historyRefreshKey, setHistoryRefreshKey] = useState('');
  const lastHistoryPhase = useRef('');
  const [lanBridge, setLanBridge] = useState(false);
  const [bridgeName, setBridgeName] = useState('n0p0_mgmt_nw');

  useEffect(() => {
    setNfsDir(cluster.storage?.nfs?.exportBase || '/srv/fleet-nfs');
  }, [clusterName, cluster.storage?.nfs?.exportBase]);

  useEffect(() => {
    if (!busy) return;
    const interval = setInterval(() => void onUpdated(), 8000);
    return () => clearInterval(interval);
  }, [busy, onUpdated]);

  useEffect(() => {
    void (async () => {
      try {
        const st = await getFleetClusterGpuStatus(clusterName);
        setGpuHostPhase(st.hostPhase || '');
        setGpuRuntimePhase(st.runtimePhase || '');
      } catch {
        setGpuHostPhase('');
        setGpuRuntimePhase('');
      }
    })();
  }, [clusterName, busy, historyRefreshKey, cluster.provisioning_phase]);

  useEffect(() => {
    if (workerNodes.length > 0 && !workerNodes.some((n) => n.name === removeNode)) {
      setRemoveNode(workerNodes[0].name);
    }
  }, [workerNodes, removeNode]);

  useEffect(() => {
    if (controlNodes.length > 0 && !controlNodes.some((n) => n.name === replaceOldNode)) {
      setReplaceOldNode(controlNodes[0].name);
    }
  }, [controlNodes, replaceOldNode]);

  useEffect(() => {
    const phaseKey = `${nodeOp?.id || ''}:${nodeOp?.phase || ''}:${storagePhase}`;
    if (
      ((nodeOp?.phase === 'Succeeded' || nodeOp?.phase === 'Failed') ||
        storagePhase === 'Applied' ||
        storagePhase === 'Failed') &&
      lastHistoryPhase.current !== phaseKey
    ) {
      lastHistoryPhase.current = phaseKey;
      setHistoryRefreshKey(phaseKey);
    }
  }, [nodeOp?.id, nodeOp?.phase, storagePhase]);

  useEffect(() => {
    if (nodeOp?.phase !== 'Succeeded' || !nodeOp.type) return;
    const key = `${nodeOp.type}:${nodeOp.node}:${nodeOp.message}`;
    if (lastPostValidated.current === key) return;
    lastPostValidated.current = key;
    void (async () => {
      try {
        const body = buildValidateBody(nodeOp, {
          addAddress,
          addHostname,
          removeNode,
          removeForce,
          replaceOldNode,
          replaceAddress,
          replaceHostname,
          replaceForce,
        });
        const result = await validateFleetClusterNodeOperation(clusterName, body, 'post');
        setPostValidate(result);
      } catch {
        setPostValidate(null);
      }
    })();
  }, [
    nodeOp,
    clusterName,
    addAddress,
    addHostname,
    removeNode,
    removeForce,
    replaceOldNode,
    replaceAddress,
    replaceHostname,
    replaceForce,
  ]);

  if (!clusterReady || cluster.read_only) {
    return null;
  }

  const canMutate = !busy && !acting && !validating;

  const runPreValidate = async (body: Record<string, unknown>) => {
    setValidating(true);
    setActionError(null);
    setPreValidate(null);
    try {
      const result = await validateFleetClusterNodeOperation(clusterName, body, 'pre');
      setPreValidate(result);
      return result.ok;
    } catch (e) {
      setActionError(e);
      return false;
    } finally {
      setValidating(false);
    }
  };

  const addWorker = async () => {
    if (!addAddress.trim()) return;
    const body = {
      type: 'addWorker',
      address: addAddress.trim(),
      hostname: addHostname.trim() || undefined,
      role: 'worker',
    };
    if (!(await runPreValidate(body))) return;
    setActing(true);
    setActionError(null);
    setPostValidate(null);
    lastPostValidated.current = '';
    try {
      await addFleetClusterWorker(clusterName, {
        address: addAddress.trim(),
        hostname: addHostname.trim() || undefined,
      });
      setAddAddress('');
      setAddHostname('');
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  const confirmRemoveWorker = async () => {
    if (!removeNode) return;
    const body = {
      type: 'removeWorker',
      node: removeNode,
      force: removeForce || undefined,
    };
    if (!(await runPreValidate(body))) return;
    setActing(true);
    setActionError(null);
    setPostValidate(null);
    lastPostValidated.current = '';
    try {
      await removeFleetClusterNode(clusterName, removeNode, removeForce || undefined);
      setRemoveOpen(false);
      setRemoveForce(false);
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  const replaceControl = async () => {
    if (!replaceOldNode || !replaceAddress.trim()) return;
    const body = {
      type: 'replaceControlPlane',
      oldNode: replaceOldNode,
      force: replaceForce || undefined,
      newNode: {
        address: replaceAddress.trim(),
        hostname: replaceHostname.trim() || undefined,
      },
    };
    if (!(await runPreValidate(body))) return;
    setActing(true);
    setActionError(null);
    setPostValidate(null);
    lastPostValidated.current = '';
    try {
      await replaceFleetClusterControlPlane(clusterName, {
        oldNode: replaceOldNode,
        newNode: {
          address: replaceAddress.trim(),
          hostname: replaceHostname.trim() || undefined,
        },
        force: replaceForce || undefined,
      });
      setReplaceAddress('');
      setReplaceHostname('');
      setReplaceForce(false);
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  const enableLocalNfs = async () => {
    const dir = nfsDir.trim() || '/srv/fleet-nfs';
    if (!dir.startsWith('/')) {
      setActionError(new Error('NFS share directory must be an absolute path'));
      return;
    }
    setActing(true);
    setActionError(null);
    try {
      await configureFleetClusterStorage(clusterName, {
        backend: 'nfs_local',
        rwo: true,
        rwx: true,
        force: nfsLocalApplied || storagePhase === 'Failed' || undefined,
        nfs: {
          exportBase: dir.replace(/\/$/, '') || '/srv/fleet-nfs',
          clusterPath: cluster.storage?.nfs?.clusterPath || clusterName,
          storageClass: cluster.storage?.nfs?.storageClass || 'nfs-client',
        },
      });
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  const kubevirtPhase = cluster.providers?.kubevirt?.phase || '';
  const kubevirtReady = kubevirtPhase === 'Ready' || !!cluster.capabilities?.kubevirt;
  const multusL2 = !!cluster.capabilities?.['network.multus_l2'];

  const prepGpuHost = async () => {
    setActing(true);
    setActionError(null);
    try {
      await prepFleetClusterGpuHost(clusterName, { force: clusterReady });
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  const enableGpuRuntime = async () => {
    setActing(true);
    setActionError(null);
    try {
      await enableFleetClusterGpu(clusterName);
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  const enableKubevirt = async (opts?: { lanBridge?: boolean }) => {
    setActing(true);
    setActionError(null);
    try {
      const withLan = opts?.lanBridge ?? lanBridge;
      const lanUpgrade = withLan && !multusL2;
      await enableFleetClusterKubevirt(clusterName, {
        lanBridge: withLan || undefined,
        bridgeName: withLan ? bridgeName.trim() || 'n0p0_mgmt_nw' : undefined,
        keepIp: withLan ? true : undefined,
        // Re-apply needs force; first-time LAN Multus on Ready KubeVirt does not.
        force: (!lanUpgrade && (kubevirtReady || kubevirtPhase === 'Failed')) || undefined,
      });
      await onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setActing(false);
    }
  };

  return (
    <div style={{ marginTop: '0.5rem', marginBottom: '0.5rem' }}>
      <Title headingLevel="h2" size="lg">
        Cluster operations
      </Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '42rem' }}>
        Node lifecycle, single-node local RWX storage, and optional KubeVirt platform. Each action
        runs an Ansible job on the hub; only one cluster operation runs at a time.
      </p>
      {nodeOp?.phase && (
        <div style={{ marginTop: '0.75rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
          <Label color={phaseColor(nodeOp.phase)}>{nodeOp.phase}</Label>
          {nodeOp.type ? (
            <span style={{ fontSize: '0.875rem' }}>{OP_LABELS[nodeOp.type] || nodeOp.type}</span>
          ) : null}
          {nodeOp.node ? (
            <span style={{ fontSize: '0.875rem' }}>
              node: <code>{nodeOp.node}</code>
            </span>
          ) : null}
          {nodeOp.force ? (
            <Label color="orange" isCompact>
              force
            </Label>
          ) : null}
          {nodeOp.message ? (
            <span style={{ fontSize: '0.875rem' }}>{nodeOp.message}</span>
          ) : null}
        </div>
      )}
      {storagePhase && (storageBusy || storagePhase === 'Failed' || nfsLocalApplied) && (
        <div style={{ marginTop: '0.75rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
          <Label color={phaseColor(storagePhase)}>{storagePhase}</Label>
          <span style={{ fontSize: '0.875rem' }}>{OP_LABELS.configureStorage}</span>
          {cluster.storage?.backend ? (
            <span style={{ fontSize: '0.875rem' }}>
              backend: <code>{cluster.storage.backend}</code>
            </span>
          ) : null}
          {cluster.storageStatus?.message ? (
            <span style={{ fontSize: '0.875rem' }}>{cluster.storageStatus.message}</span>
          ) : null}
        </div>
      )}
      {kubevirtPhase && (kubevirtBusy || kubevirtPhase === 'Failed' || kubevirtReady) && (
        <div style={{ marginTop: '0.75rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
          <Label color={phaseColor(kubevirtPhase === 'Ready' ? 'Succeeded' : kubevirtPhase)}>
            {kubevirtPhase}
          </Label>
          <span style={{ fontSize: '0.875rem' }}>{OP_LABELS.configureKubevirt}</span>
          {cluster.providers?.kubevirt?.message ? (
            <span style={{ fontSize: '0.875rem' }}>{cluster.providers.kubevirt.message}</span>
          ) : null}
        </div>
      )}
      {(gpuHostPhase || gpuRuntimePhase) &&
        (gpuBusy || gpuHostPhase === 'Failed' || gpuRuntimePhase === 'Failed' || gpuHostPhase === 'Ready') && (
        <div style={{ marginTop: '0.75rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
          {gpuHostPhase ? (
            <Label color={phaseColor(gpuHostPhase === 'Ready' ? 'Succeeded' : gpuHostPhase)}>
              host: {gpuHostPhase}
            </Label>
          ) : null}
          {gpuRuntimePhase ? (
            <Label color={phaseColor(gpuRuntimePhase === 'Ready' ? 'Succeeded' : gpuRuntimePhase)}>
              runtime: {gpuRuntimePhase}
            </Label>
          ) : null}
        </div>
      )}
      {busy && (
        <Alert
          variant="info"
          title="Cluster operation in progress"
          isInline
          style={{ marginTop: '0.75rem', maxWidth: '42rem' }}
        >
          The hub is running an Ansible job for this operation. This page refreshes automatically
          until it completes.
        </Alert>
      )}
      <ValidationChecks title="Pre-validation" result={preValidate} />
      <ValidationChecks title="Post-validation" result={postValidate} />
      <ErrorAlert error={actionError} />

      {singleNode && (
        <Form style={{ maxWidth: '36rem', marginTop: '1rem' }}>
          <Title headingLevel="h3" size="md">
            Local RWX storage
          </Title>
          <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
            Installs nfs-server on this node and StorageClass <code>nfs-client</code>. Same as{' '}
            <code>umkim cluster storage enable-local</code>.
          </p>
          <LabeledField
            label="NFS share directory"
            fieldId="opNfsDir"
            help={getFieldHelp('clusterStorage.nfsExportBase')}
          >
            <TextInput
              id="opNfsDir"
              value={nfsDir}
              onChange={(_e, v) => setNfsDir(v)}
              placeholder="/srv/fleet-nfs"
              isDisabled={!canMutate}
            />
          </LabeledField>
          <Button
            variant="primary"
            isLoading={acting}
            isDisabled={!canMutate || !nfsDir.trim().startsWith('/')}
            onClick={() => void enableLocalNfs()}
            data-testid="op-enable-local-nfs"
          >
            {storagePhase === 'Failed'
              ? 'Retry local RWX'
              : nfsLocalApplied
                ? 'Re-apply local RWX'
                : 'Enable local RWX'}
          </Button>
        </Form>
      )}

      <Form style={{ maxWidth: '36rem', marginTop: '1rem' }}>
        <Title headingLevel="h3" size="md">
          GPU (NVIDIA)
        </Title>
        <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
          Host driver prep then container runtime + device plugin. Same as{' '}
          <code>umkim cluster gpu host-prep</code> and <code>umkim cluster gpu enable</code>. On Ready
          clusters, host prep uses <code>force</code> automatically.
        </p>
        <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginTop: '0.75rem' }}>
          <Button
            variant="secondary"
            isLoading={acting}
            isDisabled={!canMutate || gpuHostPhase === 'Applying'}
            onClick={() => void prepGpuHost()}
            data-testid="op-gpu-host-prep"
          >
            {gpuHostPhase === 'Failed' ? 'Retry GPU host prep' : 'Prepare GPU host'}
          </Button>
          <Button
            variant="primary"
            isLoading={acting}
            isDisabled={!canMutate || gpuRuntimePhase === 'Applying' || gpuHostPhase !== 'Ready'}
            onClick={() => void enableGpuRuntime()}
            data-testid="op-gpu-enable"
          >
            {gpuRuntimePhase === 'Failed' ? 'Retry GPU runtime' : 'Enable GPU runtime'}
          </Button>
        </div>
      </Form>

      <Form style={{ maxWidth: '36rem', marginTop: '1rem' }}>
        <Title headingLevel="h3" size="md">
          KubeVirt platform
        </Title>
        <p style={{ marginTop: '0.35rem', fontSize: '0.875rem' }}>
          Installs Multus, CDI, and KubeVirt. Same as{' '}
          <code>umkim cluster platform enable-kubevirt</code>. Enable{' '}
          <strong>LAN Multus</strong> to put guests on the node LAN (host bridge + NAD{' '}
          <code>mgmt-l2</code> → capability <code>network.multus_l2</code>).
        </p>
        {multusL2 ? (
          <Label color="green" style={{ marginTop: '0.5rem' }}>
            Multus LAN L2 ready
          </Label>
        ) : null}
        <Checkbox
          id="op-kubevirt-lan"
          label="LAN Multus (host bridge + mgmt-l2 NAD)"
          isChecked={lanBridge || multusL2}
          isDisabled={!canMutate || multusL2}
          onChange={(_e, checked) => setLanBridge(checked)}
          style={{ marginTop: '0.75rem' }}
        />
        {(lanBridge || multusL2) && (
          <LabeledField
            label="Host bridge name"
            fieldId="opKubevirtBridge"
            help={getFieldHelp('clusterJoin.kubevirtBridgeName')}
          >
            <TextInput
              id="opKubevirtBridge"
              value={bridgeName}
              onChange={(_e, v) => setBridgeName(v)}
              isDisabled={!canMutate || multusL2}
            />
          </LabeledField>
        )}
        <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginTop: '0.75rem' }}>
          <Button
            variant="secondary"
            isLoading={acting}
            isDisabled={!canMutate}
            onClick={() => void enableKubevirt({ lanBridge: false })}
            data-testid="op-enable-kubevirt"
          >
            {kubevirtPhase === 'Failed'
              ? 'Retry KubeVirt'
              : kubevirtReady
                ? 'Re-apply KubeVirt'
                : 'Enable KubeVirt'}
          </Button>
          <Button
            variant="primary"
            isLoading={acting}
            isDisabled={!canMutate || multusL2}
            onClick={() => void enableKubevirt({ lanBridge: true })}
            data-testid="op-enable-kubevirt-lan"
          >
            {kubevirtReady ? 'Enable LAN Multus' : 'Enable KubeVirt + LAN Multus'}
          </Button>
        </div>
      </Form>

      <Form style={{ maxWidth: '36rem', marginTop: '1rem' }}>
        <Title headingLevel="h3" size="md">
          Add worker
        </Title>
        <LabeledField
          label="Address"
          fieldId="addWorkerAddress"
          isRequired
          help={getFieldHelp('clusterOperations.addWorkerAddress')}
        >
          <TextInput
            id="addWorkerAddress"
            value={addAddress}
            onChange={(_e, v) => setAddAddress(v)}
            placeholder="198.51.100.60 or hostname"
            isDisabled={!canMutate}
          />
        </LabeledField>
        <LabeledField
          label="Hostname"
          fieldId="addWorkerHostname"
          help={getFieldHelp('clusterOperations.addWorkerHostname')}
        >
          <TextInput
            id="addWorkerHostname"
            value={addHostname}
            onChange={(_e, v) => setAddHostname(v)}
            placeholder="Optional K8s node name"
            isDisabled={!canMutate}
          />
        </LabeledField>
        <Button
          variant="primary"
          onClick={() => void addWorker()}
          isDisabled={!canMutate || !addAddress.trim()}
          isLoading={acting || validating}
          data-testid="add-worker"
        >
          Add worker
        </Button>

        <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
          Remove worker
        </Title>
        {workerNodes.length === 0 ? (
          <p style={{ fontSize: '0.875rem', color: 'var(--pf-v6-global--Color--200)' }}>
            No worker nodes in the cluster node list.
          </p>
        ) : (
          <>
            <LabeledField
              label="Worker node"
              fieldId="removeWorkerNode"
              help={getFieldHelp('clusterOperations.removeWorker')}
            >
              <FormSelect
                id="removeWorkerNode"
                value={removeNode}
                onChange={(_e, v) => setRemoveNode(v)}
                isDisabled={!canMutate}
              >
                {workerNodes.map((n) => (
                  <FormSelectOption key={n.name} value={n.name} label={n.name} />
                ))}
              </FormSelect>
            </LabeledField>
            <Button
              variant="danger"
              onClick={() => {
                setActionError(null);
                setPreValidate(null);
                setRemoveOpen(true);
              }}
              isDisabled={!canMutate || !removeNode}
              data-testid="remove-worker"
            >
              Remove worker
            </Button>
          </>
        )}

        <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
          Replace control plane
        </Title>
        {controlNodes.length === 0 ? (
          <p style={{ fontSize: '0.875rem', color: 'var(--pf-v6-global--Color--200)' }}>
            No control-plane nodes in the cluster node list.
          </p>
        ) : (
          <>
            <LabeledField
              label="Node to replace"
              fieldId="replaceOldNode"
              help={getFieldHelp('clusterOperations.replaceOldNode')}
            >
              <FormSelect
                id="replaceOldNode"
                value={replaceOldNode}
                onChange={(_e, v) => setReplaceOldNode(v)}
                isDisabled={!canMutate}
              >
                {controlNodes.map((n) => (
                  <FormSelectOption key={n.name} value={n.name} label={n.name} />
                ))}
              </FormSelect>
            </LabeledField>
            <LabeledField
              label="New node address"
              fieldId="replaceNewAddress"
              isRequired
              help={getFieldHelp('clusterOperations.replaceNewAddress')}
            >
              <TextInput
                id="replaceNewAddress"
                value={replaceAddress}
                onChange={(_e, v) => setReplaceAddress(v)}
                placeholder="198.51.100.61"
                isDisabled={!canMutate}
              />
            </LabeledField>
            <LabeledField
              label="New node hostname"
              fieldId="replaceNewHostname"
              help={getFieldHelp('clusterOperations.replaceNewHostname')}
            >
              <TextInput
                id="replaceNewHostname"
                value={replaceHostname}
                onChange={(_e, v) => setReplaceHostname(v)}
                placeholder="Optional"
                isDisabled={!canMutate}
              />
            </LabeledField>
            <LabeledField
              label="Force"
              fieldId="replaceForce"
              help={getFieldHelp('clusterOperations.replaceForce')}
            >
              <Checkbox
                id="replaceForce"
                label="Force replace when the old node is unreachable"
                isChecked={replaceForce}
                onChange={(_e, checked) => setReplaceForce(checked)}
                isDisabled={!canMutate}
              />
            </LabeledField>
            <Button
              variant="primary"
              onClick={() => void replaceControl()}
              isDisabled={!canMutate || !replaceOldNode || !replaceAddress.trim()}
              isLoading={acting || validating}
              data-testid="replace-control"
            >
              Replace control plane
            </Button>
          </>
        )}
      </Form>

      <Modal
        title="Remove worker node"
        isOpen={removeOpen}
        onClose={() => setRemoveOpen(false)}
        actions={[
          <Button
            key="remove"
            variant="danger"
            onClick={() => void confirmRemoveWorker()}
            isLoading={acting || validating}
            isDisabled={!removeNode}
          >
            Remove
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setRemoveOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <p>
          Remove worker <strong>{removeNode}</strong> from cluster <strong>{clusterName}</strong>?
          This drains the node, removes it from K3s, and updates fleet inventory. Pre-validation runs
          before removal starts.
        </p>
        <LabeledField
          label="Force"
          fieldId="removeWorkerForce"
          help={getFieldHelp('clusterOperations.removeWorkerForce')}
        >
          <Checkbox
            id="removeWorkerForce"
            label="Force remove when the node is unreachable"
            isChecked={removeForce}
            onChange={(_e, checked) => setRemoveForce(checked)}
          />
        </LabeledField>
      </Modal>

      <NodeOperationHistory clusterName={clusterName} refreshKey={historyRefreshKey} />
    </div>
  );
}

function buildValidateBody(
  nodeOp: ClusterNodeOperation,
  fields: {
    addAddress: string;
    addHostname: string;
    removeNode: string;
    removeForce: boolean;
    replaceOldNode: string;
    replaceAddress: string;
    replaceHostname: string;
    replaceForce: boolean;
  },
): Record<string, unknown> {
  switch (nodeOp.type) {
    case 'addWorker':
      return {
        type: 'addWorker',
        address: fields.addAddress,
        hostname: fields.addHostname || undefined,
        role: 'worker',
      };
    case 'removeWorker':
      return {
        type: 'removeWorker',
        node: nodeOp.node || fields.removeNode,
        force: nodeOp.force ?? fields.removeForce,
      };
    case 'replaceControlPlane':
      return {
        type: 'replaceControlPlane',
        oldNode: nodeOp.node || fields.replaceOldNode,
        force: nodeOp.force ?? fields.replaceForce,
        newNode: {
          address: fields.replaceAddress,
          hostname: fields.replaceHostname || undefined,
        },
      };
    default:
      return { type: nodeOp.type, node: nodeOp.node };
  }
}
