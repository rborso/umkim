# Fleet console authentication and RBAC

> Status: v0.2 is an experimental proof of concept, not a
> production-certified identity or authorization service.

`fleet-console-auth` authenticates console users and authorizes requests
proxied to the fleet-site API (sites, clusters, FleetApp, chart library) and the optional
LLM proxy.

## Public API access

The public fleet-site and LLM API surfaces require authenticated access.
Use the console or an approved API client that obtains a supported session or
service-account credential. Do not publish direct controller endpoints, bypass
the proxy, or place tokens in documentation, shell history, or source control.

Enrollment is a separate bootstrap flow and uses its own bearer token; it is
not an anonymous public API.

## Identity providers

The chart can be configured for:

- a vault-backed local htpasswd account for break-glass use;
- LLDAP/LDAP for console users; or
- an OIDC configuration. The current OIDC exchange implementation is a stub,
  so do not represent it as a complete SSO integration.

For directory setup and the distinction between console and ingress-auth
LLDAP, see [lldap-bootstrap.md](lldap-bootstrap.md).

## Roles and organization scope

The service seeds these Fleet roles:

| Role | Intended scope |
|---|---|
| `dc-admin` | Full platform administration |
| `site-admin` | Site, cluster, FleetApp, and chart library operations |
| `cluster-admin` | Cluster and FleetApp operations; chart library is read-only |
| `org-admin` | One organization, including delegated membership and policy work; chart library is read-only |
| `customer` | Organization-scoped site, cluster, and app access; chart library is read-only |

Role bindings and organization scopes are enforced by the proxy before requests
reach the upstream services. See [multi-tenancy.md](multi-tenancy.md) for the
organization model and its limits.

## Operational guidance

- Bootstrap the first administrator through the supported installation flow;
  keep bootstrap credentials in the encrypted vault.
- Create narrowly scoped service accounts for automation and store their
  credentials in an approved secret manager.
- Use HTTPS and secure session cookies for any browser-accessible deployment.
- Rotate break-glass and automation credentials, and audit access after use.

Firewall, TLS, and external access behavior depend on the deployed inventory
and network policy. This release does not guarantee that those concerns are
fully solved; verify the live deployment before exposing the console.

See [install.md](install.md), [secrets.md](secrets.md), and
[fleet-verify.md](fleet-verify.md) for deployment and verification guidance.
