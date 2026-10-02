# BUG-20261001-A5-chart-add-example-oci-overwrites-harbor

## Stage
A5 — chart library

## Summary
`examples/umkim/chart-add-oci.json` points at `oci://ghcr.io/example/charts/whoami`. Running `umkim chart add -f` against an already Harbor-seeded `whoami` FleetChart changed `source` from `harbor` to `oci` with that fake origin. Publish job `fc-p-whoami` then failed with GHCR 403, leaving chart phase **Failed**.

## Impact
Blocks app install until chart is deleted and re-imported from Harbor.

## Evidence
`assets/A5-chart-add-attempt.txt`, `assets/A6-fc-p-whoami-logs.txt`

## Suggested fix
- Reject chart add that replaces a Ready Harbor-imported chart with an unreachable origin, or require `--force`.
- Ship example JSON with a clearly fake name (`example-whoami`) so it cannot collide with seeded charts.
- Document that seeded Harbor charts should not be re-added via example OCI.
