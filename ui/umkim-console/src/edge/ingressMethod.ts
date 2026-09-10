import type { FleetCluster, FleetSite, FleetSiteIngress } from '../api/types';

export type FleetIngressMethod = 'port_forward' | 'reverse_proxy';

export function ingressMethodLabel(method?: string): string {
  switch (method) {
    case 'port_forward':
      return 'Port forward';
    case 'reverse_proxy':
      return 'Reverse proxy';
    default:
      return method || '—';
  }
}

export function isPortForwardMethod(method?: string): boolean {
  return method === 'port_forward';
}

export function isReverseProxyMethod(method?: string): boolean {
  return method === 'reverse_proxy';
}

/** Ingress auth is only supported on port_forward clusters. */
export function ingressAuthSupported(method?: string): boolean {
  return isPortForwardMethod(method);
}

export function effectiveIngressWildcard(ent: FleetSiteIngress): string {
  const wildcard = ent.wildcard?.trim();
  if (wildcard) return wildcard;
  const zone = ent.zone?.trim();
  if (zone) return `*.${zone}`;
  return '';
}

/** First port_forward zone on a site (app siteHost defaults). */
export function fleetSitePortForwardZone(site: FleetSite): string {
  for (const ent of site.spec.edge?.ingress || []) {
    if (ent.method === 'port_forward' && ent.zone?.trim()) {
      return ent.zone.trim();
    }
  }
  return '';
}

export function siteIngressById(site: FleetSite, id: string): FleetSiteIngress | undefined {
  return site.spec.edge?.ingress?.find((i) => i.id === id);
}

export function formatIngressOptionLabel(ent: FleetSiteIngress): string {
  const method = ingressMethodLabel(ent.method);
  const availability =
    ent.method === 'port_forward'
      ? (ent.assignedClusters?.length ?? 0) > 0
        ? ' · full'
        : ' · available'
      : ' · available';
  const zone = ent.method === 'port_forward' && ent.zone ? ` — ${ent.zone}` : '';
  return `${ent.id} · ${method}${zone}${availability}`;
}

/** Compact site-list / overview summary of hub gateway + ingress entries. */
export function fleetSiteIngressSummary(site: FleetSite): string {
  const parts: string[] = [];
  if (site.spec.edge?.hubGateway) {
    parts.push(site.spec.edge.hubZone ? `hub gateway (${site.spec.edge.hubZone})` : 'hub gateway');
  }
  for (const ent of site.spec.edge?.ingress || []) {
    if (!ent.id) continue;
    const method = ingressMethodLabel(ent.method);
    const detail =
      ent.method === 'port_forward' && ent.zone
        ? `${ent.id}: ${method} → ${ent.zone}`
        : `${ent.id}: ${method}`;
    parts.push(detail);
  }
  return parts.length ? parts.join(' · ') : '—';
}

/** Port-forward ingress rows are exclusive to one cluster. */
export function ingressOptionDisabled(
  ent: FleetSiteIngress,
  currentCluster?: string,
): boolean {
  if (ent.method !== 'port_forward') return false;
  const assigned = ent.assignedClusters || [];
  return assigned.some((c) => c && c !== currentCluster);
}

/** DNS zone for site route host suggestions (port_forward only). */
export function clusterIngressZone(site: FleetSite | undefined, cluster?: FleetCluster): string {
  if (!site || !cluster || isReverseProxyMethod(cluster.method)) return '';
  if (cluster.ingress_id) {
    const ent = siteIngressById(site, cluster.ingress_id);
    if (ent?.method === 'port_forward' && ent.zone?.trim()) {
      return ent.zone.trim();
    }
  }
  return fleetSitePortForwardZone(site);
}

export function defaultPortForwardIngressRow(id = 'default'): FleetSiteIngress {
  return {
    id,
    method: 'port_forward',
    address: '',
    zone: '',
    wildcard: '',
  };
}
