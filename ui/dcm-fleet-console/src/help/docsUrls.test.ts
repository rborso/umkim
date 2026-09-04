import { describe, expect, it } from 'vitest';
import { DOCS, DOCS_BASE } from './docsUrls';

describe('docsUrls', () => {
  it('uses public docs.umkim.com base', () => {
    expect(DOCS_BASE).toBe('https://docs.umkim.com');
    expect(DOCS.exposureTlsPairs).toContain('docs.umkim.com');
    expect(DOCS.exposureTlsPairs).toContain('#supported-working-pairs');
  });

  it('points exposure help at fleet-edge working pairs', () => {
    expect(DOCS.exposureTlsPairs).toBe(
      'https://docs.umkim.com/components/fleet-edge/#supported-working-pairs',
    );
    expect(DOCS.createSiteExposure).toContain('#exposure-profiles');
    expect(DOCS.createClusterEdge).toContain('#edge-exposure-optional');
  });
});
