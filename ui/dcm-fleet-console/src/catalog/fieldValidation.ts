import { parse as parseYaml } from 'yaml';
import type { CatalogFieldDef } from '../api/types';
import { isFieldVisible } from './fieldVisibility';
import { isCatalogFieldEditable, isYamlLikeField, isFieldRequired } from './fieldUtils';

const DNS_LABEL_RE = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
const HOSTNAME_RE =
  /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/i;
const DOMAIN_RE = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/i;
const K8S_QUANTITY_RE = /^([0-9]+)([EPTGMK]i?)?$/;
const K8S_MEMORY_QUANTITY_RE = /^(\d+)(E|P|T|G|M|K)i$/;
const K8S_CPU_QUANTITY_RE = /^(\d+m|\d+(\.\d+)?)$/;
// OpenSSH authorized_keys line: type, base64 blob, optional comment (spaces allowed).
const SSH_PUBLIC_KEY_RE =
  /^(ssh-(rsa|ed25519|dss)|ecdsa-sha2-[A-Za-z0-9-]+) [A-Za-z0-9+/=]+( [^\r\n]+)?$/;

export type FieldValidationResult = { ok: true } | { ok: false; error: string };

function isEmpty(raw: string | undefined): boolean {
  return raw === undefined || raw.trim() === '';
}

/** Fields that legitimately contain spaces/newlines (SSH keys, YAML, free-form notes). */
function allowsInternalWhitespace(field: CatalogFieldDef): boolean {
  if (isYamlLikeField(field)) return true;
  const widget = field.ui_widget ?? (field.validation_schema?.['x-ui-widget'] as string | undefined);
  const format = field.validation_schema?.format;
  if (widget === 'yaml' || format === 'yaml' || widget === 'textarea') return true;
  if (widget === 'ssh-public-key' || format === 'ssh-public-key') return true;
  const path = field.path.toLowerCase();
  if (
    path.includes('ssh_public_key') ||
    path.includes('ssh-public-key') ||
    path.endsWith('.authorized_keys') ||
    path.endsWith('/authorized_keys')
  ) {
    return true;
  }
  return false;
}

function isSshPublicKeyField(field: CatalogFieldDef): boolean {
  const widget = field.ui_widget ?? (field.validation_schema?.['x-ui-widget'] as string | undefined);
  const format = field.validation_schema?.format;
  if (widget === 'ssh-public-key' || format === 'ssh-public-key') return true;
  const path = field.path.toLowerCase();
  return (
    path.includes('ssh_public_key') ||
    path.includes('ssh-public-key') ||
    path.endsWith('.authorized_keys') ||
    path.endsWith('/authorized_keys')
  );
}

function memoryFormatForField(field: CatalogFieldDef): string | undefined {
  const fmt = field.validation_schema?.format;
  if (fmt === 'k8s-memory-quantity') return fmt;
  if (field.path.startsWith('resources.memory.') && !field.validation_schema?.pattern) {
    return 'k8s-memory-quantity';
  }
  return undefined;
}

function cpuFormatForField(field: CatalogFieldDef): string | undefined {
  const fmt = field.validation_schema?.format;
  if (fmt === 'k8s-cpu-quantity') return fmt;
  if (
    field.path.startsWith('resources.cpu.') &&
    field.validation_schema?.type !== 'integer'
  ) {
    return 'k8s-cpu-quantity';
  }
  return undefined;
}

function validateFormat(
  format: string | undefined,
  widget: string | undefined,
  raw: string,
): FieldValidationResult {
  const fmt = format || widget;
  if (!fmt) return { ok: true };

  switch (fmt) {
    case 'dns-label':
      if (!DNS_LABEL_RE.test(raw)) {
        return { ok: false, error: 'Must be a valid DNS label (lowercase alphanumeric and hyphens)' };
      }
      return { ok: true };
    case 'hostname':
      if (!HOSTNAME_RE.test(raw)) {
        return { ok: false, error: 'Must be a valid hostname' };
      }
      return { ok: true };
    case 'domain':
      if (!DOMAIN_RE.test(raw)) {
        return { ok: false, error: 'Must be a valid domain name' };
      }
      return { ok: true };
    case 'k8s-memory-quantity':
      if (!K8S_MEMORY_QUANTITY_RE.test(raw)) {
        return {
          ok: false,
          error: 'Must be a Kubernetes memory quantity with binary suffix (e.g. 64Mi, 256Mi, 1Gi)',
        };
      }
      return { ok: true };
    case 'k8s-cpu-quantity':
      if (!K8S_CPU_QUANTITY_RE.test(raw)) {
        return {
          ok: false,
          error: 'Must be a CPU quantity (e.g. 1, 0.5, 500m)',
        };
      }
      if (raw === '0' || raw === '0m') {
        return { ok: false, error: 'Must be at least 1m (0.001 core)' };
      }
      return { ok: true };
    case 'k8s-quantity':
      if (!K8S_QUANTITY_RE.test(raw)) {
        return { ok: false, error: 'Must be a Kubernetes quantity (e.g. 512Mi, 2Gi)' };
      }
      return { ok: true };
    case 'yaml':
      try {
        parseYaml(raw);
        return { ok: true };
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'Invalid YAML';
        return { ok: false, error: `YAML syntax error: ${msg}` };
      }
    case 'ssh-public-key':
      if (!SSH_PUBLIC_KEY_RE.test(raw)) {
        return {
          ok: false,
          error: 'Must be an OpenSSH public key (e.g. ssh-ed25519 AAAA… or ssh-rsa AAAA…)',
        };
      }
      return { ok: true };
    default:
      return { ok: true };
  }
}

export function validateField(field: CatalogFieldDef, raw: string | undefined): FieldValidationResult {
  const schema = field.validation_schema;
  const widget = field.ui_widget;

  if (isFieldRequired(field) && isEmpty(raw)) {
    return { ok: false, error: `${field.display_name || field.path} is required` };
  }

  if (isEmpty(raw)) return { ok: true };

  // Trim for validation; allow trailing newlines on YAML / SSH keys.
  const value = raw!.trim();
  const isPassword = widget === 'password' || field.validation_schema?.['x-sensitive'] === true;
  const allowWs = allowsInternalWhitespace(field);
  if (raw !== value && !isPassword && !allowWs) {
    return { ok: false, error: 'Must not have leading or trailing spaces or tabs' };
  }
  // Most string fields are single tokens (DNS names, IPs). SSH keys and YAML need spaces.
  if (!isPassword && !allowWs && /\s/.test(value)) {
    return { ok: false, error: 'Must not contain spaces or tabs' };
  }
  if (isSshPublicKeyField(field)) {
    const sshResult = validateFormat('ssh-public-key', undefined, value);
    if (!sshResult.ok) return sshResult;
  }

  if (schema?.enum && schema.enum.length > 0) {
    if (!schema.enum.map(String).includes(value)) {
      return { ok: false, error: `Must be one of: ${schema.enum.map(String).join(', ')}` };
    }
  }

  if (schema?.minLength !== undefined && value.length < schema.minLength) {
    return { ok: false, error: `Must be at least ${schema.minLength} characters` };
  }
  if (schema?.maxLength !== undefined && value.length > schema.maxLength) {
    return { ok: false, error: `Must be at most ${schema.maxLength} characters` };
  }

  if (schema?.pattern) {
    try {
      const re = new RegExp(schema.pattern);
      if (!re.test(value)) {
        return { ok: false, error: 'Value does not match the required pattern' };
      }
    } catch {
      // ignore invalid pattern in catalog seed
    }
  }

  const type = schema?.type;
  const cpuFmt = cpuFormatForField(field);
  const memoryFmt = memoryFormatForField(field);

  if (cpuFmt) {
    const cpuResult = validateFormat(cpuFmt, undefined, value);
    if (!cpuResult.ok) return cpuResult;
  } else if (memoryFmt) {
    const memResult = validateFormat(memoryFmt, undefined, value);
    if (!memResult.ok) return memResult;
  } else if (type === 'integer') {
    const n = Number(value);
    if (!/^-?\d+$/.test(value) || !Number.isSafeInteger(n)) {
      return { ok: false, error: 'Must be an integer' };
    }
    if (schema?.minimum !== undefined && n < schema.minimum) {
      return { ok: false, error: `Must be at least ${schema.minimum}` };
    }
    if (schema?.maximum !== undefined && n > schema.maximum) {
      return { ok: false, error: `Must be at most ${schema.maximum}` };
    }
  } else if (type === 'array') {
    try {
      const parsed = JSON.parse(value);
      if (!Array.isArray(parsed)) {
        return { ok: false, error: 'Must be a JSON array' };
      }
    } catch {
      return { ok: false, error: 'Must be valid JSON array' };
    }
  } else if (type === 'object') {
    try {
      const parsed = JSON.parse(value);
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        return { ok: false, error: 'Must be a JSON object' };
      }
    } catch {
      return { ok: false, error: 'Must be valid JSON object' };
    }
  } else if (type === 'number') {
    const n = Number(value);
    if (Number.isNaN(n)) {
      return { ok: false, error: 'Must be a number' };
    }
    if (schema?.minimum !== undefined && n < schema.minimum) {
      return { ok: false, error: `Must be at least ${schema.minimum}` };
    }
    if (schema?.maximum !== undefined && n > schema.maximum) {
      return { ok: false, error: `Must be at most ${schema.maximum}` };
    }
  }

  if (widget === 'boolean' || type === 'boolean') {
    const lower = value.toLowerCase();
    if (!['true', 'false', '1', '0', ''].includes(lower)) {
      return { ok: false, error: 'Must be true or false' };
    }
  }

  if (!cpuFmt && !memoryFmt) {
    const formatResult = validateFormat(schema?.format, widget, value);
    if (!formatResult.ok) return formatResult;
  }

  if (isYamlLikeField(field) && field.path === 'values') {
    return validateFormat('yaml', 'yaml', value);
  }

  return { ok: true };
}

export function validateDeployForm(
  fields: CatalogFieldDef[],
  fieldValues: Record<string, string>,
  resolveTier: (field: CatalogFieldDef) => string,
): { errors: Record<string, string>; message: string | null } {
  const errors: Record<string, string> = {};
  for (const field of fields) {
    if (!isCatalogFieldEditable(field)) continue;
    const tier = resolveTier(field);
    if (tier === 'hidden') continue;
    if (!isFieldVisible(field, fieldValues, fields)) continue;
    const result = validateField(field, fieldValues[field.path]);
    if (!result.ok) {
      errors[field.path] = result.error;
    }
  }
  const firstPath = Object.keys(errors)[0];
  return {
    errors,
    message: firstPath ? errors[firstPath] : null,
  };
}
