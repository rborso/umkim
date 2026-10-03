import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider, useAuth } from './AuthContext';
import { canMutate } from './roles';
import type { AuthUser } from './types';

const getMe = vi.fn();
const apiLogin = vi.fn();

vi.mock('../api/authClient', () => ({
  getMe: (...args: unknown[]) => getMe(...args),
  login: (...args: unknown[]) => apiLogin(...args),
  logout: vi.fn(),
}));

function Probe() {
  const { user, loading, login } = useAuth();
  if (loading) return <span>loading</span>;
  return (
    <>
      <button type="button" onClick={() => void login('ada', 'pw')}>
        sign in
      </button>
      <span data-testid="mutate">{String(canMutate(user))}</span>
      <span data-testid="name">{user?.name ?? ''}</span>
    </>
  );
}

function user(partial: Partial<AuthUser>): AuthUser {
  return {
    kind: 'User',
    name: 'ada',
    groups: [],
    permissions: [],
    dcAdmin: false,
    ...partial,
  };
}

describe('login roles', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    getMe.mockRejectedValue(new Error('no session'));
  });

  it('keeps a viewer from mutating after login', async () => {
    apiLogin.mockResolvedValue(user({ roles: ['viewer'] }));
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    fireEvent.click(await screen.findByRole('button', { name: 'sign in' }));
    expect(await screen.findByTestId('name')).toHaveTextContent('ada');
    expect(screen.getByTestId('mutate')).toHaveTextContent('false');
  });

  it('keeps a dc-admin mutable after login', async () => {
    apiLogin.mockResolvedValue(user({ dcAdmin: true, roles: ['dc-admin'] }));
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    fireEvent.click(await screen.findByRole('button', { name: 'sign in' }));
    expect(await screen.findByTestId('name')).toHaveTextContent('ada');
    expect(screen.getByTestId('mutate')).toHaveTextContent('true');
  });
});
