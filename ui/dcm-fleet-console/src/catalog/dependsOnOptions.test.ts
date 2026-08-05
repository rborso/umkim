import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import { filteredEnumOptions, coerceEnumSelection } from './dependsOnOptions';

describe('dependsOnOptions', () => {
  const versionField: CatalogFieldDef = {
    path: 'database.version',
    editable: true,
    default: '18',
    validation_schema: { type: 'string', enum: ['18', '17', '16', '8.4', '8.3'] },
    depends_on: {
      path: 'database.engine',
      allowed_values: {
        postgres: ['18', '17', '16'],
        mysql: ['8.4', '8.3', '8'],
      },
    },
  };

  it('filters enum by depends_on source', () => {
    expect(
      filteredEnumOptions(versionField, { 'database.engine': 'postgres' })?.map(String),
    ).toEqual(['18', '17', '16']);
    expect(
      filteredEnumOptions(versionField, { 'database.engine': 'mysql' })?.map(String),
    ).toEqual(['8.4', '8.3', '8']);
  });

  it('coerces invalid selection when parent changes', () => {
    expect(
      coerceEnumSelection(versionField, '8.4', { 'database.engine': 'postgres' }),
    ).toBe('18');
  });
});
