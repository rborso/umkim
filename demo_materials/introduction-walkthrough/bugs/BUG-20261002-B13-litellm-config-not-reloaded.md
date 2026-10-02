# BUG: Hub LiteLLM reconcile Ready but pod keeps stale config.yaml until restart

**Phase:** B12/B13  
**Date:** 2026-10-02 (CEST)

## Symptom
After `umkim llm backends sync-from-apps` + `umkim llm reconcile --wait`, FleetLLM showed
`phase: Ready` and ConfigMap `fleet-llm/litellm-config` contained the correct
`Qwen3.5-2B-UD` backends (`http://10.10.0.10:8080/v1`, `http://10.10.0.14:8080/v1`).

The running LiteLLM pod still served the **bootstrap default**:

```yaml
model_list:
  - model_name: llama-cpp-demo
    litellm_params:
      model: openai/Qwen2.5-0.5B-Instruct
      api_base: http://127.0.0.1:8080/v1
```

`/v1/models` listed only `llama-cpp-demo`; hub chat to `Qwen3.5-2B-UD` failed with
invalid model name until `kubectl rollout restart deploy/litellm -n fleet-llm`.

## Workaround
Restart LiteLLM deployment after backend sync/reconcile when models do not match ConfigMap.

## Expected
Reconcile that updates the ConfigMap must roll the LiteLLM Deployment (or otherwise
reload config) so Ready implies the live proxy serves the new model_list.
