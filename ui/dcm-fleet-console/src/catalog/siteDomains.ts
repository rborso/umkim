import type { FleetSite } from '../api/types';
import { fleetSiteEffectiveDomain, fleetSiteEffectiveId, fleetSiteDisplayName } from '../api/client';

export type DomainZoneOption = {
  id: string;
  label: string;
  zone: string;
  siteId?: string;
  kind: 'hub' | 'site' | 'custom';
};

export type EdgeDomainRegistry = {
  hub_zone?: string;
  hub_wildcard?: string;
  sites?: Array<{
    site_id?: string;
    display_name?: string;
    domains?: Array<{ zone?: string; id?: string }>;
  }>;
};

export type SiteDomainFieldContext = {
  options: DomainZoneOption[];
  selectedOptionByField: Record<string, string>;
  hostPrefixPath?: string;
  onDomainChange: (fieldPath: string, optionId: string, zone: string, siteId?: string) => void;
};

const CUSTOM_ZONE = '__custom__';

export function hubZoneFromRegistry(registry: EdgeDomainRegistry | null | undefined): string {
  return registry?.hub_zone?.trim() || '';
}

export function siteZoneList(site: FleetSite): string[] {
  const zones = new Set<string>();
  const primary = fleetSiteEffectiveDomain(site);
  if (primary) zones.add(primary);
  for (const d of site.spec?.edge?.domains || []) {
    if (d.zone?.trim()) zones.add(d.zone.trim());
  }
  return [...zones];
}

export function buildDomainZoneOptions(
  sites: FleetSite[],
  registry: EdgeDomainRegistry | null | undefined,
): DomainZoneOption[] {
  const out: DomainZoneOption[] = [];
  const hubZone = hubZoneFromRegistry(registry);
  if (hubZone) {
    out.push({
      id: `hub:${hubZone}`,
      label: `Hub — ${hubZone}`,
      zone: hubZone,
      kind: 'hub',
    });
  }
  for (const site of sites) {
    const siteId = fleetSiteEffectiveId(site);
    const name = fleetSiteDisplayName(site);
    for (const zone of siteZoneList(site)) {
      out.push({
        id: `site:${siteId}:${zone}`,
        label: `${name} — ${zone}`,
        zone,
        siteId,
        kind: 'site',
      });
    }
  }
  out.push({
    id: CUSTOM_ZONE,
    label: 'Custom domain…',
    zone: '',
    kind: 'custom',
  });
  return out;
}

export function matchDomainZoneOption(
  options: DomainZoneOption[],
  zone: string,
): string {
  const trimmed = zone.trim();
  if (!trimmed) return options[0]?.id ?? CUSTOM_ZONE;
  const exact = options.find((o) => o.kind !== 'custom' && o.zone === trimmed);
  if (exact) return exact.id;
  return CUSTOM_ZONE;
}

export function isCustomDomainOption(optionId: string): boolean {
  return optionId === CUSTOM_ZONE;
}

export function fqdnFromPrefix(prefix: string, baseDomain: string): string {
  const host = prefix.trim();
  const zone = baseDomain.trim();
  if (!host || !zone) return '';
  if (host.includes('.')) return host;
  return `${host}.${zone}`;
}

export function siteIdForDomainOption(
  optionId: string,
  options: DomainZoneOption[],
): string | undefined {
  return options.find((o) => o.id === optionId)?.siteId;
}

export function buildSiteDomainContext(
  sites: FleetSite[],
  registry: EdgeDomainRegistry | null | undefined,
  fieldValues: Record<string, string>,
  selectedOptionByField: Record<string, string>,
  onDomainChange: SiteDomainFieldContext['onDomainChange'],
): SiteDomainFieldContext {
  const options = buildDomainZoneOptions(sites, registry);
  const selectedOptionByFieldOut = { ...selectedOptionByField };
  for (const path of ['values.fleet.ingress.baseDomain']) {
    if (!selectedOptionByFieldOut[path]) {
      selectedOptionByFieldOut[path] = matchDomainZoneOption(
        options,
        fieldValues[path] ?? '',
      );
    }
  }
  return {
    options,
    selectedOptionByField: selectedOptionByFieldOut,
    hostPrefixPath: 'values.fleet.ingress.host',
    onDomainChange,
  };
}

export function previewIngressFqdn(
  fieldValues: Record<string, string>,
  hostPrefixPath = 'values.fleet.ingress.host',
  baseDomainPath = 'values.fleet.ingress.baseDomain',
): string {
  return fqdnFromPrefix(
    fieldValues[hostPrefixPath] ?? '',
    fieldValues[baseDomainPath] ?? '',
  );
}

export { CUSTOM_ZONE };
