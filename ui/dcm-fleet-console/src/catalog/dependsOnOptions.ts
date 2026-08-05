import type { CatalogFieldDef } from '../api/types';

export function dependsOnRule(
  field: CatalogFieldDef,
): { path: string; allowed_values: Record<string, unknown[]> } | undefined {
  const dep = field.depends_on;
  if (!dep || typeof dep !== 'object') return undefined;
  if (typeof dep.path !== 'string' || !dep.allowed_values) return undefined;
  return dep;
}

export function filteredEnumOptions(
  field: CatalogFieldDef,
  fieldValues: Record<string, string>,
): unknown[] | undefined {
  const baseEnum = field.validation_schema?.enum;
  if (!baseEnum || baseEnum.length === 0) return baseEnum;

  const dep = dependsOnRule(field);
  if (!dep) return baseEnum;

  const source = fieldValues[dep.path];
  const sourceKey =
    source !== undefined && source !== ''
      ? source
      : fieldValues[dep.path] ?? '';
  if (sourceKey === '') return baseEnum;

  const allowed = dep.allowed_values[String(sourceKey)];
  if (!allowed || allowed.length === 0) return baseEnum;
  return allowed;
}

export function coerceEnumSelection(
  field: CatalogFieldDef,
  current: string,
  fieldValues: Record<string, string>,
): string {
  const options = filteredEnumOptions(field, fieldValues);
  if (!options || options.length === 0) return current;
  const allowed = options.map(String);
  if (current !== '' && allowed.includes(current)) return current;
  if (field.default !== undefined && field.default !== null && allowed.includes(String(field.default))) {
    return String(field.default);
  }
  return allowed[0] ?? current;
}
