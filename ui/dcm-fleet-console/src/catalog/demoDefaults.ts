/** Demo catalog items — placement policies match fleet.workload/profile + fleet.placement/scope. */

export const DEMO_CATALOG_ITEM_IDS = new Set(['demo-webapp', 'whoami-helm']);

/** KubeVirt VM catalog items — match demo-vm-single (profile=kubevirt-test). */
export const VM_CATALOG_ITEM_IDS = new Set(['vm-test', 'rocky9-vm']);

export const DEMO_WORKLOAD_PROFILE_KEY = 'fleet.workload/profile';
export const DEMO_WORKLOAD_PROFILE = 'demo';
export const VM_WORKLOAD_PROFILE = 'kubevirt-test';

export const DEMO_PLACEMENT_SCOPE_KEY = 'fleet.placement/scope';
export const DEMO_PLACEMENT_SCOPE_SINGLE = 'single';
export const DEMO_PLACEMENT_SCOPE_HA = 'ha-standby';

export const DEMO_PLACEMENT_PICKER_KEY = 'fleet.placement/picker';
export const DEMO_PLACEMENT_PICKER_LEAST_UTIL = 'least-utilized';

/** Console placement step: where and how to pick clusters. */
export type DeployPlacementOption = 'single' | 'least_utilized' | 'ha_standby';

export function demoInstanceLabelSelector(
  option: DeployPlacementOption = 'single',
): Record<string, string> {
  const labels: Record<string, string> = {
    [DEMO_WORKLOAD_PROFILE_KEY]: DEMO_WORKLOAD_PROFILE,
    [DEMO_PLACEMENT_SCOPE_KEY]:
      option === 'ha_standby' ? DEMO_PLACEMENT_SCOPE_HA : DEMO_PLACEMENT_SCOPE_SINGLE,
  };
  if (option === 'least_utilized') {
    labels[DEMO_PLACEMENT_PICKER_KEY] = DEMO_PLACEMENT_PICKER_LEAST_UTIL;
  }
  return labels;
}

/** Labels for rocky9-vm / vm-test (demo-vm-single policy). */
export function vmInstanceLabelSelector(
  option: DeployPlacementOption = 'single',
): Record<string, string> {
  return {
    [DEMO_WORKLOAD_PROFILE_KEY]: VM_WORKLOAD_PROFILE,
    [DEMO_PLACEMENT_SCOPE_KEY]:
      option === 'ha_standby' ? DEMO_PLACEMENT_SCOPE_HA : DEMO_PLACEMENT_SCOPE_SINGLE,
  };
}

/** Console default: single cluster (demo-container-single / demo-helm-single). */
export const DEMO_INSTANCE_LABEL_SELECTOR: Record<string, string> =
  demoInstanceLabelSelector('single');

export function isDemoCatalogItem(catalogItemId: string): boolean {
  return DEMO_CATALOG_ITEM_IDS.has(catalogItemId);
}

export function isVmCatalogItem(catalogItemId: string): boolean {
  return VM_CATALOG_ITEM_IDS.has(catalogItemId);
}

export function defaultLabelSelectorForCatalog(
  catalogItemId: string,
  initial?: Record<string, string>,
): Record<string, string> | undefined {
  if (initial && Object.keys(initial).length > 0) {
    return initial;
  }
  if (isVmCatalogItem(catalogItemId)) {
    return vmInstanceLabelSelector('single');
  }
  return isDemoCatalogItem(catalogItemId) ? DEMO_INSTANCE_LABEL_SELECTOR : undefined;
}
