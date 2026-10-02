Homelab subset verification (2026-10-02)
Automated: controllers/fleet-site, fleet-edge, cmd/umkim go test ./... -count=1 — pass
Layer-1 verify-fleet.sh --suite hub --profile homelab: 28 passed, 2 failed (ops.umkim-management-site; see copied report)
Manual B8/B9/B12/B13 node-op, rediscover, GPU, LiteLLM: requires hub API/WG from operator workstation per docs/fleet-verify.md
