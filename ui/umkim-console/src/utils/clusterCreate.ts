import type {
  FleetClusterCreate,
  FleetClusterNodeInput,
  FleetClusterTopology,
} from '../api/types';

/** Default HA control-plane slots (placeholders the operator replaces). */
export const defaultHaControlNodes = (clusterName: string): FleetClusterNodeInput[] =>
  [1, 2, 3].map((i) => ({
    hostname: clusterName ? `${clusterName}-${i}` : '',
    address: `198.51.100.2${i}`,
    role: 'control' as const,
  }));

/** Default single-node slot. */
export const defaultSingleNode = (clusterName: string, address: string): FleetClusterNodeInput[] => [
  {
    hostname: clusterName || 'node',
    address,
    role: 'control',
  },
];

/**
 * Node slots for the cluster create request.
 *
 * HA fills three controls plus any workers; a single node keeps its slot as-is
 * (its hostname and, importantly, its `vm` block). Node slots carry the optional
 * `vm` spec that makes the pipeline declare and create the guest, so dropping it
 * here silently turns "create this VM" into "expect an existing host".
 */
export function buildCreateNodes(
  topology: FleetClusterTopology,
  form: FleetClusterCreate,
): FleetClusterNodeInput[] {
  if (topology !== 'ha_3_node') {
    const first = form.nodes?.[0];
    return [
      {
        ...first,
        hostname: first?.hostname || form.clusterName || 'node',
        address: form.bootstrapHost,
        role: 'control',
      },
    ];
  }

  const controls = (form.nodes || defaultHaControlNodes(form.clusterName))
    .filter((n) => (n.role || 'control') === 'control')
    .slice(0, 3)
    .map((n, i) => ({
      ...n,
      role: 'control' as const,
      hostname: n.hostname || `${form.clusterName}-${i + 1}`,
    }));
  while (controls.length < 3) {
    const i = controls.length;
    const fallback = defaultHaControlNodes(form.clusterName)[i];
    controls.push({
      ...fallback,
      role: 'control' as const,
      hostname: fallback.hostname || `${form.clusterName}-${i + 1}`,
    });
  }
  const workers = (form.nodes || [])
    .filter((n) => n.role === 'worker')
    .map((n, i) => ({
      ...n,
      role: 'worker' as const,
      hostname: n.hostname || `${form.clusterName}-w${i + 1}`,
    }));
  return [...controls, ...workers];
}
