---
name: fleet-console-ui
description: >-
  Builds and maintains the UMKIM console React UI and its deployment.
  Use for ui/umkim-console, its API client, fleet-console-auth.yml, nginx
  proxying, or console image workflows.
---

# UMKIM console (S6)

## Scope

The console source is `ui/umkim-console/`. Its API client uses
`/fleet/api/` for fleet-site (including FleetApp) and `/fleet/auth/` for
login. Deployed proxy behavior is defined by the UI nginx template
and `roles/fleet_webui/`.

## Working guidance

- Use the package scripts in `ui/umkim-console/package.json` for local
  development, builds, and tests.
- Keep route handling, API client types, UI behavior, and the OpenAPI source in
  sync when an API contract changes.
- Helm installs belong on the **Apps** page (`FleetApp`). Chart publish belongs
  on **Apps → Library** (`FleetChart`), not a catalog UI.
- Deploy through the console and image playbooks using authorized inventory
  values. Do not document or hard-code an environment endpoint.
- The local proxy is a development convenience, not a substitute for deployed
  authentication and routing behavior.

## References

- [Console README](../../../ui/umkim-console/README.md)
- [Console authentication guide](../../../docs/fleet-console-auth.md)
- [FleetApp Helm installs](../../../docs/components/fleet-apps.md)
- [Design guide](../../../docs/design.md)
- [Agent routing](../../../AGENTS.md)
