import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const INFRASTRUCTURE_FIELD_HELP: Record<string, FieldHelpContent> = {
  'infra.hubIngressAuth': {
    description:
      'Enable Authelia forward-auth on the hub Traefik gateway for public app hostnames. Separate from fleet console login (Users / Groups). Requires public DNS for the auth portal host.',
    example: 'true',
    learnMoreUrl: DOCS.ingressAuth,
  },
  'infra.hubLldapWebUI': {
    description:
      'Expose the fleet-console LLDAP user-management UI over WireGuard only (not on the public internet). Requires console LLDAP deployed (fleet_auth_lldap_enabled).',
    example: 'false',
    learnMoreUrl: DOCS.consoleAuth,
  },
  'infra.hubAcme': {
    description:
      'Issue Let\'s Encrypt certificates on the hub Traefik gateway for hub-zone apps (*.hub zone). Needs public DNS and WAN :80/:443 to the hub host.',
    example: 'true',
    learnMoreUrl: DOCS.fleetEdge,
  },
  'infra.hubLlmEnabled': {
    description:
      'Deploy hub LiteLLM (OpenAI-compatible proxy). Spoke inference still comes from llama-cpp-helm on GPU clusters (fleet.capability/gpu=true).',
    example: 'false',
    learnMoreUrl: DOCS.fleetLlm,
  },
  'infra.hubLlmPublicHost': {
    description:
      'Public hostname for the LiteLLM API (HTTPS /v1/ with FleetLLMApiKey Bearer auth). Leave empty to use the in-cluster / WireGuard endpoint only.',
    example: 'llm.example.com',
    learnMoreUrl: DOCS.fleetLlm,
  },
  'infra.hubLlmBackends': {
    description:
      'Optional override for LiteLLM model backends (sets source to manual). JSON array with model and apiBase. Use Sync backends from FleetApps to rebuild from Ready llama-cpp installs.',
    example: '[{"model":"Qwen3.5-2B-UD","apiBase":"http://10.10.0.x:8080/v1"}]',
    learnMoreUrl: DOCS.fleetLlm,
  },
};
