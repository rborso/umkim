# Image build assets

This directory contains local image build contexts and staging helpers. The
complete image list, source locations, registry target, and tags are configured
by the Harbor role rather than this README.

## Workflow

- Review the relevant `Containerfile` or `Dockerfile` and any
  `build-context.sh` script before building.
- `roles/harbor/defaults/main.yml` declares configured fleet image inputs.
- `roles/harbor/tasks/push_fleet_image.yml` implements staging, build, and push
  behavior for `harbor_push_fleet`.
- Use authorized inventory values for registry access and source references.

Build contexts can include generated or staged files. This documentation does
not guarantee that a local context is complete or suitable for a remote
registry without the Harbor workflow.

## References

- [Harbor component](../docs/components/harbor.md)
- [Version reference](../VERSIONS.md)
- [Security reference](../SECURITY.md)
