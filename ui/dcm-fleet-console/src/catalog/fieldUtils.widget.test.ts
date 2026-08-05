import { describe, expect, it } from 'vitest';
import { resolveFieldWidget } from './fieldUtils';
import type { CatalogFieldDef } from '../api/types';

describe('resolveFieldWidget', () => {
  it('returns select when validation_schema.enum is set', () => {
    const field: CatalogFieldDef = {
      path: 'routing.mode',
      validation_schema: { type: 'string', enum: ['hub', 'site', 'none'] },
    };
    expect(resolveFieldWidget(field)).toBe('select');
  });

  it('keeps explicit yaml widget over enum inference', () => {
    const field: CatalogFieldDef = {
      path: 'values',
      ui_widget: 'yaml',
      validation_schema: { type: 'string', enum: ['a'] },
    };
    expect(resolveFieldWidget(field)).toBe('yaml');
  });

  it('keeps dns-label widget when set explicitly', () => {
    const field: CatalogFieldDef = {
      path: 'metadata.name',
      validation_schema: {
        type: 'string',
        'x-ui-widget': 'dns-label',
        enum: ['a', 'b'],
      },
    };
    expect(resolveFieldWidget(field)).toBe('dns-label');
  });

  it('returns cpu-quantity for k8s-cpu-quantity schema', () => {
    const field: CatalogFieldDef = {
      path: 'resources.cpu.min',
      validation_schema: {
        type: 'string',
        format: 'k8s-cpu-quantity',
        'x-ui-widget': 'cpu-quantity',
      },
    };
    expect(resolveFieldWidget(field)).toBe('cpu-quantity');
  });

  it('returns number for legacy integer CPU schema', () => {
    const field: CatalogFieldDef = {
      path: 'resources.cpu.min',
      validation_schema: { type: 'integer', minimum: 1, 'x-ui-widget': 'number' },
    };
    expect(resolveFieldWidget(field)).toBe('number');
  });
});
