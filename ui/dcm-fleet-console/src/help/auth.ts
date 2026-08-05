import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const AUTH_FIELD_HELP: Record<string, FieldHelpContent> = {
  'login.username': {
    description: 'DCM console username (LDAP or local auth depending on hub configuration).',
    example: 'linux',
    learnMoreUrl: DOCS.consoleAuth,
  },
  'login.password': {
    description: 'Account password for the selected authentication method.',
    example: '(your password)',
  },
  'org.slug': {
    description: 'Stable org identifier (DNS label). Used as a prefix for customer cluster/site names and policy IDs.',
    example: 'acme',
    learnMoreUrl: DOCS.multiTenancy,
  },
  'org.displayName': {
    description: 'Human-readable organization name shown in the console.',
    example: 'Acme Corp',
  },
  'org.sharedCpuQuota': {
    description:
      'Shared-pool CPU quota in millicores for this org. Shared-pool placements count against this budget.',
    example: '4000',
    learnMoreUrl: `${DOCS.multiTenancy}#fleetorg`,
  },
  'member.username': {
    description: 'Existing console username to add to the organization.',
    example: 'alice',
    learnMoreUrl: `${DOCS.multiTenancy}#actors`,
  },
  'member.role': {
    description: 'Org role: customer (deploy workloads) or org-admin (manage org policies and members).',
    example: 'customer',
    examples: ['customer', 'org-admin'],
    learnMoreUrl: `${DOCS.multiTenancy}#actors`,
  },
  'llmApiKey.name': {
    description: 'Short key name; stored as {org}-{name}. Used to identify the key in the console.',
    example: 'ci-bot',
    learnMoreUrl: DOCS.fleetLlm,
  },
  'llmApiKey.displayName': {
    description: 'Optional friendly label for this API key.',
    example: 'CI pipeline',
  },
  'llmApiKey.models': {
    description: 'Optional comma-separated model allow-list. Empty allows all models exposed by hub LiteLLM.',
    example: 'Qwen2.5-0.5B-Instruct',
    learnMoreUrl: DOCS.fleetLlm,
  },
};
