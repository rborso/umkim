import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { getMe, login as apiLogin, logout as apiLogout } from '../api/authClient';
import type { AuthUser, EffectivePermission } from './types';

interface AuthContextValue {
  user: AuthUser | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  can: (resource: string, verb: string, scopeKind?: string, scopeName?: string) => boolean;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const AUTH_BYPASS = import.meta.env.VITE_AUTH_BYPASS === 'true';

const bypassUser: AuthUser = {
  kind: 'User',
  name: 'bypass',
  username: 'dev',
  displayName: 'Dev bypass',
  groups: [],
  permissions: [{ resource: '*', verb: '*' }],
  dcAdmin: true,
  orgs: [],
  roles: ['dc-admin'],
};

function matchPerm(
  perms: EffectivePermission[],
  resource: string,
  verb: string,
  scopeKind?: string,
  scopeName?: string,
): boolean {
  return perms.some((p) => {
    const resOk = p.resource === '*' || p.resource === resource;
    const verbOk = p.verb === '*' || p.verb === verb;
    const scopeOk =
      !scopeKind ||
      !p.scopeKind ||
      p.scopeKind === '*' ||
      (p.scopeKind === scopeKind && (!scopeName || !p.scopeName || p.scopeName === '*' || p.scopeName === scopeName));
    return resOk && verbOk && scopeOk;
  });
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(AUTH_BYPASS ? bypassUser : null);
  const [loading, setLoading] = useState(!AUTH_BYPASS);

  const refresh = useCallback(async () => {
    if (AUTH_BYPASS) {
      setUser(bypassUser);
      setLoading(false);
      return;
    }
    try {
      const me = await getMe();
      setUser({
        ...me,
        groups: me.groups ?? [],
        permissions: me.permissions ?? [],
        dcAdmin: !!me.dcAdmin,
        orgs: me.orgs ?? [],
        roles: me.roles ?? [],
      });
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(async (username: string, password: string) => {
    const me = await apiLogin(username, password);
    setUser({
      ...me,
      groups: me.groups ?? [],
      permissions: me.permissions ?? [],
      dcAdmin: !!me.dcAdmin,
    });
  }, []);

  const logout = useCallback(async () => {
    await apiLogout();
    setUser(null);
  }, []);

  const can = useCallback(
    (resource: string, verb: string, scopeKind?: string, scopeName?: string) => {
      if (!user) return false;
      if (user.dcAdmin) return true;
      return matchPerm(user.permissions, resource, verb, scopeKind, scopeName);
    },
    [user],
  );

  const value = useMemo(
    () => ({ user, loading, login, logout, refresh, can }),
    [user, loading, login, logout, refresh, can],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

export function usePermission(resource: string, verb: string, scopeKind?: string, scopeName?: string): boolean {
  const { can } = useAuth();
  return can(resource, verb, scopeKind, scopeName);
}
