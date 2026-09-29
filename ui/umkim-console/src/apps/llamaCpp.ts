export function isLlamaCppChart(chart: string): boolean {
  const base = chart.trim().toLowerCase().replace(/^oci:\/\//, '');
  const last = base.includes('/') ? base.slice(base.lastIndexOf('/') + 1) : base;
  const name = last.replace(/:.*/, '');
  return name === 'llama-cpp-helm';
}
