import { describe, expect, it } from 'vitest';
import {
  bareMetalNodeHostVarsTemplate,
  guestHostVarsTemplate,
  mergeGuestIntoBareMetalsYaml,
  mergeGuestIntoGuestsYaml,
  templateForNewInventoryFile,
} from './inventoryVmTemplates';

describe('inventoryVmTemplates', () => {
  it('templates bare-metal host_vars with optional PXE profile', () => {
    const body = bareMetalNodeHostVarsTemplate({
      name: 'bm01',
      ansibleHost: '172.16.10.6',
      macAddress: '52:54:00:10:07:01',
      pxeDiscovery: true,
    });
    expect(body).toContain('tempxe_discovery:');
    expect(body).toContain('52:54:00:10:07:01');
  });

  it('merges a bare-metal node into bare_metals.yml', () => {
    const out = mergeGuestIntoBareMetalsYaml(
      'bare_metals:\n  hosts: {}\n',
      'bm01',
      { ansible_host: '203.0.113.11' },
    );
    expect(out).toContain('bm01:');
    expect(out).toContain('ansible_host: 203.0.113.11');
    expect(out).toContain('bare_metals:');
  });

  it('merges a guest into guests.yml', () => {
    const out = mergeGuestIntoGuestsYaml(
      'guests:\n  hosts: {}\n',
      'lab-vm-0',
      { vm_location: 'virt02', ansible_host: '172.16.10.50' },
    );
    expect(out).toContain('lab-vm-0:');
    expect(out).toContain('vm_location: virt02');
    expect(out).toContain('ansible_host: 172.16.10.50');
  });

  it('templates host_vars for a guest path', () => {
    const body = templateForNewInventoryFile('host_vars/my-guest.yml');
    expect(body).toContain('provisioning_provider: libvirt');
    expect(body).toContain('network_name: lab_nw');
  });

  it('renders host_vars with static IP when provided', () => {
    const body = guestHostVarsTemplate({
      guestName: 'g1',
      ip: '172.16.10.99',
    });
    expect(body).toContain('ip: "172.16.10.99"');
  });

  it('renders host_vars with custom sizing', () => {
    const body = guestHostVarsTemplate({
      guestName: 'g1',
      cores: 4,
      memory: 8192,
      diskSizeGi: 80,
      os: 'fedora',
    });
    expect(body).toContain('cores: 4');
    expect(body).toContain('memory: 8192');
    expect(body).toContain('size: 80');
    expect(body).toContain('os: fedora');
  });
});
