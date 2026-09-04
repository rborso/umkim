import { describe, expect, it } from 'vitest';
import { effectiveClusterJumphost } from './jumphost';

describe('effectiveClusterJumphost', () => {
  it('prefers cluster override over site default', () => {
    expect(effectiveClusterJumphost('cluster@1.2.3.4:22', 'site@5.6.7.8:22')).toEqual({
      effective: 'cluster@1.2.3.4:22',
      source: 'cluster',
    });
  });

  it('falls back to site jumphost', () => {
    expect(effectiveClusterJumphost('', 'site@5.6.7.8:22')).toEqual({
      effective: 'site@5.6.7.8:22',
      source: 'site',
    });
  });

  it('returns none when unset', () => {
    expect(effectiveClusterJumphost()).toEqual({ effective: '', source: 'none' });
  });
});
