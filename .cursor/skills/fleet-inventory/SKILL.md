---
name: fleet-inventory
description: >-
  Manages Ansible inventory, group variables, host variables, and Ansible
  configuration. Use for inventory/ changes, connectivity inputs, or undefined
  variables on limited plays.
---

# Fleet inventory (S1)

## Contract

`inventory/` is the source of deployment-specific values. Keep it free of
documentation-only assumptions and never copy its live values into public-facing
documentation.

Ansible loads `inventory/group_vars/all/common.yml` for fleet-wide defaults.
Because `group_vars/all/` is a directory, a sibling `group_vars/all.yml` does
not provide the same default scope.

## Working guidance

- Keep host-specific values under `inventory/host_vars/` and group-scoped values
  under the corresponding `inventory/group_vars/` path.
- Resolve inventory groups, aliases, limits, and connection data from the active
  inventory; do not reuse them from an example.
- Use `ansible-inventory --graph` and a scoped variable inspection to diagnose
  precedence before adding fallback values.
- Update the applicable inventory template when the supported inventory
  contract changes.

## References

- [Hub-only inventory template](../../../examples/inventory/hub-only/)
- [Installation guide](../../../docs/install.md)
- [Design guide](../../../docs/design.md)
