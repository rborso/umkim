import type { CatalogFieldDef } from '../api/types';
import { effectiveFieldValue, isFieldVisible } from './fieldVisibility';
import { isValuesOverlayPath, setByDotPath } from './valuesMerge';

export type SecretBindSpec = {
  secretName: string;
  secretKey: string;
  /** Extra secret keys sourced from non-sensitive catalog field paths. */
  secretDataFrom?: Record<string, string>;
  valuesSet?: Record<string, string>;
  valuesClear?: string[];
};

export const REDACTED_VALUE = '••••••';

export function isSensitiveField(field: CatalogFieldDef): boolean {
  if (field.validation_schema?.['x-sensitive'] === true) return true;
  if (field.path.includes('PASSWORD') || field.path.toLowerCase().includes('password')) return true;
  return false;
}

export function secretBindSpec(field: CatalogFieldDef): SecretBindSpec | undefined {
  const bind = field.validation_schema?.['x-secret-bind'];
  if (!bind || typeof bind !== 'object' || Array.isArray(bind)) return undefined;
  const b = bind as Record<string, unknown>;
  if (typeof b.secretName !== 'string' || typeof b.secretKey !== 'string') return undefined;
  const valuesSet =
    b.valuesSet && typeof b.valuesSet === 'object' && !Array.isArray(b.valuesSet)
      ? (b.valuesSet as Record<string, string>)
      : undefined;
  const valuesClear = Array.isArray(b.valuesClear)
    ? b.valuesClear.map(String)
    : undefined;
  const secretDataFrom =
    b.secretDataFrom && typeof b.secretDataFrom === 'object' && !Array.isArray(b.secretDataFrom)
      ? Object.fromEntries(
          Object.entries(b.secretDataFrom as Record<string, unknown>).map(([k, v]) => [
            k,
            String(v),
          ]),
        )
      : undefined;
  return {
    secretName: b.secretName,
    secretKey: b.secretKey,
    secretDataFrom,
    valuesSet,
    valuesClear,
  };
}

export function resolveTemplate(
  template: string,
  vars: Record<string, string>,
): string {
  let out = template;
  for (const [key, val] of Object.entries(vars)) {
    out = out.replaceAll(`{{${key}}}`, val);
  }
  return out;
}

export type CatalogInstanceSecretRequest = {
  cluster: string;
  namespace: string;
  secretName: string;
  data: Record<string, string>;
};

export function collectSecretRequests(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
  releaseName: string,
  cluster: string,
  namespace: string,
): CatalogInstanceSecretRequest[] {
  const vars = { release: releaseName, secretName: '' };
  const byName = new Map<string, CatalogInstanceSecretRequest>();

  for (const field of fields) {
    if (!isSensitiveField(field)) continue;
    if (!isFieldVisible(field, fieldValues, fields)) continue;
    const bind = secretBindSpec(field);
    if (!bind) continue;
    const raw = fieldValues[field.path];
    if (!raw || !raw.trim()) continue;

    const secretName = resolveTemplate(bind.secretName, { release: releaseName });
    vars.secretName = secretName;
    const existing = byName.get(secretName) ?? {
      cluster,
      namespace,
      secretName,
      data: {},
    };
    existing.data[bind.secretKey] = raw.trim();
    if (bind.secretDataFrom) {
      for (const [secretKey, fieldPath] of Object.entries(bind.secretDataFrom)) {
        const extra = effectiveFieldValue(fieldPath, fieldValues, fields);
        if (extra.trim()) {
          existing.data[secretKey] = extra.trim();
        }
      }
    }
    byName.set(secretName, existing);
  }
  return [...byName.values()];
}

export function applySecretBindsToValues(
  fields: CatalogFieldDef[],
  values: Record<string, unknown>,
  releaseName: string,
  fieldValues: Record<string, string>,
): Record<string, unknown> {
  const out = JSON.parse(JSON.stringify(values)) as Record<string, unknown>;
  const vars = { release: releaseName, secretName: '' };

  for (const field of fields) {
    const bind = secretBindSpec(field);
    if (!bind) continue;
    if (!isFieldVisible(field, fieldValues, fields)) continue;
    const raw = fieldValues[field.path];
    if (!raw || !raw.trim()) continue;
    const secretName = resolveTemplate(bind.secretName, { release: releaseName });
    vars.secretName = secretName;
    if (bind.valuesSet) {
      for (const [dotPath, template] of Object.entries(bind.valuesSet)) {
        setByDotPath(out, dotPath, resolveTemplate(template, vars));
      }
    }
    if (bind.valuesClear) {
      for (const dotPath of bind.valuesClear) {
        clearByDotPath(out, dotPath);
      }
    }
  }
  return out;
}

function clearByDotPath(obj: Record<string, unknown>, dotPath: string): void {
  const parts = dotPath.split('.');
  let cur: Record<string, unknown> = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    const next = cur[parts[i]];
    if (next === null || next === undefined || typeof next !== 'object' || Array.isArray(next)) {
      return;
    }
    cur = next as Record<string, unknown>;
  }
  delete cur[parts[parts.length - 1]];
}

export function stripSensitiveOverlayPaths(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
): Record<string, string> {
  const out = { ...fieldValues };
  for (const field of fields) {
    if (!isSensitiveField(field) || !isValuesOverlayPath(field.path)) continue;
    delete out[field.path];
  }
  return out;
}

export function redactDisplayValue(path: string, value: unknown, fields: CatalogFieldDef[]): unknown {
  const field = fields.find((f) => f.path === path);
  if (field && isSensitiveField(field)) return REDACTED_VALUE;
  if (path === 'values' && typeof value === 'object' && value !== null) {
    return redactValuesObject(value as Record<string, unknown>, fields);
  }
  return value;
}

function redactValuesObject(
  obj: Record<string, unknown>,
  fields: CatalogFieldDef[],
): Record<string, unknown> {
  const sensitiveSubpaths = new Set<string>();
  for (const field of fields) {
    if (!isSensitiveField(field) || !field.path.startsWith('values.')) continue;
    sensitiveSubpaths.add(field.path.slice('values.'.length));
  }
  const out = JSON.parse(JSON.stringify(obj)) as Record<string, unknown>;
  for (const sub of sensitiveSubpaths) {
    clearByDotPath(out, sub);
    // Mark parent path redacted if we cleared a leaf
    const parts = sub.split('.');
    let cur: Record<string, unknown> = out;
    for (let i = 0; i < parts.length - 1; i++) {
      const next = cur[parts[i]];
      if (next === null || next === undefined || typeof next !== 'object') break;
      cur = next as Record<string, unknown>;
    }
    if (parts.length > 0) {
      const leaf = parts[parts.length - 1];
      if (cur[leaf] === undefined && sensitiveSubpaths.has(sub)) {
        cur[leaf] = REDACTED_VALUE;
      }
    }
  }
  // Also redact common password key names anywhere in tree
  redactPasswordKeys(out);
  return out;
}

function redactPasswordKeys(obj: Record<string, unknown>): void {
  for (const [key, val] of Object.entries(obj)) {
    if (key.toLowerCase().includes('password') && typeof val === 'string') {
      obj[key] = REDACTED_VALUE;
    } else if (val !== null && typeof val === 'object' && !Array.isArray(val)) {
      redactPasswordKeys(val as Record<string, unknown>);
    }
  }
}

export function redactUserValues(
  userValues: Array<{ path: string; value: unknown }>,
  fields: CatalogFieldDef[],
): Array<{ path: string; value: unknown }> {
  return userValues.map((uv) => ({
    path: uv.path,
    value: redactDisplayValue(uv.path, uv.value, fields),
  }));
}
