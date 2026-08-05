import type { FieldHelpContent } from './fieldHelp';

export const DOMAINS_FIELD_HELP: Record<string, FieldHelpContent> = {
  'domains.dnsHost': {
    description: 'Hostname to resolve for DNS validation (A/AAAA record check against fleet edge).',
    example: 'whoami.homelab.mgmt.umkim.com',
  },
  'domains.expectedIp': {
    description: 'Optional expected IPv4/IPv6 address. When set, validation fails if DNS does not match.',
    example: '203.0.113.1',
  },
};
