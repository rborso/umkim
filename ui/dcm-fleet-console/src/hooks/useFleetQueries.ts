import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  getAcme,
  getFleetCluster,
  getHubInfrastructure,
  getIngressAuth,
  getLLM,
  listFleetApps,
  listFleetCharts,
  listFleetClusterNodes,
  listFleetClusters,
  listFleetSites,
} from '../api/client';
import { STALE_TIME_MS } from '../queryClient';

export const fleetKeys = {
  sites: ['sites'] as const,
  clusters: ['clusters'] as const,
  apps: ['apps'] as const,
  charts: ['charts'] as const,
  cluster: (name: string) => ['cluster', name] as const,
  clusterNodes: (name: string) => ['clusterNodes', name] as const,
  hubInfrastructure: ['infrastructure', 'hub'] as const,
  ingressAuth: ['infrastructure', 'ingressAuth'] as const,
  acme: ['infrastructure', 'acme'] as const,
  llm: ['infrastructure', 'llm'] as const,
};

export function useFleetSitesQuery(refetchInterval?: number) {
  return useQuery({
    queryKey: fleetKeys.sites,
    queryFn: listFleetSites,
    staleTime: STALE_TIME_MS,
    refetchInterval,
  });
}

export function useFleetClustersQuery(refetchInterval?: number) {
  return useQuery({
    queryKey: fleetKeys.clusters,
    queryFn: listFleetClusters,
    staleTime: STALE_TIME_MS,
    refetchInterval,
  });
}

export function useFleetAppsQuery(refetchInterval?: number) {
  return useQuery({
    queryKey: fleetKeys.apps,
    queryFn: listFleetApps,
    staleTime: STALE_TIME_MS,
    refetchInterval,
  });
}

export function useFleetChartsQuery(refetchInterval?: number) {
  return useQuery({
    queryKey: fleetKeys.charts,
    queryFn: listFleetCharts,
    staleTime: STALE_TIME_MS,
    refetchInterval,
  });
}

export function useFleetClusterQuery(name: string | undefined, refetchInterval?: number) {
  return useQuery({
    queryKey: fleetKeys.cluster(name || ''),
    queryFn: () => getFleetCluster(name!),
    enabled: !!name,
    staleTime: STALE_TIME_MS,
    refetchInterval,
  });
}

export function useFleetClusterNodesQuery(
  name: string | undefined,
  enabled: boolean,
  refetchInterval?: number,
) {
  return useQuery({
    queryKey: fleetKeys.clusterNodes(name || ''),
    queryFn: () => listFleetClusterNodes(name!),
    enabled: !!name && enabled,
    staleTime: STALE_TIME_MS,
    refetchInterval,
  });
}

export function useHubInfrastructureQuery() {
  return useQuery({
    queryKey: fleetKeys.hubInfrastructure,
    queryFn: getHubInfrastructure,
    staleTime: STALE_TIME_MS,
  });
}

export function useIngressAuthQuery() {
  return useQuery({
    queryKey: fleetKeys.ingressAuth,
    queryFn: getIngressAuth,
    staleTime: STALE_TIME_MS,
  });
}

export function useAcmeQuery() {
  return useQuery({
    queryKey: fleetKeys.acme,
    queryFn: getAcme,
    staleTime: STALE_TIME_MS,
  });
}

export function useLLMQuery() {
  return useQuery({
    queryKey: fleetKeys.llm,
    queryFn: getLLM,
    staleTime: STALE_TIME_MS,
  });
}

export function useInvalidateFleet() {
  const qc = useQueryClient();
  return {
    invalidateSites: () => qc.invalidateQueries({ queryKey: fleetKeys.sites }),
    invalidateClusters: () => qc.invalidateQueries({ queryKey: fleetKeys.clusters }),
    invalidateApps: () => qc.invalidateQueries({ queryKey: fleetKeys.apps }),
    invalidateCharts: () => qc.invalidateQueries({ queryKey: fleetKeys.charts }),
    invalidateCluster: (name: string) => {
      void qc.invalidateQueries({ queryKey: fleetKeys.cluster(name) });
      void qc.invalidateQueries({ queryKey: fleetKeys.clusterNodes(name) });
    },
    invalidateAll: () => qc.invalidateQueries(),
  };
}
