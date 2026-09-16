# Fleet edge

> Status: v0.2 is an experimental proof of concept. Public routing, DNS,
> firewall, and TLS behavior require deployment-specific validation.

Fleet edge turns a `FleetApp` route intent into `FleetAppRoute` resources and
controller-managed hub or site ingress configuration. The target cluster is
the one named on the FleetApp; the edge controller does not place workloads.

## Site ingress

Each `FleetSite` declares one or more **ingress entries** under
`spec.edge.ingress[]`. Clusters bind to an entry with
`FleetClusterJoin.spec.edge.ingressId`. Inventory examples:
[../../examples/site-profiles/](../../examples/site-profiles/).

| Ingress `method` | Cardinality | TLS (derived) | Ingress auth |
|---|---|---|---|
| `port_forward` | One cluster per entry (`id`, `zone` required; `address` required only when `wanAddressMode` is `fixed`) | `acme_cluster` on spoke | Allowed (zone from ingress) |
| `reverse_proxy` | Many clusters per entry (`id` required; optional `address`, `operatorNotes`) | `external` (off-cluster) | Not available |

Each `port_forward` ingress row may set **`wanAddressMode`**:

| Mode | Use when | `edge_wan_ip` in `fleet-edge-clusters` | ACME DNS preflight |
|---|---|---|---|
| `fixed` | Static public WAN | Same as `ingress.address` | Hostname A/AAAA must include expected IP |
| `dns` | DDNS or rotating WAN | Empty (DNS-only validation) | Resolve hostname + TCP/HTTP-01 connectivity only |

When `wanAddressMode` is omitted, the fleet-site controller infers `dns` if
`spec.edge.wanEgress.enabled` is true and `expectedWanIp` is empty, if
`address` is a private IP, or otherwise defaults to `fixed`. See
[port-forward-ddns](../../examples/site-profiles/port-forward-ddns/) for a DDNS
profile example.

Optional `spec.edge.hubGateway` (with `hubZone` / `hubWildcard`) enables hub-routed
public URLs for clusters using `route.mode: hub`. This replaces the former site-level
`exposure_mode` / `domains` / `external_endpoints` model.

The controller does not create external DNS records, router port-forwards,
cloud firewall rules, or third-party reverse-proxy configuration. Operators
configure those per ingress entry.

## Per-app routing modes

| Mode | Intended result | Hub required for public URL? |
|---|---|---|
| `hub` | Route through the hub gateway to the selected workload cluster | Yes |
| `site` | Route through site ingress (port-forward or reverse proxy) | No |
| `none` | The workload owns its own ingress; no FleetAppRoute is required | No |

Use `route.mode: hub` when the site has `hubGateway` enabled and DNS for
`hubHost` is ready. Use `route.mode: site` when the cluster is bound to a
`port_forward` or `reverse_proxy` ingress entry and `siteHost` is configured.

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
boundaries for each site ingress entry.
