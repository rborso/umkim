export function chartBaseName(chart: string): string {
  const trimmed = chart.trim().toLowerCase();
  const noScheme = trimmed.replace(/^oci:\/\//, '');
  const last = noScheme.includes('/') ? noScheme.slice(noScheme.lastIndexOf('/') + 1) : noScheme;
  return last.replace(/:.*/, '');
}

export function isRocky10VmChart(chart: string): boolean {
  return chartBaseName(chart) === 'rocky-10-vm';
}

export type RockyVmNetworkMode = 'pod' | 'lan';

export type RockyVmNetwork = {
  mode: RockyVmNetworkMode;
  /** Multus NetworkAttachmentDefinition as namespace/name (lan mode). */
  multusNetwork?: string;
  /** Guest NIC name (lan mode; static config). */
  interface?: string;
  /** Static IPv4 CIDR, e.g. 172.16.10.150/24. Empty = DHCP. */
  staticIP?: string;
  gateway?: string;
  dns?: string[];
};

function parseSshAuthorizedKeys(sshAuthorizedKeys: string): string[] {
  return sshAuthorizedKeys
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0 && !line.startsWith('#'));
}

function splitCsv(value: string): string[] {
  return value
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
}

function yamlScalar(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

/**
 * Build the Helm values YAML for the seeded rocky-10-vm chart from the
 * structured install-dialog fields.
 *
 * - `sshAuthorizedKeys` is one public key per line; a cloudInit block is only
 *   emitted when at least one key is present.
 * - `network` is only emitted for `lan` mode (pod/masquerade is the chart
 *   default). Static addressing is written by the chart as a NetworkManager
 *   keyfile inside the guest; leave `staticIP` empty for DHCP on the L2.
 */
export function rockyVmValuesYaml(
  cpu: string,
  memory: string,
  diskSize: string,
  sshAuthorizedKeys = '',
  network?: RockyVmNetwork,
): string {
  const cores = Number.parseInt(cpu, 10);
  const lines = [
    `cpu: ${Number.isFinite(cores) && cores > 0 ? cores : 2}`,
    `memory: ${memory.trim() || '2Gi'}`,
    'disk:',
    `  size: ${diskSize.trim() || '20Gi'}`,
  ];
  const keys = parseSshAuthorizedKeys(sshAuthorizedKeys);
  if (keys.length > 0) {
    lines.push('cloudInit:', '  sshAuthorizedKeys:');
    for (const key of keys) {
      lines.push(`    - ${yamlScalar(key)}`);
    }
  }
  if (network && network.mode === 'lan') {
    lines.push('network:', '  mode: lan');
    const multusNetwork = network.multusNetwork?.trim();
    if (multusNetwork) {
      lines.push(`  multusNetwork: ${yamlScalar(multusNetwork)}`);
    }
    const staticIP = network.staticIP?.trim();
    if (staticIP) {
      const iface = network.interface?.trim();
      if (iface) {
        lines.push(`  interface: ${yamlScalar(iface)}`);
      }
      lines.push(`  staticIP: ${yamlScalar(staticIP)}`);
      const gateway = network.gateway?.trim();
      if (gateway) {
        lines.push(`  gateway: ${yamlScalar(gateway)}`);
      }
      const dns = (network.dns ?? []).map((d) => d.trim()).filter((d) => d.length > 0);
      if (dns.length > 0) {
        lines.push('  dns:');
        for (const server of dns) {
          lines.push(`    - ${yamlScalar(server)}`);
        }
      }
    }
  }
  return lines.join('\n');
}

/** Split a free-form DNS field (comma/space separated) into server addresses. */
export function parseRockyDns(value: string): string[] {
  return splitCsv(value);
}
