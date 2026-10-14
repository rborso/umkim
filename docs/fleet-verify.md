# Fleet verification

> Status: v0.2 is an experimental proof of concept. Verification improves
> confidence but does not certify production readiness.

`scripts/verify-fleet.sh` runs the repository’s layered checks from the
repository root.

## Layers

| Layer | Purpose | Safety |
|---|---|---|
| `1` | Read-only audit and health checks | Safe to run after deployment |
| `2` | Synthetic create, verify, and cleanup checks | Changes test resources |
| `3` | Rebuild workflow followed by checks | Destructive; requires an approved maintenance window |

## Common commands

```bash
# Read-only hub audit
./scripts/verify-fleet.sh --layer 1 --suite hub

# Core synthetic checks
./scripts/verify-fleet.sh --layer 2 --suite core

# Full destructive regression
./scripts/verify-fleet.sh --layer 3 --suite regression
```

The currently registered suite names include `hub`, `core`, `edge`, `backup`,
`tenancy` (console auth and RBAC checks), `llm`, `apps`, `regression`, and
`all`. The `verify-fleet.sh --help` text may list legacy names (`nested`,
`catalog`); use `apps` for chart-library checks.

For hub LiteLLM status, use `scripts/verify-fleet-llm.sh` with fleet identity
headers (built in) or `UMKIM_TOKEN` when the Fleet API requires auth; it falls
back to `umkim llm get` when curl fails.

For a Helm app smoke test on a Ready cluster, use
`scripts/verify-fleet-app.sh` (set `FLEET_APP_CLUSTER`) or the console **Apps**
page. `scripts/verify-fleet-chart.sh` checks that the hub library lists seeded
charts. For llama-cpp profile install, PATCH upgrade, and LLM backend sync, use
`scripts/verify-fleet-llama-app-lifecycle.sh` (`FLEET_LLM_CLUSTER` on a GPU
Ready cluster). Profile CRUD, quick/extended verify, and hub-proxied chat:
`scripts/verify-fleet-llama-profile.sh` (check id `llm.profile-verify`). See
[components/fleet-apps.md](components/fleet-apps.md) and
[components/fleet-llama-profiles.md](components/fleet-llama-profiles.md).

For a site's public ingress, `umkim site ingress verify SITE` (or
`--id ENTRY`) is a read-only check of each entry: well-formed fields, a unique
wildcard, the cluster binding and its cardinality, the site-mode routes
published through it (host under the wildcard, DNS resolution, expected WAN
address in `fixed` mode), and whether the bound spoke Traefik answers over
WireGuard. The operator-managed WAN DNS record and router port-forward cannot be
reached from the hub and are reported as skipped, so a passing run is not proof
that a public URL works end to end.

## Cluster delete cleanup (from-zero e2e)

The delete cleanup stages (applications, storage export, Kubernetes reset, VM
destroy, inventory declarations) are only exercised by a full round trip:
provision a cluster from declared guests, give it real storage and apps, then
delete it with every flag and check the state. Layers 1–3 above do not cover it.

Preconditions: a YAIDAP-bound site with spare hypervisor capacity, an exported
NFS base reachable from the site network, and the chart library seeded.

```bash
SITE=…        # site id
CLUSTER=…     # fresh cluster name
NFS_SERVER=…  # storage host, reachable from the site (not from the hub)
NFS_BASE=…    # exported base directory

# 1. Declare three guests, spread across hypervisors
for i in 0 1 2; do
  umkim site host add "$SITE" --kind vm --name "$CLUSTER-cp$i" \
    --ip <mgmt-ip-$i> --hypervisor <hv-$i> --cores 4 --memory 6144 --disk 50
done

# 2. HA cluster with autoProvision and NFS storage.
#    Leave storage.nfs.clusterPath unset so the per-cluster default
#    (<cluster>-<uid8>) applies — that is what keeps a rebuild from inheriting
#    the previous cluster's volumes.
umkim cluster create -f cluster.json     # topology ha_3_node, autoProvision true,
                                         # nodes = the three guests, storage nfs_external
umkim cluster wait "$CLUSTER" --phase Ready

# 3. Cluster-local directory + forward auth (also creates the per-cluster secret)
umkim cluster ingress-auth enable "$CLUSTER" --lldap-password-file <file>

# 4. Apps (one native-LDAP chart, one OIDC chart, and one more for breadth)
umkim app create --name <app> --cluster "$CLUSTER" --chart <chart> --version <v> \
  --release <app> --namespace <app> --route-mode site --route-host <host> \
  --require-auth --service-name <svc> --service-port <port> --values-file <values>
```

**Assert the pre-state** — without it, "cleaned up" is unfalsifiable:

| Check | Expected |
|---|---|
| `virsh list` on each hypervisor | one guest domain per node, disk dirs with data |
| `umkim app list` | every app `Ready` |
| PVCs on the spoke | `Bound`, storage class from the NFS provisioner |
| `<NFS_BASE>/<cluster>-<uid8>` on the storage host | one directory per PVC, non-trivial size |
| `guests.yml` + `host_vars/` | the three guests present |
| hub objects | `fleet-cluster-<cluster>-{bootstrap,ansible-ssh}`, `fleet-ingress-auth-<cluster>`, `fleet-kubeconfig-<cluster>`, `fleet-cluster-<cluster>-inventory`, `fleet-cluster-discover-<cluster>`, `fa-vals-*`, per-app bind secrets, routes |

The commands behind those rows, run against the hub (cluster-scoped objects) and
the site (VMs, storage):

```bash
# VMs and their disks, per hypervisor
virsh list --all --name; du -sh /opt/local/<cluster>-cp*

# NFS export subdirectory and one directory per PersistentVolume
ls -1d "$NFS_BASE"/<cluster>-*; ls -1 "$NFS_BASE"/<cluster>-*; du -sh "$NFS_BASE"/<cluster>-*

# Apps, and the PVCs they bound on the spoke
umkim app list
k3s kubectl get pvc -A; k3s kubectl get sc

# Hub objects the delete must remove
kubectl -n <controller-ns> get secret | grep -E "<cluster>|lldap-bind"
kubectl -n <spoke-kubeconfig-ns> get secret | grep "$CLUSTER"
kubectl -n <controller-ns> get cm | grep -E "<cluster>|fa-vals"
kubectl -n <edge-routes-ns> get fleetapproute | grep "$CLUSTER"

# Inventory declarations
kubectl -n <controller-ns> get cm <site>-yaidap-inventory -o jsonpath='{.data.file\.guests\.yml}'
```

Then the full delete and its end state:

```bash
umkim cluster delete "$CLUSTER" \
  --cleanup-apps --cleanup-storage --cleanup-vms --cleanup-vm-declarations --wait

# Re-run the same checks: every one of them must now come back empty/gone.
# The second call must be a no-op:
umkim cluster delete "$CLUSTER"
```

| Check | Expected |
|---|---|
| `umkim cluster list` | cluster gone |
| `umkim app list` | empty |
| `umkim site host list "$SITE"` | only hypervisors and bare metal remain |
| `virsh list` on each hypervisor | no cluster domains |
| `<NFS_BASE>/<cluster>-*` | absent |
| hub secrets/CMs for the cluster | absent (`fleet-cluster-*`, `fleet-ingress-auth-*`, `fleet-kubeconfig-*`, `fa-vals-*`) |
| app routes | removed from the edge routes namespace |
| `umkim cluster delete "$CLUSTER"` again | 404 / no-op (idempotent) |

Expected leftovers, which are not defects: the shared export base itself, base
OS images on the hypervisors, per-guest cloud-init/XML files (~16 KB) that
YAIDAP's destroy leaves beside the removed disks, `.probe-*` markers from storage
validation, and the site's provision history and inventory revision log (audit
records).

One more cosmetic leftover: a stale `_meta/status.json` entry can remain for a
guest whose declaration was removed. The destroy action's completion is recorded
by the site reconciler, which can write the guest status just after the delete
removed it. It is invisible in `umkim site host list` (that reads `guests.yml`)
and is overwritten if the guest is declared again.

Cautions found by running this procedure:

- The bootstrap bundle is **cached in `fleet-cluster-<cluster>-bootstrap`**. A
  role or playbook change reaches a bootstrap only after the bundle is re-issued
  (return the join to `Pending`, or delete the secret and the job) — the runner
  image alone is not enough, and the storage role files ship inside the bundle.
- The embedded bundle tree under `controllers/fleet-site/internal/bundle/static`
  is a mirror of `roles/`; run `controllers/fleet-site/sync-bundle-static.sh`
  (checked by `scripts/check-embed-static.sh`) or the change never ships.
- A guest's recorded phase is a hint, not proof: the delete destroys every VM
  guest in the cluster regardless of phase, because a stale `Declared` would
  otherwise leave a running VM behind with its declaration already removed.

### Verified runs

Two full runs of this procedure on a YAIDAP site with a shared NFS export and
three apps (two native-LDAP charts and one OIDC chart), three-node HA cluster:

| Run | Outcome |
|---|---|
| 1 | Bring-up needed manual intervention: the storage step failed (export subdirectory never created on a remote NFS server) and the fix did not reach the bootstrap (cached bundle). Delete cleaned apps, storage, hub objects, routes and inventory, but left one VM running — the stage skipped a guest whose recorded phase said `Declared`. |
| 2 | Bring-up completed first attempt (storage `Applied` before `Provisioning`). Delete with all five flags removed all three VMs and their disks, the export directory, every hub object, the routes and the inventory declarations; the second `DELETE` was a no-op. |

Timings for run 2: guests to `Ready` ≈ 23 min (three `create_vm` actions, then
bootstrap and hub provision), ingress auth ≈ 2 min, three apps to `Ready` ≈ 8 min,
delete ≈ 6 min.

## Cluster reinstall presets

Cluster reinstall is an L2 operation. The built-in preset names are homelab-profile
labels (`profiles: [homelab]` in the check registry). This public tree does not
ship the extra-vars files those names used to load. Maintainer lab inventory is
not published. Start hub inventory from
[examples/inventory/hub-only](../examples/inventory/hub-only/) and replace
`CHANGE_ME`.

Preset names:

- `lab-sn01`
- `lab-3n01`
- `lab-ha01`

Pass a reviewed local extra-vars file with `--cluster-spec`:

```bash
./scripts/verify-fleet.sh --layer 2 \
  --cluster-preset lab-sn01 \
  --cluster-spec /path/to/local-flow.yml \
  --cluster-preset-only
```

A reinstall can delete cluster state, so confirm the target and recovery plan
before running it.

For **HA** presets (`lab-ha01`, `lab-3n01`), hub finalize Jobs allow up to
**90 minutes**. Default flow polling in [cluster-create.yml](../playbooks/flows/cluster-create.yml)
is 90 retries × 20s (30 minutes). Override `fleet_flow_poll_retries` (for example
`270` for 90 minutes) when driving HA reinstall flows.

Reports are written under `reports/verify-<layer>-<timestamp>/`. Protect
reports if they contain operational identifiers or failure output.

See [install.md](install.md) for the supported installation path and
[scripts/fleet-verify/README.md](../scripts/fleet-verify/README.md) for the
check registry and extension guidance.
