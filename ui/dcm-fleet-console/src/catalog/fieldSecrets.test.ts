import { describe, expect, it } from 'vitest';
import type { CatalogFieldDef } from '../api/types';
import {
  applySecretBindsToValues,
  collectSecretRequests,
  isSensitiveField,
  redactUserValues,
  REDACTED_VALUE,
} from './fieldSecrets';
import { buildUserValues } from './fieldUtils';

describe('fieldSecrets', () => {
  const passwordField: CatalogFieldDef = {
    path: 'values.gitea.gitea.admin.password',
    editable: true,
    validation_schema: {
      'x-sensitive': true,
      'x-secret-bind': {
        secretName: '{{release}}-admin',
        secretKey: 'password',
        secretDataFrom: { username: 'values.gitea.gitea.admin.username' },
        valuesSet: { 'gitea.gitea.admin.existingSecret': '{{secretName}}' },
        valuesClear: ['gitea.gitea.admin.password'],
      },
    },
  };
  const usernameField: CatalogFieldDef = {
    path: 'values.gitea.gitea.admin.username',
    editable: true,
    default: 'fleet',
  };

  it('detects sensitive fields', () => {
    expect(isSensitiveField(passwordField)).toBe(true);
    expect(isSensitiveField({ path: 'values.foo', editable: true })).toBe(false);
  });

  it('collects spoke secret requests', () => {
    const reqs = collectSecretRequests(
      [usernameField, passwordField],
      {
        'values.gitea.gitea.admin.username': 'fleet',
        'values.gitea.gitea.admin.password': 'MySecretPass1',
      },
      'gitea-test',
      'lab-ext02',
      'gitea',
    );
    expect(reqs).toHaveLength(1);
    expect(reqs[0]).toMatchObject({
      cluster: 'lab-ext02',
      namespace: 'gitea',
      secretName: 'gitea-test-admin',
      data: { password: 'MySecretPass1', username: 'fleet' },
    });
  });

  it('includes username from catalog default when field left empty', () => {
    const reqs = collectSecretRequests(
      [usernameField, passwordField],
      { 'values.gitea.gitea.admin.password': 'MySecretPass1' },
      'gitea-test',
      'lab-ext02',
      'gitea-test',
    );
    expect(reqs[0]?.data).toMatchObject({
      password: 'MySecretPass1',
      username: 'fleet',
    });
  });

  it('applies secret bind templates to values blob', () => {
    const out = applySecretBindsToValues(
      [passwordField],
      { gitea: { gitea: { admin: { password: 'x' } } } },
      'rel1',
      { 'values.gitea.gitea.admin.password': 'secret' },
    );
    const admin = ((out.gitea as Record<string, unknown>).gitea as Record<string, unknown>)
      .admin as Record<string, unknown>;
    expect(admin.existingSecret).toBe('rel1-admin');
    expect(admin.password).toBeUndefined();
  });

  it('omits sensitive overlay from buildUserValues', () => {
    const fields: CatalogFieldDef[] = [
      { path: 'metadata.name', editable: true, default: 'rel1' },
      {
        path: 'values',
        default: { gitea: { gitea: { admin: { username: 'fleet' } } } },
      },
      passwordField,
    ];
    const uv = buildUserValues(fields, {
      'metadata.name': 'rel1',
      'values.gitea.gitea.admin.password': 'MySecretPass1',
    });
    const valuesEntry = uv.find((u) => u.path === 'values');
    expect(valuesEntry).toBeDefined();
    expect(typeof valuesEntry!.value).toBe('string');
    expect(valuesEntry!.value).toContain('existingSecret: rel1-admin');
    expect(valuesEntry!.value).not.toMatch(/password:\s*MySecretPass1/);
  });

  it('redacts sensitive paths in review display', () => {
    const redacted = redactUserValues(
      [{ path: 'values.gitea.gitea.admin.password', value: 'secret' }],
      [passwordField],
    );
    expect(redacted[0].value).toBe(REDACTED_VALUE);
  });
});
