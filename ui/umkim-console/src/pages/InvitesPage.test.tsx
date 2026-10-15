import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../auth/AuthContext';
import { InvitesPage } from './InvitesPage';
import type { FleetCluster } from '../api/types';

const getMe = vi.fn();
const listInvites = vi.fn();
const listTenants = vi.fn();
const listFleetClusters = vi.fn();
const createInvite = vi.fn();
const deleteInvite = vi.fn();

vi.mock('../api/authClient', () => ({
  getMe: (...args: unknown[]) => getMe(...args),
  login: vi.fn(),
  logout: vi.fn(),
}));

vi.mock('../api/client', () => ({
  listInvites: (...args: unknown[]) => listInvites(...args),
  listTenants: (...args: unknown[]) => listTenants(...args),
  listFleetClusters: (...args: unknown[]) => listFleetClusters(...args),
  createInvite: (...args: unknown[]) => createInvite(...args),
  deleteInvite: (...args: unknown[]) => deleteInvite(...args),
}));

function cluster(name: string, ingressAuth: boolean, directory: string): FleetCluster {
  return {
    name,
    site: 'homelab',
    ingress_auth_enabled: ingressAuth,
    ingressAuthStatus: { directory },
  };
}

function renderPage() {
  return render(
    <AuthProvider>
      <MemoryRouter>
        <InvitesPage />
      </MemoryRouter>
    </AuthProvider>,
  );
}

describe('InvitesPage target directory', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getMe.mockResolvedValue({ kind: 'User', name: 'ada', groups: [], dcAdmin: true, roles: ['dc-admin'] });
    listTenants.mockResolvedValue({ groups: [{ name: 'labtenant', system: false }] });
    listInvites.mockResolvedValue([]);
    listFleetClusters.mockResolvedValue([
      cluster('lab-e2e-7', true, 'local'),
      cluster('rp-test', true, 'hub'),
      cluster('no-auth', false, ''),
    ]);
    createInvite.mockResolvedValue({
      id: 'inv-1',
      token: 'tok',
      path: '/fleet/invite?token=tok',
      cluster: 'lab-e2e-7',
      links: [{ cluster: 'lab-e2e-7', url: 'https://auth.example.com/invite?token=tok' }],
    });
  });

  it('offers only clusters with ingress auth, labelled by directory', async () => {
    renderPage();
    const select = await screen.findByLabelText('invite target directory');
    await waitFor(() => {
      const options = Array.from(select.querySelectorAll('option')).map((o) => o.textContent ?? '');
      expect(options).toContain('lab-e2e-7 (own directory)');
    });
    const options = Array.from(select.querySelectorAll('option')).map((o) => o.textContent ?? '');
    expect(options).toContain('Hub directory (shared)');
    expect(options).toContain('rp-test (hub directory)');
    expect(options.some((o) => o.includes('no-auth'))).toBe(false);
  });

  const createButton = () => screen.getAllByRole('button', { name: 'Create invite' })[0];

  async function pickCluster(value: string) {
    const select = await screen.findByLabelText('invite target directory');
    await waitFor(() =>
      expect(Array.from(select.querySelectorAll('option')).map((o) => o.textContent)).toContain(
        'lab-e2e-7 (own directory)',
      ),
    );
    fireEvent.change(select, { target: { value } });
  }

  it('names groups by hand for a cluster with its own directory', async () => {
    renderPage();
    await pickCluster('lab-e2e-7');

    // The hub group list does not apply: no hub checkboxes and no "create one on
    // the Tenants page" advice, which would create it in the wrong directory.
    const input = await screen.findByLabelText(/Groups in lab-e2e-7/);
    expect(screen.queryByLabelText('labtenant')).not.toBeInTheDocument();
    expect(screen.queryByText('No tenant groups yet')).not.toBeInTheDocument();

    fireEvent.change(input, { target: { value: 'labtenant, movies' } });
    fireEvent.click(createButton());

    await waitFor(() => expect(createInvite).toHaveBeenCalledTimes(1));
    expect(createInvite.mock.calls[0][0]).toMatchObject({
      groups: ['labtenant', 'movies'],
      cluster: 'lab-e2e-7',
    });
    expect(await screen.findByLabelText('invite link lab-e2e-7')).toBeInTheDocument();
  });

  it('uses the hub group list for a cluster bound to the hub directory', async () => {
    renderPage();
    await pickCluster('rp-test');

    fireEvent.click(await screen.findByLabelText('labtenant'));
    fireEvent.click(createButton());

    await waitFor(() => expect(createInvite).toHaveBeenCalledTimes(1));
    expect(createInvite.mock.calls[0][0]).toMatchObject({
      groups: ['labtenant'],
      cluster: 'rp-test',
    });
  });

  it('leaves the target empty for the shared hub directory', async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText('labtenant'));
    fireEvent.click(createButton());

    await waitFor(() => expect(createInvite).toHaveBeenCalledTimes(1));
    expect(createInvite.mock.calls[0][0].cluster).toBeUndefined();
  });

  it('shows the target directory for an existing invite', async () => {
    listInvites.mockResolvedValue([
      {
        id: 'inv-1',
        cluster: 'lab-e2e-7',
        groups: ['labtenant'],
        createdAt: '2026-10-15T06:00:00Z',
        expiresAt: '2026-10-16T06:00:00Z',
        used: false,
        expired: false,
      },
      {
        id: 'inv-2',
        groups: ['labtenant'],
        createdAt: '2026-10-15T06:00:00Z',
        expiresAt: '2026-10-16T06:00:00Z',
        used: false,
        expired: false,
      },
    ]);
    renderPage();
    expect(await screen.findByText('lab-e2e-7')).toBeInTheDocument();
    expect(screen.getByText('hub')).toBeInTheDocument();
  });
});
