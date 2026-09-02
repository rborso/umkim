# UMKIM Fleet Console

The console is the React frontend in this directory. It serves the fleet UI
under `/fleet/` and uses the fleet-site and authentication proxy paths defined
by its source and deployment templates.

## Develop

```bash
cd ui/dcm-fleet-console
npm install
export FLEET_API_PROXY="http://127.0.0.1:30912"
npm run dev
```

`FLEET_API_PROXY` is the local Vite proxy target for `/fleet/api/` and
`/fleet/auth/`. Provide an authorized environment value; this README
intentionally supplies no production endpoint. `DCM_API_PROXY` is accepted as
a fallback name for the same variable.

## Build and test

```bash
npm run build
npm test
npm run test:e2e
```

Package scripts in [package.json](package.json) are the current contract.
End-to-end tests require deployment-specific access and credentials, so review
their configuration before running them.

## API and deployment boundaries

| Browser path | Contract |
|--------------|----------|
| `/fleet/api/` | Fleet-site API (sites, clusters, FleetApp `/apps`, chart library `/charts`) |
| `/fleet/auth/` | Console authentication proxy |
| `/llm/` | Optional authenticated LLM proxy |

The client code, nginx template, and OpenAPI sources define endpoint details:

- `src/api/`
- [nginx-default.conf.template](nginx-default.conf.template)
- [Fleet-site OpenAPI](../../controllers/fleet-site/openapi.yaml)

Helm installs use the console **Apps** page (`FleetApp`). Publish charts on
**Apps → Library**. Supported `route.mode` values: `none`, `hub`, `site`.
See [fleet-apps.md](../../docs/components/fleet-apps.md) and
[examples/site-profiles/](../../examples/site-profiles/) for site exposure.

Deploy through the console playbooks and `roles/fleet_webui/`; build and image
publication are controlled by the Harbor role. Resolve all hosts, registries,
tags, and credentials from authorized inventory and secret configuration.

The Harbor image name `dcm-fleet-console` is historical.

## Scope

This README documents the UI workflow only. It does not configure a backend,
ingress, authentication provider, or external endpoint.

## References

- [Console authentication guide](../../docs/fleet-console-auth.md)
- [Fleet-site component](../../docs/components/fleet-site.md)
- [FleetApp Helm installs](../../docs/components/fleet-apps.md)
- [Installation guide](../../docs/install.md)
- [Design guide](../../docs/design.md)
