import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const AUTH_FIELD_HELP: Record<string, FieldHelpContent> = {
  'login.username': {
    description: 'UMKIM console username (LDAP or local auth depending on hub configuration).',
    example: 'linux',
    learnMoreUrl: DOCS.consoleAuth,
  },
  'login.password': {
    description: 'Account password for the selected authentication method.',
    example: '(your password)',
  },
  'llmApiKey.name': {
    description: 'Short key name used to identify the key in the console.',
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
