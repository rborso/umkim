/** Supported exposure + TLS pairs for fleet edge (see docs.umkim.com/components/fleet-edge/). */

import { DOCS, DOCS_GITHUB_FLEET_EDGE } from '../help/docsUrls';

/** Public docs — supported exposure + TLS working pairs. */
export const EXPOSURE_TLS_DOCS_URL = DOCS.exposureTlsPairs;

/** Repo source for the same section (secondary). */
export const EXPOSURE_TLS_DOCS_GITHUB_URL = DOCS_GITHUB_FLEET_EDGE;

const RECOMMENDED_TLS: Record<string, string> = {
  none: 'acme_hub',
  hub_gateway: 'acme_hub',
  port_forward: 'acme_cluster',
  reverse_proxy: 'external',
};

export function recommendedTlsForExposure(exposure: string): string {
  return RECOMMENDED_TLS[exposure] || 'acme_hub';
}

export function resolveEffectiveExposure(
  exposure: string | undefined,
  siteDefault = 'none',
): string {
  const mode = (exposure || 'inherit').trim();
  if (!mode || mode === 'inherit') {
    return siteDefault || 'none';
  }
  return mode;
}

export function resolveEffectiveTls(tls: string | undefined, effectiveExposure: string): string {
  const policy = (tls || 'inherit').trim();
  if (!policy || policy === 'inherit') {
    return recommendedTlsForExposure(effectiveExposure);
  }
  return policy;
}

export type ExposureTlsPairIssue = {
  effectiveExposure: string;
  effectiveTls: string;
  recommendedTls: string;
  /** Short operator-facing reason */
  message: string;
};

/**
 * Returns a warning when exposure + TLS is not one of the supported working pairs.
 * `inherit` is resolved against siteDefaultExposure / recommended TLS for the effective exposure.
 */
export function getExposureTlsPairIssue(
  exposure: string | undefined,
  tls: string | undefined,
  siteDefaultExposure = 'none',
): ExposureTlsPairIssue | null {
  const effectiveExposure = resolveEffectiveExposure(exposure, siteDefaultExposure);
  const effectiveTls = resolveEffectiveTls(tls, effectiveExposure);
  const recommendedTls = recommendedTlsForExposure(effectiveExposure);

  if (effectiveTls === recommendedTls) {
    return null;
  }

  let message: string;
  if (effectiveTls === 'passthrough') {
    message =
      'TLS passthrough is not a tested profile. Prefer the recommended policy for this exposure mode.';
  } else if (effectiveExposure === 'port_forward' && effectiveTls === 'external') {
    message =
      'Port-forward expects ACME on the cluster (acme_cluster). With external TLS, WAN :443→30443 has nothing terminating certificates.';
  } else if (effectiveExposure === 'reverse_proxy' && effectiveTls === 'acme_cluster') {
    message =
      'Reverse-proxy expects external TLS on Caddy/nginx. Cluster ACME on :30443 conflicts with the proxy owning :443.';
  } else if (
    (effectiveExposure === 'none' || effectiveExposure === 'hub_gateway') &&
    effectiveTls === 'acme_cluster'
  ) {
    message =
      'This exposure has no direct site ingress. Cluster ACME will not serve public site hostnames; use acme_hub (or hub catalog routes).';
  } else if (
    (effectiveExposure === 'none' || effectiveExposure === 'hub_gateway') &&
    effectiveTls === 'external'
  ) {
    message =
      'External TLS is for reverse-proxy sites. With none/hub_gateway, use acme_hub for hub routes.';
  } else if (effectiveExposure === 'port_forward' && effectiveTls === 'acme_hub') {
    message =
      'Port-forward site traffic hits the cluster, not hub Traefik LE. Use acme_cluster (and enable cluster ACME).';
  } else if (effectiveExposure === 'reverse_proxy' && effectiveTls === 'acme_hub') {
    message =
      'Reverse-proxy terminates TLS on the edge proxy. Use external (no cluster LE), not acme_hub.';
  } else {
    message = `Recommended TLS for ${effectiveExposure} is ${recommendedTls} (current effective: ${effectiveTls}).`;
  }

  return {
    effectiveExposure,
    effectiveTls,
    recommendedTls,
    message,
  };
}
