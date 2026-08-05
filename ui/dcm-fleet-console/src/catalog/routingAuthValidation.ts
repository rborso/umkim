import type { PlacementPreviewResponse, CatalogFieldDef } from '../api/types';
import { getFleetCluster, getIngressAuth } from '../api/client';
import { isFieldVisible } from './fieldVisibility';

const REQUIRE_AUTH_FIELD: CatalogFieldDef = {
  path: 'routing.require_auth',
  editable: true,
  validation_schema: {
    'x-ui-depends-on': { path: 'routing.mode', in: ['hub', 'site'] },
  },
};

export function isRequireAuthFieldVisible(routingMode: string): boolean {
  return isFieldVisible(REQUIRE_AUTH_FIELD, { 'routing.mode': routingMode });
}

export function validateRequireAuthSync(fieldValues: Record<string, string>): string | null {
  const requireAuth = fieldValues['routing.require_auth'] === 'true';
  if (!requireAuth) return null;

  const mode = fieldValues['routing.mode'] || 'hub';
  if (mode === 'dual' || mode === 'none') {
    return 'Ingress auth cannot be enabled for dual or none routing mode';
  }
  return null;
}

export function primaryClusterFromPreview(
  preview: PlacementPreviewResponse | null | undefined,
): string | undefined {
  const clusters = preview?.eligible_clusters;
  if (!clusters?.length) return undefined;
  return clusters[0].name;
}

export async function validateRequireAuthAsync(
  fieldValues: Record<string, string>,
  preview: PlacementPreviewResponse | null | undefined,
): Promise<string | null> {
  const syncErr = validateRequireAuthSync(fieldValues);
  if (syncErr) return syncErr;
  if (fieldValues['routing.require_auth'] !== 'true') return null;

  const mode = fieldValues['routing.mode'] || 'hub';
  if (mode === 'hub') {
    const auth = await getIngressAuth();
    if (!auth.hubEnabled || auth.phase !== 'Ready') {
      return 'Enable hub ingress authentication in Infrastructure first';
    }
    return null;
  }

  if (mode === 'site') {
    let cluster: string | undefined = fieldValues['routing.cluster']?.trim();
    if (!cluster) {
      cluster = primaryClusterFromPreview(preview);
    }
    if (!cluster) {
      return 'Pin routing.cluster or complete placement preview before enabling ingress auth';
    }
    const fc = await getFleetCluster(cluster);
    if (fc.ingressAuthStatus?.phase !== 'Ready') {
      return `Enable ingress forward-auth on cluster ${cluster} first`;
    }
  }

  return null;
}
