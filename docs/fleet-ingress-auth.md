# Fleet ingress authentication

> Status: v0.2 is an experimental proof of concept. Treat public ingress,
> identity data, and certificate issuance as operator-managed security work.

Fleet ingress authentication uses Authelia forward-auth with LLDAP. It is
separate from [fleet-console-auth.md](fleet-console-auth.md), which protects
the console and management APIs.

## Scope

`FleetIngressAuth` controls the hub authentication stack and per-cluster
forward-auth configuration. A FleetApp route opts in with
`route.requireAuth: true`; routes without that setting remain public by
design.

The hub can use an LLDAP directory shared with console authentication when it
is configured to do so. Otherwise ingress authentication has its own
directory. Do not mix administrator credentials, secrets, or recovery
procedures between the two deployments.

## Directory placement

Each `FleetClusterJoin.spec.ingressAuth.directory` selects the directory an
enabled cluster authenticates against:

| Value | Result |
|---|---|
| `""` / `local` | The cluster runs its own LLDAP in the `fleet-ingress-auth` namespace |
| `hub` | The cluster runs no LLDAP; Authelia binds to the shared hub (console) directory |

When the hub directory is enabled, the hub chart publishes a dedicated
`lldap-ldap` NodePort for the LDAP port (default `30389`) so spokes can bind
over WireGuard. The hub firewall drops that port on the public interface; keep
it reachable only on the overlay. Clusters using `directory: hub` bind with the
shared read-only account (`svc-bind`), not the directory administrator.

Changing `directory` is a spec change; reconcile the ingress-auth stack for it
to take effect (the console enable/disable actions do this automatically).

## Tenant groups

A route may restrict access to directory groups (tenants) with
`route.requireAuthGroups` (see [fleet-apps.md](components/fleet-apps.md)). The
controller emits a host-level Authelia rule that allows the listed groups and
then denies other authenticated users, before the zone wildcard rules. Use flat
group names that exist in the directory the cluster binds to; the hub directory
holds them when `directory: hub` is set. Groups are not per-application: create
a tenant group and grant it to the routes that belong to that tenant.

## Before enabling a public route

1. Confirm the cluster is ready and ingress matches the route:
   - `route.mode: site` needs a cluster bound to `port_forward` or `reverse_proxy`
     ingress for site-routed public URLs.
   - **Ingress auth** (Authelia forward-auth on the spoke) is only available on
     **`port_forward`** ingress (not `reverse_proxy`).
   - `route.mode: hub` needs site `hubGateway` (and hub zone DNS). Hub-routed
     routes with `route.requireAuth: true` can use hub Authelia when hub
     forward-auth is enabled.
   - `route.mode: none` is out of scope unless the chart exposes its own ingress.
2. Configure DNS for the application and authentication portal with the
   organization’s DNS provider.
3. Enable and verify the authentication stack through the authenticated
   console or approved API client.
4. Verify an unauthenticated request is challenged and an authorized user can
   reach the intended application.
5. Keep LDAP service access internal. Expose an administration UI only through
   a reviewed, restricted management path.

Never copy passwords from Kubernetes Secrets into commands, tickets, or
documentation. Rotate any credential that has been exposed outside an approved
secret-management workflow.

## TLS and network limitations

TLS, ACME, DNS, NAT, firewall rules, and session-cookie behavior are
deployment-specific. This release does not claim to fix runtime firewall or
certificate-issuance issues. Validate the live route, certificate, redirect,
and external network policy before treating an application as protected.

## Related documentation

- [fleet-edge.md](components/fleet-edge.md) — site exposure and route model
- [examples/site-profiles/](../examples/site-profiles/) — per-site ingress inventory
- [lldap-bootstrap.md](lldap-bootstrap.md) — directory administration
- [fleet-console-auth.md](fleet-console-auth.md) — management-plane auth
- [fleet-verify.md](fleet-verify.md) — verification workflow
