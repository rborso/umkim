import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import {
  augmentDeployFields,
  deployHasSiteDomainField,
  deploySupportsDomainRouting,
  FLEET_CHART_INGRESS_DOMAIN_FIELDS,
  FLEET_DOMAIN_OVERRIDE_FIELDS,
} from './fleetEdgeRoutingFields';

const minimalHelmCatalog: CatalogFieldDef[] = [
  { path: 'metadata.name', editable: true, required: true },
  { path: 'release.namespace', editable: true, default: 'default' },
];

describe('fleetEdgeRoutingFields', () => {
  it('augments helm deploy fields with routing mode and domain overrides', () => {
    const fields = augmentDeployFields('helm', minimalHelmCatalog);
    const paths = fields.map((f) => f.path);
    expect(paths).toContain('routing.mode');
    for (const field of FLEET_DOMAIN_OVERRIDE_FIELDS) {
      expect(paths).toContain(field.path);
    }
    for (const field of FLEET_CHART_INGRESS_DOMAIN_FIELDS) {
      expect(paths).toContain(field.path);
    }
  });

  it('augments container deploy fields with hub/site host overrides only', () => {
    const fields = augmentDeployFields('container', minimalHelmCatalog);
    const paths = fields.map((f) => f.path);
    expect(paths).toContain('routing.hub_host');
    expect(paths).toContain('routing.site_host');
    expect(paths).not.toContain('values.fleet.ingress.baseDomain');
  });

  it('enriches existing catalog fields with canonical depends-on rules', () => {
    const existing: CatalogFieldDef[] = [
      {
        path: 'routing.hub_host',
        display_name: 'Custom hub label',
        editable: true,
        validation_schema: {
          type: 'string',
          'x-ui-depends-on': { path: 'values.__preset.exposure', equals: 'hub' },
        },
      },
    ];
    const fields = augmentDeployFields('helm', existing);
    const hub = fields.find((f) => f.path === 'routing.hub_host');
    expect(hub?.display_name).toBe('Custom hub label');
    expect(hub?.validation_schema?.['x-ui-depends-on']).toEqual({
      path: 'routing.mode',
      in: ['hub'],
    });
  });

  it('strips dual from catalog routing.mode enums', () => {
    const withDual: CatalogFieldDef[] = [
      {
        path: 'routing.mode',
        editable: true,
        validation_schema: { type: 'string', enum: ['hub', 'site', 'dual', 'none'] },
      },
    ];
    const fields = augmentDeployFields('helm', withDual);
    const mode = fields.find((f) => f.path === 'routing.mode');
    expect(mode?.validation_schema?.enum).toEqual(['hub', 'site', 'none']);
  });

  it('does not inject routing.mode when exposure preset is present', () => {
    const withExposure: CatalogFieldDef[] = [
      { path: 'values.__preset.exposure', editable: true, default: 'chart_ingress' },
    ];
    const fields = augmentDeployFields('helm', withExposure);
    expect(fields.filter((f) => f.path === 'routing.mode')).toHaveLength(0);
  });

  it('reports domain routing support for helm and container only', () => {
    expect(deploySupportsDomainRouting('helm')).toBe(true);
    expect(deploySupportsDomainRouting('container')).toBe(true);
    expect(deploySupportsDomainRouting('operator')).toBe(false);
    expect(deploySupportsDomainRouting('vm')).toBe(false);
  });

  it('detects chart ingress base domain field after augment', () => {
    const fields = augmentDeployFields('helm', minimalHelmCatalog);
    expect(deployHasSiteDomainField(fields)).toBe(true);
    expect(deployHasSiteDomainField(augmentDeployFields('container', minimalHelmCatalog))).toBe(
      false,
    );
  });
});
