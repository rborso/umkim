# HA full-stack cluster with app bootstrapping (umkim CLI) — `lab-e2e-7`

Greenfield three-node HA cluster on site **`homelab`**, driven by the **`umkim`
CLI**, exercising node VMs from the node spec, **NFS data + Longhorn config**
storage, cluster ingress auth (Authelia + LLDAP), per-cluster Let's Encrypt, and
the whole library app set behind forward-auth — this time with **every chart
bootstrap enabled**, including the servarr stack, which needs the operator's
nCore tracker credentials.

This repeats and extends [ha-fullstack-cli.md](ha-fullstack-cli.md) (the
`lab-e2e-6` run, which left the servarr bootstrap disabled because the nCore
credential was not available). The generic procedure is
[docs/fleet-verify.md](../../docs/fleet-verify.md) → *HA cluster with node VMs,
dual storage, ingress auth, ACME, and apps*.

**Run log (2026-10-15):** working directory `/tmp/opencode/e2e7/` (not
committed): `watch.log`, `create-apps.log`, `apps/`, `secrets/`, and the helper
scripts referenced below.

## Summary

| Step | Result |
|------|--------|
| Site ingress | duplicate `homelab-ha-pf` removed — it claimed the same address and wildcard as `homelab-pf` |
| Cluster create + 3 VMs | created, one guest per hypervisor (virt04, virt01, virt02) |
| Bootstrap | **passed first time** (the Longhorn disk minimum is stated in the create body) |
| Ingress auth (Authelia + LLDAP) | spoke stack live; enabled by the controller defaults |
| ACME | Let's Encrypt certificates for every exposed host (11 routes + the portal); one host needed manual recovery from a stalled queue |
| Edge surface | the over-broad `qbittorrent` route was removed after review — 11 routes remain, all `302` → Authelia |
| Apps (7) | all `Ready` |
| App bootstraps | gitea, immich, nextcloud, paperless and **servarr** jobs `Complete` |
| nCore indexer | added by the servarr bootstrap, and a live Prowlarr search returned real results |
| Directory user | `labuser` created in the spoke LLDAP; SSO verified against every route |
| Storage split | data PVCs on `nfs-client`, servarr config PVCs on `longhorn` |

## Phase 0 — Preflight

```bash
umkim site host list homelab
```

```
NAME    KIND        PHASE  ANSIBLE_HOST  VM_LOCATION
virt01  hypervisor  Ready  172.16.10.2
virt02  hypervisor  Ready  172.16.10.3
virt03  bare_metal  Ready  172.16.10.4
virt04  hypervisor  Ready  172.16.10.6
```

Hypervisor capacity before the run (SSH via the jumphost
`linux@jump.otthol.hu:30200`):

| Host | vCPU | RAM free | Disk free (`/`) |
|---|---|---|---|
| virt01 `172.16.10.2` | 8 | 30 GiB | 453 G |
| virt02 `172.16.10.3` | 8 | 30 GiB | 106 G |
| virt04 `172.16.10.6` | 6 | 30 GiB | 450 G |

NFS on virt04 (`/srv/fleet-nfs`, export
`172.16.10.0/24(rw,sync,no_subtree_check,no_root_squash)`), base images
`/opt/local/rocky-9.qcow2` + `rocky-10.qcow2`.

### The ingress conflict had to be cleared first

`umkim site ingress verify homelab` reported:

```
homelab-pf      wildcard  fail  wildcard *.homelab.otthol.hu is also claimed by ingress "homelab-ha-pf"
homelab-ha-pf   wildcard  fail  wildcard *.homelab.otthol.hu is also claimed by ingress "homelab-pf"
```

Two `port_forward` rows claiming the same address *and* the same wildcard make
the public hostnames ambiguous — the check exists precisely so this is caught
before a cluster binds one of them. `homelab-pf` is the entry the site
description and the docs use, so the redundant `homelab-ha-pf` row was removed:

```bash
umkim site ingress delete homelab homelab-ha-pf
umkim site ingress verify homelab          # OK yes
```

`ingress delete` prints `patch reported OK but ingress count mismatch: want 3
got 4` on success — the delete landed (the list and verify agree), so the CLI's
post-delete count assertion is off by one (see Findings).

## Phase 1 — Create

`cluster-create.json`:

```json
{
  "clusterName": "lab-e2e-7",
  "siteId": "homelab",
  "bootstrapHost": "172.16.10.101",
  "topology": "ha_3_node",
  "controlPlaneVip": "172.16.10.100",
  "autoProvision": true,
  "minRootDiskGi": 40,
  "edge": { "ingressId": "homelab-pf" },
  "nodes": [
    { "hostname": "lab-e2e-7-1", "address": "172.16.10.101", "role": "control",
      "vm": { "cores": 6, "memory": 16384, "diskGi": 50, "dataDiskGi": 20 } },
    { "hostname": "lab-e2e-7-2", "address": "172.16.10.102", "role": "control",
      "vm": { "cores": 6, "memory": 16384, "diskGi": 50, "dataDiskGi": 20 } },
    { "hostname": "lab-e2e-7-3", "address": "172.16.10.103", "role": "control",
      "vm": { "cores": 6, "memory": 16384, "diskGi": 50, "dataDiskGi": 20 } }
  ],
  "storage": {
    "backend": "nfs_external",
    "minDataDiskGi": 20,
    "nfs": {
      "enabled": true,
      "server": "172.16.10.6",
      "exportBase": "/srv/fleet-nfs",
      "clusterPath": "lab-e2e-7",
      "storageClass": "nfs-client",
      "delegateHost": "virt04"
    },
    "longhorn": { "enabled": true, "device": "/dev/vdb", "storageClass": "longhorn" }
  }
}
```

```bash
umkim cluster create -f cluster-create.json
umkim cluster wait lab-e2e-7 --phase Ready      # ~22 min
```

`minDataDiskGi: 20` matches the node `vm.dataDiskGi`, so the Longhorn capacity
assertion passes on the first bootstrap — the `lab-e2e-6` run failed here and
had to be re-armed.

Guests (`least-utilized` spread, one per hypervisor):

| Guest | Hypervisor | vCPU | RAM | vda | vdb |
|---|---|---|---|---|---|
| `lab-e2e-7-1` | virt04 | 6 | 16 GiB | 50 GiB | 20 GiB |
| `lab-e2e-7-2` | virt01 | 6 | 16 GiB | 50 GiB | 20 GiB |
| `lab-e2e-7-3` | virt02 | 6 | 16 GiB | 50 GiB | 20 GiB |

## Phase 2 — Ingress auth, ACME

Both were auto-enabled by the controller defaults
(`FLEET_FCJ_DEFAULT_INGRESS_AUTH_ENABLED=true`,
`FLEET_FCJ_DEFAULT_ACME_ENABLED=true`), so no explicit enable was needed:

```
umkim cluster get lab-e2e-7
  ingress_auth_enabled = True
  acme_enabled = True
  tls_policy = "acme_cluster"
  ingress_id = "homelab-pf"
```

The cluster runs its own directory (`directory` empty → `local`): the spoke
`fleet-ingress-auth` namespace holds `lldap` (with the `svc-bind` read-only
account created by its post-install job) and `authelia`.

Authelia is deny-by-default with the whole ingress zone behind one factor:

```yaml
access_control:
  default_policy: deny
  rules:
    - domain: "auth.homelab.otthol.hu"     # the portal itself
      policy: bypass
    - domain: "*.homelab.otthol.hu"        # every app host
      policy: one_factor
    - domain: "homelab.otthol.hu"
      policy: one_factor
```

The session cookie is scoped to `homelab.otthol.hu`, so one login covers every
host under the zone.

## Phase 3 — Directory user

`umkim tenant add-user` targets the shared **hub** directory, so a spoke-local
user is created in the cluster's own LLDAP (admin password read from the spoke
`lldap-credentials` Secret and never printed):

```bash
/tmp/opencode/e2e7/create-user.sh labuser
```

Directory contents afterwards: `admin`, `svc-bind`, `labuser`.

## Phase 4 — Apps

Data PVCs on `nfs-client`, small config PVCs on `longhorn`, every route `site`
mode with `requireAuth`, and every chart bootstrap enabled. Apps are installed
in order, each waited to `Ready` before the next, so a failure lands on the app
that caused it:

```bash
/tmp/opencode/e2e7/create-apps.sh
```

| App | Chart | Route host(s) | Data PVCs → `nfs-client` | Config PVCs → `longhorn` |
|---|---|---|---|---|
| whoami | `whoami` 0.1.0 | `whoami` | — | — |
| petclinic | `pet-clinic-helm` 0.1.1 | `petclinic` | postgres 2Gi | — |
| gitea | `gitea` 0.1.9 | `gitea` | data 10Gi, postgres 5Gi | — |
| immich | `immich` 0.1.3 | `immich` | data 10Gi, postgres 10Gi | — |
| nextcloud | `nextcloud` 0.1.5 | `nextcloud`, `collabora` | data 20Gi, postgres 5Gi | — |
| paperless | `paperless` 0.1.5 | `paperless` | data 5Gi, media 20Gi, consume 2Gi, export 2Gi, postgres 5Gi | — |
| servarr | `servarr` 0.2.17 | `servarr`(seerr), `jellyfin`, `sonarr`, `radarr` | media 50Gi **RWX** | 6 config PVCs, 11Gi |

Native-LDAP charts bind the cluster directory through the Secret the
controller's job installs into the app namespace, referenced with the
`${LLDAP_BIND_SECRET}` placeholder; immich and paperless register Authelia OIDC
clients, which is what switches the cluster OIDC provider on.

```yaml
auth:
  ldap:
    enabled: true
    existingSecret: ${LLDAP_BIND_SECRET}   # -> gitea-lldap-bind / nextcloud-lldap-bind
```

### servarr: bootstrapping with the nCore credential

The `servarr-bootstrap` Secret is the one piece the FleetApp spec cannot carry
(there is no operator-secret field), so it is created directly on the spoke
before the app is installed:

```bash
/tmp/opencode/e2e7/make-servarr-secret.sh lab-e2e-7
```

Keys: `SERVARR_USERNAME`/`SERVARR_PASSWORD`, `QBITTORRENT_USERNAME`/`_PASSWORD`,
`JELLYFIN_ADMIN_USER`/`_PASSWORD` (generated), and the operator's
`NCORE_USERNAME`/`NCORE_PASSWORD` (plus `NCORE_2FA_CODE` empty; the account
`NCORE_PASSKEY` is carried for reference but the Prowlarr `ncore` definition has
no passkey field). The bootstrap job mounts the Secret with `envFrom`.

The bootstrap ran to completion on the first attempt (`servarr-bootstrap`
`Complete 1/1`, 3m21s) and ended `==> Verification passed`:

```
==> Bootstrap components: {"sonarr":true,"radarr":true,"lidarr":false,"jellyfin":true,
                            "seerr":true,"prowlarr":true,"qbittorrent":true,"flaresolverr":true}
==> Prowlarr: added FlareSolverr proxy
==> Prowlarr: added nCore indexer
==> Prowlarr: added qBittorrent download client
==> Prowlarr: added Sonarr app
==> Prowlarr: added Radarr app
...
==> Verification passed
==> Bootstrap completed successfully
```

This closes finding **#6** of the `lab-e2e-6` run: with the operator credential
supplied, the servarr bootstrap completes and verifies unattended.

## Phase 5 — Verification

```bash
/tmp/opencode/e2e7/verify-state.sh
/tmp/opencode/e2e7/verify-routes.sh labuser
/tmp/opencode/e2e7/verify-tls.sh
/tmp/opencode/e2e7/test-ncore.sh
/tmp/opencode/e2e7/verify-app-login.sh gitea labuser
```

The hub reaches the spoke's Traefik over WireGuard (`10.10.0.10:30443`); the
public port-forward target (the `172.16.10.100` VIP) serves the same routers and
was spot-checked from the jumphost.

### Assertions

| Check | Expected | Observed |
|---|---|---|
| `umkim cluster get` | `Ready`, `available`, `tls_policy: acme_cluster` | Ready / available / acme_cluster |
| `virsh list` per hypervisor | one guest per node, spread | virt04, virt01, virt02 |
| `umkim app list` | every app `Ready` | 7/7 Ready |
| PVC placement | data on NFS, config on Longhorn | see table below |
| Longhorn volumes | `attached` / `healthy` | 7/7 attached healthy |
| `<nfs-base>/<cluster>` on the storage host | one directory per NFS PVC | 13 directories, 888 MiB |
| Each app host, unauthenticated | `302` to the Authelia portal | all 11 hosts |
| Directory user login | succeeds through the portal | `POST /api/firstfactor` → 200 |
| Each app host, authenticated | reaches the app | all 11 hosts |
| Certificate per host | issued by Let's Encrypt | all 12 hosts (11 routes + portal) |
| Bootstrap jobs | `Complete` | 5/5 app bootstraps |

Nodes `Ready` on `v1.36.5+k3s1`; `nfs-client` is the **only** default
StorageClass (`longhorn` and `local-path` are non-default), and kube-vip holds
`172.16.10.100`.

### Certificate queue

The edge controller serialises spoke ACME issuance: a batch of new hosts is
issued one at a time, and it restarts the spoke Traefik between hosts to drop
the staging certificate before the production one. Observed on this run: the
portal plus the first six app hosts were issued in ~25 minutes, with a Traefik
restart per host:

```
07:17:39  restarted spoke traefik to drop staging certificate before production issuance  host=whoami.homelab.otthol.hu
07:19:56  ...  host=petclinic.homelab.otthol.hu
07:41:28  ...  host=collabora.homelab.otthol.hu
```

A host whose route still answers `404` is normally still in that queue
(`FleetAppRoute.status.tlsPhase: Queued`, message *waiting for spoke ACME
queue*); nothing on the `umkim` side is wrong. Expect the last hosts of a
12-route cluster to appear ~30–45 minutes after install.

**This run did not drain cleanly.** `servarr-qbittorrent` reached the front of
the queue, failed its challenge once (`84.1.215.254: Connection refused` during
TLS-ALPN-01 validation), and then stayed `Queued` for ~25 minutes while the
other eleven hosts were already `Ready` — the in-memory queue kept the cluster's
slot on a host that neither completed nor rotated (Findings 5–7). Two manual
steps recovered it:

```bash
# 1. clear the stalled in-memory spoke ACME queue
kubectl -n fleet-system rollout restart deployment/fleet-edge-controller
# 2. force a router change so Traefik re-attempts the certificate
kubectl --kubeconfig <spoke> -n servarr delete ingress fleet-route-servarr-qbittorrent-tls
```

The controller re-rendered the ingress within ~4 minutes and Traefik obtained
the certificate on the next attempt. All thirteen hosts then served
`C=US, O=Let's Encrypt`.

PVC placement (`kubectl get pvc -A`):

```
gitea      gitea-data, gitea-postgres                                  nfs-client
immich     immich-data, immich-postgres                                nfs-client
nextcloud  nextcloud-data, nextcloud-postgres                          nfs-client
paperless  data, media, consume, export, postgres                      nfs-client
petclinic  petclinic-postgres                                          nfs-client
servarr    servarr-media                                               nfs-client (RWX)
servarr    sonarr/radarr/prowlarr/qbittorrent/jellyfin/seerr-config,
           flaresolverr-config                                         longhorn
```

Route checks as `labuser` (Authelia session cookie scoped to the zone):

```
unauthenticated:  whoami petclinic gitea immich nextcloud collabora paperless
                  servarr jellyfin sonarr radarr                      -> 302
                  auth                                                -> 200 (portal)
authenticated:    whoami petclinic gitea immich collabora             -> 200
                  nextcloud paperless jellyfin sonarr radarr          -> app-level login redirect
                  servarr (Seerr)                                     -> 307 (app-level)
```

`collabora` is the one host that was a plain `404` until its route left the
ACME queue: before that the router did not exist on the spoke at all.

### servarr routes: only the user-facing services belong on the edge

The first cut of this run copied the `lab-e2e-6` route list, which also exposed
`qbittorrent.homelab.otthol.hu`. That is wrong: the servarr chart renders
**workloads and Services only** (no Ingress at all), and qBittorrent is the
internal download client — Sonarr/Radarr/Prowlarr reach it over the in-cluster
Service, so a public route (and its certificate) is pure attack surface.

The route was removed with a `routes` array on `PATCH /apps/{name}` (the API
accepts it; `umkim app update` does not expose a `--routes` flag, so the patch
went in over the stored session):

```bash
curl -X PATCH -H 'Content-Type: application/json' -H "Cookie: fleet_session=…" \
  -d '{"routes":[{"key":"seerr",…},{"key":"jellyfin",…},{"key":"sonarr",…},{"key":"radarr",…}]}' \
  "$FLEET_API/apps/servarr"
```

The controller pruned `FleetAppRoute/servarr-qbittorrent` and the spoke
`fleet-route-servarr-qbittorrent-tls` Ingress with the upgrade; the host now
answers `404`. Sonarr and Radarr stay exposed because they are operator UIs
worth reaching remotely — behind Authelia either way.

Traefik keeps the already-issued qbittorrent certificate in `acme.json` (it is
no longer attached to any router, so nothing serves it); it expires with the
other 90-day certificates.

### nCore indexer

```
--- indexers configured in Prowlarr ---
  nCore  implementation=Cardigann enable=True priority=25
--- live search on nCore ---
  "indexer": "nCore", "title": "Approximatrix.Simply.Fortran.v4.0.4937.Linux..."
--- indexer status (failures) ---
  []
```

The bootstrap adds the indexer without testing the login, so a live search was
run to prove the credentials work: Prowlarr returned real nCore results and
reports no indexer failures.

### Authelia OIDC for immich and paperless

Both apps registered OIDC clients (which is what turns the cluster OIDC provider
on). The provider advertises itself and accepts both clients:

```
GET /api/oidc/authorize?client_id=immich&...      -> 302 to the Authelia login
GET /api/oidc/authorize?client_id=paperless&...   -> 302 to the Authelia login
```

The immich and paperless bootstraps logged `SSO configured` / `group sso-users
created` respectively.

### Directory login into an app
The directory bind is exercised end to end for Gitea (its bootstrap adds the
`lldap` auth source):

```
gitea: login POST -> 303
gitea: home page shows "labuser" + Dashboard
```

Nextcloud's LDAP is verified directly rather than through its web form: the
`svc-bind` Secret is installed in the namespace, `occ ldap:search` lists
`Lab E2E User (labuser)`, `occ user:info labuser` reports `backend: LDAP`, and a
PHP user bind inside the pod succeeds (and rejects a wrong password). The
Nextcloud *web* login POST could not be driven from curl — the app's own
generated `admin` account behaves identically (302 back to `/login`), so that is
a limitation of the curl form/CSRF harness, not of the LDAP wiring.

## Findings

| # | Finding | Action |
|---|---|---|
| 1 | Two `port_forward` rows claimed `172.16.10.100` + `*.homelab.otthol.hu` | removed the redundant `homelab-ha-pf` row; `site ingress verify` now `OK` |
| 2 | `umkim site ingress delete` reported `patch reported OK but ingress count mismatch: want 3 got 4` although the delete succeeded | **fixed** — the post-patch re-read now polls until the patched set is visible (`waitSiteIngress` in `cmd/umkim/internal/cmd/site_ingress.go`). Verified live: a scratch entry deletes with a clean `OK` and exit 0, and unit tests cover the stale read, a genuine mismatch, and a fetch error |
| 3 | The servarr bootstrap warned `Sonarr/Radarr has no Prowlarr-synced indexers` before its own verification passed | **fixed** — `ApplicationIndexerSync` is asynchronous, so the bootstrap now polls for the synced indexer (90s budget) instead of checking once after a fixed 5s (`bootstrap/src/api/prowlarr.ts`). Needs a bootstrap image rebuild to take effect |
| 4 | The servarr bootstrap verification needs a Prowlarr-synced indexer (lab-e2e-6 #6) | **closed** — with the operator nCore credential the whole bootstrap completes and verifies |
| 5 | `servarr-qbittorrent` sat `Queued — waiting for spoke ACME queue` for ~25 minutes while every other host was `Ready` | **fixed** — `advanceSpokeACMEQueue` probed port **443** while a `port_forward` spoke serves HTTPS on **30443**, so it could never observe a live certificate and could only advance by rotating on staleness. It now uses the cluster's registered Traefik HTTPS port, releases a host whose route is already `Ready`, and logs queue transitions under the `spoke-acme-queue` logger |
| 6 | The production ACME retry token was one-shot, so a host that failed one challenge stayed `Issuing` with nothing left to retry | **fixed** — the token is re-stamped while the certificate is not live, and after two refreshes the spoke Traefik is restarted once for that hostname (`spoke_acme_evict.go`), which is the retry Traefik honours when the router configuration itself has not changed |
| 7 | `qbittorrent.homelab.otthol.hu` failed its Let's Encrypt challenge with `acme: error: 400 … 84.1.215.254: Connection refused` | environmental — a WAN-side refusal during TLS-ALPN-01 validation (the spoke resolver uses `tlschallenge=true`, so the router must forward WAN **443**). It is what exercised findings 5 and 6; both are now fixed |
| 8 | The route list exposed `qbittorrent` publicly | **fixed on the live cluster** — qBittorrent is the internal download client; the chart creates no Ingress and the *arr apps reach it in-cluster. The route (and its certificate) was removed with a `routes` array on `PATCH /apps/{name}`. The `lab-e2e-6` record carries the same over-broad list; its finding 9 points here |
| 9 | Invites and user creation were hub-directory only, so a cluster's own LLDAP had no supported path | **implemented** — `umkim invite create --cluster` / `tenant add-user --cluster` target a cluster's directory; redemption runs a Job on the spoke beside LLDAP (reusing the image the LLDAP Deployment runs, no new exposure), and the portal `/invite` page is now deployed for every ingress-auth cluster, not just `directory: hub` ones. Verified live: an invite redeemed through the cluster's own sign-up page created the user in the spoke LLDAP only, and it signed in through the spoke Authelia |
| 10 | End-to-end app login for an invited user | **verified** — a user invited into the cluster's own LLDAP signed in through the spoke Authelia and completed Immich's OIDC login (authorize → Authelia → callback → Immich session), reaching `/api/users/me` as `immichuser@homelab.otthol.hu` via Immich's `autoRegister`. Two harness notes: Authelia's issuer is derived from the request Host, so the flow must run on the public 443 path (on the WireGuard NodePort the issuer carries `:30443`, which Immich rejects); and Immich's callback takes `{url, codeVerifier}` (the full callback URL plus the PKCE verifier), not `{url, code, state}` |

Fixes 2, 5 and 6 are deployed to the hub and the CLI is rebuilt; the
`fleet-edge` and `umkim` suites are green, including new tests for the settle
logic, the queue's issued-host release, and the retry refresh/escalation budget.

## Cleanup

```bash
umkim cluster delete lab-e2e-7 \
  --cleanup-apps --cleanup-storage --cleanup-vms --cleanup-vm-declarations --wait
umkim cluster delete lab-e2e-7            # second call must be a no-op
```

Leftovers to expect: the shared export base, base OS images, and per-guest
cloud-init/XML files.
