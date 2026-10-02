# BUG-20261002-B7b: ACME inventory omits ingress method → Traefik without cert resolvers

**Stage:** B7b spoke ACME / Authelia
**Severity:** High
**Observed:** 2026-10-02 ~12:19 Europe/Budapest

## Expected
`umkim cluster acme enable lab-ha-0` pushes spoke Traefik with letsencrypt(+staging) certificate resolvers when cluster uses port_forward + acme_cluster.

## Actual
ACME Job Succeeded / acmeStatus Ready, but phase6 ManifestWork Traefik args had **no** `--certificatesresolvers.*`. Routes using `letsencrypt-staging` then 404 (`Router uses a nonexistent certificate resolver`).

## Root cause
`controllers/fleet-site/internal/acme/desired.go` BuildInventoryYAML emits `fleet_edge_clusters` with only `direct_ingress` + `tls_policy`, omitting `method`. Template gate is:
`phase6_spoke_acme = fleet_edge_acme_enabled && direct_ingress && spoke_ingress_method == "port_forward"`.
With empty method, ACME block is skipped. Compounded by hub legacy profile setting `fleet_edge_acme_preflight_enabled: false` (no staging resolver even when ACME args present).

## Fix (committed 74eb2b1)
- Pass `method` from fleet-edge-clusters into ACME + ingress-auth inventories.
- Force `fleet_edge_acme_preflight_enabled: true` when any phase6 cluster present.
- A0b staging TLS-ALPN when redirect mode (template).

**Still needs:** rebuild+redeploy `fleet-site-controller` (and ansible-runner for template) then re-run ACME enable.

## Workaround used
Local `ansible-playbook` spoke_ingress push with explicit `method: port_forward` + preflight true.
