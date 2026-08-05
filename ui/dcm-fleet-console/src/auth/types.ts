export interface EffectivePermission {
  resource: string;
  verb: string;
  scopeKind?: string;
  scopeName?: string;
  accessLevel?: string;
}

export interface AuthUser {
  kind: string;
  name: string;
  username?: string;
  displayName?: string;
  groups: string[];
  permissions: EffectivePermission[];
  dcAdmin: boolean;
  orgs?: string[];
  roles?: string[];
}

export interface OIDCConfig {
  enabled: boolean;
  issuer?: string;
  authorizeUrl?: string;
}
