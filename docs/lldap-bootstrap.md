# LLDAP bootstrap

> Status: v0.2 is an experimental proof of concept. Directory administration
> and access recovery require an organization-approved security process.

UMKIM can run LLDAP in two independent roles:

| Role | Purpose |
|---|---|
| Console directory | Human login and group lookup for fleet-console authentication |
| Ingress directory | User authentication for Authelia-protected application routes |

The ingress stack can be configured to use the console directory. Otherwise it
uses a separate directory. Treat them as separate identity stores unless shared
operation is explicitly configured.

## UMKIM console access model

Fleet roles are assigned through Fleet role bindings. The standard roles are
`dc-admin`, `viewer`, `site-admin`, and `cluster-admin`.

The chart bootstraps the configured console LDAP groups. Default group CNs are
`fleet-admins`, `fleet-viewers`, `fleet-site-admins`, and
`fleet-cluster-admins` (Fleet roles `dc-admin`, `viewer`, `site-admin`, and
`cluster-admin` are assigned via Fleet role bindings, not LDAP group names
alone). Directory membership alone is not a substitute for reviewing the
associated Fleet role binding.

The LLDAP directory administrator is a directory-management identity. It does
not automatically receive platform-administrator access to the UMKIM console.
Create named human users and grant the minimum required group membership and
role binding.

## Safe administration

- Use a restricted management channel for the LLDAP web interface.
- Keep LDAP service ports internal; do not publish them as an application
  endpoint.
- Store bootstrap and recovery credentials only in the encrypted vault or an
  approved secret manager.
- Do not retrieve, print, or paste Kubernetes Secret values into commands or
  documentation.
- Rotate recovery credentials after break-glass use.

See [fleet-console-auth.md](fleet-console-auth.md) for console RBAC,
[fleet-ingress-auth.md](fleet-ingress-auth.md) for application forward-auth,
and [secrets.md](secrets.md) for secret handling.
