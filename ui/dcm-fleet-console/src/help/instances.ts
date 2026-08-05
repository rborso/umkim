import type { FieldHelpContent } from './fieldHelp';

export const INSTANCES_FIELD_HELP: Record<string, FieldHelpContent> = {
  'instance.rollbackRevision': {
    description: 'Helm revision number to roll back to. List revisions in the instance Helm history panel.',
    example: '1',
  },
  'instance.upgradeValues': {
    description: 'YAML values merged into the current Helm release on upgrade (same format as deploy values).',
    example: 'replicaCount: 2\nimage:\n  tag: latest',
  },
};
