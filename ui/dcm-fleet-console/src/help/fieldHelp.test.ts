import { describe, expect, it } from 'vitest';
import { getFieldHelp } from './fieldHelp';

describe('getFieldHelp', () => {
  it('returns Apps cluster help', () => {
    const help = getFieldHelp('apps.clusterName');
    expect(help?.description).toMatch(/Ready workload cluster/i);
    expect(help?.example).toBe('lab-single-0');
  });

  it('returns chart library origin help', () => {
    const help = getFieldHelp('charts.origin');
    expect(help?.description).toMatch(/oci:\/\//i);
  });

  it('returns undefined for unknown keys', () => {
    expect(getFieldHelp('unknown.field')).toBeUndefined();
  });
});
