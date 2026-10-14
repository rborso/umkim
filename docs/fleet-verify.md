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

Then the full delete and its end state:

```bash
umkim cluster delete "$CLUSTER" \
  --cleanup-apps --cleanup-storage --cleanup-vms --cleanup-vm-declarations --wait
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

## Cluster reinstall presets

Cluster reinstall is an L2 operation. The built-in preset names are homelab-profile
checks (`profiles: [homelab]` in the check registry); use `--profile homelab`
when your inventory matches that profile. Preset names:

- `lab-sn01`
- `lab-3n01`
- `lab-ha01`

For example:

```bash
./scripts/verify-fleet.sh --layer 2 \
  --cluster-preset lab-sn01 --cluster-preset-only
```

Use `--cluster-spec` only with a reviewed flow file. A reinstall can delete
cluster state, so confirm the target and recovery plan before running it.

For **HA** presets (`lab-ha01`, `lab-3n01`), hub finalize Jobs allow up to
**90 minutes**. Default flow polling in [cluster-create.yml](../playbooks/flows/cluster-create.yml)
is 90 retries × 20s (30 minutes). Override `fleet_flow_poll_retries` (for example
`270` for 90 minutes) when driving HA reinstall flows.

Reports are written under `reports/verify-<layer>-<timestamp>/`. Protect
reports if they contain operational identifiers or failure output.

See [install.md](install.md) for the supported installation path and
[scripts/fleet-verify/README.md](../scripts/fleet-verify/README.md) for the
check registry and extension guidance.
