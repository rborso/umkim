import { fleetSiteEffectiveId } from '../../api/client';
import type { FleetCluster, FleetSite, FleetVM } from '../../api/types';

export type SiteVmRow = FleetVM & { cluster: string };

/**
 * Cluster names that belong to the site: the site's declared cluster list plus
 * any non-management cluster whose `site` matches the site id. Deduped, with the
 * hub cluster excluded.
 */
export function siteClusterNames(site: FleetSite, clusters: FleetCluster[]): string[] {
  const siteId = fleetSiteEffectiveId(site);
  const declared = site.clusters ?? [];
  const matched = clusters
    .filter((c) => !c.management && (c.site === siteId || declared.includes(c.name)))
    .map((c) => c.name);
  return Array.from(new Set([...declared, ...matched])).filter(
    (name) => name.length > 0 && name !== 'umkim-hub',
  );
}

/**
 * Flatten per-cluster VM list results into rows tagged with their cluster.
 * Rejected clusters are collected so the UI can note clusters where the VM API
 * is unavailable (for example KubeVirt not installed).
 */
export function combineSiteVmResults(
  clusterNames: string[],
  results: PromiseSettledResult<FleetVM[]>[],
): { rows: SiteVmRow[]; failedClusters: string[] } {
  const rows: SiteVmRow[] = [];
  const failedClusters: string[] = [];
  results.forEach((result, i) => {
    const cluster = clusterNames[i];
    if (result.status === 'fulfilled') {
      for (const vm of result.value) {
        rows.push({ ...vm, cluster });
      }
    } else if (cluster) {
      failedClusters.push(cluster);
    }
  });
  rows.sort(
    (a, b) =>
      a.cluster.localeCompare(b.cluster) ||
      a.namespace.localeCompare(b.namespace) ||
      a.name.localeCompare(b.name),
  );
  return { rows, failedClusters };
}
