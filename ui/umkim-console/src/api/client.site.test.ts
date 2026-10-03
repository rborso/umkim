import { describe, expect, it } from 'vitest';
import { fleetDefaultWorkloadSiteId, fleetSiteById } from './client';
import type { FleetSite } from './types';

function site(
  id: string,
  opts: { registry?: boolean; management?: boolean } = {},
): FleetSite {
  return {
    metadata: {
      name: id,
      annotations: opts.management
        ? { 'fleet.umkim.com/management-site': 'true' }
        : undefined,
    },
    spec: opts.registry
      ? { siteName: id, provisioningMode: 'registry' as const }
      : { siteName: id },
  };
}

describe('fleetDefaultWorkloadSiteId', () => {
  it('returns first registry non-management site', () => {
    const sites = [
      site('management', { registry: true, management: true }),
      site('site-a', { registry: true }),
      site('site-b', { registry: true }),
    ];
    expect(fleetDefaultWorkloadSiteId(sites)).toBe('site-a');
  });

  it('returns empty when only management registry site exists', () => {
    const sites = [site('management', { registry: true, management: true })];
    expect(fleetDefaultWorkloadSiteId(sites)).toBe('');
  });

  it('skips non-registry sites', () => {
    const sites = [site('legacy'), site('site-a', { registry: true })];
    expect(fleetDefaultWorkloadSiteId(sites)).toBe('site-a');
  });
});

describe('fleetSiteById', () => {
  it('resolves ingress when siteName differs from metadata.name', () => {
    const sites: FleetSite[] = [
      {
        metadata: { name: 'cr-name' },
        spec: {
          siteName: 'pretty',
          edge: { ingress: [{ id: 'wan', method: 'port_forward' }] },
        },
      },
    ];
    expect(fleetSiteById(sites, 'pretty')?.spec.edge?.ingress?.[0]?.id).toBe('wan');
  });
});
