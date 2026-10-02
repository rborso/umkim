# BUG-20261002-A2-authelia-portal-routes-missing

**Stage:** A2 — Hub LLDAP + Authelia (ingress auth)
**Environment:** Hub wipe+reinstall run2; fleet_ingress_auth_enabled at install; zone demo.otthol.hu; gateway hostNetwork replicas=1; fleet-edge-controller tag 0.2

## Expected
- umkim ingress-auth get hub Ready (OK)
- Authelia / LLDAP pods healthy (OK)
- Hub Traefik serves https://auth.demo.otthol.hu/ (Authelia portal) with Lets Encrypt cert
- fleet-traefik-fleet-routes contains fleet-authelia-portal and fleet-authelia-portal-tls

## Actual
- Ingress-auth CR Ready; Authelia pod Running
- fleet-traefik-fleet-routes still comment-only placeholder (no routers)
- https://auth.demo.otthol.hu/ -> Traefik default cert + HTTP 404
- acme.json empty (0 bytes)
- Controller log: acme-queue issuing ACME cert host=auth.demo.otthol.hu then hangs in waitNativeACMECert (no TLS router so Traefik never obtains cert)

## Root cause
1. Install Ansible keeps controller-owned fleet-routes CM (placeholder) when controller is present.
2. rebuildFleetRoutesCM (addAutheliaPortalRoutes) only runs on FleetAppRoute reconcile or after cert ready — never on greenfield with Authelia enabled and zero app routes.
3. ACME rediscover still enqueues the portal host; hostNetwork+1rep takes waitNativeACMECert, which assumes TLS routers already exist.
4. onCertReady -> rebuildFleetRoutesCM is blocked by acme.Issuing() guard while still inside the issuing critical section.

## Reproduce
1. Greenfield hub install with fleet_ingress_auth_enabled true, ACME on.
2. Do not create any FleetAppRoute / hub app.
3. Observe portal 404 + empty fleet-routes CM + ACME queue stuck on portal host.

## Evidence
- cli-logs/A2-ingress-auth-get.json
- cli-logs/A2-authelia-pods.txt
- cli-logs/A2-portal-404-before-fix.txt

## Follow-up
Fix in fleet-edge-controller: publish Authelia portal routes before native ACME wait; ensure hub routes on ACME rediscover when Authelia enabled; force-publish while issuing.


## Follow-up (FIXED)
- Patched fleet-edge-controller: ensurePublishedHubRoutes on ACME rediscover; publishFleetRoutesForNativeACME before waitNativeACMECert; onCertReady after Issuing clears.
- Image digest sha256:3bbcae786318ca8e372ba6171f9af325f72fff22c4ad3894aca7c503c3b5214c tag 0.2
- Re-test: portal routes present; https://auth.demo.otthol.hu/ LE CN=auth.demo.otthol.hu HTTP 200 (cli-logs/A2-ingress-auth-verify-pass.txt)
