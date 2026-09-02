# Harbor registry

> Status: v0.2 is an experimental proof of concept. Registry availability,
> access control, transport security, and backup remain operator
> responsibilities.

Harbor is the hub registry for fleet-built images and chart artifacts. The
`fleet` project holds images built by the repository’s Ansible workflows;
The `charts` project holds OCI Helm charts consumed by FleetApp; the `mirror`
project holds images copied from a chart publish with `imagePolicy: copy`; proxy
projects are used for selected upstream content.

## Build and publish

Use the repository workflow rather than ad-hoc registry credentials or manual
pushes:

```bash
ansible-playbook harbor.yml -l hub --tags harbor_push_fleet
```

Use the corresponding chart-push tag when publishing the repository’s charts.
Operators can also publish into the hub library with
[scripts/fleet-chart.sh](../../scripts/fleet-chart.sh) or the console Apps
Library tab (`POST /api/v1alpha1/charts`). Fleet-built images use the release image tag configured by
`umkim_version`/`harbor_fleet_image_tag`.

v0.2 hub images are built from this checkout. Sibling service-provider
clones are not required. See [install.md](../install.md) and
[VERSIONS.md](../../VERSIONS.md).

## Security and recovery

- Restrict registry administration and image-push access to trusted operators.
- Keep registry credentials in the encrypted vault.
- Verify TLS, client trust, firewall exposure, and pull policy in the live
  deployment; this release does not claim production-grade registry hardening.
- Back up Harbor data separately from the hub helper backup.

See [secrets.md](../secrets.md), [backup-restore.md](../backup-restore.md),
and [fleet-apps.md](fleet-apps.md) for related operational guidance.
