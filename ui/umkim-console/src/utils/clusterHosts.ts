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
