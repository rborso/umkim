import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ClusterOperationsPanel } from './ClusterOperationsPanel';
import type { FleetCluster } from '../../api/types';

vi.mock('../../api/client', () => ({
  listFleetClusterNodeOperations: vi.fn(async () => []),
  validateFleetClusterNodeOperation: vi.fn(),
  addFleetClusterWorker: vi.fn(),
  configureFleetClusterStorage: vi.fn(),
  enableFleetClusterKubevirt: vi.fn(),
  getFleetClusterGpuStatus: vi.fn(async () => ({
    hostPhase: 'NotPrepared',
    runtimePhase: 'Ready',
    runtimeMessage: 'NVIDIA GPU runtime enabled',
    driverLoaded: true,
    clusterGpuCapability: true,
  })),
  prepFleetClusterGpuHost: vi.fn(),
  enableFleetClusterGpu: vi.fn(),
  fetchFleetClusterNodeOperationLogs: vi.fn(),
  removeFleetClusterNode: vi.fn(),
  replaceFleetClusterControlPlane: vi.fn(),
}));

function cluster(phase: FleetCluster['provisioning_phase'], extra: Partial<FleetCluster> = {}): FleetCluster {
  return {
    name: 'lab',
    provisioning_phase: phase,
    available: phase === 'Ready',
    read_only: false,
    ...extra,
  };
}

describe('ClusterOperationsPanel hooks', () => {
  it('keeps hook order when a cluster becomes Ready and shows the bridge field', () => {
    const onUpdated = vi.fn();
    const { rerender } = render(
      <ClusterOperationsPanel cluster={cluster('Pending', { available: false })} nodes={[]} onUpdated={onUpdated} />,
    );
    expect(screen.queryByLabelText('Host bridge name')).not.toBeInTheDocument();

    rerender(
      <ClusterOperationsPanel
        cluster={cluster('Ready', { capabilities: { 'network.multus_l2': true } })}
        nodes={[]}
        onUpdated={onUpdated}
      />,
    );

    expect(screen.getByLabelText('Host bridge name')).toBeInTheDocument();
    expect(screen.getByDisplayValue('mgmt-l2-br')).toBeInTheDocument();
  });

  it('shows GPU runtime enabled when runtime is Ready but host phase was not recorded', async () => {
    const onUpdated = vi.fn();
    render(
      <ClusterOperationsPanel cluster={cluster('Ready')} nodes={[]} onUpdated={onUpdated} />,
    );
    expect(await screen.findByText(/GPU runtime enabled —/)).toBeInTheDocument();
    const enableBtn = screen.getAllByTestId('op-gpu-enable')[0];
    expect(enableBtn).toHaveTextContent('Runtime enabled');
    expect(enableBtn).toBeDisabled();
  });
});
