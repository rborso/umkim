/**
 * Canonical public docs (https://docs.umkim.com). Prefer unversioned paths
 * (latest); versioned /0.1.0/ mirrors exist in the sitemap.
 */

export const DOCS_BASE = 'https://docs.umkim.com';

export const DOCS = {
  fleetEdge: `${DOCS_BASE}/components/fleet-edge/`,
  /** Exposure + TLS working pairs (Starlight heading id). */
  exposureTlsPairs: `${DOCS_BASE}/components/fleet-edge/#supported-working-pairs`,
  exposureModes: `${DOCS_BASE}/components/fleet-edge/#exposure-modes-and-tls-policies`,
  createCluster: `${DOCS_BASE}/getting-started/create-cluster/`,
  createClusterEdge: `${DOCS_BASE}/getting-started/create-cluster/#edge-exposure-optional`,
  createSite: `${DOCS_BASE}/getting-started/create-site/`,
  createSiteExposure: `${DOCS_BASE}/getting-started/create-site/#exposure-profiles`,
  clusterStorage: `${DOCS_BASE}/operations/cluster-storage/`,
  ingressAuth: `${DOCS_BASE}/operations/ingress-auth/`,
  deployApp: `${DOCS_BASE}/getting-started/deploy-app/`,
  appDeployLifecycle: `${DOCS_BASE}/lifecycles/app-deploy/`,
  failover: `${DOCS_BASE}/lifecycles/failover/`,
  fleetLlm: `${DOCS_BASE}/components/fleet-llm/`,
  fleetSite: `${DOCS_BASE}/components/fleet-site/`,
  fleetApps: `${DOCS_BASE}/components/fleet-apps/`,
  consoleAuth: `${DOCS_BASE}/components/console-auth/`,
} as const;

/** Repo source for the same content (optional secondary link). */
export const DOCS_GITHUB_FLEET_EDGE =
  'https://github.com/rborso/umkim/blob/main/docs/components/fleet-edge.md#exposure-modes-and-tls-policies';
