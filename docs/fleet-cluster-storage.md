# Fleet cluster storage

> Status: v0.2 is an experimental proof of concept. Validate storage,
> backup, and recovery behavior for every workload before production use.

`FleetClusterJoin` accepts storage intent during cluster creation. After a
cluster is `Ready`, the authenticated storage-configuration API supports the
NFS backends only. Changing an applied NFS server or cluster export path is not
supported; migrate workloads instead.

## Supported backends

| Backend | Intended use | Constraint |
|---|---|---|
| `local_path` | K3s local storage | Default local-only option |
| `nfs_external` | Shared external NFS | Requires an operator-provided NFS server and per-cluster path |
| `nfs_local` | Single-node evaluation | Restricted to a single-node cluster |
| `longhorn` | HA cluster storage at creation | Restricted to `ha_3_node` topology at create time |

The NFS provisioner creates a subdirectory for each PVC beneath the configured
cluster path. Provide the server, export base, cluster path, optional client
mount path, and storage-class name through the console or an approved,
authenticated client.

## Single-node local RWX (`nfs_local`)

Use this when a single-node cluster needs ReadWriteMany without an external NAS.
UMKIM installs `nfs-server` on the node, exports a local path, and deploys
StorageClass `nfs-client` via `nfs-subdir-external-provisioner`.

Defaults:

| Setting | Value |
|---|---|
| Export base | `/srv/fleet-nfs` |
| Cluster path | cluster name |
| StorageClass | `nfs-client` |
| Server | bootstrap / node address (auto) |
| Export ACL | node address only |

### At create (console)

On **Create cluster** with topology `single_node`, enable **Local RWX (NFS on this
node)**. The bootstrap bundle installs the NFS server and provisioner before the
cluster reaches Ready; status is marked Applied when local provision reports
Validated.

### After Ready (console / CLI)

On cluster detail → **Operations**, use **Enable local RWX** (share directory
defaults to `/srv/fleet-nfs`). External NFS remains under the Storage panel.

CLI (same cluster operation):

```bash
umkim cluster storage enable-local CLUSTER
umkim cluster storage enable-local CLUSTER --dir /data/nfs --wait 10m
umkim cluster storage status CLUSTER
umkim node-op history --cluster CLUSTER
```

API: `POST /clusters/{name}/configure-storage` (authenticated via
fleet-console-auth). `--dir` sets the absolute path exported by nfs-server on
the node (default `/srv/fleet-nfs`). The cluster share is
`<dir>/<cluster-path>` (cluster-path defaults to the cluster name). Storage
configure is mutually exclusive with node lifecycle and KubeVirt platform jobs
and is recorded in operation history as `configureStorage`.

`nfs_local` is a convenience backend for single-node evaluation, not a
substitute for a managed shared storage service.

## NFS safety requirements

UMKIM does not secure or operate an external NFS server. Before enabling
`nfs_external`:

1. Use a dedicated export and a unique directory for each cluster.
2. Restrict access to the known cluster clients; do not use wildcard exports.
3. Keep root squashing enabled unless a documented, reviewed storage exception
   is required.
4. Confirm network reachability, mount behavior, and that the provisioner can
   create a test subdirectory.
5. Protect delegate-host SSH access if using `delegateHost`; it can create the
   configured directory remotely.

Use the storage validation operation before apply for `nfs_external`. It checks
NFS reachability, export visibility where available, a client mount, and a
provisioner-style write probe. A successful probe is not a durability,
availability, or backup guarantee. Validation is not used for `nfs_local`
(the server is created by the configure/provision job).

## Longhorn

Longhorn is validated only for `ha_3_node` topology at create time and may use either its
default data directory or a dedicated block device. Review disk selection and
failure-domain requirements before choosing it. The bootstrap asserts the device
is at least `storage.minDataDiskGi` (default 5 GiB).

## Default StorageClass

The cluster has exactly one default class, and it is never `local-path` while
another backend is present:

| Backend | Default class |
|---|---|
| NFS (`nfs_external` / `nfs_local`) | `nfs-client` |
| Longhorn, when NFS is not configured (or not left as the default) | `longhorn` |
| `local_path` | `local-path` |

When NFS is configured it owns the default: the storage role marks `nfs-client`
default and clears the flag from every other class. Setting
`fleet_cluster_nfs_set_default_sc` false hands the default to Longhorn instead,
and Longhorn claims it on its own only when NFS does not. Charts should still set
an explicit `storageClassName` per volume so placement does not depend on the
default.

The fleet ships the `local-path` provisioner and StorageClass itself and disables
the K3s `local-storage` addon. The packaged addon re-applies
`storageclass.kubernetes.io/is-default-class: "true"` to `local-path` on every
server start, which would leave the cluster with two defaults — or override the
backend's choice — after any restart. The fleet's copy keeps that flag under the
backend's control, so a reinstall lands on one default and it stays there.

## Verify

Check the resulting `StorageClass`, provisioner rollout, and a representative
PVC using [fleet-verify.md](fleet-verify.md). See [install.md](install.md) for
the supported cluster-creation workflow.
