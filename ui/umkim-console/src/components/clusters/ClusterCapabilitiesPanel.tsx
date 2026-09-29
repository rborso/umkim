import { useEffect, useState } from 'react';
import { Button, Label, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import {
  discoverFleetClusterCapabilities,
  discoverFleetClusterGpu,
  getFleetClusterGpuStatus,
} from '../../api/client';
import type { FleetClusterGpuStatus } from '../../api/client';
import type { CapabilityDiscovery, FleetCluster } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

const CAP_LABELS: Record<string, string> = {
  gpu: 'GPU',
  'storage.rwo': 'Storage RWO',
  'storage.rwx': 'Storage RWX',
  kvm: 'KVM',
  kubevirt: 'KubeVirt',
  'network.multus_l2': 'Multus LAN L2',
  ha_topology: 'HA topology',
};

function gpuClusterSummary(
  caps: Record<string, boolean>,
  discovery?: { cluster_resources?: Record<string, number> },
): string {
  if (!caps.gpu) return 'no';
  const total = discovery?.cluster_resources?.['gpu.total'];
  if (total != null && total > 0) return `yes (${total} GPUs)`;
  return 'yes';
}

type Props = {
  clusterName: string;
  cluster: FleetCluster;
  clusterReady: boolean;
  onUpdated: () => void | Promise<void>;
};

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Succeeded':
      return 'green';
    case 'Running':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

export function ClusterCapabilitiesPanel({ clusterName, cluster, clusterReady, onUpdated }: Props) {
  const [acting, setActing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [gpuInfo, setGpuInfo] = useState<FleetClusterGpuStatus | null>(null);

  useEffect(() => {
    void getFleetClusterGpuStatus(clusterName)
      .then(setGpuInfo)
      .catch(() => setGpuInfo(null));
  }, [clusterName, cluster.capability_discovery?.completed_at]);

  const discovery: CapabilityDiscovery | undefined = cluster.capability_discovery;
  const caps = cluster.capabilities || {};
  const phase = discovery?.phase || 'Idle';
  const busy = acting || phase === 'Running';

  const runDiscover = async () => {
    setActing(true);
    setError(null);
    try {
      await discoverFleetClusterCapabilities(clusterName);
      await onUpdated();
    } catch (e) {
      setError(e);
    } finally {
      setActing(false);
    }
  };

  const runGpuDiscover = async () => {
    setActing(true);
    setError(null);
    try {
      await discoverFleetClusterGpu(clusterName);
      await onUpdated();
    } catch (e) {
      setError(e);
    } finally {
      setActing(false);
    }
  };

  return (
    <>
      <Title headingLevel="h2" size="lg" style={{ marginTop: '1.5rem' }}>
        Capabilities
      </Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '42rem' }}>
        Placement capabilities are discovered from cluster nodes (KVM via host check, storage classes,
        GPU resources, control-plane count). Re-run after hardware or storage changes.
      </p>
      <ErrorAlert error={error} />
      {gpuInfo?.pciId ? (
        <p style={{ marginTop: '0.5rem', fontSize: '0.875rem', maxWidth: '42rem' }}>
          Detected hardware: <strong>{gpuInfo.marketingName || gpuInfo.pciId}</strong>
          {gpuInfo.recommendedDriverBranch || gpuInfo.driverBranch ? (
            <>
              {' '}
              — driver branch <code>{gpuInfo.recommendedDriverBranch || gpuInfo.driverBranch}</code>
            </>
          ) : null}
          {gpuInfo.driverLoaded ? ' (driver loaded on host)' : ' (host driver not loaded)'}
        </p>
      ) : null}
      <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center', marginTop: '0.75rem', flexWrap: 'wrap' }}>
        <Button
          variant="primary"
          isDisabled={!clusterReady || busy}
          isLoading={busy}
          onClick={() => void runDiscover()}
          data-testid="discover-capabilities"
        >
          {phase === 'Succeeded' ? 'Re-discover capabilities' : 'Discover capabilities'}
        </Button>
        <Button
          variant="secondary"
          isDisabled={!clusterReady || busy}
          onClick={() => void runGpuDiscover()}
        >
          Re-run GPU discovery
        </Button>
        {discovery?.phase ? (
          <Label color={phaseColor(discovery.phase)}>{discovery.phase}</Label>
        ) : null}
        {discovery?.completed_at ? (
          <span style={{ fontSize: '0.85rem', color: 'var(--pf-v6-global--Color--200)' }}>
            Last run: {new Date(discovery.completed_at).toLocaleString()}
          </span>
        ) : null}
        {discovery?.message ? (
          <span style={{ fontSize: '0.85rem' }}>{discovery.message}</span>
        ) : null}
      </div>
      <Table variant="compact" style={{ marginTop: '0.75rem', maxWidth: '36rem' }}>
        <Thead>
          <Tr>
            <Th>Capability</Th>
            <Th>Cluster</Th>
          </Tr>
        </Thead>
        <Tbody>
          {Object.keys(CAP_LABELS).map((k) => (
            <Tr key={k}>
              <Td>{CAP_LABELS[k]}</Td>
              <Td>
                {k === 'gpu' ? (
                  <Label color={caps[k] ? 'green' : 'grey'}>
                    {gpuClusterSummary(caps, discovery)}
                  </Label>
                ) : (
                  <Label color={caps[k] ? 'green' : 'grey'}>{caps[k] ? 'yes' : 'no'}</Label>
                )}
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
      {discovery?.nodes && discovery.nodes.length > 0 ? (
        <>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }}>
            Per-node
          </Title>
          <Table variant="compact" style={{ marginTop: '0.5rem', maxWidth: '48rem' }}>
            <Thead>
              <Tr>
                <Th>Node</Th>
                <Th>KVM</Th>
                <Th>GPU</Th>
                <Th>GPU count</Th>
                <Th>PCI / driver</Th>
              </Tr>
            </Thead>
            <Tbody>
              {discovery.nodes.map((n) => (
                <Tr key={n.name}>
                  <Td>
                    <code>{n.name}</code>
                  </Td>
                  <Td>
                    <Label color={n.capabilities?.kvm ? 'green' : 'grey'}>
                      {n.capabilities?.kvm ? 'yes' : 'no'}
                    </Label>
                  </Td>
                  <Td>
                    <Label color={n.capabilities?.gpu ? 'green' : 'grey'}>
                      {n.capabilities?.gpu ? 'yes' : 'no'}
                    </Label>
                  </Td>
                  <Td>{n.gpu_count != null && n.gpu_count > 0 ? n.gpu_count : '—'}</Td>
                  <Td style={{ fontSize: '0.85rem' }}>
                    {n.nvidia_pci_id ? <code>{n.nvidia_pci_id}</code> : '—'}
                    {n.driver_loaded ? ' · driver ok' : n.nvidia_pci_id ? ' · no driver' : ''}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </>
      ) : null}
    </>
  );
}
