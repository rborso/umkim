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

## Ingress service bind account

The ingress-auth stack creates a shared, low-privilege LLDAP bind account
(`svc-bind` by default) and adds it to LLDAP's `lldap_strict_readonly` group.
Applications that search or authenticate against the directory — Authelia, and
optionally apps such as Jellyfin — use this account instead of the directory
administrator (`admin` / `lldap_admin`).

This applies to clusters that deploy their own LLDAP (spokes). The hub uses the
shared fleet-console directory, which already provides a bind account, so the
hub sets `lldap.bindAccount.enabled: false` and creates no second account.

## Hub (shared) directory accounts

The fleet-console chart creates two service accounts in the hub directory when
LLDAP is enabled (values under `lldap.serviceAccounts`):

- `svc-bind` in `lldap_strict_readonly` — the shared read-only account Authelia
  and applications bind with, including spokes configured with
  `ingressAuth.directory: hub`.
- `dir-admin` in `lldap_admin` — a dedicated privileged account for directory
  administration (tenant/group and invite operations), separate from the
  bootstrap `admin` identity.

Both are created idempotently by the `lldap-service-accounts` Job. The shared
directory is exposed to spokes only over WireGuard on a dedicated NodePort; the
hub firewall drops that port on the public interface.

## Tenants

Tenant separation uses flat directory groups only; there are no per-application
groups. Create a group per tenant and reference it from routes with
`route.requireAuthGroups`. Authelia enforces membership against whichever
directory the cluster binds to. Directory membership alone does not grant
platform access; review Fleet role bindings separately.

Platform administrators manage tenants from the console **Access control →
Tenants** page: list groups with their members, create/delete tenant groups, and
add/remove directory users. LLDAP-internal `lldap_*` groups are read-only there.
Per-cluster directory placement is selected in the cluster edge tab (local vs
shared hub directory).

## Invitations

A platform administrator can mint a single-use invite from the console
(**Access control → Invitations**). The invite records the tenant groups the
new user should join and expires after 72 hours by default. Only a SHA-256 hash
of the token is stored, in the `fleet-invites` Secret; the one-time link is
shown once at creation.

Redeeming the link creates the user in the shared hub directory, adds the
recorded groups, and sets the chosen password. LLDAP exposes no GraphQL
password mutation (passwords use the OPAQUE protocol), so the controller creates
the user and group membership over GraphQL as `dir-admin` and then runs a
short-lived Job using the LLDAP image's `lldap_set_password` for the password.

The sign-up page is available on the hub console (`/fleet/invite`) and, for
clusters bound to `directory: hub`, on the Authelia portal host (`/invite`).
The cluster page proxies redemption to the hub over WireGuard on the console
auth NodePort; that port is blocked on the public interface.

- Created by the `fleet-ingress-auth` chart's `lldap-bind-account` Job on
  install/upgrade. The account name, group, and password are chart values
  (`lldap.bindAccount.*`) rendered from the `fleet_ingress_auth` role defaults.
- The password is derived per cluster from that cluster's LLDAP JWT secret and
  published in the `lldap-service-accounts` Secret in the `fleet-ingress-auth`
  namespace. Charts in other namespaces must copy or sync it; never commit it.
- `lldap_strict_readonly` grants read/search only. A leaked bind credential
  cannot change users, groups, or passwords.

## Verification

Run `scripts/verify-shared-directory.sh` on the hub for a read-only check of the
hub services, the directory service accounts, and the WireGuard-only NodePorts.
Spoke-side behavior (no spoke LLDAP, Authelia binding the hub directory, tenant
group enforcement, invite redemption) is deployment-specific; verify it against
each cluster as described above.

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
