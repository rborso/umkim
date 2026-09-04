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
| `longhorn` | HA cluster storage at creation | Restricted to the supported HA creation path |

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
umkim cluster storage enable-local lab-single-0
umkim cluster storage enable-local lab-single-0 --dir /data/nfs --wait 10m
umkim cluster storage status lab-single-0
umkim node-op history --cluster lab-single-0
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

Longhorn is validated only for the HA creation path and may use either its
default data directory or a dedicated block device. Review disk selection and
failure-domain requirements before choosing it.

## Verify

Check the resulting `StorageClass`, provisioner rollout, and a representative
PVC using [fleet-verify.md](fleet-verify.md). See [install.md](install.md) for
the supported cluster-creation workflow.
