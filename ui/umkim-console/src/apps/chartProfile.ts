import { isLlamaCppChart } from './llamaCpp';
import { chartBaseName, isRocky10VmChart } from './rockyVm';

export type ChartKind = 'kubevirt' | 'llm' | 'web' | 'custom';

export type ChartProfile = {
  kind: ChartKind;
  /** True for the curated/seeded charts with a bespoke install UI. */
  builtIn: boolean;
  /** Whether the fleet route (mode + hostname) applies to this chart. */
  supportsRoute: boolean;
  /** Namespace to default to when this chart is selected. */
  defaultNamespace?: string;
};

/** Seeded charts that expose a Service and therefore support a fleet route. */
const WEB_CHARTS = new Set(['whoami', 'pet-clinic-helm']);

/**
 * Classify a chart for the install dialog. Built-in charts get curated fields
 * and hide controls that do not apply (for example Route for a KubeVirt guest);
 * everything else is a custom chart with generic fields and an optional route.
 */
export function chartInstallProfile(chart: string): ChartProfile {
  if (isRocky10VmChart(chart)) {
    return { kind: 'kubevirt', builtIn: true, supportsRoute: false };
  }
  if (isLlamaCppChart(chart)) {
    return { kind: 'llm', builtIn: true, supportsRoute: false, defaultNamespace: 'llm' };
  }
  if (WEB_CHARTS.has(chartBaseName(chart))) {
    return { kind: 'web', builtIn: true, supportsRoute: true };
  }
  return { kind: 'custom', builtIn: false, supportsRoute: true };
}
