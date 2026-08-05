import type { CatalogFieldDef, CatalogFieldUiTier, CatalogItem, CatalogResourceSpec, UserValue } from '../api/types';
import { isFieldVisible, uiDependsOnRule } from './fieldVisibility';
import { isSensitiveField, stripSensitiveOverlayPaths, applySecretBindsToValues } from './fieldSecrets';
import { isVirtualPresetPath } from './valuePresets';
import {
  assembleValues,
  catalogHasValuesAssembly,
  getByDotPath,
  isValuesOverlayPath,
  parseValuesDefault,
  valuesToDisplayString,
} from './valuesMerge';
import { augmentDeployFields } from './fleetEdgeRoutingFields';

/** Editable fields shown in common tier when ui_tier is not set. */
export const PRIMARY_FIELD_PATHS = new Set([
  'metadata.name',
  'release.namespace',
  'routing.mode',
  'subscription.package',
  'image.reference',
]);

export function catalogResourcesFromItem(item: CatalogItem): CatalogResourceSpec[] {
  const spec = item.spec as Record<string, unknown> | undefined;
  const resources = spec?.resources;
  if (Array.isArray(resources) && resources.length > 0) {
    return resources as CatalogResourceSpec[];
  }
  const serviceType =
    (spec?.service_type as string | undefined) ||
    item.service_type ||
    item.serviceType ||
    'container';
  const fields = spec?.fields;
  if (Array.isArray(fields)) {
    return [{ name: 'app', service_type: serviceType, fields: fields as CatalogFieldDef[] }];
  }
  return [];
}

export function primaryCatalogResource(item: CatalogItem): CatalogResourceSpec | undefined {
  const resources = catalogResourcesFromItem(item);
  return resources[0];
}

export function catalogItemServiceTypeFromItem(item: CatalogItem): string {
  return primaryCatalogResource(item)?.service_type || '—';
}

export function fieldsFromCatalogItem(item: CatalogItem): CatalogFieldDef[] {
  const resource = primaryCatalogResource(item);
  const raw = resource?.fields;
  if (!Array.isArray(raw)) return [];
  return raw as CatalogFieldDef[];
}

export function isCatalogFieldEditable(field: CatalogFieldDef): boolean {
  return field.editable === true;
}

export function readonlyFieldsFromCatalog(fields: CatalogFieldDef[]): CatalogFieldDef[] {
  return fields.filter((f) => !isCatalogFieldEditable(f) && !isValuesOverlayPath(f.path));
}

function schemaMeta(field: CatalogFieldDef, key: string): unknown {
  return field.validation_schema?.[key];
}

export function isFieldRequired(field: CatalogFieldDef): boolean {
  return field.required === true || schemaMeta(field, 'x-required') === true;
}

export function resolveFieldTier(field: CatalogFieldDef): CatalogFieldUiTier {
  const explicit =
    field.ui_tier ?? (schemaMeta(field, 'x-ui-tier') as CatalogFieldUiTier | undefined);
  if (explicit) return explicit;
  if (!isCatalogFieldEditable(field) && !isValuesOverlayPath(field.path)) return 'hidden';
  if (isFieldRequired(field)) return 'required';
  if (PRIMARY_FIELD_PATHS.has(field.path)) return 'common';
  if (field.path === 'values') return 'advanced';
  if (isValuesOverlayPath(field.path)) {
    if (isSensitiveField(field)) return 'required';
    return 'common';
  }
  return 'advanced';
}

export function resolveFieldWidget(field: CatalogFieldDef): string {
  const explicit =
    field.ui_widget ?? (schemaMeta(field, 'x-ui-widget') as string | undefined);
  if (explicit) return explicit;
  if (schemaMeta(field, 'x-sensitive') === true) return 'password';
  const enums = field.validation_schema?.enum;
  if (enums && enums.length > 0) return 'select';
  if (field.path === 'values' || isYamlLikeField(field)) return 'yaml';
  if (field.path.includes('PASSWORD')) return 'password';
  if (field.path.includes('baseDomain')) return 'site-domain';
  if (field.path.startsWith('resources.cpu.')) {
    if (field.validation_schema?.type === 'integer') return 'number';
    if (schemaMeta(field, 'x-ui-widget') === 'cpu-quantity') return 'cpu-quantity';
    if (field.validation_schema?.format === 'k8s-cpu-quantity') return 'cpu-quantity';
    return 'text';
  }
  if (field.path.startsWith('resources.memory.') || field.validation_schema?.format === 'k8s-memory-quantity') {
    return 'text';
  }
  if (field.validation_schema?.type === 'integer' || field.validation_schema?.type === 'number') {
    return 'number';
  }
  if (field.path === 'metadata.name') return 'dns-label';
  if (field.path === 'routing.site_host') return 'hostname';
  return 'text';
}

/**
 * Display tier for a field. Dependents inherit their controller's tier so a
 * checkbox in Common does not reveal its inputs under a different section.
 */
export function resolveDisplayTier(
  field: CatalogFieldDef,
  fields: CatalogFieldDef[],
  seen: Set<string> = new Set(),
): ReturnType<typeof resolveFieldTier> {
  if (seen.has(field.path)) return resolveFieldTier(field);
  seen.add(field.path);
  const rule = uiDependsOnRule(field);
  const parentPath = rule?.path;
  if (parentPath && parentPath !== field.path) {
    const parent = fields.find((f) => f.path === parentPath);
    if (parent) return resolveDisplayTier(parent, fields, seen);
  }
  return resolveFieldTier(field);
}

export function partitionByTier(fields: CatalogFieldDef[]): {
  required: CatalogFieldDef[];
  common: CatalogFieldDef[];
  advanced: CatalogFieldDef[];
  hidden: CatalogFieldDef[];
  readonly: CatalogFieldDef[];
} {
  const required: CatalogFieldDef[] = [];
  const common: CatalogFieldDef[] = [];
  const advanced: CatalogFieldDef[] = [];
  const hidden: CatalogFieldDef[] = [];
  const readonly: CatalogFieldDef[] = [];

  for (const f of fields) {
    if (!isCatalogFieldEditable(f) && !isValuesOverlayPath(f.path)) {
      readonly.push(f);
      continue;
    }
    const tier = resolveDisplayTier(f, fields);
    switch (tier) {
      case 'required':
        required.push(f);
        break;
      case 'common':
        common.push(f);
        break;
      case 'advanced':
        advanced.push(f);
        break;
      case 'hidden':
        hidden.push(f);
        break;
      default:
        advanced.push(f);
    }
  }
  return { required, common, advanced, hidden, readonly };
}

/** @deprecated Use partitionByTier */
export function partitionDeployFields(fields: CatalogFieldDef[]): {
  primary: CatalogFieldDef[];
  advanced: CatalogFieldDef[];
} {
  const { required, common, advanced } = partitionByTier(fields);
  return { primary: [...required, ...common], advanced };
}

export function defaultFieldValues(fields: CatalogFieldDef[]): Record<string, string> {
  const out: Record<string, string> = {};
  const valuesDefault = parseValuesDefault(fields);

  for (const f of fields) {
    if (f.path === 'values') {
      // Full YAML textarea starts empty; base defaults come from parseValuesDefault at submit.
      continue;
    }
    if (isVirtualPresetPath(f.path)) {
      if (f.default !== undefined && f.default !== null) {
        out[f.path] = String(f.default);
      }
      continue;
    }
    if (!isFieldVisible(f, out, fields)) {
      continue;
    }
    if (isValuesOverlayPath(f.path)) {
      const subpath = f.path.slice('values.'.length);
      const fromValues = getByDotPath(valuesDefault, subpath);
      if (fromValues !== undefined && fromValues !== null) {
        out[f.path] = String(fromValues);
        continue;
      }
    }
    if (f.default !== undefined && f.default !== null) {
      if (f.path === 'values' && typeof f.default === 'object') {
        out[f.path] = valuesToDisplayString(f.default as Record<string, unknown>);
      } else if (typeof f.default === 'object') {
        out[f.path] = JSON.stringify(f.default);
      } else {
        out[f.path] = String(f.default);
      }
    }
  }
  return out;
}

function isStructuredSchemaType(field: CatalogFieldDef | undefined): boolean {
  const type = field?.validation_schema?.type;
  return type === 'array' || type === 'object';
}

export function parseFieldValue(field: CatalogFieldDef | undefined, raw: string): unknown {
  const widget = field ? resolveFieldWidget(field) : 'text';
  if (isStructuredSchemaType(field)) {
    try {
      return JSON.parse(raw);
    } catch {
      return raw;
    }
  }
  if (widget === 'boolean') {
    const lower = raw.toLowerCase();
    if (lower === 'true' || lower === '1') return true;
    if (lower === 'false' || lower === '0') return false;
    return raw;
  }
  if (
    widget === 'dns-label' ||
    widget === 'hostname' ||
    widget === 'domain' ||
    field?.validation_schema?.type === 'string'
  ) {
    return raw.trim();
  }
  if (field?.validation_schema?.type === 'integer') {
    return Number(raw.trim());
  }
  if (field?.validation_schema?.format === 'k8s-cpu-quantity') {
    return raw;
  }
  if (field?.validation_schema?.type === 'number') {
    return Number(raw);
  }
  return raw;
}

function isInstanceField(
  field: CatalogFieldDef,
  fieldValues: Record<string, string>,
  fields: CatalogFieldDef[],
): boolean {
  if (isValuesOverlayPath(field.path)) return false;
  if (isVirtualPresetPath(field.path)) return false;
  if (field.path === 'values') return false;
  if (resolveFieldTier(field) === 'hidden') return false;
  if (!isFieldVisible(field, fieldValues, fields)) return false;
  return isCatalogFieldEditable(field);
}

export function buildUserValues(
  fields: CatalogFieldDef[],
  values: Record<string, string>,
): UserValue[] {
  const releaseName = values['metadata.name']?.trim() || '';
  const mergedValues = stripSensitiveOverlayPaths(fields, values);
  const userValues: UserValue[] = [];

  for (const field of fields) {
    if (!isInstanceField(field, mergedValues, fields)) continue;
    const path = field.path;
    const raw = mergedValues[path];
    if (raw === '') {
      continue;
    }
    if (raw === undefined) {
      if (field.default === undefined || field.default === null) continue;
      if (typeof field.default === 'string' && field.default === '') continue;
      const defaultVal =
        typeof field.default === 'number' ||
        typeof field.default === 'boolean' ||
        typeof field.default === 'object'
          ? field.default
          : parseFieldValue(field, String(field.default));
      userValues.push({ path, value: defaultVal });
      continue;
    }
    userValues.push({ path, value: parseFieldValue(field, raw) });
  }

  if (catalogHasValuesAssembly(fields)) {
    let merged = assembleValues(fields, mergedValues, parseFieldValue);
    if (releaseName) {
      merged = applySecretBindsToValues(fields, merged, releaseName, values);
    }
    if (Object.keys(merged).length === 0) return userValues;
    const valuesField = fields.find((f) => f.path === 'values');
    userValues.push({
      path: 'values',
      value: serializeValuesForUserValue(valuesField, merged),
    });
  }

  return userValues;
}

/** Helm servicetype schema requires values as a YAML string (not a JSON object). */
function serializeValuesForUserValue(
  _valuesField: CatalogFieldDef | undefined,
  merged: Record<string, unknown>,
): string {
  return valuesToDisplayString(merged);
}

/** True when path should use a multi-line YAML editor. */
export function isYamlLikeField(field: CatalogFieldDef): boolean {
  if (field.path === 'values') return true;
  if (field.ui_widget === 'yaml') return true;
  const label = (field.display_name || '').toLowerCase();
  return label.includes('yaml');
}

export function validateDeployFields(
  fields: CatalogFieldDef[],
  values: Record<string, string>,
): string | null {
  for (const field of fields) {
    if (!isFieldRequired(field)) continue;
    if (resolveFieldTier(field) === 'hidden') continue;
    if (!isFieldVisible(field, values, fields)) continue;
    const v = values[field.path];
    if (v === undefined || v.trim() === '') {
      return `${field.display_name || field.path} is required`;
    }
  }
  return null;
}

export const CONTAINER_ROUTING_FIELDS: CatalogFieldDef[] = [
  {
    path: 'routing.mode',
    display_name: 'Routing mode',
    editable: true,
    default: 'hub',
    validation_schema: { type: 'string', enum: ['hub', 'site', 'none'] },
  },
  {
    path: 'routing.require_auth',
    display_name: 'Require ingress authentication',
    editable: true,
    default: false,
    validation_schema: {
      type: 'boolean',
      'x-ui-widget': 'boolean',
      'x-ui-depends-on': { path: 'routing.mode', in: ['hub', 'site'] },
    },
  },
  {
    path: 'routing.site_host',
    display_name: 'Site hostname (custom FQDN)',
    editable: true,
    default: '',
    validation_schema: {
      type: 'string',
      'x-ui-widget': 'hostname',
      'x-ui-tier': 'common',
      'x-ui-depends-on': { path: 'routing.mode', in: ['site'] },
    },
  },
  {
    path: 'routing.hub_host',
    display_name: 'Hub hostname (custom FQDN)',
    editable: true,
    default: '',
    validation_schema: {
      type: 'string',
      'x-ui-widget': 'hostname',
      'x-ui-tier': 'common',
      'x-ui-depends-on': { path: 'routing.mode', in: ['hub'] },
    },
  },
  {
    path: 'routing.cluster',
    display_name: 'Target cluster',
    editable: true,
    default: '',
    validation_schema: { type: 'string', 'x-ui-tier': 'advanced' },
  },
  {
    path: 'routing.health_path',
    display_name: 'Hub health path',
    editable: true,
    default: '/',
    validation_schema: { type: 'string' },
  },
  {
    path: 'routing.failover_mode',
    display_name: 'Failover mode (override)',
    editable: true,
    default: '',
    validation_schema: { type: 'string', enum: ['', 'manual', 'relocatable', 'standby'] },
  },
];

export function deployFieldsForServiceType(
  serviceType: string,
  catalogFields: CatalogFieldDef[],
): CatalogFieldDef[] {
  if (catalogFields.length > 0) {
    const filtered = catalogFields.filter(
      (f) => isCatalogFieldEditable(f) || isValuesOverlayPath(f.path) || isVirtualPresetPath(f.path),
    );
    return augmentDeployFields(serviceType, filtered);
  }
  if (serviceType === 'container') {
    return augmentDeployFields(serviceType, [
      {
        path: 'metadata.name',
        display_name: 'Container name (DNS label)',
        editable: true,
        required: true,
        ui_tier: 'required',
        ui_widget: 'dns-label',
        default: '',
        validation_schema: { type: 'string' },
      },
      ...CONTAINER_ROUTING_FIELDS,
    ]);
  }
  if (serviceType === 'helm') {
    return augmentDeployFields(serviceType, [
      {
        path: 'metadata.name',
        display_name: 'Helm release name',
        editable: true,
        required: true,
        ui_tier: 'required',
        ui_widget: 'dns-label',
        default: '',
        validation_schema: { type: 'string' },
      },
      {
        path: 'release.namespace',
        display_name: 'Namespace',
        editable: true,
        ui_tier: 'common',
        default: 'default',
        validation_schema: { type: 'string' },
      },
      {
        path: 'values',
        display_name: 'Values (YAML)',
        editable: true,
        ui_tier: 'advanced',
        ui_widget: 'yaml',
        default: '',
        validation_schema: { type: 'string', format: 'yaml' },
      },
      ...CONTAINER_ROUTING_FIELDS,
    ]);
  }
  if (serviceType === 'operator') {
    return [
      {
        path: 'metadata.name',
        display_name: 'Subscription name',
        editable: true,
        required: true,
        ui_tier: 'required',
        default: '',
        validation_schema: { type: 'string' },
      },
      {
        path: 'subscription.package',
        display_name: 'OLM package',
        editable: true,
        required: true,
        ui_tier: 'required',
        default: '',
        validation_schema: { type: 'string' },
      },
      {
        path: 'subscription.channel',
        display_name: 'OLM channel',
        editable: true,
        ui_tier: 'common',
        default: 'stable',
        validation_schema: { type: 'string' },
      },
      {
        path: 'subscription.source',
        display_name: 'CatalogSource',
        editable: true,
        ui_tier: 'advanced',
        default: 'operatorhubio-catalog',
        validation_schema: { type: 'string' },
      },
      {
        path: 'subscription.sourceNamespace',
        display_name: 'CatalogSource namespace',
        editable: true,
        ui_tier: 'advanced',
        default: 'olm',
        validation_schema: { type: 'string' },
      },
      {
        path: 'operatorGroup.targetNamespace',
        display_name: 'Target namespace',
        editable: true,
        ui_tier: 'common',
        default: 'operators',
        validation_schema: { type: 'string' },
      },
      {
        path: 'installPlanApproval',
        display_name: 'InstallPlan approval',
        editable: true,
        ui_tier: 'advanced',
        default: 'Automatic',
        validation_schema: { type: 'string', enum: ['Automatic', 'Manual'] },
      },
    ];
  }
  return catalogFields;
}

export function allDeployFormFields(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string> = {},
): CatalogFieldDef[] {
  return fields.filter((f) => {
    if (!isCatalogFieldEditable(f) && !isValuesOverlayPath(f.path) && !isVirtualPresetPath(f.path)) {
      return false;
    }
    if (resolveFieldTier(f) === 'hidden') return false;
    return isFieldVisible(f, fieldValues, fields);
  });
}
