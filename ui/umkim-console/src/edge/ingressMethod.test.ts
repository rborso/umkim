import { describe, expect, it } from 'vitest';
import type { FleetSite } from '../api/types';
import {
  clusterIngressZone,
  effectiveIngressWildcard,
  fleetSiteIngressSummary,
  fleetSitePortForwardZone,
  formatIngressOptionLabel,
  ingressAuthSupported,
  ingressOptionDisabled,
  ingressMethodLabel,
} from './ingressMethod';

function site(ingress: FleetSite['spec']['edge']): FleetSite {
  return {
    spec: {
      siteName: 'lab',
      edge: ingress,
    },
  };
}

describe('ingressMethod', () => {
  it('labels ingress methods', () => {
    expect(ingressMethodLabel('port_forward')).toBe('Port forward');
    expect(ingressMethodLabel('reverse_proxy')).toBe('Reverse proxy');
  });

  it('restricts ingress auth to port_forward', () => {
    expect(ingressAuthSupported('port_forward')).toBe(true);
    expect(ingressAuthSupported('reverse_proxy')).toBe(false);
  });

  it('derives wildcard from zone', () => {
    expect(effectiveIngressWildcard({ id: 'pf', method: 'port_forward', zone: 'lab.example.com' })).toBe(
      '*.lab.example.com',
    );
  });

  it('returns first port_forward zone', () => {
    const s = site({
      ingress: [
        { id: 'rp', method: 'reverse_proxy' },
        { id: 'pf', method: 'port_forward', zone: 'apps.example.com', address: '203.0.113.1' },
      ],
    });
    expect(fleetSitePortForwardZone(s)).toBe('apps.example.com');
  });

  it('marks port_forward full when assigned', () => {
    const ent = {
      id: 'pf',
      method: 'port_forward' as const,
      zone: 'lab.example.com',
      assignedClusters: ['cluster-a'],
    };
    expect(formatIngressOptionLabel(ent)).toContain('full');
    expect(ingressOptionDisabled(ent, 'cluster-b')).toBe(true);
    expect(ingressOptionDisabled(ent, 'cluster-a')).toBe(false);
  });

  it('suggests zone only for port_forward clusters', () => {
    const s = site({
      ingress: [{ id: 'pf', method: 'port_forward', zone: 'lab.example.com', address: '1.2.3.4' }],
    });
    expect(clusterIngressZone(s, { name: 'c1', ingress_id: 'pf', method: 'port_forward' })).toBe(
      'lab.example.com',
    );
    expect(clusterIngressZone(s, { name: 'c2', ingress_id: 'pf', method: 'reverse_proxy' })).toBe('');
  });

  it('summarizes hub gateway and ingress entries', () => {
    const s = site({
      hubGateway: true,
      hubZone: 'hub.example.com',
      ingress: [
        { id: 'site-pf', method: 'port_forward', zone: 'site.example.com', address: '203.0.113.50' },
        { id: 'edge-rp', method: 'reverse_proxy' },
      ],
    });
    const summary = fleetSiteIngressSummary(s);
    expect(summary).toContain('hub gateway (hub.example.com)');
    expect(summary).toContain('site-pf: Port forward → site.example.com');
    expect(summary).toContain('edge-rp: Reverse proxy');
  });
});
