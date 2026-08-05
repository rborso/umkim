# DCM Fleet Console

The console is the React frontend in this directory. It serves the fleet UI
under `/fleet/` and uses the DCM, fleet, and authentication proxy paths defined
by its source and deployment templates.

## Develop

```bash
cd ui/dcm-fleet-console
npm install
export DCM_API_PROXY="https://<api-host>"
npm run dev
```

`DCM_API_PROXY` supplies the local development proxy target for `/api/`,
`/fleet/api/`, and `/fleet/auth/`. Provide an authorized environment value;
this README intentionally supplies no default endpoint.

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
| `/api/` | DCM API |
| `/fleet/api/` | Fleet-site API |
| `/fleet/auth/` | Console authentication proxy |

The client code, nginx template, and OpenAPI sources define endpoint details:

- `src/api/`
- [nginx-default.conf.template](nginx-default.conf.template)
- [Fleet-site OpenAPI](../../controllers/fleet-site/openapi.yaml)

Deploy through the console playbooks and `roles/fleet_webui/`; build and image
publication are controlled by the Harbor role. Resolve all hosts, registries,
tags, and credentials from authorized inventory and secret configuration.

## Scope

This README documents the UI workflow only. It does not configure a backend,
ingress, authentication provider, or external endpoint.

## References

- [Console authentication guide](../../docs/fleet-console-auth.md)
- [Fleet-site component](../../docs/components/fleet-site.md)
- [Installation guide](../../docs/install.md)
- [Design guide](../../docs/design.md)
