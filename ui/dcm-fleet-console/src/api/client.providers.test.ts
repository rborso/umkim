import { describe, expect, it } from 'vitest';
import { providerCluster, providerClusterFromName, providerSite } from './client';
import type { FleetCluster, FleetSite, Provider } from './types';

function provider(name: string, metadata?: Record<string, string>): Provider {
  return { name, metadata };
}

describe('providerClusterFromName', () => {
  it('parses fleet provider name suffixes', () => {
    expect(providerClusterFromName('kubevirt-lab-ext02')).toBe('lab-ext02');
    expect(providerClusterFromName('k8s-lab-gpu03')).toBe('lab-gpu03');
    expect(providerClusterFromName('helm-lab-int01')).toBe('lab-int01');
  });

  it('returns undefined for unrelated names', () => {
    expect(providerClusterFromName('custom-provider')).toBeUndefined();
  });
});

describe('providerCluster', () => {
  it('prefers metadata.zone over name suffix', () => {
    expect(providerCluster(provider('kubevirt-lab-ext02', { zone: 'lab-int01' }))).toBe('lab-int01');
  });

  it('falls back to provider name suffix', () => {
    expect(providerCluster(provider('kubevirt-lab-ext02'))).toBe('lab-ext02');
  });
});

describe('providerSite', () => {
  const clusters: FleetCluster[] = [{ name: 'lab-ext02', site: 'homelab-ext' }];
  const sites: FleetSite[] = [
    {
      metadata: { name: 'homelab-ext' },
      spec: { siteId: 'homelab-ext', siteName: 'Homelab external' },
    },
  ];

  it('uses metadata.site when present', () => {
    expect(providerSite(provider('k8s-lab-ext02', { site: 'homelab-ext' }), clusters, sites)).toBe(
      'Homelab external',
    );
  });

  it('resolves site from fleet cluster membership', () => {
    expect(providerSite(provider('kubevirt-lab-ext02'), clusters, sites)).toBe('Homelab external');
  });
});
