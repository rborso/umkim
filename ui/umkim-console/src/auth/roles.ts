export function hasRole(user: { roles?: string[] } | null | undefined, role: string): boolean {
  return user?.roles?.includes(role) ?? false;
}

export function isViewer(user: { roles?: string[]; dcAdmin?: boolean } | null | undefined): boolean {
  if (!user || user.dcAdmin) return false;
  return hasRole(user, 'viewer');
}

export function canMutate(user: { roles?: string[]; dcAdmin?: boolean } | null | undefined): boolean {
  if (!user) return false;
  if (user.dcAdmin) return true;
  return !isViewer(user);
}
