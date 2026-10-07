import { describe, expect, it } from 'vitest';
import { eligibleClusterHosts, hostAnsibleAddr, hostSelectOptions } from './clusterHosts';
import type { SiteHostRecord } from '../api/types';

const host = (name: string, extra: Partial<SiteHostRecord> = {}): SiteHostRecord => ({
  name,
  ansible_host: `10.0.0.${name.length}`,
  kind: 'bare_metal',
  ...extra,
});

describe('hostAnsibleAddr', () => {
  it('prefers ansible_host, falls back to status, trims', () => {
    expect(hostAnsibleAddr({ name: 'a', ansible_host: ' 10.0.0.1 ' })).toBe('10.0.0.1');
    expect(hostAnsibleAddr({ name: 'a', status: { ansible_host: '10.0.0.2' } })).toBe('10.0.0.2');
    expect(hostAnsibleAddr({ name: 'a' })).toBe('');
  });
});

describe('eligibleClusterHosts', () => {
  it('keeps addressed bare-metal and vm hosts', () => {
    const out = eligibleClusterHosts([host('bm1'), host('vm1', { kind: 'vm' })]);
    expect(out.map((h) => h.name)).toEqual(['bm1', 'vm1']);
  });

  it('drops hypervisors', () => {
    expect(eligibleClusterHosts([host('hv1', { kind: 'hypervisor' })])).toEqual([]);
  });

  it('drops hosts without an address', () => {
    expect(eligibleClusterHosts([{ name: 'x', kind: 'bare_metal' }])).toEqual([]);
  });

  it('drops hosts already in a cluster', () => {
    expect(eligibleClusterHosts([host('c1', { clusters: ['lab-ha-0'] })])).toEqual([]);
    expect(eligibleClusterHosts([host('c2', { clusterIntent: { mode: 'enroll' } })])).toEqual([]);
  });
});

describe('hostSelectOptions', () => {
  const hosts = [host('a'), host('b'), host('c')];

  it('hides hosts already picked in other slots but keeps the current one', () => {
    const opts = hostSelectOptions(hosts, new Set(['a', 'b']), 'a');
    expect(opts.map((h) => h.name)).toEqual(['a', 'c']);
  });

  it('shows every host when nothing is selected', () => {
    expect(hostSelectOptions(hosts, new Set(), '').map((h) => h.name)).toEqual(['a', 'b', 'c']);
  });
});
