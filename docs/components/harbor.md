# Harbor registry

> Status: v0.1.0 is an experimental proof of concept. Registry availability,
> access control, transport security, and backup remain operator
> responsibilities.

Harbor is the hub registry for fleet-built images and chart artifacts. The
`fleet` project holds images built by the repository’s Ansible workflows;
proxy projects are used for selected upstream content.

## Build and publish

Use the repository workflow rather than ad-hoc registry credentials or manual
pushes:

```bash
ansible-playbook harbor.yml -l hub --tags harbor_push_fleet
```

Use the corresponding chart-push tag when publishing the repository’s charts.
Fleet-built images use the release image tag configured by
`umkim_version`/`harbor_fleet_image_tag`.

Some images require locally pinned sibling source repositories. Follow
[install.md](../install.md) and the version matrix before rebuilding them.

## Security and recovery

- Restrict registry administration and image-push access to trusted operators.
- Keep registry credentials in the encrypted vault.
- Verify TLS, client trust, firewall exposure, and pull policy in the live
  deployment; this release does not claim production-grade registry hardening.
- Back up Harbor data separately from the hub helper backup.

See [secrets.md](../secrets.md), [backup-restore.md](../backup-restore.md),
and [dcm.md](dcm.md) for related operational guidance.
