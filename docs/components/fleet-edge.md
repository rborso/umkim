# Fleet edge

> Status: v0.1.0 is an experimental proof of concept. Public routing, DNS,
> firewall, and TLS behavior require deployment-specific validation.

Fleet edge turns catalog placement into `FleetAppRoute` resources and
controller-managed hub or site ingress configuration.

## Routing modes

| Mode | Intended result |
|---|---|
| `hub` | Route through the hub gateway to a placed workload |
| `site` | Route through a configured site ingress path |
| `none` | The workload owns its own ingress; no FleetAppRoute is required |

Site routing requires a configured direct-ingress path. The controller does not
create external DNS records, public NAT, cloud firewall rules, or third-party
reverse-proxy configuration.

## Optional forward-auth

Set `routing.require_auth: true` only after the relevant ingress-auth stack is
ready. It is an explicit per-route opt-in; routes without it remain public by
design. See [fleet-ingress-auth.md](../fleet-ingress-auth.md).

## TLS and availability limitations

ACME issuance, certificate renewal, TLS redirects, firewall reachability, and
multi-cluster failover depend on the live environment. Do not claim that a
runtime TLS or firewall issue is fixed based solely on desired configuration.
Verify the externally reachable route, certificate, authentication challenge,
and workload health after every relevant change.

Current multi-cluster hub routing is active–backup behavior, not active–active
load sharing.

## Operate and verify

Deploy and upgrade edge resources through the supported
[install.md](../install.md) workflow. Use the edge checks in
[fleet-verify.md](../fleet-verify.md) after configuring DNS and network
boundaries.
