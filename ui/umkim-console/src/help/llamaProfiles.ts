import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const LLAMA_PROFILES_FIELD_HELP: Record<string, FieldHelpContent> = {
  'llamaProfiles.profileYaml': {
    description:
      'Helm values overlay merged with the fleet llama-cpp runtime (GPU, hostNetwork, port 8080). ' +
      'The chart runs llama-server as: -hf hfRepo:hfQuant, --alias model, then each entry in extraArgs as extra CLI flags. ' +
      'You do not paste a full shell command; map -hf / --alias / --temp / --top-p into YAML below.',
    whenToUse:
      'Create or edit profiles on this page, or tweak generated values on Apps → Install Helm chart after picking a profile.',
    examples: [
      `# Model + OpenAI id (LiteLLM model_name must match model)
hfRepo: unsloth/Qwen3.5-9B-GGUF
hfQuant: UD-Q4_K_XL
model: Qwen3.5-9B-GGUF`,
      `# Server default sampling (pairs: flag then value)
extraArgs:
  - --temp
  - "0.6"
  - --top-p
  - "0.95"
  - --top-k
  - "20"
  - --min-p
  - "0.00"`,
      `# Qwen3 chat template (fleet often uses enable_thinking: false for console/LiteLLM)
jinja: true
chatTemplateKwargs: '{"enable_thinking":false}'`,
    ],
    commonMistakes: [
      'Changing listen port — fleet keeps llama-server on 8080 for hub reachability; do not mirror --port 8001 from local recipes.',
      'Omitting model — OpenAI /v1/models and LiteLLM need the model alias from values.',
      'enable_thinking: true without a client that reads reasoning_content — replies may look empty in verify chat.',
      'Clients can still override temperature and top_p per /v1/chat/completions request; extraArgs set server defaults only.',
    ],
    relatedFields: ['hfRepo', 'hfQuant', 'model', 'extraArgs', 'jinja', 'chatTemplateKwargs', 'ctxSize'],
    learnMoreUrl: DOCS.fleetLlm,
  },
};
