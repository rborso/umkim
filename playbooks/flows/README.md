# Fleet API flows

These playbooks automate selected FleetSite, FleetClusterJoin, and FleetApp
controller API workflows. They are orchestration helpers, not a replacement
for the supported installation path.

## Available flows

- [site-create.yml](site-create.yml) creates site records.
- [cluster-create.yml](cluster-create.yml) drives cluster creation and
  provisioning steps.
- [cluster-delete.yml](cluster-delete.yml) drives cluster removal. Default:
  **detach** from hub (preserves K3s and workloads). Opt-in full K3s reset:
  `fleet_flow_cleanup_kubernetes=true`.
- [cluster-node-op.yml](cluster-node-op.yml) runs post-Ready node operations
  (add/remove worker, replace control plane) with pre- and post-validation.
- [app-install.yml](app-install.yml) installs an OCI Helm chart onto a
  **selected Ready cluster** (`fleet_flow_cluster_name` is required).
- [whoami-helm-deploy.yml](whoami-helm-deploy.yml) installs the Harbor whoami
  chart via FleetApp.
- The remaining playbooks cover optional GPU preparation and other charts
  (Gitea, llama.cpp). Each chart flow requires the target cluster name.

There is no placement engine. Do not omit `fleet_flow_cluster_name`.

## Use safely

- Provide the active inventory, credentials, and endpoint configuration through
  approved local inputs.
- Treat the files in `examples/` as fixtures, not portable deployment
  definitions.
- Confirm authorization and resource ownership before create, provision,
  delete, or cleanup operations.

## References

- [Fleet apps](../../docs/components/fleet-apps.md)
- [Fleet site](../../docs/components/fleet-site.md)
- [Installation guide](../../docs/install.md)
- [Design guide](../../docs/design.md)
- [Site exposure examples](../../examples/site-profiles/)
