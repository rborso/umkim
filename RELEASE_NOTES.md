# UMKIM v0.1.0

v0.1.0 is an experimental reference implementation for a K3s, WireGuard, OCM,
DCM, Harbor, FleetClusterJoin, and fleet-edge stack.

## Included

- Hub installation and independent workload-cluster enrollment
- DCM catalog placement with k8s, Helm, and KubeVirt provider paths
- Console authentication/RBAC and optional application ingress
- Optional backup/restore and LLM integration paths

## Not a production guarantee

- No support, certification, or complete upgrade path
- Incomplete backup/restore, HA/failover, tenancy, and application lifecycle
  coverage
- Destructive install and teardown operations
- Operator-owned firewall, TLS, identity, secret, and exposure reviews

Use matching sibling release tags, verify their pins before image builds, and
read [SECURITY.md](SECURITY.md) before deployment.
