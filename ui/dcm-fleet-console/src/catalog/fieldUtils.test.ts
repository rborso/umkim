import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import {
  buildUserValues,
  defaultFieldValues,
  deployFieldsForServiceType,
  partitionByTier,
  resolveFieldTier,
} from './fieldUtils';

describe('fieldUtils tiers', () => {
  it('reads x-ui-tier from validation_schema', () => {
    expect(
      resolveFieldTier({
        path: 'install.timeout',
        validation_schema: { 'x-ui-tier': 'advanced' },
      }),
    ).toBe('advanced');
  });

  it('infers tier from required and primary paths', () => {
    expect(resolveFieldTier({ path: 'metadata.name', required: true, editable: true })).toBe('required');
    expect(resolveFieldTier({ path: 'routing.mode', editable: true })).toBe('common');
    expect(resolveFieldTier({ path: 'install.timeout', editable: true })).toBe('advanced');
    expect(resolveFieldTier({ path: 'chart.version', editable: false })).toBe('hidden');
  });

  it('partitions fields by ui_tier', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', required: true, ui_tier: 'required', editable: true },
      { path: 'routing.mode', ui_tier: 'common', editable: true },
      { path: 'install.timeout', ui_tier: 'advanced', editable: true },
      { path: 'chart.name', editable: false },
    ];
    const parts = partitionByTier(fields);
    expect(parts.required.map((f) => f.path)).toEqual(['metadata.name']);
    expect(parts.common.map((f) => f.path)).toEqual(['routing.mode']);
    expect(parts.advanced.map((f) => f.path)).toEqual(['install.timeout']);
    expect(parts.readonly.map((f) => f.path)).toEqual(['chart.name']);
  });

  it('keeps checkbox dependents in the same tier as the checkbox', () => {
    const fields: CatalogFieldDef[] = [
      {
        path: 'values.bootstrap.enabled',
        editable: true,
        validation_schema: { 'x-ui-tier': 'common', 'x-ui-widget': 'boolean' },
      },
      {
        path: 'values.bootstrap.admin.password',
        editable: true,
        validation_schema: {
          'x-ui-tier': 'required',
          'x-ui-depends-on': { path: 'values.bootstrap.enabled', equals: true },
        },
      },
    ];
    const parts = partitionByTier(fields);
    expect(parts.required.map((f) => f.path)).toEqual([]);
    expect(parts.common.map((f) => f.path)).toEqual([
      'values.bootstrap.enabled',
      'values.bootstrap.admin.password',
    ]);
  });

  it('seeds overlay defaults from values blob', () => {
    const fields: CatalogFieldDef[] = [
      {
        path: 'values',
        default: 'fleet:\n  ingress:\n    baseDomain: seeded.example\n',
      },
      { path: 'values.fleet.ingress.baseDomain', editable: true },
    ];
    const defaults = defaultFieldValues(fields);
    expect(defaults['values.fleet.ingress.baseDomain']).toBe('seeded.example');
  });

  it('does not pre-fill values yaml textarea from catalog default', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'values', default: 'key: from-default\n' },
      { path: 'values.fleet.ingress.baseDomain', editable: true },
    ];
    const defaults = defaultFieldValues(fields);
    expect(defaults.values).toBeUndefined();
    expect(defaults['values.fleet.ingress.baseDomain']).toBeUndefined();
  });

  it('serializes array/object defaults and parses them on submit', () => {
    const fields: CatalogFieldDef[] = [
      {
        path: 'network.ports',
        editable: true,
        default: [{ container_port: 80, visibility: 'internal' }],
        validation_schema: { type: 'array' },
      },
    ];
    const defaults = defaultFieldValues(fields);
    expect(defaults['network.ports']).toBe(
      JSON.stringify([{ container_port: 80, visibility: 'internal' }]),
    );
    const uv = buildUserValues(fields, defaults);
    expect(uv).toEqual([
      {
        path: 'network.ports',
        value: [{ container_port: 80, visibility: 'internal' }],
      },
    ]);
  });

  it('buildUserValues omits cleared routing.cluster instead of applying catalog default', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true, default: 'gitea-test' },
      { path: 'routing.cluster', editable: true, default: 'lab-ext02' },
      { path: 'routing.mode', editable: true, default: 'hub' },
    ];
    const uv = buildUserValues(fields, {
      'metadata.name': 'gitea-test',
      'routing.cluster': '',
      'routing.mode': 'hub',
    });
    expect(uv.map((u) => u.path)).not.toContain('routing.cluster');
  });

  it('buildUserValues omits template-pinned chart fields when editable is unset', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true },
      { path: 'chart.repository', default: 'oci://10.10.0.1:30443/charts' },
      { path: 'chart.name', default: 'whoami' },
      { path: 'chart.version', default: '0.1.0' },
      { path: 'release.namespace', editable: true, default: 'default' },
      { path: 'values.replicaCount', editable: true, default: 1 },
      { path: 'values', editable: true, validation_schema: { type: 'string', format: 'yaml' } },
      { path: 'routing.mode', editable: true, default: 'hub' },
    ];
    const uv = buildUserValues(fields, {
      'metadata.name': 'demo-whoami-helm',
      'release.namespace': 'demo-whoami-helm',
    });
    const paths = uv.map((u) => u.path);
    expect(paths).not.toContain('chart.repository');
    expect(paths).not.toContain('chart.name');
    expect(paths).not.toContain('chart.version');
    expect(paths).toContain('metadata.name');
    expect(paths).toContain('values');
  });

  it('buildUserValues collapses values.replicaCount overlay for whoami-helm', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true },
      { path: 'values.replicaCount', editable: true, default: 1 },
      { path: 'values', editable: true, validation_schema: { type: 'string', format: 'yaml' } },
    ];
    const uv = buildUserValues(fields, { 'metadata.name': 'helm-demo' });
    const valuesEntry = uv.find((u) => u.path === 'values');
    expect(valuesEntry?.value).toBe('replicaCount: 1\n');
    expect(uv.some((u) => u.path === 'values.replicaCount')).toBe(false);
  });

  it('buildUserValues collapses overlays into values entry', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true, default: 'rel-1' },
      {
        path: 'values',
        editable: true,
        default: 'fleet:\n  ingress:\n    baseDomain: base.example\nhidden: true\n',
      },
      { path: 'values.fleet.ingress.baseDomain', editable: true },
    ];
    const values = {
      'metadata.name': 'my-release',
      'values.fleet.ingress.baseDomain': 'custom.example',
    };
    const uv = buildUserValues(fields, values);
    const names = uv.map((u) => u.path);
    expect(names).toContain('metadata.name');
    expect(names).toContain('values');
    expect(names).not.toContain('values.fleet.ingress.baseDomain');
    const valuesEntry = uv.find((u) => u.path === 'values');
    expect(typeof valuesEntry?.value).toBe('string');
    expect(valuesEntry?.value).toContain('baseDomain: custom.example');
  });

  it('serializes values as YAML string for helm catalog (object default, string schema)', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true, default: 'gitea-test01' },
      {
        path: 'values',
        editable: true,
        default: { gitea: { sqlite: { enabled: true } } },
        validation_schema: { type: 'string', format: 'yaml', 'x-ui-widget': 'yaml' },
      },
      {
        path: 'values.__preset.database',
        editable: true,
        default: 'sqlite3',
        validation_schema: {
          type: 'string',
          enum: ['sqlite3', 'postgres'],
          'x-value-preset': {
            sqlite3: { 'gitea.sqlite.enabled': true, 'gitea.postgresql.enabled': false },
          },
        },
      },
    ];
    const uv = buildUserValues(fields, {
      'metadata.name': 'gitea-test01',
      'values.__preset.database': 'sqlite3',
      'values.gitea.gitea.admin.password': 'SecretPass1234',
    });
    const valuesEntry = uv.find((u) => u.path === 'values');
    expect(typeof valuesEntry?.value).toBe('string');
    expect(valuesEntry?.value).toContain('sqlite:');
    expect(valuesEntry?.value).toContain('enabled: true');
  });

  it('injects domain override fields for helm catalog items without routing fragment', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true, required: true },
      { path: 'release.namespace', editable: true, default: 'default' },
    ];
    const deploy = deployFieldsForServiceType('helm', fields);
    const paths = deploy.map((f) => f.path);
    expect(paths).toContain('routing.mode');
    expect(paths).toContain('routing.hub_host');
    expect(paths).toContain('values.fleet.ingress.baseDomain');
  });
});
