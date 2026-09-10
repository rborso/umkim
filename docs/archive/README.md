# Archive

Historical runbooks and design notes are not shipped as supported
documentation. They may describe obsolete behavior and must not be used as an
installation, security, or recovery authority.

v0.1 used a DCM catalog and placement providers. v0.2 installs Helm charts
with FleetApp onto a cluster you select.

Use the current documents instead:

| Need | Current document |
|---|---|
| Installation | [install.md](../install.md) |
| Helm apps | [components/fleet-apps.md](../components/fleet-apps.md) |
| Verification | [fleet-verify.md](../fleet-verify.md) |
| Architecture | [design.md](../design.md) |
| FleetSite / provisioner | [components/fleet-site.md](../components/fleet-site.md) |
| Fleet CLI | [components/umkim-cli.md](../components/umkim-cli.md) |
| Backup / restore | [backup-restore.md](../backup-restore.md) |
| Security responsibilities | [SECURITY.md](../../SECURITY.md) |

Archived provisioner connector design (superseded by native YAML inventory):

| Archived | Replaced by |
|---|---|
| [design-provisioner-connector.md](design-provisioner-connector.md) | [components/fleet-site.md](../components/fleet-site.md) |
| [provisioner-connector-plan.md](provisioner-connector-plan.md) | [components/fleet-site.md](../components/fleet-site.md) |
| [plans/hub-backup-restore.md](plans/hub-backup-restore.md) | [backup-restore.md](../backup-restore.md) |
| [runbooks/hub-dr-test-20260902.md](runbooks/hub-dr-test-20260902.md) | [backup-restore.md](../backup-restore.md) (operator runbook) |

If an old link points into this directory, migrate the procedure to the
appropriate current document and validate it against the deployed version.
