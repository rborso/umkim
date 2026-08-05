# Fleet ManifestWork sources

This directory intentionally contains no rendered ManifestWork files. Rendered
objects are build artifacts; the Jinja templates in their owning roles are the
source of truth.

| Concern | Template location |
|---------|-------------------|
| Platform bootstrap | `roles/olm/templates/manifestwork-bootstrap.yaml.j2` |
| Container provider | `roles/dcm/templates/manifestwork-k8s-sp.yaml.j2` |
| Spoke ingress | `roles/spoke_ingress/templates/manifestwork-ingress.yaml.j2` |

Change a template and its role inputs rather than adding a rendered artifact
here. Update the relevant component documentation when a ManifestWork contract
changes.

## References

- [DCM component](../../docs/components/dcm.md)
- [Fleet edge component](../../docs/components/fleet-edge.md)
- [Design guide](../../docs/design.md)
