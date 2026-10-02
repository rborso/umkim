# BUG: Spoke ACME queue stuck in PendingStaging blocks later site routes

- **Stage:** B7b
- **Symptom:** `whoami-site` FleetAppRoute TLS `PendingStaging` ("requesting Let's Encrypt staging certificate (dry-run)") held the per-cluster spoke ACME queue. `whoami-site-auth` stayed `tlsPhase=Queued` / phase Pending with message "waiting for spoke ACME queue" and no ManifestWork until the queue advanced. Unauth curl returned Traefik 404 (no Ingress).
- **Root cause (combined):** Staging cert never appears on probe `upstream_host:30443` (still TRAEFIK DEFAULT CERT) because LE HTTP-01 cannot reach the spoke via public DNS (see BUG-20261002-B7-homelab-dns-hits-hub-traefik). Queue only releases after production cert probe succeeds (`advanceSpokeACMEQueue`), so failure is sticky.
- **Also:** `umkim cluster acme disable lab-ha-0` set FCJ ACME Disabled but left `fleet-edge-clusters` `tls_policy: acme_cluster`, so edge controller continued SpokeACME gating.
- **Workaround:** Force route reconcile; once auth route obtained queue slot it Applied and Authelia 302 worked with default/staging resolver. Re-enable ACME afterward.
- **Assets:** `B7b-fleetapproute.yaml`, `B7b-spoke-authelia-unauth.txt`, edge controller logs "route apply held".
