import { describe, expect, it } from 'vitest';
import { DOCS, DOCS_BASE } from './docsUrls';
import { EXPOSURE_TLS_DOCS_URL } from '../edge/exposureTlsPairs';

describe('docsUrls', () => {
  it('uses public docs.umkim.com base', () => {
    expect(DOCS_BASE).toBe('https://docs.umkim.com');
    expect(EXPOSURE_TLS_DOCS_URL).toBe(DOCS.exposureTlsPairs);
    expect(EXPOSURE_TLS_DOCS_URL).toContain('docs.umkim.com');
    expect(EXPOSURE_TLS_DOCS_URL).toContain('#supported-working-pairs');
  });

  it('points exposure help at fleet-edge working pairs', () => {
    expect(DOCS.exposureTlsPairs).toBe(
      'https://docs.umkim.com/components/fleet-edge/#supported-working-pairs',
    );
    expect(DOCS.createSiteExposure).toContain('#exposure-profiles');
    expect(DOCS.createClusterEdge).toContain('#edge-exposure-optional');
  });
});
