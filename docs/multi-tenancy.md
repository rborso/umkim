# Organization multi-tenancy

> Status: v0.2 is an experimental proof of concept. Its organization model
> is not a substitute for independently validated tenant isolation.

Organizations scope console authorization, FleetSite and FleetClusterJoin
ownership, FleetApp visibility, and shared-pool quota checks. They do **not**
define site ingress or cluster network independence — see
[components/fleet-edge.md](components/fleet-edge.md) and
[examples/site-profiles/](../examples/site-profiles/).

## Model

| Actor | Intended responsibility |
|---|---|
| `dc-admin` | Platform administration, organizations, apps, and pool assignment |
| `org-admin` | One organization’s users, bindings, resources, and dedicated policies |
| `customer` | Organization-scoped site, cluster, and app operations |

For non-platform users, site and cluster names must use the organization
prefix. The fleet-site controller validates organization ownership on
mutating requests, and the auth proxy filters scoped list results.

## Labels and pools

| Label | Meaning |
|---|---|
| `fleet.org` | Owning organization |
| `fleet.pool` | Cluster pool: `shared` or `dedicated` |

Organization-owned cluster creation defaults non-platform users to a dedicated
pool. A platform administrator controls shared-pool assignment.

Shared quotas are preflight controls, not a capacity guarantee. FleetApp
installs always name a cluster; there is no placement engine.

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
