export type JumphostSource = 'cluster' | 'site' | 'none';

export function effectiveClusterJumphost(
  clusterJumphost?: string,
  siteJumphost?: string,
): { effective: string; source: JumphostSource } {
  const cluster = clusterJumphost?.trim();
  if (cluster) {
    return { effective: cluster, source: 'cluster' };
  }
  const site = siteJumphost?.trim();
  if (site) {
    return { effective: site, source: 'site' };
  }
  return { effective: '', source: 'none' };
}
