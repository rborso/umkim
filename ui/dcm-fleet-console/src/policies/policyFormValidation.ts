import type { PolicyCreate } from '../api/types';
import { textToMatchLabels } from './placementForm';

export type PolicyFormValidationInput = PolicyCreate & {
  _matchLabelsText?: string;
};

export type PolicyFormValidationResult = {
  ok: boolean;
  errors: string[];
};

function invalidMatchLabelLines(text: string): string[] {
  const bad: string[] = [];
  for (const line of text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)) {
    const eq = line.indexOf('=');
    if (eq < 1) bad.push(line);
  }
  return bad;
}

export function validatePolicyForm(
  form: PolicyFormValidationInput,
  matchLabelsText: string,
): PolicyFormValidationResult {
  const errors: string[] = [];

  if (!form.display_name?.trim()) {
    errors.push('Display name is required');
  }
  if (!form.rego_code?.trim()) {
    errors.push('Rego code is required');
  }

  const badClusterLabels = invalidMatchLabelLines(matchLabelsText);
  if (badClusterLabels.length > 0) {
    errors.push(
      `Cluster match labels must be key=value (invalid: ${badClusterLabels.join(', ')})`,
    );
  }

  const labelSelectorText = form.label_selector
    ? Object.entries(form.label_selector)
        .map(([k, v]) => `${k}=${v}`)
        .join('\n')
    : '';
  const badInstanceLabels = invalidMatchLabelLines(labelSelectorText);
  if (badInstanceLabels.length > 0) {
    errors.push(
      `Instance label selector must be key=value (invalid: ${badInstanceLabels.join(', ')})`,
    );
  }

  const clusterCount = form.placement?.cluster_count ?? 1;
  const failoverMode = form.failover?.mode ?? 'manual';

  if (failoverMode === 'standby' && clusterCount < 2) {
    errors.push('Failover mode standby requires placement cluster_count ≥ 2');
  }
  if (failoverMode === 'relocatable' && clusterCount !== 1) {
    errors.push('Failover mode relocatable requires placement cluster_count = 1');
  }

  if (form.placement?.site_diversity && clusterCount < 2) {
    errors.push('Site diversity requires placement cluster_count ≥ 2');
  }

  const strategy = form.placement?.strategy;
  if (strategy && !['single', 'spread', 'all_matching', 'least_utilized'].includes(strategy)) {
    errors.push(`Invalid placement strategy: ${strategy}`);
  }

  // Ensure match label parser accepts the text (side effect: none if valid)
  if (matchLabelsText.trim() && !textToMatchLabels(matchLabelsText)) {
    errors.push('Cluster match labels could not be parsed');
  }

  return { ok: errors.length === 0, errors };
}
