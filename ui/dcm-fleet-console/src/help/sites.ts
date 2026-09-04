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
      'Default edge exposure for clusters in this site. Workload sites: none, port_forward, or reverse_proxy. The management (hub) site uses hub_gateway and is not editable here. Working TLS pairs: none/hub_gateway→acme_hub, port_forward→acme_cluster, reverse_proxy→external.',
    example: 'port_forward',
    examples: ['none', 'port_forward', 'reverse_proxy'],
    whenToUse:
      'Use port_forward when WAN DNAT hits cluster NodePorts; reverse_proxy when Caddy/nginx terminates TLS; none for WireGuard/hub-only access.',
    learnMoreUrl: DOCS.createSiteExposure,
  },
  'site.edgeDomain': {
    description: 'Primary DNS zone for site apps (A/AAAA and wildcard records point to WAN or VIP).',
    example: 'lab.example.com',
  },
  'site.wildcard': {
    description: 'Wildcard hostname pattern for apps in this zone (usually *. followed by the primary edge domain).',
    example: '*.lab.example.com',
  },
  'site.dnsNote': {
    description: 'Free-text operator note describing how DNS should target this site (shown on site edge / hub infrastructure summary).',
    example: 'A/AAAA *.lab.example.com → OpenWrt WAN → cluster NodePorts',
  },
  'site.endpointId': {
    description: 'Stable ID for an external WAN IP or VIP. Clusters reference this via Ingress endpoint ID for port-forward.',
    example: 'openwrt-wan',
  },
  'site.endpointAddress': {
    description: 'Public IP or VIP where WAN traffic arrives before DNAT to cluster NodePorts.',
    example: '203.0.113.50',
  },
  'site.domainZoneId': {
    description: 'Identifier for a DNS zone within the site (referenced by cluster domain bindings).',
    example: 'site-zone',
  },
  'site.domainZone': {
    description: 'FQDN zone served by this site edge (often matches the primary edge domain).',
    example: 'lab.example.com',
  },
  'site.domainTlsPolicy': {
    description:
      'TLS handling for routes in this zone. Prefer acme_cluster with port_forward, external with reverse_proxy, acme_hub with none/hub_gateway. passthrough is not a tested profile.',
    example: 'acme_cluster',
    examples: ['inherit', 'acme_hub', 'acme_cluster', 'external', 'passthrough'],
    learnMoreUrl: DOCS.exposureTlsPairs,
  },
  'site.wanEgress': {
    description:
      'Whether this site has an internet egress path for outbound traffic. Independent of public ingress / exposure mode.',
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
