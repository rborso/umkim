import { describe, expect, it } from 'vitest';
import {
  getExposureTlsPairIssue,
  recommendedTlsForExposure,
  resolveEffectiveExposure,
  resolveEffectiveTls,
} from './exposureTlsPairs';

describe('exposureTlsPairs', () => {
  it('recommends TLS per exposure', () => {
    expect(recommendedTlsForExposure('port_forward')).toBe('acme_cluster');
    expect(recommendedTlsForExposure('reverse_proxy')).toBe('external');
    expect(recommendedTlsForExposure('none')).toBe('acme_hub');
  });

  it('resolves inherit against site default', () => {
    expect(resolveEffectiveExposure('inherit', 'port_forward')).toBe('port_forward');
    expect(resolveEffectiveTls('inherit', 'reverse_proxy')).toBe('external');
  });

  it('accepts supported pairs', () => {
    expect(getExposureTlsPairIssue('port_forward', 'acme_cluster')).toBeNull();
    expect(getExposureTlsPairIssue('reverse_proxy', 'external')).toBeNull();
    expect(getExposureTlsPairIssue('none', 'acme_hub')).toBeNull();
    expect(getExposureTlsPairIssue('inherit', 'inherit', 'port_forward')).toBeNull();
  });

  it('warns on reverse_proxy + acme_cluster', () => {
    const issue = getExposureTlsPairIssue('reverse_proxy', 'acme_cluster');
    expect(issue?.recommendedTls).toBe('external');
    expect(issue?.message).toMatch(/conflicts/i);
  });

  it('warns on port_forward + external', () => {
    const issue = getExposureTlsPairIssue('port_forward', 'external');
    expect(issue?.recommendedTls).toBe('acme_cluster');
    expect(issue?.message).toMatch(/30443/);
  });

  it('warns on none + acme_cluster', () => {
    const issue = getExposureTlsPairIssue('none', 'acme_cluster');
    expect(issue?.message).toMatch(/no direct site ingress/i);
  });
});
