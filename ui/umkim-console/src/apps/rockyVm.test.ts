import { describe, expect, it } from 'vitest';
import { isRocky10VmChart, parseRockyDns, rockyVmValuesYaml } from './rockyVm';

describe('isRocky10VmChart', () => {
  it('matches short and oci chart names', () => {
    expect(isRocky10VmChart('rocky-10-vm')).toBe(true);
    expect(isRocky10VmChart('oci://registry/charts/rocky-10-vm')).toBe(true);
    expect(isRocky10VmChart('oci://registry/charts/rocky-10-vm:0.1.3')).toBe(true);
    expect(isRocky10VmChart('whoami')).toBe(false);
  });
});

describe('rockyVmValuesYaml', () => {
  it('renders sizing only when no keys or network are given', () => {
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

  it('emits a lan network block with multus for DHCP (no staticIP)', () => {
    const yaml = rockyVmValuesYaml('2', '2Gi', '20Gi', '', {
      mode: 'lan',
      multusNetwork: 'default/mgmt-l2',
    });
    expect(yaml).toContain('network:');
    expect(yaml).toContain('  mode: lan');
    expect(yaml).toContain('  multusNetwork: "default/mgmt-l2"');
    expect(yaml).not.toContain('staticIP');
    expect(yaml).not.toContain('network: pod');
  });

  it('emits static lan addressing (interface, staticIP, gateway, dns)', () => {
    const yaml = rockyVmValuesYaml('2', '2Gi', '20Gi', '', {
      mode: 'lan',
      multusNetwork: 'default/mgmt-l2',
      interface: 'enp1s0',
      staticIP: '172.16.10.150/24',
      gateway: '172.16.10.1',
      dns: ['172.16.10.1', '1.1.1.1'],
    });
    expect(yaml).toContain('  interface: "enp1s0"');
    expect(yaml).toContain('  staticIP: "172.16.10.150/24"');
    expect(yaml).toContain('  gateway: "172.16.10.1"');
    expect(yaml).toContain('  dns:');
    expect(yaml).toContain('    - "172.16.10.1"');
    expect(yaml).toContain('    - "1.1.1.1"');
  });

  it('omits interface when staticIP is set but interface is blank', () => {
    const yaml = rockyVmValuesYaml('2', '2Gi', '20Gi', '', {
      mode: 'lan',
      staticIP: '172.16.10.150/24',
    });
    expect(yaml).toContain('  staticIP: "172.16.10.150/24"');
    expect(yaml).not.toContain('interface:');
  });

  it('omits the network block for pod mode or no network', () => {
    expect(rockyVmValuesYaml('2', '2Gi', '20Gi', '', { mode: 'pod' })).not.toContain('network:');
    expect(rockyVmValuesYaml('2', '2Gi', '20Gi')).not.toContain('network:');
  });
});

describe('parseRockyDns', () => {
  it('splits on commas and whitespace and drops blanks', () => {
    expect(parseRockyDns('172.16.10.1, 1.1.1.1  8.8.8.8')).toEqual([
      '172.16.10.1',
      '1.1.1.1',
      '8.8.8.8',
    ]);
    expect(parseRockyDns('  ')).toEqual([]);
  });
});
