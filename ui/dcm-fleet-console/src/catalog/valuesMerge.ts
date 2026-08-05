import { parse as parseYaml, stringify as stringifyYaml } from 'yaml';
import type { CatalogFieldDef } from '../api/types';
import { applyValuePresets, isVirtualPresetPath } from './valuePresets';
import { isFieldVisible } from './fieldVisibility';

function isSensitiveOverlayField(field: CatalogFieldDef): boolean {
  if (field.validation_schema?.['x-sensitive'] === true) return true;
  return field.path.toLowerCase().includes('password');
}

export function isValuesOverlayPath(path: string): boolean {
  return path.startsWith('values.') && !isVirtualPresetPath(path);
}

export function valuesOverlaySubpath(path: string): string {
  return path.slice('values.'.length);
}

export function getByDotPath(obj: Record<string, unknown>, dotPath: string): unknown {
  const parts = dotPath.split('.');
  let cur: unknown = obj;
  for (const part of parts) {
    if (cur === null || cur === undefined || typeof cur !== 'object') return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}

export function setByDotPath(
  obj: Record<string, unknown>,
  dotPath: string,
  value: unknown,
): void {
  const parts = dotPath.split('.');
  let cur: Record<string, unknown> = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i];
    const next = cur[part];
    if (next === null || next === undefined || typeof next !== 'object' || Array.isArray(next)) {
      cur[part] = {};
    }
    cur = cur[part] as Record<string, unknown>;
  }
  cur[parts[parts.length - 1]] = value;
}

export function deepMerge(
  base: Record<string, unknown>,
  ...layers: Array<Record<string, unknown> | null | undefined>
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...base };
  for (const layer of layers) {
    if (!layer) continue;
    mergeInto(out, layer);
  }
  return out;
}

function mergeInto(target: Record<string, unknown>, source: Record<string, unknown>): void {
  for (const [key, value] of Object.entries(source)) {
    const existing = target[key];
    if (
      value !== null &&
      typeof value === 'object' &&
      !Array.isArray(value) &&
      existing !== null &&
      typeof existing === 'object' &&
      !Array.isArray(existing)
    ) {
      mergeInto(existing as Record<string, unknown>, value as Record<string, unknown>);
    } else {
      target[key] = value;
    }
  }
}

export function parseValuesDefault(fields: CatalogFieldDef[]): Record<string, unknown> {
  const valuesField = fields.find((f) => f.path === 'values');
  if (!valuesField?.default) return {};
  return parseValuesPayload(valuesField.default);
}

export function parseValuesPayload(raw: unknown): Record<string, unknown> {
  if (raw === null || raw === undefined) return {};
  if (typeof raw === 'object' && !Array.isArray(raw)) {
    return raw as Record<string, unknown>;
  }
  if (typeof raw === 'string') {
    const trimmed = raw.trim();
    if (!trimmed) return {};
    const parsed = parseYaml(trimmed);
    if (parsed === null || parsed === undefined) return {};
    if (typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
    return {};
  }
  return {};
}

export function buildOverlayFromFields(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
  parseValue: (field: CatalogFieldDef, raw: string) => unknown,
): Record<string, unknown> {
  const overlay: Record<string, unknown> = {};
  for (const field of fields) {
    if (!isValuesOverlayPath(field.path) || field.editable !== true) continue;
    if (isVirtualPresetPath(field.path)) continue;
    if (isSensitiveOverlayField(field)) continue;
    if (!isFieldVisible(field, fieldValues, fields)) continue;
    const subpath = valuesOverlaySubpath(field.path);
    let raw = fieldValues[field.path];
    if (raw === undefined || raw === '') {
      if (field.default === undefined || field.default === null) continue;
      setByDotPath(overlay, subpath, field.default);
      continue;
    }
    setByDotPath(overlay, subpath, parseValue(field, raw));
  }
  return overlay;
}

export function catalogHasValuesAssembly(fields: CatalogFieldDef[]): boolean {
  if (fields.some((f) => f.path === 'values' || isValuesOverlayPath(f.path))) return true;
  return false;
}

export function finalizeFleetIngressValues(merged: Record<string, unknown>): void {
  const fleet = merged.fleet as Record<string, unknown> | undefined;
  if (!fleet || typeof fleet !== 'object') return;
  const ingress = fleet.ingress as Record<string, unknown> | undefined;
  if (!ingress || typeof ingress !== 'object') return;

  const hostPrefix = typeof ingress.host === 'string' ? ingress.host.trim() : '';
  const baseDomain = typeof ingress.baseDomain === 'string' ? ingress.baseDomain.trim() : '';
  const apps = ingress.apps;
  if (Array.isArray(apps) && apps.length > 0 && hostPrefix) {
    const first = apps[0];
    if (first && typeof first === 'object' && !Array.isArray(first)) {
      (first as Record<string, unknown>).host = hostPrefix;
    }
  }

  if (!baseDomain || !hostPrefix) return;
  const fqdn = `${hostPrefix}.${baseDomain}`;

  const gitea = merged.gitea as Record<string, unknown> | undefined;
  if (!gitea || typeof gitea !== 'object') return;
  const giteaInner = gitea.gitea as Record<string, unknown> | undefined;
  if (!giteaInner || typeof giteaInner !== 'object') return;
  const config = giteaInner.config as Record<string, unknown> | undefined;
  if (!config || typeof config !== 'object') return;
  const server = (config.server as Record<string, unknown> | undefined) ?? {};
  server.DOMAIN = fqdn;
  server.ROOT_URL = `https://${fqdn}/`;
  server.SSH_DOMAIN = fqdn;
  config.server = server;
  giteaInner.config = config;
  gitea.gitea = giteaInner;
  merged.gitea = gitea;
}

export function assembleValues(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
  parseValue: (field: CatalogFieldDef, raw: string) => unknown,
): Record<string, unknown> {
  const base = parseValuesDefault(fields);
  const presetLayer = applyValuePresets(fields, fieldValues);
  const overlay = buildOverlayFromFields(fields, fieldValues, parseValue);
  const yamlText = fieldValues.values?.trim();
  const yamlLayer = yamlText ? parseValuesPayload(yamlText) : {};
  const merged = deepMerge(base, presetLayer, overlay, yamlLayer);
  finalizeFleetIngressValues(merged);
  return merged;
}

export function valuesToDisplayString(value: Record<string, unknown>): string {
  if (Object.keys(value).length === 0) return '';
  return stringifyYaml(value, { lineWidth: 0 });
}
