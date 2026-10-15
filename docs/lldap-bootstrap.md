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
add/remove directory users. Reserved groups (`lldap_*` internals and the
`fleet-*` console role groups) are not tenants; they are hidden from tenant
management and from the invitation picker, and cannot be deleted there.
Per-cluster directory placement is selected in the cluster edge tab (local vs
shared hub directory).

## Invitations

A platform administrator can mint a single-use invite from the console
(**Access control → Invitations**). The invite records the tenant groups the
new user should join and expires after 72 hours by default. Only a SHA-256 hash
of the token is stored, in the `fleet-invites` Secret; the one-time link is
shown once at creation.

An invite is either **untargeted** (redeemed against the shared hub directory)
or **targeted at one cluster** (`--cluster` on the CLI, or the **Directory**
selector on the console's *Access control → Invitations* page), which redeems
against whatever directory that cluster uses: its own LLDAP for
`directory: local`, the hub directory for `directory: hub`. Redemption creates
the user, adds the recorded groups, and sets the chosen password. The invite
list shows the target directory, and the console only offers clusters that have
ingress auth (so they have a directory).

The tenant groups a console invite offers come from the hub directory. A
cluster's own directory usually has different groups, so for a cluster-targeted
invite make sure the selected names exist **in that directory** — redemption
fails with `directory group "x" does not exist` otherwise.

LLDAP exposes no GraphQL password mutation (passwords use the OPAQUE protocol),
so the controller creates the user and group membership over GraphQL and then
runs a short-lived Job using the LLDAP image's `lldap_set_password` for the
password.

- **Hub directory.** The controller calls the hub LLDAP API directly as
  `dir-admin`.
- **A cluster's own LLDAP.** The hub cannot reach that directory — it is a
  ClusterIP Service — so redemption runs as a Job *on the spoke*, beside LLDAP,
  in the `fleet-ingress-auth` namespace. The Job reuses the image the cluster's
  own LLDAP Deployment runs and reads the admin password from the chart's
  `lldap-credentials` Secret; the invitee's username, password, and groups
  travel in a `lldap-invite-redeem` Secret that is deleted with the Job. This
  mirrors the chart's `lldap-bind-account` Job and needs no new exposure of the
  directory.

```bash
umkim invite create --groups <tenant> --cluster <cluster> --ttl-hours 72
umkim tenant add-user <tenant> <username> --password <pw> --cluster <cluster>
```

Authelia resolves the directory over the cluster network, so a user created
moments ago normally signs in immediately; a first attempt can fail with a 401
if that lookup times out (transient DNS or service blip). Retry: the account and
its password are already correct, and a direct LDAP bind succeeds.

The sign-up page is available on the hub console (`/fleet/invite`) and on the
Authelia portal host of every ingress-auth cluster (`/invite`), for both
directories: the page always posts to the hub, which redeems against whichever
directory the invite targets. The cluster page proxies redemption to the hub
over WireGuard on the console auth NodePort; that port is blocked on the public
interface.

When the hub console is not publicly reachable, the console is WireGuard-only
and a link built from the browser origin is useless to an invitee. The create
response therefore returns public sign-up links: the hub console when
`FLEET_CONSOLE_PUBLIC_URL` is set, plus the Authelia portal host of the targeted
cluster (or of every `directory: hub` cluster for an untargeted invite). Share
one of those links.

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
