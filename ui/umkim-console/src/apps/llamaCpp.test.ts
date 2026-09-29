import { describe, expect, it } from 'vitest';
import { isLlamaCppChart } from './llamaCpp';

describe('isLlamaCppChart', () => {
  it('matches short and oci chart names', () => {
    expect(isLlamaCppChart('llama-cpp-helm')).toBe(true);
    expect(isLlamaCppChart('oci://registry/charts/llama-cpp-helm')).toBe(true);
    expect(isLlamaCppChart('oci://registry/charts/llama-cpp')).toBe(true);
    expect(isLlamaCppChart('whoami')).toBe(false);
  });
});
