import { describe, expect, it } from 'vitest';
import { combineSiteVmResults, siteClusterNames } from './siteVms';
import type { FleetCluster, FleetSite, FleetSiteSpec, FleetVM } from '../../api/types';

function site(spec: Partial<FleetSiteSpec>, clusters?: string[]): FleetSite {
  return { spec: { siteName: 'homelab', ...spec }, clusters };
}

describe('siteClusterNames', () => {
  it('merges declared clusters with clusters matched by site id and dedupes', () => {
    const s = site({ siteId: 'homelab' }, ['lab-single-0', 'lab-ha-0']);
    const clusters: FleetCluster[] = [
      { name: 'lab-single-0', site: 'homelab' },
      { name: 'lab-ha-0', site: 'homelab' },
      { name: 'lab-gpu-pt-0', site: 'homelab' },
      { name: 'other-site-0', site: 'elsewhere' },
    ];
    expect(siteClusterNames(s, clusters)).toEqual(['lab-single-0', 'lab-ha-0', 'lab-gpu-pt-0']);
  });

  it('excludes the hub and management clusters', () => {
    const s = site({}, ['lab-single-0']);
    const clusters: FleetCluster[] = [
      { name: 'lab-single-0', site: 'homelab' },
      { name: 'umkim-hub', site: 'homelab', management: true },
    ];
    expect(siteClusterNames(s, clusters)).toEqual(['lab-single-0']);
  });

  it('returns declared clusters when no cluster objects are loaded', () => {
    const s = site({ siteId: 'homelab' }, ['lab-single-0']);
    expect(siteClusterNames(s, [])).toEqual(['lab-single-0']);
  });
});

describe('combineSiteVmResults', () => {
  const vm = (namespace: string, name: string): FleetVM => ({ namespace, name });

  it('tags rows with their cluster and sorts by cluster/namespace/name', () => {
    const { rows, failedClusters } = combineSiteVmResults(
      ['lab-ha-0', 'lab-single-0'],
      [
        { status: 'fulfilled', value: [vm('default', 'z')] },
        { status: 'fulfilled', value: [vm('default', 'a'), vm('llm', 'b')] },
      ],
    );
    expect(failedClusters).toEqual([]);
    expect(rows.map((r) => `${r.cluster}/${r.namespace}/${r.name}`)).toEqual([
      'lab-ha-0/default/z',
      'lab-single-0/default/a',
      'lab-single-0/llm/b',
    ]);
  });

  it('collects rejected clusters without dropping other rows', () => {
    const { rows, failedClusters } = combineSiteVmResults(
      ['lab-gpu-0', 'lab-single-0'],
      [
        { status: 'rejected', reason: new Error('no kubevirt') },
        { status: 'fulfilled', value: [vm('default', 'a')] },
      ],
    );
    expect(failedClusters).toEqual(['lab-gpu-0']);
    expect(rows.map((r) => r.cluster)).toEqual(['lab-single-0']);
  });
});
