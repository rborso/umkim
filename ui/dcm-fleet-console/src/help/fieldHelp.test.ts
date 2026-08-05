import { describe, expect, it } from 'vitest';
import { getFieldHelp, resolveCatalogFieldHelp } from './fieldHelp';
import type { CatalogFieldDef } from '../api/types';

describe('getFieldHelp', () => {
  it('returns catalog routing.cluster help', () => {
    const help = getFieldHelp('routing.cluster');
    expect(help?.description).toMatch(/Optional/i);
    expect(help?.example).toBe('spoke-a');
  });

  it('returns policy rego help', () => {
    const help = getFieldHelp('policy.regoCode');
    expect(help?.description).toMatch(/Rego/i);
    expect(help?.whenToUse).toBeTruthy();
  });

  it('returns policy strategy help with related fields', () => {
    const help = getFieldHelp('policy.strategy');
    expect(help?.relatedFields).toContain('policy.clusterCount');
  });

  it('returns undefined for unknown keys', () => {
    expect(getFieldHelp('unknown.field')).toBeUndefined();
  });
});

describe('resolveCatalogFieldHelp', () => {
  it('prefers API description over registry', () => {
    const field: CatalogFieldDef = {
      path: 'routing.cluster',
      description: 'Custom cluster from catalog seed',
      example: 'virt01',
    };
    const help = resolveCatalogFieldHelp(field);
    expect(help?.description).toBe('Custom cluster from catalog seed');
    expect(help?.example).toBe('virt01');
  });

  it('falls back to registry by path', () => {
    const field: CatalogFieldDef = { path: 'routing.mode' };
    const help = resolveCatalogFieldHelp(field);
    expect(help?.description).toMatch(/fleet edge/i);
  });
});
