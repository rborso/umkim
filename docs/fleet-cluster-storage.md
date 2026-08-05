# Fleet cluster storage

> Status: v0.1.0 is an experimental proof of concept. Validate storage,
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

Use the storage validation operation before apply. It checks NFS reachability,
export visibility where available, a client mount, and a provisioner-style
write probe. A successful probe is not a durability, availability, or backup
guarantee.

## Longhorn and local NFS

Longhorn is validated only for the HA creation path and may use either its
default data directory or a dedicated block device. Review disk selection and
failure-domain requirements before choosing it.

`nfs_local` is a convenience backend for single-node evaluation, not a
substitute for a managed shared storage service.

## Verify

Check the resulting `StorageClass`, provisioner rollout, and a representative
PVC using [fleet-verify.md](fleet-verify.md). See [install.md](install.md) for
the supported cluster-creation workflow.
