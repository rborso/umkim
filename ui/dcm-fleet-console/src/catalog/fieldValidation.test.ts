import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import { validateField, validateDeployForm } from './fieldValidation';
import { resolveFieldTier, resolveFieldWidget } from './fieldUtils';

describe('fieldValidation', () => {
  it('requires required fields', () => {
    const field: CatalogFieldDef = {
      path: 'metadata.name',
      display_name: 'Release name',
      required: true,
    };
    expect(validateField(field, '').ok).toBe(false);
    expect(validateField(field, 'whoami-01').ok).toBe(true);
  });

  it('validates dns-label widget', () => {
    const field: CatalogFieldDef = {
      path: 'metadata.name',
      ui_widget: 'dns-label',
    };
    expect(validateField(field, 'INVALID_NAME').ok).toBe(false);
    expect(validateField(field, 'whoami-spoke-a').ok).toBe(true);
    expect(validateField(field, 'site-sec\t').ok).toBe(false);
  });

  it('rejects whitespace inside hostnames', () => {
    const field: CatalogFieldDef = {
      path: 'routing.site_host',
      ui_widget: 'hostname',
    };
    expect(validateField(field, 'site-sec .apps.umkim.com').ok).toBe(false);
    expect(validateField(field, 'site-sec.apps.umkim.com').ok).toBe(true);
  });

  it('validates domain widget', () => {
    const field: CatalogFieldDef = {
      path: 'values.fleet.ingress.baseDomain',
      ui_widget: 'domain',
    };
    expect(validateField(field, 'not a domain!').ok).toBe(false);
    expect(validateField(field, 'homelab.mgmt.umkim.com').ok).toBe(true);
  });

  it('validates yaml format', () => {
    const field: CatalogFieldDef = {
      path: 'values',
      ui_widget: 'yaml',
      validation_schema: { format: 'yaml' },
    };
    expect(validateField(field, 'key: [unclosed').ok).toBe(false);
    expect(validateField(field, 'key: value\n').ok).toBe(true);
  });

  it('allows spaces in SSH public keys', () => {
    const field: CatalogFieldDef = {
      path: 'access.ssh_public_key',
      display_name: 'SSH public key (cloud-init)',
      validation_schema: { type: 'string' },
    };
    const rsa =
      'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDDD2Ras6bni3y/ZjvGCEKPRvyWPyBb+tABG/fS11uSFDgByphnrvlgRBbW1MRjTJxOwet+6y4ZeDSksylRb12qFsh6TZJZ5ZtHAgqSZMCLjfZ0UjppiGvoeprcMSrr648Ij5eASezi1DKKDetpTAGwulg1DdLlINv+FBcLnnnm+WHT43RajRuYYm3fG8gZe+81XicnfprK5C76SLvgHmDH/MfLGwqj27cj6czcLn7zAwZ7HnhODJ1IcmkvC33/v2WaX/+6erUMHCZradNePGYKogsekZcmocF2GlyXQgjhnDKWOqY6vL5uRn1Bz7lOEaA+U4w9R7kbGpv8KNF8GXoSJO4LKJ02GCSkpJZIBbDL4bp2wpwjf9URA4hCf8Nqk0KBJHZQy/qV2Z3n81ZkqkBWsJ8fYaBVbhHRjnpDmnkbT8zkuBMxlemjsyqJzCbHYepqeLrQy8eWfcGcgcTIG3cVVvpMNkrkEBLfO19JgmBXxhEgu81EE8TYpzsEbL6+k4E=';
    expect(validateField(field, rsa).ok).toBe(true);
    expect(validateField(field, `${rsa} user@host`).ok).toBe(true);
    expect(validateField(field, 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJustAFakeKeyForUnitTestOnly0123456789abcde').ok).toBe(
      true,
    );
    expect(validateField(field, 'not-a-key').ok).toBe(false);
    expect(validateField(field, 'ssh-rsa').ok).toBe(false);
  });

  it('validateDeployForm returns field errors map', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', required: true, ui_tier: 'required', editable: true },
      { path: 'routing.cluster', ui_tier: 'advanced', editable: true },
    ];
    const { errors, message } = validateDeployForm(fields, {}, resolveFieldTier);
    expect(message).toBeTruthy();
    expect(errors['metadata.name']).toBeTruthy();
    expect(errors['routing.cluster']).toBeUndefined();
  });

  it('validates k8s-memory-quantity (Mi passes, MB fails)', () => {
    const field: CatalogFieldDef = {
      path: 'resources.memory.min',
      validation_schema: { type: 'string', format: 'k8s-memory-quantity' },
    };
    expect(validateField(field, '64Mi').ok).toBe(true);
    expect(validateField(field, '256Mi').ok).toBe(true);
    expect(validateField(field, '1Gi').ok).toBe(true);
    expect(validateField(field, '64MB').ok).toBe(false);
    expect(validateField(field, '').ok).toBe(true);
  });

  it('validates DCM container memory pattern (MB/GB/TB)', () => {
    const field: CatalogFieldDef = {
      path: 'resources.memory.min',
      validation_schema: { type: 'string', pattern: '^[0-9]+(MB|GB|TB)$' },
    };
    expect(validateField(field, '64MB').ok).toBe(true);
    expect(validateField(field, '256MB').ok).toBe(true);
    expect(validateField(field, '1GB').ok).toBe(true);
    expect(validateField(field, '64Mi').ok).toBe(false);
    expect(validateField(field, '').ok).toBe(true);
  });

  it('validates DCM container CPU integer (legacy schema)', () => {
    const field: CatalogFieldDef = {
      path: 'resources.cpu.min',
      validation_schema: { type: 'integer', minimum: 1 },
    };
    expect(validateField(field, '1').ok).toBe(true);
    expect(validateField(field, '2').ok).toBe(true);
    expect(validateField(field, '0').ok).toBe(false);
    expect(validateField(field, '500m').ok).toBe(false);
  });

  it('validates integer cpu cores for container catalog', () => {
    const field: CatalogFieldDef = {
      path: 'resources.cpu.min',
      validation_schema: { type: 'integer', minimum: 1 },
    };
    expect(validateField(field, '1').ok).toBe(true);
    expect(validateField(field, '2').ok).toBe(true);
    expect(validateField(field, '0').ok).toBe(false);
    expect(validateField(field, '500m').ok).toBe(false);
  });

  it('validates k8s-cpu-quantity (millicores and decimals)', () => {
    const field: CatalogFieldDef = {
      path: 'resources.cpu.min',
      validation_schema: { type: 'string', format: 'k8s-cpu-quantity' },
    };
    expect(validateField(field, '500m').ok).toBe(true);
    expect(validateField(field, '1').ok).toBe(true);
    expect(validateField(field, '0.5').ok).toBe(true);
    expect(validateField(field, '2').ok).toBe(true);
    expect(validateField(field, '0').ok).toBe(false);
    expect(validateField(field, '0m').ok).toBe(false);
    expect(validateField(field, 'invalid').ok).toBe(false);
    expect(validateField(field, '').ok).toBe(true);
  });

  it('infers memory validation from resources.memory path', () => {
    const field: CatalogFieldDef = { path: 'resources.memory.max' };
    expect(validateField(field, '128Mi').ok).toBe(true);
    expect(validateField(field, '128MB').ok).toBe(false);
  });

  it('uses number widget for integer cpu fields', () => {
    const field: CatalogFieldDef = {
      path: 'resources.cpu.max',
      validation_schema: { type: 'integer', minimum: 1, 'x-ui-widget': 'number' },
    };
    expect(resolveFieldWidget(field)).toBe('number');
  });

  it('infers cpu validation from resources.cpu path when string quantity', () => {
    const field: CatalogFieldDef = { path: 'resources.cpu.max' };
    expect(validateField(field, '1500m').ok).toBe(true);
    expect(validateField(field, 'cores').ok).toBe(false);
  });
});
