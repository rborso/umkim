import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const SITES_FIELD_HELP: Record<string, FieldHelpContent> = {
  'site.siteId': {
    description:
      'Site name (Kubernetes CR name / fleet.site id). Clusters created under this site inherit this identifier for placement.',
    example: 'homelab',
    learnMoreUrl: DOCS.createSite,
  },
  'site.displayName': {
    description: 'Human-readable site title shown in the console.',
    example: 'Homelab',
  },
  'site.exposureMode': {
    description:
      'Site edge pattern: hub gateway catalog, port_forward ingress rows, reverse_proxy ingress rows, or no public ingress. Port-forward derives acme_cluster TLS; reverse-proxy uses external TLS; hub gateway uses hub ACME.',
    example: 'port_forward',
    examples: ['hub_gateway', 'port_forward', 'reverse_proxy'],
    whenToUse:
      'Use port_forward when WAN DNAT hits cluster NodePorts; reverse_proxy when Caddy/nginx terminates TLS; hub gateway for hub-routed public URLs; omit ingress for WireGuard-only sites.',
    learnMoreUrl: DOCS.createSiteExposure,
  },
  'site.edgeDomain': {
    description: 'Hub zone FQDN when hub gateway is enabled (A/AAAA and wildcard records point to hub WAN).',
    example: 'hub.example.com',
  },
  'site.wildcard': {
    description: 'Wildcard hostname pattern for a zone (usually *. followed by the zone).',
    example: '*.lab.example.com',
  },
  'site.dnsNote': {
    description: 'Free-text operator note describing how DNS should target this site (shown on site edge / hub infrastructure summary).',
    example: 'A/AAAA *.lab.example.com → OpenWrt WAN → cluster NodePorts',
  },
  'site.endpointId': {
    description: 'Legacy name for an ingress row id. Prefer the Ingress ID on each site ingress entry.',
    example: 'homelab-pf',
  },
  'site.endpointAddress': {
    description: 'Public IP or VIP on a port_forward ingress row where WAN traffic arrives before DNAT to cluster NodePorts. Optional when WAN address mode is DNS/DDNS.',
    example: '203.0.113.50',
  },
  'site.wanAddressMode': {
    description:
      'How fleet-edge validates site DNS for ACME. Fixed requires A/AAAA to match the WAN address; DNS resolves the hostname only (for DDNS or rotating WAN). Auto infers from WAN egress and address.',
    example: 'dns',
    examples: ['fixed', 'dns'],
    relatedFields: ['site.wanEgress', 'site.expectedWanIp', 'site.endpointAddress'],
    learnMoreUrl: DOCS.fleetEdge,
  },
  'site.domainZoneId': {
    description: 'Stable ingress row id (FleetSite.spec.edge.ingress[].id). Clusters bind with ingressId.',
    example: 'homelab-pf',
  },
  'site.domainZone': {
    description: 'DNS zone on a port_forward ingress row (required for PF; used for app siteHost defaults).',
    example: 'lab.example.com',
  },
  'site.domainTlsPolicy': {
    description:
      'TLS is derived from ingress method: port_forward → acme_cluster, reverse_proxy → external, hub gateway → acme_hub. Not set per cluster.',
    example: 'acme_cluster',
    examples: ['acme_hub', 'acme_cluster', 'external'],
    learnMoreUrl: DOCS.fleetEdge,
  },
  'site.wanEgress': {
    description:
      'Whether this site has an internet egress path for outbound traffic. Independent of public ingress entries.',
    example: 'true',
  },
  'site.expectedWanIp': {
    description: 'Optional WAN IP used when validating DNS records against the expected public address.',
    example: '203.0.113.50',
  },
  'site.dnsValidateHost': {
    description: 'Hostname to resolve when running Validate DNS on this site (A/AAAA check).',
    example: 'whoami.lab.example.com',
  },
  'site.jumphost': {
    description:
      'Default SSH bastion for hub Ansible jobs targeting clusters in this site (user@host:port). Clusters may override.',
    example: 'jump@bastion.example.com:22',
  },
  'site.region': {
    description: 'Logical region label for clusters and inventory (fleet.topology/region default).',
    example: 'homelab',
  },
  'site.failureDomain': {
    description: 'Failure domain or rack label for placement documentation.',
    example: 'rack-a',
  },
};
