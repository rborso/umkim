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

A `port_forward` entry's **effective wildcard** is its `wildcard`, or `*.<zone>`
when `wildcard` is empty. A `reverse_proxy` entry has an effective wildcard only
when it sets `wildcard` or `zone`. The wildcard is the set of public hostnames
the operator configures DNS and the router for, so it is enforced, not
decorative:

- Two `port_forward` entries on a site may not share an effective wildcard. The
  site is refused (`duplicates wildcard`) on create and patch, because DNS can
  only point one way and one of the entries could never take effect.
- A `site`-mode route host must fall under the bound entry's wildcard. The
  controller rejects a `siteHost` outside it at app create/patch and again at
  reconcile, instead of publishing a URL that never resolves.
- A `hub`-mode route host is checked the same way against the site hub gateway
  wildcard when the site sets `hubZone` / `hubWildcard`.
- A cluster that leaves `ingressId` unset binds implicitly when the site has
  exactly one `port_forward` entry; that implicit binding is shown as the entry's
  `assignedClusters`.

`umkim site ingress verify SITE` reports the same checks read-only, plus DNS
resolution of the published hosts and spoke Traefik reachability over WireGuard.
The WAN DNS record and router port-forward remain operator-managed and are
reported as skipped.

Clusters bound to **`reverse_proxy`** do not receive spoke Let's Encrypt or the
Traefik `web` entrypoint HTTP→HTTPS redirect (phase6 spoke ingress). TLS
terminates on the operator reverse proxy; site `FleetApp` routes use plain HTTP
on NodePort **30080** with per-host Ingress rules. Set `siteHost` explicitly on
each app.

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
A catalog `routing.site_host` value updates the route host; an empty catalog
host leaves the existing host in place.

## Hub independence

- Spokes run independent K3s; workload pods do not depend on the hub.
- **Hub down:** console and hub-routed URLs stop; `site` and `none` apps can
  remain reachable when site ingress is healthy.
- **Detach:** removing a cluster from the hub (default delete mode) keeps K3s
  and workloads on the spoke.

## Optional forward-auth

Set `route.requireAuth: true` only after the relevant ingress-auth stack is
ready. It is an explicit per-route opt-in; routes without it remain public by
design. `route.requireAuthGroups` further restricts the route to the listed
directory groups (tenants); leave it empty to allow any authenticated user.
See [fleet-ingress-auth.md](../fleet-ingress-auth.md).

## TLS and availability limitations

ACME issuance, certificate renewal, TLS redirects, firewall reachability, and
multi-cluster failover depend on the live environment. Do not claim that a
runtime TLS or firewall issue is fixed based solely on desired configuration.
Verify the externally reachable route, certificate, authentication challenge,
and workload health after every relevant change.

Multi-cluster **hub** routing is active–backup behavior, not active–active
load sharing. Per-site `port_forward` and `reverse_proxy` exposure is
independent of hub-gateway failover.

## Spoke ACME queue

Site routes on a `port_forward` cluster are issued one hostname at a time. The
controller keeps a per-cluster queue: the active hostname holds the slot until
its certificate is live, and a hostname that has held it longer than
`spokeACMEHostStaleAfter` (3 minutes) without a certificate is rotated to the
back so it cannot starve the others. Queue transitions are logged under the
`spoke-acme-queue` logger (queued, released, rotated), which is what makes a
stall diagnosable.

Two details decide whether a hostname can be stranded:

- The queue probes the spoke's **registered Traefik HTTPS port** (`30443` for
  `port_forward`), not 443. Probing 443 never observes a spoke certificate, so
  the queue could previously advance only by rotating on staleness.
- The production reload token (`edge.umkim.com/spoke-acme-prod-retry`) is
  re-stamped while the certificate is still not live. Re-stamping changes the
  router revision, which is enough for Traefik to retry without a restart. After
  two refreshes the spoke Traefik is restarted **once** for that hostname, as the
  retry Traefik honours when the router configuration itself has not changed. A
  one-shot token left a hostname that failed a single challenge — for example a
  connection refusal during TLS-ALPN-01 validation — in `Issuing` with nothing
  left to retry; a restart on *every* refresh was worse: it interrupts every other
  router on the spoke and, when the challenge itself keeps failing, burns through
  the CA's per-identifier authorization-failure budget, so a hostname that could
  have been issued is blocked for the next hour. Later refreshes therefore only
  re-stamp the token.

A route that answers `404` while `status.tlsPhase` is `Queued` is still in this
queue. A route stuck in `Issuing` for more than a few minutes is a bug.

## Hub gateway health probes

The fleet-edge controller can probe hub-routed gateway reachability from the pod.
Defaults target the in-cluster Traefik Service (for example
`http://gateway.<hub-ingress-namespace>.svc.cluster.local:80`) instead of
loopback on the hub node. Override with inventory vars
`fleet_edge_hub_gateway_url` and `fleet_edge_hub_gateway_http_url` when the hub
ingress namespace or Service name differs.

Disabling cluster ACME (`POST /clusters/{name}/acme/disable`) clears
`tls_policy: acme_cluster` from that cluster's row in `fleet-edge-clusters` so
edge staging queues stop treating the spoke as HTTP-01 eligible.

## Operate and verify

Deploy and upgrade edge resources through the supported
[install.md](../install.md) workflow. Use the edge checks in
[fleet-verify.md](../fleet-verify.md) after configuring DNS and network
boundaries for each site ingress entry.
