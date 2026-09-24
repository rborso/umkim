import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../auth/AuthContext';
import { ClusterDetailPage } from './ClusterDetailPage';
import type { FleetCluster } from '../api/types';

const getMe = vi.fn();
const apiLogin = vi.fn();

vi.mock('../api/authClient', () => ({
  getMe: (...args: unknown[]) => getMe(...args),
  login: (...args: unknown[]) => apiLogin(...args),
  logout: vi.fn(),
}));

const getFleetCluster = vi.fn();
const patchFleetCluster = vi.fn();

vi.mock('../api/client', () => ({
  getFleetCluster: (...args: unknown[]) => getFleetCluster(...args),
  listFleetSites: vi.fn(async () => []),
  listFleetClusterNodes: vi.fn(async () => []),
  patchFleetCluster: (...args: unknown[]) => patchFleetCluster(...args),
  deleteFleetCluster: vi.fn(),
  disableClusterIngressAuth: vi.fn(),
  disableClusterAcme: vi.fn(),
  downloadFleetClusterBootstrapBundle: vi.fn(),
  downloadFleetClusterBootstrapScript: vi.fn(),
  enableClusterIngressAuth: vi.fn(),
  enableClusterAcme: vi.fn(),
  fleetClusterIsConsoleProvisioned: () => false,
  fleetClusterIsDeleting: () => false,
  fleetClusterIsPending: () => false,
  fleetClusterPhase: (c: FleetCluster) => c.provisioning_phase || '',
  fleetSiteById: () => undefined,
  patchFleetClusterNode: vi.fn(),
  provisionFleetCluster: vi.fn(),
}));

function cluster(jumphost: string): FleetCluster {
  return {
    name: 'lab',
    site: 'homelab',
    available: true,
    provisioning_phase: 'Ready',
    jumphost,
    labels: {},
  };
}

function renderPage() {
  return render(
    <AuthProvider>
      <MemoryRouter initialEntries={['/clusters/lab']}>
        <Routes>
          <Route path="/clusters/:name" element={<ClusterDetailPage />} />
        </Routes>
      </MemoryRouter>
    </AuthProvider>,
  );
}

describe('ClusterDetailPage form polling', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    getMe.mockResolvedValue({
      kind: 'User',
      name: 'ada',
      groups: [],
      permissions: [],
      dcAdmin: true,
      roles: ['dc-admin'],
    });
    let saved = false;
    let loads = 0;
    getFleetCluster.mockImplementation(async () => {
      loads += 1;
      if (saved) return cluster('saved-host');
      return cluster(loads === 1 ? 'jump.example' : 'server-changed');
    });
    patchFleetCluster.mockImplementation(async () => {
      saved = true;
      return cluster('saved-host');
    });
  });

  it('keeps a typed jumphost across a poll and adopts the server value after save', async () => {
    renderPage();

    const edge = await screen.findByRole('tab', { name: 'Edge and ingress' });
    fireEvent.click(edge);
    const input = await screen.findByTestId('cluster-jumphost');
    expect(input).toHaveValue('jump.example');

    fireEvent.change(input, { target: { value: 'draft-jump' } });
    expect(screen.getByTestId('cluster-jumphost')).toHaveValue('draft-jump');

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000);
    });
    expect(screen.getByTestId('cluster-jumphost')).toHaveValue('draft-jump');

    fireEvent.click(screen.getByRole('button', { name: 'Save cluster settings' }));
    await waitFor(() => expect(screen.getByTestId('cluster-jumphost')).toHaveValue('saved-host'));
  });
});
