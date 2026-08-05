import type { CatalogFieldDef } from '../api/types';

export type UiDependsOnRule = {
  path: string;
  equals?: unknown;
  notEquals?: unknown;
  in?: unknown[];
};

export function uiDependsOnRule(field: CatalogFieldDef): UiDependsOnRule | undefined {
  const rule = field.validation_schema?.['x-ui-depends-on'];
  if (!rule || typeof rule !== 'object') return undefined;
  const r = rule as Record<string, unknown>;
  if (typeof r.path !== 'string') return undefined;
  return {
    path: r.path,
    equals: r.equals,
    notEquals: r.notEquals,
    in: Array.isArray(r.in) ? r.in : undefined,
  };
}

export function effectiveFieldValue(
  path: string,
  fieldValues: Record<string, string>,
  fields?: CatalogFieldDef[],
): string {
  const raw = fieldValues[path];
  if (raw !== undefined && raw !== '') return raw;
  const field = fields?.find((f) => f.path === path);
  if (field?.default !== undefined && field.default !== null && field.default !== '') {
    return String(field.default);
  }
  return '';
}

function valuesMatch(
  rule: UiDependsOnRule,
  fieldValues: Record<string, string>,
  fields?: CatalogFieldDef[],
): boolean {
  const current = effectiveFieldValue(rule.path, fieldValues, fields);
  if (rule.in !== undefined) {
    return rule.in.map(String).includes(current);
  }
  if (rule.notEquals !== undefined) {
    return current !== String(rule.notEquals);
  }
  if (rule.equals !== undefined) {
    return current === String(rule.equals);
  }
  return true;
}

export function isFieldVisible(
  field: CatalogFieldDef,
  fieldValues: Record<string, string>,
  fields?: CatalogFieldDef[],
): boolean {
  const rule = uiDependsOnRule(field);
  if (!rule) return true;
  return valuesMatch(rule, fieldValues, fields);
}

export function filterVisibleFields(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
): CatalogFieldDef[] {
  return fields.filter((f) => isFieldVisible(f, fieldValues, fields));
}

/**
 * Place fields that use x-ui-depends-on immediately after their controller field,
 * so checkbox/select toggles reveal related inputs right underneath.
 * Preserves original relative order among siblings and among roots.
 */
export function orderFieldsByDependency(fields: CatalogFieldDef[]): CatalogFieldDef[] {
  if (fields.length <= 1) return fields;

  const byPath = new Map(fields.map((f) => [f.path, f]));
  const children = new Map<string, CatalogFieldDef[]>();
  const roots: CatalogFieldDef[] = [];

  for (const field of fields) {
    const rule = uiDependsOnRule(field);
    const parentPath = rule?.path;
    if (parentPath && byPath.has(parentPath) && parentPath !== field.path) {
      const list = children.get(parentPath) ?? [];
      list.push(field);
      children.set(parentPath, list);
    } else {
      roots.push(field);
    }
  }

  const result: CatalogFieldDef[] = [];
  const emitted = new Set<string>();

  const emit = (field: CatalogFieldDef) => {
    if (emitted.has(field.path)) return;
    emitted.add(field.path);
    result.push(field);
    for (const child of children.get(field.path) ?? []) {
      emit(child);
    }
  };

  for (const field of roots) {
    emit(field);
  }
  // Orphans (parent missing from this list, e.g. filtered out) keep original order.
  for (const field of fields) {
    if (!emitted.has(field.path)) {
      result.push(field);
    }
  }
  return result;
}

/** Remove values for fields that became hidden after a parent change. */
export function pruneHiddenFieldValues(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
): Record<string, string> {
  const out = { ...fieldValues };
  for (const field of fields) {
    if (!isFieldVisible(field, out, fields)) {
      delete out[field.path];
    }
  }
  return out;
}
