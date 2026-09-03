import type { AuthUser, OIDCConfig } from '../auth/types';

const AUTH_BASE = '/fleet/auth/v1alpha1';

export class AuthError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = 'AuthError';
  }
}

async function authRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(AUTH_BASE + path, {
    credentials: 'include',
    headers: { Accept: 'application/json', ...init?.headers },
    ...init,
  });
  const text = await r.text();
  if (!r.ok) {
    throw new AuthError(path + ' → HTTP ' + r.status, r.status);
  }
  if (!text) return {} as T;
  return JSON.parse(text) as T;
}

export async function login(username: string, password: string): Promise<AuthUser> {
  return authRequest<AuthUser>('/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
}

export async function logout(): Promise<void> {
  await authRequest<void>('/logout', { method: 'POST' });
}

export async function getMe(): Promise<AuthUser> {
  return authRequest<AuthUser>('/me');
}

export async function getOIDCConfig(): Promise<OIDCConfig> {
  return authRequest<OIDCConfig>('/oidc/config');
}

export async function listAuthUsers(): Promise<unknown[]> {
  const data = await authRequest<{ users: unknown[] }>('/users');
  return data.users ?? [];
}

export async function listAuthGroups(): Promise<unknown[]> {
  const data = await authRequest<{ groups: unknown[] }>('/groups');
  return data.groups ?? [];
}

export async function listRoleBindings(): Promise<unknown[]> {
  const data = await authRequest<{ roleBindings: unknown[] }>('/role-bindings');
  return data.roleBindings ?? [];
}

export async function listServiceAccounts(): Promise<unknown[]> {
  const data = await authRequest<{ serviceAccounts: unknown[] }>('/service-accounts');
  return data.serviceAccounts ?? [];
}

export async function listAuditEvents(): Promise<unknown[]> {
  const data = await authRequest<{ auditEvents: unknown[] }>('/audit-events');
  return data.auditEvents ?? [];
}

export async function listCatalogAccess(): Promise<unknown[]> {
  const data = await authRequest<{ catalogAccess: unknown[] }>('/catalog-access');
  return data.catalogAccess ?? [];
}

export async function createRoleBinding(body: unknown): Promise<unknown> {
  return authRequest('/role-bindings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function createServiceAccount(body: unknown): Promise<{ token?: string }> {
  return authRequest('/service-accounts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function upsertCatalogAccess(body: {
  name: string;
  catalogItemId: string;
  visibility: 'public' | 'private';
  grants?: { subject: { kind: string; name: string }; accessLevel: string }[];
}): Promise<unknown> {
  return authRequest('/catalog-access', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      metadata: { name: body.name },
      spec: {
        catalogItemId: body.catalogItemId,
        visibility: body.visibility,
        grants: body.grants ?? [],
      },
    }),
  });
}

export interface FleetLLMApiKey {
  metadata?: { name?: string };
  spec?: {
    displayName?: string;
    models?: string[];
  };
  status?: {
    keyPrefix?: string;
    active?: boolean;
  };
}

export async function listLLMApiKeys(): Promise<{ apiEndpoint?: string; keys: FleetLLMApiKey[] }> {
  return authRequest('/llm-api-keys');
}

export async function createLLMApiKey(body: {
  name: string;
  displayName?: string;
  models?: string[];
}): Promise<{ key: FleetLLMApiKey; token: string }> {
  return authRequest('/llm-api-keys', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function deleteLLMApiKey(name: string): Promise<void> {
  await authRequest(`/llm-api-keys/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

export async function rotateLLMApiKey(name: string): Promise<{ key: FleetLLMApiKey; token: string }> {
  return authRequest(`/llm-api-keys/${encodeURIComponent(name)}/rotate`, { method: 'POST' });
}

export async function revokeLLMApiKey(name: string): Promise<FleetLLMApiKey> {
  return authRequest(`/llm-api-keys/${encodeURIComponent(name)}/revoke`, { method: 'POST' });
}

export async function activateLLMApiKey(name: string): Promise<FleetLLMApiKey> {
  return authRequest(`/llm-api-keys/${encodeURIComponent(name)}/activate`, { method: 'POST' });
}
