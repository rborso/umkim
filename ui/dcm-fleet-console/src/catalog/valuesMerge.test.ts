import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import {
  assembleValues,
  deepMerge,
  getByDotPath,
  setByDotPath,
} from './valuesMerge';

describe('valuesMerge', () => {
  it('deep merges nested objects with later layers winning', () => {
    const base = { fleet: { ingress: { baseDomain: 'a.example' } }, port: 80 };
    const overlay = { fleet: { ingress: { baseDomain: 'b.example' } } };
    const yamlLayer = { port: 443 };
    const merged = deepMerge(base, overlay, yamlLayer);
    expect(merged).toEqual({
      fleet: { ingress: { baseDomain: 'b.example' } },
      port: 443,
    });
  });

  it('preserves hidden keys from base when overlay patches a branch', () => {
    const base = {
      whoami: { service: { port: 80 } },
      fleet: { ingress: { baseDomain: 'apps.umkim.com' } },
    };
    const overlay = { fleet: { ingress: { baseDomain: 'custom.dev' } } };
    const merged = deepMerge(base, overlay);
    expect(getByDotPath(merged, 'whoami.service.port')).toBe(80);
    expect(getByDotPath(merged, 'fleet.ingress.baseDomain')).toBe('custom.dev');
  });

  it('assembles values from defaults, overlays, and YAML layer', () => {
    const fields: CatalogFieldDef[] = [
      {
        path: 'values',
        default: 'fleet:\n  ingress:\n    baseDomain: default.example\nwhoami:\n  replicaCount: 1\n',
      },
      {
        path: 'values.fleet.ingress.baseDomain',
        default: 'overlay.example',
      },
    ];
    const fieldValues = {
      'values.fleet.ingress.baseDomain': 'user.example',
      values: 'fleet:\n  ingress:\n    baseDomain: yaml-wins.example\n',
    };
    const merged = assembleValues(fields, fieldValues, (_f, raw) => raw);
    expect(getByDotPath(merged, 'fleet.ingress.baseDomain')).toBe('yaml-wins.example');
    expect(getByDotPath(merged, 'whoami.replicaCount')).toBe(1);
  });

  it('setByDotPath and getByDotPath round-trip', () => {
    const obj: Record<string, unknown> = {};
    setByDotPath(obj, 'bootstrap.secret.stringData.PASS', 'secret');
    expect(getByDotPath(obj, 'bootstrap.secret.stringData.PASS')).toBe('secret');
  });

  it('finalizes fleet ingress host and gitea server domain', () => {
    const merged = assembleValues(
      [
        {
          path: 'values',
          editable: true,
          default: {
            fleet: {
              ingress: {
                enabled: true,
                apps: [{ id: 'gitea', host: 'old', service: 'gitea-http', port: 3000 }],
              },
            },
            gitea: { gitea: { config: { server: {} } } },
          },
        },
        { path: 'values.fleet.ingress.host', editable: true },
        { path: 'values.fleet.ingress.baseDomain', editable: true },
      ],
      {
        'values.fleet.ingress.host': 'git',
        'values.fleet.ingress.baseDomain': 'site.example.com',
      },
      (_field, raw) => raw,
    );
    const ingress = (merged.fleet as Record<string, unknown>).ingress as Record<string, unknown>;
    expect((ingress.apps as Array<Record<string, unknown>>)[0].host).toBe('git');
    const server = (
      ((merged.gitea as Record<string, unknown>).gitea as Record<string, unknown>).config as Record<
        string,
        unknown
      >
    ).server as Record<string, string>;
    expect(server.DOMAIN).toBe('git.site.example.com');
  });
});
