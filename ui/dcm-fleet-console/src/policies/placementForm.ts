import type { ClusterRequirements, FailoverMode, PlacementSpec, PlacementStrategy } from '../api/types';

export const PLACEMENT_STRATEGY_OPTIONS: { value: PlacementStrategy; label: string }[] = [
  { value: 'single', label: 'single — one cluster' },
  { value: 'spread', label: 'spread — distribute across N clusters' },
  { value: 'all_matching', label: 'all_matching — every matching cluster' },
  { value: 'least_utilized', label: 'least_utilized — lowest CPU/memory utilization' },
];

export const FAILOVER_MODE_OPTIONS: { value: FailoverMode; label: string }[] = [
  { value: 'manual', label: 'manual — observe health only' },
  { value: 'relocatable', label: 'relocatable — cold migrate (cluster_count 1)' },
  { value: 'standby', label: 'standby — hot switch (cluster_count 2)' },
];

export const PREVIEW_SERVICE_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: 'container', label: 'container — K8s container workloads' },
  { value: 'operator', label: 'operator — OLM subscriptions' },
  { value: 'helm', label: 'helm — Helm chart releases' },
  { value: 'cluster', label: 'cluster — cluster service type' },
];

/** One label per line: key=value (supports fleet.capability/storage.rwx=true). */
export function matchLabelsToText(labels?: Record<string, string>): string {
  if (!labels) return '';
  return Object.entries(labels)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n');
}

export function textToMatchLabels(text: string): Record<string, string> | undefined {
  const lines = text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
  if (lines.length === 0) return undefined;
  const out: Record<string, string> = {};
  for (const line of lines) {
    const eq = line.indexOf('=');
    if (eq < 1) continue;
    out[line.slice(0, eq).trim()] = line.slice(eq + 1).trim();
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

export function instanceLabelSelectorToText(sel?: Record<string, string>): string {
  return matchLabelsToText(sel);
}

export function policyPlacementFromForm(
  matchLabelsText: string,
  clusterCount: number,
  strategy: PlacementSpec['strategy'],
  siteDiversity?: boolean,
): { cluster_requirements?: ClusterRequirements; placement?: PlacementSpec } {
  const matchLabels = textToMatchLabels(matchLabelsText);
  const cluster_requirements = matchLabels
    ? { matchLabels, matchExpressions: [] }
    : undefined;
  const placement: PlacementSpec | undefined =
    clusterCount > 0
      ? {
          cluster_count: clusterCount,
          strategy: strategy || 'single',
          ...(siteDiversity ? { site_diversity: true } : {}),
        }
      : undefined;
  return { cluster_requirements, placement };
}
