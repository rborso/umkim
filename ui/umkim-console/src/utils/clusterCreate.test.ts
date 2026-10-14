import { describe, expect, it } from 'vitest';
import { buildCreateNodes, defaultHaControlNodes, defaultSingleNode } from './clusterCreate';
import type { FleetClusterCreate } from '../api/types';

const form = (over: Partial<FleetClusterCreate> = {}): FleetClusterCreate => ({
  clusterName: 'lab-sn',
  siteId: 'homelab',
  bootstrapHost: '172.16.10.120',
  ...over,
});

describe('buildCreateNodes', () => {
  it('keeps the vm block on a single node', () => {
    const nodes = buildCreateNodes('single_node', form({
      nodes: [{ hostname: 'lab-sn', address: '172.16.10.120', role: 'control', vm: { cores: 4, diskGi: 50 } }],
    }));
    expect(nodes).toHaveLength(1);
    // The regression: the single-node branch used to drop vm, so "create this VM"
    // silently became "expect an existing host".
    expect(nodes[0].vm).toEqual({ cores: 4, diskGi: 50 });
    expect(nodes[0].address).toBe('172.16.10.120');
    expect(nodes[0].hostname).toBe('lab-sn');
  });

  it('keeps a selected inventory hostname on a single node', () => {
    const nodes = buildCreateNodes('single_node', form({
      nodes: [{ hostname: 'virt09', address: '172.16.10.9', role: 'control' }],
    }));
    expect(nodes[0].hostname).toBe('virt09');
  });

  it('falls back to the cluster name when a single node has no hostname', () => {
    const nodes = buildCreateNodes('single_node', form({ nodes: [] }));
    expect(nodes[0].hostname).toBe('lab-sn');
  });

  it('keeps the vm block on HA controls and workers', () => {
    const nodes = buildCreateNodes('ha_3_node', form({
      clusterName: 'lab-ha',
      nodes: [
        { hostname: 'lab-ha-1', address: '172.16.10.101', role: 'control', vm: { cores: 4 } },
        { hostname: 'lab-ha-2', address: '172.16.10.102', role: 'control', vm: { cores: 4 } },
        { hostname: 'lab-ha-3', address: '172.16.10.103', role: 'control', vm: { cores: 4 } },
        { hostname: 'lab-ha-w1', address: '172.16.10.110', role: 'worker', vm: { cores: 8, memory: 8192 } },
      ],
    }));
    expect(nodes).toHaveLength(4);
    expect(nodes[0].vm).toEqual({ cores: 4 });
    expect(nodes[3].role).toBe('worker');
    expect(nodes[3].vm).toEqual({ cores: 8, memory: 8192 });
  });

  it('fills missing HA control slots from the defaults', () => {
    const nodes = buildCreateNodes('ha_3_node', form({ clusterName: 'lab-ha', nodes: [] }));
    expect(nodes).toHaveLength(3);
    expect(nodes.map((n) => n.hostname)).toEqual(['lab-ha-1', 'lab-ha-2', 'lab-ha-3']);
    expect(nodes.every((n) => !n.vm)).toBe(true);
  });
});

describe('default node slots', () => {
  it('HA defaults are named after the cluster', () => {
    expect(defaultHaControlNodes('lab').map((n) => n.hostname)).toEqual(['lab-1', 'lab-2', 'lab-3']);
  });
  it('single default is named after the cluster', () => {
    expect(defaultSingleNode('lab', '172.16.10.1')[0].hostname).toBe('lab');
  });
});
