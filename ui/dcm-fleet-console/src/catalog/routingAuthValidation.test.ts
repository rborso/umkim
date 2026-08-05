import { describe, expect, it } from 'vitest';
import {
  isRequireAuthFieldVisible,
  primaryClusterFromPreview,
  validateRequireAuthSync,
} from './routingAuthValidation';

describe('routingAuthValidation', () => {
  it('shows require_auth only for hub and site', () => {
    expect(isRequireAuthFieldVisible('hub')).toBe(true);
    expect(isRequireAuthFieldVisible('site')).toBe(true);
    expect(isRequireAuthFieldVisible('dual')).toBe(false);
    expect(isRequireAuthFieldVisible('none')).toBe(false);
  });

  it('rejects require_auth for dual', () => {
    expect(
      validateRequireAuthSync({ 'routing.mode': 'dual', 'routing.require_auth': 'true' }),
    ).toMatch(/dual/);
  });

  it('allows require_auth false for dual', () => {
    expect(
      validateRequireAuthSync({ 'routing.mode': 'dual', 'routing.require_auth': 'false' }),
    ).toBeNull();
  });

  it('reads primary cluster from preview', () => {
    expect(
      primaryClusterFromPreview({
        eligible_clusters: [{ name: 'virt01' }, { name: 'virt02' }],
      }),
    ).toBe('virt01');
  });
});
