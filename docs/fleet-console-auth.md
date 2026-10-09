# UMKIM console authentication and RBAC

> Status: v0.2 is an experimental proof of concept, not a
> production-certified identity or authorization service.

`fleet-console-auth` authenticates UMKIM console users and authorizes requests
proxied to the fleet-site API (sites, clusters, FleetApp, chart library) and the optional
LLM proxy.

## Public API access

The public fleet-site and LLM API surfaces require authenticated access.
Use the UMKIM console or an approved API client that obtains a supported session or
service-account credential. Do not publish direct controller endpoints, bypass
the proxy, or place tokens in documentation, shell history, or source control.

Enrollment is a separate bootstrap flow and uses its own bearer token; it is
not an anonymous public API.

One deliberate exception is invite redemption: `POST /api/v1alpha1/invites/redeem`
is reachable without a session because the one-time invite token is the
credential. Every other fleet API path still requires an authenticated,
authorized session. Administrators mint invites from **Access control →
Invitations**; see [lldap-bootstrap.md](lldap-bootstrap.md).

## Identity providers

The chart can be configured for:

- a vault-backed local htpasswd account for break-glass use;
- LLDAP/LDAP for console users; or
- an OIDC configuration. The current OIDC exchange implementation is a stub,
  so do not represent it as a complete SSO integration.

For directory setup and the distinction between console and ingress-auth
LLDAP, see [lldap-bootstrap.md](lldap-bootstrap.md).

## Roles

The service seeds these Fleet roles:

| Role | Intended scope |
|---|---|
| `dc-admin` | Full platform administration, including global LLM API keys |
| `viewer` | Read-only access to sites, clusters, apps, charts, infrastructure, and health |
| `site-admin` | Site, cluster, FleetApp, and chart library operations |
| `cluster-admin` | Cluster and FleetApp operations; chart library is read-only |

Role bindings are enforced by the proxy before requests reach the upstream
services. Scoped roles (`site-admin`, `cluster-admin`) use FleetRoleBinding
scope without organization partitioning. A scoped binding can list unscoped
collections such as apps and charts, and it can mutate objects in its own
site or cluster. It does not authorize unscoped mutations (`POST` or `DELETE`
on `/apps` or `/charts`, or `cluster-enrollments`). Those require a binding
with no scopes or a `*` scope, such as `dc-admin`.

A disabled `FleetUser` is rejected on login and on later requests that present
that user's session. The login does not refresh the user's groups. Service-account
tokens are not checked against `FleetUser`. `PATCH` with `spec.disabled: false`
clears the flag.

Global LLM API keys are managed at `/api/v1alpha1/llm-api-keys` on the auth
service (dc-admin only). Browser and console traffic uses
`/fleet/auth/v1alpha1/llm-api-keys` via the nginx proxy. Key names are not
org-prefixed.

Automation can use the same auth API via `umkim auth` (list/create/rotate LLM
keys, role bindings, service accounts). Tokens are shown once on create/rotate;
use `--quiet-token` when parsing JSON only. See
[umkim-cli.md](components/umkim-cli.md).

## Operational guidance

- Bootstrap the first administrator through the supported installation flow;
  keep bootstrap credentials in the encrypted vault.
- Create narrowly scoped service accounts for automation and store their
  credentials in an approved secret manager.
- Use HTTPS and secure session cookies for any browser-accessible deployment.
- Rotate break-glass and automation credentials, and audit access after use.

Firewall, TLS, and external access behavior depend on the deployed inventory
and network policy. This release does not guarantee that those concerns are
fully solved; verify the live deployment before exposing the UMKIM console.

See [install.md](install.md), [secrets.md](secrets.md), and
[fleet-verify.md](fleet-verify.md) for deployment and verification guidance.
