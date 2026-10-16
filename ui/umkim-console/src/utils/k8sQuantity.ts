/** Kubernetes decimal SI multipliers for CPU quantities (base unit = 1 core). */
const CPU_SUFFIX_MULT: Record<string, number> = {
  n: 1e-9,
  u: 1e-6,
  m: 1e-3,
  '': 1,
  k: 1e3,
  M: 1e6,
  G: 1e9,
  T: 1e12,
  P: 1e15,
  E: 1e18,
};

/** Binary SI multipliers for memory quantities (base unit = 1 byte). */
const MEM_SUFFIX_MULT: Record<string, number> = {
  Ei: 2 ** 60,
  Pi: 2 ** 50,
  Ti: 2 ** 40,
  Gi: 2 ** 30,
  Mi: 2 ** 20,
  Ki: 2 ** 10,
  E: 1e18,
  P: 1e15,
  T: 1e12,
  G: 1e9,
  M: 1e6,
  K: 1e3,
  '': 1,
};

const CPU_QTY_RE = /^([+-]?(?:\d+\.?\d*|\.\d+))([numkMGTPE]?)$/;
const MEM_QTY_RE = /^([+-]?(?:\d+\.?\d*|\.\d+))([EPTGMK]i?)$/;

function parseQuantity(q: string, suffixMap: Record<string, number>): number | null {
  const trimmed = q.trim();
  if (!trimmed) return null;
  const m = trimmed.match(CPU_QTY_RE);
  if (!m) return null;
  const num = Number(m[1]);
  const suffix = m[2] ?? '';
  const mult = suffixMap[suffix];
  if (mult === undefined || Number.isNaN(num)) return null;
  return num * mult;
}

/** Parse a Kubernetes CPU quantity to millicores (1000m = 1 core). */
export function parseCpuToMillicores(q: string): number | null {
  const cores = parseQuantity(q, CPU_SUFFIX_MULT);
  if (cores === null) return null;
  return cores * 1000;
}

function formatCpuFromMillicores(mc: number): string {
  if (mc < 1000) {
    const rounded = Math.round(mc);
    return rounded > 0 ? `${rounded}m` : '1m';
  }
  const cores = mc / 1000;
  if (Number.isInteger(cores)) return String(cores);
  const rounded = Math.round(cores * 10) / 10;
  if (rounded < 10) return String(rounded);
  return String(Math.round(cores));
}

/** Human-friendly CPU display (e.g. 470582637n → 471m, 8 → 8). */
export function formatCpuDisplay(q: string): string {
  const mc = parseCpuToMillicores(q);
  if (mc === null) return q;
  return formatCpuFromMillicores(mc);
}

function parseMemoryBytes(q: string): number | null {
  const trimmed = q.trim();
  if (!trimmed) return null;
  const m = trimmed.match(MEM_QTY_RE);
  if (!m) return null;
  const num = Number(m[1]);
  const suffix = m[2] ?? '';
  const mult = MEM_SUFFIX_MULT[suffix];
  if (mult === undefined || Number.isNaN(num)) return null;
  return num * mult;
}

const MEM_DISPLAY_UNITS: Array<{ suffix: string; bytes: number }> = [
  { suffix: 'Ti', bytes: 2 ** 40 },
  { suffix: 'Gi', bytes: 2 ** 30 },
  { suffix: 'Mi', bytes: 2 ** 20 },
  { suffix: 'Ki', bytes: 2 ** 10 },
];

/** Human-friendly memory display (normalize to Gi/Mi/Ki). */
export function formatMemoryDisplay(q: string): string {
  const bytes = parseMemoryBytes(q);
  if (bytes === null) return q;
  for (const { suffix, bytes: unitBytes } of MEM_DISPLAY_UNITS) {
    if (bytes >= unitBytes || suffix === 'Ki') {
      const val = bytes / unitBytes;
      if (Number.isInteger(val)) return `${val}${suffix}`;
      const rounded = Math.round(val * 10) / 10;
      return `${rounded}${suffix}`;
    }
  }
  return q;
}

/** Format used/capacity pair for node resource columns. */
export function formatResourceUsagePair(
  used?: string,
  cap?: string,
  kind: 'cpu' | 'memory' = 'cpu',
): string {
  const fmt = kind === 'cpu' ? formatCpuDisplay : formatMemoryDisplay;
  if (!cap && !used) return '—';
  if (used) return `${fmt(used)} / ${cap ? fmt(cap) : '?'}`;
  return cap ? fmt(cap) : '—';
}

export type CpuQuantityUnit = 'millicores' | 'cores';

export type CpuQuantityParts = {
  amount: number;
  unit: CpuQuantityUnit;
};

/** Parse a k8s CPU quantity into editable parts (amount + unit). */
export function parseCpuQuantityParts(q: string): CpuQuantityParts {
  const trimmed = q.trim();
  if (!trimmed) return { amount: 500, unit: 'millicores' };
  const mc = parseCpuToMillicores(trimmed);
  if (mc === null) return { amount: 500, unit: 'millicores' };
  if (trimmed.endsWith('m') && !trimmed.includes('.')) {
    return { amount: Math.max(1, Math.round(mc)), unit: 'millicores' };
  }
  if (mc < 1000) {
    return { amount: Math.max(1, Math.round(mc)), unit: 'millicores' };
  }
  const cores = mc / 1000;
  if (Number.isInteger(cores)) return { amount: cores, unit: 'cores' };
  return { amount: Math.round(cores * 1000) / 1000, unit: 'cores' };
}

/** Serialize editable parts to a k8s CPU quantity string. */
export function formatCpuQuantityValue(parts: CpuQuantityParts): string {
  const amount = parts.amount;
  if (parts.unit === 'millicores') {
    const mc = Math.max(1, Math.round(amount));
    return `${mc}m`;
  }
  if (Number.isInteger(amount)) return String(amount);
  const rounded = Math.round(amount * 1000) / 1000;
  return String(rounded);
}
