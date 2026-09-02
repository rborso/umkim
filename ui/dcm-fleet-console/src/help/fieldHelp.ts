import { APPS_FIELD_HELP } from './apps';
import { AUTH_FIELD_HELP } from './auth';
import { CLUSTERS_FIELD_HELP } from './clusters';
import { INFRASTRUCTURE_FIELD_HELP } from './infrastructure';
import { SITES_FIELD_HELP } from './sites';

export type FieldHelpContent = {
  description: string;
  example?: string;
  examples?: string[];
  whenToUse?: string;
  commonMistakes?: string[];
  relatedFields?: string[];
  learnMoreUrl?: string;
};

const REGISTRY: Record<string, FieldHelpContent> = {
  ...APPS_FIELD_HELP,
  ...CLUSTERS_FIELD_HELP,
  ...SITES_FIELD_HELP,
  ...AUTH_FIELD_HELP,
  ...INFRASTRUCTURE_FIELD_HELP,
};

export function getFieldHelp(id: string): FieldHelpContent | undefined {
  return REGISTRY[id];
}
