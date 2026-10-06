export function chartBaseName(chart: string): string {
  const trimmed = chart.trim().toLowerCase();
  const noScheme = trimmed.replace(/^oci:\/\//, '');
  const last = noScheme.includes('/') ? noScheme.slice(noScheme.lastIndexOf('/') + 1) : noScheme;
  return last.replace(/:.*/, '');
}

export function isRocky10VmChart(chart: string): boolean {
  return chartBaseName(chart) === 'rocky-10-vm';
}

function parseSshAuthorizedKeys(sshAuthorizedKeys: string): string[] {
  return sshAuthorizedKeys
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0 && !line.startsWith('#'));
}

function yamlScalar(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

/**
 * Build the Helm values YAML for the seeded rocky-10-vm chart from the
 * structured install-dialog fields. `sshAuthorizedKeys` is one public key per
 * line; a cloudInit block is only emitted when at least one key is present.
 */
export function rockyVmValuesYaml(
  cpu: string,
  memory: string,
  diskSize: string,
  sshAuthorizedKeys = '',
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
  return lines.join('\n');
}
