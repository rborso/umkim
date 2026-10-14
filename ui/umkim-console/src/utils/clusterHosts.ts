import type { SiteHostRecord } from '../api/types';

/** The address Ansible uses for a host (explicit or reported in status). */
export function hostAnsibleAddr(h: SiteHostRecord): string {
  return (h.ansible_host || h.status?.ansible_host || '').trim();
}

/**
 * Inventory hosts eligible to become a cluster node: has an address, is not a
 * hypervisor, and is not already a member of a cluster.
 */
export function eligibleClusterHosts(hosts: SiteHostRecord[]): SiteHostRecord[] {
  return hosts.filter(
    (h) =>
      hostAnsibleAddr(h).length > 0 &&
      h.kind !== 'hypervisor' &&
      !(h.clusters?.length) &&
      !h.clusterIntent,
  );
}

/**
 * Options for one node slot: eligible hosts minus those already picked in other
 * slots. The slot's own current value stays visible.
 */
export function hostSelectOptions(
  eligible: SiteHostRecord[],
  selectedNames: Set<string>,
  current: string,
): SiteHostRecord[] {
  return eligible.filter((h) => h.name === current || !selectedNames.has(h.name));
}

/**
 * RFC 5737 documentation ranges used as wizard placeholders. A VM cannot be
 * created on one, so the create form must not offer to.
 */
const PLACEHOLDER_PREFIXES = ['192.0.2.', '198.51.100.', '203.0.113.'];

/** True for an address a VM guest must not be created on (placeholder or empty). */
export function isPlaceholderAddress(address: string): boolean {
  const addr = (address || '').trim();
  if (!addr) return true;
  return PLACEHOLDER_PREFIXES.some((p) => addr.startsWith(p));
}
