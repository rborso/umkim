import type { FieldHelpContent } from './fieldHelp';

export const PROVIDERS_FIELD_HELP: Record<string, FieldHelpContent> = {
  'provider.name': {
    description: 'Unique provider identifier (DNS-like name) used in placement and SPRM routing.',
    example: 'helm-virt02',
  },
  'provider.displayName': {
    description: 'Human-readable provider name shown in the console.',
    example: 'Helm virt02',
  },
  'provider.serviceType': {
    description: 'Service type this provider handles: container, helm, operator, or cluster.',
    example: 'helm',
  },
  'provider.endpoint': {
    description: 'HTTP(S) base URL of the service provider API reachable from the DCM control plane.',
    example: 'http://10.10.0.10:30192',
  },
  'provider.zone': {
    description: 'Optional zone or site label for provider affinity.',
    example: 'homelab',
  },
  'provider.operations': {
    description: 'Comma-separated SPRM operations this provider supports.',
    example: 'CREATE, DELETE, READ',
  },
};
