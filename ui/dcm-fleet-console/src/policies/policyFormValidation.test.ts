import { describe, expect, it } from 'vitest';
import { validatePolicyForm } from './policyFormValidation';

describe('validatePolicyForm', () => {
  const base = {
    display_name: 'Test policy',
    policy_type: 'GLOBAL' as const,
    rego_code: 'package dcm\n',
    placement: { cluster_count: 1, strategy: 'single' as const },
    failover: { mode: 'manual' as const },
  };

  it('requires display name and rego', () => {
    const result = validatePolicyForm(
      { ...base, display_name: '', rego_code: '' },
      '',
    );
    expect(result.ok).toBe(false);
    expect(result.errors.length).toBeGreaterThanOrEqual(2);
  });

  it('rejects standby when cluster_count < 2', () => {
    const result = validatePolicyForm(
      {
        ...base,
        placement: { cluster_count: 1, strategy: 'single' },
        failover: { mode: 'standby' },
      },
      '',
    );
    expect(result.ok).toBe(false);
    expect(result.errors.some((e) => e.includes('standby'))).toBe(true);
  });

  it('rejects relocatable when cluster_count !== 1', () => {
    const result = validatePolicyForm(
      {
        ...base,
        placement: { cluster_count: 2, strategy: 'spread' },
        failover: { mode: 'relocatable' },
      },
      '',
    );
    expect(result.ok).toBe(false);
    expect(result.errors.some((e) => e.includes('relocatable'))).toBe(true);
  });

  it('passes valid minimal form', () => {
    const result = validatePolicyForm(base, 'fleet.site=homelab');
    expect(result.ok).toBe(true);
    expect(result.errors).toEqual([]);
  });

  it('rejects malformed match label lines', () => {
    const result = validatePolicyForm(base, 'not-a-valid-line');
    expect(result.ok).toBe(false);
  });
});
