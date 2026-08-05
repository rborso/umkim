# Fleet ingress authentication

> Status: v0.1.0 is an experimental proof of concept. Treat public ingress,
> identity data, and certificate issuance as operator-managed security work.

Fleet ingress authentication uses Authelia forward-auth with LLDAP. It is
separate from [fleet-console-auth.md](fleet-console-auth.md), which protects
the console and management APIs.

## Scope

`FleetIngressAuth` controls the hub authentication stack and per-cluster
forward-auth configuration. A catalog route opts in with
`routing.require_auth: true`; routes without that setting remain public by
design.

The hub can use an LLDAP directory shared with console authentication when it
is configured to do so. Otherwise ingress authentication has its own
directory. Do not mix administrator credentials, secrets, or recovery
procedures between the two deployments.

## Before enabling a public route

1. Confirm the cluster is ready and has a supported direct-ingress path.
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

- [fleet-edge.md](components/fleet-edge.md) — route and exposure model
- [lldap-bootstrap.md](lldap-bootstrap.md) — directory administration
- [fleet-console-auth.md](fleet-console-auth.md) — management-plane auth
- [fleet-verify.md](fleet-verify.md) — verification workflow
