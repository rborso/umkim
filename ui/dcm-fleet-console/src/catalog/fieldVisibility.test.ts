import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import { isFieldVisible, orderFieldsByDependency, pruneHiddenFieldValues } from './fieldVisibility';

describe('fieldVisibility', () => {
  const postgresField: CatalogFieldDef = {
    path: 'values.gitea.postgresql.auth.password',
    editable: true,
    validation_schema: {
      'x-ui-depends-on': { path: 'values.__preset.database', equals: 'postgres' },
    },
  };

  it('shows field when equals matches', () => {
    expect(
      isFieldVisible(postgresField, { 'values.__preset.database': 'postgres' }),
    ).toBe(true);
    expect(
      isFieldVisible(postgresField, { 'values.__preset.database': 'sqlite3' }),
    ).toBe(false);
  });

  it('shows field when in matches', () => {
    const authField: CatalogFieldDef = {
      path: 'routing.require_auth',
      editable: true,
      validation_schema: {
        'x-ui-depends-on': { path: 'routing.mode', in: ['hub', 'site'] },
      },
    };
    expect(isFieldVisible(authField, { 'routing.mode': 'hub' })).toBe(true);
    expect(isFieldVisible(authField, { 'routing.mode': 'dual' })).toBe(false);
  });

  it('hides postgres fields when preset default is sqlite3 and form state is empty', () => {
    const presetField: CatalogFieldDef = {
      path: 'values.__preset.database',
      editable: true,
      default: 'sqlite3',
      validation_schema: { enum: ['sqlite3', 'postgres'] },
    };
    const postgresField: CatalogFieldDef = {
      path: 'values.gitea.postgresql.auth.database',
      editable: true,
      validation_schema: {
        'x-ui-depends-on': { path: 'values.__preset.database', equals: 'postgres' },
      },
    };
    const all = [presetField, postgresField];
    expect(isFieldVisible(postgresField, {}, all)).toBe(false);
    expect(isFieldVisible(postgresField, { 'values.__preset.database': 'postgres' }, all)).toBe(
      true,
    );
  });

  it('prunes hidden field values', () => {
    const fields = [postgresField];
    const pruned = pruneHiddenFieldValues(fields, {
      'values.__preset.database': 'sqlite3',
      'values.gitea.postgresql.auth.password': 'secret',
    });
    expect(pruned['values.gitea.postgresql.auth.password']).toBeUndefined();
  });

  it('orders dependents immediately under their controller', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'values.bootstrap.admin.email', editable: true, validation_schema: { 'x-ui-depends-on': { path: 'values.bootstrap.enabled', equals: true } } },
      { path: 'routing.mode', editable: true },
      { path: 'values.bootstrap.enabled', editable: true },
      { path: 'values.bootstrap.admin.password', editable: true, validation_schema: { 'x-ui-depends-on': { path: 'values.bootstrap.enabled', equals: true } } },
      { path: 'routing.require_auth', editable: true, validation_schema: { 'x-ui-depends-on': { path: 'routing.mode', in: ['hub', 'site'] } } },
    ];
    expect(orderFieldsByDependency(fields).map((f) => f.path)).toEqual([
      'routing.mode',
      'routing.require_auth',
      'values.bootstrap.enabled',
      'values.bootstrap.admin.email',
      'values.bootstrap.admin.password',
    ]);
  });
});
