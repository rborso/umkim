# HA full-stack cluster runbook (umkim CLI) — `lab-e2e-6`

Greenfield three-node HA cluster on site **`homelab`**, driven by the **`umkim`
CLI**, exercising node VMs from the node spec, **NFS + Longhorn** storage,
cluster ingress auth (Authelia + LLDAP), per-cluster Let's Encrypt, and the whole
library app set behind forward-auth with its post-install bootstraps.

**Run log (2026-10-14/15):** `/tmp/opencode/e2e6/watch.log`, `watch2.log`,
`servarr-bootstrap.log` (working directory; not committed).

Generic procedure: [docs/fleet-verify.md](../../docs/fleet-verify.md) →
*HA cluster with node VMs, dual storage, ingress auth, ACME, and apps*.

## Summary

| Step | Result |
|------|--------|
| Cluster create + 3 VMs | created, one guest per hypervisor |
| First bootstrap | **failed** — Longhorn device assertion (20 GiB disk vs 40 GiB minimum) |
| Re-arm after `minDataDiskGi: 20` | bootstrap completed, hub provision `Ready` |
| Ingress auth (Authelia + LLDAP) | spoke stack live; **auto-enabled** by inventory defaults |
| ACME | Let's Encrypt certificates for every host |
| Apps (7) | all `Ready`; 12 routes, all `Authelia`-gated |
| Directory user | `labuser` created in the spoke LLDAP, SSO login verified |
| Storage split | data PVCs on `nfs-client`, servarr config PVCs on `longhorn` |

Timings (UTC): create 19:36 → `Ready` 20:08 (32 min, including the failed
bootstrap and the fix), spoke auth stack ≈ 20:12, apps `Ready` 20:15–22:34,
route/cert queue drained 22:58.

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
| virt01 `172.16.10.2` | 8 | 29 GiB | 453 G |
| virt02 `172.16.10.3` | 8 | 29 GiB | 106 G |
| virt04 `172.16.10.6` | 6 | 28 GiB | 450 G |

NFS on virt04 (`/srv/fleet-nfs`, export `172.16.10.0/24(rw,sync,no_subtree_check,no_root_squash)`),
base images `/opt/local/rocky-10.qcow2` + `rocky-9.qcow2`.

Ingress `homelab-pf` (`port_forward`, address `172.16.10.100`, wildcard
`*.homelab.otthol.hu`, zone `homelab.otthol.hu`) — free, so the cluster binds it.

Targets: nodes `172.16.10.101-103`, VIP `172.16.10.100` (the port-forward
target), both inside the lab DHCP pool (`172.16.10.100-199`) — the lab's usual
practice for these test guests.

## Phase 1 — Create

`cluster-create.json` (`examples/homelab/cluster-lab-e2e-6.json` shape):

```json
{
  "clusterName": "lab-e2e-6",
  "siteId": "homelab",
  "bootstrapHost": "172.16.10.101",
  "topology": "ha_3_node",
  "controlPlaneVip": "172.16.10.100",
  "autoProvision": true,
  "minRootDiskGi": 40,
  "edge": { "ingressId": "homelab-pf" },
  "nodes": [
    { "hostname": "lab-e2e-6-1", "address": "172.16.10.101", "role": "control",
      "vm": { "cores": 6, "memory": 16384, "diskGi": 50, "dataDiskGi": 20 } },
    { "hostname": "lab-e2e-6-2", "address": "172.16.10.102", "role": "control",
      "vm": { "cores": 6, "memory": 16384, "diskGi": 50, "dataDiskGi": 20 } },
    { "hostname": "lab-e2e-6-3", "address": "172.16.10.103", "role": "control",
      "vm": { "cores": 6, "memory": 16384, "diskGi": 50, "dataDiskGi": 20 } }
  ],
  "storage": {
    "backend": "nfs_external",
    "minDataDiskGi": 20,
    "nfs": {
      "enabled": true,
      "server": "172.16.10.6",
      "exportBase": "/srv/fleet-nfs",
      "clusterPath": "lab-e2e-6",
      "storageClass": "nfs-client",
      "delegateHost": "virt04"
    },
    "longhorn": { "enabled": true, "device": "/dev/vdb", "storageClass": "longhorn" }
  }
}
```

```bash
umkim cluster create -f cluster-create.json
```

The pipeline then runs `declare_vm:<host>` → `create_vm:<host>` per node.
`least-utilized` spread the guests one per hypervisor:

| Guest | Hypervisor | vCPU | RAM | vda | vdb |
|---|---|---|---|---|---|
| `lab-e2e-6-1` | virt04 | 6 | 16 GiB | 50 GiB | 20 GiB |
| `lab-e2e-6-2` | virt01 | 6 | 16 GiB | 50 GiB | 20 GiB |
| `lab-e2e-6-3` | virt02 | 6 | 16 GiB | 50 GiB | 20 GiB |

### First bootstrap failed: Longhorn device too small

```
TASK [fleet_site_validate : Fleet cluster validate | Require Longhorn device capacity]
fatal: [lab-e2e-6-1]: FAILED! => {
  "msg": "Longhorn device too small on lab-e2e-6-1 (need 40 GiB)"
}
```

The create body asked for a 20 GiB data disk but left `storage.minDataDiskGi`
unset, so the inventory kept the role default `fleet_cluster_min_data_disk_gi: 40`.
**Fix:** set `minDataDiskGi: 20` to match the disk. Recover without rebuilding
the guests:

```bash
# 1. make the spec consistent
kubectl -n fleet-console patch fleetclusterjoin lab-e2e-6 \
  --type merge -p '{"spec":{"storage":{"minDataDiskGi":20}}}'

# 2. clear the retry latch and re-arm the pipeline (re-issues the cached bundle,
#    which is what carries the new inventory value to the node)
kubectl -n fleet-console patch fleetclusterjoin lab-e2e-6 \
  --type merge -p '{"metadata":{"annotations":{"fleet.umkim.com/pipeline-retries":"0"}}}'
kubectl -n fleet-console patch fleetclusterjoin lab-e2e-6 --subresource=status \
  --type merge -p '{"status":{"phase":"Pending","pipelineStep":"bootstrap"}}'
```

`/tmp/opencode/e2e6/bundle/inventory.ini` then carries
`fleet_cluster_min_data_disk_gi=20`, and the re-run passed. The re-issue reuses
the node WireGuard identities (`nodes-wg.json` in the bundle Secret).

## Phase 2 — Ingress auth, ACME

Both were **auto-enabled** during provisioning — the controller defaults are on
(`FLEET_FCJ_DEFAULT_INGRESS_AUTH_ENABLED=true`, `FLEET_FCJ_DEFAULT_ACME_ENABLED=true`)
and the cluster is bound to a `port_forward` entry, so no explicit `enable` was
needed. The LLDAP admin password comes from
`FLEET_INGRESS_AUTH_BOOTSTRAP_PASSWORD`.

The hub Secret `fleet-ingress-auth-lab-e2e-6` appeared at the end of bring-up,
but the spoke stack lagged: `umkim ingress-auth get` listed the cluster `Ready`
while the spoke had no `fleet-ingress-auth` namespace. Re-triggering the
reconcile produced the `fleet-ingress-auth` ManifestWork and the spoke
`lldap` + `authelia` deployments:

```bash
kubectl -n fleet-console patch fleetingressauth fleet-ingress-auth --type merge \
  -p '{"metadata":{"annotations":{"fleet.umkim.com/ingress-auth-reconcile":"<now>"}}}'
```

Spoke Authelia (`auth.homelab.otthol.hu`): `default_policy: deny`, bypass for the
portal, `one_factor` for `*.homelab.otthol.hu`, LDAP bind to
`lldap.fleet-ingress-auth.svc.cluster.local:3890`, base DN `dc=otthol,dc=local`.

## Phase 3 — Directory user

`umkim tenant add-user` targets the shared **hub** directory, so a spoke-local
directory user is created directly in the cluster's LLDAP. Run inside the LLDAP
pod (login → `createUser` → `lldap_set_password`):

```bash
POD=$(kubectl -n fleet-ingress-auth get pods -l app=lldap -o name | head -1 | cut -d/ -f2)
kubectl -n fleet-ingress-auth exec -i "$POD" -- \
  env ADMIN_PW="$LLDAP_ADMIN_PW" USER_PW="$USER_PW" sh -c '
    URL=http://localhost:17170
    token=$(curl -fsS -X POST "$URL/auth/simple/login" -H "Content-Type: application/json" \
      -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" | sed -n "s/.*\"token\":\"\([^\"]*\)\".*/\1/p")
    curl -fsS -X POST "$URL/api/graphql" -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      --data "{\"query\":\"mutation(\$u:CreateUserInput!){createUser(user:\$u){id}}\",\"variables\":{\"u\":{\"id\":\"labuser\",\"displayName\":\"Lab E2E User\",\"email\":\"labuser@homelab.otthol.hu\"}}}"
    /app/lldap_set_password --base-url "$URL" --admin-username admin \
      --admin-password "$ADMIN_PW" --username labuser --password "$USER_PW"'
```

Directory contents afterwards: `admin`, `svc-bind`, `labuser`.

## Phase 4 — Apps

Storage split per chart values (data → NFS, small config → Longhorn):

| App | Chart | Route host | Data PVCs → `nfs-client` | Config PVCs → `longhorn` |
|---|---|---|---|---|
| whoami | `whoami` 0.1.0 | `whoami` | — | — |
| petclinic | `pet-clinic-helm` 0.1.1 | `petclinic` | postgres 2Gi | — |
| gitea | `gitea` 0.1.9 | `gitea` | data 10Gi, postgres 5Gi | — |
| immich | `immich` 0.1.3 | `immich` | data 10Gi, postgres 10Gi | — |
| nextcloud | `nextcloud` 0.1.4 | `nextcloud`, `collabora` | data 20Gi, postgres 5Gi | — |
| paperless | `paperless` 0.1.5 | `paperless` | data 5Gi, media 20Gi, consume 2Gi, export 2Gi, postgres 5Gi | — |
| servarr | `servarr` 0.2.16 | `servarr`(seerr), `jellyfin`, `sonarr`, `radarr`, `qbittorrent` | media 50Gi **RWX** | 6 config PVCs, 11Gi |

`umkim app create -f <app>.json --values-file <app>.yaml`; the JSON carries the
route(s) and any `oidcClient`, the YAML the chart values.

Immich + Paperless register Authelia OIDC clients, which switches the **cluster**
Authelia OIDC provider on (clients `immich`, `paperless`).

Verified PVC placement (`kubectl -n <ns> get pvc`):

```
gitea/immich/nextcloud/paperless/petclinic   nfs-client
servarr  media                               nfs-client (RWX)
servarr  flaresolverr|jellyfin|qbittorrent|radarr|seerr|sonarr-config   longhorn
```

Longhorn: 6 volumes `attached`/`healthy`. NFS: `/srv/fleet-nfs/lab-e2e-6/` with
13 subdirectories (one per NFS PVC), 1.2 G.

Bootstrap hooks: `gitea`, `immich`, `nextcloud`, `paperless` jobs `Complete`,
each log ending `==> bootstrap complete`.

### servarr: three defects to get a clean run

1. **qBittorrent password hook deadlock.** The post-install hook that writes the
   qBittorrent WebUI password mounts `qbittorrent-config` (`ReadWriteOnce`). The
   running `qbittorrent` pod held it on another node, so the hook pod never
   started → `DeadlineExceeded`, Helm release failed. Fix: make the config PVCs
   RWX (`storage.accessMode: ReadWriteMany`; Longhorn serves RWX via its
   share-manager) and reinstall — the hook then completes in 16 s.
2. **Lidarr.** The bootstrap image defaults every component on, including
   `lidarr`, which chart 0.2.16 does not ship → the job timed out waiting for a
   Service that never appears. Fix: set `BOOTSTRAP_ENABLE_LIDARR=false` in the
   `servarr-bootstrap` Secret (the generated ConfigMap does not define that key,
   so `envFrom` does not overwrite it).
3. **Indexer verification.** The bootstrap configured qBittorrent, Sonarr,
   Radarr, Jellyfin (libraries) and Seerr, then failed at
   `Verification failed: Sonarr has no Prowlarr-synced indexers`. That check
   requires Prowlarr with an indexer, and Prowlarr's bootstrap step requires the
   operator's private tracker credentials (`NCORE_USERNAME`/`NCORE_PASSWORD`),
   which this lab does not hold. Outcome: the bootstrap is left
   `enabled: false` so the release reaches `Ready` with the configuration the
   earlier run already wrote; add the credential and re-enable to complete it.

Stuck-release recovery after a failed hook (needed twice):

```bash
kubectl -n servarr delete job servarr-bootstrap --ignore-not-found
# mark the pending release failed, then re-run the update with a changed value
```

## Phase 5 — Verification

Routes, unauthenticated (from the hub over WireGuard, `Host:` header against a
node's NodePort):

```
whoami petclinic gitea immich nextcloud collabora paperless
servarr jellyfin sonarr radarr qbittorrent           → 302 https://auth.homelab.otthol.hu/?rd=…
auth                                                  → 200 (portal)
```

As `labuser` (Authelia `POST /api/firstfactor`, session cookie scoped to
`.homelab.otthol.hu`): `whoami`, `petclinic`, `gitea`, `immich`, `qbittorrent`,
`collabora` → `200`; `nextcloud` → `/login`, `paperless` →
`/accounts/login/`, `jellyfin` → `/web/`, `seerr` → `/login` (app-level).

TLS: `openssl s_client -servername <host>` on each host shows an issuer of
`C=US, O=Let's Encrypt`, not the Traefik default.

K3s: three nodes `Ready` (`v1.36.5+k3s1`), kube-vip held `172.16.10.100` on
`lab-e2e-6-2`, StorageClasses `longhorn` + `nfs-client` + `local-path`.

## Open findings

| # | Finding | Impact |
|---|---|---|
| 1 | `vm.dataDiskGi` below `storage.minDataDiskGi` (default 40) fails the bootstrap; the `else 10 GiB` derivation is itself below that default | create-time footgun; CLI `--control-data-disk` does not set `minDataDiskGi` |
| 2 | NFS + Longhorn together mark **two** StorageClasses default | a PVC without an explicit class binds unpredictably |
| 3 | Cluster ingress-auth status `Ready` before the spoke stack is deployed | false-positive readiness |
| 4 | A hook mounting a `ReadWriteOnce` config PVC deadlocks on a multi-node cluster | servarr install fails until the volume is RWX |
| 5 | Bootstrap image enables `lidarr` by default; chart ships none | servarr bootstrap times out |
| 6 | servarr bootstrap verification requires a Prowlarr-synced indexer | needs the operator's private indexer credential |
| 7 | Collabora WOPI discovery from the Nextcloud pod uses the public host | refused (RFC1918 → public NAT loopback) and behind forward-auth |
| 8 | A failed hook leaves the release `pending-upgrade`; `app update` no-ops on an unchanged spec | manual release-state recovery |

## Cleanup

```bash
umkim cluster delete lab-e2e-6 \
  --cleanup-apps --cleanup-storage --cleanup-vms --cleanup-vm-declarations --wait
umkim cluster delete lab-e2e-6            # second call must be a no-op
```

Leftovers to expect: the shared export base, base OS images, per-guest
cloud-init/XML files, and the stale guest directories from earlier runs
(`e2e-cp1`, `lab-e2e-2-2`, `test-cp1`, `test-vm` on virt01).
