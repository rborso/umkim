import type { CatalogFieldDef } from '../api/types';
import { orderFieldsByDependency } from './fieldVisibility';

/** Injected into every helm/container deploy form when not already defined in the catalog item. */
export const FLEET_DOMAIN_OVERRIDE_FIELDS: CatalogFieldDef[] = [
  {
    path: 'routing.hub_host',
    display_name: 'Hub hostname (custom FQDN)',
    editable: true,
    default: '',
    description: 'Override hub URL. Leave empty for {release}.{hub-zone} from the fleet edge registry.',
    validation_schema: {
      type: 'string',
      'x-ui-widget': 'hostname',
      'x-ui-tier': 'common',
      'x-ui-depends-on': { path: 'routing.mode', in: ['hub'] },
    },
  },
  {
    path: 'routing.site_host',
    display_name: 'Site hostname (custom FQDN)',
    editable: true,
    default: '',
    description: 'Override site URL. Leave empty to derive from release name and site zone.',
    validation_schema: {
      type: 'string',
      'x-ui-widget': 'hostname',
      'x-ui-tier': 'common',
      'x-ui-depends-on': { path: 'routing.mode', in: ['site'] },
    },
  },
];

/** Chart-managed Traefik ingress (routing.mode=none). */
export const FLEET_CHART_INGRESS_DOMAIN_FIELDS: CatalogFieldDef[] = [
  {
    path: 'values.fleet.ingress.host',
    display_name: 'Hostname prefix',
    editable: true,
    default: '',
    description: 'Subdomain before the base domain (e.g. git → git.example.com). Defaults to release name.',
    validation_schema: {
      type: 'string',
      'x-ui-tier': 'common',
      'x-ui-widget': 'dns-label',
      'x-ui-depends-on': { path: 'routing.mode', equals: 'none' },
    },
  },
  {
    path: 'values.fleet.ingress.baseDomain',
    display_name: 'Base domain',
    editable: true,
    default: '',
    description: 'DNS zone for chart ingress. Pick a hub or site zone, or enter a custom domain.',
    validation_schema: {
      type: 'string',
      'x-ui-tier': 'common',
      'x-ui-widget': 'site-domain',
      'x-ui-depends-on': { path: 'routing.mode', equals: 'none' },
    },
  },
];

const DOMAIN_PATHS = new Set([
  'routing.mode',
  'routing.hub_host',
  'routing.site_host',
  'values.fleet.ingress.host',
  'values.fleet.ingress.baseDomain',
  'values.__preset.exposure',
]);

const FLEET_ROUTING_MODE_FIELD: CatalogFieldDef = {
  path: 'routing.mode',
  display_name: 'Routing mode',
  editable: true,
  default: 'none',
  validation_schema: {
    type: 'string',
    // Console offers hub | site | none. API/controllers still accept dual for advanced use.
    enum: ['hub', 'site', 'none'],
    'x-ui-tier': 'common',
  },
};

/** Strip dual from routing.mode enums so the console never offers it. */
function withoutDualEnum(schema: CatalogFieldDef['validation_schema']): CatalogFieldDef['validation_schema'] {
  if (!schema || !Array.isArray(schema.enum)) return schema;
  const next = schema.enum.filter((v) => v !== 'dual');
  return next.length === schema.enum.length ? schema : { ...schema, enum: next };
}

function enrichExistingField(existing: CatalogFieldDef, canonical: CatalogFieldDef): CatalogFieldDef {
  const vs = withoutDualEnum({
    ...(existing.validation_schema || {}),
    ...(canonical.validation_schema || {}),
  });
  return {
    ...canonical,
    ...existing,
    display_name: existing.display_name || canonical.display_name,
    description: existing.description || canonical.description,
    default: existing.default !== undefined ? existing.default : canonical.default,
    validation_schema: vs,
  };
}

export function augmentDeployFields(
  serviceType: string,
  catalogFields: CatalogFieldDef[],
): CatalogFieldDef[] {
  if (serviceType !== 'helm' && serviceType !== 'container') {
    return catalogFields;
  }

  const byPath = new Map<string, CatalogFieldDef>();
  for (const field of catalogFields) {
    byPath.set(field.path, field);
  }

  const additions =
    serviceType === 'helm'
      ? [
          ...(byPath.has('routing.mode') || byPath.has('values.__preset.exposure')
            ? []
            : [FLEET_ROUTING_MODE_FIELD]),
          ...FLEET_DOMAIN_OVERRIDE_FIELDS,
          ...FLEET_CHART_INGRESS_DOMAIN_FIELDS,
        ]
      : [...FLEET_DOMAIN_OVERRIDE_FIELDS];

  for (const add of additions) {
    const existing = byPath.get(add.path);
    if (!existing) {
      byPath.set(add.path, add);
      continue;
    }
    if (DOMAIN_PATHS.has(add.path)) {
      byPath.set(add.path, enrichExistingField(existing, add));
    }
  }

  // Catalog items may still declare dual in enum; never offer it in the console.
  const mode = byPath.get('routing.mode');
  if (mode) {
    byPath.set('routing.mode', {
      ...mode,
      validation_schema: withoutDualEnum(mode.validation_schema),
    });
  }

  return orderFieldsByDependency(orderDomainFields([...byPath.values()]));
}

/** Prefer Exposure → Routing mode → mode dependents, then remaining fields. */
function orderDomainFields(fields: CatalogFieldDef[]): CatalogFieldDef[] {
  const preferred = [
    'values.__preset.exposure',
    'routing.mode',
    'routing.cluster',
    'routing.require_auth',
    'routing.hub_host',
    'routing.site_host',
    'values.fleet.ingress.host',
    'values.fleet.ingress.baseDomain',
  ];
  const byPath = new Map(fields.map((f) => [f.path, f]));
  const out: CatalogFieldDef[] = [];
  const used = new Set<string>();
  for (const path of preferred) {
    const f = byPath.get(path);
    if (f) {
      out.push(f);
      used.add(path);
    }
  }
  for (const f of fields) {
    if (!used.has(f.path)) out.push(f);
  }
  return out;
}

export function deploySupportsDomainRouting(serviceType: string): boolean {
  return serviceType === 'helm' || serviceType === 'container';
}

export function deployHasSiteDomainField(fields: CatalogFieldDef[]): boolean {
  return fields.some((f) => f.path === 'values.fleet.ingress.baseDomain');
}
