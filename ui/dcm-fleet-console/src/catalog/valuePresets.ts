import type { CatalogFieldDef } from '../api/types';
import { setByDotPath } from './valuesMerge';

export const VIRTUAL_PRESET_PREFIX = 'values.__preset.';

export function isVirtualPresetPath(path: string): boolean {
  return path.startsWith(VIRTUAL_PRESET_PREFIX);
}

/** True for values.* overlay paths written into the Helm values blob (excludes virtual presets). */
export function isHelmValuesOverlayPath(path: string): boolean {
  return path.startsWith('values.') && !isVirtualPresetPath(path);
}

export function valuePresetMap(
  field: CatalogFieldDef,
): Record<string, Record<string, unknown>> | undefined {
  const presets = field.validation_schema?.['x-value-preset'];
  if (!presets || typeof presets !== 'object' || Array.isArray(presets)) return undefined;
  return presets as Record<string, Record<string, unknown>>;
}

export function instancePresetMap(
  field: CatalogFieldDef,
): Record<string, Record<string, string>> | undefined {
  const presets = field.validation_schema?.['x-instance-preset'];
  if (!presets || typeof presets !== 'object' || Array.isArray(presets)) return undefined;
  return presets as Record<string, Record<string, string>>;
}

export function applyInstancePresets(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
): Record<string, string> {
  const out = { ...fieldValues };
  for (const field of fields) {
    if (!isVirtualPresetPath(field.path)) continue;
    const presets = instancePresetMap(field);
    if (!presets) continue;
    let selected = out[field.path];
    if (selected === undefined || selected === '') {
      if (field.default !== undefined && field.default !== null) {
        selected = String(field.default);
      } else {
        continue;
      }
    }
    const preset = presets[selected];
    if (!preset) continue;
    for (const [path, value] of Object.entries(preset)) {
      out[path] = value;
    }
  }
  return out;
}

export function isValuePresetField(field: CatalogFieldDef): boolean {
  return isVirtualPresetPath(field.path) && valuePresetMap(field) !== undefined;
}

export function applyValuePresets(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
): Record<string, unknown> {
  const layer: Record<string, unknown> = {};
  for (const field of fields) {
    if (!isValuePresetField(field)) continue;
    const presets = valuePresetMap(field);
    if (!presets) continue;
    let selected = fieldValues[field.path];
    if (selected === undefined || selected === '') {
      if (field.default !== undefined && field.default !== null) {
        selected = String(field.default);
      } else {
        continue;
      }
    }
    const preset = presets[selected];
    if (!preset) continue;
    for (const [dotPath, value] of Object.entries(preset)) {
      setByDotPath(layer, dotPath, value);
    }
  }
  return layer;
}
