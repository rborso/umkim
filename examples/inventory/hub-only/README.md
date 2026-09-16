# Hub-only inventory template

Use this template for the supported greenfield management installation on
Rocky Linux 10. It contains non-operational placeholders and must be completed
locally before use.

## Start

From the repository root, run:

```bash
./scripts/bootstrap-hub-inventory.sh
```

The script copies this template into `inventory/`. Run it only when replacing
that inventory is intended, then follow the supported sequence in the
[installation guide](../../../docs/install.md).

## Review before installation

`bootstrap-hub-inventory.sh` copies these files; edit the listed ones before
install (see script output for the full list):

| File | What to set |
|------|-------------|
| `inventory/hosts.yml` | Hub host key, public IPv4 (`ansible_host`), SSH user |
| `inventory/host_vars/umkim-hub.yml` | `fleet_hub_public_ipv4` (same WAN IP) |
| `inventory/group_vars/all/site.yml` | `fleet_edge_domain`, DNS, LDAP base DN |
| `inventory/group_vars/all/fleet_sites.yml` | Management site seed |
| `inventory/group_vars/all/vault.yml` | Encrypted secrets (not copied by bootstrap) |

Also review `common.yml`, `hub/main.yml`, `fcj-workloads.yml`, and
`fleet_workloads.yml`. Greenfield `common.yml` sets
`fleet_fcj_default_ingress_auth_enabled: false`; enable ingress-auth in
`hub/main.yml` when Authelia is required. Optional Velero settings live in
`group_vars/all/fleet_backup.yml` (copy from this template if needed).

Validate inventory before running an installation playbook.

Use the variable definitions and comments in the copied files as the current
contract. Do not infer values from this README or other examples.

## Safety

- Do not commit live addresses, domains, credentials, or generated vault files.
- Keep deployment-specific overrides outside shared templates unless they are
  intended to become a documented configuration contract.
- This template does not create external infrastructure or attach workload
  clusters; use the installation and component documentation for those paths.
  After a cluster is Ready, install Helm charts with FleetApp (you must select
  the cluster).

## References

- [Installation guide](../../../docs/install.md)
- [Design guide](../../../docs/design.md)
- [FleetApp Helm installs](../../../docs/components/fleet-apps.md)
- [Secrets guide](../../../docs/secrets.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
