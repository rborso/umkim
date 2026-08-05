import type { CatalogFieldDef } from '../api/types';
import { AUTH_FIELD_HELP } from './auth';
import { CATALOG_COMMON_FIELD_HELP } from './catalogCommon';
import { CATALOG_EDITOR_FIELD_HELP } from './catalogEditor';
import { CLUSTERS_FIELD_HELP } from './clusters';
import { DEPLOY_WIZARD_FIELD_HELP } from './deployWizard';
import { DOMAINS_FIELD_HELP } from './domains';
import { INFRASTRUCTURE_FIELD_HELP } from './infrastructure';
import { INSTANCES_FIELD_HELP } from './instances';
import { POLICIES_FIELD_HELP } from './policies';
import { PROVIDERS_FIELD_HELP } from './providers';
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
  ...CATALOG_COMMON_FIELD_HELP,
  ...CATALOG_EDITOR_FIELD_HELP,
  ...DEPLOY_WIZARD_FIELD_HELP,
  ...CLUSTERS_FIELD_HELP,
  ...SITES_FIELD_HELP,
  ...POLICIES_FIELD_HELP,
  ...PROVIDERS_FIELD_HELP,
  ...DOMAINS_FIELD_HELP,
  ...AUTH_FIELD_HELP,
  ...INSTANCES_FIELD_HELP,
  ...INFRASTRUCTURE_FIELD_HELP,
};

export function getFieldHelp(id: string): FieldHelpContent | undefined {
  return REGISTRY[id];
}

/** Resolve help for catalog fields: API description wins, then registry by path. */
export function resolveCatalogFieldHelp(field: CatalogFieldDef): FieldHelpContent | undefined {
  if (field.description) {
    return {
      description: field.description,
      example: field.example,
      examples: field.examples,
    };
  }
  return getFieldHelp(field.path);
}
