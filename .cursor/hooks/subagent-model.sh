#!/usr/bin/env bash
# Deny Task subagents unless model is composer-2.5 (not fast).
set -euo pipefail
input=$(cat)
model=""
if command -v jq >/dev/null 2>&1; then
  model=$(echo "$input" | jq -r '.model // .subagent_model // empty' 2>/dev/null || true)
fi
if [[ -z "$model" ]] || [[ "$model" == "composer-2.5-fast" ]]; then
  printf '%s\n' '{"permission":"deny","user_message":"Task subagents must use composer-2.5 (not composer-2.5-fast). See AGENTS.md model policy."}'
  exit 0
fi
printf '%s\n' '{"permission":"allow"}'
