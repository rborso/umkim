import { describe, expect, it } from 'vitest';
import { chartInstallProfile } from './chartProfile';

describe('chartInstallProfile', () => {
  it('classifies rocky-10-vm as a built-in kubevirt chart without route', () => {
    expect(chartInstallProfile('rocky-10-vm')).toEqual({
      kind: 'kubevirt',
      builtIn: true,
      supportsRoute: false,
    });
    expect(chartInstallProfile('oci://10.10.0.1:30443/charts/rocky-10-vm').kind).toBe('kubevirt');
  });

  it('classifies llama-cpp-helm as built-in llm without route and namespace llm', () => {
    expect(chartInstallProfile('llama-cpp-helm')).toEqual({
      kind: 'llm',
      builtIn: true,
      supportsRoute: false,
      defaultNamespace: 'llm',
    });
  });

  it('classifies seeded web charts as built-in with route support', () => {
    expect(chartInstallProfile('whoami')).toMatchObject({
      kind: 'web',
      builtIn: true,
      supportsRoute: true,
    });
    expect(chartInstallProfile('pet-clinic-helm').kind).toBe('web');
    expect(chartInstallProfile('oci://registry/charts/pet-clinic-helm:0.1.1').kind).toBe('web');
  });

  it('falls back to a custom profile with optional route', () => {
    expect(chartInstallProfile('my-app')).toEqual({
      kind: 'custom',
      builtIn: false,
      supportsRoute: true,
    });
    expect(chartInstallProfile('oci://ghcr.io/example/my-app').builtIn).toBe(false);
  });
});
