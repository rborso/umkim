import { describe, expect, it } from 'vitest';
import {
  buildDomainZoneOptions,
  fqdnFromPrefix,
  matchDomainZoneOption,
  CUSTOM_ZONE,
} from './siteDomains';
import type { FleetSite } from '../api/types';

const sites: FleetSite[] = [
  {
    metadata: { name: 'homelab-ext' },
    spec: {
      siteId: 'homelab-ext',
      siteName: 'Homelab External',
      edge: { edgeDomain: 'site.example.com' },
    },
  },
];

describe('siteDomains', () => {
  it('builds hub and site zone options', () => {
    const options = buildDomainZoneOptions(sites, { hub_zone: 'apps.example.com' });
    expect(options.some((o) => o.zone === 'apps.example.com' && o.kind === 'hub')).toBe(true);
    expect(options.some((o) => o.zone === 'site.example.com' && o.kind === 'site')).toBe(true);
    expect(options.some((o) => o.id === CUSTOM_ZONE)).toBe(true);
  });

  it('matches custom zone when unknown', () => {
    const options = buildDomainZoneOptions(sites, { hub_zone: 'apps.example.com' });
    expect(matchDomainZoneOption(options, 'custom.example')).toBe(CUSTOM_ZONE);
  });

  it('builds fqdn from prefix and zone', () => {
    expect(fqdnFromPrefix('git', 'site.example.com')).toBe('git.site.example.com');
  });
});
