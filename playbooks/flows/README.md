# Fleet API flows

These playbooks automate selected FleetSite and FleetClusterJoin controller API
workflows. They are orchestration helpers, not a replacement for the supported
installation path, inventory validation, or access controls.

## Available flows

- [site-create.yml](site-create.yml) creates site records.
- [cluster-create.yml](cluster-create.yml) drives cluster creation and
  provisioning steps.
- [cluster-delete.yml](cluster-delete.yml) drives cluster removal.
- The remaining playbooks cover optional preparation and catalog-instance
  workflows.

The playbook source is authoritative for required variables, API calls, and
side effects. Review it and use `ansible-playbook --list-tasks` before running a
flow.

## Use safely

- Provide the active inventory, credentials, and endpoint configuration through
  approved local inputs.
- Treat the files in `examples/` as fixtures, not portable deployment
  definitions.
- Confirm authorization and resource ownership before create, provision,
  delete, or cleanup operations.
- A documentation-only change does not alter controller behavior or external
  infrastructure.

## References

- [Fleet site component](../../docs/components/fleet-site.md)
- [Installation guide](../../docs/install.md)
- [Design guide](../../docs/design.md)
