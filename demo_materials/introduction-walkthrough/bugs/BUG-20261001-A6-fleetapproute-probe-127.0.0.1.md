# BUG-20261001-A6-fleetapproute-probe-127.0.0.1

## Stage
A6 / A6b — hub-routed FleetAppRoute health

## Summary
`FleetAppRoute` for `whoami` and `whoami-auth` stays **Degraded** with probe errors like:
`Get "https://127.0.0.1/": dial tcp 127.0.0.1:443: connect: connection refused`
and `hub path unhealthy after N probes`.

External access works: public whoami HTTPS **200**; whoami-auth unauthenticated **302** to Authelia.

## Impact
Console/API route health misleading while apps serve correctly.

## Evidence
`assets/A6b-fleetapproute-status.yaml`, `assets/A6-whoami-hub-curl.txt`, `assets/A6b-curl-unauthenticated.txt`

## Suggested fix
Probe the hub Traefik entrypoint / published service address instead of `127.0.0.1` inside the controller pod (or use Host header against the hub gateway Service).
