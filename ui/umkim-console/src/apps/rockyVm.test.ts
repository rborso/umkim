import { describe, expect, it } from 'vitest';
import { isRocky10VmChart, rockyVmValuesYaml } from './rockyVm';

describe('isRocky10VmChart', () => {
  it('matches short and oci chart names', () => {
    expect(isRocky10VmChart('rocky-10-vm')).toBe(true);
    expect(isRocky10VmChart('oci://registry/charts/rocky-10-vm')).toBe(true);
    expect(isRocky10VmChart('oci://registry/charts/rocky-10-vm:0.1.3')).toBe(true);
    expect(isRocky10VmChart('whoami')).toBe(false);
  });
});

describe('rockyVmValuesYaml', () => {
  it('renders sizing only when no keys are given', () => {
    expect(rockyVmValuesYaml('2', '2Gi', '20Gi')).toBe(
      ['cpu: 2', 'memory: 2Gi', 'disk:', '  size: 20Gi'].join('\n'),
    );
  });

  it('falls back to defaults for invalid sizing', () => {
    const fallback = ['cpu: 2', 'memory: 2Gi', 'disk:', '  size: 20Gi'].join('\n');
    expect(rockyVmValuesYaml('', '', '')).toBe(fallback);
    expect(rockyVmValuesYaml('abc', '   ', '')).toBe(fallback);
  });

  it('adds a cloudInit sshAuthorizedKeys block for one key', () => {
    const yaml = rockyVmValuesYaml('4', '8Gi', '40Gi', 'ssh-ed25519 AAAA1 user@a');
    expect(yaml).toContain('cloudInit:');
    expect(yaml).toContain('  sshAuthorizedKeys:');
    expect(yaml).toContain('    - "ssh-ed25519 AAAA1 user@a"');
  });

  it('supports multiple keys and ignores blank lines', () => {
    const yaml = rockyVmValuesYaml('2', '2Gi', '20Gi', '\n ssh-rsa BBBB  \n\nssh-ed25519 CCCC\n');
    expect(yaml).toContain('    - "ssh-rsa BBBB"');
    expect(yaml).toContain('    - "ssh-ed25519 CCCC"');
    expect(yaml.match(/^ {4}- /gm)).toHaveLength(2);
  });

  it('omits cloudInit when keys are empty or blank', () => {
    expect(rockyVmValuesYaml('2', '2Gi', '20Gi', '   \n  ')).not.toContain('cloudInit');
  });

  it('escapes quotes in key comments', () => {
    const yaml = rockyVmValuesYaml('2', '2Gi', '20Gi', 'ssh-ed25519 AAAA "weird"');
    expect(yaml).toContain('\\"weird\\"');
  });
});
