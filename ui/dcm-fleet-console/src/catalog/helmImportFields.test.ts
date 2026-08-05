import { describe, expect, it } from 'vitest';
import {
  buildHelmImportCatalogItem,
  buildHelmImportFields,
  isDnsLabel,
  suggestCatalogId,
  validateOciRepository,
} from './helmImportFields';

describe('helmImportFields', () => {
  it('suggests catalog id from chart name', () => {
    expect(suggestCatalogId('nginx')).toBe('nginx-helm');
    expect(suggestCatalogId('My_Chart')).toBe('my-chart-helm');
    expect(suggestCatalogId('gitea-helm')).toBe('gitea-helm');
  });

  it('validates dns labels and oci repos', () => {
    expect(isDnsLabel('nginx-helm')).toBe(true);
    expect(isDnsLabel('Bad_ID')).toBe(false);
    expect(validateOciRepository('oci://quay.io/org')).toBeNull();
    expect(validateOciRepository('https://charts.example.com')).toMatch(/oci:\/\//);
  });

  it('builds pinned chart fields and routing fragment', () => {
    const fields = buildHelmImportFields({
      chartRepository: 'oci://registry-1.docker.io/bitnamicharts/',
      chartName: 'nginx',
      chartVersion: '15.0.0',
      valuesYaml: 'replicaCount: 1\n',
    });
    const byPath = Object.fromEntries(fields.map((f) => [f.path, f]));
    expect(byPath['chart.repository']?.default).toBe('oci://registry-1.docker.io/bitnamicharts');
    expect(byPath['chart.repository']?.editable).toBe(false);
    expect(byPath['chart.name']?.default).toBe('nginx');
    expect(byPath['chart.version']?.default).toBe('15.0.0');
    expect(byPath['values']?.default).toBe('replicaCount: 1');
    expect(byPath['routing.mode']?.validation_schema?.enum).toEqual(['hub', 'site', 'none']);
  });

  it('builds catalog create body with helm resource', () => {
    const body = buildHelmImportCatalogItem({
      catalogId: 'nginx-helm',
      displayName: 'Nginx',
      chartRepository: 'oci://quay.io/example',
      chartName: 'nginx',
      chartVersion: '1.2.3',
    });
    expect(body.id).toBe('nginx-helm');
    expect(body.service_type).toBe('helm');
    expect(body.spec?.resources?.[0]?.service_type).toBe('helm');
    expect((body.spec?.resources?.[0]?.fields || []).length).toBeGreaterThan(5);
  });
});
