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

export interface FleetOrgSpec {
  slug: string;
  displayName?: string;
  sharedQuota?: { cpuMilli?: number; memoryBytes?: number };
}

export interface FleetOrgStatus {
  sharedUsage?: { cpuMilli?: number; memoryBytes?: number };
}

export interface FleetOrg {
  metadata?: { name?: string };
  spec: FleetOrgSpec;
  status?: FleetOrgStatus;
}

export interface FleetOrgCreate {
  slug: string;
  displayName?: string;
  sharedQuota?: { cpuMilli?: number; memoryBytes?: number };
}

export async function listOrgs(): Promise<FleetOrg[]> {
  const data = await authRequest<{ orgs: FleetOrg[] }>('/orgs');
  return data.orgs ?? [];
}

export async function createOrg(body: FleetOrgCreate): Promise<FleetOrg> {
  return authRequest('/orgs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      metadata: { name: body.slug },
      spec: {
        slug: body.slug,
        displayName: body.displayName,
        sharedQuota: body.sharedQuota,
      },
    }),
  });
}

export async function patchOrg(
  slug: string,
  patch: { displayName?: string; sharedQuota?: { cpuMilli?: number; memoryBytes?: number } },
): Promise<FleetOrg> {
  return authRequest(`/orgs/${encodeURIComponent(slug)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ spec: patch }),
  });
}

export async function deleteOrg(slug: string): Promise<void> {
  await authRequest(`/orgs/${encodeURIComponent(slug)}`, { method: 'DELETE' });
}

export async function listOrgUsers(org: string): Promise<unknown[]> {
  const data = await authRequest<{ users: unknown[] }>(`/orgs/${encodeURIComponent(org)}/users`);
  return data.users ?? [];
}

export async function listOrgRoleBindings(org: string): Promise<unknown[]> {
  const data = await authRequest<{ roleBindings: unknown[] }>(
    `/orgs/${encodeURIComponent(org)}/role-bindings`,
  );
  return data.roleBindings ?? [];
}

export async function createOrgRoleBinding(org: string, body: unknown): Promise<unknown> {
  return authRequest(`/orgs/${encodeURIComponent(org)}/role-bindings`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function deleteOrgRoleBinding(org: string, name: string): Promise<void> {
  await authRequest(`/orgs/${encodeURIComponent(org)}/role-bindings/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
}

export interface FleetLLMApiKey {
  metadata?: { name?: string };
  spec?: {
    org?: string;
    displayName?: string;
    models?: string[];
  };
  status?: {
    keyPrefix?: string;
    active?: boolean;
  };
}

export async function listOrgLLMApiKeys(
  org: string,
): Promise<{ apiEndpoint?: string; keys: FleetLLMApiKey[] }> {
  return authRequest(`/orgs/${encodeURIComponent(org)}/llm-api-keys`);
}

export async function createOrgLLMApiKey(
  org: string,
  body: { name: string; displayName?: string; models?: string[] },
): Promise<{ key: FleetLLMApiKey; token: string }> {
  return authRequest(`/orgs/${encodeURIComponent(org)}/llm-api-keys`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function deleteOrgLLMApiKey(org: string, name: string): Promise<void> {
  await authRequest(`/orgs/${encodeURIComponent(org)}/llm-api-keys/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
}

export async function rotateOrgLLMApiKey(
  org: string,
  name: string,
): Promise<{ key: FleetLLMApiKey; token: string }> {
  return authRequest(`/orgs/${encodeURIComponent(org)}/llm-api-keys/${encodeURIComponent(name)}/rotate`, {
    method: 'POST',
  });
}

export async function revokeOrgLLMApiKey(org: string, name: string): Promise<FleetLLMApiKey> {
  return authRequest(`/orgs/${encodeURIComponent(org)}/llm-api-keys/${encodeURIComponent(name)}/revoke`, {
    method: 'POST',
  });
}

export async function activateOrgLLMApiKey(org: string, name: string): Promise<FleetLLMApiKey> {
  return authRequest(`/orgs/${encodeURIComponent(org)}/llm-api-keys/${encodeURIComponent(name)}/activate`, {
    method: 'POST',
  });
}
