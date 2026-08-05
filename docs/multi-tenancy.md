# Organization multi-tenancy

> Status: v0.1.0 is an experimental proof of concept. Its organization model
> is not a substitute for independently validated tenant isolation.

Organizations scope console authorization, FleetSite and FleetClusterJoin
ownership, catalog-instance visibility, and shared-pool quota checks.

## Model

| Actor | Intended responsibility |
|---|---|
| `dc-admin` | Platform administration, organizations, catalog, and pool assignment |
| `org-admin` | One organization’s users, bindings, resources, and dedicated policies |
| `customer` | Organization-scoped site, cluster, and instance operations |

For non-platform users, site and cluster names must use the organization
prefix. The fleet-site controller validates organization ownership on
mutating requests, and the auth proxy filters scoped list results.

## Labels and pools

| Label | Meaning |
|---|---|
| `fleet.org` | Owning organization |
| `fleet.pool` | Cluster pool: `shared` or `dedicated` |
| `fleet.placement/pool` | Requested workload pool |

Organization-owned cluster creation defaults non-platform users to a dedicated
pool. A platform administrator controls shared-pool assignment.

Shared quotas apply to catalog-instance requests in the shared pool. Dedicated
requests do not consume that shared quota. Quotas are preflight controls, not a
capacity guarantee; validate placement and workload limits independently.

## Policy boundaries

Organization administrators can manage only policies that are prefixed for
their organization and target that organization’s dedicated clusters. They
cannot create shared-pool or cross-organization policy rules.

## Security boundary

All public management API access must pass through authenticated
`fleet-console-auth` access. The organization model does not by itself provide
network, storage, registry, or Kubernetes namespace isolation. Design and
verify those controls separately before onboarding untrusted tenants.

See [fleet-console-auth.md](fleet-console-auth.md) for roles and
[fleet-verify.md](fleet-verify.md) for the tenancy verification suite.
