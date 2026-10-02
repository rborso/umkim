# BUG / gap: scripts/verify-fleet-llm.sh curls Fleet API /llm without auth

**Phase:** B12  
**Date:** 2026-10-02 (CEST)

## Symptom
`FLEET_LLM_EXPECT_ENABLED=true bash scripts/verify-fleet-llm.sh` failed with
`FAIL: Fleet API /llm unreachable` against both
`http://10.10.0.1:30912/fleet/api/v1alpha1` and `http://127.0.0.1:30912/...`
while `umkim llm get` (authenticated CLI) succeeded and LiteLLM pod was Ready.

Script uses bare `curl -sf "${API}/llm"` with no token.

## Workaround
Use `umkim llm get` + pod/NodePort health checks (as in assets/B12-verify-fleet-llm.txt).

## Expected
Script should use UMKIM_TOKEN / same auth as `umkim`, or document required auth env.
