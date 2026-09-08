import { parse, stringify } from 'yaml';

export type VmGuestTemplateOptions = {
  guestName: string;
  vmLocation?: string;
  ansibleHost?: string;
  memory?: number;
  cores?: number;
  diskSizeGi?: number;
  os?: string;
  ip?: string;
  macaddress?: string;
};

type GuestsDoc = {
  guests?: { hosts?: Record<string, Record<string, string>> };
  external?: { hosts?: Record<string, Record<string, string>> };
};

const DEFAULT_GUESTS = `guests:
  hosts: {}
external:
  hosts: {}
`;

export function guestHostVarsTemplate(opts: VmGuestTemplateOptions): string {
  const memory = opts.memory ?? 4096;
  const cores = opts.cores ?? 2;
  const diskSize = opts.diskSizeGi ?? 40;
  const os = opts.os?.trim() || 'rocky-10';
  const ip = opts.ip?.trim() ?? '';
  const mac = opts.macaddress?.trim() || '52:54:00:00:00:01';
  return `---
# YAIDAP guest profile for ${opts.guestName}. Edit network IP/MAC before provision.
memory: ${memory}
cores: ${cores}
os: ${os}
provisioning_provider: libvirt
configure_firewalld: false
disks:
  - name: bootdisk
    disk: vda
    bootdisk: true
    device: 0
    format: qcow2
    size: ${diskSize}
    storage: local
uplinks:
  - name: n0p0
    physical_network: lab_access_l1
    mac_address: "${mac}"
    networks:
      - zone: lab
        network_name: lab_nw
        model: virtio
        cloudinit: true
        pxe: false
        vm_network: false
        hypervisor_uplink: n0p0
        bridge_name: n0p0_lab_nw
        ip: ${ip ? `"${ip}"` : '""'}
        default_route: true
        tcp_open:
          - 22
        udp_open: []
        services_open: []
`;
}

export function mergeGuestIntoGuestsYaml(
  existing: string,
  guestName: string,
  entry: { ansible_host?: string; vm_location?: string },
): string {
  const name = guestName.trim();
  if (!name) {
    throw new Error('guest name required');
  }
  let doc: GuestsDoc;
  try {
    doc = (parse(existing?.trim() ? existing : DEFAULT_GUESTS) as GuestsDoc) ?? {};
  } catch {
    throw new Error('guests.yml is not valid YAML — fix it before adding a guest');
  }
  if (!doc.guests) {
    doc.guests = { hosts: {} };
  }
  if (!doc.guests.hosts) {
    doc.guests.hosts = {};
  }
  if (!doc.external) {
    doc.external = { hosts: {} };
  }
  if (!doc.external.hosts) {
    doc.external.hosts = {};
  }
  const hostEntry: Record<string, string> = {};
  const loc = entry.vm_location?.trim() || 'least-utilized';
  hostEntry.vm_location = loc;
  const addr = entry.ansible_host?.trim();
  if (addr) {
    hostEntry.ansible_host = addr;
  }
  doc.guests.hosts[name] = hostEntry;
  return stringify(doc).trimEnd() + '\n';
}

export function templateForNewInventoryFile(path: string): string {
  const p = path.trim();
  const hvMatch = /^host_vars\/(.+)\.yml$/.exec(p);
  if (hvMatch) {
    return guestHostVarsTemplate({ guestName: hvMatch[1] });
  }
  const clusterMatch = /^group_vars\/(.+)\.yml$/.exec(p);
  if (clusterMatch && clusterMatch[1] !== 'all' && !clusterMatch[1].startsWith('all/')) {
    return `topology: single_node
autoApply: true
mode: enroll
`;
  }
  const rootCluster = /^([a-zA-Z0-9._-]+)\.yml$/.exec(p);
  if (
    rootCluster &&
    rootCluster[1] !== 'guests' &&
    rootCluster[1] !== 'hypervisors' &&
    rootCluster[1] !== 'clusters'
  ) {
    const group = rootCluster[1];
    return `${group}:
  hosts:
    ${group}-0:
`;
  }
  return '';
}

export function guestNameFromHostVarsPath(path: string): string | undefined {
  const m = /^host_vars\/(.+)\.yml$/.exec(path.trim());
  return m?.[1];
}
