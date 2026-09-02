# Fleet edge

> Status: v0.2 is an experimental proof of concept. Public routing, DNS,
> firewall, and TLS behavior require deployment-specific validation.

Fleet edge turns a `FleetApp` route intent into `FleetAppRoute` resources and
controller-managed hub or site ingress configuration. The target cluster is
the one named on the FleetApp; the edge controller does not place workloads.

## Site exposure

Each `FleetSite` selects how that location reaches the WAN. Clusters inherit
the site default unless overridden on `FleetClusterJoin`. Inventory examples:
[../../examples/site-profiles/](../../examples/site-profiles/).

| `exposure_mode` | Meaning |
|---|---|
| `hub_gateway` | Public HTTP/S on the hub; hub forwards to spokes over WireGuard |
| `port_forward` | WAN port-forwards to spoke Traefik NodePorts (`30080`/`30443`) |
| `reverse_proxy` | External reverse proxy terminates TLS and reaches cluster ingress |
| `none` | No fleet-managed WAN exposure at the site |

Inventory snippets use `exposure_mode`; the CRD field is `exposureMode`.

The controller does not create external DNS records, router port-forwards,
cloud firewall rules, or third-party reverse-proxy configuration. Operators
configure those per site.

## Per-app routing modes

| Mode | Intended result | Hub required for public URL? |
|---|---|---|
| `hub` | Route through the hub gateway to the selected workload cluster | Yes |
| `site` | Route through site ingress (port-forward or reverse proxy) | No |
| `none` | The workload owns its own ingress; no FleetAppRoute is required | No |

Use `route.mode: hub` when the site uses `hub_gateway` and DNS for `hubHost`
is ready. Use `route.mode: site` when the site uses `port_forward` or
`reverse_proxy` and `siteHost` is configured.

## Hub independence

- Spokes run independent K3s; workload pods do not depend on the hub.
- **Hub down:** console and hub-routed URLs stop; `site` and `none` apps can
  remain reachable when site ingress is healthy.
- **Detach:** removing a cluster from the hub (default delete mode) keeps K3s
  and workloads on the spoke.

## Optional forward-auth

Set `route.requireAuth: true` only after the relevant ingress-auth stack is
ready. It is an explicit per-route opt-in; routes without it remain public by
design. See [fleet-ingress-auth.md](../fleet-ingress-auth.md).

## TLS and availability limitations

ACME issuance, certificate renewal, TLS redirects, firewall reachability, and
multi-cluster failover depend on the live environment. Do not claim that a
runtime TLS or firewall issue is fixed based solely on desired configuration.
Verify the externally reachable route, certificate, authentication challenge,
and workload health after every relevant change.

Multi-cluster **hub** routing is active–backup behavior, not active–active
load sharing. Per-site `port_forward` and `reverse_proxy` exposure is
independent of hub-gateway failover.

## Operate and verify

Deploy and upgrade edge resources through the supported
[install.md](../install.md) workflow. Use the edge checks in
[fleet-verify.md](../fleet-verify.md) after configuring DNS and network
boundaries for each site's exposure mode.
