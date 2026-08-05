import type { AuthUser } from './types';

export function primaryOrg(user: AuthUser | null | undefined): string | undefined {
  return user?.orgs?.[0];
}

export function hasRole(user: AuthUser | null | undefined, role: string): boolean {
  return user?.roles?.includes(role) ?? false;
}

export function isOrgAdmin(user: AuthUser | null | undefined): boolean {
  return hasRole(user, 'org-admin');
}

export function isCustomerUser(user: AuthUser | null | undefined): boolean {
  if (!user || user.dcAdmin) return false;
  return hasRole(user, 'customer') || (user.orgs?.length ?? 0) > 0;
}

export function orgPrefixedName(org: string | undefined, base: string): string {
  if (!org) return base;
  if (base.startsWith(`${org}-`)) return base;
  return `${org}-${base}`;
}
