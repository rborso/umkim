---
name: fleet-console-ui
description: >-
  Builds and maintains the DCM fleet console React UI and its deployment. Use for
  ui/dcm-fleet-console, its API client, fleet-console-auth.yml, nginx proxying,
  or console image workflows.
---

# Fleet console UI (S6)

## Scope

The console source is `ui/dcm-fleet-console/`. Its API client uses the DCM
`/api/` path and fleet `/fleet/api/` path; deployed proxy behavior is defined by
the UI nginx template and `roles/fleet_webui/`.

## Working guidance

- Use the package scripts in `ui/dcm-fleet-console/package.json` for local
  development, builds, and tests.
- Keep route handling, API client types, UI behavior, and the OpenAPI source in
  sync when an API contract changes.
- Deploy through the console and image playbooks using authorized inventory
  values. Do not document or hard-code an environment endpoint.
- The local proxy is a development convenience, not a substitute for deployed
  authentication and routing behavior.

## References

- [Console README](../../../ui/dcm-fleet-console/README.md)
- [Console authentication guide](../../../docs/fleet-console-auth.md)
- [Design guide](../../../docs/design.md)
- [Agent routing](../../../AGENTS.md)
