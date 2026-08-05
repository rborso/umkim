import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import { applyValuePresets } from './valuePresets';
import { assembleValues } from './valuesMerge';
import { parseFieldValue } from './fieldUtils';

describe('valuePresets', () => {
  const presetField: CatalogFieldDef = {
    path: 'values.__preset.database',
    editable: true,
    default: 'sqlite3',
    validation_schema: {
      type: 'string',
      enum: ['sqlite3', 'postgres'],
      'x-value-preset': {
        sqlite3: {
          'gitea.postgresql.enabled': false,
          'gitea.sqlite.enabled': true,
          'gitea.gitea.config.database.DB_TYPE': 'sqlite3',
        },
        postgres: {
          'gitea.postgresql.enabled': true,
          'gitea.sqlite.enabled': false,
          'gitea.gitea.config.database.DB_TYPE': 'postgres',
        },
      },
    },
  };

  it('applies sqlite preset keys', () => {
    const layer = applyValuePresets([presetField], { 'values.__preset.database': 'sqlite3' });
    expect(layer).toEqual({
      gitea: {
        postgresql: { enabled: false },
        sqlite: { enabled: true },
        gitea: { config: { database: { DB_TYPE: 'sqlite3' } } },
      },
    });
  });

  it('merges presets into assembled values', () => {
    const fields: CatalogFieldDef[] = [
      {
        path: 'values',
        default: { gitea: { persistence: { size: '5Gi' } } },
      },
      presetField,
    ];
    const merged = assembleValues(
      fields,
      { 'values.__preset.database': 'postgres' },
      parseFieldValue,
    );
    const gitea = merged.gitea as Record<string, unknown>;
    expect((gitea.postgresql as Record<string, unknown>).enabled).toBe(true);
    expect((gitea.sqlite as Record<string, unknown>).enabled).toBe(false);
    expect(
      ((gitea.gitea as Record<string, unknown>).config as Record<string, unknown>).database,
    ).toEqual({ DB_TYPE: 'postgres' });
  });
});
